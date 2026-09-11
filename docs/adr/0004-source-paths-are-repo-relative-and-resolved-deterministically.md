# A source path is repo-relative, and every other path form is resolved to it by one deterministic rule

## Current rule

The gate's single path currency is the **source path**, a repo-relative slash-separated path from
`git rev-parse --show-toplevel`. Every other path form is resolved to it by one deterministic rule.
There is no fuzzy matching, no fallback chain, and no inferred base directory.

Four path spaces, one rule each. Diff paths arrive repo-relative already. A coverage report path is
`<sources><source>` joined to `filename`. An extractor echoes back the path it was handed,
byte-identical. A human-typed path resolves against the current working directory.

A path that will not resolve refuses the run rather than passing with nothing measured:
`coverage_source_root_erased`, then `file_ambiguous`, then a coverage path with zero candidates in
the root, in that precedence. A human-typed path that names a directory, or spells a real file in a
case the tree does not use, is refused as `file_unresolved`. Case folding is rejected for
coverage-path resolution, where it can merge two real files; it is not rejected for extension
routing, which only decides whether to launch a process.

Sections below carry the reasoning and six dated amendments.

## Decision

The gate's single path currency is the **source path**: a repo-relative, slash-separated path from
`git rev-parse --show-toplevel`. Every path the gate handles is resolved to that form, or the run
fails. There is no fuzzy matching, no fallback chain, and no inferred base directory.

Four path spaces feed the gate, and each has exactly one rule.

**The diff** already produces source paths. Nothing to do.

**The coverage report** produces paths per Cobertura's contract, `<sources><source>` joined to the
`filename` attribute of each `<class>`. For each class the gate builds a candidate absolute path
per source, plus the filename itself if it is already absolute (coverlet emits an absolute filename
when no computed source root prefixes the document). Each candidate is resolved with
`filepath.EvalSymlinks` and made relative to the resolved repo root.

**The extractor** must echo `file` back byte-identical to the path the gate handed it. The gate hands
in source paths, so extractor output is already in the canonical form, and a mismatch is exit 1.

**Human-typed paths** on `--files` and `--coverage` resolve against the process cwd, then relativize.
A `--files` path outside the repo root is exit 1. A `--coverage` path may live anywhere, since a
report is not repo content.

The gate resolves the changed set and looks coverage up against it. It does not attempt to canonicalize
every path a report mentions, so a report path outside the repo root, or inside it but untouched by the
diff, is ignored in silence.

**Amended 2026-09-03.** "Case folding for macOS. Rejected" below governs **coverage-path resolution**, where the
candidate is matched against a file on disk and folding can merge two real files on Linux. It does not govern
**extension routing**, where the gate decides whether a changed path is worth handing to an extractor at all.
[ADR 0006](0006-the-csharp-extractor-is-written-in-house.md) folds case there, because a touched `Order.CS` matched
no row and passed with `changed_methods: 0`, and because the only cost of over-claiming is a process launch that
finds nothing. Recorded here so the rejection is not read wider than the evidence behind it.

