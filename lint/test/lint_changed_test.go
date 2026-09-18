package lintchanged_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// baseFile is a five-line file, committed as the fixture's starting state,
// so a case can change one line and know which line number moved.
const baseFile = "line1\nline2\nline3\nline4\nline5\n"

// stageEdit commits baseFile at rel, then edits and stages line 3, which is
// the touched line every scenario below is written against.
func stageEdit(f *fixture, rel string) {
	f.write(rel, baseFile)
	f.commitAll("base")
	f.write(rel, "line1\nline2\nCHANGED\nline4\nline5\n")
	f.stage(rel)
}

func filterArgs(extra ...string) []string {
	return append([]string{"--format", "sarif", "--language", "csharp"}, extra...)
}

// 1. A finding on a touched line survives, exit 2.
func TestFindingOnTouchedLineSurvives(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	res := f.run(sarifDoc(sarifResult("TVRM0001", "no getter", sarifLoc("Foo.cs", 3, 3))), filterArgs("--staged")...)

	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "TVRM0001") || !strings.Contains(res.stdout, "Foo.cs:3") {
		t.Fatalf("stdout missing the surviving finding: %s", res.stdout)
	}
	if !strings.Contains(res.stdout, "lint-changed waive --language csharp --path Foo.cs --rule TVRM0001") {
		t.Fatalf("stdout missing the waive command: %s", res.stdout)
	}
}

// 2. A finding on an untouched line of a touched file is dropped, exit 0.
func TestFindingOnUntouchedLineOfTouchedFileDropped(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	res := f.run(sarifDoc(sarifResult("TVRM0001", "no getter", sarifLoc("Foo.cs", 5, 5))), filterArgs("--staged")...)

	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
}

// 3. A finding in a file the diff never touched is dropped, exit 0.
func TestFindingInUntouchedFileDropped(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")
	f.write("Bar.cs", baseFile)
	f.commitAll("add bar")

	res := f.run(sarifDoc(sarifResult("TVRM0001", "no getter", sarifLoc("Bar.cs", 3, 3))), filterArgs("--staged")...)

	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
}

// 4. A multi-line span straddling a touched line survives; one straddling
// none is dropped.
func TestMultiLineSpan(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	straddles := f.run(sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 2, 4))), filterArgs("--staged")...)
	if straddles.exitCode != 2 {
		t.Fatalf("straddling span: exit code = %d, want 2\nstdout: %s", straddles.exitCode, straddles.stdout)
	}

	misses := f.run(sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 1, 1))), filterArgs("--staged")...)
	if misses.exitCode != 0 {
		t.Fatalf("non-straddling span: exit code = %d, want 0\nstdout: %s", misses.exitCode, misses.stdout)
	}
}

// 5. A finding whose primary location is untouched but whose related
// location is touched survives.
func TestRelatedLocationTouchedSurvives(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	res := f.run(sarifDoc(sarifResultRelated("TVRM0001", "msg", sarifLoc("Foo.cs", 5, 5), sarifLoc("Foo.cs", 3, 3))), filterArgs("--staged")...)

	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
}

// 6. IgnoresScope true survives an empty touched map, and it does so in the
// shape Roslyn really emits: Location.None, so no locations array at all.
func TestIgnoresScopeSurvivesEmptyDiff(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	// No staged change: the touched map is empty.

	res := f.run(sarifDoc(sarifResultNoLocation("AD0001", "analyzer crashed")), filterArgs("--staged")...)

	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "AD0001") {
		t.Fatalf("stdout does not name the analyzer load failure: %s", res.stdout)
	}
}

// 7. A matching unspent waiver suppresses a survivor, exit 0, and the
// waiver is spent against the index tree sha.
func TestMatchingWaiverSuppresses(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")
	tree := f.writeTree()

	waived := f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "known false positive")
	if waived.exitCode != 0 {
		t.Fatalf("waive: exit code = %d, stderr: %s", waived.exitCode, waived.stderr)
	}

	res := f.run(sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 3, 3))), filterArgs("--staged")...)
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "suppressed") {
		t.Fatalf("stderr missing the suppression notice: %s", res.stderr)
	}

	lines := f.waiverLogLines()
	if len(lines) != 2 {
		t.Fatalf("waiver log has %d lines, want 2 (record + spend)", len(lines))
	}
	if !strings.Contains(lines[1], tree) {
		t.Fatalf("spend record does not name the index tree %s: %s", tree, lines[1])
	}
}

