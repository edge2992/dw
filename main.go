// Command dw is an interactive picker for Claude discussion/research workspaces.
//
// It scans $DW_ROOT (default ~/dw) for projects laid out as
// <category>/<YYYY-MM-DD>-<topic>/, shows a fuzzy list, and prints the selected
// or newly-created project path to stdout. A thin shell wrapper cd's into it.
//
// Subcommands: `dw -` jumps to the last workspace, `dw list` lists workspaces,
// `dw root` prints the root, `dw version` prints the version, `dw help` shows usage.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/edge2992/dw/internal/tui"
	"github.com/edge2992/dw/internal/workspace"

	tea "github.com/charmbracelet/bubbletea"
)

const usage = `dw — discussion workspace picker

Usage:
  dw                Open the interactive picker (fuzzy list + create-on-demand)
  dw -              Jump to the last workspace (prints its path)
  dw list [--json]  List workspaces (category/name, or JSON)
  dw root           Print the workspace root
  dw version        Print the version
  dw help           Show this help

Environment:
  DW_ROOT   Workspace root (default ~/dw)

dw prints the chosen path to stdout; cd is done by a shell wrapper (see README).
`

func main() { os.Exit(run(os.Args, os.Stdout, os.Stderr, time.Now())) }

// run dispatches argv to a subcommand and returns the process exit code.
// argv[0] is the program name; argv[1] is the subcommand (or "-", or absent).
func run(argv []string, stdout, stderr io.Writer, now time.Time) int {
	if len(argv) < 2 {
		return runTUI(stdout, stderr, now) // bare `dw`
	}
	switch argv[1] {
	case "-":
		return cmdJump(stdout, stderr)
	case "list":
		return cmdList(stdout, stderr, argv[2:])
	case "root":
		return cmdRoot(stdout)
	case "version", "--version", "-v":
		return cmdVersion(stdout)
	case "help", "--help", "-h":
		return cmdHelp(stdout)
	default:
		fmt.Fprintf(stderr, "dw: unknown command %q\nRun 'dw help' for usage.\n", argv[1])
		return 2
	}
}

// runTUI scans the root, runs the interactive picker, prints the chosen path,
// and remembers it for next time. The UI renders to stderr so stdout carries
// only the chosen path.
func runTUI(stdout, stderr io.Writer, now time.Time) int {
	root := workspace.Root()
	projects, err := workspace.Scan(root)
	if err != nil {
		fmt.Fprintln(stderr, "dw: scan:", err)
		return 1
	}
	tmpl := workspace.LoadTemplate(workspace.TemplatePath())

	model := tui.New(root, tmpl, now, projects, workspace.LastPath())
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

// cmdList prints every workspace as "category/name" lines, or as JSON with --json.
func cmdList(stdout, stderr io.Writer, args []string) int {
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
	projects, err := workspace.Scan(workspace.Root())
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
func cmdRoot(stdout io.Writer) int {
	fmt.Fprintln(stdout, workspace.Root())
	return 0
}

// cmdVersion prints the build version (`dw version`).
func cmdVersion(stdout io.Writer) int {
	v := "dev"
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		v = bi.Main.Version // populated by `go install module@version`
	}
	fmt.Fprintln(stdout, "dw", v)
	return 0
}

// cmdHelp prints the usage text (`dw help`).
func cmdHelp(stdout io.Writer) int {
	fmt.Fprint(stdout, usage)
	return 0
}
