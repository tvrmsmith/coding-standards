# dotnet/

Two projects, both Roslyn.

`Tvrmsmith.Analyzers` — the C# half of the lint layer: the three custom Roslyn analyzers, plus
curated severities for the off-the-shelf analyzers they sit alongside. Everything below the Layout
section is about this one.

`Tvrmsmith.MetricGate.CSharp` — the C# half of the metric gate: a console tool that reads source
paths on stdin and writes one JSON document of method spans and cyclomatic complexities on stdout,
or its extension list when run with `--capabilities`. It is syntax only, never a project load. The
gate that consumes it lives in `gate/`;
[ADR 0006](../docs/adr/0006-the-csharp-extractor-is-written-in-house.md) records why it is written
in house.

The Claude Code plugin loader ignores this directory.

## Layout

```
Directory.Build.props                        # pinned versions, the artifacts path
src/Tvrmsmith.Analyzers/
  Tvrmsmith.Analyzers.csproj
  DiagnosticIds.cs                           # TVRM0001-0003
  Descriptors.cs                             # titles, messages, severities, help links
  AssertionSyntax.cs                         # shared .Should() and receiver-chain recognition
  CombineAssertionsOnSameObjectAnalyzer.cs   # TVRM0001
  NoSuppressionBeforeAssertionAnalyzer.cs    # TVRM0002
  NoAssertionEscapeCastAnalyzer.cs           # TVRM0003
  AnalyzerReleases.{Shipped,Unshipped}.md
  config/Tvrmsmith.Analyzers.globalconfig    # the curated severities
  build/Tvrmsmith.Analyzers.props            # nupkg auto-import, for the published package
  local/Tvrmsmith.Analyzers.Local.props      # bare-DLL injection, for local adoption
  local/tvrmsmith-scope-changed.sh           # generates the changed-files-only severities
src/Tvrmsmith.MetricGate.CSharp/
  Program.cs                                 # --capabilities, or paths on stdin, JSON on stdout
  Extractor.cs                               # per-file parse status and spans
  MethodSpanCollector.cs                     # the spans, with the signature spelling the gate keys on
  ComplexityWalker.cs                        # the cyclomatic decision points
  CapabilitiesResult.cs, ExtractionResult.cs, FileStatusResult.cs, MethodSpanResult.cs
tests/Tvrmsmith.Analyzers.Tests/             # the analyzers, against a real compilation
tests/Tvrmsmith.MetricGate.CSharp.Tests/     # the extractor, over fixtures/, through the real process
tests/Consumer/                              # stands in for a consuming repo
tests/verify-severities.sh                   # proves the analyzers and severities reach a build
artifacts/local/                             # generated; the stable path consumers point at
```

## The three custom analyzers

The guidelines the enforcement mapping found no off-the-shelf home for. All
three are C#-only, all three default to **warning** and none is ever an error: this runs
machine-locally against code other people wrote and are not being asked to change. None ships a
code fix — for all three the correct rewrite needs a judgement call the analyzer cannot make.

| ID | Rule | Flags |
|---|---|---|
| `TVRM0001` | `combine-assertions-on-same-object` | consecutive assertions picking apart one object, and `HaveCount` followed by indexing into the same collection |
| `TVRM0002` | `no-suppression-before-assertion` | `!` or `?.` in the receiver chain feeding `.Should()` |
| `TVRM0003` | `no-assertion-escape-cast` | a cast to `object` whose only purpose is to reach `ObjectAssertions` |

Two scoping decisions carry most of the precision, and both are load-bearing:

**`TVRM0001` groups on the outermost identifier, and stands down under an `AssertionScope`.**
`response.StatusCode` and `response.Headers.Location` are reached through different intermediates
but are rooted at the same `response`, and that is enough to group them. Deliberately wide — it
will sometimes name a pair that cannot actually be combined, which is affordable at warning
severity. The counterweight is the scope: under `using (new AssertionScope())`, or after a
`using var scope = new AssertionScope();`, the property grouping is not reported at all, reading
the scope as the author saying these genuinely cannot be combined. Count-then-index still fires
inside a scope, because `BeEquivalentTo` against an expected array always works and exempting it
would make the scope a way to hide it. A receiver containing a method call never opens a group
either: `Load().Page` and `Load().Age` are two calls, not one object.

