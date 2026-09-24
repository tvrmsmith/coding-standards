# A pure move is dropped by counting whitespace-blind digests, with rename detection off

**Status:** accepted 2026-09-23. Splits [ADR 0007](https://github.com/tvrmsmith/coding-standards/blob/7ff6abedb4b8992e075c5db9a352a2940419070f/docs/adr/0007-changed-method-is-a-span-holding-a-touched-line.md) with
[ADR 0014](0014-a-changed-method-is-a-span-holding-a-touched-line.md),
[ADR 0016](0016-the-gate-pins-every-git-setting-its-parsers-depend-on.md) and
[ADR 0017](0017-the-diff-scopes-agree-except-at-two-points.md). The decision is unchanged.

## Current rule

The diff runs with `--no-renames`, so a rename arrives as a delete and an add, and a
renamed-and-edited file is measured at its new location. The gate then drops an added file whose
content matches a deleted one in the same diff, so a pure `git mv` measures nothing.

Content is compared the way `-w` compares it. Per content digest, the gate counts the adds against
the deletes carrying it.

- As many adds as deletes: the same files moved, so all the adds drop.
- More adds than deletes: content exists that was not on the deleted side, so none drops.
- Fewer adds than deletes: a partial move, so the adds drop.

An added path the gate cannot read counts as a claimant of every digest.

## Decision

**Rename detection is off.** With it on, `--diff-filter=ACM` drops a file git scores as a rename,
so a renamed-and-edited file would go unmeasured. On its own, `--no-renames` makes a pure `git mv`
mark every method in the moved file changed, which is what the drop answers.

**The digest.** The gate hashes each side with every whitespace character dropped from each line and
the line structure kept. A byte comparison would put a move-plus-reindent back in the changed set,
which is the wall of failures `-w` exists to prevent.

**Counting rather than pairing** means no verdict turns on `git diff --raw` ordering.

**Which copy is read.** Under `--staged` the added side is read from the index, using the
destination object id the `--raw` record carries, so both halves of the comparison come from one
snapshot. Every other scope reads the working tree, which is the copy it measures. A deleted
submodule is skipped rather than read, since a gitlink's object id names a commit in another
repository.

**An unreadable add.** The content nobody could read could be the content any deleted path carried,
so it claims every digest. One delete against one readable add plus one unreadable add is two
claimants against one delete, so nothing drops and the readable sibling stays measured. Two deletes
against one readable add and one unreadable add is two claimants against two deletes, so the move
drops. An unreadable add is counted by the rule above rather than standing outside it, which
[PR 104](https://github.com/tvrmsmith/coding-standards/pull/104) argues.
