// 服务级后处理「条件丢弃整条日志」的编译与解析（2026-09-28）。
//
// 存储（assets_application_service_log_setting.extra_processors）不变：仍是 ES 处理器数组。
// 本模块负责条件行 UI 与处理器 JSON 的双向转换：
//   - 编译：条件行 [{field, op, value}] + 组合方式(全部/任一) → [{ drop: { if: '<painless>' } }]
//   - 解析：处理器数组 → 能还原成条件行就还原；任何认不出的形状返回 null，调用方落到「高级模式」
//
// 为什么条件用 painless `if`：ES ingest 处理器没有 Filebeat 那种 when 语法，条件统一走
// 处理器的公共参数 if。生成物是固定形状（每条条件带括号、按 &&/|| 连接、字段访问链固定），
// 解析就是按同一形状反拆——形状之外的内容一律不猜，交给高级模式原样保留。

// 条件可选的字段建议（ES 阶段真实存在的字段，可自由输入其它）。
export const DROP_FIELD_SUGGESTIONS = [
  'log_level', 'log_message', 'message', 'service', 'instance', 'log_path',
  'host_ip', 'log_name', 'application', 'project', 'environment', 'business_system',
]

export const DROP_OPERATORS = [
  { value: 'eq', label: '等于', needsValue: true },
  { value: 'ne', label: '不等于', needsValue: true },
  { value: 'contains', label: '包含', needsValue: true },
  { value: 'not_contains', label: '不包含', needsValue: true },
  { value: 'prefix', label: '开头是', needsValue: true },
  { value: 'regex', label: '正则匹配', needsValue: true },
  { value: 'exists', label: '存在', needsValue: false },
  { value: 'not_exists', label: '不存在', needsValue: false },
]

export function dropOperatorMeta(op) {
  return DROP_OPERATORS.find((item) => item.value === op) || null
}

// 字段名限定为标识符点路径：编译成 painless 安全导航链（ctx?.a?.b），也保证解析可逆。
// 含其它字符（如带连字符、带引号）的字段名编译不出可靠的条件，让调用方落到高级模式。
const FIELD_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/

function painlessString(value) {
  return `'${String(value).replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`
}

// 字段访问链：按点拆成嵌套路径（host.name → ctx?.host?.name）。
// Filebeat 事件里的多段字段本来就是嵌套对象，字面键带点的情况交给高级模式手写。
function accessor(field) {
  const parts = String(field).split('.')
  return `ctx?.${parts.join('?.')}`
}

function clause(field, op, value) {
  const target = accessor(field)
  switch (op) {
    case 'eq':
      return `(${target} == ${painlessString(value)})`
    case 'ne':
      return `(${target} != ${painlessString(value)})`
    case 'contains':
      return `(${target} != null && ${target}.toString().contains(${painlessString(value)}))`
    case 'not_contains':
      return `(${target} == null || !${target}.toString().contains(${painlessString(value)}))`
    case 'prefix':
      return `(${target} != null && ${target}.toString().startsWith(${painlessString(value)}))`
    case 'regex':
      // painless 正则字面量里 \ 与 / 都要转义。
      return `(${target} != null && ${target}.toString() =~ /${String(value).replace(/\\/g, '\\\\').replace(/\//g, '\\/')}/)`
    case 'exists':
      return `(${target} != null)`
    case 'not_exists':
      return `(${target} == null)`
    default:
      throw new Error('不支持的条件类型')
  }
}

// compileDropRule 把条件行编译成处理器数组。
// rows 为空 = 未启用（返回 []，保存后片段回到只走解析规则）。
// 行不完整（缺字段/缺值）时抛错，文案可直接展示；含怪字段名时提示改用高级模式。
export function compileDropRule({ logic = 'all', rows = [] } = {}) {
  const valid = rows.filter((row) => row && String(row.field || '').trim() !== '')
  if (valid.length === 0) return []
  const meta = rows.map((row) => {
    const field = String(row.field || '').trim()
    if (field && !FIELD_PATTERN.test(field)) {
      throw new Error(`字段名「${field}」超出结构化配置的表达范围（仅支持字母/数字/下划线的点路径），请改用高级模式直接编辑 JSON`)
    }
    const operator = dropOperatorMeta(row.op)
    if (!operator) throw new Error('不支持的条件类型')
    if (operator.needsValue && String(row.value ?? '').trim() === '') {
      throw new Error(`条件「${field} ${operator.label}」缺少值`)
    }
    return clause(field, row.op, row.value ?? '')
  })
  const joiner = logic === 'any' ? ' || ' : ' && '
  // 每条子句自带括号，直接拼接即可；解析侧按同一形状反拆（见 parseIfExpression）。
  return [{ drop: { if: meta.join(joiner) } }]
}

