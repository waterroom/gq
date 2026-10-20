//go:build windows

package app

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// firewallRuleName is shared by the TCP and UDP rules so a single delete
// removes the pair, and so "is it already allowed" is one cheap check.
const firewallRuleName = "gq LAN"

var procShellExecuteW = shell32.NewProc("ShellExecuteW")

// firewallRulesKey holds the persisted inbound/outbound rules. Reading it needs
// no elevation, and the payload is ASCII (Action=Block|App=...) regardless of
// the system language - unlike `netsh show`, whose verdict text is localised.
const firewallRulesKey = `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\FirewallRules`

// FirewallBlocked reports whether an explicit Block rule covers this exe.
//
// This is the state that made the first version of this feature useless: the
// allow rule existed and the button went green, but a Block rule left behind by
// a dismissed Windows prompt outranks it, so the machine stayed unreachable.
func FirewallBlocked() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, firewallRulesKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	names, err := k.ReadValueNames(0)
	if err != nil {
		return false
	}
	want := strings.ToLower(exe)
	for _, name := range names {
		v, _, err := k.GetStringValue(name)
		if err != nil {
			continue
		}
		if !strings.Contains(v, "Action=Block") || !strings.Contains(v, "Active=TRUE") {
			continue
		}
		for _, field := range strings.Split(v, "|") {
			if strings.HasPrefix(strings.ToLower(field), "app=") &&
				strings.TrimPrefix(strings.ToLower(field), "app=") == want {
				return true
			}
		}
	}
	return false
}

// FirewallRulePresent reports whether the inbound rule exists.
//
// `netsh show` is read-only, needs no elevation and takes a few ms, so the page
// can state the current situation rather than guess. The rule name is ASCII on
// purpose: on a Chinese Windows the surrounding output is GBK, but this string
// survives either way.
func FirewallRulePresent() bool {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name="+firewallRuleName)
	hideWindow(cmd)
	return cmd.Run() == nil
}

// RequestFirewallRule asks Windows for elevation and runs this same exe with
// --allow-firewall. The user sees one UAC prompt; the rule is added by the
// helper process, so the UI keeps running and polls for the outcome.
//
// Nothing here can bypass the prompt: adding an inbound rule is an
// administrator operation, full stop.
func RequestFirewallRule() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	args, _ := syscall.UTF16PtrFromString("--allow-firewall")
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))

	// ShellExecuteW returns a value > 32 on success; <= 32 is an error code,
	// 1223 meaning the user dismissed the UAC prompt.
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(dir)),
		uintptr(1), // SW_SHOWNORMAL
	)
	if ret <= 32 {
		if ret == 1223 {
			return fmt.Errorf("已取消管理员授权")
		}
		return fmt.Errorf("无法启动提权进程（ShellExecute=%d）", ret)
	}
	return nil
}

// AllowFirewallCLI is the elevated entry point: `lantransfer.exe --allow-firewall`.
// Started normally it fails with "requires elevation", which is logged - that is
// also how the failure path is tested.
func AllowFirewallCLI() int {
	// Started without elevation this can only fail, and the reason is worth
	// stating plainly: netsh answers in the console code page, which would land
	// in the UTF-8 log as mojibake.
	if !windows.GetCurrentProcessToken().IsElevated() {
		log.Printf("firewall: not elevated - launch this through the 放行防火墙 button so Windows asks for admin")
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		log.Printf("firewall: cannot locate exe: %v", err)
		return 1
	}

	// Remove *every* inbound rule that targets this exe, whatever it is called.
	//
	// Windows names the rules it creates from the "allow access" prompt after the
	// binary ("lantransfer"), and a prompt that was dismissed leaves a Block rule
	// there. An explicit Block outranks any Allow rule, so keeping them makes the
	// rules added below do nothing - which is exactly why the first version of
	// this feature reported success and still left the machine unreachable.
	out, err := runNetsh("advfirewall", "firewall", "delete", "rule", "name=all", "dir=in", "program="+exe)
	if err != nil {
		// Not fatal: a machine with no rules to delete answers with an error.
		log.Printf("firewall: clearing old inbound rules: %v (%s)", err, out)
	} else if out != "" {
		log.Printf("firewall: cleared old inbound rules for %s", exe)
	}

	for _, proto := range []string{"TCP", "UDP"} {
		out, err := runNetsh("advfirewall", "firewall", "add", "rule",
			"name="+firewallRuleName,
			"dir=in",
			"action=allow",
			"program="+exe,
			"enable=yes",
			"profile=any",
			"protocol="+proto,
			// Only peers on the same subnet can connect. The transfer is LAN-only
			// by nature, so this costs nothing and keeps the hole narrow.
			"remoteip=localsubnet",
		)
		if err != nil {
			log.Printf("firewall: add %s failed: %v (%s)", proto, err, out)
			return 1
		}
		log.Printf("firewall: allowed inbound %s for %s (local subnet only)", proto, exe)
	}

	// Verify the outcome instead of assuming it: if a Block rule somehow
	// survived, the allow rules above are inert and claiming success would send
	// the user back to "still does not work" with a green button.
	if FirewallBlocked() {
		// Fallback for the rules Windows created from a dismissed prompt: those
		// are named after the binary, so a program-filtered delete can miss them
		// on older builds. Deleting by that name cannot touch the gq LAN rule.
		base := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
		if out, err := runNetsh("advfirewall", "firewall", "delete", "rule", "name="+base); err == nil {
			log.Printf("firewall: removed legacy rules named %q (%s)", base, out)
		}
	}
	if FirewallBlocked() {
		log.Printf("firewall: a Block rule still covers %s; the allow rule cannot take effect", exe)
		return 2
	}
	return 0
}

func runNetsh(args ...string) (string, error) {
	cmd := exec.Command("netsh", args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	// netsh speaks the console code page (GBK here), the log is UTF-8.
	return strings.TrimSpace(strings.ToValidUTF8(string(out), "?")), err
}

// hideWindow keeps a console from flashing up: this is a windowsgui process, so
// any child console program would otherwise pop a black box on screen.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func (a *App) hFirewall(w http.ResponseWriter, r *http.Request) {
	if !localOnly(w, r) {
		return
	}
	if FirewallRulePresent() && !FirewallBlocked() {
		writeJSON(w, map[string]any{"ok": true, "present": true, "blocked": false})
		return
	}
	if err := RequestFirewallRule(); err != nil {
		log.Printf("firewall: %v", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "present": false})
}

func (a *App) hFirewallStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"present": FirewallRulePresent(),
		"blocked": FirewallBlocked(),
	})
}
