# Design: background STATE.md writer and a create-only CLI

Status: draft · Branch: `rework/async-state` · Breaking: yes (single user, no migration shim)

## 1. Summary

Today `STATE.md` is updated by whoever happens to be in the conversation: the
interactive Claude session is told to edit it directly
(`internal/workspace/assets/CLAUDE.md`, since `fcb278c`). This rework moves that
job out of the conversation. After every Claude Code turn in a workspace, a
`Stop` hook hands the turn off to a detached `dw` worker. The worker sends the
current `STATE.md` plus the turn's input/output to a cheap LLM and atomically
writes back the rewritten `STATE.md`. The session itself never waits for it.

The LLM is any OpenAI-compatible `/chat/completions` endpoint, configured
through a config file or env vars. When no endpoint is configured, dw falls back
to `claude -p` with a small model. The interactive CLI stays limited to creating
and opening ("digging") workspace directories.

## 2. Goals and non-goals

### Goals

1. **Async state capture.** After each user action (defined in §4.1), its
   input, output and results get folded into the topic's `STATE.md` in the
   background. The Claude Code session never blocks on it, and a failed
   summary never breaks the session.
2. **Cheap by default.** Each turn makes one small request: the current
   `STATE.md` plus a bounded transcript slice. The fallback defaults to
   `claude -p --model haiku`. There is no built-in default model for the HTTP
   backend, so the user always picks one explicitly (see §5.3).
3. **Works with no configuration.** No key or model configured → `claude -p`.
   No `claude` on `PATH` either → summaries are off, logged once, and nothing
   else changes.
4. **Configurable endpoint.** Base URL, model and API key come from env vars or
   a JSON config file, with one documented precedence order.
5. **Minimal CLI.** The user-facing surface is still `dw [topic]`, plus
   `init`/`version`/`help`. Two plumbing commands (`hook`, `summarize`) are
   added for the trigger and the worker.

### Non-goals

- Capturing actions outside Claude Code, such as shell commands run in a
  workspace or editor saves. That would need a `precmd` hook in the `dw init`
  wrapper. Deferred (§10, Q6).
- Keeping or replaying conversation history. Claude Code still owns transcripts.
  dw only remembers a byte offset into each transcript (§4.4).
- A conversation-log section in `STATE.md`. Summaries are folded into the
  existing sections, in line with the "書かないこと" rule in
  `assets/CLAUDE.md` (§10, Q3).
- Streaming, tool use, or multi-turn agent behaviour in the summarizer. It is
  one request and one response.
- Windows support for the worker (§4.5, §10, Q8).
- New third-party Go dependencies. The stdlib-only promise in `README.md`
  stays: `net/http`, `encoding/json`, `os/exec`, `syscall`.

## 3. Current state (as of `1e9cf8c`)

| File | Role today | Effect of rework |
| --- | --- | --- |
| `main.go` | `run()` dispatches `init`, `version`, `help`; anything else is a topic → `cmdResolve`; no args → `cmdList`. Also embeds the `shellInit` wrapper (fzf picker, `cd`). | Add `hook` and `summarize` to the dispatch and to the wrapper's passthrough `case`. `run` gains an `io.Reader` stdin. `cmdHelp` prints the effective summarizer backend. |
| `internal/workspace/workspace.go` | `Root()` (`$DW_ROOT` or `~/dw`), `Slugify`, `Scan`, `Create` (writes `STATE.md` and root `CLAUDE.md` via `writeIfAbsent`), `Resolve`. | `Create` also scaffolds `<ws>/.claude/settings.json`. New `EnsureScaffold` backfills older workspaces. New `IsWorkspace(root, dir)` for the hook. |
| `internal/workspace/state.go` | `stateTemplate`, `unresolvedHeading` (`## 未決の問い`), `readState` / `TSVRow` for the listing. | New `ReadStateFile` / `WriteStateAtomic` (compare-and-swap). The template and headings become a contract the summarizer output is checked against (§4.6). |
| `internal/workspace/last.go` | "Last visited" pin in `os.UserCacheDir()/dw/last`. | Unchanged. Its cache dir becomes the home for summarizer state (§4.4). |
| `internal/workspace/convention.go` + `assets/CLAUDE.md` | Root convention file, written once. It currently tells the **interactive** LLM to edit `STATE.md` directly. | Rewritten: the session reads `STATE.md` but no longer writes it (§4.7). |
| `docs/concepts.md` | States that State is "原則として人間が所有する" and warns that auto-generation degrades into a conversation summary. | Must be revised. This is the main conceptual conflict (§10, Q1). |
| `README.md` | Promises "zero config", "`DW_ROOT` is the only knob", and "never launches `claude` itself". | All three promises become false and must be rewritten (§7). |
| `.goreleaser.yaml` | Builds linux/darwin/windows. | Drop windows (§4.5). |

