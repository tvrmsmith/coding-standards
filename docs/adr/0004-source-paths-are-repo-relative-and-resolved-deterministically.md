# A source path is repo-relative, and every other path form is resolved to it by one deterministic rule

**Consolidated 2026-09-11.** Six dated amendments folded into the body; the 2606 words were what
forced the pass. The decision is unchanged. Three clauses were dropped that should have been kept and
two later passes put them back, so the prose is new but the rules are all here. `git log -p` on this file carries the amendments as they were written, with the dates
they were decided.

## Current rule

The gate's single path currency is the **source path**, a repo-relative slash-separated path from
`git rev-parse --show-toplevel`. Every other path form is resolved to it by one deterministic rule.
There is no fuzzy matching, no fallback chain, and no inferred base directory.

Four path spaces, one rule each. Diff paths arrive repo-relative already. A coverage report path is
`<sources><source>` joined to `filename`. An extractor echoes back the path it was handed,
byte-identical. A human-typed path resolves against the current working directory.

A path that will not resolve refuses the run rather than passing with nothing measured. The three
report-level refusals are checked per report, in this precedence: `coverage_source_root_erased`,
then `file_ambiguous`, then `coverage_outside_repo` for a report placing no class inside the root. A
human-typed path that names anything other than a regular file, or spells a real file in a case the
tree does not use, is refused as `file_unresolved`. Case folding is rejected for coverage-path
resolution, where it can merge two real files; it is not rejected for extension routing, which only
decides whether to launch a process, nor for a repo-root prefix that `os.SameFile` confirms reaches
the root directory, which merges nothing.

## Decision

Every path the gate handles is resolved to a source path, or the run fails. Four path spaces feed
the gate, and each has exactly one rule.

**The diff** already produces source paths. Nothing to do.

**The coverage report** produces paths per Cobertura's contract, `<sources><source>` joined to the
`filename` attribute of each `<class>`. For each class the gate builds one candidate per source,
resolves each with `filepath.EvalSymlinks`, and makes it relative to the resolved repo root. Two
narrowings apply. An already absolute filename is its own only candidate, with no `<source>` joined
onto it, because joining invents a path no report carried, `/src/src/app/Order.cs` off `<source>`
`/src`, and the outside-repo diagnostic quotes the first candidate. A candidate resolving to anything
other than a regular file lands nowhere, the same as one resolving nowhere at all, because a filename
of `..` or a bare directory name joins onto an in-root `<source>` to name a directory, and one such
class would otherwise stand in for a whole report that placed nothing.

**The extractor** must echo `file` back byte-identical to the path the gate handed it. The gate hands
in source paths, so extractor output is already canonical, and a mismatch is exit 1.

**Human-typed paths** on `--files` and `--coverage` resolve against the process cwd, then relativize.
A `--files` path outside the repo root is exit 1, and two more shapes are exit 1 under
`file_unresolved`. One names a non-regular inode, covering a directory, fifo, socket and device node
alike; the gate says "is a directory, not a file" for a directory and "is not a regular file" for the
rest. The other spells a real file in a case the tree does not use, which a case-insensitive
filesystem resolves to a file no coverage report is keyed by, so the run would measure a file the
report cannot cover. `srcpath.Root` checks the spelling component by component and names the path as
the developer typed it. Canonicalizing to git's spelling instead of refusing stays available as a
later relaxation. A `--coverage` path may live anywhere, since a report is not repo content.

The gate resolves the changed set and looks coverage up against it. It does not canonicalize every
path a report mentions, so a report path outside the repo root, or inside it but untouched by the
diff, is ignored in silence.

### Case folding is scoped to coverage-path resolution

"Case folding for macOS. Rejected" below governs coverage-path resolution, where the candidate is
matched against a file on disk and folding can merge two real files on Linux. It does not govern
extension routing, where the gate decides whether a changed path is worth handing to an extractor at
all. [ADR 0009](0009-the-csharp-extractor-is-written-in-house.md) folds case there, because a touched
`Order.CS` matched no row and passed with `changed_methods: 0`, and the only cost of over-claiming is
a process launch that finds nothing. The `--files` case refusal above applies the rejection's
reasoning to a path the developer typed rather than widening it.

