package gate_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// pointsFixture is the path, relative to gate/test/, to the C#
// method the dotnet extractor's own suite already scores at complexity 9.
// Copying it verbatim keeps this case's golden numbers tied to the real
// tool's fixture rather than to a value retyped by hand.
const pointsFixture = "../../dotnet/tests/Tvrmsmith.MetricGate.CSharp.Tests/fixtures/Points.cs"

// dotnetProject is the tool project this case packs and installs in place of
// the stub.
const dotnetProject = "../../dotnet/src/Tvrmsmith.MetricGate.CSharp"

// extractorPackage is the package id the tool project packs under, which is
// also the id the install resolves and the name of the .nupkg on disk.
const extractorPackage = "Tvrmsmith.MetricGate.CSharp"

// dotnetDir is the directory the pack and the install run in, relative to this
// package. dotnet/global.json sits at its root, so the muxer resolves that pin
// for both and one file governs which SDK builds the tool.
const dotnetDir = "../../dotnet"

// envRequireDotnet names the variable CI sets to forbid every skip route, so
// the cases that drive the real toolchain cannot lapse into a green skip.
const envRequireDotnet = "METRIC_GATE_REQUIRE_DOTNET"

// reasonShort is why the cases in realExtractorCases fail under -short once
// METRIC_GATE_REQUIRE_DOTNET forbids every skip route.
const reasonShort = "full-stack cases pack and install a dotnet tool, skipped with -short"

// reasonOffline is why dotnetFailure prepends its own paragraph when a dotnet
// step's output shows NU1301. The step that failed needed a package feed it
// could not reach, and naming that here keeps the paragraph identical at every
// dotnet step that reports it.
const reasonOffline = "NuGet could not reach a package feed (NU1301), which is a missing network rather than a broken gate. The full-stack cases restore packages from a feed and " + envRequireDotnet + "=1 forbids skipping them, so run them online or unset " + envRequireDotnet + "."

// reasonUnset is why those cases skip when nothing enforces it. It states the
// one thing a reader has to do to make them run, because they ask nothing of
// the machine before they decide.
var reasonUnset = "set " + envRequireDotnet + "=1 to run the full-stack cases"

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

