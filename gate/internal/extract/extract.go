// Package extract runs the language-specific half of the measurement
// (ADR 0006). It locates an extractor from a built-in table keyed by
// language, asks the located binary for its extension list, hands it the
// changed files on stdin, and checks that every path it echoes back is one
// the gate gave it.
package extract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// languages is the built-in table ADR 0006 fixes: a language yields a binary
// *name*, looked up beside the gate. Nothing on PATH, no repo config file, and
// no environment variable participates.
//
// The extensions beside the name decide one thing only, whether the gate execs
// the binary at all, so the list is an upper bound on what this language can
// ever be routed. A row that over-claims costs a launch that finds nothing,
// while a row that under-claims silently unscores real source, because a path
// whose extension no row declares never reaches an extractor.
//
// Among the paths the list does select, --capabilities is the sole authority,
// and an extractor whose --capabilities set claims none of them exits 1 with
// extractor_capabilities_mismatch. Scoring nothing would be indistinguishable
// from a clean run.
var languages = map[string]language{
	"csharp": {binary: "metric-gate-csharp", extensions: []string{".cs"}},
}

// language is one row of that table.
type language struct {
	binary     string
	extensions []string
}

// Span is one method's identity and complexity, as the extractor reports it.
//
// Signature is the parameter spelling the extractor gives the method, and it
// exists for one question: whether two spans over the same lines are two
// methods or one method reported twice. Two overloads declared on one line
// share a name and a line range and differ only here. It never reaches the
// document, where Name is the whole of a method's printed identity.
type Span struct {
	File       srcpath.Path
	Name       string
	Signature  string
	StartLine  int
	EndLine    int
	Complexity int
}

// Result is what extraction established about the changed files.
type Result struct {
	// Spans is every method span the extractors found.
	Spans []Span
	// Claimed is the changed files some extractor said it handles. A touched
	// line only counts as outside a span when it is in one of these; a line
	// in a file no extractor claims, a Markdown file say, is outside the
	// measurement rather than outside a span.
	Claimed map[srcpath.Path]bool
}

// Extract runs every located extractor over the changed files it claims. It
// returns a *report.Failure for every exit-1 cause in ADR 0006's contract.
func Extract(root srcpath.Root, changed []srcpath.Path) (Result, error) {
	// An extractor no changed file could possibly belong to is never located
	// and never launched. A docs-only change therefore passes with an empty
	// changed set in a deployment that ships the gate without an extractor
	// beside it.
	worth := worthRunning(changed)
	result := Result{Claimed: map[srcpath.Path]bool{}}
	if len(worth) == 0 {
		return result, nil
	}
	dir, err := extractorDir()
	if err != nil {
		return Result{}, err
	}
	for _, name := range worth {
		e := extractor{language: name, binary: filepath.Join(dir, binaryName(languages[name].binary)), root: root}
		spans, claimed, err := e.run(changed)
		if err != nil {
			return Result{}, err
		}
		result.Spans = append(result.Spans, spans...)
		for _, file := range claimed {
			result.Claimed[file] = true
		}
	}
	return result, nil
}

// worthRunning lists the languages at least one changed path could belong to,
// judged by the table's static extensions. This is the only use of that list:
// once a binary is launched, its own --capabilities answer decides which paths
// it is handed.
//
// The comparison folds case, so `Order.CS` routes where `Order.cs` does. Both
// macOS and Windows carry case-insensitive filesystems, which makes the odd
// spelling an ordinary file a developer creates without noticing, and the two
// directions of a wrong answer are not the same size: over-claiming costs a
// launch that finds nothing, while under-claiming leaves real source unscored
// under exit 0 pass. --capabilities remains the authority on which of the paths
// the list selects the extractor is actually handed.
func worthRunning(changed []srcpath.Path) []string {
	var worth []string
	for _, name := range sortedLanguages() {
		for _, path := range changed {
			if slices.ContainsFunc(languages[name].extensions, func(ext string) bool {
				return strings.EqualFold(ext, path.Ext())
			}) {
				worth = append(worth, name)
				break
			}
		}
	}
	return worth
}

// binaryName is the table's binary name as the running platform spells it. A
// dotnet tool installs as "<name>.exe" on Windows, and the gate hands
// exec.Command an absolute path, so Go does no PATHEXT resolution of its own.
func binaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// extractorDir is the directory the gate's own binary sits in, which is the
// only place an extractor is looked for. Not being able to name it is not
// being able to locate any extractor, so it is typed like every other way the
// run fails to reach one, rather than escaping untyped and costing the caller
// the document.
func extractorDir() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", &report.Failure{
			Code:    report.CodeExtractorFailed,
			Message: "could not locate the gate binary, so no extractor beside it: " + err.Error(),
		}
	}
	return filepath.Dir(self), nil
}

// sortedLanguages keeps the order the languages are consulted in fixed, so a
// run's output does not depend on map iteration.
func sortedLanguages() []string {
	return slices.Sorted(maps.Keys(languages))
}

