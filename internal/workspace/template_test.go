package workspace

import (
	"os"
	"path/filepath"
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
