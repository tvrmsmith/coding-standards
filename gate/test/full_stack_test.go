package gate_test

import (
	"errors"
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

// net8RuntimePrefix is the start of the dotnet --list-runtimes line that says
// the shared framework this case needs is present. The trailing dot keeps a
// future Microsoft.NETCore.App 80.x from matching, and naming the framework
// keeps Microsoft.AspNetCore.App and Microsoft.WindowsDesktop.App out.
const net8RuntimePrefix = "Microsoft.NETCore.App 8."

// reasonNoNet8Runtime is why TestFullStackDrivesTheRealDotnetExtractor skips,
// or fails when METRIC_GATE_REQUIRE_DOTNET forbids the skip. An SDK alone is
// not enough. dotnet/global.json pins a 10 SDK, the tool project targets
// net8.0 with no roll-forward, so the installed shim cannot launch on a
// machine that carries no 8.x shared framework. The framework name comes from
// net8RuntimePrefix so a move off net8.0 changes one line.
const reasonNoNet8Runtime = "no " + net8RuntimePrefix + "x runtime for the net8.0 extractor tool"

// reasonNoSDK is the other half. The case packs the tool project before it
// installs it, and a runtime-only dotnet answers --list-runtimes while
// carrying no compiler, so without this check that machine reaches dotnet pack
// and reds there instead of saying what it is missing.
const reasonNoSDK = "no dotnet SDK to pack the extractor tool with"

// listsNet8Runtime reports whether dotnet --list-runtimes output names an 8.x
// shared framework.
func listsNet8Runtime(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), net8RuntimePrefix) {
			return true
		}
	}
	return false
}

// TestListsNet8RuntimeMatchesOnlyTheSharedFramework pins the lines that count
// as an 8.x runtime and the near misses that must not.
func TestListsNet8RuntimeMatchesOnlyTheSharedFramework(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{"finds the framework among other lines", "Microsoft.AspNetCore.App 8.0.27 [/x]\nMicrosoft.NETCore.App 8.0.27 [/x]\n", true},
		{"finds it as the only line", "Microsoft.NETCore.App 8.0.0 [/x]", true},
		{"finds it past leading whitespace and a carriage return", "  Microsoft.NETCore.App 8.0.27 [/x]\r\n", true},
		{"rejects a 10 runtime", "Microsoft.NETCore.App 10.0.8 [/x]\n", false},
		{"rejects a future 80 line the prefix must not swallow", "Microsoft.NETCore.App 80.0.1 [/x]\n", false},
		{"rejects the aspnet framework", "Microsoft.AspNetCore.App 8.0.27 [/x]\n", false},
		{"rejects the windows desktop framework", "Microsoft.WindowsDesktop.App 8.0.27 [/x]\n", false},
		{"rejects empty output", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := listsNet8Runtime(c.out); got != c.want {
				t.Errorf("listsNet8Runtime = %v, want %v", got, c.want)
			}
		})
	}
}

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
		raw             string
		require         bool
		wantErrContains string
	}{
		{"1", true, ""},
		{"", false, ""},
		{"0", false, `"0"`},
		{"true", false, `"true"`},
		{"TRUE", false, `"TRUE"`},
		{"yes", false, `"yes"`},
		{" 1", false, `" 1"`},
		{"nope", false, `"1" is the only value that enables enforcement`},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s=%q", envRequireDotnet, c.raw), func(t *testing.T) {
			require, err := requireDotnet(c.raw)
			if require != c.require {
				t.Errorf("require = %v, want %v", require, c.require)
			}
			if c.wantErrContains == "" {
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("err = nil, want one containing %q", c.wantErrContains)
			}
			if !strings.Contains(err.Error(), c.wantErrContains) {
				t.Errorf("err = %q, want it to contain %q", err, c.wantErrContains)
			}
		})
	}
}

// probeDotnet answers one question: can this machine both pack the extractor
// tool and launch the net8.0 shim the pack installs. nil means yes. Anything
// else is the reason the case skips, already worded for a reader, naming the
// check that failed and what dotnet actually reported.
//
// Both halves are asked because neither implies the other. A runtime-only
// install answers --list-runtimes and has no compiler for dotnet pack, and the
// 10.x SDK a developer's machine packs with carries no 8.x shared framework
// for the shim to start on.
func probeDotnet() error {
	out, err := exec.Command("dotnet", "--list-sdks").CombinedOutput()
	listed := strings.TrimSpace(string(out))
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return fmt.Errorf("%s, and no dotnet to ask: %v", reasonNoSDK, err)
	case err != nil:
		return fmt.Errorf("%s, dotnet --list-sdks failed with %v and printed %q", reasonNoSDK, err, listed)
	case listed == "":
		return fmt.Errorf("%s, dotnet --list-sdks printed nothing, so this dotnet carries a runtime and no compiler", reasonNoSDK)
	}

	out, err = exec.Command("dotnet", "--list-runtimes").CombinedOutput()
	listed = strings.TrimSpace(string(out))
	switch {
	case err != nil:
		return fmt.Errorf("%s, dotnet --list-runtimes failed with %v and printed %q", reasonNoNet8Runtime, err, listed)
	case !listsNet8Runtime(listed):
		return fmt.Errorf("%s, dotnet --list-runtimes found no line starting with %q and listed %q", reasonNoNet8Runtime, net8RuntimePrefix, listed)
	}
	return nil
}

