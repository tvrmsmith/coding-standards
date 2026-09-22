package lintfind

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// TestParseESLintTwoFiles pins E1: the two-file skeleton example, both
// filePath values rewritten under the test root, parses to three Findings
// and no error. ESLint blocks severity 1 and 2 alike, per ADR 0010, and
// bad.js's fatal parse-error message is its own UNPARSED finding.
func TestParseESLintTwoFiles(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "a.js")
	writeSource(t, root, "bad.js")
	aPath := jsonEscape(root.Abs(srcpath.Path("a.js")))
	badPath := jsonEscape(root.Abs(srcpath.Path("bad.js")))

	doc := `[
		{"filePath":"` + aPath + `","messages":[
			{"ruleId":"no-unused-vars","severity":2,"message":"'x' is assigned a value but never used.","line":1,"column":7,"nodeType":"Identifier","messageId":"unusedVar","endLine":1,"endColumn":8},
			{"ruleId":"no-undef","severity":1,"message":"'foo' is not defined.","line":2,"column":1,"endLine":2,"endColumn":4}
		],"suppressedMessages":[],"errorCount":1,"warningCount":1},
		{"filePath":"` + badPath + `","messages":[
			{"ruleId":null,"nodeType":null,"fatal":true,"severity":2,"message":"Parsing error: Unexpected token","line":2,"column":1}
		],"suppressedMessages":[],"errorCount":1,"fatalErrorCount":1}
	]`

	findings, dropped, err := ParseESLint(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none", dropped)
	}
	if len(findings) != 3 {
		t.Fatalf("len(findings) = %d, want 3: %#v", len(findings), findings)
	}
	want := []Finding{
		{
			Rule:      "no-unused-vars",
			Message:   "'x' is assigned a value but never used.",
			Severity:  "error",
			Locations: []Location{{Path: srcpath.Path("a.js"), StartLine: 1, StartColumn: 7, EndLine: 1}},
		},
		{
			Rule:      "no-undef",
			Message:   "'foo' is not defined.",
			Severity:  "warning",
			Locations: []Location{{Path: srcpath.Path("a.js"), StartLine: 2, StartColumn: 1, EndLine: 2}},
		},
	}
	if !reflect.DeepEqual(findings[:2], want) {
		t.Errorf("findings[:2] = %#v, want %#v", findings[:2], want)
	}
	if findings[2].Rule != "UNPARSED" || !findings[2].IgnoresScope {
		t.Errorf("findings[2] = %#v, want an UNPARSED finding with IgnoresScope true", findings[2])
	}
}

// TestParseESLintNoEndLineFallsBackToLine pins E2: a message with no endLine
// parses to EndLine equal to line.
func TestParseESLintNoEndLineFallsBackToLine(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "a.js")
	path := jsonEscape(root.Abs(srcpath.Path("a.js")))

	doc := `[{"filePath":"` + path + `","messages":[{"ruleId":"no-undef","severity":2,"message":"m","line":3,"column":5}]}]`
	findings, _, err := ParseESLint(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if got := findings[0].Locations[0].EndLine; got != 3 {
		t.Errorf("EndLine = %d, want 3", got)
	}
}

// TestParseESLintEndLineSpansTheWholeMessage pins that a message whose
// endLine is past its line keeps the whole span, not just the first line.
// ADR 0010 scopes on a span holding a touched line, so collapsing this to
// the start line would silently drop a rule like no-unreachable over a block
// whose opening line the change never wrote.
func TestParseESLintEndLineSpansTheWholeMessage(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "a.js")
	path := jsonEscape(root.Abs(srcpath.Path("a.js")))

	doc := `[{"filePath":"` + path + `","messages":[{"ruleId":"no-unreachable","severity":2,"message":"m","line":3,"column":5,"endLine":7,"endColumn":2}]}]`
	findings, _, err := ParseESLint(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	want := Location{Path: srcpath.Path("a.js"), StartLine: 3, StartColumn: 5, EndLine: 7}
	if got := findings[0].Locations[0]; got != want {
		t.Errorf("location = %#v, want %#v", got, want)
	}
}

// TestParseESLintFatalMessageIsUnparsed pins E3: ESLint's fatal parse-error
// message, ruleId null, parses to one UNPARSED Finding that keeps its
// location, since ESLint knows where the syntax error is, and ignores scope,
// since a file that would not parse was never linted.
func TestParseESLintFatalMessageIsUnparsed(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "bad.js")
	path := jsonEscape(root.Abs(srcpath.Path("bad.js")))

	doc := `[{"filePath":"` + path + `","messages":[
		{"ruleId":null,"nodeType":null,"fatal":true,"severity":2,"message":"Parsing error: Unexpected token","line":2,"column":1}
	]}]`
	findings, _, err := ParseESLint(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	want := Finding{
		Rule:         "UNPARSED",
		Severity:     "error",
		IgnoresScope: true,
		Message:      findings[0].Message,
		Locations:    []Location{{Path: srcpath.Path("bad.js"), StartLine: 2, StartColumn: 1, EndLine: 2}},
	}
	if !reflect.DeepEqual(findings[0], want) {
		t.Errorf("findings[0] = %#v, want %#v", findings[0], want)
	}
}

