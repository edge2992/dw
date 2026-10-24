// Package topic discovers and creates topics laid out as
// <root>/<YYYY-MM-DD>-<name>/<name>.md. See GLOSSARY.md for the vocabulary and
// docs/concepts.md for the ideas behind the layout.
package topic

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
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

// ErrEmptyName is returned by Resolve and Create when the name is blank, or
// has nothing left once symbols are replaced.
var ErrEmptyName = errors.New("topic name is empty")

// ErrInvalidName is returned by Resolve and Create when the name holds a
// control character or is one Windows reserves (CON, NUL, COM1, …).
var ErrInvalidName = errors.New("invalid topic name")

// ErrNotFound is returned by Resolve when arg looks like an absolute path but
// does not match any existing topic. Resolve never creates a directory at an
// arbitrary absolute path, so this is the terminal outcome for that case.
var ErrNotFound = errors.New("no topic found at that path")

// replacedChars become "-" in a topic's directory and file name: each one
// breaks a path on some OS, an Obsidian [[link]], or an unquoted shell word
// (`dw 決済API #INC-0432` loses everything after the " #").
const replacedChars = "/\\:#^[]|?*<>\"'$&;(){}!~`"

// windowsReserved matches the names Windows refuses as a file name, with or
// without an extension, so CON.md could never be created there.
var windowsReserved = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(\.|$)`)

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

// Name is a topic name in the two forms dw needs.
type Name struct {
	Title string // as typed, in NFC with runs of spaces collapsed: the main note's H1
	Slug  string // what the directory and the main note are named
}

// ParseName turns what the user typed into a Name. The Slug is safe to type
// unquoted in a shell and to link from Obsidian: each run of spaces and
// replacedChars becomes one "-", and leading or trailing "-" and "." are
// dropped. Case, Japanese and a "." inside the name (vs., v2.40) are kept.
//
// Everything is NFC first, so a name some macOS tool stored decomposed (NFD)
// is the same name as the one typed normally.
func ParseName(s string) (Name, error) {
	title := strings.Join(strings.Fields(norm.NFC.String(s)), " ")
	if i := strings.IndexFunc(title, unicode.IsControl); i >= 0 {
		return Name{}, fmt.Errorf("%w: control character %q", ErrInvalidName, []rune(title[i:])[0])
	}
	if title == "" {
		return Name{}, ErrEmptyName
	}
	slug := strings.Trim(strings.Join(strings.FieldsFunc(title, isSeparator), "-"), "-.")
	if slug == "" {
		return Name{}, fmt.Errorf("%w once symbols are replaced (got %q)", ErrEmptyName, title)
	}
	if windowsReserved.MatchString(slug) {
		return Name{}, fmt.Errorf("%w: %q is reserved on Windows", ErrInvalidName, slug)
	}
	return Name{Title: title, Slug: slug}, nil
}

func isSeparator(r rune) bool {
	return unicode.IsSpace(r) || r == '-' || strings.ContainsRune(replacedChars, r)
}

// matchKey is what names are compared by: case, "_" and every separator are
// ignored, so "Datadog Cost" finds a topic made as "datadog-cost", and a name
// typed with symbols finds the topic its Slug was made into.
func matchKey(s string) string {
	return strings.Map(func(r rune) rune {
		if isSeparator(r) || r == '_' {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFC.String(s))
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

// Create makes <root>/<date>-<slug>/<slug>.md, the main note holding nothing
// but the name's Title as its H1. now supplies the date so callers/tests stay
// deterministic.
func Create(root, rawName string, now time.Time) (Topic, error) {
	name, err := ParseName(rawName)
	if err != nil {
		return Topic{}, err
	}
	return create(root, name, now)
}

func create(root string, name Name, now time.Time) (Topic, error) {
	dirName := now.Format("2006-01-02") + "-" + name.Slug
	t := parseTopic(dirName, filepath.Join(root, dirName))
	if err := os.MkdirAll(t.Path, 0o755); err != nil {
		return Topic{}, err
	}
	if _, err := writeIfAbsent(t.MainNote(), "# "+name.Title+"\n", 0o644); err != nil {
		return Topic{}, err
	}
	return t, nil
}

// Resolve finds the topic(s) under root matching arg, creating one when
// nothing matches at all:
//
//  1. Exact match on Name or DirName (compared by matchKey), or on Path for
//     an absolute arg, wins outright — one result, nothing created.
//  2. Otherwise, partial match on Name (compared by matchKey): one result if
//     there's exactly one, or every match (fzf's job to disambiguate) if
//     there's more.
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
	if path := strings.TrimSpace(arg); filepath.IsAbs(path) {
		topics, err := Scan(root)
		if err != nil {
			return nil, false, err
		}
		for _, t := range topics {
			if t.Path == path {
				return []Topic{t}, false, nil
			}
		}
		return nil, false, ErrNotFound
	}

	name, topics, err := validateAndScan(root, arg)
	if err != nil {
		return nil, false, err
	}
	if t, ok := findExact(topics, name); ok {
		return []Topic{t}, false, nil
	}

	query := matchKey(name.Slug)
	var partial []Topic
	for _, t := range topics {
		if query != "" && strings.Contains(matchKey(t.Name), query) {
			partial = append(partial, t)
		}
	}
	if len(partial) > 0 {
		return partial, false, nil // already ordered by Scan
	}

	t, err := create(root, name, time.Now())
	if err != nil {
		return nil, false, err
	}
	return []Topic{t}, true, nil
}

// New is Resolve without the partial-match step: it opens the topic whose
// name matches arg exactly, and otherwise creates one, even when arg is part
// of an existing name. That is the only way to make "アーキ定例" once
// "アーキ定例 10-28" exists. An exact match still opens rather than
// duplicates, so names stay unique.
func New(root, arg string) (t Topic, created bool, err error) {
	name, topics, err := validateAndScan(root, arg)
	if err != nil {
		return Topic{}, false, err
	}
	if t, ok := findExact(topics, name); ok {
		return t, false, nil
	}
	t, err = create(root, name, time.Now())
	return t, err == nil, err
}

func validateAndScan(root, arg string) (Name, []Topic, error) {
	name, err := ParseName(arg)
	if err != nil {
		return Name{}, nil, err
	}
	topics, err := Scan(root)
	return name, topics, err
}

func findExact(topics []Topic, name Name) (Topic, bool) {
	key := matchKey(name.Slug)
	for _, t := range topics {
		if matchKey(t.Name) == key || matchKey(t.DirName) == key {
			return t, true
		}
	}
	return Topic{}, false
}

// TSVRow formats t as one line of dw's stdout contract:
//
//	<absolute path>\t<marker + directory name>
//
// last is the most recently visited path (Recent()[0]); when it equals
// t.Path the marker is "* ", otherwise two spaces. The wrapper hides the path
// column from fzf, so the second column is both what is shown and what is
// searched.
func (t Topic) TSVRow(last string) string {
	marker := "  "
	if last != "" && last == t.Path {
		marker = "* "
	}
	return t.Path + "\t" + marker + t.DirName
}

// PinRecent moves the topics whose Path is in recent to the front of ts, in
// recent's order, leaving the rest in their existing order. Used for the bare
// `dw` listing. Paths in recent that aren't in ts are ignored.
func PinRecent(ts []Topic, recent []string) []Topic {
	out := make([]Topic, 0, len(ts))
	for _, p := range recent {
		if i := slices.IndexFunc(ts, func(t Topic) bool { return t.Path == p }); i >= 0 {
			out = append(out, ts[i])
		}
	}
	for _, t := range ts {
		if !slices.Contains(recent, t.Path) {
			out = append(out, t)
		}
	}
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
