import { beforeEach, describe, expect, it, vi } from 'vitest'
import requestUtil from '@/util/request'
import { checkManagedTargetServiceStatus, startManagedTargetService } from './monitor'

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
})
