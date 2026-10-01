/**
 * The stats seam: the summary statistics and seeded bootstrap intervals the analysis reports per arm.
 *
 * Every interval is reproducible, so each test pins a seed and asserts either an exact value
 * (degenerate inputs have one) or a bound the true interval must respect.
 */
import { describe, test } from 'node:test'
import assert from 'node:assert/strict'
import { bootstrapCi, bootstrapDiffCi, mean, median, shareNonzero } from '../stats.mjs'

describe('median', () => {
  test('is null for no values', () => {
    assert.equal(median([]), null)
  })

  test('is the only value of a single-value list', () => {
    assert.equal(median([5]), 5)
  })

  test('is the middle value of an unsorted odd-length list', () => {
    assert.equal(median([3, 1, 2]), 2)
  })

  test('is the mean of the two middle values of an even-length list', () => {
    assert.equal(median([4, 1, 3, 2]), 2.5)
  })

  test('leaves its input unsorted', () => {
    const xs = [3, 1, 2]
    median(xs)
    assert.deepEqual(xs, [3, 1, 2])
  })
})

describe('mean', () => {
  test('is null for no values', () => {
    assert.equal(mean([]), null)
  })

  test('is the sum over the count', () => {
    assert.equal(mean([1, 2, 3, 6]), 3)
  })
})

describe('shareNonzero', () => {
  test('is null for no values', () => {
    assert.equal(shareNonzero([]), null)
  })

  test('is the fraction of entries that are not zero', () => {
    assert.equal(shareNonzero([0, 0, 3, 1]), 0.5)
  })
})

describe('bootstrapCi', () => {
  test('is null for no values', () => {
    assert.equal(bootstrapCi([], median, { seed: 1 }), null)
  })

  test('collapses to the value when every entry is the same', () => {
    assert.deepEqual(bootstrapCi([7, 7, 7, 7], median, { seed: 1 }), { lo: 7, hi: 7 })
  })

  test('keeps a median interval inside the data range and around the sample median', () => {
    const ci = bootstrapCi([1, 2, 3, 4, 5, 6, 7, 8, 9], median, { seed: 1 })
    assert.ok(ci.lo > 1 && ci.hi < 9, `interval ${JSON.stringify(ci)} touches the data range`)
    assert.ok(ci.lo <= 5 && 5 <= ci.hi, `interval ${JSON.stringify(ci)} excludes the median 5`)
  })

  test('keeps a mean interval ordered within the data range', () => {
    const { lo, hi } = bootstrapCi([0, 0, 0, 10], mean, { seed: 1 })
    assert.ok(0 <= lo && lo < hi && hi <= 10, `interval ${lo}..${hi} is not ordered within 0..10`)
  })

  test('returns the same interval for the same seed', () => {
    const values = [1, 2, 3, 4, 5, 6, 7, 8, 9]
    assert.deepEqual(bootstrapCi(values, mean, { seed: 3 }), bootstrapCi(values, mean, { seed: 3 }))
  })
})

describe('bootstrapDiffCi', () => {
  test('is b minus a, collapsing when each side is constant', () => {
    assert.deepEqual(bootstrapDiffCi([1, 1, 1, 1], [5, 5, 5, 5], median, { seed: 1 }), { lo: 4, hi: 4 })
  })

  test('is null when either side is empty', () => {
    assert.equal(bootstrapDiffCi([], [1], median, { seed: 1 }), null)
    assert.equal(bootstrapDiffCi([1], [], median, { seed: 1 }), null)
  })

  test('spans zero when both sides are the same sample', () => {
    const { lo, hi } = bootstrapDiffCi([0, 1, 0, 1], [0, 1, 0, 1], shareNonzero, { seed: 2 })
    assert.ok(lo <= 0 && 0 <= hi, `interval ${lo}..${hi} excludes 0`)
  })
})
