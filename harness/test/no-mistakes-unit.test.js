/**
 * Which root-module packages tools/no-mistakes-unit.sh tests for a change, read through its
 * `select` mode, which prints the set `root` would hand to go test and runs nothing.
 *
 * A wrong selection still exits 0, only with fewer packages tested, so the set itself is pinned.
 * The cases run against this repository's own module, which needs go on PATH, so they skip
 * without it.
 */
import { execFileSync, spawnSync } from 'node:child_process'
import { realpathSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import assert from 'node:assert/strict'

const repo = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..', '..'))
const script = join(repo, 'tools', 'no-mistakes-unit.sh')
const module = 'github.com/tvrmsmith/coding-standards'
const skip = spawnSync('go', ['version'], { stdio: 'ignore' }).status !== 0 && 'no go on PATH'

/**
 * @param {string} unit
 * @param {string[]} files the changed paths no-mistakes lists
 * @param {number} count the changed-file count, larger than `files` when the list was dropped
 */
function select(unit, files, count = files.length) {
  const out = execFileSync(script, ['select', unit], {
    cwd: repo,
    encoding: 'utf8',
    env: {
      ...process.env,
      NO_MISTAKES_CHANGED_FILES: files.join('\n'),
      NO_MISTAKES_CHANGED_FILE_COUNT: String(count),
    },
  })
  return out.split('\n').filter(Boolean)
}

const pkg = (/** @type {string} */ path) => `${module}/${path}`

test('a dropped changed list selects the whole module', { skip }, () => {
  assert.deepEqual(select('gate', [], 3), ['./...'])
})

test('a changed file in no package selects every package under the unit', { skip }, () => {
  assert.deepEqual(select('internal', ['internal/notes.txt']), ['./internal/...'])
})

test('a changed package selects itself, its importers, and the tests that build them', { skip }, () => {
  const selected = select('internal', ['internal/gitscope/gitscope.go'])
  for (const expected of ['internal/gitscope', 'lint/cmd/lint-changed', 'lint/test', 'gate/test']) {
    assert.ok(selected.includes(pkg(expected)), `${expected} missing from ${selected}`)
  }
  assert.ok(!selected.includes(pkg('internal/srcpath')), 'srcpath does not import gitscope')
})

test('test variant names reduce to the package they test', { skip }, () => {
  const selected = select('gate', ['gate/test/stub/main.go'])
  assert.deepEqual(selected, [pkg('gate/test'), pkg('gate/test/stub')])
})

test('two changed packages select the union of their dependents', { skip }, () => {
  const selected = select('internal', ['internal/gitscope/a.go', 'internal/srcpath/b.go'])
  for (const expected of ['internal/gitscope', 'internal/srcpath', 'lint/test', 'gate/test']) {
    assert.ok(selected.includes(pkg(expected)), `${expected} missing from ${selected}`)
  }
})

test('changed files outside the unit do not widen its selection', { skip }, () => {
  assert.deepEqual(
    select('internal', ['README.md', 'gate/internal/join/join.go', 'internal/srcpath/b.go']),
    select('internal', ['internal/srcpath/b.go']),
  )
})
