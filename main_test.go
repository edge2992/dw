package main

import (
	"bytes"
	"io"
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
	if stderr != "" {
		t.Errorf("stderr = %q, want empty when stdout is captured rather than read by a human", stderr)
	}
}

// pretendTerminal makes isTerminal report true for the rest of the test, so
// the hints that only fire for a human at a prompt can be exercised without
// allocating one.
func pretendTerminal(t *testing.T) {
	t.Helper()
	saved := isTerminal
	isTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { isTerminal = saved })
}

// TestRun_EmptyRootHintsInsteadOfSayingNothing is the first-run case: bare dw
// with no topics used to print nothing at all and exit 0, which reads as a
// broken install.
func TestRun_EmptyRootHintsInsteadOfSayingNothing(t *testing.T) {
	root := setup(t)
	pretendTerminal(t)

	stdout, stderr, code := runCmd(t)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want the TSV contract left empty", stdout)
	}
	if !strings.Contains(stderr, root) {
		t.Errorf("stderr = %q, want it to name the root %q", stderr, root)
	}
	if !strings.Contains(stderr, "dw <name>") {
		t.Errorf("stderr = %q, want it to show how to create the first topic", stderr)
	}
}

// TestRun_RawTSVHintsAtShellIntegration covers the other way dw's stdout is
// unhelpful on its own: rows exist, but nobody wired up the wrapper, so the
// user is staring at tab-separated paths.
func TestRun_RawTSVHintsAtShellIntegration(t *testing.T) {
	setup(t)
	if _, _, code := runCmd(t, "alpha"); code != 0 {
		t.Fatal("setup creation failed")
	}
	pretendTerminal(t)

	stdout, stderr, _ := runCmd(t)
	if !strings.Contains(stdout, "alpha") {
		t.Fatalf("stdout = %q, want the topic row", stdout)
	}
	if !strings.Contains(stderr, "dw init zsh") {
		t.Errorf("stderr = %q, want the shell-integration hint", stderr)
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
	if len(cols) != 2 {
		t.Fatalf("row %q, want 2 tab-separated columns", lines[0])
	}
	if filepath.Dir(cols[0]) != root || !strings.HasSuffix(cols[0], "-Datadog Cost") {
		t.Errorf("path column %q, want <root>/<date>-Datadog Cost", cols[0])
	}
	if cols[1] != "* "+filepath.Base(cols[0]) {
		t.Errorf("marker+dir column = %q, want %q (freshly resolved topic becomes \"last\")", cols[1], "* "+filepath.Base(cols[0]))
	}
	if _, err := os.Stat(filepath.Join(cols[0], "Datadog Cost.md")); err != nil {
		t.Errorf("main note missing: %v", err)
	}

	// A second bare `dw` should now list exactly that one topic, pinned.
	stdout, _, code = runCmd(t)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if stdout != lines[0]+"\n" {
		t.Errorf("listing after creation = %q, want the pinned row %q", stdout, lines[0])
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
		t.Errorf("root has %d topic dirs after a resolving partial match, want 1 (no new topic)", dirs)
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

func TestRun_BadNameExitsTwo(t *testing.T) {
	root := setup(t)
	for _, name := range []string{"   ", "a/b", "[x]", ".hidden"} {
		_, stderr, code := runCmd(t, name)
		if code != 2 {
			t.Errorf("dw %q: code = %d, want 2 (stderr: %s)", name, code, stderr)
		}
	}
	if _, err := os.Stat(root); err == nil {
		t.Error("a rejected name must not create anything, not even the root")
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
