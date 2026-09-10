package gate_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// dotnetDir is the directory every dotnet command here runs in, relative to
// this package. dotnet/global.json sits at its root, so the muxer resolves
// that pin for the probe and for the pack alike and one file governs both.
const dotnetDir = "../../dotnet"

// globalJSONPath names the same pin the way a reader of a failure message
// finds it, from the repository root rather than from this package.
const globalJSONPath = "dotnet/global.json"

// envRequireDotnet names the variable CI sets to forbid every skip route, so
// the one case that drives the real extractor cannot lapse into a green skip.
const envRequireDotnet = "METRIC_GATE_REQUIRE_DOTNET"

// envWorkflowEnforcement carries the enforcement environment pair the CI
// wiring job reads out of .github/workflows/ci.yml with a YAML parser, in
// KEY=VALUE form. Only that job sets it, so the assertion below is a no-op on
// a developer's machine and the workflow stays the single source of the key.
const envWorkflowEnforcement = "METRIC_GATE_WORKFLOW_ENFORCEMENT"

// reasonShort is why TestFullStackDrivesTheRealDotnetExtractor skips under
// -short, or fails there when METRIC_GATE_REQUIRE_DOTNET forbids the skip.
const reasonShort = "full-stack case packs and installs a dotnet tool, skipped with -short"

// runtimeFramework is the shared framework the installed tool shim launches
// on. Naming it keeps Microsoft.AspNetCore.App and Microsoft.WindowsDesktop.App
// out of the answer.
const runtimeFramework = "Microsoft.NETCore.App"

// runtimeFloorMajor is the oldest major that can run the tool. The project
// targets net8.0 and asks for Major roll-forward, so the shim starts on 8 and
// on anything newer, and only a runtime below the target is too old.
const runtimeFloorMajor = 8

// reasonNoUsableRuntime is why TestFullStackDrivesTheRealDotnetExtractor
// skips, or fails when METRIC_GATE_REQUIRE_DOTNET forbids the skip. An SDK
// alone is not enough: a machine can carry a compiler that packs the tool and
// no shared framework the packed shim will start on.
var reasonNoUsableRuntime = fmt.Sprintf(
	"no %s runtime of major %d or newer for the extractor tool", runtimeFramework, runtimeFloorMajor)

// reasonNoPinnedSDK is the other half. The case packs the tool project before
// it installs it, and the pack resolves dotnet/global.json, so a machine whose
// SDKs all fall outside that pin reds at dotnet pack instead of saying what it
// is missing.
var reasonNoPinnedSDK = "no dotnet SDK matching the pin in " + globalJSONPath + " to pack the extractor tool with"

// listsUsableRuntime reports whether dotnet --list-runtimes output names a
// shared framework the packed shim can launch on.
func listsUsableRuntime(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if major, ok := frameworkMajor(strings.TrimSpace(line)); ok && major >= runtimeFloorMajor {
			return true
		}
	}
	return false
}

// frameworkMajor reads the major version off one dotnet --list-runtimes line,
// reporting false for a line naming some other framework or carrying no
// version this can read.
func frameworkMajor(line string) (int, bool) {
	rest, ok := strings.CutPrefix(line, runtimeFramework+" ")
	if !ok {
		return 0, false
	}
	version, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
	major, err := strconv.Atoi(firstSegment(version))
	if err != nil {
		return 0, false
	}
	return major, true
}

// firstSegment is the part of a version before its first dot, which is the
// major on every shape dotnet prints, "8.0.27" and "10.0.0-preview.1" alike.
func firstSegment(version string) string {
	segment, _, _ := strings.Cut(version, ".")
	return segment
}

// TestListsUsableRuntimeMatchesTheFrameworkAtOrAboveTheFloor pins the lines
// that count as a runtime the shim can start on and the near misses that must
// not.
func TestListsUsableRuntimeMatchesTheFrameworkAtOrAboveTheFloor(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{"finds the framework among other lines", "Microsoft.AspNetCore.App 8.0.27 [/x]\nMicrosoft.NETCore.App 8.0.27 [/x]\n", true},
		{"finds the floor itself as the only line", "Microsoft.NETCore.App 8.0.0 [/x]", true},
		{"finds it past leading whitespace", "  Microsoft.NETCore.App 8.0.27 [/x]\n", true},
		{"accepts a 10 runtime, which Major roll-forward reaches", "Microsoft.NETCore.App 10.0.8 [/x]\n", true},
		{"accepts a bare major with a carriage return after it", "Microsoft.NETCore.App 10\r\n", true},
		{"rejects a runtime below the floor", "Microsoft.NETCore.App 7.0.20 [/x]\n", false},
		{"rejects the framework the shim cannot roll back to", "Microsoft.NETCore.App 6.0.36 [/x]\n", false},
		{"rejects the aspnet framework", "Microsoft.AspNetCore.App 8.0.27 [/x]\n", false},
		{"rejects the windows desktop framework", "Microsoft.WindowsDesktop.App 8.0.27 [/x]\n", false},
		{"rejects a line carrying no readable version", "Microsoft.NETCore.App preview [/x]\n", false},
		{"rejects empty output", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := listsUsableRuntime(c.out); got != c.want {
				t.Errorf("listsUsableRuntime = %v, want %v", got, c.want)
			}
		})
	}
}

