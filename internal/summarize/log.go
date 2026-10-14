package summarize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edge2992/dw/internal/workspace"
)

// maxLogBytes is where summarize.log rotates to summarize.log.1 (one
// generation kept).
const maxLogBytes = 1 << 20

// LogPath is the summarizer's append-only log. API keys, request bodies and
// transcript or STATE.md content are never written to it.
func LogPath() string {
	return filepath.Join(workspace.CacheDir(), "summarize.log")
}

func openLog() (*os.File, error) {
	path := LogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogBytes {
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}

// Logf appends one timestamped line to the log. Logging is best effort: a
// failure here must never fail a hook or a worker.
func Logf(format string, args ...any) {
	f, err := openLog()
	if err != nil {
		return
	}
	defer f.Close()
	line := strings.ReplaceAll(fmt.Sprintf(format, args...), "\n", " ")
	fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), line)
}

func shortWS(ws string) string { return filepath.Base(ws) }

func shortSession(id string) string {
	if len(id) > 8 {
		return id[:8] + "…"
	}
	return id
}
