package srcpath

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelativizeReadsACandidateUnderTheRootAsInside(t *testing.T) {
	root := containmentRoot(t)

	rel, place, err := root.relativize(filepath.Join(root.Dir(), "src", "A.cs"))

	if err != nil || place != inside || rel != "src/A.cs" {
		t.Errorf("relativize on a candidate under the root returned %q, %v, %v, want \"src/A.cs\", inside, nil", rel, place, err)
	}
}

// This is the case that has to hold on a case-sensitive filesystem, which is
// every Linux runner and the one CI gates merges on. ADR 0004 rejects folding
// case by text precisely because two spellings there are two real directories,
// and the fold rule is only allowed to relax that where os.SameFile says the
// two names reach one inode.
func TestRelativizeRefusesTwoDistinctDirectoriesDifferingOnlyInCase(t *testing.T) {
	tmp := t.TempDir()
	lower := mkdir(t, filepath.Join(tmp, "repo"))
	upper := mkdir(t, filepath.Join(tmp, "REPO"))
	if sameDirectory(t, lower, upper) {
		t.Skip("the filesystem folded the two names into one directory, so there are no two distinct directories to keep apart")
	}
	root, err := NewRoot(lower)
	if err != nil {
		t.Fatal(err)
	}

	rel, place, err := root.relativize(filepath.Join(upper, "a.txt"))

	if err != nil || place != outside || rel != "" {
		t.Errorf("relativize on a sibling directory differing only in case returned %q, %v, %v, want \"\", outside, nil", rel, place, err)
	}
}

// Every case below that goes through miscasedRoot needs a filesystem that
// folds case, so those run on macOS only. They cover the shape a developer
// actually types, a real mis-cased spelling, and the E2E golden is the same
// shape and the same skip.
//
// The folded arm itself is driven on every filesystem by the cases further
// down, which build a Root whose resolved directory is reached under a
// case-differing symlink. That Root is one NewRoot cannot produce, so on a
// case-sensitive filesystem those cases exercise the arm's logic and not a
// state a user of the shipped binary reaches; the user-visible case is the
// macOS one here.
func TestRelativizeReadsAMisCasedRootPrefixAsFolded(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)

	// Mis-cased below the root as well as at it, and folded says nothing about
	// the components below: only the root prefix folds, and ADR 0004 still
	// refuses a case-only difference below it one layer up in named and in the
	// join. A folded answer carries no path, since the candidate's spelling of
	// those components is not one this package vouches for.
	rel, place, err := root.relativize(filepath.Join(miscased, "SRC", "a.txt"))

	if err != nil || place != folded || rel != "" {
		t.Errorf("relativize on a mis-cased root prefix returned %q, %v, %v, want \"\", folded, nil", rel, place, err)
	}
}

// A direct unit test of foldRootPrefix's own contract, and only that. The link
// here is on the candidate's side of a root the gate resolved, which is an
// input no caller can hand it: Place, Name and named all resolve the candidate
// before they relativize, and that collapses this link back onto the root's own
// spelling. The link that does reach the fold everywhere is on the root's side,
// which the named cases below build.
func TestRelativizeFoldsARootPrefixReachedThroughACaseDifferingSymlink(t *testing.T) {
	root := containmentRoot(t)
	linked := filepath.Join(filepath.Dir(root.Dir()), strings.ToUpper(filepath.Base(root.Dir())))
	if _, err := os.Stat(linked); err == nil {
		t.Skipf("the filesystem folded %s onto the root already, so there is no link to make; the macOS cases cover this shape", linked)
	}
	if err := os.Symlink(root.Dir(), linked); err != nil {
		t.Fatal(err)
	}

	rel, place, err := root.relativize(filepath.Join(linked, "src", "a.txt"))

	if err != nil || place != folded || rel != "" {
		t.Errorf("relativize through a case-differing symlink to the root returned %q, %v, %v, want \"\", folded, nil", rel, place, err)
	}
}

