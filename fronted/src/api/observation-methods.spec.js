import { beforeEach, describe, expect, it, vi } from 'vitest'
import request from '@/util/request'
import { batchRefreshHostInfo, refreshHostInfo } from '@/api/assets/host'
import { refreshApplicationServiceRuntimeStatus } from '@/api/assets/application'
import { checkLogCollectionStatus, simulateElasticsearchPipeline, startLogCollectionService, testElasticsearchCluster } from '@/api/monitor'

vi.mock('@/util/request', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

beforeEach(() => vi.clearAllMocks())

describe('状态与主机事实查询方法', () => {
  it.each([
    [() => refreshHostInfo(7), ['assets/hosts/7/refresh-info/']],
    [() => batchRefreshHostInfo([7, 8]), ['assets/hosts/refresh-info/', { ids: '7,8' }]],
    [() => checkLogCollectionStatus(7), ['monitor/log-targets/7/check-status/']],
    [() => testElasticsearchCluster(7), ['monitor/elasticsearch-clusters/7/test-connection/']],
    [() => refreshApplicationServiceRuntimeStatus(7), ['assets/application-services/7/refresh-runtime-status/', {}, 120000, {}]],
  ])('通过 GET 读取状态，保持响应不变 %#', async (query, expected) => {
    const response = { data: { code: 200, msg: 'success', data: {} } }
    request.get.mockResolvedValueOnce(response)
    expect(await query()).toBe(response)
    expect(request.get).toHaveBeenCalledWith(...expected)
    expect(request.post).not.toHaveBeenCalled()
  })

  it('逻辑服务查询保留自定义超时和后台轮询的错误提示选项', async () => {
    const options = { suppressBusinessErrorMessage: true }
    await refreshApplicationServiceRuntimeStatus(7, 45000, options)
    expect(request.get).toHaveBeenCalledWith(
      'assets/application-services/7/refresh-runtime-status/', {}, 45000, options,
    )
  })

  it('批量主机参数为空时传递空参数，由后端拒绝而非扩大为全部主机', async () => {
    await batchRefreshHostInfo([])
    expect(request.get).toHaveBeenCalledWith('assets/hosts/refresh-info/', { ids: '' })
  })

  it('样例解析和启动服务仍使用 POST，不伪装成查询', async () => {
    const sample = { pipeline: { processors: [] }, docs: [{ message: 'sample' }] }
    await simulateElasticsearchPipeline(7, sample)
    await startLogCollectionService(7)
    expect(request.post).toHaveBeenCalledWith('monitor/elasticsearch-clusters/7/pipeline-simulate/', sample)
    expect(request.post).toHaveBeenCalledWith('monitor/log-targets/7/start-service/', {})
    expect(request.get).not.toHaveBeenCalled()
  })
})
