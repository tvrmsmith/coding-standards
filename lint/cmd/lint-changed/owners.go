package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

// OwnersArgs is argv for the owners form.
type OwnersArgs struct {
	// Language is one of lintfind's waiver-log keys, which names the build
	// unit a file belongs to: a Go module, a C# project, an ESLint package.
	Language string
	// Files is every --files path, repo-relative, one per flag.
	Files []string
}

// OwnerGroup is one build unit a language branch lints and what it points
// the linter at inside it. Members are relative to the owner's directory,
// which the Go and TS branches run their linter from. The C# branch builds
// the whole project and reads no members.
type OwnerGroup struct {
	// Owner is the module directory for Go, the package directory for TS,
	// and the .csproj path for C#.
	Owner string
	// Members are package patterns for Go, since golangci-lint runs per
	// package, and changed files for TS and C#.
	Members []string
}

// runOwners prints each group as its owner, its members and then an empty
// field, every field NUL-terminated for the reason runChangedPaths gives. A
// file no build unit owns is named on stderr and left out, since its branch
// has nothing to run for it. Exit 1 is the mapping failing, which the branch
// must not read as every file being unowned.
func runOwners(args OwnersArgs, stdout, stderr io.Writer) int {
	groups, unowned, err := owners(args.Language, args.Files)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "lint-changed:", err)
		return 1
	}
	for _, file := range unowned {
		_, _ = fmt.Fprintf(stderr, "lint-changed: no %s above %s — skipped\n", unitName[args.Language], file)
	}
	for _, g := range groups {
		fields := append(append([]string{g.Owner}, g.Members...), "")
		for _, field := range fields {
			if _, err := fmt.Fprint(stdout, field, "\x00"); err != nil {
				_, _ = fmt.Fprintln(stderr, err)
				return 1
			}
		}
	}
	return 0
}

// unitName is what the skip message calls the thing a file had none of.
var unitName = map[string]string{
	lintfind.LanguageGo:     "go.mod",
	lintfind.LanguageCSharp: ".csproj",
	lintfind.LanguageTS:     "ESLint config",
}

// owners groups files by the build unit that owns each, sorted by owner, and
// returns the files nothing owns in the order given. A C# file in a
// directory holding several projects belongs to every one of them, since
// MSBuild's default glob compiles it into each, and picking one would lint it
// under settings the others may not share.
func owners(language string, files []string) ([]OwnerGroup, []string, error) {
	members := map[string][]string{}
	var unowned []string
	for _, file := range files {
		units, err := unitsOwning(language, file)
		if err != nil {
			return nil, nil, err
		}
		if len(units) == 0 {
			unowned = append(unowned, file)
			continue
		}
		for _, unit := range units {
			members[unit.owner] = append(members[unit.owner], unit.member)
		}
	}

	groups := make([]OwnerGroup, 0, len(members))
	for owner, ms := range members {
		// Compacted because several files in one Go package name it once.
		groups = append(groups, OwnerGroup{Owner: owner, Members: slices.Compact(slices.Sorted(slices.Values(ms)))})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Owner < groups[j].Owner })
	return groups, unowned, nil
}

type unit struct{ owner, member string }

// unitsOwning walks up from the file's directory to the nearest one holding
// the language's marker and answers every owner that directory holds.
func unitsOwning(language, file string) ([]unit, error) {
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		units, err := unitsIn(language, dir, file)
		if err != nil || len(units) > 0 {
			return units, err
		}
		if dir == "." || dir == filepath.Dir(dir) {
			return nil, nil
		}
	}
}

// unitsIn answers the owners dir holds for file, none when dir is not one.
// filepath.Rel makes a package pattern the same way whether the module is
// the repo root, the file's own directory or an ancestor of it, which a
// shell prefix strip had to tell apart case by case.
func unitsIn(language, dir, file string) ([]unit, error) {
	rel, err := filepath.Rel(dir, file)
	if err != nil {
		return nil, err
	}
	switch language {
	case lintfind.LanguageGo:
		ok, err := isFile(filepath.Join(dir, "go.mod"))
		if !ok || err != nil {
			return nil, err
		}
		return []unit{{dir, "./" + filepath.ToSlash(filepath.Dir(rel))}}, nil
	case lintfind.LanguageTS:
		ok, err := hasESLintConfig(dir)
		if !ok || err != nil {
			return nil, err
		}
		return []unit{{dir, rel}}, nil
	case lintfind.LanguageCSharp:
		projects, err := projectsIn(dir)
		if err != nil {
			return nil, err
		}
		units := make([]unit, 0, len(projects))
		for _, project := range projects {
			units = append(units, unit{filepath.Join(dir, project), rel})
		}
		return units, nil
	default:
		return nil, fmt.Errorf("owners: unknown language %q", language)
	}
}

// eslintConfigs is every name ESLint 8 or 9 loads a config from. The package
// is the directory holding one, since flat config does not cascade and the
// layering wrapper loads the package's config from the process cwd.
var eslintConfigs = []string{
	"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts",
	".eslintrc.js", ".eslintrc.cjs", ".eslintrc.mjs", ".eslintrc.json", ".eslintrc",
}

func hasESLintConfig(dir string) (bool, error) {
	for _, name := range eslintConfigs {
		if ok, err := isFile(filepath.Join(dir, name)); ok || err != nil {
			return ok, err
		}
	}
	return false, nil
}

// projectsIn lists the .csproj files in dir by name, sorted. A read rather
// than a glob, since a directory name holding a glob metacharacter would
// match the wrong thing, and dot-files are left out as the shell glob did.
func projectsIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var projects []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".csproj") {
			continue
		}
		ok, err := isFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if ok {
			projects = append(projects, name)
		}
	}
	return projects, nil
}

// isFile follows a symlink, as `[ -f ]` does. Only absence answers false: a
// marker that cannot be read is the mapping failing, and reading it as
// absent would send the file to an outer unit or to none, reporting clean.
func isFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}
