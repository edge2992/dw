package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edge2992/dw/internal/config"
	"github.com/edge2992/dw/internal/workspace"
)

// testCfg builds a Config rooted at root, with a templates dir that does not
// exist (so ResolveTemplate falls back to the built-in DefaultTemplate) and the
// default categories. It mirrors what config.Load would resolve, minus the file.
func testCfg(root string) config.Config {
	return config.Config{
		Root:         root,
		TemplatesDir: filepath.Join(root, "no-templates"),
		Categories:   workspace.DefaultCategories,
	}
}

// seed creates two projects under a temp root and returns a Config pointing at it.
func seed(t *testing.T) config.Config {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := workspace.Create(root, "research", "k8s pod oom", now, workspace.DefaultTemplate); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Create(root, "incident", "db outage", older, workspace.DefaultTemplate); err != nil {
		t.Fatal(err)
	}
	return testCfg(root)
}

func TestCmdList(t *testing.T) {
	cfg := seed(t)
	var out, errb bytes.Buffer
	if code := cmdList(cfg, &out, &errb, nil); code != 0 {
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
	cfg := seed(t)
	var out, errb bytes.Buffer
	if code := cmdList(cfg, &out, &errb, []string{"--json"}); code != 0 {
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
	cfg := testCfg(t.TempDir()) // empty, existing dir
	var out, errb bytes.Buffer
	if code := cmdList(cfg, &out, &errb, []string{"--json"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("empty JSON = %q, want []", strings.TrimSpace(out.String()))
	}
}

func TestCmdListRejectsExtraArg(t *testing.T) {
	cfg := testCfg(t.TempDir())
	var out, errb bytes.Buffer
	if code := cmdList(cfg, &out, &errb, []string{"bogus"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty, got %q", out.String())
	}
	if !strings.Contains(errb.String(), "unexpected argument") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestCmdRoot(t *testing.T) {
	cfg := config.Config{Root: "/tmp/my-root"}
	var out bytes.Buffer
	if code := cmdRoot(cfg, &out); code != 0 {
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
	for _, want := range []string{"Usage:", "dw new", "dw list", "dw root", "dw config", "dw init", "dw version", "DW_CONFIG", "config.yml"} {
		if !strings.Contains(s, want) {
			t.Errorf("help missing %q", want)
		}
	}
	if strings.Contains(s, "DW_ROOT") {
		t.Errorf("help should no longer mention DW_ROOT:\n%s", s)
	}
}

// newEnv returns a Config rooted at a fresh temp dir and points the cache
// (HOME/XDG_CACHE_HOME) at a separate one, so cmdNew can create under the root
// and persist the "last" pin without the cache leaking into the root and
// muddying "nothing created" assertions.
func newEnv(t *testing.T) config.Config {
	t.Helper()
	root := t.TempDir()
	cache := t.TempDir()
	t.Setenv("HOME", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	return testCfg(root)
}

func TestCmdNew(t *testing.T) {
	cfg := newEnv(t)
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	var out, errb bytes.Buffer
	if code := cmdNew(cfg, &out, &errb, []string{"--category", "research", "my topic"}, now); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	path := strings.TrimSpace(out.String())
	if !strings.HasSuffix(path, "research/2026-06-20-my-topic") {
		t.Fatalf("path = %q, want it to end with research/2026-06-20-my-topic", path)
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		t.Fatalf("workspace dir not created at %q: %v", path, err)
	}
	if _, err := os.Stat(filepath.Join(path, "README.md")); err != nil {
		t.Errorf("README.md not created: %v", err)
	}
	// SaveLast ran, so `dw -` resolves to the same path.
	var jout, jerr bytes.Buffer
	if code := cmdJump(&jout, &jerr); code != 0 {
		t.Fatalf("jump exit = %d, stderr = %s", code, jerr.String())
	}
	if strings.TrimSpace(jout.String()) != path {
		t.Errorf("jump = %q, want %q", jout.String(), path)
	}
}

func TestCmdNewTopicAfterFlag(t *testing.T) {
	cfg := newEnv(t)
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	var out, errb bytes.Buffer
	// topic before the flag must still parse (order-independent).
	if code := cmdNew(cfg, &out, &errb, []string{"my topic", "--category", "research"}, now); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.HasSuffix(strings.TrimSpace(out.String()), "research/2026-06-20-my-topic") {
		t.Errorf("path = %q", out.String())
	}
}

func TestCmdNewMissingCategory(t *testing.T) {
	cfg := newEnv(t)
	var out, errb bytes.Buffer
	if code := cmdNew(cfg, &out, &errb, []string{"my topic"}, time.Now()); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty, got %q", out.String())
	}
	if !strings.Contains(errb.String(), "category") {
		t.Errorf("stderr = %q, want it to mention category", errb.String())
	}
}

func TestCmdNewMissingTopic(t *testing.T) {
	cfg := newEnv(t)
	var out, errb bytes.Buffer
	if code := cmdNew(cfg, &out, &errb, []string{"--category", "research"}, time.Now()); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "topic") {
		t.Errorf("stderr = %q, want it to mention topic", errb.String())
	}
}

func TestCmdNewSlugifiesCategory(t *testing.T) {
	cfg := newEnv(t)
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	var out, errb bytes.Buffer
	if code := cmdNew(cfg, &out, &errb, []string{"hello", "--category", "My Cat"}, now); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	// The picker slugifies new category names before Create; cmdNew must match,
	// so a "My Cat" category lands in my-cat/ rather than a divergent "My Cat/".
	want := filepath.Join(cfg.Root, "my-cat", "2026-06-20-hello")
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Errorf("expected dir %q: %v", want, err)
	}
}

func TestCmdNewUnslugifiableTopic(t *testing.T) {
	cfg := newEnv(t)
	var out, errb bytes.Buffer
	// "!!!" slugifies to "", which the picker refuses to create — cmdNew must too.
	if code := cmdNew(cfg, &out, &errb, []string{"!!!", "--category", "research"}, time.Now()); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "topic") {
		t.Errorf("stderr = %q, want it to mention topic", errb.String())
	}
	if entries, _ := os.ReadDir(cfg.Root); len(entries) != 0 {
		t.Errorf("nothing should be created, got %v", entries)
	}
}

func TestCmdNewUnslugifiableCategory(t *testing.T) {
	cfg := newEnv(t)
	var out, errb bytes.Buffer
	if code := cmdNew(cfg, &out, &errb, []string{"hello", "--category", "!!!"}, time.Now()); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "category") {
		t.Errorf("stderr = %q, want it to mention category", errb.String())
	}
	if entries, _ := os.ReadDir(cfg.Root); len(entries) != 0 {
		t.Errorf("nothing should be created, got %v", entries)
	}
}

func TestCmdConfigPath(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom.yml")
	t.Setenv("DW_CONFIG", want)
	var out, errb bytes.Buffer
	if code := cmdConfig(&out, &errb, []string{"path"}); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestCmdConfigInitWritesAndIsNonDestructive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "config.yml")
	t.Setenv("DW_CONFIG", p)

	// first init writes the starter config (creating parent dirs)
	var out, errb bytes.Buffer
	if code := cmdConfig(&out, &errb, []string{"init"}); code != 0 {
		t.Fatalf("init exit = %d, stderr = %s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != p {
		t.Errorf("init stdout = %q, want %q", out.String(), p)
	}
	first, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}

	// second init must not clobber the (hand-edited) file
	if err := os.WriteFile(p, []byte("root: /edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := cmdConfig(&out, &errb, []string{"init"}); code != 0 {
		t.Fatalf("second init exit = %d", code)
	}
	again, _ := os.ReadFile(p)
	if string(again) != "root: /edited\n" {
		t.Errorf("init clobbered an existing config: %q", string(again))
	}
	if !strings.Contains(errb.String(), "already exists") {
		t.Errorf("expected an 'already exists' notice, got %q", errb.String())
	}
	_ = first
}

func TestCmdConfigUnknownSub(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdConfig(&out, &errb, []string{"frobnicate"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown subcommand") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestCmdInit(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		var out, errb bytes.Buffer
		if code := cmdInit(&out, &errb, []string{shell}); code != 0 {
			t.Fatalf("%s: exit = %d, stderr = %s", shell, code, errb.String())
		}
		s := out.String()
		for _, want := range []string{"dw()", "cd ", "command dw", "new"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: init output missing %q\n%s", shell, want, s)
			}
		}
	}
}

func TestCmdInitUnsupported(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdInit(&out, &errb, []string{"fish"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty, got %q", out.String())
	}
	if !strings.Contains(errb.String(), "unsupported") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestCmdInitNoShell(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdInit(&out, &errb, nil); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Setenv("DW_CONFIG", filepath.Join(t.TempDir(), "none.yml")) // hermetic: no host config
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
	t.Setenv("DW_CONFIG", filepath.Join(tmp, "none.yml"))
	var out, errb bytes.Buffer
	// no previous workspace yet -> exit 1
	if code := run([]string{"dw", "-"}, &out, &errb, time.Now()); code != 1 {
		t.Errorf("jump with no last: exit = %d, want 1", code)
	}
}

// TestRunRootFromConfig is the end-to-end check that `dw root` reflects the
// config file located via DW_CONFIG, with ~ / $ENV expanded.
func TestRunRootFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("root: $DWTEST_BASE/ws\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DW_CONFIG", cfgPath)
	t.Setenv("DWTEST_BASE", dir)

	var out, errb bytes.Buffer
	if code := run([]string{"dw", "root"}, &out, &errb, time.Now()); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	want := filepath.Join(dir, "ws")
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("root = %q, want %q", got, want)
	}
}