// extractor is one located language extractor.
type extractor struct {
	language string
	binary   string
	root     srcpath.Root
}

// capabilities is the --capabilities wire response.
type capabilities struct {
	Language   string   `json:"language"`
	Extensions []string `json:"extensions"`
}

// extraction is the wire response to a run over a file list.
type extraction struct {
	Files []struct {
		File   string `json:"file"`
		Status string `json:"status"`
	} `json:"files"`
	Spans []struct {
		File       string `json:"file"`
		Name       string `json:"name"`
		Signature  string `json:"signature"`
		StartLine  int    `json:"startLine"`
		EndLine    int    `json:"endLine"`
		Complexity int    `json:"complexity"`
	} `json:"spans"`
}

// run asks the extractor what it handles, hands it those of the changed
// files, and reads its spans back.
func (e extractor) run(changed []srcpath.Path) (spans []Span, claimed []srcpath.Path, err error) {
	caps, err := e.capabilities()
	if err != nil {
		return nil, nil, err
	}
	claimed = e.filter(changed, caps.Extensions)
	// The table only located this binary because a changed path carried one of
	// the extensions the row declares, so a binary that then claims none of
	// them disagrees with the table. Returning an empty result here would score
	// nothing and exit 0 `pass` on a real source change, which is the silent
	// outcome ADR 0006 rejects.
	if len(claimed) == 0 {
		return nil, nil, &report.Failure{
			Code:    report.CodeExtractorCapabilitiesMismatch,
			Message: fmt.Sprintf("%s extractor claims none of the changed files the language table located it for", e.language),
		}
	}
	body, err := e.invoke(claimed)
	if err != nil {
		return nil, nil, err
	}
	var parsed extraction
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nil, &report.Failure{
			Code:    report.CodeExtractorFailed,
			Message: e.language + " extractor emitted invalid JSON",
		}
	}
	spans, err = e.collect(parsed, claimed)
	if err != nil {
		return nil, nil, err
	}
	return spans, claimed, nil
}

// capabilities asks the located binary for its extension list. ADR 0006 puts
// the list on the binary rather than in the table, because a table that
// disagrees with the binary is a silent misroute.
func (e extractor) capabilities() (capabilities, error) {
	body, err := e.exec(nil, "--capabilities")
	if err != nil {
		return capabilities{}, err
	}
	var caps capabilities
	if err := json.Unmarshal(body, &caps); err != nil {
		return capabilities{}, &report.Failure{
			Code:    report.CodeExtractorFailed,
			Message: e.language + " extractor emitted invalid capabilities JSON",
		}
	}
	// The table picked this binary for a language, so a binary answering with
	// a different one is the misroute ADR 0006 puts the list on the binary to
	// catch, not a language the gate should quietly go along with.
	if caps.Language != e.language {
		return capabilities{}, &report.Failure{
			Code:    report.CodeExtractorCapabilitiesMismatch,
			Message: fmt.Sprintf("%s extractor reported language %s", e.language, caps.Language),
		}
	}
	return caps, nil
}

// filter selects the changed files carrying one of the extractor's
// extensions. It folds case for the reason worthRunning does, and has to fold
// it the same way: the two comparisons are the two halves of one routing
// decision, and a table that routes `Order.CS` to a binary whose filter then
// refuses it turns a silent miss into a capabilities mismatch on a file the
// extractor would have parsed.
func (e extractor) filter(changed []srcpath.Path, extensions []string) []srcpath.Path {
	var mine []srcpath.Path
	for _, path := range changed {
		for _, ext := range extensions {
			if strings.EqualFold(path.Ext(), ext) {
				mine = append(mine, path)
				break
			}
		}
	}
	return mine
}

// invoke runs the extractor over a file list, one path per line on stdin.
func (e extractor) invoke(files []srcpath.Path) ([]byte, error) {
	var stdin bytes.Buffer
	for _, path := range files {
		stdin.WriteString(path.String() + "\n")
	}
	return e.exec(&stdin)
}

// exec runs the located binary in the repo root and returns its stdout.
func (e extractor) exec(stdin *bytes.Buffer, args ...string) ([]byte, error) {
	cmd := exec.Command(e.binary, args...)
	cmd.Dir = e.root.Dir()
	cmd.Env = os.Environ()
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, &report.Failure{
			Code:    report.CodeExtractorFailed,
			Message: fmt.Sprintf("%s extractor exited %d", e.language, exitErr.ExitCode()),
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &report.Failure{
			Code:    report.CodeExtractorFailed,
			Message: fmt.Sprintf("%s extractor not found: %s", e.language, filepath.Base(e.binary)),
		}
	}
	// A binary that is present but will not launch, because it is not
	// executable or was built for another platform, is a different problem
	// from an absent one, and the reader needs the cause to tell them apart.
	return nil, &report.Failure{
		Code:    report.CodeExtractorFailed,
		Message: fmt.Sprintf("%s extractor %s could not be run: %v", e.language, filepath.Base(e.binary), launchCause(err)),
	}
}

