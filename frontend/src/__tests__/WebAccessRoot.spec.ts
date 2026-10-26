import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, onUnmounted } from 'vue'
import { createWebAccessGate } from '@/services/webAccess'

const harness = vi.hoisted(() => ({ gate: null as ReturnType<typeof createWebAccessGate> | null, unmounted: vi.fn(), mounted: vi.fn() }))
vi.mock('@/services/webAccess', async importOriginal => {
  const original = await importOriginal<typeof import('@/services/webAccess')>()
  return { ...original, get webAccessGate() { return harness.gate } }
})
vi.mock('@/App.vue', () => ({ default: defineComponent({ setup() { harness.mounted(); onUnmounted(harness.unmounted); return () => h('div', 'normal app') } }) }))

describe('web access root lifecycle', () => {
  beforeEach(() => { vi.clearAllMocks() })
  afterEach(() => { harness.gate?.setEnabled(false) })

  it('mounts the application directly when the switch is disabled', async () => {
    const fetchStatus = vi.fn()
    harness.gate = createWebAccessGate(fetchStatus)
    const { default: WebAccessRoot } = await import('../WebAccessRoot.vue')
    const wrapper = mount(WebAccessRoot)
    expect(wrapper.text()).toContain('normal app')
    expect(wrapper.find('[data-testid="web-access-loading"]').exists()).toBe(false)
    expect(fetchStatus).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('never mounts the application while blocked and unmounts it when access changes', async () => {
    const fetchStatus = vi.fn().mockResolvedValue({ enabled: true, blocked: true, message: '限制访问' })
    harness.gate = createWebAccessGate(fetchStatus)
    harness.gate.setEnabled(true)
    const { default: WebAccessRoot } = await import('../WebAccessRoot.vue')
    const wrapper = mount(WebAccessRoot)
    await harness.gate.check()
    await flushPromises()
    expect(harness.mounted).not.toHaveBeenCalled()
    fetchStatus.mockResolvedValue({ enabled: true, blocked: false, message: '' })
    await harness.gate.check(true)
    await flushPromises()
    expect(wrapper.text()).toContain('normal app')
    fetchStatus.mockResolvedValue({ enabled: true, blocked: true, message: '限制访问' })
    await harness.gate.check(true)
    await flushPromises()
    expect(harness.unmounted).toHaveBeenCalledOnce()
    expect(wrapper.text()).not.toContain('normal app')
    wrapper.unmount()
  })
})
