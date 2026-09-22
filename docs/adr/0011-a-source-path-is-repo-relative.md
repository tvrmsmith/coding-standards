# A source path is repo-relative, and each path space reaches it by one deterministic rule

**Status:** accepted 2026-09-22. Splits ADR 0004 with
[ADR 0012](0012-a-coverage-path-resolves-by-the-cobertura-join.md) and
[ADR 0013](0013-a-typed-path-resolves-against-the-working-directory.md). The decision is unchanged.

## Current rule

The gate's single path currency is the **source path**, a repo-relative slash-separated path from
`git rev-parse --show-toplevel`. Every path the gate handles is resolved to a source path by one
deterministic rule, or the run fails. There is no fuzzy matching, no fallback chain, and no inferred
base directory.

Four path spaces feed the gate, and each has exactly one rule. Diff paths arrive as source paths
already. An extractor echoes back the path it was handed, byte-identical. A coverage report path is
`<sources><source>` joined to `filename`, which ADR 0012 carries. A path a human typed resolves
against the current working directory, which ADR 0013 carries.

## Decision

**The diff** already produces source paths. Nothing to do.

**The extractor** must echo `file` back byte-identical to the path the gate handed it. The gate hands
in source paths, so extractor output is already canonical, and a mismatch is exit 1.

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

## Consequences

Path separators need no handling. Coverlet runs `.Replace('\\', '/')` over both source and filename
before writing, so a Windows-produced report already carries forward slashes, with the drive letter
confined to `<source>`.

`file_unmatched` from [ADR 0001](0001-crap-gate-topology.md) narrows to one meaning, a changed method
whose file matched no report path. Issue 6 already rules that exit 1.
