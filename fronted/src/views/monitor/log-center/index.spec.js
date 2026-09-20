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
    resolved_path: '/srv/tomcat/logs/application.log',
    path_pattern: '${APP_HOME}/logs/application.log', template_processing_rule_id: 91,
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
    { name: 'autoadmin-yilake-tib-poc-nginx-wuhan-test', service: 'nginx', tier: 'wuhan-test', docs: 4758176, bytes: 1054670278, ilm_state: 'hot', recognized: true, historical: true, service_enabled: true, service_collection_enabled: true },
    { name: 'autoadmin-yilake-tib-poc-nginx-hot', service: 'nginx', tier: 'hot', docs: 10, bytes: 55867, ilm_state: 'hot', recognized: true, historical: false, service_enabled: false, service_collection_enabled: true },
  ], dims: { services: [
    { id: 15, code: 'nginx', name: 'nginx', business_system: 'tib', environment: 'poc' },
  ] } } } })),
  searchElasticsearchLogs: vi.fn(() => Promise.resolve({ data: { data: { results: [], count: 0 } } })),
  searchElasticsearchLogFacetStats: vi.fn(() => Promise.resolve({ data: { data: { buckets: [] } } })),
  cleanupLogDataStream: vi.fn(),
  cleanupLogDataStreamByStream: vi.fn(),
  getLogProcessingRules: vi.fn(() => Promise.resolve({ data: { data: { results: [] } } })),
  getServiceLogConfigState: vi.fn(() => Promise.resolve({ data: { data: {
    summary: { hosts: 3, managed: 2, unmanaged: 1, synced: 1, drift: 1, never: 0, unknown: 0 },
    hosts: [
      { host_id: 10, host_ip: '10.0.0.10', host_instance_name: 'node-a', target_id: 101, config_state: 'synced', managed: true },
      { host_id: 20, host_ip: '10.0.0.20', host_instance_name: 'node-b', target_id: 102, config_state: 'drift', managed: true },
    ],
    unmanaged_hosts: [{ host_id: 30, host_ip: '10.0.0.30', host_instance_name: 'node-c', target_id: 0, managed: false }],
  } } })),
  getServiceCollectionChain: vi.fn(() => Promise.resolve({ data: { data: {
    service: { id: 15, name: 'nginx', code: 'nginx', enabled: true, log_collection_enabled: true },
    layers: [
      { key: 'agent', name: 'Agent 在线', status: 'ok', summary: '2/2 台正常', items: [] },
      { key: 'runtime', name: '采集进程', status: 'drift', summary: '1/2 台正常（1 台待处理）', items: [
        { name: 'node-a（10.0.0.10）', status: 'ok', detail: '运行中' },
        { name: 'node-b（10.0.0.20）', status: 'drift', detail: 'Filebeat 服务已停止：日志不会写入' },
      ] },
      { key: 'host_configs', name: '主机配置', status: 'drift', summary: '1/2 台正常（1 台待处理）', items: [] },
      { key: 'pipelines', name: '解析规则', status: 'ok', summary: '1 项全部一致', items: [] },
      { key: 'data_flow', name: '数据写入', status: 'warn', summary: '最近 30 分钟没有新日志写入', items: [] },
    ],
    hosts: { items: [
      { host_id: 10, host_ip: '10.0.0.10', host_instance_name: 'node-a', target_id: 101, managed: true, agent_online: true, runtime_status: 'running' },
      { host_id: 20, host_ip: '10.0.0.20', host_instance_name: 'node-b', target_id: 102, managed: true, agent_online: true, runtime_status: 'stopped' },
    ], unmanaged_items: [], summary: { total: 2 } },
    window_minutes: 30,
    checked_at: '2026-09-19T09:00:00Z',
  } } })),
  checkLogCollectionStatus: vi.fn(() => Promise.resolve({ data: { data: { exit_code: 0 } } })),
  getLogCollectionFilterRules: vi.fn(() => Promise.resolve({ data: { data: { results: [
    { id: 1, name: 'common-error', rule_type: 'include', enabled: true, pattern: '(?i)error' },
    { id: 2, name: 'drop-noise', rule_type: 'exclude', enabled: true, pattern: '(?i)healthcheck' },
  ] } } })),
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
import { formatStateTooltip } from '@/util/logFormatState'

