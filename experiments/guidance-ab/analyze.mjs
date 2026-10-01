/**
 * Joins the machine-local logs into the guidance A/B report.
 *
 * History mode takes every branch no-mistakes reviewed before the arm hook began logging as a unit, and
 * groups it by whether a transcript on that branch loaded the coding-standards skill before the
 * branch's first run, and reports every other reviewed branch as an exclusion with its reason. Arm mode
 * takes the branches the arm log assigns to an arm as the units and groups them by that arm. It leaves a
 * branch the arm log never names out of the report, since that branch is outside the experiment.
 */
import { createHash } from 'node:crypto'
import { extname, join, sep } from 'node:path'
import { tokenCost } from './pricing.mjs'
import { changedFiles } from './sources/git.mjs'
import { lintMetrics, readJsonl } from './sources/logs.mjs'
import { branchAspects, readReviewAb, sectionCounts } from './sources/review-ab.mjs'
import { readBranches, readRepos } from './sources/no-mistakes.mjs'
import { readSegments } from './sources/transcripts.mjs'
import { summarize, verdict } from './summary.mjs'

/**
 * @typedef {'guidance' | 'review-only'} ArmGroup
 * @typedef {{ repo: string | null, branch: string, group: ArmGroup }} Unit a branch the arm log assigns to an arm;
 *   `repo` is null when the logged repo has no `owner/name`, so no reviewed branch matches it
 * @typedef {{ ts: number, session_id: string, repo: string, branch: string, arm: ArmGroup | 'unassigned' }} ArmRow
 */

/** @type {Record<string, number>} */
const SEVERITY_RANK = { info: 0, warning: 1, error: 2 }

const CODE_EXTENSIONS = new Set(['ts', 'tsx', 'js', 'jsx', 'mjs', 'cjs', 'go', 'cs', 'py', 'rb', 'java', 'kt', 'rs', 'swift', 'sh'])

/** A `mktemp` directory name, which marks a throwaway checkout made by a test or a script. */
const SANDBOX_SEGMENT = /^tmp\.[A-Za-z0-9]+$/

/** A sequence number the reviewer reassigns each round, so it names no particular finding. */
const GENERIC_ID = /^(review|rev|cr|r)-?r?\d+(-\d+)*$/

const HISTORY_CAVEAT =
  'History groups are self-selected: the agent chose whether to load the skill, so a difference between groups is not caused by the skill alone.'

/**
 * The rule that turns an arm report into a verdict, fixed before the arms have data so the result
 * cannot choose it. Null until it is set, which leaves every verdict `not-set`.
 *
 * @type {import('./summary.mjs').Rule | null}
 */
export const DECISION_RULE = null

/**
 * `seed` fixes the bootstrap intervals and defaults to 1; `rule` defaults to `DECISION_RULE`.
 *
 * @param {{
 *   mode: 'history' | 'arm',
 *   paths: { stateDir: string, noMistakesHome: string, reviewAb: string, projectsDir: string },
 *   seed?: number,
 *   rule?: import('./summary.mjs').Rule | null,
 * }} options
 * @throws {Error} naming the path when state.sqlite is missing, or arms.jsonl is in arm mode
 */
