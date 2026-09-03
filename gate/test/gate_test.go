package gate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// orderService is the source file most cases touch. Its spans are canned by
// the stub, so only its length matters.
const orderService = "src/Ordering/OrderService.cs"

// placeAsync and cancel are the two spans the stub reports in orderService:
// a complexity 9 method at 41-58 and a complexity 3 method at 60-64.
var (
	placeAsync = span{File: orderService, Name: "OrderService.PlaceAsync", StartLine: 41, EndLine: 58, Complexity: 9}
	cancel     = span{File: orderService, Name: "OrderService.Cancel", StartLine: 60, EndLine: 64, Complexity: 3}
)

func TestDocsOnlyChangePassesWithNoCoverageReportPresent(t *testing.T) {
	f := newFixture(t, "main")
	f.stub = stubConfig{Extensions: []string{".cs"}}
	f.write("docs/notes.md", "first\n")
	f.write("src/Ordering/OrderService.cs", csharpFile(80))
	f.commitAll("initial")
	f.write("docs/notes.md", "first\nsecond\n")

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestDocsOnlyChangePassesWithNoExtractorInstalledAtAll(t *testing.T) {
	f := newFixture(t, "main")
	f.write("docs/notes.md", "first\n")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.write("docs/notes.md", "first\nsecond\n")

	// No changed path carries an extension the gate's table declares, so no
	// extractor is located, and the absent binary never matters.
	f.runIn(gateOnlyDir(t)).assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestFileMovedWithNoContentChangeReportsNoChangedMethods(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	// `--no-renames` splits this into a delete plus an add carrying the whole
	// file, and ADR 0003 says a rename with no content change touches nothing.
	f.git("mv", origin, moved)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestFileMovedAndReindentedReportsNoChangedMethods(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, indented(csharpFile(20), "    "))
	f.commitAll("initial")
	f.git("mv", origin, moved)
	// Reindenting is the only edit, and TouchedLines diffs with `-w`, so the
	// move has to drop the file rather than report every line of it as added.
	f.write(moved, indented(csharpFile(20), "\t\t"))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestMethodScoringExactlyAtTheThresholdPasses(t *testing.T) {
	f := newFixture(t, "main")
	boundaryFixture(t, f, 30)

	// comp² × (1 − 1)³ + comp is exactly the threshold, and the verdict
	// compares strictly greater, so the run passes and no fix applies.
	f.run().assertMatches(t, "threshold_boundary_pass", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 30.00\n")
}

func TestMethodScoringJustOverTheThresholdFails(t *testing.T) {
	f := newFixture(t, "main")
	boundaryFixture(t, f, 29)

	// One uncovered line of thirty puts the score at 30.03, the smallest
	// margin the two-decimal cell can show, and that already fails.
	f.run().assertMatches(t, "threshold_boundary_fail", 2, f.baseLabel("main"),
		"1 of 1 changed methods over CRAP threshold 30, worst score 30.03\n")
}

func TestMethodWhoseRawScoreRoundsDownOntoTheThresholdPasses(t *testing.T) {
	const boundary = "src/Ordering/Boundary.cs"
	knot := span{File: boundary, Name: "Boundary.Knot", StartLine: 10, EndLine: 80, Complexity: 30}

	f := newFixture(t, "main")
	f.write(boundary, csharpFile(100))
	f.commitAll("initial")
	f.touchLine(boundary, 20)
	// One uncovered line in sixty puts the raw score at 30.0042, over the
	// threshold, while the cell the document prints is 30. The verdict compares
	// the rounded score, so the run passes and no reader is asked to fail on a
	// digit that is nowhere in the document.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: boundary, lines: spanCoverage(11, 60, 59)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(boundary), []span{knot}),
	}

	f.run().assertMatches(t, "threshold_boundary_rounded_pass", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 30.00\n")
}

func TestTwoSameRangeMethodsScoringTheSameAreOrderedByName(t *testing.T) {
	const pair = "src/Ordering/Pair.cs"
	// Same file, same range, same complexity and the same coverage, so the two
	// rows tie on every cell the table sorts by. Only the name settles the order,
	// and the set the rows are drained from is a map.
	a := span{File: pair, Name: "Pair.A", StartLine: 14, EndLine: 14, Complexity: 1}
	b := span{File: pair, Name: "Pair.B", StartLine: 14, EndLine: 14, Complexity: 1}

	f := newFixture(t, "main")
	f.write(pair, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(pair, 14)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: pair, lines: spanCoverage(14, 1, 1)}))
	// Reported the other way round, so the document's order cannot be the order
	// the extractor happened to use.
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(pair), []span{b, a}),
	}

	f.run().assertMatches(t, "same_line_equal_scores", 0, f.baseLabel("main"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 1.00\n")
}

func TestExtractorReportingADifferentLanguageThanItWasLocatedUnderFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Language: "fsharp", Extensions: []string{".cs"}}

	f.run().assertMatches(t, "capabilities_language_mismatch", 1, f.baseLabel("main"),
		"csharp extractor reported language fsharp\n")
}

