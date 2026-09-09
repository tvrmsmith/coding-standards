package gate_test

import (
	"fmt"
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

// envRequireDotnet names the variable CI sets to forbid every skip route, so
// the one case that drives the real extractor cannot lapse into a green skip.
const envRequireDotnet = "METRIC_GATE_REQUIRE_DOTNET"

// reasonShort is why TestFullStackDrivesTheRealDotnetExtractor skips under
// -short, or fails there when METRIC_GATE_REQUIRE_DOTNET forbids the skip.
const reasonShort = "full-stack case packs and installs a dotnet tool, skipped with -short"

// reasonNoSDK is why TestFullStackDrivesTheRealDotnetExtractor skips without a
// usable .NET SDK, or fails when METRIC_GATE_REQUIRE_DOTNET forbids the skip.
const reasonNoSDK = "no usable .NET SDK: dotnet --version failed"

// requireDotnet reads a raw METRIC_GATE_REQUIRE_DOTNET value. It takes the
// string instead of reading the environment so every value has a test row
// without a case mutating process state. Only "1" and the unset empty string
// parse. Anything else, "true" and "0" included, is an error rather than a
// quiet off switch, because a value nobody meant would otherwise return this
// case to skipping while CI stayed green.
func requireDotnet(raw string) (bool, error) {
	switch raw {
	case "1":
		return true, nil
	case "":
		return false, nil
	default:
		return false, fmt.Errorf("%s=%q is not a value this suite accepts, and \"1\" is the only value that enables enforcement", envRequireDotnet, raw)
	}
}

// TestRequireDotnetAcceptsOnlyTheDocumentedValues pins the two values
// requireDotnet accepts and the near misses it has to reject.
func TestRequireDotnetAcceptsOnlyTheDocumentedValues(t *testing.T) {
	cases := []struct {
		raw     string
		require bool
		wantErr bool
	}{
		{"1", true, false},
		{"", false, false},
		{"0", false, true},
		{"true", false, true},
		{"TRUE", false, true},
		{"yes", false, true},
		{" 1", false, true},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s=%q", envRequireDotnet, c.raw), func(t *testing.T) {
			require, err := requireDotnet(c.raw)
			if require != c.require {
				t.Errorf("require = %v, want %v", require, c.require)
			}
			if (err != nil) != c.wantErr {
				t.Errorf("err = %v, want error %v", err, c.wantErr)
			}
		})
	}
}

// dotnetSkipReason decides whether TestFullStackDrivesTheRealDotnetExtractor
// may skip and, if so, whether it is actually allowed to. require reflects
// METRIC_GATE_REQUIRE_DOTNET=1, which CI sets so a missing SDK fails the run
// instead of silently skipping the only case that exercises the real
// extractor. short is checked before sdkOK, so a -short run always reports
// the -short reason even when the SDK is also missing. That ordering also
// lets the caller pass sdkOK true under -short without probing for dotnet,
// because the value cannot reach the result.
func dotnetSkipReason(short, sdkOK, require bool) (reason string, fatal bool) {
	switch {
	case short:
		reason = reasonShort
	case !sdkOK:
		reason = reasonNoSDK
	}
	return reason, reason != "" && require
}

// TestDotnetSkipReasonRefusesToSkipWhenRequired pins dotnetSkipReason's
// reason and fatal decision for every combination of -short, SDK
// availability, and METRIC_GATE_REQUIRE_DOTNET.
func TestDotnetSkipReasonRefusesToSkipWhenRequired(t *testing.T) {
	cases := []struct {
		name    string
		short   bool
		sdkOK   bool
		require bool
		reason  string
		fatal   bool
	}{
		{"runs when short is false, sdk is ok, and dotnet is not required", false, true, false, "", false},
		{"runs when short is false, sdk is ok, and dotnet is required", false, true, true, "", false},
		{"skips for -short when dotnet is not required", true, true, false, reasonShort, false},
		{"fails for -short when dotnet is required", true, true, true, reasonShort, true},
		{"skips for a missing sdk when dotnet is not required", false, false, false, reasonNoSDK, false},
		{"fails for a missing sdk when dotnet is required", false, false, true, reasonNoSDK, true},
		{"reports the -short reason over a missing sdk when dotnet is not required", true, false, false, reasonShort, false},
		{"reports the -short reason over a missing sdk when dotnet is required", true, false, true, reasonShort, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason, fatal := dotnetSkipReason(c.short, c.sdkOK, c.require)
			if reason != c.reason {
				t.Errorf("reason = %q, want %q", reason, c.reason)
			}
			if fatal != c.fatal {
				t.Errorf("fatal = %v, want %v", fatal, c.fatal)
			}
		})
	}
}

// TestFullStackDrivesTheRealDotnetExtractor is the only case in the suite
// that runs the real dotnet tool extractor end to end instead of the stub.
// Its fixture and golden are pinned to match fail_single_method's numbers, so
// if the real tool and the stub ever disagree about a span or a complexity,
// this is what catches it.
//
// The coverage report it hands the gate is still synthetic. coverlet.collector,
// the producer the C# extractor targets, writes its timestamp attribute as
// ten-digit epoch seconds, which is the one representation the gate reads, so
// building the report here rather than collecting one costs no coverage of the
// staleness rule. Collecting real coverage means running dotnet test, which is
// issue 21's work and not this case's.
func TestFullStackDrivesTheRealDotnetExtractor(t *testing.T) {
	require, err := requireDotnet(os.Getenv(envRequireDotnet))
	if err != nil {
		t.Fatal(err)
	}
	// A -short run cannot use the probe's answer, so it never pays for the
	// subprocess and the first-run initialization dotnet may do behind it.
	short := testing.Short()
	sdkOK := short || exec.Command("dotnet", "--version").Run() == nil
	if reason, fatal := dotnetSkipReason(short, sdkOK, require); reason != "" {
		if fatal {
			t.Fatalf("%s=1 forbids skipping: %s", envRequireDotnet, reason)
		}
		t.Skip(reason)
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
