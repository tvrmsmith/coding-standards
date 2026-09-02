// Package gitscope answers the two questions ADR 0003 puts to git: which
// commit the run diffs against, and which lines that diff touched. The gate
// runs git itself rather than taking hunks from a wrapper, so `-w` and
// `--diff-filter` are fixed in one place and no caller can get them wrong.
//
// A gate that measures nothing passes, so anything able to reshape the diff
// into something the hunk parser reads as empty is a silent green build. Three
// separate mechanisms can do that and each needs its own answer. Ambient config
// such as `color.ui=always` is beaten by the `-c` overrides run pins on the
// command line. Environment config such as `GIT_EXTERNAL_DIFF`, or a clean
// filter injected through the GIT_CONFIG_COUNT family, outranks a `-c` flag and
// is instead dropped from the command's environment. Neither reaches
// `.gitattributes` or git's own NUL-byte autodetection, which live in the repo
// and in the file's bytes; `--text` on the diff answers those.
//
// The diff covers tracked paths only. A brand-new source file the developer
// has not yet `git add`ed contributes no touched lines and therefore no
// changed methods, which ADR 0003 records as a deliberate limitation of the
// merge-base scope rather than an oversight.
package gitscope

import (
	"bytes"
	"crypto/sha256"
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
// side of a pure `git mv` is the whole file. So an added path whose content,
// whitespace ignored, matches a path the same diff deleted is dropped
// afterwards, and only a move that also edited the file is measured.
func (r Repo) TouchedLines(base Base) (map[srcpath.Path][]int, error) {
	patch, err := r.git(append(diffFlags, "-w", "-U0", "--no-renames", "--diff-filter=ACM", base.Commit)...)
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
// The comparison ignores whitespace, because `TouchedLines` diffs with `-w`
// and one rule cannot hold two definitions of "changed". Comparing raw bytes
// would leave a `git mv` combined with a reindent looking like a whole-file
// add, and every method in it would demand coverage attribution, which is the
// wall of failures ADR 0003 gives `-w` to prevent.
//
// An added path the gate cannot read is left measured, which is the
// conservative direction.
//
// The pairing is one to one. Each deleted blob accounts for exactly one added
// path, so `git mv src/Old.cs src/New.cs` followed by copying the result to
// src/Copy.cs drops one of the two adds and leaves the other measured, because
// only one file's worth of content moved.
func (r Repo) pureMoves(base Base) ([]srcpath.Path, error) {
	raw, err := r.git(append(rawFlags, "-z", "--abbrev=40", "--no-renames", "--diff-filter=AD", base.Commit)...)
	if err != nil {
		return nil, err
	}
	added, deleted, err := parseRawAddsAndDeletes(raw)
	if err != nil {
		return nil, err
	}
	if len(added) == 0 || len(deleted) == 0 {
		return nil, nil
	}
	carried := map[[sha256.Size]byte]int{}
	for _, blob := range deleted {
		body, err := r.git("cat-file", "blob", blob)
		if err != nil {
			return nil, err
		}
		carried[squashedDigest([]byte(body))]++
	}
	var moves []srcpath.Path
	for _, path := range added {
		body, err := os.ReadFile(r.root.Abs(path))
		if err != nil {
			continue
		}
		digest := squashedDigest(body)
		if carried[digest] == 0 {
			continue
		}
		carried[digest]--
		moves = append(moves, path)
	}
	return moves, nil
}

// squashedDigest digests body in the form `git diff -w` compares it in: every
// whitespace character dropped from each line, and the line structure kept, so
// a reindent normalises away while a real edit does not.
func squashedDigest(body []byte) [sha256.Size]byte {
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), "")
	}
	return sha256.Sum256([]byte(strings.Join(lines, "\n")))
}

// gitlinkMode is the mode git gives a submodule entry. Its object id names a
// commit in another repository, not a blob this repo can read, so a removed
// submodule contributes nothing to the deleted-side content.
const gitlinkMode = "160000"

