import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import test, { describe } from 'node:test'

import tsParser from '@typescript-eslint/parser'
import { ESLint } from 'eslint'

import base from '../base.js'
import preset from '../index.js'
import react from '../react.js'
import typed, { typedBase } from '../typed.js'
import { cases, preambles } from './cases.js'

const fixturePackage = join(dirname(fileURLToPath(import.meta.url)), 'fixture-package')

/**
 * Stands in for the consumer package's own config. The preset is spread *after* it,
 * exactly as a real package layers it.
 */
const packageOwnConfig = {
  name: 'fixture-package/own',
  files: ['**/*.{ts,tsx}'],
  languageOptions: {
    parser: tsParser,
    parserOptions: { ecmaFeatures: { jsx: true }, sourceType: 'module' },
  },
}

/**
 * The same stand-in, plus the one thing the typed layer requires of a consumer. This is
 * exactly what makes a package "ready", and what the wrapper detects.
 */
const typedPackageOwnConfig = {
  ...packageOwnConfig,
  name: 'fixture-package/own-typed',
  languageOptions: {
    ...packageOwnConfig.languageOptions,
    parserOptions: {
      ...packageOwnConfig.languageOptions.parserOptions,
      projectService: true,
      tsconfigRootDir: fixturePackage,
    },
  },
}

const eslint = new ESLint({
  cwd: fixturePackage,
  overrideConfigFile: true,
  overrideConfig: [packageOwnConfig, ...preset],
})

const typedEslint = new ESLint({
  cwd: fixturePackage,
  overrideConfigFile: true,
  overrideConfig: [typedPackageOwnConfig, ...preset, ...typed],
})

const filePathFor = (scope) =>
  join(fixturePackage, scope === 'test' ? 'src/sample.test.tsx' : 'src/Sample.tsx')

async function reportsOn(code, scope, wantsTypes = false) {
  const preamble = preambles[wantsTypes ? (scope === 'test' ? 'typedTest' : 'typedSource') : scope]
  const [result] = await (wantsTypes ? typedEslint : eslint).lintText(preamble + code, {
    filePath: filePathFor(scope),
  })
  return result.messages.map((m) => ({ rule: m.ruleId ?? `fatal: ${m.message}`, text: m.message }))
}

async function rulesFiredOn(code, scope) {
  return (await reportsOn(code, scope)).map((r) => r.rule)
}

/**
 * A case matches a report by rule id, and — for the several guidelines sharing
 * `no-restricted-syntax` — by the selector's own message too.
 */
const matcherFor = ({ rule, selector }) =>
  selector
    ? (report) => report.rule === rule && report.text === selector.message
    : (report) => report.rule === rule

const describeCase = ({ rule, selector }) => (selector ? `${rule} (${selector.selector})` : rule)

/**
 * Every rule the preset enables, flattened from the slices themselves.
 *
 * An entry set to `off` is excluded: those are the core rules the `typescript-eslint`
 * extension rules replace, and a rule that cannot report has nothing to write a fixture for.
 */
const severityOf = (setting) => (Array.isArray(setting) ? setting[0] : setting)

const enabledIn = (configs) =>
  new Set(
    configs.flatMap((config) =>
      Object.entries(config.rules ?? {})
        .filter(([, setting]) => severityOf(setting) !== 'off')
        .map(([rule]) => rule),
    ),
  )

const enabledRules = enabledIn(preset)
const typedRules = enabledIn(typed)
const allRules = new Set([...enabledRules, ...typedRules])

/**
 * A case says which rule it exercises and nothing else; whether it needs a TypeScript program
 * follows from the rule being in the typed layer. Deriving it beats a `typed: true` field,
 * which would be a second place for the same fact to live and the only one that can be wrong.
 */
const needsTypes = (rule) => typedRules.has(rule)

describe('every enabled rule has a fixture case', () => {
  test('the case list and the preset name the same rules', () => {
    const covered = new Set(cases.map((c) => c.rule))
    assert.deepEqual(
      [...allRules].filter((r) => !covered.has(r)),
      [],
      'enabled rules with no fixture case',
    )
    assert.deepEqual(
      [...covered].filter((r) => !allRules.has(r)),
      [],
      'fixture cases for rules neither layer enables',
    )
  })
})

describe('each rule fires on a violation and stays quiet on compliant code', () => {
  for (const testCase of cases) {
    const { rule, scope, violating, compliant } = testCase
    const matches = matcherFor(testCase)
    const label = describeCase(testCase)
    const typedCase = needsTypes(rule)

    test(`${label} fires`, async () => {
      const reports = await reportsOn(violating, scope, typedCase)
      assert.ok(
        reports.some(matches),
        `expected ${label}, got: ${reports.map((r) => r.rule).join(', ') || 'nothing'}`,
      )
    })

    test(`${label} stays quiet`, async () => {
      const reports = await reportsOn(compliant, scope, typedCase)
      assert.ok(
        !reports.some(matches),
        `${label} fired on the compliant sample; all reports: ${reports.map((r) => r.rule).join(', ')}`,
      )
    })
  }
})

