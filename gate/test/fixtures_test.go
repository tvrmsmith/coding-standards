package gate_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// csharpFile renders a C# source file of exactly lines lines. The stub
// extractor's spans are canned, so only the line count matters: it has to be
// long enough for the spans a case declares and for the lines it touches.
func csharpFile(lines int) string {
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, "// line %d\n", i)
	}
	return b.String()
}

// touchLine rewrites the file at rel so exactly one line differs, which the
// diff reports as a single touched line.
func (f *fixture) touchLine(rel string, line int) {
	f.t.Helper()
	f.write(rel, replaceLine(f.read(rel), line, fmt.Sprintf("// line %d, edited", line)))
}

// read returns the current content of the file at rel.
func (f *fixture) read(rel string) string {
	f.t.Helper()
	body, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(body)
}

// replaceLine substitutes the one-based line n of body.
func replaceLine(body string, n int, replacement string) string {
	lines := strings.Split(body, "\n")
	lines[n-1] = replacement
	return strings.Join(lines, "\n")
}

// span is one method the stub extractor reports.
type span struct {
	File       string `json:"file"`
	Name       string `json:"name"`
	StartLine  int    `json:"startLine"`
	EndLine    int    `json:"endLine"`
	Complexity int    `json:"complexity"`
}

// fileStatus is one per-file parse result the stub extractor reports.
type fileStatus struct {
	File   string `json:"file"`
	Status string `json:"status"`
}

// extractorOutput renders the ADR 0006 wire response the stub emits.
func extractorOutput(t *testing.T, files []fileStatus, spans []span) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"files": files, "spans": spans})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// parsed marks every named file as successfully parsed.
func parsed(files ...string) []fileStatus {
	statuses := make([]fileStatus, 0, len(files))
	for _, file := range files {
		statuses = append(statuses, fileStatus{File: file, Status: "parsed"})
	}
	return statuses
}

// coverageLine is one instrumentable line of a Cobertura report.
type coverageLine struct {
	number int
	hits   int
}

// coverageClass is one <class> element: a filename and its lines.
type coverageClass struct {
	filename string
	lines    []coverageLine
}

// spanCoverage lays out count instrumentable lines from start, the first
// covered of them recorded as hit, which is how a case pins an exact
// coverage fraction for a span.
func spanCoverage(start, count, covered int) []coverageLine {
	lines := make([]coverageLine, 0, count)
	for i := 0; i < count; i++ {
		hits := 0
		if i < covered {
			hits = 1
		}
		lines = append(lines, coverageLine{number: start + i, hits: hits})
	}
	return lines
}

// cobertura renders a coverage report in coverlet's shape, with the source
// root in <sources> and each class's filename relative to it, which is the
// pairing ADR 0004 resolves paths from.
func cobertura(sourceRoot string, classes ...coverageClass) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<coverage line-rate="0" version="1.9" timestamp="1767225600">` + "\n")
	fmt.Fprintf(&b, "  <sources><source>%s</source></sources>\n", sourceRoot)
	b.WriteString("  <packages><package name=\"Ordering\"><classes>\n")
	for _, class := range classes {
		fmt.Fprintf(&b, "    <class name=%q filename=%q>\n      <lines>\n", class.filename, class.filename)
		for _, line := range class.lines {
			fmt.Fprintf(&b, "        <line number=\"%d\" hits=\"%d\" branch=\"false\" />\n", line.number, line.hits)
		}
		b.WriteString("      </lines>\n    </class>\n")
	}
	b.WriteString("  </classes></package></packages>\n</coverage>\n")
	return b.String()
}

// failedToParse marks the named file as one the extractor could not parse.
func failedToParse(file string) []fileStatus {
	return []fileStatus{{File: file, Status: "failed"}}
}
