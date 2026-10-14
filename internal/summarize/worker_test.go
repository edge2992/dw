package summarize

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const baseState = "# Datadog コスト\n\n## 前提\n\n- (2026-10-01, 事実) Logs が主因\n\n## 却下した案\n\n## 未決の問い\n\n- Flex Logs の料金は？\n"

// fakeSummarizer replies via fn and counts calls.
type fakeSummarizer struct {
	mu    sync.Mutex
	calls int
	fn    func(req Request) (string, error)
}

func (f *fakeSummarizer) Name() string { return "fake" }

func (f *fakeSummarizer) Summarize(_ context.Context, req Request) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.fn(req)
}

func replyWith(s string) *fakeSummarizer {
	return &fakeSummarizer{fn: func(Request) (string, error) { return s, nil }}
}

type workerEnv struct {
	ws, transcript string
}

// newWorkerEnv isolates the cache dir and creates a workspace holding
// baseState plus a one-turn transcript.
func newWorkerEnv(t *testing.T) workerEnv {
	t.Helper()
	isolate(t)
	ws := filepath.Join(t.TempDir(), "2026-10-15-datadog")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ws, "STATE.md"), baseState)
	return workerEnv{ws: ws, transcript: writeTranscript(t, userText, "")}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e workerEnv) run(t *testing.T, s Summarizer) string {
	t.Helper()
	w := Worker{Config: defaults(), Summarizer: s, Now: func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC) }}
	return w.Run(context.Background(), Job{Workspace: e.ws, Session: "sess-1", Transcript: e.transcript})
}

func (e workerEnv) state(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.ws, "STATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func backups(t *testing.T, ws string) []string {
	t.Helper()
	entries, _ := os.ReadDir(HistoryDir(ws))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestWorker_WritesBacksUpAndAdvances(t *testing.T) {
	e := newWorkerEnv(t)
	updated := strings.Replace(baseState, "## 却下した案\n", "## 却下した案\n\n- 案 X は採らない。\n", 1)

	if got := e.run(t, replyWith("```markdown\n"+updated+"```")); got != resultWritten {
		t.Fatalf("result = %q", got)
	}
	if e.state(t) != updated {
		t.Errorf("STATE.md =\n%s", e.state(t))
	}
	b := backups(t, e.ws)
	if len(b) != 1 || b[0] != "2026-10-15T09:00:00Z.md" {
		t.Fatalf("backups = %v", b)
	}
	if old, _ := os.ReadFile(filepath.Join(HistoryDir(e.ws), b[0])); string(old) != baseState {
		t.Errorf("backup holds %q, want the pre-write STATE.md", old)
	}
	if readOffset("sess-1") == 0 {
		t.Error("offset did not advance after a write")
	}

	f := replyWith(updated)
	if got := e.run(t, f); got != resultNothingNew || f.calls != 0 {
		t.Errorf("second run: result=%q calls=%d, want nothing-new without an LLM call", got, f.calls)
	}
	if !strings.Contains(readLog(t), "result=written") {
		t.Errorf("log lacks the result line:\n%s", readLog(t))
	}
}

func readLog(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(LogPath())
	return string(b)
}

func TestWorker_UnchangedSkipsWriteButAdvances(t *testing.T) {
	e := newWorkerEnv(t)
	if got := e.run(t, replyWith(baseState)); got != resultUnchanged {
		t.Fatalf("result = %q", got)
	}
	if len(backups(t, e.ws)) != 0 {
		t.Error("an unchanged result made a backup")
	}
	if readOffset("sess-1") == 0 {
		t.Error("offset did not advance")
	}
}

func TestWorker_Rejections(t *testing.T) {
	big := strings.Replace(baseState, "## 前提\n", "## 前提\n\n"+strings.Repeat("- 長い前提の行\n", 100), 1)
	cases := []struct {
		name, old, reply, want string
	}{
		{"empty", baseState, "  \n", "rejected:empty"},
		{"renamed title", baseState, strings.Replace(baseState, "# Datadog コスト", "# Something else", 1), "rejected:title-changed"},
		{"lost open-questions heading", baseState, strings.Replace(baseState, "## 未決の問い", "## Open", 1), "rejected:missing-open-questions"},
		{"shrunk", big, "# Datadog コスト\n\n## 未決の問い\n\n- Flex Logs の料金は？\n", "rejected:shrunk"},
		{"dropped open question", baseState, strings.Replace(baseState, "- Flex Logs の料金は？\n", "", 1), "rejected:dropped-open-question"},
		{"question moved, not dropped", baseState,
			strings.Replace(strings.Replace(baseState, "- Flex Logs の料金は？\n", "", 1),
				"## 却下した案", "- （解決 2026-10-15）Flex Logs の料金は？ → 0.1$/GB\n\n## 却下した案", 1),
			resultWritten},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newWorkerEnv(t)
			writeFile(t, filepath.Join(e.ws, "STATE.md"), c.old)
			if got := e.run(t, replyWith(c.reply)); got != c.want {
				t.Fatalf("result = %q, want %q", got, c.want)
			}
			if strings.HasPrefix(c.want, "rejected:") {
				if e.state(t) != c.old {
					t.Error("a rejected reply was written")
				}
				if readOffset("sess-1") != 0 {
					t.Error("offset advanced on rejection")
				}
				if strings.Contains(readLog(t), c.reply) && strings.TrimSpace(c.reply) != "" {
					t.Error("rejected content was logged")
				}
			}
		})
	}
}

