import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import MonitorDisplayOrderDialog from '../MonitorDisplayOrderDialog.vue'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import type { MonitorDisplayOrder } from '@/api/channelMonitor'

const { list, getDisplayOrder, saveDisplayOrder, showError, showSuccess } = vi.hoisted(() => ({
  list: vi.fn(),
  getDisplayOrder: vi.fn(),
  saveDisplayOrder: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({ adminAPI: { channelMonitor: { list, getDisplayOrder, saveDisplayOrder } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const DraggableStub = defineComponent({
  name: 'VueDraggable',
  props: ['modelValue', 'group'],
  emits: ['update:modelValue'],
  template: '<div><slot /></div>',
})

const initialOrder: MonitorDisplayOrder = {
  group_order: ['anthropic', 'openai', 'other'],
  monitor_order: [8, 3, 1, 5],
}

function monitor(id: number, provider = 'openai', enabled = true): ChannelMonitor {
  return { id, provider, enabled, name: `Monitor ${id}`, primary_model: `model-${id}` } as ChannelMonitor
}

const wrappers: VueWrapper[] = []
function mountDialog(show = true) {
  const wrapper = mount(MonitorDisplayOrderDialog, {
    props: { show },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      VueDraggable: DraggableStub,
      Icon: true,
      ProviderIcon: true,
    } },
  })
  wrappers.push(wrapper)
  return wrapper
}

function itemIDs(wrapper: VueWrapper, group: string): string[] {
  return wrapper.get(`[data-order-group="${group}"]`).findAll('[data-order-monitor]').map(row => row.attributes('data-order-monitor'))
}

describe('MonitorDisplayOrderDialog', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    list.mockResolvedValue({ items: [monitor(5, 'grok'), monitor(3), monitor(8, 'anthropic'), monitor(1), monitor(7), monitor(6, 'gemini', false)], total: 6, pages: 1 })
    getDisplayOrder.mockResolvedValue(initialOrder)
    saveDisplayOrder.mockImplementation(async order => order)
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  })

  it('loads all unfiltered pages, preserves saved order and appends new monitors within their platform group', async () => {
    list.mockResolvedValueOnce({ items: [monitor(3), monitor(8, 'anthropic'), monitor(1)], total: 6, pages: 2 })
      .mockResolvedValueOnce({ items: [monitor(7), monitor(5, 'grok'), monitor(6, 'gemini', false)], total: 6, pages: 2 })
    const wrapper = mountDialog()
    expect(wrapper.get('[data-save-order]').attributes('disabled')).toBeDefined()
    await flushPromises()

    expect(list).toHaveBeenCalledTimes(2)
    expect(list.mock.calls.map(call => call[0])).toEqual([{ page: 1, page_size: 100 }, { page: 2, page_size: 100 }])
    expect(wrapper.findAll('[data-order-group]').map(group => group.attributes('data-order-group'))).toEqual(initialOrder.group_order)
    expect(itemIDs(wrapper, 'openai')).toEqual(['3', '1', '7'])
    expect(itemIDs(wrapper, 'other')).toEqual(['5', '6'])
    expect(wrapper.get('[data-order-monitor="6"]').text()).toContain('channelStatus.order.disabled')
    expect(saveDisplayOrder).not.toHaveBeenCalled()
  })

  it('saves group and monitor changes together only after the save button is pressed', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('[data-order-group="openai"] [data-move-group="up"]').trigger('click')
    await wrapper.get('[data-order-monitor="1"] [data-move-monitor="up"]').trigger('click')
    expect(saveDisplayOrder).not.toHaveBeenCalled()
    await wrapper.get('[data-save-order]').trigger('click')
    await flushPromises()

    expect(saveDisplayOrder).toHaveBeenCalledWith({ group_order: ['openai', 'anthropic', 'other'], monitor_order: [1, 3, 7, 8, 5, 6] })
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(showSuccess).toHaveBeenCalledWith('channelStatus.order.saved')
    expect(initialOrder.monitor_order).toEqual([8, 3, 1, 5])
  })

  it('supports draggable updates while disabling cross-group moves', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const draggables = wrapper.findAllComponents(DraggableStub)
    for (const draggable of draggables) {
      expect(draggable.props('group')).toMatchObject({ pull: false, put: false })
    }
    const groupDraggable = draggables[0]
    groupDraggable.vm.$emit('update:modelValue', [...groupDraggable.props('modelValue')].reverse())
    await flushPromises()
    const otherDraggable = wrapper.get('[data-order-group="other"]').findComponent(DraggableStub)
    otherDraggable.vm.$emit('update:modelValue', [...otherDraggable.props('modelValue')].reverse())
    await flushPromises()
    await wrapper.get('[data-save-order]').trigger('click')
    await flushPromises()
    expect(saveDisplayOrder).toHaveBeenCalledWith({ group_order: ['other', 'openai', 'anthropic'], monitor_order: [6, 5, 3, 1, 7, 8] })
  })

  it('discards unsaved changes on cancel and reloads persisted order when reopened', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('[data-order-monitor="1"] [data-move-monitor="up"]').trigger('click')
    const cancel = wrapper.findAll('button').find(button => button.text() === 'common.cancel')!
    await cancel.trigger('click')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(itemIDs(wrapper, 'openai')).toEqual(['3', '1', '7'])
    expect(getDisplayOrder).toHaveBeenCalledTimes(2)
    expect(saveDisplayOrder).not.toHaveBeenCalled()
  })

  it('prevents saving when a later page fails and allows retrying the entire load', async () => {
    list.mockResolvedValueOnce({ items: [monitor(3)], total: 2, pages: 2 })
      .mockRejectedValueOnce(new Error('Page unavailable'))
    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Page unavailable')
    expect(wrapper.get('[data-save-order]').attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('[data-order-group]')).toHaveLength(0)
    await wrapper.get('[data-save-order]').trigger('click')
    expect(saveDisplayOrder).not.toHaveBeenCalled()

    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('[data-save-order]').attributes('disabled')).toBeUndefined()
  })

  it('keeps edits available for retry after a failed save', async () => {
    saveDisplayOrder.mockRejectedValueOnce(new Error('Save unavailable'))
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('[data-order-monitor="1"] [data-move-monitor="up"]').trigger('click')
    await wrapper.get('[data-save-order]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(showError).toHaveBeenCalledWith('Save unavailable')
    expect(itemIDs(wrapper, 'openai')).toEqual(['1', '3', '7'])
    expect(wrapper.get('[data-save-order]').attributes('disabled')).toBeUndefined()
  })

  it('aborts an unfinished load on close and ignores late results', async () => {
    let resolveList!: (value: unknown) => void
    list.mockImplementationOnce(() => new Promise(resolve => { resolveList = resolve }))
    const wrapper = mountDialog()
    const signal = list.mock.calls[0][1].signal as AbortSignal
    await wrapper.setProps({ show: false })
    expect(signal.aborted).toBe(true)
    resolveList({ items: [monitor(3)], total: 1, pages: 1 })
    await flushPromises()
    expect(saveDisplayOrder).not.toHaveBeenCalled()
    expect(wrapper.find('[data-order-group]').exists()).toBe(false)
  })
})