### Checked against a report coverlet wrote

`TestFullStackScoresAReportCoverletWrote` in `gate/test/coverlet_test.go` scores a report
`dotnet test --collect:"XPlat Code Coverage"` actually produced, so the rule above is checked against
the producer and not only against this repo's reading of the format. On Unix coverlet emits
`<source>/</source>` with the class filename carrying the absolute path minus its leading slash, so a
checkout at `/work/repo` yields `work/repo/src/Points.cs`. That is what the join comes to in
practice. coverlet's fresh GUID directory and moving `timestamp` need no normalisation, and that is
the decision rather than an omission, because on the success path the machine document names no
report path and no timestamp. What the harness guarantees instead is that exactly one report exists,
which it gets from a fresh temp repo per run and a single `dotnet test`. A second run would leave a
superseded report the gate lists in `skipped_paths`, and that is a second GUID in the document. The
fixture pins its package versions as literals for the same reason it pins the SDK, so a coverlet
bump that changes the report shape is a deliberate change that reds the case. The case's own comment
carries the rest of the report shape. The hand-written `cobertura()` helper stays as it is, because
a stale, unparseable or source-root-erased report is cheap to build by hand and coverlet will not
produce one on demand.

## Considered options

**Longest-suffix matching.** Rejected outright, not even as a last resort.
`fabian-barney/crap-typescript` has an open issue about suffix matching attributing coverage to the
wrong file in a monorepo, and a repo holding `src/a/Utils.cs` and `src/b/Utils.cs` is exactly where it
silently picks one. A wrong join fails the wrong method, which is worse than stopping.

**A fallback chain.** `crap4clj` carries four path fallbacks. Rejected for the same reason. Each rung
is a guess, the chain's behaviour depends on which rung fired, and nothing in the output says which
one did.

**Absolute as the canonical form.** Rejected because repo-relative is what the rest of the gate
speaks, and findings have to be readable. Absolute is derived by joining the repo root back on, cheap
and lossless. The reverse is not.

**Case folding for macOS.** Rejected. Measured on this laptop, `realpath` does not canonicalize case,
so `/tmp/x/samples.cs` resolves to `/private/tmp/x/samples.cs` with the wrong case preserved while
`os.path.exists` returns true. The mismatch survives resolution and surfaces as an unresolved changed
method, exit 1, naming the file. Folding by default would make the gate wrong on Linux, where two
spellings really are two files.

