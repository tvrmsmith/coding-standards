# A changed method is a working-tree span holding at least one touched line

**Status:** accepted 2026-09-09. Consolidates [ADR 0003](0003-changed-method-is-a-span-holding-a-touched-line.md)
and its eight amendments. The decision is unchanged; 0003 holds the reasoning that got here.

## Current rule

A **touched line** is any new-side line reported by
`git diff -w -U0 --diff-filter=ACM --no-renames --text`. A zero-length hunk, which is what a pure
deletion produces, touches the line at its insertion point. A **changed method** is a method span in
the working tree containing at least one touched line, and where spans nest only the smallest
containing span is changed. One touched line changes the whole method, because CRAP is a per-method
number and there is no CRAP of three lines.

Spans come only from the extractor parsing the working tree. A method element in a coverage report
that the extractor did not emit is not a method as far as the gate is concerned.

Touched lines come from tracked paths only. A file never added to the index contributes nothing, so
a new source file is measured once it is staged and not before.

The rule has no special cases. A method moved within a file or between files is changed at its new
location. A deleted method has no working-tree span, so there is nothing to measure. A run whose
changed-method set is empty exits 0 before resolving any input.

## Rename detection is off, and pure moves are dropped by counting

`--no-renames` decomposes a rename into a delete and an add, so a renamed-and-edited file is
measured at its new location rather than dropped by `--diff-filter=ACM`. On its own that makes a
pure `git mv` mark every method in the moved file changed, so the gate drops an added file whose
content matches a deleted one in the same diff.

Content is compared the way `-w` compares it, by hashing each side with every whitespace character
dropped from each line and the line structure kept. A byte comparison would put a move-plus-reindent
back in the changed set, which is the wall of failures `-w` exists to prevent.

Which adds the drop applies to is decided by counting, per digest.

- As many adds as deletes carry the digest: the same files moved, so all the adds drop.
- More adds than deletes: content exists that was not on the deleted side, so none drops.
- Fewer adds than deletes: a partial move, so the adds drop.

Counting rather than pairing means no verdict turns on `git diff --raw` ordering.

Two reads are scoped. Under `--staged` the added side is read from the index, using the destination
object id the `--raw` record carries, so both halves of the comparison come from one snapshot; every
other scope reads the working tree, which is the copy it measures. An added path the gate cannot
read suppresses the drop for the whole run, because an uncounted digest would let a readable sibling
look accounted for. A deleted submodule is skipped rather than read, since a gitlink's object id
names a commit in another repository.

## The gate pins every git setting its parsers depend on

Each of these turns a real change into `changed_methods: 0`, exit 0, which is the worst outcome a
blocking gate has. Four mechanisms reach the diff and each takes a different answer.

- Ambient config such as `color.ui=always` is overridden with `-c` flags.
- Environment config such as `GIT_EXTERNAL_DIFF`, which outranks `-c`, is dropped from the
  command's environment. That hands the run to an ambient `~/.gitconfig`, so `GIT_CONFIG_GLOBAL`
  and `GIT_CONFIG_SYSTEM` are pinned at the null device, which in turn puts `safe.directory` out of
  reach, so `-c safe.directory=*` goes on the command line.
- Content filters are blanked. A clean driver named by the repo's own `.git/config` and selected by
  `.gitattributes` can print its input back unchanged and empty the patch, and no flag turns
  filtering off. The gate enumerates the configured drivers and sets each one empty in command
  scope, `required` included. Repo-local `core.fsmonitor` is blanked for the same reason.
- `--text` forces hunk headers out of a file git would summarise as "Binary files differ", covering
  UTF-16 source and any path marked `-diff` or `binary`.

The blanks travel through the `GIT_CONFIG_COUNT` family rather than `-c`, because a driver's
subsection name is repo-controlled and a `-c` argument splits on its first `=`. That is one rule:
**repo-controlled text is never interpolated where it can be read as syntax.** An earlier spelling
lost to a subsection name holding a space, and a later one to a name holding `=`, each a silent pass.

## The four scopes agree except at two points

