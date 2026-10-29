const { readFileSync } = require('node:fs')

const browserTools = [
  'openBrowserPage', 'readPage', 'screenshotPage', 'navigatePage',
  'clickElement', 'hoverElement', 'dragElement', 'typeInPage',
  'handleDialog', 'runPlaywrightCode',
].map((name) => name.toLowerCase())

function isBrowserTool(name) {
  if (typeof name !== 'string') return false
  // VS Code and MCP can prefix tool names with a namespace.
  const canonical = name.toLowerCase().replace(/[^a-z0-9]/g, '')
  return browserTools.some((tool) => canonical.endsWith(tool))
    || /(?:^|[_.:/-])(?:browser|playwright)(?:[_.:/-]|$)/i.test(name)
}

function containsBrowserCall(calls) {
  return calls.some((call) => isBrowserTool(call?.recipient_name)
    || (Array.isArray(call?.parameters?.tool_uses) && containsBrowserCall(call.parameters.tool_uses)))
}

function decision(event) {
  if (!event || typeof event.toolName !== 'string' || !event.toolName.trim()) {
    return { permissionDecision: 'deny', permissionDecisionReason: 'Cannot verify the tool request for browser approval.' }
  }
  if (isBrowserTool(event.toolName)) {
    return {
      permissionDecision: 'ask',
      permissionDecisionReason: 'Browser access requires user confirmation for this individual call. Explain the target and action; do not bypass approval.',
    }
  }
  if (Array.isArray(event.toolArgs?.tool_uses) && containsBrowserCall(event.toolArgs.tool_uses)) {
    return {
      permissionDecision: 'deny',
      permissionDecisionReason: 'Browser calls cannot be batched. Request individual approval for each browser tool.',
    }
  }
  // This harmless probe verifies host loading without ever invoking a browser.
  if (event.toolName === 'bash' && event.toolArgs?.command === 'printf DJADMIN_BROWSER_APPROVAL_PROBE') {
    return {
      permissionDecision: 'deny',
      permissionDecisionReason: 'Browser approval hook probe intercepted successfully. No browser or website was accessed.',
    }
  }
  return {}
}

let result
try {
  result = decision(JSON.parse(readFileSync(0, 'utf8')))
} catch {
  result = { permissionDecision: 'deny', permissionDecisionReason: 'Invalid browser approval hook input; tool execution must stop.' }
}
process.stdout.write(JSON.stringify(result))