// An ancestor of the root is shorter than the root, so foldRootPrefix has no
// prefix of the root's length to take and has to answer outside rather than
// slice past the end of the candidate's components. --files <parent-of-repo>
// reaches this on every filesystem.
func TestRelativizeReadsAnAncestorOfTheRootAsOutside(t *testing.T) {
	root := containmentRoot(t)

	rel, place, err := root.relativize(filepath.Dir(root.Dir()))

	if err != nil || place != outside || rel != "" {
		t.Errorf("relativize on an ancestor of the root returned %q, %v, %v, want \"\", outside, nil", rel, place, err)
	}
}

// A filesystem that will not say whether the case-differing prefix is the root
// is not the same answer as two distinct directories. Reading it as one would
// refuse a path that is in fact inside the repo as being outside it, so the
// failure travels up instead and named words it about the root. This runs on
// every filesystem, since dropping the search bit on the parent denies the stat
// whether or not the two spellings fold.
func TestRelativizeReportsAPrefixTheFilesystemWillNotStat(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root searches a directory whose mode denies it, so there is no stat failure to provoke")
	}
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := mkdir(t, filepath.Join(tmp, "parent"))
	root, err := NewRoot(mkdir(t, filepath.Join(parent, "repo")))
	if err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(parent, "REPO", "a.txt")
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	rel, place, err := root.relativize(candidate)

	if !errors.Is(err, fs.ErrPermission) || place != outside || rel != "" {
		t.Errorf("relativize on a prefix the filesystem will not stat returned %q, %v, %v, want \"\", outside, a permission error", rel, place, err)
	}
}

// EqualFold is what keeps the fold to spellings of one name. Two unrelated
// names for one inode, a bind mount or a hard-linked directory as a container
// hands them out, are one directory to os.SameFile and not a mis-spelling of
// anything, so folding them would answer "is not spelled as the repo root is"
// about a path with nothing mis-spelled.
//
// A test can make neither a bind mount nor a hard link to a directory, so the
// Root here is hand-built with a symlink for its resolved directory, outside
// NewRoot's invariant on purpose: NewRoot resolves the whole path, so no Root
// the gate builds looks like this. It stands in for the alias, and the shape a
// user reaches is the mount, not the link.
func TestRelativizeRefusesAnAliasOfTheRootSpelledUnderAnUnrelatedName(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := mkdir(t, filepath.Join(tmp, "repo"))
	alias := filepath.Join(tmp, "elsewhere")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	root := Root{resolved: alias}

	rel, place, err := root.relativize(filepath.Join(real, "a.txt"))

	if err != nil || place != outside || rel != "" {
		t.Errorf("relativize on an alias of the root under an unrelated name returned %q, %v, %v, want \"\", outside, nil", rel, place, err)
	}
}

