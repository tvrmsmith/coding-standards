// Package gitscope answers the two questions ADR 0007 puts to git: which
// commit the run diffs against, and which lines that diff touched. This
// package runs git itself rather than taking hunks from a wrapper, so `-w` and
// `--diff-filter` are fixed in one place and no caller can get them wrong.
//
// A run that measures nothing passes, so anything able to reshape the diff
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
// ~/.gitconfig. The scrub has one exception, and it is per-Repo rather than
// global: a Repo from OpenHook puts GIT_INDEX_FILE back into its own commands,
// for the reason OpenHook gives.
//
// A content filter is the third, and neither of the first two answers reach it.
// A filter driver is named by the repository's own .git/config, which the pins
// leave in place because a repo-local key is the only thing that can say what a
// checkout means, and it is selected by a `.gitattributes` line, which lives in
// the tree. That is where git-lfs and git-crypt install themselves. A clean
// driver runs over the working-tree side of every diff, so one that prints its
// input back unchanged, or prints nothing, empties the patch. There is no flag
// that turns filtering off, so the drivers the repo configures are enumerated
// and each is set empty in command scope, which outranks the repo-local value
// that named it. The blanks travel through the GIT_CONFIG_COUNT family rather
// than `-c`, because the key carries a name the repository chose and a `-c`
// argument splits on its first `=`.
//
// None of the three reach git's own NUL-byte autodetection or a `.gitattributes`
// line marking a source file `-diff`, which live in the tree and in the file's
// bytes; `--text` on the diff answers those.
//
// The diff covers tracked paths only. A brand-new source file the developer
// has not yet `git add`ed contributes no touched lines and therefore no
// changed methods. ADR 0007 states this as a flat rule for every diff scope:
// touched lines come from tracked paths only, and a file never added to the
// index contributes nothing.
package gitscope

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// BaseCandidates is ADR 0007's base resolution order. There is no HEAD~1
// fallback: a silently different base is the failure the caller cannot
// detect.
var BaseCandidates = []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"}

// Repo is a git working tree the gate measures.
type Repo struct {
	root srcpath.Root
	// index is the index file every git command this Repo runs reads, empty
	// for the repository's own .git/index. Only OpenHook sets it.
	index string
}

// Open finds the repo containing the process working directory. Every failure
// point comes back as an OpenError, so a run started outside a git repository
// gets a typed cause and a document rather than the empty stdout ADR 0008
// reserves for a malformed command line (issue 86).
//
// A toplevel git named that cannot be resolved is OpenRootUnresolvable: the
// repository is there and the filesystem is what failed. openFailure separates
// the rest.
func Open() (Repo, error) {
	out, err := run("", nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, openFailure(err)
	}
	root, err := srcpath.NewRoot(strings.TrimSpace(out))
	if err != nil {
		return Repo{}, OpenError{
			Kind:    OpenRootUnresolvable,
			Message: "could not resolve the repo root git named: " + cause(err),
		}
	}
	return Repo{root: root}, nil
}

// OpenHook is Open for a run git started as a pre-commit hook, where the index
// named in GIT_INDEX_FILE is the index the commit will write. Every git command
// the returned Repo runs reads that index.
//
// run scrubs GIT_INDEX_FILE with the rest of git's namespace, and for a run
// nothing handed an index that is right: the variable is then ambient input
// from whatever shell started the process, naming another repository's index.
// Under a hook it is the question being asked. `git commit -a` and
// `git commit -- <pathspec>` build a temporary index out of the working tree
// and point the hook at it, so a scrubbed run reads .git/index, which for `-a`
// still matches HEAD: every finding passes and the divergence hard stop sees no
// staged path at all. Naming the index here rather than keeping the variable in
// the scrub keeps the carve-out on the one caller git actually handed one.
func OpenHook() (Repo, error) {
	repo, err := Open()
	if err != nil {
		return Repo{}, err
	}
	repo.index = os.Getenv("GIT_INDEX_FILE")
	return repo, nil
}

// OpenKind names which of Open's failure points a caller is looking at. It is
// this package's own vocabulary rather than a document code, for the reason
// UnreadableDiffError gives: the words a cause is reported in belong to the
// caller. metric-gate maps each kind onto one of ADR 0008's codes at the single
// site that already maps this package's other errors.
type OpenKind int

const (
	// OpenGitUnavailable is git never running at all.
	OpenGitUnavailable OpenKind = iota
	// OpenRepoUnreadable is a repository git found and then refused to answer
	// about.
	OpenRepoUnreadable
	// OpenNoRepo is git answering that there is no repository here.
	OpenNoRepo
	// OpenRootUnresolvable is a toplevel git named that would not resolve to a
	// root path.
	OpenRootUnresolvable
)

// OpenError is Open failing to establish which repository the run measures. It
// carries a rendered message beside the kind, so a caller with no document
// prints the sentence and one with a document picks its own code off Kind.
type OpenError struct {
	Kind    OpenKind
	Message string
}

func (e OpenError) Error() string { return e.Message }

