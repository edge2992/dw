package summarize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const (
	userText      = `{"type":"user","message":{"role":"user","content":"Flex Logs は高い？"}}`
	assistantTurn = `{"type":"assistant","message":{"role":"assistant","content":[` +
		`{"type":"thinking","thinking":"private reasoning"},` +
		`{"type":"text","text":"営業に確認が必要です。"},` +
		`{"type":"tool_use","name":"Bash","input":{"command":"cat .env"}}]}}`
	toolResult = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"API_KEY=sk-abcdefghijklmnopqrstuvwxyz"}]}}`
	metaEntry  = `{"type":"user","isMeta":true,"message":{"content":"<system-reminder>"}}`
	summary    = `{"type":"summary","summary":"x"}`
)

func TestReadSlice_RendersUserAndAssistantText(t *testing.T) {
	path := writeTranscript(t, userText, assistantTurn, toolResult, metaEntry, summary, "{not json", "")
	s, err := ReadSlice(path, 0, 1<<16)
	if err != nil {
		t.Fatal(err)
	}
	want := "USER: Flex Logs は高い？\n" +
		"ASSISTANT: 営業に確認が必要です。\n" +
		`TOOL Bash: {"command":"cat .env"}` + "\n"
	if s.Text != want {
		t.Errorf("text =\n%s\nwant\n%s", s.Text, want)
	}
	if s.Skipped != 1 {
		t.Errorf("skipped = %d, want 1 malformed line", s.Skipped)
	}
	if strings.Contains(s.Text, "sk-") || strings.Contains(s.Text, "private reasoning") {
		t.Error("tool results or thinking leaked into the slice")
	}
}

func TestReadSlice_StopsBeforeAPartialLine(t *testing.T) {
	path := writeTranscript(t, userText, `{"type":"assistant","mess`) // writer mid-flush
	s, err := ReadSlice(path, 0, 1<<16)
	if err != nil {
		t.Fatal(err)
	}
	if s.End != int64(len(userText)+1) {
		t.Errorf("end = %d, want just past the first line (%d)", s.End, len(userText)+1)
	}
	if s.Skipped != 0 {
		t.Errorf("the unfinished line was parsed (skipped=%d)", s.Skipped)
	}

	again, _ := ReadSlice(path, s.End, 1<<16)
	if again.Text != "" || again.End != s.End {
		t.Errorf("re-read from the offset = %+v, want nothing new", again)
	}
}

func TestReadSlice_TailCap(t *testing.T) {
	var lines []string
	for range 200 {
		lines = append(lines, userText)
	}
	path := writeTranscript(t, append(lines, "")...)
	s, err := ReadSlice(path, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Text) > 1000 {
		t.Errorf("len = %d, over the cap", len(s.Text))
	}
	if !strings.HasPrefix(s.Text, omittedMarker+"USER: ") {
		t.Errorf("text does not start with the marker and a whole line:\n%.120s", s.Text)
	}
}

func TestReadSlice_OffsetPastEndResets(t *testing.T) {
	path := writeTranscript(t, userText, "")
	s, err := ReadSlice(path, 1<<20, 1<<16) // transcript was replaced by a shorter one
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Text, "Flex Logs") {
		t.Errorf("text = %q, want the file re-read from the start", s.Text)
	}
}

func TestScrub(t *testing.T) {
	cases := []struct{ in, mustNotContain string }{
		{"key sk-proj-abcdefghijklmnop1234 here", "sk-proj-abcdefghijklmnop1234"},
		{"aws AKIAABCDEFGHIJKLMNOP", "AKIAABCDEFGHIJKLMNOP"},
		{"gh ghp_abcdefghijklmnopqrstuvwxyz0123", "ghp_abcdefghijklmnopqrstuvwxyz0123"},
		{"-----BEGIN RSA PRIVATE KEY-----\nMIIEow\n-----END RSA PRIVATE KEY-----", "MIIEow"},
		{"ok\nexport GITHUB_TOKEN=abc123\nok", "abc123"},
		{"DB_PASSWORD = hunter2", "hunter2"},
		{"aws_secret_access_key=wJalr", "wJalr"},
	}
	for _, c := range cases {
		if got := Scrub(c.in); strings.Contains(got, c.mustNotContain) {
			t.Errorf("Scrub(%q) = %q, still contains %q", c.in, got, c.mustNotContain)
		}
	}
	if got := Scrub("ok\nexport GITHUB_TOKEN=abc\nok"); got != "ok\n[REDACTED LINE]\nok" {
		t.Errorf("only the secret line should go: %q", got)
	}
	if plain := "前提を見直す。料金は 2026-10 時点で $0.10/GB"; Scrub(plain) != plain {
		t.Errorf("Scrub changed ordinary text: %q", Scrub(plain))
	}
}

func TestTruncateKeepsUTF8(t *testing.T) {
	got := truncate("あいう", 4) // あ is 3 bytes; 4 would split い
	if got != "あ…" {
		t.Errorf("truncate = %q", got)
	}
}

func TestNewestTranscript(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	ws := "/home/me/dw/2026-10-15-foo.bar"
	dir := filepath.Join(cfg, "projects", "-home-me-dw-2026-10-15-foo-bar")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old", "new"} {
		if err := os.WriteFile(filepath.Join(dir, name+".jsonl"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := mustStat(t, filepath.Join(dir, "new.jsonl")).ModTime().Add(-1e9 * 60)
	if err := os.Chtimes(filepath.Join(dir, "old.jsonl"), past, past); err != nil {
		t.Fatal(err)
	}
	session, path, err := NewestTranscript(ws)
	if err != nil {
		t.Fatal(err)
	}
	if session != "new" || path != filepath.Join(dir, "new.jsonl") {
		t.Errorf("got %q %q", session, path)
	}
	if _, _, err := NewestTranscript("/elsewhere"); err == nil {
		t.Error("want an error when there is no transcript")
	}
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}
