import { beforeEach, describe, expect, it, vi } from 'vitest'
import requestUtil from '@/util/request'
import { precheckInventoryLimit, precheckTaskRun, runTaskNow } from './automation'

vi.mock('@/util/request', () => ({ default: { get: vi.fn(), post: vi.fn() } }))

describe('自动化预检使用 GET', () => {
  beforeEach(() => vi.clearAllMocks())

  it.each([
    [precheckInventoryLimit, 'inventories/7/precheck-limit/'],
    [precheckTaskRun, 'tasks/7/precheck/'],
  ])('保留主机列表及 limit 查询条件：%s', async (precheck, path) => {
    const response = { data: { code: 200 } }
    requestUtil.get.mockResolvedValue(response)
    expect(await precheck(7, { host_ids: [10, 11], limit: 'host-*' })).toBe(response)
    expect(requestUtil.get).toHaveBeenCalledWith(`sys/automation/${path}`, { host_ids: '10,11', limit: 'host-*' })
    expect(requestUtil.post).not.toHaveBeenCalled()
  })

  it('没有主机筛选时沿用服务端默认范围，实际执行仍为 POST', async () => {
    await precheckTaskRun(7)
    expect(requestUtil.get).toHaveBeenCalledWith('sys/automation/tasks/7/precheck/', { host_ids: undefined, limit: undefined })
    await runTaskNow(7, { host_ids: [10] })
    expect(requestUtil.post).toHaveBeenCalledWith('sys/automation/tasks/7/run_now/', { host_ids: [10] })
  })
})
