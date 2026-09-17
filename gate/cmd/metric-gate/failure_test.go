package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/internal/gitscope"
)

// gitscope stopped naming report's codes when it moved above gate/ to be
// shared, so the code an unreadable diff carries into the document is chosen
// here and nowhere else. A black-box case would have to make a real git fail
// to reach it, which is the reason the mapping is driven directly instead.
func TestUnreadableDiffReachesTheDocumentAsDiffUnparseable(t *testing.T) {
	failure, ok := asFailure(gitscope.UnreadableDiffError{Message: "could not read the diff: git printed nothing"})

	if !ok {
		t.Fatalf("asFailure did not type an UnreadableDiffError, so the run would exit 1 with no document")
	}
	if failure.Code != report.CodeDiffUnparseable {
		t.Errorf("the code is %q, want %q", failure.Code, report.CodeDiffUnparseable)
	}
	if failure.Message != "could not read the diff: git printed nothing" {
		t.Errorf("the message is %q, want the one gitscope rendered", failure.Message)
	}
}

// An UnreadableDiffError arrives wrapped wherever a caller added context on the
// way up, and errors.As is what keeps the wrapping from costing the document
// its typed code.
func TestAWrappedUnreadableDiffStillReachesTheDocument(t *testing.T) {
	wrapped := fmt.Errorf("resolving the diff: %w", gitscope.UnreadableDiffError{Message: "could not read the diff: exit status 128"})

	failure, ok := asFailure(wrapped)

	if !ok || failure.Code != report.CodeDiffUnparseable {
		t.Errorf("asFailure on a wrapped UnreadableDiffError returned %v, %v, want a diff_unparseable failure", failure, ok)
	}
}

// Anything else stays an error, which is what makes the document's error block
// a list of causes the gate recognises rather than a place every stray failure
// lands with a code that does not describe it.
func TestAnUnrecognisedCauseIsNotTypedAsAFailure(t *testing.T) {
	if failure, ok := asFailure(errors.New("the extractor process was killed")); ok {
		t.Errorf("asFailure typed an unrecognised cause as %v, want it left as an error", failure)
	}
}
