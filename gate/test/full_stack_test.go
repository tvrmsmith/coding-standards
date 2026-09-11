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

// dotnetDir is the directory the pack and the install run in, relative to this
// package. dotnet/global.json sits at its root, so the muxer resolves that pin
// for both and one file governs which SDK builds the tool.
const dotnetDir = "../../dotnet"

// envRequireDotnet names the variable CI sets to forbid every skip route, so
// the one case that drives the real extractor cannot lapse into a green skip.
const envRequireDotnet = "METRIC_GATE_REQUIRE_DOTNET"

// envWorkflowEnforcement carries the whole env block of the CI gate job's test
// step, one KEY=VALUE pair per line, read out of .github/workflows/ci.yml with
// a YAML parser by the wiring job. The workflow names no key; this suite
// searches the block for its own constant, so the Go side stays the single
// source of the name. Only that job sets it, so the assertion below is a no-op
// on a developer's machine.
const envWorkflowEnforcement = "METRIC_GATE_WORKFLOW_ENFORCEMENT"

// envGitHubActions is the variable every GitHub Actions runner sets, which is
// how the wiring case tells a developer's machine, where skipping is right,
// from CI, where an unset block means a step stopped extracting it.
const envGitHubActions = "GITHUB_ACTIONS"

// reasonShort is why TestFullStackDrivesTheRealDotnetExtractor fails under
// -short once METRIC_GATE_REQUIRE_DOTNET forbids every skip route.
const reasonShort = "full-stack case packs and installs a dotnet tool, skipped with -short"

// reasonUnset is why the case skips when nothing enforces it. It states the
// one thing a reader has to do to make the case run, because the case asks
// nothing of the machine before it decides.
var reasonUnset = "set " + envRequireDotnet + "=1 to run the full-stack case"

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

// lookupPair finds key in a block of KEY=VALUE lines, the shape the wiring job
// hands the gate job's env block over in. Searching rather than taking a fixed
// position is what keeps a second variable landing in that step from pointing
// this case at the wrong key.
func lookupPair(block, key string) (string, bool) {
	for _, line := range strings.Split(block, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && k == key {
			return v, true
		}
	}
	return "", false
}

// TestLookupPairFindsTheKeyAnywhereInTheBlock pins that the search survives a
// step growing other variables, and reports the near misses as absent.
func TestLookupPairFindsTheKeyAnywhereInTheBlock(t *testing.T) {
	cases := []struct {
		name  string
		block string
		want  string
		found bool
	}{
		{"the only pair", "METRIC_GATE_REQUIRE_DOTNET=1", "1", true},
		{"last of several", "GOFLAGS=-mod=readonly\nCGO_ENABLED=0\nMETRIC_GATE_REQUIRE_DOTNET=1", "1", true},
		{"first of several", "METRIC_GATE_REQUIRE_DOTNET=1\nGOFLAGS=-mod=readonly", "1", true},
		{"past surrounding whitespace", "  METRIC_GATE_REQUIRE_DOTNET=1  \n", "1", true},
		{"an empty value is still found", "METRIC_GATE_REQUIRE_DOTNET=", "", true},
		{"a value carrying an equals sign", "METRIC_GATE_REQUIRE_DOTNET=a=b", "a=b", true},
		{"a block naming other keys only", "GOFLAGS=-mod=readonly\nCGO_ENABLED=0", "", false},
		{"a key that only shares a prefix", "METRIC_GATE_REQUIRE_DOTNET_V2=1", "", false},
		{"a bare key with no equals sign", "METRIC_GATE_REQUIRE_DOTNET", "", false},
		{"an empty block", "", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := lookupPair(c.block, envRequireDotnet)
			if got != c.want || found != c.found {
				t.Errorf("lookupPair = %q, %v, want %q, %v", got, found, c.want, c.found)
			}
		})
	}
}

