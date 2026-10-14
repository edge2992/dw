# Design review (explorer): DESIGN-rework.md

Scope: `docs/DESIGN-rework.md` on `rework/async-state`, checked against `main.go` and `internal/workspace/*`. No code changed.
Verdict: the core shape is sound: hook → detached worker → one cheap LLM call → CAS write. Most alternatives below are worse. Two of them are worth adding as cheap complements: a debounce and a batch catch-up.

## Trigger alternatives
| Option | Pros | Cons |
| --- | --- | --- |
| **Stop hook (doc)** | Once per turn, after output exists; payload gives `transcript_path` | Per-workspace scaffold misses workspaces that already have a `settings.json` (R7); one call per turn |
| PostToolUse | Fine-grained | Fires many times per turn, so it costs more; output is not final yet. Reject |
| SessionEnd | One call per session, so the cheapest | The 1.5 s budget is enough to *spawn* a detached worker (the 2dc5b39 lesson was about synchronous work). But a crashed or killed session never fires it, and `STATE.md` stays stale for the whole session |
| Transcript watcher daemon | No hook scaffolding | A long-lived process; the stdlib has no fsnotify, so it has to poll; lifecycle and install burden. Reject |
| git hooks | Simple | Workspaces aren't committed per turn, so this captures nothing useful. Reject |
| Wrapper (`dw` runs `claude`, then summarizes on exit) | No Claude Code config at all | Only works when sessions are launched through dw; per-session granularity. The doc also says the CLI won't launch `claude` interactively |
| cron / `dw summarize --all` batch | Trivial to implement, robust, catches missed turns | Minutes of staleness; needs a session-to-workspace mapping via `~/.claude/projects/<encoded ws>/` |
| Global user hook (`~/.claude/settings.json`) + `IsWorkspace` filter | Fixes R7; no per-workspace files; no merge question (Q5) | Edits the user's global config once (a manual, documented step) |

Recommendation: keep `Stop`, and add `dw summarize --all` (already half-specified as the manual entry point) as an optional cron/launchd catch-up. Consider documenting the global-hook variant as the fix for R7/Q5 instead of JSON merging.

## Proposal file vs direct write
- Direct write + flock + CAS + same-dir temp + rename is correct and avoids the manual promotion step that `fcb278c` removed. Small gap: CAS hashes and then renames, so there is a TOCTOU window of a few ms in which an editor save can be lost. That's acceptable, but document it.
- A `STATE.proposed.md` is safer for R1/R2 (philosophy drift, silent erosion) but brings back friction, and the file itself would be context the next session reads.
- Middle ground (recommended): write directly, but keep the last N prior versions in `~/.cache/dw/history/<hash(ws)>/` (outside the workspace, respecting Isolation) plus `dw summarize --undo` or a documented `cp`. That costs much less than a proposal flow and closes R2 even without git.

## Cheap model / endpoint
- Making an OpenAI-compatible `/chat/completions` the single abstraction is the right call. It covers OpenRouter (any cheap model), Gemini Flash (Google's OpenAI-compat endpoint), Anthropic Haiku (Anthropic's OpenAI-compat endpoint), and local Ollama / llama.cpp / vLLM. No per-vendor code needed.
- Local models: the shrink guard and heading checks matter more there; small models often wrap output in prose. Consider asking for the output between sentinel markers and not only stripping fences.
- No default HTTP model plus not reading `OPENAI_*` is good. It prevents silent billing.

## Capturing I/O
- The transcript JSONL plus a byte offset is the richest source, and the offset-based coalescing is elegant. The format is undocumented (R3); keep the `last_assistant_message` degraded mode.
- `script`/`tee` and shell history only see the terminal or shell, not tool calls. They fit Q6 (non-Claude actions) later, not now.

## Risks to add or sharpen
- **Recursion:** the `DW_SUMMARIZER=1` env guard and cwd=`$TMPDIR` are the real protection; `--setting-sources ""` is unverified (Q4). Also ignore payloads with `stop_hook_active=true`. Add an integration test where a stub `claude` itself invokes `dw hook`.
- **Cost:** one call per turn re-sends the full `STATE.md`. Add a short debounce (the worker sleeps ~15–30 s before taking the lock) so a burst of quick turns becomes one call. The offset design already makes this free. Under the `claude -p` fallback this is invisible plan usage, so log a per-day call count and show it in `dw help`.
- **Races:** two sessions in one workspace are serialised by flock. Fine. A session reading `STATE.md` mid-rename always sees the old or new file. Fine.
- **Secrets:** truncation is not redaction. Add a cheap regex scrub (`sk-…`, `AKIA…`, `ghp_…`, `-----BEGIN … KEY`, `KEY=value` lines from `.env`) before sending, and default to *dropping* `Read` results for files matching `.env*`/`*secret*`.
- **Windows:** dropping it just for flock/Setsid is heavy. `O_EXCL` lockfiles plus `CREATE_NEW_PROCESS_GROUP` are stdlib-reachable. Low priority, but Q8 has a cheap answer.

## Recommendation
Approve the design with its core intact: per-turn **Stop hook → detached `dw summarize` → OpenAI-compatible endpoint (fallback `claude -p --model haiku`, env-guarded, cwd=$TMPDIR) → direct CAS+atomic-rename write to `STATE.md`**, with the CLI limited to creating and opening workspaces plus the `hook`/`summarize` plumbing. Before step 2, make four changes:
(1) a debounce before taking the lock, to cut cost; (2) rolling backups in the cache dir instead of a proposal file, to cover R2; (3) secret redaction on transcript slices; (4) verify Q4, and add a recursion test.
Optional: a `dw summarize --all` cron catch-up, and a global-hook install path that removes R7/Q5. Reject PostToolUse, watcher daemons, git hooks and shell capture for this iteration.