Prior art in git history:

- `2dc5b39` (v0.7.0) scaffolded a per-workspace `.claude/settings.json` with a
  `Stop` hook (`checkpoint.sh`). The hook copied `last_assistant_message` into
  `.dw/last-session.md`. Lessons carried over here:
  - Use `$CLAUDE_PROJECT_DIR`, not the payload `cwd`.
  - A per-workspace hook is better than a global one.
  - `SessionEnd` only has a ~1.5 s budget, so `Stop` is the right event.
  - A `Stop` hook must always exit 0.
- `3d6130b` (v1.0.0) removed that hook, the YAML config (`internal/config`,
  added in `42447be`), the TUI and categories, in favour of today's minimal
  core.

**Already gone:** the code no longer has any editing or conversation-history
feature. What remains of "editing" is the convention telling the session LLM to
write `STATE.md`, plus the "閉じる" procedure in `concepts.md` and
`assets/CLAUDE.md`. §7 covers those.

## 4. Architecture

```text
 Claude Code session in <root>/<date>-<topic>/
   │  turn ends
   ▼
 Stop hook  (.claude/settings.json → `dw hook`)
   │  reads hook JSON on stdin, validates workspace, spawns worker, exit 0   (< 50 ms)
   ▼
 dw summarize <ws> --session <id> --transcript <path>     (detached, own session)
   │  1. flock  ~/.cache/dw/locks/<hash(ws)>.lock   (blocking → serialises per workspace)
   │  2. read transcript from saved offset → bounded slice
   │  3. read STATE.md + sha256
   │  4. Summarizer.Summarize(ctx, {state, slice, date})
   │        ├─ openaiBackend  POST {base_url}/chat/completions
   │        └─ claudeBackend  claude -p --model haiku … (cwd=$TMPDIR)
   │  5. validate output
   │  6. CAS: re-hash STATE.md; equal → atomic write; else retry once
   │  7. advance offset; write status; log one line
   ▼
 <ws>/STATE.md   (read by the next session, the fzf preview, and the dw listing)
```

New package `internal/summarize`. Its only consumer is `main.go`, so it stays a
single package:

| File | Contents |
| --- | --- |
| `config.go` | `Config`, `Load()`: precedence and backend selection (§5) |
| `backend.go` | `type Summarizer interface { Summarize(ctx, Request) (string, error) }`, `New(Config)` |
| `openai.go` | OpenAI-compatible HTTP backend |
| `claudecli.go` | `claude -p` backend |
| `transcript.go` | JSONL slice reader and renderer, offsets |
| `worker.go` | `Run(ctx, ws, session, transcript)`: steps 1–7 above |
| `prompt.go` + `assets/prompt.md` | System prompt, `go:embed` (same pattern as `convention.go`) |
| `lock_unix.go`, `detach_unix.go` | `syscall.Flock`, `SysProcAttr{Setsid: true}` (`//go:build unix`) |
| `log.go` | Append-only log with size cap |

### 4.1 Trigger: what counts as a "user action"

A **user action** is one Claude Code turn: a user prompt, every tool call and
result it caused, and the final assistant message. A turn ends with Claude
Code's `Stop` event. `Stop` fires once per turn, after the output exists, and
its payload carries `session_id` and `transcript_path`. `UserPromptSubmit`
fires before any output exists. `SessionEnd` is too late and has too little
budget (lesson from `2dc5b39`).

`Create` scaffolds the hook into each workspace (embedded
`internal/workspace/assets/claude-settings.json`, written with
`writeIfAbsent`):

