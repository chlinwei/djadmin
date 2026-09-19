import { describe, expect, it } from 'vitest'

import { buildStorageUsagePieOption } from './storageUsagePie'

const groups = [
  { name: 'yilake', bytes: 1054726145, docs: 4758186, streams: 2, historicalBytes: 1054670278 },
  { name: 'nkg', bytes: 1024, docs: 5, streams: 1, historicalBytes: 0 },
  { name: 'empty-project', bytes: 0, docs: 0, streams: 0, historicalBytes: 0 },
]

describe('buildStorageUsagePieOption', () => {
  it('skips groups without usage but keeps them out of the chart only', () => {
    const option = buildStorageUsagePieOption(groups, { formatBytes: (value) => `${value}B` })
    const names = option.series[0].data.map((item) => item.name)
    // 零占用的分组在饼图里没有意义（占 0%），但它在表格里仍然存在——这里只是不画。
    expect(names).toEqual(['yilake', 'nkg'])
  })

  it('returns null when there is nothing to draw so the caller can show an empty state', () => {
    expect(buildStorageUsagePieOption([])).toBeNull()
    expect(buildStorageUsagePieOption([{ name: 'a', bytes: 0 }])).toBeNull()
  })

  it('merges the tail into 其他 to keep the pie readable, keeping the item count', () => {
    const many = Array.from({ length: 10 }, (_, index) => ({ name: `p${index}`, bytes: (index + 1) * 100, streams: 1 }))
    const option = buildStorageUsagePieOption(many, { limit: 3 })
    const data = option.series[0].data
    expect(data).toHaveLength(4)
    expect(data.at(-1).name).toBe('其他 7 项')
    // 合并项要累加，不能丢占用（否则总和与总数对不上）。
    expect(data.at(-1).bytes).toBe(many.slice(3).reduce((sum, item) => sum + item.bytes, 0))
  })

  it('formats the tooltip with usage, share, historical part and stream count', () => {
    const option = buildStorageUsagePieOption(groups, { formatBytes: (value) => `${value}B`, limit: 1 })
    const tooltip = option.tooltip.formatter({ name: 'yilake', percent: 99.9, marker: '', data: option.series[0].data[0] })
    expect(tooltip).toContain('1054726145B')
    expect(tooltip).toContain('99.9%')
    expect(tooltip).toContain('其中历史档位：1054670278B')
    expect(tooltip).toContain('data stream 2 条')
  })
})
