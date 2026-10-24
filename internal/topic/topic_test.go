package topic

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

func TestParseName(t *testing.T) {
	valid := []struct{ in, title, slug string }{
		{"決済API 5xx 14:05", "決済API 5xx 14:05", "決済API-5xx-14-05"},
		{"認証基盤　選定", "認証基盤 選定", "認証基盤-選定"},
		{"API Gateway vs. ALB", "API Gateway vs. ALB", "API-Gateway-vs.-ALB"},
		{"C# 資産の扱い", "C# 資産の扱い", "C-資産の扱い"},
		{"  決済API 5xx  INC-0431 ", "決済API 5xx INC-0431", "決済API-5xx-INC-0431"},
		{"10/28 アーキ定例", "10/28 アーキ定例", "10-28-アーキ定例"},
		{"what? (draft)", "what? (draft)", "what-draft"},
		{`a\b*c<d>e"f'g$h&i;j{k}l!m~n` + "`o|p^q[r]", `a\b*c<d>e"f'g$h&i;j{k}l!m~n` + "`o|p^q[r]", "a-b-c-d-e-f-g-h-i-j-k-l-m-n-o-p-q-r"},
		{"tab\there", "tab here", "tab-here"},
		{".obsidian", ".obsidian", "obsidian"},
		{"-- v2.40 --.", "-- v2.40 --.", "v2.40"},
		{"under_score", "under_score", "under_score"},
		{"Datadog", "Datadog", "Datadog"},
		{"CONFIG", "CONFIG", "CONFIG"},
		{"foo\u200b", "foo\u200b", "foo"},
		{"x\u202eyz", "x\u202eyz", "x-yz"},
		{"???", "???", ""},
		{"\u30b1\u3099ートウェイ", "ゲートウェイ", "ゲートウェイ"}, // NFD in, NFC out
	}
	for _, c := range valid {
		got, err := ParseName(c.in)
		if err != nil || got != (Name{Title: c.title, Slug: c.slug}) {
			t.Errorf("ParseName(%q) = %+v, %v; want {%q %q}", c.in, got, err, c.title, c.slug)
		}
	}

	invalid := []struct {
		in   string
		want error
	}{
		{"", ErrEmptyName},
		{"   ", ErrEmptyName},
		{"a\x01b", ErrInvalidName},
	}
	for _, c := range invalid {
		if _, err := ParseName(c.in); !errors.Is(err, c.want) {
			t.Errorf("ParseName(%q): err = %v, want %v", c.in, err, c.want)
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
		"読書を続ける",
		".git",
		".obsidian",
	)

	topics, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 4 {
		t.Fatalf("Scan returned %d topics, want 4 (dot-dirs must be skipped): %+v", len(topics), topics)
	}

	// Dated topics come first, newest date first; undated ones trail.
	wantOrder := []string{
		"2026-08-08-datadog-cost",
		"2026-08-07-zzz-topic",
		"2026-08-06-team-structure",
		"読書を続ける",
	}
	for i, name := range wantOrder {
		if topics[i].DirName != name {
			t.Errorf("topics[%d].DirName = %q, want %q (full order: %+v)", i, topics[i].DirName, name, topics)
		}
	}

	undated := topics[3]
	if undated.Date != "" || undated.Name != "読書を続ける" {
		t.Errorf("undated dir = %+v, want Date=\"\" Name=読書を続ける", undated)
	}
	if undated.MainNote() != filepath.Join(root, "読書を続ける", "読書を続ける.md") {
		t.Errorf("undated MainNote = %q, want <dir>/<dir>.md", undated.MainNote())
	}

	first := topics[0]
	if first.Date != "2026-08-08" || first.Name != "datadog-cost" {
		t.Errorf("first topic = %+v, want Date=2026-08-08 Name=datadog-cost", first)
	}
}

func TestScan_MissingRoot(t *testing.T) {
	topics, err := Scan(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Scan on a missing root should not error, got %v", err)
	}
	if topics != nil {
		t.Errorf("Scan on a missing root = %+v, want nil", topics)
	}
}

func TestCreate(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	tp, err := Create(root, "  調べ方を  設計する  ", now)
	if err != nil {
		t.Fatal(err)
	}
	if tp.DirName != "2026-10-24-調べ方を-設計する" || tp.Name != "調べ方を-設計する" || tp.Date != "2026-10-24" {
		t.Errorf("Create = %+v, want DirName=2026-10-24-調べ方を-設計する Name=調べ方を-設計する", tp)
	}

	note, err := os.ReadFile(filepath.Join(root, "2026-10-24-調べ方を-設計する", "調べ方を-設計する.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(note) != "# 調べ方を 設計する\n" {
		t.Errorf("main note = %q, want just the H1, spelled as typed", note)
	}

	entries, err := os.ReadDir(tp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("topic dir holds %d entries, want only the main note", len(entries))
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err == nil {
		t.Error("Create wrote a root CLAUDE.md; dw no longer writes anything outside the topic")
	}
}

func TestCreate_KeepsExistingNote(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	tp, err := Create(root, "alpha", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tp.MainNote(), []byte("my notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "alpha", now); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(tp.MainNote())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "my notes" {
		t.Errorf("main note was overwritten: %q", got)
	}
}

func TestCreate_InvalidName(t *testing.T) {
	root := t.TempDir()
	for in, want := range map[string]error{
		"???":        ErrEmptyName,
		"..":         ErrEmptyName,
		"-":          ErrEmptyName,
		"CON":        ErrInvalidName,
		"nul":        ErrInvalidName,
		"COM1":       ErrInvalidName,
		"COM\u00b9":  ErrInvalidName,
		"lpt9.notes": ErrInvalidName,
		"AUX?":       ErrInvalidName,
	} {
		if _, err := Create(root, in, time.Now()); !errors.Is(err, want) {
			t.Errorf("Create(%q): err = %v, want %v", in, err, want)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("root holds %d entries after rejected names, want 0", len(entries))
	}
}

func TestResolve_ExactMatch(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	created, err := Create(root, "Datadog Cost", now)
	if err != nil {
		t.Fatal(err)
	}
	// A second, older topic whose name contains the first, so only an exact
	// match can pick the right one.
	if _, err := Create(root, "Datadog Cost Review", now.AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}

	for _, arg := range []string{"Datadog Cost", "datadog cost", "2026-08-08-Datadog Cost"} {
		matches, wasCreated, err := Resolve(root, arg)
		if err != nil {
			t.Fatal(err)
		}
		if wasCreated {
			t.Errorf("Resolve(%q) created a topic; an exact match must not", arg)
		}
		if len(matches) != 1 || matches[0].Path != created.Path {
			t.Errorf("Resolve(%q) = %+v, want exactly %+v", arg, matches, created)
		}
	}
}

func TestResolve_ExistingNameOnAnotherDayIsReused(t *testing.T) {
	root := t.TempDir()
	old, err := Create(root, "読書", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	matches, wasCreated, err := Resolve(root, "読書")
	if err != nil {
		t.Fatal(err)
	}
	if wasCreated || len(matches) != 1 || matches[0].Path != old.Path {
		t.Errorf("Resolve = %+v (created=%v), want the existing topic so names stay unique", matches, wasCreated)
	}
}

func TestResolve_PartialMatchSingle(t *testing.T) {
	root := t.TempDir()
	created, err := Create(root, "Datadog コスト削減", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	matches, wasCreated, err := Resolve(root, "datadog")
	if err != nil {
		t.Fatal(err)
	}
	if wasCreated {
		t.Error("partial match should not create a new topic")
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
		t.Error("multiple partial matches should not create a new topic")
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
		t.Error("no match should create a new topic")
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %+v, want exactly 1", matches)
	}
	if _, err := os.Stat(matches[0].MainNote()); err != nil {
		t.Errorf("Resolve reported creation but the main note is missing: %v", err)
	}
}

func TestResolve_InvalidName(t *testing.T) {
	root := t.TempDir()
	for _, arg := range []string{"   ", "#?!", "nul"} {
		if _, _, err := Resolve(root, arg); err == nil {
			t.Errorf("Resolve(%q) succeeded, want a name error", arg)
		}
	}
}

func TestResolve_AbsolutePath(t *testing.T) {
	root := t.TempDir()
	created, err := Create(root, "datadog-cost", time.Now())
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

func TestTSVRow(t *testing.T) {
	tp := Topic{DirName: "2026-10-24-調べ方", Name: "調べ方", Date: "2026-10-24", Path: "/r/2026-10-24-調べ方"}
	if got, want := tp.TSVRow(""), "/r/2026-10-24-調べ方\t  2026-10-24-調べ方"; got != want {
		t.Errorf("TSVRow(\"\") = %q, want %q", got, want)
	}
	if got, want := tp.TSVRow(tp.Path), "/r/2026-10-24-調べ方\t* 2026-10-24-調べ方"; got != want {
		t.Errorf("TSVRow(own path) = %q, want %q", got, want)
	}
}

func TestPinRecent(t *testing.T) {
	ts := []Topic{{Path: "/a"}, {Path: "/b"}, {Path: "/c"}, {Path: "/d"}}
	paths := func(ts []Topic) string {
		var s []string
		for _, t := range ts {
			s = append(s, t.Path)
		}
		return strings.Join(s, ",")
	}
	if got, want := paths(PinRecent(ts, []string{"/c", "/missing", "/a"})), "/c,/a,/b,/d"; got != want {
		t.Errorf("PinRecent = %s, want %s (recent first in recent's order, the rest as they were)", got, want)
	}
	if got, want := paths(PinRecent(ts, nil)), "/a,/b,/c,/d"; got != want {
		t.Errorf("PinRecent with nothing recent = %s, want %s", got, want)
	}
}

// TestResolve_IgnoresCaseSpacesAndSeparators covers the duplicates the
// dogfooding runs made by accident: an old slug typed back as words, a
// missing space, a full-width space.
func TestResolve_IgnoresCaseSpacesAndSeparators(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	slug, err := Create(root, "datadog-cost", now.AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	spaced, err := Create(root, "認証基盤 選定", now)
	if err != nil {
		t.Fatal(err)
	}

	for arg, want := range map[string]string{
		"Datadog Cost": slug.Path,
		"datadog_cost": slug.Path,
		"認証基盤選定":       spaced.Path,
		"認証基盤　選定":      spaced.Path,
		"認証基盤   選定":    spaced.Path,
		"DATADOG  co":  slug.Path, // partial
	} {
		matches, created, err := Resolve(root, arg)
		if err != nil {
			t.Fatal(err)
		}
		if created || len(matches) != 1 || matches[0].Path != want {
			t.Errorf("Resolve(%q) = %+v (created=%v), want only %s", arg, matches, created, filepath.Base(want))
		}
	}
}

// TestResolve_FindsExistingNamesSpelledDifferently: topics made before names
// were rewritten keep their spaces and symbols, and macOS tools can store a
// name decomposed (NFD). Typing either name the usual way must open them, not
// create a twin, even when the name is one a new topic could not take.
func TestResolve_FindsExistingNamesSpelledDifferently(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "2026-01-01-what? (draft)", "2026-01-02-\u30b1\u3099ートウェイ",
		"2026-01-03-API Gateway vs.", "2026-01-04-API Gateway vs ALB", "2026-01-05-CON", "2026-01-06-!!!")

	for arg, want := range map[string]string{
		"what? (draft)":   "2026-01-01-what? (draft)",
		"what-draft":      "2026-01-01-what? (draft)",
		"ゲートウェイ":          "2026-01-02-\u30b1\u3099ートウェイ",
		"API Gateway vs.": "2026-01-03-API Gateway vs.",
		"CON":             "2026-01-05-CON",
		"!!!":             "2026-01-06-!!!",
	} {
		matches, created, err := Resolve(root, arg)
		if err != nil || created || len(matches) != 1 || matches[0].DirName != want {
			t.Errorf("Resolve(%q) = %+v (created=%v, err=%v), want only %s", arg, matches, created, err, want)
		}
		if tp, created, err := New(root, arg); err != nil || created || tp.DirName != want {
			t.Errorf("New(%q) = %+v (created=%v, err=%v), want %s", arg, tp, created, err, want)
		}
	}
}

// TestNew_CreatesDespitePartialMatch is the umbrella-topic case: Resolve can
// never create a name that is part of an existing one, New can.
func TestNew_CreatesDespitePartialMatch(t *testing.T) {
	root := t.TempDir()
	meeting, err := Create(root, "アーキ定例 10-28", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	umbrella, created, err := New(root, "アーキ定例")
	if err != nil {
		t.Fatal(err)
	}
	if !created || umbrella.Name != "アーキ定例" {
		t.Errorf("New(アーキ定例) = %+v (created=%v), want a new umbrella topic", umbrella, created)
	}

	again, created, err := New(root, "アーキ定例10-28")
	if err != nil {
		t.Fatal(err)
	}
	if created || again.Path != meeting.Path {
		t.Errorf("New on an existing name = %+v (created=%v), want the existing %s", again, created, meeting.DirName)
	}

	if _, _, err := New(root, "PRN"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("New(PRN): err = %v, want ErrInvalidName", err)
	}
}
