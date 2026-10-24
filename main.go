// Command dw is a topic picker. It resolves a name to a
// <DW_ROOT>/<YYYY-MM-DD>-<name>/ directory holding <name>.md (creating it if
// needed) and prints TSV to stdout; a thin shell wrapper (see `dw init`) does
// the cd, handing off to fzf to choose among candidates.
//
// See docs/concepts.md for the ideas behind the layout, and the package doc
// of internal/topic for the resolution algorithm.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/edge2992/dw/internal/topic"
)

// version is the build version, injected via -ldflags at release time
// (see .goreleaser.yaml). Plain `go install`/`go build` leave it as "dev",
// in which case cmdVersion falls back to the module version from the build info.
var version = "dev"

const usageTemplate = `dw — topic picker

Usage:
  dw                  List topics as TSV (recently visited pinned first)
  dw <name>           Resolve <name>: exact match, else partial match(es), else create
                      <root>/<YYYY-MM-DD>-<name>/<name>.md
  dw new <name>       Open <name> if it exists exactly, else create it, even when
                      <name> is part of an existing topic's name
  dw init <zsh|bash>  Print the shell function that wires dw into fzf and cd
  dw version          Print the version
  dw help             Show this help

Output (stdout, tab-separated, one row per topic):
  <absolute path>	<marker + directory name>
The marker is "* " for the most recently visited topic, "  " otherwise.
Names match ignoring case, spaces, "-" and "_". A new topic is announced on
stderr as "dw: created <path>". When <name> matches more than one topic, every
match is printed — dw never picks one for you; the shell wrapper hands the
rows to fzf.

With shell integration enabled, both forms open the fzf picker instead of
printing: bare dw lists everything, dw <name> opens with <name> pre-typed,
whatever you type that matches nothing becomes a new topic, and ctrl-o
creates what you typed even when it matches something (same as dw new).

Enable shell integration once with:  eval "$(dw init zsh)"   (or bash)

Root: %s
`

// emptyRootHint replaces the empty listing a first run would otherwise print.
// It is only shown when a human is looking (see hintFirstRun) — with the shell
// wrapper installed, the empty fzf picker says the same thing interactively.
const emptyRootHint = `dw: no topics yet in %s

  dw <name>    create your first topic (e.g. dw 調べ方を設計する)
  dw help      full usage

Enable the picker once with:  eval "$(dw init zsh)"   (or bash)
`

// integrationHint fires when someone runs dw straight from a prompt and gets
// raw TSV back. That output is meant for the shell wrapper, not for reading,
// so without this the tool looks broken rather than unconfigured.
const integrationHint = `dw: these rows are raw TSV, meant for the shell wrapper.
    If you haven't, run  eval "$(dw init zsh)"  (or bash) for the fzf picker and cd.
`

func main() { os.Exit(run(os.Args, os.Stdout, os.Stderr)) }

// run dispatches argv to a subcommand and returns the process exit code.
// argv[0] is the program name; argv[1] is the subcommand or name (if any).
func run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) >= 2 {
		switch argv[1] {
		case "new":
			return cmdNew(topic.Root(), stdout, stderr, argv[2:])
		case "init":
			return cmdInit(stdout, stderr, argv[2:])
		case "version", "--version", "-v":
			return cmdVersion(stdout)
		case "help", "--help", "-h":
			return cmdHelp(stdout)
		}
	}

	root := topic.Root()
	if len(argv) < 2 {
		return cmdList(root, stdout, stderr)
	}
	// Everything after argv[0] that isn't a reserved subcommand is the name,
	// joined back with spaces — so an unquoted `dw my topic` behaves the same
	// as `dw "my topic"`.
	return cmdResolve(root, stdout, stderr, strings.Join(argv[1:], " "))
}

// isTerminal reports whether w is a character device other than /dev/null,
// which is one too. It is how dw tells "a human is reading this" from "the
// shell wrapper or a script is capturing it": the wrapper always reads dw
// through $(...), so a terminal on stdout means shell integration is not in
// play. Declared as a variable so tests can pretend either way.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(fi, null)
}

// hintFirstRun writes guidance to stderr for the two ways dw's stdout can be
// unhelpful on its own: nothing to list, or TSV nobody asked to read. Both are
// only worth saying when stdout is a terminal — piped output belongs to the
// wrapper or a script, and rows is 0 there simply means "no topics".
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

// cmdList prints every topic under root as TSV (`dw` with no arguments),
// recently visited ones first. It never records a visit — only resolving a
// single topic does that (see printResolved).
func cmdList(root string, stdout, stderr io.Writer) int {
	topics, err := topic.Scan(root)
	if err != nil {
		fmt.Fprintln(stderr, "dw: scan:", err)
		return 1
	}
	recent := topic.Recent()
	printRows(stdout, topic.PinRecent(topics, recent), first(recent))
	hintFirstRun(stdout, stderr, root, len(topics))
	return 0
}

// cmdResolve resolves arg to one or more topics (`dw <name>`), creating one
// if nothing matches.
func cmdResolve(root string, stdout, stderr io.Writer, arg string) int {
	matches, created, err := topic.Resolve(root, arg)
	return printResolved(root, stdout, stderr, matches, created, err)
}

