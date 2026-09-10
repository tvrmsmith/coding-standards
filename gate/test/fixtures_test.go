package gate_test

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/tvrmsmith/coding-standards/gate/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
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

// moveLinesBetween cuts the one-based lines from..to (inclusive) out of the
// file at src and inserts them into dst immediately after dst's line after.
// It copies the lines verbatim, so they are byte-identical at their new home.
// Both src and dst must already exist on disk, since the helper reads each
// one before it rewrites it, and they must name different files, since after
// would then be in post-cut coordinates.
func (f *fixture) moveLinesBetween(src string, from, to int, dst string, after int) {
	f.t.Helper()
	if src == dst {
		f.t.Fatalf("moveLinesBetween got %s for both sides; use moveLinesWithin", src)
	}
	srcLines := strings.Split(f.read(src), "\n")
	cut := slices.Clone(srcLines[from-1 : to])
	f.write(src, strings.Join(slices.Delete(srcLines, from-1, to), "\n"))

	dstLines := strings.Split(f.read(dst), "\n")
	f.write(dst, strings.Join(slices.Insert(dstLines, after, cut...), "\n"))
}

// moveLinesWithin cuts the one-based lines from..to (inclusive) out of the
// file at rel and reinserts them immediately after line after, both given in
// the file's coordinates before the cut. It copies the lines verbatim, so
// they are byte-identical at their new home.
func (f *fixture) moveLinesWithin(rel string, from, to, after int) {
	f.t.Helper()
	lines := strings.Split(f.read(rel), "\n")
	cut := slices.Clone(lines[from-1 : to])
	lines = slices.Delete(lines, from-1, to)
	if after > to {
		after -= to - from + 1
	}
	f.write(rel, strings.Join(slices.Insert(lines, after, cut...), "\n"))
}

