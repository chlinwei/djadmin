import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, ref } from 'vue'
import Antd from 'ant-design-vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  batchDeleteApplication: vi.fn(() => Promise.resolve({ data: { data: null } })),
  controlApplicationDeployment: vi.fn((_id, action) => Promise.resolve({
    data: { data: {
      output: action === 'status' ? 'active' : '',
      runtime_status: action === 'status' ? 'running' : 'unknown',
    } },
  })),
  batchDeleteBusinessSystems: vi.fn(() => Promise.resolve({ data: { data: null } })),
  batchDeleteBusinessEnvironments: vi.fn(() => Promise.resolve({ data: { data: null } })),
  batchDeleteApplicationServices: vi.fn(() => Promise.resolve({ data: { data: null } })),
  batchDeleteClusterProfiles: vi.fn(() => Promise.resolve({ data: { data: null } })),
  batchDeleteApplicationDeployments: vi.fn(() => Promise.resolve({ data: { data: null } })),
  batchDeleteApplicationDeploymentTemplates: vi.fn(() => Promise.resolve({ data: { data: null } })),
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
  default: { state: { user: { timezone: 'Asia/Shanghai' }, perms: [], role_codes: [] } },
}))

vi.mock('@/util/timezone', () => ({
  formatTimeWithTimezone: vi.fn((value) => value),
}))

vi.mock('@/util/deleteConfirm', () => ({
  openDeleteConfirm: vi.fn(),
}))

import * as applicationApi from '@/api/assets/application'
import ApplicationWorkspace from './ApplicationWorkspace.vue'

function mountWorkspace(props = {}, stubs = {}, component = ApplicationWorkspace) {
  return mount(component, {
    props,
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        Dialog: true,
        VersionDialog: true,
        TemplateManagerDialog: true,
        DeploymentDialog: true,
        ApplicationServiceDialog: true,
        ClusterProfileDialog: true,
        FontAwesomeIcon: true,
        ...stubs,
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
  it('updates template portals after an in-flight response arrives on a deactivated page', async () => {
    const visible = ref(true)
    const cachedWorkspace = defineComponent({
      setup: () => () => h(KeepAlive, null, {
        default: () => visible.value ? h(ApplicationWorkspace) : h('div'),
      }),
    })
    const wrapper = mountWorkspace({}, { TemplateManagerDialog: false, TemplateDialog: true }, cachedWorkspace)
    await flushPromises()
    try {
      for (let cycle = 0; cycle < 3; cycle += 1) {
        let resolveTemplates
        applicationApi.getApplicationDeploymentTemplateList.mockImplementationOnce(() => new Promise((resolve) => {
          resolveTemplates = resolve
        }))
        const workspace = wrapper.findComponent(ApplicationWorkspace)
        await workspace.find('[data-manage-application-templates]').trigger('click')
        await flushPromises()
        visible.value = false
        await flushPromises()
        resolveTemplates({ data: { data: { results: [{ id: 2, name: `template-${cycle}`, enabled: true }], count: 1 } } })
        await flushPromises()
        visible.value = true
        await flushPromises()
        const dialog = workspace.findComponent({ name: 'TemplateManagerDialog' })
        expect(document.body.textContent).toContain(`template-${cycle}`)
        dialog.vm.$emit('update:open', false)
        await flushPromises()
        visible.value = false
        await flushPromises()
        visible.value = true
        await flushPromises()
      }
    } finally {
      wrapper.unmount()
    }
  })

  it.each([
    ['集群模型', 'getClusterProfileList'],
    ['应用定义', 'getApplicationList'],
  ])('%s 输入暂停后自动搜索，清空后恢复列表', async (label, apiName) => {
    const wrapper = mountWorkspace()
    await flushPromises()
    await clickTab(wrapper, label)
    vi.useFakeTimers()
    try {
      const request = applicationApi[apiName]
      request.mockClear()
      await wrapper.find('input.ant-input').setValue('redis')
      expect(request).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(300)
      expect(request).toHaveBeenCalledTimes(1)
      expect(request).toHaveBeenLastCalledWith(expect.objectContaining({ search: 'redis', page: 1 }))
      await wrapper.find('input.ant-input').setValue('')
      await vi.advanceTimersByTimeAsync(300)
      expect(request).toHaveBeenLastCalledWith(expect.objectContaining({ search: '', page: 1 }))
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

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

    await clickTab(wrapper, '集群模型')
    expect(wrapper.text()).toContain('新增自定义集群')
    expect(applicationApi.getClusterProfileList).toHaveBeenCalledTimes(1)

    await clickTab(wrapper, '应用定义')

    wrapper.unmount()
  })

  it('opens the template manager dialog for an application', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    await wrapper.find('[data-manage-application-templates]').trigger('click')

    const dialog = wrapper.findComponent({ name: 'TemplateManagerDialog' })
    expect(dialog.props('open')).toBe(true)
    expect(dialog.props('application')).toEqual(expect.objectContaining({ id: 1, name: 'Tomcat' }))

    wrapper.unmount()
  })

  it('updates real template portals while closing, reloading and reopening', async () => {
    const wrapper = mountWorkspace({}, { TemplateManagerDialog: false, TemplateDialog: true })
    await flushPromises()
    try {
      for (let cycle = 0; cycle < 3; cycle += 1) {
        await wrapper.find('[data-manage-application-templates]').trigger('click')
        await flushPromises()
        const dialog = wrapper.findComponent({ name: 'TemplateManagerDialog' })
        expect(document.body.textContent).toContain('tomcat-systemd')
        let resolveRefresh
        applicationApi.getApplicationList.mockImplementationOnce(() => new Promise((resolve) => {
          resolveRefresh = resolve
        }))
        vi.useFakeTimers()
        try {
          dialog.vm.$emit('changed')
          dialog.vm.$emit('update:open', false)
          await flushPromises()
          resolveRefresh({ data: { data: { results: [{ id: 1, name: 'Tomcat', versions: [], enabled: true }], count: 1 } } })
          await flushPromises()
          await vi.advanceTimersByTimeAsync(500)
          await flushPromises()
          expect(wrapper.findComponent({ name: 'TemplateManagerDialog' }).vm.$.uid).toBe(dialog.vm.$.uid)
          expect(dialog.props('open')).toBe(false)
        } finally {
          vi.useRealTimers()
        }
      }
    } finally {
      wrapper.unmount()
    }
  })

  it('displays cache and message queue category labels', async () => {
    applicationApi.getApplicationList.mockResolvedValueOnce({
      data: { data: { results: [
        { id: 2, name: 'Redis', category: 'cache', versions: [], enabled: true },
        { id: 3, name: 'IBM MQ', category: 'message_queue', versions: [], enabled: true },
      ], count: 2 } },
    })
    const wrapper = mountWorkspace()
    await flushPromises()

    const labels = wrapper.findAll('.ant-tag').map((tag) => tag.text())
    expect(labels).toContain('缓存')
    expect(labels).toContain('消息中间件')
    expect(labels).not.toContain('cache')
    expect(labels).not.toContain('message_queue')

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