export function analyze({ mode, paths, seed = 1, rule = DECISION_RULE }) {
  const armLog = readJsonl(join(paths.stateDir, 'arms.jsonl'))
  if (mode === 'arm' && !armLog.source.present) throw new Error(`arm log not found: ${join(paths.stateDir, 'arms.jsonl')}`)
  const lintLog = readJsonl(join(paths.stateDir, 'lint-runs.jsonl'))
  const reviewAb = readReviewAb(paths.reviewAb)
  const units = mode === 'arm' ? armUnits(armLog.rows) : null
  // Infinity with no arm log, which keeps every history branch.
  const hookStart = Math.min(...armLog.rows.map((row) => row.ts))
  // Infinity with no lint log, which leaves every lint metric null when a branch has no lint run.
  const lintStart = Math.min(...lintLog.rows.map((row) => row.ts))
  const repos = readRepos(paths.noMistakesHome)
  const { segments: read, source } = readSegments(paths.projectsDir)
  const segments = read.map((segment) => ({ segment, repo: repoOf(segment, repos, units ? armLog.rows : []) }))
  const branches = []
  const excluded = units ? unassignedBranches(armLog.rows, units) : []
  const decided = new Set()
  for (const branch of readBranches(paths.noMistakesHome)) {
    const firstRun = branch.runs.find((run) => stepRound(run, 'review', 1)) ?? null
    const repo = repoKey(branch.repo.upstreamUrl)
    const unit = units?.get(unitKey(repo, branch.branch))
    if (units && !unit) continue
    const own = segments
      .filter((resolved) => resolved.repo === repo && resolved.segment.gitBranch === branch.branch)
      .map(({ segment }) => segment)
    const loaded = firstRun != null && own.some((segment) => segment.loadTimes.some((at) => at < firstRun.createdAt))
    const began = Math.min(firstRun?.createdAt ?? Infinity, ...own.map((segment) => segment.firstAt))
    const group = unit ? unit.group : own.length && firstRun ? (loaded ? 'loaded' : 'not-loaded') : null
    const reason = exclusion(paths.noMistakesHome, branch, firstRun, group, { arm: units != null, hookStart, began })
    decided.add(unitKey(repo, branch.branch))
    if (reason) {
      excluded.push({ repo, branch: branch.branch, group, reason })
      continue
    }
    branches.push({
      repo,
      branch: branch.branch,
      group,
      stratum: stratum(firstRun),
      compliance: own.length ? loaded : null,
      noMistakesVersion: firstRun.noMistakesVersion,
      aspects: branchAspects(reviewAb, branch.runs, firstRun),
      sections: sectionCounts(stepRound(firstRun, 'review', 1)?.findings ?? []),
      metrics: {
        ...metrics(branch, firstRun, own),
        ...lintMetrics(
          lintLog.rows.filter((row) => parseRepoKey(row.repo) === repo && row.branch === branch.branch),
          { began, logStart: lintStart },
        ),
      },
    })
  }
  for (const [key, unit] of units ?? []) {
    if (!decided.has(key)) excluded.push({ repo: unit.repo, branch: unit.branch, group: unit.group, reason: 'no pipeline run' })
  }
  if (units) {
    const ratio = { guidance: 0, 'review-only': 0 }
    for (const { group } of branches) ratio[group] += 1
    const armCheck = { ratio, hashMismatches: hashMismatches(armLog.rows) }
    const groups = ['guidance', 'review-only']
    const stats = summarize({ groups, branches, seed })
    return { mode, groups, caveat: null, branches, excluded, armCheck, ...stats, verdict: verdict(rule, groups, stats.diff), sources: { projects: source, armLog: armLog.source, lintLog: lintLog.source, reviewAb: reviewAb.source } }
  }
  const groups = ['loaded', 'not-loaded']
  return { mode: 'history', groups, caveat: HISTORY_CAVEAT, branches, excluded, armCheck: null, ...summarize({ groups, branches, seed }), verdict: null, sources: { projects: source, armLog: armLog.source, lintLog: lintLog.source, reviewAb: reviewAb.source } }
}

/**
 * The branches the arm log assigns to an arm, each grouped by the arm of its earliest such row.
 *
 * @param {ArmRow[]} rows
 * @returns {Map<string, Unit>} keyed by `unitKey`
 */
function armUnits(rows) {
  const assigned = rows
    .filter((row) => row.arm === 'guidance' || row.arm === 'review-only')
    .sort((a, b) => a.ts - b.ts)
  /** @type {Map<string, Unit>} */
  const units = new Map()
  for (const row of assigned) {
    const repo = parseRepoKey(row.repo)
    const key = unitKey(repo, row.branch)
    if (!units.has(key)) units.set(key, { repo, branch: row.branch, group: row.arm })
  }
  return units
}

/**
 * The branches the arm log lists but never assigns, as exclusions.
 *
 * @param {ArmRow[]} rows
 * @param {Map<string, Unit>} units
 */
function unassignedBranches(rows, units) {
  const seen = new Set(units.keys())
  const excluded = []
  for (const row of rows) {
    const repo = parseRepoKey(row.repo)
    const key = unitKey(repo, row.branch)
    if (seen.has(key)) continue
    seen.add(key)
    excluded.push({ repo, branch: row.branch, group: null, reason: 'unassigned' })
  }
  return excluded
}

/**
 * Each distinct (repo, branch, logged arm) whose logged arm is not the one the hook's hash assigns:
 * the first hex digit of `sha1("<repo>#<branch>")`, `0-7` guidance and `8-f` review-only.
 *
 * @param {ArmRow[]} rows
 */
function hashMismatches(rows) {
  const mismatches = new Map()
  for (const row of rows) {
    if (row.arm === 'unassigned') continue
    const digit = createHash('sha1').update(`${row.repo}#${row.branch}`).digest('hex')[0]
    const computed = digit < '8' ? 'guidance' : 'review-only'
    if (row.arm === computed) continue
    const key = `${row.repo}\0${row.branch}\0${row.arm}`
    mismatches.set(key, { repo: parseRepoKey(row.repo), branch: row.branch, logged: row.arm, computed })
  }
  return [...mismatches.values()]
}

/**
 * @param {string | null} repo normalized `owner/name`
 * @param {string} branch
 */
