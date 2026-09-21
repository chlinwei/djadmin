import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'

import LogQueryPanel from './LogQueryPanel.vue'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))

// echarts 在 jsdom 里没有真实 canvas 渲染能力，这里只关注统计面板的数据流转，图表渲染细节不测。
vi.mock('echarts', () => ({
  init: vi.fn(() => ({ setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() })),
}))

const CLUSTER = { id: 7, name: '日志集群', is_default: true }

const LOG_DOC = {
  id: 'doc-1',
  '@timestamp': '2026-08-30T10:00:00Z',
  log_level: 'ERROR',
  service: 'tomcat-svc',
  instance: 'kul-tib-tomcat1',
  host_ip: '10.0.0.1',
  log_name: 'catalina',
  log_path: '/home/esb/tomcat/logs/catalina.out',
  log_message: 'NullPointerException',
  error_fingerprint: 'fp-1',
  app_fields: { logger: 'org.apache' },
}

const FACET_BUCKET = {
  value: 'fp-1',
  count: 5,
  sample: { ...LOG_DOC },
  trend: [{ timestamp: '2026-08-30T10:00:00Z', count: 5 }],
}

const LEVEL_BUCKETS = [
  { value: 'ERROR', count: 10, sample: { ...LOG_DOC }, trend: [] },
  { value: 'WARN', count: 3, sample: { ...LOG_DOC, log_level: 'WARN' }, trend: [] },
]

const getElasticsearchClusterList = vi.fn(() => Promise.resolve({ data: { data: { results: [CLUSTER], count: 1 } } }))
const searchElasticsearchLogs = vi.fn(() => Promise.resolve({ data: { data: { results: [LOG_DOC], count: 1, size: 100, offset: 0 } } }))
// 日志级别下拉复用同一个分面统计接口，按 field 区分返回真实级别值还是统计 tab 的指纹分面。
const searchElasticsearchLogFacetStats = vi.fn((id, params) => {
  if (params.field === 'log_level') {
    return Promise.resolve({ data: { data: { field: 'log_level', interval_minutes: 1, buckets: LEVEL_BUCKETS } } })
  }
  return Promise.resolve({ data: { data: { field: params.field, interval_minutes: 5, buckets: [FACET_BUCKET] } } })
})

vi.mock('@/api/monitor', () => ({
  getElasticsearchClusterList: (...args) => getElasticsearchClusterList(...args),
  searchElasticsearchLogs: (...args) => searchElasticsearchLogs(...args),
  searchElasticsearchLogFacetStats: (...args) => searchElasticsearchLogFacetStats(...args),
}))

function mountPanel(scope = { nodeType: 'all', nodeTitle: '全部业务' }) {
  return mount(LogQueryPanel, {
    props: { scope },
    global: {
      plugins: [Antd],
      stubs: { FontAwesomeIcon: true },
    },
    attachTo: document.body,
  })
}