// The repo root going away between NewRoot and the fold, which is the one way a
// --files path can reach named and still leave the gate unable to weigh it
// against the root: every earlier arm resolved the name itself, so what fails
// here is the root. The developer hears that rather than "is outside the repo
// root", which is the misdiagnosis issue 48 set out to remove, and it is an
// UnresolvedError so it lands under file_unresolved like every other refusal
// about a path.
//
// symlinkedMiscasedRoot skips this on a filesystem that folds case, where
// removing the link leaves the root's own spelling still reaching the
// directory.
func TestNamedRefusesAPathItCannotWeighAgainstTheRepoRoot(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	name := touch(t, filepath.Join(real, "src", "a.cs"))
	if err := os.Remove(root.Dir()); err != nil {
		t.Fatal(err)
	}

	_, err := root.named(name, dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if !strings.HasPrefix(unresolved.Reason, "could not be weighed against the repo root, ") {
		t.Errorf("Reason = %q, want it to open with %q", unresolved.Reason, "could not be weighed against the repo root, ")
	}
	if unresolved.Name != name {
		t.Errorf("Name = %q, want the name as typed, %q", unresolved.Name, name)
	}
}

func TestNamedRefusesAMisCasedRootPrefixAsASpellingRatherThanALocation(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	// Everything below the root is spelled as the tree spells it, so the only
	// thing wrong with this name is the root prefix, and issue 48's whole
	// complaint is that the gate sent the developer after the location.
	name := filepath.Join(miscased, "src", "a.cs")

	_, err := root.named(name, dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != "is not spelled as the repo root is" {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, "is not spelled as the repo root is")
	}
	if unresolved.Name != name {
		t.Errorf("Name = %q, want the name as typed, %q", unresolved.Name, name)
	}
}

// The same refusal on a case-sensitive filesystem, so CI runs the folded arm
// rather than skipping it. named resolves the candidate's symlinks and never
// the root's, so a root whose own directory is reached under a case-differing
// name folds a candidate that is spelled exactly as the tree spells it.
func TestNamedRefusesACaseDifferingRootSpellingAsASpellingRatherThanALocation(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	name := touch(t, filepath.Join(real, "src", "a.cs"))

	_, err := root.named(name, dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != "is not spelled as the repo root is" {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, "is not spelled as the repo root is")
	}
	if unresolved.Name != name {
		t.Errorf("Name = %q, want the name as typed, %q", unresolved.Name, name)
	}
}

// The ordering the two arms sit in, pinned where every runner executes it. A
// folded path that is also a directory is a directory mistake first, so the
// developer who retypes the case gets the same message on the second try.
func TestNamedRefusesACaseDifferingRootSpellingOfTheRepoRootAsADirectory(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)

	_, err := root.named(real, dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != "is a directory, not a file" {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, "is a directory, not a file")
	}
}

// --files naming the repo root is a directory mistake whichever case it is
// typed in, so the developer who retypes the case gets the same message on the
// second try rather than a new one. The mis-cased half of the pair only exists
// on a filesystem that folds; the correctly cased half below it runs everywhere
// and is what the two are compared against.
func TestNamedRefusesTheMisCasedRepoRootAsADirectory(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)

	_, err := root.named(miscased, dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != "is a directory, not a file" {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, "is a directory, not a file")
	}
}

func TestNamedRefusesTheRepoRootAsADirectory(t *testing.T) {
	root := containmentRoot(t)

	_, err := root.named(root.Dir(), dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != "is a directory, not a file" {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, "is a directory, not a file")
	}
}

// The fold rule narrows what "outside" means for --files, so this pins that it
// still means something there. It needs no skip, since a file beside the root
// is outside it on either kind of filesystem.
func TestNamedStillRefusesAPathGenuinelyOutsideTheRoot(t *testing.T) {
	root := containmentRoot(t)
	name := touch(t, filepath.Join(filepath.Dir(root.Dir()), "outside.cs"))

	_, err := root.named(name, dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != "is outside the repo root" {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, "is outside the repo root")
	}
}

// The fold is asymmetric on purpose, and this is the other side of it. A
// --files path is one a developer typed and can retype, so named tells them
// which half to fix. A coverage candidate is not: nobody typed it, so there is
// nothing to retype, and folding it in would be case folding on the
// coverage-path side, which ADR 0004 rejects. Place therefore answers what it
// always has, that the candidate landed nowhere. Making the coverage side fold
// too is a decision for a dated ADR 0004 amendment, which is Trevor's call.
func TestPlaceRefusesACandidateWhoseRootPrefixIsMisCased(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	if err := os.Symlink("src", filepath.Join(root.Dir(), "srclink")); err != nil {
		t.Fatal(err)
	}
	// Typed through a relative link, which resolves to the same mis-cased prefix
	// with the link spelled out. Resolved answering the link-free path is what
	// says Place got past the absolute and symlink checks ahead of the fold,
	// rather than turning back at one of them with the candidate as typed.
	resolved := filepath.Join(miscased, "src", "a.cs")

	placed := root.Place(filepath.Join(miscased, "srclink", "a.cs"))

	rel, in := placed.Inside()
	if in || rel != "" {
		t.Errorf("Place on a mis-cased root prefix returned %q, %v, want \"\", false", rel, in)
	}
	if placed.Resolved() != filepath.ToSlash(resolved) {
		t.Errorf("Resolved = %q, want the resolved candidate %q", placed.Resolved(), filepath.ToSlash(resolved))
	}
}