func TestExtractorClaimingNoneOfTheFilesItWasLocatedForFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// The table located csharp because a .cs file changed; a binary that then
	// handles only .fs would leave that .cs file scored by nobody.
	f.stub = stubConfig{Extensions: []string{".fs"}}

	f.run().assertMatches(t, "capabilities_claims_nothing", 1, f.baseLabel("main"),
		"csharp extractor claims none of the changed files the language table located it for\n")
}

func TestExtractorReturningTheSameSpanTwiceFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	// The identical row twice: one of the two would vanish from the table with
	// nothing said about it, which is an extractor bug and not a join the gate
	// should guess its way through.
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, placeAsync}),
	}

	f.run().assertMatches(t, "duplicate_span", 1, f.baseLabel("main"),
		"csharp extractor returned OrderService.PlaceAsync twice, covering src/Ordering/OrderService.cs lines 41-58\n")
}

func TestTwoMethodsDeclaredOnOneLineAreBothScored(t *testing.T) {
	const twins = "src/Ordering/Twins.cs"
	// `public class C { public int A() => 1; public int B() => 2; }` is valid
	// C#, and the real Roslyn extractor reports both methods over line 14. Two
	// methods sharing a range are not a duplicate, and neither may be dropped.
	first := span{File: twins, Name: "Twins.A", StartLine: 14, EndLine: 14, Complexity: 1}
	second := span{File: twins, Name: "Twins.B", StartLine: 14, EndLine: 14, Complexity: 2}

	f := newFixture(t, "main")
	f.write(twins, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(twins, 14)
	// Line 14 is the only instrumentable line, and it belongs to both methods,
	// because neither is narrower than the other.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: twins, lines: spanCoverage(14, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(twins), []span{first, second}),
	}

	f.run().assertMatches(t, "same_line_two_methods", 0, f.baseLabel("main"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 2.00\n")
}

func TestChangedSourceFileWithNoExtractorInstalledFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	// Unlike a docs-only change, a changed .cs file selects the csharp row, so
	// the absent binary is the run's failure rather than nobody's business.
	f.runIn(gateOnlyDir(t)).assertMatches(t, "extractor_missing", 1, f.baseLabel("main"),
		"csharp extractor not found: metric-gate-csharp\n")
}

func TestExtractorThatIsPresentButNotExecutableFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	dir := gateOnlyDir(t)
	plantUnrunnableExtractor(t, dir)

	f.runIn(dir).assertMatches(t, "extractor_not_executable", 1, f.baseLabel("main"),
		"csharp extractor metric-gate-csharp could not be run: permission denied\n")
}

func TestExtractorEmittingSomethingOtherThanJSONFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}, Stdout: "not json"}

	f.run().assertMatches(t, "extractor_invalid_json", 1, f.baseLabel("main"),
		"csharp extractor emitted invalid JSON\n")
}

func TestExtractorEmittingCapabilitiesThatAreNotJSONFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}, CapabilitiesStdout: "not json"}

	f.run().assertMatches(t, "extractor_invalid_capabilities_json", 1, f.baseLabel("main"),
		"csharp extractor emitted invalid capabilities JSON\n")
}

func TestExtractorSilentAboutAFileItWasGivenFails(t *testing.T) {
	const ghost = "src/Ordering/Ghost.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(ghost, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(ghost, 7)
	// Ghost.cs was handed in and never reported on, which is neither "nothing
	// here" nor "I failed", so its methods must not go unmeasured under a pass.
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "extractor_no_parse_status", 1, f.baseLabel("main"),
		"csharp extractor reported no parse status for src/Ordering/Ghost.cs\n")
}

