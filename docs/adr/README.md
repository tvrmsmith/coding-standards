# ADRs

Every ADR opens with a `## Current rule` block stating what is true today. Read that block. Read the
rest of the file only when you need the reasoning behind the rule or the history of how it changed.

## The live rules

| # | Decision | Live rule |
|---|---|---|
| [0001](0001-crap-gate-topology.md) | crap-gate topology | Complexity from a source-level walker, coverage from coverlet, joined by source span. Any single `unknown` fails the run. |
| [0002](0002-metrics-declare-their-inputs.md) | metrics declare their inputs | The gate demands an input only when a selected metric asked for it. |
| [0007](0007-changed-method-is-a-span-holding-a-touched-line.md) | changed method | A changed method is a working-tree span holding a touched line. A pure move measures nothing. |
| [0008](0008-the-machine-document-is-the-only-output.md) | machine document | Stdout is one TOON document on every exit path but two, a malformed command line and a failed stdout write. Every other exit-1 cause carries a typed code, registered in `gate/internal/report`. |
| [0009](0009-the-csharp-extractor-is-written-in-house.md) | csharp extractor | Roslyn syntax parser only, paths on stdin, spans plus per-file parse status on stdout. |
| [0010](0010-lint-blocks-on-any-warning-touching-a-changed-line.md) | lint blocks | Every warning the tool reports blocks the commit when any of its locations holds a touched line. One-shot waivers in an append-only log outside the repo are the only way past. |
| [0011](0011-a-source-path-is-repo-relative.md) | source paths | One repo-relative path currency, one deterministic rule per path space. A path that will not resolve refuses the run. |
| [0012](0012-a-coverage-path-resolves-by-the-cobertura-join.md) | coverage paths | `<source>` joined to `filename`. One path that will not resolve is ignored; a report that places nothing refuses the run. |
| [0013](0013-a-typed-path-resolves-against-the-working-directory.md) | typed paths | `--files` and `--coverage` resolve against the cwd. Only a repo-root prefix `os.SameFile` confirms folds case. |

Superseded, kept for their history and not for their rules: [0003](0003-changed-method-is-a-span-holding-a-touched-line.md)
by 0007, [0004](0004-source-paths-are-repo-relative-and-resolved-deterministically.md) by 0011, 0012
and 0013, [0005](0005-the-machine-document-is-the-only-output.md) by 0008,
[0006](0006-the-csharp-extractor-is-written-in-house.md) by 0009.

## Conventions

An accepted ADR is never reworded for style, outside a consolidation. There are four ways to change
one, and only the last removes anything.

**Amend** when the decision stands and the text is wrong, incomplete, or overtaken by a detail.
Write a paragraph opening with `**Amended YYYY-MM-DD.**` at the start of a line, and place it next
to the text it corrects, so a reader meets the correction where they meet the claim.

**Consolidate** when the decision still stands but the file is over a ceiling below. Rewrite the
file in place, folding every amendment's rule back into the body, and open it with a
`**Consolidated YYYY-MM-DD.**` paragraph saying so. Rewording is what makes a file fit, so it is
allowed here and nowhere else. The number stays, so
every link to it keeps working. No rule may be dropped and no decision may change; `git log -p` on
the file keeps the amendments as they were written. Consolidating is a decision to escalate to the
user first, not a tidy-up to make on your own.

**Supersede** when the decision itself changes, or when one ADR turns out to be more than one
decision. Write a new numbered ADR stating the rule in one pass, one ADR per decision when you are
splitting.
Give the old file a `**Superseded YYYY-MM-DD by [ADR NNNN](...)**` paragraph under its title, naming
every new ADR when you split, and leave everything else in it untouched. Add each new ADR to the
table above and move the old row down to the superseded line. Nothing is deleted, and inbound links
keep pointing at the old file, which carries the reader on to the new numbers.

**Trim** only when the word ceiling below binds and the text to be cut already stands in another ADR
this repo keeps. Ask the user first, because this is the one mode that deletes. Leave a paragraph
opening with `**Trimmed YYYY-MM-DD**` where the text was, naming the ADR that still carries it and
saying how closely, word for word or in its own words. A superseded ADR counts: it is kept for its
history, and a consequence it already states is that history.

**Length is the only trigger.** Amendments are not counted and no number of them obliges anything.
A file that still reads straight through is fine however many dated paragraphs it carries; a file
that does not is over a ceiling, and the ceiling is what says so.

**The `## Current rule` block stays at 250 words or fewer**, and the whole file at 1500 or fewer.
The block is what a reader is expected to read in full, so it has to be readable in one sitting; the
file is what they scroll when the block is not enough. A decision that will not fit is more than one
decision.

CI checks neither ceiling. `.claude/settings.json` sets both as the limits for an `adr-size`
Claude Code hook, which reports an overrun at the edit and never blocks it. No word count is
written into this file, because a hand-kept count goes stale on the first edit, which is what
[issue 87](https://github.com/tvrmsmith/coding-standards/issues/87) was. Run `wc -w` when you need
one.

If an ADR looks wrong, that is a decision to escalate to the user, not an edit to make.
