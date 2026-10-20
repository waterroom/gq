package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"lantransfer/internal/config"
)

const (
	// DefPort is tried first; the next few ports are fallbacks so a second
	// instance on the same machine still works.
	DefPort   = 45800
	PortTries = 12

	MulticastIP   = "239.255.72.89"
	announceEvery = 1500 * time.Millisecond
	peerTTL       = 7 * time.Second
	offerTimeout  = 60 * time.Second
	// maxPendingOffers caps the queue so a broken or hostile peer cannot
	// grow it without bound. Real office use never exceeds a handful.
	maxPendingOffers = 8
	// sessionIdleTimeout is refreshed when each file starts uploading, so it
	// only reaps sessions that stalled before sending any data.
	sessionIdleTimeout = 5 * time.Minute
	scanInterval  = 60 * time.Second
)

// Peer is a remote instance discovered on the LAN.
type Peer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
	OS   string `json:"os"`
	Last int64  `json:"last"`
}

// FileMeta is the metadata negotiated before a transfer starts.
type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Transfer is one send or receive session, shown in the UI.
type Transfer struct {
	ID      string     `json:"id"`
	Dir     string     `json:"dir"`  // "send" or "recv"
	Peer    string     `json:"peer"` // display name of the other side
	Files   []FileMeta `json:"files"`
	Done    int64      `json:"done"`
	Total   int64      `json:"total"`
	Cur     int        `json:"cur"`
	State   string     `json:"state"` // waiting | active | done | error | canceled
	Err     string     `json:"err,omitempty"`
	Speed   int64      `json:"speed"`
	Started int64      `json:"started"`

	lastT time.Time
	lastB int64
}

// PendingOffer is an inbound request waiting for the user to accept or reject.
type PendingOffer struct {
	ID     string     `json:"id"`
	Sender string     `json:"sender"`
	Addr   string     `json:"addr"`
	Files  []FileMeta `json:"files"`
	Total  int64      `json:"total"`

	ch   chan bool
	once sync.Once
}

func (o *PendingOffer) resolve(ok bool) {
	o.once.Do(func() { o.ch <- ok })
}

// Session tracks an accepted inbound transfer.
type Session struct {
	ID      string
	Dir     string
	Files   []FileMeta
	Names   []string
	TransID string
	Created time.Time
	// Deadline guards against a sender that negotiates and then vanishes:
	// without it the session (and its "active 0/N" row) would live forever.
	Deadline int64
}

// App owns all runtime state.
type App struct {
	mu   sync.Mutex
	cfg  *config.Config
	uiFS fs.FS

	peers    map[string]*Peer
	trans    map[string]*Transfer
	transOrd []string

	offer    []*PendingOffer
	sessions map[string]*Session

	ln   net.Listener
	srv  *http.Server
	udp  *net.UDPConn
	port int

	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func New(uiFS fs.FS) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:      cfg,
		uiFS:     uiFS,
		peers:    make(map[string]*Peer),
		trans:    make(map[string]*Transfer),
		sessions: make(map[string]*Session),
		closed:   make(chan struct{}),
	}, nil
}

// Start binds the TCP API and UDP discovery sockets and returns the UI URL.
func (a *App) Start() (string, error) {
	first := DefPort
	if v := os.Getenv("LT_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < 65536 {
			first = n
		}
	}
	ln, port, err := listenAny(first, PortTries)
	if err != nil {
		return "", err
	}
	a.ln = ln
	a.port = port
	a.srv = &http.Server{Handler: a.routes()}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("http serve: %v", err)
		}
	}()

	a.startUDP()
	a.wg.Add(1)
	go a.announceLoop()
	a.wg.Add(1)
	go a.janitorLoop()

	// First sweep is delayed so the UI can paint before 250 dials start.
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		select {
		case <-time.After(1200 * time.Millisecond):
		case <-a.closed:
			return
		}
		a.scanLAN()
		for {
			select {
			case <-time.After(scanInterval):
				a.scanLAN()
			case <-a.closed:
				return
			}
		}
	}()

	return "http://127.0.0.1:" + itoa(port) + "/", nil
}

