package gate_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ciWorkflow is the path, relative to this package, to the workflow
// that runs this suite. The file is a machine-consumed contract with GitHub
// Actions, and the assertion below reads it as one: it unmarshals the YAML and
// walks to the step that runs the suite rather than matching text against the
// raw file.
const ciWorkflow = "../../.github/workflows/ci.yml"

// gateJob is the key under jobs: whose steps run this package.
const gateJob = "gate"

// suiteStep is the name: of the step that runs the suite and then proves
// every case the step's own loop names reported PASS. The name is how this
// file finds the step.
const suiteStep = "Run the gate suite and prove the full-stack cases ran"

// vetScript is the run: of the step that vets the module. The step carries no
// name:, so its script is how this file finds it.
const vetScript = "go vet ./..."

// gateWorkingDir is the directory each of those steps declares, relative to the
// repository root, and the empty string is the root itself. It has to be the
// root: the module covers internal/ as well as gate/, and both `go test ./...`
// and `go vet ./...` select the packages under the directory they run in, so
// declaring `gate` would skip the shared packages without failing.
const gateWorkingDir = ""

// suiteGuardStepID is the step the guard names, suiteGuardReference is
// how the guard expression reads that step's outcome, and suiteGuard is the
// whole if: the step carries, built from the other two so the id is stated
// once. Comparing the whole expression is what makes `if: false` red.
//
// Three separate holes, and the assertions below close each one. A guard naming
// a step no job declares renders false on every run, so exactly one step must
// carry the id. A guard naming a step that runs later renders empty for the
// same reason, so the step declaring the id must come first. A guard naming a
// step that installs something else renders true while it stops meaning the Go
// toolchain installed, so the step carrying the id must run
// suiteGuardStepUses.
const (
	suiteGuardStepID    = "setup"
	suiteGuardStepUses  = "actions/setup-go@"
	suiteGuardReference = "steps." + suiteGuardStepID + ".outcome"
	suiteGuard          = "${{ !cancelled() && " + suiteGuardReference + " == 'success' }}"
)

// ciStep is one step of a job, decoded from ci.yml. if: and the env values are
// decoded as raw nodes rather than as Go values. Actions hands the runner the
// source text, so `01` reaches the step as the string 01, while decoding it as
// a Go value yields the integer 1 and hides a value this suite rejects.
// Node.Value is that source text, and it also keeps `if: false` and an integer
// env value from failing the unmarshal.
// continue-on-error is decoded here because it turns a red step green with
// nothing else in the file changing, on the step or on the whole job, which is
// the cheapest way to disarm the lint step below.
type ciStep struct {
	Name             string               `yaml:"name"`
	ID               string               `yaml:"id"`
	Uses             string               `yaml:"uses"`
	If               yaml.Node            `yaml:"if"`
	Run              string               `yaml:"run"`
	WorkingDirectory string               `yaml:"working-directory"`
	ContinueOnError  yaml.Node            `yaml:"continue-on-error"`
	Env              map[string]yaml.Node `yaml:"env"`
	With             map[string]yaml.Node `yaml:"with"`
}

// with is the source text of one of the step's with: inputs, empty when the
// step declares no such input. Actions hands the runner the source text, so
// the raw node is read for the same reason If and Env are.
func (s ciStep) with(key string) string {
	return s.With[key].Value
}

// ciJob is one job of the workflow. Steps are in declaration order, which is
// part of what the cases below assert, because a guard naming a step that has
// not finished renders empty rather than failing.
type ciJob struct {
	ContinueOnError yaml.Node `yaml:"continue-on-error"`
	Steps           []ciStep  `yaml:"steps"`
}