// 8. Cross-process round trip. First invocation records a waiver. A second,
// fresh process finds it and suppresses the finding. A third, fresh
// process, on a different index tree, does not find it and exits 2.
func TestCrossProcessWaiverRoundTrip(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	waived := f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "known false positive")
	if waived.exitCode != 0 {
		t.Fatalf("waive: exit code = %d, stderr: %s", waived.exitCode, waived.stderr)
	}

	doc := sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 3, 3)))

	suppressed := f.run(doc, filterArgs("--staged")...)
	if suppressed.exitCode != 0 {
		t.Fatalf("second process: exit code = %d, want 0\nstdout: %s\nstderr: %s", suppressed.exitCode, suppressed.stdout, suppressed.stderr)
	}

	// A different index tree: stage a second file so write-tree changes.
	f.write("Bar.cs", baseFile)
	f.stage("Bar.cs")

	notFound := f.run(doc, filterArgs("--staged")...)
	if notFound.exitCode != 2 {
		t.Fatalf("third process, different tree: exit code = %d, want 2\nstdout: %s\nstderr: %s", notFound.exitCode, notFound.stdout, notFound.stderr)
	}
}

// 9. Two runs against the same index tree spend the waiver once, not
// twice, so a retried commit does not burn a second waiver.
func TestSameTreeSpendsWaiverOnce(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "known false positive")
	doc := sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 3, 3)))

	first := f.run(doc, filterArgs("--staged")...)
	second := f.run(doc, filterArgs("--staged")...)
	if first.exitCode != 0 || second.exitCode != 0 {
		t.Fatalf("exit codes = %d, %d, want 0, 0", first.exitCode, second.exitCode)
	}

	lines := f.waiverLogLines()
	if len(lines) != 2 {
		t.Fatalf("waiver log has %d lines, want 2 (one record, one spend)", len(lines))
	}
}

// 10. A staged file whose disk copy differs from the index is a hard stop,
// exit 1, naming the file, even when the report has no finding in it.
func TestStagedFileDirtyIsHardStop(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")
	// Dirty the working copy again without re-staging.
	f.write("Foo.cs", "line1\nline2\nCHANGED\nline4\nDIRTY\n")

	res := f.run(sarifDoc(), filterArgs("--staged")...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "Foo.cs") {
		t.Fatalf("stderr does not name the divergent file: %s", res.stderr)
	}
}

// 11. Malformed stdin exits 1, not 2.
func TestMalformedStdinExitsOne(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	res := f.run("not json", filterArgs("--staged")...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
}

// 12. Missing --format, missing --language, and two of
// --staged/--since/--files together each exit 1 with a message naming what
// is wrong.
func TestUsageErrorsExitOne(t *testing.T) {
	f := newFixture(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		// The whole problem sentence, not the flag name. Every one of these
		// spellings also appears in the usage banner the error prints under
		// itself, so asserting on the flag alone passes against a binary that
		// diagnosed nothing and printed the banner.
		{"missing format", []string{"--language", "csharp", "--staged"}, "lint-changed: --format is required"},
		{"missing language", []string{"--format", "sarif", "--staged"}, "lint-changed: --language is required"},
		{"two scopes", []string{"--format", "sarif", "--language", "csharp", "--staged", "--since", "main"},
			"lint-changed: only one of --staged, --since, --files may be given"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := f.run("", c.args...)
			if res.exitCode != 1 {
				t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
			}
			if !strings.Contains(res.stderr, c.want) {
				t.Fatalf("stderr does not name the problem (%q): %s", c.want, res.stderr)
			}
		})
	}
}

// 13. --files scopes every line of the named files and never consults git
// for a base: it survives a finding even though nothing is staged.
func TestFilesScopesEveryLineWithoutABase(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	// No staged change at all: a --staged run would see an empty diff.

	res := f.run(sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 2, 2))), filterArgs("--files", "Foo.cs")...)

	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
}

