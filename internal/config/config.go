// Package config loads dw's settings from ~/.config/dw/config.yml.
//
// Every key is optional: a missing file, or a file omitting some keys, falls
// back to built-in defaults so dw keeps working with zero configuration. The
// $DW_CONFIG environment variable overrides only the file location (not its
// values), which keeps tests and advanced setups hermetic.
package config

import (
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

// Resolve fills empty fields with built-in defaults and expands ~ and $ENV in
// the path fields. Categories defaults to workspace.DefaultCategories only when
// the key is absent (nil); an explicit list replaces it wholesale.
func (c Config) Resolve() Config {
	if c.Root == "" {
		c.Root = filepath.Join(homeDir(), "dw")
	}
	c.Root = expandPath(c.Root)
	if c.TemplatesDir == "" {
		c.TemplatesDir = filepath.Join(configHome(), "dw", "templates")
	}
	c.TemplatesDir = expandPath(c.TemplatesDir)
	if c.Categories == nil {
		c.Categories = append([]string(nil), workspace.DefaultCategories...)
	}
	return c
}

// DefaultYAML is the starter config written by `dw config init`. Every key is
// set to its built-in default, so an untouched file behaves exactly like having
// no file at all — editing it is how you opt into overrides.
func DefaultYAML() []byte {
	return []byte(`# dw configuration — every key is optional; omitted keys use built-in defaults.

# Workspace root scanned for <category>/<YYYY-MM-DD>-<topic>/ projects.
# ~ and $ENV are expanded. Default: ~/dw
root: ~/dw

# Directory searched for per-category templates (<category>.md, then default.md).
# ~ and $ENV are expanded. Default: ~/.config/dw/templates
templates_dir: ~/.config/dw/templates

# Categories offered in the picker, in order. Replaces the built-in set entirely.
# Default: [research, incident, discussion, scratch]
categories:
  - research
  - incident
  - discussion
  - scratch
`)
}

// configHome returns ~/.config, where dw keeps its config and templates.
func configHome() string {
	return filepath.Join(homeDir(), ".config")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// expandPath expands a leading ~ to the home directory, then expands $ENV
// references (e.g. $HOME, ${XDG_DATA_HOME}). Empty input stays empty.
func expandPath(p string) string {
	switch {
	case p == "":
		return p
	case p == "~":
		p = homeDir()
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(homeDir(), p[2:])
	}
	return os.ExpandEnv(p)
}
