package gitscope

import (
	"errors"
	"strings"
	"testing"
)

// TestOpenOutsideAGitRepositoryIsTyped pins issue 86's contract at the seam
// that owns it: starting a run where no repository resolves returns an
// OpenError kinded OpenNoRepo, so a caller can put it in its document rather
// than exiting 1 with empty stdout. The code that kind maps onto is the
// caller's, pinned in gate/cmd/metric-gate and by gate/test's golden.
func TestOpenOutsideAGitRepositoryIsTyped(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANGUAGE", "")
	t.Chdir(t.TempDir())

	_, err := Open()

	var open OpenError
	if !errors.As(err, &open) {
		t.Fatalf("Open() err = %v, want a gitscope.OpenError", err)
	}
	if open.Kind != OpenNoRepo {
		t.Errorf("Kind = %v, want OpenNoRepo", open.Kind)
	}
	if !strings.HasPrefix(open.Message, "could not find a git repository: ") {
		t.Errorf("Message = %q, want it to open with the could-not-find prefix", open.Message)
	}
	if !strings.Contains(open.Message, "not a git repository") {
		t.Errorf("Message = %q, want git's own complaint inside it", open.Message)
	}
	if !listsKind(OpenKinds(), open.Kind) {
		t.Errorf("OpenKinds() = %v, which omits %v, the kind Open just returned", OpenKinds(), open.Kind)
	}
}

func listsKind(kinds []OpenKind, want OpenKind) bool {
	for _, kind := range kinds {
		if kind == want {
			return true
		}
	}
	return false
}
