package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

// runLine is one line of lint-runs.jsonl. Field order is the on-disk format,
// shared with the readers of the log, so it is part of the contract.
type runLine struct {
	TS       int64          `json:"ts"`
	Repo     string         `json:"repo"`
	Branch   string         `json:"branch"`
	Head     string         `json:"head"`
	Lang     string         `json:"lang"`
	Mode     string         `json:"mode"`
	Gate     bool           `json:"gate"`
	Blocked  bool           `json:"blocked"`
	Findings map[string]int `json:"findings"`
}

// langOfFinding maps a --lang to the lintfind language its findings carry.
var langOfFinding = map[string]string{
	"ts":     lintfind.LanguageTS,
	"go":     lintfind.LanguageGo,
	"dotnet": lintfind.LanguageCSharp,
}

// runLogRun appends one line per language branch that ran to the run log, in
// one write. The caller ignores the exit status, so a failure only has to be
// loud on stderr: 1 for an unreadable --kept or an unwritable log. A git
// lookup that fails yields an empty field rather than a failure.
func runLogRun(la LogRunArgs, _, stderr io.Writer) int {
	counts, err := keptCounts(la.Kept)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	now := time.Now().Unix()
	repo, branch, head := gitOrigin(), gitBranch(), gitHead()
	var buf bytes.Buffer
	for _, lang := range la.Langs {
		findings := counts[langOfFinding[lang]]
		if findings == nil {
			findings = map[string]int{}
		}
		b, err := json.Marshal(runLine{
			TS: now, Repo: repo, Branch: branch, Head: head, Lang: lang, Mode: la.Mode,
			Gate: os.Getenv("NM_GATE") == "1", Blocked: la.Status == 2, Findings: findings,
		})
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "encoding run log line: %v\n", err)
			return 1
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}

	if err := appendRunLog(buf.Bytes()); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// keptCounts reads the filter's --kept file into rule counts per lintfind
// language.
func keptCounts(path string) (map[string]map[string]int, error) {
	body, err := os.ReadFile(path) //nolint:gosec // G304: the dispatcher names the file its own filter run just wrote
	if err != nil {
		return nil, fmt.Errorf("reading kept findings: %w", err)
	}
	counts := map[string]map[string]int{}
	for _, line := range strings.Split(string(body), "\n") {
		lang, rule, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		if counts[lang] == nil {
			counts[lang] = map[string]int{}
		}
		counts[lang][rule]++
	}
	return counts, nil
}

// appendRunLog writes data to the run log with a single O_APPEND write, so
// concurrent runs never interleave inside one call's lines.
func appendRunLog(data []byte) error {
	dir := stateDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating run log directory: %w", err)
	}
	path := filepath.Join(dir, "lint-runs.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: path under the state directory
	if err != nil {
		return fmt.Errorf("opening run log: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing run log %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing run log %s: %w", path, err)
	}
	return nil
}

// gitOrigin is the origin URL with any userinfo removed, so a token embedded
// in a remote never reaches the log.
func gitOrigin() string {
	raw := gitOut("remote", "get-url", "origin")
	if !strings.Contains(raw, "://") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User = nil
	return u.String()
}

func gitBranch() string { return gitOut("symbolic-ref", "--short", "-q", "HEAD") }

func gitHead() string { return gitOut("rev-parse", "--verify", "-q", "HEAD") }

// gitOut is git's trimmed stdout, or "" on any failure: a lookup that cannot
// answer is an empty field, never a failed call.
func gitOut(args ...string) string {
	//nolint:gosec // G204: the literal "git" with fixed argv from the callers above; no caller passes user input.
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
