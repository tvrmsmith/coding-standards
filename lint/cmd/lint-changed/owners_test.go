package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParseOwnersTakesALanguageAndEachFile(t *testing.T) {
	cmd, err := Parse([]string{"owners", "--language", "go", "--files", "a,b.go", "--files", "c.go"})

	want := OwnersArgs{Language: "go", Files: []string{"a,b.go", "c.go"}}
	if err != nil || cmd.Kind != KindOwners || !reflect.DeepEqual(cmd.Owners, want) {
		t.Errorf("Parse = %+v, %v, want owners over %+v", cmd, err, want)
	}
}

func TestParseOwnersRefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"owners"}, "owners: --language '' is not one of csharp, go, ts"},
		{[]string{"owners", "--language", "dotnet"}, "owners: --language 'dotnet' is not one of csharp, go, ts"},
		{[]string{"owners", "--language", "go", "--staged"}, "owners: unknown argument '--staged'"},
		{[]string{"owners", "--language", "go", "--files"}, "--files needs a value"},
	}
	for _, c := range cases {
		_, err := Parse(c.args)
		var ue *UsageError
		if !errors.As(err, &ue) || ue.Problem != c.want {
			t.Errorf("Parse(%v) = %v, want the usage error %q", c.args, err, c.want)
		}
	}
}

func TestGoOwnersAreTheNearestModuleWithAPackagePerDirectory(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []OwnerGroup
	}{
		{
			"a module at the repo root",
			[]string{"main.go", "cmd/tool/main.go"},
			[]OwnerGroup{{".", []string{"./.", "./cmd/tool"}}},
		},
		{
			"a file beside a nested go.mod",
			[]string{"gate/main.go"},
			[]OwnerGroup{{"gate", []string{"./."}}},
		},
		{
			"a package repeating its module's name",
			[]string{"gate/gate/run.go"},
			[]OwnerGroup{{"gate", []string{"./gate"}}},
		},
		{
			"nested modules, the inner one winning",
			[]string{"lib.go", "gate/internal/a.go", "gate/internal/b.go"},
			[]OwnerGroup{{".", []string{"./."}}, {"gate", []string{"./internal"}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inTree(t, "go.mod", "gate/go.mod")

			groups, unowned, err := owners("go", c.files)

			if err != nil || len(unowned) != 0 || !reflect.DeepEqual(groups, c.want) {
				t.Errorf("owners(%v) = %+v, unowned %v, %v, want %+v", c.files, groups, unowned, err, c.want)
			}
		})
	}
}

func TestAFileNoUnitOwnsIsUnownedRatherThanGrouped(t *testing.T) {
	inTree(t, "mod/go.mod")

	groups, unowned, err := owners("go", []string{"loose.go", "mod/a.go", "other/b.go"})

	want := []OwnerGroup{{"mod", []string{"./."}}}
	if err != nil || !reflect.DeepEqual(groups, want) || !reflect.DeepEqual(unowned, []string{"loose.go", "other/b.go"}) {
		t.Errorf("owners = %+v, unowned %v, %v, want %+v and both loose files unowned", groups, unowned, err, want)
	}
}

// A directory is not a marker: `[ -f ]` said so for go.mod, and a directory
// named Foo.csproj is not a project to build.
func TestADirectoryNamedLikeAMarkerOwnsNothing(t *testing.T) {
	inTree(t, "go.mod/keep", "Foo.csproj/keep")

	goGroups, _, goErr := owners("go", []string{"a.go"})
	csGroups, _, csErr := owners("csharp", []string{"A.cs"})

	if goErr != nil || csErr != nil || len(goGroups) != 0 || len(csGroups) != 0 {
		t.Errorf("go %+v %v, csharp %+v %v, want no owner for either", goGroups, goErr, csGroups, csErr)
	}
}

