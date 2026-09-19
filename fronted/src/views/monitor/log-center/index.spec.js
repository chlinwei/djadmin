import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/assets/application', () => ({
  getApplicationServiceLogConfig: vi.fn(() => Promise.resolve({ data: { data: {
    log_collection_enabled: true,
    service_code: 'nginx',
    logs: [
    {
      log_definition: 81, name: 'access.log', resolved_path: '/srv/tomcat/logs/application.log',
      template_processing_rule_id: 91, template_processing_rule_name: 'tomcat rule',
      collection_enabled: null, retention_tier: 3, tier_code: 'hot', service_code: 'nginx',
      format_state: 'unverified', data_stream: 'autoadmin-yilake-tib-poc-nginx-wuhan-test',
    },
  ] } } })),
  getApplicationDeploymentList: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 13, instance_name: 'tomcat-1', host_name: 'node-3' },
  ], totalPages: 1 } } })),
  setApplicationServiceLogCollection: vi.fn((_id, enabled) => Promise.resolve({ data: { data: { log_collection_enabled: enabled } } })),
  saveApplicationServiceLogSetting: vi.fn((_id, payload) => Promise.resolve({ data: { data: {
    log_definition: payload.log_definition_id, name: 'access.log',
    resolved_path: '/srv/tomcat/logs/application.log', template_processing_rule_id: 91,
    template_processing_rule_name: 'tomcat rule', collection_enabled: payload.collection_enabled,
    retention_tier: payload.retention_tier, tier_code: 'hot', service_code: 'nginx', format_state: 'verified',
    data_stream: 'autoadmin-yilake-tib-poc-nginx-wuhan-test',
  } } })),
}))

vi.mock('@/api/monitor', () => ({
  getElasticsearchClusterList: vi.fn(() => Promise.resolve({ data: { data: { results: [{ id: 1 }] } } })),
  getLogRetentionTiers: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 1, code: 'hot', name: '热（7 天）' },
    { id: 3, code: 'std', name: '标准 30 天' },
    { id: 4, code: 'wuhan-test', name: '保留2天' },
  ] } } })),
  getLogStorageOverview: vi.fn(() => Promise.resolve({ data: { data: { data_streams: [
    { name: 'autoadmin-yilake-tib-poc-nginx-wuhan-test', tier: 'wuhan-test', docs: 4758176, bytes: 1054670278, ilm_state: 'hot', recognized: true, historical: true, service_enabled: true, service_collection_enabled: true },
    { name: 'autoadmin-yilake-tib-poc-nginx-hot', tier: 'hot', docs: 10, bytes: 55867, ilm_state: 'hot', recognized: true, historical: false, service_enabled: false, service_collection_enabled: true },
  ], dims: {} } } })),
  searchElasticsearchLogs: vi.fn(() => Promise.resolve({ data: { data: { results: [], count: 0 } } })),
  searchElasticsearchLogFacetStats: vi.fn(() => Promise.resolve({ data: { data: { buckets: [] } } })),
  cleanupLogDataStream: vi.fn(),
  getLogProcessingRules: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  getServiceLogConfigState: vi.fn(() => Promise.resolve({ data: { data: {
    summary: { hosts: 3, managed: 2, unmanaged: 1, synced: 1, drift: 1, never: 0, unknown: 0 },
    hosts: [
      { host_id: 10, host_ip: '10.0.0.10', host_instance_name: 'node-a', target_id: 101, config_state: 'synced', managed: true },
      { host_id: 20, host_ip: '10.0.0.20', host_instance_name: 'node-b', target_id: 102, config_state: 'drift', managed: true },
    ],
    unmanaged_hosts: [{ host_id: 30, host_ip: '10.0.0.30', host_instance_name: 'node-c', target_id: 0, managed: false }],
  } } })),
  applyLogTargetsForService: vi.fn(() => Promise.resolve({ data: { data: {
    job: { id: 77 }, target_total: 2,
    unmanaged_hosts: [{ host_id: 30, host_ip: '10.0.0.30', host_instance_name: 'node-c' }],
    message: '有 1 台承载主机还没有纳管日志采集，本次不会下发：node-c',
  } } })),
  getLogBatchJob: vi.fn(() => Promise.resolve({ data: { data: {
    id: 77, is_running: false, total_count: 2, success_count: 2, failed_count: 0, pending_count: 0, message: '全部完成',
  } } })),
}))

