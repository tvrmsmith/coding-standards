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

// skipCause names why TestFullStackDrivesTheRealDotnetExtractor would skip.
// The caller switches on it instead of matching the reason text, so adding a
// cause is a compile-time question rather than a string comparison that
// silently stops matching.
type skipCause int

const (
	// causeRuns is the case going ahead: no skip route applies.
	causeRuns skipCause = iota
	causeShort
	causeNoNet8Runtime
)

// reason is the text this cause skips or fails with.
func (c skipCause) reason() string {
	switch c {
	case causeShort:
		return reasonShort
	case causeNoNet8Runtime:
		return reasonNoNet8Runtime
	default:
		return ""
	}
}

// dotnetSkipReason decides whether TestFullStackDrivesTheRealDotnetExtractor
// may skip and, if so, whether it is actually allowed to. require reflects
// METRIC_GATE_REQUIRE_DOTNET=1, which CI sets so a missing runtime fails the
// run instead of silently skipping the only case that exercises the real
// extractor. net8OK says the 8.x shared framework the tool needs is present,
// which an SDK on PATH does not by itself prove.
// short is checked before net8OK, so a -short run always reports the -short
// cause even when the runtime is also missing. That ordering also lets the
// caller pass net8OK true under -short without probing for dotnet, because
// the value cannot reach the result.
func dotnetSkipReason(short, net8OK, require bool) (cause skipCause, fatal bool) {
	switch {
	case short:
		cause = causeShort
	case !net8OK:
		cause = causeNoNet8Runtime
	}
	return cause, cause != causeRuns && require
}

// TestDotnetSkipReasonRefusesToSkipWhenRequired pins dotnetSkipReason's cause
// and fatal decision, and each cause's reason text, for every combination of
// -short, runtime availability, and METRIC_GATE_REQUIRE_DOTNET.
func TestDotnetSkipReasonRefusesToSkipWhenRequired(t *testing.T) {
	cases := []struct {
		name    string
		short   bool
		net8OK  bool
		require bool
		cause   skipCause
		reason  string
		fatal   bool
	}{
		{"runs when short is false, the runtime is present, and dotnet is not required", false, true, false, causeRuns, "", false},
		{"runs when short is false, the runtime is present, and dotnet is required", false, true, true, causeRuns, "", false},
		{"skips for -short when dotnet is not required", true, true, false, causeShort, reasonShort, false},
		{"fails for -short when dotnet is required", true, true, true, causeShort, reasonShort, true},
		{"skips for a missing runtime when dotnet is not required", false, false, false, causeNoNet8Runtime, reasonNoNet8Runtime, false},
		{"fails for a missing runtime when dotnet is required", false, false, true, causeNoNet8Runtime, reasonNoNet8Runtime, true},
		{"reports the -short cause over a missing runtime when dotnet is not required", true, false, false, causeShort, reasonShort, false},
		{"reports the -short cause over a missing runtime when dotnet is required", true, false, true, causeShort, reasonShort, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cause, fatal := dotnetSkipReason(c.short, c.net8OK, c.require)
			if cause != c.cause {
				t.Errorf("cause = %v, want %v", cause, c.cause)
			}
			if got := cause.reason(); got != c.reason {
				t.Errorf("reason = %q, want %q", got, c.reason)
			}
			if fatal != c.fatal {
				t.Errorf("fatal = %v, want %v", fatal, c.fatal)
			}
		})
	}
}