**Amended 2026-09-03.** Three of the exit-1 resolution rules in Consequences are **not implemented in the tracer**
and land with [issue 16](https://github.com/tvrmsmith/coding-standards/issues/16): the erased source root naming
the MSBuild property, the report contributing zero classes inside the repo root, and the surviving `file_ambiguous`
assertion. The gate resolves what it can and lets an unresolvable report path fall through to the ignore path the
last Consequences paragraph describes, so a report produced in another checkout presents today as the wall of
unknown methods that the zero-classes diagnostic exists to replace. Every `unknown` still fails the run per
[ADR 0001](0001-crap-gate-topology.md), so nothing silently passes; only the diagnosis is deferred. The rules stand
as decided, and this notes their arrival date rather than reopening them.

**Amended 2026-09-04.** Those three rules landed with
[issue 16](https://github.com/tvrmsmith/coding-standards/issues/16), so the deferral the paragraph above records is
closed and the worktree case now exits 1 naming the mismatch rather than presenting as a wall of unknown methods.
Their typed codes are `coverage_source_root_erased`, `file_ambiguous`, and `coverage_outside_repo`, enumerated in
[ADR 0005](0005-the-machine-document-is-the-only-output.md). Two points of detail the Consequences section does not
settle, decided while implementing and recorded here:

- The three are checked per report, in discovery order, and within one report in this precedence: erased source
  root first, over the whole class list and before any candidate is built, then a class resolving to two paths
  inside the root, then the report placing no class inside it. `DeterministicReport` is tested over every class
  ahead of `UseSourceLink`, so a report carrying both shapes names the first, whatever order its classes appear in.
- "A report contributing zero classes inside the repo root fails" is unconditional, as written. Whether a candidate
  failed because it landed outside the root or because the file is gone from disk makes no difference to the
  report-level rule, and the two were tried as separate signals and reverted: a real coverlet report on Unix
  carries `<source>/</source>`, which resolves, so no on-disk signal tells "built in another checkout" apart from
  "deleted since the test run", and splitting them left the container case undiagnosed. A single unplaceable path
  is still ignored in silence, per the last Consequences paragraph. It is only a report with nothing left that
  fails.

A report carrying no `<class>` element at all raises nothing, since it placed nothing to be outside; a changed
method it fails to cover still fails the run as `unknown_changed_method`.

Two more details of the shipped diagnostics, where the Consequences section below promises something narrower than
what the gate emits:

- Consequences says the zero-in-root failure names "one example resolved path". The message names an **example
  path**, the first candidate of the first class carrying a filename, symlink-resolved when it resolved and as the
  join built it when it did not. It is the best available reading of a path the gate compared, not a
  guaranteed-resolved one, and quoting the as-built form is what tells a container mount apart from a test run
  whose files are gone. It degrades to two further shapes the section does not describe: a candidate no `<source>`
  anchored to an absolute path is quoted and named as such, since a bare relative string would read as a path
  inside the repo, and a report whose classes carry no filename to join says so instead of quoting nothing.
- Consequences describes the erased shape as filenames rooted at the `/_/` placeholder. MSBuild numbers the
  placeholder per source root past the first, `/_1/`, `/_2/` and so on, and the shipped detector matches
  `^/_[0-9]*/` to cover them.

Two further resolution rules the implementation settled, both narrowing what counts as landing inside the root:

- A candidate resolving to anything other than a **regular file** lands nowhere, the same as one that resolves
  nowhere at all. A class filename of `..`, `../..`, or a bare directory name joins onto an in-root `<source>` to
  name a directory, which sits inside the root without naming any source the report measured, and one such class
  would otherwise stand in for a whole report's worth of classes that placed nothing, suppressing the zero-in-root
  diagnostic. This is the same reasoning as the rule above, applied to what the path turns out to be rather than to
  where it landed.
- An **already absolute** filename is its own only candidate, and no `<source>` is joined onto it. Joining would
  name a path no report ever carried, `/src/src/app/Order.cs` off `<source>` `/src`, and the outside-repo
  diagnostic quotes the first candidate, so the reader would be shown a path the gate invented.

**Amended 2026-09-05.** The human-typed rule above gained two refusals with
[issue 14](https://github.com/tvrmsmith/coding-standards/issues/14), both under ADR 0005's `file_unresolved` code.
A `--files` path that resolves inside the root but names a directory rather than a regular file is exit 1, matching
the regular-file narrowing the paragraph above applies to coverage candidates. A `--files` path the tree spells in
another case is also exit 1, because a case-insensitive filesystem resolves it to a real file that no coverage
report is keyed by, so the run would measure a file the report cannot cover. That second refusal applies the
reasoning of "Case folding for macOS. Rejected" to a path the developer typed, which the 2026-09-03 amendment above
scoped to coverage-path resolution, so this records the new application rather than widening the rejection.
`srcpath.Root` checks the spelling component by component against the directory entries and names the path as the
developer typed it. Canonicalizing to git's spelling instead of refusing stays available as a later relaxation.

**Amended 2026-09-07.** The first of those two refusals is wider than the paragraph above says. It covers every
non-regular inode, not directories alone, so a `--files` path naming a fifo, socket or device node is exit 1 under
the same code. The gate says "is a directory, not a file" for a directory and "is not a regular file" for the rest,
which matches how the regular-file narrowing above already reads for coverage candidates.

**Amended 2026-09-11.** [Issue 38](https://github.com/tvrmsmith/coding-standards/issues/38) landed the
first case that consumes a report coverlet wrote. `TestFullStackScoresAReportCoverletWrote` in
`gate/test/coverlet_test.go` runs `dotnet test --collect:"XPlat Code Coverage"` over a generated fixture
solution and scores the file the collector left behind. Every other case in that suite builds its XML with
the hand-written `cobertura()` helper, so until now the coverage-report rule in the Decision section above
was only ever checked against this repo's own reading of the format. The 2026-09-04 amendment states the
Unix `<source>/</source>` shape as a fact; the suite now exercises it. The join the case drives is that `/`
plus a class filename carrying the absolute path with its leading slash stripped,
so a checkout at `/work/repo` yields `work/repo/src/Points.cs`, which is what
"`<sources><source>` joined to `filename`" comes to in practice. The same report carries `branch="False"` rather than lowercase, hit counts above 1, a
`<methods>` block duplicating the class-level `<lines>`, and `line-rate` and `lines-covered` on the root
element. The gate handled all of it already and nothing pinned it. `cobertura()` spells several of those
differently and stays exactly as it is, because a stale, unparseable or source-root-erased report is cheap
to build by hand and coverlet will not produce one on demand.

The non-determinism that argued against a case like this needs no normalisation, and that is the decision
rather than an omission. coverlet writes a fresh GUID directory per run and a `timestamp` that moves, and
neither reaches the assertion, because on the success path the machine document names no report path and no
timestamp. `skipped_paths` is empty and the crap table carries source paths only, so the golden's one hole
stays the resolved base. What the harness guarantees instead is that exactly one report exists, which it
gets from a fresh temp repo per run and a single `dotnet test`. A second run would leave a superseded report
the gate lists in `skipped_paths`, and that is a second GUID in the document. The fixture pins its package
versions as literals for the same reason it pins the SDK, so a coverlet bump that changes the report shape
is a deliberate change that reds the case.

## Considered options

**Longest-suffix matching.** The standard fallback, and the one every prior implementation reaches for.
Rejected outright, not even as a last resort. `fabian-barney/crap-typescript` has an open issue about
suffix matching attributing coverage to the wrong file in a monorepo, and a repo holding `src/a/Utils.cs`
and `src/b/Utils.cs` is exactly where it silently picks one. This gate blocks. A wrong join produces a
wrong number and fails the wrong method, which is worse than stopping.

**A fallback chain.** `crap4clj` carries four path fallbacks. Rejected for the same reason: each rung
is a guess, the chain's behaviour depends on which rung fired, and nothing in the output says which one
did. One rule that either holds or fails loudly is diagnosable; four that degrade is not.

**Absolute as the canonical form.** The report side is natively absolute and the staleness check needs
an absolute path to `stat`. Rejected because repo-relative is what the rest of the gate already speaks:
`git diff` emits it, the changed-method set is keyed on it, `--files` takes it from a human, and the
findings have to be readable. Absolute is derived by joining the repo root back on, which is cheap and
lossless. The reverse direction is not.

**Case folding for macOS.** Rejected. Measured on this laptop: `realpath` does not canonicalize case, so
`/tmp/x/samples.cs` resolves to `/private/tmp/x/samples.cs` with the wrong case preserved while
`os.path.exists` returns true. A case mismatch therefore survives resolution and surfaces as an
unresolved changed method, exit 1, naming the file. Folding by default would instead make the gate wrong
on Linux, where two spellings really are two files.

**A `--source-root` escape hatch** for reports whose root has been erased. Rejected as configuration with
no user. The failure names the MSBuild property to turn off, and the day someone hits it the flag is a
small additive change made against a real report rather than a guess.

## Consequences

Path separators need no handling. Coverlet runs `.Replace('\\', '/')` over both source and filename
before writing, so a Windows-produced report already carries forward slashes, with the drive letter
confined to `<source>`.

**A report whose source root has been erased fails the run, exit 1.** `DeterministicReport=true` emits
`<sources/>` empty with filenames rooted at the `/_/` placeholder, and `UseSourceLink=true` emits one
empty source with the raw document key, which can be a URL. Both are detected by cheap string checks and
both name the MSBuild property responsible. Stripping `/_/` and assuming the remainder is repo-relative
is probably correct and is still a guess, which is the thing this ADR refuses.

**A report contributing zero classes inside the repo root fails, naming that report, one example resolved
path, and the repo root.** This is the git-worktree case: a report produced in the main checkout and
gated from a worktree resolves entirely outside `show-toplevel`, and without the diagnostic it presents
as a wall of unknown methods. Detecting the worktree via `git rev-parse --git-common-dir` and accepting
the main checkout's paths was rejected, because that accepts coverage measured against source other than
the source being gated.

**A class yielding more than one candidate inside the repo root fails, naming both.** This is close to
unreachable: coverlet's `GetBasePaths` groups documents by path root, and on Unix every path shares root
`/`, so a Unix report has exactly one `<source>`. Multiple sources need multiple drive letters or UNC
shares, and two of those cannot both sit inside one repo. The assertion costs three lines and can only
fire when that reasoning is wrong. It is the only surviving meaning of the reason `file_ambiguous`,
which now says the report contradicted itself rather than that a fuzzy match had two hits.

**An unresolvable report path is ignored, not fatal.** `filepath.EvalSymlinks` errors on a path that no
longer exists, and a report describes a moment in the past, so a file deleted since the test run is none
of the gate's business. The changed-set side always exists, because `--diff-filter=ACM` over the working
tree guarantees it. The failure path therefore only ever fires where ignoring is correct.

The typed reason `file_unmatched` from ADR 0001 narrows to one meaning: a changed method whose file
matched no report path. Issue 6 already rules that exit 1.

Because resolution runs per report, each report is read with its own `<sources>`, which matters given
that issue 6 unions every discovered report rather than taking the newest.
