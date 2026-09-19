# go/

The Go half of the lint layer: the custom analyzers, and the curated golangci-lint config they
are enabled from.

Not to be confused with `gate/`, which is also written in Go. That one is the metric gate, a
different mode; this directory is about Go *as a linted language*.

The Claude Code plugin loader ignores this directory.

## Layout

```
.custom-gcl.yml                      # the pinned golangci-lint version and the plugin source
golangci.yml                         # the curated config, passed to the target with --config
build.sh                             # builds bin/tvrmsmith-gcl, then proves the plugin is in it
plugin/
  go.mod
  plugin.go                          # one register.Plugin call per rule
  commentblocklength/
    analyzer.go                      # the Go half of comment-block-length
    analyzer_test.go
    testdata/src/blocks/blocks.go    # the shapes it must catch, and the four it must not
  combineassertions/
    analyzer.go                      # the Go half of combine-assertions-on-same-object
    analyzer_test.go
    testdata/src/                    # the fixtures, and a stand-in testify to resolve against
test/
  cases_test.go                      # every enabled linter must report in fixtures/
  fixtures/                          # one deliberate violation per enabled linter
  smoke/                             # its own module: one violation, for build.sh and bootstrap
bin/                                 # generated; gitignored
```

## One binary, not a plugin to install

golangci-lint has two plugin systems, and only one of them is a real option. A Go plugin
(`.so`) has to be built with the exact toolchain and dependency versions of the golangci-lint
binary that loads it, and fails at load time when they differ. The **module** system compiles
the plugin into golangci-lint itself, so `build.sh` produces one binary that *is* the linter
plus the personal rules, and the target repository has nothing to install and nothing to
resolve.

The cost is the version pin in `.custom-gcl.yml`. The plugin links against golangci-lint's
internals, so the binary and the rules are built together against one version; raising it is an
edit to that file and a rerun of `build.sh`. The Go build cache makes the rerun cheap.

Nothing in an ordinary run notices when that pin ages, so
[`.github/workflows/golangci-pin.yml`](../.github/workflows/golangci-pin.yml) compares it against
the latest release once a week and files an issue when it is behind. It runs on its own schedule
rather than inside CI because an upstream release is not a reason for a pull request to go red.

`build.sh` needs some golangci-lint on `PATH` to invoke its `custom` subcommand, but which one
does not matter: `custom` builds the version it is told to, not its own.

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
./build.sh
```

The build is verified rather than trusted. A plugin that failed to register leaves a perfectly
good binary that silently enables nothing, so `build.sh` finishes by running the binary over
`test/smoke/`, which holds one deliberate violation, and fails if nothing is reported.
`help linters` cannot answer the same question: it takes no `--config`, and a custom linter does
not exist until a config declares it.

## The custom analyzers

| Linter name | Rule | Guideline | Flags |
|---|---|---|---|
| `tvrmsmith-comment-block-length` | `comment-block-length` | Comments | a run of non-documentation comments over 10 lines |
| `tvrmsmith-combine-assertions` | `combine-assertions-on-same-object` | A1 | consecutive testify assertions picking one object apart, and a length assertion followed by indexing into the same collection |

One `register.Plugin` call per rule rather than one plugin holding every analyzer: the registered
name *is* the linter name in the config, so a name per rule is what lets each be enabled,
disabled and configured on its own.

### combine-assertions-on-same-object

A1 is the one guideline no off-the-shelf rule covers in any of the three languages, so this is
the third hand-written half, beside the ESLint rule and Roslyn's `TVRM0001`.

An assertion is recognised by the callee's package as the type checker resolved it, not by an
`assert.` prefix in the source, which is what covers the package functions, the
`*assert.Assertions` methods and a suite's own `s.Equal` with one check. That last one is the
same method reached through the embedded `*assert.Assertions`, so it needs no special case. The
cost is that this rule runs in `LoadModeTypesInfo` where `comment-block-length` runs in
`LoadModeSyntax`.

Three scoping decisions, each one a false positive not taken:

- **A run ends at any other statement.** The C# half groups across an intervening statement;
  this one does not. A run broken by real work is two arrangements, and proposing one combined
  assertion over it would change what the test does.
- **The subject has to be a field selection.** `assert.NoError(t, err)` and
  `assert.Equal(t, want, got)` are already whole-value assertions, and a chain through a call
  (`load().Title`, then `load().Size`) is two calls rather than one object.
- **The root is a `types.Object`, not a name.** Two assertions on the same spelling in different
  scopes are not a group.

Which argument holds the value under test depends on the function: testify's expected-first
family (`Equal`, `Exactly`, `InDelta` and the rest) puts it second, everything else puts it
first. A function missing from that list costs a report rather than causing a wrong one, because
a group also needs the assertion beside it to reach into the same object.

`comment-block-length` is the third implementation of one rule — the ESLint rule and the Roslyn
analyzer `TVRM0006` are the other two — and the
[shared doc](../packages/eslint-plugin-tvrmsmith/docs/rules/comment-block-length.md) covers the
guideline, the threshold and why there is no autofix. Two things are specific to Go:

- **A doc comment is structural here, not syntactic.** The other two languages recognise a doc
  comment by its delimiter; Go uses `//` for everything, so the exemption is the group the parser
  attached to a declaration. The consequence is that prose sitting immediately above a `var`,
  `const` or `type` *inside a function body* is that declaration's doc comment and is exempt,
  where the other two halves would measure it.
