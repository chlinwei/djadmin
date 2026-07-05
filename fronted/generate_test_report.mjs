import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

const projectRoot = process.cwd()
const reportJsonPath = path.join(projectRoot, '.vitest-report.json')
const coverageSummaryPath = path.join(projectRoot, 'coverage', 'coverage-summary.json')
const outputPath = path.resolve(projectRoot, '..', 'FRONTEND_TEST_REPORT.md')

function runVitestForReport() {
  const vitestBin = path.join(projectRoot, 'node_modules', '.bin', 'vitest')
  const args = ['run', '--coverage', '--reporter=json', `--outputFile=${reportJsonPath}`]
  const result = spawnSync(vitestBin, args, { stdio: 'inherit', cwd: projectRoot })
  if (result.status !== 0) {
    process.exit(result.status ?? 1)
  }
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'))
}

function formatDuration(ms) {
  return `${Number(ms || 0).toFixed(0)}ms`
}

function buildMarkdown(testReport, coverageSummary) {
  const total = coverageSummary?.total || {}
  const suites = Array.isArray(testReport?.testResults) ? testReport.testResults : []

  const lines = []
  lines.push('# Frontend Test Report')
  lines.push('')
  lines.push(`- Generated At: ${new Date().toISOString()}`)
  lines.push(`- Success: ${testReport.success ? 'Yes' : 'No'}`)
  lines.push(`- Test Suites: ${testReport.numPassedTestSuites}/${testReport.numTotalTestSuites}`)
  lines.push(`- Test Cases: ${testReport.numPassedTests}/${testReport.numTotalTests}`)
  lines.push('')
  lines.push('## Coverage Summary')
  lines.push('')
  lines.push('| Metric | Covered / Total | Pct |')
  lines.push('| --- | --- | --- |')
  lines.push(`| Lines | ${total.lines?.covered ?? 0} / ${total.lines?.total ?? 0} | ${(total.lines?.pct ?? 0).toFixed(2)}% |`)
  lines.push(`| Statements | ${total.statements?.covered ?? 0} / ${total.statements?.total ?? 0} | ${(total.statements?.pct ?? 0).toFixed(2)}% |`)
  lines.push(`| Functions | ${total.functions?.covered ?? 0} / ${total.functions?.total ?? 0} | ${(total.functions?.pct ?? 0).toFixed(2)}% |`)
  lines.push(`| Branches | ${total.branches?.covered ?? 0} / ${total.branches?.total ?? 0} | ${(total.branches?.pct ?? 0).toFixed(2)}% |`)
  lines.push('')
  lines.push('## Per File Test Summary')
  lines.push('')
  lines.push('| File | Status | Passed | Failed | Duration |')
  lines.push('| --- | --- | --- | --- | --- |')

  for (const suite of suites) {
    const assertions = Array.isArray(suite.assertionResults) ? suite.assertionResults : []
    const passed = assertions.filter((item) => item.status === 'passed').length
    const failed = assertions.filter((item) => item.status === 'failed').length
    const duration = assertions.reduce((sum, item) => sum + Number(item.duration || 0), 0)
    const filePath = String(suite.name || '').replace(`${projectRoot}/`, '')
    lines.push(`| ${filePath} | ${suite.status || '-'} | ${passed} | ${failed} | ${formatDuration(duration)} |`)
  }

  return `${lines.join('\n')}\n`
}

function main() {
  runVitestForReport()

  if (!fs.existsSync(reportJsonPath)) {
    throw new Error(`Vitest JSON report not found: ${reportJsonPath}`)
  }
  if (!fs.existsSync(coverageSummaryPath)) {
    throw new Error(`Coverage summary not found: ${coverageSummaryPath}`)
  }

  const testReport = readJson(reportJsonPath)
  const coverageSummary = readJson(coverageSummaryPath)
  const markdown = buildMarkdown(testReport, coverageSummary)

  fs.writeFileSync(outputPath, markdown, 'utf8')
  console.log(`Frontend test report generated: ${outputPath}`)
}

main()