The default base resolves through `origin/HEAD`, `origin/main`, `origin/master`, local `main`, then
local `master`. Failing all of those the run exits 1 naming every ref it tried and pointing at
`--since`. Falling back to `HEAD~1` was rejected because a silently different base is the failure a
caller cannot detect.

**Amended 2026-09-09.** `HEAD` is verified before the walk, so a branch with no commit reports that.
Any git exit other than 1, at any check, reports an unreadable diff carrying git's words. Candidates
verify **unpeeled**, because `<ref>^{commit}` exits 1 on a missing object.

**Amended 2026-09-11.** `--since <ref>` verifies its ref unpeeled too, then classifies with
`cat-file -t` on the id that verify resolved, `^{}` appended to that id and never to the typed ref,
which for `<rev>:<path>` builds a pathspec rather than a peel. A missing commit object now reports
an unreadable diff rather than "does not name a commit".

`--files` carries no line information, so every method in a listed file is changed and `base` is
null. `--staged` reports index line numbers while the extractor parses the disk copy, so the gate
exits 1 on a file staged in one state and dirty on disk in another. That refusal is narrowed three
ways: it asks only about files an extractor claimed, it runs with `-w` and `core.fileMode=false` so
a reindent or a chmod is scored rather than refused, and when extraction itself fails it reports the
dirty path in place of the extractor's cause only if the check names one.

## What the extractor owes, and what the gate does with the rest

The extractor self-describes the file extensions it handles, so adding a language means shipping an
extractor rather than editing the gate. It reports per-file parse status beside its spans: a changed
file it could not parse fails the run, and one it parsed with zero spans is silent, which is what
keeps `IFoo.cs` and `Constants.cs` blameless. Only the extractor can tell those apart.

Spans are extracted for changed files only, so extraction cost scales with the change rather than
with the repo.

A touched line inside no span, a `using` directive or a field or a class attribute, contributes no
changed method and is explicitly not the `unknown` state. The gate counts these lines and prints the
count as a diagnostic, never gating on it, because most of them are legitimately not methods and any
threshold would be calibrated against nothing.

Staleness compares the report timestamp against the newest mtime among files that contributed a
changed method, not every ACM file in the diff, so a whitespace-only reformat does not invalidate an
otherwise good report.

## Considered options

**Take method line ranges from the coverage report.** No second parse, and the ranges arrive aligned
with the coverage they scope. Rejected because it makes identity depend on a language-specific
artifact the gate does not own, which is the seam [ADR 0001](0001-crap-gate-topology.md) draws, and
because a method absent from the report would be invisible rather than `unknown`.

**Measure the callers of changed methods too.** Rejected because resolving callers needs a call
graph, a call graph needs a semantic model, and the complexity walker needs none. The depth limit
would also be arbitrary.

**Count every diff line, with no whitespace filter.** Rejected because one `dotnet format` run over
a legacy file marks every method in it changed, so a formatting commit becomes a wall of failures on
code nobody wrote that day. `-w` is git's own definition of the exemption rather than one invented
here.

**Extend a comment-only filter alongside `-w`.** Rejected because deciding a line carries only a
comment needs a lexer, and a lexer is language-specific. The gate is not.

**Mark every containing span, not just the smallest.** Rejected because the coverage join already
uses smallest-containing-span, and one containment rule serving both directions is worth more than
the extra sensitivity. The container's own complexity did not change.

**Union in `git ls-files --others --exclude-standard`** so untracked files count. Rejected because
it makes a scratch file gate the run, and it adds a second input source to a definition whose value
is being one sentence long.

## Consequences

The gate runs `git` itself rather than taking hunks from a wrapper, so the rule lives in one binary
and no caller can get `-w` or `--diff-filter` subtly wrong. That is the second thing the gate shells
out to, after nothing, and it is acceptable because git is present on every machine that has a repo
to gate.

`lint-changed-dotnet.sh` only warns where this gate exits 1 on a staged-and-dirty file. It is right
to, because it reports and never blocks. This one blocks, and silent misattribution is the worst
thing a blocking gate can do.
