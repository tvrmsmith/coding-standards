// Package coverage discovers Cobertura reports, parses them, and answers
// which lines of a source file are instrumentable and which of those were
// hit. It resolves report paths by ADR 0004's one rule, and the rule cuts two
// ways: one path it cannot place inside the repo is a silent ignore, while a
// report with an erased source root, a class contradicting itself, or no class
// placed inside the root fails the run with a typed code.
package coverage

import (
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// ReportName is the file name coverlet writes a Cobertura report under.
const ReportName = "coverage.cobertura.xml"

// resultsDir is the directory component a report has to sit under to be
// discovered, at any depth below the repo root.
const resultsDir = "TestResults"

// Glob is the pattern discovery matches, relative to the repo root. The
// missing-report failure names it, so a developer who ran the tests
// somewhere else can see where the gate looked. It is composed from the two
// constants discovery actually tests, so a rename cannot leave the message
// quoting a pattern the walk no longer matches.
const Glob = "**/" + resultsDir + "/**/" + ReportName

// Source is one coverage report the gate will read, and the name the
// document gives it.
type Source struct {
	// Abs is where the report sits on disk.
	Abs string
	// Name is how a failure names the report: repo-relative whenever the
	// report resolves inside the repo, and the developer's own spelling for
	// a named one that does not, which has no repo-relative form.
	Name string
	// Origin is how the report reached the gate, which decides the remedy a
	// refusal offers.
	Origin Origin
}

// Origin distinguishes the two ways a report reaches the gate, because the
// developer's next step differs between them: a discovered report is one of
// several a directory may hold, while a named one is the single file they
// pointed at.
type Origin int

const (
	Discovered Origin = iota
	NamedOnCommandLine
)

// staleRemedy closes the stale-report message with the step that clears it.
// `dotnet test` leaves every previous run's TestResults directory on disk, so
// a discovered report the gate refused is most often one the developer has
// already replaced and does not know is still there. Clearing that directory
// does nothing for a report they named themselves, which nothing but a fresh
// run over that same path replaces.
func (s Source) staleRemedy() string {
	if s.Origin == NamedOnCommandLine {
		return "; regenerate " + s.Name + " or point --coverage at a current report"
	}
	return "; clear stale TestResults directories and re-run the tests"
}

// Newest is the freshest edit among the files that contributed a changed
// method, which is what a report has to be at least as new as.
type Newest struct {
	File srcpath.Path
	// At is truncated to whole seconds, since a Cobertura timestamp has
	// second resolution and cannot express anything finer.
	At time.Time
}

// Lines is one source file's instrumentable lines, each mapped to whether any
// report recorded a hit on it.
type Lines map[int]bool

// Set is the union of every discovered report, keyed by source path. A file
// absent from the set matched no report path at all, which is what makes a
// changed method in it unknown rather than untested.
type Set map[srcpath.Path]Lines

// Discover lists the Cobertura reports under the repo root, in a fixed order
// by Name, beside the paths the walk could not read. A skipped path is a
// report the walk may not have seen, so the document carries it under
// `skipped_paths` rather than leaving understated coverage unexplained.
func Discover(root srcpath.Root) (sources []Source, skipped []string, err error) {
	err = filepath.WalkDir(root.Dir(), func(path string, entry fs.DirEntry, err error) error {
		// One unreadable directory somewhere under the repo root must not
		// abort discovery: that would exit 1 with no document at all, where
		// the worst a skipped subtree can cost is a report the walk did not
		// see, which the reader now sees in `skipped_paths`.
		if err != nil {
			skipped = append(skipped, relative(root, path))
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Name() != ReportName {
			return nil
		}
		rel, err := filepath.Rel(root.Dir(), path)
		if err != nil {
			return nil
		}
		if !underResultsDir(rel) {
			return nil
		}
		sources = append(sources, Source{Abs: path, Name: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("discovering coverage reports: %w", err)
	}
	slices.SortFunc(sources, func(a, b Source) int { return strings.Compare(a.Name, b.Name) })
	slices.Sort(skipped)
	return sources, skipped, nil
}

// Named resolves each developer-typed path against cwd when it is relative,
// then relativizes it for the Name the document carries, which is ADR 0004's
// one rule for a human-typed path. A named path may live anywhere, inside the
// repo or outside it, and one outside has no repo-relative form, so it keeps
// the spelling the developer typed.
func Named(root srcpath.Root, cwd string, paths []string) []Source {
	sources := make([]Source, 0, len(paths))
	for _, path := range paths {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		sources = append(sources, Source{Abs: abs, Name: namedAs(root, abs, path), Origin: NamedOnCommandLine})
	}
	return sources
}

// namedAs renders a named report the way the document names paths. It
// resolves symlinks when the report is there to resolve, so a path reached
// through one relativizes against the resolved root rather than escaping it,
// and falls back to the join for a path naming nothing on disk, which still
// has to be named in the failure that says so.
func namedAs(root srcpath.Root, abs, typed string) string {
	resolved := abs
	if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = evaluated
	}
	rel, err := filepath.Rel(root.Dir(), resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return typed
	}
	return filepath.ToSlash(rel)
}

// relative renders a walked path the way the document names paths, falling
// back to the absolute path when it cannot be placed under the root.
func relative(root srcpath.Root, path string) string {
	rel, err := filepath.Rel(root.Dir(), path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// underResultsDir reports whether any directory component of rel is the
// TestResults directory the glob anchors on.
func underResultsDir(rel string) bool {
	dir := filepath.Dir(rel)
	for _, component := range strings.Split(filepath.ToSlash(dir), "/") {
		if component == resultsDir {
			return true
		}
	}
	return false
}

// Load reads every source in order and unions them into one set: a line is
// instrumentable when any report lists it, and covered when any report
// records a non-zero hit. Before a report merges, it has to read, unmarshal,
// carry a timestamp, and be no older than newest.At; failing any of those
// stops the run rather than dropping the report, because a stale report
// silently dropped is exactly the untested code the rule exists to catch.
// An empty newest.At (the zero time) can never trip the staleness rule.
func Load(root srcpath.Root, sources []Source, newest Newest) (Set, error) {
	set := Set{}
	for _, source := range sources {
		parsed, err := parseReport(source.Abs)
		if err != nil {
			return nil, &report.Failure{
				Code:    report.CodeCoverageUnparseable,
				Message: fmt.Sprintf("could not parse coverage report %s; %s", source.Name, parseCause(err)),
			}
		}
		at, ok := parsed.timestamp()
		if !ok {
			return nil, &report.Failure{
				Code:    report.CodeCoverageUnparseable,
				Message: "coverage report " + source.Name + parsed.timestampCause(),
			}
		}
		// Staleness is checked before the merge, not after it, so a refused
		// report never contributes a line even transiently.
		if at.Before(newest.At) {
			return nil, &report.Failure{
				Code: report.CodeCoverageStale,
				Message: "coverage report " + source.Name + " was written before " +
					newest.File.String() + " was last edited" + source.staleRemedy(),
			}
		}
		if err := parsed.mergeInto(set, root, source.Name); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// coberturaReport is the subset of the Cobertura schema the gate reads. Only
// <timestamp>, <sources>, each class's filename, and each line's number and
// hits matter; ADR 0001 rejects taking complexity from the report.
type coberturaReport struct {
	XMLName   xml.Name         `xml:"coverage"`
	Timestamp string           `xml:"timestamp,attr"`
	Sources   []string         `xml:"sources>source"`
	Classes   []coberturaClass `xml:"packages>package>classes>class"`
}

// timestamp reads the report's own clock, which is what the staleness rule
// judges rather than any timestamp of the file on disk. It reports false when
// the attribute is absent, empty, or not a base-10 integer, none of which the
// rule can run without.
func (r coberturaReport) timestamp() (time.Time, bool) {
	if r.Timestamp == "" {
		return time.Time{}, false
	}
	seconds, err := strconv.ParseInt(r.Timestamp, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

// timestampCause says why the staleness rule could not read the report's own
// clock, split by shape because the two shapes need different fixes: an
// attribute that is absent has to be written, while one the rule cannot parse
// is already there and only in the wrong units. The offending value is quoted
// so the developer sees what the gate read rather than what they meant.
func (r coberturaReport) timestampCause() string {
	if r.Timestamp == "" {
		return " carries no timestamp, so it cannot be judged against the code it describes"
	}
	return fmt.Sprintf(" carries an unreadable timestamp %q; it must be epoch seconds", r.Timestamp)
}

type coberturaClass struct {
	Filename string          `xml:"filename,attr"`
	Lines    []coberturaLine `xml:"lines>line"`
}

type coberturaLine struct {
	Number int `xml:"number,attr"`
	Hits   int `xml:"hits,attr"`
}

// parseReport reads one Cobertura document off disk. Each <source> is trimmed
// as it is read, so a source root that is only the whitespace an XML formatter
// left between the tags is blank to every reader of the document, the join and
// the erased-root check alike.
func parseReport(path string) (coberturaReport, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return coberturaReport{}, err
	}
	var parsed coberturaReport
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return coberturaReport{}, err
	}
	for i, source := range parsed.Sources {
		parsed.Sources[i] = strings.TrimSpace(source)
	}
	return parsed, nil
}

// parseCause renders a parse failure without the absolute path os attaches to
// a read error, which the message already names repo-relative. The operation
// stays, because "open: permission denied" and "read: permission denied" are
// different faults under the same wording and the reader cannot tell them
// apart from the bare errno text.
func parseCause(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Op + ": " + pathErr.Err.Error()
	}
	return err.Error()
}

// erasedSourceRootPlaceholder matches the placeholder DeterministicReport=true rewrites
// every class filename under once it erases <sources>. MSBuild numbers the
// placeholder per source root, so the first is /_/ and any further one, which
// a submodule or a source package adds, is /_1/, /_2/ and so on.
var erasedSourceRootPlaceholder = regexp.MustCompile(`^/_[0-9]*/`)

// sourceLinkScheme matches a filename UseSourceLink=true emits in place of a
// path: the raw source-link document key, which ADR 0004 records as one that
// can be a URL. A key carrying no scheme looks like an ordinary relative path,
// so it is the empty <source> beside it that gives the shape away.
var sourceLinkScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// mergeInto resolves each class to a source path and folds its lines into the
// set. Resolution runs per report, with that report's own <sources>. ADR
// 0004's 2026-09-04 amendment (issue 16) adds three checks ahead of the join,
// in precedence order: an erased source root voids the whole report before
// any candidate is built, a class resolving to more than one path inside the
// root contradicts itself, and a report that lands no class inside the root at
// all was measured against other source than the source being gated. Any one
// candidate resolving nowhere, or resolving outside the root, stays the silent
// ignore ADR 0004 already decided; it is only a report with nothing left that
// fails. A class the join can build no candidate from at all, a blank filename
// or a relative one with no <source> to anchor it, counts as landing nowhere
// like any other, so a report made only of those fails too. Only a report
// carrying no <class> element is silent, which is ADR 0004's one carve-out.
// The classes fold into a set of this report's own, unioned into the caller's
// only once every check has passed, so a report that fails leaves nothing of
// itself behind.
func (r coberturaReport) mergeInto(set Set, root srcpath.Root, reportPath string) error {
	if failure := r.erasedSourceRoot(reportPath); failure != nil {
		return failure
	}

	merged := Set{}
	example := ""
	for _, class := range r.Classes {
		distinct, first := placeCandidates(r.candidates(class.Filename), root)
		if example == "" {
			example = cmp.Or(first, class.Filename)
		}
		if len(distinct) > 1 {
			return &report.Failure{
				Code: report.CodeFileAmbiguous,
				Message: fmt.Sprintf(
					"class %s in coverage report %s resolved to more than one path inside the repo root, %s",
					class.Filename, reportPath, joinPaths(distinct)),
			}
		}
		if len(distinct) == 0 {
			continue
		}
		path := distinct[0]
		lines, ok := merged[path]
		if !ok {
			lines = Lines{}
			merged[path] = lines
		}
		for _, line := range class.Lines {
			lines[line.Number] = lines[line.Number] || line.Hits > 0
		}
	}

	if len(merged) == 0 && len(r.Classes) > 0 {
		return outsideRepoFailure(example, reportPath, root)
	}
	set.union(merged)
	return nil
}

// union folds another report's resolved lines in, a line staying covered once
// any report recorded a hit on it.
func (s Set) union(other Set) {
	for path, lines := range other {
		existing, ok := s[path]
		if !ok {
			existing = Lines{}
			s[path] = existing
		}
		for number, covered := range lines {
			existing[number] = existing[number] || covered
		}
	}
}

// joinPaths renders the paths one diagnostic quotes as a readable list, so a
// class contradicting itself three ways names all three rather than the first
// two. Its one caller reaches it only for a class that resolved to more than
// one path, so it takes at least two.
func joinPaths(paths []srcpath.Path) string {
	rendered := make([]string, 0, len(paths))
	for _, path := range paths {
		rendered = append(rendered, path.String())
	}
	return strings.Join(rendered[:len(rendered)-1], ", ") + " and " + rendered[len(rendered)-1]
}

// erasedSourceRoot checks the two shapes MSBuild produces when the source
// root that would otherwise anchor every class filename has been rewritten
// away, ahead of any resolution attempt: cheaper, and the candidates built
// from either shape would only mislead. DeterministicReport is tested over
// every class first, so it wins over UseSourceLink in a report carrying both
// shapes whatever order the classes appear in.
func (r coberturaReport) erasedSourceRoot(reportPath string) *report.Failure {
	for _, class := range r.Classes {
		if erasedSourceRootPlaceholder.MatchString(class.Filename) {
			return &report.Failure{
				Code: report.CodeCoverageSourceRootErased,
				Message: fmt.Sprintf(
					"coverage report %s carries no source root, erased by DeterministicReport=true; collect coverage with DeterministicReport=false",
					reportPath),
			}
		}
	}
	if r.sourceLinked() {
		return &report.Failure{
			Code: report.CodeCoverageSourceRootErased,
			Message: fmt.Sprintf(
				"coverage report %s carries a source link document key rather than a path, erased by UseSourceLink=true; collect coverage with UseSourceLink=false",
				reportPath),
		}
	}
	return nil
}

// sourceLinked reports whether the classes carry source-link document keys
// rather than paths. ADR 0004 names two halves of that shape and either one
// settles it: UseSourceLink=true writes the source root out as one blank
// <source>, and the key it leaves in the filename can be a URL but need not
// be, so a schemeless key is caught by the blank source beside it. A blank
// source alone is not enough, because an absolute filename carries its own
// root and is the legitimate shape coverlet writes when no computed source
// root prefixes the document; so is a report with no <source> at all.
func (r coberturaReport) sourceLinked() bool {
	if slices.ContainsFunc(r.Classes, func(class coberturaClass) bool {
		return sourceLinkScheme.MatchString(class.Filename)
	}) {
		return true
	}
	if len(r.Sources) == 0 {
		return false
	}
	if slices.ContainsFunc(r.Sources, func(source string) bool { return source != "" }) {
		return false
	}
	return slices.ContainsFunc(r.Classes, func(class coberturaClass) bool {
		return !filepath.IsAbs(class.Filename)
	})
}

// candidates lists every path ADR 0004 derives from one class filename. A
// filename that is already absolute carries its own root and is its own only
// candidate: joining a <source> onto it would name a path no report ever
// carried, /src/src/app/Order.cs off <source> /src, and the outside-repo
// diagnostic quotes the first candidate. A relative filename is joined to each
// <source>, and the join is absolute only when the <source> it started from
// was. An empty filename is the one spelling carved out here, because a class
// carrying no filename gives the reader no path to be shown at all and the
// join would otherwise quote the <source> itself as though the report had
// named it. Every other directory-shaped spelling, ".", "..", "../.." or a
// bare directory name, is joined like any other and refused by srcpath.Place
// for what it resolves to, so one such class cannot stand in for a whole
// report's worth of classes that placed nothing.
func (r coberturaReport) candidates(filename string) []string {
	if filename == "" {
		return nil
	}
	if filepath.IsAbs(filename) {
		return []string{filepath.FromSlash(filename)}
	}
	candidates := make([]string, 0, len(r.Sources))
	for _, source := range r.Sources {
		candidates = append(candidates, filepath.Join(source, filepath.FromSlash(filename)))
	}
	return candidates
}

// placeCandidates resolves every candidate, returning the distinct paths that
// landed inside the root, sorted so a diagnostic quoting them does not depend
// on <source> order, and the reading of the first candidate in document order,
// whether it landed or not. More than one distinct path is what makes a class
// ambiguous; ADR 0004 reasons this can only happen when the reasoning behind
// "one repo root, one drive letter" is wrong. The first candidate is returned
// because it is what the outside-repo diagnostic quotes, and a class that
// placed nowhere still has a path worth showing the reader.
func placeCandidates(candidates []string, root srcpath.Root) (distinct []srcpath.Path, first string) {
	seen := map[srcpath.Path]bool{}
	for i, candidate := range candidates {
		placed := root.Place(candidate)
		if i == 0 {
			first = placed.Resolved()
		}
		path, inside := placed.Inside()
		if inside && !seen[path] {
			seen[path] = true
			distinct = append(distinct, path)
		}
	}
	slices.Sort(distinct)
	return distinct, first
}

// outsideRepoFailure names the wrong-tree case, where no class in the report
// placed inside the root. The example is the first candidate of the first class
// in document order carrying a filename, symlink-resolved when it resolved and
// as the join built it when it did not, so the reader sees a path the gate
// compared and can tell a report from another checkout, a container mount, or a
// test run whose files are gone apart by looking at it. It is called "example
// path" rather than "example resolved path" for that reason. A candidate no
// <source> anchored is not absolute and names nothing on disk, so the message
// says so rather than quoting a relative string the reader would read as a path
// inside the repo. A report whose classes carry no filename to join builds no
// candidate at all, and says that instead.
func outsideRepoFailure(example string, reportPath string, root srcpath.Root) *report.Failure {
	compared := "example path " + example
	switch {
	case example == "":
		compared = "no class carries a filename to compare"
	case !filepath.IsAbs(filepath.FromSlash(example)):
		compared = "example path " + example + ", which no <source> anchored to an absolute path"
	}
	return &report.Failure{
		Code: report.CodeCoverageOutsideRepo,
		Message: fmt.Sprintf(
			"coverage report %s placed no class inside the repo root; %s, repo root %s",
			reportPath, compared, filepath.ToSlash(root.Dir())),
	}
}
