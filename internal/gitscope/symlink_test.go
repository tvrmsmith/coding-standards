package gitscope

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

const (
	fixtureOrigin = "src/Origin.cs"
	fixtureTarget = "src/Target.cs"
	fixtureMoved  = "src/Moved.cs"
	fixtureLink   = "src/Link.cs"
)

// An added symbolic link passes `--diff-filter=ACM` as status A, so the
// letters alone cannot keep it out of the changed set. Left in, the extractor
// follows it and reports the target's spans under the link's path, which is
// the unknown changed method issue 109 reported with no edit that clears it.
// TouchedLines drops the link.
//
// The same fixture pins issue 110. Target.cs carries Origin.cs's content byte
// for byte, so a digest read through the link claims that content against the
// deleted blob: two adds claim one delete, counting drops neither, and
// Moved.cs comes back measured as a whole-file add rather than as the move it
// is. Move detection leaves an added link out, so the link claims nothing and
// the delete explains the one add that is left.
func TestTouchedLinesDropsAnAddedSymlinkAndStillSeesTheMoveBesideIt(t *testing.T) {
	repo, head := linkAndMoveFixture(t)
	base := Base{Ref: "HEAD", Commit: head}
	// The link really did arrive as an add carrying a link's mode, so the case
	// pins the drop rather than a git that never listed the path at all.
	records, err := repo.rawChanges(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(records, func(record rawRecord) bool {
		return record.Path == srcpath.Path(fixtureLink) && record.Status == "A" && record.NewMode == symlinkMode
	}) {
		t.Fatalf("the raw listing %v holds no added link at %s, want the fixture to stage one", records, fixtureLink)
	}

	touched, err := repo.TouchedLines(base)

	if err != nil {
		t.Fatalf("TouchedLines over an added link and a move errored %v, want an empty changed set", err)
	}
	if len(touched) != 0 {
		t.Errorf("TouchedLines named %v, want the link dropped for holding no source and %s dropped as a pure move", touchedPaths(touched), fixtureMoved)
	}
}

// --staged is the other scope reading the same repository state, and it reads
// every added path's new side out of the index with `cat-file` rather than off
// disk. It has to answer what the working-tree scope answers: the link dropped
// for holding no source, and Moved.cs dropped as the move of Origin.cs, which
// needs the new-side object id of each add. Asking for the old side instead
// hands `cat-file` an added path's all-zero id, and no add is ever digested.
func TestTouchedLinesStagedDropsTheSameAddedSymlinkAndMove(t *testing.T) {
	repo, head := linkAndMoveFixture(t)
	// The working tree holds a real source file where the index holds the link,
	// so the two scopes have different answers and a --staged run that read the
	// working tree would come back naming the link's path.
	if err := os.Remove(repo.root.Abs(srcpath.Path(fixtureLink))); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, repo.Root().Dir(), fixtureLink, "// link\n")
	onDisk, err := repo.TouchedLines(Base{Ref: "HEAD", Commit: head})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(touchedPaths(onDisk), []string{fixtureLink}) {
		t.Fatalf("the working-tree scope names %v, want only %s so the two scopes disagree", touchedPaths(onDisk), fixtureLink)
	}

	touched, err := repo.TouchedLines(Base{Ref: "HEAD", Commit: head, Staged: true})

	if err != nil {
		t.Fatalf("TouchedLines --staged over an added link and a move errored %v, want an empty changed set", err)
	}
	if len(touched) != 0 {
		t.Errorf("TouchedLines --staged named %v, want the link dropped for holding no source and %s dropped as a pure move", touchedPaths(touched), fixtureMoved)
	}
}

