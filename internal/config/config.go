package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config is the on-disk identity of this installation. It lives next to the
// executable when that directory is writable, so the whole app stays portable.
type Config struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	RecvDir string `json:"recvDir"`
	// Host records which machine wrote this config. A portable exe invites
	// copying the whole folder to another PC; if the device ID travelled with
	// it, both machines would advertise the same ID and each would filter the
	// other out as "myself" during discovery.
	Host string `json:"host,omitempty"`
}

var (
	pathOnce sync.Once
	pathVal  string
)

func Path() string {
	pathOnce.Do(func() { pathVal = resolvePath() })
	return pathVal
}

func resolvePath() string {
	// LT_CONFIG lets a second instance (or a test harness) use its own
	// identity without touching the portable config next to the exe.
	if p := os.Getenv("LT_CONFIG"); p != "" {
		return p
	}
	dir := exeDir()
	if dir != "" {
		p := filepath.Join(dir, "lantransfer.json")
		// Probe writability by creating the file; a read-only stick or
		// Program Files install will fail here.
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.Close()
			return p
		}
	}
	ad, err := os.UserConfigDir()
	if err != nil {
		return "lantransfer.json"
	}
	d := filepath.Join(ad, "LanTransfer")
	_ = os.MkdirAll(d, 0o755)
	return filepath.Join(d, "config.json")
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func Load() (*Config, error) {
	p := Path()
	b, err := os.ReadFile(p)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		c := defaults()
		_ = c.Save()
		return c, nil
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		c = *defaults()
	}

	host, _ := os.Hostname()
	// Empty ID: fresh install. Host mismatch: the folder was copied from
	// another machine, so it needs a new identity. Empty Host: config written
	// by an older build, adopt it as-is rather than silently re-identifying.
	if c.ID == "" || (c.Host != "" && host != "" && c.Host != host) {
		c.ID = newID()
	}
	c.Host = host

	if strings.TrimSpace(c.Name) == "" {
		c.Name = defaultName()
	}
	if c.RecvDir == "" {
		c.RecvDir = defaultRecvDir()
	}
	_ = c.Save()
	return &c, nil
}

func defaults() *Config {
	h, _ := os.Hostname()
	return &Config{
		ID:      newID(),
		Name:    defaultName(),
		RecvDir: defaultRecvDir(),
		Host:    h,
	}
}

// Save writes atomically so a crash mid-write cannot corrupt the config.
func (c *Config) Save() error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	p := Path()
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

func defaultName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "我的电脑"
	}
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	return h
}

func defaultRecvDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "downloads"
	}
	return filepath.Join(home, "Downloads", "局域网快传")
}
