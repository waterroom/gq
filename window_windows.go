//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jchv/go-webview2"
)

// openWindow opens a native WebView2 window. It returns false when the
// WebView2 runtime is missing so the caller can fall back to a browser.
func openWindow(url string) bool {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "gq",
			Width:  1040,
			Height: 720,
			Center: true,
		},
	})
	if w == nil {
		logWebView2State()
		return false
	}
	defer w.Destroy()
	w.Navigate(url)
	w.Run()
	return true
}

// logWebView2State records why the runtime lookup failed, so a user reporting
// "it opened a browser instead of the app" has something to send back.
func logWebView2State() {
	const key = `HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	out, err := exec.Command("reg", "query", key, "/v", "pv").CombinedOutput()
	if err != nil {
		log.Printf("webview2 runtime: not registered (%v) — falling back", err)
		return
	}
	log.Printf("webview2 runtime: %s", strings.TrimSpace(string(out)))
}

// openBrowser is the fallback path. Chromium's --app mode drops the address
// bar and tabs, so the fallback still looks like an application window rather
// than a page in a browser.
func openBrowser(url string) {
	prof := filepath.Join(os.Getenv("LOCALAPPDATA"), "gq", "browser")
	_ = os.MkdirAll(prof, 0o755)

	cands := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	for _, p := range cands {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		cmd := exec.Command(p, "--app="+url, "--window-size=1040,720", "--user-data-dir="+prof)
		if err := cmd.Start(); err == nil {
			log.Printf("fallback: launched %s in app mode", p)
			return
		}
	}

	log.Printf("fallback: no chromium browser found, using default protocol handler")
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
