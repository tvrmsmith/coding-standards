# The machine document is the only output

**Status:** accepted 2026-09-09. Consolidates
[ADR 0005](0005-the-machine-document-is-the-only-output.md) and its seventeen amendments. The
decision is unchanged; 0005 holds the reasoning that got here.

## Current rule

`metric-gate` is one flat command. There are no subcommands.

Stdout is one [TOON](https://toonformat.dev) document (spec v4.1.1, pipe delimiter) and nothing
else, on every exit path, including every failure. There is no `--format` flag and no human-readable
mode. Diagnostics, progress and the one-line summary go to stderr, where a human reads them and a
pipe drops them.

`status` is `pass`, `fail` or `error`. Exit 0 is a pass, exit 2 is a threshold exceeded, exit 1 is a
tool error. A caller distinguishes "the code is bad" from "the gate is broken" on the exit code
alone, before parsing anything.

Every exit-1 cause carries a typed `error.code`. There are seventeen:

`no_diff_base`, `diff_unparseable`, `extractor_failed`, `extractor_path_mismatch`,
`extractor_capabilities_mismatch`, `extractor_duplicate_span`, `extractor_invalid_span`,
`parse_failed`, `coverage_missing`, `coverage_unparseable`, `coverage_stale`,
`coverage_source_root_erased`, `file_ambiguous`, `coverage_outside_repo`, `staged_file_dirty`,
`file_unresolved`, `unknown_changed_method`.

Adding a code is a contract change and edits that list on the same commit. `error.message` beside it
is prose for a human and no caller branches on it.

`scope` is one of `merge-base`, `staged`, `since`, `files`. `base` is null under `files`, which has
no base to name.

The crap table is present exactly when the join ran: exit 0 with methods scored, exit 2, and
`unknown_changed_method`. Its field list is fixed and does not vary with the outcome, so a consumer
writes one parser rather than one per status. A malformed command line exits 1 with empty stdout and
no typed code, because argv failed before the run had a shape to report.

## One document, one shape

The document is a fixed shape rather than a shape that grows with the run. The scalar fields are
always present. `action` and `target_coverage` are typed cells in the table rather than prose, so a
consumer reading "raise coverage to 0.8" reads `0.8` and not a sentence.

**Amended 2026-09-09.** A table with no rows encodes as `crap: []`, since with no elements there is
no uniform shape to declare. This qualifies "Its field list is fixed and does not vary with the
outcome". ADR 0005 fixed that encoding and this file lost it in consolidation, while `internal/toon`
still implements it.

`skipped_paths` lists paths no extractor claimed. Two things produce it: a changed file whose
extension no extractor in the language table handles, and a `--files` path that resolves to a real
file no extractor claims. Both proceed; neither fails the run.

**Amended 2026-09-09.** A third thing produces `skipped_paths`: a discovered coverage report a
fresher run already superseded (issue 32). It proceeds too, unless it is the only coverage the run
found, in which case the run still fails with `coverage_stale`.

**Amended 2026-09-11.** This corrects the producer count in both "Two things produce it" above and
"A third thing produces `skipped_paths`" in the amendment between, which together reach three and
name the wrong ones. There are two. The first is a `--files` path resolving to a real file no
extractor claims. The second is coverage discovery, covering the paths it could not read, a
directory it cannot enter among them, and the reports it found superseded. The `coverage_stale`
caveat in that intervening amendment stands. Nothing produces an entry for a changed file whose
extension no extractor handles, because no diff-scope selection writes the field; discovery still
writes it under every scope, so a merge-base run does emit entries. The two are ordered, the
`--files` paths first and the discovery skips after them. ADR 0005 fixed that order and this file
lost it in consolidation, while `cmd/metric-gate` still enforces it with a deliberate append and
`gate/test/golden/files_skip_before_discovery_skip.toon` still pins it. Merging the two lists and
sorting the result reads as tidier and breaks it.

One parser owns argv. Every flag the command takes, `--staged`, `--since`, `--files` and
`--coverage`, is parsed in one place and printed in one usage block. Two parsers, each rejecting the
other's flags as unknown, cannot print a usage block that tells the truth.

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

## Consequences

Every new failure mode is a contract change. Adding one means picking a typed code, adding it to the
list above, and writing the golden. That is deliberate friction: it keeps the enumeration honest and
stops the codes drifting into a bag of strings.

The goldens are the test surface. A black-box suite runs the real binary and compares whole
documents, so a field that changes shape fails loudly rather than being noticed by a consumer later.

Anything a human wants to see during a run has exactly one place to go, stderr, and it can be as
chatty as it likes there without any risk to the contract.
