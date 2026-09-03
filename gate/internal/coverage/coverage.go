// Package coverage discovers Cobertura reports, parses them, and answers
// which lines of a source file are instrumentable and which of those were
// hit. It resolves report paths by ADR 0004's one rule and ignores, rather
// than fails on, a path it cannot place inside the repo.
package coverage

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		parsed.mergeInto(set, root)
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

// mergeInto resolves each class to a source path and folds its lines into the
// set. Resolution runs per report, with that report's own <sources>.
func (r coberturaReport) mergeInto(set Set, root srcpath.Root) {
	for _, class := range r.Classes {
		path, ok := r.resolve(class.Filename, root)
		if !ok {
			continue
		}
		lines, ok := set[path]
		if !ok {
			lines = Lines{}
			set[path] = lines
		}
		for _, line := range class.Lines {
			lines[line.Number] = lines[line.Number] || line.Hits > 0
		}
	}
}

// resolve turns a class filename into a source path per ADR 0004: each
// <source> joined to the filename, plus the filename itself when it is
// already absolute, resolved through symlinks and relativized against the
// repo root. A candidate that will not resolve, or resolves outside the repo,
// is ignored rather than fatal, because a report describes a moment in the
// past.
func (r coberturaReport) resolve(filename string, root srcpath.Root) (srcpath.Path, bool) {
	for _, source := range r.Sources {
		candidate := filepath.Join(source, filepath.FromSlash(filename))
		if path, ok := root.Resolve(candidate); ok {
			return path, true
		}
	}
	if filepath.IsAbs(filename) {
		return root.Resolve(filepath.FromSlash(filename))
	}
	return "", false
}
