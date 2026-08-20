export function resolveTaskSubmitErrorMessage(error) {
  const response = error?.response?.data
  const fieldErrors = response?.data
  if (fieldErrors && typeof fieldErrors === 'object' && !Array.isArray(fieldErrors)) {
    const labels = { name: '任务名称', inventory: 'Inventory', env_vars: 'Playbook 变量' }
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

export function buildRunNowPayload(limit) {
  const runtimeLimit = String(limit || '').trim()
  return { limit: runtimeLimit }
}
