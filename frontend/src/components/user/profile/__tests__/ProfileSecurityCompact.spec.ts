import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ProfilePasswordForm from '../ProfilePasswordForm.vue'
import ProfileTotpCard from '../ProfileTotpCard.vue'
import ProfilePasskeyCard from '../ProfilePasskeyCard.vue'

const api = vi.hoisted(() => ({
  changePassword: vi.fn(),
  getStatus: vi.fn(),
  isSupported: vi.fn(),
  list: vi.fn(),
  register: vi.fn(),
  rename: vi.fn(),
  remove: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api', () => ({
  userAPI: { changePassword: api.changePassword },
  totpAPI: { getStatus: api.getStatus },
  passkeyAPI: { isSupported: api.isSupported, list: api.list, register: api.register, rename: api.rename, remove: api.remove }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.resetAllMocks()
  api.changePassword.mockResolvedValue(undefined)
  api.getStatus.mockResolvedValue({ feature_enabled: false, enabled: false })
  api.isSupported.mockReturnValue(true)
  api.list.mockResolvedValue([{ id: 7, name: 'Laptop', created_at: '2026-10-28T00:00:00Z', backup: false }])
})

const global = { stubs: { teleport: true, Icon: true, TotpSetupModal: true, TotpDisableDialog: true } }

describe('compact profile security controls', () => {
  it('keeps the password draft when closed and clears it only after a successful change', async () => {
    const wrapper = mount(ProfilePasswordForm, { props: { compact: true }, global })
    expect(wrapper.find('input').exists()).toBe(false)
    await wrapper.get('[data-testid="profile-password-manage"]').trigger('click')
    await wrapper.get('#old_password').setValue('old-password')
    await wrapper.get('#new_password').setValue('new-password')
    await wrapper.get('#confirm_password').setValue('new-password')
    await wrapper.get('[aria-label="Close modal"]').trigger('click')
    expect(wrapper.find('input').exists()).toBe(false)
    await wrapper.get('[data-testid="profile-password-manage"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('#new_password').element.value).toBe('new-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.changePassword).toHaveBeenCalledWith('old-password', 'new-password')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    await wrapper.get('[data-testid="profile-password-manage"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('#new_password').element.value).toBe('')
  })

  it('shows disabled TOTP status without an enable action', async () => {
    const wrapper = mount(ProfileTotpCard, { props: { compact: true }, global })
    await flushPromises()
    expect(wrapper.text()).toContain('profile.totp.featureDisabled')
    expect(wrapper.find('[data-testid="profile-totp-manage"]').exists()).toBe(false)
  })

  it.each([false, true])('opens the existing TOTP operation for enabled=%s', async enabled => {
    api.getStatus.mockResolvedValue({ feature_enabled: true, enabled })
    const wrapper = mount(ProfileTotpCard, { props: { compact: true }, global })
    await flushPromises()
    await wrapper.get('[data-testid="profile-totp-manage"]').trigger('click')
    expect(wrapper.find(enabled ? 'totp-disable-dialog-stub' : 'totp-setup-modal-stub').exists()).toBe(true)
  })

  it('preserves a Passkey draft and closes only the nested deletion confirmation', async () => {
    const wrapper = mount(ProfilePasskeyCard, { props: { compact: true, enabled: true }, global })
    await flushPromises()
    expect(wrapper.find('input').exists()).toBe(false)
    await wrapper.get('[data-testid="profile-passkey-manage"]').trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'profile.passkey.add')!.trigger('click')
    await wrapper.get('#passkey-name').setValue('Draft laptop')
    await wrapper.get('#passkey-add-password').setValue('current-password')
    await wrapper.get('[aria-label="Close modal"]').trigger('click')
    await wrapper.get('[data-testid="profile-passkey-manage"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('#passkey-name').element.value).toBe('Draft laptop')
    expect(wrapper.get<HTMLInputElement>('#passkey-add-password').element.value).toBe('current-password')
    await wrapper.findAll('button').find(button => button.text() === 'common.cancel')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'common.delete')!.trigger('click')
    const dialogs = wrapper.findAll('[role="dialog"]')
    expect(dialogs).toHaveLength(2)
    expect(Number((dialogs[1].element as HTMLElement).style.zIndex)).toBeGreaterThan(50)
    await dialogs[1].get('[aria-label="Close modal"]').trigger('click')
    expect(wrapper.findAll('[role="dialog"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Laptop')
    expect(api.remove).not.toHaveBeenCalled()
  })

  it('does not load Passkeys or offer management when the feature is disabled', async () => {
    const wrapper = mount(ProfilePasskeyCard, { props: { compact: true, enabled: false }, global })
    await flushPromises()
    expect(wrapper.text()).toContain('profile.passkey.featureDisabled')
    expect(wrapper.find('[data-testid="profile-passkey-manage"]').exists()).toBe(false)
    expect(api.list).not.toHaveBeenCalled()
  })
})
