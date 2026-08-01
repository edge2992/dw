package workspace

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// claudeAssetsFS holds the .claude/ tree dw drops into every workspace: the
// Stop hook that checkpoints a session, the rule that tells Claude to read that
// checkpoint back, and the settings.json that scopes the hook to this directory
// alone. Unlike README.md and CLAUDE.md these are dw's own plumbing, not the
// user's documents, so they are fixed: no templates_dir override, no
// {{placeholder}} rendering.
//
// They live as real files rather than Go string constants (the convention the
// rest of this package follows) because checkpoint.sh is a shell script full of
// $variables and quoting. Keeping it a real .sh file avoids escaping mistakes
// and lets shellcheck read it.
//
//go:embed assets/dotclaude
var claudeAssetsFS embed.FS

const (
	claudeAssetsRoot = "assets/dotclaude"
	claudeDir        = ".claude"
	// hooksSubdir marks the assets that get the execute bit. embed.FS reports a
	// fixed mode for everything it holds, so the exec bit cannot be carried over
	// from the file on disk and has to be decided here.
	hooksSubdir = "hooks"
)

// claudeAsset is one file from claudeAssetsFS, together with where it lands
// inside a workspace and what mode it needs there.
type claudeAsset struct {
	src  string      // path within claudeAssetsFS
	rel  string      // path relative to the workspace, e.g. ".claude/hooks/checkpoint.sh"
	mode os.FileMode // 0o755 for hooks, 0o644 for the rest
}

// claudeAssetPlan lists every asset in a stable order. fs.WalkDir yields
// directory entries sorted by name, so the order the scaffold reports is the
// same on every run and platform.
func claudeAssetPlan() ([]claudeAsset, error) {
	var plan []claudeAsset
	err := fs.WalkDir(claudeAssetsFS, claudeAssetsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		sub, err := filepath.Rel(claudeAssetsRoot, p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		// path.Dir, not filepath.Dir: p is an io/fs path, always slash-separated.
		if path.Dir(p) == path.Join(claudeAssetsRoot, hooksSubdir) {
			mode = 0o755
		}
		plan = append(plan, claudeAsset{src: p, rel: filepath.Join(claudeDir, sub), mode: mode})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// EnsureClaudeSettings writes the .claude/ assets p is missing and returns the
// paths it wrote, in plan order. Files that already exist are left alone —
// dw owns these three paths, but the user owns their edits to them.
func EnsureClaudeSettings(p Project) ([]string, error) {
	plan, err := claudeAssetPlan()
	if err != nil {
		return nil, err
	}
	var wrote []string
	for _, a := range plan {
		content, err := claudeAssetsFS.ReadFile(a.src)
		if err != nil {
			return nil, err
		}
		dest := filepath.Join(p.Path, a.rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		ok, err := writeIfAbsent(dest, string(content), a.mode)
		if err != nil {
			return nil, err
		}
		if ok {
			wrote = append(wrote, dest)
		}
	}
	return wrote, nil
}

// PendingClaudeSettings returns the paths EnsureClaudeSettings would write,
// without touching the filesystem — the --dry-run counterpart. It walks the
// same plan, so a dry run can never disagree with the real thing.
func PendingClaudeSettings(p Project) ([]string, error) {
	plan, err := claudeAssetPlan()
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, a := range plan {
		dest := filepath.Join(p.Path, a.rel)
		// Lstat, matching writeIfAbsent's O_EXCL: a symlink counts as present
		// even when it dangles, so neither path writes through one.
		if _, err := os.Lstat(dest); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		pending = append(pending, dest)
	}
	return pending, nil
}
