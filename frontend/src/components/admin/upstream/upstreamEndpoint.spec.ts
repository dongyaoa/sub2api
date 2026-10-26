import { describe, expect, it } from 'vitest'
import { isValidUpstreamEndpoint, upstreamEndpointErrorKey } from './upstreamEndpoint'

describe('upstream endpoint syntax', () => {
  it.each(['http://216.158.77.26:7070', 'https://8.8.8.8:8443/v1', 'http://relay.example:8080/tenant/v1', 'https://relay.example/v1', 'http://[2606:4700:4700::1111]:7070/v1', ' https://8.8.8.8:443/v1/ '])('accepts %s without changing its protocol or port', endpoint => {
    expect(isValidUpstreamEndpoint(endpoint)).toBe(true)
  })
  it.each(['216.158.77.26:7070', 'http:///8.8.8.8', 'ftp://8.8.8.8', 'http://user:secret@8.8.8.8:7070', 'http://@8.8.8.8', 'http://8.8.8.8:0', 'http://8.8.8.8:65536', 'http://8.8.8.8:', 'http://8.8.8.8:port', 'http://8.8.8.8/v1?key=secret', 'http://8.8.8.8/v1?', 'http://8.8.8.8/v1#test', 'http://8.8.8.8/v1#', 'http://8.8.8.8\\path', 'http://8.8.8.8/with space'])('rejects malformed or ambiguous endpoint %s', endpoint => {
    expect(isValidUpstreamEndpoint(endpoint)).toBe(false)
  })
  it('maps backend endpoint failures without hiding unrelated errors', () => {
    expect(upstreamEndpointErrorKey({ reason: 'CHANNEL_MONITOR_ENDPOINT_PRIVATE' })).toBe('upstreamCenter.endpointError.private')
    expect(upstreamEndpointErrorKey({ reason: 'CHANNEL_MONITOR_ENDPOINT_UNREACHABLE' })).toBe('upstreamCenter.endpointError.unreachable')
    expect(upstreamEndpointErrorKey({ metadata: { field: 'endpoint' }, message: 'invalid upstream configuration' })).toBe('upstreamCenter.form.validUrl')
    expect(upstreamEndpointErrorKey({ metadata: { field: 'models' }, message: 'models required' })).toBeUndefined()
  })
})
