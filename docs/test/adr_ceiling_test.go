// Package docs_test holds the mechanical checks over the records this repo
// keeps in prose, the way gate/test holds them over the gate binary. The
// ADR word ceilings are the first: docs/adr/README.md has stated them since
// the conventions were written, nothing measured them, and the counts the
// README carried beside them had to be hand-updated on every edit and went
// stale the first time one changed (issue 87). A rule a human has to
// remember to measure is not a rule, so it is measured here and the counts
// are gone from the prose.
package docs_test

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// adrGlob selects the records and nothing else. README.md sits in the same
// directory and is an index rather than a decision, so it is asked to meet
// neither ceiling.
const adrGlob = "../adr/[0-9]*.md"

// currentRuleHeading opens the block the smaller ceiling covers. Every ADR
// carries one until it is superseded, which is what the block-missing
// failure below asserts.
const currentRuleHeading = "## Current rule"

// The two ceilings docs/adr/README.md sets under "Conventions": the block a
// reader is expected to read in full, and the file they scroll when the
// block is not enough.
//
// Measured with wc -w semantics, whitespace-separated fields, so markdown
// syntax, link URLs and code spans all count as words. That is how the
// counts the README used to quote were taken, and a rule that measured
// differently would silently move the ceiling it claims to enforce.
const (
	blockCeiling = 250
	fileCeiling  = 1500
)

// limits is one ADR's two ceilings once any override is applied.
type limits struct {
	block int
	file  int
}

// ceilingOverride raises one ADR's limit to the length that ADR already had
// when this check landed. It is a ratchet rather than an exemption: the file
// may shrink freely and may not grow past what it was, and once it meets the
// standard ceiling the entry has to go, which
// TestGrandfatheredEntriesAreStillEarned enforces. A zero field takes the
// standard ceiling.
//
// Recording the exact current count instead was rejected. It would red every
// commit that moved one of these files by a single word, the consolidations
// that shrink them included, and it would be the README's hand-updated
// counts again under a different roof. A limit cannot go stale the way a
// measurement does.
//
// Superseded records take no entry. They are exempt by their status, read
// off the file, so the exemption cannot outlive the supersession.
type ceilingOverride struct {
	limits
	why string
}

// grandfathered is every ADR that was already over a ceiling when this check
// landed. Each is a real breach with a named next move, not a disagreement
// with the ceiling.
var grandfathered = map[string]ceilingOverride{
	"0004-source-paths-are-repo-relative-and-resolved-deterministically.md": {
		limits: limits{file: 2193},
		why:    "consolidated once already, and README's Conventions section names a split as the next move on it",
	},
	"0007-changed-method-is-a-span-holding-a-touched-line.md": {
		limits: limits{file: 1660},
		why:    "an amendment pushed it over, which is what surfaced issue 87",
	},
	"0010-lint-blocks-on-any-warning-touching-a-changed-line.md": {
		limits: limits{block: 686},
		why:    "five dated amendments landed inside the Current rule block, so folding them back is a consolidation",
	},
}

// breachKind is which of the check's three failures a record hit.
type breachKind int

const (
	noCurrentRuleBlock breachKind = iota
	blockOverCeiling
	fileOverCeiling
)

// breach is one failure, as a value rather than a sentence, so the fixture
// table below can assert what the check reported and not how it reads.
type breach struct {
	name  string
	kind  breachKind
	words int
	limit int
}

// breaches is the whole of the check issue 87 asked for, over whatever set of
// records it is handed and whatever raised limits apply to them. Superseded
// records are exempt by their status and produce nothing.
func breaches(records []adr, overrides map[string]ceilingOverride) []breach {
	var found []breach
	for _, record := range records {
		if record.superseded {
			continue
		}
		if !record.hasBlock {
			found = append(found, breach{name: record.name, kind: noCurrentRuleBlock})
			continue
		}
		limit := ceilingsFor(record.name, overrides)
		if record.blockWords > limit.block {
			found = append(found, breach{record.name, blockOverCeiling, record.blockWords, limit.block})
		}
		if record.fileWords > limit.file {
			found = append(found, breach{record.name, fileOverCeiling, record.fileWords, limit.file})
		}
	}
	return found
}

// TestEveryLiveADRMeetsItsCeilings runs the check over docs/adr. It reports
// the file, the measured count and the ceiling it broke, so the failure
// carries everything needed to act on it.
func TestEveryLiveADRMeetsItsCeilings(t *testing.T) {
	records, err := readADRs(adrGlob)
	if err != nil {
		t.Fatal(err)
	}

	for _, b := range breaches(records, grandfathered) {
		switch b.kind {
		case noCurrentRuleBlock:
			t.Errorf("%s carries no %q heading, so the block a reader is told to read in full does not exist",
				b.name, currentRuleHeading)
		case blockOverCeiling:
			t.Errorf("%s: the Current rule block is %d words, over its %d-word ceiling. A decision that will not fit in the block is more than one decision; docs/adr/README.md's Conventions section has the four ways to change an ADR",
				b.name, b.words, b.limit)
		case fileOverCeiling:
			t.Errorf("%s: the file is %d words, over its %d-word ceiling. Length is the trigger to consolidate, split or trim, and every one of those is a decision to escalate to the user rather than to make here",
				b.name, b.words, b.limit)
		}
	}
}

