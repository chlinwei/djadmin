/**
 * 存储水位"按层级聚合"的饼图配置。
 *
 * 抽成纯函数是为了能直接测：echarts 在 jsdom 里没有真实 canvas，组件只能 mock，
 * 而"画什么"（取哪些分组、零值怎么处理、其他项怎么合并）恰恰是会出错的部分。
 *
 * 两条刻意的口径：
 *  1. **零占用的分组不进饼图**：饼图表达的是占比，0 占 0%。这些分组仍然留在表格里
 *     （"这个项目一条日志都没有"本身就是有用的信息），只是不占一块扇形。
 *  2. **只画前 N 项，其余合并成"其他"**：分组多时扇形会碎到无法阅读；合并项在 tooltip 里
 *     仍然给出项数，避免"看起来只剩这几项"的误读。
 */

const DEFAULT_COLORS = ['#1677ff', '#52c41a', '#fa8c16', '#eb2f96', '#722ed1', '#13c2c2', '#faad14', '#a0d911']

/**
 * buildStorageUsagePieOption 生成饼图配置；没有可画的数据时返回 null（调用方显示空态）。
 *
 * @param {Array<{name: string, bytes: number, docs?: number, streams?: number, historicalBytes?: number}>} groups
 * @param {{title?: string, limit?: number, formatBytes?: (value: number) => string}} options
 */
export function buildStorageUsagePieOption(groups, options = {}) {
  const { title = '', limit = 8, formatBytes = (value) => String(value) } = options
  const drawable = (groups || []).filter((group) => Number(group.bytes) > 0)
  if (!drawable.length) return null

  const head = drawable.slice(0, limit)
  const tail = drawable.slice(limit)
  const data = head.map((group) => ({
    name: group.name,
    value: Number(group.bytes),
    bytes: Number(group.bytes),
    docs: Number(group.docs || 0),
    streams: Number(group.streams || 0),
    historicalBytes: Number(group.historicalBytes || 0),
  }))
  if (tail.length) {
    data.push({
      name: `其他 ${tail.length} 项`,
      value: tail.reduce((sum, group) => sum + Number(group.bytes), 0),
      bytes: tail.reduce((sum, group) => sum + Number(group.bytes), 0),
      docs: tail.reduce((sum, group) => sum + Number(group.docs || 0), 0),
      streams: tail.reduce((sum, group) => sum + Number(group.streams || 0), 0),
      historicalBytes: tail.reduce((sum, group) => sum + Number(group.historicalBytes || 0), 0),
    })
  }

  return {
    color: DEFAULT_COLORS,
    title: title ? { text: title, left: 8, top: 4, textStyle: { fontSize: 13, fontWeight: 600 } } : undefined,
    tooltip: {
      trigger: 'item',
      formatter(params) {
        const item = params.data || {}
        const lines = [`${params.marker}${params.name}：<strong>${formatBytes(item.bytes)}</strong>（${params.percent}%）`]
        if (item.historicalBytes) lines.push(`其中历史档位：${formatBytes(item.historicalBytes)}`)
        lines.push(`data stream ${item.streams} 条 · 文档 ${Number(item.docs).toLocaleString()}`)
        return lines.join('<br/>')
      },
    },
    legend: { type: 'scroll', bottom: 0, left: 'center', textStyle: { fontSize: 12 } },
    series: [
      {
        type: 'pie',
        radius: ['38%', '64%'],
        center: ['50%', '48%'],
        avoidLabelOverlap: true,
        label: { formatter: '{b}\n{d}%', fontSize: 12 },
        data,
      },
    ],
  }
}
