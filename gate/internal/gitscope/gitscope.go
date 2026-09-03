// Package gitscope answers the two questions ADR 0003 puts to git: which
// commit the run diffs against, and which lines that diff touched. The gate
// runs git itself rather than taking hunks from a wrapper, so `-w` and
// `--diff-filter` are fixed in one place and no caller can get them wrong.
//
// A gate that measures nothing passes, so anything able to reshape the diff
// into something the hunk parser reads as empty is a silent green build. Four
// separate mechanisms can do that and each needs its own answer.
//
// Ambient config such as `color.ui=always` is beaten by the `-c` overrides run
// pins on the command line. The environment outranks a `-c` flag, whether it
// reshapes the diff directly (GIT_EXTERNAL_DIFF), injects config
// (GIT_CONFIG_PARAMETERS), or points the whole run at another repository
// (GIT_DIR), so git's namespace is dropped from the command's environment
// entirely and the two config-file variables are then pinned at the null device,
// which is what keeps the scrub from handing the run back to an ambient
// ~/.gitconfig.
//
// A content filter is the third, and neither of the first two answers reach it.
// A filter driver is named by the repository's own .git/config, which the pins
// leave in place because a repo-local key is the only thing that can say what a
// checkout means, and it is selected by a `.gitattributes` line, which lives in
// the tree. That is where git-lfs and git-crypt install themselves. A clean
// driver runs over the working-tree side of every diff, so one that prints its
// input back unchanged, or prints nothing, empties the patch. There is no flag
// that turns filtering off, so the drivers the repo configures are enumerated
// and each is blanked with its own `-c` override, which is protected config and
// outranks the repo-local value that named it.
//
// None of the three reach git's own NUL-byte autodetection or a `.gitattributes`
// line marking a source file `-diff`, which live in the tree and in the file's
// bytes; `--text` on the diff answers those.
//
// The diff covers tracked paths only. A brand-new source file the developer
// has not yet `git add`ed contributes no touched lines and therefore no
// changed methods, which ADR 0003 records as a deliberate limitation of the
// merge-base scope rather than an oversight.
package gitscope

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
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
// whitespace ignored, is the only match for a path the same diff deleted is
// dropped afterwards, and only a move that also edited the file is measured.
//
// Nothing gets out of here untyped. Base resolution has already succeeded, so
// the document exists and ADR 0005's one-document rule binds: every cause below
// this line, a git invocation that failed as much as a patch the parser refused,
// comes back as a report.Failure so main can put it in the document's error
// block. Exiting 1 with an empty stdout instead is a shape the caller cannot
// tell from a crash, and typing the boundary rather than the individual return
// sites is what stops the next cause added underneath it reopening that hole.
func (r Repo) TouchedLines(base Base) (map[srcpath.Path][]int, error) {
	touched, err := r.touchedLines(base)
	if err != nil {
		return nil, unreadableDiff(err)
	}
	return touched, nil
}

func (r Repo) touchedLines(base Base) (map[srcpath.Path][]int, error) {
	neutralized, err := r.blankedFilterDrivers()
	if err != nil {
		return nil, err
	}
	patch, err := r.git(append(neutralized, append(diffFlags, "-w", "-U0", "--no-renames", "--diff-filter=ACM", base.Commit)...)...)
	if err != nil {
		return nil, err
	}
	touched, err := parseTouchedLines(patch)
	if err != nil {
		return nil, err
	}
	moved, err := r.pureMoves(base, neutralized)
	if err != nil {
		return nil, err
	}
	for _, path := range moved {
		delete(touched, path)
	}
	return touched, nil
}

