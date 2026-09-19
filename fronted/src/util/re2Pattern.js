/**
 * 正则方言（前端侧）：处理规则的**首行正则**由主机上的 Filebeat 执行，Filebeat 与服务端用的都是
 * Go 的 regexp（RE2），不是浏览器/Java 那套正则。与后端 `internal/logcollect/regex_pattern.go`
 * 同源——那边是权威判定（保存与认证时真的编译一次），这里只做两件事：
 *
 *  1. **预检**：把 RE2 明确不支持的写法提前报出来，并给出等价写法（说法与后端一致，
 *     避免"界面说行、服务端说不行"）。现场来源：规则 `springboot-tomcat-exception` 的首行正则写了
 *     `[A-Za-z\u4e00-\u9fa5]`（Java/JS 写中文范围的惯用写法），能存能调试，但主机上的 Filebeat
 *     编译不过 → 多行合并不生效；认证时报 "invalid escape sequence: `\u`"。
 *  2. **预览编译**：把 RE2 的语法翻译成浏览器能编译的形式（`\x{4e00}` → `\u4e00`），
 *     让「在线调试」能用 RE2 写法的正则试跑。没有这一步，用户按提示改成 RE2 写法后，
 *     浏览器的 `new RegExp` 反而抛 "Invalid hexadecimal escape sequence"——等于把人推回错误写法。
 *
 * 这里不是正则引擎，也不做"方言自动改写"（那只会让用户以为平台什么 Java 正则都吃）：
 * 判不了的写法交给服务端那次 RE2 编译来判。
 */

const UNICODE_ESCAPE = 'unicodeEscape'
const LOOKAROUND = 'lookaround'
const BACK_REFERENCE = 'backReference'
const ATOMIC_OR_POSSESSIVE = 'atomicOrPossessive'
// RE2 独有锚点：RE2 认，JS 不认（当字面量），预览会静默失真。
const TEXT_ANCHOR_UNSUPPORTED_BY_JS = 'textAnchorUnsupportedByJs'
const TEXT_ANCHOR_NOT_IN_RE2 = 'textAnchorNotInRe2'

const HINTS = {
  [UNICODE_ESCAPE]: 'RE2 不支持 \\uXXXX（Java/JS 的码点写法），请写成 \\x{XXXX}，例如 \\u4e00 → \\x{4e00}',
  [LOOKAROUND]: 'RE2 不支持断言（(?= / (?! / (?<= / (?<! 一个都不支持）：续行正则不需要断言——'
    + 'Filebeat 用 negate/match=after 表达“不以首行正则开头的行并入上一行”，续行正则留空即可；'
    + '其他场景请把判断挪进 ingest pipeline 处理器',
  [BACK_REFERENCE]: 'RE2 不支持 \\1 这类写法（反向引用与八进制转义都不支持），请改用命名捕获 + pipeline 处理器，或拆成多条规则',
  [ATOMIC_OR_POSSESSIVE]: 'RE2 不支持原子组 (?>) 与占有量词（如 a*+），请去掉它们',
  [TEXT_ANCHOR_NOT_IN_RE2]: 'RE2 不支持 \\Z，文本结尾请用 \\z（\\Z 没有定义）',
  [TEXT_ANCHOR_UNSUPPORTED_BY_JS]: '\\A / \\z 是 RE2 支持的锚点，但浏览器预览还原不了：在线调试对这条会失真，请以服务端认证结果为准',
}

/**
 * 单趟扫描：既收集"RE2 不支持的写法"，也顺手产出浏览器可编译的等价源码。
 * 逐个字符走而不是拿几个正则去搜：`\u` 必须排除 `\\u`（用户要的就是字面反斜杠 + u），
 * `\1` 在字符类里同样是 RE2 不认的写法，盲搜还会把正常规则判成非法。
 *
 * @param {string} pattern 用户填写的正则
 * @returns {{ kinds: string[], source: string, untranslatable: string[] }}
 *   kinds：命中的问题类型（去重）；source：可交给 `new RegExp` 的等价写法；
 *   untranslatable：预览还原不了的写法说明（有值时在线调试结果不可信）。
 */
