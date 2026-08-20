/**
 * File patterns the preset's slices are scoped to.
 *
 * The preset is runner-agnostic on purpose: vitest and jest are both in scope, often several
 * majors of vitest side by side in one repo. These globs match the file-naming conventions both
 * runners use, so nothing here depends on which runner is installed.
 */

const SCRIPT_EXTENSIONS = 'js,jsx,ts,tsx,mjs,cjs,mts,cts'

/** Test files. Both `*.test.*` / `*.spec.*` and `__tests__/` layouts. */
export const testFiles = [
  `**/*.{test,spec}.{${SCRIPT_EXTENSIONS}}`,
  `**/__tests__/**/*.{${SCRIPT_EXTENSIONS}}`,
]

/** Every linted script file — production source and tests alike. */
export const sourceFiles = [`**/*.{${SCRIPT_EXTENSIONS}}`]
