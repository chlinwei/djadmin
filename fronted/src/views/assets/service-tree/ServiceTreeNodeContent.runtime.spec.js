import { flushPromises, shallowMount } from '@vue/test-utils'
import Antd, { message } from 'ant-design-vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const lifecycle = vi.hoisted(() => ({ start: null, stop: null }))
vi.mock('@/util/keepAliveRefresh', () => ({
  useKeepAliveRefreshLifecycle: (start, stop) => { lifecycle.start = start; lifecycle.stop = stop },
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ resolve: vi.fn() }) }))
vi.mock('@/store', () => ({ default: { state: { user: {}, perms: ['assets:applications:update'], role_codes: ['operator'] } } }))
vi.mock('@/api/assets/host', () => ({ getHostById: vi.fn() }))
vi.mock('@/api/assets/application', () => ({
  getApplicationService: vi.fn(), getApplicationServiceLogConfig: vi.fn(),
  getApplicationDeployment: vi.fn(), getApplicationDeploymentList: vi.fn(),
  getApplicationServiceList: vi.fn(), getBusinessEnvironmentList: vi.fn(),
  getBusinessSystem: vi.fn(), getBusinessSystemList: vi.fn(), getProjectList: vi.fn(),
  refreshApplicationDeploymentRuntimeStatus: vi.fn(),
}))

import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'
import store from '@/store'
import {
  getApplicationService, getApplicationServiceLogConfig,
  getApplicationDeployment, getApplicationDeploymentList, refreshApplicationDeploymentRuntimeStatus,
  getBusinessSystem, getBusinessSystemList, getProjectList, getApplicationServiceList,
} from '@/api/assets/application'

