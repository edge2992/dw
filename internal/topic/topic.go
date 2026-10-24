// Package topic discovers and creates topics laid out as
// <root>/<YYYY-MM-DD>-<name>/<name>.md. See GLOSSARY.md for the vocabulary and
// docs/concepts.md for the ideas behind the layout.
package topic

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

// Topic is a single directory directly under the root.
type Topic struct {
	DirName string // directory name, e.g. "2026-10-24-調べ方を設計する"
	Name    string // topic name: DirName without the date prefix, e.g. "調べ方を設計する"
	Date    string // "2026-10-24", or "" when the dir has no date prefix
	Path    string // absolute path
}

// MainNote returns the path of the note named after the topic. It may not
// exist: topics made by hand or by older versions of dw are still topics.
func (t Topic) MainNote() string {
	return filepath.Join(t.Path, t.Name+".md")
}

var datePrefix = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.*)$`)

// ErrEmptyName is returned by Resolve and Create when the name is blank.
var ErrEmptyName = errors.New("topic name is empty")

// ErrInvalidName is returned by Resolve and Create when the name contains a
// character that would break the path or an Obsidian link to the main note.
var ErrInvalidName = errors.New(`topic name must not start with "." or contain / \ : # ^ [ ] | or control characters`)

// ErrNotFound is returned by Resolve when arg looks like an absolute path but
// does not match any existing topic. Resolve never creates a directory at an
// arbitrary absolute path, so this is the terminal outcome for that case.
var ErrNotFound = errors.New("no topic found at that path")

// forbiddenChars break either the filesystem path (/ \ :) or an Obsidian
// [[link]] to the main note (# ^ [ ] |).
const forbiddenChars = `/\:#^[]|`

// Root returns the topic root: $DW_ROOT if set, else ~/dw. There is no
// config file — this environment variable is the only knob.
//
// The result is always made absolute. Every path dw prints is documented as
// absolute (see the TSV contract in main.go), and a relative DW_ROOT would
// break that promise in a way that compounds: the shell wrapper cd's into a
// resolved topic, so the next invocation would resolve the same relative
// DW_ROOT underneath it, nesting roots deeper on every visit.
func Root() string {
	r := os.Getenv("DW_ROOT")
	if r == "" {
		home, _ := os.UserHomeDir()
		r = filepath.Join(home, "dw")
	}
	if abs, err := filepath.Abs(r); err == nil {
		return abs
	}
	return r
}

// ValidateName trims s and returns it unchanged otherwise: the name the user
// typed is the name they get, so a bad character is an error rather than
// something silently rewritten.
func ValidateName(s string) (string, error) {
	name := strings.TrimSpace(s)
	if name == "" {
		return "", ErrEmptyName
	}
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, forbiddenChars) || strings.ContainsFunc(name, unicode.IsControl) {
		return "", ErrInvalidName
	}
	return name, nil
}

// parseTopic builds a Topic from a directory name found directly under root.
func parseTopic(dirName, path string) Topic {
	t := Topic{DirName: dirName, Name: dirName, Path: path}
	if m := datePrefix.FindStringSubmatch(dirName); m != nil {
		t.Date = m[1]
		t.Name = m[2]
	}
	return t
}

// Scan lists the topics directly under root (one level, no hierarchy).
// Dot-directories (.git, .obsidian, …) are skipped. Directories without a
// date prefix are still returned, with Date == "", so they are never silently
// dropped — sortTopics just puts them last.
func Scan(root string) ([]Topic, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var topics []Topic
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		topics = append(topics, parseTopic(e.Name(), filepath.Join(root, e.Name())))
	}
	sortTopics(topics)
	return topics, nil
}

// sortTopics orders dated topics newest-first, then undated ones last.
// Without this, a plain DirName-descending sort would float letter-prefixed or
// undated dirs above dated ones (letters sort after digits), so the default
// ordering would bury the newest topic under hand-made directories.
func sortTopics(ts []Topic) {
	sort.SliceStable(ts, func(i, j int) bool {
		di, dj := ts[i].Date != "", ts[j].Date != ""
		if di != dj {
			return di // dated topics come before undated ones
		}
		if ts[i].Date != ts[j].Date {
			return ts[i].Date > ts[j].Date // newer date first
		}
		return ts[i].DirName > ts[j].DirName
	})
}

