package summarize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edge2992/dw/internal/workspace"
)

// failureThreshold is how many failed runs in a row it takes before
// `dw <topic>` warns.
const failureThreshold = 3

type status struct {
	LastOK              time.Time `json:"last_ok,omitzero"`
	LastError           string    `json:"last_error,omitempty"`
	FailingSince        time.Time `json:"failing_since,omitzero"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
}

func statusPath(ws string) string {
	return filepath.Join(workspace.CacheDir(), "status", wsKey(ws)+".json")
}

func readStatus(ws string) status {
	var s status
	if b, err := os.ReadFile(statusPath(ws)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// recordStatus folds one run's result into the workspace's status. Only a
// real write (or a deliberate no-op) counts as success and only errors and
// rejections count as failure; "nothing-new", "busy" and "conflict" say
// nothing about whether the backend works.
func recordStatus(ws, result string, now time.Time) {
	s := readStatus(ws)
	switch {
	case result == resultWritten || result == resultUnchanged:
		s = status{LastOK: now}
	case strings.HasPrefix(result, "error:") || strings.HasPrefix(result, "rejected:"):
		if s.ConsecutiveFailures == 0 {
			s.FailingSince = now
		}
		s.ConsecutiveFailures++
		s.LastError = result
	default:
		return
	}
	b, _ := json.Marshal(s)
	path := statusPath(ws)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
		_ = os.WriteFile(path, b, 0o600)
	}
}

// FailingSince reports when the current streak of failed summaries began, if
// it has reached failureThreshold.
func FailingSince(ws string) (time.Time, bool) {
	s := readStatus(ws)
	return s.FailingSince, s.ConsecutiveFailures >= failureThreshold
}
