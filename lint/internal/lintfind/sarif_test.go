package lintfind

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// TestParseSARIFRejectsMalformedJSON pins behaviour 10: a report that is not
// even JSON is refused as UnreadableReportError rather than surfacing an
// encoding/json error a caller has no code for.
func TestParseSARIFRejectsMalformedJSON(t *testing.T) {
	root := testRoot(t)

	_, _, err := ParseSARIF(strings.NewReader("{not json"), root)

	var unreadable UnreadableReportError
	if !errors.As(err, &unreadable) {
		t.Fatalf("ParseSARIF() err = %v, want an UnreadableReportError", err)
	}
	if unreadable.Format != "sarif" {
		t.Errorf("Format = %q, want %q", unreadable.Format, "sarif")
	}
}

// TestParseSARIFRejectsAbsentVersion pins behaviour 10's absent-version case:
// a well-formed SARIF document that never states its version is refused the
// same as a wrong one, rather than being read as 2.1.0 by default.
func TestParseSARIFRejectsAbsentVersion(t *testing.T) {
	root := testRoot(t)

	_, _, err := ParseSARIF(strings.NewReader(`{"runs": []}`), root)

	var unreadable UnreadableReportError
	if !errors.As(err, &unreadable) {
		t.Fatalf("ParseSARIF() err = %v, want an UnreadableReportError", err)
	}
	if unreadable.Format != "sarif" {
		t.Errorf("Format = %q, want %q", unreadable.Format, "sarif")
	}
}

// TestParseSARIFRejectsWrongVersion pins behaviour 10's wrong-version case: a
// version string other than 2.1.0 is refused rather than parsed as though it
// matched.
func TestParseSARIFRejectsWrongVersion(t *testing.T) {
	root := testRoot(t)

	_, _, err := ParseSARIF(strings.NewReader(`{"version": "2.0.0", "runs": []}`), root)

	var unreadable UnreadableReportError
	if !errors.As(err, &unreadable) {
		t.Fatalf("ParseSARIF() err = %v, want an UnreadableReportError", err)
	}
}

// TestParseSARIFEmptyRunsIsValid pins behaviour 9: "runs": [] is a valid
// empty report, not a malformed one, since a linter that ran clean still
// emits a log with no runs to report through.
func TestParseSARIFEmptyRunsIsValid(t *testing.T) {
	root := testRoot(t)

	findings, dropped, err := ParseSARIF(strings.NewReader(`{"version": "2.1.0", "runs": []}`), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %d, want 0", len(dropped))
	}
}

// TestParseSARIFEmptyResultsIsValid pins behaviour 9's other half: a run
// present with no results is a clean run, not a run this package refuses.
func TestParseSARIFEmptyResultsIsValid(t *testing.T) {
	root := testRoot(t)

	findings, dropped, err := ParseSARIF(strings.NewReader(`{"version": "2.1.0", "runs": [{"results": []}]}`), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %d, want 0", len(dropped))
	}
}

// TestParseSARIFRelativeURI pins behaviour 1, 3's relative-path half, and 5's
// endLine-present case: ruleId, message.text and level map straight across,
// and a plain relative artifactLocation.uri resolves through root.Place.
func TestParseSARIFRelativeURI(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/OrderService.cs")

	doc := `{
		"version": "2.1.0",
		"runs": [{
			"results": [{
				"ruleId": "TVRM0001",
				"level": "warning",
				"message": {"text": "Combine these assertions on the same object."},
				"locations": [{
					"physicalLocation": {
						"artifactLocation": {"uri": "src/OrderService.cs"},
						"region": {"startLine": 12, "endLine": 12}
					}
				}]
			}]
		}]
	}`

	findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %d, want 0", len(dropped))
	}
	want := []Finding{{
		Rule:     "TVRM0001",
		Message:  "Combine these assertions on the same object.",
		Severity: "warning",
		Locations: []Location{
			{Path: srcpath.Path("src/OrderService.cs"), StartLine: 12, EndLine: 12},
		},
	}}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings = %#v, want %#v", findings, want)
	}
}

