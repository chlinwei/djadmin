import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  deleteApplicationDeployment: vi.fn(() => Promise.resolve({ data: { data: null } })),
  getApplicationDeploymentList: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 11, instance_name: 'redis-1', application_id: 2, host_name: 'node-1' },
      { id: 12, instance_name: 'mysql-1', application_id: 1, host_name: 'node-2' },
      { id: 13, instance_name: 'tomcat-1', application_id: 5, host_name: 'node-3' },
      { id: 14, instance_name: 'tomcat-2', application_id: 5, host_name: 'node-4' },
    ] } },
  })),
  getApplicationList: vi.fn(() => Promise.resolve({
    data: { data: { results: [
      { id: 1, name: 'MySQL' },
      { id: 2, name: 'Redis' },
      { id: 5, name: 'Tomcat' },
    ] } },
  })),
  getApplicationVersionList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 51, application: 2, version: '1.0' }] } },
  })),
  getApplicationDeploymentTemplateList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 61, application: 2, name: 'Redis Template', enabled: true }] } },
  })),
  getApplicationService: vi.fn(() => Promise.resolve({
    data: { data: {
      id: 20,
      name: 'tomcat-group',
      code: 'tomcat-group',
      application: 5,
      topology_type: 'cluster',
      cluster_profile: 4,
      member_instances: [
        { deployment: 13, port: null },
        { deployment: 14, port: null },
      ],
    } },
  })),
  getBusinessSystemList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 3, name: '订单系统' }] } },
  })),
  getBusinessEnvironmentList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{ id: 31, business_system: 3, name: '生产环境', code: 'production', enabled: true }] } },
  })),
  getClusterProfileList: vi.fn(() => Promise.resolve({
    data: { data: { results: [{
      id: 9,
      name: 'Redis 集群',
      application: 2,
      application_name: 'Redis',
      cluster_type: 'redis',
      enabled: true,
    }, {
      id: 4,
      name: 'HA 集群',
      application: null,
      application_name: null,
      cluster_type: 'ha',
      enabled: true,
    }] } },
  })),
  saveApplicationService: vi.fn(),
}))

import ApplicationServiceDialog from './ApplicationServiceDialog.vue'

describe('ApplicationServiceDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('derives the application and member candidates from a direct cluster profile', async () => {
    const wrapper = mount(ApplicationServiceDialog, {
      props: { open: false, clusterProfileId: 9 },
      attachTo: document.body,
      global: {
        plugins: [Antd],
        stubs: {
          AModal: { template: '<div><slot /></div>' },
        },
      },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(document.body.textContent).not.toContain('部署形态')
    expect(document.body.querySelector('input.ant-input[disabled]').value).toBe('Redis')
    wrapper.unmount()
  })

  it('keeps an HA cluster application and its member instances when editing', async () => {
    const wrapper = mount(ApplicationServiceDialog, {
      props: { open: false, serviceId: 20 },
      attachTo: document.body,
      global: {
        plugins: [Antd],
        stubs: { AModal: { template: '<div><slot /></div>' } },
      },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(document.body.textContent).toContain('Tomcat')
    expect(document.body.textContent).toContain('tomcat-1 (node-3)')
    expect(document.body.textContent).toContain('tomcat-2 (node-4)')
    expect(document.body.textContent).toContain('成员实例（至少 2 个）')
    wrapper.unmount()
  })
})