// 14. waivers lists recorded waivers with their spend state and exits 0.
func TestWaiversListsRecordedWaivers(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	before := f.run("", "waivers")
	if before.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", before.exitCode)
	}
	if strings.TrimSpace(before.stdout) != "" {
		t.Fatalf("stdout = %q, want empty before any waiver is recorded", before.stdout)
	}

	f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "known false positive")

	afterRecord := f.run("", "waivers")
	if afterRecord.exitCode != 0 || !strings.Contains(afterRecord.stdout, "unspent") {
		t.Fatalf("exit code = %d, stdout = %q, want 0 and an unspent entry", afterRecord.exitCode, afterRecord.stdout)
	}

	f.run(sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 3, 3))), filterArgs("--staged")...)

	afterSpend := f.run("", "waivers")
	if !strings.Contains(afterSpend.stdout, "spent against") {
		t.Fatalf("stdout = %q, want a spent entry", afterSpend.stdout)
	}
}

// 15. Running outside a git repo exits 1 with gitscope's own message.
func TestOutsideGitRepoExitsOne(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()

	res := f.runInDir(outside, sarifDoc(), filterArgs("--staged")...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
	}
	if strings.TrimSpace(res.stderr) == "" {
		t.Fatal("stderr is empty, want gitscope's own message")
	}
}

// One waiver covers one finding. Two findings under the same rule on the same
// file, on two different changed lines, cost two waivers: the second blocks.
func TestOneWaiverCoversOneFinding(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	f.write("Foo.cs", "line1\nCHANGED\nCHANGED\nline4\nline5\n")
	f.stage("Foo.cs")

	recorded := f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "one of the two")
	if recorded.exitCode != 0 {
		t.Fatalf("recording the waiver: exit code = %d\nstderr: %s", recorded.exitCode, recorded.stderr)
	}
	doc := sarifDoc(
		sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 2, 2)),
		sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 3, 3)),
	)

	res := f.run(doc, filterArgs("--staged")...)

	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2: one waiver must not cover both findings\nstdout: %s\nstderr: %s",
			res.exitCode, res.stdout, res.stderr)
	}
	// Which finding survived, not just that one did. Exit 2 alone also comes
	// back from a run where the waiver never matched anything and both findings
	// blocked, which is the opposite of the behaviour under test.
	if !strings.Contains(res.stdout, "Foo.cs:3") {
		t.Fatalf("stdout does not report the unwaived finding at line 3: %s", res.stdout)
	}
	if strings.Contains(res.stdout, "Foo.cs:2") {
		t.Fatalf("stdout reports the waived finding at line 2, so the waiver covered neither: %s", res.stdout)
	}
}

// A waiver is spent only on a run that ends clean. A run the waiver could not
// rescue leaves it unspent, so the agent that fixes the blocking finding still
// has it.
func TestWaiverIsNotSpentOnABlockingRun(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	f.write("Foo.cs", "line1\nCHANGED\nCHANGED\nline4\nline5\n")
	f.stage("Foo.cs")

	f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "a false positive")
	doc := sarifDoc(
		sarifResult("TVRM0001", "the waived one", sarifLoc("Foo.cs", 2, 2)),
		sarifResult("TVRM0002", "the real one", sarifLoc("Foo.cs", 3, 3)),
	)

	blocked := f.run(doc, filterArgs("--staged")...)
	if blocked.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s", blocked.exitCode, blocked.stdout)
	}
	if lines := f.waiverLogLines(); len(lines) != 1 {
		t.Fatalf("waiver log has %d lines, want 1 (the record alone, no spend)", len(lines))
	}

	// The real finding is gone now, and the waiver is still there to spend.
	clean := f.run(sarifDoc(sarifResult("TVRM0001", "the waived one", sarifLoc("Foo.cs", 2, 2))), filterArgs("--staged")...)
	if clean.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", clean.exitCode, clean.stdout, clean.stderr)
	}
	if lines := f.waiverLogLines(); len(lines) != 2 {
		t.Fatalf("waiver log has %d lines, want 2 (record + spend)", len(lines))
	}
}

// A --files entry that is not already repo-relative still scopes the findings
// in it, rather than matching nothing and dropping them all.
func TestFilesAcceptsOtherSpellingsOfThePath(t *testing.T) {
	f := newFixture(t)
	f.write("src/Foo.cs", baseFile)
	f.commitAll("base")

	for _, spelling := range []string{"./src/Foo.cs", filepath.Join(f.root, "src", "Foo.cs")} {
		res := f.run(sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("src/Foo.cs", 2, 2))), filterArgs("--files", spelling)...)
		if res.exitCode != 2 {
			t.Errorf("--files %s: exit code = %d, want 2\nstdout: %s\nstderr: %s", spelling, res.exitCode, res.stdout, res.stderr)
		}
	}
}

