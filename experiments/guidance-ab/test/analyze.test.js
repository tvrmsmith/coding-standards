/**
 * History mode of `analyze`: which no-mistakes branches become units, what each unit measures from
 * its pipeline runs, and why a branch is excluded instead.
 *
 * Each case builds a throwaway world through the fixture builder: a no-mistakes state db, bare
 * repos, and Claude Code transcripts. The shared world is one repo, `acme/widgets`, whose branch
 * `feat-a` changes a TypeScript file and a doc, with one transcript session that loads the
 * coding-standards skill. A case overrides only the part it is about.
 */
import { join } from 'node:path'
import test from 'node:test'
import assert from 'node:assert/strict'
import { analyze } from '../analyze.mjs'
import { armSummaryWorld, buildWorld } from './fixture.js'

const LOADS_STANDARDS = ['coding-standards:coding-standards']

/**
 * @param {Partial<import('./fixture.js').Repo>} [repo] overrides for the widgets repo
 * @param {{ sessions?: import('./fixture.js').Session[] }} [rest]
 * @returns {import('./fixture.js').World}
 */
function widgets(repo = {}, { sessions = [widgetsSession()] } = {}) {
  return { repos: [widgetsRepo(repo)], sessions }
}

/**
 * @param {Partial<import('./fixture.js').Repo>} [overrides]
 * @returns {import('./fixture.js').Repo}
 */
function widgetsRepo(overrides = {}) {
  return {
    id: 'r1',
    upstream: 'git@github-personal:Acme/Widgets.git',
    workingPath: 'checkouts/widgets',
    branches: { 'feat-a': { files: { 'src/app.ts': 3, 'docs/notes.md': 2 } } },
    runs: [reviewedRun('run1', 1000, [])],
    ...overrides,
  }
}

/** @param {Partial<import('./fixture.js').Session>} [overrides] */
function widgetsSession(overrides = {}) {
  return { id: 's1', cwd: 'checkouts/widgets', branch: 'feat-a', skills: LOADS_STANDARDS, ...overrides }
}

/**
 * A feat-a run whose review step has the given rounds.
 *
 * @param {string} id
 * @param {number} createdAt
 * @param {...import('./fixture.js').Round} rounds
 * @returns {import('./fixture.js').Run}
 */
function reviewedRun(id, createdAt, ...rounds) {
  return { id, branch: 'feat-a', createdAt, version: '1.2.3', steps: { review: rounds } }
}

/**
 * @param {import('node:test').TestContext} t
 * @param {import('./fixture.js').World} world
 */
function history(t, world) {
  return analyze({ mode: 'history', paths: buildWorld(t, world) })
}

const TRACER_ROUND = [
  { id: 'missing-null-check', severity: 'error' },
  { id: 'naming', severity: 'warning' },
  { id: 'naming-again', severity: 'warning' },
  { id: 'nit', severity: 'info' },
]

test('a reviewed branch whose session loaded the standards is a loaded unit', (t) => {
  const report = history(t, widgets({ runs: [reviewedRun('run1', 1000, TRACER_ROUND)] }))

  assert.equal(report.mode, 'history')
  assert.deepEqual(report.groups, ['loaded', 'not-loaded'])
  assert.match(report.caveat, /self-selected/)
  assert.equal(report.branches.length, 1)
  const [branch] = report.branches
  assert.deepEqual(
    { repo: branch.repo, branch: branch.branch, group: branch.group, compliance: branch.compliance },
    { repo: 'acme/widgets', branch: 'feat-a', group: 'loaded', compliance: true },
  )
  assert.equal(branch.noMistakesVersion, '1.2.3')
  assert.deepEqual(
    { error: branch.metrics.reviewError, warning: branch.metrics.reviewWarning, info: branch.metrics.reviewInfo },
    { error: 1, warning: 2, info: 1 },
  )
})

test('a reviewed branch whose session never loaded the standards is a not-loaded unit', (t) => {
  const report = history(t, widgets({}, { sessions: [widgetsSession({ skills: [] })] }))

  assert.deepEqual(
    report.branches.map(({ group, compliance }) => ({ group, compliance })),
    [{ group: 'not-loaded', compliance: false }],
  )
})

test('a reviewed branch with no transcript is excluded', (t) => {
  const report = history(t, widgets({}, { sessions: [] }))

  assert.deepEqual(report.branches, [])
  assert.deepEqual(report.excluded, [{ repo: 'acme/widgets', branch: 'feat-a', group: null, reason: 'no transcript' }])
})

test('unique findings count each finding once at its highest severity across the first run', (t) => {
  const round2 = [
    { id: 'missing-null-check', severity: 'error' },
    { id: 'naming', severity: 'error' },
    { id: 'new-issue', severity: 'info' },
  ]
  const report = history(t, widgets({ runs: [reviewedRun('run1', 1000, TRACER_ROUND, round2, [])] }))

  const { metrics } = report.branches[0]
  assert.deepEqual(
    {
      error: metrics.reviewUniqueError,
      warning: metrics.reviewUniqueWarning,
      info: metrics.reviewUniqueInfo,
      rounds: metrics.reviewRounds,
    },
    { error: 2, warning: 1, info: 2, rounds: 3 },
  )
  assert.deepEqual(
    { error: metrics.reviewError, warning: metrics.reviewWarning, info: metrics.reviewInfo },
    { error: 1, warning: 2, info: 1 },
  )
})