// openFailure types a `rev-parse --show-toplevel` that did not answer. The
// three kinds are apart because a caller branches on the kind rather than on
// the message, and each names a different thing to go and fix.
//
// git never running at all is OpenGitUnavailable, which covers a runner with no
// git on PATH and one whose git is not executable, since both come back as
// *exec.Error and neither is answered by running `git init`. The message is
// exec's own words rather than cause's, because there is no stderr to quote
// and the argv would be all that survived.
//
// A repository git did find, and then refused to answer about, is
// OpenRepoUnreadable: a bare repository, which rev-parse says must be run in
// a work tree, or a .git the store cannot read. The two are told apart by
// asking git for the git directory rather than by reading its sentence, since
// the sentence is English and git ships translations. That probe answers for
// every repository git can open, work tree or not, so exit 0 means a
// repository is there and the toplevel is what could not be had.
//
// OpenNoRepo is what is left, git answering that there is no repository here
// at all, the one a caller fixes by running `git init` or by starting the run
// somewhere else. Its message carries git's own complaint rather than the
// argv, the convention UnreadableDiffError follows.
func openFailure(err error) error {
	var launch *exec.Error
	if errors.As(err, &launch) {
		return OpenError{
			Kind:    OpenGitUnavailable,
			Message: "could not run git: " + launch.Error(),
		}
	}
	if _, probe := run("", nil, "rev-parse", "--git-dir"); probe == nil {
		return OpenError{
			Kind:    OpenRepoUnreadable,
			Message: "could not read the git repository: " + cause(err),
		}
	}
	return OpenError{
		Kind:    OpenNoRepo,
		Message: "could not find a git repository: " + cause(err),
	}
}

// Root is the repo's resolved root, the gate's one path currency.
func (r Repo) Root() srcpath.Root { return r.root }

// Base is the commit the run diffs against, and the ref it was reached
// through.
type Base struct {
	Ref    string
	Commit string
	// Staged makes the diff compare the index against Commit rather than the
	// working tree.
	Staged bool
}

// Label renders the base as the document's `base` field, "<ref>@<7-char sha>".
func (b Base) Label() string { return b.Ref + "@" + b.Commit[:7] }

// NoBaseError reports that the run has no commit to diff against: none of
// ADR 0007's candidates resolved, --since named a ref that does not resolve to
// a commit or resolves to one sharing no history with HEAD, or the branch
// carries no commit for a resolver's HEAD check to find. The message points at
// --since, issue 14's flag, per ADR 0007, except when the branch has no commit
// or --since is itself what failed, whether the ref did not resolve, resolved to
// something other than a commit, or resolved to a commit sharing no history with
// HEAD. Naming the flag again in any of those tells the caller nothing new.
//
// A ref that does not resolve and a ref resolving to something other than a
// commit still share this one message and this one Ref field. Verifying
// unpeeled and typing the resolved id with objectType did not split them, and
// the caller sees "does not name a commit" either way, because both are a ref
// the developer can fix by naming another. What issue 68 took out of this
// error is the third case the old `^{commit}` peel swept in with them, a ref
// the repo carries whose commit object the store has lost, which is git
// failing to answer rather than a wrong ref and now reports an unreadable
// diff. The sentence above names all three arms this error does keep because
// the struct carries a field for the third.
type NoBaseError struct {
	// Ref is the ref --since named. It is empty when the default candidates
	// are what failed, and empty whenever NoCommits is true, which every
	// resolver reports from its HEAD check, ResolveRef and ResolveBase from the
	// one they make before reaching a merge base and ResolveStaged from its
	// only one.
	Ref string
	// NoCommits marks a branch git resolves no commit for, which is what
	// --staged hits before the first commit and what a branch made with
	// `git checkout --orphan` hits in a repo holding a full history. The
	// message names the branch rather than the repo for that second case,
	// where telling the developer the repo has no commits contradicts the log
	// they can print.
	NoCommits bool
	// Unrelated marks the ref resolving while the merge base does not, which
	// is a history sharing no commit with HEAD. It is carried because "does
	// not name a commit" sends the developer hunting a typo that is not
	// there.
	Unrelated bool
}

func (e NoBaseError) Error() string {
	switch {
	case e.NoCommits:
		return "no diff base: this branch has no commit"
	case e.Unrelated:
		return "no diff base: HEAD and " + e.Ref + " share no common ancestor"
	case e.Ref != "":
		return "no diff base: --since " + e.Ref + " does not name a commit"
	default:
		return "no diff base: tried " + strings.Join(BaseCandidates, ", ") + "; name one with --since <ref>"
	}
}

// UnreadableDiffError is this package failing to establish what a change
// touched: git would not answer, or it answered in a shape the parsers here
// refuse. Every cause between asking git for the diff and holding a set of
// touched lines arrives as this one type, so a caller measuring nothing can
// tell "the change touched nothing" from "the diff could not be read", which
// are the same empty map otherwise.
//
// It carries a rendered message rather than a code, because the vocabulary a
// cause is reported in belongs to the caller. metric-gate maps this onto its
// document's error block under report.CodeDiffUnparseable; a caller with no
// document prints the sentence. Holding the code here instead is what tied this
// package to that document, and it is the one thing that had to be cut for the
// package to sit above gate/ and be shared.
type UnreadableDiffError struct{ Message string }

func (e UnreadableDiffError) Error() string { return e.Message }

// errNoSuchRev is git exiting 1 on a `rev-parse --verify`, which is git
// answering that it resolves no such rev. It never reaches a caller of this
// package: each resolver catches it at the check it made and says what the no
// means there, since the same exit code is an ordinary absent rung to
// ResolveBase's walk and a branch with no commit on it to ResolveStaged.
var errNoSuchRev = errors.New("rev does not resolve")

