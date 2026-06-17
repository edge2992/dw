// Package workspace handles discovery and creation of discussion projects
// laid out as <root>/<category>/<YYYY-MM-DD>-<topic-slug>/.
package workspace

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Project is a single discussion workspace directory.
type Project struct {
	Category string `json:"category"` // e.g. "research"
	Name     string `json:"name"`     // directory name, e.g. "2026-06-13-pc-setup"
	Topic    string `json:"topic"`    // slug without the date prefix, e.g. "pc-setup"
	Date     string `json:"date"`     // "2026-06-13", or "" when the dir has no date prefix
	Title    string `json:"title"`    // title from README frontmatter, falls back to Topic
	Status   string `json:"status"`   // status from README frontmatter, e.g. "active"
	Tags     string `json:"tags"`     // raw tags from README frontmatter, e.g. "[gpu, linux]"
	Created  string `json:"created"`  // created date from README frontmatter
	Path     string `json:"path"`     // absolute path
}

var datePrefix = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.*)$`)

// Root returns the workspace root, honoring $DW_ROOT and defaulting to ~/dw.
func Root() string {
	if r := os.Getenv("DW_ROOT"); r != "" {
		return r
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "dw")
}

var slugDashes = regexp.MustCompile(`-+`)

// Slugify normalizes a free-form topic into a filesystem-friendly slug.
// Unicode letters/numbers are kept (so Japanese topics survive); whitespace and
// separators collapse to "-", and other punctuation/symbols are dropped. May
// return "" when the input has no letters or numbers (e.g. "!!!").
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
		case r == '-' || r == '_' || unicode.IsSpace(r):
			b.WriteByte('-')
		}
	}
	return strings.Trim(slugDashes.ReplaceAllString(b.String(), "-"), "-")
}

// parseProject builds a Project from a category and directory name.
func parseProject(category, name, path string) Project {
	p := Project{Category: category, Name: name, Path: path, Topic: name}
	if m := datePrefix.FindStringSubmatch(name); m != nil {
		p.Date = m[1]
		p.Topic = m[2]
	}
	fm := readFrontmatter(path)
	p.Title = fm.title
	if p.Title == "" {
		p.Title = p.Topic
	}
	p.Status = fm.status
	p.Tags = fm.tags
	p.Created = fm.created
	return p
}

// Scan walks root/<category>/<project> and returns all projects, most recent first.
func Scan(root string) ([]Project, error) {
	cats, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var projects []Project
	for _, c := range cats {
		if !c.IsDir() {
			continue
		}
		catPath := filepath.Join(root, c.Name())
		entries, err := os.ReadDir(catPath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			projects = append(projects, parseProject(c.Name(), e.Name(), filepath.Join(catPath, e.Name())))
		}
	}
	sortProjects(projects)
	return projects, nil
}

// sortProjects orders dated projects newest-first, then undated ones last.
// Without this, a plain Name-descending sort would float letter-prefixed or
// undated dirs above dated ones (ASCII letters sort after digits), so the
// default selection would land on a legacy dir instead of the newest project.
func sortProjects(ps []Project) {
	sort.SliceStable(ps, func(i, j int) bool {
		di, dj := ps[i].Date != "", ps[j].Date != ""
		if di != dj {
			return di // dated projects come before undated ones
		}
		if ps[i].Date != ps[j].Date {
			return ps[i].Date > ps[j].Date // newer date first
		}
		return ps[i].Name > ps[j].Name
	})
}

// Categories returns the available categories: the defaults first (in their
// defined priority order), then any extra categories found on disk, sorted.
func Categories(projects []Project) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(DefaultCategories))
	for _, d := range DefaultCategories {
		seen[d] = true
		out = append(out, d)
	}
	var extra []string
	for _, p := range projects {
		if !seen[p.Category] {
			seen[p.Category] = true
			extra = append(extra, p.Category)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// DefaultCategories are always offered even when empty.
var DefaultCategories = []string{"research", "incident", "discussion", "scratch"}

// Create makes <root>/<category>/<date>-<slug>/ with a README rendered from tmpl.
// now supplies the date so callers/tests stay deterministic.
func Create(root, category, topic string, now time.Time, tmpl string) (Project, error) {
	date := now.Format("2006-01-02")
	slug := Slugify(topic)
	if slug == "" {
		slug = "untitled"
	}
	name := date + "-" + slug
	path := filepath.Join(root, category, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return Project{}, err
	}
	readme := filepath.Join(path, "README.md")
	if _, err := os.Stat(readme); err != nil {
		if !os.IsNotExist(err) {
			return Project{}, err
		}
		content := RenderTemplate(tmpl, slug, category, date)
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return Project{}, err
		}
	}
	return parseProject(category, name, path), nil
}
