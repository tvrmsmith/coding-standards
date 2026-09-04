package gate_test

import (
	"fmt"
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

func TestTwoIdenticalFilesMovedTogetherReportNoChangedMethods(t *testing.T) {
	const firstOrigin = "src/Ordering/First.cs"
	const secondOrigin = "src/Ordering/Second.cs"
	const firstMoved = "src/Ordering/FirstMoved.cs"
	const secondMoved = "src/Ordering/SecondMoved.cs"
	firstVanish := span{File: firstMoved, Name: "FirstMoved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}
	secondVanish := span{File: secondMoved, Name: "SecondMoved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(firstOrigin, csharpFile(20))
	f.write(secondOrigin, csharpFile(20))
	f.commitAll("initial")
	// Two adds and two deletes carrying one content digest. Every add is
	// accounted for by a delete, so the diff explains itself as a move of both
	// and neither add is measured. Refusing to drop either, because the digest
	// has more than one claimant, would demand coverage for every method in both
	// files and turn a directory rename into the wall of failures the drop
	// exists to prevent.
	f.git("mv", firstOrigin, firstMoved)
	f.git("mv", secondOrigin, secondMoved)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(firstMoved, secondMoved), []span{firstVanish, secondVanish}),
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
	//
	// The signatures run against the names, so the arm below the name is a total
	// order pointing the other way. Without that the case would be a coin flip:
	// drop the name arm and the rows come out in whatever order the map yielded,
	// which matches this golden about half the time.
	a := span{File: pair, Name: "Pair.A", Signature: "(string)", StartLine: 14, EndLine: 14, Complexity: 1}
	b := span{File: pair, Name: "Pair.B", Signature: "(int)", StartLine: 14, EndLine: 14, Complexity: 1}

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

func TestExtractorReturningTheSameSpanWithTwoComplexitiesFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	// The same method twice under two complexities. One of the two rows still
	// vanishes from the table, and which one survived is the difference between a
	// pass and a fail here, so the duplicate test cannot let a differing score
	// make two rows into two methods.
	rescored := placeAsync
	rescored.Complexity = 40
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, rescored}),
	}

	f.run().assertMatches(t, "duplicate_span", 1, f.baseLabel("main"),
		"csharp extractor returned OrderService.PlaceAsync twice, covering src/Ordering/OrderService.cs lines 41-58\n")
}

func TestExtractorReportingAComplexityBelowTheMcCabeBaseFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	// An absent `complexity` field unmarshals to exactly this, and 0 scores 0, so
	// letting it through prints a passing row for a method nobody measured.
	uncounted := placeAsync
	uncounted.Complexity = 0
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{uncounted}),
	}

	f.run().assertMatches(t, "invalid_span_complexity", 1, f.baseLabel("main"),
		"csharp extractor reported OrderService.PlaceAsync in src/Ordering/OrderService.cs with complexity 0, which is below the McCabe base of 1\n")
}

func TestExtractorReportingAStartLineBelowOneFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	unplaced := placeAsync
	unplaced.StartLine = 0
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{unplaced}),
	}

	f.run().assertMatches(t, "invalid_span_start", 1, f.baseLabel("main"),
		"csharp extractor reported OrderService.PlaceAsync in src/Ordering/OrderService.cs with start line 0, which names no line\n")
}

func TestExtractorReportingAnEndLineBelowItsStartFails(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	// A negative width is the quietest of the three: no line is ever inside the
	// span, so the method leaves the table and the run says nothing changed.
	inverted := placeAsync
	inverted.StartLine, inverted.EndLine = 58, 41
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{inverted}),
	}

	f.run().assertMatches(t, "invalid_span_range", 1, f.baseLabel("main"),
		"csharp extractor reported OrderService.PlaceAsync in src/Ordering/OrderService.cs with end line 41 below start line 58\n")
}

