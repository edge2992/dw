package workspace

import (
	_ "embed"
	"path/filepath"
)

// conventionCLAUDEMD is the root CLAUDE.md every dw root gets once — the
// "Convention" layer from docs/concepts.md. It is dw's own plumbing, not the
// user's document, so it is a fixed Go string embed rather than a
// {{placeholder}} template like STATE.md.
//
//go:embed assets/CLAUDE.md
var conventionCLAUDEMD string

// writeConventionIfAbsent writes the root CLAUDE.md the first time root is
// used, and never touches it again — it belongs to the user from then on.
func writeConventionIfAbsent(root string) error {
	_, err := writeIfAbsent(filepath.Join(root, "CLAUDE.md"), conventionCLAUDEMD, 0o644)
	return err
}
