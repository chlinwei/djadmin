import { describe, expect, it } from 'vitest'

import { filterApplicationGroups, isApplicationFilterMissed } from './applicationFilter'

const groups = [
  { key: 'all', label: '全部规则', count: 7 },
  { key: '5', label: 'Tomcat', code: 'tomcat', count: 1 },
  { key: '8', label: 'Redis', code: 'redis', count: 2 },
  { key: '16', label: '中间件', code: 'cdm', count: 0 },
  { key: 'generic', label: '通用（不限应用）', count: 2 },
]

describe('filterApplicationGroups', () => {
  it('关键词为空（含全空白）时原样返回', () => {
    expect(filterApplicationGroups(groups, '')).toEqual(groups)
    expect(filterApplicationGroups(groups, '   ')).toEqual(groups)
    expect(filterApplicationGroups(groups, undefined)).toEqual(groups)
  })

  it('按名称匹配，且不区分大小写', () => {
    const result = filterApplicationGroups(groups, 'redis')
    expect(result.map((item) => item.key)).toEqual(['all', '8'])
    expect(filterApplicationGroups(groups, 'ToM')).toEqual(filterApplicationGroups(groups, 'tom'))
  })

  it('按编码匹配（中文名 + 英文编码时只输编码也能找到）', () => {
    const result = filterApplicationGroups(groups, 'cdm')
    expect(result.map((item) => item.key)).toEqual(['all', '16'])
  })

  it('「全部规则」始终保留，哪怕关键词只命中它一个', () => {
    const result = filterApplicationGroups(groups, '全部')
    expect(result.map((item) => item.key)).toEqual(['all'])
    // 但"通用"能被自己的名字命中（它不是保留项）。
    expect(filterApplicationGroups(groups, '通用').map((item) => item.key)).toEqual(['all', 'generic'])
  })

  it('一条都没命中时只剩「全部规则」', () => {
    expect(filterApplicationGroups(groups, '不存在的东西').map((item) => item.key)).toEqual(['all'])
  })
})

describe('isApplicationFilterMissed', () => {
  it('筛空了为真，其余为假', () => {
    const missed = filterApplicationGroups(groups, '不存在的东西')
    expect(isApplicationFilterMissed(missed, '不存在的东西')).toBe(true)

    const hit = filterApplicationGroups(groups, 'redis')
    expect(isApplicationFilterMissed(hit, 'redis')).toBe(false)

    // 没输关键词不算"筛空"（此时只剩全部的情形不存在，但语义上必须区分开）。
    expect(isApplicationFilterMissed(groups, '')).toBe(false)
    expect(isApplicationFilterMissed(groups, '   ')).toBe(false)
  })
})
