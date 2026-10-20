package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type selfView struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Port    int      `json:"port"`
	RecvDir string   `json:"recvDir"`
	IPs     []string `json:"ips"`
}

type stateView struct {
	Self      selfView       `json:"self"`
	Peers     []Peer         `json:"peers"`
	Transfers []Transfer     `json:"transfers"`
	Offers    []*PendingOffer `json:"offers"`
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", a.hState)
	mux.HandleFunc("/api/info", a.hInfo)
	mux.HandleFunc("/api/name", a.hName)
	mux.HandleFunc("/api/recvdir", a.hRecvDir)
	mux.HandleFunc("/api/open-recv-dir", a.hOpenRecvDir)
	mux.HandleFunc("/api/pick-send", a.hPickSend)
	mux.HandleFunc("/api/send-path", a.hSendPath)
	mux.HandleFunc("/api/drop-prepare", a.hDropPrepare)
	mux.HandleFunc("/api/drop-file", a.hDropFile)
	mux.HandleFunc("/api/drop-done", a.hDropDone)
	mux.HandleFunc("/api/rescan", a.hRescan)
	mux.HandleFunc("/api/offer", a.hOffer)
	mux.HandleFunc("/api/upload", a.hUpload)
	mux.HandleFunc("/api/done", a.hDone)
	mux.HandleFunc("/api/accept", a.hAccept)
	mux.HandleFunc("/api/reject", a.hReject)
	mux.Handle("/", http.FileServer(http.FS(a.uiFS)))
	return mux
}

func (a *App) hState(w http.ResponseWriter, r *http.Request) {
	a.cfgMu.RLock()
	self := selfView{
		ID:      a.cfg.ID,
		Name:    a.cfg.Name,
		Port:    a.port,
		RecvDir: a.cfg.RecvDir,
	}
	a.cfgMu.RUnlock()
	for _, ip := range localIPv4s() {
		self.IPs = append(self.IPs, ip.String())
	}

	a.mu.Lock()
	peers := make([]Peer, 0, len(a.peers))
	for _, p := range a.peers {
		peers = append(peers, *p)
	}
	trs := make([]Transfer, 0, len(a.transOrd))
	for i := len(a.transOrd) - 1; i >= 0; i-- {
		if t, ok := a.trans[a.transOrd[i]]; ok {
			trs = append(trs, *t)
		}
	}
	offers := make([]*PendingOffer, 0, len(a.offer))
	offers = append(offers, a.offer...)
	a.mu.Unlock()

	sort.Slice(peers, func(i, j int) bool { return peers[i].Name < peers[j].Name })
	writeJSON(w, stateView{Self: self, Peers: peers, Transfers: trs, Offers: offers})
}

func (a *App) hInfo(w http.ResponseWriter, r *http.Request) {
	a.cfgMu.RLock()
	id, name := a.cfg.ID, a.cfg.Name
	a.cfgMu.RUnlock()
	writeJSON(w, infoResp{ID: id, Name: name, Port: a.port, OS: "windows"})
}

func (a *App) hName(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "bad body"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "名字不能为空"})
		return
	}
	if len([]rune(name)) > 32 {
		name = string([]rune(name)[:32])
	}
	a.cfgMu.Lock()
	a.cfg.Name = name
	_ = a.cfg.Save()
	a.cfgMu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "name": name})
}

func (a *App) hRecvDir(w http.ResponseWriter, r *http.Request) {
	dir, err := pickDir()
	if err != nil || dir == "" {
		writeJSON(w, map[string]any{"ok": false})
		return
	}
	// Catch an unwritable choice now rather than letting every inbound
	// transfer fail later with "写入失败".
	if err := ensureWritable(dir); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "该目录不可写：" + err.Error()})
		return
	}
	a.cfgMu.Lock()
	a.cfg.RecvDir = dir
	_ = a.cfg.Save()
	a.cfgMu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "dir": dir})
}

// ensureWritable creates the directory and proves a file can be written there.
// A path can be selectable in the dialog yet unwritable: network shares, a
// disconnected drive letter, or a folder owned by another user.
func ensureWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".gq-write-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func (a *App) hOpenRecvDir(w http.ResponseWriter, r *http.Request) {
	a.cfgMu.RLock()
	dir := a.cfg.RecvDir
	a.cfgMu.RUnlock()
	_ = os.MkdirAll(dir, 0o755)
	_ = exec.Command("explorer", dir).Start()
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) hRescan(w http.ResponseWriter, r *http.Request) {
	go a.scanLAN()
	writeJSON(w, map[string]any{"ok": true})
}

