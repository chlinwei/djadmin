import { flushPromises, mount } from '@vue/test-utils'
import Antd from 'ant-design-vue'
import { afterEach, describe, expect, it } from 'vitest'

import LogLevelOverview from './LogLevelOverview.vue'

// 非服务节点的层级视图（架构文档 §9.5「非服务节点 = 层级下钻视图」）。
// 这里钉住的是界面契约：指标条 + 下一层清单 + 点行下钻；以及"没算出来"必须显示成"-"。

const dimension = { key: 'businessSystem', label: '业务系统' }

function row(overrides = {}) {
  return {
    key: 'system:7', title: '订单系统', scope: { nodeType: 'businessSystem', businessSystemId: 7 },
    isService: false, serviceCount: 2, collectionOff: 1, logs: 10, verified: 6, needsRecheck: 1,
    unverified: 3, disabledLogs: 3, noRule: 1, pendingPairs: 2, managedPairs: 3, unmanagedPairs: 1,
    recentDocs: null, recentDocsKnown: false, retentionTierId: null, pendingKnown: true,
    ...overrides,
  }
}

function metrics(overrides = {}) {
  return {
    services: 2, collectionOn: 1, collectionOff: 1, unverified: 3, needsRecheck: 1,
    pendingPairs: 2, managedPairs: 3, unmanagedPairs: 1, pendingKnown: true,
    ...overrides,
  }
}

function mountOverview(props = {}) {
  return mount(LogLevelOverview, {
    props: { dimension, rows: [row()], metrics: metrics(), ...props },
    attachTo: document.body,
    global: { plugins: [Antd] },
  })
}

