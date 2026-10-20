package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// Drag-and-drop sends the browser's File objects to this process, which
// streams them straight out to the peer. No temp file and no full buffering:
// a dropped 4 GB file costs one 32 KB copy buffer.
//
// Why not read the dropped file's path directly? The UI is a web page. Browsers
// never expose a real path, and the machines without a WebView2 runtime fall
// back to an Edge window, where the Win32 drop message is unreachable by
// design. Streaming through the page is the one path that works in both modes.

const (
	// dropTTL drops jobs that the page never finished (window closed mid-send).
	dropTTL = 10 * time.Minute
	// maxDropFiles keeps a single drop from turning into thousands of
	// requests against the receiver.
	maxDropFiles = 500
)

type dropJob struct {
	ID      string
	Peer    Peer
	Base    string
	Session string
	TransID string
	Files   []FileMeta
	Client  *http.Client
	Created time.Time
}

func newDropClient() *http.Client {
	// No overall timeout: a multi-gigabyte upload can legitimately outlive any
	// fixed deadline. Dial and idle timeouts still catch a dead peer.
	return &http.Client{
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

// localOnly guards endpoints that can make this machine send arbitrary data:
// without it any LAN peer could post a file and have it forwarded.
func localOnly(w http.ResponseWriter, r *http.Request) bool {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || (h != "127.0.0.1" && h != "::1") {
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]any{"ok": false, "error": "localhost only"})
		return false
	}
	return true
}

// hDropPrepare negotiates with the peer, then hands back a job id the page
// uses for each file. The offer blocks until the receiver answers, so the
// transfer row sits in "waiting" while the dialog is open — which is exactly
// what the UI already renders.
func (a *App) hDropPrepare(w http.ResponseWriter, r *http.Request) {
	if !localOnly(w, r) {
		return
	}
	var body struct {
		Peer  string     `json:"peer"`
		Files []FileMeta `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || len(body.Files) == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "bad body"})
		return
	}
	if len(body.Files) > maxDropFiles {
		writeJSON(w, map[string]any{"ok": false, "error": fmt.Sprintf("一次最多 %d 个文件", maxDropFiles)})
		return
	}

	metas := make([]FileMeta, 0, len(body.Files))
	for _, f := range body.Files {
		if f.Size < 0 {
			writeJSON(w, map[string]any{"ok": false, "error": "文件大小无效"})
			return
		}
		metas = append(metas, FileMeta{Name: cleanName(f.Name), Size: f.Size})
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

	t := a.newTransfer("send", peer.Name, metas)
	client := newDropClient()
	base := fmt.Sprintf("http://%s:%d", peer.Addr, peer.Port)

	session, failState, failMsg := a.negotiate(client, base, t.Files)
	if failState != "" {
		client.CloseIdleConnections()
		a.finishTransfer(t, failState, failMsg)
		writeJSON(w, map[string]any{"ok": false, "error": failMsg})
		return
	}

	a.mu.Lock()
	t.State = "active"
	a.mu.Unlock()

	j := &dropJob{
		ID:      newID(),
		Peer:    peer,
		Base:    base,
		Session: session,
		TransID: t.ID,
		Files:   metas,
		Client:  client,
		Created: time.Now(),
	}
	a.mu.Lock()
	a.drops[j.ID] = j
	a.mu.Unlock()

	writeJSON(w, map[string]any{"ok": true, "job": j.ID, "transfer": t.ID})
}

// hDropFile forwards one file. The request body *is* the file, so this is a
// pure pipe: read from the page, write to the peer, count the bytes.
func (a *App) hDropFile(w http.ResponseWriter, r *http.Request) {
	if !localOnly(w, r) {
		return
	}
	jobID := r.URL.Query().Get("job")
	idx := atoiSafe(r.URL.Query().Get("idx"))

	a.mu.Lock()
	j, ok := a.drops[jobID]
	var t *Transfer
	if ok {
		t = a.trans[j.TransID]
	}
	a.mu.Unlock()
	if !ok || t == nil || idx < 0 || idx >= len(j.Files) {
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]any{"ok": false, "error": "任务不存在或已结束"})
		return
	}

	a.mu.Lock()
	t.Cur = idx
	a.mu.Unlock()

	meta := j.Files[idx]
	pr := &progressReader{r: r.Body, fn: func(n int64) { a.addProgress(t, n) }}

	url := fmt.Sprintf("%s/api/upload?session=%s&idx=%d", j.Base, j.Session, idx)
	req, err := http.NewRequest(http.MethodPost, url, pr)
	if err != nil {
		a.failDrop(j, t, err.Error())
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// The browser knows the size, so state it explicitly: the receiver limits
	// its read to this count and would otherwise block on a chunked body.
	req.ContentLength = meta.Size
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := j.Client.Do(req)
	if err != nil {
		msg := "传输中断: " + err.Error()
		a.failDrop(j, t, msg)
		writeJSON(w, map[string]any{"ok": false, "error": msg})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("对方写入失败 HTTP %d", resp.StatusCode)
		a.failDrop(j, t, msg)
		writeJSON(w, map[string]any{"ok": false, "error": msg})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// hDropDone closes the session so the receiver's row flips to done.
func (a *App) hDropDone(w http.ResponseWriter, r *http.Request) {
	if !localOnly(w, r) {
		return
	}
	jobID := r.URL.Query().Get("job")

	a.mu.Lock()
	j, ok := a.drops[jobID]
	var t *Transfer
	if ok {
		delete(a.drops, jobID)
		t = a.trans[j.TransID]
	}
	a.mu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"ok": false})
		return
	}

	a.cancelRemote(j.Client, j.Base, j.Session) // POST /api/done, name is historical
	j.Client.CloseIdleConnections()
	if t != nil {
		a.finishTransfer(t, "done", "")
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) failDrop(j *dropJob, t *Transfer, msg string) {
	log.Printf("drop %s: %s", j.ID, msg)
	a.cancelRemote(j.Client, j.Base, j.Session)
	j.Client.CloseIdleConnections()
	if t != nil {
		a.finishTransfer(t, "error", msg)
	}
	a.mu.Lock()
	delete(a.drops, j.ID)
	a.mu.Unlock()
}

// reapDrops clears jobs the page abandoned (window closed mid-transfer).
func (a *App) reapDrops(now time.Time) {
	var stale []*dropJob
	a.mu.Lock()
	for id, j := range a.drops {
		if now.Sub(j.Created) > dropTTL {
			if t := a.trans[j.TransID]; t != nil && (t.State == "active" || t.State == "waiting") {
				t.State = "canceled"
				t.Err = "传输超时未完成"
				t.Speed = 0
			}
			delete(a.drops, id)
			stale = append(stale, j)
		}
	}
	a.mu.Unlock()
	for _, j := range stale {
		a.cancelRemote(j.Client, j.Base, j.Session)
		j.Client.CloseIdleConnections()
		log.Printf("drop expired: %s", j.ID)
	}
}
