// Package coverage discovers Cobertura reports, or takes the ones the
// developer named on the command line instead, parses them, and answers which
// lines of a source file are instrumentable and which of those were hit. A
// report it cannot trust at all, one whose own timestamp it cannot read or
// one stamped outside the band an honest producer's clock reaches, is refused
// rather than merged, because coverage silently dropped reaches the score as
// untested code. A report merely older than the code it describes is refused
// the same way when a human named it, but one discovery found is instead
// skipped as a report a fresher run has already superseded, since dotnet test
// leaves every earlier run's TestResults directory on disk and the developer
// never asked the gate to weigh it against the fresh one sitting beside it.
// It resolves report paths by ADR 0004's one rule, and the rule cuts two
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
	"math"
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
	// report resolves inside the repo, and the resolved absolute path for a
	// named one that does not, which has no repo-relative form. namedAs
	// records why the absolute form beats the developer's own spelling there.
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
	// originUnset is the zero value, so a Source built without an Origin
	// cannot silently pass for either of the two real ones.
	originUnset Origin = iota
	Discovered
	NamedOnCommandLine
)

// staleRemedy closes the stale-report message with the step that clears it.
// Only a named report reaches here. A stale discovered one is skipped into
// skipped_paths and never refuses on its own, so the arm that once answered for
// it was dead code claiming a path the caller cannot take. What a named report
// needs is a fresh run over that same path, which nothing else replaces. A
// Source carrying no Origin closes the message with nothing at all. The reader
// still learns which report was refused and why, which is the part the gate
// knows, and a remedy it cannot determine is worse guessed than omitted.
// Refusing loudly instead is not available here either, since an unrecovered
// panic exits 2 and ADR 0008 spends that code on "threshold exceeded", so the
// one unreachable arm would tell CI the code failed the gate.
func (s Source) staleRemedy() string {
	switch s.Origin {
	case NamedOnCommandLine:
		return "; regenerate " + s.Name + " or point --coverage at a current report"
	default:
		return ""
	}
}

// discoveredStaleRemedy is the step that clears a stale discovered report. It
// has one consumer, allSupersededFailure below, which runs when every
// discovered report was superseded and has only Names left by then, no Source
// to ask. It stays a named constant rather than a literal in that message
// because `dotnet test` leaves every previous run's TestResults directory on
// disk, and the wording of that clearing step is the part of the message most
// likely to be reworded.
const discoveredStaleRemedy = "; clear stale TestResults directories and re-run the tests"

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
//
// A present key mapping to empty Lines is a distinct answer, not a missing
// one: every report that named the file listed it with no instrumentable
// line at all. mergeInto seeds that entry deliberately, and join.Attribute
// reads it as "the report never instrumented this file" and refuses to score
// the file's changed methods. Dropping a lineless class here instead would
// silently turn that answer back into the absent-key one.
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
			skipped = append(skipped, relative(root, path))
			return nil
		}
		if !underResultsDir(rel) {
			return nil
		}
		sources = append(sources, Source{Abs: path, Name: filepath.ToSlash(rel), Origin: Discovered})
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
// repo or outside it, and one outside has no repo-relative form, so it is named
// by its absolute path.
func Named(root srcpath.Root, cwd string, paths []string) []Source {
	sources := make([]Source, 0, len(paths))
	for _, path := range paths {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		sources = append(sources, Source{Abs: abs, Name: namedAs(root, abs), Origin: NamedOnCommandLine})
	}
	return sources
}

