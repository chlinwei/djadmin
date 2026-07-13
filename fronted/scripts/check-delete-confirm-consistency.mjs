#!/usr/bin/env node

import fs from 'node:fs'
import path from 'node:path'

const projectRoot = path.resolve(process.cwd())
const viewsRoot = path.join(projectRoot, 'src', 'views')

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

function collectDeleteCheckErrors(absFile) {
  const content = fs.readFileSync(absFile, 'utf8')
  const relFile = path.relative(projectRoot, absFile).replace(/\\/g, '/')
  const errors = []

  // Rule 1: deleting actions must not use a-popconfirm anymore.
  const popconfirmDeleteRegex = /<a-popconfirm[^>]*title\s*=\s*["'][^"']*删[^"']*["'][^>]*>/g
  let popMatch = null
  while ((popMatch = popconfirmDeleteRegex.exec(content)) !== null) {
    errors.push({
      type: 'delete-popconfirm-forbidden',
      file: relFile,
      line: lineNumberAt(content, popMatch.index),
      detail: '删除动作禁止使用 a-popconfirm，请改为 openDeleteConfirm。',
    })
  }

  // Rule 2: delete tooltip button must carry delBtn class to keep width stable.
  const deleteTooltipRegex = /<a-tooltip[^>]*title\s*=\s*["']删除["'][^>]*>[\s\S]*?<\/a-tooltip>/g
  let tooltipMatch = null
  while ((tooltipMatch = deleteTooltipRegex.exec(content)) !== null) {
    const block = tooltipMatch[0]
    const hasButton = /<a-button\b/.test(block)
    const hasDelBtnClass = /<a-button[^>]*class\s*=\s*["'][^"']*\bdelBtn\b[^"']*["']/.test(block)
    if (hasButton && !hasDelBtnClass) {
      errors.push({
        type: 'delete-button-missing-delBtn',
        file: relFile,
        line: lineNumberAt(content, tooltipMatch.index),
        detail: '删除按钮缺少 delBtn 类，点击/loading 可能导致宽度抖动。',
      })
    }
  }

  return errors
}

const vueFiles = walkVueFiles(viewsRoot)
const allErrors = vueFiles.flatMap(collectDeleteCheckErrors)

console.log(`[check:delete] scan finished. files=${vueFiles.length}, violations=${allErrors.length}`)

if (allErrors.length > 0) {
  for (const item of allErrors) {
    console.error(`${item.file}:${item.line} [${item.type}] ${item.detail}`)
  }
  process.exit(1)
}

console.log('[check:delete] passed.')
