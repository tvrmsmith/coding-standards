# ADRs

Every ADR opens with a `## Current rule` block stating what is true today. Read that block. Read the
rest of the file only when you need the reasoning behind the rule or the history of how it changed.

## The live rules

| # | Decision | Live rule |
|---|---|---|
| [0001](0001-crap-gate-topology.md) | crap-gate topology | Complexity from a source-level walker, coverage from coverlet, joined by source span. Any single `unknown` fails the run. |
| [0002](0002-metrics-declare-their-inputs.md) | metrics declare their inputs | The gate demands an input only when a selected metric asked for it. |
| [0004](0004-source-paths-are-repo-relative-and-resolved-deterministically.md) | source paths | One repo-relative path currency, one deterministic rule per path space. A path that will not resolve refuses the run. |
| [0007](0007-changed-method-is-a-span-holding-a-touched-line.md) | changed method | A changed method is a working-tree span holding a touched line. A pure move measures nothing. |
| [0008](0008-the-machine-document-is-the-only-output.md) | machine document | Stdout is one TOON document on every exit path. Seventeen typed exit-1 codes. |
| [0009](0009-the-csharp-extractor-is-written-in-house.md) | csharp extractor | Roslyn syntax parser only, paths on stdin, spans plus per-file parse status on stdout. |

Superseded, kept for their history and not for their rules: [0003](0003-changed-method-is-a-span-holding-a-touched-line.md)
by 0007, [0005](0005-the-machine-document-is-the-only-output.md) by 0008,
[0006](0006-the-csharp-extractor-is-written-in-house.md) by 0009.

## Conventions

An accepted ADR is never reworded for style, outside a consolidation. There are four ways to change
one, and only the last removes anything.

**Amend** when the decision stands and the text is wrong, incomplete, or overtaken by a detail.
Write a paragraph opening with `**Amended YYYY-MM-DD.**` at the start of a line, and place it next
to the text it corrects, so a reader meets the correction where they meet the claim.

**Consolidate** when the decision still stands but the amendments have piled up past the point where
the file reads straight through. Rewrite the file in place, folding every amendment's rule back into
the body, and open it with a `**Consolidated YYYY-MM-DD.**` paragraph saying so. The number stays, so
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

**Five amendments is the consolidation trigger.** Past that the amendments outweigh the decision and
a reader has to reconstruct the rule from a changelog. ADR 0005 reached seventeen, four of them
revising one number, each claiming to supersede the others. 0004 hit six on 2026-09-11 and was
consolidated in place the same day.

**The `## Current rule` block stays under 250 words**, and the whole file under 1500. The block is
what a reader is expected to read in full, so it has to be readable in one sitting; the file is what
they scroll when the block is not enough. A decision that will not fit is more than one decision.
Neither ceiling is checked mechanically yet, which is [issue 87](https://github.com/tvrmsmith/coding-standards/issues/87).

That consolidation took 0004 from 2606 words to 1791, and it dropped three clauses it should have
kept. Two later passes, 008eae6 and 7ef466e, put them back, which is why the file now stands at
1862. It is still over 1500, and the remaining excess is reasoning rather than restatement, so
trimming it again buys little. The next move on it is a split, superseded by one new ADR per
decision, under the mechanics above.

If an ADR looks wrong, that is a decision to escalate to the user, not an edit to make.