test('a generic finding id is identified by its file and description instead', (t) => {
  const round1 = [{ id: 'review-1', severity: 'warning', file: 'src/app.ts', description: 'A' }]
  const round2 = [
    { id: 'review-1', severity: 'warning', file: 'src/app.ts', description: 'B' },
    { id: 'review-2', severity: 'info', file: 'src/app.ts', description: 'A' },
  ]
  const report = history(t, widgets({ runs: [reviewedRun('run1', 1000, round1, round2)] }))

  const { metrics } = report.branches[0]
  assert.deepEqual(
    { warning: metrics.reviewUniqueWarning, info: metrics.reviewUniqueInfo },
    { warning: 2, info: 0 },
  )
})

test('the first run is the earliest run with a review round 1', (t) => {
  const warning = [{ id: 'w1', severity: 'warning' }]
  const errors = ['e1', 'e2', 'e3', 'e4', 'e5'].map((id) => ({ id, severity: 'error' }))
  const runs = [
    { id: 'run0', branch: 'feat-a', createdAt: 900, status: 'cancelled' },
    reviewedRun('run1', 1000, warning),
    reviewedRun('run2', 2000, errors),
  ]
  const report = history(t, widgets({ runs }))

  const { metrics } = report.branches[0]
  assert.deepEqual(
    { error: metrics.reviewError, warning: metrics.reviewWarning, rounds: metrics.reviewRounds },
    { error: 0, warning: 1, rounds: 2 },
  )
})

/**
 * @param {import('node:test').TestContext} t
 * @param {import('./fixture.js').Round[]} [lint] the first run's lint rounds, or no lint step
 */
function lintFirstRun(t, lint) {
  const run = reviewedRun('run1', 1000, [])
  if (lint) run.steps = { ...run.steps, lint }
  return history(t, widgets({ runs: [run] })).branches[0].metrics.lintFirstRun
}

test('lint first run counts the findings in the first run lint round 1', (t) => {
  const findings = [
    { id: 'l1', severity: 'warning' },
    { id: 'l2', severity: 'warning' },
  ]
  assert.equal(lintFirstRun(t, [findings]), 2)
})

test('a lint round 1 with null findings counts zero', (t) => {
  assert.equal(lintFirstRun(t, [null]), 0)
})

test('a first run with no lint step has no lint first run', (t) => {
  assert.equal(lintFirstRun(t), null)
})

test('pipeline tokens sum every invocation on the branch and price them', (t) => {
  const run1 = {
    ...reviewedRun('run1', 1000, []),
    invocations: [{ input: 10, output: 20, cacheRead: 1000, cacheWrite: 100 }, {}],
  }
  const run2 = { ...reviewedRun('run2', 2000, []), invocations: [{ input: 5, output: 10, cacheRead: 0, cacheWrite: 0 }] }
  const report = history(t, widgets({ runs: [run1, run2] }))

  assert.deepEqual(report.branches[0].metrics.pipelineTokens, {
    input: 15,
    cacheWrite: 100,
    cacheRead: 1000,
    output: 30,
    cost: 390,
  })
})

test('pipeline tokens are null when every invocation lacks token counts', (t) => {
  const run1 = { ...reviewedRun('run1', 1000, []), invocations: [{}, {}] }
  const report = history(t, widgets({ runs: [run1] }))

  assert.equal(report.branches[0].metrics.pipelineTokens, null)
})

/**
 * @param {import('node:test').TestContext} t
 * @param {import('./fixture.js').Invocation[]} invocations the first run's invocations
 */
function stratum(t, invocations) {
  const run = { ...reviewedRun('run1', 1000, []), invocations }
  return history(t, widgets({ runs: [run] })).branches[0].stratum
}

test('a review under 100 lines is small', (t) => {
  assert.equal(stratum(t, [{ step: 'review', round: 1, purpose: 'review', workloadLines: 40 }]), 'small')
})

test('a review of 100 to 499 lines is medium', (t) => {
  assert.equal(stratum(t, [{ step: 'review', round: 1, purpose: 'review', workloadLines: 250 }]), 'medium')
})

test('a review of 500 lines or more is large', (t) => {
  assert.equal(stratum(t, [{ step: 'review', round: 1, purpose: 'review', workloadLines: 500 }]), 'large')
})

test('a first run with no review round 1 invocation has no stratum', (t) => {
  assert.equal(stratum(t, []), null)
})

test('a branch that changes no code file is excluded', (t) => {
  const branches = { 'feat-a': { files: { 'docs/notes.md': 2, 'package-lock.json': 5 } } }
  const report = history(t, widgets({ branches }))

  assert.deepEqual(report.branches, [])
  assert.deepEqual(report.excluded, [
    { repo: 'acme/widgets', branch: 'feat-a', group: 'loaded', reason: 'no code change' },
  ])
})

test('a branch whose head neither repo holds is excluded as diff unavailable', (t) => {
  const branches = { 'feat-a': { files: { 'src/app.ts': 3, 'docs/notes.md': 2 }, inBare: false } }
  const report = history(t, widgets({ branches }))

  assert.deepEqual(report.branches, [])
  assert.deepEqual(report.excluded, [
    { repo: 'acme/widgets', branch: 'feat-a', group: 'loaded', reason: 'diff unavailable' },
  ])
})

test('a head missing from the bare repo is diffed in the checkout instead', (t) => {
  const branches = { 'feat-a': { files: { 'src/app.ts': 3, 'docs/notes.md': 2 }, inBare: false } }
  const report = history(t, widgets({ branches, checkout: true }))

  assert.deepEqual(
    report.branches.map(({ repo, branch }) => ({ repo, branch })),
    [{ repo: 'acme/widgets', branch: 'feat-a' }],
  )
})