describe('slice scoping', () => {
  test('the React/RTL testing rules do not apply to production source', async () => {
    const fired = await rulesFiredOn(
      `export const f = () => { fireEvent.click(el) }`.replace('fireEvent', 'globalThis.fireEvent'),
      'source',
    )
    assert.ok(!fired.some((r) => r?.startsWith('testing-library/')))
  })

  test('base is React-free', () => {
    // Deduped: several slices register the same plugin, which flat config permits because
    // they all spread the same imported object.
    const plugins = [
      ...new Set(base.flatMap((config) => Object.keys(config.plugins ?? {}))),
    ].sort()
    assert.deepEqual(plugins, ['@typescript-eslint', 'jest', 'regexp', 'sonarjs', 'tvrmsmith'])
  })

  test('react carries only the React, RTL and Sonar-React plugins', () => {
    // `sonarjs` appears in both slices. Its two React rules cannot live in base — they read
    // `useState` — and its bug rules cannot live here, so the plugin is registered twice.
    const plugins = [
      ...new Set(react.flatMap((config) => Object.keys(config.plugins ?? {}))),
    ].sort()
    assert.deepEqual(plugins, [
      'jest-dom',
      'react',
      'react-hooks',
      'react-you-might-not-need-an-effect',
      'sonarjs',
      'testing-library',
    ])
  })

  test('no React rule lands in base', () => {
    const baseRules = base.flatMap((config) => Object.keys(config.rules ?? {}))
    assert.deepEqual(
      baseRules.filter((r) => r.startsWith('react')),
      [],
    )
  })

  /**
   * @param {string} rule
   * @param {import('eslint').Linter.Config[]} configs to resolve the plugin from
   */
  const isTypeAware = (rule, configs) => {
    const [namespace, ...rest] = rule.split('/')
    const plugin = configs.find((config) => config.plugins?.[namespace])?.plugins[namespace]
    return Boolean(plugin?.rules?.[rest.join('/')]?.meta?.docs?.requiresTypeChecking)
  }

  test('the default export enables no type-aware rule', () => {
    // The default export is spread over packages with no `projectService`, where a typed rule
    // throws and takes the run down. That is what the separate `typed` entry point is for.
    assert.deepEqual(
      [...enabledRules].filter((rule) => isTypeAware(rule, preset)),
      [],
    )
  })

  test('the typed layer enables nothing that would run without a program', () => {
    // The mirror of the assertion above, and the one that keeps the split honest: a rule that
    // needs no types belongs in the default export, where every package gets it.
    assert.deepEqual(
      [...typedRules].filter((rule) => !isTypeAware(rule, typed)),
      [],
    )
  })

  test('typedBase is React-free, the way base is', () => {
    const rules = typedBase.flatMap((config) => Object.keys(config.rules ?? {}))
    assert.deepEqual(
      rules.filter((r) => r.startsWith('react')),
      [],
    )
  })

  test('the typed layer turns off the base rule its jest-aware copy replaces', () => {
    // Both `@typescript-eslint/unbound-method` and `jest/unbound-method` would otherwise report
    // the same line in a test file, and the jest one is the copy that knows about `jest.fn()`.
    const inTests = typed.find((config) => config.name === 'tvrmsmith/typed/tests')
    assert.equal(inTests.rules['@typescript-eslint/unbound-method'], 'off')
    assert.equal(inTests.rules['jest/unbound-method'], 'error')
  })

  test('no vitest-plugin rule is enabled', () => {
    assert.deepEqual(
      [...enabledRules].filter((r) => r.startsWith('vitest/')),
      [],
      '@vitest/eslint-plugin duplicates the jest rules it forked; registering both double-reports every violation',
    )
  })

  test('no enabled jest rule resolves the jest package', () => {
    // `jest/no-deprecated-functions` calls `require.resolve('jest/package.json')` and
    // *throws* where jest is not installed — which is most of the target. Every other rule
    // used here decides syntactically. Enabling it would take out whole vitest packages.
    assert.ok(
      !enabledRules.has('jest/no-deprecated-functions'),
      'jest/no-deprecated-functions throws in a package without jest installed',
    )
  })
})

/**
 * The A2/A5 rules come from `eslint-plugin-jest`, and the preset applies them to vitest
 * packages too. Nothing about that is obvious from the rule ids, and one deleted `settings`
 * block silently drops the 387 files at the adoption target that import `expect` from
 * `vitest`. These assert the coverage rather than the configuration.
 */
describe('assertion-intent rules cover both runners', () => {
  const violation = `it('a', () => { expect(list.length).toBe(3) })`
  const declarations = `declare const list: string[]\n`

  const firedOn = async (source) => {
    const [result] = await eslint.lintText(source, { filePath: filePathFor('test') })
    return result.messages.map((m) => m.ruleId ?? `fatal: ${m.message}`)
  }

  test('fires on the injected globals both runners provide', async () => {
    assert.ok((await firedOn(declarations + violation)).includes('jest/prefer-to-have-length'))
  })

  test("fires where expect is imported from 'vitest'", async () => {
    const source = `import { it, expect } from 'vitest'\n` + declarations + violation
    assert.ok((await firedOn(source)).includes('jest/prefer-to-have-length'))
  })

  test('stays silent where expect is a local of the file, not an assertion', async () => {
    const source = `declare const expect: (v: unknown) => { toBe: (v: unknown) => void }\n${declarations}${violation}`
    assert.ok(!(await firedOn(source)).includes('jest/prefer-to-have-length'))
  })
})

describe('the fixture package lints clean', () => {
  const reportsFrom = async (instance) => {
    const results = await instance.lintFiles([join(fixturePackage, 'src')])
    return results.flatMap((r) =>
      r.messages.map((m) => `${r.filePath}: ${m.ruleId ?? m.message}`),
    )
  }

  test('no report on any compliant fixture file', async () => {
    assert.deepEqual(await reportsFrom(eslint), [])
  })

  test('and none with the typed layer on top', async () => {
    assert.deepEqual(await reportsFrom(typedEslint), [])
  })
})
