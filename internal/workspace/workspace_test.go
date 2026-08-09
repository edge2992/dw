package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRoot_AlwaysAbsolute(t *testing.T) {
	t.Run("relative DW_ROOT is made absolute", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("DW_ROOT", "some-relative-dir")
		want := filepath.Join(wd, "some-relative-dir")
		if got := Root(); got != want {
			t.Errorf("Root() = %q, want %q", got, want)
		}
		if !filepath.IsAbs(Root()) {
			t.Errorf("Root() = %q, want an absolute path", Root())
		}
	})

	t.Run("absolute DW_ROOT is left as-is", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "root")
		t.Setenv("DW_ROOT", abs)
		if got := Root(); got != abs {
			t.Errorf("Root() = %q, want %q", got, abs)
		}
	})

	t.Run("unset DW_ROOT falls back to ~/dw, absolute", func(t *testing.T) {
		t.Setenv("DW_ROOT", "")
		home := t.TempDir()
		t.Setenv("HOME", home)
		want := filepath.Join(home, "dw")
		if got := Root(); got != want {
			t.Errorf("Root() = %q, want %q", got, want)
		}
	})
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Datadog Cost", "datadog-cost"},
		{"  spaced out  ", "spaced-out"},
		{"under_score and-dash", "under-score-and-dash"},
		{"!!!", ""},
		{"", ""},
		{"機械学習 調査", "機械学習-調査"},
		{"a---b", "a-b"},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root,
		"2026-08-06-team-structure",
		"2026-08-08-datadog-cost",
		"2026-08-07-zzz-topic",
		"no-date-topic",
		".git",
		".obsidian",
	)

	projects, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 4 {
		t.Fatalf("Scan returned %d projects, want 4 (dot-dirs must be skipped): %+v", len(projects), projects)
	}

	// Dated projects come first, newest date first; undated ones trail.
	wantOrder := []string{
		"2026-08-08-datadog-cost",
		"2026-08-07-zzz-topic",
		"2026-08-06-team-structure",
		"no-date-topic",
	}
	for i, name := range wantOrder {
		if projects[i].Name != name {
			t.Errorf("projects[%d].Name = %q, want %q (full order: %+v)", i, projects[i].Name, name, projects)
		}
	}

	last := projects[3]
	if last.Date != "" {
		t.Errorf("undated dir got Date = %q, want \"\"", last.Date)
	}
	if last.Topic != "no-date-topic" {
		t.Errorf("undated dir Topic = %q, want %q", last.Topic, "no-date-topic")
	}

	first := projects[0]
	if first.Date != "2026-08-08" || first.Topic != "datadog-cost" {
		t.Errorf("first project = %+v, want Date=2026-08-08 Topic=datadog-cost", first)
	}
}

func TestScan_MissingRoot(t *testing.T) {
	projects, err := Scan(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Scan on a missing root should not error, got %v", err)
	}
	if projects != nil {
		t.Errorf("Scan on a missing root = %+v, want nil", projects)
	}
}

func TestCreate(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

	p, err := Create(root, "  Datadog Cost  ", now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "2026-08-08-datadog-cost" {
		t.Errorf("Name = %q, want 2026-08-08-datadog-cost", p.Name)
	}
	if p.Topic != "datadog-cost" || p.Date != "2026-08-08" {
		t.Errorf("Topic/Date = %q/%q, want datadog-cost/2026-08-08", p.Topic, p.Date)
	}

	state, err := os.ReadFile(filepath.Join(p.Path, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(string(state), "# Datadog Cost") {
		t.Errorf("STATE.md title should be the raw (trimmed) topic %q, got:\n%s", "# Datadog Cost", state)
	}

	// The root CLAUDE.md convention file is written alongside the first workspace.
	rootClaude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("root CLAUDE.md was not written: %v", err)
	}
	if len(rootClaude) == 0 {
		t.Error("root CLAUDE.md is empty")
	}

	// Creating a second workspace must not touch an existing root CLAUDE.md.
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "another topic", now); err != nil {
		t.Fatal(err)
	}
	rootClaude, err = os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rootClaude) != "custom" {
		t.Errorf("root CLAUDE.md was overwritten: got %q", rootClaude)
	}
}

func TestCreate_EmptyTopic(t *testing.T) {
	root := t.TempDir()
	if _, err := Create(root, "   ", time.Now()); !errors.Is(err, ErrEmptyTopic) {
		t.Errorf("Create with blank topic: err = %v, want ErrEmptyTopic", err)
	}
}

func containsLine(s, line string) bool {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimRight(l, "\r") == line {
			return true
		}
	}
	return false
}

func TestResolve_ExactMatch(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	created, err := Create(root, "Datadog Cost", now)
	if err != nil {
		t.Fatal(err)
	}
	// A second, older workspace with a different topic so it can't accidentally match.
	if _, err := Create(root, "Team Structure", now.AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}

	matches, wasCreated, err := Resolve(root, "Datadog Cost")
	if err != nil {
		t.Fatal(err)
	}
	if wasCreated {
		t.Error("exact match should not create a new workspace")
	}
	if len(matches) != 1 || matches[0].Path != created.Path {
		t.Errorf("matches = %+v, want exactly %+v", matches, created)
	}
}

func TestResolve_PartialMatchSingle(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	created, err := Create(root, "datadog-cost-reduction", now)
	if err != nil {
		t.Fatal(err)
	}

	matches, wasCreated, err := Resolve(root, "datadog")
	if err != nil {
		t.Fatal(err)
	}
	if wasCreated {
		t.Error("partial match should not create a new workspace")
	}
	if len(matches) != 1 || matches[0].Path != created.Path {
		t.Errorf("matches = %+v, want exactly %+v", matches, created)
	}
}

func TestResolve_PartialMatchMultiple(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	if _, err := Create(root, "datadog-cost", now); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "datadog-alerts", now.AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}

	matches, wasCreated, err := Resolve(root, "datadog")
	if err != nil {
		t.Fatal(err)
	}
	if wasCreated {
		t.Error("multiple partial matches should not create a new workspace")
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %+v, want 2 results", matches)
	}
}

func TestResolve_NoMatchCreates(t *testing.T) {
	root := t.TempDir()

	matches, wasCreated, err := Resolve(root, "brand new topic")
	if err != nil {
		t.Fatal(err)
	}
	if !wasCreated {
		t.Error("no match should create a new workspace")
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %+v, want exactly 1", matches)
	}
	if _, err := os.Stat(matches[0].Path); err != nil {
		t.Errorf("Resolve reported creation but the directory is missing: %v", err)
	}
}

func TestResolve_EmptySlug(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Resolve(root, "   !!!   "); !errors.Is(err, ErrEmptyTopic) {
		t.Errorf("Resolve with an unslugifiable topic: err = %v, want ErrEmptyTopic", err)
	}
}

func TestResolve_AbsolutePath(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	created, err := Create(root, "datadog-cost", now)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("existing", func(t *testing.T) {
		matches, wasCreated, err := Resolve(root, created.Path)
		if err != nil {
			t.Fatal(err)
		}
		if wasCreated {
			t.Error("resolving an existing absolute path should not create anything")
		}
		if len(matches) != 1 || matches[0].Path != created.Path {
			t.Errorf("matches = %+v, want exactly %+v", matches, created)
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		bogus := filepath.Join(root, "2099-01-01-does-not-exist")
		_, _, err := Resolve(root, bogus)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Resolve(%q): err = %v, want ErrNotFound", bogus, err)
		}
		if _, statErr := os.Stat(bogus); statErr == nil {
			t.Error("Resolve must not create a directory for an unresolved absolute path")
		}
	})
}
