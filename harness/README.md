# Local adoption harness

Wire a repository you work in up to the personal coding standards **without anyone else
seeing it**. Adoption is solo by design: the target repo may be shared, the standards are not,
so nothing this harness installs may appear in `git status` or in a teammate's clone.

```sh
harness/bootstrap ts     ~/dev/target-monorepo
harness/bootstrap dotnet ~/dev/target-monorepo
```

Everything lands either outside the repo (`~/.config/coding-standards`, a symlink to this
directory; `~/.config/coding-standards.props`; `~/.zshenv.local`) or in a path git ignores
(`.git/hooks/pre-commit`, and a `.vscode/settings.json` covered by `.gitignore` or
`.git/info/exclude`). `bootstrap` refuses to touch a tracked file, stages nothing, installs
nothing into the target's `package.json` or lockfile, and prints `git status --short` when it
finishes so you can see it stayed out of the way. The `dotnet` half writes nothing inside the
repo at all except the hook — the analyzers arrive through an MSBuild property, so there is no
editor file to place.

Run it once per language you want in a repo; the two share the config symlink and the hook, and
neither undoes the other.

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
| `eslint-layer.js` | Loads the package's own ESLint config, spreads the personal preset after it. The layering. |
| `lint-changed.sh` | Lints changed `.ts`/`.tsx` only, each through its own package's ESLint binary. |
| `lint-changed-dotnet.sh` | The C# counterpart: builds the projects owning the changed `.cs`, filters the diagnostics down to those files. |
| `hooks/pre-commit` | Template for the installed hook. The enforcement gate. One template, two branches, each self-gating. |
| `write-vscode-settings.mjs` | The editor half — points the extension at `eslint-layer.js`, so typing sees what committing sees. TypeScript only. |
| `bootstrap` | Installs all of the above into one repo, per language: `ts` or `dotnet`. |

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

Findings on the .NET side **report and never block**: every id the injection delivers is a
warning by design, because no build that succeeded before adoption may start failing. Compile
errors still block, as on the TypeScript side.

## Worktrees

Hooks and `info/exclude` live in the **common** git dir, so a linked worktree shares them with
the main checkout: bootstrap one worktree and the hook guards commits from all of them, on any
branch. `.vscode/settings.json` is per-checkout, so run `bootstrap ts <worktree>` once in each
worktree you actually open in an editor — it is cheap and idempotent, and the hook install is a
no-op after the first.

Two things this depends on, both verified against a linked worktree: paths are resolved
with `git rev-parse --git-path` rather than `$repo/.git/…` (in a worktree `.git` is a *file*,
so the naive path is `not a directory`), and the hook carries a marker line so a reinstall
recognises it instead of shuffling it aside and chaining to itself.

## Escape hatches

```sh
SKIP_TVRMSMITH_LINT=1 git commit …   # skip the personal hook, keep any chained one
git commit --no-verify               # skip all hooks
TVRMSMITH_ESLINT_DEBUG=1 …           # print which branch of the wrapper ran
~/.config/coding-standards/lint-changed.sh --since main
```

## Namespacing, and one cosmetic effect

The preset's plugin namespaces are renamed to `tvrmsmith-*` because a package registering
`@typescript-eslint` from its own `node_modules` while the preset registers the hub's copy is a
hard `Cannot redefine plugin`. The clash is guaranteed rather than incidental, so the rename is
unconditional. It reads well as a side effect: `tvrmsmith-testing-library/no-container` is
visibly the personal layer, `testing-library/no-container` is the package's own config.

The cost is that in a package which declares `testing-library` or `jest-dom` itself, a finding
both configs agree on is reported twice, once per namespace.
