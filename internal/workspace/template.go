package workspace

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// readmeFrontmatter opens every built-in README template. dw reads these fields
// back out of the file (see readFrontmatter) to drive the picker, so the block is
// shared rather than copied per category: a stray edit in one copy would quietly
// drop that category's projects out of the status/tags display.
const readmeFrontmatter = `---
title: {{title}}
category: {{category}}
created: {{date}}
status: active
tags: []
---

# {{title}}
`

// readmeBodies holds the headings each category's README opens with. The headings
// are themselves the instruction — a session fills in the sections it is handed,
// so this shape decides whether a topic ends up as an analysis or as a pile of
// notes. research leads with the answer rather than the chronology, incident leads
// with impact and keeps an append-only timeline, and scratch stays out of the way.
var readmeBodies = map[string]string{
	"research": `
## Question

<!-- One sentence. What decision does the answer feed? -->

## Answer

<!-- Answer first, with a confidence: high / medium / low, and what it rests on. -->

## Evidence

<!-- Mark each item fact / inference / assumption, and give it a source. -->

## So what

## What would change this

<!-- The observation that would overturn the answer, and whether you looked for it. -->

## Open / not verified

## Log

## Next
`,

	"incident": `
## Impact

<!-- Who is affected, since when, how badly. -->

## Status

## Timeline

<!-- Append only. Timestamp, then what was observed or done, in the order it happened. -->

## Observations / hypotheses / actions

<!-- Keep the three apart. -->

## Contributing factors

<!-- Not "the root cause": what had to line up, and what let it. -->

## Follow-ups
`,

	"discussion": `
## Question

## Decided

## Open

## Rejected, and what would reopen it

## Notes
`,

	"scratch": `
## Goal

## Tried

<!-- Keep the failures. A dead end recorded is a result. -->

## Keep, or move where
`,
}

// genericReadmeBody is the fallback for categories without a dedicated entry
// above — typically ones the user invented on the fly.
const genericReadmeBody = `
## Question

<!-- One sentence, and what decision it feeds. -->

## Answer

<!-- Answer first, with a confidence. -->

## Evidence

<!-- Mark each item fact / inference / assumption, and give it a source. -->

## Open / not verified

## Next
`

// claudePhilosophies holds the built-in CLAUDE.md guidance, a few lines per
// category. The point of giving every topic its own directory is that different
// kinds of work call for different habits, so research, incident, discussion and
// scratch each get their own stance rather than one generic preamble.
//
// This is deliberately only the stance. How to investigate at all — effort tiers,
// hypotheses, sourcing, when to stop — lives in .claude/rules/investigation.md,
// which every workspace also carries. Long instruction files get half-ignored, so
// the two are kept apart: shared method there, category-specific judgement here.
var claudePhilosophies = map[string]string{
	"research": `Default to the Standard tier; go Deep only when the answer drives an irreversible decision.
Keep what you verified and what you inferred visibly apart, and source both.
Land the conclusion and the next step in README.md; leave the trail in the log.`,

	"incident": `Recovery comes first. The permanent fix is a separate job, done later.
Append to the timeline; never rewrite what is already recorded.
Keep observations, hypotheses and actions taken apart.
Reconstruct each decision from what was known at the time, not from the outcome.
Complex systems rarely have one root cause: record the contributing factors and what let them line up.`,

	"discussion": `State the question before writing the discussion.
Keep decided and undecided strictly apart.
Record the rejected options and what would justify revisiting them.
Put the strongest version of the view you disagree with on the page before dismissing it.`,

	"scratch": `Quick tier by default: speed of trial beats form.
Keep the failed attempts. A dead end recorded is a result.
When it starts to matter, move it to a real category and write it up properly.`,
}

// genericClaudePhilosophy is the fallback for categories without a dedicated
// entry above — typically ones the user invented on the fly.
const genericClaudePhilosophy = `Say in one line what this directory is for and what decision it feeds.
Keep fact, inference and assumption apart, and give each its source.
Land the conclusion and the next step in README.md.`

