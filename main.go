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
	root := workspace.Root()
	projects, err := workspace.Scan(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dw: scan:", err)
		os.Exit(1)
	}
	tmpl := workspace.LoadTemplate(workspace.TemplatePath())

	model := tui.New(root, tmpl, time.Now(), projects)
	// Render the UI to stderr so stdout carries only the chosen path.
	p := tea.NewProgram(model, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dw:", err)
		os.Exit(1)
	}

	fm := final.(tui.Model)
	if fm.Err != nil {
		fmt.Fprintln(os.Stderr, "dw:", fm.Err)
		os.Exit(1)
	}
	if fm.Result == "" {
		os.Exit(1) // aborted: no cd
	}
	fmt.Println(fm.Result)
}
