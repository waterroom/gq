package main

import (
	"log"
	"os"
	"path/filepath"

	"lantransfer/internal/app"
	"lantransfer/internal/ui"
)

func main() {
	setupLog()

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

	if !openWindow(url) {
		log.Printf("webview2 unavailable, falling back to system browser")
		openBrowser(url)
		select {}
	}
	log.Printf("window closed, exiting")
}

// setupLog writes to a file next to the exe, because a windowsgui binary has no console.
func setupLog() {
	dir := exeDir()
	f, err := os.OpenFile(filepath.Join(dir, "lantransfer.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		ad, e2 := os.UserConfigDir()
		if e2 != nil {
			log.SetOutput(os.Stderr)
			return
		}
		d := filepath.Join(ad, "LanTransfer")
		os.MkdirAll(d, 0o755)
		f, err = os.OpenFile(filepath.Join(d, "lantransfer.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.SetOutput(os.Stderr)
			return
		}
	}
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
