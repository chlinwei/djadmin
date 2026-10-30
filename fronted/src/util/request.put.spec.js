import { beforeEach, describe, expect, it, vi } from 'vitest'
const mocks = vi.hoisted(() => ({ http: vi.fn() }))
vi.mock('axios', () => {
  mocks.http.interceptors = { request: { use: vi.fn() }, response: { use: vi.fn() } }
  return { default: { create: () => mocks.http } }
})
vi.mock('@/api/user', () => ({ getToken: () => 'test-token' }))
vi.mock('@/router', () => ({ default: {} }))
import request, { put } from './request'

beforeEach(() => { mocks.http.mockReset() })
describe('shared PUT requests', () => {
  it('uses the existing intercepted HTTP client without changing payload or result', async () => {
    const response = { data: { code: 200, msg: 'success', data: { id: 4 } } }
    const payload = { name: '服务调用', enabled: false }
    mocks.http.mockResolvedValue(response)
    expect(await request.put('assets/service-dependency-types/4/', payload)).toBe(response)
    expect(mocks.http).toHaveBeenCalledWith({ url: 'assets/service-dependency-types/4/', method: 'put', data: payload })
    expect(request.put).toBe(put)
  })
  it('propagates failure rather than returning a success-shaped response', async () => {
    const failure = new Error('网络不可用')
    mocks.http.mockRejectedValue(failure)
    await expect(put('assets/service-dependencies/4/', {})).rejects.toBe(failure)
  })
})
