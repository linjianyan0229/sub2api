interface UsageTiming {
  output_tokens?: number | null
  duration_ms?: number | null
  first_token_ms?: number | null
}

export function formatOutputTokensPerSecond(row: UsageTiming): string {
  const outputTokens = row.output_tokens
  const durationMs = row.duration_ms
  const firstTokenMs = row.first_token_ms ?? 0

  if (
    outputTokens == null || !Number.isFinite(outputTokens) || outputTokens < 0 ||
    durationMs == null || !Number.isFinite(durationMs) || durationMs <= 0 ||
    !Number.isFinite(firstTokenMs) || firstTokenMs < 0
  ) {
    return '-'
  }

  // Exclude first-token waiting time when recorded; otherwise use total duration.
  const generationMs = durationMs - firstTokenMs
  if (generationMs <= 0) {
    return '-'
  }

  const tokensPerSecond = outputTokens / generationMs * 1000
  return Number.isFinite(tokensPerSecond) ? `${tokensPerSecond.toFixed(1)} t/s` : '-'
}
