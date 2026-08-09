package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// isolateCache points os.UserCacheDir() at a fresh temp dir for the duration
// of the test, so LastPath/SaveLast tests never touch the real
// ~/.cache/dw/last or ~/Library/Caches/dw/last.
func isolateCache(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "") // Darwin ignores this; unset it anyway for portability
}

func TestLastPath_NoneSaved(t *testing.T) {
	isolateCache(t)
	if got := LastPath(); got != "" {
		t.Errorf("LastPath() with nothing saved = %q, want \"\"", got)
	}
}

func TestSaveLastAndLoad(t *testing.T) {
	isolateCache(t)
	dir := t.TempDir()

	if err := SaveLast(dir); err != nil {
		t.Fatal(err)
	}
	if got := LastPath(); got != dir {
		t.Errorf("LastPath() = %q, want %q", got, dir)
	}
}

func TestLastPath_StaleDirIsIgnored(t *testing.T) {
	isolateCache(t)
	dir := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveLast(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if got := LastPath(); got != "" {
		t.Errorf("LastPath() for a removed dir = %q, want \"\"", got)
	}
}
