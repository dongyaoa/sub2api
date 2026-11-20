import { afterEach, describe, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import type { PelicanMonitorGroup, PelicanRun } from '@/api/pelicanMonitor'
import zh from '@/i18n/locales/zh/pelicanMonitor'
import PelicanArtworkDialog from './PelicanArtworkDialog.vue'

function group(id: number, model: string, ids: number[]): PelicanMonitorGroup {
  const runs = ids.map(n => ({ id: n, plan_id: id, model, status: 'succeeded', reasoning_effort: 'high', created_at: '2026-11-20T10:00:00Z', started_at: null, finished_at: null, duration_ms: 70000, error: '' } satisfies PelicanRun))
  return { id, group_id: 7, model, group_name: '示例分组', group_rate_multiplier: 1.5, enabled: true, interval_seconds: 900, next_run_at: null, last_run_at: null, reasoning_effort: 'high', recent_runs: runs, latest_run: runs[0] ?? null }
}
let wrapper: VueWrapper | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined })
it('compares the selected record position, leaves missing artwork empty, and drops an unpublished plan on refresh', async () => {
  const a = group(1, 'gpt-6.2-astra', [11,10]), b = group(2, 'gpt-6-astra', [21]), c = group(3, 'gpt-6.2-sol', [33,32,31])
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { ...zh, common: { close: '关闭' } } } })
  wrapper = mount(PelicanArtworkDialog, { props: { plan: a, plans: [a,b,c], run: a.recent_runs[1]!, forceCompare: true }, global: { plugins: [i18n], stubs: { teleport: true, PelicanArtworkPreview: { props: ['run'], template: '<div :data-art="run.id" />' } } } })
  expect(wrapper.get('#modalTitle').text()).toContain('第 2 轮检测')
  expect(wrapper.get('[data-compare-plan="1"] [data-art]').attributes('data-art')).toBe('10')
  expect(wrapper.get('[data-compare-plan="2"]').text()).toContain('本轮暂无作品')
  expect(wrapper.find('[data-art="21"]').exists()).toBe(false)
  expect(wrapper.get('[data-compare-plan="3"] [data-art]').attributes('data-art')).toBe('32')
  await wrapper.setProps({ plans: [a,b] })
  expect(wrapper.find('[data-art="32"]').exists()).toBe(false)
  await wrapper.get('#modalCompareToggle').trigger('click')
  expect(wrapper.findAll('[data-art]')).toHaveLength(1)
  expect(wrapper.get('[data-art]').attributes('data-art')).toBe('10')
  expect(wrapper.get('#modalTitle').text()).toContain('#10')
  await wrapper.get('#modalBody').trigger('click')
  expect(wrapper.emitted('close')).toBeUndefined()
  await wrapper.get('#previewModal').trigger('click')
  expect(wrapper.emitted('close')).toHaveLength(1)
})
describe('artwork dialog focus', () => {
  it('restores the opener when dismissed with Escape', async () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    try {
      opener.focus()
      const plan = group(1, 'gpt-6-astra', [42])
      wrapper = mount(PelicanArtworkDialog, { props: { plan, plans: [plan], run: plan.recent_runs[0]! }, global: { plugins: [createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { ...zh, common: { close: '关闭' } } } })], stubs: { PelicanArtworkPreview: true } } })
      await flushPromises()
      expect(document.activeElement).toBe(document.querySelector('#modalCompareToggle'))
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      expect(wrapper.emitted('close')).toHaveLength(1)
      wrapper.unmount(); wrapper = undefined
      expect(document.activeElement).toBe(opener)
    } finally { opener.remove() }
  })
})
