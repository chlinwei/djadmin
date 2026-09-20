import { describe, expect, it } from 'vitest'

import { RETENTION_UNIT_OPTIONS, estimatedTotalGB, retentionHours, retentionText, retentionUnitLabel } from './logRetention'

// 保留档位的"值 + 单位"展示（迁移 000045 起支持小时）。
// 三个展示点（档位管理页的列表与表单、逻辑服务编辑弹窗的档位下拉）共用这几个函数——
// 少一处没跟上就会出现"档位页写 12 小时、服务弹窗写 12 天"。

describe('logRetention', () => {
  it('formats days and hours with the right unit', () => {
    expect(retentionText({ retention_value: 30, retention_unit: 'd' })).toBe('30 天')
    expect(retentionText({ retention_value: 12, retention_unit: 'h' })).toBe('12 小时')
    // 老数据/接口缺单位时按天（与后端兜底一致，不猜成小时）。
    expect(retentionText({ retention_value: 7 })).toBe('7 天')
    expect(retentionText({ retention_value: 7, retention_unit: '' })).toBe('7 天')
    // 值缺失返回空串，由调用方决定显示什么（不要显示"undefined 天"）。
    expect(retentionText({})).toBe('')
    expect(retentionText(null)).toBe('')
  })

  it('tolerates camelCase payloads (component-local state)', () => {
    expect(retentionText({ retentionValue: 6, retentionUnit: 'h' })).toBe('6 小时')
  })

  it('normalises unknown units to days', () => {
    expect(retentionUnitLabel('h')).toBe('小时')
    expect(retentionUnitLabel('H')).toBe('小时')
    expect(retentionUnitLabel('d')).toBe('天')
    // 分钟不在白名单里（ILM 轮询粒度 10 分钟），非法值按天兜底而不是当成小时。
    expect(retentionUnitLabel('m')).toBe('天')
    expect(retentionUnitLabel(null)).toBe('天')
  })

  it('converts the retention period to hours', () => {
    expect(retentionHours({ retention_value: 3, retention_unit: 'd' })).toBe(72)
    expect(retentionHours({ retention_value: 6, retention_unit: 'h' })).toBe(6)
  })

  it('converts hours when estimating the tier footprint', () => {
    // 容量反推（§4.6）：小时档位不折算会把预估占用虚高 24 倍。
    expect(estimatedTotalGB(10, { retention_value: 30, retention_unit: 'd' })).toBe(300)
    expect(estimatedTotalGB(10, { retention_value: 12, retention_unit: 'h' })).toBe(5)
    expect(estimatedTotalGB(0, { retention_value: 12, retention_unit: 'h' })).toBe(0)
  })

  it('offers exactly the two supported units, days first', () => {
    expect(RETENTION_UNIT_OPTIONS.map((item) => item.value)).toEqual(['d', 'h'])
    expect(RETENTION_UNIT_OPTIONS[0].label).toBe('天')
  })
})
