package gate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// usage is the block every scope.UsageError renders beneath its specific
// problem, pinned here once so a case's stderr assertion states only the
// problem line it is actually testing.
const usage = "\n\nusage: metric-gate [--staged | --since <ref> | --files <path>...] [--coverage <path>]... [--metric <name>]... [--threshold <name>=<n>]...\n" +
	"  (no flag)               the merge base of HEAD and the default branch\n" +
	"  --staged                what a commit would contain\n" +
	"  --since <ref>           the merge base of HEAD and <ref>\n" +
	"  --files <path>...       every method in each named file\n" +
	"  --coverage <path>       read this report instead of discovering one; repeatable\n" +
	"  --metric <name>         measure this metric; repeatable, defaults to every one this binary hosts\n" +
	"  --threshold <name>=<n>  the bar for one metric, as in crap=30; repeatable, never global\n"

// TestScopeUsageErrors covers every command line metric-gate refuses before it
// runs, the scope flags and --coverage alike. One parser owns argv, so one
// table owns the message format rather than two files asserting the same shape.
//
// A usage error is decided before the gate opens the repo, so the fixture
// carries no commit, no source, no report and no extractor config: nothing
// about the tree can change any of these answers, and the argv is the whole of
// what each case is about.
func TestScopeUsageErrors(t *testing.T) {
	t.Parallel()
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
		"a scope flag after a different one": {
			argv:    []string{"--staged", "--files", orderService},
			problem: "--files and --staged name two scopes; pass one",
		},
		"the same scope flag twice": {
			argv:    []string{"--staged", "--staged"},
			problem: "--staged was passed twice; pass it once",
		},
		"the same ref flag twice": {
			argv:    []string{"--since", "release", "--since", "main"},
			problem: "--since was passed twice; pass it once",
		},
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
		"a scope flag after a non-empty --files": {
			argv:    []string{"--files", orderService, "--staged"},
			problem: "--staged and --files name two scopes; pass one",
		},
		"--since followed by a single-dash argument": {
			argv:    []string{"--since", "-5"},
			problem: "--since needs a ref",
		},
		"--files followed by a single-dash argument": {
			argv:    []string{"--files", "-report.xml"},
			problem: "--files needs at least one path",
		},
		"--files with an empty path": {
			argv:    []string{"--files", ""},
			problem: "--files was handed an empty path",
		},
		"--coverage with nothing after it": {
			argv:    []string{"--coverage"},
			problem: "--coverage needs a path",
		},
		"--coverage with an empty joined value": {
			argv:    []string{"--coverage="},
			problem: "--coverage needs a path",
		},
		"--coverage with an empty separate value": {
			argv:    []string{"--coverage", ""},
			problem: "--coverage needs a path",
		},
		"--coverage given a flag as a separate value": {
			argv:    []string{"--coverage", "--nope"},
			problem: "--coverage needs a path, not the flag '--nope'",
		},
		"--coverage given a flag as a joined value": {
			argv:    []string{"--coverage=--nope"},
			problem: "--coverage needs a path, not the flag '--nope'",
		},
		"--metric with nothing after it": {
			argv:    []string{"--metric"},
			problem: "--metric needs a metric name",
		},
		"--metric with an empty joined value": {
			argv:    []string{"--metric="},
			problem: "--metric needs a metric name",
		},
		"--metric with an empty separate value": {
			argv:    []string{"--metric", ""},
			problem: "--metric needs a metric name",
		},
		"--metric given a flag as a separate value": {
			argv:    []string{"--metric", "--staged"},
			problem: "--metric needs a metric name, not the flag '--staged'",
		},
		"--metric naming an unknown metric": {
			argv:    []string{"--metric", "cyclo"},
			problem: "unknown metric 'cyclo'; this binary hosts: crap",
		},
		"--metric is case sensitive": {
			argv:    []string{"--metric", "CRAP"},
			problem: "unknown metric 'CRAP'; this binary hosts: crap",
		},
		"the same metric named on --metric twice": {
			argv:    []string{"--metric", "crap", "--metric", "crap"},
			problem: "--metric was given 'crap' twice; pass it once",
		},
		"--metric given a single-dash value": {
			argv:    []string{"--metric", "-x"},
			problem: "unknown metric '-x'; this binary hosts: crap",
		},
		"--threshold with nothing after it": {
			argv:    []string{"--threshold"},
			problem: "--threshold needs a <metric>=<value> assignment, as in 'crap=30'",
		},
		"--threshold with an empty joined value": {
			argv:    []string{"--threshold="},
			problem: "--threshold needs a <metric>=<value> assignment, as in 'crap=30'",
		},
		"--threshold with an empty separate value": {
			argv:    []string{"--threshold", ""},
			problem: "--threshold needs a <metric>=<value> assignment, as in 'crap=30'",
		},
		"--threshold given a flag as a separate value": {
			argv:    []string{"--threshold", "--staged"},
			problem: "--threshold needs a <metric>=<value> assignment, not the flag '--staged'",
		},
		"--threshold missing its '='": {
			argv:    []string{"--threshold", "crap"},
			problem: "--threshold 'crap' is missing '=', as in 'crap=30'",
		},
		"--threshold naming no metric before '='": {
			argv:    []string{"--threshold", "=30"},
			problem: "--threshold '=30' names no metric, as in 'crap=30'",
		},
		"--threshold naming an unknown metric": {
			argv:    []string{"--threshold", "cyclo=30"},
			problem: "--threshold names unknown metric 'cyclo'; this binary hosts: crap",
		},
		"--threshold given a non-numeric value": {
			argv:    []string{"--threshold", "crap=abc"},
			problem: "--threshold 'crap=abc' is not a whole number, as in 'crap=30'",
		},
		"--threshold given no value after '='": {
			argv:    []string{"--threshold", "crap="},
			problem: "--threshold 'crap=' is not a whole number, as in 'crap=30'",
		},
		"--threshold given a fractional value": {
			argv:    []string{"--threshold", "crap=12.5"},
			problem: "--threshold 'crap=12.5' is not a whole number, as in 'crap=30'",
		},
		"--threshold at zero": {
			argv:    []string{"--threshold", "crap=0"},
			problem: "--threshold crap=0 is below 1, so no method could pass",
		},
		"--threshold negative": {
			argv:    []string{"--threshold", "crap=-5"},
			problem: "--threshold crap=-5 is below 1, so no method could pass",
		},
		"the same metric named on --threshold twice": {
			argv:    []string{"--threshold", "crap=12", "--threshold", "crap=20"},
			problem: "--threshold was given crap twice; pass it once",
		},
		"--threshold given a single-dash value": {
			argv:    []string{"--threshold", "-5"},
			problem: "--threshold '-5' is missing '=', as in 'crap=30'",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, "main")

			assertUsageError(t, f.runArgs(tt.argv...), "metric-gate: "+tt.problem+usage)
		})
	}
}