test('a repo checked out under a mktemp directory is excluded as a sandbox', (t) => {
  const workingPath = 'tmp.AbC123/widgets'
  const report = history(t, widgets({ workingPath }, { sessions: [widgetsSession({ cwd: workingPath })] }))

  assert.deepEqual(
    report.excluded.map(({ reason }) => reason),
    ['sandbox repo'],
  )
})

test('a branch with a run still going is excluded as in flight', (t) => {
  const runs = [reviewedRun('run1', 1000, []), { ...reviewedRun('run2', 2000, []), status: 'running' }]
  const report = history(t, widgets({ runs }))

  assert.deepEqual(
    report.excluded.map(({ reason }) => reason),
    ['in flight'],
  )
})

test('https and ssh upstream URLs give the same owner/name key', (t) => {
  const gadgets = 'checkouts/gadgets'
  const gizmos = 'checkouts/gizmos'
  const report = history(t, {
    repos: [
      widgetsRepo(),
      widgetsRepo({ id: 'r2', upstream: 'https://github.com/Acme/Gadgets.git', workingPath: gadgets, runs: [reviewedRun('run2', 1000, [])] }),
      widgetsRepo({ id: 'r3', upstream: 'ssh://git@github.com/acme/gizmos', workingPath: gizmos, runs: [reviewedRun('run3', 1000, [])] }),
    ],
    sessions: [widgetsSession(), widgetsSession({ id: 's2', cwd: gadgets }), widgetsSession({ id: 's3', cwd: gizmos })],
  })

  assert.deepEqual(report.branches.map(({ repo }) => repo).sort(), ['acme/gadgets', 'acme/gizmos', 'acme/widgets'])
})

test('a missing state db throws naming its path', (t) => {
  const paths = buildWorld(t, { db: false })

  assert.throws(() => analyze({ mode: 'history', paths }), (error) => {
    assert.ok(error instanceof Error)
    assert.ok(error.message.includes(join(paths.noMistakesHome, 'state.sqlite')), error.message)
    return true
  })
})

/** Epoch seconds of 2026-09-21T14:13:20Z, the creation of the first run in the transcript cases. */
const FIRST_RUN_AT = 1790000000

/**
 * The shared world of the transcript cases: `feat-a` with one run and a session on its checkout.
 *
 * @param {Partial<import('./fixture.js').Session>} session
 * @param {Partial<import('./fixture.js').Repo>} [repo]
 * @returns {import('./fixture.js').World}
 */
function transcriptWorld(session, repo = {}) {
  const run = reviewedRun('run1', FIRST_RUN_AT, [{ id: 'naming', severity: 'warning' }])
  return widgets({ runs: [run], ...repo }, { sessions: [widgetsSession({ skills: [], ...session })] })
}

const M1 = { messageId: 'm1', usage: { input: 10, output: 100, cacheRead: 1000, cacheWrite: 200 } }
const M2 = { messageId: 'm2', usage: { input: 1, output: 2 } }

test('implementation tokens count a message streamed over several lines once', (t) => {
  const report = history(t, transcriptWorld({ entries: [{ ...M1, repeat: 3 }, M2] }))

  assert.deepEqual(report.branches[0].metrics.implTokens, {
    input: 11,
    cacheWrite: 200,
    cacheRead: 1000,
    output: 102,
    cost: 871,
  })
})

test('implementation tokens include subagent files and count a message copied into one once', (t) => {
  const worker = { id: 'a1', name: 'worker-1', entries: [{ messageId: 'm3', usage: { input: 4, output: 4 } }, M2] }
  const report = history(t, transcriptWorld({ entries: [{ ...M1, repeat: 3 }, M2], subagents: [worker] }))

  assert.deepEqual(report.branches[0].metrics.implTokens, {
    input: 15,
    cacheWrite: 200,
    cacheRead: 1000,
    output: 106,
    cost: 895,
  })
})

test('total cost adds the implementation and pipeline costs', (t) => {
  const run1 = {
    ...reviewedRun('run1', FIRST_RUN_AT, [{ id: 'naming', severity: 'warning' }]),
    invocations: [{ input: 10, output: 20, cacheRead: 1000, cacheWrite: 100 }],
  }
  const report = history(t, transcriptWorld({ entries: [{ ...M1, repeat: 3 }, M2] }, { runs: [run1] }))

  assert.equal(report.branches[0].metrics.pipelineTokens?.cost, 335)
  assert.equal(report.branches[0].metrics.totalCost, 1206)
})

test('a load after the first run began does not count', (t) => {
  const loadAt = (/** @type {string} */ at) => history(t, transcriptWorld({ entries: [{ at, skills: LOADS_STANDARDS }] }))

  const late = loadAt('2026-09-21T14:13:21.000Z').branches[0]
  assert.equal(late.group, 'not-loaded')
  assert.equal(late.compliance, false)
  assert.equal(loadAt('2026-09-21T14:13:19.000Z').branches[0].group, 'loaded')
})

test('a load inside a review subagent does not count', (t) => {
  const loadIn = (/** @type {string} */ name) => {
    const subagent = { id: 'a1', name, entries: [{ skills: LOADS_STANDARDS }] }
    return history(t, transcriptWorld({ entries: [M2], subagents: [subagent] })).branches[0].group
  }

  assert.equal(loadIn('rev-standards'), 'not-loaded')
  assert.equal(loadIn('worker-1'), 'loaded')
})

test('a no-mistakes gate session directory is never read', (t) => {
  const gate = widgetsSession({
    id: 's2',
    dir: '-Users-someone--no-mistakes-worktrees-r1-run1',
    entries: [{ messageId: 'g1', usage: { input: 1000 }, skills: LOADS_STANDARDS }],
  })
  const own = widgetsSession({ skills: [], entries: [M2] })
  const world = transcriptWorld({}, {})
  const report = history(t, { ...world, sessions: [own, gate] })

  assert.equal(report.branches[0].group, 'not-loaded')
  assert.equal(report.branches[0].metrics.implTokens.input, 1)
})

