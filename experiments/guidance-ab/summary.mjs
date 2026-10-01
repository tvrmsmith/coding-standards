/**
 * Reduces the per-branch measurements to the per-group statistics the report prints.
 */
import { formatEstimate } from './format.mjs'
import { bootstrapCi, bootstrapDiffCi, mean, median, shareNonzero } from './stats.mjs'

/**
 * @typedef {{ lo: number, hi: number } | null} Interval
 * @typedef {{ n: number, median: number | null, medianCi: Interval, mean: number | null, meanCi: Interval, shareNonzero: number | null }} Cell
 * @typedef {{ group: string, stratum: string | null, compliance: boolean | null, aspects: Record<string, any>, sections: Record<string, number>, metrics: Record<string, any> }} Measured the part of a branch report a summary reads
 * @typedef {{ groups: string[], branches: Measured[], seed: number }} Basis
 */

/** Reads each summary metric off a branch's measurements. */
const METRICS = {
  lintFirstRun: ({ metrics }) => metrics.lintFirstRun,
  lintPreCommit: ({ metrics }) => metrics.lintPreCommit,
  blockedCommits: ({ metrics }) => metrics.blockedCommits,
  reviewError: ({ metrics }) => metrics.reviewError,
  reviewWarning: ({ metrics }) => metrics.reviewWarning,
  reviewInfo: ({ metrics }) => metrics.reviewInfo,
  reviewUniqueError: ({ metrics }) => metrics.reviewUniqueError,
  reviewUniqueWarning: ({ metrics }) => metrics.reviewUniqueWarning,
  reviewUniqueInfo: ({ metrics }) => metrics.reviewUniqueInfo,
  reviewRounds: ({ metrics }) => metrics.reviewRounds,
  implCost: ({ metrics }) => metrics.implTokens?.cost,
  pipelineCost: ({ metrics }) => metrics.pipelineTokens?.cost,
  totalCost: ({ metrics }) => metrics.totalCost,
  wallSeconds: ({ metrics }) => metrics.wallSeconds,
}

const SEVERITIES = ['critical', 'important', 'suggestion']

/** The review aspects the design names as primary, which the others only supplement. */
const PRIMARY_ASPECTS = new Set(['standards', 'comments', 'error-handling', 'tests-quality'])

const STRATA = ['small', 'medium', 'large']

const STATISTICS = { median, mean, shareNonzero }

/**
 * @param {number[]} values
 * @param {number} seed
 * @returns {Cell}
 */
export function cell(values, seed) {
  const some = values.length > 0
  const withCi = (/** @type {(values: number[]) => number | null} */ statistic) => (some ? bootstrapCi(values, statistic, { seed }) : null)
  return {
    n: values.length,
    median: median(values),
    medianCi: withCi(median),
    mean: mean(values),
    meanCi: withCi(mean),
    shareNonzero: shareNonzero(values),
  }
}

/** @param {number | null | undefined} value */
const present = (value) => value != null

/**
 * Every summary metric's cell per group, the second group's difference from the first, and the same
 * cells within each size stratum.
 *
 * @param {Basis} basis
 */
export function summarize({ groups, branches, seed }) {
  const diff = Object.fromEntries(
    Object.keys(METRICS).map((metric) => [metric, difference(metricValues(branches, groups[0], metric), metricValues(branches, groups[1], metric), seed)]),
  )
  const compliance = Object.fromEntries(
    groups.map((group) => {
      const own = branches.filter((branch) => branch.group === group)
      return [group, { loaded: own.filter((branch) => branch.compliance === true).length, total: own.length, unknown: own.filter((branch) => branch.compliance === null).length }]
    }),
  )
  const byStratum = Object.fromEntries(
    STRATA.map((stratum) => [stratum, metricCells(groups, branches.filter((branch) => branch.stratum === stratum), seed)]),
  )
  return { summary: metricCells(groups, branches, seed), diff, compliance, byStratum, ...aspectCuts(groups, branches, seed), bySection: sectionCuts(groups, branches, seed) }
}

/**
 * The aspects named by any branch, each reported per group, and per review agent when the aspect ran
 * under more than one arm. A branch with no result for an aspect is left out of it.
 *
 * @param {string[]} groups
 * @param {Measured[]} branches
 * @param {number} seed
 */
