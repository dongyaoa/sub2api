import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import AccountProxyPoolEditor from '../AccountProxyPoolEditor.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import type { AccountProxyPoolEntry, Proxy } from '@/types'

vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy: vi.fn() } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, params?: Record<string, number>) =>
    params ? `${key}:${Object.values(params).join(',')}` : key
}) }))

enableAutoUnmount(afterEach)
beforeEach(() => { vi.clearAllMocks() })

function makeProxy(id: number, overrides: Partial<Proxy> = {}): Proxy {
  return {
    id,
    name: `Proxy ${id}`,
    protocol: 'http',
    host: `192.0.2.${id}`,
    port: 8080,
    username: null,
    status: 'active',
    expires_at: null,
    fallback_mode: 'none',
    expiry_warn_days: 7,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides
  }
}

function mountEditor(modelValue: AccountProxyPoolEntry[] = [], proxies = [1, 2, 3].map(id => makeProxy(id))) {
  return mount(AccountProxyPoolEditor, {
    props: { modelValue, proxies },
    global: { stubs: { Icon: true } }
  })
}

type EditorWrapper = ReturnType<typeof mountEditor>

function latestEntries(wrapper: EditorWrapper): AccountProxyPoolEntry[] {
  const updates = wrapper.emitted('update:modelValue')!
  return updates[updates.length - 1][0] as AccountProxyPoolEntry[]
}

function availableIds(wrapper: EditorWrapper): number[] {
  return wrapper.findComponent(ProxySelector).props('proxies').map((proxy: Proxy) => proxy.id)
}

async function chooseProxy(wrapper: EditorWrapper, id: number) {
  await wrapper.get('.select-trigger').trigger('click')
  const option = wrapper.findAll('.select-option').find(candidate =>
    candidate.find('.font-medium').text() === `Proxy ${id}`
  )
  expect(option, `Proxy ${id} should be offered in the selector`).toBeDefined()
  await option!.trigger('click')
}