// cmdNew opens the topic named exactly args, or creates it even when the
// name is part of an existing one (`dw new <name>`).
func cmdNew(root string, stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "dw new: specify a name\nUsage: dw new <name>")
		return 2
	}
	t, created, err := topic.New(root, strings.Join(args, " "))
	return printResolved(root, stdout, stderr, []topic.Topic{t}, created, err)
}

// printResolved reports the outcome of Resolve or New. A new topic is
// announced on stderr, since its row looks the same as an existing one's and
// a typo or a search term would otherwise become a topic unnoticed. The visit
// is recorded only for a single match — printing several candidates for fzf
// to choose among is not itself a visit.
func printResolved(root string, stdout, stderr io.Writer, matches []topic.Topic, created bool, err error) int {
	if err != nil {
		fmt.Fprintln(stderr, "dw:", err)
		if errors.Is(err, topic.ErrEmptyName) || errors.Is(err, topic.ErrInvalidName) {
			return 2
		}
		return 1
	}
	if created {
		fmt.Fprintln(stderr, "dw: created", matches[0].Path)
	}

	last := first(topic.Recent())
	if len(matches) == 1 {
		if err := topic.SaveLast(matches[0].Path); err != nil {
			fmt.Fprintln(stderr, "dw: warning: could not save last topic:", err)
		} else {
			last = matches[0].Path
		}
	}
	printRows(stdout, matches, last)
	// There is always at least one match (Resolve creates when nothing
	// matches), so this only ever prints the shell-integration hint.
	hintFirstRun(stdout, stderr, root, len(matches))
	return 0
}

func first(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

// printRows writes one TSV row per topic (see topic.Topic.TSVRow).
func printRows(w io.Writer, topics []topic.Topic, last string) {
	for _, t := range topics {
		fmt.Fprintln(w, t.TSVRow(last))
	}
}

// shellInit is what dw prints from `dw init`. dw is a child process, so it can
// neither cd nor drive fzf; both belong here. Interactively the picker is the
// whole interface — it always opens, however many topics exist, because a
// list of one is still a list you might not want to enter, and a list of none
// is exactly when you want to type a name. Anything typed that matches nothing
// is handed back to `command dw`, which resolves it or creates it.
//
// __dw_pick is split out of dw() so tests can drive the picker with a stub fzf
// without allocating a terminal: dw() owns the `[ -t 0 ]` gate, __dw_pick owns
// the loop. zsh and bash share this body.
const shellInit = `# Preview the head of the main note (<dir>/<dir minus date prefix>.md), or of
# README.md or STATE.md for topics that have none, then the files the topic
# holds, since its html, txt and other notes are often what tells topics apart.
__dw_preview='d={1}; n=${d##*/}; n=${n#[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-}; for f in "$d/$n.md" "$d/README.md" "$d/STATE.md"; do [ -f "$f" ] && { head -30 "$f"; echo; break; }; done; echo "--- files"; ls "$d"'

__dw_pick() {
  local rows out query key sel st
  rows="$(command dw)" || return
  query="$*"
  while :; do
    # printf '%s' rather than '%s\n': an empty list has to reach fzf as zero
    # items, not as one blank line. --scheme=history keeps dw's order (recent
    # first, then newest) among equally good matches.
    out="$(printf '%s' "$rows" | fzf --print-query --expect=ctrl-o --query="$query" \
             --delimiter=$'\t' --with-nth=2.. --prompt='dw> ' --scheme=history \
             --header='enter: open, or create if nothing matches · ctrl-o: create as typed' \
             --preview="$__dw_preview" --preview-window=right,60%)"
    st=$?
    # fzf prints the query, then the --expect key (empty for enter), then the
    # selection; the key line is there even when nothing matched.
    query="${out%%$'\n'*}"
    case "$out" in *$'\n'*) out="${out#*$'\n'}" ;; *) out='' ;; esac
    key="${out%%$'\n'*}"
    if [ "$key" = ctrl-o ]; then
      [ -n "$query" ] || return 1
      sel="$(command dw new "$query")" || return
      break
    fi
    if [ "$st" -eq 0 ]; then sel="${out#*$'\n'}"; break; fi
    # 1 means nothing matched; anything else (130) is ESC or ctrl-c.
    [ "$st" -eq 1 ] && [ -n "$query" ] || return 1
    # fzf gave up, but dw matches ignoring case and separators and may still
    # find it — and only creates a topic when that misses too.
    rows="$(command dw "$query")" || return
    case "$rows" in
      # Several name matches: pick among them, with the query cleared. Keeping
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
  if [ "${1:-}" = new ]; then
    sel="$(command dw "$@")" || return
  elif [ -t 0 ] && command -v fzf >/dev/null 2>&1; then
    sel="$(__dw_pick "$@")" || return
  else
    # No picker: resolve the arguments the way dw always has, and stop short of
    # cd'ing when that leaves more than one candidate.
    sel="$(command dw "$@")" || return
    if [ -z "$sel" ]; then
      printf 'dw: no topics yet - create one with: dw <name>\n' >&2
      return 0
    fi
    case "$sel" in
      *$'\n'*) printf '%s\n' "$sel" | cut -f2-; return 0 ;;
    esac
  fi
  [ -n "$sel" ] || return 0
  # Re-resolve the chosen path so it becomes the most recent visit (SaveLast
  # only fires for a single match — see internal/topic.Resolve).
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
	fmt.Fprintf(stdout, usageTemplate, topic.Root())
	return 0
}
