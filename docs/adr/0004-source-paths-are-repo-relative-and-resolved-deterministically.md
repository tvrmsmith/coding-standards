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
the root, in that precedence. A `--files` path is refused as `file_unresolved` when it names
anything other than a regular file, and when it spells a real file in a case the tree does not use,
among other reasons Decision lists.

Case folding is rejected for coverage-path resolution, where it can merge two real files; it is not
rejected for extension routing, which only decides whether to launch a process, nor for a repo-root
prefix that `os.SameFile` confirms reaches the root directory, which merges nothing. So an absolute
human-typed path that mis-cases the repo-root prefix places inside the repo on the strength of that
fold, and once it resolves to a regular file it is refused for its spelling rather than for its
location.

Sections below carry the reasoning and the fold history.

## Decision

Four path spaces feed the gate, and each has exactly one rule.

**The diff** already produces source paths. Nothing to do.

**The coverage report** produces paths per Cobertura's contract, `<sources><source>` joined to the
`filename` attribute of each `<class>`. For each class the gate builds a candidate absolute path per
source, resolves it with `filepath.EvalSymlinks`, and makes it relative to the resolved repo root.
Two narrowings on what counts as landing inside:

- A candidate resolving to anything other than a **regular file** lands nowhere, the same as one that
  resolves nowhere at all. A class filename of `..`, `../..`, or a bare directory name joins onto an
  in-root `<source>` to name a directory, which sits inside the root without naming any source the
  report measured, and one such class would otherwise stand in for a whole report's worth of classes
  that placed nothing, suppressing the zero-in-root diagnostic. This is the same reasoning as landing
  outside the root, applied to what the path turns out to be rather than to where it landed.
- An **already absolute** filename is its own only candidate, and no `<source>` is joined onto it.
  Coverlet emits one when no computed source root prefixes the document. Joining would name a path no
  report ever carried, `/src/src/app/Order.cs` off `<source>` `/src`, and the outside-repo diagnostic
  quotes the first candidate, so the reader would be shown a path the gate invented.

**The extractor** must echo `file` back byte-identical to the path the gate handed it. The gate hands
in source paths, so extractor output is already in the canonical form, and a mismatch is exit 1.

**Human-typed paths** on `--files` and `--coverage` resolve against the process cwd, then relativize.
A `--coverage` path may live anywhere, since a report is not repo content. Every `--files` refusal
about the path is exit 1 under the `file_unresolved` code, the one for a path outside the repo root
included, and so is a filesystem that will not answer about the path or about the root, which
carries the operating system's own words. Three more refuse a path that does resolve inside the
root:

- A path resolving inside the root that names anything other than a regular file. The gate says "is a
  directory, not a file" for a directory and "is not a regular file" for a fifo, socket or device
  node. Same narrowing the coverage side applies, applied to what the developer typed.
- A path the tree spells in another case, because a case-insensitive filesystem resolves it to a real
  file that no coverage report is keyed by, so the run would measure a file the report cannot cover.
  `srcpath.Root` checks the spelling component by component against the directory entries and names
  the path as the developer typed it. Canonicalizing to git's spelling instead of refusing stays
  available as a later relaxation. This applies the case-folding rejection below to a path the
  developer typed, a new application of that rejection rather than a widening of it.
- An **absolute** path whose repo-root prefix is mis-cased, refused as "is not spelled as the repo
  root is" rather than "is outside the repo root", so the developer retypes the half that is wrong
  instead of hunting a location mistake. A relative path carries no root prefix the developer typed,
  so it folds and goes on to the component-by-component check above.

The gate resolves the changed set and looks coverage up against it. It does not attempt to canonicalize
every path a report mentions, so a report path outside the repo root, or inside it but untouched by the
diff, is ignored in silence.

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

The rejection governs **coverage-path resolution**, where a candidate is matched against a file on disk.
It does not govern **extension routing**, where the gate only decides whether a changed path is worth
handing to an extractor at all. [ADR 0009](0009-the-csharp-extractor-is-written-in-house.md) folds case
there, because a touched `Order.CS` matched no row and passed with `changed_methods: 0`, and because the
only cost of over-claiming is a process launch that finds nothing.

