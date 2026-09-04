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
	"strings"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// ReportName is the file name coverlet writes a Cobertura report under.
const ReportName = "coverage.cobertura.xml"

// resultsDir is the directory component a report has to sit under to be
// discovered, matching the glob **/TestResults/**/coverage.cobertura.xml.
const resultsDir = "TestResults"

// Lines is one source file's instrumentable lines, each mapped to whether any
// report recorded a hit on it.
type Lines map[int]bool

// Set is the union of every discovered report, keyed by source path. A file
// absent from the set matched no report path at all, which is what makes a
// changed method in it unknown rather than untested.
type Set map[srcpath.Path]Lines

// Discover lists the Cobertura reports under the repo root, in a fixed order,
// beside the paths the walk could not read. A skipped path is a report the
// walk may not have seen, so the document carries it under `skipped_paths`
// rather than leaving understated coverage unexplained.
func Discover(root srcpath.Root) (reports []srcpath.Path, skipped []string, err error) {
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
		reports = append(reports, srcpath.FromSlash(filepath.ToSlash(rel)))
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("discovering coverage reports: %w", err)
	}
	slices.Sort(reports)
	slices.Sort(skipped)
	return reports, skipped, nil
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

// Load reads every report and unions them: a line is instrumentable when any
// report lists it, and covered when any report records a non-zero hit.
func Load(root srcpath.Root, reports []srcpath.Path) (Set, error) {
	set := Set{}
	for _, path := range reports {
		parsed, err := parseReport(root.Abs(path))
		if err != nil {
			return nil, &report.Failure{
				Code:    report.CodeCoverageUnparseable,
				Message: fmt.Sprintf("could not parse coverage report %s; %s", path, parseCause(err)),
			}
		}
		if failure := parsed.mergeInto(set, root, path); failure != nil {
			return nil, failure
		}
	}
	return set, nil
}

// coberturaReport is the subset of the Cobertura schema the gate reads. Only
// <sources>, each class's filename, and each line's number and hits matter;
// ADR 0001 rejects taking complexity from the report.
type coberturaReport struct {
	XMLName xml.Name         `xml:"coverage"`
	Sources []string         `xml:"sources>source"`
	Classes []coberturaClass `xml:"packages>package>classes>class"`
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
// a read error, which the message already names repo-relative.
func parseCause(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
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
func (r coberturaReport) mergeInto(set Set, root srcpath.Root, reportPath srcpath.Path) *report.Failure {
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
// two.
func joinPaths(paths []srcpath.Path) string {
	rendered := make([]string, 0, len(paths))
	for _, path := range paths {
		rendered = append(rendered, path.String())
	}
	if len(rendered) < 2 {
		return strings.Join(rendered, "")
	}
	return strings.Join(rendered[:len(rendered)-1], ", ") + " and " + rendered[len(rendered)-1]
}

// erasedSourceRoot checks the two shapes MSBuild produces when the source
// root that would otherwise anchor every class filename has been rewritten
// away, ahead of any resolution attempt: cheaper, and the candidates built
// from either shape would only mislead. DeterministicReport is tested over
// every class first, so it wins over UseSourceLink in a report carrying both
// shapes whatever order the classes appear in.
func (r coberturaReport) erasedSourceRoot(reportPath srcpath.Path) *report.Failure {
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
	for _, class := range r.Classes {
		if sourceLinkScheme.MatchString(class.Filename) {
			return true
		}
	}
	if len(r.Sources) == 0 {
		return false
	}
	for _, source := range r.Sources {
		if source != "" {
			return false
		}
	}
	for _, class := range r.Classes {
		if !filepath.IsAbs(class.Filename) {
			return true
		}
	}
	return false
}

// candidates lists every path ADR 0004 derives from one class filename. Each
// <source> is joined to it, plus the filename itself when it is already
// absolute; the join is absolute only when the <source> it started from was.
// A filename naming no file at all, whether it is empty, ".", ".." or a bare
// separator, yields nothing, because joining any of those onto a <source> names
// a directory, which resolves inside the root and would let one malformed class
// stand in for a whole report's worth of classes that did not. ".." is the same
// case as "." one level up, and a filename that merely starts with ".." still
// names a file, so only the bare form is carved out.
func (r coberturaReport) candidates(filename string) []string {
	switch filepath.Clean(filepath.FromSlash(filename)) {
	case ".", "..", string(filepath.Separator):
		return nil
	}
	candidates := make([]string, 0, len(r.Sources)+1)
	for _, source := range r.Sources {
		candidates = append(candidates, filepath.Join(source, filepath.FromSlash(filename)))
	}
	if filepath.IsAbs(filename) {
		candidates = append(candidates, filepath.FromSlash(filename))
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
	for _, candidate := range candidates {
		placed := root.Place(candidate)
		if first == "" {
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
func outsideRepoFailure(example string, reportPath srcpath.Path, root srcpath.Root) *report.Failure {
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