// namedAs renders a named report the way the document names paths. It
// relativizes the resolved reading of the path against the resolved root, so a
// path reached through a symlink is named repo-relative rather than escaping
// the root. A path naming nothing on disk resolves as far as it exists, since
// the root itself is very often reached through a symlink (/tmp and /var on
// macOS) and comparing an unresolved path against the resolved root would
// escape for the indirection rather than for where the path actually is. A
// path that is genuinely outside the repo has no repo-relative form, so it is
// named by the same resolved absolute path the containment test just weighed.
// The developer's own spelling was rejected for that case: the document carries
// no working directory (ADR 0008), so a relative name reaches a consumer that
// cannot resolve it, and the same report named from two directories would print
// two strings.
//
// It answers "is this inside the repo" itself, where srcpath's package doc
// claims that question for srcpath alone. srcpath.Root.Place cannot answer it
// here, since Place requires a regular file and the whole point of namedAs is
// to name a path that may be nothing. So separator handling and
// case-insensitive filesystems now have two answers, and correcting one leaves
// the other as it was, so a named report would be relativized where the other
// call site keeps the typed spelling, or the reverse. relative()
// below is a third spelling, with no escape guard at all. Issue 36 unifies all
// three behind an existence-agnostic sibling of Place, which is a change to
// srcpath and not to a branch about staleness.
func namedAs(root srcpath.Root, abs string) string {
	resolved := resolveExisting(abs)
	rel, err := filepath.Rel(root.Dir(), resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(resolved)
	}
	return filepath.ToSlash(rel)
}

// resolveExisting resolves the symlinks of the deepest ancestor of abs that is
// on disk and rejoins the components below it as they were typed. A path that
// exists whole resolves whole, and one naming nothing still comes back rooted
// where it really sits rather than behind whatever link led there. Only a
// component that is not there is climbed past: a symlink loop or an ancestor
// the process may not search is a real fault, and climbing over it would rename
// a report that does sit inside the repo into one the document names as though
// it sat outside.
func resolveExisting(abs string) string {
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved
	}
	parent := filepath.Dir(abs)
	if !errors.Is(err, fs.ErrNotExist) || parent == abs {
		return abs
	}
	return filepath.Join(resolveExisting(parent), filepath.Base(abs))
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

// clockSkewTolerance is how far ahead of the gate's own clock an instant may
// sit before the gate stops believing it, and toleranceLabel is how every
// refusal quoting the window spells it. The two are retuned together, and the
// label is written out rather than rendered from the constant because Go
// spells this duration "24h0m0s" and trimming that back to "24h" is only
// correct while the window is whole hours.
//
// An unsynchronised CI agent or a machine resumed from a suspended VM drifts
// by hours, not days, so a day of slack still lets an honest producer's report
// through. A producer that writes the timestamp attribute in epoch
// milliseconds rather than seconds, coverage.py's Cobertura writer among
// several Java ones, lands the parsed instant tens of thousands of years in
// the future, which this window separates from an honest clock by an enormous
// margin. The rule exists at all because without it, at.Before(newest.At) can
// never be true for a timestamp that far out, so the staleness comparison
// silently stops running and the report merges as though it had passed.
const (
	clockSkewTolerance = 24 * time.Hour
	toleranceLabel     = "24h"
)

// farAheadOfNow reports whether at sits further ahead of now than the gate
// tolerates, and farAheadOfNowClause is the fragment every refusal for it
// closes with. They stay together so a retune of the window cannot leave a
// refusal quoting the old one.
func farAheadOfNow(at, now time.Time) bool {
	return at.After(now.Add(clockSkewTolerance))
}

const farAheadOfNowClause = "more than " + toleranceLabel + " ahead of now"

// earliestPlausibleStamp is the oldest instant a report may claim to have been
// written at, 2000-01-01T00:00:00Z. No Cobertura producer predates it: the
// format's own tooling is younger than that, and any checkout the gate scores
// was tested by a run that happened after it. What lands below it is the same
// units fault the tolerance catches from the other side, a producer writing
// seconds since boot or since process start, or a zeroed or negative attribute
// where the writer had no clock to read. Leaving the floor off was rejected
// because such a stamp parses, holds, and sorts before every edit, so a
// discovered report carrying one is skipped as superseded and its coverage
// silently leaves the score, which is what the upper bound exists to prevent.
const earliestPlausibleStamp = 946684800

