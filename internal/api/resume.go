package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
		Project   string `json:"project"`
		Terminal  string `json:"terminal"` // optional override
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if body.SessionID == "" {
		writeErr(w, 400, fmt.Errorf("session_id required"))
		return
	}
	dir := body.CWD
	if dir == "" {
		dir = body.Project
	}
	if dir == "" {
		dir = "~"
	}
	cmd := fmt.Sprintf(`cd %s && claude --resume %s`, shellQuote(dir), body.SessionID)

	terminal, err := openInTerminal(cmd, body.Terminal)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "terminal": terminal, "command": cmd})
}

func openInTerminal(cmd, override string) (string, error) {
	if override != "" {
		return runTerminal(override, cmd)
	}
	switch runtime.GOOS {
	case "darwin":
		// Prefer iTerm if it's installed.
		if hasApp("iTerm") {
			return runTerminal("iterm", cmd)
		}
		return runTerminal("terminal", cmd)
	case "linux":
		for _, t := range []string{"gnome-terminal", "konsole", "xterm"} {
			if _, err := exec.LookPath(t); err == nil {
				return runTerminal(t, cmd)
			}
		}
		return "", fmt.Errorf("no supported terminal found on Linux (tried gnome-terminal, konsole, xterm)")
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func runTerminal(name, cmd string) (string, error) {
	switch name {
	case "terminal", "Terminal":
		script := fmt.Sprintf(`tell application "Terminal" to activate
tell application "Terminal" to do script %q`, cmd)
		return "Terminal", exec.Command("osascript", "-e", script).Run()
	case "iterm", "iTerm":
		script := fmt.Sprintf(`tell application "iTerm"
  activate
  create window with default profile
  tell current session of current window to write text %q
end tell`, cmd)
		return "iTerm", exec.Command("osascript", "-e", script).Run()
	case "gnome-terminal":
		sh := userShell()
		return "gnome-terminal", exec.Command("gnome-terminal", "--", sh, "-lc", cmd+"; exec "+sh).Start()
	case "konsole":
		sh := userShell()
		return "konsole", exec.Command("konsole", "-e", sh, "-lc", cmd+"; exec "+sh).Start()
	case "xterm":
		sh := userShell()
		return "xterm", exec.Command("xterm", "-e", sh, "-lc", cmd+"; exec "+sh).Start()
	}
	return "", fmt.Errorf("unknown terminal: %s", name)
}

func hasApp(name string) bool {
	out, err := exec.Command("mdfind", "-name", name+".app", "-onlyin", "/Applications").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// shellQuote single-quote-quotes s for shell, escaping internal single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// userShell returns the user's default login shell from $SHELL, falling back
// to /bin/bash. Always returns an absolute path so it works under -e/--.
func userShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	for _, p := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join("/bin", "bash")
}
