// Package summarize folds finished Claude Code turns into a workspace's
// STATE.md in the background. `dw hook` (Claude Code's Stop hook) spawns a
// detached `dw summarize`, which waits out a debounce, takes a per-workspace
// lock, sends the current STATE.md plus a scrubbed transcript slice to a
// cheap LLM, validates the reply, backs up the old file and swaps the new one
// in with a compare-and-swap write. See docs/DESIGN-rework.md.
package summarize

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// Backend names, as accepted by the backend setting.
const (
	BackendAuto   = "auto"
	BackendOpenAI = "openai"
	BackendClaude = "claude"
	BackendOff    = "off"
)

// Config is the summarizer configuration. Load fills it from, highest
// precedence first: environment variables, the JSON config file, defaults.
type Config struct {
	Backend       string
	BaseURL       string
	APIKey        string
	Model         string
	ClaudeCommand string
	ClaudeModel   string
	Timeout       time.Duration
	MaxInputBytes int
	Debounce      time.Duration

	// Warnings are non-fatal problems found while loading (e.g. an api_key in
	// a group- or world-readable file). Callers log them.
	Warnings []string
}

// fileConfig mirrors the config file. Durations are strings for
// time.ParseDuration; an empty or absent field means "not set".
type fileConfig struct {
	Backend       string `json:"backend"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	Model         string `json:"model"`
	ClaudeCommand string `json:"claude_command"`
	ClaudeModel   string `json:"claude_model"`
	Timeout       string `json:"timeout"`
	MaxInputBytes int    `json:"max_input_bytes"`
	Debounce      string `json:"debounce"`
}

func defaults() Config {
	return Config{
		Backend:       BackendAuto,
		ClaudeCommand: "claude",
		ClaudeModel:   "haiku",
		Timeout:       120 * time.Second,
		MaxInputBytes: 64 << 10,
		Debounce:      20 * time.Second,
	}
}

// ConfigPath is $DW_CONFIG, else <user config dir>/dw/config.json.
func ConfigPath() string {
	if p := os.Getenv("DW_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "dw", "config.json")
}

// Load resolves the configuration. A missing default config file is fine; a
// missing $DW_CONFIG, unknown keys or unparsable values are errors, so typos
// surface instead of being silently ignored. OPENAI_* variables are never
// read: a key exported for another tool must not start billing per turn.
func Load() (Config, error) {
	c := defaults()
	if err := c.loadFile(ConfigPath()); err != nil {
		return c, err
	}
	if err := c.loadEnv(); err != nil {
		return c, err
	}
	switch c.Backend {
	case BackendAuto, BackendOpenAI, BackendClaude, BackendOff:
	default:
		return c, fmt.Errorf("unknown backend %q (want auto, openai, claude or off)", c.Backend)
	}
	if c.Timeout <= 0 || c.MaxInputBytes <= 0 || c.Debounce < 0 {
		return c, errors.New("timeout and max_input_bytes must be positive, debounce not negative")
	}
	return c, nil
}

func (c *Config) loadFile(path string) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) && os.Getenv("DW_CONFIG") == "" {
		return nil
	}
	if err != nil {
		return err
	}
	var f fileConfig
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if f.APIKey != "" {
		if fi, err := os.Stat(path); err == nil && fi.Mode().Perm()&0o077 != 0 {
			c.Warnings = append(c.Warnings, fmt.Sprintf("%s holds api_key but is readable by others (mode %v); chmod 600 it", path, fi.Mode().Perm()))
		}
	}
	setString(&c.Backend, f.Backend)
	setString(&c.BaseURL, f.BaseURL)
	setString(&c.APIKey, f.APIKey)
	setString(&c.Model, f.Model)
	setString(&c.ClaudeCommand, f.ClaudeCommand)
	setString(&c.ClaudeModel, f.ClaudeModel)
	if f.MaxInputBytes != 0 {
		c.MaxInputBytes = f.MaxInputBytes
	}
	if err := setDuration(&c.Timeout, f.Timeout, "timeout"); err != nil {
		return err
	}
	return setDuration(&c.Debounce, f.Debounce, "debounce")
}

func (c *Config) loadEnv() error {
	setString(&c.Backend, os.Getenv("DW_SUMMARY_BACKEND"))
	setString(&c.BaseURL, os.Getenv("DW_LLM_BASE_URL"))
	setString(&c.APIKey, os.Getenv("DW_LLM_API_KEY"))
	setString(&c.Model, os.Getenv("DW_LLM_MODEL"))
	setString(&c.ClaudeCommand, os.Getenv("DW_CLAUDE_COMMAND"))
	setString(&c.ClaudeModel, os.Getenv("DW_CLAUDE_MODEL"))
	if v := os.Getenv("DW_SUMMARY_MAX_INPUT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("DW_SUMMARY_MAX_INPUT: %w", err)
		}
		c.MaxInputBytes = n
	}
	if err := setDuration(&c.Timeout, os.Getenv("DW_SUMMARY_TIMEOUT"), "DW_SUMMARY_TIMEOUT"); err != nil {
		return err
	}
	return setDuration(&c.Debounce, os.Getenv("DW_SUMMARY_DEBOUNCE"), "DW_SUMMARY_DEBOUNCE")
}

// setString overrides *dst with v unless v is empty: an empty value counts as
// unset, the same way Root() treats DW_ROOT.
func setString(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

func setDuration(dst *time.Duration, v, name string) error {
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = d
	return nil
}

// Selection is the concrete backend a Config resolves to.
type Selection struct {
	Backend string // BackendOpenAI, BackendClaude or BackendOff
	Reason  string // why it is off; empty otherwise
}

// lookPath is exec.LookPath, swappable for tests.
var lookPath = exec.LookPath

// Select resolves Backend "auto" and checks an explicit backend has what it
// needs. Under auto, a model plus either an API key or a base URL means the
// HTTP backend (which then requires the base URL — there is no default
// endpoint); otherwise `claude` on PATH; otherwise off. An explicit backend
// with missing pieces is an error, never a silent fallback.
func (c Config) Select() (Selection, error) {
	switch c.Backend {
	case BackendOff:
		return Selection{Backend: BackendOff, Reason: "disabled by config"}, nil
	case BackendOpenAI:
		return c.selectOpenAI()
	case BackendClaude:
		if _, err := lookPath(c.ClaudeCommand); err != nil {
			return Selection{}, fmt.Errorf("backend claude: %w", err)
		}
		return Selection{Backend: BackendClaude}, nil
	}
	if c.Model != "" && (c.APIKey != "" || c.BaseURL != "") {
		return c.selectOpenAI()
	}
	if _, err := lookPath(c.ClaudeCommand); err == nil {
		return Selection{Backend: BackendClaude}, nil
	}
	return Selection{Backend: BackendOff, Reason: fmt.Sprintf("no model configured and %q not on PATH", c.ClaudeCommand)}, nil
}

func (c Config) selectOpenAI() (Selection, error) {
	if c.BaseURL == "" || c.Model == "" {
		return Selection{}, errors.New("backend openai needs both base_url and model (there is no default endpoint)")
	}
	return Selection{Backend: BackendOpenAI}, nil
}

// Describe is the one-line summary `dw help` prints after "Summarizer:".
func Describe(c Config, err error) string {
	if err != nil {
		return "error (" + err.Error() + ")"
	}
	sel, err := c.Select()
	switch {
	case err != nil:
		return "error (" + err.Error() + ")"
	case sel.Backend == BackendOpenAI:
		return fmt.Sprintf("openai-compatible (model=%s at %s)", c.Model, c.BaseURL)
	case sel.Backend == BackendClaude:
		return fmt.Sprintf("%s -p (model=%s)", c.ClaudeCommand, c.ClaudeModel)
	default:
		return "off (" + sel.Reason + ")"
	}
}
