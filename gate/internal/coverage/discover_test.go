package coverage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// A discovered report is named by the path the walk reached it at. fs.WalkDir
// does not follow symlinks, so a report that is itself a link is a file inside
// the repo that the gate found inside the repo, and resolving it to name it
// would print the absolute path of whatever it points at into every failure
// quoting the report, into skipped_paths, and into the key discovery sorts on.
func TestDiscoverNamesASymlinkedReportByItsPathInsideTheRepo(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	results := filepath.Join(repo, "TestResults")
	if err := os.MkdirAll(results, 0o755); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(tmp, "build", "out.xml")
	if err := os.MkdirAll(filepath.Dir(elsewhere), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(elsewhere, []byte("<coverage/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(results, ReportName)); err != nil {
		t.Fatal(err)
	}
	root, err := srcpath.NewRoot(repo)
	if err != nil {
		t.Fatal(err)
	}

	sources, skipped, err := Discover(root)

	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want nothing skipped", skipped)
	}
	if len(sources) != 1 {
		t.Fatalf("Discover found %d reports, want 1", len(sources))
	}
	want := srcpath.Name("TestResults/" + ReportName)
	if sources[0].Name != want {
		t.Errorf("Name = %q, want the repo-relative %q", sources[0].Name, want)
	}
}
