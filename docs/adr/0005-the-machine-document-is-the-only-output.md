# metric-gate emits one TOON document on stdout and has no human output mode

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
