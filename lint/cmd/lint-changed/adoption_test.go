package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseAdoptedReadsEveryFlag(t *testing.T) {
	cmd, err := Parse([]string{"adopted", "--language", "csharp", "--registry-key", "/r", "--registry", "/p.props"})
	if err != nil {
		t.Fatal(err)
	}
	want := AdoptedArgs{Language: "csharp", Key: "/r", Registry: "/p.props"}
	if cmd.Kind != KindAdopted || !reflect.DeepEqual(cmd.Adopted, want) {
		t.Errorf("Parse = %+v, want adopted over %+v", cmd, want)
	}
}

// TypeScript has no registry to ask, so naming it is a caller's mistake
// rather than a question with an answer.
func TestParseAdoptedRefusesWhatItCannotAnswer(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"adopted", "--language", "ts", "--registry-key", "/r", "--registry", "/f"}, "adopted: --language must be go or csharp"},
		{[]string{"adopted", "--language", "go", "--registry", "/f"}, "adopted: --registry-key is required"},
		{[]string{"adopted", "--language", "go", "--registry-key", "/r"}, "adopted: --registry is required"},
		{[]string{"adopted", "--language", "go", "--registry-key", "/r", "--registry", "/f", "--staged"}, "adopted: unknown argument '--staged'"},
	}
	for _, c := range cases {
		_, err := Parse(c.args)
		var ue *UsageError
		if !errors.As(err, &ue) || ue.Problem != c.want {
			t.Errorf("Parse(%v) = %v, want the usage error %q", c.args, err, c.want)
		}
	}
}

const repoKey = "/Users/you/dev/repo"

func TestGoRegistryAdoption(t *testing.T) {
	cases := []struct {
		name     string
		registry string
		want     bool
	}{
		{"its own line", "/Users/you/dev/other\n" + repoKey + "\n", true},
		{"a last line with no newline", repoKey, true},
		// grep -x read each of these three as another path, so the repo went
		// unlinted in silence.
		{"a CRLF line ending", repoKey + "\r\n", true},
		{"a trailing slash", repoKey + "/\n", true},
		{"a doubled slash", "/Users/you/dev//repo\n", true},
		{"a longer path it prefixes", repoKey + "-other\n", false},
		{"a path inside it", repoKey + "/sub\n", false},
		{"a parent of it", "/Users/you/dev\n", false},
		{"an empty registry", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := adopted(AdoptedArgs{Language: "go", Key: repoKey, Registry: write(t, c.registry)})
			if err != nil || got != c.want {
				t.Errorf("adopted = %v, %v, want %v", got, err, c.want)
			}
		})
	}
}

// A hand copy of the condition bootstrap writes. A change to bootstrap does not
// update it, so this pins the shape as it stands rather than tracking bootstrap.
const bootstrapImport = `  <Import Project="/hub/Tvrmsmith.Analyzers.Local.props"
          Condition="$(MSBuildProjectDirectory.StartsWith('` + repoKey + `/')) or '$(MSBuildProjectDirectory)' == '` + repoKey + `'" />`

func TestPropsAdoption(t *testing.T) {
	cases := []struct {
		name  string
		props string
		want  bool
	}{
		{"the Import bootstrap writes", "<Project>\n" + bootstrapImport + "\n</Project>\n", true},
		{"a condition from before the root-project branch", `<Project><Import Project="x" Condition="$(MSBuildProjectDirectory.StartsWith('` + repoKey + `/'))" /></Project>`, true},
		{"an MSBuild namespace", `<Project xmlns="http://schemas.microsoft.com/developer/msbuild/2003">` + bootstrapImport + `</Project>`, true},
		{"an Import inside an ImportGroup", "<Project><ImportGroup>" + bootstrapImport + "</ImportGroup></Project>", true},
		// grep -F missed this one.
		{"quotes as entities", `<Project><Import Project="x" Condition="$(MSBuildProjectDirectory.StartsWith(&apos;` + repoKey + `/&apos;))" /></Project>`, true},
		// grep -F took each of these three for an adopted repo.
		{"the condition in a comment", "<Project>\n  <!-- StartsWith('" + repoKey + "/') -->\n</Project>\n", false},
		{"the condition on another element", `<Project><PropertyGroup Condition="$(MSBuildProjectDirectory.StartsWith('` + repoKey + `/'))" /></Project>`, false},
		{"the condition in Project rather than Condition", `<Project><Import Project="$(MSBuildProjectDirectory.StartsWith('` + repoKey + `/'))" /></Project>`, false},
		{"another repo's Import", strings.ReplaceAll("<Project>\n"+bootstrapImport+"\n</Project>\n", repoKey, repoKey+"-other"), false},
		{"an empty Project", "<Project />\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := adopted(AdoptedArgs{Language: "csharp", Key: repoKey, Registry: write(t, c.props)})
			if err != nil || got != c.want {
				t.Errorf("adopted = %v, %v, want %v", got, err, c.want)
			}
		})
	}
}

