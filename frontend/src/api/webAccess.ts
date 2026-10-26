import { buildApiUrl, buildGatewayUrl } from './url'

export const DEFAULT_WEB_ACCESS_MESSAGE = '本網站暫不向中國大陸地區提供網頁訪問服務。'

export class WebAccessLocationUnavailableError extends Error {
  constructor() {
    super('Web access location could not be checked')
    this.name = 'WebAccessLocationUnavailableError'
  }
}

export type WebAccessLocationSource = 'client_ip' | 'local_network'

export interface WebAccessStatus {
  enabled: boolean
  blocked: boolean
  country_code?: string
  location_source?: WebAccessLocationSource
  message: string
  setup_required?: boolean
}

/** A separate public request: no authentication refresh, shared settings cache or UI headers. */
export async function getWebAccessStatus(): Promise<WebAccessStatus> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 8000)
  try {
    const response = await fetch(buildApiUrl('/settings/web-access'), {
      method: 'GET',
      cache: 'no-store',
      credentials: 'include',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
    // During first installation the application API does not exist yet. Only a
    // positive setup-status response may allow the installer through this gate.
    if (response.status === 404) {
      const setupResponse = await fetch(buildGatewayUrl('/setup/status'), {
        cache: 'no-store',
        headers: { Accept: 'application/json' },
        signal: controller.signal,
      })
      if (setupResponse.ok) {
        const setup = await setupResponse.json()
        if (setup?.data?.needs_setup === true) {
          return { enabled: false, blocked: false, message: '', setup_required: true }
        }
      }
    }
    if (response.status === 503) {
      const failure = await response.json().catch(() => null)
      if (failure?.reason === 'WEB_ACCESS_LOCAL_NETWORK_UNAVAILABLE') {
        throw new WebAccessLocationUnavailableError()
      }
    }
    if (!response.ok) throw new Error('Web access status is unavailable')
    const body = await response.json()
    const status = body?.data
    if (body?.code !== 0 || typeof status?.enabled !== 'boolean' || typeof status?.blocked !== 'boolean') {
      throw new Error('Invalid web access status')
    }
    return {
      enabled: status.enabled,
      blocked: status.blocked,
      country_code: typeof status.country_code === 'string' ? status.country_code : undefined,
      location_source: status.location_source === 'client_ip' || status.location_source === 'local_network'
        ? status.location_source
        : undefined,
      message: typeof status.message === 'string' && status.message.trim()
        ? status.message
        : DEFAULT_WEB_ACCESS_MESSAGE,
    }
  } finally {
    window.clearTimeout(timeout)
  }
}
