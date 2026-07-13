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

export function buildAutomationPlaybookRoute(record) {
  const keyword = String(record?.template_name || '').trim()

  return {
    path: '/sys/automation/playbooks',
    query: keyword ? { search: keyword } : {},
  }
}
