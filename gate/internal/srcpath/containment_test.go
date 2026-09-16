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

// A component that is a file rather than a directory is climbed past exactly
// like an absent one: EvalSymlinks reports ENOTDIR partway down a path like
// this one, not fs.ErrNotExist, and before the fix the climb guard only knew
// the second, so it handed back the unresolved candidate untouched. That
// candidate shares no prefix with a root reached through a symlink, so a
// developer chasing a coverage report through a file misread as a directory
// got told their report was outside the repo instead. The root here is
// reached through a symlink for the same reason resolveExisting itself
// exists, so the case fails on Linux too and not only where macOS hands every
// run a symlinked /tmp already.
func TestNameClimbsPastAFileComponentUnderASymlinkedRoot(t *testing.T) {
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
	touch(t, filepath.Join(root.Dir(), "README.md"))

	name := root.Name(filepath.Join(link, "repo", "README.md", "TestResults", "coverage.cobertura.xml"))

	if name != "README.md/TestResults/coverage.cobertura.xml" {
		t.Errorf("Name through a file component under a symlinked root = %q, want %q", name, "README.md/TestResults/coverage.cobertura.xml")
	}
}

// A symlink cycle is a real fault and not a component that is merely absent or
// a file where a directory belongs, so the climb guard leaves it where it
// found it. Treating ELOOP like the other two would rename a report that
// might sit inside the repo into one read as though it sat outside, on the
// strength of a loop that says nothing about location at all.
func TestResolveExistingLeavesASymlinkCycleUnclimbed(t *testing.T) {
	root := containmentRoot(t)
	loop := filepath.Join(root.Dir(), "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(loop, "coverage.cobertura.xml")

	if got := resolveExisting(path); got != path {
		t.Errorf("resolveExisting on a path through a symlink cycle = %q, want it unchanged, %q", got, path)
	}
}

// An ancestor the process may not search is a real fault too, and comes back
// unclimbed for the same reason ELOOP does.
func TestResolveExistingLeavesAnUnsearchableAncestorUnclimbed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root searches a directory whose mode denies it, so there is no stat failure to provoke")
	}
	root := containmentRoot(t)
	denied := mkdir(t, filepath.Join(root.Dir(), "denied"))
	if err := os.Chmod(denied, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(denied, 0o755) })
	path := filepath.Join(denied, "sub", "coverage.cobertura.xml")

	if got := resolveExisting(path); got != path {
		t.Errorf("resolveExisting on a path under an unsearchable ancestor = %q, want it unchanged, %q", got, path)
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

	assertRootSpellingRefusal(t, err, name)
}