```json
{
  "hooks": {
    "Stop": [{ "hooks": [{ "type": "command", "command": "dw hook" }] }]
  }
}
```

`cmdResolve` calls `workspace.EnsureScaffold(path)` whenever it settles on
exactly one workspace (the same place it calls `SaveLast`). That backfills the
hook into workspaces created before this change. There is no `dw scaffold`
command; it was removed in `3d6130b` and stays removed. If a workspace already
has its own `.claude/settings.json`, dw does not merge into it. In that case
the user adds the hook by hand (§10, Q5). Opting a workspace out is just
deleting the hook entry; no extra flag is needed.

### 4.2 `dw hook`: fast and always exit 0

`cmdHook(stdin, stderr)`:

1. If `DW_SUMMARIZER=1` is set, exit 0. This guard stops recursion when the
   worker's own `claude -p` fires hooks (§6).
2. Decode stdin: `{session_id, transcript_path, stop_hook_active, …}`. Decode
   errors → log, exit 0.
3. Take `ws := $CLAUDE_PROJECT_DIR`. Require
   `workspace.IsWorkspace(Root(), ws)`, meaning `ws` is a direct,
   non-dot child of `Root()` (same rule as `Scan`). Otherwise exit 0 silently.
4. Load the config. If the backend is `off`, exit 0.
5. Spawn the worker detached (§4.3), then exit 0.

`dw hook` never writes to stdout. For `Stop` hooks, stdout and exit code 2 have
meaning to Claude Code ("keep going"). Every error path ends in `return 0`.
Spawning goes through a package-level `var spawnWorker = …` so tests can swap
it out, the same pattern as `isTerminal` in `main.go`.

### 4.3 Detachment

```go
cmd := exec.Command(self, "summarize", ws, "--session", id, "--transcript", tp)
cmd.Stdin = nil                       // /dev/null
cmd.Stdout, cmd.Stderr = logFile, logFile // NOT the hook's pipes
cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
cmd.Start(); cmd.Process.Release()
```

- `self` comes from `os.Executable()`, so the worker is the same binary as the
  hook.
- **Do not inherit stdout/stderr.** Claude Code waits for the hook's output
  pipes to close, so an inherited pipe would make the hook block for the whole
  LLM call. That would defeat the point of the design.
- `Setsid` takes the worker out of the hook's process group and session, so
  hook timeouts, Ctrl-C and closing the terminal do not kill it.
- The worker's own deadline is `timeout` (default 120 s) via
  `context.WithTimeout`, plus 30 s for the lock wait. A wedged worker can never
  outlive that.

### 4.4 Coalescing, offsets and locking

There is no queue. Each turn spawns a worker, and the workers serialise on a
**blocking** `flock(LOCK_EX)` of `~/.cache/dw/locks/<sha256(ws)[:16]>.lock`.
The lock wait is bounded at 30 s by polling `LOCK_NB`; if it times out, the
worker exits and the next turn picks up the slack.

Per-transcript progress is stored in
`~/.cache/dw/offsets/<session_id>` as a decimal byte offset. Session IDs are
globally unique, so no namespacing is needed. Once a worker holds the lock, it
reads from the offset to the current end of the file. If nothing is new (an
earlier worker already covered this turn), it exits right away. A burst of
turns therefore collapses into one or two LLM calls, without lost wakeups or
per-event bookkeeping.

The offset only advances after `STATE.md` has been written successfully. If the
transcript is shorter than the offset (rewritten or replaced), the worker resets
the offset to `max(0, size - max_input_bytes)`.

Summarizer state lives under `os.UserCacheDir()/dw/`, next to `last` from
`last.go`, and **not inside the workspace**. Anything in the workspace is
context the next session may read (the "Isolation" idea in `concepts.md`). The
workspace gains exactly one new file, `.claude/settings.json`.

### 4.5 Atomic, compare-and-swap writes

`workspace.WriteStateAtomic(dir, content string, expect [32]byte) error`:

1. Re-read `STATE.md` and hash it. If the hash differs from `expect`, return
   `ErrStateChanged`. Somebody (the human, or an editor) changed the file
   during the LLM call.
