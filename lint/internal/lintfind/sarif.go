package lintfind

import (
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// sarifVersion is the only schema version this parser speaks. Roslyn's
// <ErrorLog> has emitted 2.1.0 since VS 2019; anything else is a shape this
// package has never read and must not guess at.
const sarifVersion = "2.1.0"

// sarifLog is the subset of a SARIF 2.1 log this parser reads.
type sarifLog struct {
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Results []sarifResult `json:"results"`
}

type sarifResult struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations        []sarifLocation    `json:"locations"`
	RelatedLocations []sarifLocation    `json:"relatedLocations"`
	Suppressions     []sarifSuppression `json:"suppressions"`
}

// sarifSuppression is one entry of a result's suppressions array. Roslyn
// writes one with kind "inSource" for a diagnostic a #pragma warning disable
// or a [SuppressMessage] turned off in the repo being built.
type sarifSuppression struct {
	Kind string `json:"kind"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region struct {
			StartLine int `json:"startLine"`
			EndLine   int `json:"endLine"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

// ParseSARIF reads a SARIF 2.1 log and returns every result it can place
// inside root. The int is the count of results dropped as unplaceable.
func ParseSARIF(r io.Reader, root srcpath.Root) ([]Finding, int, error) {
	var log sarifLog
	if err := json.NewDecoder(r).Decode(&log); err != nil {
		return nil, 0, UnreadableReportError{Format: "sarif", Message: "malformed JSON: " + err.Error()}
	}
	if log.Version != sarifVersion {
		return nil, 0, UnreadableReportError{Format: "sarif", Message: "unsupported version " + quoteVersion(log.Version) + ", want " + sarifVersion}
	}

	var findings []Finding
	dropped := 0
	for _, run := range log.Runs {
		for _, result := range run.Results {
			if suppressedInSource(result) {
				continue
			}
			finding, err := placeResult(result, root)
			if err != nil {
				return nil, 0, err
			}
			if finding == nil {
				dropped++
				continue
			}
			findings = append(findings, *finding)
		}
	}
	return findings, dropped, nil
}

// suppressedInSource reports whether the target repo already turned this
// diagnostic off with a #pragma or a [SuppressMessage]. ErrorLog reports
// suppressed diagnostics the console never prints, and another repo's
// suppression is its own decision to make, so the result is dropped before
// it can block a commit.
func suppressedInSource(result sarifResult) bool {
	for _, s := range result.Suppressions {
		if s.Kind == "inSource" {
			return true
		}
	}
	return false
}

// placeResult turns one SARIF result into a Finding, or nil when every one
// of its locations falls outside root (behaviour 4, 6). An error means the
// result itself is malformed rather than merely unplaceable.
func placeResult(result sarifResult, root srcpath.Root) (*Finding, error) {
	if result.RuleID == "" {
		return nil, UnreadableReportError{Format: "sarif", Message: "result has no ruleId, so it cannot be waived"}
	}

	locations := placeLocations(result.Locations, root)
	locations = append(locations, placeLocations(result.RelatedLocations, root)...)
	// A rule that ignores scope is kept whatever its locations say. Roslyn
	// reports all four of them at Location.None, so the result carries no
	// locations array at all and dropping it here would mean an analyzer that
	// failed to load never blocked anything.
	if len(locations) == 0 && !ignoresScope(result.RuleID) {
		return nil, nil
	}

	severity := result.Level
	if severity == "" {
		severity = "warning"
	}

	return &Finding{
		Rule:         result.RuleID,
		Message:      result.Message.Text,
		Severity:     severity,
		IgnoresScope: ignoresScope(result.RuleID),
		Locations:    locations,
	}, nil
}

// placeLocations places every sarifLocation it can inside root, in order,
// silently dropping the ones it cannot (behaviour 4, 6).
func placeLocations(sarifLocations []sarifLocation, root srcpath.Root) []Location {
	var locations []Location
	for _, loc := range sarifLocations {
		if placed, ok := placeLocation(loc, root); ok {
			locations = append(locations, placed)
		}
	}
	return locations
}

// placeLocation reads one sarifLocation's region and artifactLocation.uri
// and places it inside root. ok is false when the region is absent, has no
// startLine, or the URI does not resolve inside root (behaviour 6, 4).
func placeLocation(loc sarifLocation, root srcpath.Root) (Location, bool) {
	region := loc.PhysicalLocation.Region
	if region.StartLine == 0 {
		return Location{}, false
	}
	endLine := region.EndLine
	if endLine == 0 {
		endLine = region.StartLine
	}

	path, ok := resolveURI(loc.PhysicalLocation.ArtifactLocation.URI, root)
	if !ok {
		return Location{}, false
	}
	return Location{Path: path, StartLine: region.StartLine, EndLine: endLine}, true
}

// ignoresScope reports whether rule names an analyzer that failed to load
// rather than a real diagnostic. AD0001, CS8032, CS8034 and CS9057 mean the
// analyzer itself could not run, so a clean result under it proves nothing
// about the code and the diff underneath is irrelevant to the finding.
func ignoresScope(rule string) bool {
	switch rule {
	case "AD0001", "CS8032", "CS8034", "CS9057":
		return true
	default:
		return false
	}
}

// resolveURI turns a SARIF artifactLocation.uri into a candidate absolute
// path and places it inside root. A "file://" URI is decoded and the file
// path taken from it; anything else is read as a plain path, relative or
// absolute, and joined onto root's directory when it is relative, since
// srcpath.Root.Place only resolves absolute candidates.
func resolveURI(uri string, root srcpath.Root) (srcpath.Path, bool) {
	candidate := uri
	if parsed, err := url.Parse(uri); err == nil && parsed.Scheme == "file" {
		candidate = parsed.Path
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root.Dir(), filepath.FromSlash(candidate))
	}
	return root.Place(candidate).Inside()
}

// quoteVersion renders an absent version distinctly from an empty string
// version, so the message names what was actually wrong.
func quoteVersion(v string) string {
	if v == "" {
		return "(absent)"
	}
	return `"` + v + `"`
}
