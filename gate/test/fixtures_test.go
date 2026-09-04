package gate_test

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
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

// moveLines cuts the one-based lines from..to (inclusive) out of the file at
// src and inserts them into dst immediately after dst's line after. It copies
// the lines verbatim, so they are byte-identical at their new home. Both src
// and dst must already exist on disk, since the helper reads each one before
// it rewrites it.
// The helper rewrites src before it re-reads dst, so when src and dst name the
// same file, after counts lines in the shortened file.
func (f *fixture) moveLines(src string, from, to int, dst string, after int) {
	f.t.Helper()
	srcLines := strings.Split(f.read(src), "\n")
	cut := slices.Clone(srcLines[from-1 : to])
	f.write(src, strings.Join(slices.Delete(srcLines, from-1, to), "\n"))

	dstLines := strings.Split(f.read(dst), "\n")
	f.write(dst, strings.Join(slices.Insert(dstLines, after, cut...), "\n"))
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
// pairing ADR 0004 resolves paths from. It stamps the report now, which is
// newer than any source the fixture wrote before it, so a case that is not
// about staleness is never refused for it.
func cobertura(sourceRoot string, classes ...coverageClass) string {
	return renderCobertura(nowStamp(), []string{sourceRoot}, classes...)
}

// coberturaStamped is cobertura carrying the given root timestamp attribute,
// which is the producer's own clock and the value the staleness rule reads.
// An empty stamp omits the attribute, which is the report the rule cannot
// judge.
func coberturaStamped(stamp, sourceRoot string, classes ...coverageClass) string {
	return renderCobertura(stamp, []string{sourceRoot}, classes...)
}

// nowStamp is the current second, as a Cobertura timestamp attribute spells
// it.
func nowStamp() string {
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

// setModTime backdates the file at rel, which is how a case pins the ordering
// of two edits the filesystem would otherwise stamp within the same second.
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
	return renderCobertura(nowStamp(), nil, classes...)
}

// renderCobertura is the document builder, taking the timestamp attribute and
// the <source> list directly. A case naming more than one source is a class
// with two in-root candidates (issue 16, file_ambiguous) or a report split
// across two checkouts. An empty stamp omits the timestamp attribute; every
// case that is not about staleness passes nowStamp(), since issue 15 refuses
// a report older than the source it describes and a fixed stamp would make
// every fixture stale.
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
