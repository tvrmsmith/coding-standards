package coverage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestParseReportTrimsEachSource pins the read-and-trim parseReport promises
// its callers: a <source> that an XML formatter left as whitespace comes back
// blank, so the join and the erased-root check both see an empty root rather
// than a newline and some indentation.
func TestParseReportTrimsEachSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage.cobertura.xml")
	document := `<?xml version="1.0"?>
<coverage timestamp="1756000000">
  <sources>
    <source>
    </source>
    <source>  /repo  </source>
  </sources>
  <packages><package><classes>
    <class filename="src/Points.cs"><lines><line number="7" hits="3"/></lines></class>
  </classes></package></packages>
</coverage>
`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}

	parsed, err := parseReport(path)
	if err != nil {
		t.Fatalf("parseReport(%s): %v", path, err)
	}

	want := []string{"", "/repo"}
	if !slices.Equal(parsed.Sources, want) {
		t.Errorf("Sources = %q, want %q", parsed.Sources, want)
	}
	if parsed.Timestamp != "1756000000" {
		t.Errorf("Timestamp = %q, want %q", parsed.Timestamp, "1756000000")
	}
	if len(parsed.Classes) != 1 || parsed.Classes[0].Filename != "src/Points.cs" {
		t.Fatalf("Classes = %+v, want one class for src/Points.cs", parsed.Classes)
	}
	if got := parsed.Classes[0].Lines; len(got) != 1 || got[0].Number != 7 || got[0].Hits != 3 {
		t.Errorf("Lines = %+v, want line 7 with 3 hits", got)
	}
}

// TestParseReportReturnsTheReadError pins that a report which is not there
// surfaces the os error rather than an empty document, because parseCause
// renders that error into the refusal the operator reads.
func TestParseReportReturnsTheReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.cobertura.xml")

	if _, err := parseReport(path); !os.IsNotExist(err) {
		t.Errorf("parseReport(%s) error = %v, want a not-exist error", path, err)
	}
}
