# Local adoption harness

Wire a repository you work in up to the personal coding standards **without anyone else
seeing it**. Adoption is solo by design: the target repo may be shared, the standards are not,
so nothing this harness installs may appear in `git status` or in a teammate's clone.

```sh
harness/bootstrap ts     ~/dev/target-monorepo
harness/bootstrap dotnet ~/dev/target-monorepo
harness/bootstrap go     ~/dev/target-monorepo
```

Everything lands either outside the repo (`~/.config/coding-standards`, a symlink to this
directory; `~/.config/coding-standards.props`; `~/.config/coding-standards-go-repos`;
`~/.zshenv.local`) or in a path git ignores
(`.git/hooks/pre-commit`, and a `.vscode/settings.json` covered by `.gitignore` or
`.git/info/exclude`). `bootstrap` refuses to touch a tracked file, stages nothing, installs
nothing into the target's `package.json` or lockfile, and prints `git status --short` when it
finishes so you can see it stayed out of the way. The `dotnet` and `go` halves write nothing
inside the repo at all except the hook — the analyzers arrive through an MSBuild property and
the Go rules are compiled into a binary outside it, so there is no editor file to place.

Run it once per language you want in a repo; all three share the config symlink and the hook,
and none undoes the others.

Rerun it any time — it is idempotent, and re-running is how you pick up new rules.

On a clone that has never been bootstrapped, `bootstrap ts` installs the hub's own dependencies
first: `harness/`, and then each `packages/*/` that has no `node_modules` yet. The second half is
not redundant — `harness` reaches the preset through pnpm's `link:` protocol, which symlinks the
package directory without installing *its* dependencies, and there is no workspace root to do it
instead. Without it a fresh clone dies at the smoke check with `ERR_MODULE_NOT_FOUND` on the five
plugins `base.js` imports.

## Pieces

| File | Job |
| --- | --- |
| `eslint-layer.js` | Loads the package's own ESLint config, spreads the personal preset after it. The layering, and the typed-layer gate. |
| `lint-changed.sh` | Lints changed `.ts`/`.tsx` only, each through its own package's ESLint binary. |
| `lint-changed-dotnet.sh` | The C# counterpart: builds the projects owning the changed `.cs` to SARIF, then hands every report to one `lint-changed` run, which blocks the commit on any finding touching a changed line. |
| `errorlog.props` | Sets `ErrorLog` for that build, imported through `CustomAfterMicrosoftCommonTargets`. MSBuild owns the report name because it has to expand `$(TargetFramework)` per inner build and escape the comma before the version suffix; the script passes only the prefix. |
| `lint-changed-go.sh` | The Go counterpart: runs the personal golangci-lint binary over the packages owning the changed `.go`, filters down to those files. |
| `hooks/pre-commit` | Template for the installed hook. The enforcement gate. One template, three branches, each self-gating. |
| `write-vscode-settings.mjs` | The editor half — points the extension at `eslint-layer.js`, so typing sees what committing sees. TypeScript only. |
| `bootstrap` | Installs all of the above into one repo, per language: `ts`, `dotnet` or `go`. |

## Two layers, different jobs

**The pre-commit hook is the gate.** It runs the full personal preset — custom rule, all nine
effect rules, the typescript-eslint / testing-library / jest-dom / jest slices — over exactly
the files the commit will contain. Errors block; warnings do not, on purpose (the rules that
propose restructures ship at `warn`, and a warn that blocks a commit is an error in a hat).

**The editor is feedback, not enforcement** — but it now carries the same rules. It points
`eslint.options.overrideConfigFile` at `eslint-layer.js`, the wrapper the hook already uses,
so the extension loads a config that has composed the package's own rules with the namespaced
preset. The extension then reports the full rule set, including all nine
`react-you-might-not-need-an-effect` rules, which an `overrideConfig`-only layer cannot reach:
it can add rules but cannot merge two configs, and the merging is the whole point.

Three things this depends on, all in `write-vscode-settings.mjs`'s header: `eslint.useFlatConfig`
(the wrapper is flat config, and a package on ESLint 8 has flat off by default), `changeProcessCWD`
working directories (the wrapper finds the package config via `process.cwd()`), and an ESLint
editor integration actually being installed — `bootstrap` warns when it cannot find one, because
nothing else in the pipeline notices.

The difference remains that the editor sees every file while the hook sees only changed ones,
so the editor is where an existing backlog becomes visible. On a mature codebase that is
hundreds of sites for the effect rules alone, which is why the hook scopes to changed files.

## The .NET half

Different mechanism, same two layers. Roslyn analyzers are delivered by an MSBuild property
rather than a config file, so `bootstrap dotnet` sets `CustomAfterMicrosoftCommonProps` to
`~/.config/coding-standards.props` — a generated file carrying one path-scoped `Import` per
adopted repo — and the analyzers reach every project under that path, in builds and in the IDE,
with no file placed in the repo. `dotnet/README.md` covers the mechanism.

Four things about it that are not obvious:

