// Package srcpath owns the gate's one path currency (ADR 0004): a
// repo-relative, slash-separated path from `git rev-parse --show-toplevel`.
// Every conversion that produces a Path lives here, so the invariant is
// enforced once rather than separately in extract and coverage.
//
// One containment predicate serves the package, relativize, and Place and Name
// are the two entry points onto it (issue 36).
//
// They differ in what has to be on disk. Place refuses a candidate that is not
// a regular file, so that a coverage class filename of "../.." or of a bare
// directory name cannot stand in for a report's worth of classes that placed
// nothing. Name has to name a path that may be nothing at all, a --coverage
// report the gate could not read among them, so it resolves as far as the path
// exists and answers a display string rather than a Path, because a path
// outside the repo has no repo-relative form and must not be mistaken for one.
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
	rel, place, err := r.relativize(resolved)
	if err != nil || place == outside {
		return placed
	}
	placed.inside = true
	placed.path = rel
	return placed
}

// placement is where a candidate sits with respect to the root.
type placement int

const (
	// outside is above the root, or beside it under a name that only looks like
	// the root's.
	outside placement = iota
	// inside is under the root, spelled as the root is spelled.
	inside
	// folded is under the root with the root prefix spelled in another case.
	folded
)

func (p placement) String() string {
	switch p {
	case inside:
		return "inside"
	case folded:
		return "folded"
	default:
		return "outside"
	}
}

// relativize reads an already resolved absolute path as a path under the root,
// and says where it landed. Place, Name and named all ask, and each decides for
// itself what a candidate that landed above the root means, so the one
// definition of "outside" lives here rather than being spelled three times and
// drifting.
//
// The error is the root and the candidate having no relative reading at all, a
// second drive letter on Windows rather than a location above the root. It is a
// separate answer because it is not a placement, and a caller that reports it
// says so in its own words.
func (r Root) relativize(resolved string) (Path, placement, error) {
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil {
		return "", outside, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return r.foldRootPrefix(resolved)
	}
	return Path(filepath.ToSlash(rel)), inside, nil
}

// foldRootPrefix is the second reading of a candidate filepath.Rel says
// escapes. On a case-insensitive filesystem EvalSymlinks hands back the case
// the developer typed for a component that is not itself a symlink, so with the
// root resolved to `/private/tmp/cf_test`, `/tmp/CF_TEST/sub/a.txt` resolves to
// `/private/tmp/CF_TEST/sub/a.txt` and filepath.Rel reads that as
// `../CF_TEST/sub/a.txt`, outside, when the file is in fact inside (issue 48).
// The developer then goes looking for a location mistake instead of a spelling
// one. This takes the first four components, `/private/tmp/CF_TEST`, folds them
// against `/private/tmp/cf_test`, confirms with os.SameFile that the two names
// reach one directory, and answers folded with `sub/a.txt`.
//
// os.SameFile is what keeps this from being the text folding ADR 0004 rejects.
// Two directories that differ only in case are two inodes on a case-sensitive
// filesystem, so `/tmp/REPO` beside a real `/tmp/repo` still reads as outside
// on Linux, which is the property the rejection protects. EqualFold runs first
// because it is free and because it also stops a bind mount or a hard-linked
// directory, which is one inode under two unrelated names, from folding.
//
// Only the root prefix folds. The components below it come back as the
// candidate spells them, so a case-only difference there is still refused one
// layer up.
func (r Root) foldRootPrefix(resolved string) (Path, placement, error) {
	sep := string(filepath.Separator)
	rootComponents := strings.Split(r.resolved, sep)
	components := strings.Split(resolved, sep)
	if len(components) < len(rootComponents) {
		return "", outside, nil
	}
	prefix := strings.Join(components[:len(rootComponents)], sep)
	if !strings.EqualFold(prefix, r.resolved) || !sameDir(prefix, r.resolved) {
		return "", outside, nil
	}
	below := components[len(rootComponents):]
	if len(below) == 0 {
		return ".", folded, nil
	}
	return Path(strings.Join(below, "/")), folded, nil
}

