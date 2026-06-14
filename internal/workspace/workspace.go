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
)

// Project is a single discussion workspace directory.
type Project struct {
	Category string // e.g. "research"
	Name     string // directory name, e.g. "2026-06-13-pc-setup"
	Topic    string // slug without the date prefix, e.g. "pc-setup"
	Date     string // "2026-06-13", or "" when the dir has no date prefix
	Title    string // title from README frontmatter, falls back to Topic
	Path     string // absolute path
}

var datePrefix = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.*)$`)

// Root returns the workspace root, honoring $DISCUSSION_ROOT and defaulting to ~/Discussion.
func Root() string {
	if r := os.Getenv("DISCUSSION_ROOT"); r != "" {
		return r
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Discussion")
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9-]+`)
var slugDashes = regexp.MustCompile(`-+`)

// Slugify normalizes a free-form topic into a filesystem-friendly slug.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = slugInvalid.ReplaceAllString(s, "")
	s = slugDashes.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// parseProject builds a Project from a category and directory name.
func parseProject(category, name, path string) Project {
	p := Project{Category: category, Name: name, Path: path, Topic: name}
	if m := datePrefix.FindStringSubmatch(name); m != nil {
		p.Date = m[1]
		p.Topic = m[2]
	}
	p.Title = readTitle(path)
	if p.Title == "" {
		p.Title = p.Topic
	}
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

// sortProjects orders by Name descending so dated dirs are newest-first.
func sortProjects(ps []Project) {
	sort.SliceStable(ps, func(i, j int) bool {
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
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		content := RenderTemplate(tmpl, slug, category, date)
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return Project{}, err
		}
	}
	return parseProject(category, name, path), nil
}
