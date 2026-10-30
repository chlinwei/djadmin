export const strengthOptions = [
  { value: 'strong', label: '强依赖' },
  { value: 'weak', label: '弱依赖' },
  { value: 'unknown', label: '待确认' },
]
export const strengthLabels = Object.fromEntries(strengthOptions.map((item) => [item.value, item.label]))
export const strengthColors = { strong: '#4d79bd', weak: '#bc8b32', unknown: '#8996a9' }
export const nodeWidth = 250
export const nodeHeight = 114
export const edgeLabelWidth = 125
export const edgeLabelHeight = 24

export function dependencyData(response) {
  const envelope = response?.data
  if (envelope?.code !== 200 || !envelope.data || typeof envelope.data !== 'object') {
    throw new Error(envelope?.msg || '服务依赖接口返回格式异常')
  }
  return envelope.data
}

export function dependencyPage(response) {
  const data = dependencyData(response)
  if (!Array.isArray(data.results) || !Number.isInteger(data.totalPages) || data.totalPages < 0) {
    throw new Error('分页接口返回格式异常')
  }
  return response
}

export function resolveDependencyScope(scope, businesses) {
  const business = businesses.find((item) => Number(item.id) === Number(scope.businessSystemId))
  const project = scope.projectId ?? business?.project ?? (scope.projectIds?.length === 1 ? scope.projectIds[0] : null)
  const environment = scope.environment ?? (scope.environmentIds?.length === 1 ? scope.environmentIds[0] : null)
  return {
    project: Number(project) > 0 ? Number(project) : undefined,
    environment: Number(environment) > 0 ? Number(environment) : undefined,
    business_system: business ? Number(business.id) : undefined,
    service: ['service', 'deployment'].includes(scope.nodeType) ? scope.applicationServiceId : undefined,
  }
}

export function runtimePresentation(summary = {}) {
  if (!summary.total) return { color: '#8996a9', label: '暂无启用实例' }
  if (summary.error) return { color: '#b67b15', label: '存在检查异常' }
  if (summary.unknown) return { color: '#8996a9', label: '存在未知状态' }
  if (summary.stopped === summary.total) return { color: '#d95858', label: '全部实例停止' }
  if (summary.stopped) return { color: '#b67b15', label: '部分实例停止' }
  return { color: '#159579', label: '实例运行中' }
}

export function selectDependencyTopology(topology, scope, filters = {}) {
  const inScope = (node) => scope.service
    ? node.id === Number(scope.service)
    : !scope.business_system || node.business_system_id === Number(scope.business_system)
  const selectedIds = new Set(topology.nodes.filter(inScope).map((node) => node.id))
  const search = (filters.search || '').trim().toLowerCase()
  const edges = topology.edges.filter((edge) =>
    (selectedIds.has(edge.source_service_id) || selectedIds.has(edge.target_service_id)) &&
    (!filters.strength || edge.strength === filters.strength) &&
    (!filters.type || edge.interaction_type_id === filters.type) &&
    (!search || [edge.source_name, edge.target_name, edge.function_scope, edge.protocol].some((value) => String(value || '').toLowerCase().includes(search))),
  )
  const ids = new Set(edges.flatMap((edge) => [edge.source_service_id, edge.target_service_id]))
  // Keep isolated services in the unfiltered architecture; focused neighbors are boundary nodes, not a second ownership tree.
  if (!search && !filters.strength && !filters.type) for (const id of selectedIds) ids.add(id)
  return {
    edges,
    nodes: topology.nodes.filter((node) => ids.has(node.id)).map((node) => ({ ...node, external_to_selection: !selectedIds.has(node.id) })),
  }
}

export async function layoutDependencyNodes(nodes, edges) {
  if (!nodes.length) return []
  const { default: ELK } = await import('elkjs/lib/elk.bundled.js')
  const graph = await new ELK().layout({
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'RIGHT',
      'elk.spacing.nodeNode': '65',
      'elk.layered.spacing.nodeNodeBetweenLayers': '180',
      'elk.padding': '[top=35,left=35,bottom=35,right=35]',
    },
    children: [...nodes].sort((a, b) => a.id - b.id).map((node) => ({ id: String(node.id), width: nodeWidth, height: nodeHeight })),
    edges: [...edges].sort((a, b) => a.id - b.id).map((edge) => ({
      id: String(edge.id), sources: [String(edge.source_service_id)], targets: [String(edge.target_service_id)],
    })),
  })
  return graph.children.map((node) => ({ id: node.id, position: { x: node.x, y: node.y } }))
}

