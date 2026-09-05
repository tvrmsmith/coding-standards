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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// UnresolvedError is a name --files gave that the gate refuses to place on a
// source file. It carries the name as the developer typed it, because the
// message reaches the document verbatim and that is the only spelling they can
// act on.
//
// The type exists so a caller can tell a refusal about the path from a failure
// that is not about any path, a process whose working directory was deleted for
// instance. Both come back from NamedFiles, and only the first is ADR 0005's
// file_unresolved, whose message the reader expects to name a path.
type UnresolvedError struct {
	// Name is the path as the developer typed it.
	Name string
	// Reason completes the sentence after Name, as in "does not exist".
	Reason string
}

func (e *UnresolvedError) Error() string { return e.Name + " " + e.Reason }

// NamedFiles resolves every path --files named, in the order they were typed,
// with each file listed once however many spellings named it.
//
// The identity is the resolved repo-relative path, which is the currency every
// later stage keys on. `a.cs`, `./a.cs`, an absolute spelling and a symlink to
// the file all come out of named as the same text, and spelledAsOnDisk forces
// the tree's own case on top, so one text comparison covers every way of
// writing one file. Handed the same file twice the extractor reports every span
// in it twice, and the run exits 1 accusing the extractor of a contract
// violation over a typo.
//
// Two tracked paths that are hard links to one inode stay two files, because
// they are two paths the extractor reads and two paths a coverage report is
// keyed by.
func (r Root) NamedFiles(names []string) ([]Path, error) {
	paths := make([]Path, 0, len(names))
	seen := make(map[Path]bool, len(names))
	for _, name := range names {
		path, err := r.named(name)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths, nil
}

// named resolves a path a human typed on --files. ADR 0004 resolves a
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
// A refusal says "does not exist" only when the filesystem said the path is
// not there. A parent directory the process cannot enter, a symlink cycle or a
// component that is not a directory carry what the operating system said
// instead, since sending the developer after a typo that is not there is the
// misdiagnosis NoBaseError.Unrelated was added to avoid. All of them are still
// UnresolvedError, so every refusal about the path reaches the document under
// one code. Losing the working directory is not about the path at all, so it
// travels as a plain error.
func (r Root) named(name string) (Path, error) {
	candidate := filepath.FromSlash(name)
	if !filepath.IsAbs(candidate) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolving %s against the working directory: %w", name, err)
		}
		candidate = filepath.Join(cwd, candidate)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", &UnresolvedError{Name: name, Reason: "does not exist"}
		}
		return "", unreadable(name, err)
	}
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &UnresolvedError{Name: name, Reason: "is outside the repo root"}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", &UnresolvedError{Name: name, Reason: "does not exist"}
		}
		return "", unreadable(name, err)
	}
	if !info.Mode().IsRegular() {
		return "", &UnresolvedError{Name: name, Reason: "is a directory, not a file"}
	}
	spelled, err := r.spelledAsOnDisk(rel)
	if err != nil {
		return "", unreadable(name, err)
	}
	if !spelled {
		return "", &UnresolvedError{Name: name, Reason: "is not spelled as the file on disk is"}
	}
	return Path(filepath.ToSlash(rel)), nil
}

// unreadable is the refusal for a filesystem failure that is not the path
// being absent, ENOTDIR from a name typed through a file, ELOOP from a symlink
// cycle, or EACCES on a parent directory.
//
// The reason carries the errno's own words rather than the whole fs.PathError,
// because that error quotes an absolute path the developer never typed and
// which reads differently on every machine, and the name they did type already
// opens the message.
func unreadable(name string, err error) *UnresolvedError {
	cause := err
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		cause = pathErr.Err
	}
	return &UnresolvedError{Name: name, Reason: "could not be read, " + cause.Error()}
}

// spelledAsOnDisk reports whether every component of rel is spelled the way the
// directory holding it spells it.
//
// This only bites on a case-insensitive filesystem, APFS or NTFS, where
// EvalSymlinks hands back the case the caller typed rather than the case on
// disk. The gate would then hand the extractor `src/ordering/Order.cs` while
// coverage, placed through Place, carries the tracked `src/Ordering/Order.cs`,
// join would match neither against the other, and every method in a fully
// covered file would come back unknown and fail the run. ADR 0004 fails a path
// the gate cannot place rather than matching it approximately, so a spelling the
// tree does not use is refused instead.
//
// The walk reads directories rather than comparing case-folded text, because
// folding would also merge two files a case-sensitive filesystem keeps apart.
func (r Root) spelledAsOnDisk(rel string) (bool, error) {
	dir := r.resolved
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false, err
		}
		if !slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == component }) {
			return false, nil
		}
		dir = filepath.Join(dir, component)
	}
	return true, nil
}