- **Everything above the `package` clause is exempt too.** That region holds licence headers and
  `//go:build` constraints, neither of which justifies code, and only the group the parser
  attached as the package doc would otherwise escape.

The "a doc comment ends the run beside it" rule the other two halves need has no Go equivalent:
contiguous comment lines above a declaration are one group, which the parser attaches whole, so
the shape it guards against cannot be written.

## The curated config

`golangci.yml` sets `linters.default: none` and names what it enables. A rule is there because
it was chosen, never because a preset happened to include it — the same one-directional mapping
the TypeScript preset and the C# severities follow: every guideline needs an enforcement home,
a rule does not have to trace back to a guideline.

The config is never copied into the target repository. It is passed with `--config`, so a repo
that has its own `.golangci.yml` keeps it: the personal layer is a separate run, not a merge.

### Mapped to a guideline

| Linter | Guideline | Why it is here |
|---|---|---|
| `tvrmsmith-combine-assertions` | A1 | Above. |
| `testifylint` | A2, A5 | Every checker on. Most of A2 in one linter: `empty`, `len`, `compares`, `error-is-as`, `bool-compare` and the rest each replace an assertion that reports "false != true" with one that says what was wrong. `require-error` and `go-require` are A5: an `assert` on a fatal condition lets the test carry on into a nil dereference, and `require` inside a goroutine kills the wrong stack and passes. |
| `errcheck` | Errors, unchecked result | An ignored error is the swallowed exception with no `catch` to write. |
| `errorlint` | Errors, lost context | `%v` instead of `%w` severs the chain, so `errors.Is` at the top cannot see the cause. |
| `nilerr` | Errors, fallback that masks failure | Checks the error and reports success anyway. |
| `nilnesserr` | Errors, fallback that masks failure | The same, one step subtler: returns a *different* error variable that this branch already proved is nil. |
| `bodyclose`, `sqlclosecheck`, `rowserrcheck` | Errors | A leaked handle is a silent failure that surfaces as exhaustion somewhere else entirely, in a request that did nothing wrong. `rowserrcheck` is the unchecked result again: iteration that stopped because of an error looks exactly like iteration that finished. |
| `nolintlint` | Errors, swallowed exception | A bare `//nolint` turns a linter off with no reason stated, which is the shape the Errors guideline is about, applied to the linter. `allow-unused` is on because the directives in the target repo point at its own linter set, not this one. |
| `tvrmsmith-comment-block-length` | Comments | Above. |

### Unmapped, and that is fine

The mapping runs one way. These are on because they are right, not because prose was written for
them first: `govet`, `staticcheck` (SA, S1 and QF1, not the ST naming family), `unused`,
`ineffassign`, `durationcheck`, `makezero`, `predeclared`, `gocritic` (the diagnostic tag only),
`loggercheck`, `gosec`, and the test-integrity three, `thelper`, `tparallel` and `usetesting`.

`gosec` is net-new the way `jsx-a11y` was net-new to the TypeScript preset: no other layer here
covers security, and the code this runs over is healthcare software. Two rules are off. `G104`
is `errcheck` with a different id, and `G115` flags every int conversion that could in principle
overflow, which on real code is hundreds of sites bounded by something the linter cannot see.

`gosec` runs in `_test.go` too, and this config carries no exclusion rules at all.
golangci-lint's own exclusion presets are rejected because a guideline holds in a test as much as
in production code, and an exclusion written here would hold for every repository this preset
visits, not just this one. A rule worth keeping is worth answering at the site.

That is a reversal, and the argument it reverses was a good one. `gosec` models an attacker
reaching the input, a test supplies every input itself, and the loudest rules fire hardest on
ordinary test shapes: `G204` on a subprocess built from a variable, which is every test running a
binary it just compiled; `G304` on a read from a variable path, which is every test reading its
own `t.TempDir` fixture; `G306` and `G301` on a file the test writes and deletes in one function.
42 of the 45 findings then in the repo sat in `_test.go` and none named a real weakness.