// parseRawAddsAndDeletes reads `git diff --raw -z` records, returning the
// paths the diff added and the old-side blob ids it deleted. A record is the
// metadata field ":<mode> <mode> <src> <dst> <status>" followed by the path,
// both NUL-terminated.
func parseRawAddsAndDeletes(raw string) (added []srcpath.Path, deleted []string, err error) {
	records := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	if len(records) == 1 && records[0] == "" {
		return nil, nil, nil
	}
	if len(records)%2 != 0 {
		return nil, nil, fmt.Errorf("git diff --raw emitted %d fields, want pairs", len(records))
	}
	for i := 0; i < len(records); i += 2 {
		fields := strings.Fields(records[i])
		if len(fields) != 5 {
			return nil, nil, fmt.Errorf("git diff --raw record is malformed: %q", records[i])
		}
		oldMode, newMode, src, status := strings.TrimPrefix(fields[0], ":"), fields[1], fields[2], fields[4]
		switch status {
		case "A":
			if newMode == gitlinkMode {
				continue
			}
			added = append(added, srcpath.FromSlash(records[i+1]))
		case "D":
			if oldMode == gitlinkMode {
				continue
			}
			deleted = append(deleted, src)
		}
	}
	return added, deleted, nil
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

// configOverrides pin, per invocation, every git setting that can reshape the
// output these parsers read. They go on the command line rather than being
// read from the repo, so a hostile or merely unusual .gitconfig cannot turn a
// diff the parser understands into one it silently reads as empty, which would
// pass the gate with no changed methods.
var configOverrides = []string{
	"-c", "core.quotePath=false",
	"-c", "color.ui=false",
	"-c", "diff.external=",
	"-c", "diff.noprefix=false",
	"-c", "diff.mnemonicPrefix=false",
	"-c", "diff.srcPrefix=a/",
	"-c", "diff.dstPrefix=b/",
	"-c", "diff.suppressBlankEmpty=false",
	"-c", "diff.wsErrorHighlight=none",
}

// diffFlags harden the unified diff the hunk parser reads. The prefixes are
// repeated as flags because they are what parseNewSidePath strips. `--text`
// forces a hunk out of a file git would otherwise summarise as "Binary files
// differ", which is any source carrying a NUL byte, UTF-16 for instance, and
// any path a `.gitattributes` line marks `-diff` or `binary`. Only the hunk
// headers are read, so whatever the cells hold does not matter.
var diffFlags = []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--text", "--src-prefix=a/", "--dst-prefix=b/"}

// rawFlags harden the `--raw` listing, which carries no prefixes.
var rawFlags = []string{"diff", "--no-color", "--no-ext-diff", "--raw"}

// hostileEnv names the environment variables that reshape diff output or
// inject config. They are dropped rather than overridden, because a `-c` flag
// cannot outrank GIT_CONFIG_PARAMETERS.
var hostileEnv = []string{
	"GIT_EXTERNAL_DIFF",
	"GIT_DIFF_OPTS",
	"GIT_CONFIG_PARAMETERS",
	"GIT_CONFIG_COUNT",
}

// hostileEnvPrefixes names the indexed families GIT_CONFIG_COUNT enumerates.
var hostileEnvPrefixes = []string{"GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_"}

// git runs one git command in the repo root.
func (r Repo) git(args ...string) (string, error) {
	return run(r.root.Dir(), args...)
}

// run executes git in dir, or the process working directory when dir is
// empty, and returns its stdout.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append(append([]string{}, configOverrides...), args...)...)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// sanitizedEnv is the process environment with the diff-reshaping variables
// removed. It is built rather than inherited, so what the parser reads does
// not depend on how the caller's shell was set up.
func sanitizedEnv() []string {
	env := os.Environ()
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		if !hostile(entry) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// hostile reports whether a "KEY=value" entry names a variable run drops.
func hostile(entry string) bool {
	name, _, _ := strings.Cut(entry, "=")
	for _, banned := range hostileEnv {
		if name == banned {
			return true
		}
	}
	for _, prefix := range hostileEnvPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