describe('LogQueryPanel（日志中心的日志查询面板）', () => {
  it('未选择服务时展示空状态，不发起日志查询', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('请选择左侧逻辑服务或部署实例查看日志')
    expect(searchElasticsearchLogs).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('选中逻辑服务后按服务 id 查询并渲染结果', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.setProps({
      scope: {
        nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务',
        businessSystemName: 'TIB', environmentName: '测试',
      },
    })
    await flushPromises()

    expect(getElasticsearchClusterList).toHaveBeenCalled()
    expect(searchElasticsearchLogs).toHaveBeenCalledWith(
      CLUSTER.id,
      expect.objectContaining({ application_service_id: 42 }),
    )
    expect(wrapper.text()).toContain('NullPointerException')
    wrapper.unmount()
  })

  // 回归（2026-09-21 现场）：在服务 A 输入关键词、选了级别、或从统计面板下钻后，点左侧切到服务 B，
  // 这些条件会原样带过去（统计下钻的 host_ip/指纹过滤同样残留），页面看起来像"B 查不到日志"。
  // 切换节点必须清空上一个服务的过滤条件，只有 scope 本身决定范围。
  it('切换到另一个逻辑服务时清空关键词、级别与下钻过滤，不带到新服务', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    wrapper.vm.filters.keyword = 'NullPointerException'
    wrapper.vm.filters.logLevels = ['ERROR']
    wrapper.vm.filters.hostIp = '10.0.0.1'
    wrapper.vm.filters.errorFingerprint = 'fp-1'
    await flushPromises()

    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 43, nodeTitle: 'nginx服务' } })
    await flushPromises()

    expect(wrapper.vm.filters.keyword).toBe('')
    expect(wrapper.vm.filters.logLevels).toEqual([])
    expect(wrapper.vm.filters.hostIp).toBe('')
    expect(wrapper.vm.filters.logName).toBe('')
    expect(wrapper.vm.filters.logPath).toBe('')
    expect(wrapper.vm.filters.errorFingerprint).toBe('')

    const params = searchElasticsearchLogs.mock.calls.at(-1)[1]
    expect(params.application_service_id).toBe(43)
    expect(params.keyword).toBeUndefined()
    expect(params.log_level).toBeUndefined()
    expect(params.host_ip).toBeUndefined()
    expect(params.error_fingerprint).toBeUndefined()
    wrapper.unmount()
  })

  // 关键词框：框里写着 Lucene 就必须支持 Lucene（2026-09-19 现场：用户按提示写
  // `host_ip: "192.168.201.209"` 查不到——后端为了"只搜正文"把冒号转义了）。
  // 现在两种模式可切换，且默认落在"只搜正文"这个更安全的一侧。
  it('关键词框默认「正文」模式，切到 Lucene 后把模式一起发出去', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    // 默认：正文模式（提示词也必须是正文的口径，不能写着 Lucene 却只搜正文）
    expect(wrapper.vm.filters.keywordMode).toBe('message')
    expect(wrapper.text()).toContain('正文模式')
    // 切换控件必须是**实心按钮组**（a-segmented 的选中态在这个主题下看不出来，用户反馈过），
    // 且选中项由 v-model 决定——渲染层要能直接读出"哪个在用"。
    const modeGroup = wrapper.find('.keyword-mode-row .ant-radio-group')
    expect(modeGroup.exists()).toBe(true)
    expect(modeGroup.find('.ant-radio-button-wrapper-checked').text()).toBe('正文')

    wrapper.vm.filters.keyword = 'host_ip:"192.168.201.209"'
    wrapper.vm.filters.keywordMode = 'lucene'
    await wrapper.vm.handleFilterChange()
    await flushPromises()

    const params = searchElasticsearchLogs.mock.calls.at(-1)[1]
    expect(params.keyword).toBe('host_ip:"192.168.201.209"')
    expect(params.keyword_mode).toBe('lucene')
    // 说明文案随模式切换：Lucene 模式要告诉用户"字段过滤可用"
    expect(wrapper.text()).toContain('Lucene 模式')
    expect(wrapper.text()).toContain('支持字段过滤')
    await wrapper.vm.$nextTick()
    expect(modeGroup.find('.ant-radio-button-wrapper-checked').text()).toBe('Lucene')
    wrapper.unmount()
  })

  it('没有关键词时不带 keyword_mode（省得给后端传一个没意义的参数）', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    const params = searchElasticsearchLogs.mock.calls.at(-1)[1]
    expect(params.keyword).toBeUndefined()
    expect(params.keyword_mode).toBeUndefined()
    wrapper.unmount()
  })

  it('默认预填最近 1 小时，查询范围可见且实际发起带时间的请求', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    expect(wrapper.text()).toContain('当前查询范围：')
    expect(wrapper.text()).not.toContain('未选择时的默认范围')
    const params = searchElasticsearchLogs.mock.calls.at(-1)[1]
    expect(params.start).toEqual(expect.any(String))
    expect(params.end).toEqual(expect.any(String))
    wrapper.unmount()
  })

  it('日志级别下拉从当前服务的真实数据动态生成，而不是写死的列表', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    expect(searchElasticsearchLogFacetStats).toHaveBeenCalledWith(
      CLUSTER.id,
      expect.objectContaining({ application_service_id: 42, field: 'log_level' }),
    )
    expect(wrapper.vm.logLevelOptions).toEqual([
      { label: 'ERROR', value: 'ERROR' },
      { label: 'WARN', value: 'WARN' },
    ])
    wrapper.unmount()
  })

  it('切换时间范围等过滤条件后，日志级别下拉联动重新拉取，不会停留在最初的空结果', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    const levelCallsBefore = searchElasticsearchLogFacetStats.mock.calls.filter((call) => call[1]?.field === 'log_level').length
    expect(levelCallsBefore).toBeGreaterThan(0)

    wrapper.vm.filters.timeRange = [wrapper.vm.filters.timeRange[0], wrapper.vm.filters.timeRange[1]]
    wrapper.vm.handleFilterChange()
    await flushPromises()

    const levelCallsAfter = searchElasticsearchLogFacetStats.mock.calls.filter((call) => call[1]?.field === 'log_level').length
    expect(levelCallsAfter).toBeGreaterThan(levelCallsBefore)
    wrapper.unmount()
  })

  it('选中部署实例后自动锁定实例过滤条件', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.setProps({
      scope: {
        nodeType: 'deployment', applicationServiceId: 42, nodeTitle: 'kul-tib-tomcat1',
        businessSystemName: 'TIB', environmentName: '测试',
      },
    })
    await flushPromises()

    expect(searchElasticsearchLogs).toHaveBeenCalledWith(
      CLUSTER.id,
      expect.objectContaining({ instance: 'kul-tib-tomcat1' }),
    )
    wrapper.unmount()
  })

  it('日志详情展示日志路径字段，时间显示到毫秒', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    wrapper.vm.openDetail(LOG_DOC)
    await flushPromises()

    // a-drawer 通过 Teleport 挂载到 document.body，不在 wrapper 根节点子树内，需要直接查 body 文本。
    expect(document.body.textContent).toContain('日志路径')
    expect(document.body.textContent).toContain('/home/esb/tomcat/logs/catalina.out')
    // @timestamp=2026-08-30T10:00:00Z + Asia/Shanghai → 18:00:00.000（毫秒精度）
    expect(document.body.textContent).toContain('2026-08-30 18:00:00.000')
    wrapper.unmount()
  })

  it('切换到统计 tab 后按当前统计维度查询', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    wrapper.vm.handleTabChange('stats')
    await flushPromises()

    expect(searchElasticsearchLogFacetStats).toHaveBeenCalledWith(
      CLUSTER.id,
      expect.objectContaining({ application_service_id: 42, field: 'error_fingerprint' }),
    )
    expect(wrapper.text()).toContain('fp-1')
    wrapper.unmount()
  })

  it('默认分桶粒度为自动，不向后端传 interval_minutes', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()
    wrapper.vm.handleTabChange('stats')
    await flushPromises()

    const params = searchElasticsearchLogFacetStats.mock.calls.at(-1)[1]
    expect(params.interval_minutes).toBeUndefined()
    wrapper.unmount()
  })

  it('手动选择分桶粒度后按分钟数传给后端', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()
    wrapper.vm.handleTabChange('stats')
    await flushPromises()

    wrapper.vm.statsIntervalOption = '15'
    await wrapper.vm.loadStats()
    await flushPromises()

    expect(searchElasticsearchLogFacetStats).toHaveBeenLastCalledWith(
      CLUSTER.id,
      expect.objectContaining({ interval_minutes: '15' }),
    )
    wrapper.unmount()
  })

  it('统计面板点击查看日志后写回过滤条件并切回日志查询 tab', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()
    wrapper.vm.handleTabChange('stats')
    await flushPromises()

    wrapper.vm.drillDown(FACET_BUCKET)
    await flushPromises()

    expect(wrapper.vm.activeTab).toBe('logs')
    expect(wrapper.vm.filters.errorFingerprint).toBe('fp-1')
    expect(searchElasticsearchLogs).toHaveBeenLastCalledWith(
      CLUSTER.id,
      expect.objectContaining({ error_fingerprint: 'fp-1' }),
    )
    wrapper.unmount()
  })

  // 统计维度支持「日志路径」：下钻要把具体文件写回过滤条件并带给后端（后端 buildLogQuery 支持 log_path）。
  it('按日志路径统计后下钻会按具体文件过滤', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()
    wrapper.vm.handleTabChange('stats')
    await flushPromises()

    wrapper.vm.statsField = 'log_path'
    wrapper.vm.drillDown({ value: '/home/esb/data/logs/app1/log_error.log' })
    await flushPromises()

    expect(wrapper.vm.filters.logPath).toBe('/home/esb/data/logs/app1/log_error.log')
    expect(searchElasticsearchLogs).toHaveBeenLastCalledWith(
      CLUSTER.id,
      expect.objectContaining({ log_path: '/home/esb/data/logs/app1/log_error.log' }),
    )
    wrapper.unmount()
  })

  it('reload 会重新拉取级别下拉和当前 tab 数据（面板自带的刷新按钮会调用）', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    const callsBefore = searchElasticsearchLogs.mock.calls.length
    await wrapper.vm.reload()
    await flushPromises()

    expect(searchElasticsearchLogs.mock.calls.length).toBeGreaterThan(callsBefore)
    wrapper.unmount()
  })
})
