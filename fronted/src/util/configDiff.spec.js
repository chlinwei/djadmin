import { describe, expect, it } from 'vitest'

import { CONFIG_DIFF_STATUS, DIFF_LINE_ADD, DIFF_LINE_CONTEXT, DIFF_LINE_DEL, MAX_DIFF_LINES, lineDiff } from './configDiff'

// 配置差异的行级 diff：界面靠它显示"下发会把哪几行改掉"。
// 关键正确性要求：未变的行必须判成未变（否则每行都标红，等于没有信息），
// 且行号要能对上两侧原文（用户会拿着行号去主机上核对）。

function compact(result) {
  return result.lines.map((line) => `${line.type === DIFF_LINE_ADD ? '+' : line.type === DIFF_LINE_DEL ? '-' : ' '}${line.text}`)
}

describe('configDiff.lineDiff', () => {
  it('keeps identical lines as context', () => {
    const result = lineDiff('a\nb\nc\n', 'a\nb\nc\n')
    expect(result.truncated).toBe(false)
    expect(compact(result)).toEqual([' a', ' b', ' c'])
    expect(result.lines[1]).toMatchObject({ expectedLine: 2, appliedLine: 2 })
  })

  it('marks one changed line as del + add', () => {
    const result = lineDiff('a\nb\nc\n', 'a\nB\nc\n')
    expect(compact(result)).toEqual([' a', '-B', '+b', ' c'])
  })

  it('marks pure insertions and deletions', () => {
    // 已下发侧多一行（下发时会被删掉）。
    expect(compact(lineDiff('a\nc\n', 'a\nb\nc\n'))).toEqual([' a', '-b', ' c'])
    // 期望侧多一行（下发时会新增）。
    expect(compact(lineDiff('a\nb\nc\n', 'a\nc\n'))).toEqual([' a', '+b', ' c'])
  })

  it('treats a missing side as empty and gives line numbers on that side only', () => {
    const added = lineDiff('x\ny\n', null)
    expect(added.lines.map((line) => line.type)).toEqual([DIFF_LINE_ADD, DIFF_LINE_ADD])
    // 已下发侧不存在 → 那一侧的行号必须是 null，不能是 0（0 会被读成"第 0 行"）。
    expect(added.lines.every((line) => line.appliedLine === null)).toBe(true)
    expect(added.lines.map((line) => line.expectedLine)).toEqual([1, 2])

    const removed = lineDiff(null, 'x\n')
    expect(removed.lines[0]).toMatchObject({ type: DIFF_LINE_DEL, expectedLine: null, appliedLine: 1 })
  })

  it('ignores the trailing newline instead of inventing an empty diff line', () => {
    // 同一个文件"末尾有/没有换行"不该算成一处差异（YAML 里这是常态）。
    const result = lineDiff('a\nb\n', 'a\nb')
    expect(compact(result)).toEqual([' a', ' b'])
  })

  it('falls back to whole-file replace when a side is too large', () => {
    const big = Array.from({ length: MAX_DIFF_LINES + 1 }, (_, index) => `line-${index}`).join('\n')
    const result = lineDiff(big, 'small\n')
    // 不做逐行对齐要**如实标出来**，界面据此提示"差异未逐行对齐"。
    expect(result.truncated).toBe(true)
    expect(result.lines.some((line) => line.type === DIFF_LINE_ADD)).toBe(true)
    expect(result.lines.some((line) => line.type === DIFF_LINE_DEL)).toBe(true)
    expect(result.lines.some((line) => line.type === DIFF_LINE_CONTEXT)).toBe(false)
  })

  it('describes every file status in words the user can act on', () => {
    expect(Object.keys(CONFIG_DIFF_STATUS)).toEqual(['added', 'removed', 'changed', 'unchanged'])
    // "删除"这条最容易误判成"下发不会动它"，所以文案里必须点出全量替换语义。
    expect(CONFIG_DIFF_STATUS.removed.hint).toContain('全量替换')
    expect(CONFIG_DIFF_STATUS.added.label).toBe('新增')
  })
})
