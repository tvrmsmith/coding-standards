package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tvrmsmith/coding-standards/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// registryKeyEnv names the registry key directly, for a caller that already
// knows it and whose worktree cannot say. A no-mistakes run is that caller: it
// lints in a detached worktree of the daemon's own bare gate repo, so the main
// checkout resolves to the gate's parent, which is in no registry, and the
// skip is indistinguishable from a clean result.
const registryKeyEnv = "TVRMSMITH_REGISTRY_KEY"

// runRegistryKey prints the path adoption is looked up under, the Go registry
// line and the .NET props condition both, followed by a newline. Exit 0 is the
// key and 1 is no key. There is no fallback: a key naming no adopted checkout
// sends every branch to not_wired, and the gate passes linting nothing.
func runRegistryKey(stdout, stderr io.Writer) int {
	key, err := registryKey()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, key.Dir()); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// registryKey resolves every path through its symlinks, because the .NET props
// condition is a StartsWith on the resolved project directory.
func registryKey() (srcpath.Root, error) {
	if named := os.Getenv(registryKeyEnv); named != "" {
		return namedRegistryKey(named)
	}
	repo, err := gitscope.Open()
	if err != nil {
		return srcpath.Root{}, fmt.Errorf("lint-changed: resolving the registry key: %w", err)
	}
	// A linked worktree is the same adoption as the checkout it was made from,
	// and the hook lives in the common git dir, so keying on the worktree would
	// install a hook in every worktree that then skipped, which reads exactly
	// like the layer being broken.
	main, err := repo.MainCheckout()
	if err != nil {
		return srcpath.Root{}, fmt.Errorf("lint-changed: resolving the registry key: %w", err)
	}
	return main, nil
}

// namedRegistryKey refuses a value naming no directory rather than passing it
// through. It would match no registry line and no props condition, so a typo in
// the variable would turn the gate into a silent no-op. A caller that names a
// checkout is asserting one exists.
func namedRegistryKey(named string) (srcpath.Root, error) {
	notADir := fmt.Errorf("lint-changed: %s names '%s', which is not a directory", registryKeyEnv, named)
	abs, err := filepath.Abs(named)
	if err != nil {
		return srcpath.Root{}, fmt.Errorf("%w: %w", notADir, err)
	}
	key, err := srcpath.NewRoot(abs)
	if err != nil {
		return srcpath.Root{}, fmt.Errorf("%w: %w", notADir, err)
	}
	info, err := os.Stat(key.Dir())
	if err != nil {
		return srcpath.Root{}, fmt.Errorf("%w: %w", notADir, err)
	}
	if !info.IsDir() {
		return srcpath.Root{}, notADir
	}
	return srcpath.NewRoot(spelledOnDisk(key.Dir()))
}

// spelledOnDisk is dir with each component spelled the way its parent lists it,
// which is what the shell's `cd && pwd -P` printed. On a case-insensitive
// filesystem EvalSymlinks keeps the case the caller typed, and bootstrap wrote
// the registry with pwd -P, so a mis-cased override would miss every
// case-sensitive lookup and skip in silence. A component only takes another
// spelling that os.SameFile confirms is the same directory, so two directories a
// case-sensitive filesystem keeps apart never merge. A parent it cannot list
// keeps the component as typed.
func spelledOnDisk(dir string) string {
	spelled := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(dir, spelled), string(filepath.Separator)) {
		spelled = filepath.Join(spelled, listedSpelling(spelled, component))
	}
	return spelled
}

// listedSpelling is how parent lists component, or component itself when the
// listing holds it exactly or confirms no other spelling.
func listedSpelling(parent, component string) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return component
	}
	if slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == component }) {
		return component
	}
	typed, err := os.Stat(filepath.Join(parent, component))
	if err != nil {
		return component
	}
	for _, entry := range entries {
		if !strings.EqualFold(entry.Name(), component) {
			continue
		}
		if info, err := os.Stat(filepath.Join(parent, entry.Name())); err == nil && os.SameFile(info, typed) {
			return entry.Name()
		}
	}
	return component
}
