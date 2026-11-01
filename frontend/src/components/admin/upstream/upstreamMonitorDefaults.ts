import type { UpstreamProvider } from '@/api/admin/upstreamCenter'

const defaultModels: Record<UpstreamProvider, string> = {
  openai: 'gpt-6.1-sol',
  anthropic: 'claude-sonnet-5',
  gemini: 'gemini-2.5-flash',
}

export const DEFAULT_UPSTREAM_MONITOR_INTERVAL = 60
export function defaultUpstreamMonitorModel(provider: UpstreamProvider): string {
  return defaultModels[provider]
}
