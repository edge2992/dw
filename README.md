# dw — discussion workspace

[![release](https://img.shields.io/github/v/release/edge2992/dw)](https://github.com/edge2992/dw/releases/latest)
[![license](https://img.shields.io/github/license/edge2992/dw)](LICENSE)

**Give every topic you discuss with Claude its own directory, and carry the state of that discussion — not the conversation log — from one session to the next.**

See [docs/concepts.md](docs/concepts.md) for the idea in full; this README covers the tool.

```text
$ dw datadog-cost
/Users/me/dw/2026-08-08-datadog-cost	* Datadog コスト削減	Flex Logs の料金は営業確認待ち
$ cd /Users/me/dw/2026-08-08-datadog-cost && claude
```

## Features

- **One command, one directory** — `dw <topic>` resolves to `<root>/<YYYY-MM-DD>-<topic>/`: exact match, else partial match(es), else it creates one. No separate create command.
- **State, not conversation log** — a new workspace gets one file, `STATE.md`. That's what carries forward across sessions; nothing else does.
- **A single Convention layer** — the workspace root gets exactly one `CLAUDE.md`, written once and never touched again by `dw`. It tells Claude how to treat `STATE.md`, not what any particular topic is about.
- **Ambiguity goes to fzf, not to dw** — when a topic matches more than one workspace, `dw` prints every match; it never guesses which one you meant. The shell wrapper hands the rows to `fzf` (falls back to a plain list without it).
- **Resume by default** — your last-visited workspace is pinned to the top of the listing and marked, so `dw` with no arguments and `enter` gets you back to where you left off.
- **Zero config** — one environment variable, `DW_ROOT` (default `~/dw`). No YAML, no per-category templates.
- **Zero dependencies** — the standard library only. `go build ./...` needs nothing from `go.sum`.
- **Unicode-safe slugs** — Japanese and other scripts survive slugification (`機械学習 調査` → `機械学習-調査`).

## Install

```sh
go install github.com/edge2992/dw@latest
```

Don't use Go? Grab a prebuilt binary for your OS/arch from the
[Releases](https://github.com/edge2992/dw/releases/latest) page (linux / macOS /
windows × amd64 / arm64, with `checksums.txt`). Check the installed version with
`dw version`.

## Shell integration

`dw` is a child process, so it can't change your shell's working directory itself, and it doesn't know about `fzf`. Both jobs — disambiguating multiple matches and `cd`ing — belong to a thin shell wrapper that `dw init` prints for you:

```zsh
eval "$(dw init zsh)"   # or: dw init bash
```

That wrapper:

1. Runs `command dw "$@"` and captures the TSV rows.
2. If there's more than one row, hands them to `fzf` (`--with-nth=2..` hides the path column, the preview shows `STATE.md`) — or, without `fzf` on `PATH`, just prints the rows and stops.
3. Re-resolves the chosen absolute path through `command dw` (so it becomes the new "last visited" workspace) and `cd`'s into it.

`fzf` is optional but the point of the tool without it: install it (`brew install fzf`) to get the picker experience; without it `dw` still works from the command line, just without disambiguation.

## Quickstart

```sh
dw datadog-cost        # exact/partial match jumps in; no match creates it
dw                      # list every workspace, most recently visited pinned first
```

Both forms print the same tab-separated contract to stdout:

```
<absolute path>\t<marker + title>\t<first open question>
```

- Column 1: absolute path (hidden by the fzf wrapper, useful for scripting: `cut -f1`).
- Column 2: `* ` prefix for the pinned/last-visited workspace, `  ` otherwise, then the title (`STATE.md`'s first `# ` heading, or the topic slug if there's no `STATE.md` yet).
- Column 3: the first line under `## 未決の問い` (open questions) in `STATE.md` — empty if there isn't one.

## Commands

| Command | Description |
|---|---|
| `dw` | List every workspace as TSV, most recently visited pinned first. |
| `dw <topic>` | Resolve `<topic>`: exact match, else partial match(es), else create it. |
| `dw init <zsh\|bash>` | Print the shell function that wires `dw` into `fzf` and `cd`. |
| `dw version` | Print the version. |
| `dw help` | Show usage, including the resolved `DW_ROOT`. |

Everything goes to **stdout**; diagnostics go to stderr. `dw` never asks for confirmation and never launches `claude` itself — it only ever prints paths.

## Layout

```text
<root>/CLAUDE.md                     # Convention — written once, yours from then on
<root>/<YYYY-MM-DD>-<topic-slug>/
  STATE.md                           # State — what's known about this topic
  sources/                           # Sources — collected material (create as needed)
  work/                              # Working area — disposable, not carried forward (create as needed)
```

Four layers, described fully in [docs/concepts.md](docs/concepts.md):

- **Sources** — material collected for the topic. Read-only; Claude never rewrites it.
- **State** (`STATE.md`) — what's known: premises, rejected options, open questions. The only layer that survives across sessions. Owned by the human — Claude proposes edits as a diff, never writes it directly.
- **Working area** (`work/`) — generated code, drafts, session logs. Disposable; never carried forward.
- **Convention** (`<root>/CLAUDE.md`) — how to treat the other three layers. One file, shared by every topic; `dw` writes it once when the first workspace is created and never touches it again.

`sources/` and `work/` aren't scaffolded — `dw` only ever writes `STATE.md`. Create them by hand once a topic actually needs them.

## `DW_ROOT`

The only configuration knob. Defaults to `~/dw`; set it to relocate the workspace root:

```sh
export DW_ROOT=~/discussions
```

## A note on `CLAUDE.md` and long sessions

Claude Code loads `CLAUDE.md` files by walking up the directory tree from the working directory, so running `claude` inside `<root>/<topic>/` loads `<root>/CLAUDE.md` in full at launch. What isn't documented is whether that ancestor file is *re-injected* after `/compact` the way the project-root `CLAUDE.md` is — so on a very long session, the convention may fade before `STATE.md`'s content does. If you notice Claude drifting from the convention late in a long session, that's the likely cause; re-reading `<root>/CLAUDE.md` explicitly is the workaround.

## Development

```sh
make fmt    # gofumpt + goimports (golangci-lint fmt)
make lint   # golangci-lint run
make test   # go test -race ./...
make        # all of the above
```

- **Lint/Format**: golangci-lint v2 (config `.golangci.yml`, standard set + misspell/revive; formatters gofumpt/goimports).
- **Hooks**: pre-commit framework (`.pre-commit-config.yaml`). A global pre-commit hook delegates here after gitleaks, so `pre-commit install` is not required. Setup: `uv tool install pre-commit`, `brew install golangci-lint`.
- **CI**: GitHub Actions (`.github/workflows/ci.yml`) runs build / test -race / golangci-lint.

## Release

Versioning is automated. [Release Please](https://github.com/googleapis/release-please)
parses [Conventional Commits](https://www.conventionalcommits.org/) to decide the next
version: every push to `main` updates a **release PR** (with CHANGELOG), and merging it
creates the semver tag and GitHub Release. [GoReleaser](https://goreleaser.com/) then
attaches prebuilt binaries for each OS/arch (`.github/workflows/release.yml`). `feat`
bumps the minor, `fix` the patch, `feat!`/`fix!` (or a `BREAKING CHANGE:` footer) bumps
the major.

## License

[MIT](LICENSE) © edge2992
