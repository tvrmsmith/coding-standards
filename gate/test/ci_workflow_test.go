package gate_test

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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
// every case in realExtractorCases reported PASS. The name is how this file
// finds the step, so the script underneath it stays free to be rewritten as
// long as it still judges the log the same way.
const passCheckStep = "Run the gate suite and prove the full-stack cases ran"

// TestCIPassCheckRedsTheJob runs the gate job's step out of ci.yml under bash,
// so what is under test is what the check does rather than which tokens its
// script contains. The runner's side of the contract is supplied here: bash,
// a RUNNER_TEMP to write the log into, and a `go` on PATH standing in for the
// toolchain, which is what lets the case choose the verbose log the check then
// has to judge.
//
// Membership is proven here too, in both directions and per name. Withholding
// one case's PASS line has to red the job, which is what a name missing from
// the check's own list would silently allow, and a log carrying exactly the
// names in realExtractorCases has to pass, which is what a check still waiting
// on a renamed or deleted case would not.
func TestCIPassCheckRedsTheJob(t *testing.T) {
	script := passCheckScript(t)

	// Every red-path row below is generated from realExtractorCases, so an
	// emptied slice would leave this case asserting only that a green log
	// passes, which an emptied shell loop also satisfies.
	if len(realExtractorCases) == 0 {
		t.Fatal("realExtractorCases is empty, so every membership row this case generates is vacuous")
	}

	t.Run("every case reported PASS, so the check lets the job through", func(t *testing.T) {
		assertPassCheck(t, script, verboseLog(realExtractorCases), 0, 0, nil)
	})

	// A log naming nothing has to red. That is what proves the check's own list
	// is non-empty, which no row generated from realExtractorCases can show:
	// a check that demanded no names at all would pass every other red row's
	// input too.
	t.Run("a log naming no case at all reds the job", func(t *testing.T) {
		assertPassCheck(t, script, verboseLog(nil), 0, 1, nil)
	})

	for i, name := range realExtractorCases {
		withheld := slices.Concat(realExtractorCases[:i], realExtractorCases[i+1:])
		renamed := slices.Clone(realExtractorCases)
		renamed[i] += "Twice"

		t.Run(name+"/no PASS line reds the job and is named", func(t *testing.T) {
			assertPassCheck(t, script, verboseLog(withheld), 0, 1, []string{name})
		})
		t.Run(name+"/a longer name that starts with it does not stand in for it", func(t *testing.T) {
			assertPassCheck(t, script, verboseLog(renamed), 0, 1, []string{name})
		})
		// A case that ran and skipped is the outcome the whole mechanism exists
		// to catch: enforcement off, or the env key drifted, and the suite still
		// reports ok. It prints a RUN line like a passing case does, so a check
		// keyed on RUN rather than on the result line would let it through.
		t.Run(name+"/a case that ran and skipped reds the job and is named", func(t *testing.T) {
			assertPassCheck(t, script, skippedLog(realExtractorCases, name), 0, 1, []string{name})
		})
	}

	// goExit is 2 rather than 1 so the status the step ends on is traceable to
	// the test run. set -o pipefail carries it out of the `go test | tee`
	// pipeline, which is the mechanism under test here, and an exit 1 would be
	// indistinguishable from the PASS loop's own failure path.
	t.Run("a failing test run reds the job even with every PASS line present", func(t *testing.T) {
		assertPassCheck(t, script, verboseLog(realExtractorCases), 2, 2, nil)
	})
}

