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
// is. Reading the link's own new side leaves the link claiming nothing but the
// target path it holds.
func TestTouchedLinesDropsAnAddedSymlinkAndStillSeesTheMoveBesideIt(t *testing.T) {
	repo, head := linkAndMoveFixture(t)
	base := Base{Ref: "HEAD", Commit: head}
	// The link really did arrive as an add carrying a link's mode, so the case
	// pins the drop rather than a git that never listed the path at all.
	records, err := repo.rawChanges(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(linkedPaths(records), srcpath.Path(fixtureLink)) {
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

	touched, err := repo.TouchedLines(Base{Ref: "HEAD", Commit: head, Staged: true})

	if err != nil {
		t.Fatalf("TouchedLines --staged over an added link and a move errored %v, want an empty changed set", err)
	}
	if len(touched) != 0 {
		t.Errorf("TouchedLines --staged named %v, want the link dropped for holding no source and %s dropped as a pure move", touchedPaths(touched), fixtureMoved)
	}
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
