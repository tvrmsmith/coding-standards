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

	// Mis-cased below the root as well as at it. Only the root prefix folds, so
	// the components below come back as the candidate spells them, upper case
	// and all. ADR 0004 still refuses that case-only difference one layer up, in
	// named's spelling walk and in the join, which is what this asserts by
	// holding "SRC/a.txt" rather than the tree's "src/a.txt".
	rel, place, err := root.relativize(filepath.Join(miscased, "SRC", "a.txt"))

	if err != nil || place != folded || rel != "SRC/a.txt" {
		t.Errorf("relativize on a mis-cased root prefix returned %q, %v, %v, want \"SRC/a.txt\", folded, nil", rel, place, err)
	}
}

// The same passthrough on a case-sensitive filesystem, so ubuntu-latest holds
// the property rather than skipping it: the fold stops at the root prefix and
// "SRC" comes back as the candidate spells it, which is what leaves the join
// with nothing to match and keeps case_only_path_difference at exit 1.
// relativize stats the prefix alone, so the file below it need not exist.
func TestRelativizeLeavesComponentsBelowACaseDifferingRootSpellingAsSpelled(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)

	rel, place, err := root.relativize(filepath.Join(real, "SRC", "a.txt"))

	if err != nil || place != folded || rel != "SRC/a.txt" {
		t.Errorf("relativize on a mis-cased component below a case-differing root returned %q, %v, %v, want \"SRC/a.txt\", folded, nil", rel, place, err)
	}
}

// The repo root itself under another spelling has no components below the
// prefix, and it reads as ".", the same reading Rel gives for the root spelled
// correctly. The two named cases that reach this arm stop at "is a directory,
// not a file", which the correctly spelled root produces identically, so this is
// the only assertion holding the value.
func TestRelativizeReadsTheMisCasedRepoRootAsTheRootItself(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)

	rel, place, err := root.relativize(miscased)

	if err != nil || place != folded || rel != "." {
		t.Errorf("relativize on the mis-cased repo root returned %q, %v, %v, want \".\", folded, nil", rel, place, err)
	}
}