test('a segment outside every checkout takes its repo from the session pr-link', (t) => {
  const session = { cwd: '/elsewhere/checkout-x', prRepository: 'Acme/Widgets', skills: LOADS_STANDARDS }
  const report = history(t, transcriptWorld(session))

  assert.equal(report.branches[0].branch, 'feat-a')
  assert.equal(report.branches[0].group, 'loaded')
})

test('a segment with no pr-link takes the one repo whose name is a path segment of its cwd', (t) => {
  const session = { cwd: '/Users/someone/dev/worktrees/widgets/feat-a', skills: LOADS_STANDARDS }
  const unique = history(t, transcriptWorld(session))
  assert.equal(unique.branches[0].branch, 'feat-a')

  const other = { id: 'r2', upstream: 'git@github.com:other/widgets.git', workingPath: 'checkouts/other-widgets' }
  const world = transcriptWorld(session)
  const ambiguous = history(t, { ...world, repos: [...(world.repos ?? []), other] })
  assert.deepEqual(ambiguous.excluded.map(({ branch, reason }) => ({ branch, reason })), [
    { branch: 'feat-a', reason: 'no transcript' },
  ])
})

test('a session that switches branches counts each segment on its own branch', (t) => {
  const entries = [
    { messageId: 'a1', usage: { input: 5 }, skills: LOADS_STANDARDS, at: '2026-09-21T14:00:00.000Z' },
    { messageId: 'a2', usage: { input: 7 }, branch: 'main', at: '2026-09-21T14:05:00.000Z' },
  ]
  const branch = history(t, transcriptWorld({ entries })).branches[0]

  assert.equal(branch.group, 'loaded')
  assert.equal(branch.metrics.implTokens.input, 5)
})

test('wall time runs from the first transcript line to the last step completion, less parked time', (t) => {
  const run1 = {
    ...reviewedRun('run1', FIRST_RUN_AT, [{ id: 'naming', severity: 'warning' }]),
    steps: { review: [[{ id: 'naming', severity: 'warning' }]], lint: [[]] },
    completedAt: { review: 1790003600, lint: 1790001000 },
    parkedMs: 600000,
  }
  const run2 = { ...reviewedRun('run2', 1790010000, []), status: 'completed', completedAt: { review: 1790012000 } }
  const entries = [{ at: '2026-09-21T14:00:00.000Z' }, { at: '2026-09-21T14:10:00.000Z' }]
  const report = history(t, transcriptWorld({ entries }, { runs: [run1, run2] }))

  assert.equal(report.branches[0].metrics.wallSeconds, 1790012000 - 1789999200 - 600)
})

test('a truncated line is skipped and counted, and a string message is read without throwing', (t) => {
  const entries = [
    { messageId: 'm1', skills: LOADS_STANDARDS },
    { raw: '{"type":"assistant",' },
    { stringMessage: 'a note, not a message object' },
  ]
  const report = history(t, transcriptWorld({ entries }))

  assert.equal(report.branches[0].group, 'loaded')
  assert.deepEqual(report.sources.projects, { present: true, rows: 1, skipped: 1 })
})

test('a missing projects directory leaves every branch without a transcript', (t) => {
  const report = history(t, { ...transcriptWorld({}), sessions: [] })

  assert.equal(report.sources.projects.present, false)
  assert.deepEqual(report.excluded.map(({ branch, reason }) => ({ branch, reason })), [
    { branch: 'feat-a', reason: 'no transcript' },
  ])
})

/**
 * Arm mode of `analyze`: the units are the branches the arm log names, grouped by the arm the hook
 * assigned. The shared arm world is `widgets` with branches `feat-a` and `feat-b`, each with a run at
 * 2000 and a session that loads the skill at 1500. For the raw upstream
 * `git@github-personal:Acme/Widgets.git`, `feat-a` hashes to `b` (review-only), `feat-b` to `4`
 * (guidance) and `feat-c` to `f` (review-only).
 */
const UPSTREAM = 'git@github-personal:Acme/Widgets.git'
const SEGMENT_AT = '1970-01-01T00:25:00.000Z'

/**
 * @param {string} branch
 * @param {number} ts epoch seconds
 * @param {string} arm
 * @param {string} [sessionId]
 */
function armRow(branch, ts, arm, sessionId = `s-${branch.slice(-1)}`) {
  return { ts, session_id: sessionId, repo: UPSTREAM, branch, arm }
}

/**
 * @param {string} branch
 * @param {number} [createdAt]
 * @returns {import('./fixture.js').Run}
 */
function armRun(branch, createdAt = 2000) {
  return { ...reviewedRun(`run-${branch}`, createdAt, []), branch }
}

/**
 * @param {string} branch
 * @param {Partial<import('./fixture.js').Session>} [overrides]
 * @returns {import('./fixture.js').Session}
 */
function armSession(branch, overrides = {}) {
  const id = `s-${branch.slice(-1)}`
  return { id, cwd: 'checkouts/widgets', branch, entries: [{ at: SEGMENT_AT, skills: LOADS_STANDARDS }], ...overrides }
}

/** @param {string[]} names */
function armBranches(...names) {
  return Object.fromEntries(names.map((name) => [name, { files: { 'src/app.ts': 3 } }]))
}

const ARM_ROWS = [armRow('feat-a', 1000, 'review-only'), armRow('feat-b', 1100, 'guidance')]