func TestSinceMeasuresEveryMethodChangedAcrossTheWholeBranch(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.git("checkout", "--quiet", "-b", "feature")
	f.touchLine(orderService, 62)
	f.commitAll("edit Cancel on the branch")

	f.git("checkout", "--quiet", "main")
	f.touchLine(orderService, 45)
	f.commitAll("edit PlaceAsync on main")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--since", "nope").assertMatches(t, "since_ref_unresolvable", 1, "",
		"no diff base: --since nope does not name a commit\n")
}

func TestSinceNamingATagThatDoesNotPointAtACommitFailsNamingThatRef(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("tag", "treetag", f.git("rev-parse", "HEAD^{tree}"))

	f.runArgs("--since", "treetag").assertMatches(t, "since_tag_not_a_commit", 1, "",
		"no diff base: --since treetag does not name a commit\n")
}

// An annotated tag over a tree, the one shape where the id verify hands
// over is itself a tag object and what it wraps is still nothing the run
// can diff. It shares the lightweight case's golden because the document
// is the same; the input shape is what differs. What this case pins is
// the answer the developer gets for it, a ref they can fix by naming
// another rather than an unreadable diff. It says nothing about the `^{}`:
// `cat-file -t` on the tag object bare answers `tag`, which is not
// `commit` either, so this stays green with the peel dropped. The peel is
// what TestSinceNamingAnAnnotatedTagResolvesTheBaseThroughIt and
// TestSinceNamingAnAnnotatedTagWhoseCommitObjectIsGoneReportsAnUnreadableDiff
// hold, and neither is redundant with this one.
func TestSinceNamingAnAnnotatedTagOverATreeFailsNamingThatRef(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("tag", "-a", "treetag", "-m", "the tree", f.git("rev-parse", "HEAD^{tree}"))

	f.runArgs("--since", "treetag").assertMatches(t, "since_tag_not_a_commit", 1, "",
		"no diff base: --since treetag does not name a commit\n")
}

