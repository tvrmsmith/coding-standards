# The four diff scopes agree except at two points

**Status:** accepted 2026-09-23. Splits [ADR 0007](https://github.com/tvrmsmith/coding-standards/blob/7ff6abedb4b8992e075c5db9a352a2940419070f/docs/adr/0007-changed-method-is-a-span-holding-a-touched-line.md) with
[ADR 0014](0014-a-changed-method-is-a-span-holding-a-touched-line.md),
[ADR 0015](0015-a-pure-move-is-dropped-by-counting-digests.md) and
[ADR 0016](0016-the-gate-pins-every-git-setting-its-parsers-depend-on.md). The decision is
unchanged.

## Current rule

The default base resolves through `origin/HEAD`, `origin/main`, `origin/master`, local `main`, then
local `master`. Failing all of those the run exits 1 naming every ref it tried and pointing at
`--since`. `--since <ref>` names the base directly.

The scopes differ at two points. `--files` carries no line information, so every method in a listed
file is changed and `base` is null. `--staged` reports index line numbers while the extractor parses
the disk copy, so the gate exits 1 on a file staged in one state and dirty on disk in another.

## Decision

**Verifying the base.** `HEAD` is verified before the walk, so a branch with no commit reports that.
Any git exit other than 1, at any check, reports an unreadable diff carrying git's words. Candidates
verify **unpeeled**, because `<ref>^{commit}` exits 1 on a missing object.

`--since <ref>` verifies its ref unpeeled too, then classifies with `cat-file -t` on the id that
verify resolved, `^{}` appended to that id and never to the typed ref, which for `<rev>:<path>`
builds a pathspec rather than a peel. A missing commit object reports an unreadable diff rather than
"does not name a commit", for a full sha or a plain ref. An abbreviation or an explicit peel needs
the store to resolve and still exits 1. [PR 69](https://github.com/tvrmsmith/coding-standards/pull/69) and [PR 88](https://github.com/tvrmsmith/coding-standards/pull/88) carry the
cases.

**The `--staged` refusal** is narrowed three ways. It asks only about files an extractor claimed. It
runs with `-w` and `core.fileMode=false`, so a reindent or a chmod is scored rather than refused.
When extraction itself fails, it reports the dirty path in place of the extractor's cause only if
the check names one.

## Considered options

**Falling back to `HEAD~1`** when no candidate resolves. Rejected because a silently different base
is the failure a caller cannot detect.
