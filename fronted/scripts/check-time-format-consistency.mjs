import fg from 'fast-glob'
import fs from 'node:fs/promises'
import path from 'node:path'

const ROOT = process.cwd()
const TARGET_GLOBS = ['src/**/*.vue']
const TIME_FIELDS = ['update_time', 'create_time', 'start_time', 'end_time', 'run_time']
const ALLOWED_FORMATTERS = ['formatTimeWithTimezone', 'formatDateTime', 'formatTimeDisplay']

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function getLineNumber(content, index) {
  return content.slice(0, index).split('\n').length
}

function checkExpression(expression) {
  const hitFields = TIME_FIELDS.filter((field) => {
    const fieldPattern = new RegExp(`\\b${escapeRegExp(field)}\\b`)
    return fieldPattern.test(expression)
  })

  if (hitFields.length === 0) {
    return false
  }

  const usesAllowedFormatter = ALLOWED_FORMATTERS.some((formatter) => {
    const formatterPattern = new RegExp(`\\b${escapeRegExp(formatter)}\\s*\\(`)
    return formatterPattern.test(expression)
  })

  if (usesAllowedFormatter) {
    return false
  }

  // 允许使用任意函数包装时间字段（例如 formatRecentUpdateTime(record.update_time)）。
  const wrappedByFunction = hitFields.some((field) => {
    const fnWrapPattern = new RegExp(
      `\\b[A-Za-z_$][\\w$]*\\s*\\([^)]*\\b${escapeRegExp(field)}\\b[^)]*\\)`
    )
    return fnWrapPattern.test(expression)
  })

  if (wrappedByFunction) {
    return false
  }

  return true
}

function formatIssue(filePath, line, expression) {
  return `${filePath}:${line} -> ${expression.trim()}`
}

async function main() {
  const files = await fg(TARGET_GLOBS, { cwd: ROOT, absolute: true })
  const issues = []

  for (const absPath of files) {
    const content = await fs.readFile(absPath, 'utf8')
    const relPath = path.relative(ROOT, absPath).replace(/\\/g, '/')

    const interpolationPattern = /\{\{([\s\S]*?)\}\}/g
    let match
    while ((match = interpolationPattern.exec(content)) !== null) {
      const expression = String(match[1] || '')
      if (!checkExpression(expression)) {
        continue
      }

      const line = getLineNumber(content, match.index)
      issues.push(formatIssue(relPath, line, expression))
    }
  }

  if (issues.length > 0) {
    console.error('发现时间字段直接渲染，请统一使用格式化函数:')
    for (const issue of issues) {
      console.error(`- ${issue}`)
    }
    process.exit(1)
  }

  console.log('时间显示一致性检查通过')
}

main().catch((error) => {
  console.error('检查失败:', error)
  process.exit(1)
})
