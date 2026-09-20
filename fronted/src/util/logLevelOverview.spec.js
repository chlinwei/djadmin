import { describe, expect, it } from 'vitest'

import { buildLevelMetrics, buildLevelRows, levelDimension } from './logLevelOverview'

// 日志中心层级视图的折叠口径（架构文档 §9.5「非服务节点 = 层级下钻视图」）。
//
// 这里要守住的核心是**上下层数字一致**：上层行 = 它下面那些服务行之和，指标条 = 清单之和。
// 一旦有人在上层另算一套（比如"按业务系统再查一次接口"），就会出现"指标条说 3 条未认证、
// 点进去一条都没有"这类没人敢信的界面。

function service(overrides = {}) {
  return {
    id: 1, code: 'order-api', name: 'order-api', business_system: 7, environment: 71,
    environment_name: '生产环境', log_collection_enabled: true, log_retention_tier_id: 3,
    ...overrides,
  }
}

function status(overrides = {}) {
  return {
    logs: 4, verified: 1, needs_recheck: 0, unverified: 3, disabled_logs: 0, no_rule: 0,
    pending: { hosts: 2, managed: 2, unmanaged: 0, synced: 1, drift: 1, never: 0 },
    ...overrides,
  }
}

describe('logLevelOverview', () => {
  it('maps every level to its next dimension and stops at the service level', () => {
    expect(levelDimension('all')?.key).toBe('project')
    expect(levelDimension('project')?.key).toBe('businessSystem')
    expect(levelDimension('businessSystem')?.key).toBe('environment')
    expect(levelDimension('environment')?.key).toBe('service')
    // 服务/实例节点不是层级视图（那是服务级界面）。
    expect(levelDimension('service')).toBe(null)
    expect(levelDimension('deployment')).toBe(null)
  })

  it('lists the next level for a business-system node as its environments', () => {
    const { dimension, rows } = buildLevelRows({
      scope: { nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: '订单系统' },
      services: [
        service({ id: 1, environment: 71, environment_name: '生产环境' }),
        service({ id: 2, environment: 72, environment_name: '测试环境' }),
        // 没配环境的服务也要能列出来，否则它在层级视图里就消失了。
        service({ id: 3, environment: null, environment_name: '' }),
      ],
    })

    expect(dimension.label).toBe('环境')
    expect(rows.map((row) => row.title)).toEqual(['生产环境', '测试环境', '未配置环境'])
    expect(rows.map((row) => row.serviceCount)).toEqual([1, 1, 1])
    // 环境行的下钻 scope 必须带业务系统 id + 环境 id（树按这两个字段反推高亮）。
    expect(rows[0].scope).toMatchObject({ nodeType: 'environment', businessSystemId: 7, environment: 71 })
    expect(rows[2].scope).toMatchObject({ nodeType: 'environment', environment: null })
  })

  it('aggregates service rows into the upper levels so both views agree', () => {
    const services = [
      service({ id: 1, business_system: 7, environment: 71, name: 'order-api', log_collection_enabled: true }),
      service({ id: 2, business_system: 7, environment: 72, name: 'order-job', log_collection_enabled: false }),
      service({ id: 3, business_system: 8, environment: 71, name: 'pay-api', log_collection_enabled: true }),
    ]
    const statusByService = {
      1: status({ logs: 6, unverified: 3, needs_recheck: 1, pending: { hosts: 2, managed: 2, unmanaged: 0, synced: 1, drift: 1, never: 0 } }),
      2: status({ logs: 4, unverified: 0, needs_recheck: 0, disabled_logs: 3, pending: { hosts: 1, managed: 1, unmanaged: 0, synced: 0, drift: 0, never: 1 } }),
      3: status({ logs: 2, unverified: 2, needs_recheck: 0, pending: null }),
    }

    // 项目层：两行（两个业务系统），数字 = 各自下面服务的和。
    const projectLevel = buildLevelRows({
      scope: { nodeType: 'project', projectId: 301, nodeTitle: '订单项目' },
      businessSystems: [
        { id: 7, name: '订单系统', project: 301 },
        { id: 8, name: '支付系统', project: 301 },
      ],
      services,
      statusByService,
    })
    expect(projectLevel.rows.map((row) => row.title)).toEqual(['订单系统', '支付系统'])
    expect(projectLevel.rows[0]).toMatchObject({
      serviceCount: 2, collectionOff: 1, logs: 10, unverified: 3, needsRecheck: 1,
      // 待下发 = drift + never：服务 1 有 1 台待下发、服务 2 有 1 台从未下发。
      disabledLogs: 3, pendingPairs: 2, managedPairs: 3,
    })
    // 行里没有任何服务的配置态时（服务 3 的 pending 为 null）→ 该行这一列显示"-"。
    expect(projectLevel.rows[1].pendingKnown).toBe(false)

    // 下钻到业务系统层：环境行的和 = 上一层那一行（同源聚合）。
    const systemLevel = buildLevelRows({
      scope: { nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: '订单系统' },
      businessSystems: [{ id: 7, name: '订单系统', project: 301 }],
      services,
      statusByService,
    })
    const systemTotals = buildLevelMetrics(systemLevel.rows)
    expect(systemTotals).toMatchObject({
      services: 2, collectionOff: 1, unverified: 3, needsRecheck: 1, pendingPairs: 2, managedPairs: 3,
    })

    // 叶子层（环境）：一行一个服务，点行 = 进入该服务。
    const environmentLevel = buildLevelRows({
      scope: { nodeType: 'environment', businessSystemId: 7, environment: 71, nodeTitle: '生产环境' },
      services,
      statusByService,
    })
    expect(environmentLevel.rows).toHaveLength(1)
    expect(environmentLevel.rows[0]).toMatchObject({
      title: 'order-api', isService: true, logs: 6, unverified: 3, retentionTierId: 3,
    })
    expect(environmentLevel.rows[0].scope).toMatchObject({ nodeType: 'service', applicationServiceId: 1 })
    // 服务行的下钻 scope 要带业务系统与环境，方便页面把树的上下文补全。
    expect(environmentLevel.rows[0].scope).toMatchObject({ businessSystemId: 7, environment: 71 })
  })

  it('only attaches the recent write volume on the leaf level', () => {
    const services = [service({ id: 1, code: 'order-api', environment: 71 }), service({ id: 2, code: 'order-job', environment: 71, name: 'order-job' })]
    // getLogServiceUsage 是按**服务编码**聚合的（不是 id），所以按 code 对。
    const usageByService = { 'order-api': 1200, 'order-job': 0 }

    const leaf = buildLevelRows({
      scope: { nodeType: 'environment', businessSystemId: 7, environment: 71, nodeTitle: '生产环境' },
      services,
      usageByService,
    })
    expect(leaf.rows.map((row) => row.recentDocs)).toEqual([1200, 0])
    expect(leaf.rows.every((row) => row.recentDocsKnown)).toBe(true)
    // 只有真的取到写入量才谈得上"有/没有"：取到了 0 条就是"无写入"（查询 tab 要标红的那一类）。
    expect(leaf.rows.map((row) => row.hasRecentDocs)).toEqual([true, false])

    // 上层不算这一列（要算就得按服务逐个查 ES，代价不值）：明确是"没有这项数据"而不是 0。
    const upper = buildLevelRows({
      scope: { nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: '订单系统' },
      services,
      usageByService,
    })
    expect(upper.rows[0].recentDocs).toBe(null)
    expect(upper.rows[0].recentDocsKnown).toBe(false)
    // **没取到数据 ≠ 没有写入**：hasRecentDocs 必须是 null（不知道），不能是 false。
    expect(upper.rows[0].hasRecentDocs).toBe(null)
  })

  // 查询 tab 的指标条口径：可查（采集开着）与"有没有数据"两类数字分开算。
  it('summarises the query view with queryable and silent services', () => {
    const services = [
      service({ id: 1, code: 'order-api', environment: 71, log_collection_enabled: true }),
      service({ id: 2, code: 'order-job', environment: 71, name: 'order-job', log_collection_enabled: false }),
      service({ id: 3, code: 'order-worker', environment: 71, name: 'order-worker', log_collection_enabled: true }),
    ]
    const { rows } = buildLevelRows({
      scope: { nodeType: 'environment', businessSystemId: 7, environment: 71, nodeTitle: '生产环境' },
      services,
      usageByService: { 'order-api': 500, 'order-worker': 0 },
    })

    const metrics = buildLevelMetrics(rows)
    expect(metrics.services).toBe(3)
    // 可查 = 采集总开关开着的那两个（与"有没有数据"是两件事）。
    expect(metrics.collectionOn).toBe(2)
    expect(metrics.collectionOff).toBe(1)
    expect(metrics.recentKnown).toBe(true)
    expect(metrics.recentServices).toBe(1)
    expect(metrics.recentSilentServices).toBe(1)

    // 上层：取不到写入量 → recentKnown=false，那两个数保持 0（界面显示"-"，不能显示成"0 个无写入"）。
    const upper = buildLevelRows({
      scope: { nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: '订单系统' },
      services,
    })
    const upperMetrics = buildLevelMetrics(upper.rows)
    expect(upperMetrics.recentKnown).toBe(false)
    expect(upperMetrics.recentSilentServices).toBe(0)
    expect(upperMetrics.collectionOn).toBe(2)
  })

  it('reports the pending column as unknown when the backend could not evaluate it', () => {
    const { rows } = buildLevelRows({
      scope: { nodeType: 'environment', businessSystemId: 7, environment: 71, nodeTitle: '生产环境' },
      services: [service({ id: 1 })],
      // 后端 pending 为 null（评估器不可用 / 没有启用的默认集群）：不是 0，是"没算出来"。
      statusByService: { 1: status({ pending: null }) },
    })
    expect(rows[0].pendingKnown).toBe(false)
    expect(rows[0].pendingPairs).toBe(0)
    expect(buildLevelMetrics(rows).pendingKnown).toBe(false)
  })

  it('lists only the business systems of the selected project', () => {
    // 现场反馈：点某个项目，却列出了**全部**业务系统。项目节点的下一层必须只含这个项目的业务系统
    // （业务系统行上的 project 字段是权威归属），否则指标条与清单显示的都是别的项目的数据。
    const { rows } = buildLevelRows({
      scope: { nodeType: 'project', projectId: 301, nodeTitle: '订单项目' },
      businessSystems: [
        { id: 7, name: '订单系统', project: 301 },
        { id: 8, name: '支付系统', project: 301 },
        { id: 9, name: '库存系统', project: 302 },
      ],
      services: [
        service({ id: 1, business_system: 7, name: 'order-api' }),
        service({ id: 2, business_system: 9, name: 'stock-api' }),
      ],
    })

    expect(rows.map((row) => row.title)).toEqual(['订单系统', '支付系统'])
    // 别的项目的服务不能混进这一层的数字里。
    expect(rows[0].serviceCount).toBe(1)
    expect(rows[1].serviceCount).toBe(0)
    // 建了业务系统但还没建服务的，也要列出来（否则看起来"这个项目什么都没有"）。
    expect(rows.map((row) => row.scope)).toEqual([
      { nodeType: 'businessSystem', businessSystemId: 7, nodeTitle: '订单系统' },
      { nodeType: 'businessSystem', businessSystemId: 8, nodeTitle: '支付系统' },
    ])
  })

  it('honours the tree root filters (project and environment selectors)', () => {
    const projects = [
      { id: 301, name: '订单项目' },
      { id: 302, name: '库存项目' },
    ]
    const businessSystems = [
      { id: 7, name: '订单系统', project: 301 },
      { id: 8, name: '支付系统', project: 301 },
      { id: 9, name: '库存系统', project: 302 },
    ]
    const services = [
      service({ id: 1, business_system: 7, environment: 71, name: 'order-api' }),
      service({ id: 2, business_system: 7, environment: 72, name: 'order-job' }),
      service({ id: 3, business_system: 9, environment: 71, name: 'stock-api' }),
    ]

    // 根节点：项目多选只看选中的项目；环境多选只看选中的环境里的服务。
    const { rows } = buildLevelRows({
      scope: { nodeType: 'all', nodeTitle: '全部业务', projectIds: [301], environmentIds: [71] },
      projects,
      businessSystems,
      services,
    })

    expect(rows.map((row) => row.title)).toEqual(['订单项目'])
    // 订单系统里环境 71 的服务算进来、环境 72 的不算（与存储水位 tab 的筛选口径一致）。
    expect(rows[0].serviceCount).toBe(1)
    expect(rows[0].serviceIds).toEqual([1])
  })

  it('gives nothing for a service or deployment node', () => {
    expect(buildLevelRows({ scope: { nodeType: 'service', applicationServiceId: 1 } }).rows).toEqual([])
    expect(levelDimension('deployment')).toBe(null)
  })
})