**Amended 2026-09-11.** The paragraph above rejects folding **by text**, which on Linux merges two real
files. Folding a root prefix **verified by `os.SameFile`** cannot: two directories differing only in case
are two inodes on a case-sensitive filesystem, so `/tmp/REPO` beside a real `/tmp/repo` still reads as
outside on Linux, which is the property the rejection protects. That narrow fold is now taken, with
[issue 48](https://github.com/tvrmsmith/coding-standards/issues/48) and
[issue 36](https://github.com/tvrmsmith/coding-standards/issues/36). Taking it past `--files`, to `Place`
and `Name`, was Trevor's to decide, and he approved it on 2026-09-14.

It governs the **root prefix alone**, on every door into containment: a `--files` path whose root prefix the
developer spelled mis-cased is still exit 1, now as "is not spelled as the repo root is" rather than "is
outside the repo root", so the developer retypes the half that is wrong instead of hunting a location
mistake. They spelled it when they typed the path absolute, and equally when they typed a relative one that
climbs above the root and descends back in, since the mis-cased component sits in the string either way; a
relative path that only descends inherits its prefix from the working directory and folds. And a
coverage candidate or a report name with the same prefix places and is named where it really sits rather
than being reported as escaping the repo. Folding on the coverage side is what the 2026-09-03 amendment
scoped the rejection to, and it is admitted here because `os.SameFile` supplies the evidence that
amendment's reasoning lacked, not because the rejection is being narrowed by preference.

Case **below** the root is still refused, unfolded, which is what keeps `case_only_path_difference` at exit
1 and keeps the gate from attributing coverage to a file the report did not measure. `strings.EqualFold`
runs ahead of the `os.SameFile` confirmation, so a bind mount or a hard-linked directory, one inode reached
under two unrelated names, does not fold either.

**A `--source-root` escape hatch** for reports whose root has been erased. Rejected as configuration
with no user. The failure names the MSBuild property to turn off, and the day someone hits it the
flag is a small additive change made against a real report rather than a guess.

## Consequences

Path separators need no handling. Coverlet runs `.Replace('\\', '/')` over both source and filename
before writing, so a Windows-produced report already carries forward slashes, with the drive letter
confined to `<source>`.

The three refusals below are checked per report, in discovery order, and within one report in the
order they appear here. `DeterministicReport` is tested over every class ahead of `UseSourceLink`, so
a report carrying both shapes names the first whatever order its classes appear in.
[ADR 0008](0008-the-machine-document-is-the-only-output.md) enumerates the codes.

**An erased source root fails the run, exit 1**, as `coverage_source_root_erased`.
`DeterministicReport=true` emits `<sources/>` empty with filenames rooted at the `/_/` placeholder,
which MSBuild numbers per source root past the first, so the detector matches `^/_[0-9]*/`.
`UseSourceLink=true` emits one empty source with the raw document key, which can be a URL. Both are
string checks over the whole class list, run before any candidate is built, and both name the
MSBuild property responsible. Stripping `/_/` and assuming the remainder is repo-relative is probably
correct and is still a guess, which is the thing this ADR refuses.

**A class yielding two candidates inside the root fails, naming both**, as `file_ambiguous`. This is
close to unreachable. coverlet's `GetBasePaths` groups documents by path root, and on Unix every path
shares `/`, so a Unix report has exactly one `<source>`. Multiple sources need drive letters or UNC
shares, which cannot both sit inside one repo. The assertion costs three lines and fires only when
that reasoning is wrong. It is the code's only surviving meaning, that the report contradicted itself
rather than that a fuzzy match had two hits.

**A report placing no class inside the root fails, naming the report, an example path, and
the root**, as `coverage_outside_repo`. This is the git-worktree case, where a report produced in the
main checkout resolves entirely outside `show-toplevel` and would otherwise present as a wall of
unknown methods. Reading `git rev-parse --git-common-dir` and accepting the main checkout's paths was
rejected, because that accepts coverage measured against source other than the source being gated.
The rule is unconditional, and landing outside the root and being gone from disk are the same to it.
The two were tried as separate signals and reverted, because a real coverlet report on Unix carries
`<source>/</source>`, which resolves, so nothing on disk tells "built in another checkout" apart from
"deleted since the test run". The example path is the first candidate of the first class carrying a
filename, symlink-resolved when it resolved and as the join built it when it did not, since the
as-built form is what tells a container mount apart from a test run whose files are gone. It degrades
twice. A candidate no `<source>` anchored to an absolute path is quoted and named as such, because a
bare relative string would read as a path inside the repo, and a report whose classes carry no
filename to join says so instead of quoting nothing.

**A single unresolvable report path is ignored, not fatal.** `filepath.EvalSymlinks` errors on a path
that no longer exists, and a report describes a moment in the past, so a file deleted since the test
run is none of the gate's business. The changed-set side always exists, because `--diff-filter=ACM`
over the working tree guarantees it. Only a report with nothing left fails. A report carrying no
`<class>` element at all raises nothing, since it placed nothing to be outside; a changed method it
fails to cover still fails the run as `unknown_changed_method`.

`file_unmatched` from [ADR 0001](0001-crap-gate-topology.md) narrows to one meaning, a changed method
whose file matched no report path. Issue 6 already rules that exit 1. Resolution runs per report, so
each is read with its own `<sources>`, which matters given that issue 6 unions every discovered
report rather than taking the newest.