func TestChangedFileWhoseExtensionIsSpeltInAnotherCaseIsStillMeasured(t *testing.T) {
	// macOS and Windows both carry case-insensitive filesystems, so this is an
	// ordinary file a developer creates without noticing. Routed by an exact
	// match it reaches no extractor, and the run passes with the method unscored.
	const shouted = "src/Ordering/Order.CS"
	vanish := span{File: shouted, Name: "Order.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(shouted, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(shouted, 7)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: shouted, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(shouted), []span{vanish}),
	}

	f.run().assertMatches(t, "case_variant_extension", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
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

func TestWholeFileReformatWithNoLogicChangeMeasuresNothing(t *testing.T) {
	const legacy = "src/Ordering/Legacy.cs"
	knot := span{File: legacy, Name: "Legacy.Knot", StartLine: 10, EndLine: 40, Complexity: 34}
	tangle := span{File: legacy, Name: "Legacy.Tangle", StartLine: 42, EndLine: 58, Complexity: 20}

	f := newFixture(t, "main")
	f.write(legacy, csharpFile(60))
	f.commitAll("initial")
	// Indenting the whole file is the only change. Under -w this produces no
	// "+++" line and no hunk at all for Legacy.cs, so Legacy.cs never reaches
	// the extractor and the run never reads the stub's spans.
	f.write(legacy, indented(csharpFile(60), "        "))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		// The fixture writes no coverage report anywhere, which is
		// load-bearing. A run that measured even one method here would stop
		// at coverage_missing and exit 1, so this case cannot pass by
		// accident.
		Stdout: extractorOutput(t, parsed(legacy), []span{knot, tangle}),
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
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

func TestMethodMovedBetweenFilesWithItsBodyUnchangedIsMeasuredAtItsNewLocation(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const destination = "src/Ordering/Destination.cs"
	// Does not contain line 9, the line Origin's own touch lands on, so
	// Origin's side of the move is a diagnostic and never a changed method.
	originKeep := span{File: origin, Name: "Origin.Keep", StartLine: 20, EndLine: 30, Complexity: 2}
	// Proves only the moved method is measured, not everything sitting near it.
	destinationUntouched := span{File: destination, Name: "Destination.Untouched", StartLine: 5, EndLine: 15, Complexity: 12}
	destinationVanish := span{File: destination, Name: "Destination.Vanish", StartLine: 30, EndLine: 40, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(60))
	f.write(destination, csharpFile(60))
	f.commitAll("initial")
	// Neither file is renamed, both stay status M, so the pure-move drop never
	// fires and the method's body is byte-identical at its new home. git
	// reports Origin's removal as a zero-length hunk touching line 9 alone,
	// and Destination's insertion as lines 30 through 40.
	f.moveLines(origin, 10, 20, destination, 29)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: destination, lines: spanCoverage(31, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout: extractorOutput(t, parsed(origin, destination),
			[]span{originKeep, destinationUntouched, destinationVanish}),
	}

	f.run().assertMatches(t, "moved_between_files", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestMethodMovedWithinOneFileIsMeasuredAtItsNewLocation(t *testing.T) {
	const shuffle = "src/Ordering/Shuffle.cs"
	// Holds neither the vacated position nor the moved block, so it stays out
	// of the table and shows the gate measures the move alone.
	keep := span{File: shuffle, Name: "Shuffle.Keep", StartLine: 20, EndLine: 30, Complexity: 2}
	vanish := span{File: shuffle, Name: "Shuffle.Vanish", StartLine: 35, EndLine: 45, Complexity: 4}

	f := newFixture(t, "main")
	f.write(shuffle, csharpFile(60))
	f.commitAll("initial")
	// moveLines rewrites src before it re-reads dst, so with one file the
	// after argument counts lines in the shortened file. 34 there is old line
	// 45, and the moved block lands at 35 through 45. git reports two hunks,
	// a zero-length one touching line 9 alone where the block used to sit and
	// an insertion covering lines 35 through 45. Line 9 falls inside no span,
	// so the vacated position contributes exactly one diagnostic.
	f.moveLines(shuffle, 10, 20, shuffle, 34)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: shuffle, lines: spanCoverage(36, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(shuffle), []span{keep, vanish}),
	}

	f.run().assertMatches(t, "moved_within_file", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

// TestDeletingAMethodAttributesTheZeroLengthHunkToTheLineBeforeIt pins which
// line a pure deletion touches. Before and Boundary sit either side of the
// insertion point, so the two candidate answers produce different documents.
// Move the touch to line 8 and the table names Legacy.Before over 5-8 at
// coverage 1 and score 3 instead, and the golden stops matching.
func TestDeletingAMethodAttributesTheZeroLengthHunkToTheLineBeforeIt(t *testing.T) {
	const legacy = "src/Ordering/Legacy.cs"
	before := span{File: legacy, Name: "Legacy.Before", StartLine: 5, EndLine: 8, Complexity: 3}
	boundary := span{File: legacy, Name: "Legacy.Boundary", StartLine: 9, EndLine: 9, Complexity: 3}
	// Well over the threshold and holding no touched line, so the gate never
	// measures it and it never reaches the table. Survivor is not the deleted
	// method and cannot stand in for it. The gate takes spans from the working
	// tree alone, so a deleted method has no span at all and the clause that
	// it is never measured has nothing that could falsify it.
	survivor := span{File: legacy, Name: "Legacy.Survivor", StartLine: 15, EndLine: 25, Complexity: 34}

	f := newFixture(t, "main")
	f.write(legacy, csharpFile(60))
	f.commitAll("initial")
	// Deleting lines 10-40 produces a zero-length hunk whose insertion point
	// is line 9, so line 9 is the only touched line. It falls inside Boundary
	// and outside Before, which makes Boundary the one changed method.
	f.deleteLines(legacy, 10, 40)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: legacy, lines: append(spanCoverage(5, 4, 4), spanCoverage(9, 1, 0)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(legacy), []span{before, boundary, survivor}),
	}

	f.run().assertMatches(t, "deleted_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 12.00\n")
}

func TestEveryMethodInANewlyAddedFileIsMeasured(t *testing.T) {
	const fresh = "src/Ordering/Fresh.cs"
	first := span{File: fresh, Name: "Fresh.First", StartLine: 3, EndLine: 8, Complexity: 4}
	second := span{File: fresh, Name: "Fresh.Second", StartLine: 10, EndLine: 16, Complexity: 9}

	f := newFixture(t, "main")
	// A file already on main so a base exists for Fresh.cs to diff against.
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.write(fresh, csharpFile(20))
	// `git add` is required, not incidental. ADR 0003's tracked-paths-only
	// amendment says touched lines come from tracked paths only, so an
	// unstaged new file contributes nothing.
	f.git("add", fresh)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: fresh, lines: append(spanCoverage(4, 4, 1), spanCoverage(11, 5, 1)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		// Lines 1, 2, 9, 17, 18, 19 and 20 fall inside no span, seven
		// diagnostics against an entirely-added file.
		Stdout: extractorOutput(t, parsed(fresh), []span{first, second}),
	}

	f.run().assertMatches(t, "new_file", 2, f.baseLabel("main"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 50.47\n")
}

// TestRealCommitMixingEditMoveDeletionAdditionRenameAndReflow is the
// capstone issue 13 is named for: one commit doing everything at once, and
// the gate sorting each kind of change into the right bucket without a
// special case for any of them.
//
//   - OrderService.cs gets an ordinary edit, an easy sanity check that
//     everything else in the tree does not drown it out.
//   - Legacy.cs is reflowed with no logic change. It passes --diff-filter=ACM
//     as status M, but -w leaves it with no hunk, so no touched line is
//     attributed to it and it never reaches the extractor.
//   - Origin.cs and Destination.cs repeat the moved-method case: the method
//     is measured at its new home in Destination, and Origin's own touch is
//     a diagnostic only.
//   - Doomed.cs is deleted outright and contributes nothing to the table.
//     This case does not pin --diff-filter=ACM, because git renders a deleted
//     file's new side as /dev/null, a path no extractor claims, so an
//     unfiltered deletion would be discarded downstream anyway.
//   - Stable.cs is renamed to Renamed.cs with no content change. --no-renames
//     turns that into a D/A pair, and the pure-move drop, driven by the raw
//     listing pairing them on one content digest, removes it again.
//   - Fresh.cs is added and staged, so every method in it is measured, and
//     Fresh.cs is what proves the pure-move drop above did not also catch an
//     unrelated new file that happens to share the diff.
//   - notes.md is touched but no extractor claims .md files, so its line
//     counts toward nothing, not even the outside-spans diagnostic.
func TestRealCommitMixingEditMoveDeletionAdditionRenameAndReflow(t *testing.T) {
	const legacy = "src/Ordering/Legacy.cs"
	const origin = "src/Ordering/Origin.cs"
	const destination = "src/Ordering/Destination.cs"
	const doomed = "src/Ordering/Doomed.cs"
	const stable = "src/Ordering/Stable.cs"
	const renamed = "src/Ordering/Renamed.cs"
	const fresh = "src/Ordering/Fresh.cs"
	const notes = "docs/notes.md"

	originKeep := span{File: origin, Name: "Origin.Keep", StartLine: 20, EndLine: 30, Complexity: 2}
	destinationUntouched := span{File: destination, Name: "Destination.Untouched", StartLine: 5, EndLine: 15, Complexity: 12}
	destinationVanish := span{File: destination, Name: "Destination.Vanish", StartLine: 30, EndLine: 40, Complexity: 4}
	freshFirst := span{File: fresh, Name: "Fresh.First", StartLine: 3, EndLine: 8, Complexity: 4}
	freshSecond := span{File: fresh, Name: "Fresh.Second", StartLine: 10, EndLine: 16, Complexity: 9}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(legacy, csharpFile(60))
	f.write(origin, csharpFile(60))
	f.write(destination, csharpFile(60))
	f.write(doomed, csharpFile(40))
	f.write(stable, csharpFile(30))
	f.write(notes, "first\n")
	f.commitAll("initial")

	f.touchLine(orderService, 45)
	f.write(legacy, indented(csharpFile(60), "        "))
	f.moveLines(origin, 10, 20, destination, 29)
	f.git("rm", "-q", doomed)
	f.git("mv", stable, renamed)
	f.write(fresh, csharpFile(20))
	f.git("add", fresh)
	f.write(notes, "first\nsecond\n")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(42, 10, 1)},
		coverageClass{filename: destination, lines: spanCoverage(31, 4, 1)},
		coverageClass{filename: fresh, lines: append(spanCoverage(4, 4, 1), spanCoverage(11, 5, 1)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout: extractorOutput(t, parsed(destination, fresh, orderService, origin),
			[]span{placeAsync, cancel, originKeep, destinationUntouched, destinationVanish, freshFirst, freshSecond}),
	}

	f.run().assertMatches(t, "real_commit", 2, f.baseLabel("main"),
		"2 of 4 changed methods over CRAP threshold 30, worst score 68.05\n")
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

func TestRepoWithNoResolvableDiffBaseNamesEveryRefAndPointsAtTheFlag(t *testing.T) {
	f := newFixture(t, "trunk")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// A second commit is what makes the absent HEAD~1 fallback load-bearing.
	// With one commit HEAD~1 does not resolve either, so a gate that did fall
	// back would emit this same document. With two, a fallback would resolve
	// a base and then fail downstream on the stub's empty output, which is
	// unparsable extractor JSON, rather than producing the no-diff-base error
	// this case pins.
	f.touchLine(orderService, 20)
	f.commitAll("second")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}}

	f.run().assertMatches(t, "no_diff_base", 1, "",
		"no diff base: tried origin/HEAD, origin/main, origin/master, main, master; name one with --since <ref>\n")
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

func TestRemovedGitlinkNamingABlobDoesNotDropTheAddedFile(t *testing.T) {
	const origin = "src/Ordering/Origin.cs"
	const copied = "src/Ordering/Copy.cs"
	vanish := span{File: copied, Name: "Copy.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	// A gitlink's object id names a commit in another repository, so the move
	// detection must not read it as content this repository deleted. Here the id
	// is also a blob id in this repository, which is what tells the guard apart
	// from the `cat-file` failure that otherwise hides its absence: read as a
	// blob, the removed gitlink carries Origin.cs's content, and the brand-new
	// file carrying that same content is dropped as a move and never scored.
	f.addGitlink("vendor/lib", f.git("rev-parse", "HEAD:"+origin))
	f.removeSubmoduleGitlink("vendor/lib")
	f.copyFile(origin, copied)
	f.git("add", copied)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: copied, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(copied), []span{vanish}),
	}

	f.run().assertMatches(t, "gitlink_naming_a_blob", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestACleanFilterInTheRepositorysOwnConfigDoesNotHideTheChange(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// The repo-local scope, which is where git-lfs and git-crypt install
	// themselves, and which neither the environment scrub nor the pinned config
	// files reach. The driver strips exactly what the edit below writes in, so
	// git sees the two sides of the change as equal, prints no hunk at all, and
	// the gate passes green having measured nothing.
	f.configureCleanFilter()
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

func TestAProcessFilterInTheRepositorysOwnConfigIsNeverLaunched(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// `filter.<name>.process` is the long-running protocol git prefers over
	// `.clean` when both are set, and the half git-lfs actually installs. It has
	// to be blanked beside `.clean`, or a repository that installs only this half
	// still decides what the gate is allowed to see.
	f.configureProcessFilter()
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

func TestARequiredCleanFilterInTheRepositorysOwnConfigStillMeasuresTheChange(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// `git lfs install --local` and git-crypt both mark their driver required.
	// Blanking the driver and leaving the flag makes git abort the diff rather
	// than pass the content through, so the gate would exit 1 on every run in
	// the deployment it claims to support.
	f.configureRequiredCleanFilter()
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

func TestAFilterNamedWithATrailingSpaceDoesNotBreakTheRun(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// The gate reads the repository's filter keys and blanks each one. Split on
	// whitespace, this name yields the fragment `.clean`, git refuses the
	// resulting `-c`, and the run dies before it can diff anything.
	f.configureFilterNamedWithATrailingSpace()
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

func TestAFilterNamedWithAnEqualsIsStillBlanked(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// The subsection name is the repository's to choose and an attribute value
	// can select one holding an `=`. Delivered as `-c filter.ev=il.clean=`, git
	// splits on the first `=`, sets `filter.ev`, and the real driver empties the
	// diff into a green run that measured nothing.
	f.configureFilterNamedWithAnEquals()
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

func TestAnFsmonitorHookInTheRepositorysOwnConfigIsNeverRun(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// The gate turns git's ownership check off with `-c safe.directory=*`, which
	// it can only afford if no program the repository names gets executed.
	// core.fsmonitor is one git runs to refresh the index on every diff.
	marker := f.configureFsmonitorHook()
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("the repository's core.fsmonitor hook ran: stat %s gave %v, want it never created", marker, err)
	}
}

func TestADiffGitRefusesToPrintIsATypedDocumentNotAnEmptyStdout(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	// Base resolution reads commits and still succeeds, so the document exists
	// and ADR 0005 requires it on stdout. The diff itself reads the old side's
	// blob, which is gone, so git exits 128 before printing a patch.
	blob := f.git("rev-parse", "HEAD:"+orderService)
	f.touchLine(orderService, 62)
	f.removeLooseObject(blob)

	result := f.run()

	result.assertMatchesWith(t, "diff_unreadable", 1, f.baseLabel("main"),
		"could not read the diff: fatal: unable to read "+blob+"\n",
		map[string]string{"BLOB": blob})
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

	// What this case pins is the repo-local half, the four settings above, each
	// of which configOverrides and diffFlags outrank. The two environment
	// payloads ride along rather than carrying the case: `--no-ext-diff` refuses
	// GIT_EXTERNAL_DIFF on its own, and a `-c filter.hide.clean=` outranks the
	// same key arriving through GIT_CONFIG_PARAMETERS. The environment payload no
	// flag can answer is a repository redirect, and
	// TestGitDirPointedAtAnotherRepositoryDoesNotHideTheChange is what pins the
	// scrub with one.
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

	// The filter reverses the edit and leaves the two sides equal, and the
	// GIT_CONFIG_COUNT family is the other way an ambient environment installs
	// one. Two answers hold it, the scrub that drops the variables and the
	// blanking `-c` that outranks the key wherever it came from, so this case
	// goes red only if both are gone.
	result := f.runWithEnv(cleanFilterEnv(t)...)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestGlobalGitConfigTheRunCannotParseDoesNotStopTheMeasurement(t *testing.T) {
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

	// Dropping git's namespace from the environment is not enough on its own:
	// with GIT_CONFIG_GLOBAL gone, git reads ~/.gitconfig instead. This one it
	// cannot read at all, which no `-c` override and no driver blanking can
	// answer, so the run reaches a diff only by never opening the file.
	result := f.runWithEnv(unparseableGlobalConfigHome(t)...)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestChangedMethodInADirectoryWhoseNameHoldsASpaceIsStillMeasured(t *testing.T) {
	const spaced = "My Project/A.cs"
	vanish := span{File: spaced, Name: "A.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	// git appends a TAB to the name on a "+++" line whenever the path holds a
	// space. Read literally the header names "My Project/A.cs\t", whose extension
	// is not .cs, so the language table locates no extractor for it and every
	// changed method in the file goes unscored under a pass.
	f.write(spaced, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(spaced, 7)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: spaced, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(spaced), []span{vanish}),
	}

	f.run().assertMatches(t, "spaced_diff_path", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestChangedMethodInAQuotedPathHoldingASpaceIsStillMeasured(t *testing.T) {
	const awkward = `src/a "b" c.cs`
	vanish := span{File: awkward, Name: "Awkward.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	// Both at once, which is the worse half of the same defect: the double quote
	// makes git quote the name and the space makes it append a TAB after the
	// closing quote, so unquoting the field whole fails and the run would exit 1
	// with nothing on stdout at all.
	f.write(awkward, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(awkward, 7)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: awkward, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(awkward), []span{vanish}),
	}

	f.run().assertMatches(t, "quoted_spaced_diff_path", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
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

// The cases below are issue 16: the three exit-1 diagnostics ADR 0004's
// 2026-09-03 amendment defers there, plus regression coverage for the
// resolution rules the tracer already satisfied before this issue landed.

func TestNestedSolutionLayoutScoresCorrectly(t *testing.T) {
	const nested = "src/Services/Ordering/Api/OrderService.cs"
	place := span{File: nested, Name: "OrderService.PlaceAsync", StartLine: 41, EndLine: 58, Complexity: 9}
	cancelNested := span{File: nested, Name: "OrderService.Cancel", StartLine: 60, EndLine: 64, Complexity: 3}

	f := newFixture(t, "main")
	f.write(nested, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(nested, 45)
	f.touchLine(nested, 62)
	// <source> is the repo root itself, several directories above the class,
	// which is the shape a nested solution's coverlet run actually produces.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: nested, lines: append(spanCoverage(42, 10, 10), spanCoverage(61, 3, 3)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(nested), []span{place, cancelNested}),
	}

	f.run().assertMatches(t, "nested_solution_layout", 0, f.baseLabel("main"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 9.00\n")
}

func TestReportBuiltInAnotherCheckoutFailsNamingTheMismatch(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	// A second checkout of the same source tree, entirely outside this
	// fixture's repo root. Coverlet's <source> names it faithfully; nothing
	// about the report is malformed, it was just measured against a different
	// working tree than the one being gated.
	otherCheckout := t.TempDir()
	classPath := filepath.Join(otherCheckout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	f.write("TestResults/coverage.cobertura.xml", cobertura(otherCheckout,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := resolvedPath(t, classPath)
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		fmt.Sprintf("coverage report TestResults/coverage.cobertura.xml resolved no class inside the repo root; "+
			"example resolved path %s, repo root %s\n", example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestClassWithNoFilenameDoesNotSuppressTheOutsideRepoDiagnostic(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	// The same other-checkout report, plus a malformed class carrying no
	// filename and a second <source> inside the repo root. Joining nothing
	// onto that source names src/ itself, a directory that resolves inside the
	// root, so an empty filename has to contribute no evidence rather than
	// stand in for a class that resolved. The real class resolves under
	// neither src/ nor anything else in this repo.
	otherCheckout := t.TempDir()
	classPath := filepath.Join(otherCheckout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(
		[]string{otherCheckout, filepath.Join(f.root, "src")},
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)},
		coverageClass{filename: "", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := resolvedPath(t, classPath)
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		fmt.Sprintf("coverage report TestResults/coverage.cobertura.xml resolved no class inside the repo root; "+
			"example resolved path %s, repo root %s\n", example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestDeterministicReportEmptyingSourcesFailsNamingTheProperty(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// DeterministicReport=true rewrites every filename under the /_/
	// placeholder and empties <sources>, so there is no root left to join
	// against.
	f.write("TestResults/coverage.cobertura.xml", coberturaNoSources(
		coverageClass{filename: "/_/src/Ordering/OrderService.cs", lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_source_root_erased_deterministic", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries no source root, erased by "+
			"DeterministicReport=true; collect coverage with DeterministicReport=false\n")
}

func TestUseSourceLinkFailsNamingTheProperty(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// UseSourceLink=true emits one empty <source> and the raw source-link
	// document key, a URL, as the filename.
	f.write("TestResults/coverage.cobertura.xml", cobertura("",
		coverageClass{
			filename: "https://raw.githubusercontent.com/org/repo/deadbeef/src/Ordering/OrderService.cs",
			lines:    spanCoverage(61, 3, 2),
		}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_source_root_erased_sourcelink", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries a source link document key rather than a "+
			"path, erased by UseSourceLink=true; collect coverage with UseSourceLink=false\n")
}

func TestUseSourceLinkWithASchemelessDocumentKeyFailsNamingTheProperty(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// The document key need not be a URL, and this one reads as an ordinary
	// repo-relative path. The empty <source> is the half of the shape that
	// still gives it away, and without reading it the run would degrade to the
	// wall of unknown methods the diagnostic exists to replace.
	f.write("TestResults/coverage.cobertura.xml", cobertura("",
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_source_root_erased_sourcelink", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries a source link document key rather than a "+
			"path, erased by UseSourceLink=true; collect coverage with UseSourceLink=false\n")
}

func TestReportCarryingBothErasedShapesNamesDeterministicReport(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// Both erased shapes in one report, the source-link key first. The
	// property named is DeterministicReport whatever order the classes are in,
	// because it is the one that erased <sources> for the whole document.
	f.write("TestResults/coverage.cobertura.xml", coberturaNoSources(
		coverageClass{
			filename: "https://raw.githubusercontent.com/org/repo/deadbeef/src/Ordering/OrderService.cs",
			lines:    spanCoverage(61, 3, 2),
		},
		coverageClass{filename: "/_/src/Ordering/Other.cs", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_source_root_erased_deterministic", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries no source root, erased by "+
			"DeterministicReport=true; collect coverage with DeterministicReport=false\n")
}

func TestCaseOnlyPathDifferenceIsRefusedRatherThanGuessed(t *testing.T) {
	const other = "src/Ordering/Other.cs"
	vanish := span{File: other, Name: "Other.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(other, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(other, 7)
	// The wrong-case candidate resolves on a case-insensitive filesystem and
	// does not on a case-sensitive one, but neither spelling equals
	// OrderService.cs's own case, so Cancel is unknown either way. Other.cs is
	// correctly cased so the report is not empty of in-root classes on Linux.
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: "src/Ordering/orderservice.cs", lines: spanCoverage(61, 3, 2)},
		coverageClass{filename: other, lines: spanCoverage(6, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService, other), []span{cancel, vanish}),
	}

	f.run().assertMatches(t, "case_only_path_difference", 1, f.baseLabel("main"),
		"1 changed method could not be attributed to a coverage report\n"+
			"0 of 2 changed methods over CRAP threshold 30, worst score 10.75\n")
}

func TestClassYieldingTwoCandidatesInsideRepoRootFailsNamingBoth(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	// Order.cs exists twice inside the repo root, under the two <source>
	// directories the report lists for the one class.
	f.write("src/a/Order.cs", csharpFile(20))
	f.write("src/b/Order.cs", csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// The sources are listed b before a, so the message's sorted order is not
	// the order the candidates arrive in.
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(
		[]string{filepath.Join(f.root, "src", "b"), filepath.Join(f.root, "src", "a")},
		coverageClass{filename: "Order.cs", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	// Both source paths are already repo-relative, so unlike the
	// coverage_outside_repo case the message needs no machine-specific hole.
	f.run().assertMatches(t, "file_ambiguous", 1, f.baseLabel("main"),
		"class Order.cs in coverage report TestResults/coverage.cobertura.xml resolved to more than one path "+
			"inside the repo root, src/a/Order.cs and src/b/Order.cs\n")
}

func TestReportPathOutsideRepoIsIgnoredInSilence(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	// A leftover class describing a file this repo has never had, sourced from
	// a tree entirely outside the root. The gate ignores it in silence rather
	// than tripping the zero-classes-inside-root diagnostic, because Cancel
	// already resolved inside the root from the report's other source.
	otherCheckout := t.TempDir()
	writeAbsolute(t, filepath.Join(otherCheckout, "obsolete", "Old.cs"), csharpFile(5))
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(
		[]string{f.root, otherCheckout},
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)},
		coverageClass{filename: "obsolete/Old.cs", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestReportPathNoLongerOnDiskIsIgnoredRatherThanFatal(t *testing.T) {
	const deletedFile = "src/Ordering/Deleted.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(deletedFile, csharpFile(10))
	f.commitAll("initial")
	// A report describes a moment in the past (ADR 0004): a class naming a
	// file removed since the test run must stay a silent ignore now that the
	// report is checked for more than the join alone.
	f.git("rm", "--quiet", deletedFile)
	f.commitAll("delete the file the coverage report still names")
	f.touchLine(orderService, 62)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)},
		coverageClass{filename: deletedFile, lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestReportWhoseEveryClassIsGoneFromDiskIsIgnoredRatherThanFatal(t *testing.T) {
	const deletedFile = "src/Ordering/Deleted.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(deletedFile, csharpFile(10))
	f.commitAll("initial")
	// Every class the report names is gone, which is the shape a report left
	// behind in a gitignored TestResults/ takes after a branch switch. Nothing
	// resolved outside the root, so there is no evidence of another checkout
	// and the run falls through to unknown_changed_method rather than
	// coverage_outside_repo.
	f.git("rm", "--quiet", deletedFile)
	f.commitAll("delete the only file the coverage report names")
	f.touchLine(orderService, 62)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: deletedFile, lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{cancel}),
	}

	f.run().assertMatches(t, "report_classes_all_deleted", 1, f.baseLabel("main"),
		"1 changed method could not be attributed to a coverage report\n"+
			"0 of 1 changed methods over CRAP threshold 30, worst score 0.00\n")
}

func TestReportListingTheSameSourceTwiceIsNotAmbiguous(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// Both <source> elements yield the same candidate for the one class, so
	// the class resolves to one path and file_ambiguous must not fire.
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(
		[]string{f.root, f.root},
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestAbsoluteClassFilenameWithNoSourcesScores(t *testing.T) {
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	// Coverlet emits an absolute filename and no source root when no computed
	// source root prefixes the document (ADR 0004). The filename alone is the
	// candidate, and it lands inside the root.
	f.write("TestResults/coverage.cobertura.xml", coberturaNoSources(
		coverageClass{
			filename: filepath.Join(f.root, filepath.FromSlash(orderService)),
			lines:    spanCoverage(61, 3, 2),
		}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestRunOutsideAGitRepoWritesNoDocumentAndExitsOne(t *testing.T) {
	f := &fixture{t: t, root: t.TempDir()}
	if inRepo(t, f.root) {
		t.Skip("the temp directory is itself inside a git repo")
	}

	result := f.run()

	// This failure is upstream of the document, so ADR 0005's one-TOON-document
	// rule cannot apply: there is no base and no scope to report. git's own
	// explanation is in git's own language, but the failing argv is not, and
	// gitError.Error carries it, so naming the invocation that failed keeps the
	// case specific without pinning it to English. A panic or an unrelated
	// wrapped error would satisfy "exit 1 with something on stderr" and must not
	// satisfy this.
	if result.exitCode != 1 || result.stdout != "" || !strings.Contains(result.stderr, "rev-parse") {
		t.Errorf("gate outside a repo: got exit %d, stdout %q, stderr %q; want exit 1, empty stdout, a failed rev-parse on stderr",
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