2. `os.CreateTemp(dir, ".STATE.md.tmp-*")` in the **same directory**, so the
   rename stays on one filesystem. Then write, `f.Sync()`, close, and `chmod` to
   match the original mode.
3. `os.Rename(tmp, STATE.md)`, then fsync the directory as best effort.
4. On any error, remove the temp file.

The flock serialises dw writers. CAS protects against writers that do not use
the lock. On `ErrStateChanged`, the worker repeats steps 3–6 once with the
fresh file. If the second attempt also conflicts, it gives up without moving
the offset, so the next turn retries with that slice included.

Windows: the stdlib has no `flock` or `Setsid` equivalent without
`golang.org/x/sys`. Remove `windows` from `.goreleaser.yaml`. `dw hook` gets a
`//go:build !unix` stub that just returns 0.

### 4.6 Prompt, input and output validation

**Input** (one user message):

- Today's date.
- The current `STATE.md`, verbatim.
- The transcript slice from `transcript.go`.

The slice is rendered as plain text:

```text
USER: <prompt text>
ASSISTANT: <text blocks>
TOOL Bash: <input, ≤300 bytes> → <result, ≤1000 bytes>
```

Thinking blocks, `isMeta` entries and image blocks are dropped. Lines that fail
to parse are skipped and counted in the log. The slice is capped to its **tail**
at `max_input_bytes` (default 64 KiB), with an `[earlier turns omitted]` marker.

**System prompt** (`assets/prompt.md`) restates the rules from
`assets/CLAUDE.md` "`STATE.md` に書くこと". It instructs the model to:

- Return the complete new `STATE.md` and nothing else.
- Keep the H1 and the three `##` headings from `stateTemplate`.
- Reorganise instead of appending.
- Date every premise and split facts from judgements.
- Write rejections conditionally.
- Never include conversation narration, raw logs or code.
- Return the input unchanged when the turn taught nothing durable.

**Validation** runs before any write:

- Strip one surrounding ```` ``` ```` fence if present.
- Non-empty and under 256 KiB.
- The first `# ` heading equals the current one. The summarizer may not rename
  the topic; the listing title comes from `firstHeading`.
- Contains `unresolvedHeading`. The listing's open-question column depends on
  it (`state.go`).
- Shrink guard: if the old file is over 1 KiB and the new one is under 40% of
  its size, reject the output.
- Output byte-identical to the input → skip the write, but still advance the
  offset.

Rejected output is logged with its reason and size, not its content. Nothing is
written and the offset stays put.

### 4.7 Single writer

The background worker becomes the only automated writer of `STATE.md`. The
human can still edit it at any time, and CAS keeps those edits safe.
`assets/CLAUDE.md` changes from "LLM が `STATE.md` を直接更新する" to roughly:
"`STATE.md` is maintained by dw in the background after each turn; read it at
the start; do not edit it; anything the user asks to record should be said
explicitly in the conversation so the summarizer sees it". If the session and
the worker both wrote, they would race on every turn. CAS would then throw away
the worker's output most of the time.

### 4.8 Logging and status

- **Log:** `~/.cache/dw/summarize.log`, one line per run:

  ```text
  2026-10-14T10:02:03Z ws=2026-10-14-foo session=ab12… backend=openai model=gpt-x in=12.3KB out=2.1KB dur=3.4s result=written|unchanged|nothing-new|rejected:<reason>|conflict|error:<msg>
  ```

  Once the log passes 1 MiB, it is rotated to `summarize.log.1` (one
  generation). Detached worker stderr goes to the same file, which captures
  panics. API keys and request bodies are never logged. HTTP error bodies are
  truncated to 200 bytes.
- **Status:** `~/.cache/dw/status/<hash(ws)>.json` holds
  `{last_ok, last_error, consecutive_failures}`. On a single-match
  `dw <topic>`, `cmdResolve` reads it. If `consecutive_failures >= 3`, it
  prints one stderr line:
  `dw: warning: STATE.md summaries failing since <time>; see <log path>`.
  stderr is not captured by the wrapper's `$(…)`, so the user sees this at the
  moment they re-enter the topic. The only other place errors surface is the
  log.