func TestWorker_ConflictRetriesThenGivesUp(t *testing.T) {
	e := newWorkerEnv(t)
	updated := baseState + "\n"
	statePath := filepath.Join(e.ws, "STATE.md")

	// The human edits STATE.md during every call: retry once, then give up.
	f := &fakeSummarizer{}
	f.fn = func(req Request) (string, error) {
		writeFile(t, statePath, req.State+"<!-- human edit -->\n")
		return req.State + "- extra\n", nil
	}
	if got := e.run(t, f); got != resultConflict || f.calls != 2 {
		t.Fatalf("result=%q calls=%d, want conflict after 2 calls", got, f.calls)
	}
	if readOffset("sess-1") != 0 {
		t.Error("offset advanced on conflict")
	}

	// An edit during the first call only: the retry sees it and wins.
	writeFile(t, statePath, baseState)
	f = &fakeSummarizer{}
	f.fn = func(req Request) (string, error) {
		if f.calls == 1 {
			writeFile(t, statePath, updated)
		}
		return req.State + "- extra\n", nil
	}
	if got := e.run(t, f); got != resultWritten {
		t.Fatalf("result = %q", got)
	}
	if e.state(t) != updated+"- extra\n" {
		t.Errorf("STATE.md = %q, want the human edit kept", e.state(t))
	}
}

func TestWorker_BackendErrorAndStatus(t *testing.T) {
	e := newWorkerEnv(t)
	fail := &fakeSummarizer{fn: func(Request) (string, error) { return "", errors.New("HTTP 401: bad key\nmore") }}
	for range failureThreshold {
		if got := e.run(t, fail); got != "error:HTTP 401: bad key more" {
			t.Fatalf("result = %q", got)
		}
	}
	if _, failing := FailingSince(e.ws); !failing {
		t.Error("not failing after the threshold")
	}
	if got := e.run(t, replyWith(baseState+"- x\n")); got != resultWritten {
		t.Fatalf("result = %q", got)
	}
	if _, failing := FailingSince(e.ws); failing {
		t.Error("a success did not reset the failure streak")
	}
}

func TestWorker_RecoversPanics(t *testing.T) {
	e := newWorkerEnv(t)
	boom := &fakeSummarizer{fn: func(Request) (string, error) { panic("boom") }}
	if got := e.run(t, boom); !strings.HasPrefix(got, "error:panic: boom") {
		t.Errorf("result = %q", got)
	}
}

func TestWorker_ConcurrentRunsSerialise(t *testing.T) {
	e := newWorkerEnv(t)
	f := &fakeSummarizer{fn: func(req Request) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return req.State + "- x\n", nil
	}}
	results := make([]string, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i] = e.run(t, f) })
	}
	wg.Wait()
	got := results[0] + "," + results[1]
	if got != "written,nothing-new" && got != "nothing-new,written" {
		t.Errorf("results = %s, want one write and one nothing-new", got)
	}
	if f.calls != 1 {
		t.Errorf("LLM called %d times, want 1", f.calls)
	}
}

// TestWorker_DebounceCollapsesBurst: a turn appended while the first worker
// is still debouncing is folded into that same single LLM call.
func TestWorker_DebounceCollapsesBurst(t *testing.T) {
	e := newWorkerEnv(t)
	var seen string
	f := &fakeSummarizer{fn: func(req Request) (string, error) {
		seen = req.Transcript
		return req.State + "- x\n", nil
	}}
	go func() {
		time.Sleep(50 * time.Millisecond)
		tf, err := os.OpenFile(e.transcript, os.O_APPEND|os.O_WRONLY, 0)
		if err == nil {
			_, _ = tf.WriteString(`{"type":"user","message":{"content":"second turn"}}` + "\n")
			_ = tf.Close()
		}
	}()
	w := Worker{Config: defaults(), Summarizer: f}
	start := time.Now()
	got := w.Run(context.Background(), Job{Workspace: e.ws, Session: "sess-1", Transcript: e.transcript, Debounce: 300 * time.Millisecond})
	if got != resultWritten {
		t.Fatalf("result = %q", got)
	}
	if time.Since(start) < 300*time.Millisecond {
		t.Error("debounce not applied")
	}
	if f.calls != 1 || !strings.Contains(seen, "Flex Logs") || !strings.Contains(seen, "second turn") {
		t.Errorf("calls=%d transcript=%q, want both turns in one call", f.calls, seen)
	}
}

func TestWorker_LockBusy(t *testing.T) {
	e := newWorkerEnv(t)
	saved := lockWait
	lockWait = 50 * time.Millisecond
	defer func() { lockWait = saved }()
	unlock, err := lock(context.Background(), lockPath(e.ws), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if got := e.run(t, replyWith(baseState)); got != resultBusy {
		t.Errorf("result = %q, want busy", got)
	}
}

func TestWorker_RejectsUnsafeSessionID(t *testing.T) {
	e := newWorkerEnv(t)
	w := Worker{Config: defaults(), Summarizer: replyWith(baseState)}
	if got := w.Run(context.Background(), Job{Workspace: e.ws, Session: "../../etc", Transcript: e.transcript}); !strings.HasPrefix(got, "error:") {
		t.Errorf("result = %q", got)
	}
}

func TestBackup_KeepsLast20(t *testing.T) {
	isolate(t)
	ws := "/tmp/ws"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 25 {
		if err := backup(ws, "v", start.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	b := backups(t, ws)
	if len(b) != keepBackups || b[0] != "2026-01-01T00:05:00Z.md" {
		t.Errorf("kept %d, oldest %q; want the newest 20", len(b), b[0])
	}
}