func TestCSharpOwnersAreEveryProjectInTheNearestProjectDirectory(t *testing.T) {
	inTree(t, "App/App.csproj", "App/App.Legacy.csproj", "App/.Hidden.csproj", "Lib/Lib.csproj")

	groups, unowned, err := owners("csharp", []string{"App/Views/Home.cs", "Lib/Thing.cs"})

	want := []OwnerGroup{
		{"App/App.Legacy.csproj", []string{"Views/Home.cs"}},
		{"App/App.csproj", []string{"Views/Home.cs"}},
		{"Lib/Lib.csproj", []string{"Thing.cs"}},
	}
	if err != nil || len(unowned) != 0 || !reflect.DeepEqual(groups, want) {
		t.Errorf("owners = %+v, unowned %v, %v, want %+v", groups, unowned, err, want)
	}
}

func TestCSharpProjectAtTheRepoRootHasABarePath(t *testing.T) {
	inTree(t, "Root.csproj")

	groups, _, err := owners("csharp", []string{"src/A.cs"})

	want := []OwnerGroup{{"Root.csproj", []string{"src/A.cs"}}}
	if err != nil || !reflect.DeepEqual(groups, want) {
		t.Errorf("owners = %+v, %v, want %+v", groups, err, want)
	}
}

func TestTSOwnersAreTheNearestPackageWithAnESLintConfig(t *testing.T) {
	inTree(t, "eslint.config.js", "packages/web/.eslintrc.json")

	groups, unowned, err := owners("ts", []string{"tools/build.mjs", "packages/web/src/a b.tsx", "packages/web/index.ts"})

	want := []OwnerGroup{
		{".", []string{"tools/build.mjs"}},
		{"packages/web", []string{"index.ts", "src/a b.tsx"}},
	}
	if err != nil || len(unowned) != 0 || !reflect.DeepEqual(groups, want) {
		t.Errorf("owners = %+v, unowned %v, %v, want %+v", groups, unowned, err, want)
	}
}

// A marker that cannot be read is the mapping failing. Read as absent, the
// file would fall through to an outer module and be linted as a package it
// is not in.
func TestAnUnreadableDirectoryFailsTheMapping(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions that bind the running user")
	}
	inTree(t, "go.mod", "locked/inner/a.go")
	if err := os.Chmod("locked", 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("locked", 0o700) }) //nolint:gosec // G302: a directory needs its execute bit back for t.TempDir to remove it

	_, _, err := owners("go", []string{"locked/inner/a.go"})

	if err == nil {
		t.Error("owners over an unreadable directory succeeded, want an error")
	}
}

// A mapping error exits 1 with nothing on stdout, so the branch reading the
// groups cannot mistake a partial or empty answer for every file unowned.
func TestRunOwnersExitsOneWithEmptyStdoutWhenTheMappingFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions that bind the running user")
	}
	inTree(t, "go.mod", "locked/inner/a.go")
	if err := os.Chmod("locked", 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("locked", 0o700) }) //nolint:gosec // G302: a directory needs its execute bit back for t.TempDir to remove it
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindOwners, Owners: OwnersArgs{Language: "go", Files: []string{"locked/inner/a.go"}}}, &stdout, &stderr)

	if code != 1 || stdout.String() != "" {
		t.Errorf("exit %d, stdout %q, want exit 1 and empty stdout", code, stdout.String())
	}
}

func TestRunOwnersPrintsNulTerminatedGroupsAndNamesTheUnowned(t *testing.T) {
	inTree(t, "a/go.mod", "b/go.mod")
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindOwners, Owners: OwnersArgs{Language: "go", Files: []string{"b/x.go", "a/p/y.go", "z.go"}}}, &stdout, &stderr)

	wantOut := "a\x00./p\x00\x00b\x00./.\x00\x00"
	wantErr := "lint-changed: no go.mod above z.go — skipped\n"
	if code != 0 || stdout.String() != wantOut || stderr.String() != wantErr {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, %q and %q", code, stdout.String(), stderr.String(), wantOut, wantErr)
	}
}

// inTree starts the case in a fresh directory holding each named file, its
// parent directories created as needed.
func inTree(t *testing.T, files ...string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
}
