import type { MonitorDisplayGroup, MonitorDisplayOrder, UserMonitorView } from '@/api/channelMonitor'

export const MONITOR_DISPLAY_GROUPS: readonly MonitorDisplayGroup[] = ['openai', 'anthropic', 'other']

export function getMonitorDisplayGroup(provider: string): MonitorDisplayGroup {
  return provider === 'openai' || provider === 'anthropic' ? provider : 'other'
}

export function normalizeMonitorGroupOrder(order?: readonly string[]): MonitorDisplayGroup[] {
  const groups = (order ?? []).filter((group): group is MonitorDisplayGroup =>
    MONITOR_DISPLAY_GROUPS.includes(group as MonitorDisplayGroup),
  )
  return [...new Set([...groups, ...MONITOR_DISPLAY_GROUPS])]
}

export function sortMonitorItems<T extends { id: number; provider: string }>(
  items: readonly T[],
  order?: MonitorDisplayOrder,
): T[] {
  const groups = normalizeMonitorGroupOrder(order?.group_order)
  const rank = new Map((order?.monitor_order ?? []).map((id, index) => [id, index]))
  return [...items].sort((a, b) => {
    const groupDifference = groups.indexOf(getMonitorDisplayGroup(a.provider)) - groups.indexOf(getMonitorDisplayGroup(b.provider))
    if (groupDifference) return groupDifference
    const positionA = rank.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const positionB = rank.get(b.id) ?? Number.MAX_SAFE_INTEGER
    return positionA - positionB || a.id - b.id
  })
}

export function groupMonitorItems<T extends { id: number; provider: string }>(
  items: readonly T[],
  order?: MonitorDisplayOrder,
): Array<{ id: MonitorDisplayGroup; items: T[] }> {
  const sorted = sortMonitorItems(items, order)
  return normalizeMonitorGroupOrder(order?.group_order)
    .map(id => ({ id, items: sorted.filter(item => getMonitorDisplayGroup(item.provider) === id) }))
    .filter(group => group.items.length > 0)
}

export function providerTintClass(provider: string): string {
  const tint: Record<string, string> = {
    openai: 'text-emerald-600 dark:text-emerald-300',
    anthropic: 'text-orange-600 dark:text-orange-300',
    gemini: 'text-sky-600 dark:text-sky-300',
    grok: 'text-zinc-700 dark:text-zinc-200',
    antigravity: 'text-purple-600 dark:text-purple-300',
    kimi: 'text-pink-600 dark:text-pink-300',
    zhipu: 'text-indigo-600 dark:text-indigo-300',
    deepseek: 'text-teal-600 dark:text-teal-300',
    minimax: 'text-rose-600 dark:text-rose-300',
    opencode_go: 'text-amber-700 dark:text-amber-300',
  }
  return tint[provider] ?? 'text-violet-600 dark:text-violet-300'
}

export function getMonitorLastCheckedAt(item: UserMonitorView): string | null {
  // Prefer the actual primary-model sample over a scheduler timestamp.
  for (const value of [item.timeline?.[0]?.checked_at, item.last_checked_at]) {
    if (value && Number.isFinite(Date.parse(value))) return value
  }
  return null
}

export function getMonitorFreshness(item: UserMonitorView, now: number): 'fresh' | 'stale' | 'unknown' {
  const checkedAt = getMonitorLastCheckedAt(item)
  if (!checkedAt) return 'unknown'
  const interval = Math.max(15, item.interval_seconds ?? 60)
  const jitter = Math.max(0, item.jitter_seconds ?? 0)
  const staleAfter = Math.max(180, (interval + jitter) * 3) * 1000
  return now - Date.parse(checkedAt) > staleAfter ? 'stale' : 'fresh'
}
