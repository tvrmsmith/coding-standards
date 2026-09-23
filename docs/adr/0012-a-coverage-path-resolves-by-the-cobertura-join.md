# A coverage report path resolves by the Cobertura join, and a report that places nothing refuses the run

**Status:** accepted 2026-09-22. Splits ADR 0004 with
[ADR 0011](0011-a-source-path-is-repo-relative.md) and
[ADR 0013](0013-a-typed-path-resolves-against-the-working-directory.md). The decision is unchanged.

## Current rule

A coverage report path is `<sources><source>` joined to each `<class>`'s `filename`, resolved with
`filepath.EvalSymlinks`, and made relative to the resolved repo root, giving the source path of
[ADR 0011](0011-a-source-path-is-repo-relative.md). Each report resolves against its own
`<sources>`. Case folding by text is rejected, because on Linux it can merge two real files. The one
fold is a repo-root prefix that `os.SameFile` confirms, which places a candidate where it really sits
and is carried by [ADR 0013](0013-a-typed-path-resolves-against-the-working-directory.md).

A single path that resolves nowhere, or outside the root, is ignored in silence. A whole report that
will not resolve refuses the run. Three refusals are checked for each report in this precedence:
`coverage_source_root_erased`, then `file_ambiguous`, then `coverage_outside_repo` for a report
placing no class inside the root. Reports are read in the order the gate takes them, and the first
report that fails stops the run.

## Decision

For each class the gate builds one candidate per source. Two narrowings apply. An already absolute
filename is its own only candidate, with no `<source>` joined onto it, because joining invents a path
no report carried, `/src/src/app/Order.cs` off `<source>` `/src`, and the outside-repo diagnostic
quotes the first candidate. A candidate resolving to anything other than a regular file lands
nowhere, the same as one resolving nowhere at all, because a filename of `..` or a bare directory name
joins onto an in-root `<source>` to name a directory, and one such class would otherwise stand in for
a whole report that placed nothing.

The gate resolves the changed set and looks coverage up against it. It does not canonicalize every
path a report mentions, so a report path outside the repo root, or inside it but untouched by the
diff, is ignored in silence.

**A single unresolvable report path is ignored, not fatal.** `filepath.EvalSymlinks` errors on a path
that no longer exists, and a report describes a moment in the past, so a file deleted since the test
run is none of the gate's business. The changed-set side always exists, because `--diff-filter=ACM`
over the working tree guarantees it. Only a report with nothing left fails. A report carrying no
`<class>` element at all raises nothing, since it placed nothing to be outside; a changed method it
fails to cover still fails the run as `unknown_changed_method`.

### The three refusals

`DeterministicReport` is tested over every class ahead of `UseSourceLink`, so a report carrying both
shapes names the first whatever order its classes appear in.
[ADR 0008](0008-the-machine-document-is-the-only-output.md) enumerates the codes.

**An erased source root fails the run, exit 1**, as `coverage_source_root_erased`.
`DeterministicReport=true` emits `<sources/>` empty with filenames rooted at the `/_/` placeholder,
which MSBuild numbers per source root past the first, so the detector matches `^/_[0-9]*/`.
`UseSourceLink=true` emits one empty source with the raw document key, which can be a URL. Both are
string checks over the whole class list, run before any candidate is built, and both name the
MSBuild property responsible. Stripping `/_/` and assuming the remainder is repo-relative is probably
correct and is still a guess, which is the thing ADR 0011 refuses.

**A class yielding two candidates inside the root fails, naming both**, as `file_ambiguous`. This is
close to unreachable. coverlet's `GetBasePaths` groups documents by path root, and on Unix every path
shares `/`, so a Unix report has exactly one `<source>`. Multiple sources need drive letters or UNC
shares, which cannot both sit inside one repo. The assertion costs three lines and fires only when
that reasoning is wrong. It is the code's only surviving meaning, that the report contradicted itself
rather than that a fuzzy match had two hits.

**A report placing no class inside the root fails, naming the report, an example path, and the
root**, as `coverage_outside_repo`. This is the git-worktree case, where a report produced in the
main checkout resolves entirely outside `show-toplevel` and would otherwise present as a wall of
unknown methods. The rule is unconditional, and landing outside the root and being gone from disk are
the same to it. The two were tried as separate signals and reverted, because a real coverlet report
on Unix carries `<source>/</source>`, which resolves, so nothing on disk tells "built in another
checkout" apart from "deleted since the test run". The example path is the first candidate of the
first class carrying a filename, symlink-resolved when it resolved and as the join built it when it
did not, since the as-built form is what tells a container mount apart from a test run whose files
are gone. It degrades twice. A candidate no `<source>` anchored to an absolute path is quoted and
named as such, because a bare relative string would read as a path inside the repo, and a report
whose classes carry no filename to join says so instead of quoting nothing.

### Checked against a report coverlet wrote

`TestFullStackScoresAReportCoverletWrote` in `gate/test/coverlet_test.go` scores a report
`dotnet test --collect:"XPlat Code Coverage"` actually produced, so the rule is checked against the
producer and not only against this repo's reading of the format. On Unix coverlet emits
`<source>/</source>` with the class filename carrying the absolute path minus its leading slash, so a
checkout at `/work/repo` yields `work/repo/src/Points.cs`. coverlet's fresh GUID directory and moving
`timestamp` need no normalisation, because on the success path the machine document names no report
path and no timestamp. The harness guarantees instead that exactly one report exists, from a fresh
temp repo per run and a single `dotnet test`; a second run would leave a superseded report in
`skipped_paths`. The fixture pins its package versions as literals, for the same reason it pins the
SDK, so a coverlet bump that changes the report shape is a deliberate change that reds the case. The hand-written `cobertura()` helper stays, because a stale,
unparseable or source-root-erased report is cheap to build by hand and coverlet will not produce one
on demand.

## Considered options

**Case folding for macOS.** Rejected. Measured on this laptop, `realpath` does not canonicalize case,
so `/tmp/x/samples.cs` resolves to `/private/tmp/x/samples.cs` with the wrong case preserved while
`os.path.exists` returns true. The mismatch survives resolution and surfaces as an unresolved changed
method, exit 1, naming the file. Folding by text would make the gate wrong on Linux, where two
spellings really are two files. The root-prefix fold in the Current rule merges nothing, because
`os.SameFile` compares inodes rather than text.

**Accepting the main checkout's paths from a worktree**, by reading `git rev-parse
--git-common-dir`. Rejected, because that accepts coverage measured against source other than the
source being gated.

**A `--source-root` escape hatch** for reports whose root has been erased. Rejected as configuration
with no user. The failure names the MSBuild property to turn off, and the day someone hits it the
flag is a small additive change made against a real report rather than a guess.

## Consequences

Resolution runs per report, so each is read with its own `<sources>`, which matters given that issue
6 unions every discovered report rather than taking the newest.
