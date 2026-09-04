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

// Placed is one candidate's resolution, kept whole so a caller that needs the
// absolute path a candidate landed on does not resolve symlinks a second time.
// Only landing inside the root is a distinction the gate acts on: ADR 0004
// ignores one candidate that does not, whatever the reason, and fails a whole
// report that never lands.
type Placed struct {
	// Inside reports whether the candidate landed under the root.
	Inside bool
	// Path is the repo-relative path, set only when Inside.
	Path Path
	// Resolved is the absolute slash-separated path the candidate landed on,
	// symlink-resolved when it resolves and as built when it does not. It is
	// what a diagnostic quotes, so it is the best available reading of the path
	// the gate compared.
	Resolved string
}

// Place resolves an absolute candidate path and says where it landed. A
// candidate that is not absolute names nothing to read, and resolving it
// against the process working directory would place a report's own relative
// filename inside the root by accident.
func (r Root) Place(candidate string) Placed {
	asBuilt := Placed{Resolved: filepath.ToSlash(candidate)}
	if !filepath.IsAbs(candidate) {
		return asBuilt
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return asBuilt
	}
	outside := Placed{Resolved: filepath.ToSlash(resolved)}
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil {
		return outside
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return outside
	}
	return Placed{Inside: true, Path: Path(filepath.ToSlash(rel)), Resolved: filepath.ToSlash(resolved)}
}

// FromSlash adopts an already repo-relative, slash-separated path, which is
// the form `git diff` emits.
func FromSlash(rel string) Path { return Path(rel) }
