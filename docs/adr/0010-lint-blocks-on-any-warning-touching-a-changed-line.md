# 0010. Lint blocks on any warning touching a changed line

Accepted 2026-09-17.

**Consolidated 2026-09-23.** The seven amendments of 2026-09-18 to 2026-09-23 are folded into the
body, and their reasons moved into the sections below the rule. No rule was dropped and no decision
changed; `git log -p` on this file keeps the amendments as written.

## Current rule

The pre-commit lint blocks the commit on any finding with a location, primary or related, whose span
holds a line the change touched. Every other finding is dropped silently. A path whose new side is a
symbolic link contributes no touched line.

**Amended 2026-09-24.** During an uncommitted merge, `--staged` lints only the staged paths that
differ from both `HEAD` and `MERGE_HEAD`, so a path only the incoming side changed goes unjudged. A
path differing only from `MERGE_HEAD` matches `HEAD` and could carry no touched line, and linting
every incoming path built dozens of projects per merge. See
[PR 161](https://github.com/tvrmsmith/coding-standards/pull/161).

The warning set is everything the tool reports: every C# compiler and analyzer warning in the SARIF
(CS, CA, TVRM, FAA), every golangci-lint issue, and ESLint severity 1 and 2 alike. A SARIF result
carrying an `inSource` suppression is dropped. A finding is its rule, message and locations
together, so identical reports count once.

An entry meaning the analysis never ran blocks whatever the diff says: C# `AD0001`, `CS8032`,
`CS8034` and `CS9057`, golangci-lint's `typecheck`, and a parser's `UNPARSED` fallback.

Under `--staged`, any staged path whose disk copy differs from the index stops the run before a
finding is judged.

The one way past a finding is a waiver: one rule, one path, one finding, one use, a mandatory
reason. `lint-changed` prints the command to record one. A finding with no location takes a waiver
keyed on language and rule alone. Waivers live in an append-only JSONL log at
`${XDG_STATE_HOME:-~/.local/state}/coding-standards/waivers.jsonl`. A waiver is spent only on a run
that ends clean, keyed on the index tree sha, so a retry against the same tree reuses it and a match
on a run that still blocks stays unspent.

**Amended 2026-09-23.** Only a full `--staged` run spends; `--since`, `--files` and `--only` never
do, and they accept a waiver already spent against any tree. Only a full `--staged` run is a commit,
and a no-mistakes run lints a rebased tree the waiver was never spent against. See
[PR 151](https://github.com/tvrmsmith/coding-standards/pull/151) and
[PR 156](https://github.com/tvrmsmith/coding-standards/pull/156).

Exit 0 means nothing survived, 1 means the tool broke, 2 means a finding survived.

## Why blocking, and why now

The lint harness reported and never blocked. Its header argued the case honestly: scoping by file
means a legacy file you touch one line in carries findings on lines you never wrote, so blocking
would be unfair. That reasoning is about a human opening a legacy file.

The reader these rules actually have is an agent writing new code, and an agent authors the whole
block it is judged on. For that reader the objection mostly evaporates, and a non-blocking rule is
close to no rule. An agent shown a warning and allowed to commit will commit.

So the fix was not to keep file-level scoping and block anyway. It was to build the per-line scoping
the header named as the precondition, then block on everything.

## Why any location, not the primary one

An earlier draft scoped on the primary location alone. TVRM0001 kills that: its count-then-index
shape reports the collection access as primary, which often predates the diff, and the assertion
just written as a related location. Keying on primary would drop exactly the finding the rule exists
to catch. The same rule settles comment blocks, where a block can start above the diff and extend
into it.

## What the scope leaves out

A symbolic link holds no source of its own, only the path it points at. The gate and this lint share
one changed set, so the drop made for the gate applies here too, and a finding at a symlinked `.cs`,
`.go` or `.ts` path does not block. See [issue 144](https://github.com/tvrmsmith/coding-standards/issues/144)
and [PR 143](https://github.com/tvrmsmith/coding-standards/pull/143).

An `inSource` suppression is a target repo's own `#pragma warning disable` or `[SuppressMessage]`.
The section on waivers rejects in-source suppression as this tool's escape hatch, never as a veto
over what a target repo already decided. The console path this replaced never saw a suppressed
diagnostic, so honouring them keeps adoption invisible rather than making the C# half louder than
the repo's own build.

Identity covers rule, message and locations because a project built for several target frameworks,
or a file linked into two projects, reports the same warning more than once. Without it one line of
code would print twice and cost two waivers.

## Why some entries ignore scope

Each of these entries means the analysis never ran, so a clean scope result under it proves nothing.
The four C# ids are an analyzer that failed to load. golangci-lint reports a package that will not
compile as `typecheck`, at line 1 column 0 and at exit 0, so scoping it would pass a commit whose Go
does not build whenever line 1 is untouched. `UNPARSED` is the rule a parser reports an entry under
when it cannot read it, and an entry it could not read is one it could not scope, so dropping it
would look the same as a clean file.

Roslyn reports the four C# ids at `Location.None`, so they carry no path, and their waivers key on
language and rule alone. `typecheck` and most `UNPARSED` entries carry a real location, so their
waivers stay keyed on a path. Only an entry with no usable location at all takes the path-less
route.

## Why a divergent staged file stops the run

The linter reads disk and the commit carries the index, so one divergent file makes the whole report
describe code that is not being committed. The check covers every staged path, not only the paths
the report mentions.

## Why every warning, no tiering

An earlier draft blocked only the ids with near-zero false-positive risk and left the judgment calls
advisory. An advisory rule an agent may ignore is not enforcement. TVRM0006 in particular exists
because agents over-comment, which is the behaviour most worth blocking.

An id list is also a thing to maintain, and every id left off it is a rule nobody enforces. Once
scoping is per-line, a false positive costs a targeted rewrite of a line just written, not a legacy
backlog.

## Why a waiver log and not in-source suppression

The repo's rules are adopted machine-locally against code other people wrote. In-source suppression
leaks into their builds: many projects run their own linters, and those complain about suppressions
naming rules their configuration does not define. Raising severity in `Descriptors` has the same
problem, which is why every id stays at `Warning` and the harness sets its own exit status instead.

`git commit --no-verify` is the only override available today. It skips every hook, and agents are
instructed never to use it.

An agent hits the false positive, so an agent has to be able to clear it. The property worth
protecting is not that an agent cannot waive, it is that a human can see what it waived. Append-only
gives that: nothing is ever rewritten, so reading the log top to bottom shows every waiver recorded
and every one spent, with its reason.

The threat model is an instruction-following agent, not an adversary. A log outside the repo is
enough. The store attempts no tamper-proofing.

The log sits under state, not config, because `bootstrap` symlinks `$XDG_CONFIG_HOME/coding-standards`
at the hub checkout, so a config-directory default would write the audit log into a repository the
gate guards.

A waiver is spent only on a clean run because one use is one commit that actually went through. An
agent that waives a false positive and then fixes a real finding changes the index tree, and a
waiver spent on the blocking run would no longer match.

## Why exit 2 and not 1

A hook has to tell "the gate says stop" from "the gate broke". Collapsing them means a git outage
reads the same as a real finding, and nobody can tell whether the commit was wrong or the tooling
was. The exit-code meaning is a one-way door, since the harness scripts and the hook all read it.
