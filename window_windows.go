//go:build windows

package main

import (
	"os/exec"

	"github.com/jchv/go-webview2"
)

// openWindow opens a native WebView2 window. It returns false when the
// WebView2 runtime is missing so the caller can fall back to a browser.
func openWindow(url string) bool {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "局域网快传",
			Width:  1040,
			Height: 720,
			Center: true,
		},
	})
	if w == nil {
		return false
	}
	defer w.Destroy()
	w.Navigate(url)
	w.Run()
	return true
}

func openBrowser(url string) {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	_ = cmd.Start()
}
