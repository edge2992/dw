// Command dw is a discussion workspace picker. It resolves a topic to a
// <DW_ROOT>/<YYYY-MM-DD>-<topic>/ directory (creating it if needed) and
// prints TSV to stdout; a thin shell wrapper (see `dw init`) does the cd,
// handing off to fzf when a topic resolves to more than one candidate.
//
// See docs/concepts.md for the ideas behind the layout, and the package doc
// of internal/workspace for the resolution algorithm.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/edge2992/dw/internal/workspace"
)

// version is the build version, injected via -ldflags at release time
// (see .goreleaser.yaml). Plain `go install`/`go build` leave it as "dev",
// in which case cmdVersion falls back to the module version from the build info.
var version = "dev"

const usageTemplate = `dw — discussion workspace picker

Usage:
  dw                  List workspaces as TSV (most recently visited pinned first)
  dw <topic>          Resolve <topic>: exact match, else partial match(es), else create it
  dw init <zsh|bash>  Print the shell function that wires dw into fzf and cd
  dw version          Print the version
  dw help             Show this help

Output (stdout, tab-separated, one row per workspace):
  <absolute path>	<marker + title>	<first open question>	<directory name>
The marker is "* " for the most recently visited workspace, "  " otherwise.
When <topic> matches more than one workspace, every match is printed — dw
never picks one for you; the shell wrapper hands the rows to fzf.

With shell integration enabled, both forms open the fzf picker instead of
printing: bare dw lists everything, dw <topic> opens with <topic> pre-typed,
and whatever you type that matches nothing becomes a new workspace.

Enable shell integration once with:  eval "$(dw init zsh)"   (or bash)

Root: %s
`

// emptyRootHint replaces the empty listing a first run would otherwise print.
// It is only shown when a human is looking (see hintFirstRun) — with the shell
// wrapper installed, the empty fzf picker says the same thing interactively.
const emptyRootHint = `dw: no workspaces yet in %s

  dw <topic>   create your first workspace (e.g. dw datadog-cost)
  dw help      full usage

Enable the picker once with:  eval "$(dw init zsh)"   (or bash)
`

// integrationHint fires when someone runs dw straight from a prompt and gets
// raw TSV back. That output is meant for the shell wrapper, not for reading,
// so without this the tool looks broken rather than unconfigured.
const integrationHint = `dw: shell integration is not enabled, so these rows are raw TSV.
    Run  eval "$(dw init zsh)"  (or bash) to get the fzf picker and cd.
`

func main() { os.Exit(run(os.Args, os.Stdout, os.Stderr)) }

// run dispatches argv to a subcommand and returns the process exit code.
// argv[0] is the program name; argv[1] is the subcommand or topic (if any).
func run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) >= 2 {
		switch argv[1] {
		case "init":
			return cmdInit(stdout, stderr, argv[2:])
		case "version", "--version", "-v":
			return cmdVersion(stdout)
		case "help", "--help", "-h":
			return cmdHelp(stdout)
		}
	}

	root := workspace.Root()
	if len(argv) < 2 {
		return cmdList(root, stdout, stderr)
	}
	// Everything after argv[0] that isn't a reserved subcommand is the topic,
	// joined back with spaces — so an unquoted `dw my topic` behaves the same
	// as `dw "my topic"`.
	topic := strings.Join(argv[1:], " ")
	return cmdResolve(root, stdout, stderr, topic)
}

// isTerminal reports whether w is a character device. It is how dw tells "a
// human is reading this" from "the shell wrapper or a script is capturing it":
// the wrapper always reads dw through $(...), so a terminal on stdout means
// shell integration is not in play. Declared as a variable so tests can
// pretend either way.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// hintFirstRun writes guidance to stderr for the two ways dw's stdout can be
// unhelpful on its own: nothing to list, or TSV nobody asked to read. Both are
// only worth saying when stdout is a terminal — piped output belongs to the
// wrapper or a script, and rows is 0 there simply means "no workspaces".
// stdout itself is left alone either way, so the TSV contract still holds.
func hintFirstRun(stdout, stderr io.Writer, root string, rows int) {
	if !isTerminal(stdout) {
		return
	}
	if rows == 0 {
		fmt.Fprintf(stderr, emptyRootHint, root)
		return
	}
	fmt.Fprint(stderr, integrationHint)
}

// cmdList prints every workspace under root as TSV (`dw` with no arguments).
// It never touches the "last visited" pin — only resolving a single
// workspace does that (see cmdResolve).
func cmdList(root string, stdout, stderr io.Writer) int {
	projects, err := workspace.Scan(root)
	if err != nil {
		fmt.Fprintln(stderr, "dw: scan:", err)
		return 1
	}
	last := workspace.LastPath()
	printRows(stdout, workspace.PinLast(projects, last), last)
	hintFirstRun(stdout, stderr, root, len(projects))
	return 0
}

