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

// TestScopeUsageErrors covers every command line the scope flags refuse.
//
// A usage error is decided before the gate opens the repo, so the fixture
// carries no commit, no source, no report and no extractor config: nothing
// about the tree can change any of these answers, and the argv is the whole of
// what each case is about.
func TestScopeUsageErrors(t *testing.T) {
	cases := map[string]struct {
		argv    []string
		problem string
	}{
		"an unknown flag": {
			argv:    []string{"--bogus"},
			problem: "unknown argument '--bogus'",
		},
		"a positional argument": {
			argv:    []string{orderService},
			problem: "unexpected argument '" + orderService + "'; there are no positional arguments",
		},
		"two scope flags": {
			argv:    []string{"--since", "main", "--staged"},
			problem: "--staged and --since name two scopes; pass one",
		},
		// --files arriving second on a command line that already named a
		// scope. The message that says where the paths go belongs to a
		// repeated --files alone, and this line really does name two scopes.
		"a scope flag after a different one": {
			argv:    []string{"--staged", "--files", orderService},
			problem: "--files and --staged name two scopes; pass one",
		},
		// A second --staged names no second scope, but the message treats the
		// command line as the developer wrote it rather than deciding they
		// meant one flag, which is the same call the two-flag cases make.
		"the same scope flag twice": {
			argv:    []string{"--staged", "--staged"},
			problem: "--staged and --staged name two scopes; pass one",
		},
		// --files is variadic rather than repeatable, so a second one names
		// one scope and gets the message saying where the paths go.
		"a repeated --files": {
			argv:    []string{"--files", orderService, "--files", otherService},
			problem: "--files takes every path in one list, as in 'metric-gate --files a.cs b.cs'",
		},
		"--since with nothing after it": {
			argv:    []string{"--since"},
			problem: "--since needs a ref",
		},
		"--since followed by another flag": {
			argv:    []string{"--since", "--staged"},
			problem: "--since needs a ref",
		},
		// What an unset shell variable expands to. Taken as a ref it would
		// reach the developer as the default-candidates message telling them
		// to pass the flag they just passed.
		"--since with an empty ref": {
			argv:    []string{"--since", ""},
			problem: "--since needs a ref",
		},
		"--files with nothing after it": {
			argv:    []string{"--files"},
			problem: "--files needs at least one path",
		},
		"--files followed by another scope flag": {
			argv:    []string{"--files", "--staged"},
			problem: "--files needs at least one path",
		},
		// An empty argument does not start with '-', so the slurp takes it,
		// and joined onto the working directory it names the working directory.
		"--files with an empty path": {
			argv:    []string{"--files", ""},
			problem: "--files was handed an empty path",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, "main")

			assertUsageError(t, f.runArgs(tt.argv...), "metric-gate: "+tt.problem+usage)
		})
	}
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

func TestStagedBeforeTheFirstCommitFailsWithNoBase(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.git("add", orderService)

	f.runArgs("--staged").assertMatches(t, "staged_no_commits", 1, "",
		"no diff base: this branch has no commit\n")
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
	// macOS only by construction. Every Linux runner is case sensitive, where
	// the mis-cased spelling names no file at all and the refusal is the
	// absent-path one instead. srcpath's spelling_test.go carries the
	// Linux-reachable coverage of the decision branch, so what skips here is
	// the document and the message rather than the logic.
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
	// moved. The tree is clean, so the cached and uncached listings agree here
	// and this case says nothing about cachedFlag inside pureMoves.
	// TestStagedLooksForPureMovesInTheIndexRatherThanTheWorkingTree is the one
	// that pins it.
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_pure_move", 0, f.headLabel(),
		"no changed methods, nothing to measure\n")
}

