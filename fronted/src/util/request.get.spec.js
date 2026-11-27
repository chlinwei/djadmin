import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ http: vi.fn() }))
vi.mock('axios', () => {
  mocks.http.interceptors = { request: { use: vi.fn() }, response: { use: vi.fn() } }
  return { default: { create: () => mocks.http } }
})
vi.mock('@/api/user', () => ({ getToken: () => 'test-token' }))
vi.mock('@/router', () => ({ default: {} }))
vi.mock('ant-design-vue', () => ({ message: { error: vi.fn() } }))
import { get } from './request'
import { message } from 'ant-design-vue'

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

  it('保留业务错误响应中的逐项结果，供调用方处理部分成功', async () => {
    const response = {
      config: { suppressBusinessErrorMessage: true },
      data: { code: 10001, msg: '删除失败', data: { count: 1, results: [
        { id: 1, ok: true, code: 200 },
        { id: 2, ok: false, code: 400, message: '仍被引用' },
      ] } },
    }
    const onResponse = mocks.http.interceptors.response.use.mock.calls[0][0]
    expect(() => onResponse(response)).toThrow(expect.objectContaining({
      name: 'BusinessError',
      message: '删除失败',
      code: 10001,
      response,
    }))
  })

  it('批量失败时展示每个失败目标的原因', async () => {
    const response = {
      config: {},
      data: { code: 10001, msg: '部分操作失败', data: { results: [
        { id: 1, code: 200, ok: true },
        { id: 2, code: 400, ok: false, message: '仍被引用' },
      ] } },
    }
    const onResponse = mocks.http.interceptors.response.use.mock.calls[0][0]
    expect(() => onResponse(response)).toThrow(expect.objectContaining({ response }))
    expect(message.error).toHaveBeenCalledWith('部分操作失败：#2：仍被引用')
  })

  it('单项安装未受理也展示已落库的目标及作业号', () => {
    const response = { data: { code: 600, msg: '安装下发失败', data: { results: [
      { id: 7, ok: false, message: '入队失败', detail: { created: true, install_accepted: false, job_id: 3 } },
    ] } } }
    const onResponse = mocks.http.interceptors.response.use.mock.calls[0][0]
    expect(() => onResponse(response)).toThrow(expect.objectContaining({ response }))
    expect(message.error).toHaveBeenCalledWith('安装下发失败：#7：目标已创建，安装未受理；入队失败（作业 3）')
  })

  it('非 2xx 的业务错误保留原信封，协议响应不触发业务逻辑', () => {
    const [onResponse, onError] = mocks.http.interceptors.response.use.mock.calls[0]
    const response = { status: 403, data: { code: 403, msg: '无权限', data: null } }
    expect(() => onError({ response })).toThrow(expect.objectContaining({ code: 403, response }))
    const protocol = { data: { code: 301, status: 'success' } }
    expect(onResponse(protocol)).toBe(protocol)
  })

  it('网络中断不等于写入已回滚', async () => {
    const [, onError] = mocks.http.interceptors.response.use.mock.calls[0]
    const timeout = Object.assign(new Error('timeout'), { request: {}, config: { method: 'post' } })
    await expect(onError(timeout)).rejects.toThrow('操作结果未知')
  })
})
