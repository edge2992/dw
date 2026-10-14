package summarize

import (
	_ "embed"
	"strings"
)

// systemPrompt restates the "STATE.md に書くこと" rules of the root
// CLAUDE.md for the summarizer model.
//
//go:embed assets/prompt.md
var systemPrompt string

// userMessage is the single user turn sent with systemPrompt.
func userMessage(req Request) string {
	var b strings.Builder
	b.WriteString("今日の日付: " + req.Date + "\n\n")
	b.WriteString("<current_state>\n" + req.State + "\n</current_state>\n\n")
	b.WriteString("<transcript>\n" + req.Transcript + "\n</transcript>\n")
	return b.String()
}
