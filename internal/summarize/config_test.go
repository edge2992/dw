package summarize

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var configEnv = []string{
	"DW_CONFIG", "DW_SUMMARY_BACKEND", "DW_LLM_BASE_URL", "DW_LLM_API_KEY", "DW_LLM_MODEL",
	"DW_CLAUDE_COMMAND", "DW_CLAUDE_MODEL", "DW_SUMMARY_TIMEOUT", "DW_SUMMARY_MAX_INPUT",
	"DW_SUMMARY_DEBOUNCE", "DW_SUMMARIZER",
}

// isolate points HOME (and so the cache and config dirs) at a fresh temp dir
// and clears every setting the summarizer reads.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	for _, k := range configEnv {
		t.Setenv(k, "")
	}
}

// writeConfig writes a config file and points DW_CONFIG at it.
func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DW_CONFIG", path)
	return path
}

// pathHas makes lookPath find exactly the given commands.
func pathHas(t *testing.T, cmds ...string) {
	t.Helper()
	saved := lookPath
	lookPath = func(name string) (string, error) {
		for _, c := range cmds {
			if c == name {
				return "/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = saved })
}

func TestLoad_Defaults(t *testing.T) {
	isolate(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := defaults()
	if c.Backend != want.Backend || c.BaseURL != "" || c.Model != "" || c.APIKey != "" ||
		c.ClaudeCommand != "claude" || c.ClaudeModel != "haiku" || c.Timeout != 120*time.Second ||
		c.MaxInputBytes != 65536 || c.Debounce != 20*time.Second {
		t.Errorf("defaults = %+v", c)
	}
}

func TestLoad_EnvBeatsFileBeatsDefault(t *testing.T) {
	isolate(t)
	writeConfig(t, `{"base_url":"http://file/v1","model":"file-model","timeout":"30s","debounce":"5s","max_input_bytes":1000,"claude_model":"sonnet"}`, 0o600)
	t.Setenv("DW_LLM_MODEL", "env-model")
	t.Setenv("DW_SUMMARY_DEBOUNCE", "0s")
	t.Setenv("DW_LLM_BASE_URL", "") // empty env counts as unset

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Model != "env-model" {
		t.Errorf("model = %q, want env to win", c.Model)
	}
	if c.BaseURL != "http://file/v1" || c.Timeout != 30*time.Second || c.MaxInputBytes != 1000 || c.ClaudeModel != "sonnet" {
		t.Errorf("file values not applied: %+v", c)
	}
	if c.Debounce != 0 {
		t.Errorf("debounce = %v, want env 0s", c.Debounce)
	}
}

func TestLoad_Errors(t *testing.T) {
	cases := map[string]func(t *testing.T){
		"unknown key": func(t *testing.T) { writeConfig(t, `{"modle":"x"}`, 0o600) },
		"missing DW_CONFIG": func(t *testing.T) {
			t.Setenv("DW_CONFIG", filepath.Join(t.TempDir(), "nope.json"))
		},
		"bad duration":      func(t *testing.T) { t.Setenv("DW_SUMMARY_TIMEOUT", "soon") },
		"bad backend":       func(t *testing.T) { t.Setenv("DW_SUMMARY_BACKEND", "gpt") },
		"bad max input":     func(t *testing.T) { t.Setenv("DW_SUMMARY_MAX_INPUT", "lots") },
		"zero max input":    func(t *testing.T) { t.Setenv("DW_SUMMARY_MAX_INPUT", "0") },
		"negative debounce": func(t *testing.T) { t.Setenv("DW_SUMMARY_DEBOUNCE", "-1s") },
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			arrange(t)
			if _, err := Load(); err == nil {
				t.Error("Load succeeded, want an error")
			}
		})
	}
}

func TestLoad_IgnoresOpenAIEnv(t *testing.T) {
	isolate(t)
	t.Setenv("OPENAI_API_KEY", "sk-should-not-be-read")
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "" || c.BaseURL != "" {
		t.Errorf("OPENAI_* leaked into config: %+v", c)
	}
}

func TestLoad_WarnsOnReadableKey(t *testing.T) {
	isolate(t)
	writeConfig(t, `{"api_key":"sk-x"}`, 0o644)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "readable") {
		t.Errorf("warnings = %q, want one about the file mode", c.Warnings)
	}

	writeConfig(t, `{"api_key":"sk-x"}`, 0o600)
	if c, _ := Load(); len(c.Warnings) != 0 {
		t.Errorf("warnings = %q for a 0600 file", c.Warnings)
	}
}

func TestSelect(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		path    []string
		want    string
		wantErr bool
	}{
		{"nothing configured, claude on PATH", Config{Backend: "auto", ClaudeCommand: "claude"}, []string{"claude"}, BackendClaude, false},
		{"nothing configured, no claude", Config{Backend: "auto", ClaudeCommand: "claude"}, nil, BackendOff, false},
		{"model without key or url", Config{Backend: "auto", Model: "m", ClaudeCommand: "claude"}, []string{"claude"}, BackendClaude, false},
		{"keyless local server", Config{Backend: "auto", Model: "m", BaseURL: "http://localhost:11434/v1"}, nil, BackendOpenAI, false},
		{"key, model and url", Config{Backend: "auto", Model: "m", APIKey: "k", BaseURL: "https://x/v1"}, nil, BackendOpenAI, false},
		{"key and model, no url", Config{Backend: "auto", Model: "m", APIKey: "k"}, []string{"claude"}, "", true},
		{"url without model", Config{Backend: "auto", BaseURL: "https://x/v1", ClaudeCommand: "claude"}, []string{"claude"}, BackendClaude, false},
		{"explicit openai incomplete", Config{Backend: "openai", Model: "m"}, nil, "", true},
		{"explicit claude missing", Config{Backend: "claude", ClaudeCommand: "claude"}, nil, "", true},
		{"explicit off", Config{Backend: "off"}, []string{"claude"}, BackendOff, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pathHas(t, c.path...)
			sel, err := c.cfg.Select()
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if sel.Backend != c.want {
				t.Errorf("backend = %q, want %q", sel.Backend, c.want)
			}
		})
	}
}
