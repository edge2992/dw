package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"k8s pod oom":     "k8s-pod-oom",
		"  PC_Setup  ":    "pc-setup",
		"Hello, World!":   "hello-world",
		"multi   space":   "multi-space",
		"already-slugged": "already-slugged",
		"---trim---":      "trim",
		"":                "",
		"機械学習 調査":         "機械学習-調査", // unicode letters are preserved
		"PR #42 fix":      "pr-42-fix",
		"!!!":             "", // no letters/numbers -> empty
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseProject(t *testing.T) {
	p := parseProject("research", "2026-06-13-pc-setup", "/x")
	if p.Date != "2026-06-13" || p.Topic != "pc-setup" {
		t.Errorf("got date=%q topic=%q", p.Date, p.Topic)
	}
	// directory without date prefix
	p2 := parseProject("scratch", "legacy-notes", "/y")
	if p2.Date != "" || p2.Topic != "legacy-notes" {
		t.Errorf("no-date dir: got date=%q topic=%q", p2.Date, p2.Topic)
	}
}

func TestRenderTemplate(t *testing.T) {
	out := RenderTemplate(DefaultTemplate, "my-topic", "research", "2026-06-14")
	for _, want := range []string{"title: my-topic", "category: research", "created: 2026-06-14"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered template missing %q\n%s", want, out)
		}
	}
}

func TestCreateAndScan(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)

	p, err := Create(root, "research", "K8s Pod OOM", now, DefaultTemplate)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "2026-06-14-k8s-pod-oom" {
		t.Errorf("name = %q", p.Name)
	}
	readme := filepath.Join(p.Path, "README.md")
	if _, err := os.Stat(readme); err != nil {
		t.Errorf("README not created: %v", err)
	}
	// title parsed from frontmatter
	if p.Title != "k8s-pod-oom" {
		t.Errorf("title = %q", p.Title)
	}

	// second, older project to verify ordering
	older := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := Create(root, "incident", "db outage", older, DefaultTemplate); err != nil {
		t.Fatal(err)
	}

	projects, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("scan found %d projects, want 2", len(projects))
	}
	// newest (2026-06-14) first
	if projects[0].Name != "2026-06-14-k8s-pod-oom" {
		t.Errorf("ordering wrong, first = %q", projects[0].Name)
	}
}

func TestCreateDoesNotClobberReadme(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	p, _ := Create(root, "research", "topic", now, DefaultTemplate)
	custom := "EDITED BY USER"
	if err := os.WriteFile(filepath.Join(p.Path, "README.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	// re-create same project: must not overwrite
	if _, err := Create(root, "research", "topic", now, DefaultTemplate); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(p.Path, "README.md"))
	if string(b) != custom {
		t.Errorf("README was clobbered: %q", string(b))
	}
}

func TestScanMissingRoot(t *testing.T) {
	projects, err := Scan(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Errorf("missing root should not error, got %v", err)
	}
	if projects != nil {
		t.Errorf("expected nil, got %v", projects)
	}
}

func TestScanOrdersDatedBeforeUndated(t *testing.T) {
	root := t.TempDir()
	// an undated, letter-prefixed dir would float to the top under a plain
	// Name-descending sort; it must instead sink below the dated projects.
	for _, dir := range []string{
		"research/2026-06-10-older",
		"research/2026-06-14-newest",
		"research/zzz-legacy", // undated
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	projects, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-06-14-newest", "2026-06-10-older", "zzz-legacy"}
	for i, w := range want {
		if projects[i].Name != w {
			t.Errorf("order[%d] = %q, want %q", i, projects[i].Name, w)
		}
	}
}

func TestReadFrontmatter(t *testing.T) {
	dir := t.TempDir()
	readme := "---\ntitle: PC Setup\nstatus: active\ntags: [gpu, linux]\ncreated: 2026-06-13\n---\n\n# body\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	p := parseProject("research", "2026-06-13-pc", dir)
	if p.Title != "PC Setup" || p.Status != "active" || p.Tags != "[gpu, linux]" || p.Created != "2026-06-13" {
		t.Errorf("got %+v", p)
	}
}

func TestSaveAndLoadLast(t *testing.T) {
	tmp := t.TempDir()
	// cover both os.UserCacheDir backends (XDG on Linux, HOME/Library on macOS)
	// so the test stays isolated and never touches the real cache.
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", tmp)
	if got := LastPath(); got != "" {
		t.Errorf("expected empty before save, got %q", got)
	}
	dir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveLast(dir); err != nil {
		t.Fatal(err)
	}
	if got := LastPath(); got != dir {
		t.Errorf("LastPath = %q, want %q", got, dir)
	}
	// a recorded path that no longer exists reads back as empty
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if got := LastPath(); got != "" {
		t.Errorf("stale path should read empty, got %q", got)
	}
}

func TestRootEnvAndDefault(t *testing.T) {
	// explicit DW_ROOT wins
	t.Setenv("DW_ROOT", "/tmp/custom-root")
	if got := Root(); got != "/tmp/custom-root" {
		t.Errorf("Root() with DW_ROOT = %q, want /tmp/custom-root", got)
	}
	// empty DW_ROOT falls back to ~/dw
	t.Setenv("DW_ROOT", "")
	t.Setenv("HOME", "/tmp/home")
	if got := Root(); got != filepath.Join("/tmp/home", "dw") {
		t.Errorf("Root() default = %q, want /tmp/home/dw", got)
	}
}

func TestCategories(t *testing.T) {
	ps := []Project{{Category: "custom"}, {Category: "research"}}
	cats := Categories(ps)
	// defaults + custom, deduped
	want := map[string]bool{"research": true, "incident": true, "discussion": true, "scratch": true, "custom": true}
	if len(cats) != len(want) {
		t.Errorf("got %v", cats)
	}
	for _, c := range cats {
		if !want[c] {
			t.Errorf("unexpected category %q", c)
		}
	}
}
