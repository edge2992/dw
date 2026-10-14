package workspace

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// claudeSettings is the per-workspace .claude/settings.json that wires Claude
// Code's Stop hook to `dw hook`, which hands each finished turn to the
// background STATE.md summarizer. "{{command}}" (quotes included) is replaced
// by the JSON-encoded hook command.
//
//go:embed assets/claude-settings.json
var claudeSettings string

const commandPlaceholder = `"{{command}}"`

// executable is os.Executable, swappable so tests can pin the hook command.
var executable = os.Executable

// HookCommand is the shell command the Stop hook runs: this binary's absolute
// path plus "hook", so the hook works even where dw is not on Claude Code's
// PATH (GUI or IDE launches). Falls back to plain "dw hook".
func HookCommand() string {
	exe, err := executable()
	if err != nil || exe == "" {
		return "dw hook"
	}
	return shellQuote(exe) + " hook"
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./+-]+$`)

func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func renderSettings(command string) string {
	enc, _ := json.Marshal(command) // a string always marshals
	return strings.Replace(claudeSettings, commandPlaceholder, string(enc), 1)
}

// EnsureScaffold makes sure dir/.claude/settings.json carries dw's Stop hook.
// A missing file is written; a file dw wrote earlier whose command has gone
// stale (the binary moved) is refreshed. A settings.json the user wrote or
// edited is never touched — merging into it is left to the user.
func EnsureScaffold(dir string) error {
	path := filepath.Join(dir, ".claude", "settings.json")
	want := renderSettings(HookCommand())
	cur, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		_, err := writeIfAbsent(path, want, 0o644)
		return err
	case err != nil:
		return err
	case string(cur) == want || !isOwnSettings(cur):
		return nil
	}
	return os.WriteFile(path, []byte(want), 0o644)
}

// isOwnSettings reports whether b is exactly what renderSettings produces for
// some command, i.e. dw wrote it and nobody has edited it since.
func isOwnSettings(b []byte) bool {
	var s struct {
		Hooks struct {
			Stop []struct {
				Hooks []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"Stop"`
		} `json:"hooks"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil || len(s.Hooks.Stop) != 1 || len(s.Hooks.Stop[0].Hooks) != 1 {
		return false
	}
	return renderSettings(s.Hooks.Stop[0].Hooks[0].Command) == string(b)
}
