import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { PelicanMonitorGroup, PelicanRun } from '@/api/pelicanMonitor'
import PelicanGroupCard from './PelicanGroupCard.vue'
import type { PelicanViewMode } from './pelicanFormat'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('zh-CN'), t: (key: string) => key }) }))
vi.mock('./PelicanArtworkPreview.vue', () => ({ default: { template: '<div />' } }))
function plan(id: number, ids: number[], model: string): PelicanMonitorGroup {
  const works = ids.map(runId => ({ id: runId, plan_id: id, model, reasoning_effort: 'high', status: 'succeeded', duration_ms: 1000, created_at: '2026-11-20T14:40:00Z', started_at: null, finished_at: null, error: '' }) as PelicanRun)
  return { id, group_id: 22, group_name: 'Group', group_rate_multiplier: 0.15, model, reasoning_effort: 'high', interval_seconds: 900, enabled: true, latest_run: works[0] || null, recent_runs: works, last_run_at: null, next_run_at: null }
}
const a = plan(3, [31], 'gpt-6.2-sol'), b = plan(4, [41, 42, 43], 'gpt-6.1-sol')
function render(mode: PelicanViewMode) {
  return mount(PelicanGroupCard, { props: { plans: [a, b], now: 0, mode }, global: { stubs: { PelicanArtworkPreview: true } } })
}
describe('pelican demo gallery modes', () => {
  beforeEach(() => vi.clearAllMocks())
  it('switches the single model from either picker and matches the source when opening its artwork', async () => {
    const card = render('single')
    expect(card.findAll('.pelican-work')).toHaveLength(1)
    await card.get('[data-testid="quick-model-4"]').trigger('click')
    expect(card.get('[data-testid="choose-plan-4"]').attributes('aria-pressed')).toBe('true')
    expect(card.findAll('.pelican-work')).toHaveLength(3)
    await card.get('[data-run-id="42"] button').trigger('click')
    expect(card.emitted('open')?.[0]).toEqual([b, b.recent_runs[1], false])
    await card.get('[data-testid="card-mode-paired"]').trigger('click')
    expect(card.emitted('mode')).toEqual([['paired']])
    card.unmount()
  })
  it('keeps unequal histories at the correct ordinal and retains at least one checked model', async () => {
    const card = render('paired')
    const ids = (round: number) => card.get('[data-round="' + round + '"]').findAll('.pelican-work').map(work => work.attributes('data-run-id'))
    expect(ids(0)).toEqual(['31', '41'])
    expect(ids(1)).toEqual(['42'])
    expect(ids(2)).toEqual(['43'])
    await card.get('[data-testid="compare-round-2"]').trigger('click')
    expect(card.emitted('open')?.[0]).toEqual([b, b.recent_runs[2], true])
    await card.get('[data-testid="quick-model-3"]').trigger('click')
    await card.get('[data-testid="choose-plan-4"]').trigger('click')
    expect(ids(0)).toEqual(['41'])
    expect(card.get('[data-testid="choose-plan-4"]').attributes('aria-pressed')).toBe('true')
    await card.setProps({ plans: [a] })
    expect(ids(0)).toEqual(['31'])
    card.unmount()
  })
  it('shares a single scroller across the selected matrix rows in both directions', async () => {
    const card = render('stacked')
    expect(card.findAll('[data-testid="pelican-gallery"]')).toHaveLength(1)
    expect(card.findAll('[data-model-row]').map(row => row.attributes('data-model-row'))).toEqual(['3', '4'])
    const scrollBy = vi.fn()
    const track = card.get('[data-testid="pelican-gallery"]').element
    Object.defineProperty(track, 'scrollBy', { value: scrollBy })
    await card.get('[aria-label="pelicanMonitor.card.right"]').trigger('click')
    await card.get('[aria-label="pelicanMonitor.card.left"]').trigger('click')
    expect(scrollBy.mock.calls).toEqual([[{ left:380, behavior:'smooth' }], [{ left:-380, behavior:'smooth' }]])
    await card.get('[data-testid="choose-plan-3"]').trigger('click')
    expect(card.findAll('[data-model-row]')).toHaveLength(1)
    card.unmount()
  })
})