What settled it was where the exclusion lands. This preset is the product, and it runs over other
people's repositories, so `path: _test\.go` disarms `gosec` in all of them to spare this one some
typing. The per-site answer costs more and says more: each directive names why that call is safe,
and a site whose mode was merely convenient gets tightened instead, which the sweep found for
fifteen of them. Where the rule is answered rather than fixed, the reason has to be specific
enough that it could not be pasted onto another site.

`allow-unused: true` is set on `nolintlint`, so a directive that stops suppressing anything is not
reported. That is deliberate, because a target repository's own live directives all read as dead
under this preset, but it means a `gosec` exclusion added later would silently hollow out every
directive rather than red the build. `TestThePresetStillReportsTheRulesThisRepoJustified` exists
to catch that.

### Deliberately not enabled

Written down because "we looked and said no" and "we never looked" are otherwise the same file.

| Not enabled | Why |
|---|---|
| `gocyclo`, `gocognit`, `cyclop`, `funlen`, `nestif`, `maintidx` | Complexity is the metric gate's job. It scores changed methods against a CRAP threshold that accounts for their test coverage, and a second threshold in a second tool would disagree with it on the same method. |
| `revive`, the `ST` half of `staticcheck`, `godot`, `whitespace`, `wsl`, `nlreturn`, `lll`, `varnamelen`, `tagalign` | Layout is `gofmt`'s and naming is the skill's. A rule that reports every file of a repo this layer is only visiting teaches you to ignore the run. |
| `dupl` | Duplicated Code is a smell, so it is a judgement in context. A token-count threshold reports table-driven tests. |
| `wrapcheck`, `err113` | Both make the repo adopt a wrapping policy. Which errors are sentinels and where they get wrapped is the target repo's design, not this layer's. |
| `paralleltest` | Demands `t.Parallel()` in every test. `tparallel` catches the actual bug, a test that is parallel in one half and not the other. |
| `testpackage`, `ireturn`, `nonamedreturns`, `gochecknoglobals`, `gochecknoinits`, `exhaustruct`, `mnd` | House-style bets, each with a real argument on both sides. |
| `exhaustive` | Needs to know whether a `default` case means the switch is exhaustive, which is a per-repo convention. |
| `forcetypeassert` | See A4 below. |
| `depguard`, `importas`, `goheader` | Per-repo policy with nothing to configure here. |
| `copyloopvar`, `intrange` | Obsolete: Go 1.22 fixed the loop variable and added `range` over int. |

### A4 has no Go half

A4, "don't suppress null or missing value failures", is review-only in Go, and this is the
measured answer rather than a gap.

The other two halves exist because their language offers a way to turn a missing value into a
*passing* assertion: TypeScript bans `?.` and `!` before an `expect`, and `TVRM0002` bans the
same two in C#. Go has neither. A nil dereference and a failed type assertion both panic, which
fails the test loudly, so the shape the guideline is about cannot be written that way.

The nearest real silencing is `v, _ := f()`, discarding an error and then asserting on `v`.
`errcheck`'s `check-blank` finds it, and it is off here on purpose: `_ = f()` is also how Go
says "considered and ignored" in production code, the setting cannot tell the two apart, and
turning it on repo-wide would report mostly the accepted idiom. `forcetypeassert` is off for the
same kind of reason: it flags `x.(T)` everywhere rather than before an assertion, and in a test
the unchecked assertion panics anyway.

What is left of A4 in Go is covered by `testifylint`'s `require-error`, which is the case where
an error *was* checked but with `assert`, so the test continues into the value that error made
meaningless.

## Severity, and what blocks

In an adopted repo every rule here is advisory. `harness/lint-changed-go.sh` passes
`--issues-exit-code=0` and reports the findings without failing, so a finding never blocks a
commit. A non-zero exit after that flag means the *run* broke, a tree that does not typecheck or
an unreadable config, and that does fail.

