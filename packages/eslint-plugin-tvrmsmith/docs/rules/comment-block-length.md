# `tvrmsmith/comment-block-length`

Limit how many lines one run of non-documentation comments may span.

**Guideline:** *Comments* — [`coding-standards/SKILL.md`](../../../../plugins/coding-standards/skills/coding-standards/SKILL.md#comments),
"a paragraph justifying a workaround means the code is wrong". No off-the-shelf rule exists
in either language: StyleCop has no length rule at all, Sonar's `S103` and `@stylistic/max-len`
cap a *line* rather than a comment, and `sonarjs/comment-regex` allows only one pattern per
config. The C# half is the `CommentBlockLengthAnalyzer` Roslyn analyzer (TVRM0006).

Severity in the preset: **warn** over 10 lines.

## Rule details

The skill says the judgement is *whether the comment is justifying something*, not how long
it is. No lint rule can read intent, so length stands in as the one mechanical proxy, and a weak
one. Treat a report as a prompt to re-read the code, not a verdict on it. That is also why the
rule warns and never errors: the honest fix is a judgement call, so it must not stop a build.

A **comment block** is a maximal run of own-line, non-documentation comments on consecutive
lines. It is measured in source lines from the first comment's opening line to the last
comment's closing line.

Four things end a block:

- a blank line, because the author put a gap there and two paragraphs are two comments
- any line of code
- a trailing comment (`x++ // why`), which annotates the code on its line rather than standing
  on its own. A run of trailing comments never forms a block, since measuring it would be
  measuring the code
- a doc comment, which is exempt and never joins a block

👎 Example of **incorrect** code:

```ts
// The retry loop below cannot use the shared backoff helper. That helper reads its ceiling
// from the config object, and the config object is not populated yet at this point in
// startup, because the config loader itself goes through this client. So the ceiling is
// inlined here instead. If you move the config load earlier, which was attempted in an
// earlier revision, the credential provider breaks instead, because it also reads config,
// and it is constructed before the loader. The ordering constraint is real but undocumented
// elsewhere, so it is written out here. Someone should probably restructure startup so the
// config load does not depend on the client that depends on the config, but that is a
// larger change than this fix needed, and the inline ceiling is correct in the meantime.
// Note that the value must stay under the gateway's own timeout.
// See also the comment in the credential provider.
for (let attempt = 0; attempt < 5; attempt++) {
```

👍 Examples of **correct** code:

```ts
// The config loader goes through this client, so the shared backoff helper's ceiling is not
// readable yet. Inlined instead. Must stay under the gateway timeout.
for (let attempt = 0; attempt < RETRY_CEILING; attempt++) {
```

```ts
/**
 * A doc comment is exempt at any length. The same skill section requires documenting public
 * API contracts, non-obvious logic, invariants, units and side effects, so a rule that
 * punished a thorough JSDoc block would contradict the standard it enforces.
 */
export function retryingFetch(url: string) {}
```

## Doc comments are exempt

A doc comment (`/**` in JS and TS, `///` or `/**` in C#) is skipped entirely, however long it
runs.

It also **ends** the run it sits next to rather than absorbing it. A two-line summary glued
above thirty lines of prose exempts only the summary; the prose is measured on its own. Without
that break the exemption would be the easiest way to defeat the rule. Roslyn splits the two
constructs this way of its own accord, so the C# half arrived at this first and the ESLint rule
follows it.

Measured against this repo at adoption: of 465 comment blocks, 33 exceed ten lines and all 33
are doc comments. No non-doc block exceeds ten lines. The exemption is what makes the threshold
a regression guard rather than a refactor mandate.

## Options

| Option | Default | Meaning |
|---|---|---|
| `max` | `10` | Report a block longer than this. |

In C# the same budget comes from an `.editorconfig` key, `tvrmsmith_comment_block_max_lines`,
because Roslyn has no rule-parameter mechanism of its own.

## No autofix

There cannot be one. Every honest response, whether shortening the prose, moving it to a doc
comment, extracting the code it describes into a named function, or fixing the code it
justifies, is a judgement the author has to make.
