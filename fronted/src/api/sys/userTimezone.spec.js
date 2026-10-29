import { afterEach, describe, expect, it, vi } from 'vitest'
import requestUtil from '@/util/request'
import { getCurrentUserInfo } from './userTimezone'
import { updateUserInfo } from '@/api/user'

vi.mock('@/util/request', () => ({ default: { get: vi.fn(), post: vi.fn(), patch: vi.fn() } }))
vi.mock('@/util/jsencrypt', () => ({ encrypt: vi.fn(), decrypt: vi.fn() }))
vi.mock('@/router', () => ({ default: { push: vi.fn() } }))
vi.mock('@/store', () => ({ default: { commit: vi.fn() } }))
afterEach(() => vi.clearAllMocks())

describe('self-profile API', () => {
  it('returns the profile save promise and uses one request for both fields', async () => {
    const response = { data: { data: { user: { id: 3, timezone: 'Asia/Hong_Kong' } } } }
    requestUtil.post.mockResolvedValueOnce(response)
    expect(await updateUserInfo({ phonenumber: '256', timezone: 'Asia/Hong_Kong' })).toBe(response)
    expect(requestUtil.post).toHaveBeenCalledOnce()
    expect(requestUtil.post).toHaveBeenCalledWith('sys/usercenter/updateUserInfo/', { phonenumber: '256', timezone: 'Asia/Hong_Kong' })
  })

  it('reads the current user without a target ID', () => {
    getCurrentUserInfo()
    expect(requestUtil.get).toHaveBeenCalledWith('sys/users/current/')
  })
})
