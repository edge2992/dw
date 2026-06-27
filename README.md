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

- **Dated auto-layout** — workspaces live at `<root>/<category>/<YYYY-MM-DD>-<topic-slug>/`, created for you.
- **Create on demand, no `create` command** — type a topic; if nothing matches, pick a category and `dw` makes it. Categories are created on the fly too.
- **Category chosen after the topic** — write what you're thinking about first, file it second.
- **Fuzzy jump** — fuzzy-match across `category/name` and titles, newest first, fzf-style.
- **Resume instantly** — your last workspace is pinned to the top; `dw -` jumps to it with no UI.
- **Frontmatter-aware** — shows `status` / `tags` / `created` from each README under the selection.
- **Scriptable primitives** — the TUI is sugar over plain commands: `dw new` creates, `dw list` (`--json`) streams, so you can compose your own flow (`dw list --json | fzf`) instead of the picker.
- **Unicode-safe slugs** — Japanese and other scripts survive slugification (`機械学習 調査` → `機械学習-調査`).
- **Zero-config, YAML when you want it** — works out of the box at `~/dw`; relocate the root, point templates elsewhere, or redefine categories in `~/.config/dw/config.yml`.

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

`dw` reads everything from `~/.config/dw/config.yml`. It's entirely optional —
with no file, dw uses the built-in defaults below. Scaffold one with
`dw config init`, then edit it:

```yaml
# ~/.config/dw/config.yml — every key is optional; omitted keys use the defaults.
root: ~/dw                          # workspace root
templates_dir: ~/.config/dw/templates  # per-category template directory
categories:                         # picker categories, in order (replaces the defaults)
  - research
  - incident
  - discussion
  - scratch
```

- **`root`** — workspace root. `~` and `$ENV` (e.g. `$HOME`, `${XDG_DATA_HOME}`)
  are expanded. Default: `~/dw`.
- **`templates_dir`** — where per-category templates live (also `~`/`$ENV`-expanded).
  Default: `~/.config/dw/templates`. The template for a new workspace is picked
  per category, first match wins:
  1. `<templates_dir>/<category>.md` — per-category
  2. `<templates_dir>/default.md` — shared default
  3. built-in default (works with nothing configured)

  All substitute `{{title}}`, `{{category}}`, `{{date}}`. Drop a
  `<templates_dir>/research.md` to give just the `research` category its own scaffold.
- **`categories`** — the categories offered in the picker, in order. When set it
  **replaces** the built-in list entirely; omit it to keep the defaults.
  Categories you create on the fly still appear automatically.
- **`DW_CONFIG`** (env) — overrides only the config file *location* (not its
  values). Handy for hermetic tests or keeping multiple profiles.
- **Last-workspace cache** — recorded under `os.UserCacheDir()` (`~/Library/Caches/dw/last`
  on macOS, `~/.cache/dw/last` on Linux). Drives both the top-of-list pin and `dw -`.

## Migration

> **Breaking change.** Configuration moved from environment variables to
> `~/.config/dw/config.yml`. The **`DW_ROOT` env var is no longer read** — set
> `root:` in the config instead. Templates also moved from
> `~/.config/discussion/templates/` to `~/.config/dw/templates/` (the old
> `discussion` paths are no longer searched).

If you relied on `DW_ROOT`, persist it once:

```sh
dw config init                       # writes ~/.config/dw/config.yml
# then edit it: set `root:` to your old $DW_ROOT value
```

Move any custom templates to the new location:

```sh
mkdir -p ~/.config/dw/templates
mv ~/.config/discussion/templates/* ~/.config/dw/templates/ 2>/dev/null
```

(Historical: the default root previously moved from `~/Discussion` to `~/dw`, and
`DISCUSSION_ROOT` was renamed `DW_ROOT` before being retired here.)

## Architecture

- `internal/config` — loads/resolves `~/.config/dw/config.yml` (root / templates_dir / categories), with `~` and `$ENV` expansion and built-in defaults.
- `internal/workspace` — scanning / slugification / creation / templates / last-path persistence (pure logic, tested).
- `internal/tui` — the single bubbletea fuzzy list (jump + create + category select + pin).
- `main.go` — subcommand dispatch (`run()`); loads config once and wires `dw -`, `new`, `list`, `root`, `config`, `init`, `version`, `help`, and the picker. `dw new` and the picker share the same `workspace.Create` core.

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
