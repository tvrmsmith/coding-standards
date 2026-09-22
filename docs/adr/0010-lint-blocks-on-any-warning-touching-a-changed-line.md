# 0010. Lint blocks on any warning touching a changed line

Accepted 2026-09-17.

## Current rule

The pre-commit lint blocks the commit. A finding survives to block it when any of its locations,
primary or related, has a span holding a line the change touched. A finding whose locations all fall
outside the touched set is dropped silently.

The warning set is everything the tool reports. C# takes every
compiler and analyzer warning in the SARIF, CS and CA alongside TVRM and FAA. Go takes every
golangci-lint issue. TypeScript takes ESLint severity 1 and 2 alike.

**Amended 2026-09-18.** One exclusion: a SARIF result carrying an `inSource` suppression is dropped.
The section below rejects in-source suppression as this tool's own escape hatch, never as a veto
over what a target repo already decided with `#pragma warning disable` or `[SuppressMessage]`. The
console path this replaced never saw a suppressed diagnostic, so honouring them keeps adoption
invisible rather than making the C# half louder than the repo's own build.

**Amended 2026-09-18.** A finding is identified by its rule, its message and its locations together.
Two reports carrying all three the same are one finding, so a project built for several target
frameworks and a file linked into two projects each report their warning once. Without that, one
line of code would print twice and cost two waivers.

Four C# ids ignore scope and block whatever the diff says: `AD0001`, `CS8032`, `CS8034`, `CS9057`.
Each means an analyzer failed to load, so a clean result under it proves nothing.

**Amended 2026-09-18.** Roslyn reports all four at `Location.None`, so they carry no path. A waiver
is keyed on a path, so a waiver for one of these carries no path either and keys on the language and
the rule alone. That is the only route past them.

**Amended 2026-09-21.** The general rule is that an entry means the analysis never ran, so a clean
scope result under it proves nothing. Three routes satisfy it. The four C# ids above, each an
analyzer that failed to load. golangci-lint's `typecheck` issue, which is how it reports a package
that will not compile, at line 1 column 0 and at exit 0, so scoping it would pass a commit whose Go
does not build whenever line 1 is untouched. The `UNPARSED` fallback rule a parser reports an entry
it cannot read under, since an entry it could not read is one it could not scope either, and
dropping it silently would look the same as a clean file. These differ from the C# four in one way.
`typecheck` and most `UNPARSED` entries carry a real location, so their waivers stay keyed on a
path. Only an entry with no usable location at all takes the path-less route the paragraph above
describes.

Under `--staged`, a staged path whose disk copy differs from the index stops the run before any
finding is judged. The linter reads disk and the commit carries the index, so a divergent file makes
the whole report describe code that is not being committed. The check covers every staged path, not
the paths the report mentions.

The one way past a finding is a waiver: one rule, one path, one use, a mandatory reason, recorded in
an append-only JSONL log outside the repo. `lint-changed` prints the exact command to record one.
A spend is keyed on the index tree sha, so retrying a commit against the same tree reuses the waiver
rather than burning a second.

**Amended 2026-09-18.** The log is `${XDG_STATE_HOME:-~/.local/state}/coding-standards/waivers.jsonl`.
State, not config, because `bootstrap` symlinks `$XDG_CONFIG_HOME/coding-standards` at the hub
checkout, so a config-directory default would write the audit log into a repository the gate guards.
One waiver covers exactly one finding, so two findings under the same rule and path cost two
waivers. A waiver is spent only on a run that ends clean, because one use is one commit that
actually went through; a match on a run that still blocks is reported and left unspent, so an agent
that waives a false positive and then fixes a real finding does not lose the waiver to the changed
index tree.

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

## Why exit 2 and not 1

A hook has to tell "the gate says stop" from "the gate broke". Collapsing them means a git outage
reads the same as a real finding, and nobody can tell whether the commit was wrong or the tooling
was. The exit-code meaning is a one-way door, since the harness scripts and the hook all read it.
