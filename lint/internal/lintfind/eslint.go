package lintfind

import (
	"encoding/json"
	"io"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// eslintFileResult is one file's worth of ESLint's own JSON output. The top
// level of the report is an array of these.
type eslintFileResult struct {
	FilePath string          `json:"filePath"`
	Messages []eslintMessage `json:"messages"`
	// SuppressedMessages is read by nobody. ESLint puts the results an inline
	// eslint-disable turned off into that separate array, so reading Messages
	// alone honours the target repo's own suppressions, the same courtesy the
	// SARIF path pays by dropping an inSource suppression.
}

type eslintMessage struct {
	RuleID  *string `json:"ruleId"`
	Message string  `json:"message"`
	// Severity is 2 for an error and 1 for a warning. Both block, per ADR
	// 0010, so the numbers are read once here and the string is reportage.
	Severity int `json:"severity"`
	Line     int `json:"line"`
	Column   int `json:"column"`
	EndLine  int `json:"endLine"`
}

// ParseESLint reads ESLint's own JSON report and returns every message it
// can place inside root, alongside the messages it dropped as unplaceable.
func ParseESLint(r io.Reader, root srcpath.Root) ([]Finding, []Dropped, error) {
	var results []eslintFileResult
	if err := json.NewDecoder(r).Decode(&results); err != nil {
		return nil, nil, UnreadableReportError{Format: "eslint", Message: "malformed JSON: " + err.Error()}
	}

	var findings []Finding
	var dropped []Dropped
	for _, result := range results {
		// An empty filePath names no tree at all, checked before placement
		// is even attempted. Outside: true would claim the file result
		// describes another tree, which is not this result's problem, and
		// letting it fall through to root.Place would drop every message in
		// it silently, exactly what UNPARSED exists to prevent.
		if result.FilePath == "" {
			for _, msg := range result.Messages {
				findings = append(findings, unparsed("eslint", "file result carries no filePath", msg.Message))
			}
			continue
		}

		// filePath resolves once per file result, since every message in it
		// names the same file. ESLint resolves --stdin-filename against its
		// own cwd before reporting it, so the staged-content run
		// harness/linters/ts.sh makes lands on a real repo path too.
		path, ok := root.Place(result.FilePath).Inside()
		for _, msg := range result.Messages {
			if !ok {
				dropped = append(dropped, Dropped{Rule: ruleName(msg.RuleID), URI: result.FilePath, Outside: true})
				continue
			}
			findings = append(findings, placeESLintMessage(msg, path))
		}
	}
	return findings, dropped, nil
}

// ruleName reads a message's ruleId, which ESLint writes null for a fatal
// parse error that never reached rule checking.
func ruleName(ruleID *string) string {
	if ruleID == nil {
		return ""
	}
	return *ruleID
}

// placeESLintMessage turns one ESLint message into a Finding. A message with
// no ruleId, ESLint's own shape for a fatal parse error, or one with no line
// at all, is a message this package cannot read as an ordinary finding and
// becomes unparsed instead.
func placeESLintMessage(msg eslintMessage, path srcpath.Path) Finding {
	if msg.Line == 0 {
		// Line 1 column 1, so the report still points at the file and a waiver
		// for it stays keyed on that path. Only a file result with no filePath
		// at all takes the path-less route.
		return unparsed("eslint", "message carries no usable line", msg.Message,
			Location{Path: path, StartLine: 1, StartColumn: 1, EndLine: 1})
	}
	loc := eslintLocation(msg, path)
	if msg.RuleID == nil {
		return unparsed("eslint", "message carries no ruleId", msg.Message, loc)
	}

	severity := "warning"
	if msg.Severity == 2 {
		severity = "error"
	}
	return Finding{
		Rule:      *msg.RuleID,
		Message:   msg.Message,
		Severity:  severity,
		Locations: []Location{loc},
	}
}

// eslintLocation reads one message's line, column and endLine into a
// Location. EndLine is the message's own endLine when present and Line
// otherwise, and StartColumn is 1 when column is 0 or absent.
func eslintLocation(msg eslintMessage, path srcpath.Path) Location {
	endLine := msg.EndLine
	if endLine == 0 {
		endLine = msg.Line
	}
	column := msg.Column
	if column == 0 {
		column = 1
	}
	return Location{Path: path, StartLine: msg.Line, StartColumn: column, EndLine: endLine}
}
