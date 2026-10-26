import { apiClient } from './client'
import type { UserAvailableChannel } from './channels'

// Explicit channel pricing only; no mapping-only models or global-price fallback.
// Group visibility is filtered by the server, independently of Available Channels.
export async function getCatalog(): Promise<UserAvailableChannel[]> {
  const { data } = await apiClient.get<UserAvailableChannel[]>('/model-square/catalog')
  return data
}

export default { getCatalog }