func (a *App) Close() {
	a.closeOnce.Do(func() {
		close(a.closed)
		a.sendBye()
		if a.udp != nil {
			a.udp.Close()
		}
		if a.srv != nil {
			a.srv.Close()
		}
		if a.ln != nil {
			a.ln.Close()
		}
		a.mu.Lock()
		for _, o := range a.offer {
			o.resolve(false)
		}
		a.offer = nil
		a.mu.Unlock()
		a.wg.Wait()
	})
}

func listenAny(first, tries int) (net.Listener, int, error) {
	var lastErr error
	for i := 0; i < tries; i++ {
		p := first + i
		ln, err := net.Listen("tcp4", "0.0.0.0:"+itoa(p))
		if err == nil {
			return ln, p, nil
		}
		lastErr = err
	}
	return nil, 0, lastErr
}

func (a *App) newTransfer(dir, peer string, files []FileMeta) *Transfer {
	t := &Transfer{
		ID:      newID(),
		Dir:     dir,
		Peer:    peer,
		Files:   files,
		State:   "waiting",
		Started: time.Now().UnixMilli(),
		lastT:   time.Now(),
	}
	for _, f := range files {
		t.Total += f.Size
	}
	a.mu.Lock()
	a.trans[t.ID] = t
	a.transOrd = append(a.transOrd, t.ID)
	// Keep the list bounded so a long session does not grow forever.
	if len(a.transOrd) > 60 {
		drop := a.transOrd[0]
		a.transOrd = a.transOrd[1:]
		delete(a.trans, drop)
	}
	a.mu.Unlock()
	return t
}

func (a *App) addProgress(t *Transfer, n int64) {
	if n <= 0 {
		return
	}
	a.mu.Lock()
	t.Done += n
	now := time.Now()
	if dt := now.Sub(t.lastT).Seconds(); dt >= 0.25 {
		inst := float64(t.Done-t.lastB) / dt
		if t.Speed == 0 {
			t.Speed = int64(inst)
		} else {
			t.Speed = int64(0.65*float64(t.Speed) + 0.35*inst)
		}
		t.lastT = now
		t.lastB = t.Done
	}
	a.mu.Unlock()
}

func (a *App) finishTransfer(t *Transfer, state, errMsg string) {
	a.mu.Lock()
	t.State = state
	t.Err = errMsg
	if state == "done" {
		t.Done = t.Total
	}
	t.Speed = 0
	a.mu.Unlock()
}

func (a *App) touchPeer(p *Peer) {
	a.mu.Lock()
	cur, ok := a.peers[p.ID]
	if ok {
		cur.Name = p.Name
		cur.Addr = p.Addr
		cur.Port = p.Port
		cur.OS = p.OS
		cur.Last = time.Now().UnixMilli()
	} else {
		p.Last = time.Now().UnixMilli()
		a.peers[p.ID] = p
		log.Printf("peer up: %s (%s:%d)", p.Name, p.Addr, p.Port)
	}
	a.mu.Unlock()
}

func (a *App) janitorLoop() {
	defer a.wg.Done()
	tk := time.NewTicker(2 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-tk.C:
			now := time.Now().UnixMilli()
			a.mu.Lock()
			for id, p := range a.peers {
				if now-p.Last > peerTTL.Milliseconds() {
					delete(a.peers, id)
					log.Printf("peer down: %s", p.Name)
				}
			}
			for id, s := range a.sessions {
				if s.Deadline > 0 && now > s.Deadline {
					delete(a.sessions, id)
					if t := a.trans[s.TransID]; t != nil && t.State == "active" {
						t.State = "canceled"
						t.Err = "对方中断，未开始传输"
						t.Speed = 0
					}
					log.Printf("session expired: %s", s.Dir)
				}
			}
			a.mu.Unlock()
		case <-a.closed:
			return
		}
	}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