// insertBlankLine puts an empty line into the file at rel after the one-based
// line after, which moves every line below it. It is the whitespace-only edit
// `git diff -w` still reports, since -w ignores whitespace inside a line and
// not a line the other side does not have at all.
func (f *fixture) insertBlankLine(rel string, after int) {
	f.t.Helper()
	f.write(rel, strings.Join(slices.Insert(strings.Split(f.read(rel), "\n"), after, ""), "\n"))
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

// otherService and otherRun are a second file and its one span, used by the
// --files cases, which need a file list longer than one. The span is not
// called `other`, because gate_test.go already binds that name locally twice
// and a package-level third meaning would shadow into both.
const otherService = "src/Ordering/Other.cs"

var otherRun = span{File: otherService, Name: "Other.Run", StartLine: 10, EndLine: 20, Complexity: 4}

// span is one method the stub extractor reports.
type span struct {
	File       string `json:"file"`
	Name       string `json:"name"`
	Signature  string `json:"signature"`
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
// pairing ADR 0004 resolves paths from. It stamps the report at the moment the
// case builds it, which is the moment coverlet would have written it, leaving
// coberturaStamped as the way a case asks for a stale one.
func cobertura(sourceRoot string, classes ...coverageClass) string {
	return renderCobertura(freshStamp(), []string{sourceRoot}, classes...)
}

// coberturaStamped is cobertura carrying the given root timestamp attribute,
// which is the producer's own clock and the value the staleness rule reads.
// An empty stamp omits the attribute, which is the report the rule cannot
// judge.
func coberturaStamped(stamp, sourceRoot string, classes ...coverageClass) string {
	return renderCobertura(stamp, []string{sourceRoot}, classes...)
}

// freshStamp is the current second, as a Cobertura timestamp attribute spells
// it. The real clock rather than any slack ahead of it, so no case comes to
// depend on the gate accepting a report claiming to have been produced in the
// future, which is the tolerance issue 30 exists to take away. Every case
// writes its report after the sources it describes, and equal seconds is not
// stale, so the current second is already fresh everywhere.
func freshStamp() string {
	return stampAt(time.Now())
}

// stampAt renders an instant as a Cobertura timestamp attribute spells it.
func stampAt(at time.Time) string {
	return strconv.FormatInt(at.Unix(), 10)
}

// editStamp is the whole second the file at rel was last modified, shifted by
// offset, as a Cobertura timestamp attribute spells it. The staleness rule
// compares whole seconds on both sides, so a case that means to sit exactly on
// that boundary has to read the source's own mtime rather than trust how fast
// it ran.
func (f *fixture) editStamp(rel string, offset time.Duration) string {
	f.t.Helper()
	return stampAt(f.modTime(rel).Add(offset))
}

// modTime is the truncated modification time of the file at rel.
func (f *fixture) modTime(rel string) time.Time {
	f.t.Helper()
	info, err := os.Stat(filepath.Join(f.root, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatal(err)
	}
	return info.ModTime().Truncate(time.Second)
}

// setModTime sets the modification time of the file at rel to at, which is how
// a case pins either the exact ordering of two edits or an exact tie between
// them, neither of which the filesystem's own stamping can be asked for.
func (f *fixture) setModTime(rel string, at time.Time) {
	f.t.Helper()
	if err := os.Chtimes(filepath.Join(f.root, filepath.FromSlash(rel)), at, at); err != nil {
		f.t.Fatal(err)
	}
}

// coberturaNoSources is cobertura with <sources/> empty, so every class
// filename stands alone. That document reads as an erased source root only
// when the filenames carry the /_/ placeholder DeterministicReport=true writes
// (issue 16, coverage_source_root_erased). Relative filenames with no <source>
// beside them are a different failure: nothing anchors them, so they build no
// candidate and the report is named as placing no class inside the root, code
// coverage_outside_repo in the shape the coverage_outside_repo_unanchored
// golden holds (issue 16). Absolute filenames carry their own root and are the
// legitimate coverlet shape.
// UseSourceLink=true is a different document again, keeping one <source> that
// is empty, so a case for it calls cobertura("").
func coberturaNoSources(classes ...coverageClass) string {
	return renderCobertura(freshStamp(), nil, classes...)
}

// renderCobertura is the document builder, taking the timestamp attribute and
// the <source> list directly. A case naming more than one source is a class
// with two in-root candidates (issue 16, file_ambiguous) or a report split
// across two checkouts. An empty stamp omits the timestamp attribute
// altogether, which is as close as this builder gets to the unjudgeable
// report: encoding/xml decodes an absent attribute and timestamp="" to the
// same empty string, so the gate cannot tell the two apart and neither
// spelling is worth a case of its own. Every case that is not about staleness
// passes freshStamp(), since issue 15 refuses a report older than the source
// it describes and a fixed stamp would make every fixture stale.
func renderCobertura(stamp string, sources []string, classes ...coverageClass) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	timestampAttr := ""
	if stamp != "" {
		timestampAttr = fmt.Sprintf(` timestamp="%s"`, xmlAttribute(stamp))
	}
	fmt.Fprintf(&b, `<coverage line-rate="0" version="1.9"%s>`+"\n", timestampAttr)
	if len(sources) == 0 {
		b.WriteString("  <sources/>\n")
	} else {
		b.WriteString("  <sources>")
		for _, source := range sources {
			fmt.Fprintf(&b, "<source>%s</source>", source)
		}
		b.WriteString("</sources>\n")
	}
	b.WriteString("  <packages><package name=\"Ordering\"><classes>\n")
	for _, class := range classes {
		name := xmlAttribute(class.filename)
		fmt.Fprintf(&b, "    <class name=\"%s\" filename=\"%s\">\n      <lines>\n", name, name)
		for _, line := range class.lines {
			fmt.Fprintf(&b, "        <line number=\"%d\" hits=\"%d\" branch=\"false\" />\n", line.number, line.hits)
		}
		b.WriteString("      </lines>\n    </class>\n")
	}
	b.WriteString("  </classes></package></packages>\n</coverage>\n")
	return b.String()
}

// xmlAttribute escapes a value for an XML attribute, which a filename holding
// a double quote needs and Go quoting would get wrong.
func xmlAttribute(value string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(value)); err != nil {
		panic(err)
	}
	return b.String()
}

// writeAbsolute puts content at an absolute path outside the fixture,
// creating parents, which is how a case builds the source tree of a second
// checkout that is not the git repo under test (issue 16,
// coverage_outside_repo).
func writeAbsolute(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// outsideRepoStderr is the coverage_outside_repo diagnostic as it reaches
// stderr, so the several cases that reach it name the two machine-specific
// paths once, in the same shape as the golden's holes.
func outsideRepoStderr(example, root string) string {
	return fmt.Sprintf("coverage report TestResults/coverage.cobertura.xml placed no class inside the repo root; "+
		"example path %s, repo root %s\n", example, root)
}

// outsideRepoUnanchoredStderr is the same diagnostic for a report whose first
// candidate no <source> anchored, where the quoted path is relative and the
// message has to say so rather than leaving it to read as a path inside the
// repo.
func outsideRepoUnanchoredStderr(example, root string) string {
	return fmt.Sprintf("coverage report TestResults/coverage.cobertura.xml placed no class inside the repo root; "+
		"example path %s, which no <source> anchored to an absolute path, repo root %s\n", example, root)
}

// namedOutsideRepoStderr is the coverage_outside_repo diagnostic for a case
// carrying more than one report, where outsideRepoStderr's hardcoded single
// path is no longer enough: the message has to say which report failed, not
// just that one did.
func namedOutsideRepoStderr(report, example, root string) string {
	return fmt.Sprintf("coverage report %s placed no class inside the repo root; example path %s, repo root %s\n", report, example, root)
}

// symlinkedDir creates link as a symlink to target, both absolute, and returns
// link. A case uses it to put a resolving indirection in a candidate's path,
// which is how the resolved reading of a path is told from the as-built one.
func symlinkedDir(t *testing.T, target, link string) string {
	t.Helper()
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("the filesystem does not allow symlinks: %v", err)
	}
	return link
}

// caseInsensitiveFilesystem reports whether dir's filesystem matches names
// case insensitively, which decides whether a case-only path difference is a
// difference the resolver can be asked about at all.
func caseInsensitiveFilesystem(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "case-probe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(probe)
	_, err := os.Stat(filepath.Join(dir, "CASE-PROBE"))
	switch {
	case err == nil:
		return true
	case errors.Is(err, fs.ErrNotExist):
		return false
	}
	t.Fatalf("probing %s for case sensitivity returned %v, which answers neither way", dir, err)
	return false
}

// resolvedPath is filepath.EvalSymlinks for a directory a case knows exists,
// used to take the indirection out of a temp root once, up front, so the paths
// a case builds under it are already the ones the gate will compare and a
// golden's {{EXAMPLE}}/{{ROOT}} hole is filled by joining rather than by
// running the gate's own resolver over the answer.
func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(resolved)
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

// denyReadKeepingEntry makes the existing directory at rel impossible to list
// while leaving it possible to enter, which is mode 0o111. A path through it
// still resolves and still stats, so this is what puts a --files name past
// every check that only walks the path and in front of the one that reads the
// directory to see how the tree spells its entries. Root ignores the mode, so a
// case relying on this skips there.
func (f *fixture) denyReadKeepingEntry(rel string) {
	f.t.Helper()
	if os.Geteuid() == 0 {
		f.t.Skip("running as root, which lists a directory with no read bit anyway")
	}
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.Chmod(full, 0o111); err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { os.Chmod(full, 0o755) })
}