// A --files entry naming a path outside the repo fails the run rather than
// scoping to nothing and exiting 0.
func TestFilesOutsideTheRepoExitsOne(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	outside := filepath.Join(t.TempDir(), "Elsewhere.cs")
	if err := os.WriteFile(outside, []byte(baseFile), 0o600); err != nil {
		t.Fatal(err)
	}

	res := f.run(sarifDoc(), filterArgs("--files", outside)...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
	}
}

// A staged whitespace-only edit that is then dirtied on disk is still a hard
// stop. The touched-lines diff ignores whitespace, so the file has no touched
// lines at all, and keying the divergence check on that map would miss it.
func TestWhitespaceOnlyStagedEditStillChecksDivergence(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	f.write("Foo.cs", "line1\nline2   \nline3\nline4\nline5\n")
	f.stage("Foo.cs")
	f.write("Foo.cs", "line1\nline2   \nline3\nline4\nDIRTY\n")

	res := f.run(sarifDoc(), filterArgs("--staged")...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "Foo.cs") {
		t.Fatalf("stderr does not name the divergent file: %s", res.stderr)
	}
}

// --since scopes against an explicit ref rather than the index.
func TestSinceScopesAgainstNamedRef(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	base := f.git("rev-parse", "HEAD")
	f.write("Foo.cs", "line1\nline2\nCHANGED\nline4\nline5\n")
	f.commitAll("edit")

	res := f.run(sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("Foo.cs", 3, 3))), filterArgs("--since", base)...)

	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
}

// A staged deletion whose file is still on disk is a hard stop. git reads a
// path the index no longer holds as untracked, so no index-to-worktree diff
// names it, and the build just compiled a file the commit removes.
func TestStagedDeletionStillOnDiskIsDivergent(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	f.git("rm", "--cached", "--quiet", "Foo.cs")

	res := f.run(sarifDoc(), filterArgs("--staged")...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "Foo.cs") {
		t.Fatalf("stderr does not name the staged deletion: %s", res.stderr)
	}
}

// A report whose every result fell outside the repo checked nothing, so it
// fails rather than passing the commit on a report it never read.
func TestEveryResultUnplaceableExitsOne(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")

	doc := sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("/elsewhere/Other.cs", 3, 3)))
	res := f.run(doc, filterArgs("--staged")...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "TVRM0001") || !strings.Contains(res.stderr, "Other.cs") {
		t.Fatalf("stderr does not name the dropped rule and its URI: %s", res.stderr)
	}
}

// An analyzer load failure has a route out. It is reported at no location at
// all, so the printed command carries no --path and the waiver keys on the
// language and the rule alone.
func TestAnalyzerLoadFailureCanBeWaived(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")
	doc := sarifDoc(sarifResultNoLocation("AD0001", "analyzer threw"))

	blocked := f.run(doc, filterArgs("--staged")...)
	if blocked.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", blocked.exitCode, blocked.stdout, blocked.stderr)
	}
	if strings.Contains(blocked.stdout, "--path") {
		t.Fatalf("the printed command names a path the finding does not have: %s", blocked.stdout)
	}

	recorded := f.run("", "waive", "--language", "csharp", "--rule", "AD0001", "--reason", "broken upstream")
	if recorded.exitCode != 0 {
		t.Fatalf("recording the waiver: exit code = %d\nstderr: %s", recorded.exitCode, recorded.stderr)
	}

	res := f.run(doc, filterArgs("--staged")...)
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0: the pathless waiver did not cover the load failure\nstdout: %s\nstderr: %s",
			res.exitCode, res.stdout, res.stderr)
	}
}

