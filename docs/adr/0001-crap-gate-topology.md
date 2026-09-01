# CRAP is computed from a source-level complexity walker joined to coverage by source span

The CRAP gate takes cyclomatic complexity from a source-level AST walker and coverage from coverlet, and joins the two by `(file path, start line, end line)`, attributing each covered line to the smallest span containing it. No method name, signature, or mangled CLR identifier appears in the join. Complexity extraction is language-specific and produces `{ file, qualifiedName, startLine, endLine, complexity }`; coverage parsing is format-specific and normalizes to `{ file, line, hits }`; the join, the diff scoping, the formula, the threshold, and the exit code are language-neutral.

## Considered options

**Take both numbers from the Cobertura report.** ReportGenerator already does this. It computes `comp² × (1 − cov)³ + comp` per method in `CoberturaParser.SetMethodMetrics` and fails the build against `maximumThresholdForCrapScore`, free and maintained. It also has no join at all, because complexity and coverage come off the same XML element, which eliminates two whole classes of defect.

Rejected because the complexity is coverlet's `Math.Max(1, branches.Count)` IL branch count. Measured against a scratch solution, that inflates an async method holding a single `if` from McCabe 2 to 4, and two sequential `if`s from 3 to 4. The term is then squared. It is also permanently .NET-only, since no JavaScript coverage tool writes a complexity attribute into Cobertura.

**Join by method name.** Rejected on evidence. `7Factor/crap4dotnet` normalizes fully-qualified names on both sides and handles `.ctor`, `get_`/`set_`, `op_*`, and generic arity, but nothing for state machines. Coverage for `async Task Foo()` lands on `<Foo>d__3::MoveNext`, never matches, and defaults to `Coverage = 0.0`, so every async method scores `comp² + comp` however well tested. `crap4java` keys on `class#method:line` with no parameter types, so overloads collide. Of six implementations read, the four that work join by span.

## Consequences

A span join folds compiler-generated code back onto its source method with no demangling, because coverlet records original source line numbers on `MoveNext`: `PlaceAsync` occupying source lines 32-40 reports coverage on `Samples/<PlaceAsync>d__4::MoveNext` as lines 32..40 exactly. It also makes overloads unambiguous for free, since they occupy different spans.

The smallest-containing-span rule is not optional. Coverlet emits a local function as its own element whose lines sit inside the container's span, so pooling a file's lines and filtering by span would let the container absorb them and count them twice.

Path normalization has to be deliberate. Coverlet reported `private/tmp/crapspan/src/SpanLib/Samples.cs`, with `/tmp` resolved through its symlink.

**Amended 2026-09-01.** This paragraph originally read "with the leading slash stripped and `/tmp` resolved through its symlink". Nothing was stripped. The reading ignored the `<sources>` element, and Cobertura's contract is `source + filename`: the same report carries `<sources><source>/</source></sources>`, so the path reconstructs exactly as `/private/tmp/crapspan/src/SpanLib/Samples.cs`. Only the symlink resolution is a real transform. The decision this ADR records is unaffected; the resolution rule is [ADR 0004](0004-source-paths-are-repo-relative-and-resolved-deterministically.md).

A method whose span contains no instrumentable lines is treated as fully covered rather than unknown, which is what makes the "trivial members exclude themselves arithmetically" assumption true. A method that cannot be attributed at all is excluded from scoring, reported with a typed reason, and fails the run if such methods exceed a fraction of the changed set, so a broken join can never pass as untested code.

Adding a second language means writing an extractor, not touching the gate. That was the reason for the seam and it is the reason ReportGenerator lost.

The gate is written in Go, shipped as per-platform static binaries. The seam is what allows this: because the extractor is a separate process emitting JSON, the gate never links against a language toolchain, so it can be the one artifact that runs in a repo holding neither the .NET SDK nor node. A compiled binary also fixes the gate's behaviour to its version rather than to whichever runtime a shim resolved, which matters for a metric-gate that many agents run concurrently on machines nobody controls. The cost is a cross-compile and release step the harness does not need today.
