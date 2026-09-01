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
  src/Ordering/Pricing.cs|18|71|Pricing.Quote|34|0.550|139.34|measured|split_method|null|null
  src/Ordering/OrderService.cs|41|58|OrderService.PlaceAsync|9|0.100|68.05|measured|raise_coverage|0.363|null
  src/Ordering/OrderService.cs|60|64|OrderService.Cancel|3|0.667|3.33|measured|none|null|null
  src/Ordering/Order.cs|14|14|Order.get_Id|1|1.000|1.00|structural_na|none|null|null
```

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
or on the `action` column, and never parses English. The eleven exit-1 causes each carry a typed `code`, so
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
