/**
 * Summary statistics and seeded bootstrap intervals for the guidance A/B analysis.
 */

/** The middle value of `values`, or the mean of the two middle values; null for none. */
export function median(values) {
  if (values.length === 0) return null
  const sorted = [...values].sort((a, b) => a - b)
  const mid = sorted.length >> 1
  return sorted.length % 2 === 1 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

/** The arithmetic mean of `values`; null for none. */
export function mean(values) {
  if (values.length === 0) return null
  return values.reduce((sum, v) => sum + v, 0) / values.length
}

/** The share of `values` entries that are not zero; null for none. */
export function shareNonzero(values) {
  if (values.length === 0) return null
  return values.filter((v) => v !== 0).length / values.length
}

/** Mulberry32: a 32-bit seeded PRNG giving the same stream on every run and platform. */
function seededRandom(seed) {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let t = state
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** `values.length` entries drawn from `values` with replacement. */
function resample(values, random) {
  return values.map(() => values[Math.floor(random() * values.length)])
}

/** The 2.5th and 97.5th percentiles of `draws`, which it sorts in place. */
function percentileInterval(draws) {
  draws.sort((a, b) => a - b)
  const last = draws.length - 1
  return { lo: draws[Math.floor(0.025 * last)], hi: draws[Math.ceil(0.975 * last)] }
}

/** Percentile 95% interval of `statistic` over seeded bootstrap resamples of `values`; null for none. */
export function bootstrapCi(values, statistic, { seed, resamples = 2000 }) {
  if (values.length === 0) return null
  const random = seededRandom(seed)
  const draws = Array.from({ length: resamples }, () => statistic(resample(values, random)))
  return percentileInterval(draws)
}

/** Percentile 95% interval of `statistic(b) - statistic(a)` over independent seeded resamples; null when either is empty. */
export function bootstrapDiffCi(a, b, statistic, { seed, resamples = 2000 }) {
  if (a.length === 0 || b.length === 0) return null
  const random = seededRandom(seed)
  const draws = Array.from({ length: resamples }, () => {
    const resampledA = resample(a, random)
    const resampledB = resample(b, random)
    return statistic(resampledB) - statistic(resampledA)
  })
  return percentileInterval(draws)
}
