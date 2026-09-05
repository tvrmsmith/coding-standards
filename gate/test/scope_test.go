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
	// The stub parses only orderService and reports both its spans, of which
	// only cancel holds a staged line. Under the default merge-base scope the
	// gate would hand the extractor Other.cs as well, and the stub's silence
	// about it would fail the run with extractor_no_parse_status instead of
	// producing this document, which is what makes --staged excluding the
	// unstaged edit discriminating.
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

func TestStagedRefusesADirtyFileWhoseNameReadsAsAGlob(t *testing.T) {
	// The Next.js route filename, and wildmatch magic to git. Handed to the
	// divergence check as a bare pathspec, `[1]` is a character class that
	// matches no file on disk, so git reports no difference and the gate
	// scores index line numbers against the working-tree text.
	const route = "src/Ordering/Order[1].cs"
	routeCancel := span{File: route, Name: "Order1.Cancel", StartLine: 60, EndLine: 64, Complexity: 3}

	f := newFixture(t, "main")
	f.write(route, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(route, 62)
	f.git("add", route)
	f.touchLine(route, 45)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: route, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(route), []span{routeCancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_glob_name_dirty", 1, f.headLabel(),
		"refusing to score src/Ordering/Order[1].cs: staged in one state and on disk in another\n")
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
		Stdout:     extractorOutput(t, parsed(orderService, otherService), []span{placeAsync, cancel, otherRun}),
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
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub.StdinLog = handed

	// Two spellings of each path. The skipped one is listed once in the
	// document, and the claimed one reaches the extractor once: handed it
	// twice the extractor reports every span in it twice and the run exits 1
	// over a contract it did not break.
	f.runArgs("--files", "docs/notes.md", "./docs/notes.md", orderService, "./"+orderService).
		assertMatches(t, "files_with_skipped_path", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")

	if got := readFile(t, handed); got != orderService+"\n" {
		t.Errorf("the extractor was handed %q, want the one line %q", got, orderService+"\n")
	}
}

func TestFilesNamingAFileInTheWrongCaseRefusesRatherThanMeasuresIt(t *testing.T) {
	f := newFixture(t, "main")
	if !caseInsensitiveFilesystem(t, f.root) {
		t.Skip("the filesystem is case sensitive, so the mis-cased spelling names no file and the case is a duplicate of the unresolvable one")
	}
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	// The spelling resolves here, and resolving symlinks does not settle its
	// case, so the gate would hand the extractor src/ordering/... while
	// coverage carries the tracked src/Ordering/... . The two would match
	// nothing against each other and every method in a covered file would come
	// back unknown, so the run refuses the spelling instead.
	f.runArgs("--files", "src/ordering/OrderService.cs").
		assertMatches(t, "files_wrong_case", 1, "",
			"src/ordering/OrderService.cs is not spelled as the file on disk is\n")
}

func TestFilesNamingAPathThroughAFileFailsInTheDocument(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	// A typo that puts a filename where a directory belongs. The operating
	// system answers ENOTDIR rather than saying the path is absent, and every
	// other --files refusal reaches the document, so this one does too rather
	// than exiting 1 with an empty stdout an agent cannot tell from a crash.
	f.runArgs("--files", orderService+"/x.cs").
		assertMatches(t, "files_not_a_directory", 1, "",
			"src/Ordering/OrderService.cs/x.cs could not be read, not a directory\n")
}

func TestFilesResolvesARelativePathAgainstTheWorkingDirectory(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	// The shell a pre-commit hook runs in sits wherever the developer is, not
	// at the repo root. OrderService.cs names nothing from the root, so a
	// resolution joined onto the root rather than the working directory
	// refuses a file sitting right there.
	f.runArgsFrom("src/Ordering", "--files", "OrderService.cs").
		assertMatches(t, "files_single_file", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesWithAnEmptyPathIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	// An empty argument does not start with '-', so the slurp takes it, and
	// joined onto the working directory it names the working directory.
	result := f.runArgs("--files", "")

	assertUsageError(t, result, "metric-gate: --files was handed an empty path"+usage)
}

func TestStagedMeasuresNothingForAPureMoveStagedOnItsOwn(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	f.git("mv", origin, moved)
	// ADR 0003's rule that a rename with no content change touches nothing
	// holds over the index too, so the pre-commit hook this scope exists for
	// does not demand coverage for every method in a file the developer only
	// moved. This does not pin cachedFlag inside pureMoves: that comparison
	// reads the added side off the working tree either way, so under --staged
	// on a clean tree the cached and uncached listings agree on every path.
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_pure_move", 0, f.headLabel(),
		"no changed methods, nothing to measure\n")
}

func TestARunWithScopeFlagsDoesNotChangeTheScopeOfTheNextRun(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	singleFileCoverageAndStub(t, f)

	f.runArgs("--files", orderService)

	// Bare argv after a flagged run, so the document names the merge-base
	// scope and only the touched method. A flag left behind on the fixture
	// would put --files here instead, and both the scope line and the row
	// count would go red.
	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestFilesReturnsEveryMethodInTheFileIncludingNestedOnes(t *testing.T) {
	// validate nests inside placeAsync. Under a diff the smallest containing
	// span takes the touched line and the container is not changed, but
	// --files carries no line to narrow against, so both are measured.
	validate := span{File: orderService, Name: "OrderService.PlaceAsync.Validate", StartLine: 45, EndLine: 50, Complexity: 2}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	// Lines 42-44 and 51 belong to placeAsync alone, 45-50 to validate, and
	// only 51 is uncovered, so the two carry different coverage and neither
	// row can stand in for the other.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, validate, cancel}),
	}

	f.runArgs("--files", orderService).assertMatches(t, "files_nested_span", 0, "",
		"0 of 3 changed methods over CRAP threshold 30, worst score 10.27\n")
}

func TestStagedDoesNotRefuseADirtyFileNoExtractorClaims(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	// Staged in one state and on disk in another, the shape --staged refuses,
	// on a file no extractor claims. The gate never scores it, so it has no
	// line numbers to misattribute and the run proceeds.
	f.write("docs/notes.md", "second\n")
	f.git("add", "docs/notes.md")
	f.write("docs/notes.md", "third\n")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_single_method", 0, f.headLabel(),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestSinceARefSharingNoHistoryWithHeadSaysSo(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// An orphan branch is a second root commit, so the two histories share no
	// commit and git answers the merge base with no rather than with a
	// failure. The ref itself resolves, which is what tells this apart from a
	// name that does not exist.
	f.git("checkout", "--quiet", "--orphan", "unrelated")
	f.write("docs/notes.md", "first\n")
	f.commitAll("unrelated root")
	f.git("checkout", "--quiet", "main")

	f.runArgs("--since", "unrelated").assertMatches(t, "since_unrelated_histories", 1, "",
		"no diff base: HEAD and unrelated share no common ancestor\n")
}

func TestSinceWithAnEmptyRefIsAUsageError(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	result := f.runArgs("--since", "")

	assertUsageError(t, result, "metric-gate: --since needs a ref"+usage)
}

func TestAScopeFlagAfterADifferentOneNamesBothScopes(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	// --files arriving second on a command line that already named a scope. The
	// message that says where the paths go belongs to a repeated --files alone,
	// and this line really does name two scopes.
	result := f.runArgs("--staged", "--files", orderService)

	assertUsageError(t, result, "metric-gate: --files and --staged name two scopes; pass one"+usage)
}

func TestSinceOnABranchWithNoCommitSaysTheRepoHasNone(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// An orphan branch that was never committed on. main resolves, so the ref
	// check passes and HEAD is what does not, which git reports from merge-base
	// as exit 128 rather than the exit 1 that means no. Read as a failure to
	// answer, the run blames a diff it never asked for.
	f.git("checkout", "--quiet", "--orphan", "work")

	f.runArgs("--since", "main").assertMatches(t, "since_no_commits", 1, "",
		"no diff base: this repo has no commits\n")
}

func TestStagedNamesTheDirtyFileWhenTheExtractorFailsOnIt(t *testing.T) {
	const added = "src/Ordering/New.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	// Staged and then deleted from disk, which is the widest divergence
	// --staged refuses. The extractor is handed a path with nothing to read and
	// fails on it, so no file is claimed and the ordinary divergence check sees
	// nothing. Reported as the extractor's own failure the caller is told its
	// source does not parse, when the cause is the refusal this scope exists for.
	f.write(added, csharpFile(20))
	f.git("add", added)
	if err := os.Remove(filepath.Join(f.root, filepath.FromSlash(added))); err != nil {
		t.Fatal(err)
	}
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}

	f.runArgs("--staged").assertMatches(t, "staged_extractor_dirty", 1, f.headLabel(),
		"refusing to score src/Ordering/New.cs: staged in one state and on disk in another\n")
}

func TestFilesNamingTwoHardLinksToOneInodeMeasuresBoth(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	// Two tracked paths, one inode. They are two files to the extractor and two
	// files to a coverage report, so dropping either as a duplicate spelling
	// leaves a file the developer named unmeasured under a pass.
	if err := os.Link(filepath.Join(f.root, filepath.FromSlash(orderService)),
		filepath.Join(f.root, filepath.FromSlash(otherService))); err != nil {
		t.Fatal(err)
	}
	f.commitAll("initial")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)},
		coverageClass{filename: otherService, lines: spanCoverage(11, 4, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService, otherService), []span{placeAsync, cancel, otherRun}),
	}

	f.runArgs("--files", orderService, otherService).assertMatches(t, "files_named_directly", 0, "",
		"0 of 3 changed methods over CRAP threshold 30, worst score 9.08\n")
}

// readFile is the content of a file a case asked the stub to write, which is
// how it asserts what the gate handed the extractor rather than only what came
// back out.
func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
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
