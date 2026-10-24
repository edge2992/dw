package topic

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// isolateCache points os.UserCacheDir() at a fresh temp dir for the duration
// of the test, so Recent/SaveLast tests never touch the real
// ~/.cache/dw/last or ~/Library/Caches/dw/last.
func isolateCache(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "") // Darwin ignores this; unset it anyway for portability
}

func tempDirs(t *testing.T, n int) []string {
	t.Helper()
	dirs := make([]string, n)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}
	return dirs
}

func TestRecent_NoneSaved(t *testing.T) {
	isolateCache(t)
	if got := Recent(); len(got) != 0 {
		t.Errorf("Recent() with nothing saved = %q, want none", got)
	}
}

func TestSaveLast_KeepsRecentNewestFirst(t *testing.T) {
	isolateCache(t)
	d := tempDirs(t, 3)

	for _, p := range []string{d[0], d[1], d[0], d[2]} {
		if err := SaveLast(p); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := Recent(), []string{d[2], d[0], d[1]}; !slices.Equal(got, want) {
		t.Errorf("Recent() = %q, want %q (newest first, a revisit moves up instead of repeating)", got, want)
	}
}

func TestSaveLast_ForgetsBeyondRecentMax(t *testing.T) {
	isolateCache(t)
	d := tempDirs(t, recentMax+2)
	for _, p := range d {
		if err := SaveLast(p); err != nil {
			t.Fatal(err)
		}
	}
	got := Recent()
	if len(got) != recentMax || got[0] != d[len(d)-1] {
		t.Errorf("Recent() = %q, want the %d newest, newest first", got, recentMax)
	}
}

func TestRecent_StaleDirIsIgnored(t *testing.T) {
	isolateCache(t)
	kept := t.TempDir()
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{kept, gone} {
		if err := SaveLast(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if got := Recent(); !slices.Equal(got, []string{kept}) {
		t.Errorf("Recent() after removing a dir = %q, want only %q", got, kept)
	}
}
