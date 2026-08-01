package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTemplate(t *testing.T) {
	tmplDir := filepath.Join(t.TempDir(), "templates")

	// 3. nothing on disk -> built-in DefaultTemplate
	if got := ResolveTemplate(tmplDir, "research"); got != DefaultTemplate {
		t.Fatalf("empty dir should fall back to DefaultTemplate")
	}

	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 2. <dir>/default.md is the shared fallback
	if err := os.WriteFile(filepath.Join(tmplDir, "default.md"), []byte("DEFAULT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveTemplate(tmplDir, "research"); got != "DEFAULT" {
		t.Fatalf("should use templates/default.md, got %q", got)
	}
	// 1. <dir>/<category>.md takes precedence
	if err := os.WriteFile(filepath.Join(tmplDir, "research.md"), []byte("RESEARCH"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveTemplate(tmplDir, "research"); got != "RESEARCH" {
		t.Fatalf("should use templates/research.md, got %q", got)
	}
	// category without a dedicated file falls back to default
	if got := ResolveTemplate(tmplDir, "incident"); got != "DEFAULT" {
		t.Fatalf("uncovered category should use default, got %q", got)
	}
}

func TestResolveClaudeTemplate(t *testing.T) {
	tmplDir := filepath.Join(t.TempDir(), "templates")

	// 3. nothing on disk -> the built-in for that category
	if got := ResolveClaudeTemplate(tmplDir, "research"); got != DefaultClaudeTemplate("research") {
		t.Fatalf("empty dir should fall back to DefaultClaudeTemplate")
	}

	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 2. <dir>/default.CLAUDE.md is the shared fallback
	if err := os.WriteFile(filepath.Join(tmplDir, "default.CLAUDE.md"), []byte("DEFAULT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveClaudeTemplate(tmplDir, "research"); got != "DEFAULT" {
		t.Fatalf("should use templates/default.CLAUDE.md, got %q", got)
	}
	// 1. <dir>/<category>.CLAUDE.md takes precedence
	if err := os.WriteFile(filepath.Join(tmplDir, "research.CLAUDE.md"), []byte("RESEARCH"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveClaudeTemplate(tmplDir, "research"); got != "RESEARCH" {
		t.Fatalf("should use templates/research.CLAUDE.md, got %q", got)
	}
	// category without a dedicated file falls back to default
	if got := ResolveClaudeTemplate(tmplDir, "incident"); got != "DEFAULT" {
		t.Fatalf("uncovered category should use default, got %q", got)
	}
	// the README template lives beside it and must not be picked up
	if err := os.WriteFile(filepath.Join(tmplDir, "incident.md"), []byte("README-ONLY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveClaudeTemplate(tmplDir, "incident"); got != "DEFAULT" {
		t.Fatalf("<category>.md must not satisfy the CLAUDE.md lookup, got %q", got)
	}
}

func TestResolveTemplates(t *testing.T) {
	tmplDir := filepath.Join(t.TempDir(), "templates")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "research.md"), []byte("README"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "research.CLAUDE.md"), []byte("CLAUDE"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveTemplates(tmplDir, "research")
	if got.README != "README" || got.ClaudeMD != "CLAUDE" {
		t.Errorf("got %+v", got)
	}
}

func TestDefaultClaudeTemplateVariesByCategory(t *testing.T) {
	seen := map[string]string{}
	for _, c := range DefaultCategories {
		got := DefaultClaudeTemplate(c)
		if prev, dup := seen[got]; dup {
			t.Errorf("categories %q and %q share the same built-in CLAUDE.md", prev, c)
		}
		seen[got] = c
	}
	// a category the user invented has no dedicated entry: generic fallback
	generic := DefaultClaudeTemplate("woodworking")
	if _, isPerCategory := seen[generic]; isPerCategory {
		t.Errorf("unknown category got a per-category philosophy:\n%s", generic)
	}
	if !strings.Contains(generic, genericClaudePhilosophy) {
		t.Errorf("unknown category should use genericClaudePhilosophy, got:\n%s", generic)
	}
}

func TestDefaultClaudeTemplateRenders(t *testing.T) {
	out := RenderTemplate(DefaultClaudeTemplate("research"), "my-topic", "research", "2026-06-14")
	if !strings.HasPrefix(out, "# my-topic\n") {
		t.Errorf("title not substituted:\n%s", out)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("placeholder left unrendered:\n%s", out)
	}
	// CLAUDE.md is instructions for Claude Code, not a README clone
	if strings.HasPrefix(out, "---") {
		t.Errorf("CLAUDE.md should not carry frontmatter:\n%s", out)
	}
}
