import { describe, expect, it } from 'vitest'
import type { UserMonitorView } from '@/api/channelMonitor'
import {
  getMonitorFreshness,
  getMonitorLastCheckedAt,
  groupMonitorItems,
  normalizeMonitorGroupOrder,
  sortMonitorItems,
} from '../channelMonitorDisplay'

const checkedAt = '2026-09-19T01:00:00.000Z'
const checkedAtMs = Date.parse(checkedAt)

function monitor(overrides: Partial<UserMonitorView> = {}): UserMonitorView {
  return {
    id: 1, name: 'Main', provider: 'openai', group_name: 'Main',
    primary_model: 'test-model', primary_status: 'operational',
    primary_latency_ms: 120, primary_ping_latency_ms: 20,
    availability_7d: 100, extra_models: [], timeline: [],
    last_checked_at: checkedAt,
    ...overrides,
  }
}

describe('channel monitor grouping and display order', () => {
  const items = [
    { id: 6, provider: 'gemini' },
    { id: 4, provider: 'openai' },
    { id: 2, provider: 'anthropic' },
    { id: 5, provider: 'grok' },
    { id: 3, provider: 'openai' },
    { id: 7, provider: 'future-provider' },
  ]

  it('keeps OpenAI and Anthropic separate and puts all other platforms together', () => {
    expect(groupMonitorItems(items)).toEqual([
      { id: 'openai', items: [{ id: 3, provider: 'openai' }, { id: 4, provider: 'openai' }] },
      { id: 'anthropic', items: [{ id: 2, provider: 'anthropic' }] },
      { id: 'other', items: [
        { id: 5, provider: 'grok' }, { id: 6, provider: 'gemini' }, { id: 7, provider: 'future-provider' },
      ] },
    ])
  })

  it('applies saved group and card order, appending new IDs inside their own groups', () => {
    const saved = { group_order: ['other', 'openai', 'anthropic'] as const, monitor_order: [6, 4, 5, 2, 99] }
    const source = [...items, { id: 8, provider: 'openai' }]
    const order = { ...saved, group_order: [...saved.group_order] }
    expect(sortMonitorItems(source, order).map(item => item.id)).toEqual([6, 5, 7, 4, 3, 8, 2])
    expect(source.map(item => item.id)).toEqual([6, 4, 2, 5, 3, 7, 8])
    expect(sortMonitorItems([...source].reverse(), order).map(item => item.id)).toEqual([6, 5, 7, 4, 3, 8, 2])
  })

  it('repairs incomplete group preferences and omits empty groups', () => {
    expect(normalizeMonitorGroupOrder(['other', 'invalid', 'other'])).toEqual(['other', 'openai', 'anthropic'])
    expect(groupMonitorItems([{ id: 1, provider: 'grok' }]).map(group => group.id)).toEqual(['other'])
    expect(groupMonitorItems([])).toEqual([])
  })
})

describe('channel monitor data freshness', () => {
  it('uses the real primary-model sample even when a scheduler timestamp is newer', () => {
    const item = monitor({
      last_checked_at: '2026-09-19T01:10:00.000Z',
      timeline: [{ checked_at: checkedAt, status: 'operational', latency_ms: 120, ping_latency_ms: 20 }],
    })
    expect(getMonitorLastCheckedAt(item)).toBe(checkedAt)
    expect(getMonitorFreshness(item, checkedAtMs + 600_000)).toBe('stale')
  })

  it('falls back to a valid recorded check and treats missing or invalid dates as unknown', () => {
    const item = monitor({ timeline: [{ checked_at: 'invalid', status: 'error', latency_ms: null, ping_latency_ms: null }] })
    expect(getMonitorLastCheckedAt(item)).toBe(checkedAt)
    expect(getMonitorFreshness(monitor({ last_checked_at: null }), checkedAtMs)).toBe('unknown')
    expect(getMonitorFreshness(monitor({ last_checked_at: 'invalid' }), checkedAtMs)).toBe('unknown')
  })

  it('allows three configured intervals including jitter before marking data stale', () => {
    const item = monitor({ interval_seconds: 120, jitter_seconds: 30 })
    expect(getMonitorFreshness(item, checkedAtMs + 300_000)).toBe('fresh')
    expect(getMonitorFreshness(item, checkedAtMs + 450_000)).toBe('fresh')
    expect(getMonitorFreshness(item, checkedAtMs + 450_001)).toBe('stale')
  })

  it('keeps a three-minute grace period for default and very frequent checks', () => {
    for (const item of [monitor(), monitor({ interval_seconds: 5, jitter_seconds: -10 })]) {
      expect(getMonitorFreshness(item, checkedAtMs + 180_000)).toBe('fresh')
      expect(getMonitorFreshness(item, checkedAtMs + 180_001)).toBe('stale')
    }
  })
})