// ResolveBase walks BaseCandidates and returns the merge base of HEAD and
// the first candidate that both exists and shares history with HEAD.
//
// It draws the line ResolveRef and ResolveStaged draw. Exit 1 is git answering
// no, either an absent rung or two histories sharing no commit, and ADR 0007's
// walk answers both by trying the next candidate. Every other exit code is git
// failing to answer, a ref store it cannot read or an object the walk needs and
// the store does not hold, and it comes back typed as an unreadable diff
// carrying git's own words. Walked past, it would render as the tried-refs
// list, which sends the developer after a missing branch while every ref that
// list names is sitting in the repo.
//
// The candidate is verified unpeeled, the check ResolveRef makes about its one
// ref too. A peel reads the object the ref names, and git exits 1 on an object
// its store does not hold, the same code an absent branch gives. Peeled here, a
// rung whose commit is missing reads as a rung the repo does not carry, the walk
// skips it, and the run ends at the tried-refs list naming a branch that is
// sitting in the repo, which is the failure this resolver exists to close.
// Unpeeled, the check answers about the ref alone and merge-base is what
// classifies the object. ResolveRef classifies afterwards instead, with
// `cat-file -t <sha>^{}` over the id its verify resolved, since --since can name
// a tag and "does not name a commit" is the answer that flag owes its caller.
//
// git resolves refs/tags/<name> ahead of refs/heads/<name>, so a lightweight
// tag shadowing a rung and pointing at a tree or a blob resolves at the
// unpeeled check and merge-base is what classifies it, at exit 128, and the run
// reports an unreadable diff naming that object rather than walking on. That is
// the wanted answer: the state is self-inflicted and the message names the real
// object, where a silent skip would hand back a base resolved through a
// different rung than the developer's own repo says it is on.
//
// HEAD is verified before the walk, the check ResolveRef makes and for the
// reason its comment gives. merge-base against an unborn HEAD exits 128, so
// left to fall through, a branch made with `git checkout --orphan` in a repo
// holding a full history would report the diff as unparseable for a run that
// never reached a diff.
func (r Repo) ResolveBase() (Base, error) {
	if _, err := r.verifyRev("HEAD"); err != nil {
		if errors.Is(err, errNoSuchRev) {
			return Base{}, NoBaseError{NoCommits: true}
		}
		return Base{}, err
	}
	for _, ref := range BaseCandidates {
		if _, err := r.verifyRev(ref); err != nil {
			if errors.Is(err, errNoSuchRev) {
				continue
			}
			// No fixture reaches this arm, and swapping it for a continue
			// leaves the whole suite green. The ref store is the only place a
			// ref read fails to answer at all, since every damaged loose ref
			// real git will produce is the exit 1 that means no such ref, and a
			// damaged store fails every read alike, so the HEAD check above
			// returns before the walk starts. It is kept because a continue
			// here is the swallowing this resolver exists to remove, and which
			// arm a fixture can reach is a property of today's git rather than
			// of the rule.
			return Base{}, err
		}
		mergeBase, err := r.git("merge-base", "HEAD", ref)
		if err != nil {
			if noMatch(err) {
				continue
			}
			return Base{}, unreadableDiff(err)
		}
		return Base{Ref: ref, Commit: strings.TrimSpace(mergeBase)}, nil
	}
	return Base{}, NoBaseError{}
}

// ResolveRef is ResolveBase against the one ref --since named, so the run
// measures the branch point rather than the tip.
//
// Every invocation tells exit 1, which is git answering the question with no,
// from every other exit code, which is git failing to answer it. A rev spec git
// refuses to evaluate, `--since main@{9}` against a shorter reflog, comes back
// typed as an unreadable diff, because reported as a name that does not exist
// it would send the developer hunting a typo.
//
// The ref check is unpeeled, matching ResolveBase's candidate check, so both
// resolvers read exit 1 the same way, no such ref rather than a claim about
// what the ref names. objectType is what classifies the object once the ref
// itself resolves. Exit 0 naming anything but a commit is the "does not name a
// commit" answer --since owes a caller who chose the ref by hand, a lightweight
// tag on a tree for one. Every non-zero exit is git failing to answer, which is
// what a branch whose commit object the store lacks gives, and separating those
// two is the whole point of asking: the peel this check replaces exits 1 on
// both and reported the damaged store as a ref naming no commit.
//
// It is the id the verify already resolved that gets peeled, not the rev the
// developer typed. `--since HEAD:src/Order.cs` names a blob and resolves, and
// pasting `^{}` onto that text asks git for a path called `src/Order.cs^{}`,
// which does not exist and comes back as a git failure that never happened over
// a path nobody typed. The id has no such second reading, so the blob answers
// "blob" and the run says the rev does not name a commit.
//
// Only the argv shape reaches a caller from these two checks today, and
// since_ref_unreadable pins it. `--quiet` is what makes an absent ref exit 1 at
// all, and it also swallows what rev-parse itself would say about a rev it
// declined to resolve, `--since main@{9}` against a shorter reflog for one,
// which leaves the failed command as the only thing to report. A fatal raised
// under rev-parse rather than by it still reaches stderr, a ref store git
// cannot parse for one, which base_head_unreadable pins from the default
// scope's own HEAD check, and cause prefers that sentence. Simplifying cause to
// the argv form on the strength of the first case would drop git's own account
// of the second, which is the half that names what is broken.
//
// HEAD is verified before the merge base is asked for, the same check
// ResolveBase and ResolveStaged make, because merge-base against an unborn HEAD
// exits 128 with "Not a valid object name HEAD". Left to fall through, a branch
// made with `git checkout --orphan` reports the diff as unparseable for a run
// that never reached a diff, when what the repo has is no commit on this branch.
func (r Repo) ResolveRef(ref string) (Base, error) {
	oid, err := r.verifyRev(ref)
	if err != nil {
		if errors.Is(err, errNoSuchRev) {
			return Base{}, NoBaseError{Ref: ref}
		}
		return Base{}, err
	}
	objType, err := r.objectType(oid)
	if err != nil {
		return Base{}, err
	}
	if objType != "commit" {
		return Base{}, NoBaseError{Ref: ref}
	}
	if _, err := r.verifyRev("HEAD"); err != nil {
		if errors.Is(err, errNoSuchRev) {
			return Base{}, NoBaseError{NoCommits: true}
		}
		return Base{}, err
	}
	mergeBase, err := r.git("merge-base", "HEAD", ref)
	if err != nil {
		if noMatch(err) {
			return Base{}, NoBaseError{Ref: ref, Unrelated: true}
		}
		return Base{}, unreadableDiff(err)
	}
	return Base{Ref: ref, Commit: strings.TrimSpace(mergeBase)}, nil
}

