import { computed, ref } from 'vue'
import { keysAPI } from '@/api/keys'
import { useAuthStore } from '@/stores/auth'
import type { ApiKey } from '@/types'
import { filterImageModels, isImageStudioPlatform } from './capabilities'
import { listImageModels } from './api'
import type { ImageModel } from './types'

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

// Keep every eligible key for history, but only expose groups with image models
// in the generation selector. Catalogs stay in memory for this page visit.
export async function loadImageModelCatalogs(keys: ApiKey[], signal?: AbortSignal): Promise<Map<number, ImageModel[]>> {
  const groups = new Map<string, ApiKey[]>()
  for (const key of keys) {
    const groupID = key.group_id ?? key.group?.id
    const identity = groupID == null ? `key:${key.id}` : `group:${groupID}`
    const groupKeys = groups.get(identity) || []
    groupKeys.push(key)
    groups.set(identity, groupKeys)
  }
  const pending = [...groups.values()]
  const catalogs = new Map<number, ImageModel[]>()
  let cursor = 0
  await Promise.all(Array.from({ length: Math.min(4, pending.length) }, async () => {
    while (cursor < pending.length && !signal?.aborted) {
      const groupKeys = pending[cursor++]!
      for (const key of groupKeys) {
        const platform = key.group?.platform
        if (!isImageStudioPlatform(platform)) continue
        try {
          const response = await listImageModels(key.key, signal)
          if (signal?.aborted) return
          const models = filterImageModels(platform, response.data || [])
          for (const groupKey of groupKeys) catalogs.set(groupKey.id, models)
          break
        } catch {
          if (signal?.aborted) return
          // Try another key in the same group; failure is not evidence of availability.
        }
      }
    }
  }))
  return catalogs
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