// setExecutable turns the executable bit on for the file at rel, which is the
// one index-to-working-tree difference that is not content. A filesystem that
// does not carry the bit leaves the tree identical to the index and there is no
// difference for the case to be about, so it skips there, asked of git rather
// than guessed at from the operating system's name.
func (f *fixture) setExecutable(rel string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chmod(full, info.Mode()|0o111); err != nil {
		f.t.Fatal(err)
	}
	if f.git("diff", "--name-only", "--", rel) == "" {
		f.t.Skip("git does not record the executable bit here, so there is no mode-only difference to make")
	}
}

// symlinkTo puts a symbolic link at the repo-relative rel pointing at the
// absolute target, and skips the case on a filesystem that will not make one.
func (f *fixture) symlinkTo(target, rel string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Symlink(target, full); err != nil {
		f.t.Skipf("the filesystem does not allow symlinks: %v", err)
	}
}

// removeFile deletes rel from the working tree and leaves the index holding
// it, which is the unstaged deletion half of a divergence between the two.
func (f *fixture) removeFile(rel string) {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.root, filepath.FromSlash(rel))); err != nil {
		f.t.Fatal(err)
	}
}

// denyReadFile makes the file at rel unreadable, so reading the report fails
// on the file itself rather than on its contents. Root ignores the mode, so a
// case relying on this skips there.
func (f *fixture) denyReadFile(rel string) {
	f.t.Helper()
	if os.Geteuid() == 0 {
		f.t.Skip("running as root, which reads a mode 0 file anyway")
	}
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.Chmod(full, 0o000); err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { os.Chmod(full, 0o644) })
}

