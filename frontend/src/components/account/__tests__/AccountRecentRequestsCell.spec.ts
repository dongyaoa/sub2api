import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import AccountRecentRequestsCell from '../AccountRecentRequestsCell.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

const { copyToClipboard } = vi.hoisted(() => ({ copyToClipboard: vi.fn().mockResolvedValue(true) }))

vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/utils/format', () => ({ formatDateTime: (date: Date) => date.toISOString() }))

let wrapper: VueWrapper | undefined
afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('AccountRecentRequestsCell', () => {
  it('keeps ten gray slots for empty history and uses a separate hint for load failures', async () => {
    wrapper = mount(AccountRecentRequestsCell)
    expect(wrapper.findAll('[data-testid="recent-request-placeholder"]')).toHaveLength(10)
    expect(wrapper.findAll('[data-testid="recent-request-bar"]')).toHaveLength(0)
    expect(wrapper.find('[data-testid="recent-request-time"]').exists()).toBe(false)
    const stripElement = wrapper.get('[data-testid="recent-request-strip"]').element
    expect(wrapper.find('[data-testid="recent-request-load-error"]').exists()).toBe(false)
    await wrapper.setProps({ error: true })
    expect(wrapper.findAll('[data-testid="recent-request-placeholder"]')).toHaveLength(10)
    expect(wrapper.get('[data-testid="recent-request-load-error"]').attributes('aria-label')).toBe('admin.accounts.recentRequests.loadFailed')
    expect(wrapper.text()).not.toContain('admin.accounts.recentRequests.loadFailed')
    await wrapper.setProps({ error: false, loading: true })
    expect(wrapper.attributes('aria-busy')).toBe('true')
    expect(wrapper.findAll('[data-testid="recent-request-placeholder"]')).toHaveLength(10)
    expect(wrapper.get('[data-testid="recent-request-strip"]').element).toBe(stripElement)
    expect(wrapper.find('[data-testid="recent-request-time"]').exists()).toBe(false)
    expect(wrapper.find('.animate-pulse').exists()).toBe(false)
  })

  it('shows local request time and successful/failed/unknown bars', () => {
    wrapper = mount(AccountRecentRequestsCell, {
      props: { requests: [
        { created_at: '2026-09-11T09:40:40', success: true, status_code: 200 },
        { created_at: '2026-09-11T09:40:39', success: false, status_code: 400 },
        { created_at: '2026-09-11T09:40:38' }
      ] }
    })
    expect(wrapper.get('[data-testid="recent-request-time"]').text()).toBe('09/11 09:40:40')
    const bars = wrapper.findAll('[data-testid="recent-request-bar"]')
    expect(bars[0].classes()).toContain('recent-request-bar--unknown')
    expect(bars[1].classes()).toContain('recent-request-bar--failure')
    expect(bars[2].classes()).toContain('recent-request-bar--success')
    const slots = wrapper.findAll('[data-testid="recent-request-placeholder"], [data-testid="recent-request-bar"]')
    expect(slots).toHaveLength(10)
    expect(slots.slice(0, 7).every(slot => slot.attributes('data-testid') === 'recent-request-placeholder')).toBe(true)
    expect(slots[9].attributes('title')).toBe('200')
  })

  it('preserves existing records and their order when a background refresh fails', async () => {
    wrapper = mount(AccountRecentRequestsCell, {
      props: { requests: [
        { success: false, status_code: 429 },
        { success: true, status_code: 200 }
      ] }
    })
    await wrapper.setProps({ error: true })
    expect(wrapper.findAll('[data-testid="recent-request-placeholder"]')).toHaveLength(8)
    expect(wrapper.findAll('[data-testid="recent-request-bar"]').map(bar => bar.attributes('title'))).toEqual(['200', '429'])
    expect(wrapper.find('[data-testid="recent-request-load-error"]').exists()).toBe(true)
  })

  it('opens failure details on hover and copies the HTTP error with model and proxy', async () => {
    wrapper = mount(AccountRecentRequestsCell, {
      attachTo: document.body,
      props: { requests: [{
        created_at: '2026-09-11T09:40:40Z', success: false, status_code: 400,
        error_message: 'level "minimal" not supported, valid levels: low, medium, high',
        model: 'gpt-6-astra', proxy_id: 12, proxy_name: 'US A', account_name: 'Example account'
      }] }
    })
    await wrapper.getComponent(HelpTooltip).trigger('mouseenter')
    await flushPromises()
    const tooltip = document.body.querySelector('[role="tooltip"]') as HTMLElement
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('HTTP 400')
    expect(tooltip.textContent).toContain('level "minimal" not supported')
    expect(tooltip.textContent).toContain('gpt-6-astra')
    expect(tooltip.textContent).toContain('US A')
    expect(tooltip.textContent).toContain('Example account')
    ;(tooltip.querySelector('button') as HTMLButtonElement).click()
    await flushPromises()
    expect(copyToClipboard).toHaveBeenCalledWith(expect.stringContaining('HTTP 400\n'))
    expect(copyToClipboard).toHaveBeenCalledWith(expect.stringContaining('US A'))
    expect(copyToClipboard).toHaveBeenCalledWith(expect.stringContaining('Example account'))
  })

  it.each([
    ['direct', 'admin.accounts.testProxyRoute.direct'],
    ['no_proxy', 'admin.accounts.testProxyRoute.direct'],
    ['unknown', 'admin.accounts.testProxyRoute.unknown']
  ])('translates the %s proxy route for display and copying', async (proxyName, expected) => {
    wrapper = mount(AccountRecentRequestsCell, {
      attachTo: document.body,
      props: { requests: [{ success: true, proxy_name: proxyName }] }
    })
    await wrapper.getComponent(HelpTooltip).trigger('mouseenter')
    await flushPromises()
    const tooltip = document.body.querySelector('[role="tooltip"]') as HTMLElement
    expect(tooltip.textContent).toContain(expected)
    ;(tooltip.querySelector('button') as HTMLButtonElement).click()
    await flushPromises()
    expect(copyToClipboard).toHaveBeenCalledWith(expect.stringContaining(expected))
  })

  it('retains a stream failure even when the HTTP response was 200 and limits history to ten bars', () => {
    wrapper = mount(AccountRecentRequestsCell, {
      props: { requests: Array.from({ length: 15 }, () => ({ success: false, status_code: 200, error_message: 'stream interrupted' })) }
    })
    const bars = wrapper.findAll('[data-testid="recent-request-bar"]')
    expect(bars).toHaveLength(10)
    expect(bars[0].classes()).toContain('recent-request-bar--failure')
    expect(wrapper.findAll('[data-testid="recent-request-placeholder"]')).toHaveLength(0)
  })
})