func TestCoverageWalkRecordsAPathItCouldNotRead(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	// A second results directory the walk cannot enter. Coverage may be
	// understated by whatever report it holds, so the path is reported.
	f.denyRead("TestResults/locked")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "skipped_unreadable_path", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestWellTestedMethodForItsComplexityPasses(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// Two thirds of Cancel's three instrumentable lines are covered.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestMethodWithTooLittleCoverageForItsComplexityFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	// A tenth of PlaceAsync's ten instrumentable lines is covered.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(42, 10, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "fail_single_method", 2, f.baseLabel("main"),
		"1 of 1 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestFourChangedMethodsEachGetTheFixThatApplies(t *testing.T) {
	const pricing = "src/Ordering/Pricing.cs"
	const order = "src/Ordering/Order.cs"
	quote := span{File: pricing, Name: "Pricing.Quote", StartLine: 18, EndLine: 71, Complexity: 34}
	getID := span{File: order, Name: "Order.get_Id", StartLine: 14, EndLine: 14, Complexity: 1}

	f := newFixture(t, "main")
	f.write(pricing, csharpFile(90))
	f.write(orderService, csharpFile(80))
	f.write(order, csharpFile(30))
	f.commitAll("initial")
	f.touchLine(pricing, 20)
	f.touchLine(orderService, 45)
	f.touchLine(orderService, 62)
	f.touchLine(order, 14)
	// A field, outside every span, so it is a diagnostic and not an unknown.
	f.touchLine(order, 25)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: pricing, lines: spanCoverage(19, 20, 11)},
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)},
		// Order.cs matches a report path but get_Id's one line is not
		// instrumentable, which is what makes it structural_na.
		coverageClass{filename: order, lines: spanCoverage(20, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout: extractorOutput(t, parsed(order, orderService, pricing),
			[]span{quote, placeAsync, cancel, getID}),
	}

	f.run().assertMatches(t, "four_methods", 2, f.baseLabel("main"),
		"2 of 4 changed methods over CRAP threshold 30, worst score 139.34\n")
}

func TestBranchIsScoredAgainstTheMergeBaseAndNotTheTipOfMain(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")

	f.git("checkout", "--quiet", "-b", "feature")
	f.touchLine(orderService, 62)
	f.commitAll("edit Cancel on the branch")

	// main moves on without the branch, so the tip of main is no longer an
	// ancestor of HEAD and only the merge base scopes the diff correctly. The
	// commit it moves on by edits a line inside PlaceAsync, so a diff scoped to
	// main's tip would report that method changed as well and this document
	// would not match.
	f.git("checkout", "--quiet", "main")
	f.touchLine(orderService, 45)
	f.commitAll("edit PlaceAsync on main")
	f.git("checkout", "--quiet", "feature")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	// The branch's own commit is the whole changed set, and `base` is the
	// merge base rather than main's tip. The document is the one
	// pass_single_method pins, because the same one method changed.
	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestMethodMovedToANewPathAndEditedIsScoredAtItsNewLocation(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	// git scores this as a rename, which status R would drop from an
	// ACM-filtered diff; ADR 0003 says the method appears as added lines at
	// its new location instead.
	f.git("mv", origin, moved)
	f.touchLine(moved, 7)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: moved, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.run().assertMatches(t, "renamed_file", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestANestedSpanTakesTheTouchedLineAndItsOwnCoverageFromItsContainer(t *testing.T) {
	const outer = "src/Ordering/Outer.cs"
	container := span{File: outer, Name: "Outer.Run", StartLine: 10, EndLine: 60, Complexity: 5}
	local := span{File: outer, Name: "Outer.Run.Local", StartLine: 30, EndLine: 40, Complexity: 2}

	f := newFixture(t, "main")
	f.write(outer, csharpFile(80))
	f.commitAll("initial")
	// Line 35 sits in both spans, so only the smaller one is changed.
	f.touchLine(outer, 35)
	// Lines 12-15 belong to the container alone; 31-34 sit inside the local
	// function, so the container must not absorb them.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: outer, lines: append(spanCoverage(12, 4, 4), spanCoverage(31, 4, 1)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(outer), []span{container, local}),
	}

	// One in four of the local function's own lines is covered. Were the
	// container absorbing them the local function would hold no
	// instrumentable line and report structural_na instead.
	f.run().assertMatches(t, "nested_spans", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.69\n")
}

func TestChangedMethodWithNoCoverageReportAnywhereFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_missing", 1, f.baseLabel("main"),
		"CRAP requires a coverage report, none found\n")
}

func TestRepoWithNoOriginAndNoMainOrMasterFails(t *testing.T) {
	f := newFixture(t, "trunk")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}}

	f.run().assertMatches(t, "no_diff_base", 1, "",
		"no diff base: tried origin/HEAD, origin/main, origin/master, main, master\n")
}