function aspectCuts(groups, branches, seed) {
  const byAspect = {}
  const byReviewAgent = {}
  const names = new Set(branches.flatMap((branch) => Object.keys(branch.aspects)))
  for (const aspect of names) {
    const reported = branches.flatMap((branch) => (branch.aspects[aspect] ? [{ group: branch.group, ...branch.aspects[aspect] }] : []))
    const pinned = reported.some((entry) => entry.pinned)
    byAspect[aspect] = {
      primary: PRIMARY_ASPECTS.has(aspect),
      pinned,
      groups: Object.fromEntries(groups.map((group) => [group, aspectCells(reported.filter((entry) => entry.group === group), [...SEVERITIES, 'cost'], seed)])),
    }
    if (pinned) continue
    byReviewAgent[aspect] = Object.fromEntries(
      [...new Set(reported.map((entry) => entry.arm))].map((arm) => [
        arm,
        Object.fromEntries(groups.map((group) => [group, aspectCells(reported.filter((entry) => entry.group === group && entry.arm === arm), SEVERITIES, seed)])),
      ]),
    )
  }
  return { byAspect, byReviewAgent }
}

/**
 * Each review section tag's count per group, every branch in the group contributing a zero when it
 * has no finding under the tag.
 *
 * @param {string[]} groups
 * @param {Measured[]} branches
 * @param {number} seed
 */
function sectionCuts(groups, branches, seed) {
  const tags = new Set(branches.flatMap((branch) => Object.keys(branch.sections)))
  return Object.fromEntries(
    [...tags].map((tag) => [
      tag,
      Object.fromEntries(groups.map((group) => [group, cell(branches.filter((branch) => branch.group === group).map((branch) => branch.sections[tag] ?? 0), seed)])),
    ]),
  )
}

/**
 * @param {Record<string, number | null>[]} entries
 * @param {string[]} fields
 * @param {number} seed
 */
function aspectCells(entries, fields, seed) {
  return Object.fromEntries(fields.map((field) => [field, cell(entries.map((entry) => entry[field]).filter(present), seed)]))
}

/**
 * @param {string[]} groups
 * @param {Measured[]} branches
 * @param {number} seed
 */
function metricCells(groups, branches, seed) {
  return Object.fromEntries(
    groups.map((group) => [group, Object.fromEntries(Object.keys(METRICS).map((metric) => [metric, cell(metricValues(branches, group, metric), seed)]))]),
  )
}

/** The group's recorded values of the metric. */
function metricValues(/** @type {Measured[]} */ branches, /** @type {string} */ group, /** @type {string} */ metric) {
  return branches.filter((branch) => branch.group === group).map(METRICS[metric]).filter(present)
}

/**
 * The statistics of `b` minus those of `a`, null where either side has no values.
 *
 * @param {number[]} a
 * @param {number[]} b
 * @param {number} seed
 */
function difference(a, b, seed) {
  const ci = (/** @type {keyof typeof STATISTICS} */ name) => bootstrapDiffCi(a, b, STATISTICS[name], { seed })
  const point = (/** @type {keyof typeof STATISTICS} */ name) => (a.length && b.length ? STATISTICS[name](b) - STATISTICS[name](a) : null)
  return {
    median: point('median'),
    medianCi: ci('median'),
    mean: point('mean'),
    meanCi: ci('mean'),
    shareNonzero: point('shareNonzero'),
    shareNonzeroCi: ci('shareNonzero'),
  }
}

/**
 * @typedef {{ metric: string, statistic: 'median' | 'mean' | 'shareNonzero', favour: 'lower', margin: number }} Rule
 */

/**
 * Which arm the rule favours, from the interval of the second group's statistic minus the first's:
 * wholly below the negative margin favours the second group, wholly above the margin the first.
 *
 * @param {Rule | null} rule
 * @param {string[]} groups
 * @param {Record<string, Record<string, any>>} diff
 */
export function verdict(rule, groups, diff) {
  if (!rule) return { rule, outcome: 'not-set', detail: 'no decision rule is set' }
  const point = diff[rule.metric][rule.statistic]
  const ci = diff[rule.metric][`${rule.statistic}Ci`] 
  const outcome = ci && ci.hi < -rule.margin ? groups[1] : ci && ci.lo > rule.margin ? groups[0] : 'inconclusive'
  const detail = `${rule.metric} ${rule.statistic}, ${groups[1]} minus ${groups[0]}: ${formatEstimate(point, ci)} against margin ${rule.margin}`
  return { rule, outcome, detail }
}
