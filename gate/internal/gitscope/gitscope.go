// Package gitscope answers the two questions ADR 0003 puts to git: which
// commit the run diffs against, and which lines that diff touched. The gate
// runs git itself rather than taking hunks from a wrapper, so `-w` and
// `--diff-filter` are fixed in one place and no caller can get them wrong.
//
// The diff covers tracked paths only. A brand-new source file the developer
// has not yet `git add`ed contributes no touched lines and therefore no
// changed methods, which ADR 0003 records as a deliberate limitation of the
// merge-base scope rather than an oversight.
package gitscope

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// BaseCandidates is ADR 0003's base resolution order. There is no HEAD~1
// fallback: a silently different base is the failure the caller cannot
// detect.
var BaseCandidates = []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"}

// Repo is a git working tree the gate measures.
type Repo struct {
	root srcpath.Root
}

// Open finds the repo containing the process working directory.
func Open() (Repo, error) {
	out, err := run("", "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, err
	}
	root, err := srcpath.NewRoot(strings.TrimSpace(out))
	if err != nil {
		return Repo{}, err
	}
	return Repo{root: root}, nil
}

// Root is the repo's resolved root, the gate's one path currency.
func (r Repo) Root() srcpath.Root { return r.root }

// Base is the commit the run diffs against, and the ref it was reached
// through.
type Base struct {
	Ref    string
	Commit string
}

// Label renders the base as the document's `base` field, "<ref>@<7-char sha>".
func (b Base) Label() string { return b.Ref + "@" + b.Commit[:7] }

// NoBaseError reports that none of ADR 0003's candidate refs resolved.
type NoBaseError struct{}

func (NoBaseError) Error() string {
	return "no diff base: tried " + strings.Join(BaseCandidates, ", ")
}

// ResolveBase walks BaseCandidates and returns the merge base of HEAD and
// the first candidate that both exists and shares history with HEAD.
func (r Repo) ResolveBase() (Base, error) {
	for _, ref := range BaseCandidates {
		if _, err := r.git("rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
			continue
		}
		mergeBase, err := r.git("merge-base", "HEAD", ref)
		if err != nil {
			continue
		}
		return Base{Ref: ref, Commit: strings.TrimSpace(mergeBase)}, nil
	}
	return Base{}, NoBaseError{}
}

// TouchedLines returns the new-side lines of `git diff -w -U0 --no-renames
// --diff-filter=ACM <base>`, keyed by source path and ascending within a
// file. A zero-length hunk, which is what a pure deletion produces, touches
// the line at its insertion point.
//
// `--no-renames` is not optional. With git's default rename detection on, a
// file git scores as a rename carries status R, `--diff-filter=ACM` drops it
// entirely, and a method that gained a decision point on the way to its new
// path is never scored. Decomposing the rename into a delete plus an add
// gives the add side every line, which is what ADR 0003 means by "a method
// moved between files appears as added lines at its new location".
//
// Decomposition alone would break ADR 0003's other sentence, that a file
// renamed with no content change reports no touched lines, because the add
// side of a pure `git mv` is the whole file. So an added path whose content is
// byte-identical to a path the same diff deleted is dropped afterwards, and
// only a move that also edited the file is measured.
func (r Repo) TouchedLines(base Base) (map[srcpath.Path][]int, error) {
	patch, err := r.git("-c", "core.quotePath=false", "diff", "-w", "-U0", "--no-renames", "--diff-filter=ACM", base.Commit)
	if err != nil {
		return nil, err
	}
	touched, err := parseTouchedLines(patch)
	if err != nil {
		return nil, err
	}
	moved, err := r.pureMoves(base)
	if err != nil {
		return nil, err
	}
	for _, path := range moved {
		delete(touched, path)
	}
	return touched, nil
}

// pureMoves lists the added paths carrying content some deleted path in the
// same diff carried, which is what a rename looks like once `--no-renames`
// has split it in two.
//
// Git leaves the new-side blob id all zeroes whenever the working tree copy
// differs from what is staged, so a file moved and then edited never matches
// and stays measured, which is the conservative direction.
func (r Repo) pureMoves(base Base) ([]srcpath.Path, error) {
	raw, err := r.git("diff", "--raw", "-z", "--abbrev=40", "--no-renames", "--diff-filter=AD", base.Commit)
	if err != nil {
		return nil, err
	}
	added, deletedBlobs, err := parseRawAddsAndDeletes(raw)
	if err != nil {
		return nil, err
	}
	var moves []srcpath.Path
	for path, blob := range added {
		if deletedBlobs[blob] {
			moves = append(moves, path)
		}
	}
	return moves, nil
}

