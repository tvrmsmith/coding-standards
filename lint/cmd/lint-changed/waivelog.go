package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
	"github.com/tvrmsmith/coding-standards/lint/internal/waiver"
)

// waiverLogPath is $TVRMSMITH_WAIVERS if set, else
// $XDG_CONFIG_HOME/coding-standards/waivers.jsonl, else
// ~/.config/coding-standards/waivers.jsonl. A test always sets
// TVRMSMITH_WAIVERS, so it never reaches a real home directory.
func waiverLogPath() string {
	if p := os.Getenv("TVRMSMITH_WAIVERS"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "coding-standards", "waivers.jsonl")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "coding-standards", "waivers.jsonl")
}

// runWaive records a one-shot waiver against the log.
func runWaive(wa WaiveArgs, stdout, stderr io.Writer) int {
	store, err := waiver.Open(waiverLogPath())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	w, err := store.Record(waiver.Waiver{
		Language: wa.Language,
		Path:     srcpath.FromSlash(wa.Path),
		Rule:     wa.Rule,
		Reason:   wa.Reason,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "recorded waiver %s: %s %s on %s\n", w.ID, wa.Language, wa.Rule, wa.Path)
	return 0
}

// runWaivers lists every recorded waiver with its spend state.
func runWaivers(stdout, stderr io.Writer) int {
	store, err := waiver.Open(waiverLogPath())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for _, e := range store.List() {
		state := "unspent"
		if e.SpentTree != "" {
			state = "spent against " + e.SpentTree
		}
		_, _ = fmt.Fprintf(stdout, "%s %s %s %s %s\n", e.ID, e.Language, e.Rule, e.Path, state)
	}
	return 0
}
