//go:build unix

package main

// End-to-end tests for the background summarizer: the real binary, a stub
// `claude`, and Claude Code's side played by the test.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubSummarizerClaude stands in for `claude -p`. With $HOOK_PAYLOAD set it
// first fires `dw hook` the way a user-level Stop hook would inside the
// summarizer's own claude process ($HOOK_PREFIX can strip the env guard).
// Then it waits $STUB_SLEEP seconds and replies with a valid STATE.md.
const stubSummarizerClaude = `#!/bin/sh
cat > /dev/null
if [ -n "$HOOK_PAYLOAD" ]; then
  printf '%s' "$HOOK_PAYLOAD" | $HOOK_PREFIX dw hook
fi
sleep "${STUB_SLEEP:-0}"
printf '# topic\n\n## 前提\n\n- (2026-10-15, 事実) 決まった\n\n## 却下した案\n\n## 未決の問い\n\n- 次は？\n'
`

type summarizerEnv struct {
	shellEnv
	ws, transcript, logPath string
	base                    []string
}

func newSummarizerEnv(t *testing.T) summarizerEnv {
	t.Helper()
	e := summarizerEnv{shellEnv: buildEnv(t)}
	home := filepath.Dir(e.root)
	stub := filepath.Join(e.binDir, "claude")
	if err := os.WriteFile(stub, []byte(stubSummarizerClaude), 0o755); err != nil { //nolint:gosec // a stub executable has to be executable
		t.Fatal(err)
	}
	e.logPath = filepath.Join(home, "cache", "dw", "summarize.log")
	e.base = []string{
		"PATH=" + e.binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"DW_ROOT=" + e.root,
		"HOME=" + home,
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"DW_CLAUDE_COMMAND=" + stub,
		"DW_SUMMARY_DEBOUNCE=0s",
	}
	for _, k := range []string{"DW_CONFIG", "DW_SUMMARY_BACKEND", "DW_LLM_BASE_URL", "DW_LLM_API_KEY", "DW_LLM_MODEL", "DW_SUMMARIZER", "HOOK_PAYLOAD", "HOOK_PREFIX"} {
		e.base = append(e.base, k+"=")
	}

	out, err := e.cmd(nil, "topic").Output()
	if err != nil {
		t.Fatalf("dw topic: %v", err)
	}
	e.ws = strings.SplitN(string(out), "\t", 2)[0]
	e.transcript = filepath.Join(home, "session.jsonl")
	if err := os.WriteFile(e.transcript, []byte(`{"type":"user","message":{"content":"決めよう"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e summarizerEnv) cmd(extra []string, args ...string) *exec.Cmd {
	c := exec.Command(filepath.Join(e.binDir, "dw"), args...) //nolint:gosec // test-controlled
	c.Env = append(append(os.Environ(), e.base...), extra...)
	return c
}

func (e summarizerEnv) payload() string {
	return `{"session_id":"sess-1","transcript_path":"` + e.transcript + `","stop_hook_active":false}`
}

func (e summarizerEnv) log() string {
	b, _ := os.ReadFile(e.logPath)
	return string(b)
}

// TestHook_DetachesWorker is the point of the design: the hook returns at
// once even though the summary takes seconds, and the summary still lands.
// The test holds the hook's stdout pipe and reads it to EOF, so a worker that
// inherited it would keep the read blocked for the stub's whole sleep.
func TestHook_DetachesWorker(t *testing.T) {
	e := newSummarizerEnv(t)
	c := e.cmd([]string{"CLAUDE_PROJECT_DIR=" + e.ws, "STUB_SLEEP=2"}, "hook")
	c.Stdin = strings.NewReader(e.payload())
	stdout, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(stdout)
	if err := c.Wait(); err != nil {
		t.Fatalf("dw hook: %v", err)
	}
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("hook took %v; it must not wait for the worker", took)
	}
	if len(out) != 0 {
		t.Errorf("hook wrote %q to stdout", out)
	}

	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(e.log(), "result=written") {
		if time.Now().After(deadline) {
			t.Fatalf("no summary within 10s; log:\n%s", e.log())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if b, _ := os.ReadFile(filepath.Join(e.ws, "STATE.md")); !strings.Contains(string(b), "決まった") {
		t.Errorf("STATE.md =\n%s", b)
	}
}

// TestSummarize_DoesNotRecurse runs the worker against a claude that fires
// `dw hook` itself, as a user-level Stop hook would. Neither the env guard nor
// stop_hook_active may let that spawn a nested worker.
func TestSummarize_DoesNotRecurse(t *testing.T) {
	cases := []struct {
		name  string
		extra []string
	}{
		{"DW_SUMMARIZER guard", nil},
		{"stop_hook_active without the env guard", []string{
			"HOOK_PREFIX=env -u DW_SUMMARIZER",
			`HOOK_PAYLOAD={"session_id":"sess-2","transcript_path":"/x","stop_hook_active":true}`,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newSummarizerEnv(t)
			extra := append([]string{"CLAUDE_PROJECT_DIR=" + e.ws, "HOOK_PAYLOAD=" + e.payload()}, c.extra...)
			out, err := e.cmd(extra, "summarize", "--session", "sess-1", "--transcript", e.transcript, e.ws).CombinedOutput()
			if err != nil {
				t.Fatalf("dw summarize: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "written") {
				t.Errorf("summarize said %q, want written", out)
			}
			if strings.Contains(e.log(), "hook: spawned") {
				t.Errorf("the summarizer's own claude spawned a nested worker:\n%s", e.log())
			}
		})
	}
}

// TestSummarize_RecursionTestIsLive guards the test above against passing
// vacuously: the same stub hook, outside the summarizer, does spawn.
func TestSummarize_RecursionTestIsLive(t *testing.T) {
	e := newSummarizerEnv(t)
	c := e.cmd([]string{"CLAUDE_PROJECT_DIR=" + e.ws}, "hook")
	c.Stdin = strings.NewReader(e.payload())
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.log(), "hook: spawned") {
		t.Fatalf("hook did not spawn outside the summarizer; log:\n%s", e.log())
	}
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(e.log(), "result=") && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond) // let the worker finish before TempDir cleanup
	}
}