// The same asymmetry on a case-sensitive filesystem, so CI runs the arm that
// makes Place read folded as landing nowhere rather than skipping it. The root's
// own spelling is the one that differs here, which is what a candidate's own
// resolution cannot collapse.
func TestPlaceRefusesACandidateUnderACaseDifferingRootSpelling(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	resolved := touch(t, filepath.Join(real, "src", "a.cs"))
	if err := os.Symlink("src", filepath.Join(real, "srclink")); err != nil {
		t.Fatal(err)
	}

	// Typed through a relative link, as the macOS twin does, so Resolved
	// answering the link-free path says Place got past the absolute and symlink
	// checks rather than turning back at one of them with the candidate as typed.
	placed := root.Place(filepath.Join(real, "srclink", "a.cs"))

	rel, in := placed.Inside()
	if in || rel != "" {
		t.Errorf("Place under a case-differing root spelling returned %q, %v, want \"\", false", rel, in)
	}
	if placed.Resolved() != filepath.ToSlash(resolved) {
		t.Errorf("Resolved = %q, want the resolved candidate %q", placed.Resolved(), filepath.ToSlash(resolved))
	}
}

// Name exists beside Place because a --coverage report the gate has to name in
// a failure very often names nothing on disk, and Place refuses anything that
// is not a regular file.
func TestNameReadsAnAbsentPathInsideTheRepoAsRepoRelative(t *testing.T) {
	root := containmentRoot(t)

	name := root.Name(filepath.Join(root.Dir(), "TestResults", "coverage.cobertura.xml"))

	if name != "TestResults/coverage.cobertura.xml" {
		t.Errorf("Name on an absent path inside the repo = %q, want %q", name, "TestResults/coverage.cobertura.xml")
	}
}

// The naming side of the same asymmetry Place holds. A --coverage report under
// a mis-cased root prefix is still named by the absolute path the containment
// test weighed, which is what the gate has always answered and what every
// existing coverage golden is written against. Only named acts on the fold;
// pulling the coverage side in with it needs a dated ADR 0004 amendment.
//
// It skips on Linux because this is the shape a developer types, a real
// mis-cased spelling of a directory the filesystem folds. The case below runs
// the same arm everywhere by putting the case difference on the root's own side
// instead, which resolving the candidate cannot collapse.
func TestNameReadsAPathUnderAMisCasedRootPrefixAsItsResolvedAbsolutePath(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	path := filepath.Join(miscased, "TestResults", "coverage.cobertura.xml")

	name := root.Name(path)

	if name != Name(filepath.ToSlash(path)) {
		t.Errorf("Name under a mis-cased root prefix = %q, want the resolved path %q", name, filepath.ToSlash(path))
	}
}

// The same answer on a case-sensitive filesystem, so CI runs the arm that makes
// Name read folded as outside rather than skipping it. The report is absent, as
// the ones Name has to name in a failure usually are, so the resolved path it
// comes back with is the one resolveExisting rebuilt.
func TestNameReadsAPathUnderACaseDifferingRootSpellingAsItsResolvedAbsolutePath(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	path := filepath.Join(real, "TestResults", "coverage.cobertura.xml")

	name := root.Name(path)

	if name != Name(filepath.ToSlash(path)) {
		t.Errorf("Name under a case-differing root spelling = %q, want the resolved path %q", name, filepath.ToSlash(path))
	}
}

func TestNameReadsAPathOutsideTheRepoAsItsResolvedAbsolutePath(t *testing.T) {
	root := containmentRoot(t)
	tmp := filepath.Dir(root.Dir())
	real := touch(t, filepath.Join(tmp, "reports", "coverage.xml"))
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(filepath.Join(tmp, "reports"), link); err != nil {
		t.Fatal(err)
	}

	// Named through the link, so the answer being the resolved path rather than
	// the typed one is visible rather than a coincidence of the two matching.
	name := root.Name(filepath.Join(link, "coverage.xml"))

	if name != Name(filepath.ToSlash(real)) {
		t.Errorf("Name on a path outside the repo = %q, want the resolved path %q", name, filepath.ToSlash(real))
	}
}

