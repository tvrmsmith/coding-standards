package gate_test

import (
	"os"
	"path/filepath"
	"testing"
)

// usage is the block every scope.UsageError renders beneath its specific
// problem, pinned here once so a case's stderr assertion states only the
// problem line it is actually testing.
const usage = "\n\nusage: metric-gate [--staged | --since <ref> | --files <path>...] [--coverage <path>]...\n" +
	"  (no flag)          the merge base of HEAD and the default branch\n" +
	"  --staged           what a commit would contain\n" +
	"  --since <ref>      the merge base of HEAD and <ref>\n" +
	"  --files <path>...  every method in each named file\n" +
	"  --coverage <path>  read this report instead of discovering one; repeatable\n"

func TestUnknownArgumentIsAUsageErrorNotExitTwo(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs("--bogus")

	assertUsageError(t, result, "metric-gate: unknown argument '--bogus'"+usage)
}

func TestPositionalArgumentIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs(orderService)

	assertUsageError(t, result,
		"metric-gate: unexpected argument '"+orderService+"'; there are no positional arguments"+usage)
}

func TestTwoScopesAtOnceIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs("--since", "main", "--staged")

	assertUsageError(t, result, "metric-gate: --staged and --since name two scopes; pass one"+usage)
}

func TestSinceMeasuresEveryMethodChangedAcrossTheWholeBranch(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// release sits outside gitscope.BaseCandidates, so a run that ignored the
	// ref and fell back to the default resolution would label its base main
	// and this case would go red on that line alone.
	f.git("branch", "release")

	f.git("checkout", "--quiet", "-b", "feature")
	f.touchLine(orderService, 62)
	f.commitAll("edit Cancel")
	f.touchLine(orderService, 45)
	f.commitAll("edit PlaceAsync")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--since", "release").assertMatches(t, "since_whole_branch", 0, f.baseLabel("release"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestSinceMeasuresTheBranchPointNotTheTipOfTheNamedRef(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.git("checkout", "--quiet", "-b", "feature")
	f.touchLine(orderService, 62)
	f.commitAll("edit Cancel on the branch")

	f.git("checkout", "--quiet", "main")
	f.touchLine(orderService, 45)
	f.commitAll("edit PlaceAsync on main")
	// release names main's tip, and nothing in gitscope.BaseCandidates names
	// release, so this case is red both if the run measures the tip rather
	// than the branch point and if it ignores the ref it was handed.
	f.git("branch", "release")
	f.git("checkout", "--quiet", "feature")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--since", "release").assertMatches(t, "since_single_method", 0, f.baseLabel("release"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestSinceNamingARefThatDoesNotExistFailsNamingThatRef(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--since", "nope").assertMatches(t, "since_ref_unresolvable", 1, "",
		"no diff base: --since nope does not name a commit\n")
}

func TestStagedMeasuresOnlyWhatIsStaged(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(otherService, csharpFile(30))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.touchLine(otherService, 12)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	// The stub parses only orderService and reports only cancel. Under the
	// default merge-base scope the gate would hand the extractor Other.cs as
	// well, and the stub's silence about it would fail the run with
	// extractor_no_parse_status instead of producing this document, which is
	// what makes --staged excluding the unstaged edit discriminating.
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_single_method", 0, f.headLabel(),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestStagedRefusesAFileStagedInOneStateAndDirtyInAnother(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.touchLine(orderService, 45)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_file_dirty", 1, f.headLabel(),
		"refusing to score src/Ordering/OrderService.cs: staged in one state and on disk in another\n")
}

func TestFilesNamingAPathOutsideTheRepoRootFailsNamingThatPath(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.write("../outside.cs", csharpFile(10))

	f.runArgs("--files", "../outside.cs").assertMatches(t, "files_outside_repo", 1, "",
		"../outside.cs is outside the repo root\n")
}

func TestFilesNamingAPathThatDoesNotExistFailsNamingThatPath(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--files", "src/Ordering/Missing.cs").assertMatches(t, "files_missing", 1, "",
		"src/Ordering/Missing.cs does not exist\n")
}

