import base, { assertionIntentSettings, createBase, noOptionalChainInExpect } from './base.js'
import react from './react.js'
import typed, { typedBase, typedReact } from './typed.js'

export { assertionIntentSettings, base, createBase, noOptionalChainInExpect, react }
export { typed, typedBase, typedReact }
export { sourceFiles, testFiles } from './globs.js'

/**
 * Everything that runs without type information. Spread this **after** a package's own
 * config so the personal layer wins on the rules it names:
 *
 * ```js
 * import tvrmsmith from 'eslint-config-tvrmsmith'
 * export default [...packageOwnConfig, ...tvrmsmith]
 * ```
 *
 * A package with no React should import `eslint-config-tvrmsmith/base` instead.
 *
 * The type-aware layer is **not** in here. It needs the package to configure
 * `parserOptions.projectService`, and it throws where that is missing, so it stays opt-in:
 * `eslint-config-tvrmsmith/typed`, or nothing at all if the machine-local wrapper is doing
 * the composing.
 */
export default [...base, ...react]