/**
 * @param {Partial<import('./fixture.js').World>} [overrides]
 * @returns {import('./fixture.js').World}
 */
function armWorld(overrides = {}) {
  return {
    repos: [widgetsRepo({ branches: armBranches('feat-a', 'feat-b'), runs: [armRun('feat-a'), armRun('feat-b')] })],
    sessions: [armSession('feat-a'), armSession('feat-b')],
    arms: ARM_ROWS,
    ...overrides,
  }
}

/**
 * @param {import('node:test').TestContext} t
 * @param {import('./fixture.js').World} world
 */
function arm(t, world) {
  return analyze({ mode: 'arm', paths: buildWorld(t, world) })
}

test('an arm log groups each branch it names by its assigned arm', (t) => {
  const report = arm(t, armWorld())

  assert.equal(report.mode, 'arm')
  assert.deepEqual(report.groups, ['guidance', 'review-only'])
  assert.equal(report.caveat, null)
  assert.deepEqual(report.branches.map(({ branch, group }) => ({ branch, group })), [
    { branch: 'feat-a', group: 'review-only' },
    { branch: 'feat-b', group: 'guidance' },
  ])
  assert.deepEqual(report.armCheck, { ratio: { guidance: 1, 'review-only': 1 }, hashMismatches: [] })
})

test('a missing arm log throws naming its path', (t) => {
  const paths = buildWorld(t, armWorld({ arms: undefined }))

  assert.throws(() => analyze({ mode: 'arm', paths }), (error) => {
    assert.ok(error.message.includes(join(paths.stateDir, 'arms.jsonl')), error.message)
    return true
  })
})

test('a branch the arm log only marks unassigned is excluded with no group', (t) => {
  const arms = [...ARM_ROWS, armRow('main', 1050, 'unassigned', 's-m'), armRow('main', 1150, 'unassigned', 's-m2')]
  const report = arm(t, armWorld({ arms }))

  assert.deepEqual(report.excluded, [{ repo: 'acme/widgets', branch: 'main', group: null, reason: 'unassigned' }])
  assert.equal(report.branches.length, 2)
})

test('a branch the arm log assigns but no-mistakes never reviewed is excluded as no pipeline run', (t) => {
  const arms = [...ARM_ROWS, armRow('feat-c', 1200, 'review-only')]
  const report = arm(t, armWorld({ arms }))

  assert.deepEqual(report.excluded, [{ repo: 'acme/widgets', branch: 'feat-c', group: 'review-only', reason: 'no pipeline run' }])
})

test('an arm branch whose first run predates the hook is excluded', (t) => {
  const repos = [widgetsRepo({ branches: armBranches('feat-a', 'feat-b'), runs: [armRun('feat-a'), armRun('feat-b', 900)] })]
  const report = arm(t, armWorld({ repos }))

  assert.deepEqual(report.excluded, [{ repo: 'acme/widgets', branch: 'feat-b', group: 'guidance', reason: 'predates hook' }])
})

test('an arm branch whose earliest transcript predates the hook is excluded', (t) => {
  const early = armSession('feat-b', { entries: [{ at: '1970-01-01T00:15:50.000Z', skills: LOADS_STANDARDS }] })
  const report = arm(t, armWorld({ sessions: [armSession('feat-a'), early] }))

  assert.deepEqual(report.excluded, [{ repo: 'acme/widgets', branch: 'feat-b', group: 'guidance', reason: 'predates hook' }])
})

test('an arm branch with no transcript stays in with its transcript metrics null', (t) => {
  const report = arm(t, armWorld({ sessions: [armSession('feat-a')] }))
  const featB = report.branches.find(({ branch }) => branch === 'feat-b')

  assert.equal(featB.group, 'guidance')
  assert.equal(featB.compliance, null)
  assert.equal(featB.metrics.implTokens, null)
  assert.equal(featB.metrics.wallSeconds, null)
  assert.deepEqual(report.excluded, [])
})

test('an arm branch whose session sits outside every checkout takes its repo from the arm log', (t) => {
  const elsewhere = armSession('feat-b', { cwd: '/elsewhere/x' })
  const report = arm(t, armWorld({ sessions: [armSession('feat-a'), elsewhere] }))
  const featB = report.branches.find(({ branch }) => branch === 'feat-b')

  assert.equal(featB.compliance, true)
})

test('a logged arm that differs from the hash of its repo and branch is a hash mismatch', (t) => {
  const arms = [...ARM_ROWS, armRow('feat-a', 1300, 'guidance', 's-a2'), armRow('feat-a', 1400, 'guidance', 's-a3')]
  const report = arm(t, armWorld({ arms }))

  assert.equal(report.branches.find(({ branch }) => branch === 'feat-a').group, 'review-only')
  assert.deepEqual(report.armCheck.hashMismatches, [
    { repo: 'acme/widgets', branch: 'feat-a', logged: 'guidance', computed: 'review-only' },
  ])
})

test('history mode with an arm log keeps only branches first run before the hook began', (t) => {
  const at800 = [{ at: '1970-01-01T00:13:20.000Z', skills: LOADS_STANDARDS }]
  const repos = [widgetsRepo({ branches: armBranches('feat-a', 'feat-b'), runs: [armRun('feat-a', 900), armRun('feat-b')] })]
  const sessions = [armSession('feat-a', { entries: at800 }), armSession('feat-b', { entries: at800 })]
  const report = history(t, armWorld({ repos, sessions }))

  assert.equal(report.mode, 'history')
  assert.deepEqual(report.branches.map(({ branch }) => branch), ['feat-a'])
  assert.deepEqual(report.excluded, [])
  assert.equal(report.armCheck, null)
})