const serviceScope = {
  nodeType: 'service', applicationServiceId: 15, nodeTitle: 'nginx',
  businessSystemName: 'TIB', environmentName: 'poc',
}

// 富载荷：含集群/数据时间/节点磁盘/维度码/未识别流——旧「存储水位」页能看到的全部信息
// （该页已并入日志中心，见迁移 000038，所以这里的字段不能少）。
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
    // 层次要分清：服务级总开关（apply-switch）、逐条日志的采集开关（表体行内）、
    // 以及「路径」列表头那个"原始/解析后"小开关（只在表头，不是行内控件）。
    expect(wrapper.findAll('.apply-switch .ant-switch')).toHaveLength(1)
    expect(wrapper.findAll('.ant-table-tbody .ant-switch')).toHaveLength(1)
    expect(wrapper.findAll('.ant-table-thead .ant-switch')).toHaveLength(1)
    // 无覆盖行 = 采：逐条开关处于勾选态（不再是"采集中"标签）。
    expect(wrapper.findAll('.ant-table-tbody .ant-switch-checked')).toHaveLength(1)
    expect(wrapper.vm.isLogCollected(wrapper.vm.logRows[0])).toBe(true)
    // 档位列是下拉：显示当前档位名。
    expect(wrapper.find('.ant-select-selection-item').text()).toBe('标准 30 天')
    expect(document.body.textContent).toContain('autoadmin-yilake-tib-poc-nginx-wuhan-test')
    wrapper.unmount()
  })

  // 日志在哪台机器上：日志配置 tab 必须给出承载主机的 IP——用户是按行看"这条日志在哪台服务器"的，
  // 只给"承载 3 台主机，1 台待下发"这种计数不够（现场反馈：找不到日志所在机器的 IP）。
  // 采集过滤改一次可能永久丢数据（被滤掉的记录进不了 ES），所以选完先弹确认、确认后才入库。
  // 其他列不弹：那是可逆的，而且每个都弹会把这一页变吵。
  it('asks for confirmation before saving a collection filter change', async () => {
    const { openDeleteConfirm } = await import('@/util/deleteConfirm')
    const { saveApplicationServiceLogSetting } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    saveApplicationServiceLogSetting.mockClear()

    // 取消：不落库（值也不会变，因为控件值由 record 派生）
    openDeleteConfirm.mockResolvedValueOnce(false)
    await wrapper.vm.confirmFilterChange(wrapper.vm.logRows[0], 'include', 1)
    await flushPromises()
    expect(saveApplicationServiceLogSetting).not.toHaveBeenCalled()
    // 确认文案要说清后果与生效方式
    const options = openDeleteConfirm.mock.calls.at(-1)[0]
    expect(options.title).toContain('保留（白名单）')
    expect(options.items.join('')).toContain('不会进 ES')
    expect(options.items.join('')).toContain('下发')

    // 确认：按行保存，且两个过滤列都带上（缺列会被清成"继承模板"）
    openDeleteConfirm.mockResolvedValueOnce(true)
    await wrapper.vm.confirmFilterChange(wrapper.vm.logRows[0], 'include', 1)
    await flushPromises()
    expect(saveApplicationServiceLogSetting).toHaveBeenCalledWith(15, expect.objectContaining({
      log_definition_id: 81, collection_filter_rule: 1,
    }))
    wrapper.unmount()
  })

  // 改完覆盖值（过滤/档位/逐条开关）要**就地重拉采集链路**：期望配置指纹变了，「主机配置」层的
  // 结论就从"一致"变成"待下发"。以前这里只重拉下发状态，链路停在旧结论上，用户只能去点
  // 「刷新运行态」——而那个按钮只是顺带重拉了链路，看起来像"运行态需要刷新"（2026-09-19 现场）。
  it('reloads the collection chain right after an override is saved', async () => {
    const { openDeleteConfirm } = await import('@/util/deleteConfirm')
    const { getServiceCollectionChain, checkLogCollectionStatus } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    getServiceCollectionChain.mockClear()
    checkLogCollectionStatus.mockClear()

    openDeleteConfirm.mockResolvedValueOnce(true)
    await wrapper.vm.confirmFilterChange(wrapper.vm.logRows[0], 'exclude', 2)
    await flushPromises()

    expect(getServiceCollectionChain).toHaveBeenCalledWith(15)
    // 重拉链路 ≠ 去查主机：链路是读库+读 ES，不该因为改了个过滤就往主机上打命令。
    expect(checkLogCollectionStatus).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 一致性：这一页**每个**可写控件都先确认再入库。分开写用例是因为"只给一部分加确认"
  // 正是之前的毛病——用户看到有的弹有的不弹，就会怀疑哪些改动真的保存了。
  it('asks for confirmation on every editable control in the log config tab', async () => {
    const { openDeleteConfirm } = await import('@/util/deleteConfirm')
    const { saveApplicationServiceLogSetting, setApplicationServiceLogCollection } = await import('@/api/assets/application')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    saveApplicationServiceLogSetting.mockClear()
    setApplicationServiceLogCollection.mockClear()

    // 逐条采集开关（关闭）
    openDeleteConfirm.mockClear()
    await wrapper.vm.confirmLogCollectChange(wrapper.vm.logRows[0], false)
    expect(openDeleteConfirm).toHaveBeenCalledTimes(1)
    expect(saveApplicationServiceLogSetting).toHaveBeenCalledTimes(1)

    // 保留档位
    openDeleteConfirm.mockClear()
    await wrapper.vm.confirmTierChange(wrapper.vm.logRows[0], 4)
    expect(openDeleteConfirm).toHaveBeenCalledTimes(1)
    expect(openDeleteConfirm.mock.calls.at(-1)[0].title).toContain('保留档位')
    expect(openDeleteConfirm.mock.calls.at(-1)[0].items.join('')).toContain('不迁移数据')

    // 采集过滤（两个方向同一个确认入口）
    openDeleteConfirm.mockClear()
    await wrapper.vm.confirmFilterChange(wrapper.vm.logRows[0], 'exclude', 2)
    expect(openDeleteConfirm).toHaveBeenCalledTimes(1)
    expect(openDeleteConfirm.mock.calls.at(-1)[0].title).toContain('排除（黑名单）')

    // 服务采集总开关
    openDeleteConfirm.mockClear()
    await wrapper.vm.toggleServiceCollect(false)
    expect(openDeleteConfirm).toHaveBeenCalledTimes(1)
    expect(setApplicationServiceLogCollection).toHaveBeenCalledWith(15, false)

    // 打开逐条开关（回到默认）不需要确认：它不会让任何东西停止采集。
    openDeleteConfirm.mockClear()
    await wrapper.vm.confirmLogCollectChange(wrapper.vm.logRows[0], true)
    expect(openDeleteConfirm).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 「路径」列的小开关：原始模式 ↔ 解析后。默认解析后（人想看的是"实际落在哪"），
  // 但排查"这个宏是哪一层给的"时要能看回原始 —— 两页共用 util/logPathMacro，口径一致。
  it('toggles the path column between the raw pattern and the resolved path', async () => {
    // 不依赖用例顺序：别的用例会 mockResolvedValue 覆盖日志配置（且不清除），这里显式给这一条日志定义，
    // 跑完再把模块级 mock 还原回去（否则会污染后面的用例）。
    const { getApplicationServiceLogConfig } = await import('@/api/assets/application')
    const original = getApplicationServiceLogConfig.getMockImplementation()
    getApplicationServiceLogConfig.mockResolvedValue({ data: { data: { logs: [{
      log_definition: 81,
      name: 'access.log',
      resolved_path: '/srv/tomcat/logs/application.log',
      path_pattern: '${APP_HOME}/logs/application.log',
      collection_enabled: null,
      template_processing_rule_id: 91,
      template_processing_rule_name: 'tomcat rule',
      retention_tier: 3,
      format_state: 'unverified',
      data_stream: 'autoadmin-yilake-tib-poc-nginx-wuhan-test',
    }] } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(wrapper.vm.showResolvedPath).toBe(true)
    expect(document.body.textContent).toContain('/srv/tomcat/logs/application.log')
    wrapper.vm.showResolvedPath = false
    await flushPromises()
    expect(document.body.textContent).toContain('${APP_HOME}/logs/application.log')
    expect(document.body.textContent).not.toContain('/srv/tomcat/logs/application.log')
    wrapper.unmount()
    getApplicationServiceLogConfig.mockImplementation(original)
  })

  // 采集链路：查不到日志时按层回答"断在哪"。判定来自后端（与链路体检同源），前端只呈现，
  // 所以这里验证的是接线与呈现：五层都在、异常层的状态带出来、明细进 tooltip。
  it('renders the collection chain layers with the failing layer visible', async () => {
    const { getServiceCollectionChain } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(getServiceCollectionChain).toHaveBeenCalledWith(15)
    const text = document.body.textContent
    for (const layerName of ['Agent 在线', '采集进程', '主机配置', '解析规则', '数据写入']) {
      expect(text).toContain(layerName)
    }
    // 断点要一眼可见：Filebeat 已停止的那台说明必须在明细里，而不是只在汇总计数里。
    const runtimeLines = wrapper.vm.chainLayerTooltipLines(wrapper.vm.chainLayers[1])
    expect(runtimeLines.join('\n')).toContain('Filebeat 服务已停止')
    expect(runtimeLines.join('\n')).toContain('node-b（10.0.0.20）')
    // 异常层的 class 决定了颜色，class 名是"状态 → 颜色"的唯一映射。
    expect(wrapper.findAll('.chain-layer.chain-drift')).toHaveLength(2)
    wrapper.unmount()
  })

  // 刷新运行态：落库的 Filebeat 状态是快照，只有查过才新鲜。查的是"已纳管 + agent 在线"的主机，
  // 未纳管的主机没有目标 id，查不了也不该查（后端会 400）。
  it('refreshes filebeat runtime status per managed host and reloads the chain', async () => {
    const { checkLogCollectionStatus, getServiceCollectionChain } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    getServiceCollectionChain.mockClear()

    await wrapper.vm.refreshChainRuntime()

    expect(checkLogCollectionStatus).toHaveBeenCalledTimes(2)
    expect(checkLogCollectionStatus).toHaveBeenCalledWith(101)
    expect(checkLogCollectionStatus).toHaveBeenCalledWith(102)
    // 查完要重拉链路：状态文案由后端生成，前端不自己拼（否则会和体检说法不一致）。
    expect(getServiceCollectionChain).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  // 状态未知（新纳管、刚下发重启过）时**自动查一次**，不必再手点「刷新运行态」。
  // 现场反馈："日志配置里每次修改了东西都要手动刷新运行态，麻烦"（2026-09-19）。
  it('auto-refreshes filebeat status when a host has never been checked', async () => {
    const { getServiceCollectionChain, checkLogCollectionStatus } = await import('@/api/monitor')
    getServiceCollectionChain.mockResolvedValueOnce({ data: { data: {
      service: { id: 15, name: 'nginx', code: 'nginx', enabled: true, log_collection_enabled: true },
      layers: [],
      hosts: { items: [
        { host_id: 10, host_ip: '10.0.0.10', host_instance_name: 'node-a', target_id: 101, managed: true, agent_online: true, runtime_status: '' },
      ], unmanaged_items: [], summary: { total: 1 } },
      window_minutes: 30,
      checked_at: '2026-09-19T09:00:00Z',
    } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(checkLogCollectionStatus).toHaveBeenCalledTimes(1)
    expect(checkLogCollectionStatus).toHaveBeenCalledWith(101)
    // 查完重拉链路（状态文案由后端生成）；此刻状态已有值，不会反复自动查。
    expect(getServiceCollectionChain.mock.calls.length).toBeGreaterThanOrEqual(2)
    wrapper.unmount()
  })

  // 状态已知时**不许打扰主机**：自动刷新不能变成"每打开一次页面就把全网主机 systemctl 一遍"。
  it('does not touch hosts when the runtime status is already known', async () => {
    const { checkLogCollectionStatus } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(checkLogCollectionStatus).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 下发会**重启 Filebeat**，重启后的进程态谁都不知道 → 作业一结束就自动查一次。
  // 这正是"改完还要手动刷新"的那一步：以前作业结束只重拉配置态，运行态还是重启前的旧快照。
  it('refreshes filebeat status right after the delivery job finishes', async () => {
    const { checkLogCollectionStatus } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    checkLogCollectionStatus.mockClear()

    await wrapper.vm.applyService()
    await flushPromises()

    // 默认夹具里两台"已纳管 + agent 在线"的主机各查一次（未纳管的 target_id 为空，查不了）。
    expect(checkLogCollectionStatus).toHaveBeenCalledTimes(2)
    expect(checkLogCollectionStatus).toHaveBeenCalledWith(101)
    expect(checkLogCollectionStatus).toHaveBeenCalledWith(102)
    wrapper.unmount()
  })

  // 服务停用 / 服务级采集总开关关闭不在五层里，但它是"查不到日志"最常见的原因，必须显式提示。
  it('calls out a stopped service as the reason for missing logs', async () => {
    const { getServiceCollectionChain } = await import('@/api/monitor')
    getServiceCollectionChain.mockResolvedValueOnce({ data: { data: {
      service: { id: 15, name: 'nginx', code: 'nginx', enabled: false, log_collection_enabled: true },
      layers: [], hosts: { items: [], unmanaged_items: [] }, window_minutes: 30,
    } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(wrapper.vm.chainServiceStopReason).toContain('该逻辑服务已停用')
    expect(document.body.textContent).toContain('该逻辑服务已停用')
    wrapper.unmount()
  })

  // 「未验证」标红：没验证就采集属于静默坏数据（日志查得到但级别/消息列为空、关键词搜不到、
  // 错误清单失效）。灰色标签混在表格里会被忽略，所以标签颜色就是"要不要人去处理"。
  it('marks an unverified log format in red and tells the admin how to fix it', async () => {
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    const tag = wrapper.findAll('.ant-table .ant-tag').find((node) => node.text() === '未验证')
    expect(tag, '未验证标签应该渲染出来').toBeTruthy()
    expect(tag.classes()).toContain('ant-tag-red')
    // 光红不够：要说清后果与下一步动作（点哪一行、点哪个按钮）。
    const tooltip = formatStateTooltip(wrapper.vm.logRows[0])
    expect(tooltip).toContain('静默坏数据')
    expect(tooltip).toContain('发起认证')
    wrapper.unmount()
  })

  it('shows the carrying hosts with their IPs in the log config tab', async () => {
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    // 首台完整显示（主机实例名 + IP），其余折叠成"N 台"，全部明细进 tooltip。
    expect(wrapper.vm.carryingHostsSummary).toBe('node-a（10.0.0.10） 等 3 台')
    expect(document.body.textContent).toContain('node-a（10.0.0.10）')
    // 未纳管的主机也要列出来并标注，否则用户以为这个服务就这几台机器（它与"下发不到"直接相关）。
    expect(wrapper.vm.carryingHostsTooltip).toContain('node-b（10.0.0.20）')
    expect(wrapper.vm.carryingHostsTooltip).toContain('node-c（10.0.0.30）（未纳管，配置下发不到）')
    wrapper.unmount()
  })

  // 水位只在切到那个 tab 时读，并且**必须带 application_service_id**：后端据此把 ES 查询收窄到本服务的索引，
  // 少了它就会退化成"取全集群再前端过滤"（页面看起来一样，代价差很多）。用 id 而非 code：
  // 逻辑服务编码现在允许跨业务/环境重复，只凭 code 会命中错的维度段。
  it('loads the water level lazily with application_service_id so the backend can narrow the ES query', async () => {
    const { getLogStorageOverview } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()

    expect(getLogStorageOverview).not.toHaveBeenCalled()

    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    expect(getLogStorageOverview).toHaveBeenCalledWith(1, { application_service_id: 15 })
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

    // 仍然按服务收窄查了水位（用的是选中的服务 id，而不是 logs[0]）
    expect(getLogStorageOverview).toHaveBeenCalledWith(1, { application_service_id: 15 })
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

    // 提交体是该行覆盖值的**全集**：两个采集过滤列也要原样带上，缺了会被清成"继承模板"。
    expect(saveApplicationServiceLogSetting).toHaveBeenCalledWith(15, {
      log_definition_id: 81, retention_tier: 4, collection_enabled: null,
      collection_filter_rule: null, collection_exclude_filter_rule: null,
    })
    // 档位改完要重新读配置/水位/下发状态（档位变了 → 渲染内容变 → 变成待下发）。
    expect(wrapper.vm.switchTierOpen).toBe(false)
    wrapper.unmount()
  })

  // 清理入口（2026-09-19）：改成**每条流**都能清，弹窗也换成与「日志查询」原来那套一致的
  // 共享组件（范围可选 保留 N 小时 / N 天 / 全部清空）。日志查询面板里的清理按钮已删除，
  // 清理入口统一在数据流列表（这里）。
  //
  // 分流规则：能归属到逻辑服务 → 服务维度（带 tier，只清这一条流）；未识别流 → 按流名。
  async function openCleanupDialog(wrapper, stream) {
    wrapper.vm.cleanupStream(stream)
    await flushPromises()
    return wrapper.findComponent({ name: 'LogCleanupDialog' })
  }

  it('cleans only the historical stream through the service-scoped path', async () => {
    const { cleanupLogDataStream } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const historical = wrapper.vm.storageRows.find((row) => row.tier === 'wuhan-test')
    const dialog = await openCleanupDialog(wrapper, historical)

    // 弹窗必须点名清理对象（否则用户不知道点的是哪条流），且默认是"保留最近 7 天"。
    expect(dialog.vm.targetLabel).toContain('autoadmin-yilake-tib-poc-nginx-wuhan-test')
    expect(dialog.vm.mode).toBe('days')
    expect(dialog.vm.amount).toBe(7)

    // 未确认前不发请求；选了"全部清空"后按 (服务, 档位) 清理。
    expect(cleanupLogDataStream).not.toHaveBeenCalled()
    dialog.vm.mode = 'all'
    await dialog.vm.submit()
    await flushPromises()
    expect(cleanupLogDataStream).toHaveBeenCalledWith({ service_id: 15, mode: 'all', amount: 0, tier: 'wuhan-test' })
    expect(wrapper.vm.cleanupOpen).toBe(false)
    wrapper.unmount()
  })

  // 时间窗（保留最近 N 天）必须真的传下去：这是"只清旧数据、留最近"的唯一手段。
  it('passes the keep-recent window through to the cleanup API', async () => {
    const { cleanupLogDataStream } = await import('@/api/monitor')
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const active = wrapper.vm.storageRows.find((row) => row.name.endsWith('-hot'))
    const dialog = await openCleanupDialog(wrapper, active)
    dialog.vm.mode = 'days'
    dialog.vm.amount = 30
    await dialog.vm.submit()
    await flushPromises()

    expect(cleanupLogDataStream).toHaveBeenCalledWith({ service_id: 15, mode: 'days', amount: 30, tier: 'hot' })
    wrapper.unmount()
  })

  it('cleans an active stream and says that collection continues', async () => {
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const active = wrapper.vm.storageRows.find((row) => row.name.endsWith('-hot'))
    expect(wrapper.vm.streamCleanupTarget(active)).toEqual({ kind: 'service', serviceId: 15, tier: 'hot' })
    const dialog = await openCleanupDialog(wrapper, active)
    // 活跃流最容易被误解成"停止采集"：弹窗里必须写清"清理不停采集"（提示文字在正文里，断言渲染结果）。
    expect(document.body.textContent).toContain('不会停止采集')
    expect(dialog.vm.title).toBe('清理日志数据')
    wrapper.unmount()
  })

  it('cleans an unrecognized stream by its name', async () => {
    const { cleanupLogDataStream, cleanupLogDataStreamByStream, getLogStorageOverview } = await import('@/api/monitor')
    // 未识别流单独注入（默认夹具不塞它，免得改坏"数流数量"的既有用例）。
    getLogStorageOverview.mockResolvedValueOnce({ data: { data: {
      data_streams: [
        { name: 'autoadmin-nkg-tib-prod-oldservice-std', tier: 'std', docs: 4200, bytes: 33100000, ilm_state: 'hot', recognized: false, historical: false },
      ],
      dims: { services: [] },
    } } })
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const unrecognized = wrapper.vm.storageRows.find((row) => !row.recognized)
    expect(unrecognized).toBeTruthy()
    expect(wrapper.vm.streamCleanupTarget(unrecognized)).toEqual({ kind: 'stream', stream: unrecognized.name })
    const dialog = await openCleanupDialog(wrapper, unrecognized)
    expect(dialog.vm.alertDescription).toContain('未识别流')
    dialog.vm.mode = 'all'
    await dialog.vm.submit()
    await flushPromises()

    expect(cleanupLogDataStreamByStream).toHaveBeenCalledWith({ stream: unrecognized.name, mode: 'all', amount: 0 })
    expect(cleanupLogDataStream).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 用户反馈（2026-09-19）：全量视图 / 项目 / 业务系统 / 环境节点下**整列操作都不在**——
  // 因为列定义曾经按"是否选中服务"开关，而之前那个用例只调了 vm 上的函数，没验渲染出来的列。
  // 这里改成从 DOM 断言：列头在、按钮在、点得动；并确认"切回该档位"只在选中服务时出现。
  it('renders the actions column on every stream row in the full view too', async () => {
    const wrapper = await mountPage()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    // 页面上有多张表（磁盘水位 / 按维度聚合 / 流表），必须挑**流表**来断言：
    // 之前那次漏检就是因为只看了 vm 上的函数，没确认渲染出来的到底是哪张表。
    const streamTable = wrapper.findAll('table').find((table) => table.find('thead').text().includes('操作'))
    expect(streamTable, '流表的表头里必须有「操作」列').toBeTruthy()
    const streamRows = streamTable.findAll('tbody tr').filter((row) => row.text().trim() !== '')
    expect(streamRows.length).toBeGreaterThan(0)
    // 全量视图下每行都有清理入口（历史档位流与在写的流都在）。
    expect(streamRows.every((row) => row.text().includes('清理'))).toBe(true)
    // 「切回该档位」需要服务上下文（要改该服务的日志定义），全量视图下不给。
    expect(wrapper.text()).not.toContain('切回该档位')
    wrapper.unmount()
  })

  it('offers 切回该档位 only when a service is selected', async () => {
    const wrapper = await mountPage()
    wrapper.findComponent({ name: 'ServiceTree' }).vm.$emit('select', serviceScope)
    await flushPromises()
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    expect(wrapper.text()).toContain('切回该档位')
    wrapper.unmount()
  })

  it('cleans from the all-streams view too (no service selected)', async () => {
    const { cleanupLogDataStream } = await import('@/api/monitor')
    const wrapper = await mountPage()
    // 不选服务（全量视图）——这正是用户要的"全部 data stream 也能清"。
    wrapper.vm.activeTab = 'storage'
    await flushPromises()

    const historical = wrapper.vm.storageRows.find((row) => row.historical)
    // 服务 id 由响应里的 dims 映射得到（不依赖左侧选中的是谁）。
    expect(wrapper.vm.streamCleanupTarget(historical)).toEqual({ kind: 'service', serviceId: 15, tier: 'wuhan-test' })
    const dialog = await openCleanupDialog(wrapper, historical)
    dialog.vm.mode = 'all'
    await dialog.vm.submit()
    await flushPromises()
    expect(cleanupLogDataStream).toHaveBeenCalledWith({ service_id: 15, mode: 'all', amount: 0, tier: 'wuhan-test' })
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
      collection_filter_rule: null, collection_exclude_filter_rule: null,
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
      collection_filter_rule: null, collection_exclude_filter_rule: null,
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

  // 集群级信息必须在日志中心完整保留：节点磁盘（盘快满了比任何单条流都紧急）、
  // 数据时间与集群前缀（水位是"某集群某时刻"的快照，不给这两项数字没法解读）。
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