function unitKey(repo, branch) {
  return `${repo}\0${branch}`
}

/**
 * Why the branch is not a unit, checked in precedence order, or null when it is one.
 *
 * @param {string} noMistakesHome
 * @param {import('./sources/no-mistakes.mjs').Branch} branch
 * @param {import('./sources/no-mistakes.mjs').Run | null} firstRun null when no run has a review round 1
 * @param {string | null} group null when no transcript sits on the branch, or in history mode no first run
 * @param {{ arm: boolean, hookStart: number, began: number }} cut `arm` is true in arm mode, `hookStart` is
 *   when the arm hook began logging, and `began` the earlier of the first run and the earliest transcript segment
 */
function exclusion(noMistakesHome, branch, firstRun, group, { arm, hookStart, began }) {
  const { id, defaultBranch, workingPath } = branch.repo
  if (workingPath.split(sep).some((segment) => SANDBOX_SEGMENT.test(segment))) return 'sandbox repo'
  if (branch.runs.some((run) => run.status === 'running' || run.status === 'pending')) return 'in flight'
  if (!firstRun) return 'no pipeline run'
  if (arm && began < hookStart) return 'predates hook'
  if (!arm && firstRun.createdAt >= hookStart) return 'postdates hook'
  const bare = join(noMistakesHome, 'repos', `${id}.git`)
  const changed = changedFiles([bare, workingPath], firstRun.headSha, `refs/remotes/origin/${defaultBranch}`)
  if (changed == null) return 'diff unavailable'
  if (!changed.some((path) => CODE_EXTENSIONS.has(extname(path).slice(1)))) return 'no code change'
  if (group == null && !arm) return 'no transcript'
  return null
}

/**
 * @param {import('./sources/no-mistakes.mjs').Branch} branch
 * @param {import('./sources/no-mistakes.mjs').Run} firstRun
 * @param {import('./sources/transcripts.mjs').Segment[]} segments those on the branch
 */
function metrics(branch, firstRun, segments) {
  const review = countBySeverity(stepRound(firstRun, 'review', 1)?.findings ?? [])
  const unique = countBySeverity(uniqueFindings(firstRun.steps.get('review') ?? []))
  const impl = segments.length ? implTokens(segments) : null
  const pipeline = pipelineTokens(branch.runs.flatMap((run) => run.invocations))
  return {
    reviewError: review.error,
    reviewWarning: review.warning,
    reviewInfo: review.info,
    reviewUniqueError: unique.error,
    reviewUniqueWarning: unique.warning,
    reviewUniqueInfo: unique.info,
    reviewRounds: branch.runs.reduce((sum, run) => sum + (run.steps.get('review')?.length ?? 0), 0),
    pipelineTokens: pipeline,
    implTokens: impl,
    wallSeconds: wallSeconds(branch.runs, segments),
    totalCost: pipeline || impl ? (pipeline?.cost ?? 0) + (impl?.cost ?? 0) : null,
    lintFirstRun: stepRound(firstRun, 'lint', 1)?.findings.length ?? null,
  }
}

/**
 * The repo key a segment worked in, or null when it is unresolved. The first rule that applies decides:
 * an arm row logged for the segment's session and branch names the repo; the cwd sits in a repo's
 * checkout; the session's pr-link names a known repo; exactly one known repo's name is a directory of
 * the cwd. Several no-mistakes repo rows can share one key, so the rules compare keys, not rows.
 *
 * @param {import('./sources/transcripts.mjs').Segment} segment
 * @param {import('./sources/no-mistakes.mjs').Repo[]} repos
 * @param {ArmRow[]} armRows empty outside arm mode
 * @returns {string | null} normalized `owner/name`
 */
function repoOf(segment, repos, armRows) {
  const logged = armRows.find((row) => row.session_id === segment.sessionId && row.branch === segment.gitBranch)
  if (logged) return parseRepoKey(logged.repo)
  const checkout = repos.find((repo) => segment.cwd === repo.workingPath || segment.cwd.startsWith(`${repo.workingPath}/`))
  if (checkout) return repoKey(checkout.upstreamUrl)
  const keys = new Set(repos.map((repo) => repoKey(repo.upstreamUrl)))
  if (segment.prRepository && keys.has(segment.prRepository)) return segment.prRepository
  const dirs = segment.cwd.toLowerCase().split('/')
  const named = [...keys].filter((key) => dirs.includes(key.split('/')[1]))
  return named.length === 1 ? named[0] : null
}

/**
 * @param {import('./sources/no-mistakes.mjs').Run} run
 * @param {string} step
 * @param {number} round
 */
function stepRound(run, step, round) {
  return run.steps.get(step)?.find((r) => r.round === round)
}

