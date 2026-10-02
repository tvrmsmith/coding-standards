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
import { ARM_WORLD_UPSTREAM, armSummaryWorld, buildWorld } from './fixture.js'

const CLI = join(dirname(fileURLToPath(import.meta.url)), '..', 'cli.mjs')

/**
 * @param {string[]} args
 * @param {Record<string, string>} [env] replaces the parent's environment, so no default path leaks in
 */
function cli(args, env = {}) {
  return spawnSync(process.execPath, [CLI, ...args], { encoding: 'utf8', env: { PATH: process.env.PATH, ...env } })
}

/**
 * @param {import('node:test').TestContext} t
 * @returns {string} an empty directory to serve as HOME, so no default path finds real data
 */
function emptyHome(t) {
  const home = realpathSync(mkdtempSync(join(tmpdir(), 'guidance-ab-home-')))
  t.after(() => rmSync(home, { recursive: true, force: true }))
  return home
}

/** A world whose every source exists, each with a row count of its own. */
function fullWorld() {
  const lint = [{ ts: 10, repo: ARM_WORLD_UPSTREAM, branch: 'feat-b', head: 'h1', lang: 'ts', mode: 'staged', gate: false, blocked: false, findings: {} }]
  const reviewAb = [
    { kind: 'assign', run: 'ab-1', ts: 100, repo: 'run-feat-b', head: 'abc', arms: {} },
    { kind: 'result', run: 'ab-1', ts: 101, aspect: 'standards', arm: 'general-purpose', critical: 0, important: 0, suggestion: 0, tokens: {} },
  ]
  return armSummaryWorld({ lint, reviewAb })
}

const FULL_WORLD_SOURCES = {
  projects: { present: true, rows: 4, skipped: 0 },
  armLog: { present: true, rows: 4, skipped: 0 },
  lintLog: { present: true, rows: 1, skipped: 0 },
  reviewAb: { present: true, rows: 2, skipped: 0 },
}

test('--json prints the report as JSON with every source path taken from its option', (t) => {
  const paths = buildWorld(t, fullWorld())
  const result = cli([
    'arm', '--json',
    '--state-dir', paths.stateDir,
    '--no-mistakes-home', paths.noMistakesHome,
    '--review-ab', paths.reviewAb,
    '--projects', paths.projectsDir,
  ], { HOME: emptyHome(t) })

  assert.equal(result.status, 0, result.stderr)
  const report = JSON.parse(result.stdout)
  assert.equal(report.mode, 'arm')
  assert.deepEqual(report.sources, FULL_WORLD_SOURCES)
})

test('without options the sources come from the default locations under HOME and XDG_STATE_HOME', (t) => {
  const paths = buildWorld(t, fullWorld())
  const home = emptyHome(t)
  const stateHome = join(home, 'xdg')
  mkdirSync(join(home, '.claude'), { recursive: true })
  mkdirSync(stateHome)
  symlinkSync(paths.noMistakesHome, join(home, '.no-mistakes'))
  symlinkSync(paths.projectsDir, join(home, '.claude', 'projects'))
  symlinkSync(paths.reviewAb, join(home, '.claude', 'review-ab.jsonl'))
  symlinkSync(paths.stateDir, join(stateHome, 'coding-standards'))
  const result = cli(['history', '--json'], { HOME: home, XDG_STATE_HOME: stateHome })

  assert.equal(result.status, 0, result.stderr)
  const report = JSON.parse(result.stdout)
  assert.equal(report.mode, 'history')
  assert.deepEqual(report.sources, FULL_WORLD_SOURCES)
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
