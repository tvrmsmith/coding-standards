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
	"slices"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// languageBinaries is the built-in table ADR 0006 fixes: a language yields a
// binary *name*, looked up beside the gate. Nothing on PATH, no repo config
// file, and no environment variable participates.
var languageBinaries = map[string]string{
	"csharp": "metric-gate-csharp",
}

// Span is one method's identity and complexity, as the extractor reports it.
type Span struct {
	File       srcpath.Path
	Name       string
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
	// No changed file means no extractor can be asked for anything, so none
	// is located or launched. A docs-only change therefore passes in a
	// deployment that ships the gate without an extractor beside it.
	if len(changed) == 0 {
		return Result{Claimed: map[srcpath.Path]bool{}}, nil
	}
	dir, err := extractorDir()
	if err != nil {
		return Result{}, err
	}
	result := Result{Claimed: map[srcpath.Path]bool{}}
	for _, language := range sortedLanguages() {
		e := extractor{language: language, binary: filepath.Join(dir, languageBinaries[language]), root: root}
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

// extractorDir is the directory the gate's own binary sits in, which is the
// only place an extractor is looked for.
func extractorDir() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the gate binary: %w", err)
	}
	return filepath.Dir(self), nil
}

// sortedLanguages keeps the order the languages are consulted in fixed, so a
// run's output does not depend on map iteration.
func sortedLanguages() []string {
	return slices.Sorted(maps.Keys(languageBinaries))
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
	if len(claimed) == 0 {
		return nil, nil, nil
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
	return caps, nil
}

// filter selects the changed files carrying one of the extractor's
// extensions. ADR 0004 rejects case folding, so an extension matches as
// written.
func (e extractor) filter(changed []srcpath.Path, extensions []string) []srcpath.Path {
	var mine []srcpath.Path
	for _, path := range changed {
		for _, ext := range extensions {
			if path.Ext() == ext {
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
		Message: fmt.Sprintf("%s extractor %s could not be run: %v", e.language, filepath.Base(e.binary), err),
	}
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
	for _, span := range parsed.Spans {
		spans = append(spans, Span{
			File:       srcpath.FromSlash(span.File),
			Name:       span.Name,
			StartLine:  span.StartLine,
			EndLine:    span.EndLine,
			Complexity: span.Complexity,
		})
	}
	return spans, nil
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
