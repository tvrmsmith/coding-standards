// Package srcpath owns the gate's one path currency (ADR 0004): a
// repo-relative, slash-separated path from `git rev-parse --show-toplevel`.
// Every conversion that produces a Path lives here, so the invariant is
// enforced once rather than separately in extract and coverage.
//
// One function, relativize, computes containment, and Place, Name and named are
// the three entry points onto it. All three read its placements the same way: a
// candidate whose root prefix is spelled in another case is under the root, and
// "outside" means one thing at every door (issue 36). ADR 0004's 2026-09-11
// amendment is what authorizes that, on the ground that the folding it rejects
// is folding by text, which on Linux merges two real files, and not a fold
// os.SameFile has confirmed reaches one inode.
//
// They part on relativize's other answer, a filesystem that will not say whether
// a case-differing prefix is the root. named words that one distinctly, as the
// path having "could not be weighed against the repo root", since it already
// answers a developer with a reason. Place and Name read it as outside, because
// neither has a channel to carry a reason: Place answers a bool and Name one
// display string, and the alternatives cost more than the fault is worth, an
// exit with no document at all or a new error code. How near the fault is
// differs between the two, and each door's own doc says which: Place has already
// resolved and statted the candidate by then, so what is left is the root
// itself, while Name resolves as far as the path exists and reaches the fault
// for a report that is not on disk at all. Widening Placed to carry it is issue
// 36 follow-up work.
//
// Landing under the root is not the same as being accepted. named refuses a
// --files path whose own text names the repo root directory in a case the root
// does not use, because such a path is one the developer typed and can retype,
// and telling them which half is misspelled is the whole of issue 48. The rule
// covers every component of the root prefix they spelled, not the root's last
// word alone. The gate walks the prefixes of the path it resolved the name by,
// starting at the first component their own text contributed, and asks the
// filesystem which directory each one reaches, so the word blamed is always one
// they wrote and one the root prefix names. A mis-case the working directory
// supplied, or one a symlink's stored target supplied, is accepted rather than
// blamed on a half that is not in the string and that no retyping of the name
// can clear.
//
// named carries the most user-visible policy of the three, the distinct
// refusal reasons a --files path can come back with, so a reader changing
// containment has to weigh it beside the other two rather than reading Place and
// Name alone.
//
// Place and Name differ in what has to be on disk. Place refuses a candidate that is not
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
	"syscall"
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
//
// A candidate whose root prefix is spelled in another case places where it
// really sits, per ADR 0004's 2026-09-11 amendment. The prefix is confirmed with
// os.SameFile, so the two names reach one directory and the candidate is one
// file, not two merged by text. The components below the root come back as the
// candidate spells them, which is the same thing filepath.Rel already hands back
// for a correctly spelled prefix on a case-insensitive filesystem; Place has
// never verified below-root spelling and does not start here.
//
// A filesystem that will not say whether that prefix is the root lands nowhere,
// the same as a candidate genuinely outside. Placed carries a bool and no reason,
// and ADR 0004 already ignores one candidate that does not land whatever the
// reason, so the class goes unmeasured rather than the run failing on a fault
// only named has the words for. It is narrow: EvalSymlinks and os.Stat on the
// candidate have both succeeded by then, so what is left is the root itself
// moving or becoming unreadable mid-run.
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

