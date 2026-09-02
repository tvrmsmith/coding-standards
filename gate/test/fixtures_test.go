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

// indented prefixes every non-empty line of body, which is exactly the
// difference `git diff -w` is defined to ignore.
func indented(body, prefix string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// boundaryFixture lays out one complexity 30 method whose thirty
// instrumentable lines are covered `covered` times, which is how the two
// threshold cases put the same span either side of a score of exactly 30.
func boundaryFixture(t *testing.T, f *fixture, covered int) {
	t.Helper()
	const boundary = "src/Ordering/Boundary.cs"
	knot := span{File: boundary, Name: "Boundary.Knot", StartLine: 10, EndLine: 40, Complexity: 30}

	f.write(boundary, csharpFile(60))
	f.commitAll("initial")
	f.touchLine(boundary, 20)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: boundary, lines: spanCoverage(11, 30, covered)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(boundary), []span{knot}),
	}
}

// plantUnrunnableExtractor puts a file under the extractor's name in dir that
// is present but carries no execute bit, which is the misinstall the "could
// not be run" cause exists to tell apart from an absent binary.
func plantUnrunnableExtractor(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root, which some platforms let exec a file with no execute bit")
	}
	if err := os.WriteFile(filepath.Join(dir, extractorName), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// touchLine rewrites the file at rel so exactly one line differs, which the
// diff reports as a single touched line.
func (f *fixture) touchLine(rel string, line int) {
	f.t.Helper()
	f.write(rel, replaceLine(f.read(rel), line, fmt.Sprintf("// line %d, edited", line)))
}

// deleteLines removes the one-based lines from..to of the file at rel, which
// the diff reports as a zero-length new-side hunk rather than as touched
// lines of its own.
func (f *fixture) deleteLines(rel string, from, to int) {
	f.t.Helper()
	lines := strings.Split(f.read(rel), "\n")
	f.write(rel, strings.Join(append(append([]string{}, lines[:from-1]...), lines[to:]...), "\n"))
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

// denyRead creates a directory at rel that the process cannot read, so the
// coverage walk hits a permission error on it. Root ignores the mode, so a
// case relying on this skips there.
func (f *fixture) denyRead(rel string) {
	f.t.Helper()
	if os.Geteuid() == 0 {
		f.t.Skip("running as root, which reads a mode 0 directory anyway")
	}
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(full, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chmod(full, 0o000); err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { os.Chmod(full, 0o755) })
}

// failedToParse marks the named file as one the extractor could not parse.
func failedToParse(file string) []fileStatus {
	return []fileStatus{{File: file, Status: "failed"}}
}

// addSubmoduleGitlink commits a mode 160000 entry at rel pointing at the
// fixture's own HEAD. That is what a submodule is in the index, and it needs no
// second repository on disk. The commit is made without `git add -A`, which
// would drop an entry that has no working-tree file behind it.
func (f *fixture) addSubmoduleGitlink(rel string) {
	f.t.Helper()
	f.git("update-index", "--add", "--cacheinfo", "160000,"+f.git("rev-parse", "HEAD")+","+rel)
	f.git("commit", "--quiet", "-m", "add submodule "+rel)
}

// removeSubmoduleGitlink stages the removal of the gitlink at rel.
func (f *fixture) removeSubmoduleGitlink(rel string) {
	f.t.Helper()
	f.git("update-index", "--force-remove", rel)
}

// copyFile duplicates the file at src to dst inside the fixture, byte for
// byte, which is how a case produces a second added path carrying content the
// diff also deleted.
func (f *fixture) copyFile(src, dst string) {
	f.t.Helper()
	f.write(dst, f.read(src))
}

// externalDiffScript writes an executable that prints nothing and exits 0, the
// shape of an external diff driver that hides every change from the parser.
func externalDiffScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "silent-diff")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
