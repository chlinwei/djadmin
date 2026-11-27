import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import request from '@/util/request'
import Monitor from './index.vue'

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/util/keepAliveRefresh', () => ({ useKeepAliveRefreshLifecycle: vi.fn() }))
vi.mock('@/util/request', () => ({
  default: { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn(), fileUpload: vi.fn() },
  getMediaUrl: vi.fn(),
}))
vi.mock('@/directives/permission/permission', () => ({ checkPermission: () => false }))

const response = (data) => ({ data: { code: 200, data } })
const result = (exitCode) => response({ status: exitCode === 0 ? 'success' : 'failed', exit_code: exitCode })
const originalGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element) => originalGetComputedStyle(element)

let wrapper
let installStatus
let serviceStatus
function exporter() {
  return { id: 8, exporter_type: 'node_exporter', scrape_port: 9100, managed_enabled: true, install_status: installStatus }
}

beforeEach(() => {
  installStatus = 'success'
  serviceStatus = () => Promise.resolve(result(4))
  request.get.mockImplementation((url, params) => {
    if (url === 'monitor/targets/host-overview/') {
      return Promise.resolve(response({ count: 1, results: [{
        host_id: 7, host_name: 'host-129', host_ip: '10.25.66.129', host_agent_online: true,
        exporters: [exporter()],
        ...(params?.exporter_type ? { ...exporter(), managed: true } : {}),
      }] }))
    }
    if (url === 'monitor/targets/8/check-service-status/') return serviceStatus()
    if (url === 'monitor/targets/host-group-tree/') return Promise.resolve(response({ groups: [] }))
    return Promise.resolve(response({ results: [] }))
  })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

async function mountPage() {
  wrapper = mount(Monitor, {
    attachTo: document.body,
    global: { plugins: [Antd], stubs: { FontAwesomeIcon: true, RouterLink: true } },
  })
  wrapper.vm.activeTabKey = 'managed-targets'
  await flushPromises()
  return wrapper
}

function deferred() {
  let resolve
  const promise = new Promise((done) => { resolve = done })
  return { promise, resolve }
}

describe('Exporter 安装状态', () => {
  it('安装期间显示进度，完成后等待新查询并展示真实服务状态', async () => {
    await mountPage()
    expect(wrapper.text()).toContain('服务不存在')
    const callsBefore = request.get.mock.calls.filter(([url]) => url.includes('check-service-status')).length

    installStatus = 'pending'
    await wrapper.vm.loadOverviewHosts()
    await flushPromises()
    expect(wrapper.text()).toContain('安装中')
    expect(wrapper.text()).not.toContain('服务不存在')
    expect(request.get.mock.calls.filter(([url]) => url.includes('check-service-status'))).toHaveLength(callsBefore)
    expect(wrapper.vm.exporterStatusInfo({ ...exporter(), managed_enabled: false }).text).toBe('卸载中')
    wrapper.vm.exporterFilterType = 'node_exporter'
    await wrapper.vm.loadOverviewHosts()
    await flushPromises()
    expect(wrapper.text()).toContain('安装中')
    expect(wrapper.text()).not.toContain('服务不存在')

    installStatus = 'success'
    const check = deferred()
    serviceStatus = () => check.promise
    const loading = wrapper.vm.loadOverviewHosts()
    await flushPromises()
    expect(wrapper.text()).toContain('状态待确认')
    expect(wrapper.text()).not.toContain('运行中')
    check.resolve(result(0))
    await loading
    await flushPromises()
    expect(wrapper.text()).toContain('运行中')

    serviceStatus = () => Promise.resolve(result(4))
    await wrapper.vm.loadOverviewHosts()
    await flushPromises()
    expect(wrapper.text()).toContain('服务不存在')
  })

  it('忽略安装开始前晚返回的查询；新查询失败时不虚报运行中', async () => {
    const oldCheck = deferred()
    serviceStatus = () => oldCheck.promise
    await mountPage()
    installStatus = 'pending'
    await wrapper.vm.loadOverviewHosts()
    await flushPromises()
    expect(wrapper.text()).toContain('安装中')

    installStatus = 'success'
    serviceStatus = () => Promise.reject(new Error('offline'))
    await wrapper.vm.loadOverviewHosts()
    oldCheck.resolve(result(4))
    await flushPromises()
    expect(wrapper.text()).toContain('状态待确认')
    expect(wrapper.text()).not.toContain('服务不存在')
    expect(wrapper.text()).not.toContain('运行中')

    installStatus = 'failed'
    await wrapper.vm.loadOverviewHosts()
    await flushPromises()
    expect(wrapper.text()).toContain('安装失败')
  })

  it('安装期间手动查询的旧结果不能覆盖完成后的新结果', async () => {
    installStatus = 'pending'
    await mountPage()
    const oldCheck = deferred()
    serviceStatus = () => oldCheck.promise
    const manualCheck = wrapper.vm.handleCheckServiceStatus({ id: 8 })

    installStatus = 'success'
    serviceStatus = () => Promise.resolve(result(0))
    await wrapper.vm.loadOverviewHosts()
    oldCheck.resolve(result(4))
    await manualCheck
    await flushPromises()
    expect(wrapper.text()).toContain('运行中')
    expect(wrapper.text()).not.toContain('服务不存在')
  })
})
