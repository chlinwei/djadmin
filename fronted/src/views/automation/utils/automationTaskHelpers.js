export const ASSET_HOST_ROUTE_CANDIDATES = ['/assets/hosts', '/assets/host', '/assets/hosts/index', '/assets/host/index']

export const ALL_GROUP_VALUE = '__all__'
export const ALL_GROUP_TITLE = 'All（全部主机）'
export const GROUP_TAG_PREVIEW_COUNT = 2

export const TASK_COLUMNS = [
  { title: '任务名称', dataIndex: 'name', key: 'name', width: 160, sorter: true },
  { title: '任务编码', dataIndex: 'code', key: 'code', width: 170 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 90, sorter: true },
  { title: 'Inventory', dataIndex: 'inventory_name', key: 'inventory_name', width: 180 },
  { title: '模板', dataIndex: 'template_name', key: 'template_name', width: 140 },
  { title: '执行范围', dataIndex: 'selected_group_ids', key: 'selected_group_ids', width: 260 },
  { title: '环境变量', dataIndex: 'env_vars', key: 'env_vars', width: 220 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 120, sorter: true },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 160 },
  { title: '操作', key: 'action', width: 320, fixed: 'right' },
]

export function resolveTaskListOrdering(taskSort) {
  if (!taskSort?.field || !taskSort?.order) {
    return '-id'
  }
  const sortPrefix = taskSort.order === 'descend' ? '-' : ''
  return `${sortPrefix}${taskSort.field}`
}

export function parseJsonObjectText(text, fieldLabel) {
  if (!text || !String(text).trim()) {
    return {}
  }
  try {
    const parsed = JSON.parse(text)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed
    }
    throw new Error(`${fieldLabel} 必须是 JSON 对象`)
  } catch (error) {
    throw new Error(error?.message || `${fieldLabel} 解析失败`)
  }
}

export function flattenGroupNameMap(nodes, collector = {}) {
  ;(nodes || []).forEach((node) => {
    collector[node.id] = node.name
    flattenGroupNameMap(node.children || [], collector)
  })
  return collector
}

export function formatJsonCellText(value) {
  if (!value || typeof value !== 'object') {
    return '-'
  }
  const text = JSON.stringify(value)
  return text.length > 64 ? `${text.slice(0, 64)}...` : text
}

export function getScopeTreeNode(node) {
  if (!node || typeof node !== 'object') {
    return {}
  }
  return node.dataRef || node
}