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

**Rerun it in every adopted repo after pulling this hub.** The installed hook is a copy of
`hooks/pre-commit` taken at install time, not a pointer to it, so a change to the template reaches
nothing already installed. A hook installed before the single-entrypoint consolidation calls
`lint-changed-dotnet.sh` and `lint-changed-go.sh`, which no longer exist, and every commit in that
repo fails with `No such file or directory` until `bootstrap` is run again. Reinstalling is the
fix; there are no forwarding shims.

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
| `lint-changed.sh` | The one entrypoint. Resolves the repo, computes the changed set, runs every language branch that applies, then runs one `lint-changed` over every report the branches produced and, on a full `--staged` run, spends the waivers it matched once the whole run is clean. `--only` runs a single branch. |
| `linters/common.sh` | What every branch shares: argument parsing, repo resolution with the registry key from `lint-changed registry-key`, the changed set from `lint-changed changed-paths`, the scratch directory, the ancestor walk each language owns a predicate for, `lint_changed_bin`, which builds the filter once per run and memoises the path to a file, `add_reports`, the call every branch ends on to hand its reports up, `run_filter`, the one `lint-changed` call that reads them, `spend_waivers`, and `rank_status`, the one place the ADR 0010 exit convention is folded. |
| `linters/ts.sh` | Lints the changed JavaScript and TypeScript, each file through its own package's ESLint binary, to a JSON report, then hands every report up to the dispatcher's one `lint-changed` run, which blocks the commit on any finding touching a changed line. |
| `linters/dotnet.sh` | The C# counterpart: builds the projects owning the changed `.cs` to SARIF, then hands every report up to the dispatcher's one `lint-changed` run, which blocks the commit on any finding touching a changed line. |
| `errorlog.props` | Sets `ErrorLog` for that build, imported through `CustomAfterMicrosoftCommonTargets`. MSBuild owns the report name because it has to expand `$(TargetFramework)` per inner build and escape the comma before the version suffix; the branch passes only the prefix. |
| `linters/go.sh` | The Go counterpart: runs the personal golangci-lint binary over the packages owning the changed `.go` to a JSON report per module, then hands every report up to the dispatcher's one `lint-changed` run, which blocks the commit on any finding touching a changed line. |
| `hooks/pre-commit` | Template for the installed hook. The enforcement gate. One template, one `lint-changed.sh --staged` call, whatever the repo is adopted for. |
| `write-vscode-settings.mjs` | The editor half — points the extension at `eslint-layer.js`, so typing sees what committing sees. TypeScript only. |
| `bootstrap` | Installs all of the above into one repo, per language: `ts`, `dotnet` or `go`. |

## `--only`: one branch at a time

`lint-changed.sh` runs every branch that applies, which is what the hook wants. `--only ts`,
`--only go` or `--only dotnet` narrows it to one, for a caller that wants a single language's
findings and a single language's exit code. An unknown name is rejected rather than quietly
linting nothing. The flag must come before `--files`, which swallows everything after it.

Exit codes are one convention across the three, ADR 0010's. **2 is a surviving finding**, an
ESLint warning, a C# analyzer warning or a golangci-lint issue on a line the change wrote, and
**1 is the gate breaking**: a failed build, a golangci-lint run that blew up, a missing layering
wrapper, a bad argument. A branch returns only 0 or 1, and the 2 comes from the one filter run
over every branch's reports. A 1 from any branch dominates the filter's 2, because a branch that
never ran says nothing about the code it never read. The findings the filter did read still reach
stdout.

A filter run that pairs an unreadable report with a surviving finding in another report exits 2,
not 1 as it used to. The run reads every report before it answers, so one it cannot read no longer
hides what another found. The unreadable report is still named on stderr, and no waiver is spent on
that run.

All three branches follow the whole of ADR 0010, not just its exit codes. Each runs its linter
into a machine-readable report and hands every report up. After the last branch, `lint-changed.sh`
runs one `lint-changed` process over all of them, in branch order, which keeps the findings
touching a line the change wrote, applies any waiver and sets the status. Severity does not tier:
an ESLint severity-1 warning blocks exactly as a severity-2 error does.

One process for the whole run, not one per branch, because a waiver is one commit's worth of
permission. With a filter per branch, a commit touching Go and TypeScript spent the Go waiver on
Go's clean share, then TypeScript blocked the commit, and the waiver was gone with no commit behind
it. The filter itself never spends: it writes the id of every waiver it matched to a file, and
`lint-changed.sh` spends them only when the total status is 0, so a branch that broke or a finding
that survived anywhere in the run leaves every waiver unspent. It spends only on a full `--staged`
run, the pre-commit hook's: a `--since` or `--files` run makes no commit, and an `--only` run lints
one language of one, so those runs pass a waived finding and leave the waiver unspent.

A non-spending run also accepts a waiver already spent against any tree, not only this one, so a
no-mistakes run relinting a rebased or fixed-up tree does not block on a finding its commit already
went through on. The full `--staged` run keeps the strict rule, unspent or spent against this same
tree, since the pre-commit hook is the only guard a new commit gets.

