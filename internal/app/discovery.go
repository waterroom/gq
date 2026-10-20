package app

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// announceMsg is the UDP discovery payload. Deliberately tiny: it is sent
// every 1.5s and must never be mistaken for a control channel.
type announceMsg struct {
	T    string `json:"t"` // "hi" or "bye"
	ID   string `json:"id"`
	Name string `json:"name"`
	Port int    `json:"port"`
	OS   string `json:"os"`
}

// infoResp is what GET /api/info returns; used both by the LAN sweep and by
// a human poking the API with curl.
type infoResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Port int    `json:"port"`
	OS   string `json:"os"`
}

func (a *App) startUDP() {
	conn, err := listenMulticast(DefPort)
	if err != nil {
		log.Printf("udp multicast join failed (discovery falls back to LAN sweep): %v", err)
		return
	}
	_ = conn.SetReadBuffer(64 * 1024)
	a.udp = conn

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		buf := make([]byte, 2048)
		for {
			n, src, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			var m announceMsg
			a.cfgMu.RLock()
			selfID := a.cfg.ID
			a.cfgMu.RUnlock()
			if json.Unmarshal(buf[:n], &m) != nil || m.ID == "" || m.ID == selfID {
				continue
			}
			if m.T == "bye" {
				a.mu.Lock()
				delete(a.peers, m.ID)
				a.mu.Unlock()
				continue
			}
			a.touchPeer(&Peer{
				ID:   m.ID,
				Name: m.Name,
				Addr: src.IP.String(),
				Port: m.Port,
				OS:   m.OS,
			})
		}
	}()
}

func (a *App) announceLoop() {
	defer a.wg.Done()
	if a.udp == nil {
		return
	}
	dst, err := net.ResolveUDPAddr("udp4", MulticastIP+":"+itoa(DefPort))
	if err != nil {
		return
	}
	// Rebuild per tick: the payload used to be built once, so a renamed
	// device kept advertising the old name until the process restarted.
	tk := time.NewTicker(announceEvery)
	defer tk.Stop()
	for {
		select {
		case <-tk.C:
			a.cfgMu.RLock()
			payload := mustJSON(announceMsg{T: "hi", ID: a.cfg.ID, Name: a.cfg.Name, Port: a.port, OS: "windows"})
			a.cfgMu.RUnlock()
			if _, err := a.udp.WriteToUDP(payload, dst); err != nil {
				log.Printf("announce: %v", err)
			}
		case <-a.closed:
			return
		}
	}
}

func (a *App) sendBye() {
	if a.udp == nil {
		return
	}
	dst, err := net.ResolveUDPAddr("udp4", MulticastIP+":"+itoa(DefPort))
	if err != nil {
		return
	}
	a.cfgMu.RLock()
	b := mustJSON(announceMsg{T: "bye", ID: a.cfg.ID, Name: a.cfg.Name, Port: a.port, OS: "windows"})
	a.cfgMu.RUnlock()
	_, _ = a.udp.WriteToUDP(b, dst)
}

// scanLAN probes every /24 the host is attached to. Multicast is regularly
// blocked by AP isolation, so this is not a fallback in name only - on many
// corporate networks it is the only discovery path that works.
func (a *App) scanLAN() {
	ips := localIPv4s()
	if len(ips) == 0 {
		return
	}
	client := &http.Client{
		Timeout:   300 * time.Millisecond,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	seen := make(map[string]bool)
	sem := make(chan struct{}, 128)
	var wg sync.WaitGroup

	for _, ip := range ips {
		base := ip.Mask(net.CIDRMask(24, 32)).To4()
		if base == nil {
			continue
		}
		for i := 1; i <= 254; i++ {
			t := make(net.IP, 4)
			copy(t, base)
			t[3] = byte(i)
			if t.Equal(ip) {
				continue
			}
			k := t.String()
			if seen[k] {
				continue
			}
			seen[k] = true
			wg.Add(1)
			sem <- struct{}{}
			go func(x net.IP) {
				defer wg.Done()
				defer func() { <-sem }()
				a.probe(client, x)
			}(t)
		}
	}
	wg.Wait()
}

func (a *App) probe(c *http.Client, ip net.IP) {
	url := "http://" + ip.String() + ":" + itoa(DefPort) + "/api/info"
	resp, err := c.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var info infoResp
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&info) != nil {
		return
	}
	a.cfgMu.RLock()
	selfID := a.cfg.ID
	a.cfgMu.RUnlock()
	if info.ID == "" || info.ID == selfID {
		return
	}
	a.touchPeer(&Peer{
		ID:   info.ID,
		Name: info.Name,
		Addr: ip.String(),
		Port: DefPort,
		OS:   info.OS,
	})
}

func localIPv4s() []net.IP {
	var out []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	uniq := make(map[string]bool)
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualIface(strings.ToLower(ifi.Name)) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, ad := range addrs {
			ipn, ok := ad.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipn.IP.To4()
			if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() {
				continue
			}
			s := v4.String()
			if uniq[s] {
				continue
			}
			uniq[s] = true
			out = append(out, v4)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// isVirtualIface keeps VPN/VM adapters out of the sweep. Probing a WSL or
// VMware subnet wastes 254 dials and can make discovery feel slow.
func isVirtualIface(name string) bool {
	bad := []string{
		"vmware", "virtualbox", "vethernet", "hyper-v", "hyperv",
		"tap-", "tun", "wsl", "docker", "loopback", "bluetooth",
		"npcap", "zerotier", "tailscale", "radmin", "hamachi",
	}
	for _, b := range bad {
		if strings.Contains(name, b) {
			return true
		}
	}
	return false
}
