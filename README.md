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
- **Resume by default** — your last-visited workspace is pinned to the top of the listing, so `dw` with no arguments and `enter` gets you back to where you left off.
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

1. Runs `command dw "$@"` and captures the TSV rows.
2. If there's more than one row, hands them to `fzf` (`--with-nth=2..` hides the path column, the preview shows `STATE.md`) — or, without `fzf` on `PATH`, just prints the rows and stops.
3. Re-resolves the chosen absolute path through `command dw` (so it becomes the new "last visited" workspace) and `cd`'s into it.

`fzf` is optional but the point of the tool without it: install it (`brew install fzf`) to get the picker experience; without it `dw` still works from the command line, just without disambiguation.

### Quickstart

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

Everything goes to **stdout**; diagnostics go to stderr. `dw` never asks for confirmation and never launches `claude` itself — it only ever prints paths.

| Command | Description |
|---|---|
| `dw` | List every workspace as TSV, most recently visited pinned first. |
| `dw <topic>` | Resolve `<topic>`: exact match, else partial match(es), else create it. |
| `dw init <zsh\|bash>` | Print the shell function that wires `dw` into `fzf` and `cd`. |
| `dw version` | Print the version. |
| `dw help` | Show usage, including the resolved `DW_ROOT`. |

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

Claude Code loads `CLAUDE.md` files by walking up the directory tree from the working directory, so running `claude` inside `<root>/<topic>/` loads `<root>/CLAUDE.md` in full at launch. What isn't documented is whether that ancestor file is *re-injected* after `/compact` the way the project-root `CLAUDE.md` is — so on a very long session, the convention may fade before `STATE.md`'s content does. If you notice Claude drifting from the convention late in a long session, that's the likely cause; re-reading `<root>/CLAUDE.md` explicitly is the workaround.

## Configuration

`DW_ROOT` is the only knob — there is no config file. Defaults to `~/dw`:

```sh
export DW_ROOT=~/discussions
```

Every path `dw` prints is absolute — `Root()` normalizes `DW_ROOT` against the current directory if it isn't already absolute. Still, set it to something that's absolute up front (`~/discussions`, which your shell expands before `dw` ever sees it, or `$HOME/discussions`), not a bare relative path like `discussions`: since the wrapper `cd`s you into a resolved workspace, running `dw` again from inside one would re-resolve a relative `DW_ROOT` under that workspace instead of your intended root.

## Contributing

```sh
make fmt    # gofumpt + goimports (golangci-lint fmt)
make lint   # golangci-lint run
make test   # go test -race ./...
make        # all of the above
```

Building from source needs the Go version pinned in `go.mod` (currently 1.26). The binary itself has no runtime dependencies — no cgo, no external files (the shell wrapper is embedded as a Go string, see below) — so cross-compilation just works, which is how [Releases](https://github.com/edge2992/dw/releases/latest) ships linux/macOS/windows × amd64/arm64 from one `.goreleaser.yaml` config.

Two portability notes if you're changing shell-facing behavior:

- The `dw init` wrapper targets POSIX-compatible `zsh`/`bash` only (`dw init fish` or anything else is a usage error); Windows users get a binary but no wrapper.
- The "last visited" cache path comes from `os.UserCacheDir()`, which differs by OS (`~/.cache/dw/last` on Linux, `~/Library/Caches/dw/last` on macOS) — never hardcode it.

Other project mechanics:

- **Lint/Format**: golangci-lint v2 (config `.golangci.yml`, standard set + misspell/revive; formatters gofumpt/goimports).
- **Hooks**: pre-commit framework (`.pre-commit-config.yaml`). A global pre-commit hook delegates here after gitleaks, so `pre-commit install` is not required. Setup: `uv tool install pre-commit`, `brew install golangci-lint`.
- **CI**: GitHub Actions (`.github/workflows/ci.yml`) runs build / test -race / golangci-lint.
- **Release**: fully automated. [Release Please](https://github.com/googleapis/release-please) parses [Conventional Commits](https://www.conventionalcommits.org/) on every push to `main` to maintain a release PR; merging it tags the release, and [GoReleaser](https://goreleaser.com/) attaches the binaries. `feat` bumps the minor, `fix` the patch, `feat!`/`fix!` (or a `BREAKING CHANGE:` footer) bumps the major — so a commit's type is also a release decision, not just a label.

There's no separate `CONTRIBUTING.md`; open a PR or issue against this repo.

## Reading the source

`dw` is small on purpose (see [Why](#why)) — the whole implementation is `main.go` plus `internal/workspace/`. A few things that aren't obvious from the file names alone:

- **`internal/workspace/`** holds all of the logic: `workspace.go` (`Scan`/`Resolve`/`Create`, the topic-resolution algorithm), `state.go` (`STATE.md` templating and parsing), `convention.go` (writes the root `CLAUDE.md` once), and `last.go` (the "last visited" cache). `main.go` is just argv dispatch and output formatting on top of that package.
- **Two files are both named `CLAUDE.md` and mean different things.** `/CLAUDE.md` (repo root) is this repository's *own* development guidance — instructions for working on dw's Go source. `internal/workspace/assets/CLAUDE.md` is a template, `//go:embed`ded into the binary, that `dw` writes once into every `<DW_ROOT>/CLAUDE.md` it creates (the Convention layer described above). They're unrelated; don't edit one expecting it to affect the other.
- **`main.go`'s `shellInit` constant** *is* the `dw init zsh`/`dw init bash` output — the wrapper script lives inline as a Go string rather than a separate `.sh` file, which is also why the binary has no runtime file dependencies (see Contributing).
- **`.golangci.yml`, `.pre-commit-config.yaml`, `.goreleaser.yaml`, `release-please-config.json`** are tooling configuration, not part of the `dw` package — see Contributing for what each one does.
- **`docs/concepts.md`** is the design document this tool implements; read it if `STATE.md`'s shape (前提 / 却下した案 / 未決の問い) or the four-layer split needs more context than this README gives.

## License

[MIT](LICENSE) © edge2992