### 4.9 Failure handling

| Failure | Behaviour |
| --- | --- |
| Hook payload malformed, or not a workspace | Exit 0, log at most one line, no worker |
| Backend `off` (no config, no `claude`) | Hook exits 0. `dw help` shows `Summarizer: off (…reason…)` |
| Lock wait > 30 s | Worker exits. Offset untouched; next turn covers it |
| HTTP 429 / 5xx / network error | 2 retries, backoff 2 s and 8 s, inside the overall `timeout` |
| HTTP 4xx other than 429 (bad key, bad model) | No retry. `error:` logged, status failure count +1 |
| `claude` exits non-zero / times out | Same as a 4xx: no retry. The process gets a `context` kill |
| Output fails validation | `rejected:<reason>`. No write, offset untouched |
| CAS conflict twice | `conflict`. No write, offset untouched |
| Panic in worker | Recovered at the top of `Run`, logged with stack, status failure +1 |
| Persistent failure | Slices stay bounded by the `max_input_bytes` tail cap, so a retried slice never grows without limit. Older turns drop out of the slice, but they are never lost: the transcript itself stays in Claude Code's store |

## 5. Config and endpoint

### 5.1 Sources and precedence

Highest precedence first:

1. Environment variable. An empty value counts as unset, same as `Root()`
   treats `DW_ROOT`.
2. Config file: `$DW_CONFIG` if set, else `os.UserConfigDir()/dw/config.json`
   (`~/.config/dw/config.json` on Linux, `~/Library/Application Support/dw/config.json`
   on macOS).
3. Built-in default.

There are no CLI flags for these settings. The worker is spawned by a hook,
where flags cannot be passed. The format is JSON because `encoding/json` is
stdlib, which keeps the zero-dependency promise. YAML (`42447be`) would bring
back `yaml.v3`. The file is decoded with `DisallowUnknownFields`, so typos are
reported as errors in the log and in `dw help`, not silently ignored. If the
file contains `api_key` and is group- or world-readable (`mode & 0o077 != 0`),
dw logs a warning.

### 5.2 Keys

| Config key | Env var | Default | Notes |
| --- | --- | --- | --- |
| `backend` | `DW_SUMMARY_BACKEND` | `auto` | `auto` \| `openai` \| `claude` \| `off` |
| `base_url` | `DW_LLM_BASE_URL` | `https://api.openai.com/v1` | Must include the version path. Trailing `/` trimmed. dw posts to `{base_url}/chat/completions` |
| `api_key` | `DW_LLM_API_KEY` | `""` | Sent as `Authorization: Bearer …`, omitted when empty (local servers) |
| `model` | `DW_LLM_MODEL` | `""` | No default. The user picks a cheap model for their endpoint |
| `claude_command` | `DW_CLAUDE_COMMAND` | `claude` | Looked up on `PATH` |
| `claude_model` | `DW_CLAUDE_MODEL` | `haiku` | Alias resolved by Claude Code to the current Haiku |
| `timeout` | `DW_SUMMARY_TIMEOUT` | `120s` | `time.ParseDuration` |
| `max_input_bytes` | `DW_SUMMARY_MAX_INPUT` | `65536` | Transcript slice cap |

`DW_ROOT` stays env-only for now (§10, Q7). `OPENAI_API_KEY` and
`OPENAI_BASE_URL` are **not** read. A key exported for some other tool should
not quietly start billing per Claude Code turn.

Example `~/.config/dw/config.json`:

```json
{
  "base_url": "https://openrouter.ai/api/v1",
  "model": "<a cheap model id>",
  "api_key": "sk-…"
}
```

### 5.3 Backend selection (`backend: auto`)

```text
model != "" && (api_key != "" || base_url explicitly set)  → openai
else claude_command found on PATH                           → claude
else                                                         → off
```

"Explicitly set" means `base_url` came from the env or the file, not the
default. That lets a keyless local server work (Ollama, llama.cpp:
`DW_LLM_BASE_URL=http://localhost:11434/v1`, `DW_LLM_MODEL=…`). It also keeps
a half-configured "model but no key" setup from hitting `api.openai.com` and
getting 401 on every turn. An explicit `backend: openai` with missing pieces is
a config error: it is logged, and nothing falls back silently.

