package gate_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

// retry is a third span in orderService, complexity 4, which at no coverage
// scores exactly 20. That sits under the default bar of 30 and over a bar of
// 12, which is what lets the --threshold pair below move the verdict without
// moving the score (issue 19).
var retry = span{File: orderService, Name: "OrderService.Retry", StartLine: 66, EndLine: 72, Complexity: 4}

// orderFile is a second source file, for a case that needs two changed files
// at once, with one canned span of its own.
const orderFile = "src/Ordering/Order.cs"

var orderTotal = span{File: orderFile, Name: "Order.Total", StartLine: 60, EndLine: 64, Complexity: 3}

// pricingFile is a third source file, sorting after both of the others, for
// the case that needs the tie-break winner to sit between two losers.
const pricingFile = "src/Pricing/Pricing.cs"

var pricingQuote = span{File: pricingFile, Name: "Pricing.Quote", StartLine: 60, EndLine: 64, Complexity: 3}

func TestDocsOnlyChangePassesWithNoCoverageReportPresent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write("docs/notes.md", "first\n")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.write("docs/notes.md", "first\nsecond\n")

	f.runIn(gateOnlyDir(t)).assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestFileMovedWithNoContentChangeReportsNoChangedMethods(t *testing.T) {
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

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestFileMovedAndReindentedReportsNoChangedMethods(t *testing.T) {
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, indented(csharpFile(20), "    "))
	f.commitAll("initial")
	f.git("mv", origin, moved)
	f.write(moved, indented(csharpFile(20), "\t\t"))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestTwoIdenticalFilesMovedTogetherReportNoChangedMethods(t *testing.T) {
	t.Parallel()
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
	f.git("mv", firstOrigin, firstMoved)
	f.git("mv", secondOrigin, secondMoved)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(firstMoved, secondMoved), []span{firstVanish, secondVanish}),
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestAnAddNobodyCanReadLeavesEveryOtherAddMeasured(t *testing.T) {
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
	f.git("mv", origin, moved)
	f.addUnreadable("main", "notes.md")
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: moved, lines: spanCoverage(5, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.run().assertMatches(t, "unreadable_add_leaves_sibling_measured", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 4.00\n")
}

func TestAnAddNobodyCanReadStillLeavesTheMovesTheDeletesExplainDropped(t *testing.T) {
	t.Parallel()
	const firstOrigin = "src/Ordering/First.cs"
	const secondOrigin = "src/Ordering/Second.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(firstOrigin, csharpFile(20))
	f.write(secondOrigin, csharpFile(20))
	f.commitAll("initial")
	f.git("mv", firstOrigin, moved)
	f.git("rm", secondOrigin)
	f.addUnreadable("main", "notes.md")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(moved), []span{vanish}),
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestMethodScoringExactlyAtTheThresholdPasses(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	boundaryFixture(t, f, 30)

	f.run().assertMatches(t, "threshold_boundary_pass", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 30.00\n")
}

func TestMethodScoringJustOverTheThresholdFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	boundaryFixture(t, f, 29)

	f.run().assertMatches(t, "threshold_boundary_fail", 2, f.baseLabel("main"),
		"1 of 1 changed methods over CRAP threshold 30, worst score 30.03\n")
}

func TestMethodWhoseRawScoreRoundsDownOntoTheThresholdPasses(t *testing.T) {
	t.Parallel()
	const boundary = "src/Ordering/Boundary.cs"
	knot := span{File: boundary, Name: "Boundary.Knot", StartLine: 10, EndLine: 80, Complexity: 30}

	f := newFixture(t, "main")
	f.write(boundary, csharpFile(100))
	f.commitAll("initial")
	f.touchLine(boundary, 20)
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
	t.Parallel()
	const pair = "src/Ordering/Pair.cs"
	a := span{File: pair, Name: "Pair.A", Signature: "(string)", StartLine: 14, EndLine: 14, Complexity: 1}
	b := span{File: pair, Name: "Pair.B", Signature: "(int)", StartLine: 14, EndLine: 14, Complexity: 1}

	f := newFixture(t, "main")
	f.write(pair, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(pair, 14)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: pair, lines: spanCoverage(14, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(pair), []span{b, a}),
	}

	f.run().assertMatches(t, "same_line_equal_scores", 0, f.baseLabel("main"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 1.00\n")
}

func TestExtractorReportingADifferentLanguageThanItWasLocatedUnderFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Language: "fsharp", Extensions: []string{".cs"}}

	f.run().assertMatches(t, "capabilities_language_mismatch", 1, f.baseLabel("main"),
		"csharp extractor reported language fsharp\n")
}

func TestExtractorClaimingNoneOfTheFilesItWasLocatedForFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".fs"}}

	f.run().assertMatches(t, "capabilities_claims_nothing", 1, f.baseLabel("main"),
		"csharp extractor claims none of the changed files the language table located it for\n")
}

func TestExtractorReturningTheSameSpanTwiceFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, placeAsync}),
	}

	f.run().assertMatches(t, "duplicate_span", 1, f.baseLabel("main"),
		"csharp extractor returned OrderService.PlaceAsync twice, covering src/Ordering/OrderService.cs lines 41-58\n")
}

func TestExtractorReturningTheSameSpanWithTwoComplexitiesFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
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
	t.Parallel()
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
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
	t.Parallel()
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
	t.Parallel()
	const twins = "src/Ordering/Twins.cs"
	first := span{File: twins, Name: "Twins.A", StartLine: 14, EndLine: 14, Complexity: 1}
	second := span{File: twins, Name: "Twins.B", StartLine: 14, EndLine: 14, Complexity: 2}

	f := newFixture(t, "main")
	f.write(twins, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(twins, 14)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	f.runIn(gateOnlyDir(t)).assertMatches(t, "extractor_missing", 1, f.baseLabel("main"),
		"csharp extractor not found: metric-gate-csharp\n")
}

func TestExtractorThatIsPresentButNotExecutableFails(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}, Stdout: "not json"}

	f.run().assertMatches(t, "extractor_invalid_json", 1, f.baseLabel("main"),
		"csharp extractor emitted invalid JSON\n")
}

func TestExtractorEmittingCapabilitiesThatAreNotJSONFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}, CapabilitiesStdout: "not json"}

	f.run().assertMatches(t, "extractor_invalid_capabilities_json", 1, f.baseLabel("main"),
		"csharp extractor emitted invalid capabilities JSON\n")
}

func TestExtractorSilentAboutAFileItWasGivenFails(t *testing.T) {
	t.Parallel()
	const ghost = "src/Ordering/Ghost.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(ghost, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(ghost, 7)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "extractor_no_parse_status", 1, f.baseLabel("main"),
		"csharp extractor reported no parse status for src/Ordering/Ghost.cs\n")
}

func TestCoverageWalkRecordsAPathItCouldNotRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.denyRead("TestResults/locked")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "skipped_unreadable_path", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestWellTestedMethodForItsComplexityPasses(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	cancelFixture(t, f)

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

// TestSelectingTheOnlyHostedMetricAtItsDefaultBarChangesNothing pins that both
// flags are no-ops when they name what the run would have done anyway. In a
// binary hosting one metric, "defaults to every metric the binary hosts" and
// "defaults to crap" are the same run, and naming the default bar is the bar
// that was already in force, so both documents are pass_single_method byte for
// byte.
func TestSelectingTheOnlyHostedMetricAtItsDefaultBarChangesNothing(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"naming the only hosted metric": {"--metric", "crap"},
		"naming the default bar":        {"--threshold", "crap=30"},
	}

	for name, argv := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, "main")
			cancelFixture(t, f)

			f.runWithArgs(argv...).assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
				"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
		})
	}
}

func TestMethodWithTooLittleCoverageForItsComplexityFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(42, 10, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "fail_single_method", 2, f.baseLabel("main"),
		"1 of 1 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestChangedMethodUnderTheDefaultThresholdPasses(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	retryFixture(t, f)

	f.run().assertMatches(t, "threshold_default_passes", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 20.00\n")
}

func TestTheSameMethodFailsOnceTheThresholdIsLoweredUnderItsScore(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	retryFixture(t, f)

	f.runWithArgs("--threshold", "crap=12").assertMatches(t, "threshold_lowered_fails", 2, f.baseLabel("main"),
		"1 of 1 changed methods over CRAP threshold 12, worst score 20.00\n")
}

func TestThresholdBelowAMethodsComplexityAsksForASplitRatherThanCoverage(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(42, 10, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--threshold", "crap=8").assertMatches(t, "threshold_below_complexity_splits", 2, f.baseLabel("main"),
		"1 of 1 changed methods over CRAP threshold 8, worst score 68.05\n")
}

func TestEmptyChangedSetStillReportsTheThresholdItWasGiven(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.stub = stubConfig{Extensions: []string{".cs"}}
	f.write("docs/notes.md", "first\n")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.write("docs/notes.md", "first\nsecond\n")

	f.runWithArgs("--threshold", "crap=12").assertMatches(t, "empty_changed_set_threshold", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestFourChangedMethodsEachGetTheFixThatApplies(t *testing.T) {
	t.Parallel()
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
	f.touchLine(order, 25)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: pricing, lines: spanCoverage(19, 20, 11)},
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)},
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
	f.git("checkout", "--quiet", "feature")

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestWholeFileReformatWithNoLogicChangeMeasuresNothing(t *testing.T) {
	t.Parallel()
	const legacy = "src/Ordering/Legacy.cs"
	knot := span{File: legacy, Name: "Legacy.Knot", StartLine: 10, EndLine: 40, Complexity: 34}
	tangle := span{File: legacy, Name: "Legacy.Tangle", StartLine: 42, EndLine: 58, Complexity: 20}

	f := newFixture(t, "main")
	f.write(legacy, csharpFile(60))
	f.commitAll("initial")
	f.write(legacy, indented(csharpFile(60), "        "))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(legacy), []span{knot, tangle}),
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")
}

