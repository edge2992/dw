# Configuration

`dw` runs with **zero configuration** — with no file it uses built-in defaults
(workspace root `~/dw`; the categories `research`, `incident`, `discussion`,
`scratch`). Everything below is optional.

Settings live in `~/.config/dw/config.yml`. Scaffold a starter file with
`dw config init`, then edit it:

```yaml
# ~/.config/dw/config.yml — every key is optional; omitted keys use the defaults.
root: ~/dw                              # workspace root
templates_dir: ~/.config/dw/templates   # per-category template directory
categories:                             # picker categories, in order (replaces the defaults)
  - research
  - incident
  - discussion
  - scratch
```

`dw config path` prints the resolved config location; `dw config init` writes the
starter file and refuses to overwrite an existing one.

## Keys

### `root`

Workspace root. A leading `~` and `$ENV` references (e.g. `$HOME`,
`${XDG_DATA_HOME}`) are expanded. Default: `~/dw`.

### `templates_dir`

Directory holding per-category templates (also `~`/`$ENV`-expanded). Default:
`~/.config/dw/templates`. The template for a new workspace is chosen per
category, first match wins:

1. `<templates_dir>/<category>.md` — per-category
2. `<templates_dir>/default.md` — shared default
3. the built-in default (used when nothing is configured)

All three substitute `{{title}}`, `{{category}}`, `{{date}}`. Drop a
`<templates_dir>/research.md` to give just the `research` category its own
scaffold.

### `categories`

The categories offered in the picker, in order. When set it **replaces** the
built-in list entirely; omit it (or use `[]`) to keep the defaults. Categories
you create on the fly still appear automatically.

## `DW_CONFIG` (environment)

Overrides only the config file *location*, not its values — handy for hermetic
tests or keeping multiple profiles:

```sh
DW_CONFIG=~/work/dw.yml dw root
```

## Last-workspace cache

The last chosen workspace is recorded under `os.UserCacheDir()`
(`~/Library/Caches/dw/last` on macOS, `~/.cache/dw/last` on Linux). It drives
both the top-of-list pin and `dw -`.
