#!/usr/bin/env node

import fs from 'node:fs'
import path from 'node:path'

const projectRoot = path.resolve(process.cwd())
const viewsRoot = path.join(projectRoot, 'src', 'views')

// 时间范围选择器必须复用统一交互：用户时区默认值 + 快捷预设 + 打开时刷新预设 + 显式弹层容器。
const REQUIRED_RULES = [
  {
    type: 'range-picker-missing-show-time-binding',
    test: (block) => /:show-time\s*=\s*["']/.test(block),
    detail: '缺少 :show-time 绑定，请使用 buildUserTimezoneShowTime 生成的用户时区默认时间。',
  },
  {
    type: 'range-picker-missing-presets',
    test: (block) => /:presets\s*=\s*["']/.test(block),
    detail: '缺少 :presets，请使用 buildUserTimezoneRangePresets 提供统一快捷区间。',
  },
  {
    type: 'range-picker-missing-open-change',
    test: (block) => /@openChange\s*=\s*["']/.test(block),
    detail: '缺少 @openChange，预设不会按用户当前时区刷新。',
  },
  {
    type: 'range-picker-missing-popup-container',
    test: (block) => /:getPopupContainer\s*=\s*["']/.test(block),
    detail: '缺少 :getPopupContainer，弹层挂载行为会与其他页面不一致。',
  },
  {
    type: 'range-picker-inconsistent-placeholder',
    test: (block) => /:placeholder\s*=\s*"\[\s*'开始时间'\s*,\s*'结束时间'\s*\]"/.test(block),
    detail: "placeholder 必须统一为 :placeholder=\"['开始时间', '结束时间']\"。",
  },
]

function walkVueFiles(dir, out = []) {
  if (!fs.existsSync(dir)) {
    return out
  }
  const entries = fs.readdirSync(dir, { withFileTypes: true })
  for (const entry of entries) {
    const absPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      walkVueFiles(absPath, out)
      continue
    }
    if (entry.isFile() && absPath.endsWith('.vue')) {
      out.push(absPath)
    }
  }
  return out
}

function lineNumberAt(content, index) {
  return content.slice(0, index).split('\n').length
}

function collectRangePickerErrors(absFile) {
  const content = fs.readFileSync(absFile, 'utf8')
  const relFile = path.relative(projectRoot, absFile).replace(/\\/g, '/')
  const errors = []

  const rangePickerRegex = /<a-range-picker\b[\s\S]*?\/?>/g
  let match = null
  while ((match = rangePickerRegex.exec(content)) !== null) {
    const block = match[0]
    const line = lineNumberAt(content, match.index)
    for (const rule of REQUIRED_RULES) {
      if (!rule.test(block)) {
        errors.push({ type: rule.type, file: relFile, line, detail: rule.detail })
      }
    }
  }

  return errors
}

const vueFiles = walkVueFiles(viewsRoot)
const pickerFiles = vueFiles.filter((file) => /<a-range-picker\b/.test(fs.readFileSync(file, 'utf8')))
const allErrors = pickerFiles.flatMap(collectRangePickerErrors)

console.log(`[check:range-picker] scan finished. files=${pickerFiles.length}, violations=${allErrors.length}`)

if (allErrors.length > 0) {
  for (const item of allErrors) {
    console.error(`${item.file}:${item.line} [${item.type}] ${item.detail}`)
  }
  process.exit(1)
}

console.log('[check:range-picker] passed.')
