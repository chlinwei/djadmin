import { flushPromises, shallowMount } from '@vue/test-utils'
import Antd, { message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const lifecycle = vi.hoisted(() => ({ start: null, stop: null }))
vi.mock('@/util/keepAliveRefresh', () => ({
  useKeepAliveRefreshLifecycle: (start, stop) => { lifecycle.start = start; lifecycle.stop = stop },
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ resolve: vi.fn() }) }))
vi.mock('@/store', () => ({ default: { state: { user: {} } } }))
vi.mock('@/api/assets/host', () => ({ getHostById: vi.fn() }))
vi.mock('@/api/assets/application', () => ({
  getApplicationService: vi.fn(), getApplicationServiceLogConfig: vi.fn(),
  getApplicationDeployment: vi.fn(), getApplicationDeploymentList: vi.fn(),
  getApplicationServiceList: vi.fn(), getBusinessEnvironmentList: vi.fn(),
  getBusinessSystem: vi.fn(), getBusinessSystemList: vi.fn(), getProjectList: vi.fn(),
  refreshApplicationServiceRuntimeStatus: vi.fn(),
}))

import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'
import {
  getApplicationService, getApplicationServiceLogConfig,
  getApplicationDeployment, getApplicationDeploymentList, refreshApplicationServiceRuntimeStatus,
} from '@/api/assets/application'

const response = (data) => ({ data: { data } })
const scope = (id) => ({ nodeType: 'service', applicationServiceId: id, nodeTitle: `service-${id}` })
const deployment = (id, status = 'stopped') => ({
  id, instance_name: `instance-${id}`, runtime_status: status,
  runtime_status_output: `output-${status}`, last_status_check_time: null,
})
const list = (items) => response({ results: items, totalPages: 1 })
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

