package docs_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// No live record under docs/adr breaks a ceiling, is missing its heading or
// holds a fence inside its Current rule block, so TestEveryLiveADRMeetsItsCeilings
// walks only the passing direction and would stay green if the measuring
// lifted a ceiling, truncated a block or skipped a record. These fixtures
// drive the failing direction over records written for the purpose.

// filler is n words of wc -w's kind, so a fixture can be built to a known
// count and the expected measurement worked out by hand.
func filler(n int) string { return strings.TrimRight(strings.Repeat("word ", n), " ") }

// writeRecords lays the fixtures out as files and returns a glob over them,
// so the check reads them the way it reads docs/adr rather than through a
// seam built for the test.
func writeRecords(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return filepath.Join(dir, "[0-9]*.md")
}

// fixtures is one record per way the check has to fail, plus the two ways it
// has to stay quiet. Every count below is hand-worked from filler's argument
// plus the words in the headings and prose around it.
func fixtures() map[string]string {
	return map[string]string{
		// 251-word block, one word over the standard ceiling.
		"0001-over-the-block-ceiling.md": "# Fixture one\n\n" + currentRuleHeading + "\n\n" + filler(251) + "\n",

		// The same breach, raised past by the override the table passes in.
		"0002-block-raised-by-an-override.md": "# Fixture two\n\n" + currentRuleHeading + "\n\n" + filler(251) + "\n",

		// 1518 words whole, with a block well inside its own ceiling.
		"0003-over-the-file-ceiling.md": "# Fixture three\n\n" + currentRuleHeading + "\n\n" + filler(10) +
			"\n\n## Reasoning\n\n" + filler(1500) + "\n",

		"0004-no-current-rule-heading.md": "# Fixture four\n\nProse and no block at all.\n",

		// A fenced example holding a `## ` line. Counted correctly the block
		// runs to 270 words; closing it at the fenced line reads 201 and the
		// breach disappears.
		"0005-fence-inside-the-block.md": "# Fixture five\n\n" + currentRuleHeading + "\n\n" + filler(200) +
			"\n\n```markdown\n## Not a heading, it is an example\n```\n\n" + filler(60) + "\n",

		// A `### ` subheading is part of the rule, so the block runs to 266.
		"0006-subheading-inside-the-block.md": "# Fixture six\n\n" + currentRuleHeading + "\n\n" + filler(200) +
			"\n\n### A subsection of the rule\n\n" + filler(60) + "\n",

		// Live, and it documents the supersede convention in its body. Reading
		// the whole file for the paragraph would exempt it and report nothing.
		"0007-quotes-the-supersede-convention.md": "# Fixture seven\n\n" + currentRuleHeading + "\n\n" + filler(251) +
			"\n\n## How to supersede\n\nWrite this under the title:\n\n" +
			"**Superseded 2026-01-01 by [ADR 9999](9999-x.md).** The decision is unchanged.\n",

		// Genuinely superseded, and over both ceilings. Exempt by its status.
		"0008-superseded-and-far-too-long.md": "# Fixture eight\n\n" +
			"**Superseded 2026-01-01 by [ADR 9999](9999-x.md).** Kept for its history.\n\n" +
			currentRuleHeading + "\n\n" + filler(2000) + "\n",
	}
}

// TestTheCheckReportsEveryBreach drives the failing direction of the ceiling
// check and pins exactly which records it reports and at what count, so a
// regression that widened a limit or shortened a block reds here.
func TestTheCheckReportsEveryBreach(t *testing.T) {
	overrides := map[string]ceilingOverride{
		"0002-block-raised-by-an-override.md": {limits: limits{block: 300}, why: "fixture"},
	}

	records, err := readADRs(writeRecords(t, fixtures()))
	if err != nil {
		t.Fatalf("readADRs: %v", err)
	}

	want := []breach{
		{"0001-over-the-block-ceiling.md", blockOverCeiling, 251, blockCeiling},
		{"0003-over-the-file-ceiling.md", fileOverCeiling, 1518, fileCeiling},
		{name: "0004-no-current-rule-heading.md", kind: noCurrentRuleBlock},
		{"0005-fence-inside-the-block.md", blockOverCeiling, 270, blockCeiling},
		{"0006-subheading-inside-the-block.md", blockOverCeiling, 266, blockCeiling},
		{"0007-quotes-the-supersede-convention.md", blockOverCeiling, 251, blockCeiling},
	}
	assertBreaches(t, breaches(records, overrides), want)

	// 0002 is silent above because a raised limit covers it, not because
	// anything exempted it. Dropping the override has to bring it back and
	// move nothing else.
	raised := breach{"0002-block-raised-by-an-override.md", blockOverCeiling, 251, blockCeiling}
	standard := breaches(records, nil)
	if len(standard) != len(want)+1 || !slices.Contains(standard, raised) {
		t.Errorf("with no override the check reported %+v, want the %d breach(es) above plus %+v",
			standard, len(want), raised)
	}
}

// TestAnEmptyRecordSetIsAnError is the vacuous-pass guard. A glob that stops
// matching, because the directory moved or the naming changed, has to stop
// the check rather than walk an empty set quietly.
func TestAnEmptyRecordSetIsAnError(t *testing.T) {
	if _, err := readADRs(writeRecords(t, nil)); err == nil {
		t.Error("readADRs accepted a glob matching no file, so the check would pass whatever docs/adr holds")
	}
}

func assertBreaches(t *testing.T, got, want []breach) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("the check reported %d breach(es), want %d\ngot  %+v\nwant %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("breach %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
