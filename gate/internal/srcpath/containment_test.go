package srcpath

import (
	"errors"
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

// Every case below that goes through miscasedRoot skips on a case-sensitive
// filesystem, which is every Linux runner and so every CI job.
//
// State plainly what that leaves CI running of this rule: the negative half
// above, two distinct directories differing only in case reading as outside,
// plus the direct foldRootPrefix unit test below. The production success path,
// --files refusing a mis-cased root prefix by name, runs on macOS only. No way
// of writing these cases changes that, because the fold exists for a filesystem
// ubuntu-latest is not.
func TestRelativizeReadsAMisCasedRootPrefixAsFolded(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)

	// The components below the root are mis-cased too, and come back as the
	// candidate spells them. Only the root prefix folds; ADR 0004 still refuses
	// a case-only difference below it, one layer up in named and in the join.
	rel, place, err := root.relativize(filepath.Join(miscased, "SRC", "a.txt"))

	if err != nil || place != folded || rel != "SRC/a.txt" {
		t.Errorf("relativize on a mis-cased root prefix returned %q, %v, %v, want \"SRC/a.txt\", folded, nil", rel, place, err)
	}
}

// A direct unit test of foldRootPrefix's own contract, and only that. The input
// is one no production caller can hand it: Place, Name and named all resolve
// symlinks before they relativize, and that collapses the link back onto the
// root's own spelling, so a real run never reaches the fold this way.
//
// What CI covers of the rule is therefore the negative half, two distinct
// directories differing only in case reading as outside, which the case above
// pins. The production success path runs on a case-insensitive filesystem only,
// and every case that drives it through a public entry point skips on Linux.
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

	placed := root.Place(filepath.Join(miscased, "src", "a.cs"))

	rel, in := placed.Inside()
	if in || rel != "" {
		t.Errorf("Place on a mis-cased root prefix returned %q, %v, want \"\", false", rel, in)
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
// It skips on Linux because a case-differing symlink cannot stand in here. Name
// resolves before it relativizes, and resolveExisting's EvalSymlinks collapses
// the link back onto the root's own spelling, leaving nothing folded to read.
// Only a case-insensitive filesystem keeps the typed case on a non-symlink
// component.
func TestNameReadsAPathUnderAMisCasedRootPrefixAsItsResolvedAbsolutePath(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	path := filepath.Join(miscased, "TestResults", "coverage.cobertura.xml")

	name := root.Name(path)

	if name != Name(filepath.ToSlash(path)) {
		t.Errorf("Name under a mis-cased root prefix = %q, want the resolved path %q", name, filepath.ToSlash(path))
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
