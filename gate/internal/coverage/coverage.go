// Package coverage discovers Cobertura reports, parses them, and answers
// which lines of a source file are instrumentable and which of those were
// hit. It resolves report paths by ADR 0004's one rule, and the rule cuts two
// ways: one path it cannot place inside the repo is a silent ignore, while a
// report with an erased source root, a class contradicting itself, or no class
// inside the root at all fails the run with a typed code.
package coverage

import (
	"encoding/xml"
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
				Message: "could not parse coverage report " + path.String(),
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

// parseReport reads one Cobertura document off disk.
func parseReport(path string) (coberturaReport, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return coberturaReport{}, err
	}
	var parsed coberturaReport
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return coberturaReport{}, err
	}
	return parsed, nil
}

// erasedSourceRootPrefix is the placeholder DeterministicReport=true rewrites
// every class filename under once it erases <sources>.
const erasedSourceRootPrefix = "/_/"

// sourceLinkScheme matches a filename UseSourceLink=true emits in place of a
// path: the raw source-link document key, which SourceLink always spells as a
// URI.
var sourceLinkScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// mergeInto resolves each class to a source path and folds its lines into the
// set. Resolution runs per report, with that report's own <sources>. ADR
// 0004's 2026-09-03 amendment (issue 16) adds three checks ahead of the join,
// in precedence order: an erased source root voids the whole report before
// any candidate is built, a class resolving to more than one path inside the
// root contradicts itself, and a report whose classes resolved outside the
// root and none inside it is the git-worktree case. Any one candidate
// resolving nowhere, or resolving outside the root, stays the silent ignore
// ADR 0004 already decided, and a report whose every class merely no longer
// exists on disk resolves nothing and fails nothing here.
func (r coberturaReport) mergeInto(set Set, root srcpath.Root, reportPath srcpath.Path) *report.Failure {
	if failure := r.erasedSourceRoot(reportPath); failure != nil {
		return failure
	}

	resolvedAnyClass := false
	outsideExample := ""
	for _, class := range r.Classes {
		distinct, outside := placeCandidates(r.candidates(class.Filename), root)
		if outsideExample == "" {
			outsideExample = outside
		}
		if len(distinct) > 1 {
			sorted := append([]srcpath.Path{}, distinct...)
			slices.Sort(sorted)
			return &report.Failure{
				Code: report.CodeFileAmbiguous,
				Message: fmt.Sprintf(
					"class %s in coverage report %s resolved to more than one path inside the repo root, %s and %s",
					class.Filename, reportPath, sorted[0], sorted[1]),
			}
		}
		if len(distinct) == 0 {
			continue
		}
		resolvedAnyClass = true
		path := distinct[0]
		lines, ok := set[path]
		if !ok {
			lines = Lines{}
			set[path] = lines
		}
		for _, line := range class.Lines {
			lines[line.Number] = lines[line.Number] || line.Hits > 0
		}
	}

	if !resolvedAnyClass && outsideExample != "" {
		return outsideRepoFailure(outsideExample, reportPath, root)
	}
	return nil
}

// erasedSourceRoot checks the two shapes MSBuild produces when the source
// root that would otherwise anchor every class filename has been rewritten
// away, ahead of any resolution attempt: cheaper, and the candidates built
// from either shape would only mislead. The two passes are separate on
// purpose. That is what makes DeterministicReport win over UseSourceLink in a
// report carrying both shapes, whatever order the classes appear in; one loop
// testing both conditions would report whichever class came first instead.
func (r coberturaReport) erasedSourceRoot(reportPath srcpath.Path) *report.Failure {
	for _, class := range r.Classes {
		if strings.HasPrefix(class.Filename, erasedSourceRootPrefix) {
			return &report.Failure{
				Code: report.CodeCoverageSourceRootErased,
				Message: fmt.Sprintf(
					"coverage report %s carries no source root, erased by DeterministicReport=true; collect coverage with DeterministicReport=false",
					reportPath),
			}
		}
	}
	for _, class := range r.Classes {
		if sourceLinkScheme.MatchString(class.Filename) {
			return &report.Failure{
				Code: report.CodeCoverageSourceRootErased,
				Message: fmt.Sprintf(
					"coverage report %s carries a source link document key rather than a path, erased by UseSourceLink=true; collect coverage with UseSourceLink=false",
					reportPath),
			}
		}
	}
	return nil
}

// candidates lists every absolute path ADR 0004 derives from one class
// filename: each <source> joined to it, plus the filename itself when it is
// already absolute.
func (r coberturaReport) candidates(filename string) []string {
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
// landed inside the root and the first candidate that landed outside it, or
// "" when none did. More than one distinct path is what makes a class
// ambiguous; ADR 0004 reasons this can only happen when the reasoning behind
// "one repo root, one drive letter" is wrong. The outside candidate is
// returned separately because it, and not a candidate that failed to resolve
// at all, is the evidence that the report was built in another checkout.
func placeCandidates(candidates []string, root srcpath.Root) (distinct []srcpath.Path, outside string) {
	seen := map[srcpath.Path]bool{}
	for _, candidate := range candidates {
		path, placement := root.Place(candidate)
		switch placement {
		case srcpath.OutsideRoot:
			if outside == "" {
				outside = candidate
			}
		case srcpath.InsideRoot:
			if !seen[path] {
				seen[path] = true
				distinct = append(distinct, path)
			}
		}
	}
	return distinct, outside
}

// outsideRepoFailure names the git-worktree case: no class in the report
// resolved inside the root and at least one resolved outside it. The example
// is the first such candidate in document order, resolved through symlinks
// per srcpath.ResolveOrAsBuilt so the reader sees the path the gate compared.
func outsideRepoFailure(example string, reportPath srcpath.Path, root srcpath.Root) *report.Failure {
	return &report.Failure{
		Code: report.CodeCoverageOutsideRepo,
		Message: fmt.Sprintf(
			"coverage report %s resolved no class inside the repo root; example resolved path %s, repo root %s",
			reportPath, srcpath.ResolveOrAsBuilt(example), root.Dir()),
	}
}
