// Command dw is an interactive picker for Claude discussion/research workspaces.
//
// It scans the workspace root (from ~/.config/dw/config.yml, default ~/dw) for
// projects laid out as <category>/<YYYY-MM-DD>-<topic>/, shows a fuzzy list, and
// prints the selected or newly-created project path to stdout. A thin shell
// wrapper cd's into it.
//
// Subcommands: `dw -` jumps to the last workspace, `dw new` creates one
// non-interactively, `dw list` lists workspaces, `dw root` prints the root,
// `dw config` manages the config file, `dw init` prints the shell wrapper,
// `dw version` prints the version, and `dw help` shows usage.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/edge2992/dw/internal/config"
	"github.com/edge2992/dw/internal/tui"
	"github.com/edge2992/dw/internal/workspace"

	tea "github.com/charmbracelet/bubbletea"
)

// version is the build version, injected via -ldflags at release time
// (see .goreleaser.yaml). Plain `go install`/`go build` leave it as "dev",
// in which case cmdVersion falls back to the module version from the build info.
var version = "dev"

const usage = `dw — discussion workspace picker

Usage:
  dw                       Open the interactive picker (fuzzy list + create-on-demand)
  dw -                     Jump to the last workspace (prints its path)
  dw new <topic> -c <cat>  Create a workspace non-interactively (prints its path)
  dw list [--json]         List workspaces (category/name, or JSON)
  dw root                  Print the workspace root
  dw config <path|init>    Print the config path, or write a starter config
  dw init <zsh|bash>       Print the shell wrapper that cd's into chosen paths
  dw version               Print the version
  dw help                  Show this help

Configuration:
  Settings live in ~/.config/dw/config.yml (run 'dw config init' to scaffold it).
  Keys (all optional, with built-in defaults): root, templates_dir, categories.
  DW_CONFIG   Override the config file location (path only)

dw prints the chosen path to stdout; cd is done by a shell wrapper.
Enable it once with:  eval "$(dw init zsh)"   (or bash)
`

func main() { os.Exit(run(os.Args, os.Stdout, os.Stderr, time.Now())) }

// run dispatches argv to a subcommand and returns the process exit code.
// argv[0] is the program name; argv[1] is the subcommand (or "-", or absent).
func run(argv []string, stdout, stderr io.Writer, now time.Time) int {
	// Subcommands that never read the workspace config are dispatched first, so a
	// malformed ~/.config/dw/config.yml can't break `dw help`, the `dw init` shell
	// wrapper, or the `dw config` commands you'd reach for to repair it.
	if len(argv) >= 2 {
		switch argv[1] {
		case "-":
			return cmdJump(stdout, stderr)
		case "config":
			return cmdConfig(stdout, stderr, argv[2:])
		case "init":
			return cmdInit(stdout, stderr, argv[2:])
		case "version", "--version", "-v":
			return cmdVersion(stdout)
		case "help", "--help", "-h":
			return cmdHelp(stdout)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "dw: config:", err)
		return 1
	}
	if len(argv) < 2 {
		return runTUI(cfg, stdout, stderr, now) // bare `dw`
	}
	switch argv[1] {
	case "new":
		return cmdNew(cfg, stdout, stderr, argv[2:], now)
	case "list":
		return cmdList(cfg, stdout, stderr, argv[2:])
	case "root":
		return cmdRoot(cfg, stdout)
	default:
		fmt.Fprintf(stderr, "dw: unknown command %q\nRun 'dw help' for usage.\n", argv[1])
		return 2
	}
}

// runTUI scans the root, runs the interactive picker, prints the chosen path,
// and remembers it for next time. The UI renders to stderr so stdout carries
// only the chosen path.
func runTUI(cfg config.Config, stdout, stderr io.Writer, now time.Time) int {
	projects, err := workspace.Scan(cfg.Root)
	if err != nil {
		fmt.Fprintln(stderr, "dw: scan:", err)
		return 1
	}

	categories := workspace.Categories(cfg.Categories, projects)
	model := tui.New(cfg.Root, now, projects, workspace.LastPath(), categories, cfg.TemplatesDir)
	// Render the UI to stderr so stdout carries only the chosen path.
	p := tea.NewProgram(model, tea.WithOutput(stderr))
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(stderr, "dw:", err)
		return 1
	}

	fm, ok := final.(tui.Model)
	if !ok {
		fmt.Fprintln(stderr, "dw: unexpected model type")
		return 1
	}
	if fm.Err != nil {
		fmt.Fprintln(stderr, "dw:", fm.Err)
		return 1
	}
	if fm.Result == "" {
		return 1 // aborted: no cd
	}
	// Remember the choice so `dw -` and the startup pin can resume it next time.
	_ = workspace.SaveLast(fm.Result)
	fmt.Fprintln(stdout, fm.Result)
	return 0
}

// cmdJump prints the last chosen workspace without opening the UI (`dw -`).
func cmdJump(stdout, stderr io.Writer) int {
	last := workspace.LastPath()
	if last == "" {
		fmt.Fprintln(stderr, "dw: no previous workspace")
		return 1
	}
	fmt.Fprintln(stdout, last)
	return 0
}

