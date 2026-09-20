// 配置差异的行级 diff（日志中心「查看配置差异」弹窗用）。
//
// 为什么在**前端**算行级 diff：后端已经给了两侧的完整内容（那才是它该负责的——它只能读主机上
// 真实的文件），"怎么显示差异"是展示问题。放在这里可以单测，也不必为渲染样式在后端引 diff 依赖。
//
// 用 LCS（最长公共子序列）做标准统一 diff：片段是几十行 YAML，O(n*m) 完全够用；
// 超过 MAX_DIFF_LINES 时**不做行级对齐**，退回"整体替换"（避免几百 KB 的配置把页面卡住），
// 并把这一点如实标出来（truncated），不假装算过。

// MAX_DIFF_LINES 单侧行数上限：超过就放弃逐行对齐。
export const MAX_DIFF_LINES = 2000

// 行类型：ctx = 两侧都有（未变）、add = 只在期望侧（会写入）、del = 只在已下发侧（会被删除）。
export const DIFF_LINE_CONTEXT = 'ctx'
export const DIFF_LINE_ADD = 'add'
export const DIFF_LINE_DEL = 'del'

function splitLines(text) {
  if (text === null || text === undefined) return []
  // 末尾换行不产生一个"空行"（否则每个文件都会多出一行假差异）。
  const normalized = String(text).replace(/\r\n/g, '\n')
  const lines = normalized.split('\n')
  if (lines.length && lines[lines.length - 1] === '') lines.pop()
  return lines
}

// lineDiff 对比两侧文本，返回逐行结果 [{ type, text, expectedLine, appliedLine }]。
// 行号是 1 基（界面上要标"第 N 行"，0 基容易数错）；该侧没有这一行时为 null。
export function lineDiff(expectedText, appliedText) {
  const expected = splitLines(expectedText)
  const applied = splitLines(appliedText)
  if (expected.length > MAX_DIFF_LINES || applied.length > MAX_DIFF_LINES) {
    // 太大：不做对齐，直接"全删 + 全加"，并标出来（调用方据此提示"差异未逐行对齐"）。
    return {
      lines: [
        ...applied.map((text, index) => ({ type: DIFF_LINE_DEL, text, expectedLine: null, appliedLine: index + 1 })),
        ...expected.map((text, index) => ({ type: DIFF_LINE_ADD, text, expectedLine: index + 1, appliedLine: null })),
      ],
      truncated: true,
    }
  }

  // LCS 长度表：table[i][j] = expected[i:] 与 applied[j:] 的最长公共子序列长度。
  const table = Array.from({ length: expected.length + 1 }, () => new Array(applied.length + 1).fill(0))
  for (let i = expected.length - 1; i >= 0; i -= 1) {
    for (let j = applied.length - 1; j >= 0; j -= 1) {
      table[i][j] = expected[i] === applied[j]
        ? table[i + 1][j + 1] + 1
        : Math.max(table[i + 1][j], table[i][j + 1])
    }
  }

  const lines = []
  let i = 0
  let j = 0
  while (i < expected.length && j < applied.length) {
    if (expected[i] === applied[j]) {
      lines.push({ type: DIFF_LINE_CONTEXT, text: expected[i], expectedLine: i + 1, appliedLine: j + 1 })
      i += 1
      j += 1
      continue
    }
    // 与 LCS 表一致的方向：比较"丢掉哪一侧的当前行更不亏"。
    // table[i+1][j] 是丢掉 expected[i] 之后的最优值：它不小于 table[i][j+1] 时，说明 expected[i]
    // **不在**公共子序列里 → 它是"新增"（只在期望侧）。反过来则是"删除"（只在已下发侧）。
    if (table[i + 1][j] >= table[i][j + 1]) {
      lines.push({ type: DIFF_LINE_ADD, text: expected[i], expectedLine: i + 1, appliedLine: null })
      i += 1
    } else {
      lines.push({ type: DIFF_LINE_DEL, text: applied[j], expectedLine: null, appliedLine: j + 1 })
      j += 1
    }
  }
  while (j < applied.length) {
    lines.push({ type: DIFF_LINE_DEL, text: applied[j], expectedLine: null, appliedLine: j + 1 })
    j += 1
  }
  while (i < expected.length) {
    lines.push({ type: DIFF_LINE_ADD, text: expected[i], expectedLine: i + 1, appliedLine: null })
    i += 1
  }
  return { lines: groupChangeRuns(lines), truncated: false }
}

// groupChangeRuns 把每一段"改动块"里的顺序整成**先删除、后新增**（统一 diff 的惯例，读起来是
// "现在是这样 → 将变成这样"）。改动块内的行集合不变，只是顺序——所以不影响正确性，只影响可读性。
// 行号仍然挂在各自那一行上（两侧各自连续，跨侧不保证单调）。
function groupChangeRuns(lines) {
  const result = []
  let index = 0
  while (index < lines.length) {
    if (lines[index].type === DIFF_LINE_CONTEXT) {
      result.push(lines[index])
      index += 1
      continue
    }
    const run = []
    while (index < lines.length && lines[index].type !== DIFF_LINE_CONTEXT) {
      run.push(lines[index])
      index += 1
    }
    result.push(...run.filter((line) => line.type === DIFF_LINE_DEL), ...run.filter((line) => line.type === DIFF_LINE_ADD))
  }
  return result
}

// CONFIG_DIFF_STATUS 文件状态 → 展示用的文案与颜色（与后端 status 一一对应）。
export const CONFIG_DIFF_STATUS = {
  added: { label: '新增', color: 'green', hint: '主机上没有这个片段，下发后会被写入。' },
  removed: { label: '删除', color: 'red', hint: '主机上有、期望里没有：下发是**全量替换**，它会被删掉。' },
  changed: { label: '修改', color: 'orange', hint: '两边都有但内容不同，下发后会被覆盖。' },
  unchanged: { label: '未变', color: 'default', hint: '内容一致，下发不会改动它。' },
}