export function analyzePattern(pattern) {
  const text = String(pattern || '')
  const kinds = []
  const untranslatable = []
  let source = ''
  let inClass = false

  const noteKind = (kind) => {
    if (!kinds.includes(kind)) kinds.push(kind)
  }

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index]

    if (char === '\\') {
      const next = text[index + 1]
      if (next === undefined) {
        source += char
        break
      }
      if (next === 'u') noteKind(UNICODE_ESCAPE)
      else if (next >= '1' && next <= '9') noteKind(BACK_REFERENCE)
      else if (next === 'Z') noteKind(TEXT_ANCHOR_NOT_IN_RE2)
      else if (next === 'A' || next === 'z') {
        noteKind(TEXT_ANCHOR_UNSUPPORTED_BY_JS)
        if (!untranslatable.includes(HINTS[TEXT_ANCHOR_UNSUPPORTED_BY_JS])) {
          untranslatable.push(HINTS[TEXT_ANCHOR_UNSUPPORTED_BY_JS])
        }
      }

      // RE2 的 `\x{XXXX}` 浏览器编译不了，翻译成 JS 的 `\uXXXX`（超出 BMP 的用代理对）。
      if (next === 'x' && text[index + 2] === '{') {
        const end = text.indexOf('}', index + 3)
        const hex = end > 0 ? text.slice(index + 3, end) : ''
        if (/^[0-9a-fA-F]{1,6}$/.test(hex)) {
          const codePoint = parseInt(hex, 16)
          if (codePoint <= 0xffff) {
            source += `\\u${hex.padStart(4, '0')}`
          } else if (inClass) {
            // RE2 的 `[\x{1F600}]` 是"单个码点"，JS 不代 u 标志时只能写成两个代理项，
            // 语义会变成"任一代理项"——宁可不预览，也不给一个错的预览。
            const note = `字符类里的 \\x{${hex}} 是单个多字节码点，浏览器预览无法等价还原`
            if (!untranslatable.includes(note)) untranslatable.push(note)
            source += text.slice(index, end + 1)
          } else {
            const offset = codePoint - 0x10000
            const high = (0xd800 + (offset >> 10)).toString(16)
            const low = (0xdc00 + (offset & 0x3ff)).toString(16)
            source += `\\u${high}\\u${low}`
          }
          index = end
          continue
        }
      }
      source += char + next
      index += 1
      continue
    }

    if (char === '[') {
      // `[[:alpha:]]` 这类 POSIX 类里的 `[` 不改变状态；判错的代价只是"少给一条提示"，
      // 仍有服务端那次 RE2 编译兜底。
      if (!inClass || text[index - 1] !== '[') inClass = !inClass
      source += char
      continue
    }
    if (char === ']') {
      inClass = false
      source += char
      continue
    }

    if (char === '(' && !inClass) {
      const rest = text.slice(index)
      // `(?P<name>` RE2 认，JS 只认 `(?<name>`：翻译一下（两者都不是断言）。
      if (rest.startsWith('(?P<')) {
        source += '(?<'
        index += 3
        continue
      }
      // 四种断言 RE2 一个都不支持；Go 的报错文案还会把 `(?<=` 说成 "invalid named capture"，
      // 照着那句根本想不到是断言，所以这里要把话说全。
      if (rest.startsWith('(?=') || rest.startsWith('(?!')
        || rest.startsWith('(?<=') || rest.startsWith('(?<!')) {
        noteKind(LOOKAROUND)
      } else if (rest.startsWith('(?>')) {
        noteKind(ATOMIC_OR_POSSESSIVE)
      }
      source += char
      continue
    }

    if ((char === '*' || char === '+' || char === '?') && !inClass && text[index + 1] === '+') {
      const prev = index > 0 ? text[index - 1] : ''
      if (prev && prev !== '?' && prev !== '(' && prev !== '|') {
        noteKind(ATOMIC_OR_POSSESSIVE)
      }
    }
    source += char
  }

  return { kinds, source, untranslatable }
}

/** 把命中的问题类型拼成一句能照着改的说明（没有问题时返回空串）。 */
export function describeRe2Problems(kinds) {
  return (kinds || []).map((kind) => HINTS[kind]).filter(Boolean).join('；')
}

/**
 * 预检：正则里出现 RE2 不支持的写法就抛错（错误文案与后端保存/认证时的报错同一套说法）。
 * @param {string} pattern 用户填写的正则
 * @param {string} field 字段名（出现在错误文案开头，如「首行正则」）
 */
export function assertRe2Compatible(pattern, field = '首行正则') {
  const { kinds } = analyzePattern(pattern)
  const problems = describeRe2Problems(kinds.filter((kind) => kind !== TEXT_ANCHOR_UNSUPPORTED_BY_JS))
  if (problems) {
    throw new Error(`${field}不符合 RE2（Filebeat 执行的正是这套方言）：${problems}`)
  }
}

/**
 * 用浏览器正则编译一条 RE2 写法的规则，供「在线调试」试跑。
 *
 * 与真实采集的唯一差异是引擎实现（JS 而非 RE2），支持的语法子集已由 assertRe2Compatible 收窄；
 * 断言类写法本来就会被预检拦下，所以这里不担心 JS 与 RE2 的语义差异。
 */
export function compilePreviewRegExp(pattern, field = '首行正则') {
  assertRe2Compatible(pattern, field)
  const { source, untranslatable } = analyzePattern(pattern)
  if (untranslatable.length) {
    throw new Error(`${field}无法在浏览器预览：${untranslatable.join('；')}`)
  }
  return new RegExp(source)
}