vi.mock('@/util/deleteConfirm', () => ({ openDeleteConfirm: vi.fn(() => Promise.resolve(true)) }))

import LogCenter from './index.vue'

const serviceScope = {
  nodeType: 'service', applicationServiceId: 15, nodeTitle: 'nginx',
  businessSystemName: 'TIB', environmentName: 'poc',
}

// 富载荷：含集群/数据时间/节点磁盘/维度码/未识别流——对应当前存储水位页能看到的全部信息。
function fullOverviewPayload() {
  return {
    cluster: { id: 1, index_prefix: 'autoadmin' },
    generated_at: '2026-09-19T04:00:00Z',
    allocation: [{ node: '192.168.201.123:9200', name: 'es-1', shards: 12, 'disk.used': 85899345920, 'disk.total': 109951162777, 'disk.percent': '78' }],
    dims: {
      projects: [{ id: 15, code: 'yilake', name: 'yilake' }, { id: 2, code: 'nkg', name: 'nkg' }, { id: 1, code: 'kul', name: 'KUL 项目' }],
      business_systems: [
        { id: 19, code: 'tib', name: 'TIB', project_id: 15 },
        { id: 20, code: 'cdm', name: 'CDM', project_id: 15 },
      ],
      environments: [{ id: 10, code: 'poc', name: 'poc' }, { id: 11, code: 'prod', name: '生产' }],
      services: [
        { id: 15, code: 'nginx', name: 'nginx', business_system: 'tib', environment: 'poc' },
        { id: 17, code: 'mgmt', name: 'mgmt', business_system: 'tib', environment: 'poc' },
        { id: 18, code: 'redis', name: 'redis', business_system: 'cdm', environment: 'prod' },
      ],
    },
    data_streams: [
      { name: 'autoadmin-yilake-tib-poc-nginx-hot', project: 'yilake', environment: 'poc', business_system: 'tib', service: 'nginx', tier: 'hot', health: 'green', docs: 10, bytes: 55867, ilm_state: 'hot', recognized: true, service_enabled: true, service_collection_enabled: true, backing_indices: [{ index: '.ds-autoadmin-yilake-tib-poc-nginx-hot-2026.09.19-000001', health: 'green', docs: 10, bytes: 55867, create_at: '2026-09-19T02:00:00Z', ilm_state: 'hot' }] },
      { name: 'autoadmin-yilake-tib-poc-nginx-wuhan-test', project: 'yilake', environment: 'poc', business_system: 'tib', service: 'nginx', tier: 'wuhan-test', health: 'yellow', docs: 4758176, bytes: 1054670278, ilm_state: 'hot', recognized: true, historical: true, service_enabled: true, service_collection_enabled: true, backing_indices: [] },
      { name: 'autoadmin-nkg-tib-poc-mgmt-hot', project: 'nkg', environment: 'poc', business_system: 'tib', service: 'mgmt', tier: 'hot', health: 'green', docs: 5, bytes: 1024, ilm_state: 'hot', recognized: true, service_enabled: true, service_collection_enabled: true, backing_indices: [] },
      { name: 'autoadmin-yilake-tib-poc-legacy-hot', project: 'yilake', environment: 'poc', business_system: 'tib', service: 'legacy', tier: 'hot', health: 'green', docs: 7, bytes: 4096, ilm_state: 'hot', recognized: true, historical: true, service_enabled: true, service_collection_enabled: true, backing_indices: [] },
      { name: 'manual-test-stream', health: 'green', docs: 3, bytes: 2048, ilm_state: '', recognized: false, service_enabled: false, service_collection_enabled: false, backing_indices: [] },
    ],
  }
}