func TestSinceNamingAPathInsideACommitFailsNamingThatRev(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--since", "HEAD:"+orderService).assertMatchesWith(t, "since_absent_sha_not_a_commit", 1, "",
		"no diff base: --since HEAD:"+orderService+" does not name a commit\n",
		map[string]string{"REV": "HEAD:" + orderService})
}

func TestStagedMeasuresOnlyWhatIsStaged(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(otherService, csharpFile(30))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.touchLine(otherService, 12)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_single_method", 0, f.headLabel(),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestStagedRefusesAFileStagedInOneStateAndDirtyInAnother(t *testing.T) {
	t.Parallel()
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

func TestStagedRefusesADirtyFileAmongMorePathspecsThanOneArgvCarries(t *testing.T) {
	t.Parallel()
	paths := deeplyNestedPaths(t, 120)
	dirty := paths[len(paths)-1]

	f := newFixture(t, "main")
	for _, path := range paths {
		f.write(path, csharpFile(80))
	}
	f.commitAll("initial")
	for _, path := range paths {
		f.touchLine(path, 62)
	}
	f.git("add", "-A")
	f.touchLine(dirty, 45)

	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(paths...), nil),
	}

	f.runArgs("--staged").assertMatchesWith(t, "staged_many_pathspecs_dirty", 1, f.headLabel(),
		"refusing to score "+dirty+": staged in one state and on disk in another\n",
		map[string]string{"PATH": dirty})
}

func TestStagedRefusesADirtyFileWhoseNameReadsAsAGlob(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.write("../outside.cs", csharpFile(10))

	f.runArgs("--files", "../outside.cs").assertMatches(t, "files_outside_repo", 1, "",
		"../outside.cs is outside the repo root\n")
}

func TestFilesNamingAPathThatDoesNotExistFailsNamingThatPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--files", "src/Ordering/Missing.cs").assertMatches(t, "files_missing", 1, "",
		"src/Ordering/Missing.cs does not exist\n")
}

func TestFilesMeasuresEveryMethodInTheNamedFilesRegardlessOfTheDiff(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.git("add", orderService)

	f.runArgs("--staged").assertMatches(t, "staged_no_commits", 1, "",
		"no diff base: this branch has no commit\n")
}

func TestFilesNamingTheSameFileTwiceHandsItToTheGateOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub.StdinLog = handed

	f.runArgs("--files", "docs/notes.md", "./docs/notes.md", orderService, "./"+orderService).
		assertMatches(t, "files_with_skipped_path", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")

	assertHandedToExtractor(t, handed, orderService+"\n")
}

func TestFilesNamingAFileInTheWrongCaseRefusesRatherThanMeasuresIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	if !caseInsensitiveFilesystem(t, f.root) {
		t.Skip("the filesystem is case sensitive, so the mis-cased spelling names no file and the case is a duplicate of the unresolvable one")
	}
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	f.runArgs("--files", "src/ordering/OrderService.cs").
		assertMatches(t, "files_wrong_case", 1, "",
			"src/ordering/OrderService.cs is not spelled as the file on disk is\n")
}

func TestFilesNamingAMisCasedRepoRootPrefixRefusesTheSpellingNotTheLocation(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)
	miscasedRoot := sameDirectoryUpperCased(t, f.root)

	typed := filepath.Join(miscasedRoot, filepath.FromSlash(orderService))
	f.runArgs("--files", typed).
		assertMatchesWith(t, "files_miscased_root_prefix", 1, "",
			typed+" is not spelled as the repo root is\n",
			map[string]string{"PATH": typed})
}

