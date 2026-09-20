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
  // 列表接口**不带 logs**（真实契约：logs 是嵌套结构，只有详情接口给，列表只给 log_count）。
  // 这里故意照实模拟——上一版 mock 里塞了 logs，于是"从列表记录的 logs 铺模板日志表"这个 bug
  // 在测试里看不出来（真实环境永远读到空数组 → 新建服务时这张表一直空着）。
  getApplicationDeploymentTemplateList: vi.fn(() => Promise.resolve(baseTemplateListPayload())),
  // 模板详情：logs / macro_definitions 这类嵌套结构只从这里来。
  getApplicationDeploymentTemplate: vi.fn((id) => Promise.resolve(baseTemplateDetailPayload(id))),
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
    collection_enabled: null,
    collection_filter_rule_id: 91,
    // 解析规则只来自模板日志定义：服务侧只读展示。
    template_processing_rule_id: 91,
    template_processing_rule_name: 'error | failed | critical | fatal',
    retention_tier: null,
    data_stream: 'logs-production-order-std',
  }] } } })),
  saveApplicationService: vi.fn(),
  verifyApplicationServiceLogFormat: vi.fn(() => Promise.resolve({ data: { data: {
    passed: true,
    source: 'instance',
    missing_fields: [],
    format_state: 'verified',
    format_fingerprint: 'abc123',
    format_verified_at: '2026-09-19T10:00:00Z',
    format_verified_source: 'instance',
    format_verified_by: 'zhangsan',
  } } })),
}))

vi.mock('@/api/monitor', () => ({
  getLogRetentionTiers: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 3, code: 'std', name: '标准', retention_days: 30, enabled: true },
    { id: 4, code: 'short', name: '短期', retention_days: 2, enabled: true },
    { id: 1, code: 'hot', name: '热', retention_days: 7, enabled: true, is_default: true },
  ] } } })),
  getLogProcessingRules: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  getLogCollectionFilterRules: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 91, name: 'error | failed | critical | fatal', enabled: true, application: null, filter_pattern: '(?i)(error|failed|critical|fatal)' },
  ] } } })),
}))

import ApplicationServiceDialog from './ApplicationServiceDialog.vue'

// 这两个 mock 的实现在 afterEach 里要还原（见 restoreTemplateMocks），所以要拿到模块引用。
import { getApplicationDeploymentTemplate, getApplicationDeploymentTemplateList } from '@/api/assets/application'

// 两个模板的基础 fixtures（默认 mock 用它们）。62 是 external_ha：HA 拓扑的用例要它出现在
// 模板候选里（templateOptions 按拓扑过滤 control_type）。
function baseTemplateListPayload() {
  return { data: { data: { results: [
    { id: 61, application: 2, name: 'Redis Template', enabled: true, log_count: 1 },
    { id: 62, application: 5, name: 'Tomcat Template', control_type: 'external_ha', enabled: true, log_count: 1 },
  ] } } }
}

function baseTemplateDetailPayload(id) {
  return { data: { data: id === 61
    ? {
      id: 61, application: 2, name: 'Redis Template', enabled: true,
      logs: [{ id: 81, name: 'redis.log', path_pattern: '/var/log/redis/*.log', processing_rule: null }],
    }
    : {
      id: 62, application: 5, name: 'Tomcat Template', control_type: 'external_ha', enabled: true,
      logs: [{ id: 82, name: 'application.log', path_pattern: '/srv/tomcat/logs/application.log', processing_rule: null }],
    } } }
}

// 本文件里多处按需重设模板接口的 mock，而 `vi.clearAllMocks()` 只清调用记录、**不清实现**，
// 于是前面用例设过的实现会留给后面的用例（曾经把 HA 用例的模板选择搞没了）。
// afterEach 统一还原成基础 fixtures，单条用例可以放心临时改。
function restoreTemplateMocks() {
  getApplicationDeploymentTemplateList.mockImplementation(() => Promise.resolve(baseTemplateListPayload()))
  getApplicationDeploymentTemplate.mockImplementation((id) => Promise.resolve(baseTemplateDetailPayload(id)))
}

// 单机拓扑下可用的模板对（不带 control_type，见上方 templateOptions 的拓扑过滤）：
// 用来测"日志表怎么铺"的用例把契约钉死——列表**不带 logs**，logs 只在详情里。
function plainTemplateListPayload() {
  return { data: { data: { results: [
    { id: 61, application: 2, name: 'Redis Template', enabled: true, log_count: 1 },
    { id: 62, application: 5, name: 'Tomcat Template', enabled: true, log_count: 1 },
  ] } } }
}

function plainTemplateDetailPayload(id) {
  const plain = baseTemplateDetailPayload(id)
  delete plain.data.data.control_type
  return plain
}