`dw help` gains one line after `Root:`:

```text
Summarizer: openai-compatible (model=… at https://…/v1)   |   claude -p (model=haiku)   |   off (no model configured and `claude` not on PATH)
```

### 5.4 Request

```text
POST {base_url}/chat/completions
{"model": "…", "messages": [{"role":"system","content":"<prompt.md>"},{"role":"user","content":"<state + slice>"}]}
```

The request sends no `temperature` or `max_tokens`. Several reasoning-model
endpoints reject one or the other, and the output size is already bounded by
the validation step. The reply is read from `choices[0].message.content`. The
response body is capped at 1 MiB via `io.LimitReader`, and the client is an
`http.Client` with no global timeout because the context drives cancellation.

## 6. `claude -p` fallback

```text
claude -p \
  --model <claude_model> \
  --system-prompt <prompt.md> \
  --tools "" \
  --setting-sources "" \
  --no-session-persistence \
  --output-format text
```

The user message goes on stdin and the reply comes back on stdout. The flags
were checked against the installed `claude --help`.

- **cwd = `os.TempDir()`, never the workspace or the root.** Otherwise Claude
  Code would load `<root>/CLAUDE.md` and the workspace's `.claude/settings.json`.
  The first contains instructions to edit files, and the second would fire this
  same `Stop` hook again.
- **`DW_SUMMARIZER=1` in the child env.** This is the real recursion guard.
  User-level hooks (`~/.claude/settings.json`) may still run, and if one of them
  were `dw hook`, it exits immediately.
- `--tools ""` turns the call into pure text-in/text-out. It cannot touch files.
- `--setting-sources ""` skips user, project and local settings, so a user
  default model or hooks cannot leak in. Whether an empty value means "none" is
  unverified (§10, Q4). If not, pass `--settings '{}'` plus the env guard.
- `--no-session-persistence` keeps every turn's summary from showing up as a
  session in `claude --resume`.
- **Not `--bare`.** It skips hooks, which would be nice, but it forces
  `ANTHROPIC_API_KEY` auth and never reads OAuth. A user with only a Claude
  subscription is exactly who the fallback exists for.
- The process is killed through `exec.CommandContext` on timeout, and stdout is
  capped at 1 MiB.

## 7. Removed and changed commands and docs

The CLI was already cut back in `3d6130b`. This rework removes the remaining
"the conversation edits state" surface and adds plumbing.

| Item | Change |
| --- | --- |
| `dw`, `dw <topic>` | Unchanged. A single match also runs `EnsureScaffold` and the failure-status warning (§4.8) |
| `dw init`, `dw version` | Unchanged |
| `dw help` | Adds the `Summarizer:` line. The usage block lists `hook` and `summarize` under "Plumbing" |
| **new** `dw hook` | Claude Code `Stop` hook entry point (§4.2). Not meant to be typed |
| **new** `dw summarize <ws> [--session ID --transcript PATH]` | The worker, run in the foreground with logs to stderr. With no session args, it summarises the newest transcript under `~/.claude/projects/<encoded ws>/`. That gives the user a manual "summarise now" and a debugging entry point. Also used by the integration tests |
| Reserved words | `hook` and `summarize` become reserved, like `init` and `help` today, so `dw hook` cannot create a topic named "hook". `run()` in `main.go` and the `case` list in `shellInit`'s `dw()` must both list them. Without the wrapper change, `dw hook` typed in a shell would open fzf |
| `assets/CLAUDE.md` | Session LLM no longer writes `STATE.md` (§4.7). "閉じるとき" step 2 ("更新漏れを見直す") becomes "check the summary landed; correct it by hand if needed" |
| `docs/concepts.md` | Rewrite the State paragraph ("原則として人間が所有する…自動生成に委ねると…退化する") and the "閉じる" operation to describe the background writer and the human-as-editor role. See Q1 |
| `README.md` | Replace "Zero config" with "zero dependencies, optional config". Replace "never launches `claude` itself" with "launches `claude -p` in the background only as the summarizer fallback". Replace the "Configuration" section with §5. Also fix the stale "Claude proposes edits as a diff" line, which `fcb278c` already made wrong |
| `.goreleaser.yaml` | Drop `windows` |
| "Last visited" pin (`last.go`, `PinLast`) | Kept. It is navigation, not history |

