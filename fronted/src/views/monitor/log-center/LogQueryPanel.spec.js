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
  message: '2026-08-30 18:00:00 ERROR [main] com.esb.App - NullPointerException',
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
  it('范围内没有逻辑服务时展示空状态，不发起日志查询', async () => {
    // 跨服务模式下候选集为空（范围内没有服务）同样是"查不了"：文案要说清为什么。
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('当前范围内没有可查询的逻辑服务')
    expect(searchElasticsearchLogs).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  // 上层节点的任务形态（2026-09-28，方案 A）：默认「概览」——按逻辑服务聚合，
  // 范围由左树决定，面板内不再有范围选择器；点某服务分面「查看日志」下钻到明细并收窄。
  it('上层节点默认日志：统计按逻辑服务聚合，下钻写入 service 过滤并切日志', async () => {
    searchElasticsearchLogFacetStats.mockImplementation((id, params) => {
      if (params.field === 'service') {
        return Promise.resolve({ data: { data: { field: 'service', interval_minutes: 5, buckets: [
          { value: 'tomcat', count: 9, sample: { ...LOG_DOC }, trend: [] },
        ] } } })
      }
      return Promise.resolve({ data: { data: { field: params.field, interval_minutes: 1, buckets: LEVEL_BUCKETS } } })
    })
    const wrapper = mountPanel({ nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: 'TIB' })
    await wrapper.setProps({
      candidateServices: [
        { id: 42, code: 'tomcat', name: 'tomcat服务', business_system: 7, environment: 10, environment_name: 'poc' },
        { id: 43, code: 'nginx', name: 'nginx服务', business_system: 7, environment: 10, environment_name: 'poc' },
      ],
    })
    await flushPromises()

    // 默认日志，统计维度按逻辑服务分组；范围由左树决定。
    expect(wrapper.vm.activeTab).toBe('logs')
    expect(wrapper.vm.statsField).toBe('service')
    expect(wrapper.vm.innerTabs.map((tab) => tab.label)).toEqual(['日志', '统计'])
    wrapper.vm.handleTabChange('stats')
    await flushPromises()
    expect(searchElasticsearchLogFacetStats).toHaveBeenCalledWith(
      CLUSTER.id,
      expect.objectContaining({ field: 'service', application_service_ids: '42,43' }),
    )
    // 概览的「值」列把服务编码显示成名称。
    expect(wrapper.vm.statsValueDisplay({ value: 'tomcat' })).toBe('tomcat服务')

    // 下钻：把分面值写入过滤条件、切到明细并带 service 过滤重查。
    wrapper.vm.drillDown({ value: 'tomcat' })
    await flushPromises()
    expect(wrapper.vm.filters.service).toBe('tomcat')
    expect(wrapper.vm.activeTab).toBe('logs')
    expect(searchElasticsearchLogs).toHaveBeenCalledWith(
      CLUSTER.id,
      expect.objectContaining({ service: 'tomcat', application_service_ids: '42,43' }),
    )
    // 明细多一列「逻辑服务」（编码 → 名称反查）。
    expect(wrapper.vm.columns.some((column) => column.key === 'service_name')).toBe(true)
    expect(wrapper.vm.columns.find((column) => column.key === 'service_name').customRender({ record: { service: 'nginx' } }))
      .toBe('nginx服务')
    wrapper.unmount()
  })

  // 服务/实例节点保持"日志优先"：默认日志 tab、默认按错误指纹分组。
  it('服务节点默认日志视图，统计维度默认错误指纹', async () => {
    const wrapper = mountPanel({ nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' })
    await flushPromises()
    expect(wrapper.vm.activeTab).toBe('logs')
    expect(wrapper.vm.statsField).toBe('error_fingerprint')
    expect(wrapper.vm.innerTabs.map((tab) => tab.label)).toEqual(['日志', '统计'])
    wrapper.unmount()
  })

  // 下钻过滤行必须覆盖全部可下钻维度：按 逻辑服务/业务系统/环境 下钻后若整行不渲染，
  // 条件就"看不见也叉不掉"（现场反馈）。这里钉住：显示 + 可移除。
  it('按范围维度下钻后「下钻过滤」可见且可移除', async () => {
    const wrapper = mountPanel({ nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: 'TIB' })
    await wrapper.setProps({
      candidateServices: [
        { id: 42, code: 'tomcat', name: 'tomcat服务', business_system: 7, environment: 10, environment_name: 'poc' },
      ],
    })
    await flushPromises()

    wrapper.vm.drillDown({ value: 'tomcat' })
    await flushPromises()
    expect(wrapper.vm.filters.service).toBe('tomcat')
    expect(wrapper.vm.hasDrillFilters).toBe(true)
    expect(wrapper.text()).toContain('下钻过滤')
    expect(wrapper.text()).toContain('逻辑服务 = tomcat服务')

    // 点标签上的关闭：条件清空、行随之隐藏。
    wrapper.vm.clearDrillFilter('service')
    await flushPromises()
    expect(wrapper.vm.filters.service).toBe('')
    expect(wrapper.vm.hasDrillFilters).toBe(false)
    wrapper.unmount()
  })

  // 统计只按单一维度聚合；点击桶仍可按该维度下钻到日志明细。
  it('统计请求只带主维度，点击统计桶仅按该维度下钻', async () => {
    searchElasticsearchLogFacetStats.mockImplementation((id, params) => {
      if (params.field === 'service') {
        return Promise.resolve({ data: { data: { field: 'service', interval_minutes: 5, buckets: [
          { value: 'tomcat', count: 9, sample: { ...LOG_DOC }, trend: [] },
        ] } } })
      }
      return Promise.resolve({ data: { data: { field: params.field, interval_minutes: 1, buckets: LEVEL_BUCKETS } } })
    })
    const wrapper = mountPanel({ nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: 'TIB' })
    await wrapper.setProps({
      candidateServices: [
        { id: 42, code: 'tomcat', name: 'tomcat服务', business_system: 7, environment: 10, environment_name: 'poc' },
      ],
    })
    await flushPromises()

    await wrapper.vm.loadStats()
    await flushPromises()
    const statsRequest = searchElasticsearchLogFacetStats.mock.calls.at(-1)[1]
    expect(statsRequest).toEqual(expect.objectContaining({ field: 'service' }))
    expect(statsRequest).not.toHaveProperty('sub_field')
    expect(wrapper.text()).not.toContain('子维度')
    expect(wrapper.text()).not.toContain('这一桶没有子维度数据')

    wrapper.vm.drillDown({ value: 'tomcat' })
    await flushPromises()
    expect(wrapper.vm.filters.service).toBe('tomcat')
    expect(wrapper.vm.filters.errorFingerprint).toBe('')
    expect(wrapper.vm.hasDrillFilters).toBe(true)
    wrapper.unmount()
  })

  // 统计维度只列**当前层级之下**的范围维度：环境节点不该出现业务系统/项目（范围已限定），
  // 服务节点连逻辑服务分组都没有意义（单一值）。
  it('统计维度按当前层级裁剪范围维度', async () => {
    const labelsOf = (wrapper) => wrapper.vm.statsFieldOptions.map((option) => option.label)

    const envWrapper = mountPanel({ nodeType: 'environment', businessSystemId: 7, environment: 10, nodeTitle: 'poc' })
    await envWrapper.setProps({ candidateServices: [{ id: 42, code: 'tomcat', name: 'tomcat服务', business_system: 7, environment: 10 }] })
    await flushPromises()
    expect(labelsOf(envWrapper)).toEqual(['逻辑服务', '错误指纹', '日志级别', '实例', '主机', '日志文件', '日志路径'])
    envWrapper.unmount()

    const projectWrapper = mountPanel({ nodeType: 'project', projectId: 301, nodeTitle: '订单项目' })
    await projectWrapper.setProps({ candidateServices: [{ id: 42, code: 'tomcat', name: 'tomcat服务', business_system: 7, environment: 10 }] })
    await flushPromises()
    expect(labelsOf(projectWrapper)).toEqual(['业务系统', '环境', '逻辑服务', '错误指纹', '日志级别', '实例', '主机', '日志文件', '日志路径'])
    projectWrapper.unmount()

    const allWrapper = mountPanel({ nodeType: 'all', nodeTitle: '全部业务' })
    await allWrapper.setProps({ candidateServices: [{ id: 42, code: 'tomcat', name: 'tomcat服务', business_system: 7, environment: 10 }] })
    await flushPromises()
    expect(labelsOf(allWrapper)[0]).toBe('项目')
    allWrapper.unmount()

    const serviceWrapper = mountPanel({ nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' })
    await flushPromises()
    expect(labelsOf(serviceWrapper)).toEqual(['错误指纹', '日志级别', '实例', '主机', '日志文件', '日志路径'])
    serviceWrapper.unmount()
  })

  it('条件改动后标记「条件已改」，查询后清除', async () => {
    const wrapper = mountPanel({ nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' })
    await flushPromises()
    expect(wrapper.vm.queryDirty).toBe(false)

    wrapper.vm.filters.keyword = 'timeout'
    await flushPromises()
    expect(wrapper.vm.queryDirty).toBe(true)

    await wrapper.vm.runSearch()
    await flushPromises()
    expect(wrapper.vm.queryDirty).toBe(false)
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
    expect(wrapper.vm.keywordPlaceholder).toContain('搜索日志正文')
    // 切换控件必须是**实心按钮组**（a-segmented 的选中态在这个主题下看不出来，用户反馈过），
    // 且选中项由 v-model 决定——渲染层要能直接读出"哪个在用"。
    const modeGroup = wrapper.find('.keyword-mode-switch')
    expect(modeGroup.exists()).toBe(true)
    expect(modeGroup.find('.ant-radio-button-wrapper-checked').text()).toBe('正文')

    wrapper.vm.filters.keyword = 'host_ip:"192.168.201.209"'
    wrapper.vm.filters.keywordMode = 'lucene'
    await wrapper.vm.runSearch()
    await flushPromises()

    const params = searchElasticsearchLogs.mock.calls.at(-1)[1]
    expect(params.keyword).toBe('host_ip:"192.168.201.209"')
    expect(params.keyword_mode).toBe('lucene')
    // 说明文案随模式切换：Lucene 模式的提示词要告诉用户"字段过滤可用"。
    expect(wrapper.vm.keywordPlaceholder).toContain('Lucene')
    expect(wrapper.vm.keywordPlaceholder).toContain('host_ip')
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
    wrapper.vm.runSearch()
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

  // 2026-09-21 现场：详情里的「原始消息」实际绑的是 log_message（解析后的消息），
  // 而真正的原始行是 message。现在两段分开：原始日志=message、错误日志=log_message。
  it('日志详情分开展示原始日志（message）与错误日志（log_message），并可复制', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    wrapper.vm.openDetail(LOG_DOC)
    await flushPromises()

    const text = document.body.textContent
    expect(text).toContain('原始日志')
    expect(text).toContain('错误日志')
    // message 独有前缀，证明确实取的是原始行。
    expect(text).toContain('com.esb.App')

    const heads = document.querySelectorAll('.detail-section-head')
    expect(heads.length).toBe(2)
    expect(heads[0].textContent).toContain('原始日志')
    expect(heads[1].textContent).toContain('错误日志')

    const writeText = vi.fn().mockResolvedValue()
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    // 每段标题行有「预览」「复制」两个按钮，取「复制」那个。
    // 注意 antd 会把两个汉字的按钮渲染成「复 制」（中间插空格），匹配时要忽略空白。
    const copyButton = (head) => [...head.querySelectorAll('button')]
      .find((btn) => btn.textContent.replace(/\s/g, '').includes('复制'))
    copyButton(heads[0]).click()
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith(LOG_DOC.message)
    copyButton(heads[1]).click()
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith(LOG_DOC.log_message)
    wrapper.unmount()
  })

  // 现场：详情侧边栏窄，长堆栈看不动。原始/错误日志各加一个「预览」按钮，弹大框查看。
  it('详情里的原始/错误日志可弹大框预览', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.setProps({ scope: { nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' } })
    await flushPromises()

    wrapper.vm.openDetail(LOG_DOC)
    await flushPromises()

    const previewButton = [...document.querySelectorAll('.detail-section-head')]
      .map((head) => [...head.querySelectorAll('button')].find((btn) => btn.textContent.includes('预览')))
      .find(Boolean)
    previewButton.click()
    await flushPromises()

    expect(wrapper.vm.detailTextModal.open).toBe(true)
    expect(document.querySelector('.detail-modal-content').textContent).toContain('com.esb.App')
    wrapper.unmount()
  })

  it('切换到统计 tab 后按当前统计维度查询', async () => {
    // 前面的概览用例改过分面 mock 的实现，这里恢复"默认指纹分面"再断言。
    searchElasticsearchLogFacetStats.mockImplementation((id, params) => {
      if (params.field === 'log_level') {
        return Promise.resolve({ data: { data: { field: 'log_level', interval_minutes: 1, buckets: LEVEL_BUCKETS } } })
      }
      return Promise.resolve({ data: { data: { field: params.field, interval_minutes: 5, buckets: [FACET_BUCKET] } } })
    })
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
  it('日志表格显示路径，路径下拉支持选择查询和清空且不收窄自身候选', async () => {
    const wrapper = mountPanel({ nodeType: 'service', applicationServiceId: 42, nodeTitle: 'tomcat服务' })
    await flushPromises()
    expect(wrapper.vm.columns.find((column) => column.key === 'log_path')).toEqual(
      expect.objectContaining({ dataIndex: 'log_path', width: 280, ellipsis: true }),
    )
    expect(wrapper.find('.ant-table-tbody').text()).toContain(LOG_DOC.log_path)
    const select = wrapper.find('.filter-path').findComponent({ name: 'ASelect' })
    expect(select.props('showSearch')).toBe(true)
    expect(select.props('options')).toEqual(wrapper.vm.logPathOptions)
    select.vm.$emit('update:value', '/srv/app/error.log')
    await wrapper.vm.runSearch()
    await flushPromises()
    expect(searchElasticsearchLogs).toHaveBeenLastCalledWith(CLUSTER.id,
      expect.objectContaining({ log_path: '/srv/app/error.log' }))
    expect(searchElasticsearchLogFacetStats).toHaveBeenCalledWith(CLUSTER.id,
      expect.objectContaining({ field: 'log_path', size: 50, application_service_id: 42 }))
    const pathRequest = searchElasticsearchLogFacetStats.mock.calls.filter(([, params]) => params.field === 'log_path').at(-1)[1]
    expect(pathRequest).not.toHaveProperty('log_path')
    select.vm.$emit('update:value', undefined)
    await wrapper.vm.runSearch()
    await flushPromises()
    expect(searchElasticsearchLogs.mock.calls.at(-1)[1].log_path).toBeUndefined()
    wrapper.unmount()
  })

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

  it('候选名单非空到非空时自动重查，日志和所有分面都限定到新名单', async () => {
    const wrapper = mountPanel()
    await wrapper.setProps({ candidateServices: [{ id: 42 }, { id: 43 }] })
    await flushPromises()
    searchElasticsearchLogs.mockClear()
    searchElasticsearchLogFacetStats.mockClear()

    await wrapper.setProps({ candidateServices: [{ id: 43 }] })
    await flushPromises()
    expect(searchElasticsearchLogs).toHaveBeenCalled()
    expect(searchElasticsearchLogFacetStats).toHaveBeenCalled()
    for (const [, params] of [
      ...searchElasticsearchLogs.mock.calls,
      ...searchElasticsearchLogFacetStats.mock.calls,
    ]) {
      expect(params.application_service_ids).toBe('43')
    }

    wrapper.vm.activeTab = 'stats'
    await flushPromises()
    searchElasticsearchLogFacetStats.mockClear()
    await wrapper.setProps({ candidateServices: [{ id: 42 }] })
    await flushPromises()
    expect(wrapper.vm.activeTab).toBe('stats')
    expect(searchElasticsearchLogFacetStats).toHaveBeenCalled()
    expect(searchElasticsearchLogFacetStats.mock.calls.every(([, params]) => params.application_service_ids === '42')).toBe(true)
    wrapper.unmount()
  })

  it('筛选后无服务时清空旧结果，不发送缺少服务范围的全量查询', async () => {
    const wrapper = mountPanel()
    await wrapper.setProps({ candidateServices: [{ id: 42 }] })
    await flushPromises()
    searchElasticsearchLogs.mockClear()
    searchElasticsearchLogFacetStats.mockClear()
    await wrapper.setProps({ candidateServices: [] })
    await flushPromises()
    expect(searchElasticsearchLogs).not.toHaveBeenCalled()
    expect(searchElasticsearchLogFacetStats).not.toHaveBeenCalled()
    expect(wrapper.vm.logs).toEqual([])
    expect(wrapper.vm.statsBuckets).toEqual([])
    expect(wrapper.vm.logLevelOptions).toEqual([])
    expect(wrapper.vm.pagination.total).toBe(0)
    wrapper.unmount()
  })

  it('切换范围后，晚返回的旧日志和筛选候选不会覆盖新范围', async () => {
    const oldLogs = []
    const oldOptions = []
    const originalLogs = searchElasticsearchLogs.getMockImplementation()
    const originalStats = searchElasticsearchLogFacetStats.getMockImplementation()
    searchElasticsearchLogs.mockImplementation((_id, params) => (
      params.application_service_ids === '42'
        ? new Promise((resolve) => oldLogs.push(resolve))
        : Promise.resolve({ data: { data: { results: [{ ...LOG_DOC, id: 'new-doc' }], count: 1 } } })
    ))
    searchElasticsearchLogFacetStats.mockImplementation((_id, params) => (
      params.application_service_ids === '42'
        ? new Promise((resolve) => oldOptions.push(resolve))
        : Promise.resolve({ data: { data: { buckets: [{ value: 'NEW', count: 1 }] } } })
    ))
    const wrapper = mountPanel()
    try {
      await wrapper.setProps({ candidateServices: [{ id: 42 }] })
      await flushPromises()
      expect(oldLogs.length).toBeGreaterThan(0)
      expect(oldOptions.length).toBeGreaterThan(0)
      await wrapper.setProps({ candidateServices: [{ id: 43 }] })
      await flushPromises()
      oldLogs.forEach((resolve) => resolve({ data: { data: { results: [LOG_DOC], count: 99 } } }))
      oldOptions.forEach((resolve) => resolve({ data: { data: { buckets: [{ value: 'OLD', count: 99 }] } } }))
      await flushPromises()
      expect(wrapper.vm.logs.map((row) => row.id)).toEqual(['new-doc'])
      expect(wrapper.vm.pagination.total).toBe(1)
      expect(wrapper.vm.logLevelOptions).toEqual([{ label: 'NEW', value: 'NEW' }])
      expect(wrapper.vm.loading).toBe(false)
    } finally {
      wrapper.unmount()
      searchElasticsearchLogs.mockImplementation(originalLogs)
      searchElasticsearchLogFacetStats.mockImplementation(originalStats)
    }
  })
})
