# A changed method is a working-tree span holding at least one touched line

**Status:** accepted 2026-09-23. Splits [ADR 0007](https://github.com/tvrmsmith/coding-standards/blob/7ff6abedb4b8992e075c5db9a352a2940419070f/docs/adr/0007-changed-method-is-a-span-holding-a-touched-line.md) with
[ADR 0015](0015-a-pure-move-is-dropped-by-counting-digests.md),
[ADR 0016](0016-the-gate-pins-every-git-setting-its-parsers-depend-on.md) and
[ADR 0017](0017-the-diff-scopes-agree-except-at-two-points.md). The decision is unchanged. 0007
consolidated [ADR 0003](https://github.com/tvrmsmith/coding-standards/blob/3fd48d4ad6c2a98d2332fb872ab319e80658bb91/docs/adr/0003-changed-method-is-a-span-holding-a-touched-line.md), whose last commit holds the options considered and the reasoning that
rejected each.

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
location, unless its file moved with no content change, which ADR 0015 drops. A deleted method has
no working-tree span, so there is nothing to measure. A run whose changed-method set is empty exits
0 before resolving any input.

## Decision

**Typechanges.** `--diff-filter=ACM` excludes `T`, so a typechange contributes no touched lines in
either direction. A source file replaced by a symlink is the wanted answer, since a link holds no
source to measure. A symlink replaced by a real source file is an accepted gap. Its methods stay
unmeasured until an edit touches each of them, because the file never arrives as status `A`.

Widening to `ACMT` is not the remedy. git renders a typechange as a delete plus an add, so in the
first direction the new side is the link's own blob, one line holding the path it points at, under a
name still claiming `.cs`. That line falls inside no span, so the guaranteed effect is
`touched_lines_outside_spans` going 0 to 1 at exit 0. Where the extractor follows the link and the
target's spans cover line 1, a method is measured under a path that does not hold it and the run
fails on an unknown changed method. Measuring the second direction needs a pass classifying the new
side's mode before extraction, which is [issue 84](https://github.com/tvrmsmith/coding-standards/issues/84). [PR 85](https://github.com/tvrmsmith/coding-standards/pull/85) pins both
directions.

**What the extractor owes.** The extractor self-describes the file extensions it handles, so adding
a language means shipping an extractor rather than editing the gate. It reports per-file parse
status beside its spans. A changed file it could not parse fails the run, and one it parsed with
zero spans is silent, which is what keeps `IFoo.cs` and `Constants.cs` blameless. Only the
extractor can tell those apart.

Spans are extracted for changed files only, so extraction cost scales with the change rather than
with the repo.

**Lines outside every span.** A touched line inside no span, a `using` directive or a field or a
class attribute, contributes no changed method and is explicitly not the `unknown` state. The gate
counts these lines and prints the count as a diagnostic, never gating on it, because most of them
are legitimately not methods and any threshold would be calibrated against nothing.

**Staleness** compares the report timestamp against the newest mtime among files that contributed a
changed method, not every ACM file in the diff, so a whitespace-only reformat does not invalidate an
otherwise good report.
