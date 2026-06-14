// Package tui implements the interactive dw picker: one fuzzy list that both
// jumps to an existing project and creates a new one when nothing matches.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/edge2992/dw/internal/workspace"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

const windowSize = 12

var (
	selStyle    = lipgloss.NewStyle().Reverse(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	createStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	promptStyle = lipgloss.NewStyle().Bold(true)
)

type mode int

const (
	modeBrowse mode = iota
	modeCategory
)

type rowKind int

const (
	rowProject rowKind = iota
	rowCreate
)

type row struct {
	kind  rowKind
	proj  workspace.Project // valid when kind==rowProject
	label string            // for rowCreate / category rows
}

// Model is the bubbletea model for the dw picker.
type Model struct {
	root     string
	tmpl     string
	now      time.Time
	projects []workspace.Project

	mode   mode
	query  string
	cursor int

	// carried from browse into category step
	pendingTopic string
	catQuery     string

	Result string // chosen/created absolute path; empty means abort
	Err    error
}

// New builds the initial model.
func New(root, tmpl string, now time.Time, projects []workspace.Project) Model {
	return Model{root: root, tmpl: tmpl, now: now, projects: projects, mode: modeBrowse}
}

func (m Model) Init() tea.Cmd { return nil }

// rows computes the visible rows for the current mode + query.
func (m Model) rows() []row {
	if m.mode == modeBrowse {
		return browseRows(m.projects, m.query)
	}
	return categoryRows(m.projects, m.catQuery)
}

func browseRows(projects []workspace.Project, query string) []row {
	var rows []row
	if strings.TrimSpace(query) == "" {
		for _, p := range projects {
			rows = append(rows, row{kind: rowProject, proj: p})
		}
		return rows
	}
	targets := make([]string, len(projects))
	for i, p := range projects {
		targets[i] = p.Category + "/" + p.Name + " " + p.Title
	}
	for _, mt := range fuzzy.Find(query, targets) {
		rows = append(rows, row{kind: rowProject, proj: projects[mt.Index]})
	}
	rows = append(rows, row{kind: rowCreate, label: workspace.Slugify(query)})
	return rows
}

func categoryRows(projects []workspace.Project, query string) []row {
	cats := workspace.Categories(projects)
	var rows []row
	q := strings.TrimSpace(query)
	if q == "" {
		for _, c := range cats {
			rows = append(rows, row{kind: rowProject, label: c})
		}
		return rows
	}
	for _, mt := range fuzzy.Find(query, cats) {
		rows = append(rows, row{kind: rowProject, label: cats[mt.Index]})
	}
	// offer creating a brand-new category if the typed text isn't an exact match
	slug := workspace.Slugify(q)
	exact := false
	for _, c := range cats {
		if c == slug {
			exact = true
		}
	}
	if slug != "" && !exact {
		rows = append(rows, row{kind: rowCreate, label: slug})
	}
	return rows
}

func (m *Model) clampCursor(n int) {
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		m.Result = ""
		return m, tea.Quit
	case "up", "ctrl+p":
		m.cursor--
	case "down", "ctrl+n":
		m.cursor++
	case "enter":
		return m.onEnter()
	case "backspace":
		if m.mode == modeBrowse {
			m.query = trimLast(m.query)
		} else {
			m.catQuery = trimLast(m.catQuery)
		}
		m.cursor = 0
	default:
		if in := inputText(key); in != "" {
			if m.mode == modeBrowse {
				m.query += in
			} else {
				m.catQuery += in
			}
			m.cursor = 0
		}
	}
	m.clampCursor(len(m.rows()))
	return m, nil
}

func (m Model) onEnter() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		return m, nil
	}
	m.clampCursor(len(rows))
	r := rows[m.cursor]

	if m.mode == modeBrowse {
		switch r.kind {
		case rowProject:
			m.Result = r.proj.Path
			return m, tea.Quit
		case rowCreate:
			// move to category selection
			m.pendingTopic = m.query
			m.mode = modeCategory
			m.catQuery = ""
			m.cursor = 0
			return m, nil
		}
	}

	// modeCategory: r.label is the category (existing or new)
	p, err := workspace.Create(m.root, r.label, m.pendingTopic, m.now, m.tmpl)
	if err != nil {
		m.Err = err
		return m, tea.Quit
	}
	m.Result = p.Path
	return m, tea.Quit
}

func (m Model) View() string {
	var b strings.Builder
	rows := m.rows()
	m.clampCursor(len(rows))

	if m.mode == modeBrowse {
		fmt.Fprintf(&b, "%s %s\n", promptStyle.Render("discussion>"), m.query)
	} else {
		fmt.Fprintf(&b, "%s %s\n", promptStyle.Render(fmt.Sprintf("category for %q>", m.pendingTopic)), m.catQuery)
	}

	if len(rows) == 0 {
		b.WriteString(dimStyle.Render("  (該当なし — 文字を入力して新規作成)\n"))
	}

	start, end := windowBounds(m.cursor, len(rows), windowSize)
	for i := start; i < end; i++ {
		b.WriteString(m.renderRow(rows[i], i == m.cursor))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑↓ 選択  enter 決定  esc 中止"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) renderRow(r row, selected bool) string {
	var text string
	switch {
	case r.kind == rowCreate && m.mode == modeBrowse:
		text = createStyle.Render("+ 作成: ") + m.now.Format("2006-01-02") + "-" + r.label
	case r.kind == rowCreate:
		text = createStyle.Render("+ 新規カテゴリ: ") + r.label
	case m.mode == modeCategory:
		text = r.label
	default:
		p := r.proj
		date := p.Date
		if date == "" {
			date = "          "
		}
		text = fmt.Sprintf("%s  %-10s %s", date, p.Category, p.Title)
	}
	prefix := "  "
	if selected {
		prefix = "› "
		text = selStyle.Render(text)
	}
	return prefix + text + "\n"
}

func windowBounds(cursor, n, size int) (int, int) {
	if n <= size {
		return 0, n
	}
	start := cursor - size/2
	if start < 0 {
		start = 0
	}
	end := start + size
	if end > n {
		end = n
		start = end - size
	}
	return start, end
}

// inputText returns the printable text a key contributes to the query.
// Terminals may deliver several characters in one KeyMsg, so we take all runes.
func inputText(key tea.KeyMsg) string {
	switch key.Type {
	case tea.KeyRunes:
		return string(key.Runes)
	case tea.KeySpace:
		return " "
	default:
		return ""
	}
}

func trimLast(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(r[:len(r)-1])
}
