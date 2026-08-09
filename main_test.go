package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup isolates DW_ROOT (to a fresh temp dir) and the "last visited" cache
// (to a fresh HOME) so tests never touch the real ~/dw or ~/.cache/dw/last.
// It returns the isolated root.
func setup(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	t.Setenv("DW_ROOT", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
	return root
}

func runCmd(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = run(append([]string{"dw"}, args...), &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestRun_ListEmptyRoot(t *testing.T) {
	setup(t)
	stdout, stderr, code := runCmd(t)
	if code != 0 {
		t.Errorf("code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestRun_ResolveCreatesAndLists(t *testing.T) {
	root := setup(t)

	stdout, stderr, code := runCmd(t, "Datadog", "Cost")
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout = %q, want exactly one row", stdout)
	}
	cols := strings.Split(lines[0], "\t")
	if len(cols) != 3 {
		t.Fatalf("row %q, want 3 tab-separated columns", lines[0])
	}
	if !strings.HasPrefix(cols[0], root) {
		t.Errorf("path column %q is not under root %q", cols[0], root)
	}
	if cols[1] != "* Datadog Cost" {
		t.Errorf("marker+title column = %q, want %q (freshly resolved workspace becomes \"last\")", cols[1], "* Datadog Cost")
	}

	// A second bare `dw` should now list exactly that one workspace, pinned.
	stdout, _, code = runCmd(t)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "* Datadog Cost") {
		t.Errorf("listing after creation = %q, want it to contain the pinned marker+title", stdout)
	}
}

func TestRun_PartialMatchDoesNotCreate(t *testing.T) {
	root := setup(t)
	if _, _, code := runCmd(t, "datadog-cost-reduction"); code != 0 {
		t.Fatal("setup creation failed")
	}

	stdout, stderr, code := runCmd(t, "datadog")
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout = %q, want exactly one row", stdout)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	dirs := 0
	for _, e := range entries {
		if e.IsDir() {
			dirs++
		}
	}
	if dirs != 1 {
		t.Errorf("root has %d workspace dirs after a resolving partial match, want 1 (no new workspace)", dirs)
	}
}

func TestRun_MultipleMatches(t *testing.T) {
	setup(t)
	for _, topic := range []string{"datadog-cost", "datadog-alerts"} {
		if _, _, code := runCmd(t, topic); code != 0 {
			t.Fatalf("setup creation of %q failed", topic)
		}
	}

	stdout, stderr, code := runCmd(t, "datadog")
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want exactly two rows for two ambiguous matches", stdout)
	}
}

func TestRun_EmptyTopicExitsTwo(t *testing.T) {
	setup(t)
	_, stderr, code := runCmd(t, "   ")
	if code != 2 {
		t.Errorf("code = %d, want 2 (stderr: %s)", code, stderr)
	}
}

func TestRun_AbsentAbsolutePathExitsOne(t *testing.T) {
	root := setup(t)
	bogus := filepath.Join(root, "2099-01-01-nope")
	_, stderr, code := runCmd(t, bogus)
	if code != 1 {
		t.Errorf("code = %d, want 1 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(bogus); err == nil {
		t.Error("an unresolved absolute path must not create a directory")
	}
}

func TestRun_Help(t *testing.T) {
	root := setup(t)
	stdout, _, code := runCmd(t, "help")
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	found := false
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Root:") && strings.Contains(line, root) {
			found = true
		}
	}
	if !found {
		t.Errorf("dw help output has no \"Root: <path>\" line for %q:\n%s", root, stdout)
	}
}

func TestRun_Version(t *testing.T) {
	setup(t)
	stdout, _, code := runCmd(t, "version")
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.HasPrefix(stdout, "dw ") {
		t.Errorf("stdout = %q, want it to start with \"dw \"", stdout)
	}
}

func TestRun_InitShellWrapper(t *testing.T) {
	setup(t)
	for _, shell := range []string{"zsh", "bash"} {
		stdout, _, code := runCmd(t, "init", shell)
		if code != 0 {
			t.Fatalf("dw init %s: code = %d, want 0", shell, code)
		}
		if !strings.Contains(stdout, "dw()") {
			t.Errorf("dw init %s did not print the wrapper function:\n%s", shell, stdout)
		}
	}

	if _, _, code := runCmd(t, "init"); code != 2 {
		t.Errorf("dw init with no shell: code = %d, want 2", code)
	}
	if _, _, code := runCmd(t, "init", "fish"); code != 2 {
		t.Errorf("dw init fish: code = %d, want 2", code)
	}
}
