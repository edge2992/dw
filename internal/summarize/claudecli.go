package summarize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// claudeCLI runs `claude -p` as a pure text-in/text-out call. It exists for
// users with only a Claude subscription, which is why it is not --bare
// (that forces ANTHROPIC_API_KEY auth and never reads OAuth).
type claudeCLI struct {
	command, model string
}

func (c *claudeCLI) Name() string { return "claude/" + c.model }

func (c *claudeCLI) args() []string {
	return []string{
		"-p",
		"--model", c.model,
		"--system-prompt", systemPrompt,
		"--tools", "", // no tools: it cannot touch files
		"--setting-sources", "", // no user/project/local settings, so no hooks or default model leak in
		"--no-session-persistence", // keep summaries out of `claude --resume`
		"--output-format", "text",
	}
}

func (c *claudeCLI) Summarize(ctx context.Context, req Request) (string, error) {
	cmd := exec.CommandContext(ctx, c.command, c.args()...)
	// Never the workspace or root: Claude Code would load <root>/CLAUDE.md and
	// the workspace's Stop hook from there.
	cmd.Dir = os.TempDir()
	// The real recursion guard: any `dw hook` the child still fires exits at once.
	cmd.Env = append(os.Environ(), "DW_SUMMARIZER=1")
	cmd.Stdin = strings.NewReader(userMessage(req))
	var stdout, stderr cappedBuffer
	stdout.limit, stderr.limit = maxResponseBytes, 200
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 5 * time.Second // don't hang on a grandchild holding the pipes

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s: %w", c.command, ctx.Err())
		}
		return "", fmt.Errorf("%s: %w: %s", c.command, err, strings.TrimSpace(stderr.String()))
	}
	if stdout.overflow {
		return "", errors.New("claude output larger than 1 MiB")
	}
	return stdout.String(), nil
}

// cappedBuffer keeps the first limit bytes written to it and drops the rest,
// so a runaway child cannot balloon the worker's memory.
type cappedBuffer struct {
	strings.Builder
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room < len(p) {
		b.overflow = true
		if room > 0 {
			b.Builder.Write(p[:room])
		}
		return len(p), nil
	}
	return b.Builder.Write(p)
}
