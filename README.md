# dw — discussion workspace picker

[![release](https://img.shields.io/github/v/release/edge2992/dw)](https://github.com/edge2992/dw/releases/latest)
[![license](https://img.shields.io/github/license/edge2992/dw)](LICENSE)

**Spin up a dated workspace for every topic you explore with Claude — then fuzzy-jump back to any of them.**

`dw` gives each topic its own `<category>/<YYYY-MM-DD>-<topic>/` folder with a
frontmatter README. No naming ceremony: type a topic, pick a category, and start
working. Later, fuzzy-find it and `cd` straight in.

```text
$ dw
> k8s pod                                         research/2026-06-14-k8s-pod-oom
  db outage                                       incident/2026-06-01-db-outage
  + create: 2026-06-17-k8s-pod                    (pick a category)
    status: active  tags: [gpu, linux]  created: 2026-06-14
```

## Features

- **Dated auto-layout** — every topic gets its own `<category>/<YYYY-MM-DD>-<topic>/` folder with a frontmatter README, created for you.
- **Create on demand** — type the topic first, pick a category (or invent one) second. No `create` command, no naming ceremony.
- **Fuzzy jump & resume** — fuzzy-match across names and titles, newest first; your last workspace is pinned to the top, and `dw -` returns to it with no UI.
- **Frontmatter-aware** — shows `status` / `tags` / `created` from each README under the selection.
- **Scriptable primitives** — the TUI is sugar over plain commands: `dw new` creates, `dw list --json` streams, so you can wire your own flow (`dw list --json | fzf`).
- **Unicode-safe slugs** — Japanese and other scripts survive slugification (`機械学習 調査` → `機械学習-調査`).
- **Zero-config, YAML when you want it** — works out of the box at `~/dw`; customize root, templates, and categories in `~/.config/dw/config.yml` ([docs](docs/configuration.md)).

## Install

```sh
go install github.com/edge2992/dw@latest
```

Don't use Go? Grab a prebuilt binary for your OS/arch from the
[Releases](https://github.com/edge2992/dw/releases/latest) page (linux / macOS /
windows × amd64 / arm64, with `checksums.txt`). Check the installed version with
`dw version`.

## Shell integration

`dw` is a child process, so it cannot change your shell's working directory itself.
Instead it prints the chosen path to **stdout**, and a thin wrapper function does the
`cd`. The path-producing subcommands (the picker, `dw -`, `dw new`) are captured and
`cd`'d into; the others (`list`, `root`, …) pass straight through so their output
shows up as usual.

Let `dw` generate the wrapper for you — add this to your `~/.zshrc` (or `~/.bashrc`):

```zsh
eval "$(dw init zsh)"   # use `dw init bash` for bash
```

`dw init` prints the function below; eval'ing it keeps the integration in sync with
the binary, so there's nothing to hand-copy:

```zsh
dw() {
  case "${1:-}" in
    ''|-|new)
      local dir
      dir="$(command dw "$@")" && [ -n "$dir" ] && cd "$dir" ;;
    *)
      command dw "$@" ;;
  esac
}
```

Want to land in the workspace and launch Claude in one go? Add a second wrapper:

```zsh
function dwc() {
  case "${1:-}" in
    ''|-|new)
      local dir
      dir="$(command dw "$@")" && [ -n "$dir" ] && cd "$dir" && claude ;;
    *)
      command dw "$@" ;;
  esac
}
```

## Quickstart

```sh
dw                              # open the picker: fuzzy-find, or type a new topic to create one
dw -                           # jump straight back to your last workspace
dw new "my topic" -c research  # create a workspace non-interactively and cd in
dw list                        # print every workspace as category/name
dw root                        # print the workspace root
```

Prefer to compose your own picker? The interactive TUI is just sugar over the
primitives — wire `dw list` to your favourite fuzzy finder instead:

```sh
cd "$(dw list --json | jq -r '.[].path' | fzf)"
```

In the picker:

- Type to filter, `enter` to `cd` into the highlighted workspace.
- No match? A `+ create: <date>-<slug>` row appears → `enter` → **pick a category** → it's created and you `cd` in.
- At the category step, type an unknown name to spin up a **new category**; `esc` goes **back** to browse so you can retype the topic.
- `↑/↓` (or `ctrl+p` / `ctrl+n`) to move; `esc` / `ctrl+c` to abort.

## Commands

| Command | Description |
|---|---|
| `dw` | Open the interactive picker (fuzzy list + create-on-demand). |
| `dw -` | Jump to the last workspace; prints its path. |
| `dw new <topic> -c <cat>` | Create a workspace non-interactively; prints its path. |
| `dw list` | List workspaces as `category/name`, one per line. |
| `dw list --json` | List workspaces as a JSON array (includes absolute `path`). |
| `dw root` | Print the resolved workspace root. |
| `dw config path` | Print the resolved config file path. |
| `dw config init` | Write a starter `config.yml` (won't overwrite an existing one). |
| `dw init <zsh\|bash>` | Print the shell wrapper to `eval` (see Shell integration). |
| `dw version` | Print the version. |
| `dw help` / `-h` | Show usage. |

Every path-producing command writes to **stdout**; diagnostics go to stderr. That's
what makes the `dw()` wrapper and pipelines like `dw list | fzf` work.

## Layout

```text
<root>/<category>/<YYYY-MM-DD>-<topic-slug>/
  README.md   # frontmatter-indexed entry point
```

`<root>` defaults to `~/dw` (configurable). Categories are arbitrary folders; the
defaults offered when empty are `research`, `incident`, `discussion`, `scratch`.

## Configuration

`dw` runs with zero config — it defaults to `~/dw` and a built-in category set.
To relocate the workspace root, customize templates, or redefine categories, run
`dw config init` and edit `~/.config/dw/config.yml`.

→ Full reference: **[docs/configuration.md](docs/configuration.md)**

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
bumps the minor, `fix` the patch.

## License

[MIT](LICENSE) © edge2992
