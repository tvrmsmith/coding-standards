import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import test, { describe } from 'node:test'

import tsParser from '@typescript-eslint/parser'
import { ESLint } from 'eslint'

import base from '../base.js'
import preset from '../index.js'
import react from '../react.js'
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

const eslint = new ESLint({
  cwd: fixturePackage,
  overrideConfigFile: true,
  overrideConfig: [packageOwnConfig, ...preset],
})

const filePathFor = (scope) =>
  join(fixturePackage, scope === 'test' ? 'src/sample.test.tsx' : 'src/Sample.tsx')

async function reportsOn(code, scope) {
  const [result] = await eslint.lintText(preambles[scope] + code, {
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

/** Every rule the preset enables, flattened from the slices themselves. */
const enabledRules = new Set(preset.flatMap((config) => Object.keys(config.rules ?? {})))

describe('every enabled rule has a fixture case', () => {
  test('the case list and the preset name the same rules', () => {
    const covered = new Set(cases.map((c) => c.rule))
    assert.deepEqual(
      [...enabledRules].filter((r) => !covered.has(r)),
      [],
      'enabled rules with no fixture case',
    )
    assert.deepEqual(
      [...covered].filter((r) => !enabledRules.has(r)),
      [],
      'fixture cases for rules the preset does not enable',
    )
  })
})

describe('each rule fires on a violation and stays quiet on compliant code', () => {
  for (const testCase of cases) {
    const { scope, violating, compliant } = testCase
    const matches = matcherFor(testCase)
    const label = describeCase(testCase)

    test(`${label} fires`, async () => {
      const reports = await reportsOn(violating, scope)
      assert.ok(
        reports.some(matches),
        `expected ${label}, got: ${reports.map((r) => r.rule).join(', ') || 'nothing'}`,
      )
    })

    test(`${label} stays quiet`, async () => {
      const reports = await reportsOn(compliant, scope)
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
    // Deduped: `jest` is registered by two slices (assertion-intent and test-integrity),
    // which flat config permits because both spread the same imported plugin object.
    const plugins = [
      ...new Set(base.flatMap((config) => Object.keys(config.plugins ?? {}))),
    ].sort()
    assert.deepEqual(plugins, ['@typescript-eslint', 'jest', 'tvrmsmith'])
  })

  test('react carries only the React and RTL plugins', () => {
    const plugins = react.flatMap((config) => Object.keys(config.plugins ?? {})).sort()
    assert.deepEqual(plugins, [
      'jest-dom',
      'react-hooks',
      'react-you-might-not-need-an-effect',
      'testing-library',
    ])
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
  test('no report on any compliant fixture file', async () => {
    const results = await eslint.lintFiles([join(fixturePackage, 'src')])
    const reports = results.flatMap((r) =>
      r.messages.map((m) => `${r.filePath}: ${m.ruleId ?? m.message}`),
    )
    assert.deepEqual(reports, [])
  })
})