**`TVRM0002` looks only at the receiver chain, never at arguments.** `Client.BaseAddress!` is a
setup precondition rather than the value under test, and the reference exempts it by name. A
file-wide search for `!` would catch it.

`TVRM0003` deliberately stops at the mechanical half of its guideline. *Which* target to assert
on instead is judgement, and stays **[review-only]** in the skill text.

## Curated severities

Two layers. **FluentAssertions.Analyzers `FAA0001`–`FAA0004`** is the assertion half, selected by
the enforcement mapping. All four ship as `Info`, which never surfaces in a build, so they are
elevated to `warning` — not `error`, because adoption is machine-local with no CI gate.

The second layer is **the built-in `CAxxxx` rules the SDK ships disabled**, minus a short
exclusion list, and it maps to no guideline. Same one-directional rule as the TypeScript
`test-integrity` slice: a rule that is already installed and already correct does not need a
guideline written for it first.

No package backs that layer. The SDK already loads the analyzers, and a `.globalconfig` configures
severity by id regardless of which assembly emits the diagnostic. Setting an explicit severity is
also what *enables* a rule that ships disabled, so this reaches the same rules as
`<AnalysisMode>All</AnalysisMode>` without injecting that property and without sweeping in the
exclusions.

The rules that are *on* by default are deliberately left alone. Raising them here would also
subject them to changed-files scoping, which would *reduce* what the target repo already reports.

`node ../generate-ca-severities.mjs` regenerates the block. It reads the rule set from the SDK
rather than a pinned list, so an SDK upgrade that adds `CA` rules picks them up instead of quietly
leaving them off, and it emits the two other places the same ids must appear — see
"Three files, one list" below. The exclusions and the reason for each are in the generator.

`FluentAssertions.Analyzers` is the right pairing for FluentAssertions 6.x. Against
AwesomeAssertions 9.x it emits **zero diagnostics, silently**: the analyzer gates on
`Compilation.GetTypeByMetadataName("FluentAssertions.AssertionExtensions")`, and AwesomeAssertions
v9 renamed that namespace, so the gate never opens and the run looks clean. That pairing needs
`AwesomeAssertions.Analyzers` instead. Diagnostic IDs and editorconfig keys are identical across
both, so the severity block itself is portable.

## Where the severities have to live, and why it is not obvious

A `.globalconfig` shipped only inside the nupkg does not reach a bare `<Analyzer Include>`
consumer — and local adoption *is* the bare-DLL path. So one config file is wired up twice:

| Phase | Consumption | Adds the analyzers | Adds the severities |
|---|---|---|---|
| 1 | bare `<Analyzer Include>` via `CustomAfterMicrosoftCommonProps` | `local/…Local.props` | `local/…Local.props` |
| 2 | `PackageReference` | NuGet | `build/…props` (auto-imported) |

The build stages the analyzer DLL, `FluentAssertions.Analyzers.dll`, the `.globalconfig` and the
phase-1 props file side by side into `artifacts/local/`, so every path inside that props file is
relative to itself and the directory can be moved or symlinked anywhere.

## Build and verify

```bash
dotnet test tests/Tvrmsmith.Analyzers.Tests/Tvrmsmith.Analyzers.Tests.csproj
dotnet test tests/Tvrmsmith.MetricGate.CSharp.Tests/Tvrmsmith.MetricGate.CSharp.Tests.csproj
dotnet build src/Tvrmsmith.Analyzers/Tvrmsmith.Analyzers.csproj -c Release
./tests/verify-severities.sh
```