// Load reads every source in order and unions them into one set: a line is
// instrumentable when any report lists it, and covered when any report
// records a non-zero hit. now is a parameter rather than a call to time.Now()
// here, so one run judges every report it loads against one instant rather
// than against the clock as it advances through the loop, and a reader of this
// signature sees which instant that is; a package-level clock would hide both.
//
// Before a report merges, it has to read, unmarshal, carry a readable
// timestamp inside the plausible band, from earliestPlausibleStamp to
// clockSkewTolerance ahead of now, and be no older than newest.At. Failing to
// read, unmarshal or carry a readable timestamp stops the run rather than
// dropping the report, because a bad report silently dropped is exactly the
// untested code the staleness rule exists to catch. A timestamp outside the
// band stops the run the same way whichever Origin the source carries and
// whichever side it falls out on, since a report the gate cannot trust is not
// one a fresher run superseded.
//
// A report merely older than newest.At is judged by Origin instead. One a
// human named on --coverage still refuses the run: they chose it, and
// clearing a TestResults directory produces nothing new in its place. One
// Discovered is skipped rather than refused, its Name appended to the
// returned []string, and it contributes no line, not even transiently,
// because dotnet test leaves every earlier run's TestResults directory on
// disk and a fresh report sitting beside it should not fail the run over a
// leftover. A run where every source was skipped that way still refuses,
// naming every one of them, since skipping all of them would otherwise pass
// silently with nothing scored at all.
//
// A failure returns no skipped names, not even ones the loop had already
// collected before it hit the failure. The run exits 1 with nothing scored, so
// there is no understated coverage for a skipped name to explain, and the
// document names the fault that stopped the run instead. Returning them
// alongside was rejected for that: it would list a superseded report beside an
// unrelated coverage_unparseable as though the two were both why the score is
// short, when there is no score.
//
// An empty newest.At (the zero time) can never trip the staleness rule.
func Load(root srcpath.Root, sources []Source, newest Newest, now time.Time) (Set, []string, error) {
	set := Set{}
	var skipped []string
	for _, source := range sources {
		parsed, err := parseReport(source.Abs)
		if err != nil {
			return nil, nil, &report.Failure{
				Code:    report.CodeCoverageUnparseable,
				Message: fmt.Sprintf("could not parse coverage report %s; %s", source.Name, parseCause(err)),
			}
		}
		at, err := parsed.instant(now)
		if err != nil {
			return nil, nil, &report.Failure{
				Code:    report.CodeCoverageUnparseable,
				Message: "coverage report " + source.Name + " " + err.Error(),
			}
		}
		// Staleness is checked before the merge, not after it, so a refused or
		// skipped report never contributes a line even transiently.
		if at.Before(newest.At) {
			if source.Origin == Discovered {
				skipped = append(skipped, source.Name)
				continue
			}
			return nil, nil, &report.Failure{
				Code: report.CodeCoverageStale,
				Message: "coverage report " + source.Name + " was written before " +
					newest.File.String() + " was last edited" + source.staleRemedy(),
			}
		}
		if err := parsed.mergeInto(set, root, source.Name); err != nil {
			return nil, nil, err
		}
	}
	if len(skipped) > 0 && len(skipped) == len(sources) {
		return nil, nil, allSupersededFailure(skipped, newest)
	}
	return set, skipped, nil
}

