package topic

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// recentMax is how many recently visited topics stay pinned above the rest.
// One was not enough: a long-running topic sank below a day's worth of
// short-lived ones and had to be searched for every morning.
const recentMax = 5

// statePath returns the file dw uses to remember recently chosen topics
// (e.g. ~/.cache/dw/last on Linux, ~/Library/Caches/dw/last on macOS), one
// path per line, newest first.
func statePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "dw", "last")
}

// Recent returns the recently chosen topic paths, newest first. Paths whose
// directory no longer exists are dropped, so the result may be empty.
func Recent() []string {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return nil
	}
	var paths []string
	for p := range strings.SplitSeq(string(b), "\n") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths
}

// SaveLast records path as the most recently chosen topic, keeping up to
// recentMax earlier ones behind it. Failures are returned but are non-fatal
// to callers — losing the "jump back" hint is benign.
func SaveLast(path string) error {
	paths := append([]string{path}, slices.DeleteFunc(Recent(), func(p string) bool { return p == path })...)
	if len(paths) > recentMax {
		paths = paths[:recentMax]
	}
	f := statePath()
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(strings.Join(paths, "\n")+"\n"), 0o644)
}
