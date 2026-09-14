package gate_test

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/coverage"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// coverletFixtureFramework is the target framework both fixture projects
// build for. It matches the runtime CI installs beside the pinned SDK, so the
// test assembly the collector instruments runs on a framework the workflow
// already provisions.
const coverletFixtureFramework = "net8.0"

// Package versions the fixture pins. They are literals rather than a floating
// range because the report shape is this case's input. A coverlet bump can
// change an attribute spelling or the <sources> shape, and this case exists to
// notice that, so bumping coverletVersion here is a deliberate act that can
// red the case. It is not the only thing that can.
//
// The golden's coverage 0.75 is 15 of 20 instrumentable lines, and the
// instrumentable-line count comes from Roslyn's sequence-point emission rather
// than from coverlet. The fixture copies dotnet/global.json, which rolls
// forward to the latest 10.0.x feature band, so a new SDK release can move that
// count and red this case with no commit in this repo. That exposure is
// accepted: pinning the fixture's own global.json exactly would break the case
// on any machine without that precise band and decouple it from the one pin the
// tool pack uses. Re-baseline the golden when a compiler band moves it.
const (
	testSdkVersion     = "17.11.1"
	xunitVersion       = "2.9.2"
	xunitRunnerVersion = "2.8.2"
	coverletVersion    = "6.0.2"
)

// TestFullStackScoresAReportCoverletWrote runs the left half of the canonical
// invocation from issue 11, `dotnet test --collect:"XPlat Code Coverage"`, and
// lets the gate discover and score the file coverlet wrote. Every other case
// in this suite feeds the gate a report the cobertura() helper built, which
// encodes this repo's reading of the format rather than the producer's. This
// one pins the reading against the producer.
//
// Nothing here normalises the report, and nothing needs to. coverlet writes a
// fresh GUID directory per run and a timestamp that moves, but neither reaches
// the assertion: on the success path the machine document names no report path
// and no timestamp, because skipped_paths is empty and the crap table carries
// source paths only. What the harness has to guarantee instead is that exactly
// one report exists, which it gets from a fresh temp repo per run and a single
// dotnet test. The golden's only hole stays {{BASE}}.
//
// Two separate claims, kept apart because they have different strength.
//
// What the golden's numbers depend on is narrow: the <source> root, each
// class's filename, each <line>'s number and whether its hits are above zero,
// and the timestamp the staleness rule reads. The gate's parser takes nothing
// else from the document, so those are the only parts of coverlet's output a
// golden diff can speak for.
//
// Everything else coverlet emits, the gate reads past rather than reads, and a
// golden diff would stay silent if it disappeared. assertCoverletReportShape is
// what speaks for that half: it requires the collected report to still carry
// branch="False" in coverlet's capitalised spelling, a hit count above 1, a
// <methods> block whose <lines> repeat the class-level numbers, and line-rate
// and lines-covered on the root <coverage> element. The point is that the gate
// stays correct over a report carrying all four, not that any of them moves a
// number here.
//
// The ordering below is load-bearing, and each step says why.
func TestFullStackScoresAReportCoverletWrote(t *testing.T) {
	toolDir := installRealExtractor(t)

	f := newFixture(t, "main")
	f.writeCoverletFixture()
	f.commitAll("initial")

	// The edit comes before the collection. A report written against the
	// committed tree is older than the working-tree file the gate compares it
	// to, and the gate correctly refuses that as coverage_stale.
	f.appendComment("src/Points.cs", 7)
	f.collectCoverage()

	result := f.runIn(toolDir)

	result.assertMatches(t, "coverlet_real_report", 0, f.baseLabel("main"),
		"0 of 1 changed methods over CRAP threshold 30, worst score 10.27\n")
}

