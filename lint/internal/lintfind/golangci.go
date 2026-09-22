package lintfind

import (
	"encoding/json"
	"io"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// golangciReport is the subset of golangci-lint's own JSON output this parser
// reads. Report is golangci's own bookkeeping and is never read: go.sh
// already reads golangci's exit code to tell a broken run from a clean one,
// so nothing here needs to second-guess that.
type golangciReport struct {
	Issues []golangciIssue `json:"Issues"`
}

type golangciIssue struct {
	FromLinter string             `json:"FromLinter"`
	Text       string             `json:"Text"`
	Severity   string             `json:"Severity"`
	Pos        golangciPos        `json:"Pos"`
	LineRange  *golangciLineRange `json:"LineRange"`
}

type golangciPos struct {
	Filename string `json:"Filename"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

// golangciLineRange is read for To alone. From repeats Pos.Line on every
// issue golangci attaches a range to, and a span starts where its issue is
// reported.
type golangciLineRange struct {
	To int `json:"To"`
}

// ParseGolangCI reads golangci-lint's own JSON report and returns every issue
// it can place inside root, alongside the issues it dropped as unplaceable.
func ParseGolangCI(r io.Reader, root srcpath.Root) ([]Finding, []Dropped, error) {
	var report golangciReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, nil, UnreadableReportError{Format: "golangci", Message: "malformed JSON: " + err.Error()}
	}

	var findings []Finding
	var dropped []Dropped
	for _, issue := range report.Issues {
		finding, drop, ok := placeGolangCIIssue(issue, root)
		if !ok {
			dropped = append(dropped, drop)
			continue
		}
		findings = append(findings, finding)
	}
	// Stamped here, the single success return, rather than at
	// placeGolangCIIssue's construction sites (there are several, including
	// the unparsed() calls), so a future construction site inside this
	// parser cannot forget it.
	for i := range findings {
		findings[i].Language = LanguageGo
	}
	return findings, dropped, nil
}

// placeGolangCIIssue turns one golangci issue into a Finding, or says how it
// was dropped when Pos.Filename does not resolve inside root.
func placeGolangCIIssue(issue golangciIssue, root srcpath.Root) (Finding, Dropped, bool) {
	// An empty Filename names no tree at all, so it is checked before
	// placement is even attempted. Outside: true would claim the issue
	// describes another tree, which is not this issue's problem, and letting
	// it fall through to root.Place would drop it silently, exactly what
	// UNPARSED exists to prevent.
	if issue.Pos.Filename == "" {
		return unparsed("golangci", "issue carries no filename", issue.Text), Dropped{}, true
	}

	// Pos.Filename is absolute because harness/linters/go.sh runs golangci
	// with --path-mode abs.
	path, ok := root.Place(issue.Pos.Filename).Inside()
	if !ok {
		return Finding{}, Dropped{Rule: issue.FromLinter, URI: issue.Pos.Filename, Outside: true}, false
	}

	if issue.Pos.Line == 0 {
		// Line 1 column 1, so the report still points at the file and a waiver
		// for it stays keyed on that path. Only an entry with no path at all
		// takes the path-less route.
		return unparsed("golangci", "issue carries no usable line", issue.Text,
			Location{Path: path, StartLine: 1, StartColumn: 1, EndLine: 1}), Dropped{}, true
	}
	if issue.FromLinter == "" {
		return unparsed("golangci", "issue carries no linter", issue.Text, golangciLocation(issue, path)), Dropped{}, true
	}

	severity := issue.Severity
	if severity == "" {
		severity = "warning"
	}

	return Finding{
		Rule:         issue.FromLinter,
		Message:      issue.Text,
		Severity:     severity,
		IgnoresScope: ignoresGolangCIScope(issue.FromLinter),
		Locations:    []Location{golangciLocation(issue, path)},
	}, Dropped{}, true
}

// ignoresGolangCIScope reports whether linter names an analysis that never
// actually ran rather than a defect at a location. golangci reports a
// package that fails to compile as a single "typecheck" issue at Pos.Line 1,
// Pos.Column 0, so a clean scope result under it proves nothing: the package
// was never checked. This is the same reasoning the SARIF parser applies to
// AD0001, CS8032, CS8034 and CS9057.
func ignoresGolangCIScope(linter string) bool {
	return linter == "typecheck"
}

// golangciLocation reads one issue's Pos and LineRange into a Location.
// EndLine is LineRange.To when the issue carries a LineRange and Pos.Line
// otherwise, and StartColumn falls back to 1 when golangci writes 0, which
// the typecheck linter genuinely does.
//
// In practice EndLine always equals Pos.Line today. golangci-lint 2.13.2 does
// not carry analysis.Diagnostic.End into its JSON report, so LineRange is nil
// on every issue the pinned binary produces, including the spans the two
// custom analyzers under go/plugin set. go/README.md records the consequence
// as a known gap. The branch stays because it costs nothing and reads the
// spans straight away if golangci ever writes them.
//
// A To below Pos.Line is refused, since the scope filter wants
// line >= StartLine && line <= EndLine and such a span matches no line at
// all, so the finding would vanish with neither a Dropped entry nor an
// UNPARSED marker. Every sibling parser defends its span the same way.
func golangciLocation(issue golangciIssue, path srcpath.Path) Location {
	endLine := issue.Pos.Line
	if issue.LineRange != nil && issue.LineRange.To >= issue.Pos.Line {
		endLine = issue.LineRange.To
	}
	startColumn := issue.Pos.Column
	if startColumn == 0 {
		startColumn = 1
	}
	return Location{Path: path, StartLine: issue.Pos.Line, StartColumn: startColumn, EndLine: endLine}
}
