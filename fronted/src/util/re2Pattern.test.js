import { describe, expect, it } from 'vitest'
import { analyzePattern, assertRe2Compatible, compilePreviewRegExp, describeRe2Problems } from './re2Pattern'

// 现场来源（2026-09-19）：处理规则 springboot-tomcat-exception 的首行正则写成
// `\u4e00`（Java/JS 写中文范围的惯用写法），在浏览器里能编译、能调试，但主机上的 Filebeat
// 用 Go RE2 编译直接失败 → 多行合并不生效，认证时报 "invalid escape sequence: `\u`"。
const 现场错误写法 = '^(\\d{4}-\\d{2}-\\d{2}|\\d{2}-[A-Za-z\\u4e00-\\u9fa5]{3,4}-\\d{4})'
const 现场修正写法 = '^(\\d{4}-\\d{2}-\\d{2}|\\d{2}-[A-Za-z\\x{4e00}-\\x{9fa5}]{3,4}-\\d{4})'

describe('RE2 方言预检（前端侧与后端保存/认证同一套说法）', () => {
  it('Java/JS 的 \\uXXXX 码点写法被拦下，并告诉用户写成 \\x{XXXX}', () => {
    expect(() => assertRe2Compatible(现场错误写法)).toThrow(/\\u4e00 → \\x\{4e00\}/)
    expect(() => assertRe2Compatible(现场错误写法)).toThrow(/首行正则不符合 RE2/)
  })

  it('改写为 RE2 写法后通过', () => {
    expect(() => assertRe2Compatible(现场修正写法)).not.toThrow()
  })

  it('断言（含续行最自然的否定前瞻）被拦下，并给出多行的正解', () => {
    const problem = describeRe2Problems(analyzePattern('^(?!(\\d{4}-\\d{2}-\\d{2}))').kinds)
    expect(problem).toContain('不支持断言')
    expect(problem).toContain('续行正则留空即可')
  })

  it('反向引用与原子组/占有量词被拦下', () => {
    expect(describeRe2Problems(analyzePattern('(\\w+)\\s+\\1').kinds)).toContain('反向引用与八进制转义都不支持')
    expect(describeRe2Problems(analyzePattern('a*+').kinds)).toContain('占有量词')
    expect(describeRe2Problems(analyzePattern('(?>a)').kinds)).toContain('原子组')
  })

  it('转义后的字面反斜杠不算码点写法（\\\\u4e00 是"反斜杠 + u4e00"）', () => {
    expect(analyzePattern('\\\\u4e00').kinds).toEqual([])
  })

  it('RE2 支持的能力不受影响：命名组 / 内联标志 / 字符类 / POSIX 类', () => {
    expect(() => assertRe2Compatible('(?i)^(?P<date>\\d{4}-\\d{2}-\\d{2})[[:space:]]+ERROR[\\-]')).not.toThrow()
  })
})

describe('在线调试的正则编译（把 RE2 写法翻译成浏览器能编译的形式）', () => {
  it('\\x{XXXX} 翻译成 JS 能编译的 \\uXXXX，且匹配行为一致', () => {
    const expression = compilePreviewRegExp(现场修正写法)
    expect(expression.source).toContain('\\u4e00')
    // 两个分支各自命中一次：数字日期开头，以及 `\d{2}-[A-Za-z汉字]{3,4}-\d{4}`。
    // 中文那行是重点——它证明 `\x{4e00}-\x{9fa5}` 这段范围在翻译后仍然成立。
    expect(expression.test('2026-09-19 10:00:00 ERROR')).toBe(true)
    expect(expression.test('19-Sep-2026 10:00:00 ERROR')).toBe(true)
    expect(expression.test('19-中文测-2026 10:00:00 ERROR')).toBe(true)
    expect(expression.test('随便一行续行')).toBe(false)
  })

  it('字符类里的多字节码点不做错预览：宁可报"预览不了"，也不给出错的匹配结果', () => {
    expect(() => compilePreviewRegExp('^[\\x{1F600}]')).toThrow(/浏览器预览无法等价还原/)
  })

  it('\\A / \\z 是 RE2 支持但浏览器还原不了的锚点：预览明确说失真，不静默给错结果', () => {
    expect(() => assertRe2Compatible('\\Afoo\\z')).not.toThrow()
    expect(() => compilePreviewRegExp('\\Afoo\\z')).toThrow(/在线调试对这条会失真/)
    // \Z 则是 RE2 本身就不认的写法，要在预检阶段被拦下。
    expect(() => assertRe2Compatible('foo\\Z')).toThrow(/RE2 不支持 \\Z/)
  })

  it('非法写法不会进入编译：预检先报错，而不是抛浏览器那句看不懂的 SyntaxError', () => {
    expect(() => compilePreviewRegExp(现场错误写法)).toThrow(/RE2/)
    expect(() => compilePreviewRegExp(现场错误写法)).not.toThrow(/SyntaxError/)
  })
})
