import { readdirSync } from 'node:fs'
import { join, resolve } from 'node:path'
import tsParser from '@typescript-eslint/parser'
import { ESLint } from 'eslint'
import jest from 'eslint-plugin-jest'
import { testFiles } from './globs.js'

const CANDIDATES = {
  // A5 family: the assertion or the whole test never runs
  'jest/expect-expect': 'error',
  'jest/no-focused-tests': 'error',
  'jest/no-disabled-tests': 'warn',
  'jest/no-standalone-expect': 'error',
  'jest/valid-expect-in-promise': 'error',
  'jest/no-commented-out-tests': 'warn',
  'jest/no-conditional-in-test': 'warn',
  // A2 family: the matcher does not name the expectation
  'jest/no-alias-methods': 'error',
  'jest/prefer-to-have-been-called': 'error',
  'jest/prefer-to-have-been-called-times': 'error',
  'jest/require-to-throw-message': 'warn',
  'jest/prefer-mock-promise-shorthand': 'warn',
  'jest/prefer-spy-on': 'warn',
  // hygiene, no current guideline
  'jest/no-identical-title': 'error',
  'jest/valid-title': 'warn',
  'jest/no-duplicate-hooks': 'warn',
  'jest/prefer-each': 'warn',
}

const root = resolve(process.argv[2])
const TEST_FILE = /\.(test|spec)\.(ts|tsx|js|jsx|mts|cts|mjs|cjs)$/
function* walk(dir) {
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    if (e.name === 'node_modules' || e.name.startsWith('.')) continue
    const p = join(dir, e.name)
    if (e.isDirectory()) yield* walk(p); else if (TEST_FILE.test(e.name)) yield p
  }
}
const files = [...walk(root)]
const eslint = new ESLint({
  cwd: root, ignore: false, overrideConfigFile: true,
  overrideConfig: [
    { files: ['**/*.{ts,tsx,js,jsx,mts,cts,mjs,cjs}'], languageOptions: { parser: tsParser, parserOptions: { ecmaFeatures: { jsx: true }, sourceType: 'module' } } },
    { files: testFiles, plugins: { jest }, settings: { jest: { globalPackage: 'vitest' } }, rules: CANDIDATES },
  ],
})
const counts = new Map(); const hits = new Map()
for (const r of await eslint.lintFiles(files)) {
  for (const m of r.messages) {
    const id = m.ruleId ?? `fatal: ${m.message.slice(0,70)}`
    counts.set(id, (counts.get(id) ?? 0) + 1)
    hits.set(id, (hits.get(id) ?? new Set()).add(r.filePath))
  }
}
console.log(`${files.length} test files\n${'hits'.padStart(7)} ${'files'.padStart(6)}  rule`)
for (const [id, n] of [...counts].sort((a,b)=>b[1]-a[1])) console.log(`${String(n).padStart(7)} ${String(hits.get(id).size).padStart(6)}  ${id}`)