Under `--staged` that process runs even when no branch produced a report, as long as at least one
branch reached the point of handing its reports up. ADR 0010's staged-versus-disk hard stop lives in
`lint-changed` and covers every staged path, so skipping the filter for want of a report is how a
commit whose staged files were all deleted from the working tree used to go through unexamined.
When every branch skipped, as in a repo adopted for none of the languages in the change, no filter
runs and the commit goes through, as it always did.

So every run needs Go on `PATH`, even in a repo with no Go in it. `lint-changed changed-paths` also
computes the changed set each run starts from. `linters/common.sh` builds `lint-changed` from this
hub into `${XDG_CACHE_HOME:-~/.cache}/coding-standards` once per run, which Go's build cache makes
free after the first. No Go means neither the changed set nor the filter can run, and an unexamined
change proves nothing, so the run fails the commit rather than skipping.

The changed set is the files the change adds, copies or modifies. A rename counts as a delete plus
an add, so an edit made while renaming reaches its linter. `--since <ref>` diffs the working tree
against the merge base of `HEAD` and the ref, the base the filter scopes findings against.

During an uncommitted merge, when `MERGE_HEAD` exists, `--staged` lints only the staged files that
differ from both `HEAD` and `MERGE_HEAD`: conflict resolutions and edits made while merging. Against
`HEAD` alone a merge commit's index holds everything the incoming side brought, which on a
long-lived branch meant thousands of files and dozens of .NET builds per commit. Against
`MERGE_HEAD` alone it holds everything the branch already committed, whose lines all match `HEAD`,
so the filter would drop every finding in them after paying for the builds. The staged-versus-disk
hard stop still covers every staged path.

Output is one shape across all three, whatever the linter's own format was. `lint-changed` writes
one line per surviving finding to stdout, `path:line:column: RULE: message`, repo-relative, and
nothing else; every header, diagnostic and waive command goes to stderr. One line per surviving
finding is what lets one regex read all three languages, which is what no-mistakes'
`lint.extra_linters` consumes.

### Linting a checkout that cannot say which repository it is

`TVRMSMITH_REGISTRY_KEY` names the checkout directly. The Go and .NET branches decide whether a
repository is adopted by looking its path up, in the Go registry file and in the props file's
scoping condition, and they derive that path from `git rev-parse --git-common-dir`. A caller
linting in a detached worktree of some other bare repository gets the wrong derivation, so the
lookup misses and the branch skips. A skip is silent and looks exactly like a clean result, which
is the worst way for this to fail.

The variable overrides which path is looked up, not whether the lookup happens: an override
naming a repository in neither registry still skips. A value naming no directory at all is a
different thing and fails the run, since it could only ever produce a lookup that misses, and a
typo would otherwise turn the whole gate into a silent no-op. The TypeScript branch needs no such thing,
it keys on nothing. It does need `node_modules` to exist, because it runs the target repo's own
ESLint and installs nothing. A package carrying an ESLint config but no installed `eslint` binary
fails the run with a 1 rather than skipping, since every changed file in it would otherwise reach
no linter and the commit would pass on a clean answer nothing produced.

## Two layers, different jobs

**The pre-commit hook is the gate.** It runs the full personal preset — custom rule, all nine
effect rules, the typescript-eslint / testing-library / jest-dom / jest slices — over exactly
the files the commit will contain. Severity does not tier: a warning blocks exactly as an error
does, once the finding touches a line the change wrote (ADR 0010). The rules that propose a
restructure still ship at `warn`, which now decides only how an editor paints them and how loud a
whole-repo run is, never whether a commit stops.

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
  `dotnet/README.md` covers the mechanism. `linters/dotnet.sh` turns it off and applies its
  own filter, because that one is keyed on what is staged rather than on the working tree.
- **The scoping condition doubles as the registry.** `linters/dotnet.sh` decides whether
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

Every project's report goes into the dispatcher's single `lint-changed` run, beside every other
branch's, named with a repeatable `--report` under a sticky `--format`. The script writes one
report per target framework, so a multi-targeted project reports
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
compilation nothing inspected; run `harness/bootstrap dotnet <repo>` again. A build that writes no
SARIF at all is the third, for the same reason, since a clean build still writes an empty report
and a missing one means `ErrorLog` never took effect. A report
whose results all name files outside the repo is the fourth, and `lint-changed` names each dropped
rule and URI; each report answers that question alone, so one project placing nothing still stops
the commit when another placed results. Only a URI outside the repo counts, because only that says
the report describes another tree. Roslyn writes CS1701, CS8021 and the command-line CS2xxx
warnings at no location at all, reports a whole-document diagnostic with no region, and names
generated documents that were never written to disk; those results are dropped and the commit goes
on. A report carrying no results at all is still a clean pass. And the one route past a false
positive is a waiver, one rule on one path, used once, with a reason, recorded in a log outside the
repo; `lint-changed` prints the exact command. A waiver is spent only on a run that ends clean, in
every language the commit touches, so a waived finding on a commit that blocked on something else
costs nothing.

