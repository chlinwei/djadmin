import { describe, expect, it, vi, beforeEach } from 'vitest'
import { Modal, message } from 'ant-design-vue'
import { assertBatchDeleteSucceeded, openDeleteConfirm } from '@/util/deleteConfirm'

vi.mock('ant-design-vue', () => ({
  Modal: { confirm: vi.fn() },
  message: { error: vi.fn() },
}))

// 直接触发 antd 传入的 onOk，等价于用户点击“确认删除”。
function triggerOk() {
  return Modal.confirm.mock.calls.at(-1)[0].onOk()
}

describe('assertBatchDeleteSucceeded', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('accepts only explicit success for every requested id', () => {
    const response = { data: { data: { results: [{ id: 1, ok: true }, { id: 2, ok: true }] } } }
    expect(() => assertBatchDeleteSucceeded(response, [1, 2])).not.toThrow()
    expect(message.error).not.toHaveBeenCalled()
  })

  it.each([
    ['missing response', undefined],
    ['missing results', { data: { data: { count: 1 } } }],
    ['invalid results', { data: { data: { results: {} } } }],
    ['wrong id', { data: { data: { results: [{ id: 2, ok: true }] } } }],
    ['non-boolean ok', { data: { data: { results: [{ id: 1, ok: 'true' }] } } }],
    ['missing item', { data: { data: { results: [null] } } }],
  ])('rejects %s', (_label, response) => {
    expect(() => assertBatchDeleteSucceeded(response, [1])).toThrow('删除结果异常')
    expect(message.error).toHaveBeenCalledTimes(1)
  })

  it('reports failed targets even if other items succeeded', () => {
    const response = { data: { data: { count: 1, results: [
      { id: 1, ok: true },
      { id: 2, ok: false, message: 'Inventory 被任务引用' },
      { id: 3, ok: false, message: '无删除权限' },
    ] } } }
    expect(() => assertBatchDeleteSucceeded(response, [1, 2, 3]))
      .toThrow('#2: Inventory 被任务引用；#3: 无删除权限')
    expect(message.error).toHaveBeenCalledTimes(1)
  })

  it('shows a per-item failure once through the shared confirmation', async () => {
    const pending = openDeleteConfirm({
      onConfirm: () => assertBatchDeleteSucceeded({
        data: { data: { results: [{ id: 1, ok: false, message: 'Inventory 被任务引用' }] } },
      }, [1]),
    })
    await triggerOk()
    await expect(pending).resolves.toBe(false)
    expect(message.error).toHaveBeenCalledTimes(1)
    expect(message.error).toHaveBeenCalledWith('#1: Inventory 被任务引用')
  })
})

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
