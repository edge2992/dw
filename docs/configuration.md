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
`~/.config/dw/templates`. A new workspace gets two templated files, each resolved
per category, first match wins. (It also gets a `.claude/` directory, which is
*not* templated — see [Claude Code integration](#claude-code-integration-claude).)

`README.md`:

1. `<templates_dir>/<category>.md` — per-category
2. `<templates_dir>/default.md` — shared default
3. the built-in default, which **varies by category** — shared frontmatter plus
   headings suited to that kind of work: `research` leads with the question and
   an answer-with-confidence, `incident` with impact and an append-only timeline,
   `discussion` with decided/open, `scratch` with just a goal and what was tried.
   Any other category gets a generic question/answer/evidence shape.

`CLAUDE.md` — the same convention, one extension over:

1. `<templates_dir>/<category>.CLAUDE.md` — per-category
2. `<templates_dir>/default.CLAUDE.md` — shared default
3. the built-in default, which **varies by category** — `research`, `incident`,
   `discussion` and `scratch` each carry a few lines of stance suited to that kind
   of work, and any other category gets a generic few. The shared method behind
   them lives in `.claude/rules/investigation.md`
   (see [Claude Code integration](#claude-code-integration-claude)).

Every template substitutes `{{title}}`, `{{category}}`, `{{date}}`. The two
lookups are independent, so a `<templates_dir>/research.CLAUDE.md` overrides only
the `research` CLAUDE.md and leaves its README on the built-in scaffold.

Neither file is ever overwritten once it exists — re-creating a workspace, or
running `dw scaffold`, only fills in what is missing.

### Writing the guidance in another language

The built-in `CLAUDE.md` and `README.md` are English. They are ordinary templates,
so replacing them replaces the language too: put your own
`<templates_dir>/default.CLAUDE.md` and `<templates_dir>/default.md` in place for
every category, or `<category>.CLAUDE.md` / `<category>.md` for one at a time.
`.claude/rules/investigation.md` is dw's plumbing and is not templated — edit the
copy inside a workspace if you want to reword it there.

### `categories`

The categories offered in the picker, in order. When set it **replaces** the
built-in list entirely; omit it (or use `[]`) to keep the defaults. Categories
you create on the fly still appear automatically.

## Claude Code integration (`.claude/`)

Every workspace carries its own Claude Code project config, so a session started
inside it can pick up where the last one left off and work to the same standard:

```text
<workspace>/
  .claude/
    settings.json               # registers the Stop hook below
    rules/dw-workspace.md       # auto-loaded at startup; says to read the checkpoint
    rules/investigation.md      # auto-loaded at startup; how to investigate
    hooks/checkpoint.sh         # writes the checkpoint (mode 0755)
  .dw/last-session.md           # the checkpoint itself
```

`rules/investigation.md` is the method the `CLAUDE.md` stances sit on top of: pick
an effort tier and say which one you are on, write competing answers and a
falsification condition before searching, prefer primary and independent sources,
mark each claim fact / inference / assumption with its source, record what you
looked for and did not find, and argue against your own conclusion before calling
it done. It is deliberately one file and deliberately short — a long instruction
file gets half-ignored, so the category-specific judgement stays in `CLAUDE.md`.

At the end of every turn the `Stop` hook writes `.dw/last-session.md`: YAML
frontmatter with `session_id` and `ended_at`, then the turn's final message,
verbatim. It is a **snapshot, not a log** — each turn replaces the last. On the
next startup `rules/dw-workspace.md` tells Claude to read it, and to treat it as
a draft of where you left off rather than as verified fact. Durable conclusions
belong in `README.md`.

Notes:

- **Scope.** Because `settings.json` lives inside the workspace, the hook only
  runs for sessions started there. Your other repositories are untouched — this
  is why `dw` scaffolds a project-local config instead of a global hook.
- **No LLM, no delay.** The hook only reformats what Claude Code hands it on
  stdin. Nothing is summarized and nothing is waited on.
- **Requires `jq`.** Without it the hook exits quietly and no checkpoint is
  written; everything else keeps working.
- **Not templated.** Unlike `README.md` and `CLAUDE.md`, these four files have
  fixed contents and cannot be overridden through `templates_dir`. Because dw owns
  them, a `dw` upgrade plus `dw scaffold` carries an improved `investigation.md`
  into workspaces you created months ago.
- **Never overwritten.** If you already have a `.claude/settings.json`, `dw`
  leaves it alone — which also means the `Stop` hook is not registered. Merge the
  `hooks.Stop` entry into your own file by hand, or delete it and re-run
  `dw scaffold`.

## Backfilling existing workspaces

Workspaces created before `dw` scaffolded `CLAUDE.md` and `.claude/` don't have
them, and ones created before a given `.claude/` file existed are missing that
file. `dw scaffold` walks the root and writes whatever is missing, resolving each
`CLAUDE.md` from its own category's template. It never touches an existing file,
so it is safe to re-run and needs no confirmation — an old workspace picks up
`rules/investigation.md` while keeping the `README.md` and `CLAUDE.md` you have
already written in.

```sh
dw scaffold                 # backfill everything
dw scaffold -c research     # just one category
dw scaffold --dry-run       # list what would be written, write nothing
```

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