// Create makes <root>/<date>-<name>/<name>.md, the main note holding nothing
// but the name as its H1. now supplies the date so callers/tests stay
// deterministic.
func Create(root, rawName string, now time.Time) (Topic, error) {
	name, err := ValidateName(rawName)
	if err != nil {
		return Topic{}, err
	}
	dirName := now.Format("2006-01-02") + "-" + name
	t := parseTopic(dirName, filepath.Join(root, dirName))
	if err := os.MkdirAll(t.Path, 0o755); err != nil {
		return Topic{}, err
	}
	if _, err := writeIfAbsent(t.MainNote(), "# "+name+"\n", 0o644); err != nil {
		return Topic{}, err
	}
	return t, nil
}

// Resolve finds the topic(s) under root matching arg, creating one when
// nothing matches at all:
//
//  1. Exact match on Name or DirName (ignoring case), or on Path for an
//     absolute arg, wins outright — one result, nothing created.
//  2. Otherwise, partial match on Name (ignoring case): one result if there's
//     exactly one, or every match (fzf's job to disambiguate) if there's more.
//  3. Otherwise, a new topic is created.
//
// Matching an existing name before creating is what keeps topic names unique
// within the root. An absolute arg that doesn't exactly match an existing
// topic returns ErrNotFound instead — Resolve never creates a directory at an
// arbitrary absolute path.
//
// created reports whether Resolve made a new directory. Callers should call
// SaveLast only when Resolve returns exactly one match — see main.go.
func Resolve(root, arg string) (matches []Topic, created bool, err error) {
	trimmed := strings.TrimSpace(arg)
	abs := filepath.IsAbs(trimmed)
	if !abs {
		if trimmed, err = ValidateName(trimmed); err != nil {
			return nil, false, err
		}
	}

	topics, err := Scan(root)
	if err != nil {
		return nil, false, err
	}

	for _, t := range topics {
		if (abs && t.Path == trimmed) || (!abs && (strings.EqualFold(t.Name, trimmed) || strings.EqualFold(t.DirName, trimmed))) {
			return []Topic{t}, false, nil
		}
	}
	if abs {
		return nil, false, ErrNotFound
	}

	query := strings.ToLower(trimmed)
	var partial []Topic
	for _, t := range topics {
		if strings.Contains(strings.ToLower(t.Name), query) {
			partial = append(partial, t)
		}
	}
	if len(partial) > 0 {
		return partial, false, nil // already ordered by Scan
	}

	t, err := Create(root, trimmed, time.Now())
	if err != nil {
		return nil, false, err
	}
	return []Topic{t}, true, nil
}

// TSVRow formats t as one line of dw's stdout contract:
//
//	<absolute path>\t<marker + directory name>
//
// last is the currently pinned path (LastPath()); when it equals t.Path the
// marker is "* ", otherwise two spaces. The wrapper hides the path column
// from fzf, so the second column is both what is shown and what is searched.
func (t Topic) TSVRow(last string) string {
	marker := "  "
	if last != "" && last == t.Path {
		marker = "* "
	}
	return t.Path + "\t" + marker + t.DirName
}

// PinLast moves the topic whose Path equals last to the front of ts, leaving
// the rest in their existing order. Used for the bare `dw` listing, which
// pins the most recently visited topic to the top. A no-op when last is "" or
// not found in ts.
func PinLast(ts []Topic, last string) []Topic {
	if last == "" {
		return ts
	}
	idx := -1
	for i, t := range ts {
		if t.Path == last {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return ts
	}
	out := make([]Topic, 0, len(ts))
	out = append(out, ts[idx])
	out = append(out, ts[:idx]...)
	out = append(out, ts[idx+1:]...)
	return out
}

// writeIfAbsent writes content to path unless the file already exists, and
// reports whether it wrote. Every write in this package goes through here so
// a note is never silently clobbered.
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
