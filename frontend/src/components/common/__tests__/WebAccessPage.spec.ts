import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { WebAccessPhase } from '@/services/webAccess'
import WebAccessPage from '../WebAccessPage.vue'

describe('WebAccessPage', () => {
  it('renders the bilingual message as text without manual actions', () => {
    const message = '<a href="/login">继续访问</a>'
    const wrapper = mount(WebAccessPage, { props: { phase: 'blocked', message } })
    expect(wrapper.text()).toContain('本站僅面向海外用戶提供服務')
    expect(wrapper.text()).toContain('Exclusively for overseas users')
    expect(wrapper.text()).toContain(message)
    expect(wrapper.find('a').exists()).toBe(false)
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/重新[检檢]查/)
    expect(wrapper.text()).not.toContain('Try again')
  })

  it('keeps normal page content out of checking and network-error states', async () => {
    const wrapper = mount(WebAccessPage, { props: { phase: 'checking', checking: true } })
    expect(wrapper.get('[data-testid="web-access-loading"]').text()).toContain('正在檢查訪問狀態')
    expect(wrapper.get('[data-testid="web-access-loading"]').text()).toContain('Checking availability')
    expect(wrapper.find('[data-testid="web-access-notice"]').exists()).toBe(false)
    expect(wrapper.find('button').exists()).toBe(false)
    await wrapper.setProps({ phase: 'error', checking: false })
    expect(wrapper.text()).toContain('暫時無法檢查訪問狀態')
    expect(wrapper.text()).toContain('Unable to check access')
    expect(wrapper.find('button').exists()).toBe(false)
  })

  it.each([undefined, 'checking', 'allowed'] as Array<WebAccessPhase | undefined>)('never creates restricted copy or layout before a denied decision (phase=%s)', phase => {
    const wrapper = mount(WebAccessPage, { props: { phase, message: '受限文案不应该闪现' } })
    expect(wrapper.find('[data-testid="web-access-loading"]').exists()).toBe(true)
    expect(wrapper.find('h1').exists()).toBe(false)
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.find('[data-testid="web-access-notice"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('当前地区')
    expect(wrapper.text()).not.toContain('本站僅面向海外用戶提供服務')
    expect(wrapper.text()).not.toContain('Exclusively for overseas users')
    expect(wrapper.text()).not.toContain('受限文案')
  })

  it('removes all denied content as soon as access is permitted', async () => {
    const wrapper = mount(WebAccessPage, { props: { phase: 'blocked', message: '访问受限文案' } })
    expect(wrapper.text()).toContain('访问受限文案')
    await wrapper.setProps({ phase: 'allowed' })
    expect(wrapper.find('[data-testid="web-access-loading"]').exists()).toBe(true)
    expect(wrapper.find('h1').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('访问受限')
    expect(wrapper.text()).not.toContain('Exclusively for overseas users')
  })

})