func TestMethodMovedToANewPathAndEditedIsScoredAtItsNewLocation(t *testing.T) {
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	vanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
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
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const destination = "src/Ordering/Destination.cs"
	originKeep := span{File: origin, Name: "Origin.Keep", StartLine: 20, EndLine: 30, Complexity: 2}
	destinationUntouched := span{File: destination, Name: "Destination.Untouched", StartLine: 5, EndLine: 15, Complexity: 12}
	destinationVanish := span{File: destination, Name: "Destination.Vanish", StartLine: 30, EndLine: 40, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(60))
	f.write(destination, csharpFile(60))
	f.commitAll("initial")
	f.moveLinesBetween(origin, 10, 20, destination, 29)
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
	t.Parallel()
	const shuffle = "src/Ordering/Shuffle.cs"
	keep := span{File: shuffle, Name: "Shuffle.Keep", StartLine: 20, EndLine: 30, Complexity: 2}
	vanish := span{File: shuffle, Name: "Shuffle.Vanish", StartLine: 35, EndLine: 45, Complexity: 4}

	f := newFixture(t, "main")
	f.write(shuffle, csharpFile(60))
	f.commitAll("initial")
	f.moveLinesWithin(shuffle, 10, 20, 45)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: shuffle, lines: spanCoverage(36, 4, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(shuffle), []span{keep, vanish}),
	}

	f.run().assertMatches(t, "moved_within_file", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.75\n")
}

// TestASourceFileReplacedByASymlinkContributesNoChangedMethods pins
// --diff-filter=ACM against status T. `--no-renames` is already in the argv,
// so a rename can never surface as status R, and T is the only other letter
// ACM excludes that a live case can still produce: widening the letter set to
// ACMD, or any other combination of A/C/M/D, still excludes T, so what this
// case actually pins is the filter's presence, not its exact letters. A
// source file that becomes a symlink genuinely should not be measured, which
// is why the flag stays pinned whole rather than narrowed to only the letters
// this suite happens to exercise, the alternative issue 25 considered and
// rejected. That reaches only the typechange. A `.cs` symlink added outright
// arrives as status A and passes ACM, so the letters admit it and a mode drop
// ahead of extraction is what keeps it out, which is the claim
// TestASymlinkAddedOutrightIsWithheldFromTheChangedSet pins.
// TestASymlinkReplacedByASourceFileContributesNoChangedMethods
// pins the opposite direction of the same typechange.
func TestASourceFileReplacedByASymlinkContributesNoChangedMethods(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderFile, csharpFile(80))
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.removeFile(orderFile)
	f.symlinkTo(filepath.Base(orderService), orderFile)
	f.assertTypechange("main", orderFile, realFile, symlink)
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderFile), []span{orderTotal}),
		StdinLog:   handed,
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")

	assertNotHandedToExtractor(t, handed)
}

// TestATypechangeIsWithheldWhileTheRestOfTheDiffIsMeasured is the positive
// control the two cases either side of it cannot be. Both of those assert an
// absence against the empty_changed_set golden, which is also what a run that
// extracted nothing at all produces, so a change that skipped extraction for
// an unrelated reason would leave them green for the wrong reason. Here the
// same typechange sits beside an ordinary edit, so the run measures something
// real and the document is non-empty, and the drop becomes a live extractor
// withholding one path rather than an extractor that never ran. The symlink
// points at OrderService.cs, so an extractor handed Order.cs would follow it
// and report OrderService.Cancel a second time under Order.cs.
func TestATypechangeIsWithheldWhileTheRestOfTheDiffIsMeasured(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderFile, csharpFile(80))
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.removeFile(orderFile)
	f.symlinkTo(filepath.Base(orderService), orderFile)
	f.touchLine(orderService, 62)
	f.assertTypechange("main", orderFile, realFile, symlink)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
		StdinLog:   handed,
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")

	assertHandedToExtractor(t, handed, orderService+"\n")
}

// TestASymlinkReplacedByASourceFileContributesNoChangedMethods pins the
// mirror of the case above: the other direction of status T, a committed
// `.cs` symlink replaced in the working tree by a real source file full of
// methods. The gate drops it, same as the other direction, and this case
// exists so that drop stays a decision rather than an accident. The blind
// spot it accepts is not one commit wide. Under ADR 0014 a changed method is
// a working-tree span holding a touched line, so a later status M edit to the
// now-real file measures only the methods holding the edited lines, and every
// other method in the file stays unmeasured until something touches it. The
// file never arrives as status A, which is the only route that measures all
// of it, so the gap persists.
//
// Widening the filter to ACMT is not the fix, verified against real git.
// With T included, git renders a typechange as a delete plus an add pair, so
// this direction's new side is the whole real source file, every line of it
// claimed under Order.cs, while the case above gets a new side holding the
// link's own text, the path it points at, as one line under an equally
// claimed `.cs` path. The second of those is what makes the flag edit wrong
// on its own: an extractor handed a link path follows it and reports the
// target file's spans under the link's path, which is spans under a path that
// does not hold them, worse than methods that go unmeasured until an edit
// reaches them.
//
// The mode drop TestASymlinkAddedOutrightIsWithheldFromTheChangedSet pins is
// half of what measuring this direction needs, since it already withholds a
// new side that is a link. The other half is a letter set admitting a
// typechange at all, and the two have to land together, which is issue 84.
// Until then the gap above is accepted rather than closed.
func TestASymlinkReplacedByASourceFileContributesNoChangedMethods(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.symlinkTo(filepath.Base(orderService), orderFile)
	f.commitAll("initial")
	f.removeFile(orderFile)
	f.write(orderFile, csharpFile(80))
	f.assertTypechange("main", orderFile, symlink, realFile)
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderFile), []span{orderTotal}),
		StdinLog:   handed,
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")

	assertNotHandedToExtractor(t, handed)
}

// TestASymlinkAddedOutrightIsWithheldFromTheChangedSet pins the route the
// filter letters cannot reach. A `.cs` symlink added outright arrives as
// status A and passes --diff-filter=ACM, so nothing about the letter set
// keeps it out; the gate drops it by mode instead, and this case is what
// holds that drop in place.
//
// The arrangement is the one issue 109 reported as a live false failure, so
// the stub reports a span covering line 1, the link's only line. Handed the
// link, an extractor follows it and reports OrderService.cs's spans under
// Order.cs, the coverage lookup against that path finds nothing, and the run
// exits 1 on an unknown changed method with no edit that clears it. Dropping
// the link ahead of extraction is what makes that unreachable, and a
// regression reds here as exit 1 rather than as a changed cell.
//
// The two typechange direction cases above used to rest on the opposite
// claim, that an added link reaches the extractor, each putting it to
// different work: issue 25's narrowed-filter alternative would have reached
// the added link, and an ACMT widen hands over a link path the way status A
// once did. Both arguments survive the drop, since what they are about is
// which paths the letters admit, and the mode drop now sits behind the
// letters for either route.
//
// The symlink points at OrderService.cs, its sibling in the same directory,
// so the link is not dangling.
func TestASymlinkAddedOutrightIsWithheldFromTheChangedSet(t *testing.T) {
	t.Parallel()
	orderHeader := span{File: orderFile, Name: "Order.Header", StartLine: 1, EndLine: 4, Complexity: 3}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.symlinkTo(filepath.Base(orderService), orderFile)
	f.git("add", orderFile)
	f.assertAddedAs("main", orderFile, symlink)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderFile), []span{orderHeader}),
		StdinLog:   handed,
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")

	assertNotHandedToExtractor(t, handed)
}

// TestAnAddedSymlinkOutOfTheRepoIsWithheldRatherThanRefused pins the
// changed-set answer to the hazard
// TestFilesNamingASymlinkOutOfTheRepoFailsNamingThePathAsTyped guards on the
// --files path, which issue 103 asked about. The two paths answer it
// differently and both answers are right. A --files name is a developer
// asking for that file to be measured, so a target no commit of this repo
// holds is a refusal naming the path as typed. A path in the changed set
// arrives from the diff carrying its mode, so the link is dropped for holding
// no source, and where it points never comes up.
//
// What both answers refuse is the same: measuring a file outside the repo and
// reporting it under an in-repo path. The drop here is the mode drop the case
// above pins, not a guard of its own, so nothing in the changed-set path reads
// the target at all.
func TestAnAddedSymlinkOutOfTheRepoIsWithheldRatherThanRefused(t *testing.T) {
	t.Parallel()
	const outsideLink = "src/Ordering/Outside.cs"
	outside := filepath.Join(t.TempDir(), "outside.cs")
	writeAbsolute(t, outside, csharpFile(10))

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.symlinkTo(outside, outsideLink)
	f.git("add", outsideLink)
	f.assertAddedAs("main", outsideLink, symlink)
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout: extractorOutput(t, parsed(outsideLink),
			[]span{{File: outsideLink, Name: "Outside.Header", StartLine: 1, EndLine: 4, Complexity: 3}}),
		StdinLog: handed,
	}

	f.run().assertMatches(t, "empty_changed_set", 0, f.baseLabel("main"),
		"no changed methods, nothing to measure\n")

	assertNotHandedToExtractor(t, handed)
}

// TestAnAddedSymlinkIsWithheldWhileTheRestOfTheDiffIsMeasured is the positive
// control for the two cases above, the same one
// TestATypechangeIsWithheldWhileTheRestOfTheDiffIsMeasured is for the
// typechange pair. Both of those assert an absence against
// empty_changed_set, which is also what a run that extracted nothing at all
// produces, so a change that skipped extraction for an unrelated reason would
// leave them green for the wrong reason. Here the added link sits beside an
// ordinary edit, the document is non-empty, and the drop becomes a live
// extractor handed one path and not the other.
func TestAnAddedSymlinkIsWithheldWhileTheRestOfTheDiffIsMeasured(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.symlinkTo(filepath.Base(orderService), orderFile)
	f.git("add", orderFile)
	f.touchLine(orderService, 62)
	f.assertAddedAs("main", orderFile, symlink)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
		StdinLog:   handed,
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")

	assertHandedToExtractor(t, handed, orderService+"\n")
}

