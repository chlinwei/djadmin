import { describe, expect, it, vi } from 'vitest'
import { formatStateTooltip } from './logFormatState'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))

describe('日志格式认证提示', () => {
  it('显示真实实例覆盖范围，不把豁免算成通过', () => {
    const text = formatStateTooltip({ format_state: 'waived', format_passed: 1, format_total: 3, format_waived: 2 })
    expect(text).toContain('真实实例通过 1/3')
    expect(text).toContain('豁免 2')
  })

  it('等待样本明确显示重试是否用尽', () => {
    expect(formatStateTooltip({ format_instances: [{ name: 'node-1', status: 'waiting', attempts: 4, reason: '文件为空' }] }))
      .toContain('node-1：等待样本；自动重试已用尽；文件为空')
  })
})