// The same reading on a case-sensitive filesystem, where the root's own spelling
// is the one that differs.
func TestRelativizeReadsACaseDifferingSpellingOfTheRepoRootAsTheRootItself(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)

	rel, place, err := root.relativize(real)

	if err != nil || place != folded || rel != "." {
		t.Errorf("relativize on a case-differing spelling of the repo root returned %q, %v, %v, want \".\", folded, nil", rel, place, err)
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

	if err != nil || place != folded || rel != "src/a.txt" {
		t.Errorf("relativize through a case-differing symlink to the root returned %q, %v, %v, want \"src/a.txt\", folded, nil", rel, place, err)
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

// A case-differing prefix that names nothing on disk is not the root under
// another spelling, and it is not a fault either. Root.Name is the road to it:
// resolveExisting climbs past components that are absent, so a --coverage
// report named under a REPO directory that does not exist arrives here with a
// prefix os.Stat cannot find. The answer is outside with no error, where a root
// that will not stat is a fault the caller hears about instead.
func TestRelativizeReadsAnAbsentCaseDifferingPrefixAsOutside(t *testing.T) {
	root := containmentRoot(t)
	absent := filepath.Join(filepath.Dir(root.Dir()), strings.ToUpper(filepath.Base(root.Dir())))
	if _, err := os.Stat(absent); err == nil {
		t.Skipf("the filesystem folded %s onto the root, so the prefix is on disk and this is the fold's own case", absent)
	}
	resolved := resolveExisting(filepath.Join(absent, "TestResults", "coverage.cobertura.xml"))

	rel, place, err := root.relativize(resolved)

	if err != nil || place != outside || rel != "" {
		t.Errorf("relativize on an absent case-differing prefix returned %q, %v, %v, want \"\", outside, nil", rel, place, err)
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

// Place reads a folded root prefix as landing where the candidate really sits
// (ADR 0004, amended 2026-09-11). named still refuses the same shape, because a
// --files path is one a developer typed and can retype; a coverage candidate is
// not, nobody typed it, and refusing it would drop a class the report really
// measured. os.SameFile is what separates this from the folding ADR 0004
// rejects, so no two files are merged.
func TestPlaceLandsACandidateWhoseRootPrefixIsMisCased(t *testing.T) {
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
	assertFolds(t, root, resolved)

	placed := root.Place(filepath.Join(miscased, "srclink", "a.cs"))

	rel, in := placed.Inside()
	if !in || rel != "src/a.cs" {
		t.Errorf("Place on a mis-cased root prefix returned %q, %v, want \"src/a.cs\", true", rel, in)
	}
	if placed.Resolved() != filepath.ToSlash(resolved) {
		t.Errorf("Resolved = %q, want the resolved candidate %q", placed.Resolved(), filepath.ToSlash(resolved))
	}
}

// The same answer on a case-sensitive filesystem, so CI runs the arm that makes
// Place land a folded candidate rather than skipping it. The root's own spelling
// is the one that differs here, which is what a candidate's own resolution
// cannot collapse.
func TestPlaceLandsACandidateUnderACaseDifferingRootSpelling(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	resolved := touch(t, filepath.Join(real, "src", "a.cs"))
	if err := os.Symlink("src", filepath.Join(real, "srclink")); err != nil {
		t.Fatal(err)
	}
	assertFolds(t, root, resolved)

	// Typed through a relative link, as the macOS twin does, so Resolved
	// answering the link-free path says Place got past the absolute and symlink
	// checks rather than turning back at one of them with the candidate as typed.
	placed := root.Place(filepath.Join(real, "srclink", "a.cs"))

	rel, in := placed.Inside()
	if !in || rel != "src/a.cs" {
		t.Errorf("Place under a case-differing root spelling returned %q, %v, want \"src/a.cs\", true", rel, in)
	}
	if placed.Resolved() != filepath.ToSlash(resolved) {
		t.Errorf("Resolved = %q, want the resolved candidate %q", placed.Resolved(), filepath.ToSlash(resolved))
	}
}

// The policy the fold's docs record for Place: a root the filesystem will not
// stat lands the candidate nowhere, the same as one genuinely outside, because
// Placed carries no reason and named is the only door with words for the fault.
// The class goes unmeasured rather than the run failing. Removing the link that
// is this Root's own directory is what makes isRootUnder's second stat fail
// while the candidate below the real directory still resolves.
func TestPlaceLandsNowhereWhenTheRepoRootCannotBeStatted(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	resolved := touch(t, filepath.Join(real, "src", "a.cs"))
	assertFolds(t, root, resolved)
	if err := os.Remove(root.Dir()); err != nil {
		t.Fatal(err)
	}

	placed := root.Place(resolved)

	rel, in := placed.Inside()
	if in || rel != "" {
		t.Errorf("Place against a root it cannot stat returned %q, %v, want \"\", false", rel, in)
	}
	if placed.Resolved() != filepath.ToSlash(resolved) {
		t.Errorf("Resolved = %q, want the resolved candidate %q", placed.Resolved(), filepath.ToSlash(resolved))
	}
}

// The same policy at Name, on every filesystem: a report the gate cannot weigh
// against the root is named by its absolute path, the shape a path outside the
// repo gets. Naming it repo-relative would claim a placement the gate does not
// have, and erroring would exit with no document at all. This is the shape the
// doc calls out as nearer at Name than at Place, a --coverage report that is not
// on disk yet under a prefix whose parent denies search.
func TestNameNamesAReportItCannotWeighAgainstTheRepoRootByItsAbsolutePath(t *testing.T) {
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
	path := filepath.Join(parent, "REPO", "TestResults", "coverage.cobertura.xml")
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	name := root.Name(path)

	if name != Name(filepath.ToSlash(path)) {
		t.Errorf("Name on a report it cannot weigh against the root = %q, want the absolute path %q", name, filepath.ToSlash(path))
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

// The shape no caller reaches today, a path handed over without a working
// directory joined on. It comes back as its own text, and pinning that is what
// would turn a later edit resolving it against cwd or the root red, since such
// an edit would silently place a report's bare filename inside the repo.
func TestNameReadsAPathThatIsNotAbsoluteAsItsOwnText(t *testing.T) {
	root := containmentRoot(t)

	name := root.Name(filepath.Join("TestResults", "coverage.xml"))

	if name != "TestResults/coverage.xml" {
		t.Errorf("Name on a path that is not absolute = %q, want %q", name, "TestResults/coverage.xml")
	}
}

// The naming side of what Place now does. A --coverage report under a mis-cased
// root prefix sits inside the repo, so it is named repo-relative like any other:
// naming it by the absolute path the developer's shell happened to spell would
// print one report under two strings depending on how the run was invoked.
//
// It skips on Linux because this is the shape a developer types, a real
// mis-cased spelling of a directory the filesystem folds. The case below runs
// the same arm everywhere by putting the case difference on the root's own side
// instead, which resolving the candidate cannot collapse.
func TestNameReadsAPathUnderAMisCasedRootPrefixAsRepoRelative(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	path := filepath.Join(miscased, "TestResults", "coverage.cobertura.xml")
	assertFolds(t, root, resolveExisting(path))

	name := root.Name(path)

	if name != "TestResults/coverage.cobertura.xml" {
		t.Errorf("Name under a mis-cased root prefix = %q, want %q", name, "TestResults/coverage.cobertura.xml")
	}
}

// The same answer on a case-sensitive filesystem, so CI runs the arm that names
// a folded path repo-relative rather than skipping it. The report is absent, as
// the ones Name has to name in a failure usually are, so the fold is weighed
// against the path resolveExisting rebuilt.
func TestNameReadsAPathUnderACaseDifferingRootSpellingAsRepoRelative(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	path := filepath.Join(real, "TestResults", "coverage.cobertura.xml")
	assertFolds(t, root, resolveExisting(path))

	name := root.Name(path)

	if name != "TestResults/coverage.cobertura.xml" {
		t.Errorf("Name under a case-differing root spelling = %q, want %q", name, "TestResults/coverage.cobertura.xml")
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

// assertFolds pins that the containment test reads resolved as folded, which is
// what makes a Place or Name case beside it a case about the fold. Those two
// answer a folded path and one spelled exactly as the tree spells it alike, so
// on their own their assertions cannot say which of the two the setup built: a
// filesystem or a temp path that quietly handed the case an ordinary inside
// reading would leave them green while testing nothing about the fold.
func assertFolds(t *testing.T, root Root, resolved string) {
	t.Helper()
	_, place, err := root.relativize(resolved)
	if err != nil || place != folded {
		t.Fatalf("relativize on %s returned %v, %v, want folded, nil; this case is not exercising the fold", resolved, place, err)
	}
}

// miscasedRoot is root's own directory with the last component upper-cased,
// skipping the case when that name does not reach the same directory. That is
// stricter than probing the filesystem once, since a temp root can sit under a
// case-sensitive component on a machine whose home is case-insensitive.
//
// Every case that starts from a Root NewRoot itself can build skips on a
// case-sensitive filesystem, and that is the shipped behaviour: NewRoot runs
// EvalSymlinks over the whole path, so a real Root's resolved field is always
// symlink-free and spelled exactly as the tree spells it, and no name that
// differs from it in case reaches the same directory on Linux. The fold is
// therefore macOS-only by construction. The cases that do run on Linux
// hand-build a Root the constructor cannot produce, so they exercise the arm's
// logic and not a state a Linux user reaches, and the end-to-end golden
// gate/test/golden/files_miscased_root_prefix.toon is never rendered on
// ubuntu-latest, so it can drift with the document schema without a test going
// red. Trevor has accepted that residual risk.
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
