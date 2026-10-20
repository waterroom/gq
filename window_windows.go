//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"
)

// webview2RuntimeURL is the official Evergreen bootstrapper. Fixed here rather
// than taken from the page so the endpoint cannot be turned into an
// open-any-URL gadget.
const webview2RuntimeURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// openWindow opens a native WebView2 window. On failure it returns the reason
// so the page can say why it is running in a browser instead of guessing.
func openWindow(url string) (bool, string) {
	if os.Getenv("LT_FORCE_BROWSER") == "1" {
		return false, "已按 LT_FORCE_BROWSER 强制使用浏览器窗口"
	}
	// Always log the probe result: when a user reports "it opened a browser",
	// this line alone distinguishes "not installed" from "installed but
	// failing", without asking them to query the registry by hand.
	dp := webview2DataPath()
	log.Printf("webview2: runtime=%q datapath=%q", installedRuntimeVersion(), dp)

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		// WebView2 keeps a browser profile under this path. Left empty, the
		// library picks %APPDATA%\<exe name>, which silently fails when the
		// roaming profile is remapped, read-only, or missing entirely.
		DataPath: dp,
		WindowOptions: webview2.WindowOptions{
			Title:  "gq",
			Width:  1040,
			Height: 720,
			Center: true,
		},
	})
	if w == nil {
		reason := diagnoseWebView2()
		log.Printf("webview2 init failed: %s", reason)
		return false, reason
	}
	defer w.Destroy()
	w.Navigate(url)
	w.Run()
	return true, ""
}

// webview2DataPath returns a writable profile directory, or "" to let the
// library decide. LOCALAPPDATA is the right place for a cache and is writable
// even for accounts whose roaming profile is not.
func webview2DataPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		return ""
	}
	p := filepath.Join(base, "gq", "webview2")
	if err := os.MkdirAll(p, 0o755); err != nil {
		log.Printf("webview2 data dir %s: %v", p, err)
		return ""
	}
	return p
}

// diagnoseWebView2 builds the most specific explanation it can.
//
// Order matters: a registry entry that points at files which no longer exist
// is a distinctive failure (cloned VM images, half-removed installs) and names
// itself better than the bare HRESULT. That case is exactly 0x80070002, which
// reads as "file not found" but tells the user nothing about which file.
func diagnoseWebView2() string {
	if msg := runtimeFileProblem(); msg != "" {
		return msg
	}
	if msg := lastRuntimeError(); msg != "" {
		return msg
	}
	if v := installedRuntimeVersion(); v != "" {
		return "WebView2 运行时已安装（" + v + "），但窗口初始化失败，详见 lantransfer.log"
	}
	return "未检测到 WebView2 运行时"
}

// webview2AppDirs lists the directories the runtime installs itself into when
// they exist. Both locations occur in the wild: the x86 path on 64-bit Windows
// and the native path on 32-bit installs.
func webview2AppDirs() []string {
	pf32 := os.Getenv("ProgramFiles(x86)")
	if pf32 == "" {
		pf32 = `C:\Program Files (x86)`
	}
	var out []string
	for _, pf := range []string{pf32, os.Getenv("ProgramFiles")} {
		if pf == "" {
			continue
		}
		d := filepath.Join(pf, "Microsoft", "EdgeWebView", "Application")
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// runtimeFileProblem reports a registry entry whose binaries are missing.
// Returns "" when at least one installed version actually has its executable,
// so a stale `pv` value alongside a newer working install is not reported as
// breakage. That "registry says X, disk says nothing" state is what produces
// 0x80070002 on cloned VM images and half-removed installs.
func runtimeFileProblem() string {
	v := installedRuntimeVersion()
	if v == "" {
		return ""
	}
	dirs := webview2AppDirs()
	hasExe := false
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(d, e.Name(), "msedgewebview2.exe")); err == nil {
				hasExe = true
				break
			}
		}
		if hasExe {
			return ""
		}
	}
	if len(dirs) == 0 {
		return "注册表记录 WebView2 " + v + "，但 EdgeWebView\\Application 目录不存在。安装已损坏，请重新安装运行时。"
	}
	return "注册表记录 WebView2 " + v + "，但 " + dirs[0] + " 下没有任何 msedgewebview2.exe。安装已损坏，请重新安装运行时。"
}

// lastRuntimeError reads the tail of our log for the lines the library emits.
func lastRuntimeError() string {
	if logFilePath == "" {
		return ""
	}
	f, err := os.Open(logFilePath)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	const tail = 8192
	off := st.Size() - tail
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return ""
	}
	s := string(buf)

	if i := strings.LastIndex(s, "Error calling Webview2Loader:"); i >= 0 {
		return "WebView2 加载器不可用：" + firstLine(s[i+len("Error calling Webview2Loader:"):])
	}
	if i := strings.LastIndex(s, "Result: "); i >= 0 {
		return describeHRESULT(firstLine(s[i+len("Result: "):]))
	}
	return ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if j := strings.IndexByte(s, '\n'); j >= 0 {
		s = s[:j]
	}
	// Trim again: logs are CRLF on Windows, so the cut above leaves a '\r'.
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120])
	}
	return s
}

// describeHRESULT turns the handful of codes WebView2 actually fails with into
// something actionable.
func describeHRESULT(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	switch c {
	case "80070490":
		return "未安装 WebView2 运行时（0x80070490）"
	case "80070002":
		return "WebView2 运行时文件找不到（0x80070002），安装可能不完整"
	case "80070005":
		return "数据目录不可写，拒绝访问（0x80070005）"
	case "80040154":
		return "WebView2 组件未注册（0x80040154）"
	case "8007139f":
		return "WebView2 运行时版本不匹配（0x8007139F）"
	}
	if c == "" {
		return "WebView2 初始化失败"
	}
	return "WebView2 初始化失败（0x" + strings.ToUpper(c) + "）"
}

// installedRuntimeVersion probes every place the runtime registers itself:
// machine-wide and per-user, 64-bit and 32-bit views. Read through the
// registry API rather than shelling out to reg.exe, which some machines block
// through execution policy.
func installedRuntimeVersion() string {
	const guid = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	subkeys := []string{
		`SOFTWARE\Microsoft\EdgeUpdate\Clients\` + guid,
		`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + guid,
	}
	roots := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}
	for _, root := range roots {
		for _, sub := range subkeys {
			k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			v, _, err := k.GetStringValue("pv")
			k.Close()
			if err == nil && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// openBrowser is the fallback path. Chromium's --app mode drops the address
// bar and tabs, so the fallback still looks like an application window rather
// than a page in a browser.
//
// done is closed when the browser window is closed, so the caller can exit
// instead of staying resident with the port bound. A second instance would
// then silently take the next port, leaving two receivers running.
func openBrowser(url string, done chan<- struct{}) {
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
			go func() {
				_ = cmd.Wait()
				log.Printf("fallback: browser window closed")
				close(done)
			}()
			return
		}
	}

	log.Printf("fallback: no chromium browser found, using default protocol handler")
	// The protocol handler hands off to whatever browser is registered and
	// gives us no process to watch, so the app stays resident.
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