export function graphEdges(edges) {
  const pairs = new Map()
  for (const edge of edges) {
    const key = [edge.source_service_id, edge.target_service_id].sort((a, b) => a - b).join(':')
    if (!pairs.has(key)) pairs.set(key, [])
    pairs.get(key).push(edge)
  }
  for (const group of pairs.values()) group.sort((a, b) => a.id - b.id)
  return edges.map((edge) => {
    const group = pairs.get([edge.source_service_id, edge.target_service_id].sort((a, b) => a - b).join(':'))
    const offset = (group.indexOf(edge) - (group.length - 1) / 2) * 32
    return {
      id: String(edge.id), source: String(edge.source_service_id), target: String(edge.target_service_id),
      type: 'dependency', data: { record: edge, offset, lane: group.indexOf(edge) },
      markerEnd: { type: 'arrowclosed', color: strengthColors[edge.strength] },
      style: { stroke: strengthColors[edge.strength], strokeWidth: 2, strokeDasharray: edge.strength === 'weak' ? '7 5' : edge.strength === 'unknown' ? '2 5' : undefined },
    }
  })
}

const xml = (value) => String(value).replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' }[char]))
const shortLabel = (value, limit) => {
  const chars = Array.from(String(value))
  return chars.length > limit ? `${chars.slice(0, limit - 1).join('')}…` : String(value)
}

export function dependencyEdgeGeometry({ sourceX: sx, sourceY: sy, targetX: tx, targetY: ty, offset = 0, lane = 0, minY = Math.min(sy, ty) - nodeHeight / 2 }) {
  if (tx <= sx) {
    // Return edges must go around the node rows: a horizontal Bezier would run straight through both cards.
    const top = minY - 65 - lane * 26, exit = 40 + lane * 12
    return {
      path: `M${sx},${sy} C${sx + exit},${sy} ${sx + exit},${top} ${sx + exit},${top} L${tx - exit},${top} C${tx - exit},${top} ${tx - exit},${ty} ${tx},${ty}`,
      x: (sx + tx) / 2, y: top,
      xs: [sx, sx + exit, tx - exit, tx], ys: [sy, ty, top],
    }
  }
  const bend = Math.max(70, Math.abs(tx - sx) / 2)
  return {
    path: `M${sx},${sy} C${sx + bend},${sy + offset} ${tx - bend},${ty + offset} ${tx},${ty}`,
    x: (sx + tx) / 2, y: (sy + ty) / 2 + offset * .75,
    xs: [sx, sx + bend, tx - bend, tx], ys: [sy, sy + offset, ty + offset, ty],
  }
}

export function positionedDependencyEdges(nodes, edges, labelOffsets = {}) {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const minY = Math.min(...nodes.map((node) => node.position.y))
  const positioned = graphEdges(edges).map((edge) => {
    const a = byId.get(edge.source), b = byId.get(edge.target)
    if (!a || !b) throw new Error('导出失败：拓扑包含无法定位端点的关系')
    return { ...edge, geometry: dependencyEdgeGeometry({
      sourceX: a.position.x + nodeWidth, sourceY: a.position.y + nodeHeight / 2,
      targetX: b.position.x, targetY: b.position.y + nodeHeight / 2,
      ...edge.data, minY,
    }) }
  })
  const obstacles = nodes.map((node) => ({ ...node.position, width: nodeWidth, height: nodeHeight }))
  // Manual labels are routing anchors, relative to the original midpoint so node moves still carry the line.
  for (const edge of positioned) {
    const offset = labelOffsets[edge.id]
    if (!offset) continue
    const a = byId.get(edge.source).position, b = byId.get(edge.target).position
    const sx = a.x + nodeWidth, sy = a.y + nodeHeight / 2, tx = b.x, ty = b.y + nodeHeight / 2
    const x = edge.geometry.x + offset.x, y = edge.geometry.y + offset.y
    const bend = Math.max(40, Math.min(120, Math.abs(tx - sx) / 2))
    edge.geometry = {
      path: `M${sx},${sy} C${sx + bend},${sy} ${x - bend},${y} ${x},${y} C${x + bend},${y} ${tx - bend},${ty} ${tx},${ty}`,
      x, y, xs: [sx, sx + bend, x - bend, x, x + bend, tx - bend, tx], ys: [sy, y, ty],
    }
    edge.label = { x: x - edgeLabelWidth / 2, y: y - edgeLabelHeight / 2, width: edgeLabelWidth, height: edgeLabelHeight }
    edge.labelGuide = ''
    obstacles.push(edge.label)
  }
  // Reserve fixed-size label boxes across all edges, not just edges sharing the same endpoints.
  for (const edge of [...positioned].sort((a, b) => Number(a.id) - Number(b.id))) {
    if (edge.label) continue
    const { x, y } = edge.geometry
    const halfWidth = edgeLabelWidth / 2, halfHeight = edgeLabelHeight / 2, gap = 6
    const intervals = obstacles
      .filter((box) => x + halfWidth + gap > box.x && x - halfWidth - gap < box.x + box.width)
      .map((box) => [box.y - halfHeight - gap, box.y + box.height + halfHeight + gap])
      .sort((a, b) => a[0] - b[0])
    const merged = []
    for (const interval of intervals) {
      const last = merged[merged.length - 1]
      if (last && interval[0] <= last[1]) last[1] = Math.max(last[1], interval[1])
      else merged.push([...interval])
    }
    const blocked = merged.find(([top, bottom]) => y > top && y < bottom)
    const labelY = blocked ? (y - blocked[0] <= blocked[1] - y ? blocked[0] : blocked[1]) : y
    edge.label = { x: x - halfWidth, y: labelY - halfHeight, width: edgeLabelWidth, height: edgeLabelHeight }
    edge.labelGuide = labelY === y ? '' : `M${x},${y} L${x},${labelY > y ? labelY - halfHeight : labelY + halfHeight}`
    obstacles.push(edge.label)
  }
  return positioned
}

