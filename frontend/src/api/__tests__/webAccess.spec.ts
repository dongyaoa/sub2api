import { afterEach, describe, expect, it, vi } from 'vitest'
import { getWebAccessStatus, WebAccessLocationUnavailableError } from '../webAccess'

const response = (data: unknown, status = 200) => ({ ok: status < 400, status, json: async () => data })

afterEach(() => vi.unstubAllGlobals())

describe('web access request', () => {
  it('uses an uncached independent request and returns the server decision', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ code: 0, data: { enabled: true, blocked: true, country_code: 'CN', message: '暂不支持访问' } }))
    vi.stubGlobal('fetch', fetchMock)
    expect(await getWebAccessStatus()).toMatchObject({ blocked: true, country_code: 'CN', message: '暂不支持访问' })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/settings/web-access', expect.objectContaining({ cache: 'no-store', credentials: 'include' }))
  })

  it('allows installation only when the setup-only server explicitly requires it', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(response({}, 404))
      .mockResolvedValueOnce(response({ data: { needs_setup: true } })))
    expect(await getWebAccessStatus()).toMatchObject({ blocked: false, setup_required: true })
  })

  it.each(['client_ip', 'local_network'])('preserves the validated server location source %s', async locationSource => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ code: 0, data: {
      enabled: true, blocked: false, message: '', location_source: locationSource,
    } })))
    expect((await getWebAccessStatus()).location_source).toBe(locationSource)
  })

  it.each(['local_network_unavailable', 'LOCAL_NETWORK', 5, null])('does not trust an unsupported location source %s', async locationSource => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ code: 0, data: {
      enabled: true, blocked: false, message: '', location_source: locationSource,
    } })))
    expect((await getWebAccessStatus()).location_source).toBeUndefined()
  })

  it('distinguishes an explicit local detection failure from a routine network error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({
      code: 503,
      reason: 'WEB_ACCESS_LOCAL_NETWORK_UNAVAILABLE',
      metadata: { location_source: 'local_network_unavailable' },
    }, 503)))
    await expect(getWebAccessStatus()).rejects.toBeInstanceOf(WebAccessLocationUnavailableError)
  })

  it('does not interpret a missing endpoint on an installed server as allowed', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(response({}, 404))
      .mockResolvedValueOnce(response({ data: { needs_setup: false } })))
    await expect(getWebAccessStatus()).rejects.toThrow('unavailable')
  })

  it.each([{ code: 0, data: { enabled: true } }, { code: 1, data: { enabled: false, blocked: false } }])('rejects an incomplete or unsuccessful response', async body => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(body)))
    await expect(getWebAccessStatus()).rejects.toThrow('Invalid')
  })
})
