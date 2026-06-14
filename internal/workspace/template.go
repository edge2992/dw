package workspace

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// TemplatePath returns the user's project template path (~/.config/discussion/template.md).
func TemplatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "discussion", "template.md")
}

// DefaultTemplate is used when no template file is configured.
const DefaultTemplate = `---
title: {{title}}
category: {{category}}
created: {{date}}
status: active
tags: []
---

# {{title}}

## 背景 / 目的

## 調査ログ

## 結論 / 次アクション
`

// LoadTemplate reads the template file, falling back to DefaultTemplate.
func LoadTemplate(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return DefaultTemplate
	}
	return string(b)
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
