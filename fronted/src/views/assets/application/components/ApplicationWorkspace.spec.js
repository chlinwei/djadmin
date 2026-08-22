import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  batchDeleteApplication: vi.fn(() => Promise.resolve({ data: { data: null } })),
  checkApplicationDeploymentBaseline: vi.fn(() => Promise.resolve({ data: { data: {} } })),
  controlApplicationDeployment: vi.fn(() => Promise.resolve({ data: { data: { output: 'active' } } })),
  deleteApplicationDeployment: vi.fn(() => Promise.resolve({ data: { data: null } })),
  deleteApplicationDeploymentTemplate: vi.fn(() => Promise.resolve({ data: { data: null } })),
  getApplicationDeploymentBaselineHistory: vi.fn(() => Promise.resolve({ data: { data: [] } })),
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

function mountWorkspace() {
  return mount(ApplicationWorkspace, {
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        Dialog: true,
        VersionDialog: true,
        TemplateDialog: true,
        DeploymentDialog: true,
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
    document.body.innerHTML = ''
  })

  it('switches tables and create action without component update errors', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    expect(wrapper.text()).toContain('新增应用')
    expect(applicationApi.getApplicationList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '部署模板')
    expect(wrapper.text()).toContain('新增模板')
    expect(wrapper.text()).toContain('tomcat-systemd')
    expect(applicationApi.getApplicationDeploymentTemplateList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '部署实例')
    expect(wrapper.text()).toContain('登记实例')
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '应用定义')
    await clickTab(wrapper, '部署实例')
    expect(wrapper.text()).toContain('登记实例')
    expect(applicationApi.getApplicationDeploymentList).toHaveBeenCalledTimes(2)

    wrapper.unmount()
  })

  it('ignores a stale application response after switching to deployments', async () => {
    let resolveApplicationList
    applicationApi.getApplicationList.mockImplementationOnce(() => new Promise((resolve) => {
      resolveApplicationList = resolve
    }))
    applicationApi.getApplicationDeploymentList.mockResolvedValueOnce({
      data: { data: { results: [{ id: 1, instance_name: 'current-deployment', application_name: 'Tomcat' }], count: 1 } },
    })

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
    expect(wrapper.findAll('[data-control-action="status"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-control-action="baseline"]')).toHaveLength(2)

    wrapper.unmount()
  })
})