// readCause is the cause the gate renders when it cannot read the file at rel:
// the failing operation, then the operating system's own wording for the same
// failed read, stripped of the absolute path the way parseCause strips it. The
// wording belongs to the OS, "Access is denied." rather than "permission
// denied" on Windows, so a case pins the sentence the gate owns around the hole
// and asks the OS for the rest.
func (f *fixture) readCause(rel string) string {
	f.t.Helper()
	_, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		f.t.Fatalf("reading %s returned %v, which the case needs to be a path error", rel, err)
	}
	return pathErr.Op + ": " + pathErr.Err.Error()
}

// xmlUnmarshalCause is the cause the gate renders for a report that is not
// valid XML: encoding/xml's own error for the same bytes, which the gate only
// passes through and this repo does not own the wording of. It unmarshals into
// an empty struct rather than the gate's own report type, which is unexported,
// so it holds only for an error the decoder raises before it looks at the
// target at all, a syntax error. A well-formed document whose types do not fit
// would need the real target and is not what any case here feeds it.
func xmlUnmarshalCause(t *testing.T, body string) string {
	t.Helper()
	err := xml.Unmarshal([]byte(body), &struct{}{})
	if err == nil {
		t.Fatalf("encoding/xml accepted %q, which the case needs to be malformed", body)
	}
	return err.Error()
}

// failedToParse marks the named file as one the extractor could not parse.
func failedToParse(file string) []fileStatus {
	return []fileStatus{{File: file, Status: "failed"}}
}

// addSubmoduleGitlink commits a mode 160000 entry at rel pointing at the
// fixture's own HEAD. That is what a submodule is in the index, and it needs no
// second repository on disk.
func (f *fixture) addSubmoduleGitlink(rel string) {
	f.t.Helper()
	f.addGitlink(rel, f.git("rev-parse", "HEAD"))
}