// TestAFilesRunLeavesNothingBehindThatChangesTheNextRun runs --files and then
// a bare command line over the same repo. --files writes no state, so the
// second run has to resolve the merge base and measure only the touched
// method; a scope that wrote anything to the tree or the git config would show
// up here as the wrong scope line and the wrong row count.
func TestAFilesRunLeavesNothingBehindThatChangesTheNextRun(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	singleFileCoverageAndStub(t, f)

	// The flagged run is asserted too, so a sequence that degenerates into two
	// failing runs cannot pass as the one this case is named for.
	f.runArgs("--files", orderService).assertMatches(t, "files_single_file", 0, "",
		"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")

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

func TestSinceOnABranchWithNoCommitSaysTheBranchHasNone(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// An orphan branch that was never committed on. main resolves, so the ref
	// check passes and HEAD is what does not, which git reports from merge-base
	// as exit 128 rather than the exit 1 that means no. Read as a failure to
	// answer, the run blames a diff it never asked for. The repo here holds a
	// full history, so a message about the repo having no commits would be
	// contradicted by the log the developer can print.
	f.git("checkout", "--quiet", "--orphan", "work")

	f.runArgs("--since", "main").assertMatches(t, "since_no_commits", 1, "",
		"no diff base: this branch has no commit\n")
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

func TestSinceReportsADiffGitRefusesToPrintInTheDocument(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "release")
	// The ref and the merge base both resolve, so the document exists and
	// ADR 0005 requires it on stdout under this scope too. The diff then reads
	// the old side's blob, which is gone, so git exits 128 before printing a
	// patch. Returned untyped instead, the run would exit 1 with an empty
	// stdout an agent cannot tell from a crash.
	blob := f.git("rev-parse", "HEAD:"+orderService)
	f.touchLine(orderService, 62)
	f.removeLooseObject(blob)

	f.runArgs("--since", "release").assertMatchesWith(t, "since_diff_unreadable", 1, f.baseLabel("release"),
		"could not read the diff: fatal: unable to read "+blob+"\n",
		map[string]string{"BLOB": blob})
}

func TestStagedReportsADiffGitRefusesToPrintInTheDocument(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	blob := f.git("rev-parse", "HEAD:"+orderService)
	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.removeLooseObject(blob)

	f.runArgs("--staged").assertMatchesWith(t, "staged_diff_unreadable", 1, f.headLabel(),
		"could not read the diff: fatal: unable to read "+blob+"\n",
		map[string]string{"BLOB": blob})
}

func TestStagedKeepsTheExtractorsCauseWhenTheDirtyFileIsOneNoExtractorReads(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")

	// The extractor fails on a source file that is not dirty, and a Markdown
	// file is separately staged and then edited. Asked about every path in the
	// diff, the fallback would report staged_file_dirty naming docs/notes.md
	// and replace the extractor's own cause with one about a file nothing was
	// ever going to read.
	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.write("docs/notes.md", "second\n")
	f.git("add", "docs/notes.md")
	f.write("docs/notes.md", "third\n")
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}

	f.runArgs("--staged").assertMatches(t, "staged_extractor_failed", 1, f.headLabel(),
		"csharp extractor exited 3\n")
}

func TestStagedReportsADivergenceCheckGitCannotRunInTheDocument(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}
	// The staged diff compares the index against a commit, so it reads objects
	// alone and succeeds, and the extractor answers from its canned config
	// without opening anything. The divergence check is the one command that
	// has to read the working-tree copy, and with the file unreadable it exits
	// 128 instead of naming the file. Returned untyped, the run would exit 1
	// with an empty stdout an agent cannot tell from a crash.
	f.denyReadFile(orderService)
	cause := f.divergenceStderr(orderService)

	f.runArgs("--staged").assertMatchesWith(t, "staged_divergence_unreadable", 1, f.headLabel(),
		"could not read the diff: "+cause+"\n",
		map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestStagedKeepsTheExtractorsCauseWhenTheDivergenceCheckCannotRun(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.git("add", orderService)
	// The extractor dies and the divergence check the fallback would ask
	// instead cannot run either, because the same file is unreadable. With no
	// answer of its own to substitute, the fallback leaves standing the one
	// cause the gate did establish rather than replacing it with a refusal it
	// never proved.
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}
	f.denyReadFile(orderService)

	f.runArgs("--staged").assertMatches(t, "staged_extractor_failed", 1, f.headLabel(),
		"csharp extractor exited 3\n")
}

