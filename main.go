package main

import (
	"log"
	"os"
	"path/filepath"

	"lantransfer/internal/app"
	"lantransfer/internal/ui"
)

// logFilePath is kept so the window code can read back what the WebView2
// library logged about its own failure.
var logFilePath string

func main() {
	setupLog()

	// Elevated helper mode: add the inbound firewall rule and exit. It is
	// started via the "runas" verb from the UI button, so this window never
	// blocks on a UAC prompt.
	if len(os.Args) > 1 && os.Args[1] == "--allow-firewall" {
		os.Exit(app.AllowFirewallCLI())
	}

	a, err := app.New(ui.FS())
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	defer a.Close()

	url, err := a.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	log.Printf("ui at %s", url)

	if ok, note := openWindow(url); !ok {
		log.Printf("webview2 unavailable, falling back to system browser")
		a.SetWindowFallback(note)
		// Closed when the browser window goes away; stays open when the
		// window could not be tracked (default protocol handler).
		done := make(chan struct{})
		openBrowser(url, done)
		<-done
		log.Printf("browser closed, exiting")
	}
	log.Printf("window closed, exiting")
}

// setupLog writes to a file next to the exe, because a windowsgui binary has no console.
func setupLog() {
	dir := exeDir()
	p := filepath.Join(dir, "lantransfer.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		ad, e2 := os.UserConfigDir()
		if e2 != nil {
			log.SetOutput(os.Stderr)
			return
		}
		d := filepath.Join(ad, "gq")
		os.MkdirAll(d, 0o755)
		p = filepath.Join(d, "lantransfer.log")
		f, err = os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.SetOutput(os.Stderr)
			return
		}
	}
	logFilePath = p
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
