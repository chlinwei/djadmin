export function buildAutomationInventoryRoute(record) {
  const inventoryId = Number(record?.inventory)
  const keyword = String(record?.inventory_name || '').trim()
  const query = {}

  if (keyword) {
    query.search = keyword
    query.inventory_name = keyword
  }

  if (Number.isInteger(inventoryId) && inventoryId > 0) {
    query.inventory_id = String(inventoryId)
  }

  return {
    path: '/sys/automation/inventory',
    query,
  }
}

export function buildAutomationTemplateRoute(record) {
  const rawTemplateName = String(record?.template_name || '').trim()
  const keyword = rawTemplateName.replace(/^\[(Playbook|ShellScript)\]\s*/, '')
  const templateType = Number(record?.shell_script_template) > 0 ? 'shell_script' : 'playbook'

  return {
    path: '/sys/automation/templates',
    query: keyword ? { search: keyword, type: templateType } : { type: templateType },
  }
}