// Two reports, one process, one waiver. A process per report spends whatever
// matches its own report without seeing that another report still blocks the
// commit, which burns the waiver on a commit that never went through.
func TestReportsAreFilteredInOneRun(t *testing.T) {
	f := newFixture(t)
	f.write("Foo.cs", baseFile)
	f.commitAll("base")
	f.write("Foo.cs", "line1\nCHANGED\nCHANGED\nline4\nline5\n")
	f.stage("Foo.cs")

	recorded := f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "a false positive")
	if recorded.exitCode != 0 {
		t.Fatalf("recording the waiver: exit code = %d\nstderr: %s", recorded.exitCode, recorded.stderr)
	}

	waived := filepath.Join(t.TempDir(), "waived.sarif")
	blocking := filepath.Join(t.TempDir(), "blocking.sarif")
	if err := os.WriteFile(waived, []byte(sarifDoc(sarifResult("TVRM0001", "the waived one", sarifLoc("Foo.cs", 2, 2)))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocking, []byte(sarifDoc(sarifResult("TVRM0002", "the real one", sarifLoc("Foo.cs", 3, 3)))), 0o600); err != nil {
		t.Fatal(err)
	}

	res := f.run("", filterArgs("--staged", "--report", waived, "--report", blocking)...)

	if res.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if lines := f.waiverLogLines(); len(lines) != 1 {
		t.Fatalf("waiver log has %d line(s), want 1: the waiver was spent on a run that blocked", len(lines))
	}
}

// writeReport puts a SARIF document in its own file and returns the path, for
// the cases that drive --report rather than stdin.
func writeReport(t *testing.T, name, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// One source-level warning is one finding however many reports carry it. A
// net8.0;net48 project compiles once per framework and writes one report per
// framework, and a .cs linked into two projects is compiled by both, so the
// same warning arrives twice by either route. Reported twice it would print two
// identical waive commands and cost two waivers for one line of code, which
// leaves --no-verify as the only way past a false positive.
func TestDuplicateFindingAcrossReportsIsReportedOnce(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")
	doc := sarifDoc(sarifResult("TVRM0001", "no getter", sarifLoc("Foo.cs", 3, 3)))
	args := filterArgs("--staged",
		"--report", writeReport(t, "net8.0.sarif", doc),
		"--report", writeReport(t, "net48.sarif", doc))

	blocked := f.run("", args...)

	if blocked.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", blocked.exitCode, blocked.stdout, blocked.stderr)
	}
	if got := strings.Count(blocked.stdout, "TVRM0001:"); got != 1 {
		t.Fatalf("the finding is reported %d times, want 1: %s", got, blocked.stdout)
	}
	if got := strings.Count(blocked.stdout, "waive --language"); got != 1 {
		t.Fatalf("%d waive commands printed, want 1: %s", got, blocked.stdout)
	}

	recorded := f.run("", "waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "TVRM0001", "--reason", "a false positive")
	if recorded.exitCode != 0 {
		t.Fatalf("recording the waiver: exit code = %d\nstderr: %s", recorded.exitCode, recorded.stderr)
	}

	res := f.run("", args...)
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0: the single printed waiver did not clear the run\nstdout: %s\nstderr: %s",
			res.exitCode, res.stdout, res.stderr)
	}
}

// The unplaceable-report hard stop is asked of each report on its own. A
// project whose SARIF URIs all resolve outside the repo checked no code at all,
// and merging its counts with another report's lets it ride on results it did
// not produce.
func TestUnplaceableReportBesideAPlaceableOneExitsOne(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")
	outside := writeReport(t, "outside.sarif",
		sarifDoc(sarifResult("TVRM0001", "msg", sarifLoc("/elsewhere/Other.cs", 3, 3))))
	// Placeable but out of scope, so the run would otherwise end clean at 0.
	inside := writeReport(t, "inside.sarif",
		sarifDoc(sarifResult("TVRM0002", "msg", sarifLoc("Foo.cs", 5, 5))))

	res := f.run("", filterArgs("--staged", "--report", outside, "--report", inside)...)

	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "outside.sarif") {
		t.Fatalf("stderr does not name the report that placed nothing: %s", res.stderr)
	}
}

// Roslyn reports CS1701, CS1702, CS8021 and the command-line CS2xxx warnings at
// Location.None, writes a diagnostic about a whole document with no region, and
// names source-generated documents that were never written to disk. Every one
// of those reports checked the code, so none of them is the unplaceable-report
// hard stop, which exists for a report describing another tree. Tripping on one
// refuses the commit with no waiver route out, since a dropped result never
// becomes a waivable finding.
func TestOrdinaryDropCausesAreNotTheHardStop(t *testing.T) {
	cases := []struct {
		name   string
		result string
	}{
		{"no location at all", sarifResultNoLocation("CS8021", "no value for RazorTargetName")},
		{"no region on an in-repo path", sarifResult("CS0219", "assigned but never used", sarifLocNoRegion("Foo.cs"))},
		{"a generated document not on disk", sarifResult("CS8618", "non-nullable field", sarifLoc("Gen/Logging.g.cs", 3, 3))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			stageEdit(f, "Foo.cs")

			res := f.run(sarifDoc(tc.result), filterArgs("--staged")...)

			if res.exitCode != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", res.exitCode, res.stdout, res.stderr)
			}
		})
	}
}