// TestParseSARIFAbsoluteFileURIWithPercentEncoding pins behaviour 3's other
// half: an absolute file:// URI resolves through root.Place the same as a
// relative one, and percent-encoding in it, a space in a directory name, is
// decoded before the path is placed.
func TestParseSARIFAbsoluteFileURIWithPercentEncoding(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "my repo/src/Foo.cs")

	uri := "file://" + filepath.ToSlash(root.Dir()) + "/my%20repo/src/Foo.cs"
	doc := sarifDoc(t, uri, 5, 0)

	findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %d, want 0", len(dropped))
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	want := Location{Path: srcpath.Path("my repo/src/Foo.cs"), StartLine: 5, EndLine: 5}
	if findings[0].Locations[0] != want {
		t.Errorf("Locations[0] = %+v, want %+v", findings[0].Locations[0], want)
	}
}

// TestParseSARIFDropsResultWithNoPlaceableLocation pins behaviour 4 and 6: a
// location whose URI lands outside root, and one whose region has no
// startLine, are both unplaceable. A result whose only location is
// unplaceable is dropped whole and counted, not returned as a Finding with
// no Locations.
func TestParseSARIFDropsResultWithNoPlaceableLocation(t *testing.T) {
	root := testRoot(t)
	// Both files exist on disk, so neither result is rejected merely for
	// naming a path that is not there: containment alone rejects the first and
	// the absent region alone rejects the second.
	writeSource(t, root, "src/Foo.cs")
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "Foo.cs")
	if err := os.WriteFile(outsideFile, []byte("// outside the root\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() err = %v", err)
	}

	doc := `{
		"version": "2.1.0",
		"runs": [{
			"results": [
				{
					"ruleId": "TVRM0001",
					"message": {"text": "outside the root"},
					"locations": [{
						"physicalLocation": {
							"artifactLocation": {"uri": "file://` + filepath.ToSlash(outsideFile) + `"},
							"region": {"startLine": 1}
						}
					}]
				},
				{
					"ruleId": "TVRM0002",
					"message": {"text": "no region at all"},
					"locations": [{
						"physicalLocation": {
							"artifactLocation": {"uri": "src/Foo.cs"}
						}
					}]
				}
			]
		}]
	}`

	findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
	if len(dropped) != 2 {
		t.Errorf("dropped = %d, want 2", len(dropped))
	}
}

// TestParseSARIFSkeletonExample pins behaviour 2, 5's single-line case, and
// the assignment's own worked example verbatim: locations[] come first, then
// relatedLocations[] in order, and a region with startLine but no endLine is
// a single line.
func TestParseSARIFSkeletonExample(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/OrderService.cs")

	doc := `{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs": [
			{
				"tool": {"driver": {"name": "Microsoft (R) Visual C# Compiler", "version": "4.14.0"}},
				"results": [
					{
						"ruleId": "TVRM0001",
						"level": "warning",
						"message": {"text": "Combine these assertions on the same object."},
						"locations": [
							{"physicalLocation": {
								"artifactLocation": {"uri": "file://` + filepath.ToSlash(root.Dir()) + `/src/OrderService.cs"},
								"region": {"startLine": 12, "startColumn": 9, "endLine": 12, "endColumn": 40}}}
						],
						"relatedLocations": [
							{"physicalLocation": {
								"artifactLocation": {"uri": "file://` + filepath.ToSlash(root.Dir()) + `/src/OrderService.cs"},
								"region": {"startLine": 14, "startColumn": 9}}}
						]
					}
				],
				"columnKind": "utf16CodeUnits"
			}
		]
	}`

	findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %d, want 0", len(dropped))
	}
	want := []Finding{{
		Rule:     "TVRM0001",
		Message:  "Combine these assertions on the same object.",
		Severity: "warning",
		Locations: []Location{
			{Path: srcpath.Path("src/OrderService.cs"), StartLine: 12, EndLine: 12},
			{Path: srcpath.Path("src/OrderService.cs"), StartLine: 14, EndLine: 14},
		},
	}}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings = %#v, want %#v", findings, want)
	}
}

// TestParseSARIFDefaultsMissingLevelToWarning pins behaviour 1's default:
// SARIF 2.1 defaults an absent level to "warning", and this parser must
// match that rather than leaving Severity empty.
func TestParseSARIFDefaultsMissingLevelToWarning(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/Foo.cs")

	doc := sarifDoc(t, "src/Foo.cs", 1, 0)
	findings, _, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Severity != "warning" {
		t.Errorf("Severity = %q, want %q", findings[0].Severity, "warning")
	}
}