The C# half no longer sits here. Its findings block the commit when they touch a changed line
(ADR 0010), and Go joins it in the third slice of
[issue 108](https://github.com/tvrmsmith/coding-standards/issues/108). The injected Roslyn ids
still ship at `Warning`, for the reason this config's rules are advisory today, because this
runs machine-locally over code other people wrote and are not being asked to change.

The hub's own root module is the exception, because here the code is ours to change. The
`gate (go)` job in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs this same binary
and config from the repository root over `./...`, with the default exit code, so any finding under
`gate/`, `lint/`, `internal/gitscope/` or `internal/srcpath/` reds the build. Clearing one means
tightening the code, or a `//nolint` that names the linter and gives a reason true of that site.
Never a new id in `golangci.yml`'s `gosec.excludes`, which would also disarm the rule in every repo
this layer visits.

`gosec` reaches `_test.go` in that run like every other linter, which is what the `//nolint:gosec`
directives across this repository's own tests answer, and what the tightened file modes beside them
made unnecessary.

Two Go cases hold that shut, because every other way of disarming the sweep leaves CI green.
`TestCIDeclaresTheBlockingLintStep` in `gate/test/ci_workflow_test.go` pins the step's `run:` line
byte for byte and rejects `continue-on-error` on the step and on the job.
`TestThePresetStillReportsTheRulesThisRepoJustified` in `gate/test/preset_test.go` reds if `gosec`
leaves `linters.enable`, if `gosec.excludes` gains an id this repository answered at the site
(`G204`, `G301`, `G302`, `G304`, `G306`, `G702`, `G703`), if `run.issues-exit-code` appears, which
overrides the exit code from inside the config rather than on the command line, or if any of the
four keys that decide how much code the linters run over gains an entry: an
`linters.exclusions.rules` entry naming `gosec` or naming no linter at all, an
`linters.exclusions.paths` or `paths-except` entry, a non-empty `linters.exclusions.presets`, or
`run.tests`. That last group is what the enable list and `gosec.excludes` miss. Those two decide
which rules are armed, these decide which code the armed rules see, so `path: .` or
`run.tests: false` empties the sweep with the enable list untouched. `gosecReachDisarms` is the
function that answers them, and its own table cases feed it each of those configs, since the
committed preset carries none of them.

`go/plugin/` and `go/test/` are separate modules, so the root-module run never reaches them. The
`lint plugin (go)` job runs `go vet` and `go test` over `go/plugin`, and it does run this preset
over `go/test/smoke` and `go/test/fixtures`, but only as fixtures holding deliberate violations.
`build.sh` runs the preset over `smoke` with `--issues-exit-code 0` and fails only when the output
carries no `tvrmsmith-comment-block-length` finding, which proves the plugin is linked in and
says nothing about any other rule. `go/test/cases_test.go` runs it over `fixtures` and fails when
an enabled linter reports nothing. Either way a finding in these two modules never blocks on its
own, because what is checked is that a rule *fired*, not that the code is clean.

## Tests

```bash
cd plugin && go test ./...
./build.sh
cd test && go test . -count=1
```

Three layers, and each answers a question the one before it cannot.

**The analyzer tests** run each analyzer over its own `testdata/src` through `analysistest`,
which fails on an unexpected diagnostic as loudly as on a missing one. That is what makes a
handful of fixtures enough: each carries the shape the rule exists to find, and beside it the
shapes that would be false positives. For `comment-block-length` that is a licence header, a
long doc comment, two paragraphs split by a blank line and a run of trailing comments; for
`combine-assertions` it is separate objects, an interrupted run, whole-value assertions, a chain
through calls, a length assertion with no indexing and an index with no length assertion.
`combine-assertions` resolves testify out of its own `testdata/src` rather than the module
cache, which is the standard `analysistest` trick and keeps the plugin module free of a testify
dependency it would otherwise carry only for tests.

**`build.sh`** answers whether the plugin reached the binary at all, which the analyzer tests
cannot: a module plugin that failed to register leaves a perfectly good golangci-lint that
silently enables nothing. It lints `test/smoke/` and fails if the one deliberate violation there
is not reported. A linter named in the config but missing from the binary is a hard error
(`plugin "x" not found`), so the run also proves every custom name registered, not just the one
that reported.

**`test/cases_test.go`** answers whether the *config* does anything. It reads the enable list out
of `golangci.yml`, runs the binary over `test/fixtures/`, and fails on any enabled linter that
reported nothing, plus the reverse, a fixture for a linter since turned off. The expectation is
read from the config rather than listed in the test, so enabling a linter without writing a
fixture fails rather than passing quietly. It has already earned itself once: it caught that
`issues.uniq-by-line`, which defaults to true, was hiding `bodyclose` behind `gosec` and
`sqlclosecheck` behind `rowserrcheck`, because each pair reports on the same line. Being
enabled and being silent look identical from outside.

`go test ./...` is wrong in `test/`: the fixtures are deliberate violations, and the `printf`
one fails the vet pass `go test` runs over the package. Only the module root package is a suite.

The plugin module requires Go 1.26, which is what `golang.org/x/tools` requires; the toolchain
line in `go.mod` lets an older `go` fetch it.

## Adoption

```bash
harness/bootstrap go <repo>
```

That builds the binary, adds the repo to `~/.config/coding-standards-go-repos`, installs the
pre-commit hook and then proves the binary can load the repo. The registry file exists because
the Go side is the one language with nowhere else to record adoption: nothing is installed in
the target and the binary is machine-wide, so without it, bootstrapping one repo would silently
start linting every other repo the hook guards.

There is no editor half yet. The hook is the whole gate.
