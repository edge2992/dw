// Command dw is an interactive picker for Claude discussion/research workspaces.
//
// It scans $DISCUSSION_ROOT (default ~/Discussion) for projects laid out as
// <category>/<YYYY-MM-DD>-<topic>/, shows a fuzzy list, and prints the selected
// or newly-created project path to stdout. A thin shell wrapper cd's into it.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/edge2992/dw/internal/tui"
	"github.com/edge2992/dw/internal/workspace"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// `dw -` jumps straight back to the last chosen workspace, no UI.
	if len(os.Args) > 1 && os.Args[1] == "-" {
		last := workspace.LastPath()
		if last == "" {
			fmt.Fprintln(os.Stderr, "dw: no previous workspace")
			os.Exit(1)
		}
		fmt.Println(last)
		return
	}

	root := workspace.Root()
	projects, err := workspace.Scan(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dw: scan:", err)
		os.Exit(1)
	}
	tmpl := workspace.LoadTemplate(workspace.TemplatePath())

	model := tui.New(root, tmpl, time.Now(), projects, workspace.LastPath())
	// Render the UI to stderr so stdout carries only the chosen path.
	p := tea.NewProgram(model, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dw:", err)
		os.Exit(1)
	}

	fm, ok := final.(tui.Model)
	if !ok {
		fmt.Fprintln(os.Stderr, "dw: unexpected model type")
		os.Exit(1)
	}
	if fm.Err != nil {
		fmt.Fprintln(os.Stderr, "dw:", fm.Err)
		os.Exit(1)
	}
	if fm.Result == "" {
		os.Exit(1) // aborted: no cd
	}
	// Remember the choice so `dw -` and the startup pin can resume it next time.
	_ = workspace.SaveLast(fm.Result)
	fmt.Println(fm.Result)
}
