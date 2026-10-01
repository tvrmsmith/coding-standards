/**
 * The `cli.mjs` command: its modes, option defaults, output formats and exit codes, run as a child
 * process the way a user runs it.
 */
import { spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, realpathSync, rmSync, symlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import assert from 'node:assert/strict'
import { armSummaryWorld, buildWorld } from './fixture.js'

const CLI = join(dirname(fileURLToPath(import.meta.url)), '..', 'cli.mjs')

/**
 * @param {string[]} args
 * @param {Record<string, string>} [env] replaces the parent's environment, so no default path leaks in
 */
function cli(args, env = {}) {
  return spawnSync(process.execPath, [CLI, ...args], { encoding: 'utf8', env: { PATH: process.env.PATH, ...env } })
}

test('--json prints the report as JSON with every source path taken from its option', (t) => {
  const paths = buildWorld(t, armSummaryWorld())
  const result = cli([
    'arm', '--json',
    '--state-dir', paths.stateDir,
    '--no-mistakes-home', paths.noMistakesHome,
    '--review-ab', paths.reviewAb,
    '--projects', paths.projectsDir,
  ])

  assert.equal(result.status, 0, result.stderr)
  assert.equal(JSON.parse(result.stdout).mode, 'arm')
})

test('without options the sources come from the default locations under HOME and XDG_STATE_HOME', (t) => {
  const paths = buildWorld(t, armSummaryWorld())
  const home = realpathSync(mkdtempSync(join(tmpdir(), 'guidance-ab-home-')))
  t.after(() => rmSync(home, { recursive: true, force: true }))
  const stateHome = join(home, 'xdg')
  mkdirSync(join(home, '.claude'), { recursive: true })
  mkdirSync(stateHome)
  symlinkSync(paths.noMistakesHome, join(home, '.no-mistakes'))
  symlinkSync(paths.projectsDir, join(home, '.claude', 'projects'))
  symlinkSync(paths.stateDir, join(stateHome, 'coding-standards'))
  const result = cli(['history'], { HOME: home, XDG_STATE_HOME: stateHome })

  assert.equal(result.status, 0, result.stderr)
  assert.ok(result.stdout.includes('mode: history'), result.stdout)
})

test('a missing or unknown mode prints usage and exits 2', () => {
  for (const args of [[], ['nope']]) {
    const result = cli(args)

    assert.equal(result.status, 2)
    assert.match(result.stderr, /usage/i)
  }
})

test('an analysis error prints its message to stderr and exits 1', (t) => {
  const paths = buildWorld(t, { ...armSummaryWorld(), arms: undefined })
  const result = cli(['arm', '--state-dir', paths.stateDir, '--no-mistakes-home', paths.noMistakesHome, '--projects', paths.projectsDir])

  assert.equal(result.status, 1)
  assert.ok(result.stderr.includes('arms.jsonl'), result.stderr)
})