/**
 * @param {number} ts
 * @param {string} lang
 * @param {Record<string, number>} findings
 * @param {Record<string, unknown>} [rest]
 */
function lintRow(ts, lang, findings, rest = {}) {
  return { ts, repo: UPSTREAM, branch: 'feat-b', head: 'abc', lang, mode: 'staged', gate: false, blocked: false, findings, ...rest }
}

test('lint metrics total each language earliest non-gate run and count the blocked commits', (t) => {
  const lint = [
    lintRow(10, 'ts', { 'no-unused-vars': 2, eqeqeq: 1 }, { blocked: true }),
    lintRow(20, 'ts', {}),
    lintRow(15, 'go', { errcheck: 4 }, { blocked: true }),
    lintRow(5, 'ts', { eqeqeq: 9 }, { gate: true, blocked: true }),
  ]
  const report = arm(t, armWorld({ lint }))
  const [featA, featB] = report.branches

  assert.equal(featB.metrics.lintPreCommit, 7)
  assert.equal(featB.metrics.blockedCommits, 2)
  assert.equal(featA.metrics.lintPreCommit, null)
  assert.equal(featA.metrics.blockedCommits, null)
})

test('a missing lint log nulls every lint metric without throwing', (t) => {
  const report = arm(t, armWorld())

  assert.deepEqual(report.sources.lintLog, { present: false, rows: 0, skipped: 0 })
  for (const { metrics } of report.branches) {
    assert.equal(metrics.lintPreCommit, null)
    assert.equal(metrics.blockedCommits, null)
  }
})

test('a malformed arm log line is skipped and counted among valid rows', (t) => {
  const arms = [ARM_ROWS[0], '{"ts":', ARM_ROWS[1]]
  const report = arm(t, armWorld({ arms }))

  assert.deepEqual(report.branches.map(({ branch, group }) => ({ branch, group })), [
    { branch: 'feat-a', group: 'review-only' },
    { branch: 'feat-b', group: 'guidance' },
  ])
  assert.deepEqual(report.sources.armLog, { present: true, rows: 2, skipped: 1 })
})

const RUN1 = '01J00000000000000000RUN001'
const RUN2 = '01J00000000000000000RUN002'

/**
 * @param {string} run
 * @param {number} ts
 * @param {string} repo
 * @param {Record<string, [string, string | null]>} arms
 */
function assignRow(run, ts, repo, arms) {
  return { kind: 'assign', run, ts, repo, head: 'abc1234', arms }
}

/**
 * @param {string} run
 * @param {number} ts
 * @param {string} aspect
 * @param {string} arm
 * @param {[number, number, number]} severities critical, important, suggestion
 * @param {Record<string, number | null>} [tokens] every field null when omitted
 */
function resultRow(run, ts, aspect, arm, [critical, important, suggestion], tokens = {}) {
  const all = { in: null, cw: null, cr: null, out: null, think: null, ...tokens }
  return { kind: 'result', run, ts, aspect, arm, critical, important, suggestion, tokens: all, seconds: 1, model: null }
}

/** The shared feat-a runs: RUN1 and RUN2 each have a review round 1. */
const REVIEW_RUNS = [
  { ...reviewedRun(RUN1, 1000, []) },
  { ...reviewedRun(RUN2, 2000, []) },
]

const STANDARDS_ARM = { standards: ['general-purpose', null] }

const TRACER_REVIEW_AB = [
  assignRow('aaaa0001', 100, RUN1, STANDARDS_ARM),
  resultRow('aaaa0001', 101, 'standards', 'general-purpose', [1, 2, 3], { in: 10, cw: 100, cr: 1000, out: 20, think: 15 }),
  assignRow('bbbb0001', 50, 'unknown', { standards: ['low-effort', '-'] }),
  resultRow('bbbb0001', 51, 'standards', 'low-effort', [9, 9, 9]),
]

/**
 * @param {import('node:test').TestContext} t
 * @param {import('./fixture.js').LogRow[]} reviewAb
 */
function reviewAbReport(t, reviewAb) {
  return history(t, { ...widgets({ runs: REVIEW_RUNS }), reviewAb })
}

/** @param {{ branches: { branch: string, aspects: any }[] }} report */
function featAAspects(report) {
  return report.branches.find(({ branch }) => branch === 'feat-a').aspects
}

test('a branch takes its aspects from the first review assign, with cost priced from the kept result', (t) => {
  const report = reviewAbReport(t, TRACER_REVIEW_AB)

  assert.deepEqual(featAAspects(report), {
    standards: { arm: 'general-purpose', pinned: false, critical: 1, important: 2, suggestion: 3, cost: 335 },
  })
})

test('a model other than null or "-" joins the agent in the arm string', (t) => {
  const report = reviewAbReport(t, [
    assignRow('aaaa0001', 100, RUN1, { ...STANDARDS_ARM, 'tests-quality': ['general-purpose', 'sonnet'] }),
    resultRow('aaaa0001', 101, 'standards', 'general-purpose', [1, 2, 3]),
    resultRow('aaaa0001', 102, 'tests-quality', 'general-purpose/sonnet', [0, 0, 1]),
  ])

  assert.deepEqual(featAAspects(report)['tests-quality'], {
    arm: 'general-purpose/sonnet',
    pinned: true,
    critical: 0,
    important: 0,
    suggestion: 1,
    cost: null,
  })
})

