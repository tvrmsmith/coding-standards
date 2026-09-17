package report

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryErrorCodeIsPinnedByAGolden reads every golden in gate/test/golden
// and checks the registry and the goldens name the same set of codes in both
// directions: a registered code no golden names would ship unverified, and a
// golden naming a code the registry does not know is a typo in the golden.
//
// This lives in report rather than gate/test because gate/test is a
// black-box suite whose package doc says nothing there reaches inside the
// gate (harness_test.go); the dependency runs the other way, this test
// reaching out to read the suite's goldens.
func TestEveryErrorCodeIsPinnedByAGolden(t *testing.T) {
	named := goldenCodes(t)

	registered := map[string]bool{}
	for _, code := range Codes() {
		registered[code] = true
	}

	var unpinned []string
	for _, code := range Codes() {
		if !named[code] {
			unpinned = append(unpinned, code)
		}
	}
	if len(unpinned) > 0 {
		t.Errorf("registered but no golden names them: %s", strings.Join(unpinned, ", "))
	}

	var unknown []string
	for code := range named {
		if !registered[code] {
			unknown = append(unknown, code)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		t.Errorf("named by a golden but not registered: %s", strings.Join(unknown, ", "))
	}
}

// goldenCodes collects the value of every `code: <value>` line across the
// suite's golden documents.
func goldenCodes(t *testing.T) map[string]bool {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "..", "test", "golden", "*.toon"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no goldens found; every code would read as unpinned for the wrong reason")
	}
	found := map[string]bool{}
	for _, path := range matches {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			if code, ok := strings.CutPrefix(strings.TrimSpace(line), "code:"); ok {
				found[strings.TrimSpace(code)] = true
			}
		}
	}
	return found
}
