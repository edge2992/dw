package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTemplate(t *testing.T) {
	base := t.TempDir()
	tmplDir := filepath.Join(base, "templates")
	legacy := filepath.Join(base, "template.md")

	// 4. nothing on disk -> built-in DefaultTemplate
	if got := resolveTemplate(tmplDir, legacy, "research"); got != DefaultTemplate {
		t.Fatalf("empty dir should fall back to DefaultTemplate")
	}

	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 3. legacy template.md only
	if err := os.WriteFile(legacy, []byte("LEGACY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTemplate(tmplDir, legacy, "research"); got != "LEGACY" {
		t.Fatalf("should use legacy path, got %q", got)
	}
	// 2. templates/default.md overrides legacy
	if err := os.WriteFile(filepath.Join(tmplDir, "default.md"), []byte("DEFAULT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTemplate(tmplDir, legacy, "research"); got != "DEFAULT" {
		t.Fatalf("should use templates/default.md, got %q", got)
	}
	// 1. templates/<category>.md takes precedence
	if err := os.WriteFile(filepath.Join(tmplDir, "research.md"), []byte("RESEARCH"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTemplate(tmplDir, legacy, "research"); got != "RESEARCH" {
		t.Fatalf("should use templates/research.md, got %q", got)
	}
	// category without a dedicated file falls back to default
	if got := resolveTemplate(tmplDir, legacy, "incident"); got != "DEFAULT" {
		t.Fatalf("uncovered category should use default, got %q", got)
	}
}
