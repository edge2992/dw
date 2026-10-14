package workspace

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// stateFileName is the state document every workspace carries.
const stateFileName = "STATE.md"

// unresolvedHeading is the single source of truth for the "open questions"
// heading: both stateTemplate (what Create writes) and readState (what dw
// parses back out) reference this constant, so the literal never drifts
// between the two.
const unresolvedHeading = "## 未決の問い"

// stateTemplate is written verbatim (with {{title}} substituted) for every
// newly created workspace. No frontmatter: the date already lives in the
// directory name, so it is never duplicated here.
const stateTemplate = "# {{title}}\n" +
	"\n" +
	"## 前提\n" +
	"\n" +
	"<!-- 事実と判断を分け、それぞれに日付を添える。判断のほうが先に古くなる -->\n" +
	"\n" +
	"## 却下した案\n" +
	"\n" +
	"<!-- 条件付きで書く：「案 X は採らない。理由は Y。Y が崩れたら再検討」 -->\n" +
	"\n" +
	unresolvedHeading + "\n" +
	"\n" +
	"<!-- 先頭行が dw の一覧に出る -->\n"

// renderState fills stateTemplate's {{title}} placeholder with the raw topic
// string the user typed (trimmed, not slugified).
func renderState(title string) string {
	return strings.Replace(stateTemplate, "{{title}}", title, 1)
}

// stateInfo holds what dw's listing needs out of a workspace's STATE.md.
type stateInfo struct {
	title      string // first "# " heading; "" if none
	unresolved string // first non-empty, non-comment line under unresolvedHeading; "" if none
}

// ErrStateChanged is returned by WriteStateAtomic when STATE.md no longer
// matches the hash the caller read it with: someone else wrote it meanwhile.
var ErrStateChanged = errors.New("STATE.md changed since it was read")

// ReadStateFile returns dir/STATE.md and its sha256, the token
// WriteStateAtomic later checks against.
func ReadStateFile(dir string) (string, [32]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, stateFileName))
	if err != nil {
		return "", [32]byte{}, err
	}
	return string(b), sha256.Sum256(b), nil
}

// WriteStateAtomic replaces dir/STATE.md with content, but only if the file
// still hashes to expect (compare-and-swap); otherwise it returns
// ErrStateChanged and leaves the file alone. The new content is written to a
// temp file in the same directory and renamed over STATE.md, so a reader sees
// either the old file or the new one, never half of each. The window between
// the hash check and the rename is a few milliseconds; an editor save landing
// exactly there is lost, which the rolling backups cover.
func WriteStateAtomic(dir, content string, expect [32]byte) (err error) {
	path := filepath.Join(dir, stateFileName)
	cur, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if sha256.Sum256(cur) != expect {
		return ErrStateChanged
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".STATE.md.tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), fi.Mode().Perm()); err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	if d, derr := os.Open(dir); derr == nil { // best effort: persist the rename itself
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Title returns the first "# " heading of a STATE.md body, or "".
func Title(content string) string {
	return firstHeading(skipFrontmatter(splitLines(content)))
}

// OpenQuestions returns every non-empty, non-comment line in the
// "## 未決の問い" section of a STATE.md body, and whether the section exists.
func OpenQuestions(content string) (lines []string, found bool) {
	return openQuestions(skipFrontmatter(splitLines(content)))
}

// openQuestions scans the unresolvedHeading section, stopping at the next
// heading. Blank lines and "<!--" comment lines are skipped.
func openQuestions(all []string) (lines []string, found bool) {
	for _, line := range all {
		t := strings.TrimSpace(line)
		if !found {
			found = t == unresolvedHeading
			continue
		}
		switch {
		case strings.HasPrefix(t, "#"):
			return lines, true
		case t != "" && !strings.HasPrefix(t, "<!--"):
			lines = append(lines, t)
		}
	}
	return lines, found
}

func splitLines(content string) []string {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	return lines
}

// readState extracts stateInfo from dir/STATE.md. Every failure mode —
// missing file, missing H1, missing "## 未決の問い" section, a section that
// is empty or contains only an HTML comment — comes back as a zero-value
// field; callers fall back to the directory topic and an empty question.
//
// It reads the whole file rather than a fixed prefix: STATE.md is meant to
// grow (前提 and 却下した案 accumulate dated entries over a topic's life), and
// a line cap would eventually push "## 未決の問い" out of view, silently
// blanking the one column dw's listing exists to show.
func readState(dir string) stateInfo {
	lines := readLines(filepath.Join(dir, stateFileName))
	lines = skipFrontmatter(lines) // tolerate old data that still has YAML frontmatter
	return stateInfo{
		title:      firstHeading(lines),
		unresolved: firstUnresolvedLine(lines),
	}
}

// readLines returns every line of path, or nil if it can't be opened.
func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	return lines
}

// skipFrontmatter drops a leading "---" ... "---" YAML block, for
// compatibility with workspaces created by the old per-category template.
// New STATE.md files never have frontmatter, so this is a no-op for them.
func skipFrontmatter(lines []string) []string {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return lines
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return lines[i+1:]
		}
	}
	return nil // unterminated frontmatter within the read window: nothing usable left
}

// firstHeading returns the text of the first top-level "# " heading, or "".
func firstHeading(lines []string) string {
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "# "))
		}
	}
	return ""
}

// firstUnresolvedLine returns the first non-empty, non-comment line under
// unresolvedHeading, stopping at the next heading. Returns "" when the
// heading is absent, the section is empty, or it holds only a "<!--" comment.
func firstUnresolvedLine(lines []string) string {
	if qs, _ := openQuestions(lines); len(qs) > 0 {
		return qs[0]
	}
	return ""
}

// TSVRow formats p as one line of dw's stdout contract:
//
//	<absolute path>\t<marker + title>\t<first open question>\t<directory name>
//
// last is the currently pinned path (LastPath()); when it equals p.Path the
// marker is "* ", otherwise two spaces. No column-width alignment is done —
// callers get raw TSV and are expected to consume it as such (e.g. fzf).
//
// The directory name is repeated in the last column even though it is already
// the basename of the first one, because fzf cannot search a field it does not
// display: the wrapper hides column 1 with --with-nth=2.., which takes the
// path out of the search scope as well. Without a visible copy, a workspace
// whose STATE.md title has been rewritten (the expected workflow) could not be
// found by its slug or its date — and fzf reporting "no match" is what makes
// the wrapper offer to create a new topic, so the picker would be inviting
// duplicates of workspaces that already exist.
func (p Project) TSVRow(last string) string {
	info := readState(p.Path)
	title := info.title
	if title == "" {
		title = p.Topic
	}
	marker := "  "
	if last != "" && last == p.Path {
		marker = "* "
	}
	return p.Path + "\t" + marker + title + "\t" + info.unresolved + "\t" + p.Name
}

// PinLast moves the project whose Path equals last to the front of ps,
// leaving the rest in their existing order. Used for the bare `dw` listing,
// which pins the most recently visited workspace to the top. A no-op when
// last is "" or not found in ps.
func PinLast(ps []Project, last string) []Project {
	if last == "" {
		return ps
	}
	idx := -1
	for i, p := range ps {
		if p.Path == last {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return ps
	}
	out := make([]Project, 0, len(ps))
	out = append(out, ps[idx])
	out = append(out, ps[:idx]...)
	out = append(out, ps[idx+1:]...)
	return out
}
