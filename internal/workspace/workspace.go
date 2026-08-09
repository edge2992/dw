// Package workspace discovers and creates discussion workspaces laid out as
// <root>/<YYYY-MM-DD>-<topic-slug>/STATE.md. See docs/concepts.md for the
// concept these types and functions implement.
package workspace

import (
	"errors"
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
	Name  string // directory name, e.g. "2026-08-08-datadog-cost"
	Topic string // slug without the date prefix, e.g. "datadog-cost"
	Date  string // "2026-08-08", or "" when the dir has no date prefix
	Path  string // absolute path
}

var datePrefix = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.*)$`)

var slugDashes = regexp.MustCompile(`-+`)

// ErrEmptyTopic is returned by Resolve and Create when arg/rawTopic has no
// letters or digits to slug (e.g. "", "!!!").
var ErrEmptyTopic = errors.New("topic has no letters or digits to slug")

// ErrNotFound is returned by Resolve when arg looks like an absolute path but
// does not match any existing workspace. Resolve never creates a directory at
// an arbitrary absolute path, so this is the terminal outcome for that case.
var ErrNotFound = errors.New("no workspace found at that path")

// Root returns the workspace root: $DW_ROOT if set, else ~/dw. There is no
// config file — this environment variable is the only knob.
func Root() string {
	if r := os.Getenv("DW_ROOT"); r != "" {
		return r
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "dw")
}

// Slugify normalizes a free-form topic into a filesystem-friendly slug.
// Unicode letters/numbers are kept (so Japanese topics survive); whitespace
// and separators collapse to "-", and other punctuation/symbols are dropped.
// May return "" when the input has no letters or numbers (e.g. "!!!").
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

// parseProject builds a Project from a directory name found directly under root.
func parseProject(name, path string) Project {
	p := Project{Name: name, Topic: name, Path: path}
	if m := datePrefix.FindStringSubmatch(name); m != nil {
		p.Date = m[1]
		p.Topic = m[2]
	}
	return p
}

// Scan lists the workspaces directly under root (one level, no category
// hierarchy). Dot-directories (.git, .obsidian, …) are skipped. Directories
// without a date prefix are still returned, with Date == "", so they are
// never silently dropped — sortProjects just puts them last.
func Scan(root string) ([]Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var projects []Project
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		projects = append(projects, parseProject(e.Name(), filepath.Join(root, e.Name())))
	}
	sortProjects(projects)
	return projects, nil
}

// sortProjects orders dated projects newest-first, then undated ones last.
// Without this, a plain Name-descending sort would float letter-prefixed or
// undated dirs above dated ones (ASCII letters sort after digits), so the
// default ordering would bury the newest project under legacy directories.
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

// Create scaffolds a new workspace <root>/<date>-<slug>/STATE.md for
// rawTopic, and writes the root CLAUDE.md convention file too if root doesn't
// have one yet (see convention.go). rawTopic is only trimmed, not slugified,
// before it becomes the STATE.md title — the slug is just the directory name.
// now supplies the date so callers/tests stay deterministic.
func Create(root, rawTopic string, now time.Time) (Project, error) {
	title := strings.TrimSpace(rawTopic)
	slug := Slugify(title)
	if slug == "" {
		return Project{}, ErrEmptyTopic
	}
	date := now.Format("2006-01-02")
	name := date + "-" + slug
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return Project{}, err
	}
	if _, err := writeIfAbsent(filepath.Join(path, stateFileName), renderState(title), 0o644); err != nil {
		return Project{}, err
	}
	if err := writeConventionIfAbsent(root); err != nil {
		return Project{}, err
	}
	return parseProject(name, path), nil
}

// Resolve finds the workspace(s) under root matching arg, creating one when
// nothing matches at all. The algorithm (see docs in the repo's task spec /
// docs/concepts.md "Operations"):
//
//  1. Exact match on Topic, Name, or (for an absolute arg) Path wins outright
//     — one result, nothing created.
//  2. Otherwise, partial match (strings.Contains on Topic): one result if
//     there's exactly one, or every match (fzf's job to disambiguate) if
//     there's more than one.
//  3. Otherwise, a new workspace is created.
//
// An absolute arg that doesn't exactly match an existing workspace returns
// ErrNotFound instead of falling through to partial-match/create — Resolve
// never creates a directory at an arbitrary absolute path.
//
// created reports whether Resolve made a new directory. Callers should call
// SaveLast only when Resolve returns exactly one match — see main.go.
func Resolve(root, arg string) (matches []Project, created bool, err error) {
	trimmed := strings.TrimSpace(arg)
	slug := Slugify(trimmed)
	if slug == "" {
		return nil, false, ErrEmptyTopic
	}

	projects, err := Scan(root)
	if err != nil {
		return nil, false, err
	}

	abs := filepath.IsAbs(trimmed)
	for _, p := range projects {
		if p.Topic == slug || p.Name == slug || (abs && p.Path == trimmed) {
			return []Project{p}, false, nil
		}
	}
	if abs {
		return nil, false, ErrNotFound
	}

	var partial []Project
	for _, p := range projects {
		if strings.Contains(p.Topic, slug) {
			partial = append(partial, p)
		}
	}
	if len(partial) > 0 {
		return partial, false, nil // already ordered by Scan
	}

	p, err := Create(root, trimmed, time.Now())
	if err != nil {
		return nil, false, err
	}
	return []Project{p}, true, nil
}

// writeIfAbsent writes content to path unless the file already exists, and
// reports whether it wrote. Every write in this package goes through here so
// a workspace is never silently clobbered.
func writeIfAbsent(path, content string, mode os.FileMode) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.Write([]byte(content)); err != nil {
		_ = f.Close()
		return false, err
	}
	if err := f.Close(); err != nil {
		return false, err
	}
	return true, nil
}
