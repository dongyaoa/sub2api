import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { useAdminSettingsStore, useAppStore, useAuthStore } from '@/stores'
import type { PublicSettings, User } from '@/types'
import AppSidebar from '../AppSidebar.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key, locale: ref('zh') }),
}))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: ref(false), refreshBatchImageAccess: vi.fn() }),
}))

describe('Model Square sidebar visibility', () => {
  it.each(['user', 'admin'] as const)('is independent of Available Channels for %s', async (role) => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.cachedPublicSettings = { available_channels_enabled: false } as PublicSettings
    useAuthStore().user = { id: 1, role } as User
    vi.spyOn(useAdminSettingsStore(), 'fetch').mockResolvedValue(undefined)
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
    })
    await router.push('/model-square')
    const wrapper = mount(AppSidebar, {
      global: { plugins: [pinia, router], stubs: { VersionBadge: true, Icon: true } },
    })
    try {
      expect(wrapper.find('a[href="/model-square"]').text()).toContain('模型广场')
      expect(wrapper.find('a[href="/available-channels"]').exists()).toBe(false)

      appStore.cachedPublicSettings.available_channels_enabled = true
      await nextTick()
      expect(wrapper.find('a[href="/model-square"]').exists()).toBe(true)
      expect(wrapper.find('a[href="/available-channels"]').exists()).toBe(true)
    } finally {
      wrapper.unmount()
    }
  })
})