// blankedFilterDrivers is a `-c <key>=` override for every content filter the
// repository configures, which is what a diff has to be run under for a clean
// driver not to decide what the gate can see.
//
// The drivers are enumerated rather than named, because their names are the
// repository's to choose. All three keys of the interface are blanked. `.clean`
// is the one-shot driver, `.process` is the long-running protocol git prefers
// when it is set, which is the half git-lfs actually installs, and blanking the
// latter makes git fall back to the former, which is blanked beside it.
// `.required` is the third: with it left true and the driver blanked, git aborts
// the diff with "clean filter failed" instead of passing the content through, so
// every run in a repository that ran `git lfs install --local` or git-crypt
// would exit 1. Blanking all three is what makes those repositories measurable.
//
// In a git-lfs repository this measures the pointer-expanded content rather than
// the pointer file. That is the same text Roslyn parses off the working tree, so
// the two halves of the measurement agree.
//
// The keys come back NUL-separated. A filter's subsection name is arbitrary text
// and may hold spaces, so splitting the listing on whitespace would break
// `filter.my driver.clean` into two fragments, leave the real driver installed,
// and hand git a `-c .clean=` it refuses to parse.
func (r Repo) blankedFilterDrivers() ([]string, error) {
	out, err := r.git("config", "--name-only", "--get-regexp", "-z", filterDriverKeys)
	if noMatch(err) {
		// `git config --get-regexp` exits 1 on no match, which is the ordinary
		// case of a repository configuring no filter at all.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var overrides []string
	for _, key := range strings.Split(strings.TrimSuffix(out, "\x00"), "\x00") {
		if key == "" {
			continue
		}
		overrides = append(overrides, "-c", key+"=")
	}
	return overrides, nil
}

// filterDriverKeys matches every spelling of a content filter driver.
const filterDriverKeys = `^filter\..*\.(clean|process|required)$`

// noMatch reports whether err is `git config --get-regexp` finding nothing,
// which it reports as exit 1 with no output. Any other exit code is a real
// failure to read the repository's config and is not an empty answer.
func noMatch(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
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
// Every unreadable side resolves towards measuring, which is the conservative
// direction. An added path the gate cannot read stays measured, and so does
// every add paired with a deleted object the gate cannot read: a `cat-file`
// failure, which is what a blobless partial clone gives offline, drops that
// object from the comparison rather than escaping and leaving the run with no
// document at all.
//
// Ambiguity resolves by counting rather than by picking a winner. For each
// content digest the diff compares how many paths were added carrying it
// against how many were deleted carrying it. Added no more than deleted means
// every add is accounted for by a delete, so all of them are dropped, and moving
// two identical files together measures nothing, which is the pure-move rule.
// Added more than deleted means content appeared that the deleted side does not
// explain, so none are dropped and `git mv src/Old.cs src/New.cs` followed by
// copying the result to src/Copy.cs leaves both adds measured. `--no-renames` is
// what removes git's own answer to which of the two is the move, and guessing
// would silently unscore a brand-new file. Counting depends on no `git diff
// --raw` ordering, so the answer is the same whichever order git lists them in.
func (r Repo) pureMoves(base Base, neutralized []string) ([]srcpath.Path, error) {
	raw, err := r.git(append(neutralized, append(rawFlags, "-z", "--abbrev=40", "--no-renames", "--diff-filter=AD", base.Commit)...)...)
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
			continue
		}
		carried[squashedDigest([]byte(body))]++
	}
	digests := map[srcpath.Path][sha256.Size]byte{}
	claimants := map[[sha256.Size]byte]int{}
	for _, path := range added {
		body, err := os.ReadFile(r.root.Abs(path))
		if err != nil {
			continue
		}
		digest := squashedDigest(body)
		digests[path] = digest
		claimants[digest]++
	}
	var moves []srcpath.Path
	for _, path := range added {
		digest, read := digests[path]
		if !read || carried[digest] == 0 || claimants[digest] > carried[digest] {
			continue
		}
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
//
// git appends a TAB to the name on a "---" or "+++" line whenever the path
// holds a space, so a reader can tell where a name with spaces ends, and it
// does that whether or not the name is quoted. The TAB sits outside the closing
// quote, so it has to come off before the quote check: left on the unquoted
// form it makes the extension read ".cs\t", no extractor is located for the
// file, and every changed method under a directory with a space in its name
// goes unmeasured under a pass. A path whose own last character is a TAB is
// quoted and carries that TAB escaped inside the quotes, so trimming one here
// can never eat part of a name.
func parseNewSidePath(field string) (srcpath.Path, error) {
	field = strings.TrimSuffix(field, "\t")
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

// unreadableDiff types whatever went wrong between asking git for the diff and
// having a set of touched lines. One code covers the whole stretch because every
// cause on it says the same thing to a caller, that the gate could not establish
// what the change touched and therefore measured nothing, and because a code per
// cause would be a list to extend every time a line is added under the boundary.
func unreadableDiff(err error) error {
	var failure *report.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return &report.Failure{
		Code:    report.CodeDiffUnparseable,
		Message: "could not read the diff: " + cause(err),
	}
}

// configOverrides pin, per invocation, every git setting that can reshape the
// output these parsers read. They go on the command line rather than being
// read from the repo, so a hostile or merely unusual .gitconfig cannot turn a
// diff the parser understands into one it silently reads as empty, which would
// pass the gate with no changed methods.
//
// `core.fsmonitor` and `core.pager` are the two entries that are not about the
// shape of the output either. Both name a program the repository chooses and
// git runs. git executes the fsmonitor hook to refresh the index, which a diff
// does on every invocation, and it spawns the pager whenever stdout is a
// terminal. Blanking the first turns the refresh back into a plain stat walk,
// and pinning the second at `cat` stops a repo-named pager from standing between
// git and the parsers.
//
// `safe.directory` is the entry that pays for those two. git honours it only
// from protected config, which the pinned-empty config files no longer supply,
// so without it the gate refuses any working tree owned by another uid and a
// container runner that mounts the checkout gets exit 1 where the developer's
// own git works. What the ownership check buys is that a repository's own config
// cannot run a program on behalf of whoever wanders into the directory, and the
// four subcommands the gate runs, diff, rev-parse, merge-base and cat-file, reach
// exactly four repo-named programs: the filter drivers, which blankedFilterDrivers
// blanks, the external diff and textconv drivers, which `--no-ext-diff` and
// `--no-textconv` refuse, and these two. None of the four survives, and the gate
// never writes to the tree.
var configOverrides = []string{
	"-c", "safe.directory=*",
	"-c", "core.fsmonitor=",
	"-c", "core.pager=cat",
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

// gitNamespace is the prefix of git's own environment namespace, and run drops
// every variable carrying it.
//
// The whole namespace goes rather than a list of the dangerous ones, because
// naming them is a list that has to be extended for every variable somebody
// thinks of and the one nobody thought of is the same silent pass again.
// GIT_EXTERNAL_DIFF and GIT_DIFF_OPTS reshape the diff, GIT_CONFIG_PARAMETERS
// and the GIT_CONFIG_COUNT family inject config a `-c` flag cannot outrank, and
// GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE and their relatives outrank cmd.Dir and
// answer every question about a different repository. That last family is not
// exotic: git exports it into every hook it runs, and a pre-commit hook is what
// this gate is built to be.
//
// Nothing in the namespace is needed to run git. The repo comes from cmd.Dir
// and the settings the parsers depend on come from configOverrides.
const gitNamespace = "GIT_"

// pinnedConfigFiles is what run puts back after the scrub.
//
// Dropping GIT_CONFIG_GLOBAL and GIT_CONFIG_SYSTEM with the rest of the
// namespace sends git to its default config locations instead, so the run would
// read whatever ~/.gitconfig and /etc/gitconfig happen to hold. That is the same
// hole the scrub exists to close, only reached through a file rather than a
// variable: a `filter.*.clean` entry there installs a driver over every file the
// diff reads, and nothing on the command line turns filtering off. Both are
// therefore pinned at the null device, an empty file on every platform Go names
// it for, and the run carries the settings it needs on the command line.
//
// This does not answer the same key set in the repository's own .git/config,
// which the pins deliberately leave readable. blankedFilterDrivers is what
// covers that scope.
var pinnedConfigFiles = []string{
	"GIT_CONFIG_GLOBAL=" + os.DevNull,
	"GIT_CONFIG_SYSTEM=" + os.DevNull,
}

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
		return "", &gitError{args: args, err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return stdout.String(), nil
}

// gitError is one failed git invocation. The two halves are kept apart because
// they have different readers: the whole thing, argv included, is what a
// developer needs off stderr, while the document's error block wants git's own
// complaint on its own, since the argv is a fixed flag list carrying nothing a
// caller can act on.
type gitError struct {
	args   []string
	err    error
	stderr string
}

func (e *gitError) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.args, " "), e.err, e.stderr)
}

func (e *gitError) Unwrap() error { return e.err }

// cause is what git said, or the whole error when it was not git that spoke.
func cause(err error) string {
	var gitErr *gitError
	if errors.As(err, &gitErr) && gitErr.stderr != "" {
		return gitErr.stderr
	}
	return err.Error()
}

// sanitizedEnv is the process environment with git's own namespace removed and
// the two config-file variables pinned. It is built rather than inherited, so
// neither what the parser reads, nor which repository it reads it from, nor
// whose config it reads it under depends on how the caller's shell was set up.
func sanitizedEnv() []string {
	env := os.Environ()
	kept := make([]string, 0, len(env)+len(pinnedConfigFiles))
	for _, entry := range env {
		if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, gitNamespace) {
			continue
		}
		kept = append(kept, entry)
	}
	return append(kept, pinnedConfigFiles...)
}
