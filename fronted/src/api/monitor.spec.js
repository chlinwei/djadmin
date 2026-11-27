import { beforeEach, describe, expect, it, vi } from 'vitest'
import requestUtil from '@/util/request'
import {
  batchCreateLogCollectionTargets, batchStartLogCollectionTargets, batchStopLogCollectionTargets,
  batchCreateMonitorTargets, batchStartMonitorTargets, batchStopMonitorTargets,
  checkManagedTargetServiceStatus, startManagedTargetService,
} from './monitor'

vi.mock('@/util/request', () => ({ default: { get: vi.fn(), post: vi.fn() } }))

describe('Exporter 状态查询', () => {
  beforeEach(() => vi.clearAllMocks())

  it('状态查询用 GET，启动动作仍用 POST', async () => {
    await checkManagedTargetServiceStatus(7)
    expect(requestUtil.get).toHaveBeenCalledWith('monitor/targets/7/check-service-status/')
    expect(requestUtil.post).not.toHaveBeenCalled()
    await startManagedTargetService(7)
    expect(requestUtil.post).toHaveBeenCalledWith('monitor/targets/7/start-service/')
  })

  describe('目标批量操作', () => {
    const hostRequest = { host_ids: [7], install_now: true }
    it.each([
      [batchCreateLogCollectionTargets, [[7], true], 'log-targets/batch-create/', hostRequest],
      [batchStartLogCollectionTargets, [[9]], 'log-targets/batch-start-service/', { ids: [9] }],
      [batchStopLogCollectionTargets, [[9]], 'log-targets/batch-stop-service/', { ids: [9] }],
      [batchCreateMonitorTargets, [hostRequest], 'targets/batch-create/', hostRequest],
      [batchStartMonitorTargets, [[9]], 'targets/batch-start-service/', { ids: [9] }],
      [batchStopMonitorTargets, [[9]], 'targets/batch-stop-service/', { ids: [9] }],
    ])('仅由页面处理逐项业务错误: %s', async (send, args, path, body) => {
      await send(...args)
      expect(requestUtil.post).toHaveBeenLastCalledWith(`monitor/${path}`, body, null, { suppressBusinessErrorMessage: true })
    })
  })
})
