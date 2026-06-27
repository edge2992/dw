package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edge2992/dw/internal/workspace"

	tea "github.com/charmbracelet/bubbletea"
)

// defaultCats is the category list the picker offers in tests that exercise the
// create flow; it mirrors the config default (workspace.DefaultCategories).
var defaultCats = workspace.DefaultCategories

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func send(m Model, keys ...string) Model {
	for _, k := range keys {
		updated, _ := m.Update(key(k))
		m = updated.(Model)
	}
	return m
}

func typeStr(m Model, s string) Model {
	for _, r := range s {
		m = send(m, string(r))
	}
	return m
}

func TestJumpToExistingProject(t *testing.T) {
	projects := []workspace.Project{
		{Category: "research", Name: "2026-06-13-pc-setup", Topic: "pc-setup", Date: "2026-06-13", Title: "pc-setup", Path: "/d/research/2026-06-13-pc-setup"},
		{Category: "incident", Name: "2026-06-10-db", Topic: "db", Date: "2026-06-10", Title: "db", Path: "/d/incident/2026-06-10-db"},
	}
	m := New("/d", time.Now(), projects, "", nil, "")

	// empty query, enter selects first (newest) project
	m = send(m, "enter")
	if m.Result != "/d/research/2026-06-13-pc-setup" {
		t.Fatalf("Result = %q", m.Result)
	}
}

func TestFilterThenJump(t *testing.T) {
	projects := []workspace.Project{
		{Category: "research", Name: "2026-06-13-pc-setup", Title: "pc-setup", Path: "/d/a"},
		{Category: "incident", Name: "2026-06-10-db-outage", Title: "db-outage", Path: "/d/b"},
	}
	m := New("/d", time.Now(), projects, "", nil, "")
	m = typeStr(m, "outage")
	// first fuzzy row should be the db-outage project; enter jumps
	m = send(m, "enter")
	if m.Result != "/d/b" {
		t.Fatalf("Result = %q", m.Result)
	}
}

func TestCreateFlow(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "", defaultCats, "")

	// type a topic with no existing match
	m = typeStr(m, "new idea")
	rows := m.rows()
	if len(rows) == 0 || rows[len(rows)-1].kind != rowCreate {
		t.Fatalf("expected a create row, got %+v", rows)
	}
	// move cursor to the create row (last) and confirm -> enters category mode
	for i := 0; i < len(rows)-1; i++ {
		m = send(m, "down")
	}
	m = send(m, "enter")
	if m.mode != modeCategory {
		t.Fatalf("expected category mode")
	}
	// pick the first category (research) and confirm -> creates
	m = send(m, "enter")
	want := filepath.Join(root, "research", "2026-06-14-new-idea")
	if m.Result != want {
		t.Fatalf("Result = %q, want %q", m.Result, want)
	}
	if _, err := os.Stat(filepath.Join(want, "README.md")); err != nil {
		t.Fatalf("README not created: %v", err)
	}
}

func TestNewCategoryCreation(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "", defaultCats, "")

	m = typeStr(m, "topic")
	// jump to create row
	rows := m.rows()
	for i := 0; i < len(rows)-1; i++ {
		m = send(m, "down")
	}
	m = send(m, "enter") // -> category mode

	// type a brand new category name
	m = typeStr(m, "spike")
	crows := m.rows()
	if crows[len(crows)-1].kind != rowCreate {
		t.Fatalf("expected new-category create row, got %+v", crows)
	}
	for i := 0; i < len(crows)-1; i++ {
		m = send(m, "down")
	}
	m = send(m, "enter")
	want := filepath.Join(root, "spike", "2026-06-14-topic")
	if m.Result != want {
		t.Fatalf("Result = %q, want %q", m.Result, want)
	}
}

func TestMultiRuneInput(t *testing.T) {
	// terminals can deliver several chars in a single KeyMsg
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "", nil, "")
	m = send(m, "ptytest") // one KeyMsg carrying 7 runes
	if m.query != "ptytest" {
		t.Fatalf("query = %q, want ptytest", m.query)
	}
	rows := m.rows()
	if rows[len(rows)-1].kind != rowCreate || rows[len(rows)-1].label != "ptytest" {
		t.Fatalf("expected create row labelled ptytest, got %+v", rows)
	}
}

func TestBrowseCreateRowMatchesSlug(t *testing.T) {
	// symbol-only query yields no slug -> no create row offered
	for _, r := range browseRows(nil, "!!!") {
		if r.kind == rowCreate {
			t.Fatal("create row should not appear for an empty-slug query")
		}
	}
	// japanese query is preserved, and the create label equals the slug that
	// Create will actually use for the directory name
	jp := browseRows(nil, "機械学習 調査")
	if len(jp) != 1 || jp[0].kind != rowCreate {
		t.Fatalf("expected one create row, got %+v", jp)
	}
	if want := workspace.Slugify("機械学習 調査"); jp[0].label != want {
		t.Fatalf("create label = %q, want %q", jp[0].label, want)
	}
}