describe('service-tree non-blocking runtime checks', () => {
  let wrapper
  beforeEach(() => {
    vi.resetAllMocks()
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    getApplicationService.mockImplementation((id) => Promise.resolve(response({ id, member_instances: [] })))
    getApplicationServiceLogConfig.mockResolvedValue(response({ logs: [] }))
    getApplicationDeploymentList.mockImplementation(({ application_service }) => Promise.resolve(list([deployment(application_service * 10)])))
    refreshApplicationServiceRuntimeStatus.mockResolvedValue(response({ summary: {} }))
    vi.spyOn(message, 'success').mockImplementation(() => {})
    vi.spyOn(message, 'error').mockImplementation(() => {})
  })
  afterEach(() => {
    wrapper?.unmount()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })
  function mountNode() {
    wrapper = shallowMount(ServiceTreeNodeContent, {
      props: { scope: scope(1) },
      global: {
        plugins: [Antd], directives: { permission: () => {} },
        renderStubDefaultSlot: true,
        stubs: {
          FontAwesomeIcon: true,
          ATooltip: { inheritAttrs: false, template: '<span><slot /></span>' },
        },
      },
    })
  }

  it('renders cached rows and ends page loading before a slow Agent check completes', async () => {
    const check = deferred()
    refreshApplicationServiceRuntimeStatus.mockReturnValue(check.promise)
    mountNode()
    await flushPromises()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.entity.id).toBe(1)
    expect(wrapper.vm.rows[0].runtime_status).toBe('stopped')
    expect(wrapper.text()).toContain('状态检查中')
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledWith(1, 120000, { suppressBusinessErrorMessage: true })
    check.resolve(response({ summary: {} }))
    await flushPromises()
  })

  it('shows the complete operating-system version from reactive host snapshots instead of deployment version', async () => {
    mountNode()
    await flushPromises()
    await wrapper.setProps({
      hostSnapshots: [{ id: 100, os_type: 'Red Hat Enterprise Linux', os_version: '8.10 (Ootpa)', system: { hostname: 'node-100' } }],
    })
    const column = wrapper.vm.columns.find((item) => item.key === 'host_os_version')
    expect(column.title).toBe('操作系统')
    expect(column.width).toBe(300)
    expect(column.customRender({ record: { host_id: '100', version: '1.0' } })).toBe('Red Hat Enterprise Linux 8.10 (Ootpa)')
    expect(column.customRender({ record: { host: 100 } })).toBe('Red Hat Enterprise Linux 8.10 (Ootpa)')
    expect(column.customRender({ record: { host: 999 } })).toBe('-')
    expect(wrapper.vm.columns.some((item) => item.key === 'version')).toBe(false)
    expect(wrapper.vm.columns.find((item) => item.key === 'deployment_action')).toMatchObject({ fixed: 'right', width: 90 })
    expect(wrapper.vm.tableWidth).toBe(1460)
    await wrapper.setProps({ hostSnapshots: [{ id: 100, os_version: null, system: { hostname: 'node-100' } }] })
    expect(column.customRender({ record: { host: 100 } })).toBe('-')
    await wrapper.setProps({ hostSnapshots: [{ id: 100, os_type: 'Ubuntu', os_version: '24.04 LTS' }] })
    expect(column.customRender({ record: { host: 100 } })).toBe('Ubuntu 24.04 LTS')
    await wrapper.setProps({ hostSnapshots: [{ id: 100, os_type: 'Red Hat Enterprise Linux', os_version: null }] })
    expect(column.customRender({ record: { host: 100 } })).toBe('Red Hat Enterprise Linux')
    await wrapper.setProps({ hostSnapshots: [{ id: 100, os_version: '8.10 (Ootpa)' }] })
    expect(column.customRender({ record: { host: 100 } })).toBe('8.10 (Ootpa)')
  })

  it.each([undefined, 'redis', 'ha'])('shows the deployment HA metric only for HA clusters (%s)', async (clusterType) => {
    getApplicationDeployment.mockResolvedValue(response({
      id: 10, instance_name: 'instance-10', runtime_status: 'running', ha_role: 'unknown',
      cluster_type: clusterType, ports: [],
    }))
    mountNode()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'deployment', deploymentId: 10 } })
    await flushPromises()
    expect(wrapper.vm.metrics.some((item) => item.label === '主备状态')).toBe(clusterType === 'ha')
    expect(wrapper.vm.metrics.some((item) => item.label === '运行状态')).toBe(true)
    expect(wrapper.vm.metrics.some((item) => item.label === '监听端口')).toBe(true)
    if (clusterType === 'ha') {
      expect(wrapper.vm.metrics.find((item) => item.label === '主备状态').value).toBe('未知')
    } else {
      expect(wrapper.text()).not.toContain('主备状态')
    }
  })

  it('deduplicates slow polling and updates only status fields without reloading details', async () => {
    const check = deferred()
    refreshApplicationServiceRuntimeStatus.mockReturnValue(check.promise)
    mountNode()
    await flushPromises()
    lifecycle.start()
    vi.advanceTimersByTime(45000)
    await flushPromises()
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(getApplicationDeploymentList).toHaveBeenCalledTimes(1)
    getApplicationDeploymentList.mockResolvedValue(list([{ ...deployment(10, 'running'), instance_name: 'changed-metadata' }]))
    check.resolve(response({ summary: { running: 1 } }))
    await flushPromises()
    expect(wrapper.vm.rows[0]).toMatchObject({
      instance_name: 'instance-10', runtime_status: 'running', runtime_status_output: 'output-running',
    })
    expect(wrapper.vm.descendants[0].runtime_status).toBe('running')
    expect(wrapper.vm.runningCount).toBe(1)
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.runtimeRefreshing).toBe(false)
    expect(getApplicationService).toHaveBeenCalledTimes(1)
    expect(getApplicationServiceLogConfig).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(15000)
    await flushPromises()
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledTimes(2)
    expect(getApplicationService).toHaveBeenCalledTimes(1)
  })

  it('switches services without waiting and prevents old results from altering the new node', async () => {
    const first = deferred(), second = deferred()
    refreshApplicationServiceRuntimeStatus.mockImplementation((id) => id === 1 ? first.promise : second.promise)
    mountNode()
    await flushPromises()
    await wrapper.setProps({ scope: scope(2) })
    await flushPromises()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.rows[0].id).toBe(20)
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledTimes(2)
    first.resolve(response({ summary: { running: 1 } }))
    await flushPromises()
    expect(wrapper.vm.rows[0].id).toBe(20)
    expect(wrapper.vm.runtimeRefreshing).toBe(true)
    expect(getApplicationDeploymentList).toHaveBeenCalledTimes(2)
    second.resolve(response({ summary: {} }))
    await flushPromises()
  })

  it('finishes manual page refresh without waiting and reuses the pending status request', async () => {
    const check = deferred()
    refreshApplicationServiceRuntimeStatus.mockReturnValue(check.promise)
    mountNode()
    await flushPromises()
    await wrapper.vm.refresh()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.runtimeRefreshing).toBe(true)
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledTimes(1)
    getApplicationDeploymentList.mockResolvedValue(list([deployment(10, 'running')]))
    check.resolve(response({ summary: { running: 1 } }))
    await flushPromises()
    expect(wrapper.vm.rows[0].runtime_status).toBe('running')
    expect(message.success).toHaveBeenCalledTimes(1)
  })

  it('reuses an in-flight service check after navigating away and back', async () => {
    const first = deferred(), second = deferred()
    refreshApplicationServiceRuntimeStatus.mockImplementation((id) => id === 1 ? first.promise : second.promise)
    mountNode()
    await flushPromises()
    await wrapper.setProps({ scope: scope(2) })
    await flushPromises()
    await wrapper.setProps({ scope: scope(1) })
    await flushPromises()
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledTimes(2)
    getApplicationDeploymentList.mockResolvedValue(list([deployment(10, 'running')]))
    first.resolve(response({ summary: {} }))
    await flushPromises()
    expect(wrapper.vm.rows[0].runtime_status).toBe('running')
    second.resolve(response({ summary: {} }))
    await flushPromises()
    expect(wrapper.vm.rows[0].id).toBe(10)
  })

  it('discards a late status-list response after switching nodes', async () => {
    const check = deferred(), statusList = deferred(), otherCheck = deferred()
    refreshApplicationServiceRuntimeStatus.mockReturnValueOnce(check.promise).mockReturnValue(otherCheck.promise)
    mountNode()
    await flushPromises()
    getApplicationDeploymentList.mockReturnValueOnce(statusList.promise)
    check.resolve(response({ summary: {} }))
    await flushPromises()
    await wrapper.setProps({ scope: scope(2) })
    await flushPromises()
    expect(wrapper.vm.rows[0].id).toBe(20)
    statusList.resolve(list([deployment(10, 'running')]))
    await flushPromises()
    expect(wrapper.vm.rows[0].id).toBe(20)
    expect(wrapper.vm.runtimeRefreshing).toBe(true)
    otherCheck.resolve(response({ summary: {} }))
    await flushPromises()
  })

  it('preserves cached data and displays background errors without blocking the page', async () => {
    refreshApplicationServiceRuntimeStatus.mockRejectedValue(new Error('Agent timeout'))
    mountNode()
    await flushPromises()
    expect(wrapper.vm.rows[0].runtime_status).toBe('stopped')
    expect(wrapper.vm.runtimeRefreshing).toBe(false)
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.runtimeRefreshError).toBe('Agent timeout')
    expect(wrapper.text()).toContain('状态刷新失败')
  })

  it('stops polling and discards inactive results, then resumes on activation', async () => {
    const check = deferred()
    refreshApplicationServiceRuntimeStatus.mockReturnValueOnce(check.promise)
    mountNode()
    await flushPromises()
    lifecycle.start()
    lifecycle.stop()
    vi.advanceTimersByTime(30000)
    check.resolve(response({ summary: {} }))
    await flushPromises()
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(getApplicationDeploymentList).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.rows[0].runtime_status).toBe('stopped')
    lifecycle.start()
    await flushPromises()
    expect(getApplicationService).toHaveBeenCalledTimes(2)
    expect(refreshApplicationServiceRuntimeStatus).toHaveBeenCalledTimes(2)
  })
})
