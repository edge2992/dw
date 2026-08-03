package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// claudeAssetDests is every path EnsureClaudeSettings is expected to write,
// relative to the workspace. Spelled out here rather than derived from
// claudeAssetPlan, so adding an asset without meaning to fails the tests.
var claudeAssetDests = []string{
	".claude/hooks/checkpoint.sh",
	".claude/rules/dw-workspace.md",
	".claude/rules/investigation.md",
	".claude/settings.json",
}

func TestEnsureClaudeSettingsWritesAllFiles(t *testing.T) {
	dir := t.TempDir()
	p := Project{Path: dir, Category: "research"}

	wrote, err := EnsureClaudeSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrote) != len(claudeAssetDests) {
		t.Fatalf("wrote %v, want %d files", wrote, len(claudeAssetDests))
	}
	for i, rel := range claudeAssetDests {
		want := filepath.Join(dir, rel)
		if wrote[i] != want {
			t.Errorf("wrote[%d] = %q, want %q (order must be deterministic)", i, wrote[i], want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Errorf("%s not written: %v", rel, err)
		}
	}

	// The hook is executed by Claude Code, so it has to be runnable. Check the
	// bit rather than the exact mode: umask can clear group/other.
	fi, err := os.Stat(filepath.Join(dir, ".claude/hooks/checkpoint.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("checkpoint.sh mode = %v, want the owner execute bit set", fi.Mode().Perm())
	}

	// settings.json has to survive Claude Code's parser, and has to point at the
	// hook we just wrote — a typo there fails silently at runtime.
	b, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks struct {
			Stop []struct {
				Hooks []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"Stop"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, b)
	}
	if len(settings.Hooks.Stop) != 1 || len(settings.Hooks.Stop[0].Hooks) != 1 {
		t.Fatalf("settings.json does not register exactly one Stop hook:\n%s", b)
	}
	h := settings.Hooks.Stop[0].Hooks[0]
	if h.Type != "command" {
		t.Errorf("Stop hook type = %q, want %q", h.Type, "command")
	}
	if !strings.Contains(h.Command, ".claude/hooks/checkpoint.sh") {
		t.Errorf("Stop hook command = %q, want it to run checkpoint.sh", h.Command)
	}
}

func TestEnsureClaudeSettingsNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	p := Project{Path: dir, Category: "research"}

	settings := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := `{"hooks": {}} // EDITED BY USER`
	if err := os.WriteFile(settings, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, err := EnsureClaudeSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wrote {
		if w == settings {
			t.Errorf("EnsureClaudeSettings reported writing the existing %q", settings)
		}
	}
	if b, _ := os.ReadFile(settings); string(b) != custom {
		t.Errorf("settings.json was clobbered: %q", string(b))
	}
	// the other assets are still backfilled around it
	if len(wrote) != len(claudeAssetDests)-1 {
		t.Errorf("wrote %v, want the %d assets other than settings.json", wrote, len(claudeAssetDests)-1)
	}
}

func TestPendingClaudeSettingsMatchesEnsure(t *testing.T) {
	dir := t.TempDir()
	p := Project{Path: dir, Category: "research"}

	// --dry-run must predict exactly what a real run writes, so the two share
	// claudeAssetPlan rather than each deciding for themselves.
	pending, err := PendingClaudeSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	wrote, err := EnsureClaudeSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != len(wrote) {
		t.Fatalf("pending = %v, wrote = %v", pending, wrote)
	}
	for i := range pending {
		if pending[i] != wrote[i] {
			t.Errorf("pending[%d] = %q, wrote[%d] = %q", i, pending[i], i, wrote[i])
		}
	}

	// Nothing left to do once they exist, and PendingClaudeSettings must not
	// have created anything on its own.
	pending, err = PendingClaudeSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("pending after scaffolding = %v, want none", pending)
	}
	wrote, err = EnsureClaudeSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrote) != 0 {
		t.Errorf("re-running wrote %v, want none", wrote)
	}
}

func TestEnsureClaudeSettingsDoesNotFollowDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "outside.json")
	if err := os.Symlink(target, filepath.Join(dir, ".claude", "settings.json")); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureClaudeSettings(Project{Path: dir, Category: "research"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("dangling symlink target was written: %v", err)
	}
}
