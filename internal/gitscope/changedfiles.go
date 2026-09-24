package gitscope

import (
	"errors"
	"slices"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// StagedFiles is the lint harness's --staged changed set: the files a commit of
// the index adds, copies or modifies, which are the files a linter has lines in.
//
// With no merge in progress it is `git diff --cached` against HEAD. The bare
// form rather than naming HEAD, because on an unborn branch the bare form diffs
// against the empty tree and a named HEAD fails, so the first commit of a repo
// still lints.
//
// During an uncommitted merge it is only the paths that differ from both
// parents: a conflict resolution, or an edit made while merging. Against HEAD
// alone the index holds everything the incoming side brought, thousands of files
// and dozens of .NET builds on a long-lived branch. Against MERGE_HEAD alone it
// holds everything the branch already committed, whose lines all match HEAD, so
// the filter would drop every finding in them after paying for the builds. Only
// a path in both can carry a finding the filter keeps.
func (r Repo) StagedFiles() ([]srcpath.Path, error) {
	if _, err := r.verifyRev("MERGE_HEAD"); err != nil {
		if errors.Is(err, errNoSuchRev) {
			return r.changedNames("--cached", "--diff-filter=ACM")
		}
		return nil, err
	}
	incoming, err := r.changedNames("--cached", "--diff-filter=ACM", "MERGE_HEAD")
	if err != nil {
		return nil, err
	}
	// No --diff-filter against HEAD: whatever the letter, a path listed here
	// differs from HEAD, and that is the whole question this side answers.
	own, err := r.changedNames("--cached", "HEAD")
	if err != nil {
		return nil, err
	}
	differsFromHead := make(map[srcpath.Path]bool, len(own))
	for _, path := range own {
		differsFromHead[path] = true
	}
	return slices.DeleteFunc(incoming, func(path srcpath.Path) bool {
		return !differsFromHead[path]
	}), nil
}

// FilesSince is the lint harness's --since changed set: the working-tree files
// that add, copy or modify something against the merge base of HEAD and ref.
// The merge base rather than ref's tip, because it is the base the filter
// scopes findings against, and a file only ref's side changed has no line in
// that scope for a finding to survive on.
func (r Repo) FilesSince(ref string) ([]srcpath.Path, error) {
	base, err := r.ResolveRef(ref)
	if err != nil {
		return nil, err
	}
	return r.changedNames("--diff-filter=ACM", base.Commit)
}

// changedNames is `git diff --name-only` over args, one path per record.
//
// `--no-renames` for the reason TouchedLines carries it. A file git scores as
// a rename carries status R, which `--diff-filter=ACM` drops, so an edit made on
// the way to a new path would reach no linter while the filter still counted
// its lines as changed. Split into a delete and an add, the add is linted.
func (r Repo) changedNames(args ...string) ([]srcpath.Path, error) {
	out, err := r.git(slices.Concat([]string{"diff", "--name-only", "-z", "--no-renames"}, args)...)
	if err != nil {
		return nil, unreadableDiff(err)
	}
	var paths []srcpath.Path
	for _, record := range nulRecords(out) {
		paths = append(paths, srcpath.FromSlash(record))
	}
	return paths, nil
}
