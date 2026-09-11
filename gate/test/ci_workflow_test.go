package gate_test

import (
	"fmt"
	"os"
	"regexp"
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