// ResolveStaged is HEAD, the commit `git diff --cached` compares the index
// against. It draws the same line ResolveRef does: exit 1 is a branch with no
// commit on it, and anything else is git failing to read the one it has.
func (r Repo) ResolveStaged() (Base, error) {
	commit, err := r.verifyRev("HEAD")
	if err != nil {
		if errors.Is(err, errNoSuchRev) {
			return Base{}, NoBaseError{NoCommits: true}
		}
		return Base{}, err
	}
	return Base{Ref: "HEAD", Commit: commit, Staged: true}, nil
}

// verifyRev resolves rev, answering errNoSuchRev when git exits 1 and an
// unreadable diff on every other exit code. It promises no commit id. Every
// check here is unpeeled, so for a ref whose commit object the store lacks git
// exits 0 and prints an id the object store does not hold, which is the fact
// the merge base or the cat-file check that follows fails on. Two of the five
// call sites read the string, ResolveStaged's HEAD check and ResolveRef's ref
// check, which hands the id to objectType, and either can be handed a dangling
// id that way. The other three discard it and want only which of the two arms
// fired.
//
// Every rev-parse check the resolvers make shares this rather than spelling the
// same two arms out each time, which is what makes one reading of noMatch the
// reading all three hold. objectType is the one check that does not, and it
// says at its own definition why its non-zero exits read differently.
// ResolveRef's HEAD check is the reason this one is worth sharing: no
// fixture makes git read a named ref and then fail to answer about HEAD at all,
// since every damaged HEAD real git will produce is either exit 1 or a
// repository it refuses to open at all, so a second copy of the unreadable arm
// there could not be reached by a test.
//
// The no comes back as one sentinel rather than as an error the caller hands
// in, because what exit 1 means is the caller's to say and what exit 1 is is
// this helper's.
func (r Repo) verifyRev(rev string) (string, error) {
	out, err := r.git("rev-parse", "--verify", "--quiet", rev)
	if err != nil {
		if noMatch(err) {
			return "", errNoSuchRev
		}
		return "", unreadableDiff(err)
	}
	return strings.TrimSpace(out), nil
}

