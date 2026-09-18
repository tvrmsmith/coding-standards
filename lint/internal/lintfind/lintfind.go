// Package lintfind holds the finding currency shared by every language's
// linter, plus the parsers that read each linter's own report format into
// it. SARIF 2.1, the shape Roslyn's <ErrorLog> emits, is the first parser.
package lintfind

import "github.com/tvrmsmith/coding-standards/internal/srcpath"

// Finding is one diagnostic, in the one shape every language reports into.
type Finding struct {
	Rule         string
	Message      string
	Severity     string
	IgnoresScope bool
	Locations    []Location
}

// Location is a span in one file, inclusive of both lines.
type Location struct {
	Path      srcpath.Path
	StartLine int
	EndLine   int
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
