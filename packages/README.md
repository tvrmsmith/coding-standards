# packages/

npm packages.

- `eslint-config-tvrmsmith` — the curated preset, off the shelf apart from the
  one custom rule it registers. Most rules trace to a named guideline; the mapping
  runs one way, so a rule need not trace back to one.
  Assumes no test runner: vitest and jest are both in scope, so neither is baked in.
  `eslint-plugin-jest` is registered anyway, for the assertion rules that decide
  syntactically and so cover both — its README explains the one setting that rests on.
- `eslint-plugin-tvrmsmith` — the one v1 custom rule, `combine-assertions-on-same-object`
 . The preset depends on it by `link:`, so a consumer that installs the preset
  gets the rule registered and set to `warn` with no extra wiring.

Consumed by local path while the rules churn; published to GitHub Packages once they settle.
The Claude Code plugin loader ignores this directory.
