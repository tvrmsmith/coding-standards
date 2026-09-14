package gate_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ciWorkflow is the path, relative to the gate module root, to the workflow
// that runs this suite. The file is a machine-consumed contract with GitHub
// Actions, and the assertion below reads it as one: it unmarshals the YAML and
// walks to the step that runs the suite rather than matching text against the
// raw file.
const ciWorkflow = "../../.github/workflows/ci.yml"

// gateJob is the key under jobs: whose steps run this package.
const gateJob = "gate"

// passCheckStep is the name: of the step that runs the suite and then proves
// every case the step's own loop names reported PASS. The name is how this file
// finds the step, so the script underneath it stays free to be rewritten as
// long as it still judges the log the same way.
const passCheckStep = "Run the gate suite and prove the full-stack cases ran"

// passLoopHeader opens the loop in the step's script that names one case per
// entry in realExtractorCases. passLoopNames reads the names back out of it so
// a case added to the Go slice and not to the workflow reds here rather than
// going unchecked on CI forever.
const passLoopHeader = "for name in"

// gateWorkingDir is the directory the step declares, relative to the repository
// root. `go test ./...` selects this module's packages only because it runs
// there.
const gateWorkingDir = "gate"

// passCheckGuard is the if: the step carries, and passCheckGuardStepID is the
// step it names. Comparing the whole expression is what makes `if: false` red,
// and requiring the id to be declared is what makes deleting `id: setup` off
// the toolchain step red too: without it the condition is false on every run
// and the check silently never executes.
const (
	passCheckGuard       = "${{ !cancelled() && steps.setup.outcome == 'success' }}"
	passCheckGuardStepID = "setup"
)

// TestCIDeclaresThePassCheckStep reads ci.yml as the declarative contract it is
// and asserts the gate job declares the PASS check step once, gated on a step
// that exists, running where `go test ./...` selects this module, with
// enforcement on. Every one of those can drift without any Go test noticing,
// because the step runs on the runner rather than here.
func TestCIDeclaresThePassCheckStep(t *testing.T) {
	body, err := os.ReadFile(ciWorkflow)
	if err != nil {
		t.Fatal(err)
	}

	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name             string         `yaml:"name"`
				ID               string         `yaml:"id"`
				If               any            `yaml:"if"`
				Run              string         `yaml:"run"`
				WorkingDirectory string         `yaml:"working-directory"`
				Env              map[string]any `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(body, &workflow); err != nil {
		t.Fatalf("%s: %v", ciWorkflow, err)
	}

	job, ok := workflow.Jobs[gateJob]
	if !ok {
		t.Fatalf("%s declares no %q job, so this case cannot find the step that runs the suite", ciWorkflow, gateJob)
	}

	var found, guardTargets int
	for _, step := range job.Steps {
		if step.ID == passCheckGuardStepID {
			guardTargets++
		}
		if step.Name != passCheckStep {
			continue
		}
		found++

		if guard := scalar(step.If); guard != passCheckGuard {
			t.Errorf("%s: the %q step carries if: %q, want %q. Any other expression is the check running when it should not or, worse, quietly not running",
				ciWorkflow, passCheckStep, guard, passCheckGuard)
		}
		names, want := passLoopNames(step.Run), slices.Sorted(slices.Values(realExtractorCases))
		if !slices.Equal(names, want) {
			t.Errorf("%s: the %q step's PASS loop names %v, want %v, the suite's own realExtractorCases. A case missing there runs on CI unproven and can stop running with the job green",
				ciWorkflow, passCheckStep, names, want)
		}
		if step.WorkingDirectory != gateWorkingDir {
			t.Errorf("%s: the %q step declares working-directory %q, want %q, which is where its `go test ./...` selects this module",
				ciWorkflow, passCheckStep, step.WorkingDirectory, gateWorkingDir)
		}

		// The env key is read through the suite's own parser rather than
		// compared to a literal, so the workflow and the suite agree on what
		// enables enforcement. Without it both real-toolchain cases skip and the
		// PASS loop reds for the wrong reason.
		value, set := step.Env[envRequireDotnet]
		raw := scalar(value)
		switch enforce, err := requireDotnet(raw, set); {
		case err != nil:
			t.Errorf("%s: the %q step sets %s=%q, which this suite rejects: %v",
				ciWorkflow, passCheckStep, envRequireDotnet, raw, err)
		case !enforce:
			t.Errorf("%s: the %q step does not set %s to a value that enables enforcement, so both real-toolchain cases would skip",
				ciWorkflow, passCheckStep, envRequireDotnet)
		}
	}

	if found != 1 {
		t.Errorf("%s: the %q job holds %d steps named %q, want exactly one",
			ciWorkflow, gateJob, found, passCheckStep)
	}
	if guardTargets != 1 {
		t.Errorf("%s: the %q job declares %d steps with id: %s, want exactly one. The PASS check's guard reads steps.%s.outcome, which is false for a step that does not exist, so the check would never run",
			ciWorkflow, gateJob, guardTargets, passCheckGuardStepID, passCheckGuardStepID)
	}
}

// scalar renders a YAML scalar the workflow schema allows to be written
// unquoted. Actions coerces `if: false` and `TIMEOUT: 12` to strings, so
// decoding them as strings would fail the whole unmarshal and red this case
// with a message about the file rather than about the PASS check.
func scalar(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// passLoopNames returns the case names the PASS loop iterates over, sorted.
// The loop header is one shell line broken across several with trailing
// backslashes, so the continuations are joined before the names are split out.
// An absent or unrecognised header returns nothing, which reds the comparison.
func passLoopNames(run string) []string {
	lines := strings.Split(run, "\n")
	var header string
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), passLoopHeader) {
			continue
		}
		for ; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			header += " " + strings.TrimSuffix(line, `\`)
			if !strings.HasSuffix(line, `\`) {
				break
			}
		}
		break
	}
	header = strings.TrimPrefix(strings.TrimSpace(header), passLoopHeader)
	header = strings.TrimSuffix(strings.TrimSpace(header), "do")
	header = strings.TrimSuffix(strings.TrimSpace(header), ";")
	return slices.Sorted(slices.Values(strings.Fields(header)))
}
