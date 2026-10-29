const assert = require('node:assert/strict')
const { execFileSync } = require('node:child_process')
const { readFileSync } = require('node:fs')
const path = require('node:path')
const { test } = require('node:test')

const script = path.join(__dirname, 'browser-approval.cjs')

function invoke(input) {
  return JSON.parse(execFileSync(process.execPath, [script], {
    input, encoding: 'utf8', timeout: 2000,
  }))
}

function check(toolName, toolArgs = {}) {
  return invoke(JSON.stringify({ toolName, toolArgs }))
}

for (const name of [
  'openBrowserPage', 'readPage', 'screenshotPage', 'navigatePage',
  'clickElement', 'hoverElement', 'dragElement', 'typeInPage',
  'handleDialog', 'runPlaywrightCode', 'functions.openBrowserPage',
  'mcp_vscode_runPlaywrightCode', 'mcp__playwright__browser_navigate',
  'browser_snapshot', 'playwright.browser_evaluate',
]) {
  test(`requests individual confirmation for ${name}`, () => {
    assert.equal(check(name).permissionDecision, 'ask')
    assert.equal(check(name).permissionDecision, 'ask')
  })
}

for (const name of ['view', 'rg', 'glob', 'apply_patch', 'bash', 'web_fetch', 'tool_search']) {
  test(`preserves normal permission checks for ${name}`, () => {
    assert.deepEqual(check(name, { description: 'Read the browser approval policy' }), {})
  })
}

test('rejects browser calls hidden in a mixed parallel batch', () => {
  assert.equal(check('multi_tool_use.parallel', {
    tool_uses: [
      { recipient_name: 'functions.view', parameters: { path: '/example' } },
      { recipient_name: 'functions.runPlaywrightCode', parameters: {} },
    ],
  }).permissionDecision, 'deny')
})

test('rejects nested browser batches', () => {
  assert.equal(check('multi_tool_use.parallel', {
    tool_uses: [{
      recipient_name: 'multi_tool_use.parallel',
      parameters: { tool_uses: [{ recipient_name: 'functions.readPage', parameters: {} }] },
    }],
  }).permissionDecision, 'deny')
})

test('leaves non-browser parallel batches to normal permission checks', () => {
  assert.deepEqual(check('multi_tool_use.parallel', {
    tool_uses: [{ recipient_name: 'functions.view', parameters: {} }],
  }), {})
})

test('does not echo URLs, credentials or other tool arguments', () => {
  const result = JSON.stringify(check('runPlaywrightCode', { secret: 'private-test-value' }))
  assert.ok(!result.includes('private-test-value'))
})

for (const input of ['', '{', '{}', 'null', '{"toolName":42}']) {
  test(`denies invalid event input ${JSON.stringify(input)}`, () => {
    assert.equal(invoke(input).permissionDecision, 'deny')
  })
}

test('intercepts a harmless host-loading probe without browser access', () => {
  const result = check('bash', { command: 'printf DJADMIN_BROWSER_APPROVAL_PROBE' })
  assert.equal(result.permissionDecision, 'deny')
  assert.match(result.permissionDecisionReason, /probe intercepted successfully/)
})

test('uses the Copilot runtime hook schema and a repository-local command', () => {
  const configuration = JSON.parse(readFileSync(path.join(__dirname, 'browser-approval.json'), 'utf8'))
  assert.equal(configuration.version, 1)
  assert.deepEqual(configuration.hooks, {
    preToolUse: [{
      type: 'command',
      command: 'node .github/hooks/browser-approval.cjs',
      cwd: '.',
      timeoutSec: 5,
    }],
  })
})
