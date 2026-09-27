import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ProxySelector from '../ProxySelector.vue'
import type { Proxy } from '@/types'

const testProxy = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, params?: { count: number }) => params ? `${key}:${params.count}` : key
}) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.clearAllMocks() })

async function openSelector(props: {
  proxies?: Proxy[]
  allowNone?: boolean
  placeholder?: string
  showAvailability?: boolean
  ariaLabel?: string
} = {}) {
  const wrapper = mount(ProxySelector, {
    props: { modelValue: null, proxies: [1, 2].map(id => ({
      id, name: `Proxy ${id}`, host: 'localhost', port: 8080, protocol: 'http'
    } as Proxy)), ...props },
    global: { stubs: { Icon: true } }
  })
  await wrapper.get('.select-trigger').trigger('click')
  return wrapper
}

describe('proxy connection tests', () => {
  it('does not restart an individual test when a batch is started', async () => {
    let finish!: (result: object) => void
    testProxy.mockImplementation((id: number) => id === 1
      ? new Promise(resolve => { finish = resolve })
      : Promise.resolve({ success: true, country: 'GB' }))
    const wrapper = await openSelector()
    await wrapper.findAll('.test-btn')[0].trigger('click')
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy.mock.calls.map(([id]) => id)).toEqual([1, 2])
    expect(wrapper.findAll('.test-btn')[0].attributes('disabled')).toBeDefined()
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeDefined()
    await wrapper.get('.batch-test-btn').trigger('click')
    expect(testProxy).toHaveBeenCalledTimes(2)
    finish({ success: true, country: 'US' })
    await flushPromises()
    expect(wrapper.text()).toContain('US')
    expect(wrapper.findAll('.test-btn')[0].attributes('disabled')).toBeUndefined()
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeUndefined()
  })

  it('shows per-proxy outcomes and allows another batch after a failure', async () => {
    testProxy.mockImplementation((id: number) => id === 1
      ? Promise.reject(new Error('offline'))
      : Promise.resolve({ success: true, country: 'GB' }))
    const wrapper = await openSelector()
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('admin.proxies.testFailed')
    expect(wrapper.text()).toContain('GB')
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeUndefined()
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy).toHaveBeenCalledTimes(4)
  })

  it('tests only matching proxies and shows the filtered availability count', async () => {
    testProxy.mockResolvedValue({ success: true })
    const wrapper = await openSelector({ allowNone: false, showAvailability: true })
    await wrapper.get('input').setValue('proxy 2')

    expect(wrapper.get('.select-availability').text()).toContain('admin.accounts.proxyPool.availableCount:1')
    expect(wrapper.get('.batch-test-btn').text()).toBe('admin.accounts.proxyPool.testFiltered')
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy.mock.calls.map(([id]) => id)).toEqual([2])

    await wrapper.get('input').setValue('no matches')
    expect(wrapper.get('.select-availability').text()).toContain('admin.accounts.proxyPool.availableCount:0')
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeDefined()
    await wrapper.get('.batch-test-btn').trigger('click')
    expect(testProxy).toHaveBeenCalledTimes(1)
  })

  it('preserves testing all proxies in the default selector even with no search matches', async () => {
    testProxy.mockResolvedValue({ success: true })
    const wrapper = await openSelector()
    await wrapper.get('input').setValue('no matches')
    expect(wrapper.findAll('.test-btn')).toHaveLength(0)
    expect(wrapper.get('.batch-test-btn').attributes('title')).toBe('admin.proxies.batchTest')
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeUndefined()
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy.mock.calls.map(([id]) => id)).toEqual([1, 2])
  })

  it('keeps the initial filtered batch when the query changes during testing', async () => {
    let finish!: (result: object) => void
    testProxy.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = await openSelector({ allowNone: false, showAvailability: true })
    await wrapper.get('input').setValue('Proxy 1')
    await wrapper.get('.batch-test-btn').trigger('click')
    await wrapper.get('input').setValue('Proxy 2')
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeDefined()
    expect(testProxy.mock.calls.map(([id]) => id)).toEqual([1])

    finish({ success: true })
    await flushPromises()
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeUndefined()
    expect(testProxy).toHaveBeenCalledTimes(1)
  })

  it('can search location and IP information returned by a connection test', async () => {
    testProxy.mockResolvedValue({ success: true, city: 'Singapore', ip_address: '203.0.113.9' })
    const wrapper = await openSelector({ allowNone: false })
    await wrapper.findAll('.test-btn')[0].trigger('click')
    await flushPromises()

    for (const query of ['singapore', '203.0.113.9']) {
      await wrapper.get('input').setValue(query)
      expect(wrapper.findAll('.select-option')).toHaveLength(1)
      expect(wrapper.get('.select-option').text()).toContain('Proxy 1')
    }
  })
})

describe('proxy pool selector', () => {
  it('preserves the default no-proxy option', async () => {
    const wrapper = await openSelector()
    expect(wrapper.get('.select-trigger').text()).toBe('admin.accounts.noProxy')
    await wrapper.findAll('.select-option')[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[null]])
  })

  it('uses a placeholder and excludes the no-proxy option when required', async () => {
    const wrapper = await openSelector({
      allowNone: false,
      placeholder: 'Select a proxy',
      ariaLabel: 'New pool proxy',
      showAvailability: true
    })
    expect(wrapper.get('.select-trigger').text()).toBe('Select a proxy')
    expect(wrapper.get('.select-trigger').attributes('aria-label')).toBe('New pool proxy')
    expect(wrapper.findAll('.select-option')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('admin.accounts.noProxy')
    expect(wrapper.get('input').attributes('placeholder')).toBe('admin.accounts.proxyPool.searchPlaceholder')
    await wrapper.findAll('.select-option')[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[1]])
  })

  it('shows an empty state for a pool with no available proxies', async () => {
    const wrapper = await openSelector({ proxies: [], allowNone: false, showAvailability: true })
    expect(wrapper.get('.select-empty').text()).toBe('common.noOptionsFound')
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeDefined()
  })

  it.each([
    '  US East  ',
    'socks5://edge.example.com:1080',
    '1080',
    '203.0.113.1',
    'united states',
    'us',
    'virginia',
    'ashburn'
  ])('searches proxy names, addresses and locations using %s', async (query) => {
    const wrapper = await openSelector({
      allowNone: false,
      proxies: [{
        id: 1, name: 'US East', host: 'edge.example.com', port: 1080, protocol: 'socks5',
        ip_address: '203.0.113.1', country: 'United States', country_code: 'US',
        region: 'Virginia', city: 'Ashburn'
      }, {
        id: 2, name: 'London', host: 'uk.example.com', port: 8080, protocol: 'http',
        country: 'United Kingdom', country_code: 'GB'
      }] as Proxy[]
    })
    await wrapper.get('input').setValue(query)
    expect(wrapper.findAll('.select-option')).toHaveLength(1)
    expect(wrapper.get('.select-option').text()).toContain('US East')
  })
})
