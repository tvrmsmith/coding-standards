# guidance-ab

Answers one question: does loading the coding-standards skill change what the no-mistakes review
finds, what it costs, and how long the work takes? The script joins logs that already sit on the
machine and prints the result per group. It needs Node 24 and has no dependencies.

## Run

```sh
node cli.mjs arm      # the experiment: branches the arm log assigned to guidance or review-only
node cli.mjs history  # observational: branches grouped by whether a transcript loaded the skill
```

Both print a text report. `--json` prints the full report as JSON instead. Arm mode compares the two
assigned arms and ends with the decision rule's verdict. History mode groups are self-selected, so its
report says so and carries no verdict.

| Option | Default |
| --- | --- |
| `--state-dir D` | `${XDG_STATE_HOME:-$HOME/.local/state}/coding-standards` |
| `--no-mistakes-home D` | `$HOME/.no-mistakes` |
| `--review-ab F` | `$HOME/.claude/review-ab.jsonl` |
| `--projects D` | `$HOME/.claude/projects` |
| `--seed N` | `1`, which fixes the bootstrap intervals |

A missing or unknown mode prints usage and exits 2. An analysis error, such as a missing
`arms.jsonl` in arm mode, prints its message and exits 1.

## Inputs

| Input | Source |
| --- | --- |
| `arms.jsonl` in the state dir | The hook that assigns each session an arm |
| `lint-runs.jsonl` in the state dir | The pre-commit lint runs |
| `state.sqlite` and `repos/` in the no-mistakes home | No-mistakes runs, review rounds, and the bare clones the diffs come from |
| the review-ab file | The per-aspect review results and their arms |
| the projects dir | Claude Code transcripts, for skill loads, tokens, and wall time |

## Decision rule

`DECISION_RULE` in `analyze.mjs` ships as `null`, so every arm verdict reads `not set`. Set it before
the arms have data: a metric, a statistic, `favour: 'lower'`, and a margin. The verdict compares the
rule's diff interval to the margin and reports `guidance`, `review-only`, or `inconclusive`.

## Data stays local

Every input is machine-local and this directory holds no data from any of them. Run the script where
the logs live and keep its output out of the repository.
