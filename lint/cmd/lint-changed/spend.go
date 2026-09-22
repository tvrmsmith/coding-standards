package main

import (
	"fmt"
	"io"

	"github.com/tvrmsmith/coding-standards/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/lint/internal/waiver"
)

// runSpend marks one or more already-matched waivers spent against the
// current index tree. It is the dispatcher's call alone: the dispatcher is
// the only thing that has seen every language branch's report and knows the
// whole commit went through.
//
// This still runs from a pre-commit hook, before git has actually written the
// commit: a developer who aborts the commit message editor after every branch
// came back clean leaves this spend on the log with no commit behind it. What
// bounds that damage is waiver.Store.Spend being a no-op the second time
// against the same tree, so the retried commit that follows reuses the same
// spend rather than costing a second waiver. Same-tree reuse is why this
// command is safe to call speculatively, not an incidental convenience.
func runSpend(sa SpendArgs, stdout, stderr io.Writer) int {
	store, err := waiver.Open(waiverLogPath())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	repo, err := gitscope.OpenHook()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	trees := &indexTree{repo: repo}

	byID := make(map[string]waiver.Waiver, len(store.List()))
	for _, e := range store.List() {
		byID[e.ID] = e.Waiver
	}

	for _, id := range sa.IDs {
		w, ok := byID[id]
		if !ok {
			_, _ = fmt.Fprintf(stderr, "no waiver recorded with id %s\n", id)
			return 1
		}
		tree, err := trees.sha()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		if err := store.Spend(w, tree); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "waiver %s spent\n", w.ID)
	}
	return 0
}