// A relative name that only descends carries no root prefix, so the mis-cased
// prefix that folds here came from the working directory the shell cd'd through,
// and refusing "src/a.cs" for the spelling of a directory that is not in the
// string would blame a half the developer cannot retype. It is accepted, and
// spelledAsOnDisk still holds every component below the root to the tree's own
// spelling.
func TestNamedAcceptsARelativeNameResolvedThroughAMisCasedWorkingDirectory(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	// t.Chdir sets PWD as well as the process directory, which is what makes
	// os.Getwd hand named the mis-cased spelling rather than the tree's own.
	// os.Getwd only prefers PWD when it stats as the same file, so without this
	// the case degrades into an ordinary inside reading that never reaches the
	// fold and still passes.
	t.Chdir(miscased)
	if cwd, err := os.Getwd(); err != nil || cwd != miscased {
		t.Fatalf("os.Getwd returned %q, %v, want the mis-cased %q; this case is not exercising the fold", cwd, err, miscased)
	}
	assertFolds(t, root, filepath.Join(miscased, "src", "a.cs"))

	rel, err := root.named(filepath.Join("src", "a.cs"), dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a relative name under a mis-cased working directory returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// A relative name that climbs above the root and descends back in does spell the
// prefix, so it is refused like the absolute one. This is the same developer
// mistake issue 48 is about, typed from one directory in rather than from the
// shell's root, and the mis-cased component is in the string they can retype.
func TestNamedRefusesAMisCasedRootPrefixARelativeNameClimbedOutTo(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	t.Chdir(filepath.Join(root.Dir(), "src"))
	name := filepath.Join("..", "..", filepath.Base(miscased), "src", "a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same climb typed from a working directory that is a symlink out of the
// repo, which is where reading the name two ways comes apart. The shell reports
// the logical directory, so the climb pops "ln" and "repo" off the text and
// lands on the mis-cased root, exactly as it does from any other directory one
// level in. A walk that resolved each prefix before popping the parents would
// leave through the link into the other tree instead and answer for a path the
// gate never placed the file by. The link points deep enough into that tree
// that popping two parents there reaches a directory with no root of any
// spelling under it, which is what makes the two readings disagree.
func TestNamedRefusesAMisCasedRootPrefixClimbedOutOfASymlinkedWorkingDirectory(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	elsewhere := mkdir(t, filepath.Join(filepath.Dir(root.Dir()), "other", "deep", "deeper"))
	if err := os.Symlink(elsewhere, filepath.Join(root.Dir(), "ln")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root.Dir(), "ln"))
	assertFolds(t, root, filepath.Join(miscased, "src", "a.cs"))
	name := filepath.Join("..", "..", filepath.Base(miscased), "src", "a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same shape on a case-sensitive filesystem, so the merge gate runs it.
func TestNamedRefusesACaseDifferingRootSpellingClimbedOutOfASymlinkedWorkingDirectory(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	elsewhere := mkdir(t, filepath.Join(filepath.Dir(real), "other", "deep", "deeper"))
	if err := os.Symlink(elsewhere, filepath.Join(real, "ln")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(real, "ln"))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))
	name := filepath.Join("..", "..", filepath.Base(real), "src", "a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same refusal reached by a name that opens with a component rather than
// with "..", so the climb only exists once filepath.Clean has folded it into a
// leading parent. The developer's text is longer than the path it reaches, and
// the mis-cased component still has to line up against the one component of the
// resolved candidate their own "REPO" produced.
func TestNamedRefusesAMisCasedRootPrefixAnInteriorClimbReachedOutTo(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	t.Chdir(root.Dir())
	// Spelled rather than joined, because filepath.Join cleans its arguments and
	// would hand named a name whose climb is already a leading parent.
	name := filepath.FromSlash("src/../../" + filepath.Base(miscased) + "/src/a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The climbing-out refusal on a case-sensitive filesystem, so CI runs it rather
// than skipping it. The case difference is on the root's own side here, so the
// spelling the developer types to reach the tree, "repo", is the one the root
// does not use, and their text is what puts it in the candidate.
func TestNamedRefusesACaseDifferingRootSpellingARelativeNameClimbedOutTo(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(filepath.Join(real, "src"))
	name := filepath.Join("..", "..", filepath.Base(real), "src", "a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same interior climb on a case-sensitive filesystem.
func TestNamedRefusesACaseDifferingRootSpellingAnInteriorClimbReachedOutTo(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(real)
	// Spelled rather than joined, because filepath.Join cleans its arguments and
	// would hand named a name whose climb is already a leading parent.
	name := filepath.FromSlash("src/../../" + filepath.Base(real) + "/src/a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// An absolute name typed through a symlinked ancestor of the repo, which every
// macOS run hands the gate already, since t.TempDir sits under /var and the
// resolved candidate carries /private/var. The developer's text and the
// candidate then have different component counts, and reading their text from
// the left lines "REPO" up against a word one place along, credits the mis-case
// to nobody, and accepts the very name issue 48 exists to refuse. Lining the
// two up from the right end is what keeps the refusal.
func TestNamedRefusesAMisCasedRootPrefixTypedThroughASymlinkedAncestor(t *testing.T) {
	tmp := t.TempDir()
	resolvedTmp, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedTmp == tmp {
		t.Skipf("%s has no symlinked ancestor, so no name typed through it can cross one", tmp)
	}
	root, err := NewRoot(mkdir(t, filepath.Join(tmp, "repo")))
	if err != nil {
		t.Fatal(err)
	}
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	// Typed under the unresolved temp directory, the spelling a developer's
	// shell hands them, with only the root component re-cased.
	name := filepath.Join(tmp, filepath.Base(miscased), "src", "a.cs")
	assertFolds(t, root, filepath.Join(miscased, "src", "a.cs"))

	_, err = root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same refusal on a case-sensitive filesystem, where the shortcut stands in
// for the symlinked ancestor macOS supplies.
func TestNamedRefusesACaseDifferingRootSpellingTypedThroughASymlinkedAncestor(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	under := mkdir(t, filepath.Join(tmp, "a", "b"))
	real := mkdir(t, filepath.Join(under, "repo"))
	link := filepath.Join(under, "REPO")
	if _, err := os.Stat(link); err == nil {
		t.Skipf("the filesystem folded %s onto %s already, so there is no link to make", link, real)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	shortcut := filepath.Join(tmp, "s")
	if err := os.Symlink(under, shortcut); err != nil {
		t.Fatal(err)
	}
	root := Root{resolved: link}
	touch(t, filepath.Join(real, "src", "a.cs"))
	name := filepath.Join(shortcut, "repo", "src", "a.cs")
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))

	_, err = root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// A name whose text climbs below the root component. The walk reaches the root
// before the climb and answers there, so what the developer wrote underneath
// changes nothing about the half they are sent back to retype.
func TestNamedRefusesAMisCasedRootPrefixWithAClimbBelowIt(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	// Spelled rather than joined, because filepath.Join would clean the climb
	// away before named ever sees it.
	name := filepath.FromSlash(filepath.ToSlash(miscased) + "/src/../src/a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same climb on a case-sensitive filesystem, so the merge gate runs it.
func TestNamedRefusesACaseDifferingRootSpellingWithAClimbBelowIt(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	name := filepath.FromSlash(filepath.ToSlash(real) + "/src/../src/a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// A mis-case the shell introduced above the repo root, on a name that climbs,
// in the shape a developer actually types: they cd'd through a mis-cased parent
// of the repo and the filesystem folded it. The root component in their --files
// string is spelled exactly as the tree spells it, so there is nothing in that
// string to retype, and refusing would be the mirror of the bug the climb
// refusal exists to fix. The case below runs the same arm on a case-sensitive
// filesystem.
func TestNamedAcceptsARelativeNameUnderAMisCasedParentOfTheRoot(t *testing.T) {
	root, miscasedParent := miscasedParentRoot(t)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	t.Chdir(filepath.Join(miscasedParent, "repo", "src"))
	if cwd, err := os.Getwd(); err != nil || cwd != filepath.Join(miscasedParent, "repo", "src") {
		t.Fatalf("os.Getwd returned %q, %v, want the mis-cased spelling; this case is not exercising the fold", cwd, err)
	}
	assertFolds(t, root, filepath.Join(miscasedParent, "repo", "src", "a.cs"))
	// "repo" spelled as the tree spells it. The mis-cased component is DEV,
	// above it, which only the working directory supplied.
	name := filepath.Join("..", "..", "repo", "src", "a.cs")

	rel, err := root.named(name, dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name climbing out under a mis-cased parent returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// A mis-cased root prefix followed by a symlink whose target is deeper than the
// link. The candidate then has more components than the text that named it, and
// the mis-cased word the developer typed is still the root's own. Nothing about
// the resolved candidate says which of its components their "REPO" produced,
// which is why the walk asks the filesystem instead.
func TestNamedRefusesAMisCasedRootPrefixAheadOfACountChangingSymlink(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "deep", "inner", "a.cs"))
	// Stored relative, so resolving it keeps the developer's own spelling of the
	// root and still lands two components where they typed one.
	if err := os.Symlink(filepath.Join("deep", "inner"), filepath.Join(root.Dir(), "link")); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(miscased, "link", "a.cs")
	assertFolds(t, root, filepath.Join(miscased, "deep", "inner", "a.cs"))

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same shape on a case-sensitive filesystem, so the merge gate runs it.
func TestNamedRefusesACaseDifferingRootSpellingAheadOfACountChangingSymlink(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "deep", "inner", "a.cs"))
	if err := os.Symlink(filepath.Join("deep", "inner"), filepath.Join(real, "link")); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(real, "link", "a.cs")
	assertFolds(t, root, filepath.Join(real, "deep", "inner", "a.cs"))

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// A symlink named something else pointing at the repo root. The prefix the
// developer typed does reach the root, so the walk stops there, but "link" is
// not the root's name in another case, it is a different name for the same
// directory. There is nothing to retype, so the name is accepted. This is what
// separates a mis-spelling from an alias.
func TestNamedAcceptsARootReachedThroughALinkNamedSomethingElse(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	tmp := filepath.Dir(root.Dir())
	if err := os.Symlink(miscased, filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)
	assertFolds(t, root, filepath.Join(miscased, "src", "a.cs"))

	rel, err := root.named(filepath.Join("link", "src", "a.cs"), dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name reaching the root through a link of another name returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// The same acceptance on a case-sensitive filesystem.
func TestNamedAcceptsACaseDifferingRootReachedThroughALinkNamedSomethingElse(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	tmp := filepath.Dir(real)
	if err := os.Symlink(real, filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))

	rel, err := root.named(filepath.Join("link", "src", "a.cs"), dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name reaching the root through a link of another name returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// The developer's text spelling a mis-cased component of the root prefix that
// is not the root's last word. The root prefix is every directory that names
// the root, so a parent they typed in a case the root does not use is theirs to
// retype even though "repo" underneath it is spelled correctly. It differs from
// the accepting case below only in climbing one level higher, which moves that
// same parent from the shell's side of the climb point to theirs.
func TestNamedRefusesAMisCasedRootComponentAboveTheLastOneTheTextSpelled(t *testing.T) {
	root, real := rootUnderASymlinkedMiscasedParent(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(filepath.Join(real, "src"))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))
	// The parent as the tree spells it, which is the spelling the root does not
	// use on this side of the pair.
	parent := filepath.Base(filepath.Dir(real))
	name := filepath.Join("..", "..", "..", parent, filepath.Base(real), "src", "a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// The same refusal in the shape a developer reaches on a filesystem that folds,
// and the twin of the accepting case below, which uses this same tree and
// climbs one level less, so the shell supplies the mis-cased parent instead.
func TestNamedRefusesAMisCasedParentOfTheRootTheTextSpelled(t *testing.T) {
	root, miscasedParent := miscasedParentRoot(t)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	t.Chdir(filepath.Join(root.Dir(), "src"))
	assertFolds(t, root, filepath.Join(miscasedParent, "repo", "src", "a.cs"))
	name := filepath.Join("..", "..", "..", filepath.Base(miscasedParent), "repo", "src", "a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// A mis-case the shell introduced above the repo root, on a name that climbs.
// The developer spelled the root component exactly as the tree spells it, and
// the only component in another case is one their string never carried, so
// refusing would send them to retype a half they cannot reach: every retyping
// of this name re-enters the mis-cased directory through the working directory.
// Reading the climb point's location rather than which component is mis-cased
// refuses this, which is the mirror of the bug the climb refusal exists to fix.
//
// The same file from the same shell is accepted as --files a.cs, so accepting
// here is also what keeps one file from getting two verdicts on how its name
// happens to be written.
func TestNamedAcceptsARelativeNameWhoseWorkingDirectoryIsMisCasedAboveTheRoot(t *testing.T) {
	root, real := rootUnderASymlinkedMiscasedParent(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(filepath.Join(real, "src"))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))
	// "repo" spelled as the tree spells it. The mis-cased component is the
	// parent above it, which only the working directory supplied.
	name := filepath.Join("..", "..", filepath.Base(real), "src", "a.cs")

	rel, err := root.named(name, dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name climbing out under a mis-cased parent returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// The same shape on a filesystem that folds, where the shortening link is an
// ordinary one and only the root's own spelling carries the case. NewRoot
// resolves symlinks but not case, so a developer who passed the mis-cased
// parent to --repo-root leaves the gate holding this Root.
func TestNamedAcceptsAMisCasedRootSpellingReachedThroughASymlinkedWorkingDirectory(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	under := mkdir(t, filepath.Join(tmp, "a", "b"))
	parent := mkdir(t, filepath.Join(under, "dev"))
	miscasedParent := filepath.Join(under, "DEV")
	if _, err := os.Stat(miscasedParent); err != nil || !sameDirectory(t, parent, miscasedParent) {
		t.Skipf("the filesystem is case sensitive, so %s does not name the same directory as %s", miscasedParent, parent)
	}
	real := mkdir(t, filepath.Join(parent, "repo"))
	root, err := NewRoot(filepath.Join(miscasedParent, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(under, filepath.Join(tmp, "s")); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(filepath.Join(tmp, "s", "dev", "repo", "src"))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))
	name := filepath.Join("..", "..", "repo", "src", "a.cs")

	rel, err := root.named(name, dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name climbing out through a symlinked working directory returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// A working directory that reaches the root through a symlink shortening the
// path. The shell's own spelling is three components shorter than the resolved
// candidate here, so a walk that located the developer's text by counting the
// working directory would line "dev" up against a word they never typed. Asking
// the filesystem which prefix is the root does not care how many components the
// shortcut saved.
func TestNamedAcceptsARelativeNameWhoseWorkingDirectoryReachesTheRootThroughASymlink(t *testing.T) {
	root, real, shortcut := rootReachedThroughAShortcut(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(filepath.Join(shortcut, "dev", "repo", "src"))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))
	name := filepath.Join("..", "..", "repo", "src", "a.cs")

	rel, err := root.named(name, dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name climbing out through a symlinked working directory returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// The refusing twin over that same tree, where the text does spell the parent
// in a case the root does not use. The shell's own spelling is three components
// shorter than the resolved candidate here, so a walk that located the text by
// counting the working directory would line "dev" up against a word the
// developer never typed and drop issue 48's refusal for everyone whose shell
// reaches the repo through a link.
func TestNamedRefusesAMisCasedParentTypedThroughAShortcutWorkingDirectory(t *testing.T) {
	root, real, shortcut := rootReachedThroughAShortcut(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(filepath.Join(shortcut, "dev", "repo", "src"))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))
	name := filepath.Join("..", "..", "..", filepath.Base(filepath.Dir(real)), "repo", "src", "a.cs")

	_, err := root.named(name, dirNames{})

	assertRootSpellingRefusal(t, err, name)
}

// A name whose descent passes through a symlink whose target spells the root in
// another case. The developer typed "link/a.cs", which carries no root
// component at all, so the mis-cased spelling the name resolves to came from the
// link's stored target rather than from their keyboard, and refusing would send
// them to retype a half of a string that is not in it. Locating their text by
// depth alone refuses this, because the link's target lands the mis-cased
// component at exactly the position their own text occupies.
func TestNamedAcceptsARelativeNameWhoseSymlinkTargetSpellsTheRootMisCased(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	tmp := filepath.Dir(root.Dir())
	// Stored mis-cased, which is the spelling EvalSymlinks hands back and the
	// component the developer's own name never carries.
	if err := os.Symlink(filepath.Join(miscased, "src"), filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)
	assertFolds(t, root, filepath.Join(miscased, "src", "a.cs"))

	rel, err := root.named(filepath.Join("link", "a.cs"), dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name descending through a mis-cased symlink target returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// The same acceptance on a case-sensitive filesystem, where the root's own
// spelling is the one that differs and the link target spells the tree's.
func TestNamedAcceptsADescentThroughASymlinkUnderACaseDifferingRootSpelling(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	tmp := filepath.Dir(real)
	if err := os.Symlink(filepath.Join(real, "src"), filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))

	rel, err := root.named(filepath.Join("link", "a.cs"), dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name descending through a symlink into the repo returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// A name that spells the root exactly as the root spells it and then descends
// through a symlink whose target re-cases it. The root component in the
// resolved candidate is the link's word and not theirs, and blaming it would
// refuse a name with nothing in it to retype: the developer typed "repo", which
// is what the gate would send them back to type again.
func TestNamedAcceptsADescentThroughASymlinkUnderACorrectlySpelledRootPrefix(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	if err := os.Symlink(miscased, filepath.Join(root.Dir(), "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(root.Dir()))
	assertFolds(t, root, filepath.Join(miscased, "src", "a.cs"))
	name := filepath.Join(filepath.Base(root.Dir()), "link", "src", "a.cs")

	rel, err := root.named(name, dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name descending through a link that re-cases the root returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// The same acceptance on a case-sensitive filesystem, where the root's own
// spelling is the one that differs and the link leads back to the tree's.
func TestNamedAcceptsADescentThroughASymlinkSpellingTheRootAsTheTreeDoes(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	if err := os.Symlink(real, filepath.Join(real, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(real))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))
	name := filepath.Join(filepath.Base(root.Dir()), "link", "src", "a.cs")

	rel, err := root.named(name, dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name descending through a link back into the repo returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// A shell already inside a case-differing spelling of the repo, naming a file
// beside it, which is the most ordinary folded invocation there is. The climb
// point is deeper than the root, so no root component is left for the
// developer's text to have spelled and the name is accepted.
func TestNamedAcceptsANameTypedFromInsideACaseDifferingRootSpelling(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(filepath.Join(real, "src"))
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))

	rel, err := root.named("a.cs", dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a name typed from inside the repo returned %q, %v, want \"src/a.cs\", nil", rel, err)
	}
}

// The fold-accept road still holds every component below the root to the tree's
// own spelling. The mis-cased working directory folds the prefix and the name is
// accepted for it, and then "SRC" is refused as the file's own spelling, the
// refusal that sends the developer to retype the half that is theirs. Nothing on
// this road is matched approximately for having taken it.
func TestNamedRefusesABelowRootMisCaseReachedThroughAMisCasedWorkingDirectory(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))
	t.Chdir(miscased)
	if cwd, err := os.Getwd(); err != nil || cwd != miscased {
		t.Fatalf("os.Getwd returned %q, %v, want the mis-cased %q; this case is not exercising the fold", cwd, err, miscased)
	}
	assertFolds(t, root, filepath.Join(miscased, "SRC", "a.cs"))
	name := filepath.Join("SRC", "a.cs")

	_, err := root.named(name, dirNames{})

	assertUnresolvedReason(t, err, "is not spelled as the file on disk is")
}

// placement names itself for the failure messages the containment cases print,
// so a case that goes red says "folded" rather than "2". Nothing in production
// reads the rendering, which is why it is asserted here rather than through a
// door.
func TestPlacementNamesItself(t *testing.T) {
	for _, testCase := range []struct {
		place placement
		want  string
	}{{inside, "inside"}, {folded, "folded"}, {outside, "outside"}} {
		if got := testCase.place.String(); got != testCase.want {
			t.Errorf("placement(%d).String() = %q, want %q", int(testCase.place), got, testCase.want)
		}
	}
}

// The acceptance half of the same policy on a case-sensitive filesystem, so
// deleting the typedTheRootPrefix call and refusing every fold goes red on the
// Linux merge gate rather than only on the macOS twin, which skips there.
func TestNamedAcceptsARelativeNameThatOnlyDescendsUnderACaseDifferingRootSpelling(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)
	touch(t, filepath.Join(real, "src", "a.cs"))
	t.Chdir(real)
	assertFolds(t, root, filepath.Join(real, "src", "a.cs"))

	rel, err := root.named(filepath.Join("src", "a.cs"), dirNames{})

	if err != nil || rel != "src/a.cs" {
		t.Errorf("named on a relative name under a case-differing root spelling returned %q, %v, want \"src/a.cs\", nil", rel, err)
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

	assertRootSpellingRefusal(t, err, name)
}

// The ordering the two arms sit in, pinned where every runner executes it. A
// folded path that is also a directory is a directory mistake first, so the
// developer who retypes the case gets the same message on the second try.
func TestNamedRefusesACaseDifferingRootSpellingOfTheRepoRootAsADirectory(t *testing.T) {
	root, real := symlinkedMiscasedRoot(t)

	_, err := root.named(real, dirNames{})

	assertUnresolvedReason(t, err, "is a directory, not a file")
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

	assertUnresolvedReason(t, err, "is a directory, not a file")
}

func TestNamedRefusesTheRepoRootAsADirectory(t *testing.T) {
	root := containmentRoot(t)

	_, err := root.named(root.Dir(), dirNames{})

	assertUnresolvedReason(t, err, "is a directory, not a file")
}

// The fold rule narrows what "outside" means for --files, so this pins that it
// still means something there. It needs no skip, since a file beside the root
// is outside it on either kind of filesystem.
func TestNamedStillRefusesAPathGenuinelyOutsideTheRoot(t *testing.T) {
	root := containmentRoot(t)
	name := touch(t, filepath.Join(filepath.Dir(root.Dir()), "outside.cs"))

	_, err := root.named(name, dirNames{})

	assertUnresolvedReason(t, err, "is outside the repo root")
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
	// Place lands nowhere for anything it reads as outside the root, so without
	// this the case stays green if the removal stops provoking the root stat and
	// the candidate simply reads as outside.
	if _, _, err := root.relativize(resolved); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("relativize on %s returned %v, want the root-stat fault; this case is not exercising the fault arm", resolved, err)
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
	// Name answers the absolute path for anything it reads as outside the root,
	// so without this the case stays green if the mode stops provoking a stat
	// failure and the fault arm is never reached.
	if _, _, err := root.relativize(resolveExisting(path)); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("relativize on %s returned %v, want a permission fault; this case is not exercising the fault arm", path, err)
	}

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

// rootReachedThroughAShortcut is a Root under an upper-cased symlink to a real
// lower-cased parent, returned with the real repo directory and with a second
// symlink that shortens the path to that parent's own parent. A shell sitting
// under the shortcut reports a working directory with fewer components than the
// root has, which is what separates counting the climb on the shell's spelling
// from counting it on the resolved directory.
func rootReachedThroughAShortcut(t *testing.T) (root Root, real, shortcut string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	under := mkdir(t, filepath.Join(tmp, "a", "b"))
	realParent := mkdir(t, filepath.Join(under, "dev"))
	miscasedParent := filepath.Join(under, "DEV")
	if _, err := os.Stat(miscasedParent); err == nil {
		t.Skipf("the filesystem folded %s onto %s already, so there is no link to make", miscasedParent, realParent)
	}
	if err := os.Symlink(realParent, miscasedParent); err != nil {
		t.Fatal(err)
	}
	shortcut = filepath.Join(tmp, "s")
	if err := os.Symlink(under, shortcut); err != nil {
		t.Fatal(err)
	}
	return Root{resolved: filepath.Join(miscasedParent, "repo")}, mkdir(t, filepath.Join(realParent, "repo")), shortcut
}

// miscasedParentRoot is a Root whose parent directory the filesystem also
// answers to under an upper-cased spelling, returned with that spelling. It is
// the folding filesystem's counterpart to rootUnderASymlinkedMiscasedParent,
// reaching the shape where the mis-cased component sits above the repo root
// rather than at it, and it skips where no such spelling reaches the parent.
//
// The root's own last component is spelled identically either way, so a --files
// name that spells "repo" and climbs no higher carries nothing mis-cased.
func miscasedParentRoot(t *testing.T) (Root, string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := mkdir(t, filepath.Join(tmp, "dev"))
	miscasedParent := filepath.Join(tmp, "DEV")
	if _, err := os.Stat(miscasedParent); err != nil || !sameDirectory(t, parent, miscasedParent) {
		t.Skipf("the filesystem is case sensitive, so %s does not name the same directory as %s", miscasedParent, parent)
	}
	root, err := NewRoot(mkdir(t, filepath.Join(parent, "repo")))
	if err != nil {
		t.Fatal(err)
	}
	return root, miscasedParent
}

// rootUnderASymlinkedMiscasedParent is a Root whose resolved directory sits
// under an upper-cased symlink to a real lower-cased parent, returned with the
// real repo directory beneath that parent. It is how a case-sensitive
// filesystem reaches the shape where the mis-cased component is above the repo
// root rather than at it: the root's own last component is spelled identically
// either way, and only the parent differs, which is the half a developer's
// --files text never supplies.
//
// Like symlinkedMiscasedRoot it builds the Root by hand, outside NewRoot's
// invariant, because NewRoot resolves the whole path and no Root it builds
// holds a name that can fold. A filesystem that folds case cannot build it,
// since the upper-cased parent already names the directory.
func rootUnderASymlinkedMiscasedParent(t *testing.T) (Root, string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realParent := mkdir(t, filepath.Join(tmp, "dev"))
	link := filepath.Join(tmp, "DEV")
	if _, err := os.Stat(link); err == nil {
		t.Skipf("the filesystem folded %s onto %s already, so there is no link to make", link, realParent)
	}
	if err := os.Symlink(realParent, link); err != nil {
		t.Fatal(err)
	}
	return Root{resolved: filepath.Join(link, "repo")}, mkdir(t, filepath.Join(realParent, "repo"))
}

// assertRootSpellingRefusal is the answer every mis-cased-root case wants back
// from named, the refusal that names the root rather than the location or the
// file, carrying the name as typed because that is the only spelling the
// developer can retype. The cases differ in the setup that reaches it, so each
// keeps its own and calls this.
func assertRootSpellingRefusal(t *testing.T, err error, name string) {
	t.Helper()
	unresolved := assertUnresolvedReason(t, err, "is not spelled as the repo root is")
	if unresolved.Name != name {
		t.Errorf("Name = %q, want the name as typed, %q", unresolved.Name, name)
	}
}

// assertUnresolvedReason is the refusal every named case that is about the path
// wants back, read as the typed error rather than as message text, and returned
// so a case that also cares which name it quotes can go on to check that.
func assertUnresolvedReason(t *testing.T, err error, want string) *UnresolvedError {
	t.Helper()
	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != want {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, want)
	}
	return unresolved
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
