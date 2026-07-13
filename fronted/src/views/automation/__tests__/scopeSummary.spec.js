import { buildScopeSummaryText, flattenGroupPathMap } from '../scopeSummary'

describe('scopeSummary helpers', () => {
  it('builds path map recursively for nested groups', () => {
    const pathMap = flattenGroupPathMap([
      {
        id: 100,
        name: '根组',
        children: [
          { id: 101, name: '子组A', children: [] },
          { id: 102, name: '子组B', children: [{ id: 103, name: '末级组' }] },
        ],
      },
    ])

    expect(pathMap[100]).toBe('根组')
    expect(pathMap[101]).toBe('根组 / 子组A')
    expect(pathMap[103]).toBe('根组 / 子组B / 末级组')
  })

  it('returns all-hosts text when no groups and no hosts are selected', () => {
    const result = buildScopeSummaryText({
      selectedGroupIds: [],
      selectedHostIds: [],
    })

    expect(result).toBe('全部主机（按分组范围）')
  })

  it('keeps full group path in preview labels', () => {
    const groupPathMap = flattenGroupPathMap([
      {
        id: 1,
        name: '生产',
        children: [{ id: 2, name: '华东', children: [{ id: 3, name: '核心区' }] }],
      },
    ])

    const result = buildScopeSummaryText({
      selectedGroupIds: [3],
      selectedHostIds: [],
      groupPathMap,
    })

    expect(result).toBe('已选1组：生产 / 华东 / 核心区')
  })

  it('returns selected-host summary when only hosts are selected', () => {
    const result = buildScopeSummaryText({
      selectedGroupIds: [],
      selectedHostIds: [1, 2, 3],
    })

    expect(result).toBe('已选0组（3台主机）')
  })

  it('supports non-default empty label when configured', () => {
    const result = buildScopeSummaryText({
      selectedGroupIds: [],
      selectedHostIds: [],
      emptyAsAllHosts: false,
      emptyLabel: '未配置范围',
    })

    expect(result).toBe('未配置范围')
  })

  it('falls back to generated group label when map is missing', () => {
    const result = buildScopeSummaryText({
      selectedGroupIds: [88],
      selectedHostIds: [],
      groupPathMap: {},
      groupNameMap: {},
    })

    expect(result).toBe('已选1组：分组#88')
  })

  it('uses truncation when selected group count exceeds preview limit', () => {
    const result = buildScopeSummaryText({
      selectedGroupIds: [10, 11, 12, 13],
      groupNameMap: {
        10: 'A组',
        11: 'B组',
        12: 'C组',
        13: 'D组',
      },
      maxPreviewCount: 2,
    })

    expect(result).toBe('已选4组：A组；B组 等')
  })
})
