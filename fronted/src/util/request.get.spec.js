import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ http: vi.fn() }))
vi.mock('axios', () => {
  mocks.http.interceptors = { request: { use: vi.fn() }, response: { use: vi.fn() } }
  return { default: { create: () => mocks.http } }
})
vi.mock('@/api/user', () => ({ getToken: () => 'test-token' }))
vi.mock('@/router', () => ({ default: {} }))
import { get } from './request'

beforeEach(() => mocks.http.mockReset())

describe('共享 GET 请求', () => {
  it('使用 URL 参数，不发送请求体，保留原始响应', async () => {
    const response = { data: { code: 200, msg: 'success', data: {} } }
    mocks.http.mockResolvedValueOnce(response)
    expect(await get('assets/hosts/refresh-info/', { ids: '7,8' })).toBe(response)
    expect(mocks.http).toHaveBeenCalledWith({
      url: 'assets/hosts/refresh-info/', method: 'get', params: { ids: '7,8' }, suppressBusinessErrorMessage: false,
    })
  })

  it('透传超时和错误提示策略，同时仍向调用方抛出失败', async () => {
    const failure = new Error('状态查询失败')
    mocks.http.mockRejectedValueOnce(failure)
    await expect(get('status/', {}, 45000, { suppressBusinessErrorMessage: true })).rejects.toBe(failure)
    expect(mocks.http).toHaveBeenCalledWith({
      url: 'status/', method: 'get', params: {}, timeout: 45000, suppressBusinessErrorMessage: true,
    })
  })
})