// TestRequireDotnetDecidesTheFullStackOutcome runs the full-stack case in a
// child copy of this test binary against a dotnet that cannot serve it, once
// with METRIC_GATE_REQUIRE_DOTNET=1 and once with it unset. Only a real run
// proves the variable name the child reads is the one CI sets and that the
// fatal route is reachable at all.
func TestRequireDotnetDecidesTheFullStackOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("forks child test binaries that each rebuild the gate, skipped with -short")
	}

	// A runtime-only host: the 8.x shared framework the shim needs is there,
	// and there is no SDK to pack with. Before the probe asked about the SDK
	// this machine ran the case and red at dotnet pack. The stub sticks to
	// shell builtins so it does not depend on what else the PATH carries.
	const stub = "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"--list-runtimes) echo 'Microsoft.NETCore.App 8.0.27 [/x/shared/Microsoft.NETCore.App]' ;;\n" +
		"esac\n"

	// The child runs the full-stack case and nothing else. Widening this
	// pattern would let the child re-enter this test and fork forever.
	const childCase = "^TestFullStackDrivesTheRealDotnetExtractor$"

	// childRan is what -test.v prints once the child is past TestMain and into
	// the case. Asserting it is what separates "the child failed for the reason
	// under test" from "the child never built" or "-run matched nothing".
	const childRan = "=== RUN   TestFullStackDrivesTheRealDotnetExtractor"

	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "dotnet"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	path := stubDir + string(os.PathListSeparator) + os.Getenv("PATH")

	// -test.short is passed explicitly, so what the child does never depends
	// on the flags this parent happens to run under.
	runChild := func(t *testing.T, require string) (string, error) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run", childCase, "-test.v", "-test.short=false")
		cmd.Env = append(childEnv(require), "PATH="+path)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	mustContain := func(t *testing.T, out string, wants ...string) {
		t.Helper()
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("child output does not contain %q. output:\n%s", want, out)
			}
		}
	}

	t.Run("the enforced run fails instead of skipping", func(t *testing.T) {
		out, err := runChild(t, "1")
		if err == nil {
			t.Fatalf("child exited zero, want a failure. output:\n%s", out)
		}
		mustContain(t, out, childRan, envRequireDotnet, "forbids skipping", reasonNoSDK)
	})

	t.Run("the unset run skips and passes", func(t *testing.T) {
		out, err := runChild(t, "")
		if err != nil {
			t.Fatalf("child failed with %v, want a pass. output:\n%s", err, out)
		}
		mustContain(t, out, childRan, "--- SKIP", reasonNoSDK)
	})
}

// TestTestMainRefusesAnUnusableRequireDotnet runs this binary with nothing
// selected, so the only thing under test is TestMain's read of
// METRIC_GATE_REQUIRE_DOTNET. That guard is what stops a -run filter excluding
// the full-stack case from leaving a typo undetected, and the parse it calls
// is covered as a pure function while the wiring around it is not.
func TestTestMainRefusesAnUnusableRequireDotnet(t *testing.T) {
	if testing.Short() {
		t.Skip("forks child test binaries that each rebuild the gate, skipped with -short")
	}

	runChild := func(t *testing.T, require string) (string, error) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run", "^$")
		cmd.Env = childEnv(require)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("a value nobody meant fails the package", func(t *testing.T) {
		out, err := runChild(t, "true")
		if err == nil {
			t.Fatalf("child exited zero, want a failure. output:\n%s", out)
		}
		for _, want := range []string{envRequireDotnet, `"true"`} {
			if !strings.Contains(out, want) {
				t.Errorf("child output does not contain %q. output:\n%s", want, out)
			}
		}
	})

	t.Run("the unset variable runs the package", func(t *testing.T) {
		if out, err := runChild(t, ""); err != nil {
			t.Fatalf("child failed with %v, want a pass. output:\n%s", err, out)
		}
	})
}

// childEnv copies this process's environment with METRIC_GATE_REQUIRE_DOTNET
// dropped, then sets it only when require is non-empty. Dropping it first is
// what lets the unset case run under CI, which sets the variable for the
// parent.
func childEnv(require string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, envRequireDotnet+"=") {
			continue
		}
		env = append(env, kv)
	}
	if require != "" {
		env = append(env, envRequireDotnet+"="+require)
	}
	return env
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
	// reason is empty only when nothing stands in the way, so a skip or a
	// fatal always states one. A -short run cannot use the probe's answer, so
	// it never pays for the subprocess and the first-run initialization dotnet
	// may do behind it.
	var reason string
	switch {
	case testing.Short():
		reason = reasonShort
	default:
		if err := probeDotnet(); err != nil {
			reason = err.Error()
		}
	}
	if reason != "" {
		if require {
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