// addGitlink commits a mode 160000 entry at rel naming objectID, whatever kind
// of object that id turns out to name. The commit is made without `git add -A`,
// which would drop an entry that has no working-tree file behind it.
func (f *fixture) addGitlink(rel, objectID string) {
	f.t.Helper()
	f.git("update-index", "--add", "--cacheinfo", "160000,"+objectID+","+rel)
	f.git("commit", "--quiet", "-m", "add gitlink "+rel)
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

// writeUTF16LE puts content at rel encoded as UTF-16LE behind a byte order
// mark, which is a C# source encoding Visual Studio still writes and which git
// calls binary, because every ASCII character carries a NUL byte beside it.
func (f *fixture) writeUTF16LE(rel, content string) {
	f.t.Helper()
	body := []byte{0xff, 0xfe}
	for _, unit := range utf16.Encode([]rune(content)) {
		body = append(body, byte(unit), byte(unit>>8))
	}
	f.write(rel, string(body))
}

// hideEditFilter writes an attributes file marking every .cs file filtered and
// an executable clean filter stripping back out exactly what touchLine writes
// in, and returns the two config values that install them. A clean filter runs
// on the working-tree side of every diff and no command-line flag turns one
// off, so a run that reads this config sees the two sides of the change as
// equal and measures nothing.
func hideEditFilter(t *testing.T) (attributesFile, cleanCommand string) {
	t.Helper()
	dir := t.TempDir()
	attributesFile = filepath.Join(dir, "attributes")
	if err := os.WriteFile(attributesFile, []byte("*.cs filter=hide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A path rather than a command line, because the config value has to survive
	// GIT_CONFIG_PARAMETERS' own single-quote packing intact.
	cleanCommand = filepath.Join(dir, "hide-edit")
	if err := os.WriteFile(cleanCommand, []byte("#!/bin/sh\nexec sed 's/, edited//'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return attributesFile, cleanCommand
}

// cleanFilterEnv installs that filter through the GIT_CONFIG_COUNT family.
func cleanFilterEnv(t *testing.T) []string {
	t.Helper()
	attributes, clean := hideEditFilter(t)
	return []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=core.attributesFile",
		"GIT_CONFIG_VALUE_0=" + attributes,
		"GIT_CONFIG_KEY_1=filter.hide.clean",
		"GIT_CONFIG_VALUE_1=" + clean,
	}
}

// cleanFilterParameters installs the same filter through the one-variable form,
// GIT_CONFIG_PARAMETERS, which is what git itself exports.
func cleanFilterParameters(t *testing.T) string {
	t.Helper()
	attributes, clean := hideEditFilter(t)
	return fmt.Sprintf("GIT_CONFIG_PARAMETERS='core.attributesFile=%s' 'filter.hide.clean=%s'", attributes, clean)
}

// unparseableGlobalConfigHome writes a home directory whose ~/.gitconfig git
// refuses to read, and returns the environment pointing git at it.
//
// The payload is a broken section header rather than a hostile setting on
// purpose. Every setting the gate cares about it can outrank with a `-c` flag
// or blank by enumeration, so a case built on one of those would stay green
// with the config-file pin deleted. A file git cannot parse is answerable only
// by not reading the file: git aborts every invocation with "bad config line",
// so with GIT_CONFIG_GLOBAL unpinned the run has no diff and no document at all.
func unparseableGlobalConfigHome(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core\n\tquotePath = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// XDG_CONFIG_HOME is the other place git looks for a global config, and an
	// ambient one would answer ahead of the home this case just built.
	return []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config")}
}

// configureCleanFilter installs the hiding filter in the fixture's own
// .git/config under key, which is `filter.hide.clean` or `filter.hide.process`,
// and commits the .gitattributes line selecting it. This is the repo-local
// scope, the one git-lfs and git-crypt write to, and neither the environment
// scrub nor the pinned config files reach it.
func (f *fixture) configureCleanFilter() {
	f.t.Helper()
	_, clean := hideEditFilter(f.t)
	f.write(".gitattributes", "*.cs filter=hide\n")
	f.git("config", "filter.hide.clean", clean)
}

// configureProcessFilter does the same through `filter.hide.process`, the
// long-running protocol git prefers over `.clean` when both are set and the half
// git-lfs installs. The driver quits without speaking a word of that protocol,
// so a run that launches it dies rather than measuring anything, which is a
// regression that fails in a second instead of hanging until the suite's
// deadline.
func (f *fixture) configureProcessFilter() {
	f.t.Helper()
	script := filepath.Join(f.t.TempDir(), "mute-process")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.write(".gitattributes", "*.cs filter=hide\n")
	f.git("config", "filter.hide.process", script)
}

// configureRequiredCleanFilter installs the hiding filter and marks it
// required, which is what `git lfs install --local` and git-crypt write. git
// then refuses to fall back when the driver produces nothing, so blanking the
// driver without also clearing this flag aborts the diff instead of passing the
// content through, and every run in such a repository exits 1.
func (f *fixture) configureRequiredCleanFilter() {
	f.t.Helper()
	_, clean := hideEditFilter(f.t)
	f.write(".gitattributes", "*.cs filter=hide\n")
	f.git("config", "filter.hide.clean", clean)
	f.git("config", "filter.hide.required", "true")
}

// configureFilterNamedWithATrailingSpace configures a driver whose subsection
// name ends in a space, alongside the ordinary hiding filter that the
// .gitattributes line actually selects.
//
// A subsection name is arbitrary text, so `filter.hide .clean` is one key. A
// reader that splits the config listing on whitespace turns it into `filter.hide`
// and `.clean`, and the second is a key git refuses on the command line, so the
// gate hands git an unparseable `-c` and dies on every run in this repository.
// A name holding a space anywhere else has the same cause, but no .gitattributes
// line can select it, since an attribute value cannot contain a space, so this
// spelling is the one the gate can be caught on.
func (f *fixture) configureFilterNamedWithATrailingSpace() {
	f.t.Helper()
	_, clean := hideEditFilter(f.t)
	f.write(".gitattributes", "*.cs filter=hide\n")
	f.git("config", "filter.hide.clean", clean)
	f.git("config", "filter.hide .clean", clean)
}

// configureFilterNamedWithAnEquals configures the hiding filter under a
// subsection name holding an `=`, and selects it from .gitattributes, which an
// attribute value holding an `=` can do.
//
// A `-c` argument is split on its first `=`, so blanking this driver that way
// sends the override to `filter.ev` and leaves the real one installed. git then
// runs it over the working-tree side, the diff comes back empty, and the gate
// reports no changed methods and exits 0.
func (f *fixture) configureFilterNamedWithAnEquals() {
	f.t.Helper()
	_, clean := hideEditFilter(f.t)
	f.write(".gitattributes", "*.cs filter=ev=il\n")
	f.git("config", "filter.ev=il.clean", clean)
}

// configureFsmonitorHook installs a repo-local core.fsmonitor hook that appends
// a line to a marker file, and returns the marker's path. git runs the hook to
// refresh the index, which a diff does on every invocation, so the marker
// existing after a run means the repository chose a program the gate executed.
func (f *fixture) configureFsmonitorHook() string {
	f.t.Helper()
	dir := f.t.TempDir()
	marker := filepath.Join(dir, "ran")
	script := filepath.Join(dir, "fsmonitor")
	body := fmt.Sprintf("#!/bin/sh\necho ran >> %s\nexit 1\n", marker)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.git("config", "core.fsmonitor", script)
	return marker
}

// constantTextconvScript writes an executable that prints the same line for
// every input, which is a textconv driver that flattens both sides of a diff
// into one identical text and so erases every hunk header.
func constantTextconvScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "constant-textconv")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho constant\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// removeLooseObject deletes the loose object file for sha from the fixture's
// object store, which is how a case reproduces an object the gate cannot read.
func (f *fixture) removeLooseObject(sha string) {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.root, ".git", "objects", sha[:2], sha[2:])); err != nil {
		f.t.Fatal(err)
	}
}

// gitStderr is what git prints when the command at args fails in the fixture.
// A case whose document quotes git's own complaint asks git for the sentence
// rather than freezing one release's wording into a golden, the way readCause
// asks the operating system for its own.
func (f *fixture) gitStderr(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.root
	cmd.Env = append(os.Environ(), gitEnv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		f.t.Fatalf("git %s succeeded, which the case needs to fail", strings.Join(args, " "))
	}
	return strings.TrimSpace(stderr.String())
}

// divergenceStderr is what git prints when the index-to-working-tree
// comparison a --staged run makes over rel fails. The argv comes from the
// production builder rather than a copy of it, so the sentence a case pins is
// the one the gate quotes back even after the flags change.
//
// That makes a case using this one pin "the gate quotes git verbatim" and not
// "the gate asks git the right question": a wrong flag list moves the gate's
// message and this expectation together. What guards the argv itself is
// TestStagedRefusesAFileStagedInOneStateAndDirtyInAnother, which turns on the
// comparison finding a real divergence.
func (f *fixture) divergenceStderr(rel string) string {
	f.t.Helper()
	return f.gitStderr(gitscope.DivergenceArgs([]srcpath.Path{srcpath.Path(rel)})...)
}

// toonEscaped renders text the way a TOON string field escapes it, which is
// what a golden's hole holds when the cause it stands for carries a quote or a
// newline. git's own complaint about a file it cannot open carries both.
func toonEscaped(text string) string {
	quoted := strconv.Quote(text)
	return quoted[1 : len(quoted)-1]
}

// corruptPackedRefs packs every ref and appends a line git cannot read, so
// reading any ref, HEAD included, fails rather than answering no.
//
// It is the ref store rather than one branch file because git reports every
// damaged branch file, an empty one, junk in place of the sha, a name it
// refuses, a symref loop, as the exit 1 that means no such ref. The store is
// the only place a case can make the HEAD check fail to answer at all, which
// is the difference the gate draws between a branch with no commit and a repo
// it cannot read.
func (f *fixture) corruptPackedRefs() {
	f.t.Helper()
	f.git("pack-refs", "--all")
	path := filepath.Join(f.root, ".git", "packed-refs")
	body, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, "not a ref line\n"...), 0o644); err != nil {
		f.t.Fatal(err)
	}
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

