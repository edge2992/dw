package workspace

import (
	"bufio"
	"os"
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

// readTitle extracts the `title:` field from a project's README frontmatter.
func readTitle(dir string) string {
	f, err := os.Open(dir + "/README.md")
	if err != nil {
		return ""
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
		if inFront && strings.HasPrefix(line, "title:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		}
	}
	return ""
}