// allSupersededFailure refuses the run when every source Load saw was a
// Discovered report skipped as superseded, leaving nothing to score. It closes
// with discoveredStaleRemedy directly rather than calling Source.staleRemedy,
// because every name it is given reached the skipped list through that one
// origin and by the time the loop ends the Source itself is gone, only its
// Name kept.
func allSupersededFailure(skipped []string, newest Newest) *report.Failure {
	noun, verb := "report", "was"
	if len(skipped) > 1 {
		noun, verb = "reports", "were"
	}
	return &report.Failure{
		Code: report.CodeCoverageStale,
		Message: fmt.Sprintf("coverage %s %s %s written before %s was last edited%s",
			noun, joinNames(skipped), verb, newest.File.String(), discoveredStaleRemedy),
	}
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

// instant reads the report's own clock, which is what the staleness rule
// judges rather than any timestamp of the file on disk, and answers only for a
// stamp the gate is willing to trust: readable, no further ahead of now than
// clockSkewTolerance, and no older than earliestPlausibleStamp. Every rejection
// shape resolves here, both the verdict and the sentence explaining it, so a
// further shape cannot refuse under one wording and explain itself with
// another, and the caller never reads Timestamp back to word its own. The
// error is the reason the staleness rule cannot run, worded for the refusal
// message with the offending value quoted so the developer sees what the gate
// read rather than what they meant, and split by shape because the shapes need
// different fixes: an attribute that is absent has to be written, while any
// value ParseInt cannot read as base-10 seconds, another representation of an
// instant or plain garbage alike, is already there and is a value to rewrite
// rather than one to add. A value ParseInt reads but time.Unix cannot hold
// shares that wording, since it is the same fix, the attribute in front of the
// developer is not epoch seconds. A stamp outside the plausible band names the
// end it fell out of, since a clock a day behind the producer's and a stamp
// from before the format existed are not the same fault.
func (r coberturaReport) instant(now time.Time) (time.Time, error) {
	if r.Timestamp == "" {
		return time.Time{}, errors.New("carries no timestamp, so it cannot be judged against the code it describes")
	}
	seconds, err := strconv.ParseInt(r.Timestamp, 10, 64)
	if err != nil || seconds > maxEpochSeconds {
		return time.Time{}, fmt.Errorf("carries an unreadable timestamp %q; it must be epoch seconds", r.Timestamp)
	}
	at := time.Unix(seconds, 0)
	if farAheadOfNow(at, now) {
		return time.Time{}, fmt.Errorf("carries a timestamp %q %s; it must be epoch seconds, or the clock on this machine is behind the one that wrote it",
			r.Timestamp, farAheadOfNowClause)
	}
	if seconds < earliestPlausibleStamp {
		return time.Time{}, fmt.Errorf("carries a timestamp %q from before %s; it must be epoch seconds",
			r.Timestamp, time.Unix(earliestPlausibleStamp, 0).UTC().Format(time.RFC3339))
	}
	return at, nil
}

// The highest stamp time.Unix can hold without the internal seconds field it
// builds wrapping. Go stores the value offset by the seconds between year 1 and
// 1970, so a stamp within a few tens of billions of MaxInt64 wraps into the
// distant past, and a report carrying one would then be judged older than the
// code and skipped as superseded rather than refused, which is exactly the
// untrustworthy stamp the tolerance rule above exists to refuse. Only that end
// wraps. Adding a positive offset to a stamp near MinInt64 cannot underflow,
// and the instant it names lands below earliestPlausibleStamp, which refuses it
// there with the message that fits it. No real producer reaches either end,
// since epoch milliseconds and even nanoseconds stay far inside the band, so
// this is the guard against a hand-written or corrupted value rather than
// against a units bug.
const (
	secondsFromYearOneToEpoch = (1969*365 + 1969/4 - 1969/100 + 1969/400) * 24 * 60 * 60
	maxEpochSeconds           = math.MaxInt64 - secondsFromYearOneToEpoch
)

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
					class.Filename, reportPath, joinNames(distinct)),
			}
		}
		if len(distinct) == 0 {
			continue
		}
		path := distinct[0]
		lines, ok := merged[path]
		if !ok {
			// Seeded before the lines are folded in, so a class with no
			// <line> at all still leaves a present key mapping to empty
			// Lines. That entry is the answer join.Attribute reads as an
			// uninstrumented file; skipping such a class would collapse it
			// into the absent-key meaning.
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

// joinNames renders the names one diagnostic quotes as a readable list, so a
// class contradicting itself three ways names all three rather than the first
// two. It is generic over any ~string rather than srcpath.Path alone, so the
// same rendering serves a file_ambiguous diagnostic and the all-superseded
// coverage_stale one, which names []string report names that have no
// srcpath.Path of their own, some of them (a report named on --coverage) not
// even inside the repo. It is "names" rather than "paths" for exactly that:
// what it joins is how the document names a thing, which is not always a path.
// A single element returns alone rather than joining against nothing, which
// the file_ambiguous caller never needs, since a class resolving to one path
// is not ambiguous, but the coverage_stale caller does. Empty returns the
// empty string rather than indexing off the end of the slice; no caller passes
// it today, both being guarded, and a diagnostic quoting nothing is a worse
// fault to hand a developer than a panic is to debug.
func joinNames[T ~string](names []T) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return string(names[0])
	}
	rendered := make([]string, 0, len(names))
	for _, name := range names {
		rendered = append(rendered, string(name))
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
