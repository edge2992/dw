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
- **Names you can type in a shell** — the directory and the main note drop the spaces and symbols that break a shell word, a path, or an Obsidian link. Each run of spaces (full-width included) or of `/ \ : # ^ [ ] | ? * < > " ' $ & ; ( ) { } ! ~` and backtick becomes one `-`, and a leading or trailing `-` or `.` is dropped. Japanese, capitals and a `.` inside the name are kept, so `決済API 5xx 14:05` becomes `決済API-5xx-14-05`. `dw` prints the rewrite on stderr, and the main note's heading keeps the name as you typed it.
- **The filesystem is the source of truth** — `dw` lists whatever directories sit under the root. Topics made by hand in Obsidian show up too; there's no index to keep in sync.
- **One picker** — `dw` always opens an `fzf` picker. Type to filter, or keep typing past the last match and `enter` creates that topic. When what you type is part of an existing name (a recurring incident, an umbrella topic), `ctrl-o` creates it anyway.
- **Resume by default** — the five topics you visited most recently are pinned to the top, so a long-running topic doesn't sink under a day of short ones.
- **Zero config** — one environment variable (`DW_ROOT`). The only dependency is `golang.org/x/text`, for Unicode normalization.

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
2. Hands them to `fzf`, every time. The path column is hidden, and `dw <name>` arrives as `--query=<name>`. The preview shows the files in the topic on one line, then the head of the main note (or of `README.md` or `STATE.md` for older topics without one).
3. On `enter` over a row, re-resolves that path through `command dw` (so it becomes the most recent visit) and `cd`s into it.
4. If nothing matched what you typed, hands the query to `command dw`, which matches it ignoring case, spaces, `-` and `_`, and creates the topic only if that misses too.
5. On `ctrl-o`, hands the query to `command dw new`, which creates a topic with exactly that name even if it is part of an existing one. Only an exact name match opens instead.

So the picker never dead-ends: `enter` on a match opens it, `enter` on no match creates it, `ctrl-o` creates what you typed, `esc` does nothing. Every new topic is announced as `dw: created <path>`.

Without `fzf`, or outside a terminal, the wrapper resolves your arguments directly and prints the candidates if there's more than one.

### Commands

```sh
dw                      # open the picker: filter, or type a new name and press enter
dw 調べ方                # same picker, pre-filtered
```

| Command               | Description                                                                                                         |
| --------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `dw`                  | Open the picker over every topic, recently visited pinned first.                                                    |
| `dw <name>`           | Open the picker with `<name>` pre-typed. Without shell integration: exact match, else partial match(es), else create. |
| `dw new <name>`       | Create `<name>` and `cd` into it, even when it is part of an existing name. An exact match opens instead.          |
| `dw init <zsh\|bash>` | Print the shell function that wires `dw` into `fzf` and `cd`.                                                       |
| `dw version`          | Print the version.                                                                                                  |
| `dw help`             | Show usage, including the resolved `DW_ROOT`.                                                                       |

`command dw` prints a tab-separated contract to stdout, one row per topic:

```
<absolute path>\t<marker + directory name>
```

The marker is `* ` for the most recently visited topic and two spaces otherwise. Diagnostics go to stderr, including `dw: created <path>` whenever a topic is made, preceded by `dw: "<as typed>" → <name>` when the name was rewritten. A name that is empty once rewritten, holds a control character, or is reserved on Windows (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`) exits 2 and creates nothing. Names are compared in Unicode NFC against the topic name (the directory name minus its date), ignoring case, spaces, `_` and the symbols above: exact matches first, then substrings. So `dw 決済API 5xx 14:05` opens `決済API-5xx-14-05`, and topics made by older versions with spaces in their names are still found. Any `command dw <name>` that settles on one topic counts as a visit.

### What `dw` creates

```text
<root>/<YYYY-MM-DD>-<name>/
  <name>.md          # main note: "# <name as typed>"
```

That's all. Add `logs/`, `sources/`, images, or more notes whenever a topic needs them; `dw` doesn't look at them. It never writes into a topic that already exists.

Topics are listed newest date first. Directories without a date prefix (say, `読書を続ける/` made in Obsidian) are topics too, with the whole directory name as their topic name, and come last. Every directory directly under the root is a topic, so keep anything else (an attachments folder, say) outside it or give it a name starting with `.`.

A topic that has grown html, txt, or other notes reads best when its main note links to them. A topic from an older `dw` or made by hand gets a main note the moment you add `<name>.md` to it; `dw` never adds one for you.

### Known limits

- bash, and zsh with `interactive_comments`, treat an unquoted ` #` as the start of a comment, so `dw 決済API #123` drops `#123` before `dw` sees it. Quote the name to keep it; it becomes `決済API-123`.

### With Claude Code

The usual loop is `dw <name>`, then `claude` in the topic, and `dw` again the next day to pick it back up. `dw` stops at the `cd`; it never launches `claude` and never writes a `CLAUDE.md`.

Left alone, Claude doesn't know what the main note is for. Asked to "save where we got to", it tends to start a new file such as `結論.md` and leave `<name>.md` as a bare heading, so the picker's preview shows nothing of the discussion. A `CLAUDE.md` you write once at the root fixes that. `dw` lists only directories, so the file never shows up as a topic:

```markdown
Each directory here is a topic. Its main note is `<directory name without the YYYY-MM-DD- prefix>.md`. Write conclusions, current status, and links to other files in the topic into that main note, not into a new file.
```

Claude Code keeps sessions and auto memory per directory, in a folder named after the path with every non-ASCII character turned into `-`. Two topics created on the same day whose names differ only in Japanese characters of the same length, such as `ジョブ基盤-選定` and `帳票サーバ-選定`, therefore share one folder. `claude --continue` and `--resume` still pick sessions by the exact directory, but auto memory is shared, so notes from one topic can surface in the other.

## Configuration

`DW_ROOT` is the only setting, and it defaults to `~/dw`. Point it at a folder inside your Obsidian vault if that's where you want topics to live:

```sh
export DW_ROOT=~/Documents/vault/topics
```

`dw` makes `DW_ROOT` absolute, but set it to an absolute path anyway (`~/…` or `$HOME/…`). The wrapper `cd`s you into a topic, so a relative `DW_ROOT` would be resolved again from inside that topic the next time you ran `dw`.

## License

[MIT](LICENSE) © edge2992
