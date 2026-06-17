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

// ResolveTemplate picks the template for a category using the convention-based
// search order, falling back to the built-in DefaultTemplate.
func ResolveTemplate(category string) string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "discussion", "templates")
	legacy := filepath.Join(home, ".config", "discussion", "template.md")
	return resolveTemplate(dir, legacy, category)
}

// resolveTemplate implements the search order with injectable paths for testing:
//  1. <dir>/<category>.md  (category-specific)
//  2. <dir>/default.md     (shared default)
//  3. legacyPath           (~/.config/discussion/template.md, backward compat)
//  4. DefaultTemplate      (built-in)
func resolveTemplate(dir, legacyPath, category string) string {
	for _, p := range []string{
		filepath.Join(dir, category+".md"),
		filepath.Join(dir, "default.md"),
		legacyPath,
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return DefaultTemplate
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
