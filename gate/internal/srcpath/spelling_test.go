package srcpath

import (
	"os"
	"path/filepath"
	"testing"
)

// The spelling comparison is the one rule in this package a black-box case
// cannot reach on a case-sensitive filesystem, which is every Linux runner: a
// mis-cased name resolves to nothing there, so the run refuses it as absent
// long before named asks how the tree spells it. Reading the directory answers
// the question on either kind of filesystem, so these cases run everywhere and
// the branch is exercised on the machine that gates merges.
func TestSpelledAsOnDiskAcceptsTheSpellingTheTreeUses(t *testing.T) {
	root := spellingRoot(t)

	spelled, err := root.spelledAsOnDisk(filepath.Join("src", "Ordering", "OrderService.cs"))

	if err != nil || !spelled {
		t.Errorf("spelledAsOnDisk on the tree's own spelling returned %v, %v, want true, nil", spelled, err)
	}
}

func TestSpelledAsOnDiskRefusesADirectoryComponentInAnotherCase(t *testing.T) {
	root := spellingRoot(t)

	spelled, err := root.spelledAsOnDisk(filepath.Join("src", "ordering", "OrderService.cs"))

	if err != nil || spelled {
		t.Errorf("spelledAsOnDisk on a mis-cased directory returned %v, %v, want false, nil", spelled, err)
	}
}

func TestSpelledAsOnDiskRefusesAFilenameInAnotherCase(t *testing.T) {
	root := spellingRoot(t)

	spelled, err := root.spelledAsOnDisk(filepath.Join("src", "Ordering", "orderservice.cs"))

	if err != nil || spelled {
		t.Errorf("spelledAsOnDisk on a mis-cased filename returned %v, %v, want false, nil", spelled, err)
	}
}

func TestSpelledAsOnDiskReportsADirectoryItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which reads a directory with no read bit anyway")
	}
	root := spellingRoot(t)
	closed := filepath.Join(root.Dir(), "src", "Ordering")
	if err := os.Chmod(closed, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(closed, 0o755) })

	// A filesystem that will not answer is neither a match nor a mismatch, and
	// reported as a mismatch it would tell the developer their spelling is
	// wrong when the tree never said so.
	_, err := root.spelledAsOnDisk(filepath.Join("src", "Ordering", "OrderService.cs"))

	if err == nil {
		t.Error("spelledAsOnDisk on a directory it cannot read returned no error, want the filesystem's own cause")
	}
}

// spellingRoot builds a throwaway tree holding src/Ordering/OrderService.cs and
// returns it as a Root, resolved the way the gate resolves the repo root.
func spellingRoot(t *testing.T) Root {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src", "Ordering"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "Ordering", "OrderService.cs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