// TestParseESLintOnlySuppressedMessagesIsClean pins E4: a file result whose
// messages is empty but whose suppressedMessages holds an entry parses to no
// Findings at all. Honouring the target repo's own eslint-disable is the
// point.
func TestParseESLintOnlySuppressedMessagesIsClean(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "a.js")
	path := jsonEscape(root.Abs(srcpath.Path("a.js")))

	doc := `[{"filePath":"` + path + `","messages":[],"suppressedMessages":[
		{"ruleId":"no-unused-vars","severity":2,"message":"suppressed","line":1,"column":1}
	]}]`
	findings, dropped, err := ParseESLint(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(findings) != 0 || len(dropped) != 0 {
		t.Errorf("findings = %v, dropped = %v, want none", findings, dropped)
	}
}

// TestParseESLintDropsMessageOutsideRoot pins E5: a file result whose
// filePath is outside root parses to no Finding and one Dropped naming the
// filePath as the report wrote it.
func TestParseESLintDropsMessageOutsideRoot(t *testing.T) {
	root := testRoot(t)
	outside := t.TempDir() + "/a.js"

	doc := `[{"filePath":"` + jsonEscape(outside) + `","messages":[
		{"ruleId":"no-unused-vars","severity":2,"message":"m","line":1,"column":1}
	]}]`
	findings, dropped, err := ParseESLint(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
	want := []Dropped{{Rule: "no-unused-vars", URI: outside, Outside: true}}
	if !reflect.DeepEqual(dropped, want) {
		t.Errorf("dropped = %#v, want %#v", dropped, want)
	}
}

// TestParseESLintNoLineIsUnparsed pins E6: a message with no line at all
// parses to one UNPARSED Finding with no Locations.
func TestParseESLintNoLineIsUnparsed(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "a.js")
	path := jsonEscape(root.Abs(srcpath.Path("a.js")))

	doc := `[{"filePath":"` + path + `","messages":[{"ruleId":"no-undef","severity":2,"message":"m"}]}]`
	findings, _, err := ParseESLint(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(findings) != 1 || findings[0].Rule != "UNPARSED" || !findings[0].IgnoresScope {
		t.Fatalf("findings = %#v, want one UNPARSED, IgnoresScope true", findings)
	}
	if len(findings[0].Locations) != 0 {
		t.Errorf("Locations = %v, want none", findings[0].Locations)
	}
}

// TestParseESLintEmptyArrayIsValid pins E7: [] parses to no findings, no
// dropped and no error.
func TestParseESLintEmptyArrayIsValid(t *testing.T) {
	root := testRoot(t)

	findings, dropped, err := ParseESLint(strings.NewReader("[]"), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(findings) != 0 || len(dropped) != 0 {
		t.Errorf("findings = %v, dropped = %v, want none", findings, dropped)
	}
}

// TestParseESLintEmptyFilePathIsUnparsed pins E9: a file result naming no
// path at all is checked before placement and parses to one UNPARSED Finding
// with no Locations, and no Dropped entry. An empty filePath does not
// describe another tree, which Outside: true would claim, and dropping it
// silently is exactly what UNPARSED exists to prevent.
func TestParseESLintEmptyFilePathIsUnparsed(t *testing.T) {
	root := testRoot(t)

	doc := `[{"filePath":"","messages":[{"ruleId":"no-unused-vars","severity":2,"message":"'x' is assigned a value but never used.","line":1,"column":7}],"suppressedMessages":[]}]`
	findings, dropped, err := ParseESLint(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseESLint() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none", dropped)
	}
	if len(findings) != 1 || findings[0].Rule != "UNPARSED" || !findings[0].IgnoresScope {
		t.Fatalf("findings = %#v, want one UNPARSED, IgnoresScope true", findings)
	}
	if len(findings[0].Locations) != 0 {
		t.Errorf("Locations = %v, want none", findings[0].Locations)
	}
}

// TestParseESLintRejectsWrongShape pins E8: a top-level JSON object rather
// than an array, and text that is not JSON at all, are both refused as
// UnreadableReportError whose Format is "eslint".
func TestParseESLintRejectsWrongShape(t *testing.T) {
	root := testRoot(t)

	for _, doc := range []string{`{}`, "not json at all"} {
		_, _, err := ParseESLint(strings.NewReader(doc), root)
		var unreadable UnreadableReportError
		if !errors.As(err, &unreadable) {
			t.Fatalf("doc %s: ParseESLint() err = %v, want an UnreadableReportError", doc, err)
		}
		if unreadable.Format != "eslint" {
			t.Errorf("doc %s: Format = %q, want %q", doc, unreadable.Format, "eslint")
		}
	}
}