Removed and not coming back: `dw scaffold`, `dw config`, `.dw/last-session.md`,
and any session or log carry-over.

## 8. Testing strategy

Everything runs under the existing `go test -race ./...` in CI. Nothing touches
the network or a real `claude`.

- **Config (`config_test.go`)** — a table test over env × file × default for
  every key, using `t.Setenv` and a temp `DW_CONFIG`. It also covers:
  - The backend-selection matrix in §5.3, including "model without key and
    default URL → claude".
  - Unknown keys → error.
  - An empty env value means unset.
  - The world-readable `api_key` warning.
- **OpenAI backend** — against `httptest.Server`:
  - Path is `/chat/completions` (with and without a trailing slash on the base
    URL).
  - The `Authorization` header is present only when a key is set.
  - Body shape.
  - 429 → retried, 400 → not retried.
  - Context timeout.
  - Oversized response is capped.
- **`claude` backend** — a stub `claude` script on `PATH`, using the same
  technique as `stubFzf` in `shellinit_test.go`. The stub records argv, stdin,
  `pwd` and `DW_SUMMARIZER`. Tests assert the exact flags, that cwd is not under
  `DW_ROOT`, the env guard, and the non-zero-exit and timeout paths.
- **Transcript (`transcript_test.go`)** — sanitised JSONL fixtures in
  `internal/summarize/testdata/`: text-only turn, tool use with a large result,
  thinking blocks, malformed lines, a truncated last line (writer mid-flush).
  Tests check rendering, the tail cap, and offset reset on shrink.
- **Worker (`worker_test.go`)** — a fake `Summarizer`. Tests cover:
  - written / unchanged / nothing-new.
  - Every validation rejection (lost `## 未決の問い`, renamed H1, shrink
    guard, code fence stripped).
  - CAS: the fake modifies `STATE.md` mid-call → retry, then `conflict`.
  - The offset advances only on success.
  - Two `Run`s in goroutines on one workspace → serialised, the second reports
    `nothing-new`.
  - No `.STATE.md.tmp-*` left behind after a forced write error.
- **Workspace package** — `WriteStateAtomic` and `EnsureScaffold` with
  `writeIfAbsent` semantics (an existing `settings.json` is left untouched).
  `IsWorkspace` covers root, nested, dot-dir and outside-root cases.
- **`main_test.go`** — `run` gains stdin. Tests cover:
  - `dw hook` with a valid payload → `spawnWorker` called once with the right
    args.
  - Garbage payload, non-workspace dir, `DW_SUMMARIZER=1`, backend off → exit
    0, nothing spawned, empty stdout.
  - `dw help` shows each `Summarizer:` variant.
  - Failure warning shown at 3 consecutive failures.
- **Detachment integration (`//go:build unix`)** — build the real binary as in
  `buildEnv`, point `DW_CLAUDE_COMMAND` at a stub that sleeps 2 s and echoes a
  valid `STATE.md`, and pipe a payload into `dw hook`. Assertions:
  - The hook returns in under 500 ms even with its stdout attached to a pipe
    the test keeps reading until EOF. That proves no inherited pipe.
  - `STATE.md` is updated within a 10 s polling deadline.
  - The log has `result=written`.
- **Shell wrapper** — extend `TestShellInit_ParsesInBothShells` and the pick
  tests so that `dw hook` and `dw summarize` pass straight through to
  `command dw` without calling fzf.
- **Manual acceptance** — run a real session in a scratch `DW_ROOT` once with
  each backend: a cheap OpenAI-compatible model, `claude -p` haiku, and off.
  Check that turns never feel slower and that `STATE.md` changes after a turn
  that settles a question.

## 9. Implementation order

Each step is a separate PR that stays green on its own.

1. `workspace.WriteStateAtomic`, `IsWorkspace`, `EnsureScaffold` and the
   settings asset, with tests.
2. `internal/summarize`: config, the two backends, transcript reader, worker,
   with a fake summarizer. Plus `dw summarize` in the foreground.