// caseLabel renders a raw environment value for a subtest name. It exists
// because %q would put real double quotes in the name, and go test writes the
// name verbatim into the junit report, where a quote closes the name attribute
// early and the whole report stops parsing. The empty string gets a word of its
// own so its row still reads as itself.
func caseLabel(raw string) string {
	if raw == "" {
		return "empty"
	}
	return raw
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
		t.Run(fmt.Sprintf("%s=%s set=%v", envRequireDotnet, caseLabel(c.raw), c.set), func(t *testing.T) {
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

// TestDotnetFailureNamesAnOfflineRestore pins dotnetFailure against literal
// expected strings rather than against its own formatting, so a change to the
// function has to keep matching text a reader chose by hand.
func TestDotnetFailureNamesAnOfflineRestore(t *testing.T) {
	cases := []struct {
		name string
		step string
		err  error
		out  string
		want string
	}{
		{
			name: "NU1301 output gets the offline reason prepended",
			step: "dotnet pack",
			err:  errors.New("exit status 1"),
			out:  "error NU1301: Unable to load the service index for source https://api.nuget.org/v3/index.json.\n",
			want: "NuGet could not reach a package feed (NU1301), which is a missing network rather than a broken gate. The full-stack cases restore packages from a feed and METRIC_GATE_REQUIRE_DOTNET=1 forbids skipping them, so run them online or unset METRIC_GATE_REQUIRE_DOTNET.\n" +
				"dotnet pack: exit status 1\nerror NU1301: Unable to load the service index for source https://api.nuget.org/v3/index.json.\n",
		},
		{
			name: "a compile error passes through unchanged",
			step: "dotnet pack",
			err:  errors.New("exit status 1"),
			out:  "error CS1002: ; expected\n",
			want: "dotnet pack: exit status 1\nerror CS1002: ; expected\n",
		},
		{
			name: "empty output passes through unchanged",
			step: "dotnet tool install Tvrmsmith.MetricGate.CSharp 0.1.0",
			err:  errors.New("exit status 1"),
			out:  "",
			want: "dotnet tool install Tvrmsmith.MetricGate.CSharp 0.1.0: exit status 1\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dotnetFailure(c.step, c.err, []byte(c.out))
			if got != c.want {
				t.Errorf("dotnetFailure(%q, %v, %q) = %q, want %q", c.step, c.err, c.out, got, c.want)
			}
		})
	}
}

// realExtractorCases names every case that drives the real dotnet toolchain
// and so decides on METRIC_GATE_REQUIRE_DOTNET. The child selection and the
// per-case outcome lines the enforcement rows assert are both derived from it,
// so adding a case here is the one edit that puts it under those rows. ci.yml
// retypes the same names in the loop that greps the verbose log for each one's
// PASS line, and it does not read them from here, so a new case is added in
// both places by hand. TestCIDeclaresTheSuiteStep reads that script's own
// loop list and requires it to hold exactly these names, so a case added here
// and not there, or left there after a rename, reds rather than running on CI
// unproven.
//
// TestRequireDotnetDecidesTheFullStackOutcome makes a name listed here that no
// case implements fail, on every row rather than the SKIP ones alone, because
// caseBlock looks for the case's RUN line first and a name nothing implements
// produces no RUN line to find.
var realExtractorCases = []string{
	"TestFullStackDrivesTheRealDotnetExtractor",
	"TestFullStackScoresAReportCoverletWrote",
}

// requireRealDotnet is the one decision standing between a case and the real
// dotnet toolchain, and every entry point that drives the toolchain reaches it,
// because dotnetCmd is the only place in the package that builds a dotnet
// invocation and it calls this. The run has to say it means to drive the
// toolchain: one that does reds on a machine that cannot serve it, one that
// does not skips before the first dotnet call. Because it is that chokepoint,
// it also makes membership of realExtractorCases a precondition of reaching the
// toolchain, so a case the enforcement rows and the CI PASS loop do not cover
// reds on any machine rather than skipping unnoticed. Nothing here reads the
// machine, and calling it twice decides the same way.
func requireRealDotnet(t *testing.T) {
	t.Helper()
	top, _, _ := strings.Cut(t.Name(), "/")
	if !slices.Contains(realExtractorCases, top) {
		t.Fatalf("%s drives the real dotnet toolchain but is not in realExtractorCases, so it gets no enforcement row and no CI PASS grep. Add it there and to ci.yml's loop.", top)
	}
	if !enforceDotnet {
		t.Skip(reasonUnset)
	}
	if testing.Short() {
		t.Fatalf("%s=1 forbids skipping: %s", envRequireDotnet, reasonShort)
	}
}

// dotnetCmd is the only place this package builds a dotnet invocation, which is
// what turns requireRealDotnet from a convention every caller has to remember
// into the one route to the toolchain. A new call site reaches dotnet through
// here or not at all, and here has already decided.
func dotnetCmd(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()
	requireRealDotnet(t)
	//nolint:gosec // the literal "dotnet" on PATH with argv this package writes; requireRealDotnet above is the only gate that matters here
	cmd := exec.Command("dotnet", args...)
	cmd.Dir = dir
	return cmd
}

// dotnetFailure renders a dotnet step's failure as step, its error and the
// combined output on the next line, matching what a bare t.Fatalf already
// printed. When out contains NU1301, it prepends reasonOffline first, so the
// offline cause reads before the raw failure text instead of being mistaken
// for a broken gate.
func dotnetFailure(step string, err error, out []byte) string {
	msg := fmt.Sprintf("%s: %v\n%s", step, err, out)
	if strings.Contains(string(out), "NU1301") {
		return reasonOffline + "\n" + msg
	}
	return msg
}

// childCase selects those cases in a child and nothing else. Widening this
// pattern would let the child re-enter the forking tests and fork forever.
var childCase = "^(" + strings.Join(realExtractorCases, "|") + ")$"

// childOutcome is a result line a child test binary prints for one case. A row
// states the outcome once and reads the exit disposition off it through
// failing, so the two cannot disagree.
type childOutcome struct {
	line string
}

// failing reports whether reaching this outcome makes the child exit non-zero.
func (o childOutcome) failing() bool { return o.line == "FAIL" }

// The two result lines a child can print for a case these rows drive.
// outcomeFail is the one the enforcement fatal produces.
var (
	outcomeSkip = childOutcome{line: "SKIP"}
	outcomeFail = childOutcome{line: "FAIL"}
)

// caseBlock is the -test.v output one named case produced, from its RUN line
// up to the result line for the given outcome. Everything the case logged
// lands in there, so matching a reason against the block rather than against
// the whole child output is what stops one case's message from standing in for
// a sibling that took some other route entirely. A case that never ran, or ran
// and reached a different outcome, has no block and is an error.
func caseBlock(out, name string, outcome childOutcome) (string, error) {
	start := strings.Index(out, "=== RUN   "+name+"\n")
	if start < 0 {
		return "", fmt.Errorf("the child printed no RUN line for %s, so the case never started", name)
	}
	rest := out[start:]
	end := strings.Index(rest, "--- "+outcome.line+": "+name+" (")
	if end < 0 {
		return "", fmt.Errorf("the child printed no %q line for %s, so the case took some other route", outcome.line, name)
	}
	return rest[:end], nil
}

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
	//nolint:gosec // re-execs os.Args[0], this very test binary, with -test.run flags this file builds; running a child copy of itself is what the helper is for
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

// TestRequireDotnetDecidesTheFullStackOutcome runs the full-stack cases in a
// child copy of this test binary with METRIC_GATE_REQUIRE_DOTNET set and
// unset. They ask the machine nothing before they decide, so the variable and
// -short are the whole input, and only a real run proves the fatal route is
// reachable at all rather than dead behind a skip.
func TestRequireDotnetDecidesTheFullStackOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("forks child test binaries that each rebuild the gate, skipped with -short")
	}

	// Every row below asserts one block per name in realExtractorCases, so an
	// emptied slice would leave the rows asserting nothing at all.
	if len(realExtractorCases) == 0 {
		t.Fatal("realExtractorCases is empty, so every row below is vacuous")
	}

	// -test.short is passed explicitly in every row, so what a child does never
	// depends on the flags this parent happens to run under.
	const long, short = "-test.short=false", "-test.short=true"

	// outcome and wants are asserted against each case's own block, so every
	// name in realExtractorCases has to reach that outcome for its own
	// documented reason and no sibling's message can cover for it.
	cases := []struct {
		name    string
		require string
		short   string
		outcome childOutcome
		wants   []string
	}{
		{
			name:    "the unset run skips for the documented reason",
			short:   long,
			outcome: outcomeSkip,
			wants:   []string{reasonUnset},
		},
		{
			name:    "-short does not change the unset run",
			short:   short,
			outcome: outcomeSkip,
			wants:   []string{reasonUnset},
		},
		{
			name:    "-short fails when enforcement forbids the skip",
			require: "1",
			short:   short,
			outcome: outcomeFail,
			wants:   []string{envRequireDotnet + "=1 forbids skipping: " + reasonShort},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := runChild(t, c.require, "-test.run", childCase, "-test.v", c.short)
			if c.outcome.failing() && err == nil {
				t.Fatalf("child exited zero, want a failure. output:\n%s", out)
			}
			if !c.outcome.failing() && err != nil {
				t.Fatalf("child failed with %v, want a pass. output:\n%s", err, out)
			}
			for _, name := range realExtractorCases {
				block, err := caseBlock(out, name, c.outcome)
				if err != nil {
					t.Errorf("%v. output:\n%s", err, out)
					continue
				}
				for _, want := range c.wants {
					if !strings.Contains(block, want) {
						t.Errorf("%s reached %s but said nothing containing %q. what it printed:\n%s",
							name, c.outcome.line, want, block)
					}
				}
			}
		})
	}
}