/**
 * The size class of the change the first review round saw, or null when no review call recorded it.
 *
 * @param {import('./sources/no-mistakes.mjs').Run} firstRun
 */
function stratum(firstRun) {
  const lines = firstRun.invocations.find(
    (i) => i.step === 'review' && i.round === 1 && i.purpose === 'review',
  )?.workloadLines
  if (lines == null) return null
  if (lines < 100) return 'small'
  return lines < 500 ? 'medium' : 'large'
}

/**
 * Token totals and their cost in input-token equivalents, or null when no invocation recorded any.
 *
 * @param {import('./sources/no-mistakes.mjs').Invocation[]} invocations
 */
function pipelineTokens(invocations) {
  const recorded = invocations.filter((i) => [i.input, i.cacheWrite, i.cacheRead, i.output].some((n) => n != null))
  if (!recorded.length) return null
  const sum = (/** @type {(i: import('./sources/no-mistakes.mjs').Invocation) => number | null} */ pick) =>
    recorded.reduce((total, i) => total + (pick(i) ?? 0), 0)
  const input = sum((i) => i.input)
  const cacheWrite = sum((i) => i.cacheWrite)
  const cacheRead = sum((i) => i.cacheRead)
  const output = sum((i) => i.output)
  return tokenTotals(input, cacheWrite, cacheRead, output)
}

/**
 * Seconds from the first transcript line to the last step completion, less the time runs sat parked
 * waiting on a human, or null when no step recorded a completion or no transcript exists.
 *
 * @param {import('./sources/no-mistakes.mjs').Run[]} runs
 * @param {import('./sources/transcripts.mjs').Segment[]} segments
 */
function wallSeconds(runs, segments) {
  const completions = runs.map((run) => run.completedAt).filter((at) => at != null)
  if (!completions.length || !segments.length) return null
  const parked = runs.reduce((total, run) => total + (run.parkedMs ?? 0) / 1000, 0)
  return Math.max(...completions) - Math.min(...segments.map((segment) => segment.firstAt)) - parked
}

/**
 * Token totals and their cost over the segments, priced like the pipeline tokens.
 *
 * @param {import('./sources/transcripts.mjs').Segment[]} segments
 */
function implTokens(segments) {
  const sum = (/** @type {keyof import('./sources/transcripts.mjs').Usage} */ key) =>
    segments.reduce((total, segment) => total + segment.usage[key], 0)
  return tokenTotals(sum('input'), sum('cacheWrite'), sum('cacheRead'), sum('output'))
}

/** Prices the counts in input-token equivalents. */
function tokenTotals(/** @type {number} */ input, /** @type {number} */ cacheWrite, /** @type {number} */ cacheRead, /** @type {number} */ output) {
  return { input, cacheWrite, cacheRead, output, cost: tokenCost(input, cacheWrite, cacheRead, output) }
}

/** @param {import('./sources/no-mistakes.mjs').Finding[]} findings */
function countBySeverity(findings) {
  const count = (/** @type {string} */ severity) => findings.filter((f) => f.severity === severity).length
  return { error: count('error'), warning: count('warning'), info: count('info') }
}

/**
 * Every distinct finding across the rounds, each kept at the highest severity any round gave it.
 *
 * @param {import('./sources/no-mistakes.mjs').Round[]} rounds
 */
function uniqueFindings(rounds) {
  /** @type {Map<string, import('./sources/no-mistakes.mjs').Finding>} */
  const unique = new Map()
  for (const finding of rounds.flatMap((round) => round.findings)) {
    const identity = GENERIC_ID.test(finding.id) ? `${finding.file}\0${finding.description}` : finding.id
    const seen = unique.get(identity)
    if (!seen || SEVERITY_RANK[finding.severity] > SEVERITY_RANK[seen.severity]) unique.set(identity, finding)
  }
  return [...unique.values()]
}

/**
 * `owner/name` from a no-mistakes upstream URL, which must have one.
 *
 * @param {string} url
 */
function repoKey(url) {
  const key = parseRepoKey(url)
  if (key == null) throw new Error(`cannot parse owner/name from upstream URL: ${url}`)
  return key
}

/**
 * `owner/name`, lowercased, from an scp-style, https, or ssh URL, the ssh user optional; null for any
 * other value, such as the empty or local-path origin a log row can carry.
 *
 * @param {unknown} url
 */
function parseRepoKey(url) {
  const match = /^(?:git@[^:/]+:|https:\/\/[^/]+\/|ssh:\/\/(?:[^@/]+@)?[^/]+\/)([^/]+)\/([^/]+?)(?:\.git)?$/.exec(String(url))
  return match ? `${match[1]}/${match[2]}`.toLowerCase() : null
}