// TestDeletingAMethodAttributesTheZeroLengthHunkToTheLineBeforeIt pins which
// line a pure deletion touches. Before and Boundary sit either side of the
// insertion point, so the two candidate answers produce different documents.
// Move the touch to line 8 and the table names Legacy.Before over 5-8 at
// coverage 1 and score 3 instead, and the golden stops matching.
func TestDeletingAMethodAttributesTheZeroLengthHunkToTheLineBeforeIt(t *testing.T) {
	t.Parallel()
	const legacy = "src/Ordering/Legacy.cs"
	before := span{File: legacy, Name: "Legacy.Before", StartLine: 5, EndLine: 8, Complexity: 3}
	boundary := span{File: legacy, Name: "Legacy.Boundary", StartLine: 9, EndLine: 9, Complexity: 3}
	survivor := span{File: legacy, Name: "Legacy.Survivor", StartLine: 15, EndLine: 25, Complexity: 34}

	f := newFixture(t, "main")
	f.write(legacy, csharpFile(60))
	f.commitAll("initial")
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
	t.Parallel()
	const fresh = "src/Ordering/Fresh.cs"
	first := span{File: fresh, Name: "Fresh.First", StartLine: 3, EndLine: 8, Complexity: 4}
	second := span{File: fresh, Name: "Fresh.Second", StartLine: 10, EndLine: 16, Complexity: 9}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.write(fresh, csharpFile(20))
	f.git("add", fresh)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: fresh, lines: append(spanCoverage(4, 4, 1), spanCoverage(11, 5, 1)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(fresh), []span{first, second}),
	}

	f.run().assertMatches(t, "new_file", 2, f.baseLabel("main"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 50.47\n")
}

// TestANewFileNeverAddedToTheIndexContributesNoChangedMethods is the other
// half of the pair above: Scratch.cs is written but never `git add`ed, so
// ADR 0014's tracked-paths-only rule says it contributes nothing, and the
// changed set is OrderService.cs alone.
//
// The stub's stdout is canned and stays silent about Scratch.cs on purpose.
// Issue 26 asks for an over-threshold span on the untracked file, but
// extract.collect (gate/internal/extract/extract.go) exits 1 with
// extractor_path_mismatch on a span for a path the gate never handed out.
// Under correct behaviour the untracked file is never handed to the
// extractor, so a canned span for it would red this case for the wrong
// reason: union untracked files into the scope instead, and the gate hands
// Scratch.cs to the extractor, which reports no parse status for a file the
// stub was never told to claim, and the run exits 1 with an error document in
// place of this golden. The StdinLog assertion below states the same rule
// directly, by naming exactly what the extractor was handed.
func TestANewFileNeverAddedToTheIndexContributesNoChangedMethods(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	f.write("src/Ordering/Scratch.cs", csharpFile(20))
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(42, 10, 8)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}
	handed := filepath.Join(t.TempDir(), "handed")
	f.stub.StdinLog = handed

	f.run().assertMatches(t, "untracked_new_file", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 9.65\n")

	assertHandedToExtractor(t, handed, orderService+"\n")
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
	t.Parallel()
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
	f.moveLinesBetween(origin, 10, 20, destination, 29)
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
	t.Parallel()
	const outer = "src/Ordering/Outer.cs"
	container := span{File: outer, Name: "Outer.Run", StartLine: 10, EndLine: 60, Complexity: 5}
	local := span{File: outer, Name: "Outer.Run.Local", StartLine: 30, EndLine: 40, Complexity: 2}

	f := newFixture(t, "main")
	f.write(outer, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(outer, 35)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: outer, lines: append(spanCoverage(12, 4, 4), spanCoverage(31, 4, 1)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(outer), []span{container, local}),
	}

	f.run().assertMatches(t, "nested_spans", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.69\n")
}

func TestChangedMethodWithNoCoverageReportAnywhereFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_missing", 1, f.baseLabel("main"),
		"CRAP requires a coverage report, none found matching **/TestResults/**/coverage.cobertura.xml under the repo root\n")
}

// TestExplicitlySelectingCRAPStillDemandsTheCoverageItDeclares is the
// declares-implies-reads half of issue 19's drift test. CRAP's catalogue entry
// declares InputCoverage, so a run that selects it by name must still go
// looking for a report and must still fail when there is none. A gate that
// resolved coverage off anything other than the declaration, the old
// unconditional rule among them, would satisfy this case by accident; the
// reads-implies-declares half, in cmd/metric-gate, is what catches that.
func TestExplicitlySelectingCRAPStillDemandsTheCoverageItDeclares(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--metric", "crap").assertMatches(t, "coverage_missing", 1, f.baseLabel("main"),
		"CRAP requires a coverage report, none found matching **/TestResults/**/coverage.cobertura.xml under the repo root\n")
}

func TestRepoWithNoResolvableDiffBaseNamesEveryRefAndPointsAtTheFlag(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "trunk")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 20)
	f.commitAll("second")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}}

	f.run().assertMatches(t, "no_diff_base", 1, "",
		"no diff base: tried origin/HEAD, origin/main, origin/master, main, master; name one with --since <ref>\n")
}

func TestChangedMethodInAFileNoReportPathMatchedFails(t *testing.T) {
	t.Parallel()
	const ghost = "src/Ordering/Ghost.cs"
	vanish := span{File: ghost, Name: "Ghost.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(ghost, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(ghost, 7)
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

// TestFileInAReportCarryingNoInstrumentableLinesIsUnknownRatherThanCovered
// pins issue 17's earned structural_na: a <class> with no <line> at all is
// a report that never instrumented the file, not a file of trivial members,
// so its changed method comes back unknown with the typed reason rather
// than structural_na treated as fully covered.
func TestFileInAReportCarryingNoInstrumentableLinesIsUnknownRatherThanCovered(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "file_uninstrumented", 1, f.baseLabel("main"),
		"1 changed method could not be attributed to a coverage report\n"+
			"0 of 1 changed methods over CRAP threshold 30, worst score 0.00\n")
}

// TestFileWhoseOnlyInstrumentedLineWasNeverHitIsMeasuredAtZero pins the
// boundary the file_uninstrumented arm depends on. The file's entire coverage
// entry is a single hits="0" line, so entry non-emptiness rests on an
// uncovered line alone, which is what keeps the span off the
// file_uninstrumented arm and reads it measured at coverage 0. Span length is
// not this case's value: deleted_method already pins a one-line span at
// coverage 0. A change that turned this unknown, or read it as structural_na
// treated as fully covered, would fail here.
func TestFileWhoseOnlyInstrumentedLineWasNeverHitIsMeasuredAtZero(t *testing.T) {
	t.Parallel()
	const money = "src/Ordering/Money.cs"
	amount := span{File: money, Name: "Money.Amount", StartLine: 7, EndLine: 7, Complexity: 1}

	f := newFixture(t, "main")
	f.write(money, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(money, 7)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: money, lines: spanCoverage(7, 1, 0)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(money), []span{amount}),
	}

	f.run().assertMatches(t, "only_instrumented_line_never_hit", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 2.00\n")
}

// TestFileWithNoLinesInOneReportIsStillMeasuredFromAnother pins where the
// emptiness rule lives. The gate judges it in the join, over the union of
// every report it consumed, so one report listing the file with no line
// cannot make the method unknown while another report carries its coverage.
// An implementation that judged emptiness per report inside mergeInto would
// fail this case; the current one keeps it measured.
//
// The fixture directory names carry the case. Discovery sorts reports by
// path, so tests/Integration.Tests, which carries the hit lines, merges
// before tests/Unit.Tests, which is empty. The empty entry landing second is
// what makes a union that replaced or went sticky per report fail here, and
// the invariant it pins is that an empty entry contributes nothing to the
// union rather than erasing another report's lines. Renaming either directory
// so the empty report sorts first would leave the case passing while quietly
// losing that discrimination.
func TestFileWithNoLinesInOneReportIsStillMeasuredFromAnother(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Unit.Tests/TestResults/run/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService}))
	f.write("tests/Integration.Tests/TestResults/run/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "empty_entry_union", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.00\n")
}

// TestFileWithNoLinesInTheFirstReportIsStillMeasuredFromALaterOne pins the
// opposite merge direction to the case above. Here tests/Alpha.Tests, which
// is empty, sorts before tests/Zulu.Tests, which carries the hit lines, so
// the union starts from a pre-seeded empty entry that a later report has to
// upgrade with real lines. That direction is the one the file_uninstrumented
// arm makes load-bearing: an entry left empty here turns a covered method
// into exit 1.
func TestFileWithNoLinesInTheFirstReportIsStillMeasuredFromALaterOne(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService}))
	f.write("tests/Zulu.Tests/TestResults/run/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "empty_entry_union", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.00\n")
}

// TestTwoUnknownChangedMethodsAreCountedInThePlural pins two changed methods
// in one file, both under the file_uninstrumented arm, reported as two rows
// under a single plural message. The sibling mixed-reason case pins two
// unknowns of different reasons across two files; this one pins that a single
// empty entry disqualifies every span in its file rather than the first.
func TestTwoUnknownChangedMethodsAreCountedInThePlural(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 45)
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_uninstrumented_methods", 1, f.baseLabel("main"),
		"2 changed methods could not be attributed to a coverage report\n"+
			"0 of 2 changed methods over CRAP threshold 30, worst score 0.00\n")
}

// TestTwoUnknownChangedMethodsCarryTheirOwnReasons pins that one run can
// produce two different unknown reasons and that each row carries its own.
// Ghost.cs is in no report at all and comes back file_unmatched;
// OrderService.cs is listed with no instrumentable line and comes back
// file_uninstrumented, both counted by the one plural message.
func TestTwoUnknownChangedMethodsCarryTheirOwnReasons(t *testing.T) {
	t.Parallel()
	const ghost = "src/Ordering/Ghost.cs"
	vanish := span{File: ghost, Name: "Ghost.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(ghost, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(ghost, 7)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(ghost, orderService), []span{vanish, cancel}),
	}

	f.run().assertMatches(t, "mixed_unknown_reasons", 1, f.baseLabel("main"),
		"2 changed methods could not be attributed to a coverage report\n"+
			"0 of 2 changed methods over CRAP threshold 30, worst score 0.00\n")
}

