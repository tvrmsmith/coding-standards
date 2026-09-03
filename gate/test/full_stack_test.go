package gate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pointsFixture is the path, relative to the gate module root, to the C#
// method the dotnet extractor's own suite already scores at complexity 9.
// Copying it verbatim keeps this case's golden numbers tied to the real
// tool's fixture rather than to a value retyped by hand.
const pointsFixture = "../../dotnet/tests/Tvrmsmith.MetricGate.CSharp.Tests/fixtures/Points.cs"

// dotnetProject is the tool project this case packs and installs in place of
// the stub.
const dotnetProject = "../../dotnet/src/Tvrmsmith.MetricGate.CSharp"

// TestFullStackDrivesTheRealDotnetExtractor is the only case in the suite
// that runs the real dotnet tool extractor end to end instead of the stub.
// Its fixture and golden are pinned to match fail_single_method's numbers, so
// if the real tool and the stub ever disagree about a span or a complexity,
// this is what catches it.
func TestFullStackDrivesTheRealDotnetExtractor(t *testing.T) {
	if testing.Short() {
		t.Skip("full-stack case packs and installs a dotnet tool, skipped with -short")
	}
	if err := exec.Command("dotnet", "--version").Run(); err != nil {
		t.Skip("no usable .NET SDK: dotnet --version failed")
	}

	fullStackBinDir := installRealExtractor(t)

	f := newFixture(t, "main")
	f.write("src/Points.cs", readFixture(t, pointsFixture))
	f.commitAll("initial")
	f.appendComment("src/Points.cs", 16)
	f.write("TestResults/coverage.cobertura.xml", cobertura(f.root,
		coverageClass{filename: "src/Points.cs", lines: spanCoverage(6, 10, 1)}))

	result := f.runIn(fullStackBinDir)

	result.assertMatches(t, "full_stack", 2, f.baseLabel("main"),
		"1 of 1 changed methods over CRAP threshold 30, worst score 68.05\n")
}

// readFixture reads a file relative to the gate module root, failing the
// test rather than returning an error a caller might ignore.
func readFixture(t *testing.T, rel string) string {
	t.Helper()
	body, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// appendComment appends a trailing comment to line n of the file at rel,
// touching that one line without disturbing any other, and without removing
// a construct the real extractor would count towards complexity.
func (f *fixture) appendComment(rel string, n int) {
	f.t.Helper()
	lines := strings.Split(f.read(rel), "\n")
	lines[n-1] += " // touched"
	f.write(rel, strings.Join(lines, "\n"))
}

// installRealExtractor builds a fresh metric-gate binary and packs and
// installs the real dotnet tool extractor beside it, in a directory separate
// from the shared stub-based binDir. It returns that directory.
func installRealExtractor(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	build := exec.Command("go", "build", "-o", filepath.Join(dir, "metric-gate"), "./cmd/metric-gate")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building metric-gate: %v\n%s", err, out)
	}

	// `dotnet tool install` resolves the package id and version out of the
	// NuGet global packages folder once anything has extracted it there, and
	// the csproj pins one version forever. Sharing the machine's folder would
	// install run 1's build on every later run, so this case would verify a
	// stale extractor and keep passing after a real regression in the tool.
	nugetPackages := t.TempDir()
	dotnet := func(args ...string) *exec.Cmd {
		cmd := exec.Command("dotnet", args...)
		cmd.Env = append(os.Environ(), "NUGET_PACKAGES="+nugetPackages)
		return cmd
	}

	packDir := t.TempDir()
	pack := dotnet("pack", dotnetProject, "-c", "Release", "-o", packDir)
	if out, err := pack.CombinedOutput(); err != nil {
		t.Fatalf("dotnet pack: %v\n%s", err, out)
	}

	install := dotnet("tool", "install",
		"--tool-path", dir, "--add-source", packDir, "Tvrmsmith.MetricGate.CSharp")
	if out, err := install.CombinedOutput(); err != nil {
		t.Fatalf("dotnet tool install: %v\n%s", err, out)
	}
	return dir
}
