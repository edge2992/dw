// Package config loads dw's settings from ~/.config/dw/config.yml.
//
// Every key is optional: a missing file, or a file omitting some keys, falls
// back to built-in defaults so dw keeps working with zero configuration. The
// $DW_CONFIG environment variable overrides only the file location (not its
// values), which keeps tests and advanced setups hermetic.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/edge2992/dw/internal/workspace"

	"gopkg.in/yaml.v3"
)

// Config holds dw's resolved settings. The YAML keys are all optional; Resolve
// fills empty fields with built-in defaults and expands ~ / $ENV in the paths.
type Config struct {
	Root         string   `yaml:"root"`          // workspace root
	TemplatesDir string   `yaml:"templates_dir"` // per-category template dir
	Categories   []string `yaml:"categories"`    // picker categories, in order
}

// Path returns the config file location: $DW_CONFIG when set, else
// ~/.config/dw/config.yml. UserConfigDir is deliberately avoided so the path
// stays ~/.config/dw on macOS too, matching where dw keeps templates.
func Path() string {
	if p := os.Getenv("DW_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(configHome(), "dw", "config.yml")
}

// Load reads and resolves the config. A missing file is not an error: it yields
// the all-defaults config.
func Load() (Config, error) {
	var c Config
	b, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return c.Resolve(), nil
		}
		return Config{}, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	return c.Resolve(), nil
}

// Built-in default path values, in their ~-relative display form. These are the
// single source of truth: Resolve expands them at runtime and DefaultYAML writes
// them verbatim into the starter config, so the two can't drift.
const (
	defaultRoot         = "~/dw"
	defaultTemplatesDir = "~/.config/dw/templates"
)

// Resolve fills empty fields with built-in defaults and expands ~ and $ENV in
// the user-supplied path fields. Defaults are expanded with ~ only (never $ENV),
// so a home path containing a literal '$' can't corrupt them. Categories falls
// back to the built-in set when the key is absent or empty.
func (c Config) Resolve() Config {
	c.Root = resolvePath(c.Root, defaultRoot)
	c.TemplatesDir = resolvePath(c.TemplatesDir, defaultTemplatesDir)
	if len(c.Categories) == 0 {
		c.Categories = append([]string(nil), workspace.DefaultCategories...)
	}
	return c
}

// resolvePath returns the user value expanded (~ and $ENV), or the built-in
// default expanded with ~ only — defaults are trusted and skip $ENV expansion.
func resolvePath(v, def string) string {
	if v == "" {
		return expandTilde(def)
	}
	return expandPath(v)
}

// DefaultYAML is the starter config written by `dw config init`. Every key is
// set to its built-in default, so an untouched file behaves exactly like having
// no file at all — editing it is how you opt into overrides.
func DefaultYAML() []byte {
	var cats strings.Builder
	for _, c := range workspace.DefaultCategories {
		fmt.Fprintf(&cats, "  - %s\n", c)
	}
	return fmt.Appendf(nil, `# dw configuration — every key is optional; omitted keys use built-in defaults.

# Workspace root scanned for <category>/<YYYY-MM-DD>-<topic>/ projects.
# ~ and $ENV are expanded. Default: %[1]s
root: %[1]s

# Directory searched for per-category templates (<category>.md, then default.md).
# ~ and $ENV are expanded. Default: %[2]s
templates_dir: %[2]s

# Categories offered in the picker, in order. Replaces the built-in set entirely.
# Default: the built-in categories listed below.
categories:
%[3]s`, defaultRoot, defaultTemplatesDir, cats.String())
}

// configHome returns ~/.config, where dw keeps its config.
func configHome() string {
	return filepath.Join(homeDir(), ".config")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// expandTilde expands a leading ~ or ~/ to the home directory; other input is
// returned unchanged.
func expandTilde(p string) string {
	switch {
	case p == "~":
		return homeDir()
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}

// expandPath expands a leading ~ to the home directory, then expands $ENV
// references (e.g. $HOME, ${XDG_DATA_HOME}).
func expandPath(p string) string {
	return os.ExpandEnv(expandTilde(p))
}
