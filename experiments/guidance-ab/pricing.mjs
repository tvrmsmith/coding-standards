/**
 * Prices token counts in input-token equivalents, so the pipeline, implementation and review aspect
 * costs stay comparable.
 */

/**
 * @param {number} input
 * @param {number} cacheWrite
 * @param {number} cacheRead
 * @param {number} output
 */
export function tokenCost(input, cacheWrite, cacheRead, output) {
  return input + cacheWrite * 1.25 + cacheRead * 0.1 + output * 5
}