func TestChangedMethodInAFileNoReportPathMatchedFails(t *testing.T) {
	const ghost = "src/Ordering/Ghost.cs"
	vanish := span{File: ghost, Name: "Ghost.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(ghost, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(ghost, 7)
	// The report knows OrderService.cs and has never heard of Ghost.cs.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(ghost, orderService), []span{vanish, cancel}),
	}

	f.run().assertMatches(t, "unknown_changed_method", 1, f.baseLabel("main"),
		"1 changed method could not be attributed to a coverage report\n"+
			"0 of 2 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestFileTheExtractorCouldNotParseFails(t *testing.T) {
	const broken = "src/Ordering/Broken.cs"

	f := newFixture(t, "main")
	f.write(broken, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(broken, 7)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, failedToParse(broken), nil),
	}

	f.run().assertMatches(t, "parse_failed", 1, f.baseLabel("main"),
		"csharp extractor could not parse src/Ordering/Broken.cs\n")
}

func TestExtractorEchoingAPathItWasNotGivenFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed("src/Ordering/Imposter.cs"), nil),
	}

	f.run().assertMatches(t, "extractor_path_mismatch", 1, f.baseLabel("main"),
		"csharp extractor returned a path it was not given: src/Ordering/Imposter.cs\n")
}

func TestExtractorExitingNonZeroFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}

	f.run().assertMatches(t, "extractor_failed", 1, f.baseLabel("main"),
		"csharp extractor exited 3\n")
}

func TestCoverageReportThatIsNotValidXMLFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/fixed/coverage.cobertura.xml", "<coverage><packages>\n")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_unparseable", 1, f.baseLabel("main"),
		"could not parse coverage report TestResults/fixed/coverage.cobertura.xml\n")
}