test('a pinned aspect with no assign entry reports the result arm verbatim', (t) => {
  const comments = resultRow('aaaa0001', 102, 'comments', 'general-purpose/sonnet', [0, 1, 0], { in: 2, cw: null, cr: null, out: 2, think: 0 })
  const report = reviewAbReport(t, [...TRACER_REVIEW_AB, comments])

  assert.deepEqual(featAAspects(report).comments, {
    arm: 'general-purpose/sonnet',
    pinned: true,
    critical: 0,
    important: 1,
    suggestion: 0,
    cost: 12,
  })
})

test('severities come from the earliest assign on the first run, while cost sums every assign', (t) => {
  const report = reviewAbReport(t, [
    ...TRACER_REVIEW_AB,
    assignRow('aaaa0002', 200, RUN1, STANDARDS_ARM),
    resultRow('aaaa0002', 201, 'standards', 'general-purpose', [0, 0, 0], { in: 5, cw: null, cr: null, out: 1, think: 0 }),
  ])

  assert.equal(featAAspects(report).standards.critical, 1)
  assert.equal(featAAspects(report).standards.cost, 345)
})

test('an assign on a later run adds cost but not severities', (t) => {
  const report = reviewAbReport(t, [
    ...TRACER_REVIEW_AB,
    assignRow('aaaa0003', 300, RUN2, STANDARDS_ARM),
    resultRow('aaaa0003', 301, 'standards', 'general-purpose', [7, 0, 0], { in: 1, cw: 0, cr: 0, out: 0, think: 0 }),
  ])

  assert.equal(featAAspects(report).standards.critical, 1)
  assert.equal(featAAspects(report).standards.cost, 336)
})

test('a first assign whose aspect has no result reports null instead of falling forward', (t) => {
  const report = reviewAbReport(t, [
    assignRow('aaaa0001', 100, RUN1, STANDARDS_ARM),
    assignRow('aaaa0002', 200, RUN1, STANDARDS_ARM),
    resultRow('aaaa0002', 201, 'standards', 'general-purpose', [1, 2, 3]),
  ])

  assert.equal(featAAspects(report).standards, null)
})

test('a later duplicate result for the same assign and aspect is ignored, cost included', (t) => {
  const report = reviewAbReport(t, [
    ...TRACER_REVIEW_AB,
    resultRow('aaaa0001', 105, 'standards', 'general-purpose', [5, 0, 0], { in: 1000, cw: 0, cr: 0, out: 0, think: 0 }),
  ])

  assert.equal(featAAspects(report).standards.critical, 1)
  assert.equal(featAAspects(report).standards.cost, 335)
})

test('a branch with no assign on its first run has no aspects', (t) => {
  const report = reviewAbReport(t, TRACER_REVIEW_AB.slice(2))

  assert.deepEqual(featAAspects(report), {})
})

test('a missing review-ab log leaves every branch with no aspects', (t) => {
  const report = history(t, widgets({ runs: REVIEW_RUNS }))

  assert.equal(report.sources.reviewAb.present, false)
  assert.deepEqual(featAAspects(report), {})
})

test('a malformed review-ab line is skipped and counted among valid rows', (t) => {
  const report = reviewAbReport(t, [...TRACER_REVIEW_AB, '{"kind":"result",'])

  assert.equal(featAAspects(report).standards.critical, 1)
  assert.equal(report.sources.reviewAb.skipped, 1)
})

test('a section tag is the bracket that opens a finding, after an optional severity word', (t) => {
  const descriptions = [
    '[Code smells / Feature Envy] moves data',
    'Important — [Naming] bad name [src/app.ts:12]',
    'warning: [Tests / Assertions] tighten [src/app.ts:3-5]',
    '[Naming] other name',
    'uses arr[0] without a check',
    '[src/app.ts:3] is wrong',
    '[Collection(Fixture)] attribute missing',
    'no tag here',
  ]
  const round1 = descriptions.map((description, i) => ({ id: `f${i}`, severity: 'info', description }))
  const report = history(t, widgets({ runs: [reviewedRun('run1', 1000, round1)] }))

  assert.deepEqual(report.branches[0].sections, { 'Code smells / Feature Envy': 1, Naming: 2, 'Tests / Assertions': 1 })
})

test('findings with no bracket tag leave the sections empty', (t) => {
  const report = history(t, widgets({ runs: [reviewedRun('run1', 1000, TRACER_ROUND)] }))

  assert.deepEqual(report.branches[0].sections, {})
})

test('a summary cell holds the count, median, mean and share of non-zero values with seeded intervals', (t) => {
  const report = arm(t, armSummaryWorld())

  assert.deepEqual(report.summary.guidance.reviewError, {
    n: 2,
    median: 3,
    medianCi: { lo: 3, hi: 3 },
    mean: 3,
    meanCi: { lo: 3, hi: 3 },
    shareNonzero: 1,
  })
  assert.equal(report.diff.reviewError.median, -2)
  assert.deepEqual(report.diff.reviewError.medianCi, { lo: -2, hi: -2 })
})

test('a metric no branch in the group recorded has an empty cell', (t) => {
  const report = arm(t, armSummaryWorld())

  assert.deepEqual(report.summary.guidance.lintPreCommit, {
    n: 0,
    median: null,
    medianCi: null,
    mean: null,
    meanCi: null,
    shareNonzero: null,
  })
})

test('compliance counts the branches whose transcripts loaded the skill, and those with no transcript', (t) => {
  const report = arm(t, armSummaryWorld({ withoutSession: ['feat-c'] }))

  assert.deepEqual(report.compliance['review-only'], { loaded: 1, total: 2, unknown: 1 })
  assert.deepEqual(report.compliance.guidance, { loaded: 2, total: 2, unknown: 0 })
})