export function dependencyGraphBounds(nodes, edges, labelOffsets = {}) {
  const positioned = positionedDependencyEdges(nodes, edges, labelOffsets)
  const xs = [...nodes.flatMap((node) => [node.position.x, node.position.x + nodeWidth]), ...positioned.flatMap((edge) => [...edge.geometry.xs, edge.label.x, edge.label.x + edge.label.width])]
  const ys = [...nodes.flatMap((node) => [node.position.y, node.position.y + nodeHeight]), ...positioned.flatMap((edge) => [...edge.geometry.ys, edge.label.y, edge.label.y + edge.label.height])]
  const x = Math.min(0, ...xs), y = Math.min(0, ...ys)
  return { x, y, width: Math.max(0, ...xs) - x, height: Math.max(0, ...ys) - y }
}

export function dependencySvg(nodes, edges, title, labelOffsets = {}) {
  const bounds = dependencyGraphBounds(nodes, edges, labelOffsets)
  const positioned = positionedDependencyEdges(nodes, edges, labelOffsets)
  const minX = bounds.x - 24, minY = bounds.y - 70
  const width = Math.max(800, bounds.width + 48), height = Math.max(350, bounds.height + 94)
  const paths = positioned.map((edge) => {
    const { path } = edge.geometry
    return `<path d="${path}" fill="none" stroke="${edge.style.stroke}" stroke-width="2" ${edge.style.strokeDasharray ? `stroke-dasharray="${edge.style.strokeDasharray}"` : ''} marker-end="url(#${edge.data.record.strength})"/>`
  }).join('')
  const labels = positioned.map((edge) => {
    const { x, y, width, height } = edge.label
    const text = `${shortLabel(edge.data.record.function_scope, 6)} · ${strengthLabels[edge.data.record.strength]}`
    return `<g class="edge-label"><title>${xml(edge.data.record.function_scope)} · ${strengthLabels[edge.data.record.strength]}</title>${edge.labelGuide ? `<path d="${edge.labelGuide}" fill="none" stroke="#b9c6d8" stroke-dasharray="3 3"/>` : ''}<rect x="${x}" y="${y}" width="${width}" height="${height}" rx="4" fill="#f8faff" stroke="#e2e8f1"/><text x="${x + width / 2}" y="${y + height / 2}" dominant-baseline="central" text-anchor="middle" fill="${edge.style.stroke}" font-size="11">${xml(text)}</text></g>`
  }).join('')
  const boxes = nodes.map(({ position: { x, y }, data: { record } }) => {
    const state = runtimePresentation(record.runtime_summary), summary = record.runtime_summary
    return `<g><title>${xml(record.name)} · ${xml(record.business_system_name)} / ${xml(record.code)}</title><rect x="${x}" y="${y}" width="${nodeWidth}" height="${nodeHeight}" rx="8" fill="white" stroke="${state.color}"/><text x="${x + 14}" y="${y + 27}" font-size="15" fill="#243b5a">${xml(shortLabel(record.name, 14))}</text><text x="${x + 14}" y="${y + 48}" font-size="11" fill="#79879c">${xml(shortLabel(`${record.business_system_name} · ${record.code}`, 20))}</text><text x="${x + 14}" y="${y + 72}" font-size="12" fill="${state.color}">${state.label}</text><text x="${x + 14}" y="${y + 94}" font-size="11" fill="#79879c">运行 ${summary.running} / 停止 ${summary.stopped} / 异常 ${summary.error} / 未知 ${summary.unknown}</text></g>`
  }).join('')
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${minX} ${minY} ${width} ${height}" width="${width}" height="${height}" font-family="sans-serif"><defs>${Object.entries(strengthColors).map(([key, color]) => `<marker id="${key}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0L10 5L0 10Z" fill="${color}"/></marker>`).join('')}</defs><rect x="${minX}" y="${minY}" width="${width}" height="${height}" fill="#f7faff"/><text x="${minX + 24}" y="${minY + 28}" fill="#29405f" font-size="16">${xml(title)}</text>${paths}${boxes}${labels}</svg>`
}
