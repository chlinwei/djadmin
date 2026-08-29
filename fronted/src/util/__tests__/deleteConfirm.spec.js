import { describe, expect, it, vi, beforeEach } from 'vitest'
import { Modal, message } from 'ant-design-vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'

vi.mock('ant-design-vue', () => ({
  Modal: { confirm: vi.fn() },
  message: { error: vi.fn() },
}))

// 直接触发 antd 传入的 onOk，等价于用户点击“确认删除”。
function triggerOk() {
  return Modal.confirm.mock.calls.at(-1)[0].onOk()
}

describe('openDeleteConfirm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('resolves true when confirmation succeeds', async () => {
    const pending = openDeleteConfirm({ onConfirm: vi.fn().mockResolvedValue(undefined) })
    await triggerOk()

    await expect(pending).resolves.toBe(true)
    expect(message.error).not.toHaveBeenCalled()
  })

  it('swallows failures so antd cannot leak an unhandled rejection', async () => {
    const pending = openDeleteConfirm({ onConfirm: vi.fn().mockRejectedValue(new Error('boom')) })

    await expect(triggerOk()).resolves.toBeUndefined()
    await expect(pending).resolves.toBe(false)
  })

  it('reports http errors that the request interceptor left silent', async () => {
    const error = Object.assign(new Error('主机组巡检不能使用应用上下文变量'), { isAxiosError: true })
    const pending = openDeleteConfirm({ onConfirm: vi.fn().mockRejectedValue(error) })
    await triggerOk()

    await expect(pending).resolves.toBe(false)
    expect(message.error).toHaveBeenCalledWith('主机组巡检不能使用应用上下文变量')
  })

  it('does not double-report business errors already shown by the interceptor', async () => {
    const pending = openDeleteConfirm({ onConfirm: vi.fn().mockRejectedValue(new Error('巡检组已被任务使用')) })
    await triggerOk()

    await expect(pending).resolves.toBe(false)
    expect(message.error).not.toHaveBeenCalled()
  })
})
