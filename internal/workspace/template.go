package workspace

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DefaultTemplate is used when no template file is configured.
const DefaultTemplate = `---
title: {{title}}
category: {{category}}
created: {{date}}
status: active
tags: []
---

# {{title}}

## Background / Goal

## Research Log

## Conclusion / Next Actions
`

// claudePhilosophies holds the built-in CLAUDE.md guidance, three lines per
// category. The point of giving every topic its own directory is that different
// kinds of work call for different habits, so research, incident, discussion and
// scratch each get their own stance rather than one generic preamble. The lines
// are Japanese because that is what the workspaces themselves are written in.
var claudePhilosophies = map[string]string{
	"research": `検証した事実と推測を必ず書き分ける。
出典・実行したコマンド・生ログへのリンクを残す。
結論と次の一手を README.md に集約する。`,

	"incident": `時系列を壊さない。既存の記録は書き換えず追記する。
事実・仮説・実施した対処を分けて記録する。
復旧を最優先し、恒久対策は落ち着いてから別立てで考える。`,

	"discussion": `論点を先に立ててから議論を書く。
決まったことと未決のことを必ず分ける。
反対意見とトレードオフも省略せずに残す。`,

	"scratch": `使い捨て前提。体裁より試行の速度を優先する。
失敗した試みも消さずにそのまま残す。
残す価値が出てきたら適切なカテゴリへ移す。`,
}

// genericClaudePhilosophy is the fallback for categories without a dedicated
// entry above — typically ones the user invented on the fly.
const genericClaudePhilosophy = `このディレクトリで何をしたいのかを最初に短く書く。
事実と推測を書き分け、判断の根拠へのリンクを残す。
結論と次の一手を README.md に集約する。`

// ResolveTemplate picks the template for a category from templatesDir using the
// convention-based search order, falling back to the built-in DefaultTemplate:
//  1. <templatesDir>/<category>.md  (category-specific)
//  2. <templatesDir>/default.md     (shared default)
//  3. DefaultTemplate               (built-in)
func ResolveTemplate(templatesDir, category string) string {
	for _, p := range []string{
		filepath.Join(templatesDir, category+".md"),
		filepath.Join(templatesDir, "default.md"),
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return DefaultTemplate
}

// DefaultClaudeTemplate returns the built-in CLAUDE.md for a category. Unlike
// DefaultTemplate it has to be a function, since the guidance varies per
// category. Deliberately terse and frontmatter-free: this file is read by Claude
// Code as instructions, and nothing in dw parses it (readFrontmatter only ever
// looks at README.md).
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