describe('ApplicationServiceDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    restoreTemplateMocks()
    vi.clearAllMocks()
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

  // 模板没给默认值的宏必须在本服务填，否则保存不了：下发时路径里的 ${VAR} 按
  // 模板默认 → 服务覆盖 → 实例变量 找值，一个都没有时那台主机要么被跳过、要么拼出坏路径
  // （Filebeat 监听不到文件，采集静默为空）。这是"事后才发现"的坑，所以在保存时拦住。
  it('blocks saving while a template macro without a default is left empty', async () => {
    const { getApplicationDeploymentTemplateList, getApplicationService, getApplicationVersionList, saveApplicationService } = await import('@/api/assets/application')
    getApplicationVersionList.mockResolvedValue({ data: { data: { results: [{ id: 51, application: 5, version: '1.0' }] } } })
    // 把表单填成"除了宏以外都合法"（HA 集群还要求 VIP），否则校验会先拦下来，
    // 这条用例就测不到宏这一关。
    getApplicationService.mockResolvedValue({ data: { data: {
      id: 20,
      name: 'tomcat-group',
      code: 'tomcat-group',
      application: 5,
      application_version: 51,
      deployment_template: 62,
      topology_type: 'cluster',
      cluster_profile: 4,
      access_address: '10.0.0.100',
      business_system: 3,
      environment: 31,
      member_instances: [{ deployment: 13 }, { deployment: 14 }],
    } } })
    getApplicationDeploymentTemplateList.mockResolvedValue({ data: { data: { results: [{
      id: 62,
      application: 5,
      name: 'Tomcat Template',
      control_type: 'external_ha',
      enabled: true,
      macro_definitions: [
        { name: 'APP_HOME', value: '', description: '应用目录' },
        { name: 'LOG_DIR', value: '/var/log/tomcat', description: '日志目录' },
      ],
      logs: [{ id: 82, name: 'application.log', path_pattern: '${APP_HOME}/logs/application.log', processing_rule: null }],
    }] } } })

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

    // 只有"模板没给默认值"的那条算缺；有默认值的照旧继承。
    expect(wrapper.vm.missingMacros.map((macro) => macro.name)).toEqual(['APP_HOME'])
    expect(wrapper.text()).toContain('模板没给默认值的宏必须在这里填：${APP_HOME}')
    // 表格里要能看见"必填"，而不是显示成"继承"（没有可继承的东西）。
    expect(wrapper.findAll('.ant-tag').map((tag) => tag.text()).join('|')).toContain('必填')

    await wrapper.vm.submit()
    await flushPromises()
    expect(saveApplicationService).not.toHaveBeenCalled()

    // 填上之后红标消失，也能保存了（覆盖值随 payload 提交，模板的默认值不动）。
    wrapper.vm.setMacroValue('APP_HOME', '/opt/tomcat')
    await flushPromises()
    expect(wrapper.vm.missingMacros).toHaveLength(0)
    expect(wrapper.text()).not.toContain('模板没给默认值的宏必须在这里填')
    await wrapper.vm.submit()
    await flushPromises()
    expect(saveApplicationService).toHaveBeenCalled()
    const payload = saveApplicationService.mock.calls.at(-1)[0]
    expect(payload.macro_values.APP_HOME).toBe('/opt/tomcat')
    wrapper.unmount()
  })

  // 2026-09-20：编辑弹窗不再编辑逐条日志配置（统一走日志中心「日志配置」），
  // 所以保存时**绝不能提交 log_settings**——那个字段是整表替换语义，提交空集合会把该服务
  // 已有的采集开关/档位/过滤覆盖值全部删掉。字段缺省（null）= 后端整块跳过、一行都不动。
  it('never submits log_settings so saving cannot wipe per-log overrides', async () => {
    const { getApplicationService, getApplicationVersionList, saveApplicationService, getApplicationDeploymentTemplateList } = await import('@/api/assets/application')
    getApplicationVersionList.mockResolvedValue({ data: { data: { results: [{ id: 51, application: 5, version: '1.0' }] } } })
    getApplicationDeploymentTemplateList.mockResolvedValue({ data: { data: { results: [{
      id: 62, application: 5, name: 'Tomcat Template', control_type: 'external_ha', enabled: true, macro_definitions: [],
    }] } } })
    getApplicationService.mockResolvedValue({ data: { data: {
      id: 20, name: 'tomcat-group', code: 'tomcat-group', application: 5, application_version: 51,
      deployment_template: 62, topology_type: 'cluster', cluster_profile: 4,
      access_address: '10.0.0.100', business_system: 3, environment: 31,
      member_instances: [{ deployment: 13 }, { deployment: 14 }],
    } } })
    saveApplicationService.mockResolvedValue({ data: { data: { id: 20 } } })

    const wrapper = mount(ApplicationServiceDialog, {
      props: { open: false, serviceId: 20 },
      attachTo: document.body,
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    await wrapper.vm.submit()
    await flushPromises()

    expect(saveApplicationService).toHaveBeenCalled()
    const payload = saveApplicationService.mock.calls.at(-1)[0]
    // 关键断言：字段根本不出现在提交体里（不是空数组）——空数组会被后端当成"全量为空"从而删掉覆盖值。
    expect('log_settings' in payload).toBe(false)
    // 服务级的两个日志默认值照旧提交（它们不是逐条覆盖值）。
    expect(payload.log_collection_enabled).toBeDefined()
    expect('log_retention_tier' in payload).toBe(true)
    wrapper.unmount()
  })

  // 逐条配置的入口在日志中心：弹窗里要写明去哪儿改，并说清"这里保存不会改动它们"。
  it('points per-log configuration to the log center instead of editing it here', async () => {
    const wrapper = mount(ApplicationServiceDialog, {
      props: { open: false, serviceId: 20 },
      attachTo: document.body,
      global: { plugins: [Antd], stubs: { AModal: { template: '<div><slot /></div>' } } },
    })
    await wrapper.setProps({ open: true })
    await flushPromises()

    expect(document.body.textContent).toContain('逐条日志配置')
    expect(document.body.textContent).toContain('日志中心')
    expect(document.body.textContent).toContain('不会改动')
    // 旧的「模板日志」表必须真的不在了（列头是它最显眼的标志）。
    expect(document.body.textContent).not.toContain('处理规则（模板）')
    expect(document.body.textContent).not.toContain('采集过滤（保留）')
    wrapper.unmount()
  })

})
