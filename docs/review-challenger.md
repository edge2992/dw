# Challenger review: DESIGN-rework.md (rework/async-state)

Ranked by impact on whether this creates real value. Each item has a concrete fix.

## 1. Q1: per-turn writes contradict the concept, not only its wording (critical)
- concepts.md says more than "humans own State". It says 議論する means "State には触らない", and that writing State is itself the thinking. Rewriting State after every turn moves half-formed ideas, Working-area experiments and a pass/fail from one test into State mid-discussion. That is the "会話の要約に退化" failure mode, sped up.
- The doc treats the per-turn trigger as given ("user action = one turn"). That is where the conflict comes from. The requirement also says "results", and a result usually exists only at the end of a discussion.
- **Fix:** change the trigger from turn to session boundary. Summarise the whole session once, either detached from `SessionEnd` (the ~1.5 s budget applies to the hook, and a Setsid-detached worker outlives it, which undercuts the §4.1 reasoning) or lazily at the next `dw <topic>` or SessionStart. Write the result as a diff proposal next to State (cache dir, or `STATE.proposed.md`) and apply it on `dw <topic>` with a one-key accept. That keeps "確定は手でやる" with almost no manual cost. If direct writes stay, at least gate them to session end.

## 2. Repeated full rewrites cause compounding drift (high)
- Rewriting the whole file N times per session is "要約の要約", which "Why this works" itself names as the thing that degrades. The H1, heading and 40% shrink guards miss slow erosion of exactly what matters most: rejected ideas and their conditions, dates, and the split between facts and judgements.
- **Fix:** (a) fewer rewrites (item 1); (b) keep each prior version in the cache dir (`~/.cache/dw/history/<hash>/<ts>.md`, last 20) so `dw` can show or revert; (c) add a validation that every existing rejection or `## 未決の問い` line either survives or is explicitly marked resolved, instead of relying only on size.

## 3. R4 privacy: default-on exfiltration of tool output (high)
- Every turn sends tool results (`cat .env`, logs, customer data) to whatever endpoint is set. Truncating to 1000 bytes does not stop a secret from leaking. A default `base_url` of api.openai.com makes a third-party destination the easy path.
- Tool output is also the least State-worthy input. Sending it costs privacy and quality at the same time.
- **Fix:** send USER and ASSISTANT text only by default (opt-in `include_tool_results`). Run a regex redaction pass for obvious keys and tokens (`sk-`, `AKIA`, `ghp_`, `-----BEGIN`, `KEY=`). Make the `openai` backend require an explicit `base_url` with no default. Print the destination in `dw help` and on the first summarisation for each workspace.

## 4. Hidden cost on the claude -p fallback (medium-high)
- The user is on a Claude subscription. One background Haiku call per turn draws from the same rate-limit pool as the interactive session, so it can throttle the work it is meant to support, invisibly. Coalescing helps only with bursts, not normal pacing.
- **Fix:** item 1 cuts calls 10-50x. Otherwise add `min_interval` (for example 10 min per workspace) and record call counts in the log so `dw help` can show "N summaries today".

## 5. Over-engineering for a single user (medium)
- flock + CAS + per-session offsets + status JSON + log rotation + config precedence + two backends + recursion guard + EnsureScaffold backfill. Most of the concurrency machinery exists only because of the per-turn trigger.
- **Fix:** with a session-end trigger and proposals, drop offsets (read the whole session tail-capped), drop the 30 s lock polling (use a non-blocking lock and skip), and drop the status file (put warnings in the proposal step). Keep CAS and atomic write. Ship the claude backend plus the minimal OpenAI-compatible HTTP backend, and nothing else.

## 6. Hook and detach details (medium)
- `"command": "dw hook"` depends on the hook's PATH (GUI-launched or IDE Claude Code may not have `~/go/bin`). **Fix:** scaffold the absolute path from `os.Executable()` and refresh it in `EnsureScaffold` if it is stale.
- A project `.claude/settings.json` hook may need a trust or approval prompt in Claude Code, and a file the user never wrote appears in every workspace. **Fix:** verify the approval UX manually. Consider one user-level hook (`~/.claude/settings.json`) gated by `IsWorkspace`. That also fixes R7 and Q5 (no merging per workspace).
- `$CLAUDE_PROJECT_DIR` is wrong if claude was started in a subdirectory of the workspace. **Fix:** walk up to the nearest direct child of `Root()`.
- R6 understates staleness. The worst case is 30 s lock wait plus a 120 s timeout. **Fix:** have `dw <topic>` wait briefly (≤5 s) on the workspace lock, or print "summary pending".
- Verify Q4 (`--setting-sources ""`) and the recursion guard before step 2. If user-level hooks still fire in `claude -p`, every summary spawns a nested hook.

## 7. Session-LLM no-write rule weakens explicit recording (medium)
- "Say it in the conversation so the summarizer sees it" turns a deterministic edit into a cheap model's guess. **Fix:** let the session write State when the user explicitly asks, and rely on CAS plus proposals to reconcile. Or add `dw note "<line>"` that appends to proposals verbatim.

## 8. Unchallenged alternatives worth one line in the doc
- Do nothing automatic, and make the 閉じる step one command (`dw close` = summarise the session to a diff and accept). That meets "async, cheap, configurable" at the lowest risk. The doc should say why it lost.
- Q3 (`## 直近の作業`): keep it out of STATE.md. If "input/output" must be captured literally, write it to a Working-area log that is not carried over.
