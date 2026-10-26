import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import UpstreamSupplierNavigation from './UpstreamSupplierNavigation.vue'
import UpstreamBackToTop from './UpstreamBackToTop.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const items = [{ id: null, name: 'All', count: 2 }, { id: 1, name: 'Alpha', count: 4 }, { id: 2, name: 'Beta', count: 3 }]
let wrapper: VueWrapper | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('docked upstream navigation', () => {
  it('shows the complete navigation immediately and keeps it open after switching', async () => {
    wrapper = mount(UpstreamSupplierNavigation, { props: { items, selectedId: 1 } })
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(3)
    expect(wrapper.find('[aria-expanded]').exists()).toBe(false)
    expect(wrapper.find('button:not([role="tab"])').exists()).toBe(false)
    expect(wrapper.get('#supplier-filter-1').attributes('aria-selected')).toBe('true')
    await wrapper.get('#supplier-filter-2').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[2]])
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(3)
    await wrapper.setProps({ selectedId: 2 })
    expect(wrapper.get('#supplier-filter-2').attributes('tabindex')).toBe('0')
  })

  it('supports vertical and horizontal keyboard navigation in the visible list', async () => {
    wrapper = mount(UpstreamSupplierNavigation, { attachTo: document.body, props: { items, selectedId: 1 } })
    await wrapper.get('#supplier-filter-1').trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement?.id).toBe('supplier-filter-2')
    expect(wrapper.emitted('select')).toEqual([[2]])
    await wrapper.get('#supplier-filter-2').trigger('keydown', { key: 'ArrowRight' })
    expect(document.activeElement?.id).toBe('supplier-filter-all')
    await wrapper.get('#supplier-filter-all').trigger('keydown', { key: 'End' })
    expect(document.activeElement?.id).toBe('supplier-filter-2')
  })

  it('uses document scrolling for back-to-top and respects reduced motion and the mobile menu', async () => {
    let y = 0
    vi.spyOn(window, 'scrollY', 'get').mockImplementation(() => y)
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })))
    wrapper = mount(UpstreamBackToTop)
    expect(wrapper.find('[data-testid="upstream-back-to-top"]').exists()).toBe(false)
    y = 800; window.dispatchEvent(new Event('scroll')); await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="upstream-back-to-top"]').trigger('click')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 0, behavior: 'smooth' })
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true })))
    await wrapper.get('[data-testid="upstream-back-to-top"]').trigger('click')
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 0, behavior: 'auto' })
    await wrapper.setProps({ mobileOpen: true })
    expect(wrapper.find('[data-testid="upstream-back-to-top"]').exists()).toBe(false)
  })
})
