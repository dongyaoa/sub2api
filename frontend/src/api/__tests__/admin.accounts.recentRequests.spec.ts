import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { post } }))

import accountsAPI, { getBatchRecentRequests, type AccountRecentRequest } from '@/api/admin/accounts'

describe('admin recent account requests API', () => {
  beforeEach(() => post.mockReset())

  it('is exposed on the shared accounts API and reads the endpoint map', async () => {
    const rows: AccountRecentRequest[] = [{ created_at: '2026-09-11T09:40:40Z', success: false, status_code: 400 }]
    post.mockResolvedValueOnce({ data: { '12': rows } })
    expect(accountsAPI.getBatchRecentRequests).toBe(getBatchRecentRequests)
    await expect(accountsAPI.getBatchRecentRequests([12])).resolves.toEqual({ requests: { '12': rows } })
    expect(post).toHaveBeenCalledWith('/admin/accounts/recent-requests/batch', { account_ids: [12] })
  })

  it('does not send an invalid empty batch', async () => {
    await expect(getBatchRecentRequests([])).resolves.toEqual({ requests: {} })
    expect(post).not.toHaveBeenCalled()
  })

  it('treats accounts without history as an empty success', async () => {
    post.mockResolvedValueOnce({ data: { '12': [] } })
    await expect(getBatchRecentRequests([12])).resolves.toEqual({ requests: { '12': [] } })
  })

  it('splits larger table pages into batches accepted by the endpoint', async () => {
    const ids = Array.from({ length: 201 }, (_, index) => index + 1)
    post.mockResolvedValueOnce({ data: { '1': [] } }).mockResolvedValueOnce({ data: { '201': [] } })
    await expect(getBatchRecentRequests(ids)).resolves.toEqual({ requests: { '1': [], '201': [] } })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/recent-requests/batch', { account_ids: ids.slice(0, 200) })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/recent-requests/batch', { account_ids: [201] })
  })
})
