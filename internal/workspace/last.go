package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

// statePath returns the file dw uses to remember the last chosen workspace
// (e.g. ~/.cache/dw/last on Linux, ~/Library/Caches/dw/last on macOS).
func statePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "dw", "last")
}

// LastPath returns the most recently chosen workspace path, or "" if there is
// none yet or the recorded directory no longer exists.
func LastPath() string {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(b))
	if p == "" {
		return ""
	}
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		return ""
	}
	return p
}

// SaveLast records path as the most recently chosen workspace. Failures are
// returned but are non-fatal to callers — losing the "jump back" hint is benign.
func SaveLast(path string) error {
	f := statePath()
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(path+"\n"), 0o644)
}
