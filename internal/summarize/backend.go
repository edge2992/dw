package summarize

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Request is one summarization: the current STATE.md and what happened since
// it was last updated.
type Request struct {
	Date       string // today, YYYY-MM-DD
	State      string // current STATE.md, verbatim
	Transcript string // scrubbed, rendered transcript slice
}

// Summarizer turns a Request into the complete new STATE.md.
type Summarizer interface {
	Summarize(ctx context.Context, req Request) (string, error)
	// Name identifies the backend and model in the log, e.g. "claude/haiku".
	Name() string
}

// ErrOff means the configuration resolves to no backend at all.
var ErrOff = errors.New("summarizer is off")

// maxResponseBytes caps what is read back from either backend.
const maxResponseBytes = 1 << 20

// New builds the Summarizer c selects.
func New(c Config) (Summarizer, error) {
	sel, err := c.Select()
	if err != nil {
		return nil, err
	}
	switch sel.Backend {
	case BackendOpenAI:
		return &openAI{
			baseURL: strings.TrimRight(c.BaseURL, "/"),
			apiKey:  c.APIKey,
			model:   c.Model,
			client:  &http.Client{}, // no global timeout: the context drives cancellation
			backoff: []time.Duration{2 * time.Second, 8 * time.Second},
		}, nil
	case BackendClaude:
		return &claudeCLI{command: c.ClaudeCommand, model: c.ClaudeModel}, nil
	}
	return nil, ErrOff
}
