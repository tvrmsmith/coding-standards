# packages/

npm packages.

- `eslint-config-tvrmsmith` — the curated preset, off the shelf apart from the
  one custom rule it registers. Every rule traces to a named guideline.
  Assumes no test runner: vitest and jest are both in scope, so neither is baked in.
- `eslint-plugin-tvrmsmith` — the one v1 custom rule, `combine-assertions-on-same-object`
 . The preset depends on it by `link:`, so a consumer that installs the preset
  gets the rule registered and set to `warn` with no extra wiring.

Consumed by local path while the rules churn; published to GitHub Packages once they settle.
The Claude Code plugin loader ignores this directory.
