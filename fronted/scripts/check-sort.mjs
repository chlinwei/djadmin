#!/usr/bin/env node

import fs from 'node:fs'
import path from 'node:path'

const projectRoot = path.resolve(process.cwd())
const viewsRoot = path.join(projectRoot, 'src', 'views')

function escapeRegExp(text) {
  // Escape user-provided field names before embedding into dynamic RegExp patterns.
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function runSortConfigConsistencyCheck() {
  console.log('\n[check:sort] 关键页面排序一致性检查')

  const checks = [
    {
      file: 'src/views/automation/automationtask/index.vue',
      requiredSorters: ['name', 'enabled', 'update_time'],
      requiredHandlerFieldLists: ['name', 'enabled', 'update_time'],
    },
    {
      file: 'src/views/automation/workflow/index.vue',
      requiredSorters: ['name', 'enabled', 'update_time'],
      requiredHandlerFieldLists: ['name', 'enabled', 'update_time'],
    },
    {
      file: 'src/views/automation/playbooks/index.vue',
      requiredSorters: ['name', 'update_time'],
      requiredHandlerFieldLists: ['name', 'update_time'],
    },
    {
      file: 'src/views/sys/scheduler/index.vue',
      requiredSorters: ['last_run_time', 'next_run_time'],
      requiredHandlerFieldLists: ['last_run_time', 'next_run_time'],
    },
    {
      file: 'src/views/automation/logs/index.vue',
      requiredSorters: ['job_id', 'id', 'status', 'start_time', 'duration_seconds'],
      requiredHandlerFieldLists: ['status', 'start_time', 'duration_seconds'],
      requiredHandlerListCount: 2,
      requiredSorterCountByField: {
        job_id: 1,
        id: 1,
        status: 2,
        start_time: 2,
        duration_seconds: 2,
      },
    },
  ]

  const errors = []

  for (const item of checks) {
    const absPath = path.join(projectRoot, item.file)
    if (!fs.existsSync(absPath)) {
      errors.push(`[missing-file] ${item.file}`)
      continue
    }

    const content = fs.readFileSync(absPath, 'utf8')

    for (const field of item.requiredSorters) {
      // Match one column block that declares `dataIndex: <field>` and includes `sorter: true`
      // nearby (bounded by ~220 chars to avoid matching across unrelated sections).
      const fieldPattern = new RegExp(
        `dataIndex\\s*:\\s*['\"]${escapeRegExp(field)}['\"][\\s\\S]{0,220}?sorter\\s*:\\s*true`,
        'm',
      )
      if (!fieldPattern.test(content)) {
        errors.push(`[missing-sorter] ${item.file} -> ${field}`)
      }
    }

    if (item.requiredSorterCountByField) {
      for (const [field, requiredCount] of Object.entries(item.requiredSorterCountByField)) {
        // Global variant of the same matcher above, used to count duplicate sortable fields.
        const fieldPattern = new RegExp(
          `dataIndex\\s*:\\s*['\"]${escapeRegExp(field)}['\"][\\s\\S]{0,220}?sorter\\s*:\\s*true`,
          'gm',
        )
        const matchedCount = Array.from(content.matchAll(fieldPattern)).length
        if (matchedCount < Number(requiredCount)) {
          errors.push(
            `[insufficient-sorter-count] ${item.file} -> ${field}, required: ${requiredCount}, actual: ${matchedCount}`,
          )
        }
      }
    }

    // Extract handler white-list arrays like:
    // const allowedFields = ['name', 'update_time']
    // const sortableFields = ['status']
    const handlerRegex = /const\s+(?:allowedFields|sortableFields)\s*=\s*\[([^\]]*)\]/g
    const handlerLists = []
    let match = null
    while ((match = handlerRegex.exec(content)) !== null) {
      const rawList = match[1]
      // Parse all quoted field names inside one [...] list.
      const values = Array.from(rawList.matchAll(/['\"]([^'\"]+)['\"]/g)).map((m) => m[1])
      if (values.length > 0) {
        handlerLists.push(values)
      }
    }

    if (item.requiredHandlerListCount && handlerLists.length < item.requiredHandlerListCount) {
      errors.push(
        `[missing-handler-list] ${item.file} -> expected at least ${item.requiredHandlerListCount}, got ${handlerLists.length}`,
      )
      continue
    }

    const required = item.requiredHandlerFieldLists
    const matchedHandlerCount = handlerLists.filter((list) => required.every((field) => list.includes(field))).length
    const expectedCount = item.requiredHandlerListCount || 1
    if (matchedHandlerCount < expectedCount) {
      errors.push(
        `[handler-fields-incomplete] ${item.file} -> required fields: ${required.join(', ')}, matched handlers: ${matchedHandlerCount}`,
      )
    }

    // Ensure list requests still go through resolve*Ordering() helper and pass ordering to API params.
    const hasResolveOrderingCall = /resolve[A-Za-z]+Ordering\(\)/.test(content)
    const hasOrderingAssignment = /ordering\s*:|\.ordering\s*=/.test(content)
    if (!hasResolveOrderingCall || !hasOrderingAssignment) {
      errors.push(`[missing-ordering-call] ${item.file} -> expected resolve*Ordering() + ordering assignment`)
    }
  }

  if (errors.length > 0) {
    console.error('Sort config consistency check failed:')
    for (const error of errors) {
      console.error(`- ${error}`)
    }
    process.exit(1)
  }

  console.log('Sort config consistency check passed.')
}

function walkVueFiles(dir, out = []) {
  if (!fs.existsSync(dir)) {
    return out
  }
  const items = fs.readdirSync(dir, { withFileTypes: true })
  for (const item of items) {
    const absPath = path.join(dir, item.name)
    if (item.isDirectory()) {
      walkVueFiles(absPath, out)
      continue
    }
    if (item.isFile() && absPath.endsWith('.vue')) {
      out.push(absPath)
    }
  }
  return out
}

function lineNumberAt(content, index) {
  return content.slice(0, index).split('\n').length
}

function runSortGapScan() {
  console.log('\n[check:sort] 全项目排序缺口扫描')

  const candidateFields = new Set([
    'id',
    'job_id',
    'run_id',
    'name',
    'enabled',
    'status',
    'create_time',
    'update_time',
    'start_time',
    'end_time',
    'run_time',
    'last_run_time',
    'next_run_time',
    'duration_seconds',
  ])

  function collectMissingSorters(absFile) {
    const content = fs.readFileSync(absFile, 'utf8')
    const results = []
    // Match a single plain object literal containing dataIndex (column definition candidate).
    const objectRegex = /\{[^{}]*?dataIndex\s*:\s*['\"]([A-Za-z0-9_]+)['\"][^{}]*?\}/gm
    let match = null
    while ((match = objectRegex.exec(content)) !== null) {
      const field = String(match[1] || '').trim()
      if (!candidateFields.has(field)) {
        continue
      }
      const block = match[0]
      // Detect whether this candidate object explicitly enables sorting.
      const hasSorter = /sorter\s*:\s*true/.test(block)
      if (hasSorter) {
        continue
      }
      // Try extracting display title for easier gap report reading.
      const titleMatch = block.match(/title\s*:\s*['\"]([^'\"]+)['\"]/)
      results.push({
        file: path.relative(projectRoot, absFile).replace(/\\/g, '/'),
        line: lineNumberAt(content, match.index),
        field,
        title: titleMatch?.[1] || '-',
      })
    }
    return results
  }

  const allFiles = walkVueFiles(viewsRoot)
  const missing = allFiles.flatMap(collectMissingSorters)
  missing.sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line)

  console.log(`Sort gap scan finished. files=${allFiles.length}, gaps=${missing.length}`)
  if (missing.length === 0) {
    return
  }

  for (const item of missing) {
    console.log(`${item.file}:${item.line} | ${item.field} | ${item.title}`)
  }
}

runSortConfigConsistencyCheck()
runSortGapScan()

console.log('\n[check:sort] Completed.')
