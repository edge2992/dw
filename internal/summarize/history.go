package summarize

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/edge2992/dw/internal/workspace"
)

// keepBackups is how many prior STATE.md versions are kept per workspace.
const keepBackups = 20

// wsKey names a workspace's files in the cache dir.
func wsKey(ws string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(ws)))
	return hex.EncodeToString(sum[:8])
}

// HistoryDir holds a workspace's rolling STATE.md backups, named by UTC
// RFC 3339 timestamp so lexical order is chronological. It lives in the cache
// dir, not the workspace, so the backups never become session context.
func HistoryDir(ws string) string {
	return filepath.Join(workspace.CacheDir(), "history", wsKey(ws))
}

// backup saves content as the newest version and prunes all but the last
// keepBackups.
func backup(ws, content string, now time.Time) error {
	dir := HistoryDir(ws)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := now.UTC().Format(time.RFC3339) + ".md"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir) // sorted by name, oldest first
	if err != nil {
		return err
	}
	for i := 0; i < len(entries)-keepBackups; i++ {
		_ = os.Remove(filepath.Join(dir, entries[i].Name()))
	}
	return nil
}
