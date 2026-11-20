import { describe, expect, it } from 'vitest'
import { formatBaselineOutcomeSummary, hasBaselineMetrics } from './baselineOutcome'

describe('baseline outcomes', () => {
  it('only displays compliance metrics for verified targets', () => {
    expect(hasBaselineMetrics('noncompliant')).toBe(true)
    expect(hasBaselineMetrics('interrupted')).toBe(false)
    expect(hasBaselineMetrics('execution_error')).toBe(false)
  })

  it('separates compliance failures from execution errors in a mixed scan', () => {
    expect(formatBaselineOutcomeSummary({ total: 3, noncompliant: 1, execution_error: 2 }))
      .toBe('3 台：0 通过 / 1 存在不符合 / 2 执行异常 / 0 跳过')
  })
})
