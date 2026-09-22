package gitscope

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
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
	const (
		origin = "src/Origin.cs"
		target = "src/Target.cs"
		moved  = "src/Moved.cs"
		link   = "src/Link.cs"
	)
	body := "// one\n// two\n// three\n"

	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet")
	writeFixtureFile(t, dir, origin, body)
	writeFixtureFile(t, dir, target, body)
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "initial")
	writeFixtureFile(t, dir, moved, body)
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(origin))); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Target.cs", filepath.Join(dir, filepath.FromSlash(link))); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", "--all")

	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repo{root: root}
	head := fixtureHead(t, repo)
	// The link really did arrive as an add carrying a link's mode, so the case
	// pins the drop rather than a git that never listed the path at all.
	records, err := repo.rawChanges(Base{Ref: "HEAD", Commit: head}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(linkedPaths(records), srcpath.Path(link)) {
		t.Fatalf("the raw listing %v holds no added link at %s, want the fixture to stage one", records, link)
	}

	touched, err := repo.TouchedLines(Base{Ref: "HEAD", Commit: head})

	if err != nil {
		t.Fatalf("TouchedLines over an added link and a move errored %v, want an empty changed set", err)
	}
	if len(touched) != 0 {
		t.Errorf("TouchedLines named %v, want the link dropped for holding no source and %s dropped as a pure move", touchedPaths(touched), moved)
	}
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
