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

// Placement is what became of one candidate path. Neither NotOnDisk nor
// OutsideRoot is fatal here, per ADR 0004, but they are different evidence and
// a caller may escalate on one and not the other: a candidate that resolved
// somewhere outside the root says the report was built against another working
// tree, while one that did not resolve at all says only that the file has
// moved on since the test run. coverage.mergeInto fails a report whose classes
// were all OutsideRoot on the strength of that difference.
type Placement int

const (
	// NotOnDisk is a candidate filepath.EvalSymlinks could not read, or one
	// that is not absolute and so names nothing to read.
	NotOnDisk Placement = iota
	// OutsideRoot is a candidate that resolved to a real file, elsewhere.
	OutsideRoot
	// InsideRoot is a candidate that resolved under the root.
	InsideRoot
)

// Placed is one candidate's resolution, kept whole so a caller that needs the
// absolute path a candidate landed on does not resolve symlinks a second time.
type Placed struct {
	Placement Placement
	// Path is the repo-relative path, set only for InsideRoot.
	Path Path
	// Resolved is the absolute slash-separated path the candidate landed on,
	// set for InsideRoot and OutsideRoot and empty for NotOnDisk. It is what a
	// diagnostic quotes, so it is the path the gate actually compared.
	Resolved string
}

// Place resolves an absolute candidate path and says where it landed.
func (r Root) Place(candidate string) Placed {
	if !filepath.IsAbs(candidate) {
		return Placed{Placement: NotOnDisk}
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return Placed{Placement: NotOnDisk}
	}
	outside := Placed{Placement: OutsideRoot, Resolved: filepath.ToSlash(resolved)}
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil {
		return outside
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return outside
	}
	return Placed{Placement: InsideRoot, Path: Path(filepath.ToSlash(rel)), Resolved: filepath.ToSlash(resolved)}
}

// FromSlash adopts an already repo-relative, slash-separated path, which is
// the form `git diff` emits.
func FromSlash(rel string) Path { return Path(rel) }