3. `dw hook`, detachment, status warning, and the wrapper passthrough.
4. Docs: `assets/CLAUDE.md`, `concepts.md`, `README.md`, and the goreleaser
   change. The `feat!:` commit that triggers the major release.

## 10. Risks and open questions

**Risks**

- **R1: Concept regression.** `concepts.md` argues that letting automation
  write State turns it into a conversation summary. A cheap model rewriting the
  file every turn is exactly that risk. Mitigations:
  - The prompt forbids narration and asks for unchanged output when nothing
    durable happened.
  - The human remains the editor, and CAS protects their edits.
  - The "考えが変わったか" check stays in the closing procedure.

  Still, the rework changes the tool's philosophy, not just its plumbing.
- **R2: Quality drift and silent data loss.** A full rewrite on every turn can
  erode content. The H1, heading and shrink checks catch gross failures, not
  subtle ones. Git-tracking the root (users already may; `Scan` skips `.git`)
  is the cheap safety net. Should dw recommend it?
- **R3: Undocumented transcript format.** `transcript_path` JSONL is Claude
  Code internals. Mitigation: a defensive parser, fixtures, and skip-and-count
  on unknown lines. If parsing breaks entirely, there is a degraded mode that
  summarises from the `Stop` payload alone (whatever final-message field the
  payload carries; `2dc5b39` read `last_assistant_message`).
- **R4: Privacy.** Transcripts include tool output, so anything a session read
  (`.env`, logs with tokens) gets sent to the configured endpoint every turn.
  Mitigations:
  - Tool results are truncated.
  - The `OPENAI_*` env vars are never read implicitly.
  - Per-workspace opt-out by removing the hook.
  - Documented prominently.
- **R5: Cost and quota.** One call per turn. Under the `claude -p` fallback it
  draws from the user's Claude plan in the background, which is invisible
  spend. Coalescing (§4.4) and the 64 KiB cap bound it. The cost is shown in
  `dw help` only indirectly, through the backend line.
- **R6: Staleness.** If the user quits and immediately re-opens the topic, the
  new session may read `STATE.md` before the last summary lands. That is
  seconds at most. Accepted and documented.
- **R7: Hook missing.** A workspace with a pre-existing `.claude/settings.json`
  gets no hook, so summaries silently never happen there.

**Open questions**

1. **Q1:** Is moving `STATE.md` ownership to a background LLM an intended
   change to `docs/concepts.md`, or should the worker write a *proposal*
   (`STATE.proposed.md`) that the human promotes? The proposal variant is safer
   but brings back the manual step that `fcb278c` just removed. This doc
   assumes direct writes.
2. **Q2:** The request says `state.md`, but the codebase uses `STATE.md`
   (`stateFileName`). This doc keeps `STATE.md`. Renaming would also touch the
   fzf `--preview` in `shellInit` and every existing workspace.
3. **Q3:** Should `STATE.md` gain a bounded `## 直近の作業` section (last N
   actions, one line each)? That would satisfy "input/output and results" more
   literally, but contradicts "書かないこと: 会話の経過". The default here is no.
4. **Q4:** Does `--setting-sources ""` mean "load nothing" on the installed
   `claude` version? Verify before step 2. Otherwise use `--settings '{}'`.
5. **Q5:** For workspaces that already have `.claude/settings.json`, should
   `EnsureScaffold` merge the hook in (JSON read-modify-write)? That breaks the
   "`writeIfAbsent` everywhere" rule in `workspace.go`. The alternative is to
   print a one-time hint.
6. **Q6:** Are non-Claude actions (shell commands in a workspace) in scope
   later? If so, the `dw init` wrapper would need a `precmd` hook feeding the
   same worker.
7. **Q7:** Should `DW_ROOT` also be settable in `config.json` now that a config
   file exists? Keeping it env-only preserves today's single documented knob
   for the core.
8. **Q8:** Is dropping the Windows release acceptable, or is a Windows build
   (picker only, summaries off) worth keeping?
9. **Q9:** Fallback model: the `haiku` alias, or a pinned ID for
   reproducibility? The alias tracks the cheapest current tier automatically,
   so it is the default here.
