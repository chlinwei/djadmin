import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  batchDeleteApplication: vi.fn(() => Promise.resolve({ data: { data: null } })),
  checkApplicationServiceBaseline: vi.fn(() => Promise.resolve({ data: { data: { baseline_pass_rate: 100 } } })),
  checkApplicationDeploymentBaseline: vi.fn(() => Promise.resolve({ data: { data: {} } })),
  controlApplicationDeployment: vi.fn((_id, action) => Promise.resolve({
    data: { data: {
      output: action === 'status' ? 'active' : '',
      runtime_status: action === 'status' ? 'running' : 'unknown',
    } },
  })),
  deleteBusinessSystem: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteApplicationService: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteClusterProfile: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteApplicationDeployment: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteApplicationDeploymentTemplate: vi.fn(() => Promise.resolve({ data: { data: null } })),
  getApplicationDeploymentBaselineHistory: vi.fn(() => Promise.resolve({ data: { data: [] } })),
  getBusinessSystemList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 1, name: '订单系统', code: 'order-system', deployment_count: 2 }], count: 1 } },
  })),
  getApplicationServiceList: vi.fn(() => Promise.resolve({
    data: { data: { results: [], count: 0 } },
  })),
  getClusterProfileList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 9, name: 'Redis 集群', code: 'redis', profile_type: 'builtin', cluster_type: 'redis', enabled: true }], count: 1 } },
  })),
  getApplicationDeploymentList: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 1, instance_name: 'tomcat-prod', application_name: 'Tomcat', control_type: 'systemd' },
      { id: 2, instance_name: 'ha-prod', application_name: 'HA App', control_type: 'external_ha' },
    ], count: 2 } },
  })),
  getApplicationDeploymentTemplateList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 2, name: 'tomcat-systemd', application_name: 'Tomcat', enabled: true }], count: 1 } },
  })),
  getApplicationList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 1, name: 'Tomcat', versions: [], enabled: true }], count: 1 } },
  })),
}))

vi.mock('@/store', () => ({
  default: { state: { user: { timezone: 'Asia/Shanghai' } } },
}))

vi.mock('@/util/timezone', () => ({
  formatTimeWithTimezone: vi.fn((value) => value),
}))

vi.mock('@/util/deleteConfirm', () => ({
  openDeleteConfirm: vi.fn(),
}))

import * as applicationApi from '@/api/assets/application'
import ApplicationWorkspace from './ApplicationWorkspace.vue'

function mountWorkspace(props = {}) {
  return mount(ApplicationWorkspace, {
    props,
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        Dialog: true,
        VersionDialog: true,
        TemplateDialog: true,
        DeploymentDialog: true,
        ApplicationServiceDialog: true,
        ClusterProfileDialog: true,
        FontAwesomeIcon: true,
      },
      directives: {
        permission: () => {},
      },
    },
  })
}

async function clickTab(wrapper, label) {
  const tab = wrapper.findAll('.ant-tabs-tab').find((item) => item.text().includes(label))
  expect(tab).toBeTruthy()
  await tab.trigger('click')
  await flushPromises()
}