// assertPassCheck runs the step's script over log with a `go` that exits
// goExit, and requires the script to end on wantExit. Stating the status rather
// than a bare pass or fail is what keeps bash missing from PATH, or the script
// dying before the PASS loop, from standing in for the loop's own `exit 1`.
// wantExit 0 is the job going through.
func assertPassCheck(t *testing.T, script, log string, goExit, wantExit int, names []string) {
	t.Helper()

	stdout, stderr, err := runPassCheck(t, script, log, goExit)
	out := fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr)

	if wantExit == 0 {
		if err != nil {
			t.Fatalf("the check failed with %v, want it to let the job through. %s", err, out)
		}
		return
	}

	if err == nil {
		t.Fatalf("the check exited zero, want a failure. %s", out)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("the check ended with %v rather than a nonzero exit status, so nothing here proves the script ran at all. %s", err, out)
	}
	if exit.ExitCode() != wantExit {
		t.Fatalf("the check exited %d, want %d. Another status is the script reddening for some reason other than the one this row drives. %s",
			exit.ExitCode(), wantExit, out)
	}
	for _, name := range names {
		if !strings.Contains(stderr, name) {
			t.Errorf("the check reds without naming %s, so the log says nothing about which case did not run. %s", name, out)
		}
	}
}

// verboseLog is what `go test -v` writes for a run in which every named case
// passed. It is the input the PASS check reads, and building it here is what
// lets a case withhold one name's result without running the suite.
func verboseLog(passed []string) string {
	return caseLog(passed, "")
}

// skippedLog is verboseLog with one name reporting SKIP instead of PASS. A
// skipped case still prints its RUN line and still leaves the package ok, which
// is the shape a run with enforcement off writes.
func skippedLog(names []string, skipped string) string {
	return caseLog(names, skipped)
}

func caseLog(names []string, skipped string) string {
	var b strings.Builder
	for _, name := range names {
		outcome := "PASS"
		if name == skipped {
			outcome = "SKIP"
		}
		fmt.Fprintf(&b, "=== RUN   %s\n--- %s: %s (1.23s)\n", name, outcome, name)
	}
	b.WriteString("PASS\nok  \tgithub.com/tvrmsmith/coding-standards/gate/test\t1.234s\n")
	return b.String()
}

// runPassCheck executes the workflow step's own script under bash with a `go`
// on PATH that replays log and exits goExit. It returns both of the script's
// streams, stderr being where the check reports a case that did not run and
// stdout the tee'd log it judged, together with its exit error.
func runPassCheck(t *testing.T, script, log string, goExit int) (string, string, error) {
	t.Helper()

	runnerTemp := t.TempDir()
	logPath := filepath.Join(runnerTemp, "go-test-output.txt")
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	stub := "#!/bin/sh\ncat " + logPath + "\nexit " + strconv.Itoa(goExit) + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RUNNER_TEMP="+runnerTemp)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// passCheckScript is the run script of the gate job's PASS check step, found
// by the step's name rather than by anything in its body. It fails the test
// rather than returning empty when the job or the step cannot be found, so a
// workflow restructure reds here instead of leaving the assertions above with
// nothing to say.
func passCheckScript(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile(ciWorkflow)
	if err != nil {
		t.Fatal(err)
	}

	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
				If   string `yaml:"if"`
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

	var found []string
	var passCheckIf string
	conditions := map[string]bool{}
	for _, step := range job.Steps {
		if step.If != "" {
			conditions[step.If] = true
		}
		if step.Name == passCheckStep {
			found = append(found, step.Run)
			passCheckIf = step.If
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: the %q job holds %d steps named %q, want exactly one",
			ciWorkflow, gateJob, len(found), passCheckStep)
	}

	// Everything below runs the script directly, so nothing else here would
	// notice the step being gated off. A condition the step does not share with
	// its siblings is the check quietly ceasing to run while every assertion
	// stays green.
	if passCheckIf == "" {
		t.Fatalf("%s: the %q step carries no if:, so it no longer shares its siblings' guard %v",
			ciWorkflow, passCheckStep, slices.Sorted(maps.Keys(conditions)))
	}
	if len(conditions) != 1 {
		t.Fatalf("%s: the %q job's gated steps carry %d distinct if: conditions, want one they all share: %v",
			ciWorkflow, gateJob, len(conditions), slices.Sorted(maps.Keys(conditions)))
	}
	return found[0]
}
