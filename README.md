# dw — discussion workspace

[![release](https://img.shields.io/github/v/release/edge2992/dw)](https://github.com/edge2992/dw/releases/latest)
[![license](https://img.shields.io/github/license/edge2992/dw)](LICENSE)

**Give every topic you discuss with Claude its own directory, and carry the state of that discussion — not the conversation log — from one session to the next.**

```text
$ dw datadog-cost
/Users/me/dw/2026-08-08-datadog-cost	* Datadog コスト削減	Flex Logs の料金は営業確認待ち
$ cd /Users/me/dw/2026-08-08-datadog-cost && claude
```

## Why

A conversation ends and the log is gone the next time you start one — Claude has no memory of what you decided, what you ruled out, or what's still open. Summarizing the log doesn't fix this either: a summary degrades every time it's re-summarized, and it carries the previous session's framing along with it.

`dw` takes a different approach: instead of carrying the conversation forward, it carries a single document — `STATE.md` — that records what's actually known about a topic (premises, rejected options, open questions), written by you and updated by you. Nothing else about the session persists, and nothing needs to. See [docs/concepts.md](docs/concepts.md) for the reasoning in full; this README covers the tool.

- **One command, one directory** — `dw <topic>` resolves to `<root>/<YYYY-MM-DD>-<topic>/`: exact match, else partial match(es), else it creates one. No separate create command.
- **State, not conversation log** — a new workspace gets one file, `STATE.md`. That's what carries forward across sessions.
- **A single Convention layer** — the workspace root gets exactly one `CLAUDE.md`, written once and never touched again by `dw`. It tells Claude how to treat `STATE.md`, not what any particular topic is about.
- **Ambiguity goes to `fzf`, not to `dw`** — when a topic matches more than one workspace, `dw` prints every match; it never guesses which one you meant.
- **One command, no modes** — `dw` always opens the picker, however many workspaces you have. Filter to find one, or keep typing past the last match and `enter` creates that topic. There is nothing else to learn.
- **Resume by default** — your last-visited workspace is pinned to the top of the picker, so `dw` and `enter` gets you back to where you left off.
- **Zero config, zero dependencies** — one environment variable (`DW_ROOT`), no YAML, and the Go standard library only.

## Install

```sh
go install github.com/edge2992/dw@latest
```

Don't use Go? Grab a prebuilt binary for your OS/arch from the
[Releases](https://github.com/edge2992/dw/releases/latest) page (linux / macOS /
windows × amd64 / arm64, with `checksums.txt`). Check the installed version with
`dw version`.

## Usage

### Shell integration

`dw` is a child process, so it can't change your shell's working directory itself, and it doesn't know about `fzf`. Both jobs — disambiguating multiple matches and `cd`ing — belong to a thin shell wrapper that `dw init` prints for you:

```zsh
eval "$(dw init zsh)"   # or: dw init bash
```

That wrapper:

1. Runs `command dw` and captures the TSV rows.
2. Hands them to `fzf` — always, not only when there's more than one. `--with-nth=2..` hides the path column, the preview shows `STATE.md`, and `dw <topic>` arrives as `--query=<topic>` so the picker opens already filtered.
3. If you `enter` on a row, re-resolves its absolute path through `command dw` (so it becomes the new "last visited" workspace) and `cd`'s into it.
4. If nothing matched what you typed, hands the query to `command dw` instead — which finds it by slug or, failing that, creates it. Either way you end up in the workspace.

So the picker never dead-ends: `enter` on a match opens it, `enter` on no match creates it, `esc` does nothing.

`fzf` is optional but the point of the tool without it: install it (`brew install fzf`) to get the picker experience. Without it — or outside a terminal — the wrapper falls back to resolving your arguments directly and printing anything ambiguous.

### Quickstart

```sh
dw                      # open the picker: filter, or type a new topic and press enter
dw datadog-cost         # same picker, pre-filtered to "datadog-cost"
```

Both forms print the same tab-separated contract to stdout:

```
<absolute path>\t<marker + title>\t<first open question>\t<directory name>
```

Everything goes to **stdout**; diagnostics go to stderr. `dw` never asks for confirmation and never launches `claude` itself — it only ever prints paths.

| Command               | Description                                                                                                                                   |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `dw`                  | Open the picker over every workspace, most recently visited pinned first.                                                                     |
| `dw <topic>`          | Open the picker with `<topic>` pre-typed. Without shell integration, resolves `<topic>`: exact match, else partial match(es), else create it. |
| `dw init <zsh\|bash>` | Print the shell function that wires `dw` into `fzf` and `cd`.                                                                                 |
| `dw version`          | Print the version.                                                                                                                            |
| `dw help`             | Show usage, including the resolved `DW_ROOT`.                                                                                                 |

Run `dw` straight from a prompt without `eval "$(dw init zsh)"` and you'll get raw TSV plus a note on stderr telling you so — and on a fresh install with no workspaces, a short getting-started block instead of the silence earlier versions printed.

### What `dw` creates

```text
<root>/CLAUDE.md                     # Convention — written once, yours from then on
<root>/<YYYY-MM-DD>-<topic-slug>/
  STATE.md                           # State — what's known about this topic
  sources/                           # Sources — collected material (create as needed)
  work/                              # Working area — disposable, not carried forward (create as needed)
```

Four layers, described fully in [docs/concepts.md](docs/concepts.md): **Sources** (read-only material collected for the topic), **State** (`STATE.md`, the only layer that survives across sessions — Claude proposes edits as a diff, never writes it directly), **Working area** (`work/`, disposable generated output), and **Convention** (`<root>/CLAUDE.md`, shared by every topic). `sources/` and `work/` aren't scaffolded — create them by hand once a topic actually needs them.

### A note on `CLAUDE.md` and long sessions

Claude Code loads `CLAUDE.md` files by walking up the directory tree from the working directory, so running `claude` inside `<root>/<topic>/` loads `<root>/CLAUDE.md` in full at launch. What isn't documented is whether that ancestor file is _re-injected_ after `/compact` the way the project-root `CLAUDE.md` is — so on a very long session, the convention may fade before `STATE.md`'s content does. If you notice Claude drifting from the convention late in a long session, that's the likely cause; re-reading `<root>/CLAUDE.md` explicitly is the workaround.

## Configuration

`DW_ROOT` is the only knob — there is no config file. Defaults to `~/dw`:

```sh
export DW_ROOT=~/discussions
```

Every path `dw` prints is absolute — `Root()` normalizes `DW_ROOT` against the current directory if it isn't already absolute. Still, set it to something that's absolute up front (`~/discussions`, which your shell expands before `dw` ever sees it, or `$HOME/discussions`), not a bare relative path like `discussions`: since the wrapper `cd`s you into a resolved workspace, running `dw` again from inside one would re-resolve a relative `DW_ROOT` under that workspace instead of your intended root.

## License

[MIT](LICENSE) © edge2992