// cmdResolve resolves arg to one or more workspaces (`dw <topic>`), creating
// one if nothing matches. SaveLast is called only when Resolve settles on
// exactly one workspace — printing several candidates for fzf to choose
// among is not itself a "visit".
func cmdResolve(root string, stdout, stderr io.Writer, arg string) int {
	matches, _, err := workspace.Resolve(root, arg)
	if err != nil {
		switch {
		case errors.Is(err, workspace.ErrEmptyTopic):
			fmt.Fprintln(stderr, "dw:", err)
			return 2
		case errors.Is(err, workspace.ErrNotFound):
			fmt.Fprintln(stderr, "dw:", err)
			return 1
		default:
			fmt.Fprintln(stderr, "dw:", err)
			return 1
		}
	}

	last := workspace.LastPath()
	if len(matches) == 1 {
		if err := workspace.SaveLast(matches[0].Path); err != nil {
			fmt.Fprintln(stderr, "dw: warning: could not save last workspace:", err)
		} else {
			last = matches[0].Path
		}
	}
	printRows(stdout, matches, last)
	// Resolve always yields at least one match (it creates when nothing
	// matches), so this only ever prints the shell-integration hint.
	hintFirstRun(stdout, stderr, root, len(matches))
	return 0
}

// printRows writes one TSV row per project (see workspace.Project.TSVRow).
func printRows(w io.Writer, projects []workspace.Project, last string) {
	for _, p := range projects {
		fmt.Fprintln(w, p.TSVRow(last))
	}
}

// shellInit is what dw prints from `dw init`. dw is a child process, so it can
// neither cd nor drive fzf; both belong here. Interactively the picker is the
// whole interface — it always opens, however many workspaces exist, because a
// list of one is still a list you might not want to enter, and a list of none
// is exactly when you want to type a name. Anything typed that matches nothing
// is handed back to `command dw`, which resolves it or creates it.
//
// __dw_pick is split out of dw() so tests can drive the picker with a stub fzf
// without allocating a terminal: dw() owns the `[ -t 0 ]` gate, __dw_pick owns
// the loop. zsh and bash share this body.
const shellInit = `__dw_pick() {
  local rows out query sel st
  rows="$(command dw)" || return
  query="$*"
  while :; do
    # printf '%s' rather than '%s\n': an empty list has to reach fzf as zero
    # items, not as one blank line.
    out="$(printf '%s' "$rows" | fzf --print-query --query="$query" \
             --delimiter=$'\t' --with-nth=2.. --prompt='dw> ' \
             --header='enter: open · type a new topic + enter: create' \
             --preview='head -40 {1}/STATE.md' --preview-window=right,60%)"
    st=$?
    # --print-query puts the query on the first line, whatever the exit status.
    query="${out%%$'\n'*}"
    if [ "$st" -eq 0 ]; then sel="${out#*$'\n'}"; break; fi
    # 1 means nothing matched; anything else (130) is ESC or ctrl-c.
    [ "$st" -eq 1 ] && [ -n "$query" ] || return 1
    # fzf gave up, but dw matches on the slug and may still find it — and only
    # creates a workspace when that misses too.
    rows="$(command dw "$query")" || return
    case "$rows" in
      # Several slug matches: pick among them, with the query cleared. Keeping
      # it would re-open fzf on a query fzf already failed to match, forever.
      *$'\n'*) query=''; continue ;;
      *) sel="$rows"; break ;;
    esac
  done
  printf '%s\n' "$sel"
}

dw() {
  case "${1:-}" in
    init|help|version|--help|-h|--version|-v)
      command dw "$@"
      return ;;
  esac

  local sel
  if [ -t 0 ] && command -v fzf >/dev/null 2>&1; then
    sel="$(__dw_pick "$@")" || return
  else
    # No picker: resolve the arguments the way dw always has, and stop short of
    # cd'ing when that leaves more than one candidate.
    sel="$(command dw "$@")" || return
    if [ -z "$sel" ]; then
      printf 'dw: no workspaces yet - create one with: dw <topic>\n' >&2
      return 0
    fi
    case "$sel" in
      *$'\n'*) printf '%s\n' "$sel" | cut -f2-; return 0 ;;
    esac
  fi
  [ -n "$sel" ] || return 0
  # Re-resolve the chosen path so it becomes the new "last visited" workspace
  # (SaveLast only fires for a single match — see internal/workspace.Resolve).
  sel="$(command dw "${sel%%$'\t'*}")" || return
  cd "${sel%%$'\t'*}"
}
`

// cmdInit prints the shell wrapper for the requested shell (`dw init zsh|bash`).
func cmdInit(stdout, stderr io.Writer, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "dw init: specify a shell\nUsage: dw init <zsh|bash>")
		return 2
	}
	switch args[0] {
	case "zsh", "bash":
		// io.WriteString, not fmt.Fprint: shellInit is a shell script full of
		// literal %-directives (printf '%s\n' ...), which `go vet` would
		// otherwise flag as a missing Printf argument.
		io.WriteString(stdout, shellInit) //nolint:errcheck // best-effort; a broken stdout pipe is not actionable here
		return 0
	default:
		fmt.Fprintf(stderr, "dw init: unsupported shell %q (supported: zsh, bash)\n", args[0])
		return 2
	}
}

// cmdVersion prints the build version (`dw version`). Released binaries carry
// the version injected via -ldflags; for `go install module@version` builds it
// falls back to the module version recorded in the build info.
func cmdVersion(stdout io.Writer) int {
	v := version
	if v == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version
		}
	}
	fmt.Fprintln(stdout, "dw", v)
	return 0
}

// cmdHelp prints the usage text, including the resolved root (`dw help`).
func cmdHelp(stdout io.Writer) int {
	fmt.Fprintf(stdout, usageTemplate, workspace.Root())
	return 0
}