Nor does it govern a **repo-root prefix verified by `os.SameFile`**, taken with
[issue 48](https://github.com/tvrmsmith/coding-standards/issues/48) and
[issue 36](https://github.com/tvrmsmith/coding-standards/issues/36). What the rejection refuses is
folding **by text**, which on Linux merges two real files. Confirming one inode cannot: two
directories differing only in case are two inodes on a case-sensitive filesystem, so `/tmp/REPO`
beside a real `/tmp/repo` still reads as outside on Linux, which is the property the rejection
protects. That is why the coverage side admits this fold, on the evidence `os.SameFile` supplies
rather than on preference.

It governs the root prefix alone, at every door into containment: a coverage candidate or a report
display name whose prefix is mis-cased places and is named where it really sits rather than being
reported as escaping the repo, and a mis-cased root prefix on an absolute `--files` path is refused
for how the root is spelled, per Decision's third `--files` bullet.

Case **below** the root is still refused, unfolded, which is what keeps `case_only_path_difference` at
exit 1 and keeps the gate from attributing coverage to a file the report did not measure.
`strings.EqualFold` runs ahead of the `os.SameFile` confirmation, so a bind mount or a hard-linked
directory, one inode reached under two unrelated names, does not fold either.

**A `--source-root` escape hatch** for reports whose root has been erased. Rejected as configuration with
no user. The failure names the MSBuild property to turn off, and the day someone hits it the flag is a
small additive change made against a real report rather than a guess.

## Consequences

Path separators need no handling. Coverlet runs `.Replace('\\', '/')` over both source and filename
before writing, so a Windows-produced report already carries forward slashes, with the drive letter
confined to `<source>`.

Three exit-1 rules land here, typed `coverage_source_root_erased`, `file_ambiguous` and
`coverage_outside_repo` in [ADR 0008](0008-the-machine-document-is-the-only-output.md). They are checked
per report, in the order the reports arrive, which is discovery order when the gate found them and
the order the flags were typed when `--coverage` named them, and within one report in that order. The erased source root is tested
over the whole class list before any candidate is built.

**A report whose source root has been erased fails the run, exit 1.** `DeterministicReport=true` emits
`<sources/>` empty with filenames rooted at a `/_/` placeholder, and `UseSourceLink=true` emits one
empty source with the raw document key, which can be a URL. Both are detected by cheap string checks and
both name the MSBuild property responsible. MSBuild numbers the placeholder per source root past the
first, `/_1/`, `/_2/` and so on, so the detector matches `^/_[0-9]*/`. `DeterministicReport` is tested
over every class ahead of `UseSourceLink`, so a report carrying both shapes names the first, whatever
order its classes appear in. Stripping `/_/` and assuming the remainder is repo-relative is probably
correct and is still a guess, which is the thing this ADR refuses.

**A class yielding more than one candidate inside the repo root fails, naming both.** This is close to
unreachable: coverlet's `GetBasePaths` groups documents by path root, and on Unix every path shares root
`/`, so a Unix report has exactly one `<source>`. Multiple sources need multiple drive letters or UNC
shares, and two of those cannot both sit inside one repo. The assertion costs three lines and can only
fire when that reasoning is wrong. It is the only surviving meaning of the reason `file_ambiguous`,
which now says the report contradicted itself rather than that a fuzzy match had two hits.

**A report contributing zero classes inside the repo root fails, naming that report, an example path,
and the repo root.** This is the git-worktree case: a report produced in the main checkout and gated
from a worktree resolves entirely outside `show-toplevel`, and without the diagnostic it presents as a
wall of unknown methods. Detecting the worktree via `git rev-parse --git-common-dir` and accepting the
main checkout's paths was rejected, because that accepts coverage measured against source other than the
source being gated.

The rule is unconditional. Whether a candidate failed because it landed outside the root or because the
file is gone from disk makes no difference at report level, and the two were tried as separate signals
and reverted: a real coverlet report on Unix carries `<source>/</source>`, which resolves, so no on-disk
signal tells "built in another checkout" apart from "deleted since the test run", and splitting them
left the container case undiagnosed.

**Only a report with nothing left fails.** A single unplaceable path is still ignored in silence, per
the "unresolvable report path" rule below. A report carrying no `<class>` element at all raises
nothing, since it placed nothing to be outside; a changed method it fails to cover still fails the run
as `unknown_changed_method`.

The **example path** is the first candidate of the first class carrying a filename, symlink-resolved when
it resolved and as the join built it when it did not. It is the best available reading of a path the gate
compared, not a guaranteed-resolved one, and quoting the as-built form is what tells a container mount
apart from a test run whose files are gone. It degrades to two further shapes: a candidate no `<source>`
anchored to an absolute path is quoted and named as such, since a bare relative string would read as a
path inside the repo, and a report whose classes carry no filename to join says so instead of quoting
nothing.

**An unresolvable report path is ignored, not fatal.** `filepath.EvalSymlinks` errors on a path that no
longer exists, and a report describes a moment in the past, so a file deleted since the test run is none
of the gate's business. The changed-set side always exists, because `--diff-filter=ACM` over the working
tree guarantees it. The failure path therefore only ever fires where ignoring is correct.

The typed reason `file_unmatched` from ADR 0001 narrows to one meaning: a changed method whose file
matched no report path. Issue 6 already rules that exit 1.

Because resolution runs per report, each report is read with its own `<sources>`, which matters given
that issue 6 unions every discovered report rather than taking the newest.

## History

Accepted with six dated amendments, five folded on 2026-09-11 and one dropped.

- **2026-09-03**, first entry. Scoped the case-folding rejection to coverage-path resolution, once
  ADR 0009 landed the fold for extension routing. **Folded**, because it recorded that arriving.
- **2026-09-03**, second entry. Deferred three exit-1 rules to
  [issue 16](https://github.com/tvrmsmith/coding-standards/issues/16). **Dropped**, because the
  deferral closed when the rules landed and Consequences now states them directly.
- **2026-09-04**. Recorded those three exit-1 rules arriving. **Folded**.
- **2026-09-05**. Added the human-typed refusals that came with
  [issue 14](https://github.com/tvrmsmith/coding-standards/issues/14). **Folded**.
- **2026-09-07**. Widened the first of those refusals from directories to every non-regular inode.
  **Folded**.
- **2026-09-11**. Narrowed the case-folding rejection to exempt a repo-root prefix that
  `os.SameFile` confirms, and refused an absolute `--files` path with a mis-cased root prefix.
  **Folded** into two places, the Rejected alternatives entry it narrows and Decision's third
  `--files` bullet.

No decision changed in the fold.
