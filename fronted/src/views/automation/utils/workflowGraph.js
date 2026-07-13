export const START_EDGE_TYPE = 'straight'
export const WORKFLOW_EDGE_TYPE = 'smoothstep'
export const WORKFLOW_EDGE_PATH_OPTIONS = { borderRadius: 18, offset: 20 }

export function normalizeEdgeCondition(condition) {
  const text = String(condition || 'success').trim().toLowerCase()
  if (text === 'failure' || text === 'always') {
    return text
  }
  return 'success'
}

export function resolveEdgeColor(condition) {
  const normalized = normalizeEdgeCondition(condition)
  if (normalized === 'failure') {
    return '#ff4d4f'
  }
  if (normalized === 'always') {
    return '#1677ff'
  }
  return '#52c41a'
}

export function buildWorkflowEdgeStyle(condition = 'success') {
  return { stroke: resolveEdgeColor(condition) }
}

export function buildWorkflowEdgeLabelStyle() {
  return { fill: '#333', fontSize: '12px' }
}

export function resolveWorkflowEdgeType(condition = 'success') {
  return normalizeEdgeCondition(condition) === 'always' ? START_EDGE_TYPE : WORKFLOW_EDGE_TYPE
}

export function resolveWorkflowEdgePathOptions(condition = 'success') {
  return normalizeEdgeCondition(condition) === 'always' ? undefined : WORKFLOW_EDGE_PATH_OPTIONS
}

export function applyWorkflowEdgeVisual(edge, condition) {
  const normalized = normalizeEdgeCondition(condition)
  edge.type = resolveWorkflowEdgeType(normalized)
  const pathOptions = resolveWorkflowEdgePathOptions(normalized)
  if (pathOptions) {
    edge.pathOptions = pathOptions
  } else {
    delete edge.pathOptions
  }
  edge.data = { ...(edge.data || {}), condition: normalized }
  edge.label = normalized
  edge.style = buildWorkflowEdgeStyle(normalized)
}
