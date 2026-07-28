import { computed, ref } from 'vue'
import { keysAPI } from '@/api/keys'
import { useAuthStore } from '@/stores/auth'
import type { ApiKey } from '@/types'
import { isImageStudioPlatform } from './capabilities'

const loaded = ref(false)
const loading = ref(false)
const hasAllowedKey = ref(false)
let pendingLoad: Promise<boolean> | null = null
const pageSize = 100

export function keyAllowsImageStudio(key: ApiKey): boolean {
  return (
    key.status === 'active' &&
    key.group?.allow_image_generation === true &&
    isImageStudioPlatform(key.group?.platform)
  )
}

export async function listEligibleImageKeys(): Promise<ApiKey[]> {
  const keys: ApiKey[] = []
  let page = 1
  while (true) {
    const response = await keysAPI.list(page, pageSize, {
      status: 'active',
      sort_by: 'created_at',
      sort_order: 'desc',
    })
    keys.push(...(response.items || []).filter(keyAllowsImageStudio))
    if (page >= response.pages || (response.items || []).length === 0) return keys
    page += 1
  }
}

async function loadImageStudioAccess(force = false): Promise<boolean> {
  const authStore = useAuthStore()
  if (!authStore.isAuthenticated) {
    loaded.value = true
    hasAllowedKey.value = false
    return false
  }
  if (loaded.value && !force) return hasAllowedKey.value
  if (pendingLoad && !force) return pendingLoad

  loading.value = true
  pendingLoad = listEligibleImageKeys()
    .then((keys) => {
      hasAllowedKey.value = keys.length > 0
      loaded.value = true
      return hasAllowedKey.value
    })
    .catch(() => {
      hasAllowedKey.value = false
      loaded.value = true
      return false
    })
    .finally(() => {
      loading.value = false
      pendingLoad = null
    })
  return pendingLoad
}

export function useImageStudioAccess() {
  return {
    canUseImageStudio: computed(() => hasAllowedKey.value),
    imageStudioAccessLoaded: computed(() => loaded.value),
    imageStudioAccessLoading: computed(() => loading.value),
    refreshImageStudioAccess: loadImageStudioAccess,
  }
}
