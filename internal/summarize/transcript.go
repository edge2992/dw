package summarize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Claude Code's transcript is JSONL, one entry per line. Its format is
// undocumented, so parsing is defensive: unknown entry and block types are
// ignored, and lines that fail to parse are skipped and counted.
type transcriptEntry struct {
	Type        string `json:"type"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// maxToolInput caps what is kept of a tool call's input: enough to show what
// was touched, not what it contained.
const maxToolInput = 200

const omittedMarker = "[earlier turns omitted]\n"

// Slice is the rendered part of a transcript a worker read.
type Slice struct {
	Text    string // scrubbed USER/ASSISTANT/TOOL lines, tail-capped
	End     int64  // offset just past the last complete line consumed
	Skipped int    // lines that failed to parse
}

// ReadSlice renders the transcript at path from offset to its last complete
// line. If the file is now shorter than offset (rewritten or replaced), it
// restarts from the last maxBytes. The raw read is bounded too, and the
// rendered text keeps only its last maxBytes, on a line boundary.
func ReadSlice(path string, offset int64, maxBytes int) (Slice, error) {
	f, err := os.Open(path)
	if err != nil {
		return Slice{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Slice{}, err
	}
	size := fi.Size()
	if offset > size {
		offset = max(0, size-int64(maxBytes))
	}
	// Raw JSONL is mostly tool results that rendering drops, so allow far
	// more raw input than rendered output — but not without limit.
	start := max(offset, size-64*int64(maxBytes))
	raw, err := io.ReadAll(io.NewSectionReader(f, start, size-start))
	if err != nil {
		return Slice{}, err
	}
	if start > offset { // landed mid-line: skip to the next full one
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			return Slice{End: offset}, nil
		}
		raw, start = raw[i+1:], start+int64(i+1)
	}
	complete := bytes.LastIndexByte(raw, '\n') + 1 // the last line may still be mid-flush
	s := Slice{End: start + int64(complete)}

	var b strings.Builder
	for _, line := range bytes.Split(raw[:complete], []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e transcriptEntry
		if err := json.Unmarshal(line, &e); err != nil {
			s.Skipped++
			continue
		}
		b.WriteString(renderEntry(e))
	}
	s.Text = tailCap(b.String(), maxBytes)
	return s, nil
}

// renderEntry turns one entry into "USER: …", "ASSISTANT: …" and
// "TOOL <name>: <input>" lines. Tool results, thinking and images are
// dropped: they are the least State-worthy input and the likeliest to carry
// secrets (`cat .env`, logs).
func renderEntry(e transcriptEntry) string {
	if e.IsMeta || e.IsSidechain || (e.Type != "user" && e.Type != "assistant") {
		return ""
	}
	role := strings.ToUpper(e.Type)
	var text string
	if err := json.Unmarshal(e.Message.Content, &text); err == nil {
		return renderLine(role, text)
	}
	var blocks []contentBlock
	if err := json.Unmarshal(e.Message.Content, &blocks); err != nil {
		return ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			b.WriteString(renderLine(role, bl.Text))
		case "tool_use":
			b.WriteString(renderLine("TOOL "+bl.Name, truncate(string(bl.Input), maxToolInput)))
		}
	}
	return b.String()
}

func renderLine(label, text string) string {
	text = strings.TrimSpace(Scrub(text))
	if text == "" {
		return ""
	}
	return label + ": " + text + "\n"
}

// tailCap keeps the last maxBytes of s, starting on a line boundary, and
// marks the cut.
func tailCap(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	tail := s[len(s)-max(0, maxBytes-len(omittedMarker)):]
	if i := strings.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	return omittedMarker + tail
}

// truncate cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

// NewestTranscript finds the most recently modified transcript Claude Code
// keeps for sessions run in ws, under
// <claude config dir>/projects/<ws, every non-ASCII-alphanumeric char as "-">/,
// and returns its session ID and path. It backs the manual `dw summarize`.
func NewestTranscript(ws string) (session, path string, err error) {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".claude")
	}
	dir := filepath.Join(base, "projects", nonAlnum.ReplaceAllString(ws, "-"))
	matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	var newest time.Time
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err == nil && fi.ModTime().After(newest) {
			newest, path = fi.ModTime(), m
		}
	}
	if path == "" {
		return "", "", fmt.Errorf("no transcript under %s", dir)
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl"), path, nil
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

var (
	pemBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*-----.*?(-----END [A-Z0-9 ]*-----|$)`)
	// A whole line assigning something whose name says it is secret:
	// API_KEY=…, export GITHUB_TOKEN=…, DB_PASSWORD = ….
	secretAssignment = regexp.MustCompile(`(?im)^.*(key|token|secret|password)[a-z0-9_]*["']?\s*=.*$`)
	secretToken      = regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{20,}`)
)

// Scrub redacts the obvious credential shapes from text before it leaves the
// machine. It is a seatbelt, not a guarantee — which is why tool results are
// not sent at all.
func Scrub(text string) string {
	text = pemBlock.ReplaceAllString(text, "[REDACTED PEM]")
	text = secretAssignment.ReplaceAllString(text, "[REDACTED LINE]")
	return secretToken.ReplaceAllString(text, "[REDACTED]")
}
