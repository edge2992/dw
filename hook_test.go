package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edge2992/dw/internal/summarize"
)

// recordSpawns replaces spawnWorker for the rest of the test and returns the
// jobs it was handed.
func recordSpawns(t *testing.T) *[]summarize.Job {
	t.Helper()
	var jobs []summarize.Job
	saved := spawnWorker
	spawnWorker = func(j summarize.Job) error { jobs = append(jobs, j); return nil }
	t.Cleanup(func() { spawnWorker = saved })
	return &jobs
}

// hookWorkspace creates a workspace and points the hook environment at it,
// with the claude backend selected through a stub that exists on disk.
func hookWorkspace(t *testing.T) string {
	t.Helper()
	setup(t)
	stdout, _, code := runCmd(t, "hooked")
	if code != 0 {
		t.Fatal("setup creation failed")
	}
	ws := strings.SplitN(stdout, "\t", 2)[0]
	t.Setenv("CLAUDE_PROJECT_DIR", ws)
	stub := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // a stub executable has to be executable
		t.Fatal(err)
	}
	t.Setenv("DW_CLAUDE_COMMAND", stub)
	return ws
}

const validPayload = `{"session_id":"abc-123","transcript_path":"/tmp/t.jsonl","stop_hook_active":false}`

func TestHook_SpawnsWorkerForWorkspace(t *testing.T) {
	ws := hookWorkspace(t)
	jobs := recordSpawns(t)

	stdout, stderr, code := runWithStdin(t, validPayload, "hook")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0 and silence", code, stdout, stderr)
	}
	want := summarize.Job{Workspace: ws, Session: "abc-123", Transcript: "/tmp/t.jsonl"}
	if len(*jobs) != 1 || (*jobs)[0] != want {
		t.Errorf("spawned %+v, want exactly %+v", *jobs, want)
	}
}

func TestHook_NeverSpawnsWhenItShouldNot(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		env     map[string]string
	}{
		{"garbage payload", "not json", nil},
		{"stop_hook_active", `{"session_id":"abc","transcript_path":"/tmp/t","stop_hook_active":true}`, nil},
		{"missing session", `{"transcript_path":"/tmp/t"}`, nil},
		{"inside the summarizer", validPayload, map[string]string{"DW_SUMMARIZER": "1"}},
		{"not a workspace", validPayload, map[string]string{"CLAUDE_PROJECT_DIR": os.TempDir()}},
		{"no project dir", validPayload, map[string]string{"CLAUDE_PROJECT_DIR": ""}},
		{"backend off", validPayload, map[string]string{"DW_SUMMARY_BACKEND": "off"}},
		{"bad config", validPayload, map[string]string{"DW_SUMMARY_BACKEND": "bogus"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hookWorkspace(t)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			jobs := recordSpawns(t)
			stdout, _, code := runWithStdin(t, c.payload, "hook")
			if code != 0 || stdout != "" {
				t.Errorf("code=%d stdout=%q, want 0 and no stdout", code, stdout)
			}
			if len(*jobs) != 0 {
				t.Errorf("spawned %+v, want nothing", *jobs)
			}
		})
	}
}

func TestHelp_ShowsSummarizer(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"off", map[string]string{"DW_CLAUDE_COMMAND": "no-such-claude-binary"}, "Summarizer: off ("},
		{"claude", map[string]string{"DW_CLAUDE_COMMAND": "sh"}, "Summarizer: sh -p (model=haiku)"},
		{"openai", map[string]string{"DW_LLM_BASE_URL": "http://localhost:11434/v1", "DW_LLM_MODEL": "m"}, "Summarizer: openai-compatible (model=m at http://localhost:11434/v1)"},
		{"error", map[string]string{"DW_LLM_API_KEY": "k", "DW_LLM_MODEL": "m"}, "Summarizer: error ("},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setup(t)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			stdout, _, _ := runCmd(t, "help")
			if !strings.Contains(stdout, c.want) {
				t.Errorf("dw help has no %q line:\n%s", c.want, stdout)
			}
		})
	}
}

// TestResolve_WarnsAfterRepeatedFailures drives three failing summaries
// through the real worker (the claude stub replies with nothing, which is
// rejected), then checks entering the workspace says so.
func TestResolve_WarnsAfterRepeatedFailures(t *testing.T) {
	ws := hookWorkspace(t)
	transcript := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"type":"user","message":{"content":"hi"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, _, code := runCmd(t, "summarize", "--session", "s", "--transcript", transcript, ws); code != 1 {
			t.Fatalf("summarize run %d: code = %d, want 1 from the failing stub", i, code)
		}
		if i == 1 {
			if _, stderr, _ := runCmd(t, "hooked"); strings.Contains(stderr, "failing") {
				t.Errorf("warned after 2 failures: %q", stderr)
			}
		}
	}
	_, stderr, _ := runCmd(t, "hooked")
	if !strings.Contains(stderr, "STATE.md summaries failing since") || !strings.Contains(stderr, summarize.LogPath()) {
		t.Errorf("stderr = %q, want the failure warning naming the log", stderr)
	}
}

func TestResolve_BackfillsHook(t *testing.T) {
	root := setup(t)
	ws := filepath.Join(root, "2026-01-01-old")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runCmd(t, "old"); code != 0 {
		t.Fatal("resolve failed")
	}
	if _, err := os.Stat(filepath.Join(ws, ".claude", "settings.json")); err != nil {
		t.Errorf("pre-existing workspace did not get its hook: %v", err)
	}
}

func TestSummarize_RejectsNonWorkspace(t *testing.T) {
	setup(t)
	_, stderr, code := runCmd(t, "summarize", "--session", "s", "--transcript", "/x", t.TempDir())
	if code != 1 || !strings.Contains(stderr, "not a workspace") {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
	if _, _, code := runCmd(t, "summarize", "--session", "s"); code != 2 {
		t.Errorf("--session without --transcript: code = %d, want 2", code)
	}
}