describe('LogLevelOverview', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('shows the level metrics and the next-level rows', async () => {
    const wrapper = mountOverview()

    expect(wrapper.text()).toContain('订单系统')
    // 配置 tab 的指标条讲"配置欠账"：已开采集 / 采集关闭 / 未认证日志 / 待下发。
    expect(wrapper.text()).toContain('下辖逻辑服务')
    expect(wrapper.text()).toContain('已开采集')
    expect(wrapper.text()).toContain('采集关闭')
    expect(wrapper.text()).toContain('未认证日志')
    // 待下发的单位是 (服务 × 主机)，后缀给出分母，避免被读成"2 台机器"。
    expect(wrapper.text()).toContain('待下发')
    expect(wrapper.text()).toContain('/ 3 台·服务')
    // 行上也直接给出欠账数字（哪一片欠得多一眼可见）。
    expect(wrapper.text()).toContain('未挂规则')
    wrapper.unmount()
  })

  // 两个 tab 的分工（这是这一版的修正点）：查询讲"能不能查/有没有数据"，配置讲"配置欠账"，
  // 两边的指标条与列**都不重叠**——否则这一层的两个 tab 会长得几乎一样。
  it('keeps the two tabs telling different stories', async () => {
    const config = mountOverview()
    const query = mountOverview({ variant: 'query' })

    // 配置 tab：有欠账列（未认证日志 / 未挂规则 / 配置态），没有数据列（最近写入）。
    expect(config.text()).toContain('未认证日志')
    expect(config.text()).toContain('未挂规则')
    expect(config.text()).toContain('配置态')
    expect(config.text()).not.toContain('无写入')
    expect(config.text()).not.toContain('最近 30 天写入')

    // 查询 tab：有"能不能查"，**没有**任何配置欠账列。
    expect(query.text()).toContain('可查')
    expect(query.text()).not.toContain('未认证日志')
    expect(query.text()).not.toContain('未挂规则')
    expect(query.text()).not.toContain('配置态')
    expect(query.text()).not.toContain('待下发')
    // 这一层算不出写入量 → 那一格**根本不出现**（不摆"-/本层不统计"的占位），
    // 只在底部给一句可操作的指引。
    expect(query.text()).not.toContain('无写入')
    expect(query.text()).toContain('要看写入量请下钻到环境层')

    // 上半层的列也不同：配置给欠账数字，查询给可查/关闭。
    expect(config.findAll('.ant-table-thead th').map((th) => th.text()).join('|')).toContain('未挂规则')
    expect(query.findAll('.ant-table-thead th').map((th) => th.text()).join('|')).toContain('可查')

    config.unmount()
    query.unmount()
  })

  // 能算出写入量的层级（环境）：那一格才出现，并且分母给出"有写入的服务数"。
  it('shows the silent-services tile only where the write volume is actually known', () => {
    const wrapper = mountOverview({
      variant: 'query',
      dimension: { key: 'service', label: '逻辑服务' },
      rows: [
        row({ isService: true, title: 'order-api', serviceCount: 1, recentDocs: 1200, recentDocsKnown: true, hasRecentDocs: true }),
        row({ isService: true, title: 'order-job', serviceCount: 1, recentDocs: 0, recentDocsKnown: true, hasRecentDocs: false }),
      ],
      metrics: metrics({ services: 2, recentKnown: true, recentServices: 1, recentSilentServices: 1 }),
    })

    expect(wrapper.text()).toContain('无写入')
    expect(wrapper.text()).toContain('/ 1 个有写入')
    // 上层的两个 tab 都不该出现这一格（配置 tab 本来没有；查询 tab 上层算不出来）。
    wrapper.unmount()
    const upper = mountOverview({ variant: 'query', metrics: metrics({ recentKnown: false }) })
    expect(upper.text()).not.toContain('无写入')
    upper.unmount()
  })

  it('drills down when a row is clicked or its action is used', async () => {
    const wrapper = mountOverview()

    // 整行可点：层级视图的主要动作就是下钻，不该逼用户在表格里找按钮。
    // （注意 tbody 的第一行是 antd 的隐藏测量行，要按 .ant-table-row 取。）
    await wrapper.find('.ant-table-tbody tr.ant-table-row').trigger('click')
    expect(wrapper.emitted('drilldown')?.at(-1)).toEqual([{ nodeType: 'businessSystem', businessSystemId: 7 }])

    await wrapper.find('.ant-table-tbody button').trigger('click')
    expect(wrapper.emitted('drilldown')).toHaveLength(2)
    wrapper.unmount()
  })

  it('labels the action per level and per tab', async () => {
    const upper = mountOverview()
    expect(upper.find('.ant-table-tbody button').text()).toBe('下钻')
    upper.unmount()

    // 叶子层（环境）：行就是服务，动作用词按 tab 区分（点进去就是服务级界面）。
    const leafConfig = mountOverview({
      dimension: { key: 'service', label: '逻辑服务' },
      rows: [row({ isService: true, title: 'order-api', serviceCount: 1, retentionTierId: 3 })],
      retentionTiers: [{ id: 3, name: '标准 30 天' }],
    })
    expect(leafConfig.find('.ant-table-tbody button').text()).toBe('日志配置')
    expect(leafConfig.text()).toContain('标准 30 天')
    leafConfig.unmount()

    const leafQuery = mountOverview({
      dimension: { key: 'service', label: '逻辑服务' },
      variant: 'query',
      rows: [row({ isService: true, title: 'order-api', serviceCount: 1, recentDocs: 1200, recentDocsKnown: true })],
      recentDocsTitle: '最近 30 天写入',
    })
    expect(leafQuery.find('.ant-table-tbody button').text()).toBe('查询日志')
    expect(leafQuery.text()).toContain('最近 30 天写入')
    expect(leafQuery.text()).toContain('1,200 条')
    // 查询 tab 上要明说"检索必须选到具体服务"，否则用户会以为这个层级能直接搜。
    expect(leafQuery.text()).toContain('日志检索按逻辑服务切分')
    leafQuery.unmount()
  })

  it('renders an unknown pending column as a dash instead of zero', async () => {
    const wrapper = mountOverview({
      rows: [row({ pendingKnown: false, pendingPairs: 0 })],
      metrics: metrics({ pendingKnown: false, pendingPairs: 0 }),
      pendingError: '没有已启用的默认 Elasticsearch 集群',
    })

    // 配置态没算出来：这一列显示"-"，并给出原因——绝不显示成"都已同步"。
    expect(wrapper.find('.ant-table-tbody').text()).toContain('-')
    expect(wrapper.text()).toContain('配置态没算出来')
    expect(wrapper.text()).toContain('没有已启用的默认 Elasticsearch 集群')
    wrapper.unmount()
  })

  it('explains an empty scope instead of showing a blank table', async () => {
    const wrapper = mountOverview({ rows: [], metrics: metrics({ services: 0, managedPairs: 0 }) })

    expect(wrapper.find('.ant-table').exists()).toBe(false)
    expect(wrapper.text()).toContain('这个范围里还没有逻辑服务')
    wrapper.unmount()
  })

  it('keeps the read-only promise visible: writes stay at the service level', async () => {
    const wrapper = mountOverview()
    expect(wrapper.text()).toContain('写操作仍然以')
    expect(wrapper.text()).toContain('逻辑服务')
    await flushPromises()
    wrapper.unmount()
  })
})
