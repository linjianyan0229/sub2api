interface UsageTiming {
  output_tokens?: number | null
  duration_ms?: number | null
}

export function getAverageTps(row: UsageTiming): number | null {
  const outputTokens = row.output_tokens
  const durationMs = row.duration_ms

  if (
    outputTokens == null || !Number.isFinite(outputTokens) || outputTokens < 0 ||
    durationMs == null || !Number.isFinite(durationMs) || durationMs <= 0
  ) {
    return null
  }

  const tokensPerSecond = outputTokens / (durationMs / 1000)
  return Number.isFinite(tokensPerSecond) ? tokensPerSecond : null
}

export function formatAverageTps(row: UsageTiming): string {
  const tps = getAverageTps(row)
  return tps == null ? '-' : `${tps.toFixed(1)} t/s`
}

export function getAverageTpsClass(row: UsageTiming): string {
  const tps = getAverageTps(row)
  if (tps == null) return 'text-gray-400 dark:text-gray-500'
  if (tps > 20) return 'text-emerald-600 dark:text-emerald-400'
  if (tps >= 10) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-red-600 dark:text-red-400'
}
