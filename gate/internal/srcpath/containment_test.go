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

// Every fold case below this one skips on a case-sensitive filesystem, which
// is every Linux runner, because there a mis-cased root prefix names nothing
// and the answer is the outside one the case above pins. The branch they cover
// is reachable only where the filesystem folds, so they run on the developer's
// macOS laptop and the one above runs on CI. Between them both readings of the
// rule are exercised somewhere.
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

// This is the positive half of the fold rule on a case-sensitive filesystem,
// beside the negative half the two-distinct-directories case above pins there.
// A symlink whose last component differs from its target only in case reaches
// one directory under two spellings, and os.Stat follows it, so EqualFold and
// os.SameFile both hold and foldRootPrefix's success path runs on Linux CI. The
// macOS-only cases stay, because a symlink is not the same shape as a
// filesystem that folds every name.
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

// The fold rule narrows what "outside" means, so this pins that it still means
// something. It needs no skip, since a file beside the root is outside it on
// either kind of filesystem.
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

// Place is the coverage side, where nobody typed the path and there is nothing
// to retype, so a folded root prefix places the candidate rather than refusing
// it. The components below the root keep the candidate's own case, which is
// what leaves gate/test's case_only_path_difference at exit 1.
func TestPlaceAcceptsACandidateWhoseRootPrefixIsMisCased(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)
	touch(t, filepath.Join(root.Dir(), "src", "a.cs"))

	placed := root.Place(filepath.Join(miscased, "src", "a.cs"))

	rel, in := placed.Inside()
	if !in || rel != "src/a.cs" {
		t.Errorf("Place on a mis-cased root prefix returned %q, %v, want \"src/a.cs\", true", rel, in)
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

// Name's folded arm, which is the --coverage side of the fold rule: a report
// under a mis-cased root prefix is named repo-relative like an exact one, and
// not by an absolute machine path that reads as though it sat outside the repo.
//
// Unlike the relativize case above this one cannot be reached through a
// case-differing symlink, so it skips on Linux. Name resolves before it
// relativizes, and resolveExisting's EvalSymlinks collapses the link back onto
// the root's own spelling, leaving nothing folded to read. A non-symlink
// component that keeps the typed case is what a case-insensitive filesystem
// alone produces.
func TestNameReadsAPathUnderAMisCasedRootPrefixAsRepoRelative(t *testing.T) {
	root := containmentRoot(t)
	miscased := miscasedRoot(t, root)

	name := root.Name(filepath.Join(miscased, "TestResults", "coverage.cobertura.xml"))

	if name != "TestResults/coverage.cobertura.xml" {
		t.Errorf("Name under a mis-cased root prefix = %q, want %q", name, "TestResults/coverage.cobertura.xml")
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
