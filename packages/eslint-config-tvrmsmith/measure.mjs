#!/usr/bin/env node
/**
 * Count what the preset would report against a target repo, by rule.
 *
 * ```sh
 * pnpm measure ~/dev/target-monorepo
 * pnpm measure ~/dev/target-monorepo jest/        # one rule-id prefix only
 * ```
 *
 * Lives here rather than in `harness/` because it needs a parser and an ESLint of its own,
 * and the harness deliberately has neither — it runs each target package's own binary.
 *
 * Blast radius before adoption, and the measurement a custom-rule proposal has to clear:
 * a shape earns a rule from a counted frequency, not a hypothesised one.
 *
 * This runs the preset **alone**, resolving everything from the hub — it does not layer the
 * target's own config the way `lint-changed.sh` does, and it does not need the target's
 * `node_modules` to be installed. So the counts are the personal layer's, which is the
 * question being asked, and they hold on a target whose pnpm store has been pruned.
 */
import { readdirSync, statSync } from 'node:fs'
import { join, resolve } from 'node:path'

import tsParser from '@typescript-eslint/parser'
import { ESLint } from 'eslint'
import preset from './index.js'

const [target, prefix = ''] = process.argv.slice(2)
if (!target) {
  console.error('usage: pnpm measure <target-repo> [rule-id-prefix]')
  process.exit(2)
}
const root = resolve(target)

const TEST_FILE = /\.(test|spec)\.(ts|tsx|js|jsx|mts|cts|mjs|cjs)$/

/** `node_modules` dwarfs the tree and none of it is the target's own code. */
function* walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name.startsWith('.')) continue
    const path = join(dir, entry.name)
    if (entry.isDirectory()) yield* walk(path)
    else if (TEST_FILE.test(entry.name)) yield path
  }
}

const files = [...walk(root)]
if (files.length === 0) {
  console.error(`no test files under ${root}`)
  process.exit(1)
}

const eslint = new ESLint({
  cwd: root,
  ignore: false,
  overrideConfigFile: true,
  overrideConfig: [
    {
      files: ['**/*.{ts,tsx,js,jsx,mts,cts,mjs,cjs}'],
      languageOptions: {
        parser: tsParser,
        parserOptions: { ecmaFeatures: { jsx: true }, sourceType: 'module' },
      },
    },
    ...preset,
  ],
})

const counts = new Map()
const filesHit = new Map()
for (const result of await eslint.lintFiles(files)) {
  for (const message of result.messages) {
    const id = message.ruleId ?? `fatal: ${message.message}`
    if (!id.startsWith(prefix)) continue
    counts.set(id, (counts.get(id) ?? 0) + 1)
    filesHit.set(id, (filesHit.get(id) ?? new Set()).add(result.filePath))
  }
}

console.log(`${files.length} test files under ${root}\n`)
console.log(`${'hits'.padStart(7)} ${'files'.padStart(6)}  rule`)
for (const [id, n] of [...counts].sort((a, b) => b[1] - a[1])) {
  console.log(`${String(n).padStart(7)} ${String(filesHit.get(id).size).padStart(6)}  ${id}`)
}
