/**
 * `render` turns a report into plain text: the title, caveat, sources, compliance, totals table, cuts,
 * exclusions, arm check and verdict line, with numbers trimmed to two decimals.
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { analyze } from '../analyze.mjs'
import { render } from '../render.mjs'
import { armSummaryWorld, buildWorld } from './fixture.js'

/**
 * @param {import('node:test').TestContext} t
 * @param {'arm' | 'history'} mode
 */
function renderedWorld(t, mode) {
  return render(analyze({ mode, paths: buildWorld(t, armSummaryWorld()) }))
}

test('an arm report names its mode, compliance, metric rows and an unset decision rule', (t) => {
  const text = renderedWorld(t, 'arm')

  assert.match(text, /mode: arm/)
  assert.match(text, /guidance: 2\/2 loaded \(0 unknown\)/)
  assert.match(text, /^reviewError .*3 \[3, 3\].*-2 \[-2, -2\]$/m)
  assert.match(text, /Decision rule: not set/)
  assert.doesNotMatch(text, /self-selected/)
})

test('a history report prints its self-selection caveat under a history title', (t) => {
  const text = renderedWorld(t, 'history')

  assert.match(text, /mode: history/)
  assert.match(text, /self-selected/)
})

test('the sources section names each input it did not find and counts the lines it skipped', (t) => {
  const world = armSummaryWorld({ reviewAb: ['{"kind":"result",'] })
  const text = render(analyze({ mode: 'arm', paths: buildWorld(t, world) }))

  assert.match(text, /^Sources\nprojects: 4 rows\narmLog: 4 rows\nlintLog: not found\nreviewAb: 0 rows, 1 skipped$/m)
})

test('numbers print with at most two decimals, trailing zeros trimmed, and null as n/a', () => {
  const cell = (/** @type {number | null} */ median) => ({ n: 1, median, medianCi: null, mean: null, meanCi: null, shareNonzero: null })
  const diff = { median: null, medianCi: null }
  const text = render({
    mode: 'arm',
    groups: ['guidance', 'review-only'],
    caveat: null,
    compliance: { guidance: { loaded: 0, total: 0, unknown: 0 }, 'review-only': { loaded: 0, total: 0, unknown: 0 } },
    summary: {
      guidance: { reviewError: cell(2.3456), reviewWarning: cell(2.5) },
      'review-only': { reviewError: cell(null), reviewWarning: cell(0) },
    },
    diff: { reviewError: diff, reviewWarning: diff },
    byStratum: {}, byAspect: {}, byReviewAgent: {}, bySection: {},
    excluded: [],
    armCheck: { ratio: { guidance: 0, 'review-only': 0 }, hashMismatches: [] },
    verdict: { rule: null, outcome: 'not-set', detail: '' },
    sources: {},
  })
  const line = (/** @type {string} */ metric) => text.split('\n').find((candidate) => candidate.startsWith(metric))

  assert.match(line('reviewError'), /2\.35 \[n\/a, n\/a\]\s+n\/a \[n\/a, n\/a\]/)
  assert.match(line('reviewWarning'), /2\.5 \[n\/a, n\/a\]/)
})

test('exclusions, arm check and a set verdict each print their lines', (t) => {
  const world = armSummaryWorld()
  const rule = { metric: 'reviewError', statistic: 'median', favour: 'lower', margin: 1 }
  const text = render(analyze({ mode: 'arm', paths: buildWorld(t, { ...world, arms: [...world.arms, { ts: 1000, session_id: 's-x', repo: world.arms[0].repo, branch: 'main', arm: 'unassigned' }] }), rule }))

  assert.match(text, /^Excluded$/m)
  assert.match(text, /unassigned: no group 1/)
  assert.match(text, /arm ratio: guidance 2, review-only 2/)
  assert.match(text, /hash mismatches: 0/)
  assert.match(text, /Decision rule: review-only: reviewError median/)
})

test('each cut with data prints a titled table of its rows', (t) => {
  const workload = { 'feat-b': 40, 'feat-e': 600, 'feat-a': 50, 'feat-c': 700 }
  const world = armSummaryWorld({ run: (name) => ({ invocations: [{ workloadLines: workload[name] }] }) })
  const text = render(analyze({ mode: 'arm', paths: buildWorld(t, world) }))

  assert.match(text, /^By stratum$/m)
  assert.match(text, /^small reviewError .*3 \[3, 3\]/m)
})