describe('account proxy pool editor', () => {
  it('adds the selected proxy at default concurrency 20, resets the choice and prevents duplicate additions', async () => {
    const wrapper = mountEditor()
    expect((wrapper.get('[data-testid="proxy-pool-new-concurrency"]').element as HTMLInputElement).value).toBe('20')
    expect(wrapper.get('[data-testid="proxy-pool-add"]').attributes('disabled')).toBeDefined()

    await chooseProxy(wrapper, 2)
    await wrapper.get('[data-testid="proxy-pool-add"]').trigger('click')

    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 2, concurrency: 20 }])
    expect(wrapper.get('[data-testid="proxy-pool-total"]').text()).toBe('20')
    expect(wrapper.get('.select-value').text()).toBe('admin.accounts.proxyPool.selectProxy')
    expect(availableIds(wrapper)).toEqual([1, 3])
    expect(wrapper.get('[data-testid="proxy-pool-add"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="proxy-pool-add"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toHaveLength(1)
  })

  it('adds only unused active unexpired proxies with the configured integer concurrency and keeps existing capacities', async () => {
    const existing = [{ proxy_id: 2, concurrency: 30 }]
    const proxies = [
      makeProxy(1, { latency_status: 'failed' }),
      makeProxy(2),
      makeProxy(3, { expires_at: '2999-01-01T00:00:00Z' }),
      makeProxy(4, { status: 'inactive' }),
      makeProxy(5, { status: 'expired' }),
      makeProxy(6, { expires_at: '2000-01-01T00:00:00Z' }),
      makeProxy(7, { expires_at: 'invalid' })
    ]
    const wrapper = mountEditor(existing, proxies)

    await wrapper.get('[data-testid="proxy-pool-new-concurrency"]').setValue('31.9')
    expect(availableIds(wrapper)).toEqual([1, 3])
    await wrapper.get('[data-testid="proxy-pool-add-all"]').trigger('click')

    expect(latestEntries(wrapper)).toEqual([
      { proxy_id: 2, concurrency: 30 },
      { proxy_id: 1, concurrency: 31 },
      { proxy_id: 3, concurrency: 31 }
    ])
    expect(existing).toEqual([{ proxy_id: 2, concurrency: 30 }])
    expect(wrapper.get('[data-testid="proxy-pool-total"]').text()).toBe('92')
    expect(availableIds(wrapper)).toEqual([])
    expect(wrapper.get('[data-testid="proxy-pool-add-all"]').attributes('disabled')).toBeDefined()
  })

  it('makes a removed proxy available again without changing the other proxy capacity', async () => {
    const wrapper = mountEditor([{ proxy_id: 1, concurrency: 30 }, { proxy_id: 2, concurrency: 12 }])
    expect(availableIds(wrapper)).toEqual([3])

    await wrapper.findAll('[data-testid="proxy-pool-entry"]')[0].get('button').trigger('click')

    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 2, concurrency: 12 }])
    expect(availableIds(wrapper)).toEqual([1, 3])
    expect(wrapper.get('[data-testid="proxy-pool-total"]').text()).toBe('12')
  })

  it('excludes proxies selected in other rows from replacements and releases the old selection', async () => {
    const wrapper = mountEditor(
      [{ proxy_id: 1, concurrency: 30 }, { proxy_id: 2, concurrency: 12 }],
      [makeProxy(1), makeProxy(2), makeProxy(3), makeProxy(4, { status: 'inactive' })]
    )
    const rows = wrapper.findAll('[data-testid="proxy-pool-entry"]')
    expect(rows[0].findAll('option').map(option => option.attributes('value'))).toEqual(['1', '3'])
    expect(rows[1].findAll('option').map(option => option.attributes('value'))).toEqual(['2', '3'])

    await rows[1].get('select').setValue('3')

    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 30 }, { proxy_id: 3, concurrency: 12 }])
    expect(availableIds(wrapper)).toEqual([2])
  })

  it('keeps existing unavailable and missing IDs when another row is edited', async () => {
    const inactive = makeProxy(8, { status: 'inactive' })
    const existing = [
      { proxy_id: 99, concurrency: 9 },
      { proxy_id: 8, concurrency: 13, proxy: inactive },
      { proxy_id: 2, concurrency: 17 }
    ]
    const wrapper = mountEditor(existing, [makeProxy(1), makeProxy(2, { status: 'expired' })])
    const rows = wrapper.findAll('[data-testid="proxy-pool-entry"]')
    expect((rows[0].get('select').element as HTMLSelectElement).value).toBe('99')
    expect(rows[0].get('option[value="99"]').attributes('disabled')).toBeDefined()
    expect((rows[1].get('select').element as HTMLSelectElement).value).toBe('8')
    expect(rows[1].get('option[value="8"]').text()).toContain('Proxy 8')
    expect((rows[2].get('select').element as HTMLSelectElement).value).toBe('2')
    expect(availableIds(wrapper)).toEqual([1])

    await rows[0].get('input').setValue('11')

    expect(latestEntries(wrapper)).toEqual([
      { proxy_id: 99, concurrency: 11 },
      { proxy_id: 8, concurrency: 13, proxy: inactive },
      { proxy_id: 2, concurrency: 17 }
    ])
    expect(existing[0].concurrency).toBe(9)
  })

  it('normalizes both new and existing row concurrency to positive integers', async () => {
    const wrapper = mountEditor()
    await wrapper.get('[data-testid="proxy-pool-new-concurrency"]').setValue('0')
    await chooseProxy(wrapper, 1)
    await wrapper.get('[data-testid="proxy-pool-add"]').trigger('click')
    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 1 }])

    const capacity = wrapper.get('[data-testid="proxy-pool-entry"] input')
    await capacity.setValue('7.9')
    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 7 }])
    await capacity.setValue('-3')
    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 1 }])
    await capacity.setValue('100001')
    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 100000 }])
  })

  it('rejects a batch of more than 256 proxies while allowing a valid selected addition', async () => {
    const wrapper = mountEditor([], Array.from({ length: 257 }, (_, index) => makeProxy(index + 1)))
    expect(wrapper.get('[data-testid="proxy-pool-add-all"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[role="status"]').text()).toContain('admin.accounts.proxyPool.limitHint')
    await wrapper.get('[data-testid="proxy-pool-add-all"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()

    await chooseProxy(wrapper, 1)
    await wrapper.get('[data-testid="proxy-pool-add"]').trigger('click')
    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 20 }])
  })

  it('limits an edited row to remaining account capacity without changing another row', async () => {
    const existing = [{ proxy_id: 1, concurrency: 80000 }, { proxy_id: 2, concurrency: 10 }]
    const wrapper = mountEditor(existing)
    const rows = wrapper.findAll('[data-testid="proxy-pool-entry"]')
    expect(rows[1].get('input').attributes('max')).toBe('20000')

    await rows[1].get('input').setValue('50000')

    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 80000 }, { proxy_id: 2, concurrency: 20000 }])
    expect(wrapper.get('[data-testid="proxy-pool-total"]').text()).toBe('100000')
    expect(existing).toEqual([{ proxy_id: 1, concurrency: 80000 }, { proxy_id: 2, concurrency: 10 }])

    await rows[0].get('input').setValue('70000')
    expect(rows[1].get('input').attributes('max')).toBe('30000')
    await rows[1].get('input').setValue('30000')
    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 70000 }, { proxy_id: 2, concurrency: 30000 }])
    expect(wrapper.get('[data-testid="proxy-pool-total"]').text()).toBe('100000')
  })

  it('rejects a batch above total concurrency 100000 while allowing a selected addition at the limit', async () => {
    const wrapper = mountEditor([{ proxy_id: 1, concurrency: 99980 }])
    expect(wrapper.get('[data-testid="proxy-pool-add-all"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="proxy-pool-add-all"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()

    await chooseProxy(wrapper, 2)
    await wrapper.get('[data-testid="proxy-pool-add"]').trigger('click')
    expect(latestEntries(wrapper)).toEqual([{ proxy_id: 1, concurrency: 99980 }, { proxy_id: 2, concurrency: 20 }])
    expect(wrapper.get('[data-testid="proxy-pool-total"]').text()).toBe('100000')

    await chooseProxy(wrapper, 3)
    expect(wrapper.get('[data-testid="proxy-pool-add"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="proxy-pool-add"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toHaveLength(1)
  })
})