func TestMoveWithAnExtraCopyMeasuresBothAddedPaths(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	const copied = "src/Ordering/Copy.cs"
	movedVanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}
	copiedVanish := span{File: copied, Name: "Copy.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	// Two added paths carry the deleted file's content and `--no-renames` is
	// what removed git's own answer to which of them is the move, so neither is
	// dropped. Picking one would silently unscore a brand-new file.
	f.git("mv", origin, moved)
	f.copyFile(moved, copied)
	f.git("add", copied)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: moved, lines: spanCoverage(6, 4, 1)},
		coverageClass{filename: copied, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(copied, moved), []span{movedVanish, copiedVanish}),
	}

	f.run().assertMatches(t, "move_with_copy", 0, f.baseLabel("main"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestADeletedObjectTheGateCannotReadStillEmitsADocument(t *testing.T) {
	const gone = "src/Ordering/Gone.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(gone, csharpFile(20))
	f.commitAll("initial")
	// A blobless partial clone has no local object for a deleted file, and the
	// promisor fetch that `cat-file` then attempts fails offline. Removing the
	// object is that condition, and the move detection has to skip the object
	// rather than let the error escape and leave the run with no document.
	blob := f.git("rev-parse", "HEAD:"+gone)
	f.git("rm", "--quiet", gone)
	f.removeLooseObject(blob)
	f.touchLine(orderService, 62)
	// An added path of some kind is what sends the move detection to read the
	// deleted side at all.
	f.write("docs/notes.md", "new file\n")
	f.git("add", "docs/notes.md")
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestRemovingASubmoduleStillEmitsADocument(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// A gitlink's object id names a commit in another repository, so reading it
	// as a blob fails. The move detection has to skip it rather than let the
	// error escape and leave the run with no document at all.
	f.addSubmoduleGitlink("vendor/lib")
	f.removeSubmoduleGitlink("vendor/lib")
	f.touchLine(orderService, 62)
	// An added path of some kind is what sends the move detection to read the
	// deleted side at all, and the gitlink is on that deleted side.
	f.write("docs/notes.md", "new file\n")
	f.git("add", "docs/notes.md")
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestHostileGitConfigAndEnvironmentDoNotHideTheChange(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	// Each of these reshapes `git diff` into something the hunk parser reads as
	// no change at all: colour codes ahead of every header, a w/ prefix instead
	// of b/, no prefix, or an external driver that prints nothing. A gate that
	// measures nothing passes, so every one of them is a silent green build.
	f.git("config", "color.ui", "always")
	f.git("config", "diff.mnemonicPrefix", "true")
	f.git("config", "diff.noprefix", "true")
	f.git("config", "diff.external", externalDiffScript(t))

	// The config the variable carries is a clean filter, which no command-line
	// flag turns off, rather than one of the settings the `-c` overrides already
	// pin. A payload a flag beats would leave this case green with the whole
	// environment scrub deleted.
	result := f.runWithEnv(
		"GIT_EXTERNAL_DIFF="+externalDiffScript(t),
		cleanFilterParameters(t),
	)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestGitDirPointedAtAnotherRepositoryDoesNotHideTheChange(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	// These outrank the working directory the gate runs git in, so every
	// question it asks would be answered by a second, unchanged repository: the
	// root resolves there, the diff comes back empty, and the run passes with no
	// changed methods. git exports them into every hook it runs, so they are
	// present in the exact deployment this gate is built for.
	other := newFixture(t, "main")
	other.write("src/Ordering/Elsewhere.cs", csharpFile(20))
	other.commitAll("initial")

	result := f.runWithEnv(
		"GIT_DIR="+filepath.Join(other.root, ".git"),
		"GIT_WORK_TREE="+other.root,
		"GIT_INDEX_FILE="+filepath.Join(other.root, ".git", "index"),
		"GIT_CEILING_DIRECTORIES="+f.root,
	)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestATextconvDriverDoesNotHideTheChange(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	// A textconv driver replaces both sides of the diff with whatever it prints,
	// and this one prints the same constant for every input, so every hunk header
	// disappears. The attribute lives in the repository, ahead of anything the
	// run can pass on the command line, and only `--no-textconv` answers it.
	f.write(".gitattributes", "*.cs diff=hide\n")
	f.commitAll("initial")
	f.git("config", "diff.hide.textconv", constantTextconvScript(t))
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestLinesDeletedFromInsideAMethodChangeIt(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// A pure deletion produces a zero-length new-side hunk, "@@ -62,2 +61,0 @@".
	// Nothing was added, but Cancel is not the method it was.
	f.deleteLines(orderService, 62, 63)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestFirstLineOfAFileDeletedChangesTheMethodThatStartsThere(t *testing.T) {
	const head = "src/Ordering/Head.cs"
	first := span{File: head, Name: "Head.First", StartLine: 1, EndLine: 5, Complexity: 2}

	f := newFixture(t, "main")
	f.write(head, csharpFile(20))
	f.commitAll("initial")
	// git reports the insertion point of a deletion at the top of a file as 0,
	// which is not a line. Line 1 is the nearest line that exists.
	f.deleteLines(head, 1, 1)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: head, lines: spanCoverage(2, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(head), []span{first}),
	}

	f.run().assertMatches(t, "top_of_file_deletion", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 2.15\n")
}

func TestAnAddedLineThatLooksLikeAFileHeaderDoesNotStealLaterHunks(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// An added line whose own text starts with "++ " renders as "+++ fake".
	// Reading that as a file header would reattribute the file's later hunks to
	// a path that does not exist, and Cancel would go unscored under a pass.
	f.write(orderService, replaceLine(f.read(orderService), 45, "++ fake"))
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_hunks_one_file", 2, f.baseLabel("main"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestTwoCoverageReportsAreUnionedRatherThanOverwritten(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// Two test projects each list all three of Cancel's lines and each hits a
	// different one, so a hit anywhere is a hit and two of three are covered.
	// Neither report alone reaches that, whichever order the walk reads them
	// in, so letting the later one overwrite the earlier scores one of three.
	f.write("TestResults/unit/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 4}, {number: 62, hits: 0}, {number: 63, hits: 0}}}))
	f.write("TestResults/integration/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 0}, {number: 62, hits: 1}, {number: 63, hits: 0}}}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestCoverageReportOutsideAResultsDirectoryIsNotFound(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// ADR 0004 anchors discovery on a TestResults directory. A report at the
	// repo root is not one, and quietly reading it would make the gate's idea
	// of coverage depend on stray files.
	f.write("coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_missing", 1, f.baseLabel("main"),
		"CRAP requires a coverage report, none found\n")
}

func TestTwoOverloadsDeclaredOnOneLineAreBothScored(t *testing.T) {
	const calc = "src/Ordering/Calc.cs"
	// `public class C { public int F(int x) => 1; public int F(string x) => 2; }`
	// is valid C#. The two overloads share a file, a name and a line range, and
	// only their parameter lists tell them apart, so a duplicate test that reads
	// name alone calls this an extractor contract violation and scores neither.
	first := span{File: calc, Name: "Calc.F", Signature: "(int)", StartLine: 14, EndLine: 14, Complexity: 1}
	second := span{File: calc, Name: "Calc.F", Signature: "(string)", StartLine: 14, EndLine: 14, Complexity: 2}

	f := newFixture(t, "main")
	f.write(calc, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(calc, 14)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: calc, lines: spanCoverage(14, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(calc), []span{first, second}),
	}

	// Two rows under one name, carrying the two complexities and so the two
	// scores. Collapsing them into one would lose whichever the map dropped.
	f.run().assertMatches(t, "same_line_two_overloads", 0, f.baseLabel("main"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 2.00\n")
}

func TestChangedMethodInAUTF16SourceFileIsStillMeasured(t *testing.T) {
	const wide = "src/Ordering/Wide.cs"
	vanish := span{File: wide, Name: "Wide.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	// UTF-16 puts a NUL beside every ASCII character, so git autodetects the file
	// as binary and prints "Binary files differ" in place of any hunk header. The
	// gate would then see no touched line, measure nothing, and pass.
	f.writeUTF16LE(wide, csharpFile(20))
	f.commitAll("initial")
	f.writeUTF16LE(wide, replaceLine(csharpFile(20), 7, "// line 7, edited"))
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: wide, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(wide), []span{vanish}),
	}

	f.run().assertMatches(t, "binary_source_file", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestGitattributesMarkingSourceUndiffableDoesNotHideTheChange(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	// `-diff` lives in the repository, ahead of anything the run can pass on the
	// command line, and it suppresses every hunk header for the paths it names.
	f.write(".gitattributes", "*.cs -diff\n")
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestCleanFilterInjectedThroughTheEnvironmentDoesNotHideTheChange(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	// GIT_CONFIG_COUNT config outranks a -c flag, and there is no flag for this
	// anyway, so the only defence is dropping the variables from the child's
	// environment. The filter reverses the edit and leaves the two sides equal.
	result := f.runWithEnv(cleanFilterEnv(t)...)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestChangedMethodInAPathGitQuotesIsStillMeasured(t *testing.T) {
	const weird = `src/Ordering/we"ird.cs`
	vanish := span{File: weird, Name: "Weird.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	// A path holding a double quote is one git quotes and escapes whatever
	// core.quotePath says, so no -c flag can turn this off. Read literally the
	// header names a path no extractor was given and no report ever matched.
	f.write(weird, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(weird, 7)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: weird, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(weird), []span{vanish}),
	}

	f.run().assertMatches(t, "quoted_diff_path", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestMissingCoverageFailureStillReportsThePathsTheWalkCouldNotRead(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// The one directory that could have held a report is unreadable. Reporting
	// "none found" without saying so reads as a repo with no coverage at all,
	// rather than one whose coverage the run could not reach.
	f.denyRead("TestResults/locked")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_missing_with_skipped_path", 1, f.baseLabel("main"),
		"CRAP requires a coverage report, none found\n")
}

func TestRunOutsideAGitRepoWritesNoDocumentAndExitsOne(t *testing.T) {
	f := &fixture{t: t, root: t.TempDir()}
	if inRepo(t, f.root) {
		t.Skip("the temp directory is itself inside a git repo")
	}

	result := f.run()

	// This failure is upstream of the document, so ADR 0005's one-TOON-document
	// rule cannot apply: there is no base and no scope to report.
	if result.exitCode != 1 || result.stdout != "" || !strings.Contains(result.stderr, "not a git repository") {
		t.Errorf("gate outside a repo: got exit %d, stdout %q, stderr %q; want exit 1, empty stdout, stderr naming the missing repo",
			result.exitCode, result.stdout, result.stderr)
	}
}

// inRepo reports whether dir sits inside a git working tree, which decides
// whether the non-repo case can run at all.
func inRepo(t *testing.T, dir string) bool {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), gitEnv...)
	return cmd.Run() == nil
}