// errNoRelativeReading is the root and the candidate sharing no common root at
// all, a second drive letter on Windows. filepath.Rel's own error is dropped
// for it because that text quotes two absolute paths the developer never typed;
// named answers in the gate's words instead.
var errNoRelativeReading = errors.New("no path relative to the repo root")

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
// and says where it landed. Place, Name and named all ask, so the comparison
// against the root is computed once here rather than spelled three times and
// drifting, and all three read the answer the same way, which is the whole of
// issue 36.
//
// inside and folded are both under the root and differ only in how the root
// prefix is spelled. A caller that acts on containment alone can treat them
// alike; named tells them apart because it owes the developer a reason, not
// because they sit in different places.
//
// An error is a question the placement could not be answered at all, and there
// are two of them. errNoRelativeReading is the root and the candidate having no
// relative reading, a second drive letter on Windows rather than a location
// above the root. The other is the filesystem refusing to say whether a
// case-differing prefix is the root, which isRootUnder carries up verbatim. They are
// separate answers because neither is a placement, and named tells them apart to
// pick its words.
func (r Root) relativize(resolved string) (Path, placement, error) {
	rel, err := filepath.Rel(r.resolved, resolved)
	if err != nil {
		return "", outside, errNoRelativeReading
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
// because it is free and because it keeps the fold to spellings of one name: two
// unrelated names for one inode, a bind mount or a hard-linked directory, are
// not case variants of each other and do not fold.
//
// Only the root prefix folds. The components below it come back as the candidate
// spells them, which is exactly what filepath.Rel returns for a correctly
// spelled prefix on the same filesystem, so the folded Path is no weaker than
// the inside one beside it. A candidate that is the root under another spelling
// has no components below and answers ".", again the reading Rel gives for the
// root spelled correctly.
//
// Below-root spelling is a separate question. The fold stops at the root prefix,
// so a candidate mis-cased below it keeps that mis-spelling in the Path it comes
// back as, and the join finds no tracked path matching it: that is what keeps
// `case_only_path_difference` at exit 1, on the coverage side that never reaches
// named. For --files the counterpart is spelledAsOnDisk, which named walks
// against the directory entries and refuses on a mismatch. This function answers
// neither and must not be read as having done so.
func (r Root) foldRootPrefix(resolved string) (Path, placement, error) {
	sep := string(filepath.Separator)
	rootComponents := pathComponents(r.resolved)
	components := pathComponents(resolved)
	if len(components) < len(rootComponents) {
		return "", outside, nil
	}
	prefix := strings.Join(components[:len(rootComponents)], sep)
	if prefix == "" {
		prefix = sep
	}
	if !strings.EqualFold(prefix, r.resolved) {
		return "", outside, nil
	}
	same, err := r.isRootUnder(prefix)
	if err != nil {
		return "", outside, err
	}
	if !same {
		return "", outside, nil
	}
	below := components[len(rootComponents):]
	if len(below) == 0 {
		return ".", folded, nil
	}
	return Path(strings.Join(below, "/")), folded, nil
}

// isRootUnder reports whether spelling reaches the root's own directory. The
// two names are not interchangeable, which is why this hangs off the Root
// rather than comparing two strings: spelling is a candidate's prefix and may
// be nothing at all, which answers false, since a name that is not there is not
// the root under another spelling. The root is a directory the gate resolved at
// startup, so any failure to stat it, including its absence, is a fault that
// travels up.
//
// Every other stat failure on spelling travels up too, EACCES on a parent among
// them. Reading one as two genuinely distinct directories would refuse a path
// that is in fact inside the repo as being outside it, which is the
// misdiagnosis issue 48 set out to remove. Only named acts on that distinction,
// with a refusal worded about the root; Place and Name have no reason to carry
// and read the fault as outside, which their own docs record.
func (r Root) isRootUnder(spelling string) (bool, error) {
	spelled, err := os.Stat(spelling)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	root, err := os.Stat(r.resolved)
	if err != nil {
		return false, err
	}
	return os.SameFile(spelled, root), nil
}

// Name is a path as the gate's document names it, in one of three shapes:
// repo-relative when it sits under the root, the resolved absolute path when it
// does not, and its own slash-separated text when it was handed over without a
// working directory joined on. It is a display string and not a Path, because a
// path outside the repo has no repo-relative form and must not be mistaken for
// one.
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
// An absolute path has the separators it ends in trimmed before anything reads
// it, so a developer who typed a trailing slash gets the name the same path
// without one gets. This is the single entry point a human-typed path arrives
// at, so the normalisation lives here rather than in each caller that builds
// one, and it goes no further than the trailing run for the reason
// trimTrailingSeparators gives.
//
// It answers one of three shapes. A path under the repo root is named
// repo-relative, whether the root prefix is spelled as the root is or in another
// case that os.SameFile confirms reaches the same directory: the report sits
// where it sits, and naming it by where the developer's shell happened to spell
// it would print one report under two strings. A path genuinely outside the repo
// is named by the resolved absolute path the containment test just weighed, not
// by the developer's own spelling, because the document carries no working
// directory (ADR 0005) and a relative name reaches a consumer that cannot
// resolve it.
//
// The third shape is an input that is not absolute, which comes back as its own
// slash-separated text. That is a caller which has not joined its working
// directory on yet, and no caller reaches it today: coverage.Named joins cwd
// before it asks. Resolving it here instead would place a report's own relative
// filename inside the root by accident, which is why the join belongs to the
// caller.
//
// A filesystem that will not say whether a case-differing prefix is the root is
// read as the second shape, the resolved absolute path, the same as a path
// genuinely outside. That is a known limitation and not an oversight. Name
// answers one display string and has no channel to carry a reason, and the two
// ways out both cost more than the fault is worth here: erroring would exit the
// run with no document at all, which ADR 0005's 2026-09-02 amendment rejects for
// a coverage-side filesystem failure, and a new code saying the gate could not
// weigh the path is a change to ADR 0008's list. named is the one door that
// words the fault distinctly, "could not be weighed against the repo root",
// because it already answers a developer with a reason. Place reads it the same
// way as here, but it is further from the fault than Name is: Place has resolved
// and statted the candidate before it asks, where resolveExisting climbs past a
// component that is not there, so a --coverage report that does not exist yet
// under a case-differing prefix whose parent denies search reaches this arm and
// is named absolute. It is the report's own name that suffers, and the run still
// carries a document saying so.
func (r Root) Name(path string) Name {
	if !filepath.IsAbs(path) {
		return Name(filepath.ToSlash(path))
	}
	resolved := resolveExisting(trimTrailingSeparators(path))
	rel, place, err := r.relativize(resolved)
	if err != nil || place == outside {
		return Name(filepath.ToSlash(resolved))
	}
	return Name(rel)
}

// trimTrailingSeparators drops the separators abs ends in, down to but not
// through the volume root, so "<root>/README.md/" asks about the same file
// "<root>/README.md" asks about. Untrimmed, EvalSymlinks answers ENOTDIR for
// the trailing slash on a regular file, resolveExisting climbs, and rejoining
// filepath.Base onto filepath.Dir names the report "README.md/README.md".
//
// Only the trailing run goes. filepath.Clean would also pop ".." against the
// component before it, and that is the one normalisation containment cannot
// afford: with "<root>/link" a symlink out of the repository, the lexical pop
// turns "<root>/link/../x" into "<root>/x" and a report that sits elsewhere is
// named repo-relative, where EvalSymlinks pops the same ".." against the
// resolved link and answers where the file really is. Root.Place resolves
// without cleaning for the same reason, so the two agree about one string.
func trimTrailingSeparators(abs string) string {
	root := len(filepath.VolumeName(abs)) + 1
	trimmed := abs
	for len(trimmed) > root && os.IsPathSeparator(trimmed[len(trimmed)-1]) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}

// resolveExisting resolves the symlinks of the deepest ancestor of abs that is
// on disk and rejoins the components below it as they were typed. A path that
// exists whole resolves whole, and one naming nothing still comes back rooted
// where it really sits rather than behind whatever link led there, which
// matters because the repo root is very often reached through a symlink (/tmp
// and /var on macOS) and an unresolved path compared against the resolved root
// escapes for the indirection rather than for where it actually is.
//
// Only a component with nothing resolvable at that depth is climbed past.
// EvalSymlinks says so two ways, fs.ErrNotExist for a component that is
// absent and ENOTDIR for one that is a file standing where a directory would
// have to be, and a file holds nothing beneath it either, so the two report
// the same thing about the depth and climb alike. A symlink loop or an
// ancestor the process may not search is a real fault instead, not an
// absence, and climbing over either would rename a report that does sit
// inside the repo into one named as though it sat outside.
func resolveExisting(abs string) string {
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved
	}
	parent := filepath.Dir(abs)
	if parent == abs || !nothingResolvableHere(err) {
		return abs
	}
	return filepath.Join(resolveExisting(parent), filepath.Base(abs))
}

// nothingResolvableHere reports whether err is EvalSymlinks saying there is
// nothing to resolve at this depth, an absent component or one that is a
// regular file rather than a directory. The filesystem reports the second as
// ENOTDIR rather than fs.ErrNotExist, since it is a file and not nothing, but
// a file holds nothing beneath it either, so climbing past it is the same
// move as climbing past an absence. A symlink loop or a directory the process
// may not search is a different question, whether the path resolves at all,
// and answers false.
func nothingResolvableHere(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
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
// that is not about any path. Both come back from NamedFiles, and only the
// first is ADR 0008's file_unresolved, whose message the reader expects to
// name a path.
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
//
// cwd is what a relative name resolves against. The caller reads it, once,
// rather than NamedFiles calling os.Getwd itself, the same way coverage.Named
// already takes a cwd; see measure's doc comment for why the read happens
// before this is ever called.
func (r Root) NamedFiles(names []string, cwd string) ([]Path, error) {
	paths := make([]Path, 0, len(names))
	seen := make(map[Path]bool, len(names))
	dirs := dirNames{}
	for _, name := range names {
		path, err := r.named(name, cwd, dirs)
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
// misdiagnosis NoBaseError.Unrelated was added to avoid. A filesystem that
// declines to say whether a case-differing prefix is the root is refused in its
// own words rather than as the name being unreadable, because the name resolved
// a moment earlier and what failed is the root: the gate then knows nothing
// about where the path sits and must not guess at either refusal below. A path
// the root cannot be relativized against at all, a second drive letter on
// Windows rather than a location above the root, says that in the gate's own
// words rather than carrying filepath.Rel's, which quote two absolute paths the
// developer never typed.
//
// A name whose root prefix the developer spelled in another case is refused too,
// and it says so rather than reusing the "not spelled as the file on disk is"
// refusal below, because the mis-cased component is the root and not the file and
// the reader has to know which half to retype. named is the only caller that
// refuses on that reading. Place and Name place and name the same path where it
// really sits, because a coverage candidate is not one a developer typed and
// there is nobody to send back to the keyboard; the fold is confirmed by
// os.SameFile, so it merges no two files (ADR 0004, amended 2026-09-11).
//
// Naming the root is the test, not typing an absolute path. A relative name
// names it too once it climbs above the root and descends back in, since
// --files ../../REPO/src/a.cs typed one directory inside the repo puts REPO in
// the string the developer can retype, and refusing that is the whole point of
// the refusal. What no name of either shape supplies is the working directory
// its climb starts from, or the target text a symlink in it resolves to, and
// refusing for a mis-case there would quote a name and blame a half of it that
// is not in the string they typed and cannot be retyped, no matter how the name
// is written. typedTheRootPrefix separates the two by walking the path named
// built a prefix at a time, from the first component their own text
// contributed, so every word of the root's own name that they spelled is
// weighed and nothing else is. The fold is accepted for the rest, and the name
// goes on to spelledAsOnDisk, which walks every component below the root
// against the tree's own entries, so nothing is matched approximately for
// having taken that road.
//
// That refusal is weighed after the mode checks, so --files naming the repo
// root itself answers "is a directory, not a file" in either case, and a
// developer who retypes the case gets the same message on the second try rather
// than a new one. A mis-cased regular file still reaches the spelling refusal,
// having passed those checks. The outside refusal stays ahead of them, since a
// path above the root has nothing inside the repo to stat.
//
// All of them are still UnresolvedError, so every refusal about the path
// reaches the document under one code. The working directory itself is read
// once by the caller and handed in as cwd, so losing it is caught before named
// ever runs and never reaches here at all.
func (r Root) named(name, cwd string, dirs dirNames) (Path, error) {
	candidate := filepath.FromSlash(name)
	if filepath.IsAbs(candidate) {
		// An absolute name says where it is on its own, so the working
		// directory bears on nothing about it and every component below is the
		// developer's own. Clearing cwd here is what tells firstTypedComponent
		// so, and it is the same thing the caller's read not happening used to
		// say.
		cwd = ""
	} else {
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
	if errors.Is(err, errNoRelativeReading) {
		return "", &UnresolvedError{Name: name, Reason: "has no path relative to the repo root"}
	}
	if err != nil {
		return "", unweighable(name, err)
	}
	if place == outside {
		return "", &UnresolvedError{Name: name, Reason: "is outside the repo root"}
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
	if place == folded && r.typedTheRootPrefix(candidate, firstTypedComponent(cwd, name)) {
		return "", &UnresolvedError{Name: name, Reason: "is not spelled as the repo root is"}
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

// typedTheRootPrefix reports whether the developer's own --files text spells a
// component of the repo root's prefix in a case the root does not use, which is
// what makes the mis-spelling theirs to retype. The root prefix is the whole
// run of directories that names the root, not its last word alone, so
// --files /u/t/DEV/repo/src/a.cs against a root of /u/t/dev/repo is blamed on
// DEV (ADR 0004).
//
// It walks the prefixes of candidate, the lexical path named already built and
// placed the file by, shallowest first, and resolves each one. Walking that
// path rather than the raw typed string is what keeps the two readings of one
// name together: filepath.Join collapsed every ".." against the working
// directory before any link was resolved, and a walk that resolved first and
// popped afterwards would climb out of a symlinked working directory into
// another tree and answer for a path named never weighed. Only components from
// typed onward are the developer's own, so a directory the shell cd'd through
// is never examined and a mis-case up there is never blamed on a string that
// does not carry it.
//
// Every comparison is made on the resolved side, where the component count is
// the root's own and no symlink can shift it. A prefix that reaches anywhere
// other than one of the root's own directories is passed over rather than
// ending the walk, since a name is free to descend, climb back out and name the
// root after that, as "src/../../REPO/src/a.cs" does. Nothing below the root is
// ever weighed here; spelledAsOnDisk holds every component under it to the
// tree's own spelling.
//
// A word is blamed only when the resolved prefix still ends in that same word,
// which says the last hop renamed nothing and the word really is theirs. That
// is what separates a mis-spelling from a different name: a link named "link"
// pointing at the root, or at a mis-cased directory, hands back a tail they
// never wrote, so nothing is blamed and the name stands.
//
// Resolving is what counting cannot do. A symlink anywhere in the text adds or
// drops components, above the root or below it, so no arithmetic over the
// resolved candidate says which word of theirs produced which part of it. A
// prefix the filesystem will not resolve names nothing and is passed over too.
func (r Root) typedTheRootPrefix(candidate string, typed int) bool {
	sep := string(filepath.Separator)
	components := pathComponents(filepath.Clean(candidate))
	rootComponents := pathComponents(r.resolved)
	for i := typed; i < len(components); i++ {
		word := components[i]
		if word == "" {
			continue
		}
		reached, err := filepath.EvalSymlinks(strings.Join(components[:i+1], sep))
		if err != nil {
			continue
		}
		prefix := pathComponents(reached)
		if !namesTheRootPrefix(prefix, rootComponents) {
			continue
		}
		last := len(prefix) - 1
		if prefix[last] == word && word != rootComponents[last] {
			return true
		}
	}
	return false
}

// firstTypedComponent gives the index in named's lexical candidate of the first
// component the developer's own text contributed. An absolute name contributes
// all of them. A relative one contributes everything past the working directory
// less the parents its text climbed, which is exact because filepath.Join
// collapsed those parents against the working directory lexically, one for one.
// The count is taken on that lexical pair alone and never on a resolved path,
// so no symlink can shift it.
func firstTypedComponent(cwd, name string) int {
	if cwd == "" {
		return 0
	}
	typed := pathComponents(filepath.Clean(filepath.FromSlash(name)))
	parents := 0
	for parents < len(typed) && typed[parents] == ".." {
		parents++
	}
	return max(len(pathComponents(cwd))-parents, 0)
}

// namesTheRootPrefix reports whether a resolved prefix is one of the
// directories that name the root, read case-insensitively, so that the only
// words typedTheRootPrefix weighs are the root's own. A prefix below the root
// is not one of them, and neither is one that leaves the root's ancestry, which
// a name is free to do and come back from.
func namesTheRootPrefix(components, rootComponents []string) bool {
	if len(components) > len(rootComponents) {
		return false
	}
	for i, component := range components {
		if !strings.EqualFold(component, rootComponents[i]) {
			return false
		}
	}
	return true
}

// pathComponents splits a cleaned absolute path into its components, so that
// foldRootPrefix and typedTheRootPrefix count the root's length and index
// against it with one function and the two cannot disagree. strings.Split alone
// cannot be that function, since it answers two elements for the filesystem
// root, "/" and its Windows "C:\" counterpart, where there is one component.
// The root is the only cleaned path with a trailing separator, so dropping the
// empty element it produces is the whole of the correction. No repo the gate
// resolves sits at the filesystem root today, and filepath.Rel reads every path
// under such a root as inside without ever reaching the fold, so this keeps the
// two counts honest rather than fixing a refusal a developer meets.
func pathComponents(path string) []string {
	sep := string(filepath.Separator)
	components := strings.Split(path, sep)
	if len(components) > 1 && components[len(components)-1] == "" {
		return components[:len(components)-1]
	}
	return components
}

// unreadable is the refusal for a filesystem failure reading the name itself
// and not something else, ENOTDIR from a name typed through a file, ELOOP from
// a symlink cycle, or EACCES on a parent directory. A failure statting the repo
// root or a case-differing spelling of it is not one of those, and named words
// that one about the root instead, since "<name> could not be read" would
// accuse a name the gate resolved successfully.
func unreadable(name string, err error) *UnresolvedError {
	return &UnresolvedError{Name: name, Reason: "could not be read, " + errnoText(err)}
}

// unweighable is the refusal for the other half, a filesystem failure about the
// repo root rather than about the name. It is worded here rather than at
// named's one call site so that the reason a refusal names the root sits beside
// unreadable, the refusal that names the path, and the two cannot drift into
// describing one fault class two ways.
func unweighable(name string, err error) *UnresolvedError {
	return &UnresolvedError{Name: name, Reason: "could not be weighed against the repo root, " + errnoText(err)}
}

// errnoText is what the operating system said, without the fs.PathError around
// it, because that error quotes an absolute path the developer never typed and
// which reads differently on every machine. Each refusal names the path it is
// about in its own words instead.
func errnoText(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
	}
	return err.Error()
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
