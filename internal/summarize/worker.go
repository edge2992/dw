package summarize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/edge2992/dw/internal/workspace"
)

// Run results, as logged after "result=". Errors and rejections are logged as
// "error:<msg>" and "rejected:<reason>".
const (
	resultWritten    = "written"
	resultUnchanged  = "unchanged"
	resultNothingNew = "nothing-new"
	resultBusy       = "busy"
	resultConflict   = "conflict"
)

// lockWait bounds how long a worker queues behind another one for the same
// workspace. Giving up is safe: the offset has not moved, so the next turn's
// worker picks the slice up.
var lockWait = 30 * time.Second

var errLockBusy = errors.New("workspace lock busy")

// Session IDs become file names, so they are held to a safe alphabet.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Job is one summarize run for one Claude Code session in one workspace.
type Job struct {
	Workspace  string
	Session    string
	Transcript string
	// Debounce is slept before taking the lock, so a burst of turns
	// collapses into a single LLM call: the first worker to get through
	// reads every turn written meanwhile, and the rest find nothing new.
	Debounce time.Duration
}

// Worker folds a transcript slice into STATE.md.
type Worker struct {
	Config     Config
	Summarizer Summarizer
	Now        func() time.Time // defaults to time.Now
}

type runStats struct {
	in, out, skipped int
}

// Run performs one job and returns its result. It never panics: a panic is
// recovered, logged with its stack, and reported as an error. Every run
// appends one line to the log and updates the workspace's status.
func (w Worker) Run(ctx context.Context, job Job) (result string) {
	start := time.Now()
	var st runStats
	defer func() {
		if r := recover(); r != nil {
			Logf("panic: %v\n%s", r, debug.Stack())
			result = fmt.Sprintf("error:panic: %v", r)
		}
		recordStatus(job.Workspace, result, w.now())
		Logf("ws=%s session=%s backend=%s in=%s out=%s skipped=%d dur=%s result=%s",
			shortWS(job.Workspace), shortSession(job.Session), w.Summarizer.Name(),
			kb(st.in), kb(st.out), st.skipped, time.Since(start).Round(100*time.Millisecond), result)
	}()
	return w.run(ctx, job, &st)
}

func (w Worker) run(ctx context.Context, job Job, st *runStats) string {
	if !sessionIDPattern.MatchString(job.Session) {
		return "error:invalid session id"
	}
	if job.Debounce > 0 {
		select {
		case <-ctx.Done():
			return errorResult(ctx.Err())
		case <-time.After(job.Debounce):
		}
	}
	unlock, err := lock(ctx, lockPath(job.Workspace), lockWait)
	if errors.Is(err, errLockBusy) {
		return resultBusy
	}
	if err != nil {
		return errorResult(fmt.Errorf("lock: %w", err))
	}
	defer unlock()

	offset := readOffset(job.Session)
	slice, err := ReadSlice(job.Transcript, offset, w.Config.MaxInputBytes)
	if err != nil {
		return errorResult(fmt.Errorf("transcript: %w", err))
	}
	st.skipped = slice.Skipped
	if slice.Text == "" {
		if slice.End != offset {
			saveOffset(job.Session, slice.End)
		}
		return resultNothingNew
	}

	ctx, cancel := context.WithTimeout(ctx, w.Config.Timeout)
	defer cancel()
	now := w.now()
	// A human editing STATE.md during the LLM call makes the write miss its
	// compare-and-swap; redo the call once against their version.
	for range 2 {
		old, sum, err := workspace.ReadStateFile(job.Workspace)
		if err != nil {
			return errorResult(err)
		}
		req := Request{Date: now.Format("2006-01-02"), State: old, Transcript: slice.Text}
		st.in = len(userMessage(req))
		out, err := w.Summarizer.Summarize(ctx, req)
		if err != nil {
			return errorResult(err)
		}
		st.out = len(out)
		out, err = validate(old, out)
		if err != nil {
			return "rejected:" + err.Error()
		}
		if strings.TrimSpace(out) == strings.TrimSpace(old) {
			saveOffset(job.Session, slice.End)
			return resultUnchanged
		}
		if err := backup(job.Workspace, old, now); err != nil {
			return errorResult(fmt.Errorf("backup: %w", err))
		}
		err = workspace.WriteStateAtomic(job.Workspace, out, sum)
		if errors.Is(err, workspace.ErrStateChanged) {
			continue
		}
		if err != nil {
			return errorResult(err)
		}
		saveOffset(job.Session, slice.End)
		return resultWritten
	}
	return resultConflict
}

func (w Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func lockPath(ws string) string {
	return filepath.Join(workspace.CacheDir(), "locks", wsKey(ws)+".lock")
}

func errorResult(err error) string {
	return "error:" + truncate(strings.ReplaceAll(err.Error(), "\n", " "), 300)
}

func kb(n int) string { return fmt.Sprintf("%.1fKB", float64(n)/1024) }

// The offset is how far into a session's transcript STATE.md already
// reflects. It only moves after a successful write (or a deliberate no-op),
// so a failed run leaves its slice for the next one.
func offsetPath(session string) string {
	return filepath.Join(workspace.CacheDir(), "offsets", session)
}

func readOffset(session string) int64 {
	b, err := os.ReadFile(offsetPath(session))
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func saveOffset(session string, off int64) {
	path := offsetPath(session)
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err == nil {
		err = os.WriteFile(path, []byte(strconv.FormatInt(off, 10)+"\n"), 0o600)
	}
	if err != nil {
		Logf("session=%s: could not save offset: %v", shortSession(session), err)
	}
}

// maxStateBytes caps an acceptable STATE.md.
const maxStateBytes = 256 << 10

// rejection is why validate refused a model's output; it is logged as
// "rejected:<reason>", never with the output itself.
type rejection string

func (r rejection) Error() string { return string(r) }

var listMarker = regexp.MustCompile(`^(?:[-*+]|\d+[.)])\s+`)

// validate checks a model's proposed STATE.md against the current one before
// anything is written, and returns it normalised (one code fence stripped,
// trailing newline). The checks catch gross damage, not subtle drift; the
// rolling backups are for the rest.
func validate(old, out string) (string, error) {
	out = stripFence(strings.TrimSpace(out))
	switch {
	case out == "":
		return "", rejection("empty")
	case len(out) > maxStateBytes:
		return "", rejection("too-large")
	case workspace.Title(out) != workspace.Title(old):
		// dw's listing shows this title; the summarizer may not rename the topic.
		return "", rejection("title-changed")
	}
	if _, found := workspace.OpenQuestions(out); !found {
		return "", rejection("missing-open-questions")
	}
	if len(old) > 1024 && len(out)*10 < len(old)*4 {
		return "", rejection("shrunk")
	}
	// An open question may be answered and moved, but its text has to survive
	// somewhere: silently dropping one is the erosion this check exists for.
	oldQuestions, _ := workspace.OpenQuestions(old)
	for _, q := range oldQuestions {
		if !strings.Contains(out, listMarker.ReplaceAllString(q, "")) {
			return "", rejection("dropped-open-question")
		}
	}
	return out + "\n", nil
}

// stripFence removes one ``` fence wrapped around the whole reply.
func stripFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return s
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s[i+1:]), "```"))
}
