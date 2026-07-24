export function createStartNode(startNodeId) {
  return {
    id: startNodeId,
    type: 'workflow-start-node',
    position: { x: 60, y: 220 },
    draggable: false,
    connectable: false,
    selectable: false,
    data: { name: 'START', node_type: 'start' },
    style: {
      border: 'none',
      background: 'transparent',
      padding: '0',
      boxShadow: 'none',
    },
  }
}

export function formatNodeLabel(data) {
  const typeLabel = data.node_type === 'workflow' ? 'Workflow' : '任务'
  return `${data.name || '未命名节点'} (${typeLabel})`
}

export function makeNodeFromConfig(config, index = 0) {
  const START_X = 60
  const START_NODE_WIDTH = 112
  const RUNTIME_NODE_WIDTH = 220
  const LAYER_X_GAP = 280
  const EDGE_GAP = LAYER_X_GAP - RUNTIME_NODE_WIDTH
  const FIRST_RUNTIME_X = START_X + START_NODE_WIDTH + EDGE_GAP

  const x = Number(config.x)
  const y = Number(config.y)
  const position = {
    x: Number.isFinite(x) ? x : FIRST_RUNTIME_X + (index % 4) * LAYER_X_GAP,
    y: Number.isFinite(y) ? y : 220 + Math.floor(index / 4) * 140,
  }
  const data = {
    key: config.key,
    name: config.name,
    node_type: String(config.node_type || 'task').toLowerCase() === 'workflow' ? 'workflow' : 'task',
    task_id: config.task_id,
    workflow_id: config.workflow_id,
  }
  return {
    id: config.key,
    type: 'workflow-node',
    position,
    draggable: true,
    data,
    label: formatNodeLabel(data),
    style: {
      border: 'none',
      background: 'transparent',
      padding: '0',
      boxShadow: 'none',
    },
  }
}

export function isSystemEdge(edge, startNodeId, startEdgePrefix) {
  if (!edge || typeof edge !== 'object') {
    return false
  }
  return edge.source === startNodeId || String(edge.id || '').startsWith(startEdgePrefix)
}

export function ensureStartEdgesForGraph({
  flowNodes,
  flowEdges,
  startNodeId,
  startEdgePrefix,
  startEdgeType,
  markerEnd,
  buildWorkflowEdgeStyle,
  buildWorkflowEdgeLabelStyle,
}) {
  const runtimeNodes = flowNodes.filter((item) => item.id !== startNodeId)
  const runtimeNodeIds = new Set(runtimeNodes.map((item) => item.id))

  const incomingCount = {}
  runtimeNodes.forEach((item) => {
    incomingCount[item.id] = 0
  })

  flowEdges.forEach((item) => {
    if (!item || item.source === startNodeId) {
      return
    }
    if (runtimeNodeIds.has(item.target)) {
      incomingCount[item.target] = (incomingCount[item.target] || 0) + 1
    }
  })

  const rootKeys = runtimeNodes.filter((item) => (incomingCount[item.id] || 0) === 0).map((item) => item.id)

  const businessEdges = flowEdges.filter(
    (item) => item.source !== startNodeId && !String(item.id || '').startsWith(startEdgePrefix),
  )

  rootKeys.forEach((nodeKey) => {
    businessEdges.push({
      id: `${startEdgePrefix}${nodeKey}`,
      type: startEdgeType,
      source: startNodeId,
      target: nodeKey,
      data: { condition: 'always' },
      markerEnd,
      style: buildWorkflowEdgeStyle('always'),
      labelStyle: buildWorkflowEdgeLabelStyle(),
      selectable: false,
      updatable: false,
      focusable: false,
    })
  })

  return businessEdges
}

