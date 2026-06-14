// Package tui implements the interactive dw picker: one fuzzy list that both
// jumps to an existing project and creates a new one when nothing matches.
package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/edge2992/dw/internal/workspace"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/sahilm/fuzzy"
)

const windowSize = 12

var (
	selStyle    = lipgloss.NewStyle().Reverse(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	createStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	pinStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
	promptStyle = lipgloss.NewStyle().Bold(true)
)

type mode int

const (
	modeBrowse mode = iota
	modeCategory
)

type rowKind int

const (
	rowProject  rowKind = iota // jump target; proj is set
	rowCategory                // category choice; label is the category name
	rowCreate                  // "create new" action; label is the slug to create
)

type row struct {
	kind  rowKind
	proj  workspace.Project // valid when kind==rowProject
	label string            // category name (rowCategory) or slug (rowCreate)
}

// Model is the bubbletea model for the dw picker.
type Model struct {
	root       string
	tmpl       string
	now        time.Time
	projects   []workspace.Project
	categories []string // available categories, computed once at startup

	mode   mode
	query  string
	cursor int

	// carried from browse into category step
	pendingTopic string
	catQuery     string

	lastPath string // previously chosen project, marked "←前回" in the list
	hasPin   bool   // whether lastPath actually matched a listed project

	Result string // chosen/created absolute path; empty means abort
	Err    error
}

// New builds the initial model. lastPath, when it matches a project, is pinned
// to the top of the browse list so the most common move — resuming the previous
// workspace — is one Enter away; pass "" to disable pinning.
func New(root, tmpl string, now time.Time, projects []workspace.Project, lastPath string) Model {
	pinned, ok := pinLast(projects, lastPath)
	return Model{
		root:       root,
		tmpl:       tmpl,
		now:        now,
		projects:   pinned,
		categories: workspace.Categories(projects),
		mode:       modeBrowse,
		lastPath:   lastPath,
		hasPin:     ok,
	}
}

// pinLast returns projects with the lastPath entry moved to the front, leaving
// the input slice untouched, and reports whether it matched. A blank or
// unmatched lastPath returns the input as-is with ok=false.
func pinLast(projects []workspace.Project, lastPath string) ([]workspace.Project, bool) {
	if lastPath == "" {
		return projects, false
	}
	for i, p := range projects {
		if p.Path != lastPath {
			continue
		}
		out := make([]workspace.Project, 0, len(projects))
		out = append(out, p)
		out = append(out, projects[:i]...)
		out = append(out, projects[i+1:]...)
		return out, true
	}
	return projects, false
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// rows computes the visible rows for the current mode + query.
func (m Model) rows() []row {
	if m.mode == modeBrowse {
		return browseRows(m.projects, m.query)
	}
	return categoryRows(m.categories, m.catQuery)
}

// fuzzyIndices returns the indices of targets matching query, or all indices in
// order when query is blank.
func fuzzyIndices(query string, targets []string) []int {
	if strings.TrimSpace(query) == "" {
		idx := make([]int, len(targets))
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	matches := fuzzy.Find(query, targets)
	idx := make([]int, len(matches))
	for i, mt := range matches {
		idx[i] = mt.Index
	}
	return idx
}

func browseRows(projects []workspace.Project, query string) []row {
	targets := make([]string, len(projects))
	for i, p := range projects {
		targets[i] = p.Category + "/" + p.Name + " " + p.Title
	}
	var rows []row
	for _, i := range fuzzyIndices(query, targets) {
		rows = append(rows, row{kind: rowProject, proj: projects[i]})
	}
	// only offer "create" when the query yields a non-empty slug, so the
	// displayed name always matches the directory Create will make
	if slug := workspace.Slugify(query); slug != "" {
		rows = append(rows, row{kind: rowCreate, label: slug})
	}
	return rows
}

func categoryRows(categories []string, query string) []row {
	var rows []row
	for _, i := range fuzzyIndices(query, categories) {
		rows = append(rows, row{kind: rowCategory, label: categories[i]})
	}
	// offer creating a brand-new category if the typed text isn't an exact match
	if slug := workspace.Slugify(query); slug != "" && !slices.Contains(categories, slug) {
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

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		m.Result = ""
		return m, tea.Quit
	case "esc":
		// From category selection, esc steps back to browse so a mistyped
		// topic can be fixed without restarting; from browse it aborts.
		if m.mode == modeCategory {
			m.mode = modeBrowse
			m.catQuery = ""
			m.cursor = 0
			return m, nil
		}
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
	if m.cursor < 0 || m.cursor >= len(rows) {
		return m, nil
	}
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

// View implements tea.Model.
func (m Model) View() string {
	var b strings.Builder
	rows := m.rows()

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
	esc := "esc 中止"
	if m.mode == modeCategory {
		esc = "esc 戻る"
	}
	help := "↑↓ 選択  enter 決定  " + esc
	// Surface the shell shortcut only when there is a pinned project to return
	// to, so the "←前回" marker is paired with how to jump there without the UI.
	if m.mode == modeBrowse && m.hasPin {
		help += "  ·  dw - で前回へ"
	}
	b.WriteString(dimStyle.Render(help))
	b.WriteString("\n")
	return b.String()
}

func (m Model) renderRow(r row, selected bool) string {
	var text string
	switch r.kind {
	case rowCreate:
		if m.mode == modeBrowse {
			text = createStyle.Render("+ 作成: ") + m.now.Format("2006-01-02") + "-" + r.label
		} else {
			text = createStyle.Render("+ 新規カテゴリ: ") + r.label
		}
	case rowCategory:
		text = r.label
	default: // rowProject
		p := r.proj
		date := p.Date
		if date == "" {
			date = "          "
		}
		// runewidth.FillRight pads by display cells, so wide (CJK) category
		// names stay aligned where a plain %-10s would drift.
		text = date + "  " + runewidth.FillRight(p.Category, 10) + " " + p.Title
	}
	prefix := "  "
	if selected {
		prefix = "› "
		text = selStyle.Render(text)
	}
	line := prefix + text
	// Mark the previously chosen project so its presence at the top reads as
	// "resume last" rather than an oddly old entry. Kept outside selStyle so the
	// marker stays its own color even when the row is highlighted.
	if r.kind == rowProject && m.lastPath != "" && r.proj.Path == m.lastPath {
		line += pinStyle.Render(" ←前回")
	}
	out := line + "\n"
	if selected && r.kind == rowProject {
		if meta := metaLine(r.proj); meta != "" {
			out += dimStyle.Render("    "+meta) + "\n"
		}
	}
	return out
}

// metaLine renders the dim one-line detail shown under the selected project:
// the status, tags, and created date drawn from the README frontmatter. Empty
// fields are skipped, and it returns "" when nothing is worth showing.
func metaLine(p workspace.Project) string {
	var parts []string
	if p.Status != "" {
		parts = append(parts, "status:"+p.Status)
	}
	if p.Tags != "" && p.Tags != "[]" {
		parts = append(parts, "tags:"+p.Tags)
	}
	created := p.Created
	if created == "" {
		created = p.Date
	}
	if created != "" {
		parts = append(parts, "created:"+created)
	}
	return strings.Join(parts, "  ")
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