describe('ApplicationWorkspace tab switching', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    applicationApi.getApplicationDeploymentList.mockResolvedValue({
      data: { data: { results: [
        { id: 1, instance_name: 'tomcat-prod', application_name: 'Tomcat', control_type: 'systemd' },
        {
          id: 2, instance_name: 'ha-prod', application_name: 'HA App', control_type: 'external_ha',
          enabled: false, runtime_status: 'error', runtime_status_output: 'Agent gRPC 通道未连接',
        },
      ], count: 2 } },
    })
    document.body.innerHTML = ''
  })

  it('switches tables and create action without component update errors', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    expect(wrapper.text()).toContain('新增应用')
    expect(applicationApi.getApplicationList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '逻辑服务')
    expect(wrapper.text()).toContain('新增逻辑服务')
    expect(applicationApi.getApplicationServiceList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '集群模型')
    expect(wrapper.text()).toContain('新增自定义集群')
    expect(applicationApi.getClusterProfileList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '部署模板')
    expect(wrapper.text()).toContain('新增模板')
    expect(wrapper.text()).toContain('tomcat-systemd')
    expect(applicationApi.getApplicationDeploymentTemplateList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '部署实例')
    expect(wrapper.text()).toContain('登记实例')
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(2)

    await clickTab(wrapper, '应用定义')
    await clickTab(wrapper, '部署实例')
    expect(wrapper.text()).toContain('登记实例')
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(4)

    wrapper.unmount()
  })

  it('ignores a stale application response after switching to deployments', async () => {
    let resolveApplicationList
    applicationApi.getApplicationList.mockImplementationOnce(() => new Promise((resolve) => {
      resolveApplicationList = resolve
    }))
    const currentDeploymentResponse = {
      data: { data: { results: [{ id: 1, instance_name: 'current-deployment', application_name: 'Tomcat' }], count: 1 } },
    }
    applicationApi.getApplicationDeploymentList.mockResolvedValue(currentDeploymentResponse)

    const wrapper = mountWorkspace()
    await Promise.resolve()
    await clickTab(wrapper, '部署实例')
    expect(wrapper.text()).toContain('current-deployment')

    resolveApplicationList({
      data: { data: { results: [{ id: 1, name: 'stale-application', versions: [], enabled: true }], count: 1 } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('current-deployment')
    expect(wrapper.text()).not.toContain('stale-application')
    wrapper.unmount()
  })

  it('hides start and stop controls for external HA deployments', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()
    await clickTab(wrapper, '部署实例')

    expect(wrapper.findAll('[data-control-action="start"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-control-action="stop"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-control-action="status"]')).toHaveLength(0)
    expect(wrapper.findAll('[data-control-action="baseline"]')).toHaveLength(2)

    wrapper.unmount()
  })

  it('creates a cluster directly from a built-in profile', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()
    await clickTab(wrapper, '集群模型')

    await wrapper.find('[data-create-cluster-profile]').trigger('click')
    const serviceDialog = wrapper.findComponent({ name: 'ApplicationServiceDialog' })
    expect(serviceDialog.props('open')).toBe(true)
    expect(serviceDialog.props('clusterProfileId')).toBe(9)

    serviceDialog.vm.$emit('saved')
    await flushPromises()
    expect(wrapper.text()).toContain('模型：Redis 集群')
    expect(applicationApi.getApplicationServiceList).toHaveBeenLastCalledWith(expect.objectContaining({
      cluster_profile: 9,
    }))

    wrapper.unmount()
  })

  it('exposes the runtime check failure reason', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()
    await clickTab(wrapper, '部署实例')

    expect(wrapper.find('.runtime-error-detail').exists()).toBe(true)
    const errorTooltip = wrapper.findAllComponents({ name: 'ATooltip' })
      .find((tooltip) => tooltip.props('title') === 'Agent gRPC 通道未连接')
    expect(errorTooltip).toBeTruthy()

    wrapper.unmount()
  })

  it('rechecks runtime status after a successful start', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()
    await clickTab(wrapper, '部署实例')
    applicationApi.controlApplicationDeployment.mockClear()
    applicationApi.getApplicationDeploymentList.mockClear()

    await wrapper.find('[data-control-action="start"]').trigger('click')
    await flushPromises()
    const confirmButton = document.body.querySelector('.ant-modal-confirm-btns .ant-btn-primary')
    expect(confirmButton).toBeTruthy()
    confirmButton.click()
    await flushPromises()

    expect(applicationApi.controlApplicationDeployment.mock.calls).toEqual([
      [1, 'start'],
      [1, 'status'],
    ])
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('checks visible runtime states on manual refresh and schedules ten-second polling', async () => {
    const intervalSpy = vi.spyOn(globalThis, 'setInterval')
    const wrapper = mountWorkspace()
    await flushPromises()
    await clickTab(wrapper, '部署实例')

    const pollingCall = intervalSpy.mock.calls.find((call) => call[1] === 10000)
    expect(pollingCall).toBeTruthy()
    applicationApi.controlApplicationDeployment.mockClear()
    applicationApi.getApplicationDeploymentList.mockClear()
    pollingCall[0]()
    await flushPromises()
    expect(applicationApi.controlApplicationDeployment.mock.calls).toEqual([
      [1, 'status', { suppressBusinessErrorMessage: true }],
      [2, 'status', { suppressBusinessErrorMessage: true }],
    ])
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(1)

    applicationApi.controlApplicationDeployment.mockClear()
    applicationApi.getApplicationDeploymentList.mockClear()
    const refreshButton = wrapper.findAll('button').find((button) => button.text().includes('刷新'))
    expect(refreshButton).toBeTruthy()
    await refreshButton.trigger('click')
    await flushPromises()

    expect(applicationApi.controlApplicationDeployment.mock.calls).toEqual([
      [1, 'status', { suppressBusinessErrorMessage: true }],
      [2, 'status', { suppressBusinessErrorMessage: true }],
    ])
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    intervalSpy.mockRestore()
  })

  it('sends manual status requests while a background poll is still pending', async () => {
    applicationApi.controlApplicationDeployment.mockImplementation(() => new Promise(() => {}))
    const wrapper = mountWorkspace()
    await flushPromises()
    await clickTab(wrapper, '部署实例')
    expect(applicationApi.controlApplicationDeployment).toHaveBeenCalledTimes(2)

    applicationApi.controlApplicationDeployment.mockClear()
    const refreshButton = wrapper.findAll('button').find((button) => button.text().includes('刷新'))
    expect(refreshButton.attributes('disabled')).toBeUndefined()
    await refreshButton.trigger('click')
    await Promise.resolve()

    expect(applicationApi.controlApplicationDeployment.mock.calls).toEqual([
      [1, 'status', { suppressBusinessErrorMessage: true }],
      [2, 'status', { suppressBusinessErrorMessage: true }],
    ])
    wrapper.unmount()
  })

  it('opens deployments with exact business-system and environment filters', async () => {
    const wrapper = mountWorkspace({
      serviceScope: { businessSystemId: 7, environment: 'testing' },
    })
    await flushPromises()
    await wrapper.setProps({
      serviceScope: { businessSystemId: 8, environment: 'production' },
    })
    await flushPromises()

    expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toContain('部署实例')
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledWith(expect.objectContaining({
      application_service__business_system: 8,
      application_service__environment: 'production',
    }))
    wrapper.unmount()
  })
})
