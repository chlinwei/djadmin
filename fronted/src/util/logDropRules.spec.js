import { describe, expect, it } from 'vitest'
import { compileDropRule, parseDropRule } from './logDropRules'

describe('compileDropRule', () => {
  it('returns [] when there are no rows (未启用)', () => {
    expect(compileDropRule({ logic: 'all', rows: [] })).toEqual([])
    expect(compileDropRule({ logic: 'all', rows: [{ field: '', op: 'eq', value: 'x' }] })).toEqual([])
  })

  it('compiles eq with quoted value', () => {
    expect(compileDropRule({ logic: 'all', rows: [{ field: 'log_level', op: 'eq', value: 'ERROR' }] }))
      .toEqual([{ drop: { if: "(ctx?.log_level == 'ERROR')" } }])
  })

  it('compiles dotted field as safe navigation chain', () => {
    const [processor] = compileDropRule({ logic: 'all', rows: [{ field: 'host.name', op: 'exists', value: '' }] })
    expect(processor.drop.if).toBe('(ctx?.host?.name != null)')
  })

  it('escapes quotes and backslashes in values', () => {
    const [processor] = compileDropRule({ logic: 'all', rows: [{ field: 'service', op: 'eq', value: "a'b\\c" }] })
    expect(processor.drop.if).toBe("(ctx?.service == 'a\\'b\\\\c')")
  })

  it('compiles every operator', () => {
    const compile = (op, value = 'v') => compileDropRule({ logic: 'all', rows: [{ field: 'f', op, value }] })[0].drop.if
    expect(compile('eq')).toBe("(ctx?.f == 'v')")
    expect(compile('ne')).toBe("(ctx?.f != 'v')")
    expect(compile('contains')).toBe("(ctx?.f != null && ctx?.f.toString().contains('v'))")
    expect(compile('not_contains')).toBe("(ctx?.f == null || !ctx?.f.toString().contains('v'))")
    expect(compile('prefix')).toBe("(ctx?.f != null && ctx?.f.toString().startsWith('v'))")
    expect(compile('regex')).toBe("(ctx?.f != null && ctx?.f.toString() =~ /v/)")
    expect(compile('exists', '')).toBe('(ctx?.f != null)')
    expect(compile('not_exists', '')).toBe('(ctx?.f == null)')
  })

  it('joins rows with && for all and || for any', () => {
    const rows = [
      { field: 'a', op: 'eq', value: '1' },
      { field: 'b', op: 'eq', value: '2' },
    ]
    expect(compileDropRule({ logic: 'all', rows })[0].drop.if)
      .toBe("(ctx?.a == '1') && (ctx?.b == '2')")
    expect(compileDropRule({ logic: 'any', rows })[0].drop.if)
      .toBe("(ctx?.a == '1') || (ctx?.b == '2')")
  })

  it('rejects missing value and weird field names', () => {
    expect(() => compileDropRule({ logic: 'all', rows: [{ field: 'f', op: 'eq', value: '' }] })).toThrow(/缺少值/)
    expect(() => compileDropRule({ logic: 'all', rows: [{ field: 'weird-field', op: 'eq', value: 'x' }] })).toThrow(/高级模式/)
  })
})

describe('parseDropRule', () => {
  it('parses empty array as disabled empty card', () => {
    expect(parseDropRule([])).toEqual({ logic: 'all', rows: [] })
  })

  it('round-trips a compiled rule', () => {
    const rows = [
      { field: 'log_level', op: 'eq', value: "er'ror" },
      { field: 'host.name', op: 'contains', value: 'test' },
      { field: 'log_path', op: 'not_exists', value: '' },
    ]
    const items = compileDropRule({ logic: 'any', rows })
    expect(parseDropRule(items)).toEqual({ logic: 'any', rows })
  })

  it('returns null for anything beyond the generated shape', () => {
    expect(parseDropRule([{ drop: {} }])).toBeNull()
    expect(parseDropRule([{ drop: { if: 'true' } }])).toBeNull()
    expect(parseDropRule([{ drop: { if: "(ctx?.f == 'a' && ctx?.g == 'b' || ctx?.h == 'c')" } }])).toBeNull()
    expect(parseDropRule([{ drop_fields: { fields: ['a'] } }])).toBeNull()
    expect(parseDropRule([{ drop: { if: '(ctx?.f == 1)' } }])).toBeNull()
    expect(parseDropRule('not-array')).toBeNull()
  })
})
