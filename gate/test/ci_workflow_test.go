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

// gateWorkingDir is the directory the step declares, relative to the repository
// root, and the empty string is the root itself. It has to be the root: the
// module covers internal/ as well as gate/, and `go test ./...` selects the
// packages under the directory it runs in, so declaring `gate` here would skip
// the shared packages without failing.
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
// enforcement on and the reviewed script underneath. Every one of those can
// drift without any Go test noticing, because the step runs on the runner
// rather than here.
func TestCIDeclaresTheSuiteStep(t *testing.T) {
	body, err := os.ReadFile(ciWorkflow)
	if err != nil {
		t.Fatalf("reading the workflow this case reads as a contract at %s: %v", ciWorkflow, err)
	}

	// if: and the env values are decoded as raw nodes rather than as Go values.
	// Actions hands the runner the source text, so `01` reaches the step as the
	// string 01, while decoding it as a Go value yields the integer 1 and hides
	// a value this suite rejects. Node.Value is that source text, and it also
	// keeps `if: false` and an integer env value from failing the unmarshal.
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name             string               `yaml:"name"`
				ID               string               `yaml:"id"`
				Uses             string               `yaml:"uses"`
				If               yaml.Node            `yaml:"if"`
				Run              string               `yaml:"run"`
				WorkingDirectory string               `yaml:"working-directory"`
				Env              map[string]yaml.Node `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(body, &workflow); err != nil {
		t.Fatalf("parsing the workflow this case reads as a contract at %s: %v", ciWorkflow, err)
	}

	job, ok := workflow.Jobs[gateJob]
	if !ok {
		t.Fatalf("%s declares no %q job, so this case cannot find the step that runs the suite", ciWorkflow, gateJob)
	}

	if len(realExtractorCases) == 0 {
		t.Fatal("realExtractorCases is empty, so the name check below would pass over a script naming nothing")
	}

	var found, guardTargets int
	guardDeclared := false
	for i, step := range job.Steps {
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