// resolveExisting's whole reason. The gate resolves the repo root, so a path
// typed through the link that leads to it compares against a resolved root and
// escapes for the indirection rather than for where it sits. macOS hands every
// run this shape already, since /tmp and /var are links.
func TestNameReadsAPathTypedThroughASymlinkedRootAsRepoRelative(t *testing.T) {
	tmp := t.TempDir()
	mkdir(t, filepath.Join(tmp, "actual", "repo"))
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(filepath.Join(tmp, "actual"), link); err != nil {
		t.Fatal(err)
	}
	root, err := NewRoot(filepath.Join(link, "repo"))
	if err != nil {
		t.Fatal(err)
	}

	name := root.Name(filepath.Join(link, "repo", "TestResults", "coverage.xml"))

	if name != "TestResults/coverage.xml" {
		t.Errorf("Name on a path through the root's own symlink = %q, want %q", name, "TestResults/coverage.xml")
	}
}

// symlinkedMiscasedRoot is a Root whose resolved directory is an upper-cased
// symlink to a real lower-cased one, returned with that real directory. It is
// how a case-sensitive filesystem reaches the fold: two spellings, one inode,
// which is what sameDir asks for, and the root's own spelling is the one that
// differs, so resolving the candidate cannot collapse it.
//
// It builds that Root by hand, outside NewRoot's invariant and on purpose:
// NewRoot resolves the whole path, so a real Root's directory is never a
// symlink, and on a case-sensitive filesystem no Root the gate builds holds a
// name that can fold. The cases driven from here are therefore a unit-level
// exercise of the folded arm on Linux, and the user-visible shape is the macOS
// one that miscasedRoot builds.
//
// A filesystem that folds case cannot build it, since the upper-cased name is
// already the directory, and there the miscasedRoot cases cover the same arm.
func symlinkedMiscasedRoot(t *testing.T) (Root, string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := mkdir(t, filepath.Join(tmp, "repo"))
	link := filepath.Join(tmp, "REPO")
	if _, err := os.Stat(link); err == nil {
		t.Skipf("the filesystem folded %s onto %s already, so there is no link to make", link, real)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	return Root{resolved: link}, real
}

// miscasedRoot is root's own directory with the last component upper-cased,
// skipping the case when that name does not reach the same directory. That is
// stricter than probing the filesystem once, since a temp root can sit under a
// case-sensitive component on a machine whose home is case-insensitive.
func miscasedRoot(t *testing.T, root Root) string {
	t.Helper()
	dir := root.Dir()
	miscased := filepath.Join(filepath.Dir(dir), strings.ToUpper(filepath.Base(dir)))
	if miscased == dir {
		t.Fatalf("upper-casing %s changed nothing, so the case cannot tell a fold from an exact match", dir)
	}
	if _, err := os.Stat(miscased); err != nil || !sameDirectory(t, dir, miscased) {
		t.Skipf("the filesystem is case sensitive, so %s does not name the same directory as %s", miscased, dir)
	}
	return miscased
}

// sameDirectory reports whether two names reach one directory, which is how a
// case tells a filesystem that folded them apart from one that kept them.
func sameDirectory(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	infoB, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(infoA, infoB)
}

// containmentRoot is an empty throwaway directory as a Root, resolved the way
// the gate resolves the repo root. It is named rather than being the temp
// directory itself, because the fold cases re-spell the last component and
// t.TempDir()'s own is a number that upper-cases to itself.
func containmentRoot(t *testing.T) Root {
	t.Helper()
	root, err := NewRoot(mkdir(t, filepath.Join(t.TempDir(), "repo")))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// mkdir creates dir and returns it, so a case can build a tree in expressions
// rather than in four lines of error handling each.
func mkdir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// touch writes an empty file at path, creating its parents.
func touch(t *testing.T, path string) string {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
