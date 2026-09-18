package report

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInternalErrorGoldenMatchesTheDocument pins
// gate/test/golden/internal_error.toon against a Document built directly,
// rather than against the real binary the way the black-box suite in
// gate/test pins every other golden. That suite drives gate/test's own
// fixtures through the real binary, and no fifth gitscope.OpenKind exists for
// it to hit; CodeInternalError is unreachable by construction from any
// OpenKind gitscope declares today (openkinds_test.go in internal/gitscope
// pins that OpenKinds is exactly the kinds openCode maps). Document is plain
// data, so building one here and rendering it is the only way to pin the
// shape this code's golden takes without inventing a kind that does not
// exist.
func TestInternalErrorGoldenMatchesTheDocument(t *testing.T) {
	doc := Document{
		Scope: "merge-base",
		Base:  nil,
		Failure: &Failure{
			Code:    CodeInternalError,
			Message: "the gate has no error code for gitscope.OpenKind 4",
		},
	}

	body, err := doc.Stdout()
	if err != nil {
		t.Fatalf("Stdout(): %v", err)
	}

	want, err := os.ReadFile(filepath.Join("..", "..", "test", "golden", "internal_error.toon"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(want) {
		t.Errorf("Stdout() = %q, want %q", body, want)
	}
}