// writeCoverletFixture lays down a two-project solution `dotnet test` can
// restore, build and collect coverage over. The projects are generated here
// rather than checked in because a csproj referencing Microsoft.NET.Test.Sdk
// on disk is what ci.yml's wiring job counts as a suite, and this is a fixture
// no job executes.
func (f *fixture) writeCoverletFixture() {
	f.t.Helper()

	// The fixture pins its SDK from the same file that pins the tool pack's,
	// read rather than retyped so the two cannot drift apart.
	f.write("global.json", readFixture(f.t, filepath.Join(dotnetDir, "global.json")))

	// Nothing in the asserted document depends on this. The gate's changed set
	// comes from a diff against the base commit, which lists tracked paths
	// only, so the build output under bin/ and obj/ could never reach it. The
	// file is here so the fixture repo a reader lands in mid-debug reports a
	// clean tree rather than pages of build output.
	f.write(".gitignore", "bin/\nobj/\n")

	// A hand-written solution file is what lets the case run the literal
	// canonical invocation, `dotnet test` with no project argument.
	f.write("Fixture.slnx", `<Solution>
  <Project Path="src/Lib.csproj" />
  <Project Path="tests/Tests.csproj" />
</Solution>
`)

	// Copied verbatim from the dotnet extractor's own fixture. That is what
	// ties this golden's complexity 9 to the number the extractor's suite
	// already scores the same method at.
	f.write("src/Points.cs", readFixture(f.t, pointsFixture))

	// ContinuousIntegrationBuild and DeterministicSourcePaths are pinned off
	// rather than left to the environment. Either one on erases the source
	// root, which is ADR 0004's coverage_source_root_erased case, covered by
	// the hand-built helper. Inheriting them would make this case emit a
	// different document on CI than on a laptop, and MSBuild reads an
	// environment variable as a property, so a runner exporting either name
	// decides for a project that does not state it. Both projects carry the
	// pins, because coverlet reports the test assembly's own sources into the
	// same Cobertura document and the gate refuses the whole report on the
	// first class whose filename lost its root. ImplicitUsings is on because
	// Points.cs names Exception without a using.
	f.write("src/Lib.csproj", `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>`+coverletFixtureFramework+`</TargetFramework>
    <Nullable>enable</Nullable>
    <ImplicitUsings>enable</ImplicitUsings>
    <AssemblyName>Lib</AssemblyName>
    <RootNamespace>Fixtures</RootNamespace>
    <ContinuousIntegrationBuild>false</ContinuousIntegrationBuild>
    <DeterministicSourcePaths>false</DeterministicSourcePaths>
  </PropertyGroup>
</Project>
`)

	// RollForward matches the packed tool's own setting, so a machine carrying
	// the pinned SDK band and no 8.x shared framework launches the testhost on
	// the runtime it has instead of failing the run outright. It moves nothing
	// in the golden, because Roslyn still compiles for net8.0 and the
	// instrumentable-line count is its output, not the testhost's.
	f.write("tests/Tests.csproj", `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>`+coverletFixtureFramework+`</TargetFramework>
    <RollForward>Major</RollForward>
    <Nullable>enable</Nullable>
    <ImplicitUsings>enable</ImplicitUsings>
    <IsPackable>false</IsPackable>
    <ContinuousIntegrationBuild>false</ContinuousIntegrationBuild>
    <DeterministicSourcePaths>false</DeterministicSourcePaths>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="Microsoft.NET.Test.Sdk" Version="`+testSdkVersion+`" />
    <PackageReference Include="xunit" Version="`+xunitVersion+`" />
    <PackageReference Include="xunit.runner.visualstudio" Version="`+xunitRunnerVersion+`" />
    <PackageReference Include="coverlet.collector" Version="`+coverletVersion+`" />
  </ItemGroup>
  <ItemGroup>
    <ProjectReference Include="../src/Lib.csproj" />
  </ItemGroup>
</Project>
`)

	// One call into one arm of the switch. That single call is what produces
	// the 15 of 20 instrumentable lines the golden's coverage 0.75 comes from,
	// so changing it changes the golden.
	f.write("tests/PointsTests.cs", `using Fixtures;
using Xunit;

public class PointsTests
{
    [Fact]
    public void AllReturnsTheFirstSwitchArm()
    {
        Assert.Equal(1, new Points().All(1, "s"));
    }
}
`)
}

// collectCoverage runs the canonical invocation once in the fixture root and
// leaves the report where coverlet put it, under a GUID directory in
// TestResults, for the gate to discover. Running it twice would leave a
// superseded report the gate lists in skipped_paths, which is a second GUID in
// the document.
func (f *fixture) collectCoverage() {
	f.t.Helper()
	cmd := dotnetCmd(f.t, f.root, "test", "--collect:XPlat Code Coverage")
	// The restore uses the machine's ordinary NuGet cache rather than the
	// throwaway NUGET_PACKAGES installRealExtractor sets up. That isolation
	// exists because the tool project pins one version forever and would
	// otherwise resolve run 1's local build. These are immutable public
	// packages, so a cold restore on every run would cost CI minutes and prove
	// nothing.
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("dotnet test --collect:\"XPlat Code Coverage\": %v\n%s", err, out)
	}

	// The exit status says the tests passed, not that the collector wrote
	// anything. A run where coverlet.collector resolves but collects nothing
	// exits zero with no report, and a second report would reach the document
	// only as an extra GUID in skipped_paths. Either way the case would red as
	// an opaque golden diff, so the count is checked here where the cause can
	// be named.
	//
	// The count is the gate's own discovery, run here rather than restated, so
	// what this guard counts and what the gate later reads cannot disagree.
	root, err := srcpath.NewRoot(f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	reports, skipped, err := coverage.Discover(root)
	if err != nil {
		f.t.Fatalf("discovering the cobertura report coverlet wrote under %s: %v", f.root, err)
	}
	if len(reports) != 1 || len(skipped) > 0 {
		f.t.Fatalf("the collection left %d reports matching %s under %s and %d paths the walk could not read, want exactly one report and nothing skipped: %v %v\n%s",
			len(reports), coverage.Glob, f.root, len(skipped), reports, skipped, out)
	}

	f.assertCoverletReportShape(reports[0].Abs, string(out))
}

// scoredClassSuffix is how the one class the golden scores is found in the
// report. coverlet writes the class filename as the absolute path minus its
// leading slash, so the suffix is what identifies it without pinning the temp
// repo's own path. It leads with the separator, so a sibling under a directory
// merely ending "src" cannot answer for the fixture's own source.
const scoredClassSuffix = "/src/Points.cs"