// A link already committed and then retargeted arrives as status M, not A, so
// it reaches the changed set by a second route the same drop has to close. The
// patch carries the one line the link holds, and an extractor handed the path
// follows the new target and reports its spans under the link's name, which is
// issue 109's failure from an edit rather than from an add. The drop keys on
// the new side's mode, so the status it arrives under does not matter.
func TestTouchedLinesDropsARetargetedSymlink(t *testing.T) {
	repo, head := retargetedLinkFixture(t)
	base := Base{Ref: "HEAD", Commit: head}
	// The link really did arrive as a modification carrying a link's mode, so
	// the case pins the drop rather than a git that listed nothing.
	records, err := repo.rawChanges(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(records, func(record rawRecord) bool {
		return record.Path == srcpath.Path(fixtureLink) && record.Status == "M"
	}) {
		t.Fatalf("the raw listing %v holds no modified %s, want the fixture to retarget the committed link", records, fixtureLink)
	}

	touched, err := repo.TouchedLines(base)

	if err != nil {
		t.Fatalf("TouchedLines over a retargeted link errored %v, want an empty changed set", err)
	}
	if len(touched) != 0 {
		t.Errorf("TouchedLines named %v, want %s dropped for holding no source", touchedPaths(touched), fixtureLink)
	}
}

// retargetedLinkFixture commits a link beside the two files it can point at,
// then repoints it on disk. A link's blob holds the target path rather than
// any of the target's content, so writing a different path into it is a real
// change to the link's own blob rather than a no-op git would leave out of the
// diff.
func retargetedLinkFixture(t *testing.T) (Repo, string) {
	t.Helper()

	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet")
	writeFixtureFile(t, dir, fixtureOrigin, "// origin\n")
	writeFixtureFile(t, dir, fixtureTarget, "// target\n")
	if err := os.Symlink("Origin.cs", filepath.Join(dir, filepath.FromSlash(fixtureLink))); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "initial")
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(fixtureLink))); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Target.cs", filepath.Join(dir, filepath.FromSlash(fixtureLink))); err != nil {
		t.Fatal(err)
	}

	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repo{root: root}
	return repo, fixtureHead(t, repo)
}

// linkAndMoveFixture stages an added link beside a pure move, with the link's
// target holding the moved file's content byte for byte, so a digest read
// through the link is the one that mismeasures the move. The index and the
// working tree hold the same state, which is what lets the two scopes be
// asked the same question.
func linkAndMoveFixture(t *testing.T) (Repo, string) {
	t.Helper()
	body := "// one\n// two\n// three\n"

	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet")
	writeFixtureFile(t, dir, fixtureOrigin, body)
	writeFixtureFile(t, dir, fixtureTarget, body)
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "initial")
	writeFixtureFile(t, dir, fixtureMoved, body)
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(fixtureOrigin))); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Target.cs", filepath.Join(dir, filepath.FromSlash(fixtureLink))); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", "--all")

	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repo{root: root}
	return repo, fixtureHead(t, repo)
}

// Skipping an added link is not the same answer as digesting the link's own
// text, and the fixtures above cannot tell the two apart: no deleted blob in
// them holds the path a link points at (issue 145). Here Origin.cs holds
// exactly that path, "Target.cs", and moves to Moved.cs beside a link to
// Target.cs. A digest of the link's text, os.Readlink on disk or `cat-file` on
// the link's blob under --staged, makes the link a second claimant of the one
// deleted blob, so counting drops neither add and Moved.cs comes back measured
// as a whole-file add. The skip leaves the move with its one claimant. The
// content carries no trailing newline because a link's text has none, and
// squashedDigest keeps line structure, so "Target.cs\n" would never collide.
func TestTouchedLinesKeepsTheMoveWhenAnAddedLinkCouldClaimTheSameDeletedBlob(t *testing.T) {
	cases := []struct {
		name   string
		staged bool
	}{
		{"working tree", false},
		{"staged", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo, head := linkClaimsMovedBlobFixture(t)

			touched, err := repo.TouchedLines(Base{Ref: "HEAD", Commit: head, Staged: c.staged})

			if err != nil {
				t.Fatalf("TouchedLines over an added link naming the same content as a deleted blob errored %v, want an empty changed set", err)
			}
			if len(touched) != 0 {
				t.Errorf("TouchedLines named %v, want %s dropped as a pure move and the link dropped for holding no source", touchedPaths(touched), fixtureMoved)
			}
		})
	}
}

// linkClaimsMovedBlobFixture stages a pure move of a file whose whole content
// is the text a link to Target.cs holds, beside such a link, with the index and
// the working tree holding the same state.
func linkClaimsMovedBlobFixture(t *testing.T) (Repo, string) {
	t.Helper()

	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet")
	writeFixtureFile(t, dir, fixtureTarget, "// target\n")
	writeFixtureFile(t, dir, fixtureOrigin, "Target.cs")
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "initial")
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(fixtureOrigin))); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, dir, fixtureMoved, "Target.cs")
	if err := os.Symlink("Target.cs", filepath.Join(dir, filepath.FromSlash(fixtureLink))); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", "--all")

	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repo{root: root}
	return repo, fixtureHead(t, repo)
}

// touchedPaths is the changed set's paths in a fixed order, so a failure reads
// the same whichever order the map ranges in.
func touchedPaths(touched map[srcpath.Path][]int) []string {
	var paths []string
	for path := range touched {
		paths = append(paths, path.String())
	}
	sort.Strings(paths)
	return paths
}
