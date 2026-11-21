package gate_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// gatePrefix is the import path prefix that marks a dependency as belonging
// to gate/ rather than sitting above it. The trailing slash matters: without
// it, a future sibling package named e.g. gateway would also match.
const gatePrefix = "github.com/tvrmsmith/coding-standards/gate/"

// TestInternalDoesNotDependOnGate holds the property internal/gitscope.go
// claims in its own package doc: internal/ sits above gate/ so a second
// binary can share it, which only holds while nothing under internal/
// imports back down into gate/. Go will not stop that regression on its
// own, so this test asks the compiler's own view of the dependency graph
// rather than grepping import lines, which an alias or a blank import could
// dodge.
func TestInternalDoesNotDependOnGate(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("go", "list", "-test", "-f", "{{.ImportPath}}{{range .Deps}} {{.}}{{end}}", "./internal/...")
	cmd.Dir = "../.."
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("go list -test ./internal/... exited %v, so this guard could not inspect the dependency graph at all. One way this happens: a file under internal/ imports a gate/internal/... package, which Go's own internal-package rule forbids loading from outside gate/, so the regression surfaces as a load error here rather than as a matched dependency below. stderr:\n%s", err, stderr.String())
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	var packages int
	var violations []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		packages++
		pkg := fields[0]
		var offending []string
		for _, dep := range fields[1:] {
			if strings.HasPrefix(dep, gatePrefix) {
				offending = append(offending, dep)
			}
		}
		if len(offending) > 0 {
			violations = append(violations, pkg+" -> "+strings.Join(offending, ", "))
		}
	}

	if packages == 0 {
		t.Fatal("go list -test ./internal/... named no packages, so this guard walked an empty graph and would pass regardless of what internal/ imports")
	}

	for _, v := range violations {
		t.Errorf("%s carries a dependency on gate/. The root module exists so internal/gitscope and internal/srcpath sit above gate/ and can be shared by a second binary; a dependency running the other way silently undoes that decoupling and makes UnreadableDiffError, OpenError and the asFailure translation in gate/cmd/metric-gate/main.go dead weight for nothing", v)
	}
}