// hPickSend opens the native file picker, then hands the selection to the
// send goroutine. The picker runs on this request's goroutine, so the UI stays
// responsive (it polls /api/state independently).
func (a *App) hPickSend(w http.ResponseWriter, r *http.Request) {
	peerID := r.URL.Query().Get("peer")
	mode := r.URL.Query().Get("mode")

	a.mu.Lock()
	p, ok := a.peers[peerID]
	var peer Peer
	if ok {
		peer = *p
	}
	a.mu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "设备已离线"})
		return
	}

	var paths []string
	var err error
	if mode == "dir" {
		var d string
		d, err = pickDir()
		if d != "" {
			paths = []string{d}
		}
	} else {
		paths, err = pickFiles()
	}
	if err != nil {
		log.Printf("pick: %v", err)
	}
	if len(paths) == 0 {
		writeJSON(w, map[string]any{"ok": false, "canceled": true})
		return
	}

	id, err := a.startSend(peer, paths)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "transfer": id})
}

// hSendPath is the scriptable version of hPickSend: it takes absolute paths
// instead of opening a dialog. Localhost only, because otherwise any LAN peer
// could make this machine exfiltrate arbitrary files.
func (a *App) hSendPath(w http.ResponseWriter, r *http.Request) {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err != nil || (h != "127.0.0.1" && h != "::1") {
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]any{"ok": false, "error": "localhost only"})
		return
	}
	var body struct {
		Peer  string   `json:"peer"`
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || len(body.Paths) == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "bad body"})
		return
	}
	a.mu.Lock()
	p, ok := a.peers[body.Peer]
	var peer Peer
	if ok {
		peer = *p
	}
	a.mu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "设备已离线"})
		return
	}
	id, err := a.startSend(peer, body.Paths)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "transfer": id})
}

// ---- inbound side -------------------------------------------------------

type offerReq struct {
	From struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"from"`
	Files []FileMeta `json:"files"`
}

type offerResp struct {
	Session string `json:"session,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (a *App) hOffer(w http.ResponseWriter, r *http.Request) {
	var req offerReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, offerResp{Error: "bad body"})
		return
	}
	if len(req.Files) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, offerResp{Error: "empty"})
		return
	}

	sender := req.From.Name
	if sender == "" {
		sender = "未知设备"
	}
	addr := r.RemoteAddr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		addr = h
	}

	var total int64
	for _, f := range req.Files {
		total += f.Size
	}

	a.mu.Lock()
	if len(a.offer) >= maxPendingOffers {
		a.mu.Unlock()
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, offerResp{Error: "busy"})
		return
	}
	o := &PendingOffer{
		ID:     newID(),
		Sender: sender,
		Addr:   addr,
		Files:  req.Files,
		Total:  total,
		ch:     make(chan bool, 1),
	}
	// Queue instead of reject: two colleagues sending at once is normal, and
	// bouncing the second one with "busy" is the wrong answer.
	a.offer = append(a.offer, o)
	a.mu.Unlock()

	// LT_AUTO_ACCEPT=1 skips the prompt: used by the end-to-end test harness
	// and by headless installs where nobody is watching the screen.
	if os.Getenv("LT_AUTO_ACCEPT") == "1" {
		o.resolve(true)
	}

	defer func() {
		a.mu.Lock()
		for i, x := range a.offer {
			if x == o {
				a.offer = append(a.offer[:i], a.offer[i+1:]...)
				break
			}
		}
		a.mu.Unlock()
	}()

	select {
	case ok := <-o.ch:
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			writeJSON(w, offerResp{Error: "rejected"})
			return
		}
	case <-time.After(offerTimeout):
		w.WriteHeader(http.StatusRequestTimeout)
		writeJSON(w, offerResp{Error: "timeout"})
		return
	case <-r.Context().Done():
		return
	}

	sid := newID()
	a.cfgMu.RLock()
	recvDir := a.cfg.RecvDir
	a.cfgMu.RUnlock()
	dir := uniqueDir(filepath.Join(recvDir, fmt.Sprintf("%s_%s", cleanName(sender), time.Now().Format("0102-150405"))))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("mkdir recv dir: %v", err)
	}

	s := &Session{
		ID:       sid,
		Dir:      dir,
		Files:    req.Files,
		Created:  time.Now(),
		Deadline: time.Now().Add(sessionIdleTimeout).UnixMilli(),
	}
	for _, f := range req.Files {
		s.Names = append(s.Names, cleanName(f.Name))
	}
	t := a.newTransfer("recv", sender, req.Files)
	t.State = "active"
	s.TransID = t.ID

	a.mu.Lock()
	a.sessions[sid] = s
	a.mu.Unlock()

	writeJSON(w, offerResp{Session: sid})
}

