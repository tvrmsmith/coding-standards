// Package srcpath owns the gate's one path currency (ADR 0004): a
// repo-relative, slash-separated path from `git rev-parse --show-toplevel`.
// Every conversion into that form lives here, so the invariant is enforced
// once rather than separately in extract and coverage.
package srcpath

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Path is a repo-relative, slash-separated source path.
type Path string

// String renders the path as it appears in the gate's output.
func (p Path) String() string { return string(p) }

// Ext is the path's file extension, including the dot, returned exactly as the
// path spells it. Extractor routing compares it case-insensitively, in
// extract.worthRunning and extract.filter, so a language table row spelling its
// extensions in lower case still routes `Order.CS`.
func (p Path) Ext() string { return filepath.Ext(string(p)) }

// Root is a repo root with its symlinks already resolved, which is what
// makes relativizing a resolved candidate meaningful.
type Root struct {
	resolved string
}

// NewRoot resolves dir's symlinks and returns it as a Root.
func NewRoot(dir string) (Root, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return Root{}, fmt.Errorf("resolving repo root %s: %w", dir, err)
	}
	return Root{resolved: resolved}, nil
}

// Dir is the resolved absolute repo root, the working directory the gate
// runs git and the extractor in.
func (r Root) Dir() string { return r.resolved }

// Abs joins p back onto the root.
func (r Root) Abs(p Path) string {
	return filepath.Join(r.resolved, filepath.FromSlash(string(p)))
}

// Resolve turns an absolute candidate path into a source path. It reports
// false when the candidate does not resolve or resolves outside the root;
// ADR 0004 makes both of those "not the gate's business" rather than fatal,
// because a report describes a moment in the past.
func (r Root) Resolve(candidate string) (Path, bool) {
	if !filepath.IsAbs(candidate) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return Path(filepath.ToSlash(rel)), true
}

// FromSlash adopts an already repo-relative, slash-separated path, which is
// the form `git diff` emits.
func FromSlash(rel string) Path { return Path(rel) }

// ResolveOrAsBuilt resolves candidate's symlinks when it can, and returns it
// exactly as built otherwise. It exists for a diagnostic that wants "the best
// available reading of this path" without reaching for filepath.EvalSymlinks
// itself; issue 16's coverage_outside_repo message is the one caller.
func ResolveOrAsBuilt(candidate string) string {
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		return resolved
	}
	return candidate
}
