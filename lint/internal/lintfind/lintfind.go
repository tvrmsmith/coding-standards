// Package lintfind holds the finding currency shared by every language's
// linter, plus the parsers that read each linter's own report format into
// it. SARIF 2.1, the shape Roslyn's <ErrorLog> emits, is the first parser;
// golangci-lint's and ESLint's own JSON formats are the second and third.
package lintfind

import (
	"io"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// Parser reads one linter's own report format into the shared Finding
// currency, alongside the entries it could not place inside root.
// ParseSARIF, ParseGolangCI and ParseESLint all match this shape.
type Parser func(r io.Reader, root srcpath.Root) ([]Finding, []Dropped, error)

// Finding is one diagnostic, in the one shape every language reports into.
type Finding struct {
	Rule         string
	Message      string
	Severity     string
	IgnoresScope bool
	Locations    []Location
	// Language is the waiver-log key for the linter that produced this
	// finding. The parser stamps it, because the parser is the only thing
	// that knows.
	Language string
}

// The three waiver-log keys a Finding's Language carries. These strings are
// already written to users' waiver logs on disk, so they must not change.
const (
	LanguageCSharp = "csharp"
	LanguageGo     = "go"
	LanguageTS     = "ts"
)

// Location is a span in one file, inclusive of both lines. StartColumn is the
// only column: nothing scopes on columns (ADR 0003/0007 scope on lines) and
// the one output line lint-changed prints per finding carries a single
// column.
type Location struct {
	Path        srcpath.Path
	StartLine   int
	StartColumn int
	EndLine     int
}

// UnreadableReportError is a report this package refuses to read: malformed
// JSON, a schema version it does not speak, or a shape it cannot make sense
// of. Format names which report format was being read, so a caller with
// several parsers can tell them apart without parsing the message.
type UnreadableReportError struct {
	Format  string
	Message string
}

func (e UnreadableReportError) Error() string { return e.Message }

// unparsed is the finding a report entry this package cannot read reports
// under. Dropping it silently is the one thing a blocking gate must not do:
// a dropped finding is indistinguishable from a clean file.
//
// IgnoresScope is true for the same reason AD0001 sets it: an entry this
// package could not read is one it could not scope either, so a clean scope
// result under it proves nothing. It keeps whatever location the entry did
// carry, so the report can still point at the file; an entry with no usable
// location passes none and takes the Location.None waiver route already
// built for AD0001.
func unparsed(format, problem, detail string, locations ...Location) Finding {
	return Finding{
		Rule:         "UNPARSED",
		Severity:     "error",
		IgnoresScope: true,
		Message:      format + ": " + problem + ": " + detail,
		Locations:    locations,
	}
}
