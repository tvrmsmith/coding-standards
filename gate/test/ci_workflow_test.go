package gate_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// passCheckMarker is the grep the PASS check runs per name. It locates the
// step and the loop; it is not the assertion. A step that stopped checking
// PASS lines is a restructure this case has to red on rather than silently
// assert nothing about.
const passCheckMarker = `^--- PASS: $name \(`

// TestCILoopsOverEveryRealExtractorCase pins the one thing ci.yml cannot
// derive. The workflow retypes the names in realExtractorCases into a shell
// loop, and a name added to the slice but not to that loop is never proven to
// have run in CI, which is the exact hole the PASS check exists to close. This
// case closes it in the other direction, at merge time.
func TestCILoopsOverEveryRealExtractorCase(t *testing.T) {
	script := passCheckScript(t)

	looped, err := forLoopWords(script)
	if err != nil {
		t.Fatalf("%s: the gate job's PASS check step no longer holds a loop this case can read: %v\nscript:\n%s",
			ciWorkflow, err, script)
	}

	for _, name := range realExtractorCases {
		if !slices.Contains(looped, name) {
			t.Errorf("%s: the PASS check loops over %v, which does not name %s. A case in realExtractorCases that CI never proves ran is the hole the PASS check exists to close.",
				ciWorkflow, looped, name)
		}
	}
	for _, name := range looped {
		if !slices.Contains(realExtractorCases, name) {
			t.Errorf("%s: the PASS check loops over %s, which is not a case in realExtractorCases. It was renamed or deleted, so CI is waiting on a PASS line no run can print.",
				ciWorkflow, name)
		}
	}
}

// TestCIPassCheckRedsTheJob runs the gate job's step out of ci.yml under bash,
// so what is under test is what the check does rather than which tokens its
// script contains. The runner's side of the contract is supplied here: bash,
// a RUNNER_TEMP to write the log into, and a `go` on PATH standing in for the
// toolchain, which is what lets the case choose the verbose log the check then
// has to judge.
func TestCIPassCheckRedsTheJob(t *testing.T) {
	script := passCheckScript(t)

	last := len(realExtractorCases) - 1
	renamed := slices.Clone(realExtractorCases)
	renamed[last] += "Twice"

	cases := []struct {
		name    string
		ran     []string
		goExit  int
		wantErr bool
		names   []string
	}{
		{
			name: "every case reported PASS, so the check lets the job through",
			ran:  realExtractorCases,
		},
		{
			name:    "a case with no PASS line reds the job and is named",
			ran:     realExtractorCases[:last],
			wantErr: true,
			names:   []string{realExtractorCases[last]},
		},
		{
			name:    "a longer name that starts with a case's name does not stand in for it",
			ran:     renamed,
			wantErr: true,
			names:   []string{realExtractorCases[last]},
		},
		{
			name:    "a failing test run reds the job even with every PASS line present",
			ran:     realExtractorCases,
			goExit:  1,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stderr, err := runPassCheck(t, script, verboseLog(c.ran), c.goExit)
			if c.wantErr && err == nil {
				t.Fatalf("the check exited zero, want a failure. stderr:\n%s", stderr)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("the check failed with %v, want it to let the job through. stderr:\n%s", err, stderr)
			}
			for _, name := range c.names {
				if !strings.Contains(stderr, name) {
					t.Errorf("the check reds without naming %s, so the log says nothing about which case did not run. stderr:\n%s", name, stderr)
				}
			}
		})
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
// on PATH that replays log and exits goExit. It returns the script's stderr,
// which is where the check reports a case that did not run, and its exit
// error.
func runPassCheck(t *testing.T, script, log string, goExit int) (string, error) {
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
	var stderr strings.Builder
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// passCheckScript is the run script of the gate job's PASS check step. It
// fails the test rather than returning empty when the job, the steps or the
// step cannot be found, so a workflow restructure reds here instead of leaving
// the assertion above with nothing to say.
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
	var names []string
	for _, step := range job.Steps {
		if strings.Contains(step.Run, passCheckMarker) {
			found = append(found, step.Run)
			names = append(names, step.Name)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: the %q job holds %d steps whose script greps for %q, want exactly one: %v",
			ciWorkflow, gateJob, len(found), passCheckMarker, names)
	}
	return found[0]
}

// forLoopHeader matches the opening of a shell for loop at the start of a
// line, which is what keeps a `for ` inside a comment or inside prose from
// being read as the loop.
var forLoopHeader = regexp.MustCompile(`(?m)^[ \t]*for[ \t]+[A-Za-z_][A-Za-z0-9_]*[ \t]+in[ \t]+`)

// forLoopWords is the iteration list of the one for loop in a shell script,
// with line continuations folded away. It is a normalised model of what the
// loop iterates over, which is the meaning this case asserts on; the words
// themselves are the names CI demands a PASS line for.
func forLoopWords(script string) ([]string, error) {
	folded := strings.ReplaceAll(script, "\\\n", " ")

	found := forLoopHeader.FindAllStringIndex(folded, -1)
	if len(found) != 1 {
		return nil, fmt.Errorf("the script holds %d `for <var> in <words>` loops, want exactly one", len(found))
	}

	list := folded[found[0][1]:]
	if end := strings.IndexAny(list, ";\n"); end >= 0 {
		list = list[:end]
	}
	words := strings.Fields(list)
	if len(words) == 0 {
		return nil, fmt.Errorf("the loop iterates over nothing")
	}
	return words, nil
}