func TestStagedRefusesAMoveWhoseIndexContentTheWorkingTreeNoLongerHolds(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	edited := span{File: moved, Name: "Moved.Edited", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	before := f.read(origin)
	f.git("mv", origin, moved)
	f.touchLine(moved, 7)
	f.git("add", moved)
	// The index now holds the edit and the working tree holds the pre-move
	// text again, which is the divergence --staged exists to refuse. Compared
	// against the working-tree copy the added side digests equal to the deleted
	// blob, so the gate calls the pair a pure move, drops Moved.cs from the
	// diff, and never hands it to an extractor, and the run passes on the file
	// the guard was written for.
	f.write(moved, before)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{edited}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_moved_file_dirty", 1, f.headLabel(),
		"refusing to score "+moved+": staged in one state and on disk in another\n")
}

func TestStagedNamesEveryDirtyFileInOneMessage(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(otherService, csharpFile(30))
	f.commitAll("initial")

	// Two files staged in one state and on disk in another. Both are the
	// gate's to score, so a message naming one of them sends the developer
	// back for a second run to find the other. What this pins is the ", " join.
	// The order the two names come out in is dirtyMessage's own sort, which
	// this case cannot tell from the sort claimedFiles already did upstream, so
	// the unit case beside dirtyMessage is what holds that rule.
	f.touchLine(orderService, 62)
	f.touchLine(otherService, 12)
	f.git("add", orderService, otherService)
	f.touchLine(orderService, 45)
	f.touchLine(otherService, 20)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService, otherService), []span{placeAsync, cancel, otherRun}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_two_files_dirty", 1, f.headLabel(),
		"refusing to score src/Ordering/OrderService.cs, src/Ordering/Other.cs: staged in one state and on disk in another\n")
}