// sameDir reports whether two names reach one directory. A name it cannot stat
// is not the root under another spelling, since the root is a directory the
// gate has already resolved.
func sameDir(a, b string) bool {
	infoA, err := os.Stat(a)
	if err != nil {
		return false
	}
	infoB, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

// Name is a path as the gate's document names it, repo-relative when it sits
// under the root and the resolved absolute path when it does not. It is a
// display string and not a Path, because a path outside the repo has no
// repo-relative form and must not be mistaken for one.
type Name string

// String renders the name as it appears in the gate's output.
func (n Name) String() string { return string(n) }

// Name places an absolute path that need not exist on disk, which is what a
// failure quoting a --coverage report needs and Place cannot give. Place
// refuses anything that is not a regular file, so that a class filename of
// "../.." or of a bare directory name cannot stand in for a report's worth of
// classes that placed nothing, and the whole point here is to name a path that
// may be nothing at all.
//
// A path that is genuinely outside the repo is named by the resolved absolute
// path the containment test just weighed, not by the developer's own spelling.
// The document carries no working directory (ADR 0005), so a relative name
// reaches a consumer that cannot resolve it, and one report named from two
// directories would print two strings. A folded root prefix is named
// repo-relative, the same as an exact one, since it is the same directory.
//
// A path that is not absolute comes back as its own slash-separated text.
// Resolving it here against the process working directory would place a
// report's own relative filename inside the root by accident, so the caller
// joins its working directory on first, which is what coverage.Named does.
func (r Root) Name(path string) Name {
	if !filepath.IsAbs(path) {
		return Name(filepath.ToSlash(path))
	}
	resolved := resolveExisting(path)
	rel, place, err := r.relativize(resolved)
	if err != nil || place == outside {
		return Name(filepath.ToSlash(resolved))
	}
	return Name(rel)
}

// resolveExisting resolves the symlinks of the deepest ancestor of abs that is
// on disk and rejoins the components below it as they were typed. A path that
// exists whole resolves whole, and one naming nothing still comes back rooted
// where it really sits rather than behind whatever link led there, which
// matters because the repo root is very often reached through a symlink (/tmp
// and /var on macOS) and an unresolved path compared against the resolved root
// escapes for the indirection rather than for where it actually is.
//
// Only a component that is not there is climbed past: a symlink loop or an
// ancestor the process may not search is a real fault, and climbing over it
// would rename a report that does sit inside the repo into one named as though
// it sat outside.
func resolveExisting(abs string) string {
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved
	}
	parent := filepath.Dir(abs)
	if !errors.Is(err, fs.ErrNotExist) || parent == abs {
		return abs
	}
	return filepath.Join(resolveExisting(parent), filepath.Base(abs))
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
	dirs := dirNames{}
	for _, name := range names {
		path, err := r.named(name, dirs)
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
// same rule Place applies, because no extractor claims a directory or a fifo,
// so the gate would measure nothing and exit 0 pass over a tree the developer
// believes they gated. A directory is named as one, since that is the mistake
// a developer actually makes, and every other mode is refused as not a regular
// file rather than being called a directory it is not.
//
// A refusal says "does not exist" only when the filesystem said the path is
// not there. A parent directory the process cannot enter, a symlink cycle or a
// component that is not a directory carry what the operating system said
// instead, since sending the developer after a typo that is not there is the
// misdiagnosis NoBaseError.Unrelated was added to avoid. A path the root cannot
// be relativized against at all, a second drive letter on Windows rather than a
// location above the root, says that in the gate's own words rather than
// carrying filepath.Rel's, which quote two absolute paths the developer never
// typed.
//
// A name whose root prefix is spelled in another case is refused too, and it
// says so rather than reusing the "not spelled as the file on disk is" refusal
// below, because the mis-cased component is the root and not the file and the reader
// has to know which half to retype. Place treats the same path as inside,
// because a coverage report the gate resolves is not something a developer can
// retype. All of them are still
// UnresolvedError, so every refusal about the path reaches the document under
// one code. Losing the working directory is not about the path at all, so it
// travels as a plain error.
func (r Root) named(name string, dirs dirNames) (Path, error) {
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
	rel, place, err := r.relativize(resolved)
	if err != nil {
		return "", &UnresolvedError{Name: name, Reason: "has no path relative to the repo root"}
	}
	switch place {
	case outside:
		return "", &UnresolvedError{Name: name, Reason: "is outside the repo root"}
	case folded:
		return "", &UnresolvedError{Name: name, Reason: "is not spelled as the repo root is"}
	}
	// EvalSymlinks already resolved this path, so absence here is a delete
	// racing the two syscalls rather than a name the developer mistyped, and it
	// reads as the filesystem failure it is.
	info, err := os.Stat(resolved)
	if err != nil {
		return "", unreadable(name, err)
	}
	if info.IsDir() {
		return "", &UnresolvedError{Name: name, Reason: "is a directory, not a file"}
	}
	if !info.Mode().IsRegular() {
		return "", &UnresolvedError{Name: name, Reason: "is not a regular file"}
	}
	spelled, err := r.spelledAsOnDisk(filepath.FromSlash(string(rel)), dirs)
	if err != nil {
		return "", unreadable(name, err)
	}
	if !spelled {
		return "", &UnresolvedError{Name: name, Reason: "is not spelled as the file on disk is"}
	}
	return rel, nil
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
//
// dirs carries the listings already read, so `--files` over a few hundred
// paths in one tree reads each ancestor directory once rather than once per
// path.
func (r Root) spelledAsOnDisk(rel string, dirs dirNames) (bool, error) {
	dir := r.resolved
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		entries, err := dirs.of(dir)
		if err != nil {
			return false, err
		}
		if !slices.Contains(entries, component) {
			return false, nil
		}
		dir = filepath.Join(dir, component)
	}
	return true, nil
}

// dirNames memoizes directory listings for the length of one NamedFiles call,
// keyed by absolute directory path. It holds entry names alone, since that is
// the whole of what the spelling walk compares, and it is deliberately not
// shared across calls: a listing older than the run would answer for a tree
// that has since changed.
type dirNames map[string][]string

// of is the entry names in dir, reading it the first time it is asked for. A
// directory it could not read is not remembered, so a transient failure does
// not answer for every later path under it.
func (d dirNames) of(dir string) ([]string, error) {
	if names, ok := d[dir]; ok {
		return names, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	d[dir] = names
	return names, nil
}
