import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'ant-design-vue'
import { reportTargetBatch } from './targetBatch'

vi.mock('ant-design-vue', () => ({
  message: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
  Modal: { warning: vi.fn() },
}))

describe('target batch feedback', () => {
  beforeEach(() => vi.clearAllMocks())

  it('continues to the accepted install job even when another host failed', async () => {
    const data = { install_batch: { id: 17 }, results: [
      { host: 'node-1', ok: true, detail: { install_accepted: true, job_id: 17 } },
      { host: 'node-2', ok: false, message: 'target already managed' },
    ] }
    const request = Promise.reject({ response: { data: { code: 10001, data } } })
    expect(await reportTargetBatch('纳管并下发安装', request)).toBe(data)
    expect(message.warning).toHaveBeenCalledWith(expect.stringContaining('node-2：target already managed'))
    expect(message.success).not.toHaveBeenCalled()
  })

  it('reports committed targets separately from failed dispatch and returns data for refresh', async () => {
    const data = { results: [{ id: 7, ok: false, detail: { job_id: 3, created: true, install_accepted: false }, message: '入队失败' }] }
    const request = Promise.reject({ response: { data: { code: 600, data } } })
    expect(await reportTargetBatch('纳管并下发安装', request)).toBe(data)
    expect(message.error).toHaveBeenCalledWith(expect.stringContaining('#7：目标已创建，安装未受理；入队失败（作业 3）'))
  })

  it('does not turn a transport or validation failure into a completed action', async () => {
    const error = new Error('请求失败')
    await expect(reportTargetBatch('批量启动', Promise.reject(error))).rejects.toBe(error)
    expect(message.success).not.toHaveBeenCalled()
  })
})