func (a *App) hUpload(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session")
	idx := atoiSafe(r.URL.Query().Get("idx"))

	a.mu.Lock()
	s, ok := a.sessions[sid]
	if ok {
		s.Deadline = time.Now().Add(sessionIdleTimeout).UnixMilli()
	}
	a.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if idx < 0 || idx >= len(s.Files) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	meta := s.Files[idx]
	name := s.Names[idx]
	dst := uniquePath(filepath.Join(s.Dir, name))
	tmp := dst + ".part"

	f, err := os.Create(tmp)
	if err != nil {
		log.Printf("create %s: %v", tmp, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.mu.Lock()
	t := a.trans[s.TransID]
	if t != nil {
		t.Cur = idx
		t.State = "active"
	}
	a.mu.Unlock()

	pw := &progressWriter{w: f}
	if t != nil {
		pw.fn = func(n int64) { a.addProgress(t, n) }
	}
	buf := make([]byte, 512*1024)
	n, copyErr := io.CopyBuffer(pw, io.LimitReader(r.Body, meta.Size), buf)
	closeErr := f.Close()

	if copyErr != nil || closeErr != nil || n != meta.Size {
		os.Remove(tmp)
		if t != nil {
			a.finishTransfer(t, "error", fmt.Sprintf("写入失败 (%d/%d)", n, meta.Size))
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		if t != nil {
			a.finishTransfer(t, "error", "重命名失败")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// No addProgress here: progressWriter already accounted for every byte
	// while copying. Adding n again would double-count the whole file.
	w.WriteHeader(http.StatusOK)
}

func (a *App) hDone(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session")
	a.mu.Lock()
	s, ok := a.sessions[sid]
	if ok {
		delete(a.sessions, sid)
	}
	var t *Transfer
	if ok {
		t = a.trans[s.TransID]
	}
	a.mu.Unlock()
	if ok && t != nil {
		a.finishTransfer(t, "done", "")
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) hAccept(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	a.mu.Lock()
	var o *PendingOffer
	for _, x := range a.offer {
		if body.ID == "" || x.ID == body.ID {
			o = x
			break
		}
	}
	a.mu.Unlock()
	if o == nil {
		writeJSON(w, map[string]any{"ok": false})
		return
	}
	o.resolve(true)
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) hReject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	a.mu.Lock()
	var o *PendingOffer
	for _, x := range a.offer {
		if body.ID == "" || x.ID == body.ID {
			o = x
			break
		}
	}
	a.mu.Unlock()
	if o == nil {
		writeJSON(w, map[string]any{"ok": false})
		return
	}
	o.resolve(false)
	writeJSON(w, map[string]any{"ok": true})
}

// ---- helpers ------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func atoiSafe(s string) int {
	n := 0
	if s == "" {
		return -1
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return -1
		}
		n = n*10 + int(s[i]-'0')
		if n > 1<<20 {
			return -1
		}
	}
	return n
}

// cleanName strips path separators and characters Windows refuses, then
// dodges the reserved DOS device names.
func cleanName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			b.WriteRune('_')
		default:
			if r < 32 {
				continue
			}
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.Trim(out, ". ")

	ext := filepath.Ext(out)
	stem := strings.ToUpper(strings.TrimSuffix(out, ext))
	switch stem {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		out = "_" + out
	}
	if out == "" {
		out = "file"
	}
	// Win32 MAX_PATH is 260; leave room for the directory and a " (n)" suffix.
	if len([]rune(out)) > 180 {
		r := []rune(out)
		if e := filepath.Ext(out); e != "" {
			keep := 180 - len(e)
			if keep > 0 {
				out = string(r[:keep]) + e
			}
		} else {
			out = string(r[:180])
		}
	}
	return out
}

func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; i < 1000; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(c); os.IsNotExist(err) {
			return c
		}
	}
	return p
}

// uniqueDir is uniquePath for receive folders: two peers with the same name
// sending in the same second must not share a directory.
func uniqueDir(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	for i := 1; i < 1000; i++ {
		c := fmt.Sprintf("%s (%d)", p, i)
		if _, err := os.Stat(c); os.IsNotExist(err) {
			return c
		}
	}
	return p
}