// A report that checked nothing never swallows a finding another report found.
// The hard stop is the tool breaking and prints on stderr, while a surviving
// finding is the commit's own problem and has to reach stdout with its waive
// command, at exit 2, or the developer is told to fix a report instead of the
// code.
func TestARealFindingOutranksAnUncheckedReport(t *testing.T) {
	f := newFixture(t)
	stageEdit(f, "Foo.cs")
	found := writeReport(t, "real.sarif",
		sarifDoc(sarifResult("TVRM0001", "no getter", sarifLoc("Foo.cs", 3, 3))))
	noloc := writeReport(t, "noloc.sarif", sarifDoc(sarifResultNoLocation("CS1701", "assuming assembly reference")))
	outside := writeReport(t, "outside.sarif",
		sarifDoc(sarifResult("TVRM0002", "msg", sarifLoc("/elsewhere/Other.cs", 3, 3))))

	both := f.run("", filterArgs("--staged", "--report", found, "--report", noloc)...)
	if both.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", both.exitCode, both.stdout, both.stderr)
	}
	if !strings.Contains(both.stdout, "TVRM0001") || !strings.Contains(both.stdout, "waive --language") {
		t.Fatalf("the finding or its waive command never reached stdout: %s", both.stdout)
	}

	// The reports are given unchecked-first, since a run that stopped at the
	// first failure would never open the one holding the finding.
	withOutside := f.run("", filterArgs("--staged", "--report", outside, "--report", found)...)
	if withOutside.exitCode != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", withOutside.exitCode, withOutside.stdout, withOutside.stderr)
	}
	if !strings.Contains(withOutside.stdout, "TVRM0001") {
		t.Fatalf("the finding never reached stdout: %s", withOutside.stdout)
	}
	if !strings.Contains(withOutside.stderr, "outside.sarif") {
		t.Fatalf("stderr does not name the report that checked nothing: %s", withOutside.stderr)
	}
}

// Every form of commit is driven through a real hook, because the form decides
// which index git hands it. `git commit -a` and `git commit -- <pathspec>`
// build a temporary index and name it in GIT_INDEX_FILE, so a gate that reads
// .git/index reads content matching HEAD and passes every finding, and the
// commit goes through carrying the code the gate was pointed away from. A
// pre-staged index is the one case that already worked, which is why it is the
// third assertion here rather than the only one.
func TestPreCommitHookBlocksEveryFormOfCommit(t *testing.T) {
	forms := []struct {
		name   string
		commit []string
		stage  bool
	}{
		{"commit -a", []string{"commit", "-a", "-m", "edit"}, false},
		{"commit with a pathspec", []string{"commit", "-m", "edit", "--", "Foo.cs"}, false},
		{"add then commit", []string{"commit", "-m", "edit"}, true},
	}
	for _, form := range forms {
		t.Run(form.name, func(t *testing.T) {
			f := newFixture(t)
			f.write("Foo.cs", baseFile)
			f.commitAll("base")
			f.installPreCommitHook(writeReport(t, "build.sarif",
				sarifDoc(sarifResult("TVRM0001", "no getter", sarifLoc("Foo.cs", 3, 3)))))
			base := f.head()
			f.write("Foo.cs", "line1\nline2\nCHANGED\nline4\nline5\n")
			if form.stage {
				f.stage("Foo.cs")
			}

			code, out := f.gitTry(form.commit...)

			if code == 0 {
				t.Fatalf("the commit went through with a finding on the changed line: %s", out)
			}
			if !strings.Contains(out, "TVRM0001") {
				t.Fatalf("the hook output does not name the finding: %s", out)
			}
			if f.head() != base {
				t.Fatalf("HEAD moved to %s, so the refused commit was written anyway", f.head())
			}
		})
	}
}