// objectType names the type of the object oid peels to, `commit` for a commit
// and for an annotated tag on one, `tree` or `blob` for a tag over either,
// lightweight or annotated.
//
// It is the one check in the package that does not read exit 1 as git answering
// no, because `cat-file -t` has no such answer to give: an object the store does
// not hold is exit 128, so every non-zero exit here is git failing to answer and
// comes back as an unreadable diff carrying its words. Routed through verifyRev's
// reading instead, the missing object would come back as a no and put the
// resolver back on the misreport issue 68 removed.
//
// The argument peels rather than being asked about bare, since `-t <tag oid>`
// answers "tag" for an annotated tag whose target commit the store lacks, which
// is that same conflation one object further out.
func (r Repo) objectType(oid string) (string, error) {
	out, err := r.git("cat-file", "-t", oid+"^{}")
	if err != nil {
		return "", unreadableDiff(err)
	}
	return strings.TrimSpace(out), nil
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
// gives the add side every line, which is what ADR 0007 means by a method
// moved between files being "measured at its new location rather than dropped
// by `--diff-filter=ACM`".
//
// Decomposition alone would leave a pure `git mv` marking every method in the
// moved file changed, because the add side is the whole file. So ADR 0007 also
// says "the gate drops an added file whose content matches a deleted one" in
// the same diff. An added path whose content, whitespace ignored, matches a
// path the same diff deleted is dropped afterwards, provided the diff deletes
// at least as many blobs with that content as it adds, and only a move that
// also edited the file is measured.
//
// ACM also excludes T, so a typechange contributes no touched lines in either
// direction, and the two drops have different reasons. A source file replaced
// by a symlink is the wanted answer, since a link holds no source to measure,
// and widening the letters to ACMT would hand the extractor the link path,
// which it follows to report the target's spans under a name that does not
// hold them. A symlink replaced by a real source file is an accepted gap, and
// measuring it needs a pass classifying the new side's mode before extraction,
// which is issue 84 rather than a wider letter set. See the ADR 0007 amendment
// beginning "`--diff-filter=ACM` excludes `T`,".
//
// Nothing gets out of here untyped. Every cause below this line, a git
// invocation that failed as much as a patch the parser refused, comes back as
// an UnreadableDiffError, which each caller renders in whatever way it reports
// a cause: metric-gate maps it onto the document's error block, where ADR 0008's
// one-document rule binds, because base resolution having succeeded means the
// document exists. Exiting 1 with an empty stdout instead is a shape the caller
// cannot tell from a crash, and typing the boundary rather than the individual
// return sites is what stops the next cause added underneath it reopening that
// hole.
func (r Repo) TouchedLines(base Base) (map[srcpath.Path][]int, error) {
	touched, err := r.touchedLines(base)
	if err != nil {
		return nil, unreadableDiff(err)
	}
	return touched, nil
}

func (r Repo) touchedLines(base Base) (map[srcpath.Path][]int, error) {
	drivers, err := r.filterDrivers()
	if err != nil {
		return nil, err
	}
	args := slices.Concat(diffFlags, []string{"-w", "-U0", "--no-renames", "--diff-filter=ACM"}, cachedFlag(base), []string{base.Commit})
	patch, err := r.gitBlanking(drivers, args...)
	if err != nil {
		return nil, err
	}
	touched, err := parseTouchedLines(patch)
	if err != nil {
		return nil, err
	}
	moved, err := r.pureMoves(base, drivers)
	if err != nil {
		return nil, err
	}
	for _, path := range moved {
		delete(touched, path)
	}
	return touched, nil
}

// cachedFlag is `--cached` when base.Staged, which is what turns a diff's
// working-tree comparison into an index comparison. One diff code path
// serves both ADR 0007's default scope and --staged this way, rather than a
// second near-copy of TouchedLines and pureMoves.
func cachedFlag(base Base) []string {
	if base.Staged {
		return []string{"--cached"}
	}
	return nil
}

// DivergentFromIndex lists the paths whose working-tree copy differs from
// what is staged, keeping the order it was given. A --staged run asks this
// only about the files it is about to score, since a dirty file the run
// never claimed cannot be misattributed to the wrong content.
//
// The comparison runs under the same blanked filter drivers as TouchedLines,
// and under the same flags, diffFlags plus the `-w` TouchedLines adds at its
// own call site, because it has to answer the question the gate actually asks:
// does the text the extractor read off disk match the index content the line
// numbers came from. git's own answer runs the repository's clean driver over
// the working tree, so a driver that normalises the edit away reports the two
// sides as equal and the guard passes on exactly the divergence it exists to
// refuse.
//
// `-w` is what keeps the guard's definition of changed the gate's own. Staging
// a file and then letting an editor reindent the working copy shifts no line
// and changes no complexity, so there is nothing to misattribute, and without
// the flag git names the path and only re-staging clears the refusal. It cannot
// hide a divergence that moves a line, since that is --ignore-blank-lines
// rather than -w, and it cannot hide a content edit.
//
// The listing is `--numstat` rather than `--name-only` for `-w` to reach it at
// all. git applies the whitespace options while it generates a patch, and
// --name-only never generates one, so it names a reindented file whatever it is
// asked to ignore. numstat counts the lines the same patch holds, and a
// whitespace-only difference leaves git printing no record for the path.
//
// Every path travels as a `:(literal)` pathspec, because git reads a bare
// pathspec as a wildmatch pattern. `Order[1].cs` would name a character class
// and match neither the file it spells nor anything else, so the guard would
// pass on the very file it was asked about, and `[id].tsx` is the ordinary
// Next.js route filename rather than an exotic one. Pathspecs compose, so
// `--numstat -z` names the divergent subset directly and a commit touching two
// hundred files costs one git rather than two hundred. core.quotePath is
// pinned false, so the NUL-separated records come back as git spells them, and
// with renames off each record is the two counts and the path under one NUL,
// which is why the path is what follows the second tab rather than a record of
// its own. A record without those two tabs is a shape this parser does not know,
// so it is refused rather than read as a path that happens to hold a tab.
//
// Those pathspecs go out in batches under divergenceBudget rather than all on
// one command line, because a command line has a ceiling nothing here was
// enforcing. A changeset on the order of ten thousand source files runs past
// ARG_MAX, exec fails with E2BIG, and the gate reports diff_unparseable, which
// names the wrong problem, since git parsed nothing because it never ran. The
// union of what the batches name is the same answer the one invocation gave,
// because the caller asks only whether any of the paths is divergent, and an
// ordinary changeset still costs one git.
//
// In a repository whose clean driver transforms content, git-lfs or git-crypt
// rather than the pass-through case above, this refuses more than the developer
// edited. `git add` wrote the index blob through the filter and the blanked
// comparison reads the working tree raw, so once a file's stat cache is
// invalidated by a fresh clone, a checkout or a touch, the two sides differ for
// a file nobody changed and no edit clears the refusal. The comparison is still
// the honest one, since the extractor does read text the index line numbers did
// not come from. The durable answer is having the extractor read the index blob
// under --staged, which issue 14 leaves to a follow-up.
//
// `--no-renames` travels with the diffFlags for the reason TouchedLines and
// pureMoves carry it, that one package cannot hold two definitions of what git
// reports. An index-to-working-tree comparison has no added side for git to pair
// a deletion with, since a path in the tree and not in the index is untracked
// and no diff lists it, so the flag changes no answer today. It is what keeps
// this call answering the same question if it is ever handed two commits, where
// a pair git scored as a rename would come back under one name and leave the
// other file unnamed in the refusal.
//
// `core.fileMode=false` goes on the divergence invocations alone, because the
// executable bit is not content. Staging a file and then running chmod +x on
// it leaves the text the extractor reads exactly as the index line numbers
// describe, so there is nothing to misattribute, and without the pin git names
// the path, the run refuses with staged_file_dirty and no edit clears it. The
// pin cannot hide a content divergence, since git still compares the blobs.
//
// Every failure is typed, so a missing filter binary lands in the document's
// error block rather than exiting 1 with an empty stdout, which is the shape
// TouchedLines already holds itself to on the same path.
func (r Repo) DivergentFromIndex(paths []srcpath.Path) ([]srcpath.Path, error) {
	// A short circuit that saves spawning git when there is nothing to ask it
	// about. It changes no answer, since the trailing filter over paths returns
	// an empty result for an empty slice whatever git prints.
	if len(paths) == 0 {
		return nil, nil
	}
	// The drivers are resolved once rather than per batch. They are a property
	// of the repository and not of the paths, so asking again would spawn a
	// git per batch to learn the same answer.
	drivers, err := r.filterDrivers()
	if err != nil {
		return nil, unreadableDiff(err)
	}
	named := map[srcpath.Path]bool{}
	for _, batch := range divergenceBatches(paths) {
		out, err := r.gitBlanking(drivers, DivergenceArgs(batch)...)
		if err != nil {
			return nil, unreadableDiff(err)
		}
		batchNamed, err := parseNumstatPaths(out)
		if err != nil {
			return nil, err
		}
		maps.Copy(named, batchNamed)
	}
	var divergent []srcpath.Path
	for _, path := range paths {
		if named[path] {
			divergent = append(divergent, path)
		}
	}
	return divergent, nil
}

// StagedChange is one path the index changes against a base, with whether the
// index drops it. The deletion flag is what DivergentFromIndex cannot answer on
// its own: a path the index no longer holds is untracked as far as git is
// concerned, so no diff names it however the working tree looks.
type StagedChange struct {
	Path    srcpath.Path
	Deleted bool
}

// StagedPaths is every path the index changes against base, named the way git
// names it. No -w and no --diff-filter, since the question here is which files
// the commit carries at all rather than which lines it wrote.
func (r Repo) StagedPaths(base Base) ([]StagedChange, error) {
	out, err := r.git("diff", "--cached", "--name-status", "-z", "--no-renames", base.Commit)
	if err != nil {
		return nil, unreadableDiff(err)
	}
	// --no-renames keeps every record two NUL-terminated fields, a status and a
	// path, so the pairs can be read off without a per-status shape.
	fields := strings.Split(out, "\x00")
	var changes []StagedChange
	for i := 0; i+1 < len(fields); i += 2 {
		status, name := fields[i], fields[i+1]
		if status == "" || name == "" {
			continue
		}
		changes = append(changes, StagedChange{Path: srcpath.FromSlash(name), Deleted: status == "D"})
	}
	return changes, nil
}

// WriteTree writes the index out as a tree object and returns its sha, the
// identity a lint waiver is matched and spent against.
func (r Repo) WriteTree() (string, error) {
	out, err := r.git("write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DivergenceArgs is the whole argv DivergentFromIndex hands git for one batch
// of paths. It is exported so the black-box suite can put the same question to
// git that the gate puts, rather than restating the flags in a second place
// where one of the two can drift.
func DivergenceArgs(paths []srcpath.Path) []string {
	pathspecs := make([]string, 0, len(paths))
	for _, path := range paths {
		pathspecs = append(pathspecs, pathspec(path))
	}
	return slices.Concat([]string{"-c", "core.fileMode=false"}, diffFlags,
		[]string{"-w", "--numstat", "-z", "--no-renames", "--"}, pathspecs)
}

// pathspec is how one path travels to git. Both the argv builder and the
// budget read it, so what a batch is charged is the text the batch sends, and
// a magic word added here cannot leave the accounting behind.
func pathspec(path srcpath.Path) string {
	return ":(literal)" + string(path)
}

// divergenceBudget is the pathspec text one DivergentFromIndex invocation is
// allowed to carry. An ordinary changeset of a few hundred paths is a few KiB,
// so it stays one invocation, and the figure is set by the tightest of the
// three ceilings the gate ships against rather than by the most generous.
//
// Windows is the tightest. build.sh cross-builds windows/amd64, exec there goes
// through CreateProcessW, and lpCommandLine is capped at 32767 characters for
// the whole command line rather than at an argv block. 30 KiB of pathspecs
// leaves over two thousand characters for the diff flags, the resolved path of
// git.exe and the quoting Go adds around each argument, which is headroom no
// realistic layout eats. macOS caps argv at a fixed 1 MiB. Linux takes
// max(min(6 MiB, RLIMIT_STACK/4), 131072) with argv and the environment sharing
// it, so a process started under a small `ulimit -s` has 128 KiB for both
// together, and 30 KiB of pathspecs fits beside any environment a shell exports.
//
// It is one fixed number on every platform, not a GOOS-conditional one and not
// one derived from the live rlimit. Two machines batch identically that way, so
// a divergence answer does not depend on whose shell asked, and no caller comes
// near enough to any of the three ceilings for the difference to buy anything.
const divergenceBudget = 30 << 10

// divergenceBatches splits paths into the invocations DivergentFromIndex runs,
// keeping the order it was given. A path costs its pathspec plus the NUL that
// terminates it in the kernel's argv block, and a batch closes when the next
// path would take it past the budget.
func divergenceBatches(paths []srcpath.Path) [][]srcpath.Path {
	var batches [][]srcpath.Path
	var batch []srcpath.Path
	cost := 0
	for _, path := range paths {
		next := len(pathspec(path)) + 1
		// A path whose own pathspec passes the budget still goes, alone.
		// Dropping it would answer a question about a file nobody asked about,
		// so git or exec is left to be the one that refuses it.
		if len(batch) > 0 && cost+next > divergenceBudget {
			batches = append(batches, batch)
			batch, cost = nil, 0
		}
		batch = append(batch, path)
		cost += next
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

// DivergenceBatchCount is how many invocations DivergentFromIndex makes over
// paths. It is exported for the reason DivergenceArgs is, so the black-box
// suite can ask the production code whether its fixture actually splits. The
// budget itself stays unexported, because a case holding the byte figure is a
// case that keeps passing against a stale copy of it after this one moves.
func DivergenceBatchCount(paths []srcpath.Path) int {
	return len(divergenceBatches(paths))
}

// parseNumstatPaths reads the set of paths out of `--numstat -z` output. It is
// its own function because a real git cannot be made to print a record this
// parser refuses, so the refusal is only reachable, and only testable, from
// here.
//
// A binary difference prints its two counts as "-", which is a shape this reads
// like any other, since the counts are not what the caller asked about. A
// record without both tabs is a shape the parser does not know, so it is
// refused rather than read as a path that happens to hold a tab.
func parseNumstatPaths(out string) (map[srcpath.Path]bool, error) {
	named := map[srcpath.Path]bool{}
	for _, record := range nulRecords(out) {
		fields := strings.SplitN(record, "\t", 3)
		if len(fields) != 3 {
			return nil, UnreadableDiffError{
				Message: "could not read the diff: git printed the numstat record " + strconv.Quote(record),
			}
		}
		named[srcpath.FromSlash(fields[2])] = true
	}
	return named, nil
}

// filterDrivers is the config key of every content filter the repository
// configures. Blanking each of them is what a diff has to be run under for a
// clean driver not to decide what the gate can see.
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
// A subsection name is arbitrary text the repository chooses, so the key never
// gets parsed as syntax on either leg of the trip. It comes back NUL-separated,
// because a listing split on whitespace breaks `filter.my driver.clean` in two,
// and it goes back out through blankingEnv rather than a `-c` flag, because a
// `-c` argument is split on its first `=` and `filter.ev=il.clean=` therefore
// sets `filter.ev` and leaves the real driver installed.
func (r Repo) filterDrivers() ([]string, error) {
	out, err := r.git("config", "--name-only", "--get-regexp", "-z", filterDriverKeys)
	if noMatch(err) {
		// `git config --get-regexp` exits 1 on no match, which is the ordinary
		// case of a repository configuring no filter at all.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nulRecords(out), nil
}

// nulRecords splits the output of a `-z` git command into its records. Every
// reader in this package goes through it, so a reader added later cannot
// forget that a NUL-terminated stream ends in a trailing empty field and pick
// up a phantom record. No git record any of these parsers accepts is empty, so
// an empty one is dropped wherever it appears rather than only at the end.
func nulRecords(out string) []string {
	var records []string
	for _, record := range strings.Split(out, "\x00") {
		if record != "" {
			records = append(records, record)
		}
	}
	return records
}

// blankingEnv sets each key to the empty string through the GIT_CONFIG_COUNT
// family, which carries the key and its value in separate variables and so has
// no delimiter for the key's own text to collide with. The family lands in
// command scope, the same scope as configOverrides, so the two combine rather
// than one replacing the other, and it outranks the repository's own config the
// way a `-c` flag does.
//
// This is the transport for every override whose text the repository controls.
// configOverrides keeps the `-c` route because each of its keys is a fixed
// literal this package wrote.
func blankingEnv(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	env := []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(len(keys))}
	for i, key := range keys {
		env = append(env,
			fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, key),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=", i))
	}
	return env
}

// filterDriverKeys matches every spelling of a content filter driver.
const filterDriverKeys = `^filter\..*\.(clean|process|required)$`

// noMatch reports whether err is git exiting 1, which is git answering the
// question with no rather than failing to answer it. What the no means belongs
// to the caller, so each one says it where it asks.
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
// wall of failures ADR 0007 gives `-w` to prevent.
//
// Every unreadable side resolves towards measuring, which is the conservative
// direction. An added path the gate cannot read stays measured, and it counts
// against every other add in that run: the unknown content could be the one a
// deleted blob explains, so counting without it would leave a readable sibling
// looking accounted for and drop a file nobody moved. It is counted rather
// than used to switch move detection off, so a run with more deletes than adds
// still drops the moves the deletes do explain. An add paired with a deleted
// object the gate cannot read stays measured too: a `cat-file` failure, which
// is what a blobless partial clone gives offline, drops that object from the
// comparison rather than escaping and leaving the run with no document at all.
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
func (r Repo) pureMoves(base Base, drivers []string) ([]srcpath.Path, error) {
	args := slices.Concat(rawFlags, []string{"-z", "--abbrev=40", "--no-renames", "--diff-filter=AD"}, cachedFlag(base), []string{base.Commit})
	raw, err := r.gitBlanking(drivers, args...)
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
	unreadable := 0
	for _, add := range added {
		body, err := r.addedContent(base, add)
		if err != nil {
			// ADR 0007's 2026-09-16 amendment. An add the gate cannot
			// digest could be carrying any deleted
			// blob's content, so it is counted as a claimant of every digest
			// rather than of none. It can never be proven a move itself, and
			// it leaves every readable add one claimant nearer to unproven,
			// which is the conservative direction: the run measures it.
			unreadable++
			continue
		}
		digest := squashedDigest(body)
		digests[add.Path] = digest
		claimants[digest]++
	}
	var moves []srcpath.Path
	for _, add := range added {
		digest, read := digests[add.Path]
		if !read {
			continue
		}
		if carried[digest] == 0 || claimants[digest]+unreadable > carried[digest] {
			continue
		}
		moves = append(moves, add.Path)
	}
	return moves, nil
}

// addedContent reads the new side of an added path, from the same snapshot the
// deleted side comes out of.
//
// Under --staged that is the index blob the raw record names rather than the
// file on disk. Read off disk, a path moved, edited, staged, and then restored
// on disk to its pre-move text digests equal to the deleted blob, so the gate
// calls it a pure move and drops it from the diff. It is then never handed to
// an extractor, never claimed, and so never among the paths the staged_file_dirty
// guard is asked about, and the run passes on exactly the divergence that guard
// exists to refuse.
func (r Repo) addedContent(base Base, add addedFile) ([]byte, error) {
	if base.Staged {
		body, err := r.git("cat-file", "blob", add.Blob)
		return []byte(body), err
	}
	return os.ReadFile(r.root.Abs(add.Path))
}

// addedFile is one path a diff added, with the object id of its new-side
// content. The id is all zeros for a working-tree diff, where the new side is
// the file on disk and no object holds it.
type addedFile struct {
	Path srcpath.Path
	Blob string
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
// paths the diff added with their new-side blob ids and the old-side blob ids
// it deleted. A record is the metadata field ":<mode> <mode> <src> <dst>
// <status>" followed by the path, both NUL-terminated.
func parseRawAddsAndDeletes(raw string) (added []addedFile, deleted []string, err error) {
	records := nulRecords(raw)
	if len(records)%2 != 0 {
		return nil, nil, fmt.Errorf("git diff --raw emitted %d fields, want pairs", len(records))
	}
	for i := 0; i < len(records); i += 2 {
		fields := strings.Fields(records[i])
		if len(fields) != 5 {
			return nil, nil, fmt.Errorf("git diff --raw record is malformed: %q", records[i])
		}
		oldMode, newMode, src, dst, status := strings.TrimPrefix(fields[0], ":"), fields[1], fields[2], fields[3], fields[4]
		switch status {
		case "A":
			if newMode == gitlinkMode {
				continue
			}
			added = append(added, addedFile{Path: srcpath.FromSlash(records[i+1]), Blob: dst})
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
// having a set of touched lines. One type covers the whole stretch because every
// cause on it says the same thing to a caller, that the run could not establish
// what the change touched and therefore measured nothing, and because a type per
// cause would be a list to extend every time a line is added under the boundary.
func unreadableDiff(err error) error {
	var unreadable UnreadableDiffError
	if errors.As(err, &unreadable) {
		return unreadable
	}
	return UnreadableDiffError{Message: "could not read the diff: " + cause(err)}
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
// and the GIT_CONFIG_COUNT family inject config into the same scope the `-c`
// overrides occupy, so an inherited one sets keys the overrides never name, and
// GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE and their relatives outrank cmd.Dir and
// answer every question about a different repository. That last family is not
// exotic: git exports it into every hook it runs, and a pre-commit hook is what
// this gate is built to be.
//
// Nothing else in the namespace is needed to run git. The repo comes from
// cmd.Dir and the settings the parsers depend on come from configOverrides,
// and the one caller that needs the index git handed a hook asks for it by
// name through OpenHook.
const gitNamespace = "GIT_"

// pinnedConfigFiles is what run puts back after the scrub, alongside whatever
// blankingEnv contributes for the invocation.
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
// which the pins deliberately leave readable. filterDrivers and blankingEnv
// are what cover that scope.
var pinnedConfigFiles = []string{
	"GIT_CONFIG_GLOBAL=" + os.DevNull,
	"GIT_CONFIG_SYSTEM=" + os.DevNull,
}

// git runs one git command in the repo root.
func (r Repo) git(args ...string) (string, error) {
	return run(r.root.Dir(), r.indexEnv(), args...)
}

// indexEnv puts the hook index back after the scrub, and contributes nothing
// for a Repo that was not opened for a hook.
func (r Repo) indexEnv() []string {
	if r.index == "" {
		return nil
	}
	return []string{"GIT_INDEX_FILE=" + r.index}
}

// gitBlanking is git with every named config key forced empty for the length of
// the one invocation, which is how the repository's filter drivers are taken out
// of the commands that read content.
func (r Repo) gitBlanking(keys []string, args ...string) (string, error) {
	return run(r.root.Dir(), append(r.indexEnv(), blankingEnv(keys)...), args...)
}

// run executes git in dir, or the process working directory when dir is
// empty, and returns its stdout.
func run(dir string, extraEnv []string, args ...string) (string, error) {
	//nolint:gosec // the executable is the literal "git", the one caller-supplied element is a rev handed to read-only subcommands, and every element is argv, so no shell parses them
	cmd := exec.Command("git", append(append([]string{}, configOverrides...), args...)...)
	cmd.Dir = dir
	cmd.Env = append(sanitizedEnv(), extraEnv...)
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
	// Nothing was captured when the process never ran, exec.ErrNotFound for a
	// missing git or a blanked driver binary git tried to launch, and a
	// trailing ": " would leave the reader looking for a complaint that was
	// never made.
	if e.stderr == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.args, " "), e.err)
	}
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.args, " "), e.err, e.stderr)
}

func (e *gitError) Unwrap() error { return e.err }

// cause is what git said, or the whole error when it was not git that spoke.
//
// A filesystem failure carries the operating system's own words without the
// fs.PathError around them, which quotes an absolute path the document is not
// allowed to print (ADR 0004) and which reads differently on every machine.
// The sentence naming what the gate was doing supplies the rest.
func cause(err error) string {
	var gitErr *gitError
	if errors.As(err, &gitErr) && gitErr.stderr != "" {
		return gitErr.stderr
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
	}
	return err.Error()
}

// sanitizedEnv is the process environment with git's own namespace removed and
// the two config-file variables pinned. It is built
// rather than inherited, so neither what the parser reads, nor which repository
// it reads it from, nor whose config it reads it under depends on how the
// caller's shell was set up.
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