// sameDirectoryUpperCased is dir with every letter of its text upper-cased,
// skipping the case unless that name reaches the same directory. os.SameFile is
// stricter than probing the filesystem once with caseInsensitiveFilesystem,
// because a temp root can sit under a case-sensitive component on a machine
// whose own volume folds case, and then only some of the prefix folds.
//
// The skip fires on CI, which is ubuntu-latest, because this case goes end to
// end through a real mis-cased spelling and that needs a filesystem that folds.
// It is the shape a developer actually types, so it stays, and it is the only
// coverage the shipped refusal has: nothing verifies it on ubuntu-latest, and
// the golden gate/test/golden/files_miscased_root_prefix.toon is never rendered
// there, so it can drift with the document schema without a test going red.
// Trevor has accepted that residual risk.
//
// The fold is macOS-only by construction, not by an omission a job could close.
// NewRoot runs EvalSymlinks over the whole path, so a real Root's resolved field
// is symlink-free and spelled exactly as the tree spells it, and on a
// case-sensitive filesystem no other spelling reaches that directory. The unit
// cases in internal/srcpath do run the folded arm on Linux, but they
// hand-build a Root the constructor cannot produce, so they test the arm's logic
// rather than a state a Linux user reaches. Symlinking an upper-cased name onto
// the fixture repo here would not help: EvalSymlinks collapses the link and
// nothing folds.
func sameDirectoryUpperCased(t *testing.T, dir string) string {
	t.Helper()
	upper := strings.ToUpper(dir)
	if upper == dir {
		t.Skip("the fixture root has no letter to re-case, so there is no mis-cased prefix to type")
	}
	want, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(upper)
	if err != nil || !os.SameFile(want, got) {
		t.Skipf("the filesystem is case sensitive, so %s does not name the fixture root", upper)
	}
	return upper
}

func TestFilesNamingAPathThroughAFileFailsInTheDocument(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--files", orderService+"/x.cs").
		assertMatches(t, "files_not_a_directory", 1, "",
			"src/Ordering/OrderService.cs/x.cs could not be read, not a directory\n")
}

func TestFilesResolvesARelativePathAgainstTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	f.runArgsFrom("src/Ordering", "--files", "OrderService.cs").
		assertMatches(t, "files_single_file", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestStagedMeasuresNothingForAPureMoveStagedOnItsOwn(t *testing.T) {
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	f.git("mv", origin, moved)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	singleFileCoverageAndStub(t, f)

	f.runArgs("--files", orderService).assertMatches(t, "files_single_file", 0, "",
		"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestFilesReturnsEveryMethodInTheFileIncludingNestedOnes(t *testing.T) {
	t.Parallel()
	validate := span{File: orderService, Name: "OrderService.PlaceAsync.Validate", StartLine: 45, EndLine: 50, Complexity: 2}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("checkout", "--quiet", "--orphan", "unrelated")
	f.write("docs/notes.md", "first\n")
	f.commitAll("unrelated root")
	f.git("checkout", "--quiet", "main")

	f.runArgs("--since", "unrelated").assertMatches(t, "since_unrelated_histories", 1, "",
		"no diff base: HEAD and unrelated share no common ancestor\n")
}

func TestSinceOnABranchWithNoCommitSaysTheBranchHasNone(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("checkout", "--quiet", "--orphan", "work")

	f.runArgs("--since", "main").assertMatches(t, "since_no_commits", 1, "",
		"no diff base: this branch has no commit\n")
}

func TestStagedNamesTheDirtyFileWhenTheExtractorFailsOnIt(t *testing.T) {
	t.Parallel()
	const added = "src/Ordering/New.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.write(added, csharpFile(20))
	f.git("add", added)
	f.removeFile(added)
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}

	f.runArgs("--staged").assertMatches(t, "staged_extractor_dirty", 1, f.headLabel(),
		"refusing to score src/Ordering/New.cs: staged in one state and on disk in another\n")
}

// TestFilesNamingTwoHardLinksToOneInodeMeasuresBoth states the dedupe rule
// rather than guarding today's implementation of it. NamedFiles dedupes on the
// repo-relative path string and two hard links are two distinct strings, so no
// mutation of the current code turns this case red. It goes red only against a
// rewrite that dedupes on inode identity, which is the rewrite it exists to
// forbid. The symlink case next to it is what exercises EvalSymlinks.
func TestFilesNamingTwoHardLinksToOneInodeMeasuresBoth(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "release")
	blob := f.git("rev-parse", "HEAD:"+orderService)
	f.touchLine(orderService, 62)
	f.removeLooseObject(blob)

	f.runArgs("--since", "release").assertMatchesWith(t, "since_diff_unreadable", 1, f.baseLabel("release"),
		"could not read the diff: fatal: unable to read "+blob+"\n",
		map[string]string{"BLOB": blob})
}

