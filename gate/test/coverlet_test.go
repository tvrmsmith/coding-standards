package gate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
// The ordering below is load-bearing, and each step says why.
func TestFullStackScoresAReportCoverletWrote(t *testing.T) {
	requireRealDotnet(t)

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

	// The build writes generated .cs files under obj/, and without this they
	// land in the changed set and the gate measures them.
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
	// the hand-built helper; inheriting them would make this case emit a
	// different document on CI than on a laptop. ImplicitUsings is on because
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

	f.write("tests/Tests.csproj", `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>`+coverletFixtureFramework+`</TargetFramework>
    <Nullable>enable</Nullable>
    <ImplicitUsings>enable</ImplicitUsings>
    <IsPackable>false</IsPackable>
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
	cmd := exec.Command("dotnet", "test", "--collect:XPlat Code Coverage")
	cmd.Dir = f.root
	// The restore uses the machine's ordinary NuGet cache rather than the
	// throwaway NUGET_PACKAGES installRealExtractor sets up. That isolation
	// exists because the tool project pins one version forever and would
	// otherwise resolve run 1's local build. These are immutable public
	// packages, so a cold restore on every run would cost CI minutes and prove
	// nothing.
	cmd.Env = append(os.Environ(), gitEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("dotnet test --collect:\"XPlat Code Coverage\": %v\n%s", err, out)
	}
}
