import base, { createBase, noOptionalChainInExpect } from './base.js'
import react from './react.js'

export { base, createBase, noOptionalChainInExpect, react }
export { sourceFiles, testFiles } from './globs.js'

/**
 * Everything. Spread this **after** a package's own config so the personal layer wins
 * on the rules it names:
 *
 * ```js
 * import tvrmsmith from 'eslint-config-tvrmsmith'
 * export default [...packageOwnConfig, ...tvrmsmith]
 * ```
 *
 * A package with no React should import `eslint-config-tvrmsmith/base` instead.
 */
export default [...base, ...react]
