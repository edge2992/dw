# dw

[![release](https://img.shields.io/github/v/release/edge2992/dw)](https://github.com/edge2992/dw/releases/latest)
[![license](https://img.shields.io/github/license/edge2992/dw)](LICENSE)

**Give every topic you think about its own directory and a note named after it. Start one the moment the idea comes to you, then find it by name and pick up where you left off.**

```text
$ dw 調べ方を設計する
$ pwd
/Users/me/vault/topics/2026-10-24-調べ方を設計する
$ ls
調べ方を設計する.md
```

## Why

Some ideas are a passing thought; others turn into projects that run for months, and you can't tell which is which on the day you have them. `dw` gives both the same shape, so nothing ever has to be migrated or "promoted":

- **One topic, one directory** — `<root>/<YYYY-MM-DD>-<name>/<name>.md`. The main note shares the topic's name, so it's easy to tell apart in Obsidian's quick switcher and links.
- **Only the name is required** — no categories, statuses, or templates. The main note starts as a single `# <name>` line.
- **Names stay as you typed them** — Japanese, spaces, and capitals are kept. A character that would break a path or an Obsidian link (`/ \ : # ^ [ ] |`, a leading `.`) is an error, never a silent rewrite.
- **The filesystem is the source of truth** — `dw` lists whatever directories sit under the root. Topics made by hand in Obsidian show up too; there's no index to keep in sync.
- **One command, no modes** — `dw` always opens an `fzf` picker. Type to filter, or keep typing past the last match and `enter` creates that topic.
- **Resume by default** — the last topic you visited is pinned to the top.
- **Zero config, zero dependencies** — one environment variable (`DW_ROOT`) and the Go standard library only.

See [docs/concepts.md](docs/concepts.md) for the reasoning and [GLOSSARY.md](GLOSSARY.md) for the vocabulary.

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

`dw` is a child process, so it can't change your shell's working directory, and it doesn't know about `fzf`. Both jobs belong to a thin shell wrapper that `dw init` prints:

```zsh
eval "$(dw init zsh)"   # or: dw init bash
```

The wrapper:

1. Runs `command dw` and captures the TSV rows.
2. Hands them to `fzf`, every time. The path column is hidden, the preview shows the main note (or the directory listing for a topic without one), and `dw <name>` arrives as `--query=<name>`.
3. On `enter` over a row, re-resolves that path through `command dw` (so it becomes the last-visited topic) and `cd`s into it.
4. If nothing matched what you typed, hands the query to `command dw`, which matches it ignoring case and creates the topic only if that misses too.

So the picker never dead-ends: `enter` on a match opens it, `enter` on no match creates it, `esc` does nothing.

Without `fzf`, or outside a terminal, the wrapper resolves your arguments directly and prints the candidates if there's more than one.

### Commands

```sh
dw                      # open the picker: filter, or type a new name and press enter
dw 調べ方                # same picker, pre-filtered
```

| Command               | Description                                                                                                         |
| --------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `dw`                  | Open the picker over every topic, last visited pinned first.                                                        |
| `dw <name>`           | Open the picker with `<name>` pre-typed. Without shell integration: exact match, else partial match(es), else create. |
| `dw init <zsh\|bash>` | Print the shell function that wires `dw` into `fzf` and `cd`.                                                       |
| `dw version`          | Print the version.                                                                                                  |
| `dw help`             | Show usage, including the resolved `DW_ROOT`.                                                                       |

`command dw` prints a tab-separated contract to stdout, one row per topic:

```
<absolute path>\t<marker + directory name>
```

The marker is `* ` for the last-visited topic and two spaces otherwise. Diagnostics go to stderr. An invalid or empty name exits 2 and creates nothing. Names match against the topic name (the directory name minus its date), ignoring case: exact matches first, then substrings.

### What `dw` creates

```text
<root>/<YYYY-MM-DD>-<name>/
  <name>.md          # main note: "# <name>"
```

That's all. Add `logs/`, `sources/`, images, or more notes whenever a topic needs them; `dw` doesn't look at them. It never writes into a topic that already exists.

Topics are listed newest date first. Directories without a date prefix (say, `読書を続ける/` made in Obsidian) are topics too, with the whole directory name as their topic name, and come last.

## Configuration

`DW_ROOT` is the only setting, and it defaults to `~/dw`. Point it at a folder inside your Obsidian vault if that's where you want topics to live:

```sh
export DW_ROOT=~/Documents/vault/topics
```

`dw` makes `DW_ROOT` absolute, but set it to an absolute path anyway (`~/…` or `$HOME/…`). The wrapper `cd`s you into a topic, so a relative `DW_ROOT` would be resolved again from inside that topic the next time you ran `dw`.

## License

[MIT](LICENSE) © edge2992
