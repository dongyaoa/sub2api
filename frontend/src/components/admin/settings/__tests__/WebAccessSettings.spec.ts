import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import WebAccessSettings from '../WebAccessSettings.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('web access settings', () => {
  it('edits a multiline IP allowlist without losing an unfinished trailing line', async () => {
    const wrapper = mount(WebAccessSettings, {
      props: { enabled: false, message: '', ipAllowlist: ['203.0.113.10'] },
      global: { stubs: { BaseDialog: true } },
    })
    await wrapper.get('#web-access-allowlist').setValue('203.0.113.10\n2001:db8::/48\n')
    expect(wrapper.emitted('update:ipAllowlist')?.[0]).toEqual([['203.0.113.10', '2001:db8::/48', '']])
    expect(wrapper.get('#web-access-message').attributes('maxlength')).toBe('500')
  })

  it('previews escaped custom text without changing access settings', async () => {
    document.title = 'Settings - Test Site'
    const wrapper = mount(WebAccessSettings, {
      props: { enabled: false, message: '<strong>测试提示</strong>', ipAllowlist: [] },
      global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show" data-testid="preview-dialog"><slot /></div>' } } },
    })
    await wrapper.get('[data-testid="web-access-preview"]').trigger('click')
    const preview = wrapper.get('[data-testid="preview-dialog"]')
    expect(preview.text()).toContain('本站僅面向海外用戶提供服務')
    expect(preview.text()).toContain('Exclusively for overseas users')
    expect(preview.text()).toContain('<strong>测试提示</strong>')
    expect(preview.find('strong').exists()).toBe(false)
    expect(preview.findAll('button')).toHaveLength(0)
    expect(document.title).toBe('Settings - Test Site')
    expect(wrapper.emitted('update:enabled')).toBeUndefined()
    expect(wrapper.emitted('update:message')).toBeUndefined()
    wrapper.unmount()
  })
})
