import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import RechargePromotionSettings from '../RechargePromotionSettings.vue'
import { defaultRechargePromotion, fromBeijingInput, toBeijingInput } from '../rechargePromotionSettings'
import type { RechargePromotion } from '@/types/payment'

const mocks = vi.hoisted(() => ({ getConfig: vi.fn(), updateConfig: vi.fn() }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: mocks }))

enableAutoUnmount(afterEach)
const campaign: RechargePromotion = {
  enabled: true, title: '充值加赠', subtitle: '充值达到门槛即可参与', currency: 'CNY', max_bonus: 0,
  starts_at: '2026-09-30T16:00:00Z', ends_at: '2026-10-07T16:00:00Z', active: false,
  tiers: [{ min_amount: 50, bonus_percent: 5 }, { min_amount: 100, bonus_percent: 10 }],
}

function render() {
  return mount(RechargePromotionSettings, {
    global: { plugins: [createI18n({ legacy: false, locale: 'zh', fallbackLocale: 'en' })] },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.getConfig.mockResolvedValue({ data: { recharge_promotion: campaign } })
  mocks.updateConfig.mockResolvedValue({ data: {} })
})

describe('recharge campaign settings', () => {
  it('loads a disabled example without publishing it when no campaign exists', async () => {
    mocks.getConfig.mockResolvedValue({ data: { recharge_promotion: null } })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-testid="promotion-enabled"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.findAll('[data-testid="promotion-tier"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('这是本地示例')
    expect(mocks.updateConfig).not.toHaveBeenCalled()
  })

  it('shows Beijing dates and saves only campaign fields, with sorted tiers and UTC instants', async () => {
    const wrapper = render()
    await flushPromises()
    expect((wrapper.get('[data-testid="promotion-starts"]').element as HTMLInputElement).value).toBe('2026-10-01T00:00')
    await wrapper.get('[data-testid="promotion-starts"]').setValue('2026-10-01T09:30')
    await wrapper.get('[data-testid="promotion-threshold-0"]').setValue('300')
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    await flushPromises()
    expect(mocks.updateConfig).toHaveBeenCalledWith({ recharge_promotion: {
      enabled: true, title: campaign.title, subtitle: campaign.subtitle, currency: 'CNY', max_bonus: 0,
      starts_at: '2026-10-01T01:30:00.000Z', ends_at: '2026-10-07T16:00:00.000Z',
      tiers: [{ min_amount: 100, bonus_percent: 10 }, { min_amount: 300, bonus_percent: 5 }],
    } })
    expect(wrapper.get('[role="status"]').text()).toBe('活动已保存')
    expect(wrapper.emitted('saved')).toEqual([[]])
    expect(wrapper.find('[data-testid="promotion-unsaved"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="promotion-save"]').attributes('disabled')).toBeDefined()
  })

  it.each([
    ['promotion-threshold-1', '50', '充值门槛不能重复'],
    ['promotion-threshold-0', '0.001', '最多保留两位小数'],
    ['promotion-percent-0', '101', '不超过 100%'],
    ['promotion-cap', '-1', '单笔赠送上限'],
    ['promotion-ends', '2026-09-30T00:00', '结束时间必须晚于'],
  ])('prevents invalid submission for %s', async (testId, value, feedback) => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get(`[data-testid="${testId}"]`).setValue(value)
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    expect(mocks.updateConfig).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain(feedback)
  })

  it('preserves edits on a failed save and allows retry', async () => {
    mocks.updateConfig.mockRejectedValueOnce({ message: '保存失败，请重试' })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="promotion-title"]').setValue('新活动')
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('保存失败，请重试')
    expect((wrapper.get('[data-testid="promotion-title"]').element as HTMLInputElement).value).toBe('新活动')
    expect(wrapper.find('[data-testid="promotion-unsaved"]').exists()).toBe(true)
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.get('[data-testid="promotion-save"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    await flushPromises()
    expect(mocks.updateConfig).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('shows load failure without exposing a writable example and retries explicitly', async () => {
    mocks.getConfig.mockRejectedValueOnce(new Error('无法加载配置'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('无法加载配置')
    expect(wrapper.find('[data-testid="promotion-save"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="promotion-save"]').exists()).toBe(true)
  })

  it('saves an explicit disable without changing rules', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[data-testid="promotion-save"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="promotion-enabled"]').trigger('click')
    expect(wrapper.find('[data-testid="promotion-unsaved"]').exists()).toBe(true)
    expect(mocks.updateConfig).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    await flushPromises()
    expect(mocks.updateConfig.mock.calls[0]?.[0].recharge_promotion.enabled).toBe(false)
    expect(mocks.updateConfig.mock.calls[0]?.[0].recharge_promotion.tiers).toEqual(campaign.tiers)
    expect(wrapper.get('[data-testid="promotion-enabled"]').attributes('aria-checked')).toBe('false')
  })

  it('keeps input edits local until the save button is explicitly clicked', async () => {
    const wrapper = render()
    await flushPromises()
    const input = wrapper.get('[data-testid="promotion-title"]')
    await input.setValue('新的充值活动')
    const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    input.element.dispatchEvent(event)
    await flushPromises()
    expect(mocks.updateConfig).not.toHaveBeenCalled()
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('disables editing while saving and prevents duplicate submissions', async () => {
    let resolveSave: (() => void) | undefined
    mocks.updateConfig.mockImplementation(() => new Promise<void>(resolve => { resolveSave = resolve }))
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="promotion-title"]').setValue('新的充值活动')
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="promotion-enabled"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="promotion-save"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    expect(mocks.updateConfig).toHaveBeenCalledTimes(1)
    resolveSave?.()
    await flushPromises()
    expect(wrapper.get('fieldset').attributes('disabled')).toBeUndefined()
    expect(wrapper.emitted('saved')).toEqual([[]])
  })

  it('exposes reload to refresh persisted settings and clear unsaved state', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="promotion-title"]').setValue('尚未保存的修改')
    mocks.getConfig.mockResolvedValueOnce({ data: { recharge_promotion: { ...campaign, title: '后台新配置' } } })
    await wrapper.vm.reload()
    await flushPromises()
    expect((wrapper.get('[data-testid="promotion-title"]').element as HTMLInputElement).value).toBe('后台新配置')
    expect(wrapper.find('[data-testid="promotion-unsaved"]').exists()).toBe(false)
    expect(mocks.updateConfig).not.toHaveBeenCalled()
  })

  it('preserves an existing currency outside the default choices', async () => {
    mocks.getConfig.mockResolvedValue({ data: { recharge_promotion: { ...campaign, currency: 'EUR' } } })
    const wrapper = render()
    await flushPromises()
    expect((wrapper.get('[data-testid="promotion-currency"]').element as HTMLSelectElement).value).toBe('EUR')
  })

  it('previews base credit and the highest eligible bonus using the saved site multiplier', async () => {
    mocks.getConfig.mockResolvedValue({ data: { recharge_promotion: campaign, balance_recharge_multiplier: 0.15 } })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="promotion-preview-amount"]').setValue('100')
    const amounts = Object.fromEntries(wrapper.findAll('.preview-calculator dl > div').map(row => [row.get('dt').text(), row.get('dd').text()]))
    expect(amounts).toEqual({ 基础到账: '$15.00', 活动加赠: '+$1.50', 合计到账: '$16.50' })
    expect(mocks.updateConfig).not.toHaveBeenCalled()
    expect(wrapper.emitted('saved')).toBeUndefined()
  })

  it('keeps preview inputs out of the saved payload and preserves clean, dirty and saved states', async () => {
    const wrapper = render()
    await flushPromises()
    const amountInput = wrapper.get('[data-testid="promotion-preview-amount"]')
    await amountInput.setValue('333')
    expect(wrapper.find('[data-testid="promotion-unsaved"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="promotion-save"]').attributes('disabled')).toBeDefined()
    expect(mocks.updateConfig).not.toHaveBeenCalled()
    expect(wrapper.emitted('saved')).toBeUndefined()

    await wrapper.get('[data-testid="promotion-title"]').setValue('已修改的活动')
    await amountInput.setValue('200')
    expect(wrapper.find('[data-testid="promotion-unsaved"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="promotion-save"]').attributes('disabled')).toBeUndefined()
    expect(mocks.updateConfig).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="promotion-save"]').trigger('click')
    await flushPromises()
    const payload = mocks.updateConfig.mock.calls[0]?.[0]
    expect(Object.keys(payload)).toEqual(['recharge_promotion'])
    expect(Object.keys(payload.recharge_promotion).sort()).toEqual(['currency', 'enabled', 'ends_at', 'max_bonus', 'starts_at', 'subtitle', 'tiers', 'title'])
    expect(payload.recharge_promotion.title).toBe('已修改的活动')
    expect(payload.recharge_promotion.tiers).toEqual(campaign.tiers)

    await amountInput.setValue('500')
    expect(wrapper.get('[role="status"]').text()).toBe('活动已保存')
    expect(wrapper.find('[data-testid="promotion-unsaved"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="promotion-save"]').attributes('disabled')).toBeDefined()
    expect(mocks.updateConfig).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('saved')).toEqual([[]])
  })
})

describe('Beijing input conversion', () => {
  it('creates a generic disabled campaign from tomorrow for seven days in Beijing time', () => {
    const example = defaultRechargePromotion(new Date('2026-12-31T23:00:00Z'))
    expect(example.title).toBe('充值加赠')
    expect(example.subtitle).toBe('')
    expect(example.enabled).toBe(false)
    expect(toBeijingInput(example.starts_at)).toBe('2027-01-02T00:00')
    expect(toBeijingInput(example.ends_at)).toBe('2027-01-09T00:00')
  })

  it('round trips the same instant regardless of the browser timezone', () => {
    expect(toBeijingInput('2026-10-01T00:00:00+08:00')).toBe('2026-10-01T00:00')
    expect(fromBeijingInput('2026-10-01T00:00')).toBe('2026-09-30T16:00:00.000Z')
    expect(fromBeijingInput('2026-02-30T09:00')).toBe('')
    expect(fromBeijingInput('')).toBe('')
  })
})