func TestFilesMeasuresEveryMethodInTheNamedFilesRegardlessOfTheDiff(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(otherService, csharpFile(30))
	f.commitAll("initial")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)},
		coverageClass{filename: otherService, lines: spanCoverage(11, 4, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService, otherService), []span{placeAsync, cancel, other}),
	}

	f.runArgs("--files", orderService, otherService).assertMatches(t, "files_named_directly", 0, "",
		"0 of 3 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesListsAFileNoExtractorHandlesAsSkippedRatherThanMeasured(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--files", "docs/notes.md", orderService).assertMatches(t, "files_with_skipped_path", 0, "",
		"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestSinceWithNoRefIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs("--since")

	assertUsageError(t, result, "metric-gate: --since needs a ref"+usage)
}

func TestSinceFollowedByAnotherFlagIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs("--since", "--staged")

	assertUsageError(t, result, "metric-gate: --since needs a ref"+usage)
}

func TestFilesWithNoPathIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs("--files")

	assertUsageError(t, result, "metric-gate: --files needs at least one path"+usage)
}

func TestFilesFollowedByAnotherScopeFlagIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs("--files", "--staged")

	assertUsageError(t, result, "metric-gate: --files needs at least one path"+usage)
}

func TestRepeatedFilesFlagSaysWhereThePathsGo(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(otherService, csharpFile(30))
	f.commitAll("initial")

	result := f.runArgs("--files", orderService, "--files", otherService)

	assertUsageError(t, result,
		"metric-gate: --files takes every path in one list, as in 'metric-gate --files a.cs b.cs'"+usage)
}

func TestStagedBeforeTheFirstCommitFailsWithNoBase(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.git("add", orderService)

	f.runArgs("--staged").assertMatches(t, "staged_no_commits", 1, "",
		"no diff base: this repo has no commits\n")
}

func TestFilesNamingTheSameFileTwiceHandsItToTheGateOnce(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	// Two spellings of one path. The skipped file is what makes the case
	// discriminating: a list handed on twice lists it twice, and the same
	// repetition reaches the extractor, which then reports every span in the
	// file twice and fails the run over a contract it did not break.
	f.runArgs("--files", "docs/notes.md", "./docs/notes.md", orderService).
		assertMatches(t, "files_with_skipped_path", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesNamingAnAbsolutePathMeasuresTheFileItNames(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	f.runArgs("--files", filepath.Join(f.root, filepath.FromSlash(orderService))).
		assertMatches(t, "files_single_file", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesNamingADirectoryFailsNamingThatPath(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--files", "src/Ordering").assertMatches(t, "files_directory", 1, "",
		"src/Ordering is a directory, not a file\n")
}

func TestStagedRefusesAFileACleanFilterMakesLookUnmodified(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.hideMarkedEdit()
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	// The clean filter strips this marker, so git's own answer to whether the
	// working tree differs from the index is no, and the gate would score
	// index line numbers against text on disk that does not match them.
	f.write(orderService, replaceLine(f.read(orderService), 45, "// line 45, hidden"))

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_file_dirty", 1, f.headLabel(),
		"refusing to score src/Ordering/OrderService.cs: staged in one state and on disk in another\n")
}

// hideMarkedEdit installs a repo-local clean filter over every .cs file that
// strips the ", hidden" marker, which is the shape git-lfs and git-crypt take:
// a driver named by the repository's own config and selected by a
// .gitattributes line, reached by neither the environment scrub nor the pinned
// config files.
func (f *fixture) hideMarkedEdit() {
	f.t.Helper()
	clean := filepath.Join(f.t.TempDir(), "hide-marked")
	if err := os.WriteFile(clean, []byte("#!/bin/sh\nexec sed 's/, hidden//'\n"), 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.write(".gitattributes", "*.cs filter=hide\n")
	f.git("config", "filter.hide.clean", clean)
}

// singleFileCoverageAndStub is the coverage report and stub extractor the
// --files cases that name one file share, both of which expect the two spans
// of orderService and nothing else.
func singleFileCoverageAndStub(t *testing.T, f *fixture) {
	t.Helper()
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}
}
