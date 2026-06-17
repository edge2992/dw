package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/edge2992/dw/internal/workspace"
)

// seed creates two projects under a temp root and points DW_ROOT at it.
func seed(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("DW_ROOT", root)
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := workspace.Create(root, "research", "k8s pod oom", now, workspace.DefaultTemplate); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Create(root, "incident", "db outage", older, workspace.DefaultTemplate); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCmdList(t *testing.T) {
	seed(t)
	var out, errb bytes.Buffer
	if code := cmdList(&out, &errb, nil); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	got := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{"research/2026-06-14-k8s-pod-oom", "incident/2026-06-01-db-outage"}
	if len(got) != len(want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCmdListJSON(t *testing.T) {
	seed(t)
	var out, errb bytes.Buffer
	if code := cmdList(&out, &errb, []string{"--json"}); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var projects []workspace.Project
	if err := json.Unmarshal(out.Bytes(), &projects); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	if projects[0].Name != "2026-06-14-k8s-pod-oom" || projects[0].Path == "" {
		t.Errorf("unexpected first project: %+v", projects[0])
	}
}

func TestCmdListEmptyRoot(t *testing.T) {
	t.Setenv("DW_ROOT", t.TempDir()) // empty, existing dir
	var out, errb bytes.Buffer
	if code := cmdList(&out, &errb, []string{"--json"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("empty JSON = %q, want []", strings.TrimSpace(out.String()))
	}
}

func TestCmdRoot(t *testing.T) {
	t.Setenv("DW_ROOT", "/tmp/my-root")
	var out bytes.Buffer
	if code := cmdRoot(&out); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out.String()) != "/tmp/my-root" {
		t.Errorf("root = %q", out.String())
	}
}

func TestCmdVersion(t *testing.T) {
	var out bytes.Buffer
	if code := cmdVersion(&out); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.HasPrefix(out.String(), "dw ") {
		t.Errorf("version = %q, want it to start with 'dw '", out.String())
	}
}

func TestCmdHelp(t *testing.T) {
	var out bytes.Buffer
	if code := cmdHelp(&out); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	s := out.String()
	for _, want := range []string{"Usage:", "dw list", "dw root", "dw version", "DW_ROOT"} {
		if !strings.Contains(s, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"dw", "bogus"}, &out, &errb, time.Now())
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestRunJump(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", tmp)
	var out, errb bytes.Buffer
	// no previous workspace yet -> exit 1
	if code := run([]string{"dw", "-"}, &out, &errb, time.Now()); code != 1 {
		t.Errorf("jump with no last: exit = %d, want 1", code)
	}
}
