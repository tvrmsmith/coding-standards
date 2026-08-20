/**
 * Write the editor half of the personal layer into `<repo>/.vscode/settings.json`,
 * preserving whatever is already in that file.
 *
 *   node write-vscode-settings.mjs <repo-root> <package-dir>...
 *
 * The editor points at the **same wrapper the pre-commit hook uses**, through
 * `eslint.options.overrideConfigFile`. That single setting is what makes the editor layer
 * whole: `eslint-layer.js` re-loads the package's own config from `process.cwd()` and
 * spreads the namespaced preset after it, so every rule the CLI enforces — plugin rules
 * included — is defined by the config the extension loads, and no rule arrives from a
 * plugin the package does not declare.
 *
 * Why not `eslint.options.overrideConfig`, the obvious setting: a bare `rules` object is the
 * only shape both ESLint 8 and 9 accept, it cannot register a plugin, and a rule from an
 * undeclared plugin paints every file red with `Definition for rule 'x' was not found`. That
 * limits it to core rules. `overrideConfigFile` looks worse at first glance because it replaces
 * rather than merges, but the objection dissolves once the file being pointed at does the
 * merging itself — which is exactly what `eslint-layer.js` is.
 *
 * Verified by driving a package's own installed ESLint the way the extension does — its
 * `ESLint`/`FlatESLint` class, `cwd` set to the working directory, `lintText` on a buffer.
 * The layered config resolves with zero fatal errors and every plugin rule defined, where
 * the `overrideConfig` route reached none of them.
 *
 * Two settings carry the rest of it:
 *
 * - `eslint.useFlatConfig`, because `eslint-layer.js` is a flat config and an ESLint 8 package
 *   has flat off by default. 8.57.x loads it through `FlatESLint` without complaint.
 * - `eslint.workingDirectories` in the object form with `changeProcessCWD`, not the string
 *   form. `eslint-layer.js` finds the package config by `process.cwd()`, so a working
 *   directory that only sets the API's `cwd` option silently degrades to "no package
 *   config found, personal layer only" — which loses the package's parser and reports the
 *   whole file as a parse error rather than failing loudly.
 *
 * Cost, unchanged in kind from the previous layer but now identical to the commit gate:
 * `no-restricted-syntax` is a core rule, so inside test files the preset's A4/D11
 * selectors replace a package's own. `bootstrap` names the affected packages when it runs.
 *
 * Prerequisite, worth stating because nothing here can check it: none of this is read by
 * anything unless an ESLint editor integration is installed (`dbaeumer.vscode-eslint`, or
 * Zed's `lsp.eslint`). `bootstrap` warns when it cannot find one.
 */
import { existsSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const [repoRoot, ...packageDirs] = process.argv.slice(2)
if (!repoRoot || packageDirs.length === 0) {
  console.error('usage: write-vscode-settings.mjs <repo-root> <package-dir>...')
  process.exit(2)
}

// Resolved, not as invoked: bootstrap installs `~/.config/coding-standards` as a symlink to
// this directory, and a settings file naming the link would break the moment the hub moves.
const layerPath = join(realpathSync(dirname(fileURLToPath(import.meta.url))), 'eslint-layer.js')

const settingsPath = join(repoRoot, '.vscode', 'settings.json')

/** `.vscode/settings.json` is JSONC in VS Code, so read it tolerantly. */
function readExisting() {
  if (!existsSync(settingsPath)) return {}
  const raw = readFileSync(settingsPath, 'utf8')
  const stripped = raw.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
  if (stripped.trim() === '') return {}
  return JSON.parse(stripped)
}

const settings = readExisting()

const options = { ...(settings['eslint.options'] ?? {}) }
options.overrideConfigFile = layerPath

// Clean up after the core-rule-only layer this replaced: its hand-copied A4/D11 selectors
// now come from the preset itself, and leaving them behind would apply them to every file
// rather than to test files, on top of the layered config.
if (options.overrideConfig?.rules) {
  const { 'no-restricted-syntax': _dropped, ...rest } = options.overrideConfig.rules
  if (Object.keys(rest).length > 0) options.overrideConfig = { ...options.overrideConfig, rules: rest }
  else {
    const { rules: _rules, ...withoutRules } = options.overrideConfig
    if (Object.keys(withoutRules).length > 0) options.overrideConfig = withoutRules
    else delete options.overrideConfig
  }
}

settings['eslint.options'] = options
settings['eslint.useFlatConfig'] = true

// Without these the extension runs ESLint from the workspace root: flat-config packages
// would find no config at all (flat config does not cascade), and legacy ones would be
// linted from the wrong cwd. `changeProcessCWD` is what `eslint-layer.js` reads.
settings['eslint.workingDirectories'] = packageDirs
  .map((dir) => `./${relative(repoRoot, resolve(dir))}`)
  .sort()
  .map((directory) => ({ directory, changeProcessCWD: true }))

mkdirSync(dirname(settingsPath), { recursive: true })
writeFileSync(settingsPath, `${JSON.stringify(settings, null, 2)}\n`)
console.log(`wrote ${settingsPath} (${settings['eslint.workingDirectories'].length} working directories)`)
console.log(`  overrideConfigFile: ${layerPath}`)