func TestFilesListsEveryUnhandledPathSortedRatherThanAsTyped(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.write("docs/architecture.md", "first\n")
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	// The two unhandled paths are named in reverse sorted order, so a document
	// that echoed the command line back would list notes.md first.
	f.runArgs("--files", "docs/notes.md", "docs/architecture.md", orderService).
		assertMatches(t, "files_two_skipped_paths", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesNamingAFileInADirectoryItCannotReadFailsInTheDocument(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.denyReadKeepingEntry("src/Ordering")

	// The directory can be entered, so the path resolves and names a regular
	// file, and only the spelling check has to read the directory itself. A
	// filesystem that will not answer is not the tree disagreeing with the
	// spelling, so the refusal carries what the operating system said instead
	// of accusing the developer of a typo.
	f.runArgs("--files", orderService).
		assertMatches(t, "files_unreadable_directory", 1, "",
			"src/Ordering/OrderService.cs could not be read, permission denied\n")
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
	if f.git("diff", "--name-only", "--", orderService) != "" {
		t.Skip("the clean filter did not hide the edit here, so git reports the ordinary divergence and the case cannot be about the blanked driver")
	}

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

func TestStagedScoresAFileWhoseOnlyDifferenceOnDiskIsTheExecutableBit(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	// The bit is not content. The text on disk is byte for byte what the index
	// holds, so the extractor reads the lines the staged numbers describe and
	// there is nothing to misattribute, but a plain diff still names the path
	// and the run would refuse a tree no edit can clean.
	f.setExecutable(orderService)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_single_method", 0, f.headLabel(),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestFilesNamingASymlinkMeasuresTheFileItPointsAt(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.symlinkTo(filepath.Join(f.root, filepath.FromSlash(orderService)), "src/Ordering/Link.cs")
	singleFileCoverageAndStub(t, f)

	// The extractor and the coverage report are both keyed by the target's
	// path. Measured under the link's own path instead, the file would match
	// no report path and every method in it would come back unknown.
	f.runArgs("--files", "src/Ordering/Link.cs").assertMatches(t, "files_single_file", 0, "",
		"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesNamingASymlinkOutOfTheRepoFailsNamingThePathAsTyped(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	outside := filepath.Join(t.TempDir(), "outside.cs")
	writeAbsolute(t, outside, csharpFile(10))
	f.symlinkTo(outside, "src/Ordering/Outside.cs")

	// The link is inside the root and the file it names is not. Stopping at
	// the link, the gate would measure a file no commit of this repo holds and
	// report it under a path inside the repo.
	f.runArgs("--files", "src/Ordering/Outside.cs").
		assertMatches(t, "files_symlink_outside_repo", 1, "",
			"src/Ordering/Outside.cs is outside the repo root\n")
}

func TestFilesListsANamedSkipAheadOfOneCoverageDiscoveryFound(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)
	f.denyRead("TestResults/locked")

	// Both producers of skipped_paths at once, which is the order ADR 0005
	// fixes: the paths the developer named, then whatever coverage discovery
	// could not read. "TestResults/locked" sorts ahead of "docs/notes.md", so
	// a merge that sorted the two or appended them the other way round goes
	// red here.
	f.runArgs("--files", "docs/notes.md", orderService).
		assertMatches(t, "files_skip_before_discovery_skip", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestSinceNamingARevisionGitWillNotEvaluateReportsAnUnreadableDiff(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	// A reflog entry the log does not have. git refuses the revision rather
	// than answering that it names no commit, and reported as a ref that does
	// not exist it would send the developer looking for a branch they never
	// typed.
	f.runArgs("--since", "HEAD@{99}").assertMatches(t, "since_ref_unreadable", 1, "",
		"could not read the diff: git rev-parse --verify --quiet HEAD@{99}^{commit}: exit status 128\n")
}

func TestStagedLooksForPureMovesInTheIndexRatherThanTheWorkingTree(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	// The index holds one added file and no deletion. The working tree holds
	// that add beside a deletion nobody staged, which is the pure move the
	// index does not have. Read off the working tree the two would pair up,
	// Moved.cs would be dropped as a file the developer only renamed, and a
	// staged addition would go unscored under exit 0 pass.
	f.copyFile(origin, moved)
	f.git("add", moved)
	f.removeFile(origin)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: moved, lines: spanCoverage(5, 5, 4)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_add_over_unstaged_deletion", 0, f.headLabel(),
		"0 of 1 changed methods over CRAP threshold 30, worst score 4.13\n")
}

func TestFilesOrdersTheSpansItFoundRatherThanKeepingTheExtractorsOrder(t *testing.T) {
	const calc = "src/Ordering/Calc.cs"
	// Two methods declared on one line, so the document's own sort by
	// descending score ties on every key it compares and the rows come out in
	// the order the join handed them over. That order is the join's to fix,
	// since a real extractor emits spans in whatever order it walked the file.
	f1 := span{File: calc, Name: "Calc.F", Signature: "(int)", StartLine: 14, EndLine: 14, Complexity: 1}
	g := span{File: calc, Name: "Calc.G", Signature: "(string)", StartLine: 14, EndLine: 14, Complexity: 1}

	f := newFixture(t, "main")
	f.write(calc, csharpFile(20))
	f.commitAll("initial")
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: calc, lines: spanCoverage(14, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(calc), []span{g, f1}),
	}

	f.runArgs("--files", calc).assertMatches(t, "files_two_overloads_sorted", 0, "",
		"0 of 2 changed methods over CRAP threshold 30, worst score 1.00\n")
}

func TestStagedScoresAFileTheWorkingCopyOnlyReindented(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	// Format on save after staging. Every line keeps its number and its
	// content, which is exactly what `git diff -w` is defined to ignore and
	// what the gate's own diff already ignores, so there is nothing to
	// misattribute. Compared without -w the guard names the path and the run
	// refuses a tree only re-staging can clean.
	f.write(orderService, indented(f.read(orderService), "    "))
	if f.git("diff", "--name-only", "--", orderService) == "" {
		t.Fatal("the reindent left the working copy matching the index, so nothing here is for -w to hide")
	}

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_single_method", 0, f.headLabel(),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestStagedRefusesAFileTheWorkingCopyAddedABlankLineTo(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	// The other side of the reindent case. A blank line added after staging
	// moves every line below it, so the spans the extractor reports off disk sit
	// one line away from the staged content the coverage numbers describe, and
	// the run would score the wrong lines and report a number rather than an
	// error. -w forgives whitespace inside a line and not a line the index does
	// not have, which is what keeps this refusal standing.
	f.insertBlankLine(orderService, 10)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_file_dirty", 1, f.headLabel(),
		"refusing to score src/Ordering/OrderService.cs: staged in one state and on disk in another\n")
}

func TestFilesNamingOnlyUnhandledPathsPassesWithoutReachingCoverage(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")
	f.denyRead("TestResults/locked")
	f.stub = stubConfig{Extensions: []string{".cs"}}

	// No extractor claims the one named path, so the changed set is empty and
	// ADR 0003 exits 0 pass before any input is resolved. The locked directory
	// is what makes that visible: coverage discovery would list it in
	// skipped_paths, so a document carrying only the named path proves the
	// early exit ran ahead of discovery.
	f.runArgs("--files", "docs/notes.md").
		assertMatches(t, "files_only_unhandled_path", 0, "",
			"no changed methods, nothing to measure\n")
}

func TestSinceReportsAMergeBaseGitCannotWalkAsAnUnreadableDiff(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "other")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.removeLooseObject(f.git("rev-parse", "HEAD"))

	// Both `rev-parse --verify` checks pass, since neither reads the commit
	// they name, and the walk merge-base has to make is the first thing that
	// touches the missing object. Left to fall through to the exit-1 arm this
	// would come back as two histories sharing no commit, sending the
	// developer after a branch relationship that is not the problem.
	cause := f.gitStderr("merge-base", "HEAD", "other")

	f.runArgs("--since", "other").assertMatchesWith(t, "since_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": cause})
}

