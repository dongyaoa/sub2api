import { apiClient } from './client'
import type { UserAvailableChannel } from './channels'

// Reuse the permission-filtered catalog shape without the Available Channels switch.
export async function getCatalog(): Promise<UserAvailableChannel[]> {
  const { data } = await apiClient.get<UserAvailableChannel[]>('/model-square/catalog')
  return data
}

export default { getCatalog }
