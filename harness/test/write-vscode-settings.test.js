/**
 * The editor layer is the one half of the harness with no live consumer on this machine —
 * nothing installed reads `.vscode/settings.json`'s ESLint keys — so its output has to be
 * gated here or a regression is invisible until someone installs the extension.
 */
import { execFileSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const script = join(harness, 'write-vscode-settings.mjs')
const layer = join(harness, 'eslint-layer.js')

/** @param {string | null} existing raw contents of a pre-existing settings.json */
function run(existing, packages = ['src/a', 'src/b']) {
  const repo = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-editor-'))
  try {
    if (existing !== null) {
      mkdirSync(join(repo, '.vscode'), { recursive: true })
      writeFileSync(join(repo, '.vscode', 'settings.json'), existing)
    }
    execFileSync(process.execPath, [script, repo, ...packages.map((p) => join(repo, p))], {
      encoding: 'utf8',
    })
    return JSON.parse(readFileSync(join(repo, '.vscode', 'settings.json'), 'utf8'))
  } finally {
    rmSync(repo, { recursive: true, force: true })
  }
}

test('points the extension at the layering wrapper by absolute path', () => {
  const settings = run(null)
  assert.equal(settings['eslint.options'].overrideConfigFile, layer)
  assert.equal(settings['eslint.useFlatConfig'], true)
})

test('working directories change the process cwd, which the wrapper reads', () => {
  const settings = run(null, ['src/b', 'src/a'])
  assert.deepEqual(settings['eslint.workingDirectories'], [
    { directory: './src/a', changeProcessCWD: true },
    { directory: './src/b', changeProcessCWD: true },
  ])
})

test('drops the hand-copied selectors the core-rule-only layer used to write', () => {
  const settings = run(
    JSON.stringify({
      'eslint.options': { overrideConfig: { rules: { 'no-restricted-syntax': ['error', {}] } } },
    }),
  )
  assert.equal(settings['eslint.options'].overrideConfig, undefined)
})

test('keeps any other overrideConfig rule the user set by hand', () => {
  const settings = run(
    JSON.stringify({
      'eslint.options': {
        overrideConfig: { rules: { 'no-restricted-syntax': ['error', {}], 'no-debugger': 'error' } },
      },
    }),
  )
  assert.deepEqual(settings['eslint.options'].overrideConfig.rules, { 'no-debugger': 'error' })
})

test('preserves unrelated settings and tolerates JSONC comments', () => {
  const settings = run('{\n  // a comment\n  "editor.formatOnSave": true\n}\n')
  assert.equal(settings['editor.formatOnSave'], true)
  assert.equal(settings['eslint.options'].overrideConfigFile, layer)
})