// An apostrophe in the path has to be encoded to survive MSBuild's own
// quoting, and grep searched for it raw.
func TestPropsPathHoldingAQuote(t *testing.T) {
	props := `<Project><Import Project="x" Condition="$(MSBuildProjectDirectory.StartsWith(&apos;/Users/you/dev/it&apos;s/&apos;))" /></Project>`

	got, err := adopted(AdoptedArgs{Language: "csharp", Key: "/Users/you/dev/it's", Registry: write(t, props)})

	if err != nil || !got {
		t.Errorf("adopted = %v, %v, want adopted", got, err)
	}
}

// A path holding an ampersand leaves bootstrap's raw write as a file no XML
// reader loads, MSBuild included. grep still found the key in it, so the repo
// read as adopted while every build importing the props failed.
func TestPropsMSBuildCannotLoadIsBroken(t *testing.T) {
	props := strings.ReplaceAll("<Project>\n"+bootstrapImport+"\n</Project>\n", repoKey, "/Users/you/dev/R&D")
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindAdopted, Adopted: AdoptedArgs{Language: "csharp", Key: "/Users/you/dev/R&D", Registry: write(t, props)}}, &stdout, &stderr)

	if code != 1 || stdout.String() != "" || !strings.Contains(stderr.String(), "is not well-formed XML") {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 naming the malformed file", code, stdout.String(), stderr.String())
	}
}

// A write cut off before any content landed leaves a props file with no root
// element, which MSBuild cannot load any more than a malformed one.
func TestPropsWithNoRootElementIsBroken(t *testing.T) {
	for _, props := range []string{"", " \n\t\n", "<?xml version=\"1.0\"?>\n", "<!-- nothing yet -->\n"} {
		t.Run(fmt.Sprintf("%q", props), func(t *testing.T) {
			var stdout, stderr strings.Builder

			code := run(Command{Kind: KindAdopted, Adopted: AdoptedArgs{Language: "csharp", Key: repoKey, Registry: write(t, props)}}, &stdout, &stderr)

			if code != 1 || stdout.String() != "" || !strings.Contains(stderr.String(), "no root element") {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 naming the missing root", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestAdoptedPrintsItsAnswer(t *testing.T) {
	cases := []struct {
		registry string
		want     string
	}{
		{repoKey + "\n", "adopted\n"},
		{"/Users/you/dev/other\n", "not-adopted\n"},
	}
	for _, c := range cases {
		var stdout, stderr strings.Builder

		code := run(Command{Kind: KindAdopted, Adopted: AdoptedArgs{Language: "go", Key: repoKey, Registry: write(t, c.registry)}}, &stdout, &stderr)

		if code != 0 || stdout.String() != c.want {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and %q", code, stdout.String(), stderr.String(), c.want)
		}
	}
}

// No registry is a machine that never bootstrapped the language, the common
// case for every repo on it, so it must not fail anyone's commit.
func TestAdoptedWithNoRegistryIsNotAdopted(t *testing.T) {
	for _, language := range []string{"go", "csharp"} {
		got, err := adopted(AdoptedArgs{Language: language, Key: repoKey, Registry: filepath.Join(t.TempDir(), "absent")})
		if err != nil || got {
			t.Errorf("%s: adopted = %v, %v, want not adopted", language, got, err)
		}
	}
}

func TestAdoptedUnreadableRegistryIsBroken(t *testing.T) {
	_, err := adopted(AdoptedArgs{Language: "go", Key: repoKey, Registry: t.TempDir()})
	if err == nil {
		t.Error("adopted over a directory = nil error, want the read failure")
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "registry")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
