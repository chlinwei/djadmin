export const WORKFLOW_ACTION_TOOLTIP_PROPS = {
  placement: 'top',
  autoAdjustOverflow: false,
  // Keep tooltip visually above buttons and avoid hover layer hijacking click focus.
  align: { offset: [0, -10] },
  overlayClassName: 'dj-action-tooltip',
}

export const WORKFLOW_COLUMNS = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 180, sorter: true },
  { title: '图规模', dataIndex: 'node_count', key: 'node_count', width: 160 },
  { title: 'Inventory', dataIndex: 'default_inventory_name', key: 'default_inventory_name', width: 190 },
  { title: '执行范围', dataIndex: 'execution_scope_summary', key: 'execution_scope_summary', width: 320 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100, sorter: true },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 140, sorter: true },
  { title: '操作', key: 'action', width: 280, fixed: 'right' },
]

export function resolveWorkflowListOrdering(listSort) {
  if (!listSort?.field || !listSort?.order) {
    return '-id'
  }
  const sortPrefix = listSort.order === 'descend' ? '-' : ''
  return `${sortPrefix}${listSort.field}`
}

export function toCloneName(sourceName) {
  const baseName = String(sourceName || '').trim() || 'Workflow'
  return `${baseName}-副本`
}

export function cloneDefaultExtraVars(source) {
  if (!source || typeof source !== 'object' || Array.isArray(source)) {
    return {}
  }
  return JSON.parse(JSON.stringify(source))
}

export function cloneGraphItems(items) {
  if (!Array.isArray(items)) {
    return []
  }
  return JSON.parse(JSON.stringify(items))
}

export function isWorkflowLaunchDisabled(record) {
  return !Boolean(record?.enabled)
}

export function buildLimitPayload(limitText = '') {
  return {
    limit: String(limitText || '').trim(),
  }
}

export function buildScopePreviewTitle(record) {
  return `执行范围主机预览 / ${record?.name || '-'} (${record?.id || '-'})`
}

export async function runWorkflowPrecheck(precheckFn, workflowId, payload = {}) {
  const normalizedPayload = buildLimitPayload(payload.limit)
  const basePayload = { ...normalizedPayload, limit: '' }
  const baseRes = await precheckFn(workflowId, basePayload)
  const baseData = baseRes?.data?.data || {}

  let data = baseData
  if (normalizedPayload.limit) {
    const narrowedRes = await precheckFn(workflowId, normalizedPayload)
    data = narrowedRes?.data?.data || {}
  }

  return {
    baseData,
    data,
    payload: normalizedPayload,
  }
}

export function buildWorkflowPrecheckMessage(data) {
  const scopeLabel = data.use_global_scope
    ? `Inventory: ${String(data.inventory_name || '-')}`
    : '未启用 Workflow 全局 Inventory'

  if (!data.ok) {
    return String(data.message || '预检失败，请检查范围配置')
  }

  const hostCount = Number(data.resolved_host_count || 0)
  if (data.use_global_scope) {
    return `${scopeLabel}，匹配主机 ${hostCount} 台`
  }
  return `${scopeLabel}，将按各任务节点执行范围运行`
}
