# A typed path resolves against the working directory, and the repo-root prefix alone folds case

**Status:** accepted 2026-09-22. Splits ADR 0004 with
[ADR 0011](0011-a-source-path-is-repo-relative.md) and
[ADR 0012](0012-a-coverage-path-resolves-by-the-cobertura-join.md). The decision is unchanged.

## Current rule

A path on `--files` or `--coverage` resolves against the process working directory, then
relativizes to the source path of [ADR 0011](0011-a-source-path-is-repo-relative.md). A `--coverage`
path may live anywhere, since a report is not repo content.

A `--files` path is refused, exit 1, when it lies outside the repo root, names anything other than a
regular file, or spells a real file in a case the tree does not use. Every refusal about the path
carries `file_unresolved`.

Case below the repo root is never folded. A repo-root prefix spelled in another case folds when
`os.SameFile` confirms it reaches the root directory, which merges nothing, at every door into
containment. A coverage candidate or report name places where it really sits. A `--files` path whose
mis-cased prefix the developer typed is refused as "is not spelled as the repo root is". Extension
routing folds case too, because it only decides whether to launch a process.

## Decision

**Human-typed paths** resolve against the process cwd, then relativize. An absolute name is not
joined onto the cwd, because that would name a path nobody typed.

The `--files` refusals, in the order they are weighed:

- **Outside the repo root.** Weighed first, since a path above the root has nothing inside the repo
  to stat.
- **A non-regular inode**, covering a directory, fifo, socket and device node alike. The gate says
  "is a directory, not a file" for a directory and "is not a regular file" for the rest, because no
  extractor claims either, and the gate would otherwise exit 0 over a tree the developer believes
  they gated.
- **A mis-cased repo-root prefix the developer typed.** Weighed after the mode checks, so `--files`
  naming the repo root itself answers "is a directory, not a file" in either case.
- **A real file spelled in a case the tree does not use.** A case-insensitive filesystem resolves it
  to a file no coverage report is keyed by, so the run would measure a file the report cannot cover.
  `srcpath.Root` checks the spelling component by component below the root and names the path as the
  developer typed it. Canonicalizing to git's spelling instead of refusing stays available as a
  later relaxation.

A refusal says "does not exist" only when the filesystem said the path is not there. A parent the
process cannot enter, a symlink cycle, or a component that is not a directory carries what the
operating system said instead. The working directory itself is read before any path resolves, so
losing it is a separate failure and not a `file_unresolved`.

### The repo-root prefix

The prefix fold governs the **root prefix alone**. The developer typed the prefix when they typed the
path absolute, and equally when they typed a relative path that climbs above the root and descends
back in, since the mis-cased component sits in the string either way. That path is refused, so the
developer retypes the half that is wrong instead of hunting a location mistake. A relative path that
only descends inherits its prefix from the working directory and folds, because nothing they typed
can be retyped to fix it. A coverage candidate or a report name is not a path anyone typed, so it
folds and is named where it really sits rather than reported as escaping the repo.

Case **below** the root is still refused, unfolded, which is what keeps `case_only_path_difference`
at exit 1 and keeps the gate from attributing coverage to a file the report did not measure.
`strings.EqualFold` runs ahead of the `os.SameFile` confirmation, so a bind mount or a hard-linked
directory, one inode reached under two unrelated names, does not fold either.

## Considered options

**Folding by text.** Rejected in [ADR 0012](0012-a-coverage-path-resolves-by-the-cobertura-join.md),
because on Linux it merges two real files. The prefix fold is admitted because `os.SameFile`
supplies the evidence that reasoning lacked: two directories differing only in case are two inodes on
a case-sensitive filesystem, so `/tmp/REPO` beside a real `/tmp/repo` still reads as outside on
Linux. Taken with [issue 48](https://github.com/tvrmsmith/coding-standards/issues/48) and
[issue 36](https://github.com/tvrmsmith/coding-standards/issues/36). Taking it past `--files`, to
coverage placement and naming, was Trevor's to decide, and he approved it on 2026-09-14.

**Refusing case in extension routing.** Rejected.
[ADR 0009](0009-the-csharp-extractor-is-written-in-house.md) folds case there, because a touched
`Order.CS` matched no row and passed with `changed_methods: 0`, and the only cost of over-claiming is
a process launch that finds nothing. The `--files` spelling refusal applies ADR 0012's reasoning to
a path the developer typed rather than widening it.

## Consequences

`srcpath` is the one owner of containment, so "outside" means one thing at every door (issue 36).