- **Builds are already scoped to changed files.** Roslyn has no diff awareness, so the injection
  generates a per-file global `AnalyzerConfig` before every `CoreCompile` and only the files git
  reports as changed report anything — the same changed-files-only rule the TypeScript half has,
  in builds and in the IDE. `-p:TvrmsmithAnalyzersScopeToChanged=false` shows the whole backlog;
  `dotnet/README.md` covers the mechanism. `lint-changed-dotnet.sh` turns it off and applies its
  own filter, because that one is keyed on what is staged rather than on the working tree.
- **The scoping condition doubles as the registry.** `lint-changed-dotnet.sh` decides whether
  a repo is adopted by looking for its own `StartsWith('<repo>/')` in the props file, which is
  why the hook template needs no per-language state and skips rather than fails in a repo that
  was only bootstrapped for TypeScript.
- **The IDE needs `launchctl`, not a shell profile.** A GUI-launched Rider or VS Code inherits
  from launchd and never reads `~/.zshenv.local`, so `bootstrap dotnet` also runs
  `launchctl setenv` and installs a `RunAtLoad` LaunchAgent so the variable survives a reboot.
- **Linting a changed `.cs` takes two builds.** Analyzers only run when `csc` runs, so an
  up-to-date project reports nothing and the run has to be forced with `--no-incremental`.
  Forcing it with `BuildProjectReferences=false` alone is wrong, though — on a tree whose
  dependencies were never built the compiler reports the whole file as undefined types
  (phantom `CS0246`/`CS0234`). So an ordinary incremental build runs first to make the
  dependencies real, and the forced pass second.

Findings on the .NET side **block the commit when they touch a line the change wrote**. Every id
the injection delivers is still a warning, because no build that succeeded before adoption may
start failing, so the build's own exit status is unchanged. The blocking verdict is the hook's:
the diagnostics pass writes SARIF via the `ErrorLog` set in `errorlog.props`, `lint-changed` keeps
the findings whose locations hold a touched line, and its exit status becomes the script's. Exit 2
is a surviving finding, 1 is anything breaking, in the filter or in the script itself, and the two
stay distinct so a hook can tell them apart. A failed build reports 1 rather than passing MSBuild's
own status through, since MSBuild exiting 2 for its own reasons must not read as a surviving
finding.

Every project's report goes into a single `lint-changed` run, named with a repeatable `--report`.
A process per report spent whatever waiver matched its own report without knowing another report
still blocked the commit, so one process now sees the whole commit's findings and makes one spend
decision. The script writes one report per target framework, so a multi-targeted project reports
the same source-level warning several times; `lint-changed` collapses those to one finding, on the
identity `CONTEXT.md` gives under Finding.

A diagnostic the target repo already turned off with a `#pragma` or a `[SuppressMessage]` is
dropped before scoping. `ErrorLog` reports those where the console never printed them, and that
repo's decision stands.

Compile errors still block, as on the TypeScript side.

Five consequences worth knowing before you hit them. A staged file whose disk copy differs is now
a hard stop rather than a printed caveat, because MSBuild compiles disk while the commit carries
the index, and failing a commit over code it does not contain would be worse than refusing to
guess. That covers a staged `.cs` deleted from the working tree, so a change naming C# reaches
`lint-changed` even when there was nothing left to build. Analyzer props the registry says should
be there and are not is the second, since a build with no analyzers loaded reports clean on a
compilation nothing inspected; run `harness/bootstrap dotnet <repo>` again. A build that writes no SARIF at all is a hard stop for the same reason, since a clean build
still writes an empty report and a missing one means `ErrorLog` never took effect. A report
whose results all name files outside the repo is the fourth, and `lint-changed` names each dropped
rule and URI; each report answers that question alone, so one project placing nothing still stops
the commit when another placed results. Only a URI outside the repo counts, because only that says
the report describes another tree. Roslyn writes CS1701, CS8021 and the command-line CS2xxx
warnings at no location at all, reports a whole-document diagnostic with no region, and names
generated documents that were never written to disk; those results are dropped and the commit goes
on. A report carrying no results at all is still a clean pass. And the one route past a false
positive is a waiver, one rule on one path, used once, with a reason, recorded in a log outside the
repo; `lint-changed` prints the exact command. A waiver is spent only on a run that ends clean, so a
waived finding on a commit that blocked on something else costs nothing.

The C# hook now needs Go on `PATH`, even in a repo with no Go in it. The script builds
`lint-changed` from this hub into `${XDG_CACHE_HOME:-~/.cache}/coding-standards` on every run,
which Go's build cache makes free after the first. No Go means the filter cannot run, and an
unrun filter proves nothing, so the script fails the commit rather than skipping.

ADR 0010 carries the rule and the reasoning.

## The Go half

One binary and one config file, both outside the target. `go/build.sh` compiles the custom
analyzers into a golangci-lint binary, `bootstrap go` builds it and `lint-changed-go.sh` runs it
with `--config` pointed at the hub's `go/golangci.yml`, so a repo with its own `.golangci.yml`
keeps it untouched — the personal layer is a separate run, not a merge.

