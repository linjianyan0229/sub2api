import { describe, expect, it } from 'vitest'

import { formatAverageTps, getAverageTps, getAverageTpsClass } from '@/utils/usageThroughput'

describe('average TPS', () => {
  it('divides output tokens by total duration in seconds', () => {
    expect(getAverageTps({
      output_tokens: 100,
      duration_ms: 2500,
    })).toBe(40)
  })

  it.each([null, undefined, 0, 1000, 3000, 4000, -1, NaN, Infinity])('ignores first-token latency %s', (firstTokenMs) => {
    const row = {
      output_tokens: 100,
      duration_ms: 3000,
      first_token_ms: firstTokenMs,
    }
    expect(formatAverageTps(row)).toBe('33.3 t/s')
  })

  it('keeps zero output distinct from missing token data', () => {
    expect(formatAverageTps({
      output_tokens: 0,
      duration_ms: 3000,
    })).toBe('0.0 t/s')
  })

  it.each([null, undefined, 0, -1, NaN, Infinity])('rejects invalid total duration %s', (durationMs) => {
    const row = {
      output_tokens: 100,
      duration_ms: durationMs,
    }
    expect(formatAverageTps(row)).toBe('-')
    expect(getAverageTpsClass(row)).toBe('text-gray-400 dark:text-gray-500')
  })

  it.each([null, undefined, -1, NaN, Infinity])('rejects invalid output tokens %s', (outputTokens) => {
    const row = {
      output_tokens: outputTokens,
      duration_ms: 3000,
    }
    expect(formatAverageTps(row)).toBe('-')
    expect(getAverageTpsClass(row)).toBe('text-gray-400 dark:text-gray-500')
  })

  it.each([
    [0, 'text-red-600 dark:text-red-400'],
    [9.99, 'text-red-600 dark:text-red-400'],
    [10, 'text-yellow-600 dark:text-yellow-400'],
    [15, 'text-yellow-600 dark:text-yellow-400'],
    [20, 'text-yellow-600 dark:text-yellow-400'],
    [20.01, 'text-emerald-600 dark:text-emerald-400'],
    [50, 'text-emerald-600 dark:text-emerald-400'],
  ])('colors an unrounded average TPS of %s correctly', (tps, expectedClass) => {
    expect(getAverageTpsClass({
      output_tokens: tps * 100,
      duration_ms: 100_000,
    })).toBe(expectedClass)
  })
})
