package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runCheckpoint scaffolds a workspace, runs its checkpoint.sh with the given
// Stop-hook payload on stdin, and returns the workspace path. The hook is a
// shell script, so the only way to know it works is to actually run it.
func runCheckpoint(t *testing.T, dir, payload string, env ...string) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(dir, ".claude", "hooks", "checkpoint.sh"))
	cmd.Stdin = strings.NewReader(payload)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("checkpoint.sh failed: %v\n%s", err, out)
	}
	// A Stop hook that exits non-zero tells Claude to keep going, and anything
	// it prints is noise in the session, so both must stay empty.
	if len(out) != 0 {
		t.Errorf("checkpoint.sh wrote output: %q", out)
	}
}

// newCheckpointWorkspace builds a real workspace to run the hook against,
// skipping when the tools the hook needs are not installed.
func newCheckpointWorkspace(t *testing.T) string {
	t.Helper()
	for _, bin := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	p, err := Create(t.TempDir(), "research", "hook", time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC), testTemplates("research"))
	if err != nil {
		t.Fatal(err)
	}
	return p.Path
}

func TestCheckpointHookWritesLastSession(t *testing.T) {
	dir := newCheckpointWorkspace(t)
	// quotes and newlines in the message are why the hook uses jq rather than
	// picking the JSON apart in shell
	payload := `{"session_id":"abc123","hook_event_name":"Stop","cwd":"/elsewhere",` +
		`"last_assistant_message":"decided to try \"region\" detection\nnext: 20 sheets"}`
	runCheckpoint(t, dir, payload, "CLAUDE_PROJECT_DIR="+dir)

	b, err := os.ReadFile(filepath.Join(dir, ".dw", "last-session.md"))
	if err != nil {
		t.Fatalf("last-session.md not written: %v", err)
	}
	got := string(b)
	for _, want := range []string{
		"session_id: abc123",
		"ended_at: 20",
		`decided to try "region" detection`,
		"next: 20 sheets",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("last-session.md missing %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("last-session.md does not start with frontmatter:\n%s", got)
	}
	// no stray temp files left behind by the write-then-rename
	entries, err := os.ReadDir(filepath.Join(dir, ".dw"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "last-session.md" {
		t.Errorf(".dw contains %v, want only last-session.md", entries)
	}
}

func TestCheckpointHookOverwritesPreviousRun(t *testing.T) {
	dir := newCheckpointWorkspace(t)
	runCheckpoint(t, dir, `{"session_id":"first","last_assistant_message":"OLD"}`, "CLAUDE_PROJECT_DIR="+dir)
	runCheckpoint(t, dir, `{"session_id":"second","last_assistant_message":"NEW"}`, "CLAUDE_PROJECT_DIR="+dir)

	b, err := os.ReadFile(filepath.Join(dir, ".dw", "last-session.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	// the checkpoint is the latest state, not a growing log
	if strings.Contains(got, "OLD") || strings.Contains(got, "first") {
		t.Errorf("previous run was appended to rather than replaced:\n%s", got)
	}
	if !strings.Contains(got, "NEW") || !strings.Contains(got, "session_id: second") {
		t.Errorf("latest run missing:\n%s", got)
	}
}

func TestCheckpointHookWithoutJqIsSilentNoop(t *testing.T) {
	dir := newCheckpointWorkspace(t)
	// A PATH holding nothing but bash, rather than PATH minus jq's directory:
	// subtracting that directory takes bash down with it wherever the two live
	// side by side, as they do on the CI runner (/usr/bin). The hook checks for
	// jq before it reaches for anything else, so bash alone is enough to run it.
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	binDir := t.TempDir()
	if err := os.Symlink(bash, filepath.Join(binDir, "bash")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	if _, err := exec.LookPath("jq"); err == nil {
		t.Fatal("jq is still reachable on a PATH that should only hold bash")
	}

	// Must still exit 0 silently: a machine without jq gets no checkpoint, not
	// a session that refuses to stop.
	runCheckpoint(t, dir, `{"session_id":"abc","last_assistant_message":"hi"}`,
		"PATH="+binDir, "CLAUDE_PROJECT_DIR="+dir)
	if _, err := os.Stat(filepath.Join(dir, ".dw")); !os.IsNotExist(err) {
		t.Errorf(".dw was created without jq: %v", err)
	}
}

func TestCheckpointHookWithoutProjectDirIsSilentNoop(t *testing.T) {
	dir := newCheckpointWorkspace(t)
	// Without CLAUDE_PROJECT_DIR there is no workspace to write to; guessing
	// from cwd would scatter .dw/ directories outside dw's root.
	runCheckpoint(t, dir, `{"session_id":"abc","last_assistant_message":"hi"}`, "CLAUDE_PROJECT_DIR=")
	if _, err := os.Stat(filepath.Join(dir, ".dw")); !os.IsNotExist(err) {
		t.Errorf(".dw was created without CLAUDE_PROJECT_DIR: %v", err)
	}
}

func TestCheckpointHookWithoutMessageIsSilentNoop(t *testing.T) {
	dir := newCheckpointWorkspace(t)
	// An empty checkpoint is worse than none: next session would read a stub
	// and treat it as where we left off.
	runCheckpoint(t, dir, `{"session_id":"abc","hook_event_name":"Stop"}`, "CLAUDE_PROJECT_DIR="+dir)
	if _, err := os.Stat(filepath.Join(dir, ".dw", "last-session.md")); !os.IsNotExist(err) {
		t.Errorf("last-session.md written for a payload with no message: %v", err)
	}
}
