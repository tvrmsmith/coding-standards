/**
 * The CLI layering wrapper: run any package's own ESLint config, then the personal
 * preset on top.
 *
 * ```sh
 * cd <package> && ESLINT_USE_FLAT_CONFIG=true npx eslint --config ~/.config/coding-standards/eslint-layer.js src/foo.ts
 * ```
 *
 * `--config` *replaces* the auto-discovered config, so this file re-loads the package's
 * own config from `process.cwd()` first and spreads the preset after it — flat config
 * does not cascade, so composition has to happen at invocation.
 *
 * Two branches, because a repo mid-migration has both config formats in it:
 *
 * - **flat** (`eslint.config.js`, ESLint 9) — dynamic `import()`.
 * - **legacy** (`.eslintrc*`, ESLint 8) — `FlatCompat.config()` translation,
 *   which needs the linter in flat mode: hence `ESLINT_USE_FLAT_CONFIG=true`, required on
 *   ESLint 8 and harmless on 9.
 *
 * Resolution note: every bare import below resolves from *this file's* directory, not the
 * linted package's. The preset and its plugins therefore come from the hub's own
 * `node_modules` — which is the only thing that works, since pnpm's strict layout means
 * `@eslint/eslintrc` is not resolvable from the linted package's directory even though
 * ESLint itself depends on it.
 */
import { existsSync, readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { join, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

import { FlatCompat } from '@eslint/eslintrc'
import js from '@eslint/js'
import tvrmsmith from 'eslint-config-tvrmsmith'

const cwd = process.cwd()
const debug = process.env.TVRMSMITH_ESLINT_DEBUG === '1'

const FLAT_CONFIG_NAMES = [
  'eslint.config.js',
  'eslint.config.mjs',
  'eslint.config.cjs',
  'eslint.config.ts',
  'eslint.config.mts',
]

const LEGACY_CONFIG_NAMES = [
  '.eslintrc.js',
  '.eslintrc.cjs',
  '.eslintrc.mjs',
  '.eslintrc.json',
  '.eslintrc.yaml',
  '.eslintrc.yml',
  '.eslintrc',
]

/** @param {string[]} names */
function findConfig(names) {
  for (const name of names) {
    const path = resolve(cwd, name)
    if (existsSync(path)) return path
  }
  return null
}

/** JSON with comments — `.eslintrc.json` is JSONC by ESLint's own reader. */
function parseJsonc(source) {
  const stripped = source
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
  return JSON.parse(stripped)
}

async function loadPackageConfig() {
  const flatPath = findConfig(FLAT_CONFIG_NAMES)
  if (flatPath) {
    const mod = await import(pathToFileURL(flatPath).href)
    const exported = mod.default ?? []
    if (debug) console.error(`[tvrmsmith] flat config: ${flatPath}`)
    return { format: 'flat', path: flatPath, config: Array.isArray(exported) ? exported : [exported] }
  }

  const legacyPath = findConfig(LEGACY_CONFIG_NAMES)
  if (!legacyPath) {
    if (debug) console.error(`[tvrmsmith] no package config under ${cwd}; personal layer only`)
    return { format: 'none', path: null, config: [] }
  }

  if (/\.ya?ml$/.test(legacyPath)) {
    throw new Error(
      `[tvrmsmith] ${legacyPath}: YAML .eslintrc is not supported by the layering wrapper. ` +
        'Convert it to JSON, or lint this package without the personal layer.',
    )
  }

  const raw = legacyPath.endsWith('.json') || legacyPath.endsWith('.eslintrc')
    ? parseJsonc(readFileSync(legacyPath, 'utf8'))
    : createRequire(join(cwd, 'noop.cjs'))(legacyPath)

  const compat = new FlatCompat({
    baseDirectory: cwd,
    resolvePluginsRelativeTo: cwd,
    recommendedConfig: js.configs.recommended,
    allConfig: js.configs.all,
  })

  if (debug) console.error(`[tvrmsmith] legacy config via FlatCompat: ${legacyPath}`)
  return { format: 'legacy', path: legacyPath, config: compat.config(raw) }
}

/**
 * `no-restricted-syntax` is a core rule, so the preset's config object replaces the
 * package's options wholesale rather than merging them — the preset documents this and
 * offers `createBase({ extraRestrictedSyntax })` as the escape hatch.
 *
 * Passing the package's selectors through is *not* the default, and deliberately so. A package
 * that sets `no-restricted-syntax` almost always aims it at feature code — banned imports, hex
 * colour literals, bespoke component shapes — and either scopes it away from tests already or
 * would fire noisily if those selectors were re-scoped onto tests. So the pass-through restores
 * nothing and costs false positives. This warns instead of guessing.
 */
function warnOnRestrictedSyntaxCollision(packageConfig) {
  const collides = packageConfig.some(
    (entry) => entry?.rules?.['no-restricted-syntax'] !== undefined && entry.files === undefined,
  )
  if (collides) {
    console.error(
      '[tvrmsmith] note: this package configures `no-restricted-syntax` unscoped. Inside test ' +
        'files the personal layer replaces its selectors with its own (A4, D11). Elsewhere they stand.',
    )
  }
}

/**
 * Rename every plugin namespace the preset registers, and rewrite its rule ids to match.
 *
 * Without this the layering does not run at all: a package whose own config loads
 * `@typescript-eslint` and the preset loading its own copy is
 * `ConfigError: Key "plugins": Cannot redefine plugin "@typescript-eslint"` — flat config
 * rejects two different plugin objects under one namespace, on ESLint 8 and 9 alike. The
 * clash is guaranteed here, not incidental: the wrapper resolves plugins from the hub and
 * the package resolves them from its own `node_modules`.
 *
 * The alternative — dropping our registration and borrowing the package's plugin — was
 * rejected: it silently binds the personal rules to whatever plugin *version* the package
 * pins, and a rule that version does not have is a hard "Definition for rule not found"
 * that fails the whole run. Any repo pinning `testing-library` v6 hits exactly that, since the
 * preset enables v7-only rules.
 *
 * Side benefit, worth keeping: the namespace tells you where a violation came from.
 * `tvrmsmith-testing-library/prefer-screen-queries` is the personal layer;
 * `testing-library/prefer-screen-queries` is the package's own config.
 */
function namespacePreset(configs) {
  const rename = (plugin) =>
    plugin === 'tvrmsmith' ? plugin : `tvrmsmith-${plugin.replace(/^@/, '').replace(/\//g, '-')}`

  return configs.map((entry) => {
    if (!entry?.plugins) return entry

    const renamed = new Map(Object.keys(entry.plugins).map((name) => [name, rename(name)]))
    const plugins = Object.fromEntries(
      Object.entries(entry.plugins).map(([name, plugin]) => [renamed.get(name), plugin]),
    )
    const rules = Object.fromEntries(
      Object.entries(entry.rules ?? {}).map(([ruleId, setting]) => {
        const slash = ruleId.indexOf('/')
        if (slash === -1) return [ruleId, setting]
        const target = renamed.get(ruleId.slice(0, slash))
        return [target ? `${target}${ruleId.slice(slash)}` : ruleId, setting]
      }),
    )

    return { ...entry, plugins, rules }
  })
}

const { config: packageConfig } = await loadPackageConfig()
warnOnRestrictedSyntaxCollision(packageConfig)

export default [...packageConfig, ...namespacePreset(tvrmsmith)]