// addOrigin creates a second, bare repository under a temp dir of its own and
// wires it in as the fixture's `origin`, pushing branch to it so
// refs/remotes/origin/<branch> exists afterward. The remote is initialised
// with the same default branch name so its own HEAD, which setOriginHead
// reads, names something real. Pushing alone leaves
// refs/remotes/origin/HEAD unset, which is what makes the no-set-head case
// distinct from this one.
func (f *fixture) addOrigin(branch string) {
	f.t.Helper()
	remote := filepath.Join(f.t.TempDir(), "origin.git")
	f.git("init", "--bare", "--quiet", "-b", branch, remote)
	f.git("remote", "add", "origin", remote)
	f.git("push", "--quiet", "origin", branch)
}

// setOriginHead points refs/remotes/origin/HEAD at the remote's own default
// branch, asking the remote rather than guessing, which is what a real clone
// leaves behind and a bare push on its own does not.
func (f *fixture) setOriginHead() {
	f.t.Helper()
	f.git("remote", "set-head", "origin", "--auto")
}

// requireDistinctCommits fails the case unless the two refs name different
// commits. A case pinning one BaseCandidates rung ahead of another separates
// them by the changed-method count its golden carries, and that count only
// separates them while the two refs sit at different commits: a setup that
// let them converge would go on passing while proving nothing about the order.
func (f *fixture) requireDistinctCommits(a, b string) {
	f.t.Helper()
	if f.git("rev-parse", a) == f.git("rev-parse", b) {
		f.t.Fatalf("%s and %s are the same commit, so the case cannot separate the two rungs", a, b)
	}
}