func TestStagedReportsADiffGitRefusesToPrintInTheDocument(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")

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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}
	f.denyReadFile(orderService)
	cause := f.divergenceStderr(orderService)

	f.runArgs("--staged").assertMatchesWith(t, "staged_divergence_unreadable", 1, f.headLabel(),
		"could not read the diff: "+cause+"\n",
		map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestStagedKeepsTheExtractorsCauseWhenTheDivergenceCheckCannotRun(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}
	f.denyReadFile(orderService)

	f.runArgs("--staged").assertMatches(t, "staged_extractor_failed", 1, f.headLabel(),
		"csharp extractor exited 3\n")
}

func TestStagedRefusesAMoveWhoseIndexContentTheWorkingTreeNoLongerHolds(t *testing.T) {
	t.Parallel()
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
	f.write(moved, before)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{edited}),
	}

	f.runArgs("--staged").assertMatches(t, "staged_moved_file_dirty", 1, f.headLabel(),
		"refusing to score "+moved+": staged in one state and on disk in another\n")
}

func TestStagedNamesEveryDirtyFileInOneMessage(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(otherService, csharpFile(30))
	f.commitAll("initial")

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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.write("docs/architecture.md", "first\n")
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	f.runArgs("--files", "docs/notes.md", "docs/architecture.md", orderService).
		assertMatches(t, "files_two_skipped_paths", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesNamingAFileInADirectoryItCannotReadFailsInTheDocument(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.denyReadKeepingEntry("src/Ordering")

	f.runArgs("--files", orderService).
		assertMatches(t, "files_unreadable_directory", 1, "",
			"src/Ordering/OrderService.cs could not be read, permission denied\n")
}

// readFile is the content of a file a case asked the stub to write, which is
// how it asserts what the gate handed the extractor rather than only what came
// back out.
func readFile(t *testing.T, path string) string {
	t.Helper()
	//nolint:gosec // path is a file the case asked the stub extractor to write inside t.TempDir, and reading it back is the assertion
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestFilesNamingAnAbsolutePathMeasuresTheFileItNames(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)

	f.runArgs("--files", filepath.Join(f.root, filepath.FromSlash(orderService))).
		assertMatches(t, "files_single_file", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesNamingADirectoryFailsNamingThatPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--files", "src/Ordering").assertMatches(t, "files_directory", 1, "",
		"src/Ordering is a directory, not a file\n")
}

func TestStagedRefusesAFileACleanFilterMakesLookUnmodified(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.hideMarkedEdit()
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
	f.write(orderService, replaceLine(f.read(orderService), 45, "// line 45, hidden"))
	if f.git("diff", "--name-only", "--", orderService) != "" {
		t.Fatal("the clean filter did not hide the edit here, so git reports the ordinary divergence and the case cannot be about the blanked driver")
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
	//nolint:gosec // git spawns this sed script as filter.hide.clean to strip the marker, so it must be executable, and G306 tests the mode as a bitwise subset of 0600, which no execute bit can satisfy
	if err := os.WriteFile(clean, []byte("#!/bin/sh\nexec sed 's/, hidden//'\n"), 0o700); err != nil {
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.symlinkTo(filepath.Join(f.root, filepath.FromSlash(orderService)), "src/Ordering/Link.cs")
	singleFileCoverageAndStub(t, f)

	f.runArgs("--files", "src/Ordering/Link.cs").assertMatches(t, "files_single_file", 0, "",
		"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestFilesNamingASymlinkOutOfTheRepoFailsNamingThePathAsTyped(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	outside := filepath.Join(t.TempDir(), "outside.cs")
	writeAbsolute(t, outside, csharpFile(10))
	f.symlinkTo(outside, "src/Ordering/Outside.cs")

	f.runArgs("--files", "src/Ordering/Outside.cs").
		assertMatches(t, "files_symlink_outside_repo", 1, "",
			"src/Ordering/Outside.cs is outside the repo root\n")
}

func TestFilesListsANamedSkipAheadOfOneCoverageDiscoveryFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")
	singleFileCoverageAndStub(t, f)
	f.denyRead("TestResults/locked")

	f.runArgs("--files", "docs/notes.md", orderService).
		assertMatches(t, "files_skip_before_discovery_skip", 0, "",
			"0 of 2 changed methods over CRAP threshold 30, worst score 9.08\n")
}

func TestSinceNamingARevisionGitWillNotEvaluateReportsAnUnreadableDiff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.runArgs("--since", "HEAD@{99}").assertMatches(t, "since_ref_unreadable", 1, "",
		"could not read the diff: git rev-parse --verify --quiet HEAD@{99}: exit status 128\n")
}

func TestStagedLooksForPureMovesInTheIndexRatherThanTheWorkingTree(t *testing.T) {
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
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

// TestAnAddedSymlinkIsNeverDigestedAsItsTarget pins the other half of what the
// case above is about, which snapshot each scope digests an added path from.
// The staged branch reads the index blob, which for a link is the one line
// holding the target path; the working-tree branch reads the file on disk,
// which followed the link and digested the target's whole content. Issue 110
// reported that asymmetry, and move detection now leaves an added link out
// altogether, so neither scope digests one.
//
// Link.cs is dropped from the changed set either way, for holding no source,
// so the link is not what the misread costs. Moved.cs is. Digested through
// the link, Link.cs claims the content Origin.cs carried, two adds claim one
// deleted blob, and counting leaves both measured, so a file the developer
// only renamed comes back as a whole-file add. Under the misread this run
// measures Moved.Vanish and exits 1 for want of a coverage report.
func TestAnAddedSymlinkIsNeverDigestedAsItsTarget(t *testing.T) {
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	const target = "src/Ordering/Target.cs"
	const link = "src/Ordering/Link.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.copyFile(origin, target)
	f.commitAll("initial")
	f.copyFile(origin, moved)
	f.removeFile(origin)
	f.symlinkTo(filepath.Base(target), link)
	f.git("add", moved, link)
	f.assertAddedAs("main", link, symlink)

	handed := filepath.Join(t.TempDir(), "handed")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
		StdinLog:   handed,
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")

	assertNotHandedToExtractor(t, handed)
}

func TestFilesOrdersTheSpansItFoundRatherThanKeepingTheExtractorsOrder(t *testing.T) {
	t.Parallel()
	const calc = "src/Ordering/Calc.cs"
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("docs/notes.md", "first\n")
	f.commitAll("initial")
	f.denyRead("TestResults/locked")
	f.stub = stubConfig{Extensions: []string{".cs"}}

	f.runArgs("--files", "docs/notes.md").
		assertMatches(t, "files_only_unhandled_path", 0, "",
			"no changed methods, nothing to measure\n")
}

func TestSinceReportsAMergeBaseGitCannotWalkAsAnUnreadableDiff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "other")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.removeLooseObject(f.git("rev-parse", "HEAD"))

	cause := f.gitStderr("merge-base", "HEAD", "other")

	f.runArgs("--since", "other").assertMatchesWith(t, "since_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": cause})
}

func TestSinceNamingABranchWhoseCommitObjectIsGoneReportsAnUnreadableDiff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "other")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	gone := f.git("rev-parse", "other")
	f.removeLooseObject(gone)

	cause := f.gitStderr("cat-file", "-t", gone+"^{}")

	f.runArgs("--since", "other").assertMatchesWith(t, "since_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestSinceNamingAnExplicitPeelOfAGoneCommitFailsNamingThatRev(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "other")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.removeLooseObject(f.git("rev-parse", "other"))

	peeled := "other^{commit}"

	f.runArgs("--since", peeled).assertMatchesWith(t, "since_absent_sha_not_a_commit", 1, "",
		"no diff base: --since "+peeled+" does not name a commit\n",
		map[string]string{"REV": peeled})
}

