import type { ApiKey } from '@/types'
import { listEligibleImageKeys } from '@/features/image-studio/access'

export function keyAllowsVideoStudio(key: ApiKey): boolean {
  return (
    key.status === 'active' &&
    key.group?.allow_image_generation === true &&
    key.group?.platform === 'grok'
  )
}

export async function listEligibleVideoKeys(): Promise<ApiKey[]> {
  return (await listEligibleImageKeys()).filter(keyAllowsVideoStudio)
}
