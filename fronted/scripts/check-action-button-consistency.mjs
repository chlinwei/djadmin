#!/usr/bin/env node

import fs from 'node:fs'
import path from 'node:path'

const projectRoot = path.resolve(process.cwd())
const viewsRoot = path.join(projectRoot, 'src', 'views')

// These pages predate the executable button standard. New action-column pages are strict by default.
const legacyActionFiles = new Set([
  'src/views/assets/application/components/ApplicationServiceDialog.vue',
  'src/views/assets/application/components/ApplicationWorkspace.vue',
  'src/views/assets/application/components/TemplateDialog.vue',
  'src/views/assets/application/components/VersionDialog.vue',
  'src/views/assets/credential/index.vue',
  'src/views/assets/environments/index.vue',
  'src/views/assets/host/index.vue',
  'src/views/assets/projects/index.vue',
  'src/views/audit/operationLog/index.vue',
  'src/views/audit/webssh/index.vue',
  'src/views/automation/inventory/index.vue',
  'src/views/automation/logs/tabs/JobRunRecordsTab/index.vue',
  'src/views/automation/logs/tabs/MonitorInstallHistoryTab/index.vue',
  'src/views/automation/logs/tabs/WorkflowRunRecordsTab/index.vue',
  'src/views/automation/templates/index.vue',
  'src/views/automation/workflow/list/index.vue',
  'src/views/monitor/index.vue',
  'src/views/monitor/log-parsers/index.vue',
  'src/views/monitor/log-retention/index.vue',
  'src/views/monitor/log-storage/index.vue',
  'src/views/sys/menu/index.vue',
  'src/views/sys/role/index.vue',
  'src/views/sys/scheduler/index.vue',
  'src/views/sys/sysconfig/index.vue',
  'src/views/sys/user/index.vue',
])

const strictActionFiles = new Set([
  'src/views/automation/automationtask/components/TaskListCard/index.vue',
  'src/views/inspection/index.vue',
])

const actionRules = {
  编辑: {
    attributes: [
      ['size="small"', /\bsize\s*=\s*["']small["']/],
      ['type="primary"', /\btype\s*=\s*["']primary["']/],
    ],
    icon: /\b(?:pen-to-square|edit)\b/,
    expectedIcon: /\bpen-to-square\b/,
    iconName: 'pen-to-square',
  },
  运行: {
    attributes: [
      ['size="small"', /\bsize\s*=\s*["']small["']/],
      ['type="primary"', /\btype\s*=\s*["']primary["']/],
      ['ghost', /(?:^|\s)ghost(?:\s|=|$)/],
    ],
    icon: /["']play["']/,
    expectedIcon: /["']play["']/,
    iconName: 'play',
  },
  删除: {
    attributes: [
      ['class="delBtn"', /\bclass\s*=\s*["'][^"']*\bdelBtn\b[^"']*["']/],
      ['size="small"', /\bsize\s*=\s*["']small["']/],
      ['type="primary"', /\btype\s*=\s*["']primary["']/],
      ['danger', /(?:^|\s)danger(?:\s|=|$)/],
    ],
    icon: /\btrash-can\b/,
    expectedIcon: /\btrash-can\b/,
    iconName: 'trash-can',
  },
}

function walkVueFiles(directory, result = []) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const absolutePath = path.join(directory, entry.name)
    if (entry.isDirectory()) walkVueFiles(absolutePath, result)
    else if (entry.isFile() && entry.name.endsWith('.vue')) result.push(absolutePath)
  }
  return result
}

function lineNumberAt(content, index) {
  return content.slice(0, index).split('\n').length
}

function validateActionButtons(content, relativePath) {
  const errors = []
  const buttonPattern = /<a-button\b[^>]*>[\s\S]*?<\/a-button>/g
  let match
  while ((match = buttonPattern.exec(content)) !== null) {
    const block = match[0]
    const action = Object.keys(actionRules).find((name) => actionRules[name].icon.test(block))
    if (!action) continue
    const buttonTag = block.match(/<a-button\b[^>]*>/)[0]
    const line = lineNumberAt(content, match.index)
    const prefix = content.slice(Math.max(0, match.index - 300), match.index)
    const tooltipPattern = new RegExp(`<a-tooltip\\b[^>]*\\btitle\\s*=\\s*["']${action}["'][^>]*>\\s*$`)
    if (!tooltipPattern.test(prefix)) {
      errors.push(`${relativePath}:${line} [${action}] 按钮必须由 title="${action}" 的 a-tooltip 直接包裹`)
    }
    const rule = actionRules[action]
    for (const [attributeName, pattern] of rule.attributes) {
      if (!pattern.test(buttonTag)) {
        errors.push(`${relativePath}:${line} [${action}] 按钮必须配置 ${attributeName}`)
      }
    }
    if (!rule.expectedIcon.test(block)) {
      errors.push(`${relativePath}:${line} [${action}] 必须使用 ${rule.iconName} 图标`)
    }
  }
  return errors
}

const files = walkVueFiles(viewsRoot)
const errors = []
let strictFileCount = 0

for (const absolutePath of files) {
  const content = fs.readFileSync(absolutePath, 'utf8')
  const relativePath = path.relative(projectRoot, absolutePath).replace(/\\/g, '/')
  const hasActionColumn = /column\.key\s*={2,3}\s*["']action["']/.test(content)
  const isStrict = strictActionFiles.has(relativePath) || (hasActionColumn && !legacyActionFiles.has(relativePath))
  if (!isStrict) continue
  strictFileCount += 1
  errors.push(...validateActionButtons(content, relativePath))
}

console.log(`[check:action-buttons] strict files=${strictFileCount}, violations=${errors.length}`)
if (errors.length > 0) {
  for (const error of errors) console.error(error)
  process.exit(1)
}
console.log('[check:action-buttons] passed.')