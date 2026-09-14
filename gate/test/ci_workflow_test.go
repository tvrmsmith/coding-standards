package gate_test

import (
	"errors"
	"fmt"
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

	t.Run("every case reported PASS, so the check lets the job through", func(t *testing.T) {
		assertPassCheck(t, script, realExtractorCases, 0, false, nil)
	})

	for i, name := range realExtractorCases {
		withheld := slices.Concat(realExtractorCases[:i], realExtractorCases[i+1:])
		renamed := slices.Clone(realExtractorCases)
		renamed[i] += "Twice"

		t.Run(name+"/no PASS line reds the job and is named", func(t *testing.T) {
			assertPassCheck(t, script, withheld, 0, true, []string{name})
		})
		t.Run(name+"/a longer name that starts with it does not stand in for it", func(t *testing.T) {
			assertPassCheck(t, script, renamed, 0, true, []string{name})
		})
	}

	t.Run("a failing test run reds the job even with every PASS line present", func(t *testing.T) {
		assertPassCheck(t, script, realExtractorCases, 1, true, nil)
	})
}

// assertPassCheck runs the step's script over the log a run of ran would have
// written and asserts the outcome. A failure has to be the script's own `exit
// 1` rather than any nonzero end, so bash missing from PATH or the script
// dying before it reaches the PASS loop cannot pass for the check working.
func assertPassCheck(t *testing.T, script string, ran []string, goExit int, wantErr bool, names []string) {
	t.Helper()

	stdout, stderr, err := runPassCheck(t, script, verboseLog(ran), goExit)
	out := fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr)

	if !wantErr {
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
		t.Fatalf("the check ended with %v rather than a nonzero exit status, so nothing here proves the PASS loop ran at all. %s", err, out)
	}
	if exit.ExitCode() != 1 {
		t.Fatalf("the check exited %d, want the 1 its own failure path reports. Another status is the script dying before it judged the log. %s",
			exit.ExitCode(), out)
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
	var b strings.Builder
	for _, name := range passed {
		fmt.Fprintf(&b, "=== RUN   %s\n--- PASS: %s (1.23s)\n", name, name)
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
	for _, step := range job.Steps {
		if step.Name == passCheckStep {
			found = append(found, step.Run)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: the %q job holds %d steps named %q, want exactly one",
			ciWorkflow, gateJob, len(found), passCheckStep)
	}
	return found[0]
}
