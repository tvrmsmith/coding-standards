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

// Placement is what became of one candidate path. ADR 0004 ignores both
// NotOnDisk and OutsideRoot rather than failing on either, but they are
// different evidence: a candidate that resolved somewhere outside the root
// says the report was built against another working tree, while one that did
// not resolve at all says only that the file has moved on since the test run.
type Placement int

const (
	// NotOnDisk is a candidate filepath.EvalSymlinks could not read, or one
	// that is not absolute and so names nothing to read.
	NotOnDisk Placement = iota
	// OutsideRoot is a candidate that resolved to a real file, elsewhere.
	OutsideRoot
	// InsideRoot is a candidate that resolved under the root, and is the only
	// placement carrying a Path.
	InsideRoot
)

// Place resolves an absolute candidate path and says where it landed. The
// Path is meaningful only for InsideRoot, since a path outside the root has
// no repo-relative spelling.
func (r Root) Place(candidate string) (Path, Placement) {
	if !filepath.IsAbs(candidate) {
		return "", NotOnDisk
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", NotOnDisk
	}
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil {
		return "", OutsideRoot
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", OutsideRoot
	}
	return Path(filepath.ToSlash(rel)), InsideRoot
}

// Resolve turns an absolute candidate path into a source path. It reports
// false when the candidate does not resolve or resolves outside the root;
// ADR 0004 makes both of those "not the gate's business" rather than fatal,
// because a report describes a moment in the past.
func (r Root) Resolve(candidate string) (Path, bool) {
	path, placement := r.Place(candidate)
	return path, placement == InsideRoot
}

// FromSlash adopts an already repo-relative, slash-separated path, which is
// the form `git diff` emits.
func FromSlash(rel string) Path { return Path(rel) }

// ResolveOrAsBuilt resolves candidate's symlinks when it can, and returns it
// as built otherwise, slash-separated either way so a diagnostic reads the
// same on every platform. It exists for a diagnostic that wants "the best
// available reading of this path" without reaching for filepath.EvalSymlinks
// itself; issue 16's coverage_outside_repo message is the one caller.
func ResolveOrAsBuilt(candidate string) string {
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		return filepath.ToSlash(resolved)
	}
	return filepath.ToSlash(candidate)
}