This branch needs Go on `PATH` like the other two, for the shared `lint-changed` build described
above. A missing `dotnet` in an adopted repo fails the same way and for the same reason, since
nothing compiled the changed C# and nothing inspected it. The PATH check sits below the adoption
check, so a repo wired only for TypeScript or Go still passes the branch without needing the .NET
SDK.

ADR 0010 carries the rule and the reasoning.

## The Go half

One binary and one config file, both outside the target. `go/build.sh` compiles the custom
analyzers into a golangci-lint binary, `bootstrap go` builds it and `linters/go.sh` runs it
with `--config` pointed at the hub's `go/golangci.yml`, so a repo with its own `.golangci.yml`
keeps it untouched — the personal layer is a separate run, not a merge.

Three things about it differ from the other two:

- **A registry file decides which repos are adopted.** `~/.config/coding-standards-go-repos`
  holds one resolved repo path per line. The other halves can read adoption off something that
  already exists — an ESLint config in the package, a path-scoped `Import` in the props file —
  but the Go side installs nothing in the target and its binary is machine-wide, so without the
  registry, bootstrapping one repo would silently start linting every other repo the hook
  guards. The line is the **main checkout**, even when `bootstrap go` is pointed at a linked
  worktree, and `linters/go.sh` resolves the same way before it looks itself up. Both use
  `git rev-parse --git-common-dir`, whose parent is the main checkout in either case.
  `test/lint-changed-go.test.js` pins all four combinations of registered/not and
  worktree/not, because a skip that should have been a run is silent and looks exactly like a
  repo with no findings.
- **A finding blocks when it touches a changed line**, the same arrangement as the .NET half.
  `--issues-exit-code=0` stays, and now means golangci-lint's own exit code is reserved for one
  thing: the run itself broke. The verdict over a finding belongs to `lint-changed`, which reads
  the JSON report golangci-lint writes with `--output.json.path`. A package that fails to compile
  arrives as a single `typecheck` issue at line 1, so that rule ignores scope rather than sailing
  through on an untouched first line.
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

The .NET half is keyed on the main checkout too, and getting there took one extra move. Its
registry is the path-scoped `<Import>` in `~/.config/coding-standards.props`, and that same
`StartsWith('<repo>/')` condition is what MSBuild tests against `MSBuildProjectDirectory` when it
decides whether to load the analyzers. A worktree outside the main checkout's path fails the test
inside MSBuild no matter what the hook believes, so resolving the registry lookup alone would have
fired the hook where no analyzer loads and reported nothing, which reads as a clean tree. So
`linters/dotnet.sh` imports the analyzer props directly rather than through the scoped
wrapper: adoption is already settled by the registry check above it, and the condition has no
second job to do. Ambient IDE and CLI builds still go through the wrapper, so **they** reach the
main checkout only, and `bootstrap dotnet <worktree>` does not change that: it registers the
worktree's main checkout, so no worktree path ever enters the condition. Commits from a worktree
are linted; typing in one is not.

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

`lint-changed.sh` is the only thing that spends. A `lint-changed` filter run invoked directly
reports each waiver it matched on stderr and leaves it unspent, however clean the run, because a
direct run is not a commit. The dispatcher spends through `lint-changed spend --waiver <id>`, only
on a full `--staged` run with no `--only`, and only after every branch and the filter came back
clean.

The two skips below defeat every check at once, which is why the hook no longer offers them:

```sh
SKIP_TVRMSMITH_LINT=1 git commit …   # skip the personal hook, keep any chained one
git commit --no-verify               # skip all hooks
TVRMSMITH_ESLINT_DEBUG=1 …           # print which branch of the wrapper ran, and the typed decision
TVRMSMITH_TYPED_LINT=0 …             # force the type-aware layer off for this run
TVRMSMITH_TYPED_LINT=1 …             # force it on
TVRMSMITH_REGISTRY_KEY=/path/to/repo # answer the adoption question for a caller the lookup gets wrong
~/.config/coding-standards/lint-changed.sh --since main
```

`TVRMSMITH_REGISTRY_KEY` sets the key the `go` and `dotnet` scripts look up, and only a caller that
already knows the answer should set it. Without it, `lint-changed registry-key` derives the key
from the parent of `git rev-parse --git-common-dir`, which is the main checkout for an ordinary
worktree and the wrong directory entirely for a checkout git does not think is related to the adopted one. The no-mistakes pipeline
is that caller: it lints in a worktree of a bare repository it keeps under `~/.no-mistakes`, so the
derived key named that bare repo, missed the registry and skipped every run in silence. It passes
the registered checkout instead, in `NO_MISTAKES_REPO_PATH`.

It moves which path is looked up. It does not bypass the lookup, so an override naming a path
nobody adopted still skips, and findings still come from the tree the script is run in.

Nothing here is a wiring guide, and the no-mistakes side of it is not released. Configuring that
integration needs a build supporting `lint.extra_linters`; on one without it, a `lint:` block in
`~/.no-mistakes/config.yaml` does not get ignored, it fails the daemon on every command, because
global config rejects unknown keys.

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