// TestRequireDotnetDecidesTheFullStackOutcome runs the full-stack case in a
// child copy of this test binary, once with METRIC_GATE_REQUIRE_DOTNET=1 and
// once with it unset, against three stub dotnets: one that exits non-zero, one
// that succeeds while listing no 8.x shared framework, and one that records
// having been called at all so the -short run can prove it never probed. The
// helpers each carry their own table, but only a real run proves the variable
// name the child reads is the one CI sets and that the fatal path is reachable
// at all.
func TestRequireDotnetDecidesTheFullStackOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("forks child test binaries that each rebuild the gate, skipped with -short")
	}

	const stubRefusesToRun = "#!/bin/sh\necho 'stub dotnet refuses to run' >&2\nexit 3\n"
	// A machine carrying only the SDK dotnet/global.json pins reaches this
	// state, an installed toolchain that lists no 8.x shared framework. The
	// AspNetCore line rides along so the run proves the probe wants the one
	// framework the tool launches on rather than any 8 line at all.
	const stubListsNoNet8 = "#!/bin/sh\ncat <<'EOF'\n" +
		"Microsoft.AspNetCore.App 8.0.0 [/x/shared/Microsoft.AspNetCore.App]\n" +
		"Microsoft.NETCore.App 10.0.0 [/x/shared/Microsoft.NETCore.App]\n" +
		"EOF\n"

	// The child runs the full-stack case and nothing else. Widening this
	// pattern would let the child re-enter this test and fork forever.
	const childCase = "^TestFullStackDrivesTheRealDotnetExtractor$"

	// Every child is given -test.short explicitly, so what it does never
	// depends on the flags this parent happens to run under.
	runChild := func(t *testing.T, stubBody, require string, args ...string) (string, error) {
		t.Helper()
		stubDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(stubDir, "dotnet"), []byte(stubBody), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], append([]string{"-test.run", childCase, "-test.v"}, args...)...)
		cmd.Env = append(childEnv(require), "PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	mustContain := func(t *testing.T, out string, wants []string) {
		t.Helper()
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("child output does not contain %q. output:\n%s", want, out)
			}
		}
	}

	scenarios := []struct {
		name string
		stub string
		want []string
	}{
		{
			name: "dotnet cannot run at all",
			stub: stubRefusesToRun,
			want: []string{reasonNoNet8Runtime, "exit status 3", "stub dotnet refuses to run"},
		},
		{
			name: "dotnet runs and lists no 8.x runtime",
			stub: stubListsNoNet8,
			want: []string{reasonNoNet8Runtime, "found no line starting with", "Microsoft.NETCore.App 10.0.0"},
		},
	}

	// bothArms runs the enforced and the unset child over one stub and asserts
	// the first fails naming the variable and the second skips, both quoting
	// want.
	bothArms := func(t *testing.T, stub string, want []string, args ...string) {
		t.Helper()
		t.Run("the enforced run fails instead of skipping", func(t *testing.T) {
			out, err := runChild(t, stub, "1", args...)
			if err == nil {
				t.Fatalf("child exited zero, want a failure. output:\n%s", out)
			}
			mustContain(t, out, append([]string{envRequireDotnet, "forbids skipping"}, want...))
		})

		t.Run("the unset run skips and passes", func(t *testing.T) {
			out, err := runChild(t, stub, "", args...)
			if err != nil {
				t.Fatalf("child failed with %v, want a pass. output:\n%s", err, out)
			}
			mustContain(t, out, append([]string{"--- SKIP"}, want...))
		})
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			bothArms(t, s.stub, s.want, "-test.short=false")
		})
	}

	// The -short guard exists so the case pays for no dotnet subprocess it
	// cannot use the answer of. A stub that records being run is what turns
	// that into an assertion rather than a claim in a comment.
	t.Run("-short decides before dotnet is ever run", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "probed")
		stub := "#!/bin/sh\ntouch " + marker + "\n"

		bothArms(t, stub, []string{reasonShort}, "-test.short=true")

		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the stub dotnet ran under -short: os.Stat(%q) = %v, want a not-exist error", marker, err)
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
	// A -short run cannot use the probe's answer, so it never pays for the
	// subprocess and the first-run initialization dotnet may do behind it.
	short := testing.Short()
	net8OK := true
	// The probe's error and output are what tell a reader whether dotnet is
	// absent from PATH, failed when it ran, or ran fine and simply carries no
	// 8.x framework, and the enforced run reports nothing else about the
	// machine.
	var detailNoNet8Runtime string
	if !short {
		out, probeErr := exec.Command("dotnet", "--list-runtimes").CombinedOutput()
		listed := strings.TrimSpace(string(out))
		switch {
		case errors.Is(probeErr, exec.ErrNotFound):
			net8OK = false
			detailNoNet8Runtime = fmt.Sprintf("%s, and no dotnet to ask: %v", reasonNoNet8Runtime, probeErr)
		case probeErr != nil:
			net8OK = false
			detailNoNet8Runtime = fmt.Sprintf("%s, dotnet --list-runtimes failed with %v and printed %q", reasonNoNet8Runtime, probeErr, listed)
		case !listsNet8Runtime(listed):
			net8OK = false
			detailNoNet8Runtime = fmt.Sprintf("%s, dotnet --list-runtimes found no line starting with %q and listed %q", reasonNoNet8Runtime, net8RuntimePrefix, listed)
		}
	}
	if cause, fatal := dotnetSkipReason(short, net8OK, require); cause != causeRuns {
		reason := cause.reason()
		if cause == causeNoNet8Runtime {
			reason = detailNoNet8Runtime
		}
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
