export function parseShellEnvText(text) {
  const input = String(text || '').trim()
  if (!input) return {}
  const result = {}
  for (const item of input.split(/[;\n]+/).map((s) => s.trim()).filter(Boolean)) {
    const i = item.indexOf('=')
    if (i <= 0) throw new Error(`Shell 环境变量格式错误: ${item}`)
    const key = item.slice(0, i).trim()
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) throw new Error(`Shell 环境变量名不合法: ${key}`)
    result[key] = item.slice(i + 1).trim()
  }
  return result
}

export function formatShellEnvText(envVars) {
  if (!envVars || typeof envVars !== 'object' || Array.isArray(envVars)) return ''
  return Object.entries(envVars).map(([k, v]) => `${k}=${String(v ?? '')}`).join(';')
}

export function resolveTaskSubmitErrorMessage(error) {
  const response = error?.response?.data
  const fieldErrors = response?.data
  if (fieldErrors && typeof fieldErrors === 'object' && !Array.isArray(fieldErrors)) {
    const labels = { name: '任务名称', inventory: 'Inventory', shell_parameters: 'Shell 参数字符串', env_vars: '环境变量' }
    const messages = []
    for (const [field, value] of Object.entries(fieldErrors)) {
      const text = (Array.isArray(value) ? value.join('；') : String(value || '')).trim()
      if (!text) continue
      if (field === 'name' && /already exists/i.test(text)) {
        messages.push('任务名称已存在')
        continue
      }
      messages.push(field === 'non_field_errors' ? text : `${labels[field] || field}: ${text}`)
    }
    if (messages.length) return messages.join('；')
  }
  return response?.msg || error?.message || '任务保存失败'
}

export function buildRunNowPayload(limit, isShellTask, shellArgs) {
  const runtimeLimit = String(limit || '').trim()
  return isShellTask
    ? { limit: runtimeLimit, shell_parameters: String(shellArgs || '').trim() }
    : { limit: runtimeLimit }
}
