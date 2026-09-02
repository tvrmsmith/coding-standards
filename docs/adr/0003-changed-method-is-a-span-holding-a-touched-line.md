# A changed method is a working-tree span holding at least one touched line

Scope is changed methods, and this is the whole definition. A **touched line** is any new-side line reported by `git diff -w -U0 --diff-filter=ACM`; a zero-length hunk, which is what a pure deletion produces, touches the line at its insertion point. A **changed method** is a method span in the working tree containing at least one touched line, and where spans nest, only the smallest containing span is changed. One touched line changes the whole method, because CRAP is a per-method number and there is no CRAP of three lines.

Spans come only from the extractor parsing the working tree. The coverage report contributes lines and hits and never spans, so a method element in the report that the extractor did not emit is not a method as far as the gate is concerned.

The rule has no special cases. A method moved within a file or between files appears as added lines at its new location, so it is changed there. A file renamed with no content change reports no touched lines, so nothing is measured. A deleted method has no working-tree span, so there is nothing to measure. A new file is entirely added lines, so every method in it is measured.

**Amended 2026-09-02.** Those first two sentences pull against each other under git's default rename detection, and the gate holds both by adding `--no-renames` plus a content check. With detection on, a file git scores as a rename gets status `R` and `--diff-filter=ACM` drops it, so a renamed-and-edited file contributes no touched lines at all and the gate passes a change that added a decision point. `--no-renames` restores the moved-method sentence by decomposing the rename into a `D` for the old path and an `A` for the new one, whose lines are all added. On its own, though, it breaks the renamed-with-no-content-change sentence, because a pure `git mv` then marks every method in the moved file as changed. So the gate additionally **drops an added file whose content is byte-identical to a deleted one in the same diff**. Both sentences then hold as written: a renamed-and-edited file is measured at its new location, and a pure move measures nothing.

**Amended 2026-09-02.** Touched lines come from **tracked paths only**. `git diff <base>` does not see a file that has never been added to the index, so a brand-new source file the developer has not staged contributes no touched lines and no changed methods, and the run exits 0 on it. That leaves "a new file is entirely added lines, so every method in it is measured" true only once the file is staged. The alternative, unioning in `git ls-files --others --exclude-standard`, was rejected: it makes an untracked scratch file gate the run, and it adds a second input source to a definition whose whole value is being one sentence long. The limitation is recorded here and in the `gitscope` package doc rather than worked around.

## Considered options

**Take method line ranges from the coverage report.** No second parse, and the ranges arrive already aligned with the coverage they scope. Rejected because it makes identity depend on a language-specific artifact the gate does not own, which is the seam [ADR 0001](0001-crap-gate-topology.md) exists to draw, and because a method absent from the report would then be invisible rather than `unknown`.

**Measure the callers of changed methods as well as the changed methods.** A method whose body is untouched can lose coverage because its callee changed, and under this rule that degradation is never scored until someone touches the method. Rejected because resolving callers needs a call graph, a call graph needs a semantic model, and issue 3 established that the complexity walker needs no semantic model. The depth limit would also be arbitrary. This is the same bargain the harness already took when it scoped linting to changed files.

**Count every diff line, with no whitespace filter.** One fewer flag and one fewer rule. Rejected because a single `dotnet format` run over a legacy file marks every method in it changed, and the gate blocks, so a formatting commit becomes a wall of failures on code nobody wrote that day. That breaks the map's rule that nothing which passed yesterday fails today as thoroughly as moving the threshold would. `-w` is git's own definition of the exemption rather than one invented here.

**Extend a comment-only filter alongside `-w`.** Rejected because deciding that a line carries only a comment needs a lexer, and a lexer is language-specific. The gate is not.

**Mark every containing span, not just the smallest.** More sensitive to a change inside a local function. Rejected because the coverage join already uses smallest-containing-span, and one containment rule serving both directions is worth more than the extra sensitivity. The container's own complexity did not change.

## Consequences

The gate runs `git` itself rather than taking hunks from a wrapper, so the rule lives in one binary and no caller can get `-w` or `--diff-filter` subtly wrong. That is the second thing the gate shells out to, after nothing, and it is acceptable because git is present on every machine that has a repo to gate.

The default base resolves through `origin/HEAD`, then `origin/main`, then `origin/master`, then local `main`, then local `master`, and failing all of those the run exits 1 naming every ref it tried and pointing at `--since`. Local-only repos are ordinary for a solo setup, and a shallow clone that cannot reach the merge-base still fails loudly. Falling back to `HEAD~1` was rejected because a silently different base is the failure the caller cannot detect.

The four diff modes agree except for two points. `--files` carries no line information, so **every method in a listed file is changed**, which is a stated rule rather than an accident. `--staged` reports index line numbers while the extractor parses the disk copy, so a file staged in one state and dirty in another would map hunk ranges onto the wrong text; the gate **exits 1** naming those files. `lint-changed-dotnet.sh` only warns at the same fork, and it is right to, because it reports and never blocks. This one blocks, and silent misattribution is the worst thing a blocking gate can do.

The extractor contract from ADR 0001 grows two obligations, both because the gate must distinguish "not my language" and "nothing here" from "I failed".

- It **self-describes the file extensions it handles**, so the gate hands it only changed files it can use and adding a language means shipping an extractor rather than editing the gate.
- It **reports per-file parse status** beside its spans. A changed file the extractor could not parse fails the run with exit 1. A changed file it parsed with zero spans is silent, which is what keeps `IFoo.cs` and `Constants.cs` blameless. Only the extractor can tell those apart.

Spans are extracted for changed files only. Nothing else can contain a touched line, and the smallest-containing-span join reaches no further than the file it is in, so extraction cost scales with the change rather than with the repo.

A touched line falling inside no span, a `using` directive or a field or a class attribute, contributes no changed method and is explicitly **not** the `unknown` state. `unknown` means a changed method the join could not attribute; a line outside every span never becomes a changed method at all. The gate counts these lines and prints the count as a diagnostic, never gating on it, because most of them are legitimately not methods and any threshold would be calibrated against nothing.

Two rules recorded on [issue 6](https://github.com/tvrmsmith/coding-standards/issues/6) narrow. Staleness compares the report timestamp against the newest mtime among **files that contributed a changed method**, not every ACM file in the diff, so a whitespace-only reformat no longer invalidates an otherwise good report. And a run whose changed-method set is **empty** exits 0 before resolving any input, because ADR 0002 makes the gate demand an input only when a selected metric asked for it, and a metric with nothing to compute is not asking. A docs-only commit therefore passes in a repo where nobody ran the tests.