// TestTestMainRefusesAnUnusableRequireDotnet runs this binary with nothing
// selected, so the only thing under test is TestMain's read of
// METRIC_GATE_REQUIRE_DOTNET. That guard is what stops a -run filter excluding
// the full-stack cases from leaving a typo undetected, and the parse it calls
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

// TestFullStackDrivesTheRealDotnetExtractor is one of the two cases in the
// suite that run the real dotnet tool extractor end to end instead of the
// stub, and they pin different halves of the contract. This one pins spans and
// complexity against the stub's numbers, and
// TestFullStackScoresAReportCoverletWrote pins the coverage report's format
// against the producer that writes it. Its fixture and golden are pinned to
// match fail_single_method's numbers, so if the real tool and the stub ever
// disagree about a span or a complexity, this is what catches it.
//
// The coverage report it hands the gate is still synthetic. coverlet.collector,
// the producer the C# extractor targets, writes its timestamp attribute as
// ten-digit epoch seconds, which is the one representation the gate reads, so
// building the report here rather than collecting one costs no coverage of the
// staleness rule. Coverage a real dotnet test run produced is what
// TestFullStackScoresAReportCoverletWrote scores, which is why this case can
// stay on a report it builds and keep its span numbers exact.
func TestFullStackDrivesTheRealDotnetExtractor(t *testing.T) {
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

// readFixture reads a file relative to gate/test/, failing the test rather
// than returning an error a caller might ignore.
func readFixture(t *testing.T, rel string) string {
	t.Helper()
	//nolint:gosec // rel is a checked-in fixture path spelled by the calling case, resolved relative to gate/test/ and reaching the dotnet fixtures at the repo root
	body, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// appendComment appends a trailing comment to line n of the file at rel,
// touching that one line without disturbing any other, and without removing
// a construct the real extractor would count towards complexity. count runs
// below len(lines) for a newline-terminated file, where strings.Split yields a
// trailing empty element that is not a line n can name.
func (f *fixture) appendComment(rel string, n int) {
	f.t.Helper()
	lines := strings.Split(f.read(rel), "\n")
	count := len(lines)
	if count > 0 && lines[count-1] == "" {
		count--
	}
	if n < 1 || n > count {
		f.t.Fatalf("%s holds %d lines, so it has no line %d to append a comment to", rel, count, n)
	}
	lines[n-1] += " // touched"
	f.write(rel, strings.Join(lines, "\n"))
}

// installRealExtractor builds a fresh metric-gate binary and packs and
// installs the real dotnet tool extractor beside it, in a directory separate
// from the shared stub-based binDir. It returns that directory.
func installRealExtractor(t *testing.T) string {
	t.Helper()
	requireRealDotnet(t)
	// A directory holding the gate and no extractor beside it is exactly what
	// this case needs before it installs the real tool into it.
	dir := gateOnlyDir(t)

	// `dotnet tool install` resolves the package id and version out of the
	// NuGet global packages folder once anything has extracted it there, and
	// the csproj pins one version forever. Sharing the machine's folder would
	// install run 1's build on every later run, so this case would verify a
	// stale extractor and keep passing after a real regression in the tool.
	// The tradeoff is that this fresh folder makes dotnet pack restore from a
	// feed on every run, and dotnetFailure names that cause when it fails
	// offline instead of leaving NU1301 to read as a broken gate.
	nugetPackages := t.TempDir()
	// Both commands run under dotnet/, so dotnet/global.json is an ancestor
	// and the muxer resolves the SDK pin the same way here as it does for a
	// developer packing by hand. Every path handed to them is absolute for
	// that reason: the working directory is the pin's, not this package's.
	dotnet := func(args ...string) *exec.Cmd {
		cmd := dotnetCmd(t, dotnetDir, args...)
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
		t.Fatal(dotnetFailure("dotnet pack", err, out))
	}

	// The install names the version the pack just produced and reads a config
	// whose source list is packDir and nothing else. Either alone leaves a
	// hole: --add-source appends to the machine's sources rather than replacing
	// them, and an unversioned install takes the highest version any source
	// offers, so an id this repo has never published could be claimed on
	// nuget.org and installed here instead of the tool under test.
	version := packedVersion(t, packDir)
	install := dotnet("tool", "install",
		"--tool-path", dir,
		"--configfile", localFeedConfig(t, packDir),
		"--version", version,
		extractorPackage)
	if out, err := install.CombinedOutput(); err != nil {
		t.Fatal(dotnetFailure(fmt.Sprintf("dotnet tool install %s %s", extractorPackage, version), err, out))
	}
	return dir
}

// packedVersion is the version of the one extractor package in packDir, read
// off the file dotnet pack just wrote so the csproj stays the only place the
// version is stated.
func packedVersion(t *testing.T, packDir string) string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(packDir, extractorPackage+".*.nupkg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("dotnet pack wrote %d %s packages into %s, want exactly one: %v",
			len(found), extractorPackage, packDir, found)
	}
	name := filepath.Base(found[0])
	return strings.TrimSuffix(strings.TrimPrefix(name, extractorPackage+"."), ".nupkg")
}

// localFeedConfig writes a NuGet config whose only source is packDir and
// returns its path. It is written into a temporary directory rather than the
// repository, which carries no NuGet config on purpose, and <clear/> is what
// drops the machine's own sources instead of adding to them.
func localFeedConfig(t *testing.T, packDir string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "NuGet.config")
	body := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key="pack-output" value="` + packDir + `" />
  </packageSources>
</configuration>
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