test('with no decision rule set the verdict is not-set', (t) => {
  const report = arm(t, armSummaryWorld())

  assert.equal(report.verdict.outcome, 'not-set')
  assert.equal(report.verdict.rule, null)
})

test('the verdict favours the arm whose diff interval clears the margin', (t) => {
  const rule = { metric: 'reviewError', statistic: 'median', favour: 'lower', margin: 1 }
  const outcome = (/** @type {object} */ world, /** @type {typeof rule} */ chosen) =>
    analyze({ mode: 'arm', paths: buildWorld(t, world), rule: chosen }).verdict.outcome

  assert.equal(outcome(armSummaryWorld(), rule), 'review-only')
  assert.equal(outcome(armSummaryWorld(), { ...rule, margin: 3 }), 'inconclusive')
  assert.equal(outcome(armSummaryWorld({ errors: { guidance: 1, 'review-only': 3 } }), rule), 'guidance')
})

test('a history report has no verdict', (t) => {
  const report = history(t, widgets())

  assert.equal(report.verdict, null)
})

test('the stratum cut holds a cell per group and metric for the branches of that size', (t) => {
  const workload = { 'feat-b': 40, 'feat-e': 600, 'feat-a': 50, 'feat-c': 700 }
  const report = arm(t, armSummaryWorld({ run: (name) => ({ invocations: [{ workloadLines: workload[name] }] }) }))

  assert.equal(report.byStratum.small.guidance.reviewError.n, 1)
  assert.equal(report.byStratum.large['review-only'].reviewError.median, 1)
})

/** Review-ab rows giving every arm-world branch a standards result whose arm and criticals follow its group. */
function aspectLog() {
  const standards = { 'feat-b': ['general-purpose', 2], 'feat-e': ['general-purpose', 2], 'feat-a': ['low-effort', 0], 'feat-c': ['low-effort', 0] }
  return Object.entries(standards).flatMap(([name, [arm, critical]], i) => [
    assignRow(`ab-${name}`, 100 + i, `run-${name}`, { standards: [arm, null], simplification: ['general-purpose', null] }),
    resultRow(`ab-${name}`, 200 + i, 'standards', arm, [critical, 0, 0]),
    resultRow(`ab-${name}`, 300 + i, 'simplification', 'general-purpose', [0, 1, 0]),
  ])
}

test('the aspect cuts report each aspect per group, and the non-pinned ones per review agent', (t) => {
  const report = arm(t, armSummaryWorld({ reviewAb: aspectLog() }))

  assert.equal(report.byAspect.standards.primary, true)
  assert.equal(report.byAspect.standards.pinned, false)
  assert.equal(report.byAspect.standards.groups.guidance.critical.median, 2)
  assert.equal(report.byAspect.simplification.primary, false)
  assert.equal(report.byReviewAgent.standards['low-effort']['review-only'].critical.median, 0)
  assert.equal('simplification' in report.byReviewAgent, false)
})

test('the section cut counts a tag for every branch in the group, zero where it is absent', (t) => {
  const tagged = ['[Naming] one', '[Naming] two'].map((description, i) => ({ id: `n${i}`, severity: 'info', description }))
  const run = (/** @type {string} */ name) => (name === 'feat-b' ? { steps: { review: [tagged] } } : {})
  const report = arm(t, armSummaryWorld({ run }))

  const naming = report.bySection.Naming.guidance
  assert.equal(naming.n, 2)
  assert.equal(naming.median, 1)
  assert.equal(naming.mean, 1)
})

test('an aspect is pinned when its most common arm covers at least 95% of its kept results', (t) => {
  /** One unjoined assign and result per row, since a run keeps one result per aspect. */
  const elsewhere = (/** @type {string} */ aspect, /** @type {string} */ arm, /** @type {number} */ count) =>
    Array.from({ length: count }, (_, i) => {
      const run = `${aspect}-${arm}-${i}`.replace(/\W/g, '')
      return [assignRow(run, 10, 'unknown', {}), resultRow(run, 11, aspect, arm, [0, 0, 0])]
    }).flat()
  const report = reviewAbReport(t, [
    assignRow('aaaa0001', 100, RUN1, STANDARDS_ARM),
    resultRow('aaaa0001', 101, 'standards', 'general-purpose', [1, 2, 3]),
    resultRow('aaaa0001', 102, 'comments', 'general-purpose/sonnet', [0, 1, 0]),
    ...elsewhere('comments', 'general-purpose/sonnet', 20),
    ...elsewhere('comments', 'general-purpose', 1),
    ...elsewhere('standards', 'general-purpose', 10),
    ...elsewhere('standards', 'low-effort', 10),
  ])

  assert.equal(featAAspects(report).comments.pinned, true)
  assert.equal(featAAspects(report).standards.pinned, false)
})

test('the small stratum ends at 99 lines and medium starts at 100', (t) => {
  const review = (/** @type {number} */ workloadLines) => [{ step: 'review', round: 1, purpose: 'review', workloadLines }]

  assert.equal(stratum(t, review(99)), 'small')
  assert.equal(stratum(t, review(100)), 'medium')
})

test('a diff interval that only touches the margin is inconclusive on either side', (t) => {
  const rule = { metric: 'reviewError', statistic: 'median', favour: 'lower', margin: 2 }
  const outcome = (/** @type {object} */ world) => analyze({ mode: 'arm', paths: buildWorld(t, world), rule }).verdict.outcome

  assert.equal(outcome(armSummaryWorld()), 'inconclusive')
  assert.equal(outcome(armSummaryWorld({ errors: { guidance: 1, 'review-only': 3 } })), 'inconclusive')
})
