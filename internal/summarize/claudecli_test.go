package summarize

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// stubClaude records its argv (NUL-separated), stdin, working directory and
// DW_SUMMARIZER into $STUB_DIR, then prints $STUB_REPLY and exits
// $STUB_EXIT — or, with $STUB_SLEEP set, becomes `sleep` so a timeout kill
// lands on the process actually holding the pipes.
const stubClaude = `#!/bin/sh
for a in "$@"; do printf '%s\0' "$a"; done > "$STUB_DIR/args"
cat > "$STUB_DIR/stdin"
pwd > "$STUB_DIR/pwd"
printf '%s' "$DW_SUMMARIZER" > "$STUB_DIR/env"
[ -n "$STUB_SLEEP" ] && exec sleep "$STUB_SLEEP"
printf '%s' "$STUB_REPLY"
echo "stub stderr" >&2
exit "${STUB_EXIT:-0}"
`

// installStubClaude writes the stub as <tmp>/bin/claude and returns its path
// and the directory it records into.
func installStubClaude(t *testing.T) (bin, dir string) {
	t.Helper()
	base := t.TempDir()
	bin = filepath.Join(base, "claude")
	if err := os.WriteFile(bin, []byte(stubClaude), 0o755); err != nil { //nolint:gosec // a stub executable has to be executable
		t.Fatal(err)
	}
	dir = filepath.Join(base, "rec")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_DIR", dir)
	return bin, dir
}

func readRec(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestClaudeCLI_InvocationIsIsolated(t *testing.T) {
	bin, dir := installStubClaude(t)
	t.Setenv("STUB_REPLY", "# T\n")
	t.Setenv("DW_SUMMARIZER", "")
	root := t.TempDir()
	t.Chdir(root) // the worker's own cwd must not leak into the child

	c := &claudeCLI{command: bin, model: "haiku"}
	out, err := c.Summarize(context.Background(), Request{Date: "2026-10-15", State: "# T\n", Transcript: "USER: hi\n"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "# T\n" {
		t.Errorf("out = %q", out)
	}

	args := strings.Split(strings.TrimSuffix(readRec(t, dir, "args"), "\x00"), "\x00")
	want := []string{
		"-p", "--model", "haiku", "--system-prompt", systemPrompt, "--tools", "",
		"--setting-sources", "", "--no-session-persistence", "--output-format", "text",
	}
	if !slices.Equal(args, want) {
		t.Errorf("argv = %q\nwant %q", args, want)
	}
	if slices.Contains(args, "--bare") {
		t.Error("--bare would break subscription (OAuth) auth")
	}
	if got := readRec(t, dir, "env"); got != "1" {
		t.Errorf("DW_SUMMARIZER = %q in the child, want 1", got)
	}
	pwd := strings.TrimSpace(readRec(t, dir, "pwd"))
	if tmp, _ := filepath.EvalSymlinks(os.TempDir()); pwd != tmp && pwd != os.TempDir() {
		t.Errorf("child cwd = %q, want the temp dir", pwd)
	}
	if strings.HasPrefix(pwd, root) {
		t.Errorf("child ran inside %q", root)
	}
	if stdin := readRec(t, dir, "stdin"); !strings.Contains(stdin, "USER: hi") || !strings.Contains(stdin, "<current_state>") {
		t.Errorf("stdin = %q", stdin)
	}
}

func TestClaudeCLI_Failures(t *testing.T) {
	bin, _ := installStubClaude(t)

	t.Setenv("STUB_EXIT", "3")
	_, err := (&claudeCLI{command: bin, model: "haiku"}).Summarize(context.Background(), Request{})
	if err == nil || !strings.Contains(err.Error(), "stub stderr") {
		t.Errorf("non-zero exit: err = %v, want it with the child's stderr", err)
	}

	t.Setenv("STUB_EXIT", "0")
	t.Setenv("STUB_SLEEP", "5")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := (&claudeCLI{command: bin, model: "haiku"}).Summarize(ctx, Request{}); err == nil {
		t.Error("timeout: want an error")
	}
	if time.Since(start) > 4*time.Second {
		t.Errorf("timeout took %v; the child was not killed", time.Since(start))
	}
}