// TestTheWorkflowEnablesEnforcement closes the gap between the key this suite
// reads and the key CI sets. The wiring job parses .github/workflows/ci.yml
// with a YAML reader, hands the gate job's whole env block over in
// METRIC_GATE_WORKFLOW_ENFORCEMENT, and this searches it for the constant the
// suite actually reads. Rename one without the other and the mismatch reds here
// rather than turning enforcement off in silence.
//
// The skip is for a developer's machine only. Under GitHub Actions an absent
// block means the step that extracts it was renamed or deleted, which would
// otherwise leave this case a permanent silent skip, so there it fails.
func TestTheWorkflowEnablesEnforcement(t *testing.T) {
	block, ok := os.LookupEnv(envWorkflowEnforcement)
	if !ok {
		if os.Getenv(envGitHubActions) != "" {
			t.Fatalf("%s is unset under %s, so no step handed this case the gate job's env block: the step that extracts it was renamed or deleted and nothing is checking that CI still enables enforcement",
				envWorkflowEnforcement, envGitHubActions)
		}
		t.Skipf("%s is unset, so there is no extracted workflow env block to search; the CI wiring job sets it", envWorkflowEnforcement)
	}

	value, found := lookupPair(block, envRequireDotnet)
	if !found {
		t.Fatalf("the gate job's test step sets %q, none of which is %s, so the workflow key and the Go constant have drifted and enforcement is off: the full-stack case would skip in CI and the real extractor would never run",
			block, envRequireDotnet)
	}

	require, err := requireDotnet(value, true)
	if err != nil {
		t.Fatalf("the gate job sets %s=%q, which this suite refuses: %v", envRequireDotnet, value, err)
	}
	if !require {
		t.Fatalf("the gate job sets %s=%q, which does not enable enforcement, so the full-stack case would skip in CI and the real extractor would never run", envRequireDotnet, value)
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

// childTimeout bounds a child. A test binary invoked directly, rather than
// through `go test`, defaults -test.timeout to 0, so without this a child that
// blocks holds CombinedOutput open until the parent's own timeout panics and
// reports nothing about which child hung.
const childTimeout = "-test.timeout=2m"

// runChild runs a child copy of this test binary with
// METRIC_GATE_REQUIRE_DOTNET set to require, or dropped when require is empty.
// It returns the child's combined output and its exit error.
func runChild(t *testing.T, require string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{childTimeout}, args...)...)
	cmd.Env = childEnv(require)
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

// TestRequireDotnetDecidesTheFullStackOutcome runs the full-stack case in a
// child copy of this test binary with METRIC_GATE_REQUIRE_DOTNET set and
// unset. The case asks the machine nothing before it decides, so the variable
// and -short are the whole input, and only a real run proves the fatal route
// is reachable at all rather than dead behind a skip.
func TestRequireDotnetDecidesTheFullStackOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("forks child test binaries that each rebuild the gate, skipped with -short")
	}

	// -test.short is passed explicitly in every row, so what a child does never
	// depends on the flags this parent happens to run under.
	const long, short = "-test.short=false", "-test.short=true"

	cases := []struct {
		name    string
		require string
		short   string
		wantErr bool
		wants   []string
	}{
		{
			name:  "the unset run skips without touching dotnet",
			short: long,
			wants: []string{"--- SKIP", reasonUnset},
		},
		{
			name:  "-short does not change the unset run",
			short: short,
			wants: []string{"--- SKIP", reasonUnset},
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
			out, err := runChild(t, c.require, "-test.run", childCase, "-test.v", c.short)
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
		out, err := runChild(t, "true", "-test.run", "^$")
		if err == nil {
			t.Fatalf("child exited zero, want a failure. output:\n%s", out)
		}
		mustContain(t, out, envRequireDotnet, `"true"`)
	})

	t.Run("the unset variable runs the package", func(t *testing.T) {
		if out, err := runChild(t, "", "-test.run", "^$"); err != nil {
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
	// Nothing here asks the machine what it carries. A run that means to drive
	// the real extractor says so, and then a machine that cannot serve it reds
	// at the pack rather than skipping green; a run that does not say so skips
	// before the first dotnet call.
	if !enforceDotnet {
		t.Skip(reasonUnset)
	}
	if testing.Short() {
		t.Fatalf("%s=1 forbids skipping: %s", envRequireDotnet, reasonShort)
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
	// A directory holding the gate and no extractor beside it is exactly what
	// this case needs before it installs the real tool into it.
	dir := gateOnlyDir(t)

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