// assertCoverletReportShape checks the report coverlet just wrote still carries
// the shapes the gate reads past. Reading the file here is reading the
// producer's serialized output as this case's input, not as a proxy for gate
// behaviour: a coverlet release that stops emitting them leaves the golden's
// numbers untouched, so without this the case would keep passing over an input
// it was never written for.
func (f *fixture) assertCoverletReportShape(path, dotnetOut string) {
	f.t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}

	var report struct {
		LineRate     string `xml:"line-rate,attr"`
		LinesCovered string `xml:"lines-covered,attr"`
		Classes      []struct {
			Filename string `xml:"filename,attr"`
			Lines    []struct {
				Number int    `xml:"number,attr"`
				Hits   int    `xml:"hits,attr"`
				Branch string `xml:"branch,attr"`
			} `xml:"lines>line"`
			Methods []struct {
				Lines []struct {
					Number int `xml:"number,attr"`
				} `xml:"lines>line"`
			} `xml:"methods>method"`
		} `xml:"packages>package>classes>class"`
	}
	if err := xml.Unmarshal(body, &report); err != nil {
		f.t.Fatalf("%s: %v", path, err)
	}

	// Scoped to the one class the golden scores. coverlet reports the test
	// assembly's own sources into the same document, so PointsTests.cs carrying
	// a shape must not stand in for Points.cs losing it.
	var classFilenames []string
	var scored []int
	for i, class := range report.Classes {
		classFilenames = append(classFilenames, class.Filename)
		if strings.HasSuffix(class.Filename, scoredClassSuffix) {
			scored = append(scored, i)
		}
	}
	if len(scored) != 1 {
		f.t.Fatalf("%s holds %d classes whose filename ends %s, want exactly one. None and the shape checks below would pass over a report that never scored the fixture's own source; more than one and a shape lost by one entry would be hidden by a sibling carrying it. Classes: %v\n%s",
			path, len(scored), scoredClassSuffix, classFilenames, dotnetOut)
	}

	class := report.Classes[scored[0]]
	var branchFalse, hitsAboveOne bool
	classLines := map[int]bool{}
	for _, line := range class.Lines {
		classLines[line.Number] = classLines[line.Number] || line.Hits > 0
		if line.Branch == "False" {
			branchFalse = true
		}
		if line.Hits > 1 {
			hitsAboveOne = true
		}
	}
	// Distinct line numbers, not <line> entries. coverlet emits a sibling method
	// per lambda or local function whose sequence points overlap the enclosing
	// one, so counting entries would raise this floor above the report-wide
	// lines-covered it is compared against and red a sound report.
	covered := 0
	for _, hit := range classLines {
		if hit {
			covered++
		}
	}
	// A precondition rather than a shape the gate reads past: the gate does read
	// the per-line hits, so an empty scored class would red the golden on its
	// own. It stops the case here so the rows below, which all speak for shapes
	// no golden diff reports, cannot report an empty class as a lost shape.
	if len(classLines) == 0 || covered == 0 {
		f.t.Fatalf("%s scored class %s carries %d <line> entries and %d of them hit, want both above zero, so every shape check below would speak for a class the report left empty or never ran\n%s",
			path, class.Filename, len(classLines), covered, dotnetOut)
	}

	methodLines := map[int]bool{}
	for _, method := range class.Methods {
		for _, line := range method.Lines {
			methodLines[line.Number] = true
		}
	}
	everyClassLineRepeated := true
	for number := range classLines {
		if !methodLines[number] {
			everyClassLineRepeated = false
		}
	}

	// The root summary is read as the numbers it claims to be rather than as
	// two non-empty strings. Both are report-wide, so the scored class's own
	// hit lines are a floor on lines-covered and proof that a zero rate is
	// wrong, which is as far as this class alone can speak for them.
	lineRate, lineRateErr := strconv.ParseFloat(report.LineRate, 64)
	linesCovered, linesCoveredErr := strconv.Atoi(report.LinesCovered)

	for _, shape := range []struct {
		got  bool
		want string
	}{
		{branchFalse, `a line of the scored class with branch="False", the capitalised spelling no hand-built report in this suite uses`},
		{hitsAboveOne, "a line of the scored class hit more than once, which the hand-built reports never produce"},
		{everyClassLineRepeated, "a <methods> line for every class-level line number of the scored class, so the same line arrives twice"},
		{lineRateErr == nil && lineRate > 0, fmt.Sprintf("a line-rate on the root element parsing as a number above zero, got %q, half of the summary the gate must not start trusting over the %d hit lines of the scored class", report.LineRate, covered)},
		{linesCoveredErr == nil && linesCovered >= covered, fmt.Sprintf("a lines-covered on the root element parsing as at least the %d hit lines of the scored class, got %q, the other half of that summary", covered, report.LinesCovered)},
	} {
		if !shape.got {
			f.t.Errorf("coverlet %s wrote %s without %s. The gate reads past this shape rather than reading it, so no golden diff would report its loss",
				coverletVersion, path, shape.want)
		}
	}
}
