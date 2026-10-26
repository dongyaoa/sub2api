import { extractApiErrorCode, extractApiErrorMetadata } from '@/utils/apiError'

/** Checks URL syntax only. The server validates public addresses and DNS results. */
export function isValidUpstreamEndpoint(value: string | null | undefined): boolean {
  const endpoint = value?.trim() || ''
  if (!/^https?:\/\//i.test(endpoint) || /[\s\\?#]/.test(endpoint)) return false
  const authority = endpoint.match(/^https?:\/\/([^/]+)/i)?.[1]
  if (!authority || authority.includes('@') || authority.endsWith(':')) return false
  try {
    const url = new URL(endpoint)
    return Boolean(url.hostname && !url.username && !url.password && (!url.port || Number(url.port) > 0))
  } catch {
    return false
  }
}

/** Return a translation key without exposing raw endpoint diagnostics. */
export function upstreamEndpointErrorKey(error: unknown): string | undefined {
  switch (extractApiErrorCode(error)) {
    case 'CHANNEL_MONITOR_ENDPOINT_PRIVATE': return 'upstreamCenter.endpointError.private'
    case 'CHANNEL_MONITOR_ENDPOINT_UNREACHABLE': return 'upstreamCenter.endpointError.unreachable'
    case 'CHANNEL_MONITOR_INVALID_ENDPOINT':
    case 'CHANNEL_MONITOR_ENDPOINT_SCHEME':
    case 'CHANNEL_MONITOR_ENDPOINT_PATH': return 'upstreamCenter.form.validUrl'
  }
  if (extractApiErrorMetadata(error)?.field === 'endpoint' || extractApiErrorMetadata(error)?.field === 'newapi_api_base') return 'upstreamCenter.form.validUrl'
  return undefined
}