func TestEscAborts(t *testing.T) {
	m := New("/d", time.Now(),
		[]workspace.Project{{Path: "/d/x", Name: "n"}}, "", nil, "")
	m = send(m, "esc")
	if m.Result != "" {
		t.Fatalf("esc should abort, Result = %q", m.Result)
	}
}

func TestEscFromCategoryReturnsToBrowse(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "", defaultCats, "")

	// reach category mode via the create row
	m = typeStr(m, "topic")
	rows := m.rows()
	for i := 0; i < len(rows)-1; i++ {
		m = send(m, "down")
	}
	m = send(m, "enter")
	if m.mode != modeCategory {
		t.Fatalf("expected category mode")
	}

	// esc steps back to browse, keeping the typed topic, without aborting
	m = send(m, "esc")
	if m.mode != modeBrowse {
		t.Fatalf("esc should return to browse, mode = %v", m.mode)
	}
	if m.Result != "" {
		t.Fatalf("esc to browse must not set Result, got %q", m.Result)
	}
	if m.query != "topic" {
		t.Fatalf("query should be preserved, got %q", m.query)
	}
}

func TestLastPathPinnedToTop(t *testing.T) {
	projects := []workspace.Project{
		{Name: "2026-06-14-newest", Date: "2026-06-14", Title: "newest", Path: "/d/a/2026-06-14-newest"},
		{Name: "2026-06-10-older", Date: "2026-06-10", Title: "older", Path: "/d/b/2026-06-10-older"},
	}
	// pin the older project; empty query + enter must resume it, not the newest
	m := New("/d", time.Now(), projects, "/d/b/2026-06-10-older", nil, "")
	m = send(m, "enter")
	if m.Result != "/d/b/2026-06-10-older" {
		t.Fatalf("pinned project not first, Result = %q", m.Result)
	}
}

func TestLastPathUnmatchedIsNoop(t *testing.T) {
	projects := []workspace.Project{
		{Name: "2026-06-14-newest", Date: "2026-06-14", Path: "/d/a/2026-06-14-newest"},
		{Name: "2026-06-10-older", Date: "2026-06-10", Path: "/d/b/2026-06-10-older"},
	}
	// a stale lastPath that no longer matches must leave ordering untouched
	m := New("/d", time.Now(), projects, "/d/gone", nil, "")
	m = send(m, "enter")
	if m.Result != "/d/a/2026-06-14-newest" {
		t.Fatalf("unmatched pin should be a no-op, Result = %q", m.Result)
	}
}

func TestPinMarkerAndHint(t *testing.T) {
	projects := []workspace.Project{
		{Name: "2026-06-14-newest", Date: "2026-06-14", Title: "newest", Path: "/d/a/2026-06-14-newest"},
		{Name: "2026-06-10-older", Date: "2026-06-10", Title: "older", Path: "/d/b/2026-06-10-older"},
	}
	// with a matching pin, the marker and the shell hint both show
	m := New("/d", time.Now(), projects, "/d/b/2026-06-10-older", nil, "")
	view := m.View()
	if !strings.Contains(view, "← last") {
		t.Errorf("expected pin marker in view:\n%s", view)
	}
	if !strings.Contains(view, "dw -") {
		t.Errorf("expected dw - hint in view:\n%s", view)
	}

	// without a pin, neither the marker nor the hint appear
	m2 := New("/d", time.Now(), projects, "", nil, "")
	view2 := m2.View()
	if strings.Contains(view2, "← last") || strings.Contains(view2, "dw -") {
		t.Errorf("unpinned view should have no marker/hint:\n%s", view2)
	}
}

func TestMetaLine(t *testing.T) {
	got := metaLine(workspace.Project{Status: "active", Tags: "[gpu, linux]", Created: "2026-06-13"})
	want := "status:active  tags:[gpu, linux]  created:2026-06-13"
	if got != want {
		t.Fatalf("metaLine = %q, want %q", got, want)
	}
	// empty tags ("[]") are dropped; created falls back to the dir Date
	got = metaLine(workspace.Project{Status: "done", Tags: "[]", Date: "2026-01-01"})
	if want := "status:done  created:2026-01-01"; got != want {
		t.Fatalf("metaLine fallback = %q, want %q", got, want)
	}
	if got := metaLine(workspace.Project{}); got != "" {
		t.Fatalf("empty project should yield empty meta, got %q", got)
	}
}

func TestCreateUsesCategoryTemplate(t *testing.T) {
	tmplDir := t.TempDir()
	cat := defaultCats[0] // first default category offered
	if err := os.WriteFile(filepath.Join(tmplDir, cat+".md"), []byte("CATEGORY BODY"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "", defaultCats, tmplDir)

	// browse: type a topic, move to the create row
	m = typeStr(m, "topic")
	rows := m.rows()
	for i := 0; i < len(rows)-1; i++ {
		m = send(m, "down")
	}
	m = send(m, "enter") // -> category mode
	// confirm the first default category (cat)
	m = send(m, "enter")

	if m.Result == "" {
		t.Fatalf("create did not produce a Result path (Err=%v)", m.Err)
	}

	b, err := os.ReadFile(filepath.Join(m.Result, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if got != "CATEGORY BODY" {
		t.Fatalf("README = %q, want category template body", got)
	}
}
