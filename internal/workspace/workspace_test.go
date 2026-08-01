package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testTemplates is what Create receives when a test doesn't care about template
// contents — the built-ins, exactly as the real callers resolve them.
func testTemplates(category string) Templates {
	return Templates{README: DefaultTemplate, ClaudeMD: DefaultClaudeTemplate(category)}
}

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

	p, err := Create(root, "research", "K8s Pod OOM", now, testTemplates("research"))
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
	// dir name is slugified, but the title keeps the topic as typed
	if p.Title != "K8s Pod OOM" {
		t.Errorf("title = %q", p.Title)
	}

	// second, older project to verify ordering
	older := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := Create(root, "incident", "db outage", older, testTemplates("incident")); err != nil {
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

func TestCreateTitleKeepsTypedTopic(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)

	// casing and spaces survive into the README title, while the dir is slugged
	p, err := Create(root, "research", "Rust GC 設計メモ", now, testTemplates("research"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "2026-06-14-rust-gc-設計メモ" {
		t.Errorf("dir name = %q", p.Name)
	}
	if p.Title != "Rust GC 設計メモ" {
		t.Errorf("title = %q, want the topic as typed", p.Title)
	}

	// symbol-only input has no slug, so both dir and title fall back to "untitled"
	p2, err := Create(root, "research", "!!!", now, testTemplates("research"))
	if err != nil {
		t.Fatal(err)
	}
	if p2.Name != "2026-06-14-untitled" || p2.Title != "untitled" {
		t.Errorf("symbol-only: name=%q title=%q", p2.Name, p2.Title)
	}
}

func TestCreateDoesNotClobberReadme(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	p, _ := Create(root, "research", "topic", now, testTemplates("research"))
	custom := "EDITED BY USER"
	if err := os.WriteFile(filepath.Join(p.Path, "README.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	// re-create same project: must not overwrite
	if _, err := Create(root, "research", "topic", now, testTemplates("research")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(p.Path, "README.md"))
	if string(b) != custom {
		t.Errorf("README was clobbered: %q", string(b))
	}
}

func TestCreateWritesClaudeMD(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	p, err := Create(root, "research", "K8s Pod OOM", now, testTemplates("research"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p.Path, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not created: %v", err)
	}
	got := string(b)
	want := RenderTemplate(DefaultClaudeTemplate("research"), "K8s Pod OOM", "research", "2026-06-14")
	if got != want {
		t.Errorf("CLAUDE.md = %q, want %q", got, want)
	}
	if strings.Contains(got, "{{") {
		t.Errorf("placeholder left unrendered:\n%s", got)
	}
}

func TestCreateDoesNotClobberClaudeMD(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	p, _ := Create(root, "research", "topic", now, testTemplates("research"))
	custom := "EDITED BY USER"
	claude := filepath.Join(p.Path, "CLAUDE.md")
	if err := os.WriteFile(claude, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "research", "topic", now, testTemplates("research")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(claude)
	if string(b) != custom {
		t.Errorf("CLAUDE.md was clobbered: %q", string(b))
	}
}

func TestEnsureClaudeMD(t *testing.T) {
	root := t.TempDir()
	// a workspace as it looked before dw scaffolded CLAUDE.md: README only
	dir := filepath.Join(root, "research", "2026-06-14-legacy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	readme := "---\ntitle: Legacy Notes\n---\n\n# body\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	projects, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("scan found %d projects, want 1", len(projects))
	}

	tmpl := DefaultClaudeTemplate("research")
	wrote, err := EnsureClaudeMD(projects[0], tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("EnsureClaudeMD reported no write for a missing CLAUDE.md")
	}
	b, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not written: %v", err)
	}
	// rendered from the frontmatter title and the dir's date prefix
	if want := RenderTemplate(tmpl, "Legacy Notes", "research", "2026-06-14"); string(b) != want {
		t.Errorf("CLAUDE.md = %q, want %q", string(b), want)
	}

	// re-running is a no-op, and never touches a file the user has edited
	custom := "EDITED BY USER"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err = EnsureClaudeMD(projects[0], tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("EnsureClaudeMD reported a write for an existing CLAUDE.md")
	}
	b, _ = os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(b) != custom {
		t.Errorf("CLAUDE.md was clobbered: %q", string(b))
	}
}

func TestEnsureClaudeMDUndatedDirUsesCreated(t *testing.T) {
	root := t.TempDir()
	// no date prefix, so the date has to come from the frontmatter instead
	dir := filepath.Join(root, "scratch", "legacy-notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	readme := "---\ntitle: Legacy\ncreated: 2026-01-02\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	projects, _ := Scan(root)
	tmpl := "{{title}}|{{category}}|{{date}}"
	if _, err := EnsureClaudeMD(projects[0], tmpl); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if want := "Legacy|scratch|2026-01-02"; string(b) != want {
		t.Errorf("CLAUDE.md = %q, want %q", string(b), want)
	}
}

func TestEnsureClaudeMDDoesNotFollowDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.md")
	claude := filepath.Join(dir, "CLAUDE.md")
	if err := os.Symlink(target, claude); err != nil {
		t.Fatal(err)
	}
	p := Project{Path: dir, Title: "Legacy", Category: "research"}

	wrote, err := EnsureClaudeMD(p, "scaffolded")
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("EnsureClaudeMD reported a write for an existing symlink")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("dangling symlink target was written: %v", err)
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

func TestCategories(t *testing.T) {
	ps := []Project{{Category: "custom"}, {Category: "research"}}
	cats := Categories(DefaultCategories, ps)
	// base defaults + custom, deduped
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

func TestCategoriesReplacesBase(t *testing.T) {
	// a config-provided base replaces the defaults wholesale, in order, and
	// on-disk extras still get appended (sorted) after it.
	ps := []Project{{Category: "zeta"}, {Category: "alpha"}}
	cats := Categories([]string{"foo", "bar", "foo"}, ps)
	want := []string{"foo", "bar", "alpha", "zeta"}
	if len(cats) != len(want) {
		t.Fatalf("cats = %v, want %v", cats, want)
	}
	for i := range want {
		if cats[i] != want[i] {
			t.Errorf("cats[%d] = %q, want %q (full %v)", i, cats[i], want[i], cats)
		}
	}
}