// parseRawAddsAndDeletes reads `git diff --raw -z` records, returning each
// added path with its new-side blob id and the set of blob ids the diff
// deleted. A record is the metadata field ":<mode> <mode> <src> <dst> <status>"
// followed by the path, both NUL-terminated.
func parseRawAddsAndDeletes(raw string) (added map[srcpath.Path]string, deleted map[string]bool, err error) {
	added, deleted = map[srcpath.Path]string{}, map[string]bool{}
	records := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	if len(records) == 1 && records[0] == "" {
		return added, deleted, nil
	}
	if len(records)%2 != 0 {
		return nil, nil, fmt.Errorf("git diff --raw emitted %d fields, want pairs", len(records))
	}
	for i := 0; i < len(records); i += 2 {
		fields := strings.Fields(records[i])
		if len(fields) != 5 {
			return nil, nil, fmt.Errorf("git diff --raw record is malformed: %q", records[i])
		}
		src, dst, status := fields[2], fields[3], fields[4]
		switch status {
		case "A":
			if !isNullBlob(dst) {
				added[srcpath.FromSlash(records[i+1])] = dst
			}
		case "D":
			deleted[src] = true
		}
	}
	return added, deleted, nil
}

// isNullBlob reports whether an id is git's all-zero placeholder, which the
// new side carries when git did not hash the working tree copy.
func isNullBlob(id string) bool {
	return strings.Trim(id, "0") == ""
}

// parseTouchedLines reads hunk headers out of a unified diff.
//
// A "+++ " line only names the new-side file while the parser is inside a
// file's preamble, between its `diff --git` line and its first hunk. Inside a
// hunk body, an added line whose own text starts with "++ " renders as
// "+++ x", and treating that as a file header would silently reattribute
// every later hunk of the real file to a path that does not exist.
func parseTouchedLines(patch string) (map[srcpath.Path][]int, error) {
	touched := map[srcpath.Path][]int{}
	var current srcpath.Path
	inPreamble := false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inPreamble = true
		case inPreamble && strings.HasPrefix(line, "+++ "):
			path, err := parseNewSidePath(strings.TrimPrefix(line, "+++ "))
			if err != nil {
				return nil, err
			}
			current = path
			inPreamble = false
		case strings.HasPrefix(line, "@@ "):
			inPreamble = false
			start, count, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			touched[current] = append(touched[current], hunkLines(start, count)...)
		}
	}
	return touched, nil
}

// hunkLines expands a new-side hunk range. A count of zero is a pure
// deletion, which touches the single line it was removed from; git reports
// that insertion point as 0 when the deletion is at the top of the file, and
// the first line is the nearest line that exists.
func hunkLines(start, count int) []int {
	if count == 0 {
		if start < 1 {
			start = 1
		}
		return []int{start}
	}
	lines := make([]int, 0, count)
	for i := 0; i < count; i++ {
		lines = append(lines, start+i)
	}
	return lines
}

// parseNewSidePath strips the "b/" prefix git puts on the new-side path,
// unquoting it first when git had to quote it.
func parseNewSidePath(field string) (srcpath.Path, error) {
	if strings.HasPrefix(field, `"`) {
		unquoted, err := strconv.Unquote(field)
		if err != nil {
			return "", fmt.Errorf("unquoting diff path %s: %w", field, err)
		}
		field = unquoted
	}
	return srcpath.FromSlash(strings.TrimPrefix(field, "b/")), nil
}

// parseHunkHeader reads the new-side start line and line count out of a
// "@@ -a,b +c,d @@" header, where a missing count means one line.
func parseHunkHeader(header string) (start, count int, err error) {
	_, rest, ok := strings.Cut(header, "+")
	if !ok {
		return 0, 0, fmt.Errorf("hunk header has no new-side range: %s", header)
	}
	rangeField, _, ok := strings.Cut(rest, " ")
	if !ok {
		return 0, 0, fmt.Errorf("hunk header is malformed: %s", header)
	}
	startField, countField, hasCount := strings.Cut(rangeField, ",")
	start, err = strconv.Atoi(startField)
	if err != nil {
		return 0, 0, fmt.Errorf("hunk header start line %q: %w", startField, err)
	}
	count = 1
	if hasCount {
		count, err = strconv.Atoi(countField)
		if err != nil {
			return 0, 0, fmt.Errorf("hunk header line count %q: %w", countField, err)
		}
	}
	return start, count, nil
}

// git runs one git command in the repo root.
func (r Repo) git(args ...string) (string, error) {
	return run(r.root.Dir(), args...)
}

// run executes git in dir, or the process working directory when dir is
// empty, and returns its stdout.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
