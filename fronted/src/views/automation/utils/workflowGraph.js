export const START_EDGE_TYPE = 'straight'
export const WORKFLOW_EDGE_TYPE = 'straight'

export function normalizeWorkflowEdgeKind(condition) {
  const text = String(condition || 'success').trim().toLowerCase()
  return text === 'always' ? 'always' : 'success'
}

export function resolveWorkflowEdgeColor(condition) {
  const kind = normalizeWorkflowEdgeKind(condition)
  if (kind === 'always') {
    return '#1677ff'
  }
  return '#52c41a'
}

export function buildWorkflowEdgeStyle(condition = 'success') {
  return { stroke: resolveWorkflowEdgeColor(condition) }
}

export function buildWorkflowEdgeLabelStyle() {
  return { fill: '#333', fontSize: '12px' }
}

export function resolveWorkflowEdgeType(condition = 'success') {
  return normalizeWorkflowEdgeKind(condition) === 'always' ? START_EDGE_TYPE : WORKFLOW_EDGE_TYPE
}

export function resolveWorkflowEdgePathOptions(condition = 'success') {
  return undefined
}
