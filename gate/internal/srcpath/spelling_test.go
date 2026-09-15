package srcpath

import (
	"errors"
	"io/fs"
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

	spelled, err := root.spelledAsOnDisk(filepath.Join("src", "Ordering", "OrderService.cs"), dirNames{})

	if err != nil || !spelled {
		t.Errorf("spelledAsOnDisk on the tree's own spelling returned %v, %v, want true, nil", spelled, err)
	}
}

func TestSpelledAsOnDiskRefusesADirectoryComponentInAnotherCase(t *testing.T) {
	root := spellingRoot(t)

	spelled, err := root.spelledAsOnDisk(filepath.Join("src", "ordering", "OrderService.cs"), dirNames{})

	if err != nil || spelled {
		t.Errorf("spelledAsOnDisk on a mis-cased directory returned %v, %v, want false, nil", spelled, err)
	}
}

func TestSpelledAsOnDiskRefusesAFilenameInAnotherCase(t *testing.T) {
	root := spellingRoot(t)

	spelled, err := root.spelledAsOnDisk(filepath.Join("src", "Ordering", "orderservice.cs"), dirNames{})

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
	spelled, err := root.spelledAsOnDisk(filepath.Join("src", "Ordering", "OrderService.cs"), dirNames{})

	if !errors.Is(err, fs.ErrPermission) || spelled {
		t.Errorf("spelledAsOnDisk on a directory it cannot read returned %v, %v, want false and a permission error", spelled, err)
	}
}

// relativize's third answer, the root and the candidate having no relative
// reading at all, is a second drive letter on Windows. No --files input on a
// unix runner reaches it, since filepath.Rel there fails only when one side is
// relative and the other absolute, which the public API cannot produce. A Root
// built by hand can, and it is the only way to hold the wording of the refusal
// apart from the "is outside the repo root" one next to it.
func TestRelativizeReportsARootTheCandidateHasNoRelativeReadingAgainst(t *testing.T) {
	root := Root{resolved: filepath.Join("relative", "root")}

	rel, place, err := root.relativize(filepath.Join(t.TempDir(), "OrderService.cs"))

	if err == nil || rel != "" || place != outside {
		t.Errorf("relativize against a relative root returned %q, %v, %v, want \"\", outside and an error", rel, place, err)
	}
}

func TestNamedSaysAPathHasNoReadingRelativeToTheRootRatherThanCallingItOutside(t *testing.T) {
	// The file exists, so the refusal cannot be the absence one, and it is
	// absolute, so it is not the working-directory join either.
	dir := t.TempDir()
	file := filepath.Join(dir, "OrderService.cs")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	root := Root{resolved: filepath.Join("relative", "root")}

	_, err := root.named(file, dirNames{})

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("named returned %v, want an *UnresolvedError", err)
	}
	if unresolved.Reason != "has no path relative to the repo root" {
		t.Errorf("Reason = %q, want %q", unresolved.Reason, "has no path relative to the repo root")
	}
	if unresolved.Name != file {
		t.Errorf("Name = %q, want the name as typed, %q", unresolved.Name, file)
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