// parseDropRule 把处理器数组还原成条件行；不可还原（形状超出本模块生成物）时返回 null，
// 调用方应落入「高级模式」原样编辑，绝不丢用户手写的内容。
// 返回 { logic, rows }；空数组 → { logic: 'all', rows: [] }（未启用的空卡片）。
export function parseDropRule(items) {
  if (!Array.isArray(items)) return null
  if (items.length === 0) return { logic: 'all', rows: [] }
  if (items.length !== 1) return null
  const item = items[0]
  if (!item || typeof item !== 'object' || Array.isArray(item)) return null
  // 恰好一个处理器键 drop，且配置里只有 if（本工具的生成物形状）。
  if (Object.keys(item).length !== 1 || !item.drop || typeof item.drop !== 'object') return null
  if (Object.keys(item.drop).length !== 1 || typeof item.drop.if !== 'string') return null
  return parseIfExpression(item.drop.if)
}

// ── if 表达式反拆 ────────────────────────────────────────────────────────────
// 生成物固定为 (clause) joiner (clause)…，顶层按深度 0 的 && / || 切分即可；
// 两种连接符同时出现（或括号不平衡）说明不是本工具生成的 → null。

const ACCESSOR = String.raw`ctx(?:\?\.[A-Za-z_][A-Za-z0-9_]*)+`
// painless 单引号字符串的值：允许任意转义序列，但不允许裸引号。
const VALUE = String.raw`(?:[^'\\]|\\.)*`

const CLAUSE_PATTERNS = [
  // eq/ne 的值捕获组**带引号**（'…'），contains/prefix 的值在 toString().contains('…') 里、
  // 捕获组不带引号——parseClause 按 op 分别剥离，所以这里两 shapes 都按生成物原样写。
  { op: 'eq', re: new RegExp(String.raw`^\((${ACCESSOR}) == ('${VALUE}')\)$`), quoted: true },
  { op: 'ne', re: new RegExp(String.raw`^\((${ACCESSOR}) != ('${VALUE}')\)$`), quoted: true },
  { op: 'contains', re: new RegExp(String.raw`^\((${ACCESSOR}) != null && \1\.toString\(\)\.contains\('(${VALUE})'\)\)$`) },
  { op: 'not_contains', re: new RegExp(String.raw`^\((${ACCESSOR}) == null \|\| !\1\.toString\(\)\.contains\('(${VALUE})'\)\)$`) },
  { op: 'prefix', re: new RegExp(String.raw`^\((${ACCESSOR}) != null && \1\.toString\(\)\.startsWith\('(${VALUE})'\)\)$`) },
  { op: 'regex', re: new RegExp(String.raw`^\((${ACCESSOR}) != null && \1\.toString\(\) =~ \/(.*)\/\)$`), slashed: true },
  { op: 'exists', re: new RegExp(String.raw`^\((${ACCESSOR}) != null\)$`) },
  { op: 'not_exists', re: new RegExp(String.raw`^\((${ACCESSOR}) == null\)$`) },
]

function unescapePainless(raw) {
  return raw.replace(/\\(.)/g, '$1')
}

function parseField(accessorText) {
  if (!accessorText.startsWith('ctx?.')) return null
  const path = accessorText.slice('ctx?.'.length)
  const parts = path.split('?.')
  if (parts.some((part) => !/^[A-Za-z_][A-Za-z0-9_]*$/.test(part))) return null
  return parts.join('.')
}

function parseClause(text) {
  for (const { op, re, quoted, slashed } of CLAUSE_PATTERNS) {
    const match = text.match(re)
    if (!match) continue
    const field = parseField(match[1])
    if (!field) return null
    if (op === 'exists' || op === 'not_exists') return { field, op, value: '' }
    if (quoted) return { field, op, value: unescapePainless(match[2].slice(1, -1)) }
    if (slashed) return { field, op, value: unescapePainless(match[2]) }
    return { field, op, value: unescapePainless(match[2]) }
  }
  return null
}

function parseIfExpression(expression) {
  const text = expression.trim()
  if (!text) return null
  // 顶层切分：深度 0 处的 ' && ' / ' || '。引号内的内容不算分隔符（值里可能字面含 " && "）。
  const separators = []
  let depth = 0
  let inQuote = false
  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i]
    if (inQuote) {
      if (ch === '\\') i += 1
      else if (ch === "'") inQuote = false
      continue
    }
    if (ch === "'") inQuote = true
    else if (ch === '(') depth += 1
    else if (ch === ')') depth -= 1
    else if (depth === 0 && text.startsWith(' && ', i)) separators.push({ at: i, logic: 'all' })
    else if (depth === 0 && text.startsWith(' || ', i)) separators.push({ at: i, logic: 'any' })
  }
  if (inQuote || depth !== 0) return null
  const logics = new Set(separators.map((item) => item.logic))
  if (logics.size > 1) return null
  const logic = logics.size ? logics.values().next().value : 'all'
  const parts = []
  let cursor = 0
  for (const sep of separators) {
    parts.push(text.slice(cursor, sep.at))
    cursor = sep.at + 4
  }
  parts.push(text.slice(cursor))
  const rows = []
  for (const part of parts) {
    const row = parseClause(part.trim())
    if (!row) return null
    rows.push(row)
  }
  return { logic, rows }
}
