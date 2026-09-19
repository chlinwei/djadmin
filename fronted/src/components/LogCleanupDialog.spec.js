import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { message } from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

// 数据流清理弹窗（2026-09-19）：与「日志查询」原来那套清理入口**合并成一个**——
// 那个面板里的按钮已删除，清理统一从数据流列表进（日志中心 → 存储水位，每条流一个）。
// 组件要同时支持两种作用域：按逻辑服务（带 tier 收窄到某条流）/ 按数据流名（未识别流）。
vi.mock('@/api/monitor', () => ({
  cleanupLogDataStream: vi.fn(() => Promise.resolve({ data: { data: { matched: true } } })),
  cleanupLogDataStreamByStream: vi.fn(() => Promise.resolve({ data: { data: { matched: true } } })),
}))

import LogCleanupDialog from './LogCleanupDialog.vue'
import { cleanupLogDataStream, cleanupLogDataStreamByStream } from '@/api/monitor'

function mountDialog(scope) {
  return mount(LogCleanupDialog, {
    props: { open: true, scope },
    attachTo: document.body,
    global: { plugins: [Antd] },
  })
}

describe('LogCleanupDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('opens with the keep-recent-7-days default and does not call the API before submit', async () => {
    const wrapper = mountDialog({ kind: 'service', serviceId: 15, tier: 'hot', label: 'autoadmin-x-hot' })
    await flushPromises()

    expect(wrapper.vm.mode).toBe('days')
    expect(wrapper.vm.amount).toBe(7)
    expect(cleanupLogDataStream).not.toHaveBeenCalled()
    // 清理对象必须写在弹窗里（否则用户不知道清的是哪条流）。
    expect(document.body.textContent).toContain('autoadmin-x-hot')
    wrapper.unmount()
  })

  it('submits a service-scoped cleanup with the chosen window', async () => {
    const wrapper = mountDialog({ kind: 'service', serviceId: 15, tier: 'wuhan-test', label: 'x' })
    wrapper.vm.mode = 'days'
    wrapper.vm.amount = 30
    await wrapper.vm.submit()
    await flushPromises()

    expect(cleanupLogDataStream).toHaveBeenCalledWith({ service_id: 15, mode: 'days', amount: 30, tier: 'wuhan-test' })
    expect(cleanupLogDataStreamByStream).not.toHaveBeenCalled()
    expect(wrapper.emitted('cleaned')).toBeTruthy()
    wrapper.unmount()
  })

  it('submits a stream-scoped cleanup for an unrecognized stream', async () => {
    const wrapper = mountDialog({ kind: 'stream', stream: 'autoadmin-nkg-tib-prod-old-std', label: 'y（未识别流）' })
    wrapper.vm.mode = 'all'
    await wrapper.vm.submit()
    await flushPromises()

    expect(cleanupLogDataStreamByStream).toHaveBeenCalledWith({ stream: 'autoadmin-nkg-tib-prod-old-std', mode: 'all', amount: 0 })
    expect(cleanupLogDataStream).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 时间窗必须填数：留空提交等于"清 0 天前的数据"（几乎等于全清），拦在发请求之前。
  it('refuses an empty keep window instead of firing a request', async () => {
    const warningSpy = vi.spyOn(message, 'warning').mockImplementation(() => {})
    const wrapper = mountDialog({ kind: 'service', serviceId: 15, tier: 'hot', label: 'x' })
    wrapper.vm.mode = 'days'
    wrapper.vm.amount = 0
    await wrapper.vm.submit()
    await flushPromises()

    expect(cleanupLogDataStream).not.toHaveBeenCalled()
    expect(warningSpy).toHaveBeenCalled()
    warningSpy.mockRestore()
    wrapper.unmount()
  })

  // 没有作用域（调用方给错）时不许发请求：宁可什么都不做，也不能清错东西。
  it('does nothing when the scope is missing', async () => {
    const wrapper = mountDialog(null)
    await wrapper.vm.submit()
    await flushPromises()

    expect(cleanupLogDataStream).not.toHaveBeenCalled()
    expect(cleanupLogDataStreamByStream).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
