// Package srcpath owns the gate's one path currency (ADR 0004): a
// repo-relative, slash-separated path from `git rev-parse --show-toplevel`.
// Every conversion that produces a Path lives here, so the invariant is
// enforced once rather than separately in extract and coverage.
//
// Two sites in coverage render a report name as a display string rather than a
// Path, and neither goes through Place. namedAs is a second owner of the
// containment decision, because it has to name a path that may not exist on
// disk and Place requires a regular file. relative is a third spelling of the
// repo-relative rendering with no escape guard at all, correct only because its
// callers hand it paths the walk already found under the root. Issue 36 unifies
// all three behind an existence-agnostic sibling of Place.
package srcpath

import (
	"fmt"
	"os"
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
// The fields are unexported because a repo-relative path only means anything
// when the candidate landed, and Inside hands the caller both at once so the
// compiler holds that pairing rather than a doc comment.
type Placed struct {
	inside   bool
	path     Path
	resolved string
}

// Inside is the repo-relative path the candidate landed on, and whether it
// landed under the root at all. The path is empty when it did not.
func (p Placed) Inside() (Path, bool) { return p.path, p.inside }

// Resolved is the slash-separated reading of the candidate, symlink-resolved
// and absolute when it resolves and as built when it does not, which for a
// candidate that was never absolute is the relative text itself. It is what a
// diagnostic quotes, so it is the best available reading of the path the gate
// compared.
func (p Placed) Resolved() string { return p.resolved }

// Place resolves an absolute candidate path and says where it landed. A
// candidate that is not absolute names nothing to read, and resolving it
// against the process working directory would place a report's own relative
// filename inside the root by accident. A candidate that resolves to anything
// other than a regular file lands nowhere either: a class filename of "../.."
// or of a bare directory name joins onto an in-root source root to name a
// directory, which is inside the root without naming any source the report
// measured, and one such class would otherwise stand in for a whole report's
// worth of classes that placed nothing.
func (r Root) Place(candidate string) Placed {
	placed := Placed{resolved: filepath.ToSlash(candidate)}
	if !filepath.IsAbs(candidate) {
		return placed
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return placed
	}
	placed.resolved = filepath.ToSlash(resolved)
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return placed
	}
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil {
		return placed
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return placed
	}
	placed.inside = true
	placed.path = Path(filepath.ToSlash(rel))
	return placed
}

// FromSlash adopts an already repo-relative, slash-separated path, which is
// the form `git diff` emits.
func FromSlash(rel string) Path { return Path(rel) }

// Named resolves a path a human typed on --files. ADR 0004 resolves a
// relative name against the process working directory and then relativizes
// it to the root, and makes a path the gate cannot place inside the repo exit
// 1 rather than be matched approximately. An absolute name already says where
// it is, so joining it onto the working directory would name a path nobody
// typed and report a file that exists as missing.
//
// A name that resolves to anything other than a regular file is refused, the
// same rule Place applies, because no extractor claims a directory: the gate
// would measure nothing and exit 0 pass over a tree the developer believes
// they gated.
//
// The three errors name the path as the human typed it, name, not as the gate
// resolved it, because that message reaches the document verbatim and a
// resolved absolute path would tell the reader nothing about what they typed.
func (r Root) Named(name string) (Path, error) {
	candidate := filepath.FromSlash(name)
	if !filepath.IsAbs(candidate) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		candidate = filepath.Join(cwd, candidate)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("%s does not exist", name)
	}
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the repo root", name)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%s does not exist", name)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is a directory, not a file", name)
	}
	return Path(filepath.ToSlash(rel)), nil
}
