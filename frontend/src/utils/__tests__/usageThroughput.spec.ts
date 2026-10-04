import { describe, expect, it } from 'vitest'

import { formatOutputTokensPerSecond } from '@/utils/usageThroughput'

describe('formatOutputTokensPerSecond', () => {
  it('excludes first-token latency and converts milliseconds to seconds', () => {
    expect(formatOutputTokensPerSecond({
      output_tokens: 100,
      duration_ms: 3000,
      first_token_ms: 1000,
    })).toBe('50.0 t/s')
  })

  it.each([null, undefined, 0])('uses total duration when first-token latency is %s', (firstTokenMs) => {
    expect(formatOutputTokensPerSecond({
      output_tokens: 100,
      duration_ms: 3000,
      first_token_ms: firstTokenMs,
    })).toBe('33.3 t/s')
  })

  it('keeps zero output distinct from missing token data', () => {
    expect(formatOutputTokensPerSecond({
      output_tokens: 0,
      duration_ms: 3000,
      first_token_ms: 1000,
    })).toBe('0.0 t/s')
  })

  it.each([null, undefined, 0, -1, NaN, Infinity])('rejects invalid total duration %s', (durationMs) => {
    expect(formatOutputTokensPerSecond({
      output_tokens: 100,
      duration_ms: durationMs,
    })).toBe('-')
  })

  it.each([-1, NaN, Infinity, 3000, 4000])('rejects invalid first-token latency %s', (firstTokenMs) => {
    expect(formatOutputTokensPerSecond({
      output_tokens: 100,
      duration_ms: 3000,
      first_token_ms: firstTokenMs,
    })).toBe('-')
  })

  it.each([null, undefined, -1, NaN, Infinity])('rejects invalid output tokens %s', (outputTokens) => {
    expect(formatOutputTokensPerSecond({
      output_tokens: outputTokens,
      duration_ms: 3000,
    })).toBe('-')
  })
})