`Tvrmsmith.Analyzers.Tests` runs each analyzer over snippets compiled against the real
FluentAssertions 6.12.2, with a fixture type that carries its own `Should()` returning a bespoke
assertions type — the shape `TVRM0003` exists for. Every rule has both halves: the violating
shape it must flag, and the compliant rewrite plus the near-misses it must stay silent on.

`verify-severities.sh` builds `tests/Consumer` — which has Central Package Management on and
references neither analyzer package, the way a consuming repo would — three times:

1. **baseline**, no injection → zero `FAA` diagnostics. Keeps the control honest.
2. **injected** via `CustomAfterMicrosoftCommonProps` → `FAA0001`, `FAA0002` and
   `TVRM0001`–`TVRM0003` as *warnings*, no `NU1008`/`NU1010`. For the `FAA` ids severity is the
   load-bearing assertion: both ship as `Info`, so `warning FAA0001` can only mean the
   `.globalconfig` was applied, not merely that the DLL loaded. The `TVRM` ids are already
   warnings in their own descriptors, so what they prove is delivery.
3. **packaged**, consuming the nupkg from a temp feed → the same five warnings. Package layout
   fails quietly when it fails (a dependency carrying `exclude="Analyzers"` restores clean and
   emits nothing), so it gets consumed for real rather than inspected. Each run packs a unique
   `0.1.0-verify.<epoch>` version, because NuGet caches by id/version and a rebuilt `0.1.0` would
   otherwise restore as whatever `0.1.0` was first seen.

It then builds a fourth consumer — a throwaway git repository holding both violation files — to
cover changed-file scoping. The two files carry disjoint id sets, so "scoped correctly" and
"suppressed everything" are distinguishable, which is the failure that section exists to catch.

## Changed-file scoping

Roslyn has no diff awareness: analyzers run over the whole compilation, always. On a mature
codebase that is thousands of pre-existing sites across hundreds of files, all of it in code
nobody is being asked to change.

So `Tvrmsmith.Analyzers.Local.props` runs `tvrmsmith-scope-changed.sh` before `CoreCompile`. The
script writes a second global `AnalyzerConfig` — every injected id at `none`, re-raised to
`warning` in a per-file section for each changed file — and prints its path, which the target
adds to `@(EditorConfigFiles)`. A plain `dotnet build` is scoped with no wrapper, and so is the
IDE: design-time builds run `CoreCompile` too.

| | |
|---|---|
| changed set | working tree against `HEAD`, plus untracked files |
| widen it | `TVRMSMITH_SCOPE_SINCE=main` — everything the branch changed |
| turn it off | `-p:TvrmsmithAnalyzersScopeToChanged=false` — the whole standing backlog |
| regeneration window | `TVRMSMITH_SCOPE_TTL` seconds, default 5 |

Three things about it are load-bearing and fail silently when wrong:

- **`global_level = 100`.** The curated config deliberately sits at the default level 0. At equal
  levels the conflicting entries cancel and the scoping config is ignored entirely — measured, and
  no conflict warning is emitted. Levels only order global configs against each other, so a
  matching entry in the target repo's own `.editorconfig` still outranks this one regardless.
- **Section paths must be absolute**, and resolved, for the same reason the `StartsWith` condition
  must be.
- **The generated file is a `csc` input.** Rewriting it with identical content would still move
  its timestamp and recompile the whole tree, so the script only publishes a real change. The TTL
  exists for the same reason in reverse: one build of a large solution compiles dozens of
  projects and the script runs once per project.

The pre-commit hook (`harness/lint-changed-dotnet.sh`) builds with scoping **off** on purpose. It
is keyed on what is *staged* and this is keyed on the working tree; letting both filter would make
the reported set their intersection, silently.

## Phase-1 install

`harness/bootstrap dotnet <repo>` does all of this and then proves it. By hand it is:

```xml
<!-- ~/.config/coding-standards.props — a *sibling* of ~/.config/coding-standards, which is a
     symlink into this repo; a file "inside" it would land in the working tree. -->
<Project>
  <Import Project="$(HOME)/dev/personal/coding-standards/dotnet/artifacts/local/Tvrmsmith.Analyzers.Local.props"
          Condition="$(MSBuildProjectDirectory.StartsWith('/Users/you/dev/target-monorepo/'))
                     or '$(MSBuildProjectDirectory)' == '/Users/you/dev/target-monorepo'" />
</Project>
```

