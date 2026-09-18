package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tvrmsmith/coding-standards/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/internal/srcpath"
	"github.com/tvrmsmith/coding-standards/lint/internal/waiver"
)

// waiverLogPath is $TVRMSMITH_WAIVERS if set, else
// $XDG_STATE_HOME/coding-standards/waivers.jsonl, else
// ~/.local/state/coding-standards/waivers.jsonl. A test always sets
// TVRMSMITH_WAIVERS, so it never reaches a real home directory.
//
// State rather than config, and that is the whole point of the directory
// choice. The log is a record of what happened, not something anyone edits, and
// bootstrap symlinks $XDG_CONFIG_HOME/coding-standards at the hub checkout, so
// a config-directory default would write the audit log into the repository the
// gate guards. The waiver log has to live outside every repo.
func waiverLogPath() string {
	if p := os.Getenv("TVRMSMITH_WAIVERS"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "coding-standards", "waivers.jsonl")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "state", "coding-standards", "waivers.jsonl")
}

// runWaive records a one-shot waiver against the log.
func runWaive(wa WaiveArgs, stdout, stderr io.Writer) int {
	path, err := waivePathOf(wa.Path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := waiver.Open(waiverLogPath())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	w, err := store.Record(waiver.Waiver{
		Language: wa.Language,
		Path:     path,
		Rule:     wa.Rule,
		Reason:   wa.Reason,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "recorded waiver %s: %s %s on %s\n", w.ID, wa.Language, wa.Rule, target(path))
	return 0
}

// waivePathOf places --path the way --files is placed, through the repo root,
// so a waiver recorded from a subdirectory or with an absolute name carries the
// canonical repo-relative path a finding's location will be matched on. An
// unplaceable name fails the record rather than being written as a waiver that
// can never match anything.
//
// An absent --path is the analyzer-load case, which keys on language and rule
// alone and never reaches git.
func waivePathOf(name string) (srcpath.Path, error) {
	if name == "" {
		return "", nil
	}
	repo, err := gitscope.Open()
	if err != nil {
		return "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("reading the working directory: %w", err)
	}
	named, err := repo.Root().NamedFiles([]string{name}, cwd)
	if err != nil {
		return "", err
	}
	return named[0], nil
}

// target names what a pathless waiver covers, since an empty column would read
// as a field the record lost rather than one it never had.
func target(path srcpath.Path) string {
	if path == "" {
		return "(any location)"
	}
	return string(path)
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
		_, _ = fmt.Fprintf(stdout, "%s %s %s %s %s\n", e.ID, e.Language, e.Rule, target(e.Path), state)
	}
	return 0
}