func TestFileTheExtractorCouldNotParseFails(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestExtractorVolunteeringASpanInAPathItWasNotGivenFails is the span arm of
// the same refusal, and the case above is the file-status arm. The two are
// separate walks over the response, and this one is the arm the gate now leans
// on alone: nothing downstream filters spans against the file list any more, so
// an extractor that volunteers a span in a file the gate never handed in would
// otherwise have that method measured and folded into the score. Every
// file-status entry here is legitimate, so only the span walk can catch it.
func TestExtractorVolunteeringASpanInAPathItWasNotGivenFails(t *testing.T) {
	t.Parallel()
	imposter := span{File: "src/Ordering/Imposter.cs", Name: "Imposter.Run", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel, imposter}),
	}

	f.run().assertMatches(t, "extractor_path_mismatch", 1, f.baseLabel("main"),
		"csharp extractor returned a path it was not given: src/Ordering/Imposter.cs\n")
}

func TestExtractorExitingNonZeroFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{Extensions: []string{".cs"}, ExitCode: 3}

	f.run().assertMatches(t, "extractor_failed", 1, f.baseLabel("main"),
		"csharp extractor exited 3\n")
}

func TestCoverageReportThatIsNotValidXMLFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	const malformed = "<coverage><packages>\n"
	f.write("TestResults/fixed/coverage.cobertura.xml", malformed)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	cause := xmlUnmarshalCause(t, malformed)
	f.run().assertMatchesWith(t, "coverage_unparseable", 1, f.baseLabel("main"),
		"could not parse coverage report TestResults/fixed/coverage.cobertura.xml; "+cause+"\n",
		map[string]string{"CAUSE": cause})
}

func TestCoverageReportTheGateCannotReadFailsNamingTheCauseOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/fixed/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.denyReadFile("TestResults/fixed/coverage.cobertura.xml")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	cause := f.readCause("TestResults/fixed/coverage.cobertura.xml")
	f.run().assertMatchesWith(t, "coverage_unreadable", 1, f.baseLabel("main"),
		"could not parse coverage report TestResults/fixed/coverage.cobertura.xml; "+cause+"\n",
		map[string]string{"CAUSE": cause})
}

func TestMoveWithAnExtraCopyMeasuresBothAddedPaths(t *testing.T) {
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const moved = "src/Ordering/Moved.cs"
	const copied = "src/Ordering/Copy.cs"
	movedVanish := span{File: moved, Name: "Moved.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}
	copiedVanish := span{File: copied, Name: "Copy.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
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
	t.Parallel()
	const gone = "src/Ordering/Gone.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(gone, csharpFile(20))
	f.commitAll("initial")
	blob := f.git("rev-parse", "HEAD:"+gone)
	f.git("rm", "--quiet", gone)
	f.removeLooseObject(blob)
	f.touchLine(orderService, 62)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.addSubmoduleGitlink("vendor/lib")
	f.removeSubmoduleGitlink("vendor/lib")
	f.touchLine(orderService, 62)
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
	t.Parallel()
	const origin = "src/Ordering/Origin.cs"
	const copied = "src/Ordering/Copy.cs"
	vanish := span{File: copied, Name: "Copy.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(origin, csharpFile(20))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	blob := f.git("rev-parse", "HEAD:"+orderService)
	f.touchLine(orderService, 62)
	f.removeLooseObject(blob)

	result := f.run()

	result.assertMatchesWith(t, "diff_unreadable", 1, f.baseLabel("main"),
		"could not read the diff: fatal: unable to read "+blob+"\n",
		map[string]string{"BLOB": blob})
}

func TestHostileGitConfigAndEnvironmentDoNotHideTheChange(t *testing.T) {
	t.Parallel()
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

	f.git("config", "color.ui", "always")
	f.git("config", "diff.mnemonicPrefix", "true")
	f.git("config", "diff.noprefix", "true")
	f.git("config", "diff.external", externalDiffScript(t))

	result := f.runWithEnv(
		"GIT_EXTERNAL_DIFF="+externalDiffScript(t),
		cleanFilterParameters(t),
	)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestGitDirPointedAtAnotherRepositoryDoesNotHideTheChange(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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
	t.Parallel()
	const head = "src/Ordering/Head.cs"
	first := span{File: head, Name: "Head.First", StartLine: 1, EndLine: 5, Complexity: 2}

	f := newFixture(t, "main")
	f.write(head, csharpFile(20))
	f.commitAll("initial")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
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

func TestCoverageReportOutsideAResultsDirectoryIsNotFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_missing", 1, f.baseLabel("main"),
		"CRAP requires a coverage report, none found matching **/TestResults/**/coverage.cobertura.xml under the repo root\n")
}

func TestTwoOverloadsDeclaredOnOneLineAreBothScored(t *testing.T) {
	t.Parallel()
	const calc = "src/Ordering/Calc.cs"
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

	f.run().assertMatches(t, "same_line_two_overloads", 0, f.baseLabel("main"),
		"0 of 2 changed methods over CRAP threshold 30, worst score 2.00\n")
}

func TestChangedMethodInAUTF16SourceFileIsStillMeasured(t *testing.T) {
	t.Parallel()
	const wide = "src/Ordering/Wide.cs"
	vanish := span{File: wide, Name: "Wide.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
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
	t.Parallel()
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

	result := f.runWithEnv(cleanFilterEnv(t)...)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestGlobalGitConfigTheRunCannotParseDoesNotStopTheMeasurement(t *testing.T) {
	t.Parallel()
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

	result := f.runWithEnv(unparseableGlobalConfigHome(t)...)

	result.assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestChangedMethodInADirectoryWhoseNameHoldsASpaceIsStillMeasured(t *testing.T) {
	t.Parallel()
	const spaced = "My Project/A.cs"
	vanish := span{File: spaced, Name: "A.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
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
	t.Parallel()
	const awkward = `src/a "b" c.cs`
	vanish := span{File: awkward, Name: "Awkward.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
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
	t.Parallel()
	const weird = `src/Ordering/we"ird.cs`
	vanish := span{File: weird, Name: "Weird.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.denyRead("TestResults/locked")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_missing_with_skipped_path", 1, f.baseLabel("main"),
		"CRAP requires a coverage report, none found matching **/TestResults/**/coverage.cobertura.xml under the repo root\n")
}

func TestLibraryCoveredByTwoTestProjectsIsScoredOnTheUnion(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Unit.Tests/TestResults/run/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 1}, {number: 62, hits: 0}, {number: 63, hits: 0}}}))
	f.write("tests/Integration.Tests/TestResults/run/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 0}, {number: 62, hits: 1}, {number: 63, hits: 1}}}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_projects_union", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.00\n")
}

func TestCoverageReportStampedInEpochMillisecondsIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", coberturaStamped("1767225600000", f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_timestamp_ahead_of_now", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries a timestamp \"1767225600000\" more than 24h ahead of now; it must be epoch seconds, or the clock on this machine is behind the one that wrote it\n")
}

func TestCoverageReportStampedJustBeyondToleranceIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	stamp := stampAt(time.Now().Add(25 * time.Hour))
	f.write("TestResults/coverage.cobertura.xml", coberturaStamped(stamp, f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatchesWith(t, "coverage_timestamp_beyond_tolerance", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries a timestamp "+
			strconv.Quote(stamp)+" more than 24h ahead of now; it must be epoch seconds, or the clock on this machine is behind the one that wrote it\n",
		map[string]string{"STAMP": stamp})
}

func TestCoverageReportStampedInsideToleranceIsScored(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", coberturaStamped(stampAt(time.Now().Add(23*time.Hour)), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestNamedCoverageReportStampedFarAheadIsRefusedToo(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("artifacts/coverage.xml", coberturaStamped("1767225600000", f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", "artifacts/coverage.xml").assertMatches(t, "named_coverage_timestamp_ahead_of_now", 1,
		f.baseLabel("main"),
		"coverage report artifacts/coverage.xml carries a timestamp \"1767225600000\" more than 24h ahead of now; it must be epoch seconds, or the clock on this machine is behind the one that wrote it\n")
}

// timestamp="0" is the units fault the tolerance catches from the other
// side: a producer with no clock to read, or one writing seconds since boot
// rather than since the epoch. The int64 floor is the other end of the
// range guard, a stamp time.Unix holds without underflowing, so nothing
// upstream refuses it and the plausibility band is the only rule that can.
// Both are discovered reports, the one Origin issue 32 skips as superseded,
// so this pins that a stamp the gate cannot trust is refused rather than
// dropped quietly into skipped_paths. A fresh report sits beside each, the
// shape of the future-stamped case below, so a gate that skipped the
// floor-stamped one would still score the run and pass. Without that
// sibling the skip would run out of reports and refuse anyway, under a
// different code, and the case would hold whether the floor guard existed
// or not.
func TestCoverageReportStampedBeforeTheFloorIsRefusedNotSkipped(t *testing.T) {
	t.Parallel()
	for _, stamp := range []string{"0", "-9223372036854775808"} {
		t.Run(stamp, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, "main")
			f.write(orderService, csharpFile(80))
			f.commitAll("initial")
			f.touchLine(orderService, 62)
			f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml", coberturaStamped(freshStamp(), f.root,
				coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
			f.write("tests/Beta.Tests/TestResults/run/coverage.cobertura.xml", coberturaStamped(stamp, f.root,
				coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
			f.stub = stubConfig{
				Extensions: []string{".cs"},
				Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
			}

			f.run().assertMatchesWith(t, "coverage_timestamp_before_floor", 1, f.baseLabel("main"),
				"coverage report tests/Beta.Tests/TestResults/run/coverage.cobertura.xml carries a timestamp "+
					strconv.Quote(stamp)+" from before 2000-01-01T00:00:00Z; it must be epoch seconds\n",
				map[string]string{"STAMP": stamp})
		})
	}
}

func TestCoverageReportStampedExactlyAtTheFloorIsScored(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	edited := time.Unix(946684800, 0)
	f.setModTime(orderService, edited)
	f.write("TestResults/coverage.cobertura.xml", coberturaStamped("946684800", f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestFutureStampedReportBesideAFreshOneStillRefuses(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml", coberturaStamped(freshStamp(), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.write("tests/Beta.Tests/TestResults/run/coverage.cobertura.xml", coberturaStamped("1767225600000", f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_timestamp_ahead_beside_fresh", 1, f.baseLabel("main"),
		"coverage report tests/Beta.Tests/TestResults/run/coverage.cobertura.xml carries a timestamp \"1767225600000\" more than 24h ahead of now; it must be epoch seconds, or the clock on this machine is behind the one that wrote it\n")
}

func TestCoverageReportOlderThanTheCodeItDescribesIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", coberturaStamped(f.editStamp(orderService, -365*24*time.Hour), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_stale", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml was written before src/Ordering/OrderService.cs was last edited; clear stale TestResults directories and re-run the tests\n")
}

func TestNamedCoverageReportIsUsedAndDiscoveryIsIgnored(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 4, 0)}))
	f.write("artifacts/coverage.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", "artifacts/coverage.xml").assertMatches(t, "named_coverage_report", 0,
		f.baseLabel("main"), "0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestNamingAReportSkipsDiscoveryEntirely(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("artifacts/coverage.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.write("TestResults/run/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.denyRead("TestResults/locked")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", "artifacts/coverage.xml").assertMatches(t, "named_coverage_report", 0,
		f.baseLabel("main"), "0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestNamedReportReachedThroughASymlinkIsNamedRepoRelative(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("artifacts/coverage.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	symlinkedDir(t, filepath.Join(f.root, "artifacts"), filepath.Join(f.root, "link"))
	f.runWithArgs("--coverage", "link/coverage.xml").assertMatches(
		t, "named_coverage_stale", 1, f.baseLabel("main"),
		"coverage report artifacts/coverage.xml was written before src/Ordering/OrderService.cs was last edited; "+
			"regenerate artifacts/coverage.xml or point --coverage at a current report\n")
}

func TestStaleReportIsRefusedAheadOfItsErasedSourceRoot(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml",
		renderCobertura(f.editStamp(orderService, -time.Second), nil,
			coverageClass{filename: "/_/src/Ordering/OrderService.cs", lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_stale", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml was written before src/Ordering/OrderService.cs was last edited; clear stale TestResults directories and re-run the tests\n")
}

func TestCoverageReportWithNoTimestampIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", coberturaStamped("", f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_untimestamped", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries no timestamp, so it cannot be judged against the code it describes\n")
}

// Producers other than coverlet write the attribute in a different
// representation, so the developer meeting this has a timestamp in front of
// them and needs to be told to rewrite it rather than that it is missing.
// Neither spelling is a base-10 integer, and reading either as one would
// judge the report against the Unix epoch instead.
// The int64 ceiling is a base-10 integer ParseInt reads happily, so it
// reaches the refusal only because the gate range-checks the value before
// building an instant from it. time.Unix offsets what it stores by the
// seconds between year 1 and 1970, so this stamp wraps into the distant
// past, and a report carrying it would otherwise be judged older than the
// code and skipped as superseded rather than refused as untrustworthy.
// Only the ceiling wraps. The int64 floor is a stamp time.Unix holds
// without underflowing, and the plausibility band refuses it afterwards
// with the wording that fits it, so it belongs to the floor case below
// rather than here.
func TestCoverageReportWithAnUnreadableTimestampIsRefused(t *testing.T) {
	t.Parallel()
	stamps := map[string]string{
		"ISO-8601 instant":   "2026-01-01T00:00:00Z",
		"fractional seconds": "1767225600.123",
		"int64 ceiling":      "9223372036854775807",
	}
	for name, stamp := range stamps {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, "main")
			f.write(orderService, csharpFile(80))
			f.commitAll("initial")
			f.touchLine(orderService, 62)
			f.write("TestResults/coverage.cobertura.xml", coberturaStamped(stamp, f.root,
				coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
			f.stub = stubConfig{
				Extensions: []string{".cs"},
				Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
			}

			f.run().assertMatchesWith(t, "coverage_timestamp_not_epoch_seconds", 1, f.baseLabel("main"),
				"coverage report TestResults/coverage.cobertura.xml carries an unreadable timestamp "+
					strconv.Quote(stamp)+"; it must be epoch seconds\n",
				map[string]string{"STAMP": stamp})
		})
	}
}

