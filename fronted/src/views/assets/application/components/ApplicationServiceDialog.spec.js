import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
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
    data: { data: { results: [{
      id: 61,
      application: 2,
      name: 'Redis Template',
      enabled: true,
      logs: [{ id: 81, name: 'redis.log', path_pattern: '/var/log/redis/*.log', collection_enabled: true, processing_rule: null }],
    }, {
      id: 62,
      application: 5,
      name: 'Tomcat Template',
      control_type: 'external_ha',
      enabled: true,
      logs: [{ id: 82, name: 'application.log', path_pattern: '/srv/tomcat/logs/application.log', collection_enabled: true, processing_rule: null }],
    }] } },
  })),
  getApplicationService: vi.fn(() => Promise.resolve({
    data: { data: {
      id: 20,
      name: 'tomcat-group',
      code: 'tomcat-group',
      application: 5,
      deployment_template: 62,
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
  getApplicationServiceLogConfig: vi.fn(() => Promise.resolve({ data: { data: { logs: [{
    log_definition: 81,
    name: 'application.log',
    resolved_path: '/srv/tomcat/logs/application.log',
    template_collection_enabled: true,
    collection_enabled: true,
    collection_mode: 'error_only',
    filter_pattern: '(?i)(error|failed|critical|fatal)',
    collection_filter_rule_id: 91,
    processing_rule_id: null,
    effective_processing_rule_name: '',
    retention_tier: null,
    data_stream: 'logs-production-order-std',
  }] } } })),
  saveApplicationService: vi.fn(),
}))

vi.mock('@/api/monitor', () => ({
  getLogRetentionTiers: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  getLogProcessingRules: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  getLogCollectionFilterRules: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 91, name: 'error | failed | critical | fatal', enabled: true, application: null, filter_pattern: '(?i)(error|failed|critical|fatal)' },
  ] } } })),
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

  it('pre-fills business system and environment from the tree scope when creating fresh', async () => {
    const wrapper = mount(ApplicationServiceDialog, {
      props: { open: false, initialBusinessSystemId: 3, initialEnvironmentId: 31 },
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

    expect(wrapper.vm.form.business_system).toBe(3)
    expect(wrapper.vm.form.environment).toBe(31)
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

  // 回归用例：已绑定的实例不能因为"派生的应用不同"而从成员列表里消失。
  //
  // 2026-09-18 现场：yilake nginx（应用 15）绑定了实例 yilake-nginx-105，但该实例最早挂在
  // redis 下，后端派生的 application_id 是 8 ≠ 15；旧版 availableDeploymentOptions 只用
  // 应用相等来过滤，两个 watcher 又拿它去删 selectedDeploymentIds，于是库里关联还在，
  // 编辑弹窗里成员却是空的，用户以为"绑定的实例丢了"。
  it('keeps a bound instance whose derived application differs from the service application', async () => {
    // 该实例（id=12）的派生 application_id=1，而服务 20 的应用是 5 —— 必须仍然显示。
    const { getApplicationService } = await import('@/api/assets/application')
    getApplicationService.mockResolvedValueOnce({
      data: { data: {
        id: 20,
        name: 'tomcat-group',
        code: 'tomcat-group',
        application: 5,
        deployment_template: 62,
        topology_type: 'cluster',
        cluster_profile: 4,
        member_instances: [
          { deployment: 12, port: null },
          { deployment: 13, port: null },
        ],
      } },
    })

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

    expect(wrapper.vm.selectedDeploymentIds).toEqual([12, 13])
    expect(document.body.textContent).toContain('mysql-1 (node-2)')
    expect(document.body.textContent).toContain('tomcat-1 (node-3)')
    wrapper.unmount()
  })

  // 从已有实例中添加成员：实例已在别的服务下（派生 application_id 与当前服务不同）也要能绑上。
  // 此前只能走「新增部署实例」→ 按主机+实例名去重转成编辑的绕路，且那条路会撞上后端的回读缺陷。
  it('binds an existing instance picked from the candidate list', async () => {
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

    // 候选里能看到"另一个应用"下的实例（id=12，派生 application_id=1），
    // 但已经绑定的 13/14 不出现在候选里。
    const candidateIds = wrapper.vm.addableDeploymentOptions.map((item) => item.value)
    expect(candidateIds).toContain(12)
    expect(candidateIds).not.toContain(13)
    expect(candidateIds).not.toContain(14)

    wrapper.vm.addPickedDeployment(12)
    await flushPromises()
    expect(wrapper.vm.selectedDeploymentIds).toEqual([13, 14, 12])
    expect(document.body.textContent).toContain('mysql-1 (node-2)')
    wrapper.unmount()
  })

  it('shows the effective error-only collection policy for each template log', async () => {
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

    expect(document.body.textContent).toContain('采集策略')
    expect(document.body.textContent).toContain('过滤规则')
    expect(document.body.textContent).toContain('error | failed | critical | fatal')
    wrapper.unmount()
  })

  it('allows a new service to configure template log policies before its first save', async () => {
    const wrapper = mount(ApplicationServiceDialog, {
      props: { open: false },
      attachTo: document.body,
      global: {
        plugins: [Antd],
        stubs: { AModal: { template: '<div><slot /></div>' } },
      },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    wrapper.vm.form.application = 2
    wrapper.vm.form.deployment_template = 61
    await flushPromises()

    expect(document.body.textContent).toContain('redis.log')
    expect(document.body.textContent).toContain('采集策略')
    wrapper.unmount()
  })
})