import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ref } from 'vue'
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

describe('Integration docs navigation', () => {
  it.each([
    { role: 'user', simple: false },
    { role: 'user', simple: true },
    { role: 'admin', simple: false },
  ] as const)('is available for $role (simple: $simple) without image access', async ({ role, simple }) => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useAppStore().cachedPublicSettings = { available_channels_enabled: false } as PublicSettings
    const auth = useAuthStore()
    auth.user = { id: 1, role } as User
    vi.spyOn(auth, 'isSimpleMode', 'get').mockReturnValue(simple)
    vi.spyOn(useAdminSettingsStore(), 'fetch').mockResolvedValue(undefined)
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
    })
    await router.push('/integration-docs?category=video')
    const wrapper = mount(AppSidebar, {
      global: { plugins: [pinia, router], stubs: { VersionBadge: true, Icon: true } },
    })
    try {
      const link = wrapper.get('a[href="/integration-docs"]')
      expect(link.text()).toContain('nav.integrationDocs')
      expect(link.classes()).toContain('sidebar-link-active')
      expect(wrapper.find('a[href="/batch-image"]').exists()).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })
})
