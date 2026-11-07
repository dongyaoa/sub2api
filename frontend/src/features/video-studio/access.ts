import type { ApiKey } from '@/types'
import { keysAPI } from '@/api/keys'
import { listVideoModels } from './api'
import { filterVideoModels } from './capabilities'
import type { VideoModel } from './types'

export function keyAllowsVideoStudio(key: ApiKey): boolean {
  return (
    key.status === 'active' &&
    key.group?.allow_image_generation === true &&
    ['grok', 'openai', 'gemini', 'composite'].includes(key.group?.platform || '')
  )
}

export async function listEligibleVideoKeys(): Promise<ApiKey[]> {
  const keys: ApiKey[] = []
  let page = 1
  while (true) {
    const response = await keysAPI.list(page, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    keys.push(...(response.items || []).filter(keyAllowsVideoStudio))
    if (page >= response.pages || (response.items || []).length === 0) return keys
    page += 1
  }
}

// All eligible keys remain usable for history; generation requires a model.
export async function loadVideoModelCatalogs(keys: ApiKey[], signal?: AbortSignal): Promise<Map<number, VideoModel[]>> {
  const groups = new Map<string, ApiKey[]>()
  for (const key of keys) {
    const groupID = key.group_id ?? key.group?.id
    const identity = groupID == null ? `key:${key.id}` : `group:${groupID}`
    const groupKeys = groups.get(identity) || []
    groupKeys.push(key)
    groups.set(identity, groupKeys)
  }
  const pending = [...groups.values()]
  const catalogs = new Map<number, VideoModel[]>()
  let cursor = 0
  await Promise.all(Array.from({ length: Math.min(4, pending.length) }, async () => {
    while (cursor < pending.length && !signal?.aborted) {
      const groupKeys = pending[cursor++]!
      // A failed key must not hide a group's other working keys.
      for (const key of groupKeys) {
        try {
          const response = await listVideoModels(key.key, signal)
          if (signal?.aborted) return
          const models = filterVideoModels(response.data || [])
          for (const groupKey of groupKeys) catalogs.set(groupKey.id, models)
          break
        } catch {
          if (signal?.aborted) return
          // A failed lookup never counts as evidence of video availability.
        }
      }
    }
  }))
  return catalogs
}
