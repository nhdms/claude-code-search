package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const hookMarker = "# claude-search hook"

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Manage the Claude Code Stop hook that auto-syncs new conversations",
	}
	cmd.AddCommand(newHookInstallCmd(), newHookUninstallCmd(), newHookStatusCmd())
	return cmd
}

func defaultHookCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "claude-search import", nil
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return resolved + " import", nil
}

func settingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func newHookInstallCmd() *cobra.Command {
	var cmdLine string
	var matcher string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Add a Stop hook to ~/.claude/settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdLine == "" {
				def, err := defaultHookCommand()
				if err != nil {
					return err
				}
				cmdLine = def
			}
			path := settingsPath()
			settings, err := loadSettings(path)
			if err != nil {
				return err
			}
			added := upsertStopHook(settings, matcher, cmdLine)
			if err := saveSettings(path, settings); err != nil {
				return err
			}
			if added {
				fmt.Printf("✓ Added Stop hook → %s\n  command: %s\n", path, cmdLine)
			} else {
				fmt.Printf("✓ Hook already present in %s\n  command: %s\n", path, cmdLine)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cmdLine, "command", "", "Command to run (default: <abs path to claude-search> import)")
	cmd.Flags().StringVar(&matcher, "matcher", "", "Hook matcher (empty = always)")
	return cmd
}

func newHookUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the Stop hook from ~/.claude/settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := settingsPath()
			settings, err := loadSettings(path)
			if err != nil {
				return err
			}
			removed := removeStopHook(settings)
			if !removed {
				fmt.Printf("No claude-search Stop hook found in %s\n", path)
				return nil
			}
			if err := saveSettings(path, settings); err != nil {
				return err
			}
			fmt.Printf("✓ Removed Stop hook from %s\n", path)
			return nil
		},
	}
}

func newHookStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show installed Stop hooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := settingsPath()
			settings, err := loadSettings(path)
			if err != nil {
				return err
			}
			entries := stopHooks(settings)
			if len(entries) == 0 {
				fmt.Printf("%s: no Stop hooks installed\n", path)
				return nil
			}
			fmt.Printf("%s:\n", path)
			for _, e := range entries {
				cs := ""
				if isClaudeSearch(e.Command) {
					cs = " ← claude-search"
				}
				fmt.Printf("  matcher=%q  cmd=%s%s\n", e.Matcher, e.Command, cs)
			}
			return nil
		},
	}
}

type hookEntry struct {
	Matcher string
	Command string
}

func loadSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func saveSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

// upsertStopHook adds a Stop hook with the given matcher+command, deduping any
// existing claude-search entries (so re-running after a path change refreshes
// the binary location). Returns true if anything actually changed.
func upsertStopHook(settings map[string]any, matcher, command string) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	stopArr, _ := hooks["Stop"].([]any)

	changed := false
	// Strip any existing claude-search hooks; we'll re-add a fresh one.
	stopArr, removed := filterStopArr(stopArr, isClaudeSearch)
	if removed > 0 {
		changed = true
	}

	stopArr = append(stopArr, map[string]any{
		"matcher": matcher,
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	})
	hooks["Stop"] = stopArr
	return changed || true
}

func removeStopHook(settings map[string]any) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	stopArr, _ := hooks["Stop"].([]any)
	if len(stopArr) == 0 {
		return false
	}
	next, removed := filterStopArr(stopArr, isClaudeSearch)
	if removed == 0 {
		return false
	}
	if len(next) == 0 {
		delete(hooks, "Stop")
	} else {
		hooks["Stop"] = next
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	return true
}

func filterStopArr(arr []any, isOurs func(cmd string) bool) ([]any, int) {
	out := make([]any, 0, len(arr))
	removed := 0
	for _, item := range arr {
		entry, _ := item.(map[string]any)
		inner, _ := entry["hooks"].([]any)
		filtered := make([]any, 0, len(inner))
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			cmd, _ := hm["command"].(string)
			if isOurs(cmd) {
				removed++
				continue
			}
			filtered = append(filtered, h)
		}
		if len(filtered) == 0 {
			continue
		}
		entry["hooks"] = filtered
		out = append(out, entry)
	}
	return out, removed
}

func isClaudeSearch(cmd string) bool {
	return cmd != "" && (containsToken(cmd, "claude-search") || containsToken(cmd, "claude_search"))
}

func containsToken(s, token string) bool {
	return len(s) >= len(token) && (indexOf(s, token) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func stopHooks(settings map[string]any) []hookEntry {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	stopArr, _ := hooks["Stop"].([]any)
	var out []hookEntry
	for _, item := range stopArr {
		entry, _ := item.(map[string]any)
		matcher, _ := entry["matcher"].(string)
		inner, _ := entry["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			cmd, _ := hm["command"].(string)
			out = append(out, hookEntry{Matcher: matcher, Command: cmd})
		}
	}
	return out
}
