package gate_test

import (
	"os"
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
// every case in realExtractorCases reported PASS. The name is how this file
// finds the step, so the script underneath it stays free to be rewritten as
// long as it still judges the log the same way. That the loop names both cases
// is proven by CI itself going green with both reporting PASS, not here.
const passCheckStep = "Run the gate suite and prove the full-stack cases ran"

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
				Name             string            `yaml:"name"`
				ID               string            `yaml:"id"`
				If               string            `yaml:"if"`
				WorkingDirectory string            `yaml:"working-directory"`
				Env              map[string]string `yaml:"env"`
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

		if step.If != passCheckGuard {
			t.Errorf("%s: the %q step carries if: %q, want %q. Any other expression is the check running when it should not or, worse, quietly not running",
				ciWorkflow, passCheckStep, step.If, passCheckGuard)
		}
		if step.WorkingDirectory != gateWorkingDir {
			t.Errorf("%s: the %q step declares working-directory %q, want %q, which is where its `go test ./...` selects this module",
				ciWorkflow, passCheckStep, step.WorkingDirectory, gateWorkingDir)
		}

		// The env key is read through the suite's own parser rather than
		// compared to a literal, so the workflow and the suite agree on what
		// enables enforcement. Without it both real-toolchain cases skip and the
		// PASS loop reds for the wrong reason.
		raw, set := step.Env[envRequireDotnet]
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
