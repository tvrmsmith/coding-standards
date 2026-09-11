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

An accepted ADR is never rewritten in place and never reworded for style. There are two ways to
change one.

**Amend** when the decision stands and the text is wrong, incomplete, or overtaken by a detail.
Write a paragraph opening with `**Amended YYYY-MM-DD.**` at the start of a line, and place it next
to the text it corrects, so a reader meets the correction where they meet the claim.

**Supersede** when the decision itself changes, and when amendments have accumulated past the point
where the file can be read straight through. Write a new numbered ADR stating the rule in one pass.
Give the old file a `**Superseded YYYY-MM-DD by [ADR NNNN](...)**` paragraph under its title and
leave everything else in it untouched. Add the new ADR to the table above and move the old row down
to the superseded line. Nothing is deleted.

**Five amendments is the consolidation trigger.** Past that the amendments outweigh the decision and
a reader has to reconstruct the rule from a changelog. ADR 0005 reached seventeen, four of them
revising one number, each claiming to supersede the others.

**The `## Current rule` block stays under 250 words**, and the whole file under 1500. The block is
what a reader is expected to read in full, so it has to be readable in one sitting; the file is what
they scroll when the block is not enough. A decision that will not fit is more than one decision.
0007, 0008 and 0009 run 193 to 240 words in the block and 784 to 1499 in the file.

0004 is past both triggers, six amendments and 2513 words, and is the next one to consolidate.

If an ADR looks wrong, that is a decision to escalate to the user, not an edit to make.
