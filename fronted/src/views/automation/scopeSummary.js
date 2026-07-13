export function flattenGroupPathMap(nodes, collector = {}, parentPath = '') {
  ;(nodes || []).forEach((node) => {
    const currentName = String(node?.name || '').trim() || `分组#${node?.id || ''}`
    const currentPath = parentPath ? `${parentPath} / ${currentName}` : currentName
    collector[node.id] = currentPath
    flattenGroupPathMap(node.children || [], collector, currentPath)
  })
  return collector
}

export function buildScopeSummaryText({
  selectedGroupIds,
  selectedHostIds,
  groupPathMap,
  groupNameMap,
  maxPreviewCount = 3,
  emptyAsAllHosts = true,
  emptyLabel = '未选择范围（0台主机）',
}) {
  const groupIds = Array.isArray(selectedGroupIds) ? selectedGroupIds : []
  const hostIds = Array.isArray(selectedHostIds) ? selectedHostIds : []

  if (groupIds.length === 0 && hostIds.length === 0) {
    return emptyAsAllHosts ? '全部主机（按分组范围）' : emptyLabel
  }
  if (groupIds.length === 0) {
    return `已选0组（${hostIds.length}台主机）`
  }

  const pathMap = groupPathMap || {}
  const nameMap = groupNameMap || {}
  const labels = groupIds.map((id) => pathMap[id] || nameMap[id] || `分组#${id}`)

  if (labels.length > maxPreviewCount) {
    return `已选${labels.length}组：${labels.slice(0, maxPreviewCount).join('；')} 等`
  }
  return `已选${labels.length}组：${labels.join('；')}`
}