export function autoLayoutTreeNodes({ flowNodes, flowEdges, startNodeId, startEdgePrefix }) {
  const START_X = 60
  const START_NODE_WIDTH = 112
  const RUNTIME_NODE_WIDTH = 220
  const START_Y = 220
  const START_NODE_HEIGHT = 64
  const RUNTIME_NODE_HEIGHT = 72
  const RUNTIME_BASE_Y = START_Y + START_NODE_HEIGHT / 2 - RUNTIME_NODE_HEIGHT / 2
  const LAYER_X_GAP = 280
  const EDGE_GAP = LAYER_X_GAP - RUNTIME_NODE_WIDTH
  const FIRST_RUNTIME_X = START_X + START_NODE_WIDTH + EDGE_GAP
  const ROW_Y_GAP = 120

  const runtimeNodes = flowNodes.filter((item) => item.id !== startNodeId)
  if (runtimeNodes.length === 0) {
    return flowNodes
  }

  const nodeMap = {}
  const incomingCount = {}
  const parentsMap = {}
  const orderMap = {}

  runtimeNodes.forEach((node, index) => {
    nodeMap[node.id] = node
    incomingCount[node.id] = 0
    parentsMap[node.id] = []
    orderMap[node.id] = index
  })

  const businessEdges = flowEdges.filter(
    (edge) => !isSystemEdge(edge, startNodeId, startEdgePrefix),
  )
  businessEdges.forEach((edge) => {
    if (!nodeMap[edge.source] || !nodeMap[edge.target]) {
      return
    }
    incomingCount[edge.target] += 1
    parentsMap[edge.target].push(edge.source)
  })

  let roots = runtimeNodes.filter((node) => incomingCount[node.id] === 0).map((node) => node.id)
  if (roots.length === 0) {
    roots = runtimeNodes.map((node) => node.id)
  }

  const depthMap = {}
  const queue = []
  roots.forEach((key) => {
    depthMap[key] = 1
    queue.push(key)
  })

  while (queue.length > 0) {
    const current = queue.shift()
    const currentDepth = Number(depthMap[current] || 1)
    businessEdges.forEach((edge) => {
      if (edge.source !== current || !nodeMap[edge.target]) {
        return
      }
      const nextDepth = currentDepth + 1
      if (!depthMap[edge.target] || nextDepth < depthMap[edge.target]) {
        depthMap[edge.target] = nextDepth
        queue.push(edge.target)
      }
    })
  }

  runtimeNodes.forEach((node) => {
    if (!depthMap[node.id]) {
      depthMap[node.id] = 1
    }
  })

  const grouped = {}
  runtimeNodes.forEach((node) => {
    const depth = depthMap[node.id]
    if (!grouped[depth]) {
      grouped[depth] = []
    }
    grouped[depth].push(node.id)
  })

  const placedY = {}
  const depths = Object.keys(grouped).map((item) => Number(item)).sort((a, b) => a - b)
  depths.forEach((depth) => {
    const layerNodeKeys = grouped[depth]
    layerNodeKeys.sort((a, b) => {
      const parentsA = parentsMap[a] || []
      const parentsB = parentsMap[b] || []
      const avgA = parentsA.length > 0
        ? parentsA.reduce((sum, key) => sum + Number(placedY[key] ?? RUNTIME_BASE_Y), 0) / parentsA.length
        : RUNTIME_BASE_Y
      const avgB = parentsB.length > 0
        ? parentsB.reduce((sum, key) => sum + Number(placedY[key] ?? RUNTIME_BASE_Y), 0) / parentsB.length
        : RUNTIME_BASE_Y
      if (avgA === avgB) {
        return Number(orderMap[a] || 0) - Number(orderMap[b] || 0)
      }
      return avgA - avgB
    })

    const count = layerNodeKeys.length
    const yStart = count === 1 ? RUNTIME_BASE_Y : RUNTIME_BASE_Y - ((count - 1) * ROW_Y_GAP) / 2

    layerNodeKeys.forEach((key, index) => {
      const node = nodeMap[key]
      if (!node) {
        return
      }
      node.position = {
        x: FIRST_RUNTIME_X + (depth - 1) * LAYER_X_GAP,
        y: count === 1 ? RUNTIME_BASE_Y : yStart + index * ROW_Y_GAP,
      }
      placedY[key] = node.position.y
    })
  })

  return [createStartNode(startNodeId), ...runtimeNodes]
}

export function resolveTaskNameFromNodeData(nodeData, taskNameMap) {
  const taskId = Number(nodeData?.task_id)
  if (!Number.isInteger(taskId) || taskId <= 0) {
    return '未选择任务'
  }
  return taskNameMap.get(taskId) || `任务#${taskId}`
}

export function resolveWorkflowNameFromNodeData(nodeData, workflowNameMap) {
  const workflowId = Number(nodeData?.workflow_id)
  if (!Number.isInteger(workflowId) || workflowId <= 0) {
    return '未选择Workflow'
  }
  return workflowNameMap.get(workflowId) || `Workflow#${workflowId}`
}

export function resolveNodeEnableStatusByData(nodeData, taskEnabledMap, workflowEnabledMap) {
  const nodeType = String(nodeData?.node_type || '').toLowerCase()
  if (nodeType === 'task') {
    const taskId = Number(nodeData?.task_id)
    if (!Number.isInteger(taskId) || taskId <= 0) {
      return { visible: true, status: 'missing', label: '任务未配置' }
    }
    if (!taskEnabledMap.has(taskId)) {
      return { visible: false, status: 'unknown', label: '' }
    }
    return taskEnabledMap.get(taskId)
      ? { visible: true, status: 'enabled', label: '任务已启用' }
      : { visible: true, status: 'disabled', label: '任务已禁用' }
  }

  if (nodeType === 'workflow') {
    const workflowId = Number(nodeData?.workflow_id)
    if (!Number.isInteger(workflowId) || workflowId <= 0) {
      return { visible: true, status: 'missing', label: 'Workflow未配置' }
    }
    if (!workflowEnabledMap.has(workflowId)) {
      return { visible: false, status: 'unknown', label: '' }
    }
    return workflowEnabledMap.get(workflowId)
      ? { visible: true, status: 'enabled', label: 'Workflow已启用' }
      : { visible: true, status: 'disabled', label: 'Workflow已禁用' }
  }

  return { visible: false, status: 'unknown', label: '' }
}
