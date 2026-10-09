import { describe, expect, it, vi } from 'vitest'
import { formatStateTooltip } from './logFormatState'

vi.mock('@/store', () => ({ default: { state: { user: { timezone: 'Asia/Shanghai' } } } }))

describe('日志格式认证提示', () => {
  it('需重新验证提示区分实际解析配置与规则描述变更', () => {
    const text = formatStateTooltip({ format_state: 'needs_recheck' })
    expect(text).toContain('实际解析配置')
    expect(text).toContain('修改规则描述、备注或原样保存不会触发重新验证')
  })

  it('已验证继续显示配置指纹未变', () => {
    expect(formatStateTooltip({ format_state: 'verified', format_verified_source: 'instance' }))
      .toContain('配置指纹未变，不需要再检查')
  })
})
