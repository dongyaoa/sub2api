import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { IntelligenceChannelPrompt, IntelligenceLocalChannel } from '@/api/admin/intelligenceMonitor'
import IntelligencePromptSettings from './IntelligencePromptSettings.vue'

const mocks = vi.hoisted(() => ({ channels: vi.fn() }))
vi.mock('@/api/admin/intelligenceMonitor', () => ({ intelligenceMonitorAPI: { localChannels: mocks.channels }, PELICAN_PROMPT: 'Default pelican HTML' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const channel = (account_id: number, name: string): IntelligenceLocalChannel => ({ account_id, name, platform: 'openai', type: 'apikey', status: 'active' })
let wrapper: VueWrapper | undefined
function render(props: { groupId?: number | null; customPrompt?: string; channelPrompts?: IntelligenceChannelPrompt[]; allowChannels?: boolean } = {}) {
  wrapper = mount(IntelligencePromptSettings, {
    props: {
      customPrompt: '', channelPrompts: [], ...props,
      'onUpdate:customPrompt': (value: string) => { void wrapper!.setProps({ customPrompt: value }) },
      'onUpdate:channelPrompts': (value: IntelligenceChannelPrompt[]) => { void wrapper!.setProps({ channelPrompts: value }) },
    },
    global: { stubs: { Icon: true } },
  })
  return wrapper
}
function validate(view: VueWrapper) { return (view.vm as unknown as { validate: () => boolean }).validate() }
beforeEach(() => { vi.resetAllMocks(); mocks.channels.mockResolvedValue({ items: [channel(11, 'North'), channel(12, 'South')] }) })
afterEach(() => { wrapper?.unmount(); wrapper = undefined })

describe('optional local monitoring prompts', () => {
  it('offers a single prompt outside local monitoring without fetching channel settings', async () => {
    const view = render({ allowChannels: false, groupId: 5, customPrompt: 'Custom pelican', channelPrompts: [{ account_id: 11, prompt: 'Old channel override' }] })
    await flushPromises()
    expect(view.find('[data-testid="toggle-channel-prompts"]').exists()).toBe(false)
    expect(mocks.channels).not.toHaveBeenCalled()
    expect(validate(view)).toBe(true)
    await view.get('#intelligence-custom-prompt').setValue('')
    expect(view.emitted('update:customPrompt')).toEqual([['']])
  })
  it('keeps the default optional and only loads channels after expansion with a group selected', async () => {
    const view = render()
    expect(view.get('#intelligence-custom-prompt').attributes('placeholder')).toBe('Default pelican HTML')
    expect(validate(view)).toBe(true)
    expect(mocks.channels).not.toHaveBeenCalled()
    await view.get('[data-testid="toggle-channel-prompts"]').trigger('click')
    expect(view.text()).toContain('intelligenceMonitor.promptSettings.selectGroup')
    expect(mocks.channels).not.toHaveBeenCalled()
    await view.setProps({ groupId: 5 }); await flushPromises()
    expect(mocks.channels).toHaveBeenCalledWith(5, expect.any(AbortSignal))
    expect(view.findAll('[data-channel-id]')).toHaveLength(2)
    expect(view.emitted('update:channelPrompts')).toBeUndefined()
  })

  it('edits independent channel prompts and clears back to the current plan prompt', async () => {
    const view = render({ groupId: 5 })
    await view.get('#intelligence-custom-prompt').setValue('Draw a pelican in HTML')
    await view.get('[data-testid="toggle-channel-prompts"]').trigger('click'); await flushPromises()
    expect(view.get('#intelligence-channel-prompt-11').attributes('placeholder')).toBe('Draw a pelican in HTML')
    await view.get('#intelligence-channel-prompt-11').setValue('North animation')
    await view.get('#intelligence-channel-prompt-12').setValue('South animation')
    expect(view.props('channelPrompts')).toEqual([{ account_id: 11, prompt: 'North animation' }, { account_id: 12, prompt: 'South animation' }])
    await view.get('[data-clear-channel="11"]').trigger('click')
    expect(view.props('channelPrompts')).toEqual([{ account_id: 12, prompt: 'South animation' }])
    expect((view.get('#intelligence-channel-prompt-11').element as HTMLTextAreaElement).value).toBe('')
  })

  it('ignores a late response for the previous group and cancels its request', async () => {
    let resolveFirst!: (value: { items: IntelligenceLocalChannel[] }) => void
    mocks.channels.mockReturnValueOnce(new Promise(resolve => { resolveFirst = resolve }))
    const view = render({ groupId: 5 })
    await view.get('[data-testid="toggle-channel-prompts"]').trigger('click')
    const firstSignal = mocks.channels.mock.calls[0]![1] as AbortSignal
    await view.setProps({ groupId: 6 }); await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    resolveFirst({ items: [channel(99, 'Previous group')] }); await flushPromises()
    expect(view.find('[data-channel-id="99"]').exists()).toBe(false)
    expect(view.get('[data-channel-id="11"]').text()).toContain('North')
  })

  it('allows ordinary saving after a load failure and supports retry', async () => {
    mocks.channels.mockRejectedValueOnce(new Error('Unavailable'))
    const view = render({ groupId: 5, customPrompt: 'Draw HTML' })
    await view.get('[data-testid="toggle-channel-prompts"]').trigger('click'); await flushPromises()
    expect(view.text()).toContain('intelligenceMonitor.promptSettings.loadFailed')
    expect(validate(view)).toBe(true)
    await view.get('[data-testid="refresh-prompt-channels"]').trigger('click'); await flushPromises()
    expect(view.text()).not.toContain('intelligenceMonitor.promptSettings.loadFailed')
    expect(view.findAll('[data-channel-id]')).toHaveLength(2)
  })

  it('preserves saved prompts for unavailable channels until explicitly cleared', async () => {
    const view = render({ groupId: 5, channelPrompts: [{ account_id: 99, prompt: 'Saved prompt' }] })
    await flushPromises()
    expect(view.get('[data-channel-id="99"]').text()).toContain('intelligenceMonitor.promptSettings.unavailable')
    expect((view.get('#intelligence-channel-prompt-99').element as HTMLTextAreaElement).value).toBe('Saved prompt')
    await view.get('[data-clear-channel="99"]').trigger('click')
    expect(view.props('channelPrompts')).toEqual([])
    expect(view.find('[data-channel-id="99"]').exists()).toBe(false)
  })

  it('validates per-prompt, total size and channel limits while counting Unicode characters', async () => {
    const view = render({ customPrompt: '🦤'.repeat(8000) })
    expect(validate(view)).toBe(true)
    await view.setProps({ customPrompt: 'a'.repeat(8001) })
    expect(validate(view)).toBe(false)
    await view.vm.$nextTick()
    expect(view.text()).toContain('intelligenceMonitor.promptSettings.tooLong')
    await view.setProps({ customPrompt: 'a', channelPrompts: Array.from({ length: 8 }, (_, index) => ({ account_id: index + 1, prompt: 'a'.repeat(8000) })) })
    expect(validate(view)).toBe(false)
    await view.setProps({ customPrompt: '', channelPrompts: Array.from({ length: 201 }, (_, index) => ({ account_id: index + 1, prompt: 'a' })) })
    expect(validate(view)).toBe(false)
    await view.setProps({ channelPrompts: [] })
    expect(validate(view)).toBe(true)
  })
})
