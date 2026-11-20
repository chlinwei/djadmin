const outcomes = {
  pending: ['等待中', 'default'],
  running: ['扫描中', 'processing'],
  success: ['完成', 'green'],
  compliant: ['通过', 'green'],
  noncompliant: ['存在不符合', 'red'],
  execution_error: ['执行异常', 'orange'],
  interrupted: ['执行中断 · 结果未确认', 'orange'],
  mixed: ['存在不符合及执行异常', 'orange'],
  skipped: ['已跳过', 'default'],
  canceled: ['已取消', 'default']
}

export const statusLabel = (outcome) => outcomes[outcome]?.[0] ?? '—'
export const statusColor = (outcome) => outcomes[outcome]?.[1] ?? 'default'
export const hasBaselineMetrics = (outcome) => outcome === 'compliant' || outcome === 'noncompliant'

export function formatBaselineOutcomeSummary(summary) {
  if (!summary?.total) return '—'
  const parts = [
    `${summary.compliant ?? 0} 通过`, `${summary.noncompliant ?? 0} 存在不符合`,
    `${summary.execution_error ?? 0} 执行异常`, `${summary.skipped ?? 0} 跳过`
  ]
  for (const [key, label] of [['pending', '等待中'], ['running', '扫描中'], ['canceled', '取消']]) {
    if (summary[key] > 0) parts.push(`${summary[key]} ${label}`)
  }
  return `${summary.total} 台：${parts.join(' / ')}`
}