async function mountPage() {
  const wrapper = mount(LogCenter, {
    attachTo: document.body,
    global: {
      plugins: [Antd],
      stubs: {
        // 树与查询面板各有一整套自己的数据流与 spec，这里只验证本页的接线（scope → 各 tab 的数据）。
        ServiceTree: { name: 'ServiceTree', props: ['selectedScope'], emits: ['select', 'stats-change'], template: '<div class="stub-tree" />' },
        LogQueryPanel: { name: 'LogQueryPanel', props: ['scope'], template: '<div class="stub-query-panel" />' },
        LogFormatVerifyDialog: { name: 'LogFormatVerifyDialog', props: ['open', 'serviceId', 'target', 'deploymentOptions'], template: '<div class="stub-verify" />' },
        // echarts 在 jsdom 里没有真实 canvas：只验证 option 接线（option 的构造另有纯函数单测）。
        StorageUsagePie: { name: 'StorageUsagePie', props: ['option'], template: '<div class="stub-pie" />' },
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('日志中心（服务树 + 三个 tab）', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('asks for a service instead of firing requests before one is selected', async () => {
    const { getApplicationServiceLogConfig } = await import('@/api/assets/application')
    const wrapper = await mountPage()

    expect(document.body.textContent).toContain('请在左侧选择逻辑服务或部署实例')
    expect(getApplicationServiceLogConfig).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 命中服务后：日志配置来自 assets 域的 log-config，采集开关按"无覆盖行=采"的口径展示。
  it('renders the log config of the selected service', async () => {
    const { getApplicationServiceLogConfig } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(getApplicationServiceLogConfig).toHaveBeenCalledWith(15)
    // 名称与路径故意取不同值：名称列为空时（列缺 dataIndex/插槽）这里才会失败，
    // 若名称是路径的子串，路径列会把断言"顶替"通过。
    expect(document.body.textContent).toContain('access.log')
    expect(document.body.textContent).toContain('/srv/tomcat/logs/application.log')
    expect(document.body.textContent).toContain('tomcat rule')
    // 页面上有两个开关，分属两层：服务级总开关（apply-switch）与逐条日志的采集开关（表格里）。
    expect(wrapper.findAll('.apply-switch .ant-switch')).toHaveLength(1)
    expect(wrapper.findAll('.ant-table .ant-switch')).toHaveLength(1)
    // 无覆盖行 = 采：逐条开关处于勾选态（不再是"采集中"标签）。
    expect(wrapper.findAll('.ant-table .ant-switch-checked')).toHaveLength(1)
    expect(wrapper.vm.isLogCollected(wrapper.vm.logRows[0])).toBe(true)
    // 档位列是下拉：显示当前档位名。
    expect(wrapper.find('.ant-select-selection-item').text()).toBe('标准 30 天')
    expect(document.body.textContent).toContain('autoadmin-yilake-tib-poc-nginx-wuhan-test')
    wrapper.unmount()
  })

  // 水位只在切到那个 tab 时读，并且**必须带 service_code**：后端据此把 ES 查询收窄到本服务的索引，
  // 少了它就会退化成"取全集群再前端过滤"（页面看起来一样，代价差很多）。
  it('loads the water level lazily with service_code so the backend can narrow the ES query', async () => {
    const { getLogStorageOverview } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(getLogStorageOverview).not.toHaveBeenCalled()

    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    expect(getLogStorageOverview).toHaveBeenCalledWith(1, { service_code: 'nginx' })
    // 断言完整流名（只出现在 Data Stream 列），而不是 "wuhan-test"——那是档位列的内容。
    expect(document.body.textContent).toContain('autoadmin-yilake-tib-poc-nginx-wuhan-test')
    // 服务停用的那条流要标注"已停用"，而不是和正常采集长得一样。
    expect(document.body.textContent).toContain('已停用')
    wrapper.unmount()
  })

  // 回归：点开服务的瞬间，配置还在路上时**不能**按"总开关已关闭"渲染——否则会闪一条
  // 黄色警告再消失（用户看到的"最开始有个黄色的界面"就是这个）。同理也不能显示上一个
  // 服务的日志（串台）。这里用一个人为挂起的请求把"在途"这一刻固定住。
  it('does not flash the switched-off warning or stale logs while the config is loading', async () => {
    const { getApplicationServiceLogConfig } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    const tree = wrapper.findComponent({ name: 'ServiceTree' })

    // 第一个服务：正常返回一条日志
    tree.vm.$emit('select', serviceScope)
    await flushPromises()
    expect(document.body.textContent).toContain('access.log')

    // 第二个服务：请求挂起，模拟"刚点开、数据还没回来"
    let resolvePending
    getApplicationServiceLogConfig.mockImplementationOnce(() => new Promise((resolve) => { resolvePending = resolve }))
    tree.vm.$emit('select', { ...serviceScope, applicationServiceId: 16, nodeTitle: 'redis' })
    await flushPromises()

    // 在途时：不许有"总开关已关闭"的黄色告警，也不许残留上一个服务的日志行。
    expect(wrapper.vm.serviceCollectEnabled).toBeNull()
    expect(document.body.textContent).not.toContain('服务采集总开关已关闭')
    expect(document.body.textContent).not.toContain('access.log')
    // 开关此刻是禁用+loading，避免用户在"状态未知"时误点。
    const masterSwitch = wrapper.find('.apply-switch .ant-switch')
    expect(masterSwitch.classes()).toContain('ant-switch-disabled')
    expect(masterSwitch.classes()).toContain('ant-switch-loading')

    // 数据回来后：仍然是采集开启，且没有黄色告警。
    resolvePending({ data: { data: { log_collection_enabled: true, service_code: 'redis', logs: [
      { log_definition: 91, name: 'redis.log', resolved_path: '/var/log/redis/redis.log', tier_code: 'hot',
        template_processing_rule_id: 12, template_processing_rule_name: 'redis rule', collection_enabled: null },
    ] } } })
    await flushPromises()

    expect(document.body.textContent).not.toContain('服务采集总开关已关闭')
    expect(document.body.textContent).toContain('redis.log')
    wrapper.unmount()
  })

  // 服务级采集总开关：与逐条开关是两层，缺了它用户会以为"逐条都关了"其实只是总开关关了
  // （或者反过来，找不到关掉整个服务采集的地方）。
  it('shows the service-level collection switch and saves it per service', async () => {
    const { setApplicationServiceLogCollection } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(wrapper.vm.serviceCollectEnabled).toBe(true)
    expect(document.body.textContent).toContain('服务采集总开关')

    await wrapper.vm.toggleServiceCollect(false)
    await flushPromises()

    expect(setApplicationServiceLogCollection).toHaveBeenCalledWith(15, false)
    expect(wrapper.vm.serviceCollectEnabled).toBe(false)
    wrapper.unmount()
  })

  // 总开关关掉时必须明说"逐条开关不生效"，否则用户会去逐条打开却发现没效果。
  it('warns that per-log switches do not apply while the service switch is off', async () => {
    const { getApplicationServiceLogConfig } = await import('@/api/assets/application')
    getApplicationServiceLogConfig.mockResolvedValueOnce({ data: { data: {
      log_collection_enabled: false,
      service_code: 'nginx',
      logs: [{
        log_definition: 81, name: 'access.log', resolved_path: '/srv/tomcat/logs/application.log',
        template_processing_rule_id: 91, template_processing_rule_name: 'tomcat rule',
        collection_enabled: null, retention_tier: 3, service_code: 'nginx',
        format_state: 'unverified', data_stream: 'autoadmin-yilake-tib-poc-nginx-wuhan-test',
      }],
    } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(wrapper.vm.serviceCollectEnabled).toBe(false)
    expect(document.body.textContent).toContain('服务采集总开关已关闭')
    expect(document.body.textContent).toContain('逐条的采集开关不生效')
    wrapper.unmount()
  })

  // 下发状态只能是主机级聚合：文案要说"承载 N 台主机 / M 台待下发"，不能写成"本服务已同步"。
  it('aggregates the per-host config state instead of inventing a service-level one', async () => {
    const { getServiceLogConfigState } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(getServiceLogConfigState).toHaveBeenCalledWith(15)
    expect(wrapper.vm.applySummaryText).toContain('承载 3 台主机')
    expect(wrapper.vm.applySummaryText).toContain('1 台已同步')
    expect(wrapper.vm.applySummaryText).toContain('1 台待下发')
    expect(wrapper.vm.applySummaryText).toContain('1 台未纳管')
    // 未纳管的主机要显式提示（下发不到它们，不能静默少下发）
    expect(wrapper.vm.applyUnmanagedText).toContain('node-c')
    wrapper.unmount()
  })

  // 服务级下发：调服务级接口（而不是自己拼目标 id），随后按作业 id 轮询进度。
  it('applies for the whole service and polls the created batch job', async () => {
    const { applyLogTargetsForService, getLogBatchJob } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    await wrapper.vm.applyService()
    await flushPromises()

    expect(applyLogTargetsForService).toHaveBeenCalledWith(15)
    expect(getLogBatchJob).toHaveBeenCalledWith(77)
    expect(wrapper.vm.applyJob.total_count).toBe(2)
    expect(wrapper.vm.applyJobPercent).toBe(100)
    wrapper.unmount()
  })

  // 改档位会留下历史流（旧流停止写入、数据按原档位保留到期）。它必须被标出来：
  // 现场 nginx 的旧流还留着 1.0 GB / 475 万条，如果和当前档位的流长得一样（绿标签"采集中"），
  // 用户会以为数据还在往那里写，也看不懂"本服务磁盘占用"为什么这么大。
  it('marks streams of a retired tier as historical instead of showing them as collecting', async () => {
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    // 判定来自后端字段（只有后端知道该服务当前生效的档位集合，全局视图里前端也推不出）。
    const byName = Object.fromEntries(wrapper.vm.storageRows.map((row) => [row.name, row]))
    expect(byName['autoadmin-yilake-tib-poc-nginx-wuhan-test'].historical).toBe(true)
    expect(byName['autoadmin-yilake-tib-poc-nginx-hot'].historical).toBe(false)
    expect(wrapper.vm.isHistoricalTier(byName['autoadmin-yilake-tib-poc-nginx-wuhan-test'])).toBe(true)
    expect(wrapper.vm.isHistoricalTier({ recognized: true, tier: 'wuhan-test' })).toBe(false)

    expect(document.body.textContent).toContain('历史档位（已停写）')
    // 占用必须拆开：活跃占用是当前成本，历史占用是"还占着盘但已不再写入"的沉淀，
    // 合成一个数既看不出问题，也没法判断该不该清理。
    expect(wrapper.vm.storageSummary).toMatchObject({
      historicalStreams: 1, activeBytes: 55867, historicalBytes: 1054670278,
    })
    expect(document.body.textContent).toContain('活跃 54.6 KB')
    expect(document.body.textContent).toContain('历史 1005.8 MB')
    wrapper.unmount()
  })

  // 模板里的日志定义被删光之后：这个服务什么都不采了，但 ES 里的存量流还在。
  // 页面不能因为"没有日志定义"就当作没有流（数据还在，占地还在）。
  it('lists leftover streams as historical when the template has no log definitions', async () => {
    const { getApplicationServiceLogConfig } = await import('@/api/assets/application')
    const { getLogStorageOverview } = await import('@/api/monitor')
    getApplicationServiceLogConfig.mockResolvedValueOnce({ data: { data: {
      log_collection_enabled: true, service_code: 'nginx', logs: [],
    } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    // 仍然按服务收窄查了水位（用的是顶层 service_code，而不是 logs[0]）
    expect(getLogStorageOverview).toHaveBeenCalledWith(1, { service_code: 'nginx' })
    expect(wrapper.vm.storageRows).toHaveLength(2)
    // 展示层不再自己推断历史流（判定在后端），这里只保证"没有日志定义也不隐藏存量流"。
    expect(wrapper.vm.storageRows.map((row) => row.name)).toContain('autoadmin-yilake-tib-poc-nginx-wuhan-test')
    wrapper.unmount()
  })

  // 「切回该档位」：候选是"当前生效档位不等于该历史档位"的日志，逐条按行提交；
  // 每条请求都要带上采集开关的当前值——按行接口把提交体当作该行覆盖值的全集，缺一列会清掉它。
  it('switches selected logs back to the historical tier, one row per request', async () => {
    const { saveApplicationServiceLogSetting } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const historical = wrapper.vm.storageRows.find((row) => row.tier === 'wuhan-test')
    wrapper.vm.openSwitchTier(historical)
    await flushPromises()

    // 当前生效档位是 hot，所以这条日志是候选；目标档位 wuhan-test 的 id 是 4。
    expect(wrapper.vm.switchTierCandidates.map((row) => row.log_definition)).toEqual([81])
    expect(wrapper.vm.switchTierSelected).toEqual([81])

    await wrapper.vm.submitSwitchTier()
    await flushPromises()

    expect(saveApplicationServiceLogSetting).toHaveBeenCalledWith(15, {
      log_definition_id: 81, retention_tier: 4, collection_enabled: null,
    })
    // 档位改完要重新读配置/水位/下发状态（档位变了 → 渲染内容变 → 变成待下发）。
    expect(wrapper.vm.switchTierOpen).toBe(false)
    wrapper.unmount()
  })

  // 「立即清理」：只清理这一条流（带 tier），并且确认框里必须写清"不可恢复 + 流对象保留"——
  // 它是唯一会真删数据的入口，文案不能含糊。
  it('cleans only the historical stream after an explicit confirmation', async () => {
    const { openDeleteConfirm } = await import('@/util/deleteConfirm')
    const { cleanupLogDataStream } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const historical = wrapper.vm.storageRows.find((row) => row.tier === 'wuhan-test')
    wrapper.vm.cleanupHistoricalStream(historical)
    await flushPromises()

    expect(openDeleteConfirm).toHaveBeenCalledTimes(1)
    const options = openDeleteConfirm.mock.calls[0][0]
    expect(options.title).toContain('历史档位')
    expect(options.summary).toContain('不可恢复')
    expect(options.summary).toContain('只影响这个档位')
    expect(options.items[0]).toContain('autoadmin-yilake-tib-poc-nginx-wuhan-test')

    // 没有确认前不发请求；确认后才按 (服务, 档位) 清理。
    expect(cleanupLogDataStream).not.toHaveBeenCalled()
    await options.onConfirm()
    expect(cleanupLogDataStream).toHaveBeenCalledWith({ service_id: 15, mode: 'all', tier: 'wuhan-test' })
    wrapper.unmount()
  })

  it('opens the shared verify dialog with the log row as target', async () => {
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(wrapper.vm.verifyDialogVisible).toBe(false)
    wrapper.vm.openVerifyDialog(wrapper.vm.logRows[0])
    await flushPromises()

    expect(wrapper.vm.verifyDialogVisible).toBe(true)
    const dialog = wrapper.findComponent({ name: 'LogFormatVerifyDialog' })
    expect(dialog.props('target').log_definition).toBe(81)
    expect(dialog.props('serviceId')).toBe(15)
    // 实例候选来自该服务已绑定的部署实例（后端按库里的绑定取实例）。
    expect(dialog.props('deploymentOptions').map((item) => item.value)).toEqual([13])
    wrapper.unmount()
  })

  // 内联改采集开关：勾选态在库里是"无覆盖"（null），取消才落 false——与编辑弹窗同一口径。
  // 传参必须带上另一列（档位）的当前值：接口把提交体当作该行覆盖值的全集，缺一列等于清掉它。
  it('saves the collection switch as a per-row override and keeps the tier value', async () => {
    const { saveApplicationServiceLogSetting } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    await wrapper.vm.saveOverride(wrapper.vm.logRows[0], { collection_enabled: false })
    await flushPromises()

    expect(saveApplicationServiceLogSetting).toHaveBeenCalledWith(15, {
      log_definition_id: 81, collection_enabled: false, retention_tier: 3,
    })
    // 用后端回读的行更新界面（不在前端拼状态）
    expect(wrapper.vm.logRows[0].format_state).toBe('verified')
    wrapper.unmount()
  })

  it('sends null to mean "follow the default" when switching collection back on', async () => {
    const { saveApplicationServiceLogSetting } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    await wrapper.vm.saveOverride(wrapper.vm.logRows[0], { collection_enabled: null })
    await flushPromises()

    expect(saveApplicationServiceLogSetting).toHaveBeenCalledWith(15, {
      log_definition_id: 81, collection_enabled: null, retention_tier: 3,
    })
    wrapper.unmount()
  })

  // 保存失败：控件值由 record 派生（不做乐观更新），所以失败时界面自然留在原值，
  // 不会出现"界面显示改了、库里没改"。
  it('keeps the row unchanged and surfaces the error when saving fails', async () => {
    const { saveApplicationServiceLogSetting } = await import('@/api/assets/application')
    saveApplicationServiceLogSetting.mockRejectedValueOnce({ response: { data: { msg: '档位不存在' } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    const before = { ...wrapper.vm.logRows[0] }
    await wrapper.vm.saveOverride(wrapper.vm.logRows[0], { retention_tier: 99 })
    await flushPromises()

    expect(wrapper.vm.logRows[0].retention_tier).toBe(before.retention_tier)
    expect(wrapper.vm.savingRows[81]).toBeUndefined()
    wrapper.unmount()
  })

  // 存储水位页的集群级信息必须整合过来：节点磁盘（盘快满了比任何单条流都紧急）、
  // 数据时间与集群前缀（水位是"某集群某时刻"的快照，不给这两项数字没法解读）。
  it('shows the cluster, data time and node disk levels from the water level payload', async () => {
    const { getLogStorageOverview } = await import('@/api/monitor')
    getLogStorageOverview.mockResolvedValueOnce({ data: { data: fullOverviewPayload() } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    expect(wrapper.vm.storageCluster.index_prefix).toBe('autoadmin')
    expect(wrapper.vm.storageGeneratedAt).toBe('2026-09-19T04:00:00Z')
    expect(wrapper.vm.storageAllocation).toHaveLength(1)
    expect(document.body.textContent).toContain('Elasticsearch 节点磁盘')
    expect(document.body.textContent).toContain('78%')
    // 使用率高的节点要一眼看出来（>=70 橙、>=85 红）。
    expect(document.body.textContent).toContain('数据时间：2026-09-19T04:00:00Z')
    wrapper.unmount()
  })

  // 后备索引：真实磁盘占用的最细粒度（旧页的「后备索引」按钮），要能展开。
  it('expands the backing indices of a stream', async () => {
    const { getLogStorageOverview } = await import('@/api/monitor')
    getLogStorageOverview.mockResolvedValueOnce({ data: { data: fullOverviewPayload() } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    // 按流名取：全量视图里同档位的流不止一条（mgmt 也是 hot），按 tier 取会拿到没有后备索引的那条。
    const row = wrapper.vm.storageRows.find((item) => item.name === 'autoadmin-yilake-tib-poc-nginx-hot')
    wrapper.vm.toggleBackingIndices(row)
    await flushPromises()
    expect(document.body.textContent).toContain('.ds-autoadmin-yilake-tib-poc-nginx-hot-2026.09.19-000001')
    // 再点一次收起。
    wrapper.vm.toggleBackingIndices(row)
    expect(wrapper.vm.expandedStream).toBeNull()
    wrapper.unmount()
  })

  // 未选中服务时：水位 tab 变成全量视图——按树的层级过滤，并且把**未识别流**照旧列出来
  // （旧页的「未识别」容器；藏掉它们就等于让人看不见手工建的流和维度已删除的遗留数据）。
  it('shows the global view with unrecognized streams when no service is selected', async () => {
    const { getLogStorageOverview } = await import('@/api/monitor')
    getLogStorageOverview.mockResolvedValueOnce({ data: { data: fullOverviewPayload() } })
    const wrapper = await mountPage()
    // 选"全部业务"：不带服务，走全量
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', { nodeType: 'all', nodeTitle: '全部业务', projectIds: [], environmentIds: [] })
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    // 全量调用不带 service_code
    expect(getLogStorageOverview).toHaveBeenLastCalledWith(1, undefined)
    expect(wrapper.vm.unrecognizedStreams).toHaveLength(1)
    expect(document.body.textContent).toContain('无法按')
    expect(document.body.textContent).toContain('未识别')
    // 未识别流排在最后，且不属于任何服务，不该被当成历史档位（那需要"本服务生效档位"这个前提）。
    expect(wrapper.vm.visibleStorageRows.at(-1).name).toBe('manual-test-stream')
    expect(wrapper.vm.isHistoricalTier(wrapper.vm.visibleStorageRows.at(-1))).toBe(false)

    // 按项目过滤：只剩该项目的流（未识别的仍保留）
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', { nodeType: 'project', projectId: 15, nodeTitle: 'yilake' })
    await flushPromises()
    expect(wrapper.vm.visibleStorageRows.every((row) => !row.recognized || row.project === 'yilake')).toBe(true)
    expect(wrapper.vm.visibleStorageRows.some((row) => row.project === 'nkg')).toBe(false)
    wrapper.unmount()
  })

  // 容量视角统计：选到"全部业务"时按**项目**聚合占用（降序 + 占比），这是回答
  // "哪个项目最占盘"的地方；未识别流没有维度，不进聚合。
  it('aggregates disk usage by the next level of the selected tree node', async () => {
    const { getLogStorageOverview } = await import('@/api/monitor')
    getLogStorageOverview.mockResolvedValue({ data: { data: fullOverviewPayload() } })
    const wrapper = await mountPage()
    const tree = wrapper.findComponent({ name: 'ServiceTree' })

    // 全部业务 → 按项目
    tree.vm.$emit('select', { nodeType: 'all', nodeTitle: '全部业务', projectIds: [], environmentIds: [] })
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    expect(wrapper.vm.groupDimension.label).toBe('项目')
    const byName = Object.fromEntries(wrapper.vm.storageGroups.map((item) => [item.name, item]))
    // **每个项目都要出现**，没有日志的项目也在（"一条日志都没有"本身就是要看的信息）。
    expect(Object.keys(byName).sort()).toEqual(['KUL 项目', 'nkg', 'yilake'])
    expect(byName['KUL 项目'].streams).toBe(0)
    expect(byName['KUL 项目'].bytes).toBe(0)
    // 占用降序（有占用的在前，零占用的按名字排在后面）+ 占比
    expect(wrapper.vm.storageGroups[0].name).toBe('yilake')
    expect(byName.yilake.bytes).toBe(1054726145 + 4096)
    expect(byName.nkg.bytes).toBe(1024)
    expect(wrapper.vm.storageGroups.at(-1).name).toBe('KUL 项目')
    // 历史档位占用单独列出来（它才是"沉淀"，只看总数会以为都是活跃数据）
    expect(byName.yilake.historicalBytes).toBe(1054670278 + 4096)
    expect(byName.yilake.historicalStreams).toBe(2)
    expect(byName.yilake.unhealthy).toBe(1)
    expect(document.body.textContent).toContain('按项目统计')
    expect(document.body.textContent).toContain('无日志')
    // 未识别流不进聚合（没有项目维度），但仍列在明细里
    expect(wrapper.vm.storageGroups.some((item) => item.name === '(未设置)')).toBe(false)

    // 项目 → 按业务系统：该项目下**所有**业务系统都列出（cdm 一条流都没有）
    tree.vm.$emit('select', { nodeType: 'project', projectId: 15, nodeTitle: 'yilake' })
    await flushPromises()
    expect(wrapper.vm.groupDimension.label).toBe('业务系统')
    expect(wrapper.vm.storageGroups.map((item) => item.name)).toEqual(['TIB', 'CDM'])
    expect(Object.fromEntries(wrapper.vm.storageGroups.map((item) => [item.name, item.streams])).CDM).toBe(0)

    // 业务系统 → 按环境：环境由该业务系统名下**服务**反推（环境不挂在业务系统下）
    tree.vm.$emit('select', { nodeType: 'businessSystem', businessSystemId: 20, nodeTitle: 'CDM' })
    await flushPromises()
    expect(wrapper.vm.groupDimension.label).toBe('环境')
    // 环境显示的是名称（'生产'）而不是编码（'prod'）——与项目/业务系统/服务一致
    expect(wrapper.vm.storageGroups.map((item) => item.name)).toEqual(['生产'])

    // 环境 → 按逻辑服务：该环境下的服务全列出
    tree.vm.$emit('select', { nodeType: 'environment', businessSystemId: 19, environment: 10, nodeTitle: 'poc' })
    await flushPromises()
    expect(wrapper.vm.groupDimension.label).toBe('逻辑服务')
    // 占用降序：nginx（约 1.0 GB）> legacy（4 KB）> mgmt（1 KB）
    expect(wrapper.vm.storageGroups.map((item) => item.name)).toEqual(['nginx', 'legacy', 'mgmt'])
    // "流里有、维度表里没有"的服务要补登出来（否则这条流从统计里消失，而明细里还在）
    const legacy = wrapper.vm.storageGroups.find((item) => item.name === 'legacy')
    expect(legacy.extra).toBe(true)
    expect(document.body.textContent).toContain('维度已停用')
    wrapper.unmount()
  })

  // 饼图：option 由纯函数构造（见 util/storageUsagePie.test.js），这里只验证接线与"零值不画"。
  it('passes a pie option built from non-empty groups to the chart', async () => {
    const { getLogStorageOverview } = await import('@/api/monitor')
    getLogStorageOverview.mockResolvedValue({ data: { data: fullOverviewPayload() } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', { nodeType: 'all', nodeTitle: '全部业务', projectIds: [], environmentIds: [] })
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const pie = wrapper.findComponent({ name: 'StorageUsagePie' })
    expect(pie.exists()).toBe(true)
    const names = pie.props('option').series[0].data.map((item) => item.name)
    expect(names).toEqual(['yilake', 'nkg'])   // kul 是 0 占用，不画进饼图
    expect(pie.props('option').title.text).toBe('项目占用占比')
    wrapper.unmount()
  })

  // 加载失败必须与"确实没有日志定义"区分开：两者在界面上都是空表格（2026-09-19 现场教训）。
  it('surfaces a failed log config load instead of showing an empty table', async () => {
    const { getApplicationServiceLogConfig } = await import('@/api/assets/application')
    getApplicationServiceLogConfig.mockRejectedValueOnce({ response: { data: { msg: '资产不存在' } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(wrapper.vm.configError).toBe('资产不存在')
    expect(document.body.textContent).toContain('资产不存在')
    wrapper.unmount()
  })
})
