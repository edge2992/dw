package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/edge2992/dw/internal/workspace"
)

func TestPathDefaultAndOverride(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")
	t.Setenv("DW_CONFIG", "")
	if got, want := Path(), filepath.Join("/tmp/home", ".config", "dw", "config.yml"); got != want {
		t.Errorf("default Path = %q, want %q", got, want)
	}
	t.Setenv("DW_CONFIG", "/custom/where.yml")
	if got := Path(); got != "/custom/where.yml" {
		t.Errorf("override Path = %q, want /custom/where.yml", got)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DW_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.yml"))

	c, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	if want := filepath.Join(home, "dw"); c.Root != want {
		t.Errorf("Root = %q, want %q", c.Root, want)
	}
	if want := filepath.Join(home, ".config", "dw", "templates"); c.TemplatesDir != want {
		t.Errorf("TemplatesDir = %q, want %q", c.TemplatesDir, want)
	}
	if !reflect.DeepEqual(c.Categories, workspace.DefaultCategories) {
		t.Errorf("Categories = %v, want defaults %v", c.Categories, workspace.DefaultCategories)
	}
}

func TestLoadExpandsPathsAndReplacesCategories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DWTEST_BASE", "/srv/dwbase")
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	body := "" +
		"root: ~/work/dw\n" +
		"templates_dir: $DWTEST_BASE/tmpl\n" +
		"categories:\n  - foo\n  - bar\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DW_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := filepath.Join(home, "work", "dw"); c.Root != want {
		t.Errorf("Root = %q, want %q (~ expanded)", c.Root, want)
	}
	if c.TemplatesDir != "/srv/dwbase/tmpl" {
		t.Errorf("TemplatesDir = %q, want /srv/dwbase/tmpl ($ENV expanded)", c.TemplatesDir)
	}
	if !reflect.DeepEqual(c.Categories, []string{"foo", "bar"}) {
		t.Errorf("Categories = %v, want [foo bar] (full replace)", c.Categories)
	}
}

func TestLoadInvalidYAMLErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(cfgPath, []byte("root: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DW_CONFIG", cfgPath)
	if _, err := Load(); err == nil {
		t.Error("expected an error for malformed YAML")
	}
}

func TestExpandPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")
	t.Setenv("FOO", "/srv/foo")
	cases := map[string]string{
		"":          "",
		"~":         "/tmp/home",
		"~/dw":      "/tmp/home/dw",
		"$FOO/x":    "/srv/foo/x",
		"${FOO}/y":  "/srv/foo/y",
		"/abs/path": "/abs/path",
		"~notme/dw": "~notme/dw", // only a leading ~ or ~/ is expanded
	}
	for in, want := range cases {
		if got := expandPath(in); got != want {
			t.Errorf("expandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultYAMLIsValidAndAllDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(cfgPath, DefaultYAML(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DW_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("DefaultYAML should parse: %v", err)
	}
	// The starter file encodes exactly the built-in defaults.
	if want := filepath.Join(home, "dw"); c.Root != want {
		t.Errorf("Root = %q, want %q", c.Root, want)
	}
	if !reflect.DeepEqual(c.Categories, workspace.DefaultCategories) {
		t.Errorf("Categories = %v, want %v", c.Categories, workspace.DefaultCategories)
	}
}