func TestStagedReportsAHeadGitCannotReadAsAnUnreadableDiff(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.corruptPackedRefs()

	// git answers the HEAD check with exit 128 rather than the exit 1 that
	// means no commit. Read as no commit, the run would tell a developer whose
	// repo holds a full history to make their first one, and re-staging would
	// never clear it because staging is not what is broken.
	cause := f.gitStderr("rev-parse", "--verify", "--quiet", "HEAD")

	f.runArgs("--staged").assertMatchesWith(t, "staged_head_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": cause})
}

func TestFilesKeepsTheExtractorsCauseWhenExtractionDies(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}

	// --files has no divergence check to substitute a cause of its own, so what
	// the extractor said is what the document carries. The base stays null
	// because this scope resolves none, and skipped_paths stays empty because
	// the list of what no extractor claimed is built after extraction returns.
	f.runArgs("--files", orderService).assertMatches(t, "files_extractor_failed", 1, "",
		"csharp extractor exited 3\n")
}

func TestStagedTakesANamedCoverageReport(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)

	// Discoverable, and it leaves Cancel uncovered. Reading it instead of the
	// named report scores Cancel 12.00, which is what makes the case tell one
	// parser owning argv apart from --coverage being dropped on a scope flag.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 0)}))
	// Named, outside any TestResults directory so discovery cannot reach it.
	f.write("artifacts/coverage.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged", "--coverage", "artifacts/coverage.xml").
		assertMatches(t, "staged_single_method", 0, f.headLabel(),
			"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestFilesStopsCollectingPathsAtTheCoverageFlag(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	// A --files list is variadic, so the risk this case pins is the flag after
	// it being swallowed as a path. That would fail the run naming
	// artifacts/coverage.xml as an unresolvable source file rather than
	// reading it as the report.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 0)}))
	f.write("artifacts/coverage.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--files", orderService, "--coverage", "artifacts/coverage.xml").
		assertMatches(t, "files_single_file", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}