// TestParseSARIFIgnoresScopeForAnalyzerLoadFailures pins behaviour 7: a
// result under AD0001, CS8032, CS8034 or CS9057 sets IgnoresScope, since
// those rules mean the analyzer itself failed to load and a clean run under
// it proves nothing.
func TestParseSARIFIgnoresScopeForAnalyzerLoadFailures(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/Foo.cs")

	for _, rule := range []string{"AD0001", "CS8032", "CS8034", "CS9057"} {
		doc := `{"version": "2.1.0", "runs": [{"results": [{
			"ruleId": "` + rule + `",
			"message": {"text": "analyzer failed to load"},
			"locations": [{"physicalLocation": {
				"artifactLocation": {"uri": "src/Foo.cs"},
				"region": {"startLine": 1}}}]
		}]}]}`

		findings, _, err := ParseSARIF(strings.NewReader(doc), root)
		if err != nil {
			t.Fatalf("rule %s: ParseSARIF() err = %v, want nil", rule, err)
		}
		if len(findings) != 1 || !findings[0].IgnoresScope {
			t.Errorf("rule %s: IgnoresScope = %v, want true", rule, findings)
		}
	}

	// TVRM0001 is an ordinary rule and must not be flagged.
	doc := sarifDoc(t, "src/Foo.cs", 1, 0)
	findings, _, err := ParseSARIF(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if findings[0].IgnoresScope {
		t.Errorf("IgnoresScope = true for TVRM0001, want false")
	}
}

// TestParseSARIFKeepsScopeIgnoringRuleWithNoLocations pins the Location.None
// case behaviour 7 actually arrives in: Roslyn reports all four analyzer-load
// failures with no location at all, so the result carries no locations array.
// Dropping it as unplaceable would mean an analyzer that failed to load never
// blocked anything.
func TestParseSARIFKeepsScopeIgnoringRuleWithNoLocations(t *testing.T) {
	root := testRoot(t)

	for _, rule := range []string{"AD0001", "CS8032", "CS8034", "CS9057"} {
		doc := `{"version": "2.1.0", "runs": [{"results": [{
			"ruleId": "` + rule + `",
			"message": {"text": "analyzer failed to load"}
		}]}]}`

		findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)
		if err != nil {
			t.Fatalf("rule %s: ParseSARIF() err = %v, want nil", rule, err)
		}
		if len(dropped) != 0 {
			t.Errorf("rule %s: dropped = %d, want 0", rule, len(dropped))
		}
		if len(findings) != 1 {
			t.Fatalf("rule %s: len(findings) = %d, want 1", rule, len(findings))
		}
		if !findings[0].IgnoresScope {
			t.Errorf("rule %s: IgnoresScope = false, want true", rule)
		}
		if len(findings[0].Locations) != 0 {
			t.Errorf("rule %s: Locations = %v, want none", rule, findings[0].Locations)
		}
	}

	// An ordinary rule with no location is still dropped: nothing places it and
	// nothing exempts it.
	doc := `{"version": "2.1.0", "runs": [{"results": [{
		"ruleId": "TVRM0001", "message": {"text": "a finding"}
	}]}]}`
	findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(findings) != 0 || len(dropped) != 1 {
		t.Errorf("findings = %v, dropped = %d, want none and 1", findings, len(dropped))
	}
}

// TestParseSARIFDropsInSourceSuppressedResult pins that a diagnostic the
// target repo already turned off with a #pragma or a [SuppressMessage] never
// reaches a Finding. ErrorLog reports those; the console never printed them.
func TestParseSARIFDropsInSourceSuppressedResult(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/Foo.cs")

	doc := `{"version": "2.1.0", "runs": [{"results": [
		{
			"ruleId": "CA1822",
			"message": {"text": "suppressed in source"},
			"suppressions": [{"kind": "inSource"}],
			"locations": [{"physicalLocation": {
				"artifactLocation": {"uri": "src/Foo.cs"},
				"region": {"startLine": 1}}}]
		},
		{
			"ruleId": "TVRM0001",
			"message": {"text": "not suppressed"},
			"locations": [{"physicalLocation": {
				"artifactLocation": {"uri": "src/Foo.cs"},
				"region": {"startLine": 2}}}]
		}
	]}]}`

	findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %d, want 0: a suppressed result is not an unplaceable one", len(dropped))
	}
	if len(findings) != 1 || findings[0].Rule != "TVRM0001" {
		t.Fatalf("findings = %#v, want only TVRM0001", findings)
	}
}

