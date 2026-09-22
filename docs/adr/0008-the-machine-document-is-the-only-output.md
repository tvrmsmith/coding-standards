# The machine document is the only output

**Status:** accepted 2026-09-09. Consolidates
[ADR 0005](https://github.com/tvrmsmith/coding-standards/blob/3fd48d4ad6c2a98d2332fb872ab319e80658bb91/docs/adr/0005-the-machine-document-is-the-only-output.md) and its seventeen amendments. The
decision is unchanged; 0005 holds the reasoning that got here.

**Consolidated 2026-09-17.** This folds the five dated amendments this file carried back into the
body and adds the stdout write carve-out the code has implemented since
[issue 81](https://github.com/tvrmsmith/coding-standards/issues/81). No rule is dropped and no
decision changes. The amendments recorded what consolidation from 0005 had lost, the `crap: []`
encoding, the `action` tokens, the `skipped_paths` producers and their order, and the flag list,
so folding them in is the point rather than a side effect. `git log -p` on this file keeps them as
they were written.

## Current rule

`metric-gate` is one flat command. There are no subcommands.

Stdout is one [TOON](https://toonformat.dev) document (spec v4.1.1, pipe delimiter) and nothing
else, on every exit path, including every failure. There is no `--format` flag and no human-readable
mode. Diagnostics, progress and the one-line summary go to stderr, where a human reads them and a
pipe drops them.

`status` is `pass`, `fail` or `error`. Exit 0 is a pass, exit 2 is a threshold exceeded, exit 1 is a
tool error. A caller distinguishes "the code is bad" from "the gate is broken" on the exit code
alone, before parsing anything.

Every exit-1 cause carries a typed `error.code`. `gate/internal/report` enumerates the codes, where
`register` declares each one, and that registry is the contract. `error.message` beside it is prose
for a human and no caller branches on it. Adding a code is a contract change, registering it and
writing the golden that pins it; `report.TestEveryErrorCodeIsPinnedByAGolden` fails on a code no
golden names, and on a golden naming a code the registry does not know.

Two exits carry neither. A malformed command line exits 1 with empty stdout and no typed code,
because argv failed before the run had a shape to report. A stdout write that fails exits 1 with no
typed code either, because the document is the thing that did not arrive and the code would have to
be printed where the write just failed. Nothing else leaves the document behind.

## One document, one shape

The document is a fixed shape rather than a shape that grows with the run. The scalar fields are
always present and the field list does not vary with the outcome, so a consumer writes one parser
rather than one per status.

`scope` is one of `merge-base`, `staged`, `since`, `files`. `base` is null under `files`, which has
no base to name.

The crap table is present exactly when the join ran: exit 0 with methods scored, exit 2, and
`unknown_changed_method`. A table with no rows encodes as `crap: []`, since with no elements there
is no uniform shape to declare.

`action` and `target_coverage` are typed cells in the table rather than prose, so a consumer reading
"raise coverage to 0.8" reads `0.8` and not a sentence. `action` is one of `raise_coverage`,
`split_method` or `none`. `target_coverage` is the coverage that would bring the method under the
threshold at its current complexity, and it is `null` on every row whose `action` is not
`raise_coverage`; `internal/crap` owns the arithmetic and the direction it rounds. On a scored row,
`measured` or `structural_na`, `split_method` is emitted exactly when complexity exceeds the
threshold, because at full coverage CRAP reduces to complexity and no test can rescue the method. An
`unknown` row carries `none` whatever its complexity, since the join produced no score to act on.
Adding a token is a contract change and edits this paragraph on the same commit, the same way adding
a cause registers its `error.code` in `gate/internal/report`.

**Amended 2026-09-22.** Two things consolidating
[ADR 0005](https://github.com/tvrmsmith/coding-standards/blob/3fd48d4ad6c2a98d2332fb872ab319e80658bb91/docs/adr/0005-the-machine-document-is-the-only-output.md) into this file lost, both restored here
(issue 89). The first is how a typed float cell is rendered. A column's precision is a rounding rule
and not a padding rule: the cell is rounded half up to that precision and then written in the spec's
§2 canonical decimal form, no exponent, no padded trailing zeros, and no fraction at all once it is
zero. `coverage` and `target_coverage` round at three decimals, `score` at two. Fixed-width padding,
`1.000` on a fully covered method, was rejected on the spec escalation as a deliberate §2 violation,
and rounding then rendering canonically is not one. `cellToken` in `gate/internal/toon` is where it
lives.

The second is a document, whole, so the shape this section describes can also be read:

```
status: fail
tool: metric-gate/0.1.0
spec: toon/4.1.1
scope: merge-base
base: origin/main@9f3c110
changed_methods: 4
touched_lines_outside_spans: 1
skipped_paths: []
metrics[1|]{name|threshold|measured|failed}:
  crap|30|4|2
crap[4|]{file|start|end|name|complexity|coverage|score|state|action|target_coverage|reason}:
  src/Ordering/Pricing.cs|18|71|Pricing.Quote|34|0.55|139.34|measured|split_method|null|null
  src/Ordering/OrderService.cs|41|58|OrderService.PlaceAsync|9|0.1|68.05|measured|raise_coverage|0.363|null
  src/Ordering/OrderService.cs|60|64|OrderService.Cancel|3|0.667|3.33|measured|none|null|null
  src/Ordering/Order.cs|14|14|Order.get_Id|1|1|1|structural_na|none|null|null
```

The last row is the precision rule in one line: a coverage of 1.0 and a score of 1.0 both render as
`1`, never as `1.000` or `1.00`. `TestEncode_ADR0008WorkedExample` pins these exact bytes, so the
document above and the encoder cannot drift apart.

`skipped_paths` lists paths no extractor claimed, and two things produce it. The first is a
`--files` path that resolves to a real file no extractor claims. The second is coverage discovery,
covering the paths it could not read, a directory it cannot enter among them, and the reports it
found superseded (issue 32). Both proceed and neither fails the run, unless a superseded report is
the only coverage the run found, in which case the run still fails with `coverage_stale`. Nothing
produces an entry for a changed file whose extension no extractor handles, because no diff-scope
selection writes the field; discovery still writes it under every scope, so a merge-base run does
emit entries. The two are ordered, the `--files` paths first and the discovery skips after them,
which `cmd/metric-gate` enforces with a deliberate append and
`gate/test/golden/files_skip_before_discovery_skip.toon` pins. Merging the two lists and sorting the
result reads as tidier and breaks it.

One parser owns argv. Every flag the command takes, `--staged`, `--since`, `--files`, `--coverage`,
`--metric` and `--threshold`, is parsed in one place and printed in one usage block. Two parsers,
each rejecting the other's flags as unknown, cannot print a usage block that tells the truth.

## Considered options

**Human-readable stdout with `--json` for machines.** The familiar shape, and what most CLIs do.
Rejected because it makes the machine path the second-class one. A gate's primary caller is CI, and
the failure it must never have is a machine consumer parsing prose that changed wording between
releases. Making the human read stderr costs a human one flag they do not have to remember.

**A `--format` flag with `toon` and `json` values.** Rejected because two output formats is two
contracts, two sets of goldens, and a question at every new field about whether both spell it the
same way. One format is a decision that stays decided.

**Subcommands.** Rejected because the gate does one thing. A subcommand tree is a promise of a
second thing, and the flat command is the shape that can grow a flag without reorganising.

**Untyped errors, message only.** Rejected because a caller that wants to treat a missing coverage
report differently from a broken one has nothing to branch on but wording. The typed code is the
part of the error that is a contract; the message is not.

**Exit 1 for every failure, pass or broken.** Rejected because CI cannot tell a red build from a
broken tool without parsing, and the one thing a gate owes its caller is that distinction at the
cheapest possible price.

**A typed code for the stdout write failure too.** Rejected because there is nowhere to put it. The
cause is stdout refusing the bytes, so the document carrying the code would take the same path that
just failed, and a short write leaves a truncated document behind rather than nothing. The exit code
is the only signal a caller can trust on this one cause, which is why the rule above carves it out
rather than asking the gate to honour a contract stdout has already broken.

## Consequences

Every new failure mode is a contract change. Adding one means picking a typed code, registering it
in `gate/internal/report`, and writing the golden. That is deliberate friction: it keeps the
enumeration honest and stops the codes drifting into a bag of strings.

The goldens are the test surface. A black-box suite runs the real binary and compares whole
documents, so a field that changes shape fails loudly rather than being noticed by a consumer later.

Anything a human wants to see during a run has exactly one place to go, stderr, and it can be as
chatty as it likes there without any risk to the contract.
