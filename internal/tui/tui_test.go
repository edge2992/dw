package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edge2992/dw/internal/workspace"

	tea "github.com/charmbracelet/bubbletea"
)

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
	m := New("/d", workspace.DefaultTemplate, time.Now(), projects)

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
	m := New("/d", workspace.DefaultTemplate, time.Now(), projects)
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
	m := New(root, workspace.DefaultTemplate, now, nil)

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
	m := New(root, workspace.DefaultTemplate, now, nil)

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
	m := New(root, workspace.DefaultTemplate, now, nil)
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
	m := New("/d", workspace.DefaultTemplate, time.Now(),
		[]workspace.Project{{Path: "/d/x", Name: "n"}})
	m = send(m, "esc")
	if m.Result != "" {
		t.Fatalf("esc should abort, Result = %q", m.Result)
	}
}
