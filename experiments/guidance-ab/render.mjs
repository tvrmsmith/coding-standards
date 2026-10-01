/**
 * Renders the guidance A/B report as plain text for a terminal.
 */
import { formatEstimate, formatNumber } from './format.mjs'

/**
 * @param {Record<string, any>} report an `analyze` report
 * @returns {string}
 */
export function render(report) {
  const { mode, groups, caveat, compliance, summary, diff, excluded, armCheck, verdict } = report
  const totals = Object.keys(diff).map((metric) => [
    metric,
    ...groups.map((group) => cellText(summary[group][metric])),
    formatEstimate(diff[metric].median, diff[metric].medianCi),
  ])
  const sections = [
    [`guidance A/B report, mode: ${mode}`],
    caveat ? [caveat] : [],
    groups.map((group) => `${group}: ${compliance[group].loaded}/${compliance[group].total} loaded (${compliance[group].unknown} unknown)`),
    ['Totals', ...table(['metric', ...groups, `diff (${groups[1]} - ${groups[0]})`], totals)],
    ...cuts(report),
    ['Excluded', ...excludedLines(excluded, groups)],
    armCheck ? armCheckLines(armCheck) : [],
    [verdict && verdict.outcome !== 'not-set' ? `Decision rule: ${verdict.outcome}: ${verdict.detail}` : 'Decision rule: not set'],
  ]
  return `${sections.filter((lines) => lines.length).map((lines) => lines.join('\n')).join('\n\n')}\n`
}

/** @param {{ median: number | null, medianCi: { lo: number, hi: number } | null }} cell */
function cellText({ median, medianCi }) {
  return formatEstimate(median, medianCi)
}

/**
 * Left-aligned columns under a header row.
 *
 * @param {string[]} header
 * @param {string[][]} rows
 */
function table(header, rows) {
  const all = [header, ...rows]
  const widths = header.map((_, column) => Math.max(...all.map((row) => row[column].length)))
  return all.map((row) => row.map((text, column) => text.padEnd(widths[column])).join('  ').trimEnd())
}

/**
 * One table per cut, each row a label and the median of each group's cell. An empty cut prints nothing.
 *
 * @param {Record<string, any>} report
 * @returns {string[][]}
 */
function cuts({ groups, byStratum, byAspect, byReviewAgent, bySection }) {
  /** @type {(label: string, perGroup: Record<string, any>) => string[]} */
  const row = (label, perGroup) => [label, ...groups.map((group) => cellText(perGroup[group]))]
  /** @type {(label: string, perGroup: Record<string, Record<string, any>>) => string[][]} */
  const fieldRows = (label, perGroup) =>
    Object.keys(perGroup[groups[0]]).map((field) => row(`${label} ${field}`, Object.fromEntries(groups.map((group) => [group, perGroup[group][field]]))))
  const stratum = Object.entries(byStratum).flatMap(([name, perGroup]) => fieldRows(name, perGroup))
  const aspect = Object.entries(byAspect).flatMap(([name, { primary, pinned, groups: perGroup }]) =>
    fieldRows(`${name}${primary ? ' (primary)' : ''}${pinned ? ' (pinned)' : ''}`, perGroup),
  )
  const agent = Object.entries(byReviewAgent).flatMap(([name, arms]) =>
    Object.entries(arms).flatMap(([arm, perGroup]) => fieldRows(`${name} ${arm}`, perGroup)),
  )
  const section = Object.entries(bySection).map(([tag, perGroup]) => row(tag, perGroup))
  return [
    ['By stratum', stratum],
    ['By aspect', aspect],
    ['By review agent', agent],
    ['By section', section],
  ]
    .filter(([, rows]) => rows.length)
    .map(([title, rows]) => [title, ...table(['', ...groups], rows)])
}

/**
 * @param {{ group: string | null, reason: string }[]} excluded
 * @param {string[]} groups
 */
function excludedLines(excluded, groups) {
  if (!excluded.length) return ['none']
  const reasons = [...new Set(excluded.map(({ reason }) => reason))]
  return reasons.map((reason) => {
    const counts = [...groups, null].map((group) => [group ?? 'no group', excluded.filter((e) => e.reason === reason && e.group === group).length])
    return `${reason}: ${counts.filter(([, n]) => n).map(([name, n]) => `${name} ${n}`).join(', ')}`
  })
}

/** @param {{ ratio: Record<string, number>, hashMismatches: unknown[] }} armCheck */
function armCheckLines({ ratio, hashMismatches }) {
  const arms = Object.entries(ratio).map(([arm, n]) => `${arm} ${formatNumber(n)}`).join(', ')
  return ['Arm check', `arm ratio: ${arms}`, `hash mismatches: ${hashMismatches.length}`]
}