func TestSinceNamingAFullShaTheStoreDoesNotHoldReportsAnUnreadableDiff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	gone := f.git("commit-tree", f.git("rev-parse", "HEAD^{tree}"), "-m", "unreferenced")
	f.removeLooseObject(gone)

	cause := f.gitStderr("cat-file", "-t", gone+"^{}")

	f.runArgs("--since", gone).assertMatchesWith(t, "since_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestSinceNamingAnAbbreviatedShaTheStoreDoesNotHoldFailsNamingThatRev(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	gone := f.git("commit-tree", f.git("rev-parse", "HEAD^{tree}"), "-m", "unreferenced")
	f.removeLooseObject(gone)

	abbreviated := gone[:8]

	f.runArgs("--since", abbreviated).assertMatchesWith(t, "since_absent_sha_not_a_commit", 1, "",
		"no diff base: --since "+abbreviated+" does not name a commit\n",
		map[string]string{"REV": abbreviated})
}

func TestSinceNamingAFullShaTheStoreHoldsResolvesTheBaseFromIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	base := f.git("rev-parse", "HEAD")

	f.git("checkout", "--quiet", "-b", "feature")
	f.touchLine(orderService, 62)
	f.commitAll("edit Cancel")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--since", base).assertMatches(t, "since_single_method", 0, f.baseLabel(base),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestSinceNamingAnAnnotatedTagResolvesTheBaseThroughIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("tag", "-a", "release", "-m", "the release")

	f.git("checkout", "--quiet", "-b", "feature")
	f.touchLine(orderService, 62)
	f.commitAll("edit Cancel")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runArgs("--since", "release").assertMatches(t, "since_single_method", 0, f.baseLabel("release"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestSinceNamingAnAnnotatedTagWhoseCommitObjectIsGoneReportsAnUnreadableDiff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("tag", "-a", "other", "-m", "the tag")
	tagged := f.git("rev-parse", "other^{commit}")
	tagObject := f.git("rev-parse", "other")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.removeLooseObject(tagged)

	cause := f.gitStderr("cat-file", "-t", tagObject+"^{}")

	f.runArgs("--since", "other").assertMatchesWith(t, "since_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestStagedReportsAHeadGitCannotReadAsAnUnreadableDiff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.corruptPackedRefs()

	cause := f.gitStderr("rev-parse", "--verify", "--quiet", "HEAD")

	f.runArgs("--staged").assertMatchesWith(t, "staged_head_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": cause})
}

func TestFilesKeepsTheExtractorsCauseWhenExtractionDies(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}

	f.runArgs("--files", orderService).assertMatches(t, "files_extractor_failed", 1, "",
		"csharp extractor exited 3\n")
}

func TestStagedTakesANamedCoverageReport(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.touchLine(orderService, 62)
	f.git("add", orderService)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 0)}))
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

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

func TestFilesResumesParsingAfterTheWholeListRatherThanInsideIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(otherService, csharpFile(30))
	f.commitAll("initial")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 0)}))
	f.write("artifacts/coverage.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 9), spanCoverage(61, 3, 2)...)},
		coverageClass{filename: otherService, lines: spanCoverage(11, 4, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService, otherService), []span{placeAsync, cancel, otherRun}),
	}

	f.runArgs("--files", orderService, otherService, "--coverage", "artifacts/coverage.xml").
		assertMatches(t, "files_named_directly", 0, "",
			"0 of 3 changed methods over CRAP threshold 30, worst score 9.08\n")
}
