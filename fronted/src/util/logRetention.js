// 保留档位的"值 + 单位"展示（迁移 000045 起支持小时，见架构文档 §4.5）。
//
// 抽成共享函数的原因：同一件事有三个展示点——档位管理页的列表与表单、逻辑服务编辑弹窗的
// "默认保留档位"下拉。少一处没跟上就会变成"档位页写 12 小时、服务弹窗写 12 天"。
//
// 单位只有两种（后端白名单同样如此）：`d` = 天、`h` = 小时。

export const RETENTION_UNIT_DAY = 'd'
export const RETENTION_UNIT_HOUR = 'h'

// 单位下拉的选项（表单用）。顺序固定：天在前（存量语义、绝大多数档位用它）。
export const RETENTION_UNIT_OPTIONS = [
  { label: '天', value: RETENTION_UNIT_DAY },
  { label: '小时', value: RETENTION_UNIT_HOUR },
]

// retentionUnitLabel 单位的中文名；未知单位按天（与后端兜底一致）。
export function retentionUnitLabel(unit) {
  return String(unit || '').toLowerCase() === RETENTION_UNIT_HOUR ? '小时' : '天'
}

// retentionText 档位的保留期文本，如"12 小时"、"30 天"。
// 入参是后端 DTO（`retention_value` / `retention_unit`）；缺值返回空串，由调用方决定显示什么。
export function retentionText(tier) {
  const value = tier?.retention_value ?? tier?.retentionValue
  if (value === null || value === undefined || value === '') return ''
  return `${value} ${retentionUnitLabel(tier?.retention_unit ?? tier?.retentionUnit)}`
}

// retentionHours 保留期折算成小时（容量反推与范围校验用；前端只用来显示与提示）。
export function retentionHours(tier) {
  const value = Number(tier?.retention_value ?? tier?.retentionValue ?? 0)
  return retentionUnitLabel(tier?.retention_unit ?? tier?.retentionUnit) === '小时' ? value : value * 24
}

// estimatedTotalGB 按"每天写入量 × 保留期"反推稳态占用（与服务端 estimatedTierTotalGB 同口径，
// 保留 2 位小数）。小时档位必须折算，否则小时档位的预估容量会虚高 24 倍。
export function estimatedTotalGB(dailySizeGB, tier) {
  const days = retentionHours(tier) / 24
  return Math.round((Number(dailySizeGB) || 0) * days * 100) / 100
}