// requireDotnet reads a METRIC_GATE_REQUIRE_DOTNET value and whether it was
// set at all. It takes both instead of reading the environment so every value
// has a test row without a case mutating process state. Leaving the variable
// out is the only way to disable enforcement. Once it is set, "1" is the only
// value that parses, and everything else, the empty string and "true" and "0"
// alike, is an error rather than a quiet off switch. A workflow expression
// naming a key that does not exist renders to the empty string, which is
// exactly the accident that would otherwise return this case to skipping while
// CI stayed green.
func requireDotnet(raw string, set bool) (bool, error) {
	if !set {
		return false, nil
	}
	if raw == "1" {
		return true, nil
	}
	return false, fmt.Errorf("%s=%q is not a value this suite accepts, and \"1\" is the only value that enables enforcement", envRequireDotnet, raw)
}

// TestRequireDotnetAcceptsOnlyTheDocumentedValues pins the two states
// requireDotnet accepts and the near misses it has to reject.
func TestRequireDotnetAcceptsOnlyTheDocumentedValues(t *testing.T) {
	cases := []struct {
		raw             string
		set             bool
		require         bool
		wantErrContains string
	}{
		{"1", true, true, ""},
		{"", false, false, ""},
		{"", true, false, `""`},
		{"0", true, false, `"0"`},
		{"true", true, false, `"true"`},
		{"TRUE", true, false, `"TRUE"`},
		{"yes", true, false, `"yes"`},
		{" 1", true, false, `" 1"`},
		{"nope", true, false, `"1" is the only value that enables enforcement`},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s=%q set=%v", envRequireDotnet, c.raw, c.set), func(t *testing.T) {
			require, err := requireDotnet(c.raw, c.set)
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

// dotnetOutput runs one dotnet command in dir and returns its trimmed stdout,
// its trimmed stderr, and the error. The two streams stay apart because only
// stdout carries the answer: a first-run banner or a restore warning on stderr
// would otherwise make an empty answer look like a real one.
func dotnetOutput(dir string, args ...string) (string, string, error) {
	cmd := exec.Command("dotnet", args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// installedSDKs describes what the machine actually carries, unfiltered by the
// pin. It exists for a failure message, so a reader sees the gap between the
// pin and the disk rather than only that there was one.
func installedSDKs() string {
	listed, _, err := dotnetOutput(".", "--list-sdks")
	switch {
	case err != nil:
		return fmt.Sprintf("dotnet --list-sdks also failed, with %v", err)
	case listed == "":
		return "dotnet --list-sdks printed nothing"
	default:
		return "dotnet --list-sdks printed " + strconv.Quote(listed)
	}
}

// probeDotnet answers one question: can this machine both pack the extractor
// tool and launch the shim the pack installs. nil means yes. Anything else is
// the reason the case skips, already worded for a reader, naming the check
// that failed and what dotnet actually reported.
//
// Both halves are asked because neither implies the other. An SDK outside the
// pin, or no SDK at all, reds at dotnet pack, and an SDK alone says nothing
// about which shared frameworks are on disk for the shim to start on.
//
// The SDK half runs where installRealExtractor packs, so the muxer resolves
// dotnet/global.json for both and the probe asks the question the pack will
// ask rather than a looser one.
func probeDotnet() error {
	version, stderr, err := dotnetOutput(dotnetDir, "--version")
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return fmt.Errorf("%s, and no dotnet to ask: %v", reasonNoPinnedSDK, err)
	case err != nil:
		return fmt.Errorf("%s, dotnet --version under %s failed with %v and said %q; %s",
			reasonNoPinnedSDK, globalJSONPath, err, stderr, installedSDKs())
	case version == "":
		return fmt.Errorf("%s, dotnet --version printed nothing, so this dotnet carries a runtime and no compiler; %s",
			reasonNoPinnedSDK, installedSDKs())
	}

	listed, stderr, err := dotnetOutput(".", "--list-runtimes")
	switch {
	case err != nil:
		return fmt.Errorf("%s, dotnet --list-runtimes failed with %v and said %q", reasonNoUsableRuntime, err, stderr)
	case !listsUsableRuntime(listed):
		return fmt.Errorf("%s, dotnet --list-runtimes printed %q", reasonNoUsableRuntime, listed)
	}
	return nil
}

// TestTheWorkflowEnablesEnforcement closes the gap between the key this suite
// reads and the key CI sets. The wiring job parses .github/workflows/ci.yml
// with a YAML reader, hands the gate job's environment pair over in
// METRIC_GATE_WORKFLOW_ENFORCEMENT, and this compares it against the constant
// the suite actually reads. Rename one without the other and the mismatch reds
// here rather than turning enforcement off in silence.
func TestTheWorkflowEnablesEnforcement(t *testing.T) {
	pair, ok := os.LookupEnv(envWorkflowEnforcement)
	if !ok {
		t.Skipf("%s is unset, so there is no extracted workflow pair to compare against; the CI wiring job sets it", envWorkflowEnforcement)
	}

	key, value, split := strings.Cut(pair, "=")
	if !split {
		t.Fatalf("%s=%q is not the KEY=VALUE pair the wiring job is meant to hand over", envWorkflowEnforcement, pair)
	}
	if key != envRequireDotnet {
		t.Fatalf("the gate job's test step sets %q but this suite reads %q, so the workflow key and the Go constant have drifted and enforcement is off: the full-stack case would skip in CI and the real extractor would never run",
			key, envRequireDotnet)
	}

	require, err := requireDotnet(value, true)
	if err != nil {
		t.Fatalf("the gate job sets %s=%q, which this suite refuses: %v", key, value, err)
	}
	if !require {
		t.Fatalf("the gate job sets %s=%q, which does not enable enforcement, so the full-stack case would skip in CI and the real extractor would never run", key, value)
	}
}

// childCase selects the full-stack case in a child and nothing else. Widening
// this pattern would let the child re-enter the forking tests and fork
// forever.
const childCase = "^TestFullStackDrivesTheRealDotnetExtractor$"

// childRan is what -test.v prints once a child is past TestMain and into the
// full-stack case. Asserting it is what separates "the child failed for the
// reason under test" from "the child never built" or "-run matched nothing".
const childRan = "=== RUN   TestFullStackDrivesTheRealDotnetExtractor"

// runChild runs a child copy of this test binary with
// METRIC_GATE_REQUIRE_DOTNET set to require, or dropped when require is empty,
// plus any extra environment entries laid over that. It returns the child's
// combined output and its exit error.
func runChild(t *testing.T, require string, extraEnv []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(childEnv(require), extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// mustContain reports every want a child's output is missing, rather than
// stopping at the first, so one run names the whole gap.
func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("child output does not contain %q. output:\n%s", want, out)
		}
	}
}

// stubDotnetPath writes a shell stub named dotnet into a temporary directory
// and returns a PATH that finds it first. The stubs stick to shell builtins so
// what they answer does not depend on what else the PATH carries.
func stubDotnetPath(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dotnet"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return "PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// TestRequireDotnetDecidesTheFullStackOutcome runs the full-stack case in a
// child copy of this test binary against machines that cannot serve it, with
// METRIC_GATE_REQUIRE_DOTNET set and unset. Only a real run proves the fatal
// route is reachable at all, and each subtest asserts the arm-specific tail of
// the probe's reason rather than the prefix all the arms share, so a stub that
// never executes reds instead of passing having proven nothing.
func TestRequireDotnetDecidesTheFullStackOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("forks child test binaries that each rebuild the gate, skipped with -short")
	}

	// A dotnet with no compiler: it answers --list-runtimes with a framework
	// the shim could start on and has no SDK for the pack, so --version prints
	// nothing. Before the probe asked about the SDK this machine ran the case
	// and red at dotnet pack.
	noSDK := stubDotnetPath(t, "#!/bin/sh\n"+
		"case \"$1\" in\n"+
		"--list-runtimes) echo 'Microsoft.NETCore.App 8.0.27 [/x/shared/Microsoft.NETCore.App]' ;;\n"+
		"esac\n")

	// The mirror image: a compiler matching the pin, and the only shared
	// framework on disk is older than the floor, so the packed shim has
	// nothing to launch on even though the pack itself would succeed.
	oldRuntime := stubDotnetPath(t, "#!/bin/sh\n"+
		"case \"$1\" in\n"+
		"--version) echo '10.0.100' ;;\n"+
		"--list-sdks) echo '10.0.100 [/x/sdk]' ;;\n"+
		"--list-runtimes) echo 'Microsoft.NETCore.App 6.0.36 [/x/shared/Microsoft.NETCore.App]' ;;\n"+
		"esac\n")

	// -test.short is passed explicitly in every row, so what a child does never
	// depends on the flags this parent happens to run under.
	const long, short = "-test.short=false", "-test.short=true"

	cases := []struct {
		name    string
		env     string
		require string
		short   string
		wantErr bool
		wants   []string
	}{
		{
			name:    "the enforced run fails instead of skipping",
			env:     noSDK,
			require: "1",
			short:   long,
			wantErr: true,
			wants:   []string{envRequireDotnet, "forbids skipping", reasonNoPinnedSDK, "printed nothing"},
		},
		{
			name:  "the unset run skips and passes",
			env:   noSDK,
			short: long,
			wants: []string{"--- SKIP", reasonNoPinnedSDK, "printed nothing"},
		},
		{
			name:    "a runtime below the floor is not enough for the enforced run",
			env:     oldRuntime,
			require: "1",
			short:   long,
			wantErr: true,
			wants:   []string{"forbids skipping", reasonNoUsableRuntime, "Microsoft.NETCore.App 6.0.36"},
		},
		{
			name:  "a runtime below the floor skips the unenforced run",
			env:   oldRuntime,
			short: long,
			wants: []string{"--- SKIP", reasonNoUsableRuntime, "Microsoft.NETCore.App 6.0.36"},
		},
		{
			// -short never reaches the probe, so these two rows run against
			// whatever dotnet the machine has and still decide the same way.
			name:  "-short skips when nothing forbids it",
			short: short,
			wants: []string{"--- SKIP", reasonShort},
		},
		{
			name:    "-short fails when enforcement forbids the skip",
			require: "1",
			short:   short,
			wantErr: true,
			wants:   []string{envRequireDotnet, "forbids skipping", reasonShort},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var extraEnv []string
			if c.env != "" {
				extraEnv = []string{c.env}
			}
			out, err := runChild(t, c.require, extraEnv, "-test.run", childCase, "-test.v", c.short)
			if c.wantErr && err == nil {
				t.Fatalf("child exited zero, want a failure. output:\n%s", out)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("child failed with %v, want a pass. output:\n%s", err, out)
			}
			mustContain(t, out, append([]string{childRan}, c.wants...)...)
		})
	}
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

	t.Run("a value nobody meant fails the package", func(t *testing.T) {
		out, err := runChild(t, "true", nil, "-test.run", "^$")
		if err == nil {
			t.Fatalf("child exited zero, want a failure. output:\n%s", out)
		}
		mustContain(t, out, envRequireDotnet, `"true"`)
	})

	t.Run("the unset variable runs the package", func(t *testing.T) {
		if out, err := runChild(t, "", nil, "-test.run", "^$"); err != nil {
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
	require, err := requireDotnet(os.LookupEnv(envRequireDotnet))
	if err != nil {
		t.Fatal(err)
	}
	// reason is empty only when nothing stands in the way, so a skip or a
	// fatal always states one. A -short run cannot use the probe's answer, so
	// it never pays for the subprocess and the first-run initialization dotnet
	// may do behind it.
	var reason string
	if testing.Short() {
		reason = reasonShort
	} else if err := probeDotnet(); err != nil {
		reason = err.Error()
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
	// Both commands run under dotnet/, so dotnet/global.json is an ancestor
	// and the muxer resolves the SDK pin the same way here as it does for a
	// developer packing by hand. Every path handed to them is absolute for
	// that reason: the working directory is the pin's, not this package's.
	dotnet := func(args ...string) *exec.Cmd {
		cmd := exec.Command("dotnet", args...)
		cmd.Dir = dotnetDir
		cmd.Env = append(os.Environ(), "NUGET_PACKAGES="+nugetPackages)
		return cmd
	}

	project, err := filepath.Abs(dotnetProject)
	if err != nil {
		t.Fatal(err)
	}

	packDir := t.TempDir()
	pack := dotnet("pack", project, "-c", "Release", "-o", packDir)
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