// gateJobSpec is the gate job, read out of ci.yml.
func gateJobSpec(t *testing.T) ciJob {
	t.Helper()

	body, err := os.ReadFile(ciWorkflow)
	if err != nil {
		t.Fatalf("reading the workflow this case reads as a contract at %s: %v", ciWorkflow, err)
	}

	var workflow struct {
		Jobs map[string]ciJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(body, &workflow); err != nil {
		t.Fatalf("parsing the workflow this case reads as a contract at %s: %v", ciWorkflow, err)
	}

	job, ok := workflow.Jobs[gateJob]
	if !ok {
		t.Fatalf("%s declares no %q job, so this case cannot find the steps it pins", ciWorkflow, gateJob)
	}
	return job
}

// lintStep is the name: of the step that runs the personal preset with a
// blocking exit code. lintGuard is the whole if: it carries, naming both the
// toolchain step and the step that builds the binary, so a failed build skips
// the lint rather than reding it with a message pointing at a missing file.
//
// lintScript is the step's run: line, held byte for byte rather than picked
// apart into flags. Every way of disarming this step is an edit to that line:
// a `|| true` or `; exit 0` suffix swallows the exit status, a dropped
// `--config` runs golangci-lint's own plugin-free default set, `--disable=gosec`
// or `--default=none` empties it, `--new-from-rev` hides everything already on
// disk, `--issues-exit-code 0` is the flag harness/lint-changed-go.sh passes
// because it runs over other people's repositories, and narrowing `./...` to
// one directory leaves the rest of the module unlinted. A predicate per hole
// closes the holes someone thought of; the snapshot closes the rest.
const (
	lintStep        = "Lint the root module with the personal preset"
	lintBuildStepID = "gcl"
	lintGuard       = "${{ !cancelled() && " + suiteGuardReference +
		" == 'success' && steps." + lintBuildStepID + ".outcome == 'success' }}"
	lintScript = "./go/bin/tvrmsmith-gcl run --config ./go/golangci.yml --output.text.print-issued-lines=false ./..."
)

// The build step installs the golangci-lint the plugin pins, and that version's
// own go directive is ahead of the root module's. Without a setup-go declaring
// the plugin module's Go first, `go install` switches toolchains and pulls a
// ~90MB zip from the module proxy on every run, which is the TLS handshake
// timeout that failed this job. So the plugin toolchain step is pinned here by
// the file it reads its version from, and the step must run before the build
// and carry no id:, because the suite's guard reads steps.setup and must keep
// meaning the root install rather than this one.
const (
	pluginGoVersionKey  = "go-version-file"
	pluginGoVersionFile = "go/plugin/go.mod"
	pluginGoCacheKey    = "cache-dependency-path"
	pluginGoCacheFile   = "go/plugin/go.sum"
)

// TestCIDeclaresTheBlockingLintStep reads ci.yml as the declarative contract it
// is and asserts the gate job still runs the personal preset over the whole
// root module with a blocking exit code, off a binary built on the Go the
// plugin module declares. Renaming the step, narrowing it to gate/, marking it
// continue-on-error, editing its command in any way or dropping the plugin
// toolchain install ahead of the build reds this case instead of quietly
// disarming the gosec sweep this branch cleared.
func TestCIDeclaresTheBlockingLintStep(t *testing.T) {
	job := gateJobSpec(t)
	assertFailureIsFatal(t, job.ContinueOnError, fmt.Sprintf("the %q job", gateJob))

	var found, buildSteps, pluginToolchains int
	buildDeclared := false
	for i, step := range job.Steps {
		if strings.HasPrefix(step.Uses, suiteGuardStepUses) && step.with(pluginGoVersionKey) == pluginGoVersionFile {
			pluginToolchains++
			if buildDeclared {
				t.Errorf("%s: step %d of the %q job installs the %s toolchain after the step with id: %s, which is the step that needs it. The build runs on whatever Go came before it and downloads a toolchain from the module proxy",
					ciWorkflow, i+1, gateJob, pluginGoVersionFile, lintBuildStepID)
			}
			if step.ID != "" {
				t.Errorf("%s: step %d of the %q job installs the %s toolchain and declares id: %s. It must carry no id, because %s is read by the suite step's guard and has to keep meaning the root Go install",
					ciWorkflow, i+1, gateJob, pluginGoVersionFile, step.ID, suiteGuardReference)
			}
			if cache := step.with(pluginGoCacheKey); cache != pluginGoCacheFile {
				t.Errorf("%s: step %d of the %q job installs the %s toolchain with %s: %q, want %q, the sum file beside the go.mod it reads",
					ciWorkflow, i+1, gateJob, pluginGoVersionFile, pluginGoCacheKey, cache, pluginGoCacheFile)
			}
		}
		if step.ID == lintBuildStepID {
			buildSteps++
			buildDeclared = true
			assertFailureIsFatal(t, step.ContinueOnError, fmt.Sprintf("the step with id: %s", lintBuildStepID))
		}
		if step.Name != lintStep {
			continue
		}
		found++

		if !buildDeclared {
			t.Errorf("%s: the %q step is declared before any step with id: %s, so its guard reads an outcome that is still empty and the step skips with the job green",
				ciWorkflow, lintStep, lintBuildStepID)
		}
		if guard := step.If.Value; guard != lintGuard {
			t.Errorf("%s: step %d, the %q step, carries if: %q, want %q. Any other expression is the gate running when it should not or, worse, quietly not running",
				ciWorkflow, i+1, lintStep, guard, lintGuard)
		}
		if step.WorkingDirectory != gateWorkingDir {
			t.Errorf("%s: the %q step declares working-directory %q, want %q, the repository root. The module covers internal/gitscope and internal/srcpath as well as gate/, and the pattern selects the packages under the directory the step runs in, so declaring `gate` would leave those two unlinted without failing",
				ciWorkflow, lintStep, step.WorkingDirectory, gateWorkingDir)
		}
		assertFailureIsFatal(t, step.ContinueOnError, fmt.Sprintf("the %q step", lintStep))
		if step.Run != lintScript {
			t.Errorf("%s: the %q step's run: line is not the reviewed one. Update lintScript once the new command is what this repo wants to run, and check it still blocks on a finding. got:\n%s\nwant:\n%s",
				ciWorkflow, lintStep, step.Run, lintScript)
		}
	}

	if found != 1 {
		t.Errorf("%s: the %q job holds %d steps named %q, want exactly one. Without it nothing keeps a new exec.Command or os.ReadFile from reintroducing a G204 or G304 with CI green",
			ciWorkflow, gateJob, found, lintStep)
	}
	if pluginToolchains != 1 {
		t.Errorf("%s: the %q job holds %d steps installing the Go %s declares, want exactly one ahead of the step with id: %s. Without it that step's `go install` of the pinned golangci-lint sees an older toolchain and fetches a ~90MB zip from the module proxy on every run",
			ciWorkflow, gateJob, pluginToolchains, pluginGoVersionFile, lintBuildStepID)
	}
	if buildSteps != 1 {
		t.Errorf("%s: the %q job declares %d steps with id: %s, want exactly one. The lint step's guard reads that id's outcome, which is false for a step that does not exist, so the lint would never run",
			ciWorkflow, gateJob, buildSteps, lintBuildStepID)
	}
}

// assertFailureIsFatal requires that continue-on-error is absent from what the
// node was decoded off. Actions defaults it to false, and any value at all is
// worth reding here: `true` turns a finding green outright, and an expression
// renders on the runner where this suite cannot see what it came to.
func assertFailureIsFatal(t *testing.T, continueOnError yaml.Node, what string) {
	t.Helper()
	if continueOnError.Value != "" {
		t.Errorf("%s: %s declares continue-on-error: %s. The lint step exists so a gosec finding reds the build, and this key leaves the job green whatever it reports",
			ciWorkflow, what, continueOnError.Value)
	}
}

// suiteScript is the step's run: script, held here as an intentional
// snapshot of a machine-consumed declarative artifact rather than grepped for
// tokens. Nothing in this suite executes the script, so a snapshot is what
// makes every edit to it deliberate: a dropped case name, a deleted trailing
// exit 1, a rewritten loop, a narrowed `go test` pattern, each reds this case
// until someone updates the constant to match.
const suiteScript = `set -euo pipefail
go test ./... -count=1 -v -timeout 12m | tee "$RUNNER_TEMP/gate-tests.txt"
# One name per case in the suite's realExtractorCases list. The names
# are retyped here rather than read out of the Go source, so add a new
# case in both places. TestCIDeclaresTheSuiteStep holds this whole
# script as a snapshot constant and compares it byte for byte, so no
# edit here lands without a deliberate edit there. What this loop
# itself catches is a name listed here that stopped reporting PASS.
# The pattern is anchored because Go's verbose output always follows
# the name with a space and a parenthesised duration, so an
# unanchored match would let a longer name that starts with this one
# stand in for it.
# Every missing name is collected and reported together, so one run
# names the whole gap rather than the first case alphabetically.
# grep's status is read rather than tested, because 1 is "no match"
# while 2 or more is grep failing to read the log at all, which proves
# nothing about the name and must not be reported as a case that
# did not run.
missing=""
for name in TestFullStackDrivesTheRealDotnetExtractor \
            TestFullStackScoresAReportCoverletWrote; do
  status=0
  grep -qE -- "^--- PASS: $name \(" "$RUNNER_TEMP/gate-tests.txt" || status=$?
  if [ "$status" -ge 2 ]; then
    echo "grep exited $status reading $RUNNER_TEMP/gate-tests.txt, so this" >&2
    echo "step proved nothing about whether $name ran." >&2
    exit "$status"
  fi
  if [ "$status" -eq 1 ]; then
    missing="$missing $name"
  fi
done
if [ -n "$missing" ]; then
  echo "The gate suite passed without these cases reporting PASS:$missing" >&2
  echo "A case that drives the real dotnet toolchain did not run. Either" >&2
  echo "the go test line above no longer selects its package, the case" >&2
  echo "was renamed, or enforcement is off." >&2
  exit 1
fi
`

// TestCIDeclaresTheSuiteStep reads ci.yml as the declarative contract it is
// and asserts the gate job declares the PASS check step once, gated on a step
// declared ahead of it, running where `go test ./...` selects this module, with
// enforcement on and the reviewed script underneath. It holds the vet step to
// the same directory for the same reason. Every one of those can
// drift without any Go test noticing, because the step runs on the runner
// rather than here.
func TestCIDeclaresTheSuiteStep(t *testing.T) {
	job := gateJobSpec(t).Steps

	if len(realExtractorCases) == 0 {
		t.Fatal("realExtractorCases is empty, so the name check below would pass over a script naming nothing")
	}

	var found, vetFound, guardTargets int
	guardDeclared := false
	for i, step := range job {
		declaredBefore := guardDeclared
		if step.ID == suiteGuardStepID {
			guardTargets++
			guardDeclared = true
			if !strings.HasPrefix(step.Uses, suiteGuardStepUses) {
				t.Errorf("%s: step %d of the %q job declares id: %s but runs %q, want an action beginning %s. The guard stays true over any action carrying the id, so on another one it renders success while it has stopped meaning the Go toolchain installed",
					ciWorkflow, i+1, gateJob, suiteGuardStepID, step.Uses, suiteGuardStepUses)
			}
		}
		guard := step.If.Value
		if !declaredBefore && strings.Contains(guard, suiteGuardReference) {
			t.Errorf("%s: step %d of the %q job (%q) reads %s, which no earlier step declares id: %s for. The expression renders empty for a step that has not finished, its own step included, so the guard is false and the step skips with the job green",
				ciWorkflow, i+1, gateJob, step.Name, suiteGuardReference, suiteGuardStepID)
		}
		if strings.TrimSpace(step.Run) == vetScript {
			vetFound++
			if step.WorkingDirectory != gateWorkingDir {
				t.Errorf("%s: the `%s` step declares working-directory %q, want %q, the repository root, which is where it vets internal/gitscope and internal/srcpath rather than gate/ alone",
					ciWorkflow, vetScript, step.WorkingDirectory, gateWorkingDir)
			}
		}

		if step.Name != suiteStep {
			continue
		}
		found++

		if guard != suiteGuard {
			t.Errorf("%s: the %q step carries if: %q, want %q. Any other expression is the check running when it should not or, worse, quietly not running",
				ciWorkflow, suiteStep, guard, suiteGuard)
		}
		if step.Run != suiteScript {
			t.Errorf("%s: the %q step's run: script is not the reviewed one. Update suiteScript once the new script is what this repo wants to run. got:\n%s\nwant:\n%s",
				ciWorkflow, suiteStep, step.Run, suiteScript)
		}

		// The snapshot above says ci.yml still carries the reviewed script. It
		// says nothing about which cases that script names, so the loop's own
		// list is read out of the step's script and compared with the suite's
		// slice as a set, both directions. Containment one way would miss a name
		// left in the script after a half-applied rename, which CI then greps for
		// and never finds.
		switch looped, err := scriptLoopNames(step.Run); {
		case err != nil:
			t.Errorf("%s: the %q step's script: %v", ciWorkflow, suiteStep, err)
		default:
			wantNames := slices.Sorted(slices.Values(realExtractorCases))
			slices.Sort(looped)
			if !slices.Equal(looped, wantNames) {
				t.Errorf("%s: the %q step's loop walks %v, want exactly the suite's realExtractorCases %v. A name the suite no longer defines makes CI grep for a case that can never report PASS, and a case the suite defines but the loop omits runs on CI unproven",
					ciWorkflow, suiteStep, looped, wantNames)
			}
		}
		if step.WorkingDirectory != gateWorkingDir {
			t.Errorf("%s: the %q step declares working-directory %q, want %q, the repository root, which is where its `go test ./...` reaches every package in the module rather than gate/ alone",
				ciWorkflow, suiteStep, step.WorkingDirectory, gateWorkingDir)
		}

		// The env key is read through the suite's own parser rather than
		// compared to a literal, so the workflow and the suite agree on what
		// enables enforcement. Without it both real-toolchain cases skip and the
		// PASS loop reds for the wrong reason.
		value, set := step.Env[envRequireDotnet]
		raw := value.Value
		switch enforce, err := requireDotnet(raw, set); {
		case err != nil:
			t.Errorf("%s: the %q step sets %s=%q, which this suite rejects: %v",
				ciWorkflow, suiteStep, envRequireDotnet, raw, err)
		case !enforce:
			t.Errorf("%s: the %q step does not set %s to a value that enables enforcement, so both real-toolchain cases would skip",
				ciWorkflow, suiteStep, envRequireDotnet)
		}
	}

	if found != 1 {
		t.Errorf("%s: the %q job holds %d steps named %q, want exactly one",
			ciWorkflow, gateJob, found, suiteStep)
	}
	if vetFound != 1 {
		t.Errorf("%s: the %q job holds %d steps running `%s`, want exactly one. Without it nothing pins where vet runs, and a step declaring working-directory: gate would stop vetting the shared packages with nothing red",
			ciWorkflow, gateJob, vetFound, vetScript)
	}
	if guardTargets != 1 {
		t.Errorf("%s: the %q job declares %d steps with id: %s, want exactly one. The PASS check's guard reads %s, which is false for a step that does not exist, so the check would never run",
			ciWorkflow, gateJob, guardTargets, suiteGuardStepID, suiteGuardReference)
	}
}

// scriptLoopNames is the case names the pass-check script's loop walks, taken
// from the list between `for name in ` and the `; do` that closes it. The line
// continuations are dropped and the rest split on whitespace, so a name is a
// whole token of the list rather than a substring of the script, and the two
// lists can be compared as sets in both directions.
func scriptLoopNames(script string) ([]string, error) {
	const openLoop, closeLoop = "for name in ", "; do"
	start := strings.Index(script, openLoop)
	if start < 0 {
		return nil, fmt.Errorf("it declares no %q loop, so it names no case at all", openLoop)
	}
	rest := script[start+len(openLoop):]
	end := strings.Index(rest, closeLoop)
	if end < 0 {
		return nil, fmt.Errorf("its %q list never closes with %q", openLoop, closeLoop)
	}
	return strings.Fields(strings.ReplaceAll(rest[:end], "\\", " ")), nil
}
