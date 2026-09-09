# metric-gate emits one TOON document on stdout and has no human output mode

**Superseded 2026-09-09 by [ADR 0008](0008-the-machine-document-is-the-only-output.md).** The
decision is unchanged. 0008 states it in one pass with the seventeen amendments below folded in,
including the typed-code count they revised four times; this file stays for the reasoning and the
dated history, and is not the file to read for the live rule.

`metric-gate` is a flat command with no subcommands. Its stdout is always a single [TOON](https://toonformat.dev/)
document, spec v4.1.1, encoded with the pipe delimiter. There is no `--format` flag, no text renderer, and no
JSON renderer. A human running the binary by hand reads the same document, plus one summary line on stderr:
`2 of 4 changed methods over CRAP threshold 30, worst score 139.34`.

The document carries the whole changed-method set, not only the failures, as a tabular array under a key named
for the metric. A second metric adds a second key.

```toon
status: fail
tool: metric-gate/0.1.0
spec: toon/4.1.1
scope: since
base: main@9f3c110
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

**Amended 2026-09-02.** The `coverage` and `score` cells originally read `0.550`, `0.100`, `1.000`
and `1.00`. Spec v4.1.1 section 2 makes stripping fractional trailing zeros a MUST for any value in
the canonical range, so `1.500` renders as `1.5`, and section 7.2 makes a quoted `"0.550"` the only
way to keep the padding, which would turn every measured cell into a string. Fixed-width columns
lost. Precision is a rounding rule, not a padding rule: the gate rounds `coverage` and
`target_coverage` at three decimals and `score` at two, then renders the canonical form. Every score
issue 12 names survives the change untouched, since `68.05`, `3.33`, `139.34` and `0.363` carry no
trailing zero. The decision this ADR records is unaffected; only the example's digits move.

**Amended 2026-09-02.** `target_coverage` rounds **up** at three decimals rather than half up.
`1 - cbrt((30 - 9) / 81)` is 0.36236, and at coverage 0.362 the score is 30.03, still over the
threshold, while at 0.363 it is 29.94. Half-up rounding would print a target a developer can hit and
still fail, which defeats the reason the cell exists.

**Amended 2026-09-02.** The `unknown_changed_method` path emits **two** lines on stderr, the failure
message and then the summary line, so line 5's "one summary line on stderr" describes every path but
this one. It is the one exit-1 cause that still carries a table, because the join is what discovers
it, so it is also the one where both the error and the scored-method count have something to say. A
reader who saw only the failure message would not know how many methods did score, and one who saw
only the summary would not know why the run failed. The summary line keeps its position last, so a
consumer reading the final line of stderr gets the same field on every path.

**Amended 2026-09-02.** The `scope` field's value for the default diff mode is the token
`merge-base`. The worked example above prints `scope: since` because it was written against a run
given `--since`, and neither this ADR nor [ADR 0003](0003-changed-method-is-a-span-holding-a-touched-line.md)
fixed the default's spelling, which left a machine-contract value undocumented while every golden
baked it in. The token is recorded here rather than renamed, because `merge-base` says what the base
actually is and `since` is the name of the flag that overrides it. One token per diff mode, and a
consumer may branch on it.

**Amended 2026-09-02.** `skipped_paths` is not the `--files` list. It carries the paths **coverage discovery could
not read**, on every run rather than under one flag, and the Consequences section below describes a narrower field
than the one the gate emits. One unreadable directory under the repo root must not abort discovery, because that
would exit 1 with no document at all, while the worst a skipped subtree costs is a report the walk did not see. The
field is what keeps the resulting understated coverage from being unexplained, and it is populated even on the
`coverage_missing` failure, where an unreadable `TestResults` subtree is the likeliest reason no report was found.
It still never gates. The `--files` list the Consequences section names is a second producer that arrives with that
flag, in a later issue.

**Amended 2026-09-02.** Two typed exit-1 codes join the enumeration, both from the extractor seam.
`extractor_capabilities_mismatch` is an extractor whose `--capabilities` set claims none of the changed paths the
gate's static table routed to it, recorded in
[ADR 0006](0006-the-csharp-extractor-is-written-in-house.md). `extractor_duplicate_span` is the same span reported
twice, where a span's identity is `(file, name, startLine, endLine, signature)`. Both are upstream of the join, so
both emit `status: error` with an `error` block and no table, exactly as the rule below says.

**Amended 2026-09-03.** The enumeration is **eleven** codes, and this list supersedes every count above. Two more
joined after the paragraph above was written. `diff_unparseable` is any failure on the diff path, typed at the
boundary rather than at hand-picked call sites, so a git invocation that fails cannot reach the caller as a bare
exit 1 with empty stdout; that shape was chosen deliberately, because guarding two parse sites left the rule true
by luck rather than by construction. `extractor_invalid_span` is a span whose numbers violate the contract, a
complexity below 1, a start line below 1, or an end line before its start, each of which otherwise reads as a clean
pass, a zero-complexity row scoring 0 or a method that vanishes from the table entirely. The full list is
`no_diff_base`, `diff_unparseable`, `extractor_failed`, `extractor_path_mismatch`,
`extractor_capabilities_mismatch`, `extractor_duplicate_span`, `extractor_invalid_span`, `parse_failed`,
`coverage_missing`, `coverage_unparseable`, and `unknown_changed_method`. Adding a code is a contract change and
belongs in this list on the same commit.

**Amended 2026-09-03.** The **staleness** cause this ADR treats as typed is deferred to
[issue 15](https://github.com/tvrmsmith/coding-standards/issues/15) and is absent from the list above. The gate
reads no report timestamp today, so a stale report scores silently rather than failing. Recorded so the gap reads
as a deferral rather than an omission.

**Amended 2026-09-04.** The enumeration is **fourteen** codes, and this list supersedes every count above.
Three joined with [issue 16](https://github.com/tvrmsmith/coding-standards/issues/16), all three on the
coverage-path side and all three upstream of the join, so each emits `status: error` with an `error` block and no
table. `coverage_source_root_erased` is a report MSBuild wrote without a usable source root, naming the property
responsible, `DeterministicReport` or `UseSourceLink`. `file_ambiguous` is one class resolving to more than one
path inside the repo root, which [ADR 0004](0004-source-paths-are-repo-relative-and-resolved-deterministically.md)
narrows to "the report contradicted itself". `coverage_outside_repo` is a report carrying classes of which none
land inside the repo root, the git-worktree and container cases. The full list is `no_diff_base`,
`diff_unparseable`, `extractor_failed`, `extractor_path_mismatch`, `extractor_capabilities_mismatch`,
`extractor_duplicate_span`, `extractor_invalid_span`, `parse_failed`, `coverage_missing`, `coverage_unparseable`,
`coverage_source_root_erased`, `file_ambiguous`, `coverage_outside_repo`, and `unknown_changed_method`. Adding a
code is a contract change and belongs in this list on the same commit, which is the 2026-09-03 rule restated
against the list that now governs.

**Amended 2026-09-04.** [Issue 14](https://github.com/tvrmsmith/coding-standards/issues/14) fills in the `scope`
field's remaining values and adds two codes.

The four tokens are `merge-base` for the default, and `staged`, `since` and `files` for the flags that override
it, each named for the flag rather than for what it resolves to, which is the spelling
`lint-changed-dotnet.sh` already uses. `merge-base` keeps the exception the 2026-09-02 amendment gave it, since no
flag spells the default and there is nothing to name it after. Under `--files` there is no commit to diff against,
so `base` is `null` and `touched_lines_outside_spans` is `0`; a file list carries no line information, so ADR 0003
makes every method in a listed file changed and neither field has anything to say.

The enumeration is **sixteen** codes, and this list supersedes every count above. `staged_file_dirty` is a
`--staged` run refusing a file staged in one state and on disk in another: the index reports the line numbers and
the extractor parses the disk copy, so scoring it would attribute coverage to the wrong text, which ADR 0003 makes
exit 1 rather than a warning. `file_unresolved` is a `--files` path the gate cannot place inside the repo root,
because it does not exist or because it resolves above the root, and its message names the path as the developer
typed it rather than as the gate resolved it. Both are upstream of the join, so both emit `status: error` with an
`error` block and no table. The full list is `no_diff_base`, `diff_unparseable`, `extractor_failed`,
`extractor_path_mismatch`, `extractor_capabilities_mismatch`, `extractor_duplicate_span`,
`extractor_invalid_span`, `parse_failed`, `coverage_missing`, `coverage_unparseable`,
`coverage_source_root_erased`, `file_ambiguous`, `coverage_outside_repo`, `staged_file_dirty`, `file_unresolved`,
and `unknown_changed_method`.

The second `skipped_paths` producer the 2026-09-02 amendment predicted arrives here. A path named on `--files` that
resolves inside the repo but that no extractor claims is neither measured nor an error, so it is listed rather than
dropped in silence, and the named paths sort ahead of whatever coverage discovery could not read. The field still
never gates.

**Amended 2026-09-05.** `file_unresolved` has more causes than the two the 2026-09-04 amendment named. A `--files`
path earns it when it does not exist, when it resolves above the repo root, when it resolves inside the root but
names a directory rather than a regular file, when it names a file the tree holds under a different spelling, which
a case-insensitive filesystem otherwise resolves to a path no coverage report is keyed by, and when the filesystem
refuses to answer at all, which carries the underlying cause rather than escaping the document. The gate refuses all
of them rather than matching approximately, which is the rule CONTEXT.md's source path already states.
`staged_file_dirty` is narrower than that amendment said: the divergence check asks only about files an extractor
claimed, so a file staged in one state and dirty on disk in another does not refuse the run when nothing scored it,
because a file the gate never measured is one it cannot misattribute. The exception is a `--staged` run whose
extraction fails, which checks divergence before reporting the extractor's own failure, so a staged file deleted
from disk is named for what it is. `--files` is variadic and not repeatable, so a second `--files` is a usage error
and issue 11's "(repeatable)" spelling does not hold for it. The enumeration stays at **sixteen** codes and the list
above stands unchanged.

**Amended 2026-09-05.** The enumeration is **fifteen** codes, and this list supersedes every count above. One
joined with [issue 15](https://github.com/tvrmsmith/coding-standards/issues/15), which closes the deferral the
2026-09-03 amendment recorded: the gate now reads the report's own `timestamp` attribute and stats every file
that contributed a changed method, so that amendment's "The gate reads no report timestamp today" no longer
describes the binary. `coverage_stale` is a report whose timestamp predates the newest of those mtimes. It emits
`status: error` with an `error` block and no table, and no method is scored against a report the run refused. The
message names the report and closes with the step that clears it, which differs by how the report reached the
gate: a discovered one says to clear stale `TestResults` directories and re-run the tests, and one named on
`--coverage` says to regenerate that file or point the flag somewhere current. Clearing a directory does nothing
for a path the developer typed. The full list is `no_diff_base`, `diff_unparseable`, `extractor_failed`,
`extractor_path_mismatch`, `extractor_capabilities_mismatch`, `extractor_duplicate_span`,
`extractor_invalid_span`, `parse_failed`, `coverage_missing`, `coverage_unparseable`, `coverage_stale`,
`coverage_source_root_erased`, `file_ambiguous`, `coverage_outside_repo`, and `unknown_changed_method`.

Two rules under that code are decisions rather than mechanics. **A report the staleness rule cannot judge is
refused as `coverage_unparseable`, not trusted.** A missing `timestamp` attribute, an empty one, and one that is
not base-10 epoch seconds all refuse, and the message separates an absent or empty attribute from one that is
present and unreadable, since a report carrying `timestamp="2026-01-01T00:00:00Z"` needs its units fixed and
telling its author the attribute is absent sends them looking for something already there. Absent and empty
share one wording because `encoding/xml` decodes both to the same empty string and the gate cannot tell them
apart. **Staleness compares whole truncated seconds, and equal seconds is not
stale**, which deviates from the "with zero tolerance" wording on issue 15. The Cobertura attribute carries
second resolution and cannot express anything finer, so a literal sub-second comparison would refuse reports on
an ordering the format never recorded.

`--coverage` also narrows the 2026-09-02 amendment's "on every run rather than under one flag". `skipped_paths`
carries what coverage discovery could not read, and a run given `--coverage` never walks, so the field is empty
there by construction rather than because the walk found everything readable. That is the honest answer: the gate
cannot report on directories it had no reason to open, and filling the field from a walk the developer opted out
of would invent a diagnostic. A consumer reading `skipped_paths: []` therefore learns nothing about the
filesystem on a `--coverage` run, and the reports it should ask about are the ones named on the command line.

The same change adds argument parsing, so the binary can now reject its own command line, and that run is the one
exception to "stdout is one TOON document". A malformed invocation writes nothing to stdout, prints the usage
error to stderr, and exits with the existing tool-error 1. The document reports on a repository the gate
examined, and a run whose arguments never parsed never chose one to examine, so there is nothing for a document
to describe. No typed code covers it and the enumeration stays at fifteen, because the codes name what the gate
found in a repository and this run has no repository.

**Amended 2026-09-07.** Two corrections to the 2026-09-05 amendment on `file_unresolved` above, neither of them a
change of decision. First, `file_unresolved` covers every non-regular inode, not the directory case alone. The gate
says "is a directory, not a file" for a directory and "is not a regular file" for a fifo, socket or device node, so
a `--files` path naming any of them is refused under the same code. Second, the extraction-failure exception for
`staged_file_dirty` asks only about the paths the extractor was handed and failed on, not about every path in the
diff, so an unrelated dirty staged file no longer replaces the extractor's own cause. A file staged and then
deleted from disk is still named for what it is, because that path is one the extractor was handed.

**Amended 2026-09-09.** The two counts stated on 2026-09-05 crossed. The issue 14 amendment says **sixteen**,
counting `staged_file_dirty` and `file_unresolved`; the issue 15 amendment says **fifteen**, counting
`coverage_stale`; each was written against a list the other had not landed yet, and each claims to supersede every
count above it. The union is **seventeen**, which supersedes both, and the Consequences section below still says
"fourteen exit-1 causes" because it predates all three. The full list is `no_diff_base`, `diff_unparseable`,
`extractor_failed`, `extractor_path_mismatch`, `extractor_capabilities_mismatch`, `extractor_duplicate_span`,
`extractor_invalid_span`, `parse_failed`, `coverage_missing`, `coverage_unparseable`, `coverage_stale`,
`coverage_source_root_erased`, `file_ambiguous`, `coverage_outside_repo`, `staged_file_dirty`, `file_unresolved`,
and `unknown_changed_method`. No code is added or removed here; only the count and the one list that governs are.

**Amended 2026-09-09.** One parser owns argv. The 2026-09-05 amendment on issue 15 records that the binary can
reject its own command line, and issue 14 adds the scope flags to the same command line, so `--staged`, `--since`,
`--files` and `--coverage` are parsed in one place and one usage block lists all four. Two parsers, each rejecting
the other's flags as unknown, cannot print a usage block that tells the truth. The exit-1-with-empty-stdout shape
that amendment gives a malformed invocation is unchanged, and no typed code covers it, for the reason it gives:
the codes name what the gate found in a repository and a run whose arguments never parsed never chose one.

**Amended 2026-09-09.** The 2026-09-07 amendment above describes the `staged_file_dirty` extraction-failure
exception as asking "only about the paths the extractor was handed and failed on". `Extract` fails atomically, so
there is no per-path "failed on" to ask about, and the gate asks `extract.Routable` over the changed set, a
membership test against the static extension table. Read the exception as: when extraction fails under `--staged`,
the gate asks whether any routable changed path is staged in one state and dirty on disk in another, and reports
`staged_file_dirty` in place of the extractor's own cause only when that check names such a path. An extractor
failure with no divergent path keeps its own cause, which is the decision the 2026-09-07 paragraph intended and
the wording missed.

**Amended 2026-09-09.** The staged-and-dirty refusal is narrowed a third way, beside the claimed-files-only
narrowing and the extraction-failure exception. The divergence check runs git with `-w` and with
`-c core.fileMode=false`, so a file staged in one state and then reindented or chmod'd on disk is scored rather
than refused. [ADR 0003](0003-changed-method-is-a-span-holding-a-touched-line.md) states the rule flatly, that a
file staged in one state and dirty in another exits 1 naming those files. Read it with this narrowing: the refusal
fires on a change that can move a line, not on one that changes only whitespace or the executable bit, because
neither can shift a line number and the only reason to refuse is that the gate would otherwise score index content
against working-tree line numbers.

Three parts of that shape are decisions in their own right.

**The fix instruction is two typed cells, never prose.** `action` is one of `raise_coverage`, `split_method`, or
`none`, and `target_coverage` is the coverage that would bring the method under the threshold at its current
complexity, `1 - cbrt((T - comp) / comp²)`, or `null`. `split_method` is emitted exactly when complexity exceeds
the threshold, because at full coverage CRAP reduces to `comp` and no test can rescue the method. The numbers
alone leave the reader to work out which of two unrelated instructions applies, and the arithmetic already knows.

**The field list is fixed and never varies with the outcome.** `reason` is present and `null` on every measured
row. A header that changed shape would force a consumer to branch on `status` before it could read a row.

**The table is present exactly when the join ran.** That is exit 0 with methods scored, exit 2, and the single
exit-1 cause `unknown_changed_method`, which the join is what discovers. Every other exit-1 cause is upstream of
the join and emits `status: error` with an `error` block and no table. An empty changed-method set emits
`crap: []`, since with no elements there is no uniform shape to declare.

## Considered options

**A human text mode as the default, TOON behind `--format toon`.** The reflex, and the shape the harness scripts
have. Rejected because the only thing text carried that a table could not was the instruction sentence, and once
`action` and `target_coverage` are cells that gap closes. What remains is two renderers that must agree on every
field forever, of which the machine one is exercised by tests and the human one is exercised by nobody, so the
human one drifts.

**Text and TOON and JSON.** Rejected for the same reason, one worse. TOON round-trips to JSON losslessly, so a
caller that wants `jq` pipes through the reference CLI, and the binary carries one encoder rather than two that
have to stay in step.

**Selecting the format by whether stdout is a TTY.** Rejected outright. Output that changes shape under a pipe is
the failure where you debug the version you cannot see.

**Comma, the TOON default delimiter.** Rejected because C# qualified names contain commas. `Repo.Lookup<TKey,TValue>`
would be quoted on most generic methods, where under pipe no cell needs quoting in practice. The quoting rules are
still implemented for the cells that do.

## Consequences

An agent is the expected reader, and this is the surface it acts on. It branches on `status`, then on `error.code`
or on the `action` column, and never parses English. The fourteen exit-1 causes each carry a typed `code`, so
"your report is stale" and "your method is too complex" are distinguishable without inspecting prose, which the
exit code alone cannot do.

The two diagnostics ADR 0003 requires, the count of touched lines falling outside every span and the skipped-path
list under `--files`, are document fields rather than a footer. Both exist to signal a gap in the extractor, which
is exactly the thing a reader should notice without reading prose, and ADR 0003 already rules they never gate.

The encoder is written in-house against the TOON spec rather than taken from a port. The gate emits one fixed
uniform table and never decodes, so this is a header line and a row writer. Both Go ports were unstable at the time
of writing, the official `toon-format/toon-go` marked in development and the community `alpkeskin/gotoon` carrying
no status, and ADR 0001 chose a static binary precisely so the gate's behaviour is fixed to its version rather than
to whatever a machine resolved. The spec version is pinned in the document header so a future reader knows what it
conforms to.

Adding a field to the table is a breaking change for anything reading rows positionally. The `[N]` count and the
`{fields}` header make that detectable rather than silent, which is the guarantee TOON's tabular form exists to
give.