// pushOrphanHistoryToOrigin builds a commit with no parent, so it shares no
// history with whatever HEAD names, and force-pushes it to origin's branch,
// which is how a case makes refs/remotes/origin/<branch> a candidate that
// exists but merge-base cannot relate to HEAD (issue 35). It leaves the
// fixture back on the branch it started on, fetches so the remote-tracking ref
// is refreshed from what was actually pushed rather than trusted from the push
// output, and fails the case if that ref is missing or still shares history
// with HEAD.
//
// The fetch runs with remote.origin.followRemoteHEAD=never, because from git
// 2.48 a plain fetch writes refs/remotes/origin/HEAD, and a case that means to
// reach the origin/<branch> rung would silently be pinning the origin/HEAD one
// instead, differently on different runners.
//
// The orphan commit stages one explicit path rather than the whole tree, so a
// case's untracked files survive the round trip: a `git add -A` here would
// sweep them onto the orphan branch and the checkout back would then delete
// them from the working tree. Nothing else about the tree survives, so the
// helper must run on a clean tree and before the case stamps its coverage
// report: `git rm -rf .` drops uncommitted edits to tracked files, and the
// checkout back bumps every tracked file's mtime, which would leave the report
// older than the code it describes and trip the staleness rule. Both
// preconditions are checked rather than left to the doc.
func (f *fixture) pushOrphanHistoryToOrigin(branch string) {
	f.t.Helper()
	if dirty := f.git("status", "--porcelain", "--untracked-files=no"); dirty != "" {
		f.t.Fatalf("pushOrphanHistoryToOrigin needs a clean tree, because `git rm -rf .` drops uncommitted edits:\n%s", dirty)
	}
	if _, err := os.Stat(filepath.Join(f.root, "TestResults")); err == nil {
		f.t.Fatal("pushOrphanHistoryToOrigin must run before the case stamps its coverage report, because the checkout back bumps every tracked file's mtime")
	}
	current := f.git("symbolic-ref", "--short", "HEAD")
	f.git("checkout", "--quiet", "--orphan", "unrelated-history")
	f.git("rm", "--quiet", "-rf", ".")
	f.write("unrelated.txt", "shares no history with "+current+"\n")
	f.git("add", "--", "unrelated.txt")
	f.git("commit", "--quiet", "-m", "unrelated history")
	f.git("push", "--quiet", "--force", "origin", "unrelated-history:"+branch)
	f.git("checkout", "--quiet", current)
	f.git("-c", "remote.origin.followRemoteHEAD=never", "fetch", "--quiet", "origin")

	f.git("rev-parse", "--verify", "--quiet", "origin/"+branch+"^{commit}")
	cmd := exec.Command("git", "merge-base", "HEAD", "origin/"+branch)
	cmd.Dir = f.root
	cmd.Env = append(os.Environ(), gitEnv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		f.t.Fatalf("origin/%s still shares history with HEAD after the orphan push", branch)
	}
	// Exit 1 is git answering "no common ancestor"; every other exit code is
	// git failing to answer at all, so only exit 1 confirms the helper really
	// produced unrelated history. The distinction is the helper's own, drawn
	// here so a broken fixture cannot read as a working one; ResolveBase drops
	// any merge-base failure alike, and that quiet fall-through is what the
	// caller pins. gitscope draws it in ResolveRef and noMatch.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		f.t.Fatalf("merge-base HEAD origin/%s failed to answer: %v\n%s",
			branch, err, strings.TrimSpace(stderr.String()))
	}
}