// DefaultReadmeTemplate returns the built-in README.md for a category: the shared
// frontmatter block plus that category's headings, or the generic ones for a
// category dw does not know about.
func DefaultReadmeTemplate(category string) string {
	body, ok := readmeBodies[category]
	if !ok {
		body = genericReadmeBody
	}
	return readmeFrontmatter + body
}

// ResolveTemplate picks the README template for a category from templatesDir using
// the convention-based search order, falling back to the built-in one:
//  1. <templatesDir>/<category>.md    (category-specific)
//  2. <templatesDir>/default.md       (shared default)
//  3. DefaultReadmeTemplate(category) (built-in, varies by category)
func ResolveTemplate(templatesDir, category string) string {
	for _, p := range []string{
		filepath.Join(templatesDir, category+".md"),
		filepath.Join(templatesDir, "default.md"),
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return DefaultReadmeTemplate(category)
}

// DefaultClaudeTemplate returns the built-in CLAUDE.md for a category.
// Deliberately terse and frontmatter-free: this file is read by Claude Code as
// instructions, and nothing in dw parses it (readFrontmatter only ever looks at
// README.md).
func DefaultClaudeTemplate(category string) string {
	body, ok := claudePhilosophies[category]
	if !ok {
		body = genericClaudePhilosophy
	}
	return "# {{title}}\n\n" + body + "\n"
}

// ResolveClaudeTemplate picks the CLAUDE.md template for a category using the
// same convention as ResolveTemplate, one extension over:
//  1. <templatesDir>/<category>.CLAUDE.md  (category-specific)
//  2. <templatesDir>/default.CLAUDE.md     (shared default)
//  3. DefaultClaudeTemplate(category)      (built-in, varies by category)
func ResolveClaudeTemplate(templatesDir, category string) string {
	for _, p := range []string{
		filepath.Join(templatesDir, category+".CLAUDE.md"),
		filepath.Join(templatesDir, "default.CLAUDE.md"),
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return DefaultClaudeTemplate(category)
}

// Templates bundles the templates for the files Create scaffolds. It exists so
// callers name what they pass: two bare string arguments could be swapped
// silently, writing the CLAUDE.md body into README.md.
type Templates struct {
	README   string
	ClaudeMD string
}

// ResolveTemplates resolves every template a new workspace needs for category.
// See ResolveTemplate and ResolveClaudeTemplate for the individual search orders.
func ResolveTemplates(templatesDir, category string) Templates {
	return Templates{
		README:   ResolveTemplate(templatesDir, category),
		ClaudeMD: ResolveClaudeTemplate(templatesDir, category),
	}
}

// RenderTemplate fills the {{title}}/{{category}}/{{date}} placeholders.
func RenderTemplate(tmpl, title, category, date string) string {
	r := strings.NewReplacer(
		"{{title}}", title,
		"{{category}}", category,
		"{{date}}", date,
	)
	return r.Replace(tmpl)
}

// frontmatter holds the fields dw reads from a project's README frontmatter.
type frontmatter struct {
	title   string
	status  string
	tags    string // raw value, e.g. "[gpu, linux]"
	created string
}

// readFrontmatter parses the leading YAML frontmatter of a project's README in
// a single pass. Missing fields come back as "".
func readFrontmatter(dir string) frontmatter {
	var fm frontmatter
	f, err := os.Open(filepath.Join(dir, "README.md"))
	if err != nil {
		return fm
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inFront := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "---" {
			if !inFront {
				inFront = true
				continue
			}
			break // end of frontmatter
		}
		if !inFront {
			continue
		}
		switch {
		case strings.HasPrefix(line, "title:"):
			fm.title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		case strings.HasPrefix(line, "status:"):
			fm.status = strings.TrimSpace(strings.TrimPrefix(line, "status:"))
		case strings.HasPrefix(line, "tags:"):
			fm.tags = strings.TrimSpace(strings.TrimPrefix(line, "tags:"))
		case strings.HasPrefix(line, "created:"):
			fm.created = strings.TrimSpace(strings.TrimPrefix(line, "created:"))
		}
	}
	return fm
}
