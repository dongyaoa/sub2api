/** Input, cache creation, and cache read tokens are stored as separate counts. */
export function getCacheHitRate(
  inputTokens = 0,
  cacheCreationTokens = 0,
  cacheReadTokens = 0,
): number {
  const promptTokens = inputTokens + cacheCreationTokens + cacheReadTokens
  return promptTokens > 0 ? (cacheReadTokens / promptTokens) * 100 : 0
}