const response = (data) => ({ data: { data } })
const scope = (id) => ({ nodeType: 'service', applicationServiceId: id, nodeTitle: `service-${id}` })
const deployment = (id, status = 'stopped') => ({
  id, instance_name: `instance-${id}`, runtime_status: status,
  runtime_status_output: `output-${status}`, last_status_check_time: null,
})
const list = (items) => response({ results: items, totalPages: 1 })
const checked = (items) => response({ results: items })
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
    store.state.perms = ['assets:applications:update']
    store.state.role_codes = ['operator']
    refreshApplicationDeploymentRuntimeStatus.mockImplementation((ids) => Promise.resolve(checked(ids.map((id) => deployment(id)))))
    vi.spyOn(message, 'success').mockImplementation(() => {})
    vi.spyOn(message, 'error').mockImplementation(() => {})
  })
  afterEach(() => {
    wrapper?.unmount()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })
  function mountNode(nodeScope = scope(1)) {
    wrapper = shallowMount(ServiceTreeNodeContent, {
      props: { scope: nodeScope },
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

  it.each([
    { nodeType: 'all', projectIds: [1], environmentIds: [72] },
    { nodeType: 'project', projectId: 1 },
    { nodeType: 'businessSystem', businessSystemId: 7 },
    { nodeType: 'environment', businessSystemId: 7, environment: 72 },
    { nodeType: 'service', applicationServiceId: 1 },
    { nodeType: 'deployment', deploymentId: 10 },
  ])('checks only the current visible instances when entering any node %j', async (nodeScope) => {
    getProjectList.mockResolvedValue(list([{ id: 1, business_systems: [7] }]))
    getBusinessSystemList.mockResolvedValue(list([{ id: 7, project: 1 }]))
    getBusinessSystem.mockResolvedValue(response({ id: 7, project: 1 }))
    getApplicationServiceList.mockResolvedValue(list([{ id: 1, business_system: 7, environment: 72 }]))
    getApplicationDeploymentList.mockResolvedValue(list([
      { ...deployment(10, 'unknown'), application_service_ids: [1], enabled: true },
      { ...deployment(20, 'unknown'), application_service_ids: [1], enabled: false },
    ]))
    getApplicationDeployment.mockResolvedValue(response({ ...deployment(10, 'unknown'), enabled: true }))
    const check = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValueOnce(check.promise)
    mountNode(nodeScope)
    await flushPromises()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.runtimeRefreshing).toBe(true)
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledWith([10], 150000, { suppressBusinessErrorMessage: true })
    check.resolve(checked([deployment(10, 'running')]))
    await flushPromises()
    expect(wrapper.vm.runtimeRefreshing).toBe(false)
    if (nodeScope.nodeType === 'deployment') {
      expect(wrapper.vm.detail.runtime_status).toBe('running')
      expect(getApplicationDeployment).toHaveBeenCalledTimes(1)
    } else {
      expect(wrapper.vm.descendants.find((item) => item.id === 10).runtime_status).toBe('running')
      expect(wrapper.vm.descendants.find((item) => item.id === 20).runtime_status).toBe('unknown')
    }
  })

  it('reuses overlapping instance checks across root and service scopes', async () => {
    getProjectList.mockResolvedValue(list([]))
    getBusinessSystemList.mockResolvedValue(list([{ id: 7 }]))
    getApplicationServiceList.mockResolvedValue(list([{ id: 1, business_system: 7 }, { id: 2, business_system: 7 }]))
    getApplicationDeploymentList.mockImplementation(({ application_service }) => Promise.resolve(list(
      application_service ? [deployment(10)] : [
        { ...deployment(10), application_service_ids: [1, 2] },
        { ...deployment(20), application_service_ids: [2] },
      ],
    )))
    const check = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValueOnce(check.promise)
    mountNode({ nodeType: 'all' })
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledWith([10, 20], 150000, { suppressBusinessErrorMessage: true })
    await wrapper.setProps({ scope: scope(1) })
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(1)
    check.resolve(checked([deployment(10, 'running'), deployment(20, 'running')]))
    await flushPromises()
    expect(wrapper.vm.rows.map((item) => item.id)).toEqual([10])
    expect(wrapper.vm.rows[0].runtime_status).toBe('running')
    expect(wrapper.vm.summaryStatus.label).toBe('运行正常')
  })

  it('checks every instance in a large scope in sequential URL batches', async () => {
    const deployments = Array.from({ length: 205 }, (_, index) => deployment(index + 1, 'unknown'))
    getApplicationDeploymentList.mockResolvedValue(list(deployments))
    refreshApplicationDeploymentRuntimeStatus.mockImplementation((ids) => Promise.resolve(checked(ids.map((id) => deployment(id, 'running')))))
    mountNode()
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus.mock.calls.map(([ids]) => ids.length)).toEqual([...Array(25).fill(8), 5])
    expect(refreshApplicationDeploymentRuntimeStatus.mock.calls.flatMap(([ids]) => ids)).toEqual(deployments.map((item) => item.id))
    expect(wrapper.vm.runningCount).toBe(205)
    expect(wrapper.vm.descendants.every((item) => item.runtime_status === 'running')).toBe(true)
  })

  it('merges a finished wave before the next wave and preserves it when that wave times out', async () => {
    const deployments = Array.from({ length: 17 }, (_, index) => deployment(index + 1, 'unknown'))
    getApplicationDeploymentList.mockResolvedValue(list(deployments))
    const first = deferred(), second = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    mountNode()
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(1)
    first.resolve(checked(deployments.slice(0, 8).map((item) => deployment(item.id, 'running'))))
    await flushPromises()
    expect(wrapper.vm.runningCount).toBe(8)
    expect(wrapper.vm.descendants[8].runtime_status).toBe('unknown')
    expect(wrapper.vm.runtimeRefreshing).toBe(true)
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(2)
    second.reject(new Error('timeout of 150000ms exceeded'))
    await flushPromises()
    expect(wrapper.vm.runningCount).toBe(8)
    expect(wrapper.vm.runtimeRefreshing).toBe(false)
    expect(wrapper.vm.runtimeRefreshError).toContain('timeout')
    expect(message.success).not.toHaveBeenCalled()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(2)
  })

  it('checks a newly selected instance without waiting for unrelated queued waves and stops the old queue', async () => {
    const deployments = Array.from({ length: 17 }, (_, index) => deployment(index + 1, 'unknown'))
    getApplicationDeploymentList.mockImplementation(({ application_service }) => Promise.resolve(list(
      application_service === 1 ? deployments : [deployment(9, 'unknown')],
    )))
    const first = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValueOnce(first.promise)
    mountNode()
    await flushPromises()
    await wrapper.setProps({ scope: scope(2) })
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus.mock.calls.map(([ids]) => ids)).toEqual([
      [1, 2, 3, 4, 5, 6, 7, 8], [9],
    ])
    expect(wrapper.vm.runtimeRefreshing).toBe(false)
    expect(wrapper.vm.rows[0].id).toBe(9)
    first.resolve(checked(deployments.slice(0, 8).map((item) => deployment(item.id, 'running'))))
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(2)
    expect(wrapper.vm.rows[0].id).toBe(9)
  })

  it('does not send queued waves after the page becomes inactive', async () => {
    const deployments = Array.from({ length: 17 }, (_, index) => deployment(index + 1, 'unknown'))
    getApplicationDeploymentList.mockResolvedValue(list(deployments))
    const first = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValueOnce(first.promise)
    mountNode()
    await flushPromises()
    lifecycle.stop()
    first.resolve(checked(deployments.slice(0, 8).map((item) => deployment(item.id, 'running'))))
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.runningCount).toBe(0)
    expect(wrapper.vm.runtimeRefreshing).toBe(false)
  })

  it('merges per-instance failures and makes the incomplete refresh explicit', async () => {
    refreshApplicationDeploymentRuntimeStatus.mockResolvedValueOnce(checked([{
      ...deployment(10, 'error'), error: 'Agent 未连接',
    }]))
    mountNode()
    await flushPromises()
    expect(wrapper.vm.rows[0].runtime_status).toBe('error')
    expect(wrapper.vm.runtimeRefreshError).toContain('Agent 未连接')
    expect(message.success).not.toHaveBeenCalled()
  })

  it('does not dispatch a runtime check when the user can only view the service tree', async () => {
    store.state.perms = ['assets:applications:view']
    mountNode()
    await flushPromises()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.rows[0].runtime_status).toBe('stopped')
    expect(refreshApplicationDeploymentRuntimeStatus).not.toHaveBeenCalled()
  })

  it('renders cached rows and ends page loading before a slow Agent check completes', async () => {
    const check = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValue(check.promise)
    mountNode()
    await flushPromises()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.entity.id).toBe(1)
    expect(wrapper.vm.rows[0].runtime_status).toBe('stopped')
    expect(wrapper.text()).toContain('状态检查中')
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledWith([10], 150000, { suppressBusinessErrorMessage: true })
    check.resolve(checked([deployment(10)]))
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
    expect(wrapper.vm.columns.find((item) => item.key === 'deployment_action')).toBeUndefined()
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
      cluster_type: clusterType,
    }))
    mountNode()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'deployment', deploymentId: 10 } })
    await flushPromises()
    expect(wrapper.vm.metrics.some((item) => item.label === '主备状态')).toBe(clusterType === 'ha')
    expect(wrapper.vm.metrics.some((item) => item.label === '运行状态')).toBe(true)
    expect(wrapper.vm.metrics.some((item) => item.label === '监听端口')).toBe(false)
    expect(wrapper.text()).not.toContain('监听端口')
    expect(wrapper.find('.metric-band').classes()).toContain('metric-band--instance')
    expect(wrapper.findAll('.metric-item')).toHaveLength(clusterType === 'ha' ? 2 : 1)
    if (clusterType === 'ha') {
      expect(wrapper.vm.metrics.find((item) => item.label === '主备状态').value).toBe('未知')
    } else {
      expect(wrapper.text()).not.toContain('主备状态')
    }
  })

  it('keeps template port descriptions on the logical-service page', async () => {
    getApplicationService.mockResolvedValue(response({
      id: 1, member_instances: [], ports: [{ name: 'main', protocol: 'tcp', port: 8080 }],
    }))
    mountNode()
    await flushPromises()
    expect(wrapper.text()).toContain('监听端口')
    expect(wrapper.text()).toContain('main · TCP 8080')
    expect(wrapper.find('.metric-band').classes()).not.toContain('metric-band--instance')
  })

  it('deduplicates slow polling and updates only status fields without reloading details', async () => {
    const check = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValue(check.promise)
    mountNode()
    await flushPromises()
    lifecycle.start()
    vi.advanceTimersByTime(45000)
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(getApplicationDeploymentList).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.summaryStatus).toEqual({ label: '1 个异常实例', color: 'error' })
    expect(wrapper.vm.abnormalRows.map((item) => item.id)).toEqual([10])
    wrapper.vm.abnormalDetailsOpen = true
    check.resolve(checked([{ ...deployment(10, 'running'), instance_name: 'changed-metadata' }]))
    await flushPromises()
    expect(wrapper.vm.rows[0]).toMatchObject({
      instance_name: 'instance-10', runtime_status: 'running', runtime_status_output: 'output-running',
    })
    expect(wrapper.vm.descendants[0].runtime_status).toBe('running')
    expect(wrapper.vm.runningCount).toBe(1)
    expect(wrapper.vm.summaryStatus).toEqual({ label: '运行正常', color: 'success' })
    expect(wrapper.vm.abnormalRows).toEqual([])
    expect(wrapper.vm.abnormalCount).toBe(0)
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.runtimeRefreshing).toBe(false)
    expect(getApplicationService).toHaveBeenCalledTimes(1)
    expect(getApplicationServiceLogConfig).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(15000)
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(2)
    expect(getApplicationService).toHaveBeenCalledTimes(1)
  })

  it('switches services without waiting and prevents old results from altering the new node', async () => {
    const first = deferred(), second = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockImplementation(([id]) => id === 10 ? first.promise : second.promise)
    mountNode()
    await flushPromises()
    await wrapper.setProps({ scope: scope(2) })
    await flushPromises()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.rows[0].id).toBe(20)
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(2)
    first.resolve(checked([deployment(10, 'running')]))
    await flushPromises()
    expect(wrapper.vm.rows[0].id).toBe(20)
    expect(wrapper.vm.runtimeRefreshing).toBe(true)
    expect(getApplicationDeploymentList).toHaveBeenCalledTimes(2)
    second.resolve(checked([deployment(20)]))
    await flushPromises()
  })

  it('finishes manual page refresh without waiting and reuses the pending status request', async () => {
    const check = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockReturnValue(check.promise)
    mountNode()
    await flushPromises()
    await wrapper.vm.refresh()
    expect(wrapper.vm.loading).toBe(false)
    expect(wrapper.vm.runtimeRefreshing).toBe(true)
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(1)
    check.resolve(checked([deployment(10, 'running')]))
    await flushPromises()
    expect(wrapper.vm.rows[0].runtime_status).toBe('running')
    expect(message.success).toHaveBeenCalledTimes(1)
  })

  it('reuses an in-flight service check after navigating away and back', async () => {
    const first = deferred(), second = deferred()
    refreshApplicationDeploymentRuntimeStatus.mockImplementation(([id]) => id === 10 ? first.promise : second.promise)
    mountNode()
    await flushPromises()
    await wrapper.setProps({ scope: scope(2) })
    await flushPromises()
    await wrapper.setProps({ scope: scope(1) })
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(2)
    first.resolve(checked([deployment(10, 'running')]))
    await flushPromises()
    expect(wrapper.vm.rows[0].runtime_status).toBe('running')
    second.resolve(checked([deployment(20)]))
    await flushPromises()
    expect(wrapper.vm.rows[0].id).toBe(10)
  })

  it('rejects incomplete or invalid status snapshots instead of reporting success', async () => {
    refreshApplicationDeploymentRuntimeStatus.mockResolvedValueOnce(checked([]))
    mountNode()
    await flushPromises()
    expect(wrapper.vm.runtimeRefreshError).toContain('实例明细不完整')
    expect(wrapper.vm.rows[0].runtime_status).toBe('stopped')
    expect(message.success).not.toHaveBeenCalled()
    refreshApplicationDeploymentRuntimeStatus.mockResolvedValueOnce(checked([{ id: 20, runtime_status: 'invalid' }]))
    await wrapper.setProps({ scope: scope(2) })
    await flushPromises()
    expect(wrapper.vm.runtimeRefreshError).toContain('无效运行状态')
  })

  it('preserves cached data and displays background errors without blocking the page', async () => {
    refreshApplicationDeploymentRuntimeStatus.mockRejectedValue(new Error('Agent timeout'))
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
    refreshApplicationDeploymentRuntimeStatus.mockReturnValueOnce(check.promise)
    mountNode()
    await flushPromises()
    lifecycle.start()
    lifecycle.stop()
    vi.advanceTimersByTime(30000)
    check.resolve(checked([deployment(10, 'running')]))
    await flushPromises()
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(getApplicationDeploymentList).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.rows[0].runtime_status).toBe('stopped')
    lifecycle.start()
    await flushPromises()
    expect(getApplicationService).toHaveBeenCalledTimes(2)
    expect(refreshApplicationDeploymentRuntimeStatus).toHaveBeenCalledTimes(2)
  })
})
