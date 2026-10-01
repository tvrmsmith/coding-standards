/**
 * Number and interval text shared by the report's verdict detail and its rendering.
 */

/** A number with at most 2 decimals and no trailing zeros, or `n/a` for null. */
export function formatNumber(/** @type {number | null | undefined} */ value) {
  return value == null ? 'n/a' : String(Number(value.toFixed(2)))
}

/** `<point> [<lo>, <hi>]`, with `n/a` for each missing part. */
export function formatEstimate(/** @type {number | null} */ point, /** @type {{ lo: number, hi: number } | null} */ ci) {
  return `${formatNumber(point)} [${formatNumber(ci?.lo)}, ${formatNumber(ci?.hi)}]`
}
