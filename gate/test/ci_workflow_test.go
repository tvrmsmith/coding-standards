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
type ciStep struct {
	Name             string               `yaml:"name"`
	ID               string               `yaml:"id"`
	Uses             string               `yaml:"uses"`
	If               yaml.Node            `yaml:"if"`
	Run              string               `yaml:"run"`
	WorkingDirectory string               `yaml:"working-directory"`
	Env              map[string]yaml.Node `yaml:"env"`
}

// gateJobSteps is the gate job's steps in declaration order, read out of
// ci.yml. Order is part of what the cases below assert, because a guard naming
// a step that has not finished renders empty rather than failing.
func gateJobSteps(t *testing.T) []ciStep {
	t.Helper()

	body, err := os.ReadFile(ciWorkflow)
	if err != nil {
		t.Fatalf("reading the workflow this case reads as a contract at %s: %v", ciWorkflow, err)
	}

	var workflow struct {
		Jobs map[string]struct {
			Steps []ciStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(body, &workflow); err != nil {
		t.Fatalf("parsing the workflow this case reads as a contract at %s: %v", ciWorkflow, err)
	}

	job, ok := workflow.Jobs[gateJob]
	if !ok {
		t.Fatalf("%s declares no %q job, so this case cannot find the steps it pins", ciWorkflow, gateJob)
	}
	return job.Steps
}

// lintStep is the name: of the step that runs the personal preset with a
// blocking exit code, lintBinary and lintConfig are the binary it runs and the
// config it reads, both written relative to the repository root, and
// lintTarget is the package pattern that reaches the whole module. lintGuard
// is the whole if: it carries, naming both the toolchain step and the step
// that builds the binary, so a failed build skips the lint rather than reding
// it with a message pointing at a missing file.
const (
	lintStep        = "Lint the root module with the personal preset"
	lintBuildStepID = "gcl"
	lintBinary      = "./go/bin/tvrmsmith-gcl"
	lintConfig      = "./go/golangci.yml"
	lintTarget      = "./..."
	lintGuard       = "${{ !cancelled() && " + suiteGuardReference +
		" == 'success' && steps." + lintBuildStepID + ".outcome == 'success' }}"
)

// lintExitCodeFlag is the flag that turns the gate off. harness/lint-changed-go.sh
// passes it with 0 because it runs over other people's repositories, and the
// same flag on this step would leave every gosec finding in the module
// reported and CI green.
const lintExitCodeFlag = "--issues-exit-code"

// TestCIDeclaresTheBlockingLintStep reads ci.yml as the declarative contract it
// is and asserts the gate job still runs the personal preset over the whole
// root module with a blocking exit code. The step's argv is split into tokens
// and each one asserted for what it means, rather than snapshotted, because
// what matters is the binary, the config, the breadth and the absence of the
// exit-code flag. Renaming the step, narrowing it to gate/, or appending
// --issues-exit-code 0 each reds this case instead of quietly disarming the
// gosec sweep this branch cleared.
func TestCIDeclaresTheBlockingLintStep(t *testing.T) {
	job := gateJobSteps(t)

	var found, buildSteps int
	buildDeclared := false
	for i, step := range job {
		if step.ID == lintBuildStepID {
			buildSteps++
			buildDeclared = true
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

		argv := strings.Fields(step.Run)
		switch {
		case len(argv) == 0 || argv[0] != lintBinary:
			t.Errorf("%s: the %q step runs %q, want the first word to be %s, the binary go/build.sh writes with the personal plugin compiled in. Plain golangci-lint enables none of these rules",
				ciWorkflow, lintStep, step.Run, lintBinary)
		case len(argv) < 2 || argv[1] != "run":
			t.Errorf("%s: the %q step runs %q, want the `run` subcommand, which is the only one that reports findings",
				ciWorkflow, lintStep, step.Run)
		}
		if !slices.Contains(argv, lintTarget) {
			t.Errorf("%s: the %q step runs %q, want the %s pattern so every package in the root module is linted rather than one directory",
				ciWorkflow, lintStep, step.Run, lintTarget)
		}
		if !slices.Contains(argv, lintConfig) {
			t.Errorf("%s: the %q step runs %q, want --config %s so it reads the preset this repo maintains rather than golangci-lint's defaults",
				ciWorkflow, lintStep, step.Run, lintConfig)
		}
		for _, arg := range argv {
			if arg == lintExitCodeFlag || strings.HasPrefix(arg, lintExitCodeFlag+"=") {
				t.Errorf("%s: the %q step passes %s. The default is 1, and this step exists so a finding reds the build; with the flag every gosec finding in the module is reported and CI stays green",
					ciWorkflow, lintStep, arg)
			}
		}
	}

	if found != 1 {
		t.Errorf("%s: the %q job holds %d steps named %q, want exactly one. Without it nothing keeps a new exec.Command or os.ReadFile from reintroducing a G204 or G304 with CI green",
			ciWorkflow, gateJob, found, lintStep)
	}
	if buildSteps != 1 {
		t.Errorf("%s: the %q job declares %d steps with id: %s, want exactly one. The lint step's guard reads that id's outcome, which is false for a step that does not exist, so the lint would never run",
			ciWorkflow, gateJob, buildSteps, lintBuildStepID)
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
	job := gateJobSteps(t)

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