// cmdNew creates a workspace non-interactively and prints its path, so the
// shell wrapper can cd into it (`dw new <topic> --category <cat>`). It is the
// scriptable counterpart of the picker's create-on-demand flow, sharing the
// same workspace.Create core. We hand-parse args (instead of flag.FlagSet) so
// the topic and -c/--category can appear in any order.
func cmdNew(cfg config.Config, stdout, stderr io.Writer, args []string, now time.Time) int {
	const usage = "Usage: dw new <topic> --category <cat>"
	var category string
	var topicParts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--category" || a == "-c":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "dw new: %s needs a value\n%s\n", a, usage)
				return 2
			}
			i++
			category = args[i]
		case strings.HasPrefix(a, "--category="):
			category = strings.TrimPrefix(a, "--category=")
		case strings.HasPrefix(a, "-c="):
			category = strings.TrimPrefix(a, "-c=")
		default:
			topicParts = append(topicParts, a)
		}
	}
	topic := strings.TrimSpace(strings.Join(topicParts, " "))
	if topic == "" {
		fmt.Fprintf(stderr, "dw new: missing topic\n%s\n", usage)
		return 2
	}
	category = strings.TrimSpace(category)
	if category == "" {
		fmt.Fprintf(stderr, "dw new: --category is required\n%s\n", usage)
		return 2
	}
	// Match the picker, which only offers create when the value yields a
	// non-empty slug — so the two creation paths accept the same inputs.
	if workspace.Slugify(topic) == "" {
		fmt.Fprintf(stderr, "dw new: topic %q has no letters or digits to slugify\n%s\n", topic, usage)
		return 2
	}
	catSlug := workspace.Slugify(category)
	if catSlug == "" {
		fmt.Fprintf(stderr, "dw new: category %q has no letters or digits to slugify\n%s\n", category, usage)
		return 2
	}
	// The picker slugifies a new category name before creating it; do the same
	// so `dw new -c "My Cat"` and the picker both land in my-cat/, never two
	// directories for the same logical category.
	category = catSlug
	tmpl := workspace.ResolveTemplate(cfg.TemplatesDir, category)
	p, err := workspace.Create(cfg.Root, category, topic, now, tmpl)
	if err != nil {
		fmt.Fprintln(stderr, "dw:", err)
		return 1
	}
	// Remember the choice so `dw -` and the startup pin can resume it next time.
	_ = workspace.SaveLast(p.Path)
	fmt.Fprintln(stdout, p.Path)
	return 0
}

// shellInit is the wrapper function dw prints from `dw init`. It captures the
// path dw emits for the path-returning subcommands (bare dw, "-", new) and cd's into
// it; every other subcommand passes through untouched. zsh and bash share this
// POSIX-compatible body.
const shellInit = `dw() {
  case "${1:-}" in
    ''|-|new)
      local dir
      dir="$(command dw "$@")" && [ -n "$dir" ] && cd "$dir" ;;
    *)
      command dw "$@" ;;
  esac
}
`

// cmdInit prints the shell wrapper for the requested shell (`dw init zsh|bash`),
// so users can `eval "$(dw init zsh)"` instead of hand-copying it.
func cmdInit(stdout, stderr io.Writer, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "dw init: specify a shell\nUsage: dw init <zsh|bash>")
		return 2
	}
	switch args[0] {
	case "zsh", "bash":
		fmt.Fprint(stdout, shellInit)
		return 0
	default:
		fmt.Fprintf(stderr, "dw init: unsupported shell %q (supported: zsh, bash)\n", args[0])
		return 2
	}
}

// cmdList prints every workspace as "category/name" lines, or as JSON with --json.
func cmdList(cfg config.Config, stdout, stderr io.Writer, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "dw list: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	projects, err := workspace.Scan(cfg.Root)
	if err != nil {
		fmt.Fprintln(stderr, "dw: scan:", err)
		return 1
	}
	if *asJSON {
		if projects == nil {
			projects = []workspace.Project{} // emit "[]" rather than "null"
		}
		b, err := json.MarshalIndent(projects, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, "dw:", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	for _, p := range projects {
		fmt.Fprintln(stdout, p.Category+"/"+p.Name) // literal "/" keeps output pipe-stable
	}
	return 0
}

// cmdRoot prints the resolved workspace root (`dw root`).
func cmdRoot(cfg config.Config, stdout io.Writer) int {
	fmt.Fprintln(stdout, cfg.Root)
	return 0
}

// cmdConfig manages the config file (`dw config path|init`). `path` prints the
// resolved config location; `init` writes a starter config there, refusing to
// clobber an existing file so a hand-edited config is never lost.
func cmdConfig(stdout, stderr io.Writer, args []string) int {
	const usage = "Usage: dw config <path|init>"
	if len(args) != 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "path":
		fmt.Fprintln(stdout, config.Path())
		return 0
	case "init":
		p := config.Path()
		switch _, err := os.Stat(p); {
		case err == nil:
			fmt.Fprintf(stderr, "dw config: %s already exists, leaving it untouched\n", p)
			fmt.Fprintln(stdout, p)
			return 0
		case !os.IsNotExist(err):
			fmt.Fprintln(stderr, "dw config:", err)
			return 1
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintln(stderr, "dw config:", err)
			return 1
		}
		if err := os.WriteFile(p, config.DefaultYAML(), 0o644); err != nil {
			fmt.Fprintln(stderr, "dw config:", err)
			return 1
		}
		fmt.Fprintln(stdout, p)
		return 0
	default:
		fmt.Fprintf(stderr, "dw config: unknown subcommand %q\n%s\n", args[0], usage)
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

// cmdHelp prints the usage text (`dw help`).
func cmdHelp(stdout io.Writer) int {
	fmt.Fprint(stdout, usage)
	return 0
}