Three things about it differ from the other two:

- **A registry file decides which repos are adopted.** `~/.config/coding-standards-go-repos`
  holds one resolved repo path per line. The other halves can read adoption off something that
  already exists — an ESLint config in the package, a path-scoped `Import` in the props file —
  but the Go side installs nothing in the target and its binary is machine-wide, so without the
  registry, bootstrapping one repo would silently start linting every other repo the hook
  guards. The line is the **main checkout**, even when `bootstrap go` is pointed at a linked
  worktree, and `lint-changed-go.sh` resolves the same way before it looks itself up. Both use
  `git rev-parse --git-common-dir`, whose parent is the main checkout in either case.
  `test/lint-changed-go.test.js` pins all four combinations of registered/not and
  worktree/not, because a skip that should have been a run is silent and looks exactly like a
  repo with no findings.
- **Findings report and never block**, which the .NET half no longer does.
  `--issues-exit-code=0` makes it explicit, and it also means a non-zero exit is unambiguous:
  the run itself broke. This is the last half in that position, not a settled convention. Go
  moves to the .NET arrangement in the third slice of
  [issue 108](https://github.com/tvrmsmith/coding-standards/issues/108), which drops that flag
  and pipes golangci-lint's JSON through `lint-changed`.
- **There is no editor half yet.** The hook is the whole gate.

`bootstrap go` is also the one mode that accepts the hub itself as its target. The other two
would be pointing the hub's own tooling at the hub, which is a mistake every time; the Go one is
not, because the hub carries real Go code of its own in `gate/` and `internal/`.

## Worktrees

Hooks and `info/exclude` live in the **common** git dir, so a linked worktree shares them with
the main checkout: bootstrap one worktree and the hook guards commits from all of them, on any
branch. `.vscode/settings.json` is per-checkout, so run `bootstrap ts <worktree>` once in each
worktree you actually open in an editor — it is cheap and idempotent, and the hook install is a
no-op after the first.

The Go registry follows the same rule from the other direction: it is keyed on the main
checkout, so one `bootstrap go` covers every worktree the shared hook fires in. See
[The Go half](#the-go-half).

Two things this depends on, both verified against a linked worktree: paths are resolved
with `git rev-parse --git-path` rather than `$repo/.git/…` (in a worktree `.git` is a *file*,
so the naive path is `not a directory`), and the hook carries a marker line so a reinstall
recognises it instead of shuffling it aside and chaining to itself.

## Escape hatches

Past a genuine false positive on the .NET side, record a waiver rather than skipping the hook.
`lint-changed` prints the exact command for each finding that blocked, and it prints the binary by
full path, since the hook builds it into the cache directory and never puts it on `PATH`:

```sh
/path/to/lint-changed waive --language <lang> --path <path> --rule <rule> --reason <why>
```

`--path` is the one flag the printed command may leave out. An analyzer load failure is reported at
no location at all, so its waiver keys on the language and the rule alone.

The log is `${XDG_STATE_HOME:-~/.local/state}/coding-standards/waivers.jsonl`, and
`TVRMSMITH_WAIVERS` overrides it. State rather than config, because `~/.config/coding-standards` is
the symlink to this hub, and an audit log must not land inside a repo the gate guards.
`lint-changed waivers` lists every record with its spend state.

The two skips below defeat every check at once, which is why the hook no longer offers them:

```sh
SKIP_TVRMSMITH_LINT=1 git commit …   # skip the personal hook, keep any chained one
git commit --no-verify               # skip all hooks
TVRMSMITH_ESLINT_DEBUG=1 …           # print which branch of the wrapper ran, and the typed decision
TVRMSMITH_TYPED_LINT=0 …             # force the type-aware layer off for this run
TVRMSMITH_TYPED_LINT=1 …             # force it on
~/.config/coding-standards/lint-changed.sh --since main
```

## The typed layer is decided per package

The preset's type-aware rules throw where the package has no TypeScript program, so the wrapper
adds them only where the package's own config already sets `parserOptions.projectService`,
`parserOptions.project`, or the older `EXPERIMENTAL_useProjectService`. A repo becomes ready by
configuring typed linting for itself, which is work it wants anyway; nothing here needs touching.
`TVRMSMITH_TYPED_LINT` overrides the detector in either direction, which is how you try the layer
on a package before committing to it.

## Namespacing, and one cosmetic effect

The preset's plugin namespaces are renamed to `tvrmsmith-*` because a package registering
`@typescript-eslint` from its own `node_modules` while the preset registers the hub's copy is a
hard `Cannot redefine plugin`. The clash is guaranteed rather than incidental, so the rename is
unconditional. It reads well as a side effect: `tvrmsmith-testing-library/no-container` is
visibly the personal layer, `testing-library/no-container` is the package's own config.

The cost is that in a package which declares `testing-library` or `jest-dom` itself, a finding
both configs agree on is reported twice, once per namespace.
