package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pinExecutable makes HookCommand see exe as the running binary.
func pinExecutable(t *testing.T, exe string) {
	t.Helper()
	saved := executable
	executable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executable = saved })
}

func readSettings(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHookCommand(t *testing.T) {
	pinExecutable(t, "/usr/local/bin/dw")
	if got := HookCommand(); got != "/usr/local/bin/dw hook" {
		t.Errorf("HookCommand() = %q", got)
	}
	pinExecutable(t, "/Users/me/my bin/dw's")
	if got, want := HookCommand(), `'/Users/me/my bin/dw'\''s' hook`; got != want {
		t.Errorf("HookCommand() = %q, want %q", got, want)
	}
	saved := executable
	executable = func() (string, error) { return "", errors.New("unknown") }
	defer func() { executable = saved }()
	if got := HookCommand(); got != "dw hook" {
		t.Errorf("HookCommand() without an executable = %q, want the PATH fallback", got)
	}
}

func TestCreate_ScaffoldsStopHook(t *testing.T) {
	pinExecutable(t, "/opt/dw/bin/dw")
	p, err := Create(t.TempDir(), "topic", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := readSettings(t, p.Path)
	if !strings.Contains(got, `"Stop"`) || !strings.Contains(got, `"command": "/opt/dw/bin/dw hook"`) {
		t.Errorf("settings.json =\n%s\nwant a Stop hook running the absolute dw path", got)
	}
}

func TestEnsureScaffold_RefreshesOnlyItsOwnStaleFile(t *testing.T) {
	dir := t.TempDir()
	pinExecutable(t, "/old/dw")
	if err := EnsureScaffold(dir); err != nil {
		t.Fatal(err)
	}

	pinExecutable(t, "/new/dw")
	if err := EnsureScaffold(dir); err != nil {
		t.Fatal(err)
	}
	if got := readSettings(t, dir); !strings.Contains(got, "/new/dw hook") {
		t.Errorf("stale dw-written settings not refreshed:\n%s", got)
	}

	userOwned := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/old/dw hook"}]}]},"model":"opus"}`
	if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(userOwned), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureScaffold(dir); err != nil {
		t.Fatal(err)
	}
	if got := readSettings(t, dir); got != userOwned {
		t.Errorf("user-owned settings.json was rewritten:\n%s", got)
	}
}

func TestIsWorkspace(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"2026-01-01-a", "2026-01-01-a/nested", ".git"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "rootlink")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		dir  string
		want bool
	}{
		{filepath.Join(root, "2026-01-01-a"), true},
		{filepath.Join(link, "2026-01-01-a"), true},
		{root, false},
		{filepath.Join(root, "2026-01-01-a", "nested"), false},
		{filepath.Join(root, ".git"), false},
		{filepath.Join(root, "missing"), false},
		{t.TempDir(), false},
		{"", false},
		{"2026-01-01-a", false},
	}
	for _, c := range cases {
		if got := IsWorkspace(root, c.dir); got != c.want {
			t.Errorf("IsWorkspace(%q) = %v, want %v", c.dir, got, c.want)
		}
	}
}

func TestWriteStateAtomic(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "# old\n")
	if err := os.Chmod(filepath.Join(dir, stateFileName), 0o600); err != nil {
		t.Fatal(err)
	}
	_, sum, err := ReadStateFile(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := WriteStateAtomic(dir, "# new\n", sum); err != nil {
		t.Fatal(err)
	}
	got, _, _ := ReadStateFile(dir)
	if got != "# new\n" {
		t.Errorf("STATE.md = %q after write", got)
	}
	if fi, _ := os.Stat(filepath.Join(dir, stateFileName)); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want the original 0600 kept", fi.Mode().Perm())
	}

	if err := WriteStateAtomic(dir, "# lost\n", sum); !errors.Is(err, ErrStateChanged) {
		t.Errorf("stale hash: err = %v, want ErrStateChanged", err)
	}
	if got, _, _ := ReadStateFile(dir); got != "# new\n" {
		t.Errorf("STATE.md = %q, a CAS miss must not write", got)
	}
	assertNoTempFiles(t, dir)
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(dir, ".STATE.md.tmp-*"))
	if len(left) > 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestOpenQuestionsAndTitle(t *testing.T) {
	content := "# T\n\n## 未決の問い\n\n<!-- c -->\n- q1\n\n- q2\n## next\n- not a question\n"
	qs, found := OpenQuestions(content)
	if !found || len(qs) != 2 || qs[0] != "- q1" || qs[1] != "- q2" {
		t.Errorf("OpenQuestions = %q, %v", qs, found)
	}
	if _, found := OpenQuestions("# T\n"); found {
		t.Error("OpenQuestions found a section that is not there")
	}
	if got := Title(content); got != "T" {
		t.Errorf("Title = %q", got)
	}
}
