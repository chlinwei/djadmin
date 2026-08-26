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
  deleteBusinessEnvironment: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteApplicationService: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteClusterProfile: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteApplicationDeployment: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteApplicationDeploymentTemplate: vi.fn(() => Promise.resolve({ data: { data: null } })),
  getApplicationDeploymentBaselineHistory: vi.fn(() => Promise.resolve({ data: { data: [] } })),
  getBusinessSystemList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 1, name: '订单系统', code: 'order-system', deployment_count: 2 }], count: 1 } },
  })),
  getProjectList: vi.fn(() => Promise.resolve({
    data: { data: { results: [], count: 0 } },
  })),
  getBusinessEnvironmentList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 71, name: '生产环境', code: 'production', business_system: 1, business_system_name: '订单系统', service_count: 0, deployment_count: 0, enabled: true }], count: 1 } },
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
    expect(applicationApi.getClusterProfileList).toHaveBeenCalledTimes(2)

    await clickTab(wrapper, '部署模板')
    expect(wrapper.text()).toContain('新增模板')
    expect(wrapper.text()).toContain('tomcat-systemd')
    expect(applicationApi.getApplicationDeploymentTemplateList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '应用定义')

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
    expect(wrapper.findAll('.ant-tag').some((tag) => tag.text().includes('模型：'))).toBe(false)

    wrapper.unmount()
  })

})