// TestParseSARIFKeepsExternallySuppressedResult pins the other half: only an
// inSource suppression is the repo's own decision. A suppression of any other
// kind leaves the diagnostic in play.
func TestParseSARIFKeepsExternallySuppressedResult(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/Foo.cs")

	doc := `{"version": "2.1.0", "runs": [{"results": [{
		"ruleId": "CA1822",
		"message": {"text": "suppressed elsewhere"},
		"suppressions": [{"kind": "external"}],
		"locations": [{"physicalLocation": {
			"artifactLocation": {"uri": "src/Foo.cs"},
			"region": {"startLine": 1}}}]
	}]}]}`

	findings, _, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
}

// TestParseSARIFConcatenatesEveryRun pins behaviour 8: multiple runs
// contribute their results in run order, concatenated rather than only the
// first or last run being read.
func TestParseSARIFConcatenatesEveryRun(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/A.cs")
	writeSource(t, root, "src/B.cs")

	doc := `{"version": "2.1.0", "runs": [
		{"results": [{"ruleId": "TVRM0001", "message": {"text": "first"}, "locations": [
			{"physicalLocation": {"artifactLocation": {"uri": "src/A.cs"}, "region": {"startLine": 1}}}
		]}]},
		{"results": [{"ruleId": "TVRM0002", "message": {"text": "second"}, "locations": [
			{"physicalLocation": {"artifactLocation": {"uri": "src/B.cs"}, "region": {"startLine": 2}}}
		]}]}
	]}`

	findings, dropped, err := ParseSARIF(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseSARIF() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %d, want 0", len(dropped))
	}
	if len(findings) != 2 {
		t.Fatalf("len(findings) = %d, want 2", len(findings))
	}
	if findings[0].Rule != "TVRM0001" || findings[1].Rule != "TVRM0002" {
		t.Errorf("findings in order = [%s, %s], want [TVRM0001, TVRM0002]", findings[0].Rule, findings[1].Rule)
	}
}

// TestParseSARIFRefusesResultWithNoRuleID pins behaviour 11: a finding
// nobody can name cannot be waived, so a result with no ruleId is refused as
// UnreadableReportError rather than surfacing with an empty Rule.
func TestParseSARIFRefusesResultWithNoRuleID(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "src/Foo.cs")

	doc := `{"version": "2.1.0", "runs": [{"results": [{
		"message": {"text": "no rule id"},
		"locations": [{"physicalLocation": {
			"artifactLocation": {"uri": "src/Foo.cs"},
			"region": {"startLine": 1}}}]
	}]}]}`

	_, _, err := ParseSARIF(strings.NewReader(doc), root)

	var unreadable UnreadableReportError
	if !errors.As(err, &unreadable) {
		t.Fatalf("ParseSARIF() err = %v, want an UnreadableReportError", err)
	}
}

// sarifDoc builds a single-run, single-result SARIF document naming uri at
// startLine, with endLine included only when nonzero. It is the fixture
// builder for tests that only care about one result's placement.
func sarifDoc(t *testing.T, uri string, startLine, endLine int) string {
	t.Helper()
	region := `"startLine": ` + strconv.Itoa(startLine)
	if endLine != 0 {
		region += `, "endLine": ` + strconv.Itoa(endLine)
	}
	return `{"version": "2.1.0", "runs": [{"results": [{
		"ruleId": "TVRM0001",
		"message": {"text": "a finding"},
		"locations": [{"physicalLocation": {
			"artifactLocation": {"uri": "` + uri + `"},
			"region": {` + region + `}}}]
	}]}]}`
}

// writeSource creates path under root's directory, since srcpath.Root.Place
// refuses a candidate that does not resolve to a regular file on disk.
func writeSource(t *testing.T, root srcpath.Root, path string) {
	t.Helper()
	abs := filepath.Join(root.Dir(), filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatalf("MkdirAll() err = %v", err)
	}
	if err := os.WriteFile(abs, []byte("// test fixture\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() err = %v", err)
	}
}

// testRoot gives a srcpath.Root rooted at a fresh temp directory, the way a
// real caller resolves the repo root rather than faking the type.
func testRoot(t *testing.T) srcpath.Root {
	t.Helper()
	root, err := srcpath.NewRoot(t.TempDir())
	if err != nil {
		t.Fatalf("srcpath.NewRoot() err = %v", err)
	}
	return root
}
