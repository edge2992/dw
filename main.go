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
  <absolute path>	<marker + title>	<first open question>
The marker is "* " for the most recently visited workspace, "  " otherwise.
When <topic> matches more than one workspace, every match is printed — dw
never picks one for you; the shell wrapper hands the rows to fzf.

Enable shell integration once with:  eval "$(dw init zsh)"   (or bash)

Root: %s
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
	return 0
}

// printRows writes one TSV row per project (see workspace.Project.TSVRow).
func printRows(w io.Writer, projects []workspace.Project, last string) {
	for _, p := range projects {
		fmt.Fprintln(w, p.TSVRow(last))
	}
}

// shellInit is the function dw prints from `dw init`. It captures the TSV dw
// emits, hands multi-row output to fzf when available (falling back to a
// plain printed list otherwise), re-resolves the chosen absolute path through
// `command dw` (so that becomes the new "last visited" workspace — see
// internal/workspace.Resolve), and cd's into it. zsh and bash share this
// POSIX-compatible body.
const shellInit = `dw() {
  case "${1:-}" in
    init|help|version|--help|-h|--version|-v)
      command dw "$@" ;;
    *)
      local rows
      rows="$(command dw "$@")" || return
      [ -n "$rows" ] || return 0
      if [ "$(printf '%s\n' "$rows" | wc -l)" -gt 1 ]; then
        if command -v fzf >/dev/null 2>&1; then
          rows="$(printf '%s\n' "$rows" | fzf --delimiter=$'\t' --with-nth=2.. \
                    --preview='head -40 {1}/STATE.md' --preview-window=right,60%)" || return
          rows="$(command dw "${rows%%$'\t'*}")" || return
        else
          printf '%s\n' "$rows" | cut -f2-
          return 0
        fi
      fi
      cd "${rows%%$'\t'*}" ;;
  esac
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