// Discovery consumes reports in Name order, so the stale one here is the
// second the loader reaches, and the loader still judges it rather than
// stopping at the first. That much this case pinned before issue 32, when
// judging a discovered report stale refused the run; issue 32 keeps the
// judging and changes what it does, because refusing here would fail every
// edit-and-test iteration on a leftover TestResults directory that a fresh
// run has already made irrelevant.
//
// The stale report carries a fourth instrumentable line the fresh one never
// lists. An implementation that merges a stale report before dropping it,
// rather than skipping it outright before the merge, scores Cancel over
// four lines instead of three: 0.5 and 4.13, not 0.667 and 3.33.
func TestSupersededDiscoveredReportIsSkippedAndTheFreshOneScores(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(freshStamp(), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.write("tests/Beta.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 4, 0)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "superseded_report_skipped", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestMethodWhoseOnlyCoverageWasSupersededIsUnknownNotMisscored(t *testing.T) {
	t.Parallel()
	const ghost = "src/Ordering/Ghost.cs"
	vanish := span{File: ghost, Name: "Ghost.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(ghost, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(ghost, 7)
	f.setModTime(ghost, f.modTime(orderService))
	f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(freshStamp(), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.write("tests/Beta.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: ghost, lines: spanCoverage(5, 4, 4)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(ghost, orderService), []span{vanish, cancel}),
	}

	f.run().assertMatches(t, "superseded_report_was_sole_coverage", 1, f.baseLabel("main"),
		"1 changed method could not be attributed to a coverage report\n"+
			"0 of 2 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestSupersededNameIsNotCarriedIntoALaterReportFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	const malformed = "<coverage><packages>\n"
	f.write("tests/Beta.Tests/TestResults/run/coverage.cobertura.xml", malformed)
	f.denyRead("tests/Zeta.Tests/TestResults/locked")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	cause := xmlUnmarshalCause(t, malformed)
	f.run().assertMatchesWith(t, "unparseable_report_after_a_superseded_one", 1, f.baseLabel("main"),
		"could not parse coverage report tests/Beta.Tests/TestResults/run/coverage.cobertura.xml; "+cause+"\n",
		map[string]string{"CAUSE": cause})
}

func TestSupersededReportAndUnreadablePathAreOneSortedSkippedList(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(freshStamp(), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.write("tests/Beta.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.denyRead("tests/Zeta.Tests/TestResults/locked")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "superseded_report_and_unreadable_path", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestEveryDiscoveredReportStaleRefusesNamingThemAll(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.write("tests/Beta.Tests/TestResults/run/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_stale_every_report", 1, f.baseLabel("main"),
		"coverage reports tests/Alpha.Tests/TestResults/run/coverage.cobertura.xml and "+
			"tests/Beta.Tests/TestResults/run/coverage.cobertura.xml were written before "+
			"src/Ordering/OrderService.cs was last edited; clear stale TestResults directories and re-run the tests\n")
}

// dotnet test Sln.sln --results-directory ./TestResults writes every
// project's report under one shared root, the layout that broke the
// grouping fix tried and reverted on the #15 branch: any rule keeping only
// the newest report per directory keeps one of these and scores Cancel on a
// third of its lines rather than the union of both. The two are stamped two
// seconds apart, both of them fresh, so such a rule has an unambiguous
// loser to drop; stamped on the same second, as cobertura() would stamp
// them, a rule that keeps both on a tie would pass this case unnoticed.
// The document is the union either report alone cannot produce, which
// two_projects_union already holds for two reports found under separate
// results directories.
func TestTwoProjectsSharingOneResultsDirectoryKeepBothReports(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/6f1c8a/coverage.cobertura.xml", coberturaStamped(f.editStamp(orderService, 0), f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 1}, {number: 62, hits: 0}, {number: 63, hits: 0}}}))
	f.write("TestResults/b90d21/coverage.cobertura.xml", coberturaStamped(f.editStamp(orderService, 2*time.Second), f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 0}, {number: 62, hits: 1}, {number: 63, hits: 1}}}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_projects_union", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.00\n")
}

func TestStalenessIsJudgedOnEveryNamedReportNotJustTheFirst(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("artifacts/unit.xml", coberturaStamped(freshStamp(), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.write("artifacts/integration.xml", coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", "artifacts/unit.xml", "--coverage", "artifacts/integration.xml").assertMatches(
		t, "named_coverage_stale_second", 1, f.baseLabel("main"),
		"coverage report artifacts/integration.xml was written before src/Ordering/OrderService.cs was last edited; "+
			"regenerate artifacts/integration.xml or point --coverage at a current report\n")
}

func TestNamedReportOutsideTheRepoIsRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	typed := filepath.Join(t.TempDir(), "coverage.xml")
	writeAbsolute(t, typed, coberturaStamped(freshStamp(), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 4, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", typed).assertMatches(t, "named_report_outside_repo", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 4.13\n")
}

func TestStaleNamedReportOutsideTheRepoIsNamedAbsolutely(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	typed := filepath.Join(t.TempDir(), "coverage.xml")
	writeAbsolute(t, typed, coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 4, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	named := resolvedPath(t, typed)
	f.runWithArgs("--coverage", typed).assertMatchesWith(t, "named_report_outside_repo_stale", 1,
		f.baseLabel("main"),
		"coverage report "+named+" was written before src/Ordering/OrderService.cs was last edited; "+
			"regenerate "+named+" or point --coverage at a current report\n",
		map[string]string{"REPORT": named})
}

func TestRelativeNamedReportOutsideTheRepoIsNamedAbsolutely(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	outside := filepath.Join(t.TempDir(), "coverage.xml")
	writeAbsolute(t, outside, coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 4, 2)}))
	workdir := filepath.Join(f.root, "src")
	typed, err := filepath.Rel(workdir, outside)
	if err != nil {
		t.Fatal(err)
	}
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	named := resolvedPath(t, outside)
	f.runFromWithArgs("src", "--coverage", typed).assertMatchesWith(t, "named_report_outside_repo_stale", 1,
		f.baseLabel("main"),
		"coverage report "+named+" was written before src/Ordering/OrderService.cs was last edited; "+
			"regenerate "+named+" or point --coverage at a current report\n",
		map[string]string{"REPORT": named})
}

func TestNamedReportOutsideTheRepoIsNamedPastItsSymlink(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	link := symlinkedDir(t, realDir, filepath.Join(dir, "link"))
	writeAbsolute(t, filepath.Join(realDir, "coverage.xml"),
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 4, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	named := filepath.ToSlash(filepath.Join(resolvedPath(t, realDir), "coverage.xml"))
	f.runWithArgs("--coverage", filepath.Join(link, "coverage.xml")).assertMatchesWith(
		t, "named_report_outside_repo_stale", 1, f.baseLabel("main"),
		"coverage report "+named+" was written before src/Ordering/OrderService.cs was last edited; "+
			"regenerate "+named+" or point --coverage at a current report\n",
		map[string]string{"REPORT": named})
}

func TestReportStampedAtTheSecondTheSourceWasEditedIsFresh(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.setModTime(orderService, f.modTime(orderService).Add(500*time.Millisecond))
	f.write("TestResults/coverage.cobertura.xml", coberturaStamped(f.editStamp(orderService, 0), f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_at_edit_second", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 5.67\n")
}

func TestReportStampedOneSecondBeforeTheEditIsStale(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_stale", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml was written before src/Ordering/OrderService.cs was last edited; clear stale TestResults directories and re-run the tests\n")
}

func TestStalenessJudgesTheReportAgainstTheNewestChangedFile(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(orderFile, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(orderFile, 62)
	edited := f.modTime(orderService)
	f.setModTime(orderFile, edited.Add(-10*time.Second))
	f.setModTime(orderService, edited)
	f.write("TestResults/coverage.cobertura.xml",
		coberturaStamped(stampAt(edited.Add(-5*time.Second)), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)},
			coverageClass{filename: orderFile, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService, orderFile), []span{cancel, orderTotal}),
	}

	f.run().assertMatches(t, "coverage_stale_newest_file", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml was written before src/Ordering/OrderService.cs was last edited; clear stale TestResults directories and re-run the tests\n")
}

func TestChangedFilesEditedInTheSameSecondNameTheSmallestPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(orderFile, csharpFile(80))
	f.write(pricingFile, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(orderFile, 62)
	f.touchLine(pricingFile, 62)
	edited := f.modTime(orderService)
	for _, file := range []string{orderService, orderFile, pricingFile} {
		f.setModTime(file, edited)
	}
	f.write("TestResults/coverage.cobertura.xml",
		coberturaStamped(stampAt(edited.Add(-time.Second)), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)},
			coverageClass{filename: orderFile, lines: spanCoverage(61, 3, 3)},
			coverageClass{filename: pricingFile, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout: extractorOutput(t, parsed(pricingFile, orderFile, orderService),
			[]span{pricingQuote, orderTotal, cancel}),
	}

	f.run().assertMatches(t, "coverage_stale_tie_break", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml was written before src/Ordering/Order.cs was last edited; clear stale TestResults directories and re-run the tests\n")
}

func TestCoverageFlagRepeatedUnionsEveryNamedReport(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("artifacts/unit.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 1}, {number: 62, hits: 0}, {number: 63, hits: 0}}}))
	f.write("artifacts/integration.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 0}, {number: 62, hits: 1}, {number: 63, hits: 0}}}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", "artifacts/unit.xml", "--coverage=artifacts/integration.xml").assertMatches(
		t, "repeated_coverage_flag", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestAbsoluteNamedReportIsNamedRepoRelativeInTheDocument(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("artifacts/coverage.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", filepath.Join(f.root, "artifacts", "coverage.xml")).assertMatches(
		t, "named_coverage_stale", 1, f.baseLabel("main"),
		"coverage report artifacts/coverage.xml was written before src/Ordering/OrderService.cs was last edited; "+
			"regenerate artifacts/coverage.xml or point --coverage at a current report\n")
}

func TestRelativeNamedReportResolvesAgainstTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("tests/placeholder.txt", "")
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("artifacts/from-subdir.xml",
		coberturaStamped(f.editStamp(orderService, -time.Second), f.root,
			coverageClass{filename: orderService, lines: spanCoverage(61, 3, 3)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runFromWithArgs("tests", "--coverage", "../artifacts/from-subdir.xml").assertMatches(
		t, "named_relative_stale", 1, f.baseLabel("main"),
		"coverage report artifacts/from-subdir.xml was written before src/Ordering/OrderService.cs was last edited; "+
			"regenerate artifacts/from-subdir.xml or point --coverage at a current report\n")
}

func TestNamedReportThatIsNotOnDiskIsRefusedAsUnparseable(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", "artifacts/typo.xml").assertMatches(
		t, "named_report_absent", 1, f.baseLabel("main"),
		"could not parse coverage report artifacts/typo.xml; open: no such file or directory\n")
}

func TestAbsentNamedReportUnderASymlinkedRootIsStillNamedRepoRelative(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	link := symlinkedDir(t, f.root, filepath.Join(t.TempDir(), "link"))
	f.runWithArgs("--coverage", filepath.Join(link, "artifacts", "typo.xml")).assertMatches(
		t, "named_report_absent", 1, f.baseLabel("main"),
		"could not parse coverage report artifacts/typo.xml; open: no such file or directory\n")
}

func TestNamedReportThroughAFileComponentIsStillNamedRepoRelative(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("artifacts/coverage.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	link := symlinkedDir(t, f.root, filepath.Join(t.TempDir(), "link"))
	f.runWithArgs("--coverage", filepath.Join(link, "artifacts", "coverage.xml", "typo.xml")).assertMatches(
		t, "named_report_through_a_file", 1, f.baseLabel("main"),
		"could not parse coverage report artifacts/coverage.xml/typo.xml; open: not a directory\n")
}

func TestAChangedFileMissingFromTheWorkingTreeFailsInsideTheDocument(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("checkout", "--quiet", "-b", "feature")
	f.touchLine(orderService, 62)
	f.commitAll("edit Cancel on the branch")
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.git("update-index", "--skip-worktree", orderService)
	absent := filepath.Join(f.root, filepath.FromSlash(orderService))
	if err := os.Remove(absent); err != nil {
		t.Fatal(err)
	}
	_, statErr := os.Stat(filepath.Join(resolvedPath(t, f.root), filepath.FromSlash(orderService)))
	if statErr == nil {
		t.Fatalf("%s is still readable, which the case needs it not to be", orderService)
	}

	f.runWithArgs().assertMatches(t, "changed_file_unreadable", 1, f.baseLabel("main"),
		"could not read "+orderService+" to date it against the coverage report: no such file or directory\n")
}

func TestCoverageFlagTakesAValueBeginningWithOneDashAsAPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("-report.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.runWithArgs("--coverage", "-report.xml").assertMatches(t, "named_coverage_report", 0,
		f.baseLabel("main"), "0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

// assertUsageError checks the shape ADR 0008 gives a malformed command line,
// the one documentless exit the metric-gate package doc records as sanctioned:
// exit 1, nothing on stdout, and the cause plus the usage line on stderr.
func assertUsageError(t *testing.T, result runResult, stderr string) {
	t.Helper()
	if result.exitCode == 1 && result.stdout == "" && result.stderr == stderr {
		return
	}
	t.Errorf("exit code: got %d, want 1\nstdout: got %q, want %q\nstderr: got %q, want %q",
		result.exitCode, result.stdout, "", result.stderr, stderr)
}

func TestNestedSolutionLayoutScoresCorrectly(t *testing.T) {
	t.Parallel()
	const nested = "src/Services/Ordering/Api/OrderService.cs"
	place := span{File: nested, Name: "OrderService.PlaceAsync", StartLine: 41, EndLine: 58, Complexity: 9}
	cancelNested := span{File: nested, Name: "OrderService.Cancel", StartLine: 60, EndLine: 64, Complexity: 3}

	f := newFixture(t, "main")
	f.write(nested, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(nested, 45)
	f.touchLine(nested, 62)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	otherCheckout := resolvedPath(t, t.TempDir())
	classPath := filepath.Join(otherCheckout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	f.write("TestResults/coverage.cobertura.xml", cobertura(otherCheckout,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := filepath.ToSlash(classPath)
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		outsideRepoStderr(example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestReportSourceMisCasingTheRepoRootPrefixIsMeasuredInside(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(sameDirectoryUpperCased(t, f.root),
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestNamedReportBuiltInAnotherCheckoutIsQuotedAbsolutely(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	otherCheckout := resolvedPath(t, t.TempDir())
	classPath := filepath.Join(otherCheckout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	typed := filepath.Join(t.TempDir(), "coverage.xml")
	writeAbsolute(t, typed, cobertura(otherCheckout,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := filepath.ToSlash(classPath)
	root := resolvedPath(t, f.root)
	named := resolvedPath(t, typed)
	f.runWithArgs("--coverage", typed).assertMatchesWith(t, "named_report_outside_repo_wrong_checkout", 1,
		f.baseLabel("main"),
		fmt.Sprintf("coverage report %s placed no class inside the repo root; example path %s, repo root %s\n",
			named, example, root),
		map[string]string{"REPORT": named, "EXAMPLE": example, "ROOT": root})
}

// TestForeignReportBesideAHealthyOneFailsNamingTheForeignOne pins issue 16's
// per-report failure against the multi-report shape none of the existing
// fixtures exercise. Discovery sorts reports, so "good" loads before "stale"
// and "zz-good" sorts after it, so the foreign report is neither the first
// nor the last of the three. Naming it rules out an implementation that names
// whichever report it saw first as well as one that names the last, rather
// than passing by accident of there being only one report to name. The
// healthy reports each cover two of Cancel's three lines, which scores 3.33
// and passes on its own, so the failure document also pins that healthy
// coverage is discarded rather than merged in alongside the report that
// failed.
func TestForeignReportBesideAHealthyOneFailsNamingTheForeignOne(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	f.write("TestResults/good/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))

	otherCheckout := resolvedPath(t, t.TempDir())
	classPath := filepath.Join(otherCheckout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	f.write("TestResults/stale/coverage.cobertura.xml", cobertura(otherCheckout,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))

	f.write("TestResults/zz-good/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))

	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := filepath.ToSlash(classPath)
	root := resolvedPath(t, f.root)
	const foreign = "TestResults/stale/coverage.cobertura.xml"
	f.run().assertMatchesWith(t, "coverage_outside_repo_beside_healthy", 1, f.baseLabel("main"),
		namedOutsideRepoStderr(foreign, example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestClassWithNoFilenameDoesNotSuppressTheOutsideRepoDiagnostic(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	otherCheckout := resolvedPath(t, t.TempDir())
	classPath := filepath.Join(otherCheckout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
		[]string{otherCheckout, filepath.Join(f.root, "src")},
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)},
		coverageClass{filename: "", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := filepath.ToSlash(classPath)
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		outsideRepoStderr(example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestReportReachedThroughASymlinkQuotesThePathItResolvedTo(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	base := resolvedPath(t, t.TempDir())
	checkout := filepath.Join(base, "checkout")
	classPath := filepath.Join(checkout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	link := symlinkedDir(t, checkout, filepath.Join(base, "link"))
	f.write("TestResults/coverage.cobertura.xml", cobertura(link,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := filepath.ToSlash(classPath)
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		outsideRepoStderr(example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestClassFilenameNamingNoFileDoesNotSuppressTheOutsideRepoDiagnostic(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	otherCheckout := resolvedPath(t, t.TempDir())
	classPath := filepath.Join(otherCheckout, filepath.FromSlash(orderService))
	writeAbsolute(t, classPath, csharpFile(80))
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
		[]string{otherCheckout, filepath.Join(f.root, "src"), filepath.Join(f.root, "src", "Ordering")},
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)},
		coverageClass{filename: ".", lines: spanCoverage(1, 1, 1)},
		coverageClass{filename: "..", lines: spanCoverage(1, 1, 1)},
		coverageClass{filename: "/", lines: spanCoverage(1, 1, 1)},
		coverageClass{filename: "../..", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := filepath.ToSlash(classPath)
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		outsideRepoStderr(example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestClassFilenameClimbingBackIntoTheRootScores(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(filepath.Join(f.root, "src", "Ordering"),
		coverageClass{filename: "../Ordering/OrderService.cs", lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestTwoClassesOnOneFileInOneReportAreUnioned(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: []coverageLine{{number: 61, hits: 4}, {number: 62, hits: 0}}},
		coverageClass{filename: orderService, lines: []coverageLine{{number: 63, hits: 1}}}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestReportMeasuredAgainstAContainerMountFailsNamingTheMismatch(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	mounted := filepath.Join(t.TempDir(), "srv", "repo", filepath.FromSlash(orderService))
	f.write("TestResults/coverage.cobertura.xml", cobertura("/",
		coverageClass{
			filename: strings.TrimPrefix(filepath.ToSlash(mounted), "/"),
			lines:    spanCoverage(61, 3, 2),
		}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := filepath.ToSlash(mounted)
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		outsideRepoStderr(example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestRelativeSourceIsRefusedRatherThanResolvedAgainstTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura("src",
		coverageClass{filename: "Ordering/OrderService.cs", lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	example := "src/Ordering/OrderService.cs"
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo_unanchored", 1, f.baseLabel("main"),
		outsideRepoUnanchoredStderr(example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestReportWithNoSourcesAndRelativeFilenamesFailsNamingTheMismatch(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", coberturaNoSources(
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo_unanchored", 1, f.baseLabel("main"),
		outsideRepoUnanchoredStderr(orderService, root),
		map[string]string{"EXAMPLE": orderService, "ROOT": root})
}

func TestReportWhoseEveryClassCarriesNoFilenameFailsSayingSo(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: "", lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo_no_filename", 1, f.baseLabel("main"),
		fmt.Sprintf("coverage report TestResults/coverage.cobertura.xml placed no class inside the repo root; "+
			"no class carries a filename to compare, repo root %s\n", root),
		map[string]string{"ROOT": root})
}

func TestDeterministicReportEmptyingSourcesFailsNamingTheProperty(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
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

func TestUseSourceLinkDocumentKeyBesideARealSourceRootFailsNamingTheProperty(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
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

func TestWhitespaceOnlySourceReadsAsBlankAndNamesTheProperty(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(" ",
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_source_root_erased_sourcelink", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries a source link document key rather than a "+
			"path, erased by UseSourceLink=true; collect coverage with UseSourceLink=false\n")
}

func TestWhitespacePaddedSourceRootStillAnchorsTheJoin(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(" "+f.root+" ",
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestBlankSourceWithAnAbsoluteFilenameScores(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura("",
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

func TestReportCarryingBothErasedShapesNamesDeterministicReport(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
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

func TestNumberedDeterministicPlaceholderFailsNamingTheProperty(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", coberturaNoSources(
		coverageClass{filename: "/_1/src/Ordering/OrderService.cs", lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_source_root_erased_deterministic", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries no source root, erased by "+
			"DeterministicReport=true; collect coverage with DeterministicReport=false\n")
}

func TestErasedSourceRootIsNamedAheadOfAnAmbiguousClass(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("src/a/Order.cs", csharpFile(20))
	f.write("src/b/Order.cs", csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
		[]string{filepath.Join(f.root, "src", "a"), filepath.Join(f.root, "src", "b")},
		coverageClass{filename: "Order.cs", lines: spanCoverage(1, 1, 1)},
		coverageClass{filename: "/_/src/Ordering/OrderService.cs", lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "coverage_source_root_erased_deterministic", 1, f.baseLabel("main"),
		"coverage report TestResults/coverage.cobertura.xml carries no source root, erased by "+
			"DeterministicReport=true; collect coverage with DeterministicReport=false\n")
}

func TestAmbiguousClassIsNamedAheadOfTheReportPlacingNothingInside(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("src/a/Order.cs", csharpFile(20))
	f.write("src/b/Order.cs", csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	otherCheckout := t.TempDir()
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
		[]string{otherCheckout, filepath.Join(f.root, "src", "a"), filepath.Join(f.root, "src", "b")},
		coverageClass{filename: "obsolete/Old.cs", lines: spanCoverage(1, 1, 1)},
		coverageClass{filename: "Order.cs", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "file_ambiguous", 1, f.baseLabel("main"),
		"class Order.cs in coverage report TestResults/coverage.cobertura.xml resolved to more than one path "+
			"inside the repo root, src/a/Order.cs and src/b/Order.cs\n")
}

func TestCaseOnlyPathDifferenceIsRefusedRatherThanGuessed(t *testing.T) {
	t.Parallel()
	const other = "src/Ordering/Other.cs"
	vanish := span{File: other, Name: "Other.Vanish", StartLine: 5, EndLine: 9, Complexity: 4}

	f := newFixture(t, "main")
	if !caseInsensitiveFilesystem(t, f.root) {
		t.Skip("the filesystem is case sensitive, so the wrong-case candidate never resolves and the case is a duplicate of the unresolvable one")
	}
	f.write(orderService, csharpFile(80))
	f.write(other, csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.touchLine(other, 7)
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("src/a/Order.cs", csharpFile(20))
	f.write("src/b/Order.cs", csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
		[]string{filepath.Join(f.root, "src", "b"), filepath.Join(f.root, "src", "a")},
		coverageClass{filename: "Order.cs", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "file_ambiguous", 1, f.baseLabel("main"),
		"class Order.cs in coverage report TestResults/coverage.cobertura.xml resolved to more than one path "+
			"inside the repo root, src/a/Order.cs and src/b/Order.cs\n")
}

func TestClassYieldingThreeCandidatesInsideRepoRootFailsNamingAllOfThem(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write("src/a/Order.cs", csharpFile(20))
	f.write("src/b/Order.cs", csharpFile(20))
	f.write("src/c/Order.cs", csharpFile(20))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
		[]string{
			filepath.Join(f.root, "src", "c"),
			filepath.Join(f.root, "src", "a"),
			filepath.Join(f.root, "src", "b"),
		},
		coverageClass{filename: "Order.cs", lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "file_ambiguous_three_paths", 1, f.baseLabel("main"),
		"class Order.cs in coverage report TestResults/coverage.cobertura.xml resolved to more than one path "+
			"inside the repo root, src/a/Order.cs, src/b/Order.cs and src/c/Order.cs\n")
}

func TestReportPathOutsideRepoIsIgnoredInSilence(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)

	otherCheckout := t.TempDir()
	writeAbsolute(t, filepath.Join(otherCheckout, "obsolete", "Old.cs"), csharpFile(5))
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
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
	t.Parallel()
	const deletedFile = "src/Ordering/Deleted.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(deletedFile, csharpFile(10))
	f.commitAll("initial")
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

func TestReportWhoseEveryClassIsGoneFromDiskFailsNamingTheMismatch(t *testing.T) {
	t.Parallel()
	const deletedFile = "src/Ordering/Deleted.cs"

	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.write(deletedFile, csharpFile(10))
	f.commitAll("initial")
	f.git("rm", "--quiet", deletedFile)
	f.commitAll("delete the only file the coverage report names")
	f.touchLine(orderService, 62)

	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: deletedFile, lines: spanCoverage(1, 1, 1)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{cancel}),
	}

	example := filepath.ToSlash(filepath.Join(f.root, filepath.FromSlash(deletedFile)))
	root := resolvedPath(t, f.root)
	f.run().assertMatchesWith(t, "coverage_outside_repo", 1, f.baseLabel("main"),
		outsideRepoStderr(example, root),
		map[string]string{"EXAMPLE": example, "ROOT": root})
}

func TestReportCarryingNoClassesDoesNotTripTheOutsideRepoDiagnostic(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{cancel}),
	}

	f.run().assertMatches(t, "report_with_no_classes", 1, f.baseLabel("main"),
		"1 changed method could not be attributed to a coverage report\n"+
			"0 of 1 changed methods over CRAP threshold 30, worst score 0.00\n")
}

func TestReportListingTheSameSourceTwiceIsNotAmbiguous(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", renderCobertura(freshStamp(),
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
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
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

// TestRunOutsideAGitRepoFailsWithATypedCodeAndADocument settles issue 86:
// running outside a git repository gets a typed code and a document, the same
// as every other exit-1 cause this early in the pipeline. scope comes from
// argv, which parsed fine, and base is null, the shape no_diff_base already
// pins for a branch with no commit.
func TestRunOutsideAGitRepoFailsWithATypedCodeAndADocument(t *testing.T) {
	t.Parallel()
	f := &fixture{t: t, root: t.TempDir()}
	if inRepo(t, f.root) {
		t.Skip("the temp directory is itself inside a git repo")
	}

	f.run().assertMatches(t, "no_git_repo", 1, "",
		"could not find a git repository: fatal: not a git repository (or any of the parent directories): .git\n")
}

// A runner with no git on PATH. Told to init a repository it would go looking
// for one it already has.
func TestRunWithNoGitOnPathFailsWithItsOwnCodeRatherThanNoGitRepo(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")

	f.runWithEnv("PATH=").assertMatches(t, "git_unavailable", 1, "",
		"could not run git: exec: \"git\": executable file not found in $PATH\n")
}

// A bare repository, which git opens and then refuses to name a toplevel for,
// since there is no work tree to measure. git found the repository, so
// no_git_repo would be the wrong answer.
func TestRunInABareRepositoryFailsWithItsOwnCodeRatherThanNoGitRepo(t *testing.T) {
	t.Parallel()
	f := &fixture{t: t, root: t.TempDir()}
	f.git("init", "--bare", "--quiet")

	f.run().assertMatches(t, "git_repo_unreadable", 1, "",
		"could not read the git repository: fatal: this operation must be run in a work tree\n")
}

// A toplevel git named that is not on disk. Real git cannot be made to answer
// that way, since it prints the directory it is already running in, so the
// case stands a git of its own on PATH. The arm still has to answer for the
// filesystem losing the root between the two calls.
func TestRunWhereTheRootGitNamesDoesNotResolveFailsInsideTheDocument(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	bin := t.TempDir()
	script := "#!/bin/sh\necho " + filepath.Join(t.TempDir(), "gone") + "\n"
	//nolint:gosec // the case puts this stand-in git on PATH for the gate to spawn as a subprocess, so it must be executable, and G306 tests the mode as a bitwise subset of 0600, which no execute bit can satisfy
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	f.runWithEnv("PATH="+bin).assertMatches(t, "repo_root_unresolvable", 1, "",
		"could not resolve the repo root git named: no such file or directory\n")
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

// TestStdoutRefusingTheWriteExitsOneRatherThanTheDocumentsOwnCode uses
// /dev/full, a character device that answers every write with ENOSPC, to
// force the real failure issue 81 describes: the document builds and the run
// would otherwise pass, but the write that was supposed to deliver it never
// lands. A run reading exit 0 back would have approved a change nobody saw.
func TestStdoutRefusingTheWriteExitsOneRatherThanTheDocumentsOwnCode(t *testing.T) {
	t.Parallel()
	info, err := os.Stat("/dev/full")
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("/dev/full does not exist on this platform")
	}
	if err != nil {
		t.Fatalf("stat /dev/full: %v", err)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		t.Skip("/dev/full is not a character device on this platform")
	}
	full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening /dev/full: %v", err)
	}
	defer func() {
		if err := full.Close(); err != nil {
			t.Errorf("closing /dev/full: %v", err)
		}
	}()

	f := newFixture(t, "main")
	f.stub = stubConfig{Extensions: []string{".cs"}}
	f.write("docs/notes.md", "first\n")
	f.write("src/Ordering/OrderService.cs", csharpFile(80))
	f.commitAll("initial")
	f.write("docs/notes.md", "first\nsecond\n")

	exitCode, stderr := f.runWithStdout(full)

	if exitCode != 1 || !strings.Contains(stderr, "writing the document to stdout") {
		t.Errorf("gate with stdout refusing every write: got exit %d, stderr %q; want exit 1, stderr naming the write failure",
			exitCode, stderr)
	}
}

func TestOriginHeadOutranksLocalMainAsTheDiffBase(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.addOrigin("main")
	f.setOriginHead()
	f.touchLine(orderService, 42)
	f.commitAll("second")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_hunks_one_file", 2, f.baseLabel("origin/HEAD"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestOriginMainOutranksLocalMainWhenNoOriginHeadExists(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.addOrigin("main")
	f.touchLine(orderService, 42)
	f.commitAll("second")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_hunks_one_file", 2, f.baseLabel("origin/main"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestOriginHeadOutranksOriginMainWhenTheRemoteDefaultBranchIsNotMain(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "trunk")
	f.addOrigin("trunk")
	f.setOriginHead()
	f.touchLine(orderService, 42)
	f.commitAll("second")
	f.git("push", "--quiet", "origin", "main")
	f.requireDistinctCommits("origin/main", "origin/HEAD")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_hunks_one_file", 2, f.baseLabel("origin/HEAD"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestOriginMainOutranksOriginMasterWhenBothRemoteBranchesExist(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.addOrigin("main")
	f.touchLine(orderService, 42)
	f.commitAll("second")
	f.git("branch", "master")
	f.git("push", "--quiet", "origin", "master")
	f.requireDistinctCommits("origin/main", "origin/master")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_hunks_one_file", 2, f.baseLabel("origin/main"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestOriginMasterOutranksLocalMainWhenBothExist(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "master")
	f.addOrigin("master")
	f.touchLine(orderService, 42)
	f.commitAll("second")
	f.requireDistinctCommits("origin/master", "main")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_hunks_one_file", 2, f.baseLabel("origin/master"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestLocalMainOutranksLocalMasterWhenBothBranchesExist(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("branch", "master")
	f.touchLine(orderService, 42)
	f.commitAll("second")
	f.requireDistinctCommits("main", "master")
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

func TestOriginMasterResolvesOnAMasterDefaultBranch(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "master")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.addOrigin("master")
	f.touchLine(orderService, 42)
	f.commitAll("second")
	f.requireDistinctCommits("origin/master", "master")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: append(spanCoverage(42, 10, 1), spanCoverage(61, 3, 2)...)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "two_hunks_one_file", 2, f.baseLabel("origin/master"),
		"1 of 2 changed methods over CRAP threshold 30, worst score 68.05\n")
}

func TestLocalMasterResolvesWhenNoRemoteExists(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "master")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.touchLine(orderService, 62)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	f.run().assertMatches(t, "pass_single_method", 0, f.baseLabel("master"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 3.33\n")
}

func TestRemoteMainWithUnrelatedHistoryFallsThroughToLocalMain(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.addOrigin("main")
	f.pushOrphanHistoryToOrigin("main")
	if head := f.git("for-each-ref", "--format=%(refname)", "refs/remotes/origin/HEAD"); head != "" {
		t.Fatalf("refs/remotes/origin/HEAD exists (%s), so the case no longer reaches the origin/main rung", head)
	}
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

func TestBranchWithNoCommitInARepoHoldingHistorySaysSoRatherThanNamingEveryRef(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("checkout", "--quiet", "--orphan", "fresh")
	f.git("rev-parse", "--verify", "--quiet", "main^{commit}")

	f.run().assertMatches(t, "base_no_commits", 1, "",
		"no diff base: this branch has no commit\n")
}

func TestARefStoreGitCannotReadReportsAnUnreadableDiffRatherThanNamingEveryRef(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.corruptPackedRefs()

	cause := f.gitStderr("rev-parse", "--verify", "--quiet", "HEAD")

	f.run().assertMatchesWith(t, "base_head_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestAMergeBaseGitCannotWalkReportsAnUnreadableDiffRatherThanTryingTheNextRef(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("checkout", "--quiet", "-b", "topic")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.removeLooseObject(f.git("rev-parse", "HEAD"))

	cause := f.gitStderr("merge-base", "HEAD", "main")

	f.run().assertMatchesWith(t, "base_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestBeforeTheFirstCommitTheDefaultScopeSaysSoRatherThanNamingEveryRef(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))

	f.run().assertMatches(t, "base_no_commits", 1, "",
		"no diff base: this branch has no commit\n")
}

func TestACandidateWhoseCommitObjectIsGoneReportsAnUnreadableDiffRatherThanNamingEveryRef(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("checkout", "--quiet", "-b", "topic")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.removeLooseObject(f.git("rev-parse", "main"))

	cause := f.gitStderr("merge-base", "HEAD", "main")

	f.run().assertMatchesWith(t, "base_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestATagShadowingACandidateWithANonCommitReportsAnUnreadableDiffRatherThanSkippingTheRung(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.git("checkout", "--quiet", "-b", "topic")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.git("tag", "main", f.git("rev-parse", "HEAD^{tree}"))

	cause := f.gitStderr("merge-base", "HEAD", "main")

	f.run().assertMatchesWith(t, "base_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}

func TestAnUnreadableRungStopsTheRunRatherThanResolvingABaseThroughALaterOne(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "main")
	f.write(orderService, csharpFile(80))
	f.commitAll("initial")
	f.addOrigin("main")
	f.git("checkout", "--quiet", "-b", "remote-tip")
	f.touchLine(orderService, 40)
	f.commitAll("remote tip")
	remoteTip := f.git("rev-parse", "HEAD")
	f.git("push", "--quiet", "origin", "remote-tip:main")
	f.git("checkout", "--quiet", "main")
	f.git("branch", "--quiet", "-D", "remote-tip")
	f.git("checkout", "--quiet", "-b", "topic")
	f.touchLine(orderService, 62)
	f.commitAll("second")
	f.requireDistinctCommits("origin/main", "main")
	if fallback := f.baseLabel("main"); fallback != "main@"+f.git("rev-parse", "main")[:7] {
		t.Fatalf("local main is %s rather than a base behind HEAD, so the walk has no healthy rung to fall through to", fallback)
	}
	f.removeLooseObject(remoteTip)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: orderService, lines: spanCoverage(61, 3, 2)}))
	f.stub = stubConfig{
		Extensions: []string{".cs"},
		Stdout:     extractorOutput(t, parsed(orderService), []span{placeAsync, cancel}),
	}

	cause := f.gitStderr("merge-base", "HEAD", "origin/main")

	f.run().assertMatchesWith(t, "base_merge_base_unreadable", 1, "",
		"could not read the diff: "+cause+"\n", map[string]string{"CAUSE": toonEscaped(cause)})
}
