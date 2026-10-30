import { flushPromises, mount } from '@vue/test-utils'
import { h, KeepAlive, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import BaselinePage from './index.vue'
import requestUtil from '@/util/request'
import { message } from 'ant-design-vue'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' }, perms: ['baseline:view', 'baseline:scan'], role_codes: ['operator'] } } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/util/request', () => ({ default: { get: vi.fn() } }))
vi.mock('ant-design-vue', () => ({ message: { error: vi.fn() } }))

let wrapper
const response = (results) => ({ data: { data: { results, count: results.length } } })
const scanCalls = () => requestUtil.get.mock.calls.filter(([url]) => url === 'sys/security/scans/')

async function loadPage(status = 'running') {
  vi.useFakeTimers()
  vi.clearAllMocks()
  let state = status
  requestUtil.get.mockImplementation((url) => Promise.resolve(response(
    url === 'sys/security/scans/' ? [{ id: 28, status: state }] : [],
  )))
  const visible = ref(true)
  wrapper = mount({
    setup: () => () => h(KeepAlive, null, {
      default: () => visible.value ? h(BaselinePage) : h('div'),
    }),
  }, {
    global: {
      stubs: Object.fromEntries([
        'a-tabs', 'a-tab-pane', 'a-badge', 'a-menu', 'a-menu-item', 'a-sub-menu',
        'a-button', 'a-tooltip', 'a-tag', 'a-space', 'a-table', 'a-popconfirm',
        'a-input', 'a-form-item', 'a-switch', 'a-textarea', 'a-form', 'a-modal',
        'a-select', 'a-alert', 'FontAwesomeIcon',
      ].map((name) => [name, true])),
      directives: { permission: {} },
    },
  })
  const page = wrapper.findComponent(BaselinePage)
  page.vm.activeTab = 'scans'
  await flushPromises()
  return { page, visible, setStatus: (value) => { state = value } }
}

afterEach(() => {
  wrapper?.unmount()
  vi.useRealTimers()
})

describe('扫描记录自动刷新', () => {
  it.each(['pending', 'running'])('%s 自动更新终态并停止轮询', async (status) => {
    const { page, setStatus } = await loadPage(status)
    const initial = scanCalls().length
    setStatus('failed')
    await vi.advanceTimersByTimeAsync(2999)
    expect(scanCalls()).toHaveLength(initial)
    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()
    expect(scanCalls()).toHaveLength(initial + 1)
    expect(page.vm.scans[0].status).toBe('failed')
    await vi.advanceTimersByTimeAsync(9000)
    expect(scanCalls()).toHaveLength(initial + 1)
  })

  it('切走页签停止，返回读取最新状态', async () => {
    const { page, setStatus } = await loadPage()
    page.vm.activeTab = 'baselines'
    await flushPromises()
    const initial = scanCalls().length
    await vi.advanceTimersByTimeAsync(6000)
    expect(scanCalls()).toHaveLength(initial)
    setStatus('success')
    page.vm.activeTab = 'scans'
    await flushPromises()
    expect(scanCalls()).toHaveLength(initial + 1)
    expect(page.vm.scans[0].status).toBe('success')
  })

  it('缓存失活停止，返回立即刷新，卸载后不再请求', async () => {
    const { page, visible, setStatus } = await loadPage()
    visible.value = false
    await flushPromises()
    const initial = scanCalls().length
    await vi.advanceTimersByTimeAsync(6000)
    expect(scanCalls()).toHaveLength(initial)
    setStatus('success')
    visible.value = true
    await flushPromises()
    expect(page.vm.scans[0].status).toBe('success')
    expect(scanCalls()).toHaveLength(initial + 1)
    wrapper.unmount()
    wrapper = null
    await vi.advanceTimersByTimeAsync(6000)
    expect(scanCalls()).toHaveLength(initial + 1)
  })

  it('旧请求不覆盖翻页结果，轮询保留当前分页', async () => {
    const { page } = await loadPage()
    let resolveOld
    requestUtil.get.mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    const oldRequest = page.vm.loadScans()
    page.vm.handleScanTableChange({ current: 2, pageSize: 20 })
    await flushPromises()
    resolveOld(response([{ id: 99, status: 'success' }]))
    await oldRequest
    expect(page.vm.scans[0].id).toBe(28)
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(scanCalls().at(-1)[1]).toEqual({ type: 'baseline', page: 2, page_size: 20 })
  })

  it('刷新失败明确提示，不将扫描误置为完成且可继续重试', async () => {
    const { page } = await loadPage()
    requestUtil.get.mockRejectedValueOnce(new Error('网络中断'))
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(message.error).toHaveBeenCalledWith('网络中断')
    expect(page.vm.scans[0].status).toBe('running')
    const failedCalls = scanCalls().length
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(scanCalls()).toHaveLength(failedCalls + 1)
  })
})
