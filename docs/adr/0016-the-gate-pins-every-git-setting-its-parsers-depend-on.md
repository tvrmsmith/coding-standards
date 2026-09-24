# The gate pins every git setting its parsers depend on

**Status:** accepted 2026-09-23. Splits [ADR 0007](https://github.com/tvrmsmith/coding-standards/blob/7ff6abedb4b8992e075c5db9a352a2940419070f/docs/adr/0007-changed-method-is-a-span-holding-a-touched-line.md) with
[ADR 0014](0014-a-changed-method-is-a-span-holding-a-touched-line.md),
[ADR 0015](0015-a-pure-move-is-dropped-by-counting-digests.md) and
[ADR 0017](0017-the-diff-scopes-agree-except-at-two-points.md). The decision is unchanged.

## Current rule

Every git setting the gate's parsers depend on is pinned by the gate, because each unpinned one
turns a real change into `changed_methods: 0`, exit 0, which is the worst outcome a blocking gate
has. Four mechanisms reach the diff, and each takes a different answer: `-c` flags, a scrubbed
environment, blanked content filters, and `--text`.

**Repo-controlled text is never interpolated where it can be read as syntax.**

## Decision

- Ambient config such as `color.ui=always` is overridden with `-c` flags.
- Environment config such as `GIT_EXTERNAL_DIFF`, which outranks `-c`, is dropped from the
  command's environment. That hands the run to an ambient `~/.gitconfig`, so `GIT_CONFIG_GLOBAL`
  and `GIT_CONFIG_SYSTEM` are pinned at the null device, which in turn puts `safe.directory` out of
  reach, so `-c safe.directory=*` goes on the command line.
- Content filters are blanked. A clean driver named by the repo's own `.git/config` and selected by
  `.gitattributes` can print its input back unchanged and empty the patch, and no flag turns
  filtering off. The gate enumerates the configured drivers and sets each one empty in command
  scope, `required` included. Repo-local `core.fsmonitor` is blanked for the same reason.
- `--text` forces hunk headers out of a file git would summarise as "Binary files differ", covering
  UTF-16 source and any path marked `-diff` or `binary`.

The blanks travel through the `GIT_CONFIG_COUNT` family rather than `-c`, because a driver's
subsection name is repo-controlled and a `-c` argument splits on its first `=`. That is the
interpolation rule above. An earlier spelling lost to a subsection name holding a space, and a later
one to a name holding `=`, each a silent pass.
