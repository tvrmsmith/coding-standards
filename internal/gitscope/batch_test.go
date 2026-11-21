package gitscope

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

func TestDivergenceBatchesKeepAnOrdinaryChangesetOnOneInvocation(t *testing.T) {
	paths := ordinaryPaths(200)

	batches := divergenceBatches(paths)

	if len(batches) != 1 || !slices.Equal(batches[0], paths) {
		t.Errorf("divergenceBatches over %d ordinary paths returned %d batches, want one holding all of them in order", len(paths), len(batches))
	}
}

func TestDivergenceBatchesSplitPastTheBudgetWithoutLosingAPath(t *testing.T) {
	paths := ordinaryPaths(budgetCrossingCount)

	batches := divergenceBatches(paths)

	if len(batches) < 2 {
		t.Fatalf("divergenceBatches over %d bytes of pathspec text returned %d batches, want more than one", pathspecCost(paths), len(batches))
	}
	if rejoined := slices.Concat(batches...); !slices.Equal(rejoined, paths) {
		t.Errorf("the batches rejoined hold %d paths, want the %d they were given, in order", len(rejoined), len(paths))
	}
	for i, batch := range batches {
		if cost := pathspecCost(batch); cost > divergenceBudget {
			t.Errorf("batch %d costs %d bytes of pathspec text, want at most the %d budget", i, cost, divergenceBudget)
		}
	}
}

func TestDivergenceBatchesSendAPathLargerThanTheBudgetAlone(t *testing.T) {
	huge := srcpath.Path("src/" + strings.Repeat("d", divergenceBudget) + ".cs")

	batches := divergenceBatches([]srcpath.Path{huge})

	if len(batches) != 1 || len(batches[0]) != 1 || batches[0][0] != huge {
		t.Errorf("divergenceBatches over one %d byte pathspec returned %d batches, want one holding that path", pathspecCost([]srcpath.Path{huge}), len(batches))
	}
}

func TestDivergenceBatchesOfNoPathsAreNone(t *testing.T) {
	if batches := divergenceBatches(nil); len(batches) != 0 {
		t.Errorf("divergenceBatches over no paths returned %d batches, want none", len(batches))
	}
}

func TestDivergenceBatchCountIsTheNumberOfInvocations(t *testing.T) {
	cases := map[string][]srcpath.Path{
		"no paths":                       nil,
		"an ordinary changeset":          ordinaryPaths(200),
		"a changeset past the budget":    ordinaryPaths(budgetCrossingCount),
		"a changeset of several batches": ordinaryPaths(budgetCrossingCount * 3),
	}

	for name, paths := range cases {
		t.Run(name, func(t *testing.T) {
			want := len(divergenceBatches(paths))

			if got := DivergenceBatchCount(paths); got != want {
				t.Errorf("DivergenceBatchCount over %d paths answered %d, want the %d invocations DivergentFromIndex makes", len(paths), got, want)
			}
		})
	}
}

func TestDivergenceBatchCountSeparatesASplittingLayoutFromAWholeOne(t *testing.T) {
	if got := DivergenceBatchCount(ordinaryPaths(200)); got != 1 {
		t.Errorf("DivergenceBatchCount over an ordinary changeset answered %d, want one invocation", got)
	}
	if got := DivergenceBatchCount(ordinaryPaths(budgetCrossingCount)); got < 2 {
		t.Errorf("DivergenceBatchCount over %d bytes of pathspec text answered %d, want more than one invocation", pathspecCost(ordinaryPaths(budgetCrossingCount)), got)
	}
}

// budgetCrossingCount is how many ordinaryPaths it takes to run just past the
// budget, which is the smallest fixture that has to split.
var budgetCrossingCount = divergenceBudget/pathspecCost(ordinaryPaths(1)) + 1

// pathspecCost is what a set of paths costs in the kernel's argv block, every
// pathspec plus the NUL terminating it. The pathspec text comes from the
// production builder, so a magic word added there cannot leave this accounting
// behind and let a real batch run past the budget with the split case green.
// Only the NUL is the case's own restatement.
func pathspecCost(paths []srcpath.Path) int {
	total := 0
	for _, path := range paths {
		total += len(pathspec(path)) + 1
	}
	return total
}

// ordinaryPaths lays out count source paths of the length a real repository
// holds, which is the size the single-invocation case is about.
func ordinaryPaths(count int) []srcpath.Path {
	paths := make([]srcpath.Path, 0, count)
	for i := range count {
		paths = append(paths, srcpath.Path(fmt.Sprintf("src/Ordering/Handlers/OrderHandler%03d.cs", i)))
	}
	return paths
}