```bash
export CustomAfterMicrosoftCommonProps="$HOME/.config/coding-standards.props"
```

The trailing slash on the repo path is not cosmetic: without it `StartsWith('/a/b')` also matches
`/a/b-other`. The second branch is what it costs: `MSBuildProjectDirectory` carries no trailing
slash, so a `.csproj` sitting *at* the repo root reports exactly `<repo>` and the prefix test can
never match it. The path must be the **resolved** one — MSBuild normalises
`MSBuildProjectDirectory` but does not resolve symlinks, so a prefix reached through one never
matches, and neither does the equality branch.

Nothing is committed to the target repo. The `StartsWith` condition scopes the blast radius; the
env var is global, but the props file is a no-op outside that path — verified against an
unrelated C# repo, where the `Analyzer` item group holds only the SDK's own implicit analyzers
and `WarningsNotAsErrors` is empty. Bare `<Analyzer Include>` rather than `PackageReference` is
what sidesteps Central Package Management — no `NU1008`/`NU1010`, and no dependency on the env
var being set at restore time.

Set `TvrmsmithAnalyzersEnabled=false` to switch it off for a build.

### TreatWarningsAsErrors

Injection must never fail a build that succeeded without it, and a repo that sets
`TreatWarningsAsErrors=true` — most do — would take every id here as a build *error*. So
`Tvrmsmith.Analyzers.Local.props` also sets:

```
WarningsNotAsErrors += TVRM0001-3;FAA0001-4;<every enabled CA id>;AD0001;CS8032;CS8034;CS9057
```

### Three files, one list

The enabled ids have to appear in three places, and only the first announces itself when they
drift:

1. `config/Tvrmsmith.Analyzers.globalconfig` — the severities.
2. `local/Tvrmsmith.Analyzers.Local.props` — `WarningsNotAsErrors`. An id missing here becomes a
   build *error* wherever the target sets `TreatWarningsAsErrors`, the one outcome the injection
   promises never to cause.
3. `local/tvrmsmith-scope-changed.sh` — the `ids=` list. An id missing here escapes changed-files
   scoping and reports the whole standing backlog on every build.

`generate-ca-severities.mjs` writes all three from one exclusion table. `--check` exits non-zero
when they are stale.

`WarningsNotAsErrors` is an allowlist, not a switch — it becomes `csc /warnaserror-:<ids>`, so
only the named ids are demoted. Everything the target already fails on keeps failing;
`verify-severities.sh` asserts both halves against a `TreatWarningsAsErrors=true` consumer. The
last four ids are not ours to emit but exist only because we injected — an analyzer that threw,
an assembly that would not load, one built against a newer compiler — and each is a warning, so
without the exemption a crash in our code becomes a broken build of someone else's.

The exemption is set from an import that runs *before* the csproj body, so a project assigning
`WarningsNotAsErrors` without `$(WarningsNotAsErrors)` in the value would silently drop it.
`bootstrap dotnet` scans the target for that rather than trusting it.

### IDE

The language server reads the same MSBuild evaluation, so nothing extra is needed beyond the env
var being visible to it. A GUI-launched IDE inherits from launchd and never sees a shell profile,
which is why `bootstrap dotnet` also runs `launchctl setenv` and installs a `RunAtLoad`
LaunchAgent. To check what the IDE will actually get:

```bash
dotnet msbuild <proj> -t:Compile -p:DesignTimeBuild=true -p:SkipCompilerExecution=true \
  -p:ProvideCommandLineArgs=true -getItem:CscCommandLineArgs
```

`/analyzer:`, `/analyzerconfig:` and `/warnaserror-:` in that output are the whole story.

Phase 2 publishes the package to GitHub Packages under `tvrmsmith`, not to nuget.org.