// launchCause strips the wrapping os/exec puts around a spawn failure, so the
// message carries the reason and not a machine-specific absolute path.
func launchCause(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

// collect checks the echoed paths and the per-file parse status, then turns
// the wire spans into the gate's own currency.
func (e extractor) collect(parsed extraction, handed []srcpath.Path) ([]Span, error) {
	given := map[string]bool{}
	for _, path := range handed {
		given[path.String()] = true
	}
	// ADR 0004 makes the echo byte-identical, so a path the gate never sent
	// is checked before anything is read out of the response.
	for _, echoed := range echoedPaths(parsed) {
		if !given[echoed] {
			return nil, &report.Failure{
				Code:    report.CodeExtractorPathMismatch,
				Message: fmt.Sprintf("%s extractor returned a path it was not given: %s", e.language, echoed),
			}
		}
	}
	status := map[string]string{}
	for _, file := range parsed.Files {
		status[file.File] = file.Status
	}
	// ADR 0003 makes per-file status an obligation so the gate can tell
	// "nothing here" from "I failed". A path the gate handed in and the
	// extractor never reported on is neither, and silently scoring its file
	// with zero spans would let every method in it go unmeasured under a
	// passing run.
	for _, path := range handed {
		switch status[path.String()] {
		case "parsed":
		case "failed":
			return nil, &report.Failure{
				Code:    report.CodeParseFailed,
				Message: fmt.Sprintf("%s extractor could not parse %s", e.language, path),
			}
		default:
			return nil, &report.Failure{
				Code:    report.CodeExtractorFailed,
				Message: fmt.Sprintf("%s extractor reported no parse status for %s", e.language, path),
			}
		}
	}
	spans := make([]Span, 0, len(parsed.Spans))
	// The same method emitted twice is an extractor contract violation, because
	// one of the two rows would silently vanish from the table. Two distinct
	// methods sharing a line range are not: `class C { int A() => 1; int B() =>
	// 2; }` is valid C# and so is `class C { int F(int x) => 1; int F(string x)
	// => 2; }`, and every method in both must be scored. The first pair differs
	// by name and the second only by signature, so the duplicate test takes
	// both. ADR 0001's span identity, which the coverage join uses, stays file
	// and line range alone.
	seen := map[Span]bool{}
	for _, span := range parsed.Spans {
		converted := Span{
			File:       srcpath.FromSlash(span.File),
			Name:       span.Name,
			Signature:  span.Signature,
			StartLine:  span.StartLine,
			EndLine:    span.EndLine,
			Complexity: span.Complexity,
		}
		if err := e.checkNumbers(converted); err != nil {
			return nil, err
		}
		identity := converted
		identity.Complexity = 0
		if seen[identity] {
			return nil, &report.Failure{
				Code: report.CodeExtractorDuplicateSpan,
				Message: fmt.Sprintf("%s extractor returned %s twice, covering %s lines %d-%d",
					e.language, converted.Name, converted.File, converted.StartLine, converted.EndLine),
			}
		}
		seen[identity] = true
		spans = append(spans, converted)
	}
	return spans, nil
}

// checkNumbers rejects a span whose numbers cannot describe a method, which is
// the one part of the wire contract nothing else on this path reads back.
//
// Each of the three is a silent pass rather than a visible wrong answer. A
// complexity below 1 is arithmetically impossible, McCabe's base being 1, and an
// absent `complexity` field unmarshals to exactly that 0, which scores 0 and
// prints a passing row for a method nobody measured. A start line below 1 names
// no line of any file. An end line below its start gives the span a negative
// width, and the smallest-containing-span rule then never selects it, so the
// method leaves the table entirely and the run reports "no changed methods".
//
// The gate already refuses a mismatched path, a mismatched capability set, an
// absent parse status and a duplicated span on the same principle, that a broken
// extractor must not read as a clean run, so these take the same shape.
func (e extractor) checkNumbers(span Span) error {
	var wrong string
	switch {
	case span.Complexity < 1:
		wrong = fmt.Sprintf("complexity %d, which is below the McCabe base of 1", span.Complexity)
	case span.StartLine < 1:
		wrong = fmt.Sprintf("start line %d, which names no line", span.StartLine)
	case span.EndLine < span.StartLine:
		wrong = fmt.Sprintf("end line %d below start line %d", span.EndLine, span.StartLine)
	default:
		return nil
	}
	return &report.Failure{
		Code: report.CodeExtractorInvalidSpan,
		Message: fmt.Sprintf("%s extractor reported %s in %s with %s",
			e.language, span.Name, span.File, wrong),
	}
}

// echoedPaths lists every path the response mentions, in the order it
// mentions them.
func echoedPaths(parsed extraction) []string {
	paths := make([]string, 0, len(parsed.Files)+len(parsed.Spans))
	for _, file := range parsed.Files {
		paths = append(paths, file.File)
	}
	for _, span := range parsed.Spans {
		paths = append(paths, span.File)
	}
	return paths
}
