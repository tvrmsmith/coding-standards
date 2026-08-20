import combineAssertionsOnSameObject from './rules/combine-assertions-on-same-object.js'

/**
 * The custom half of the tvrmsmith lint layer: the guidelines with no off-the-shelf rule
 * in either language. Everything that *does* have one lives in `eslint-config-tvrmsmith`.
 *
 * Exactly one rule in v1. The other two custom rules from the enforcement mapping —
 * `no-suppression-before-assertion` and `no-assertion-escape-cast` — are C#-only; their
 * TypeScript halves are covered off the shelf (see `eslint-config-tvrmsmith/base`) or do
 * not exist as a TypeScript shape at all.
 *
 * Flat-config only. A consumer registers it themselves, or gets it through the preset:
 *
 * ```js
 * import tvrmsmith from 'eslint-config-tvrmsmith'
 * export default [...packageOwnConfig, ...tvrmsmith]
 * ```
 *
 * @type {import('eslint').ESLint.Plugin}
 */
const plugin = {
  meta: { name: 'eslint-plugin-tvrmsmith', version: '0.1.0' },
  rules: {
    'combine-assertions-on-same-object': combineAssertionsOnSameObject,
  },
}

export default plugin
export const rules = plugin.rules
