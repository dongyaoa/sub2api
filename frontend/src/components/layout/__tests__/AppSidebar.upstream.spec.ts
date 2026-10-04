import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { useAdminSettingsStore, useAppStore, useAuthStore } from '@/stores'
import { usePelicanMonitorStore } from '@/stores/pelicanMonitor'
import type { PublicSettings, User } from '@/types'
import AppSidebar from '../AppSidebar.vue'

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key, locale: ref('zh') }),
}))
vi.mock('@/api/pelicanMonitor', () => ({
  pelicanMonitorAPI: { config: async () => ({ enabled: false, title: '', description: '', notice: '' }) },
}))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: ref(false), refreshBatchImageAccess: vi.fn() }),
}))

describe('upstream center and pelican navigation', () => {
  it.each(['user', 'admin'] as const)('applies admin and public display visibility for %s', async role => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useAppStore().cachedPublicSettings = { payment_enabled: false } as PublicSettings
    const auth = useAuthStore()
    auth.user = { id: 1, role } as User
    auth.token = 'test-token'
    vi.spyOn(useAdminSettingsStore(), 'fetch').mockResolvedValue(undefined)
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
    await router.push('/dashboard')
    const wrapper = mount(AppSidebar, { global: { plugins: [pinia, router], stubs: { VersionBadge: true, Icon: true } } })
    try {
      await flushPromises()
      expect(wrapper.find('a[href="/admin/upstreams"]').exists()).toBe(role === 'admin')
      expect(wrapper.find('a[href="/pelican-monitor"]').exists()).toBe(false)
      usePelicanMonitorStore().apply({ enabled: true, title: '', description: '', notice: '' })
      await nextTick()
      expect(wrapper.get('a[href="/pelican-monitor"]').text()).toContain('pelicanMonitor.title')
      usePelicanMonitorStore().disable()
      await nextTick()
      expect(wrapper.find('a[href="/pelican-monitor"]').exists()).toBe(false)
      expect(wrapper.find('a[href="/model-square"]').exists()).toBe(true)
    } finally {
      wrapper.unmount()
      vi.restoreAllMocks()
    }
  })
})
