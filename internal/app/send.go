package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type sendItem struct {
	path string
	meta FileMeta
}

// startSend expands the selection into a flat file list and kicks off the
// transfer. Directory structure is flattened on purpose: it keeps the
// receiver free of path-traversal handling, and one-file-per-request is
// already the fastest shape on a LAN.
func (a *App) startSend(peer Peer, paths []string) (string, error) {
	var items []sendItem
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		if st.IsDir() {
			_ = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
				if err != nil || info == nil || info.IsDir() {
					return nil
				}
				items = append(items, sendItem{path, FileMeta{Name: filepath.Base(path), Size: info.Size()}})
				return nil
			})
			continue
		}
		items = append(items, sendItem{p, FileMeta{Name: filepath.Base(p), Size: st.Size()}})
	}
	if len(items) == 0 {
		return "", fmt.Errorf("没有可发送的文件")
	}

	metas := make([]FileMeta, 0, len(items))
	for _, it := range items {
		metas = append(metas, it.meta)
	}

	t := a.newTransfer("send", peer.Name, metas)
	go a.doSend(t, peer, items)
	return t.ID, nil
}

func (a *App) doSend(t *Transfer, peer Peer, items []sendItem) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	defer client.CloseIdleConnections()

	base := fmt.Sprintf("http://%s:%d", peer.Addr, peer.Port)

	// 1. Offer: the receiver shows a prompt and only then answers.
	offerBody, _ := json.Marshal(map[string]any{
		"from":  map[string]string{"id": a.cfg.ID, "name": a.cfg.Name},
		"files": t.Files,
	})
	req, err := http.NewRequest(http.MethodPost, base+"/api/offer", bytes.NewReader(offerBody))
	if err != nil {
		a.finishTransfer(t, "error", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		a.finishTransfer(t, "error", "对方无响应（可能已离线或防火墙拦截）")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden:
		a.finishTransfer(t, "canceled", "对方拒绝了本次传输")
		return
	case http.StatusRequestTimeout:
		a.finishTransfer(t, "error", "对方长时间未确认")
		return
	case http.StatusConflict:
		a.finishTransfer(t, "error", "对方正在接收另一个传输")
		return
	default:
		a.finishTransfer(t, "error", fmt.Sprintf("协商失败 HTTP %d %s", resp.StatusCode, string(body)))
		return
	}

	var or offerResp
	if err := json.Unmarshal(body, &or); err != nil || or.Session == "" {
		a.finishTransfer(t, "error", "对方返回了无效会话")
		return
	}

	a.mu.Lock()
	t.State = "active"
	a.mu.Unlock()

	// 2. Upload one file per request. Serial is deliberate: parallel uploads
	// on a 1GbE link just add disk contention on the receiver.
	for i, it := range items {
		a.mu.Lock()
		t.Cur = i
		a.mu.Unlock()

		if err := a.uploadOne(client, base, or.Session, i, it, t); err != nil {
			log.Printf("upload %s: %v", it.meta.Name, err)
			a.finishTransfer(t, "error", err.Error())
			a.cancelRemote(client, base, or.Session)
			return
		}
	}

	// 3. Close the session so the receiver can flush its UI.
	dreq, _ := http.NewRequest(http.MethodPost, base+"/api/done?session="+or.Session, nil)
	if dresp, err := client.Do(dreq); err == nil {
		dresp.Body.Close()
	}
	a.finishTransfer(t, "done", "")
}

func (a *App) uploadOne(client *http.Client, base, session string, idx int, it sendItem, t *Transfer) error {
	f, err := os.Open(it.path)
	if err != nil {
		return fmt.Errorf("打开 %s 失败: %w", it.meta.Name, err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}

	pr := &progressReader{r: f, fn: func(n int64) { a.addProgress(t, n) }}
	url := fmt.Sprintf("%s/api/upload?session=%s&idx=%d", base, session, idx)
	req, err := http.NewRequest(http.MethodPost, url, pr)
	if err != nil {
		return err
	}
	req.ContentLength = st.Size()
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("传输中断: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("对方写入失败 HTTP %d", resp.StatusCode)
	}
	return nil
}

func (a *App) cancelRemote(c *http.Client, base, session string) {
	req, err := http.NewRequest(http.MethodPost, base+"/api/done?session="+session, nil)
	if err != nil {
		return
	}
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
	}
}

type progressReader struct {
	r  io.Reader
	fn func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 && p.fn != nil {
		p.fn(int64(n))
	}
	return n, err
}

// progressWriter is the inbound counterpart. Without it io.Copy only reports
// the total once at the end, which makes the receiver's bar jump from 0 to
// 100% in a single step.
type progressWriter struct {
	w  io.Writer
	fn func(int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if n > 0 && p.fn != nil {
		p.fn(int64(n))
	}
	return n, err
}