// TestGrandfatheredEntriesAreStillEarned keeps the ratchet from rusting. An
// entry naming no file, or one whose file now meets the standard ceiling, is
// a raised limit nothing needs, and leaving it in place would let the file
// grow straight back to it.
func TestGrandfatheredEntriesAreStillEarned(t *testing.T) {
	records, err := readADRs(adrGlob)
	if err != nil {
		t.Fatal(err)
	}

	measured := map[string]adr{}
	for _, record := range records {
		measured[record.name] = record
	}

	for _, name := range slices.Sorted(maps.Keys(grandfathered)) {
		over := grandfathered[name]
		record, ok := measured[name]
		if !ok {
			t.Errorf("grandfathered names %s (%s), which %s matches no file for. Delete the entry, or fix the name",
				name, over.why, adrGlob)
			continue
		}
		if over.block > 0 && record.blockWords <= blockCeiling {
			t.Errorf("%s: the Current rule block is %d words, inside the %d-word ceiling, so the raised limit of %d is no longer earned (%s). Delete the block field",
				name, record.blockWords, blockCeiling, over.block, over.why)
		}
		if over.file > 0 && record.fileWords <= fileCeiling {
			t.Errorf("%s: the file is %d words, inside the %d-word ceiling, so the raised limit of %d is no longer earned (%s). Delete the file field",
				name, record.fileWords, fileCeiling, over.file, over.why)
		}
	}
}

// adr is one record on disk, measured. Name is the base name, which is the
// key grandfathered uses, since the directory is fixed by adrGlob.
type adr struct {
	name       string
	superseded bool
	hasBlock   bool
	blockWords int
	fileWords  int
}

// readADRs measures every record the glob matches, in the sorted order
// filepath.Glob returns. A glob matching nothing is a failure rather than a
// vacuous pass: this check would otherwise stay green on a renamed directory
// while measuring nothing at all, the same guard TestInternalDoesNotDependOnGate
// makes about an empty dependency graph.
func readADRs(glob string) ([]adr, error) {
	paths, err := filepath.Glob(glob)
	if err != nil {
		return nil, fmt.Errorf("globbing %s: %w", glob, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s matched no file, so this check walked an empty set and would pass whatever the directory holds", glob)
	}

	records := make([]adr, 0, len(paths))
	for _, path := range paths {
		text, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		records = append(records, measure(filepath.Base(path), string(text)))
	}
	return records, nil
}

// measure counts one record whole and again over its Current rule block.
//
// Superseded is read off the `**Superseded YYYY-MM-DD by ...**` paragraph
// the supersede convention puts under the title, rather than off a list kept
// here. 0003, 0005 and 0006 are kept for their history and not for their
// rules, so neither ceiling is asked of them, and a record superseded later
// is exempt the moment the paragraph lands.
func measure(name, text string) adr {
	block, found := currentRule(text)
	return adr{
		name:       name,
		superseded: strings.Contains(preamble(text), "\n**Superseded "),
		hasBlock:   found,
		blockWords: words(block),
		fileWords:  words(text),
	}
}

// preamble is the text above the first level-2 heading, which is where the
// supersede convention puts the status paragraph. Reading the whole file
// instead would exempt a live record that merely quotes the convention, and a
// record skipped that way fails nothing while measuring nothing.
func preamble(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			return strings.Join(lines[:i], "\n")
		}
	}
	return text
}

// words is wc -w: whitespace-separated fields, nothing stripped.
func words(text string) int { return len(strings.Fields(text)) }

// currentRule is the text under the Current rule heading, up to the next
// level-2 heading, and whether the heading was there at all.
//
// A `### ` subheading does not close the block. It is a subsection of the
// rule, and a reader told to read the block in full reads it too, so
// counting it out would let a block grow without bound behind one subheading.
//
// A ``` fence is tracked, so a `## ` line inside a fenced example is markdown
// the block shows rather than the heading that ends it. Closing on one would
// drop the rest of the block from the count and take the ceiling off it.
func currentRule(text string) (string, bool) {
	var block []string
	inside, fenced := false, false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
		}
		switch {
		case !fenced && trimmed == currentRuleHeading:
			inside = true
		case inside && !fenced && strings.HasPrefix(trimmed, "## "):
			return strings.Join(block, "\n"), true
		case inside:
			block = append(block, line)
		}
	}
	return strings.Join(block, "\n"), inside
}

// ceilingsFor is the standard pair unless this record carries an override.
// A missing entry and an entry raising only one of the two leave the same
// zero behind, which both read as the standard ceiling.
func ceilingsFor(name string, overrides map[string]ceilingOverride) limits {
	limit := overrides[name].limits
	if limit.block == 0 {
		limit.block = blockCeiling
	}
	if limit.file == 0 {
		limit.file = fileCeiling
	}
	return limit
}
