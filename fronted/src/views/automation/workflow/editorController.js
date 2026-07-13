import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Modal, message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { MarkerType } from '@vue-flow/core'
import {
  goToAssetHost,
  removeLimitToken,
  resolveMatchedHostLimitToken,
  toggleLimitToken,
} from '../utils/scopeHelpers'
import {
  START_EDGE_TYPE,
  applyWorkflowEdgeVisual,
  buildWorkflowEdgeLabelStyle,
  buildWorkflowEdgeStyle,
  normalizeEdgeCondition,
  resolveWorkflowEdgePathOptions,
  resolveWorkflowEdgeType,
} from '../utils/workflowGraph'
import {
  autoLayoutTreeNodes,
  collectCascadeNodeIds as collectCascadeNodeIdsFromGraph,
  createStartNode,
  ensureStartEdgesForGraph,
  formatNodeLabel,
  isSystemEdge,
  makeNodeFromConfig,
  resolveNodeEnableStatusByData,
  resolveTaskNameFromNodeData,
  resolveWorkflowNameFromNodeData,
} from '../utils/workflowEditorGraphUtils'
import {
  getTaskList,
  getWorkflowList,
  getWorkflowDetail,
  createWorkflow,
  updateWorkflow,
  getInventoryList,
  precheckInventoryLimit,
} from '@/api/sys/automation'

export function useWorkflowEditorController() {

const START_NODE_ID = 'start'
const START_EDGE_PREFIX = 'start-edge-'
const TASK_DEFAULT_NODE_NAME = '任务节点'
const WORKFLOW_DEFAULT_NODE_NAME = 'Workflow节点'

const route = useRoute()
const router = useRouter()
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const pageLoading = ref(false)
const submitting = ref(false)
const isCreateMode = ref(true)
const editingId = ref(null)

const taskRecords = ref([])
const workflowRecords = ref([])
const inventoryRecords = ref([])

const form = reactive({
  name: '',
  description: '',
  enabled: true,
  default_inventory: undefined,
  default_limit: '',
  default_extra_vars_text: '{}',
  remark: '',
})

const flowNodes = ref([])
const flowEdges = ref([])
const canvasWrapRef = ref(null)
const selectedNodeId = ref('')
const selectedEdgeId = ref('')
const isConnecting = ref(false)
const edgeQuickPosition = reactive({ top: 12, left: 12 })

const convergenceOptions = [
  { label: 'Any（任一父节点满足即可）', value: 'any' },
  { label: 'All（所有父节点都要满足）', value: 'all' },
]

const taskOptions = computed(() => taskRecords.value.map((item) => ({ label: `${item.name} (${item.code})`, value: item.id })))
const taskNameMap = computed(() => {
  const map = new Map()
  taskRecords.value.forEach((item) => {
    const id = Number(item.id)
    if (Number.isInteger(id) && id > 0) {
      map.set(id, String(item.name || '').trim())
    }
  })
  return map
})
const workflowOptions = computed(() => workflowRecords.value
  .filter((item) => Number(item.id || 0) > 0)
  .map((item) => ({ label: `${item.name}`, value: item.id })))
const inventoryOptions = computed(() => inventoryRecords.value
  .filter((item) => Number(item.id || 0) > 0)
  .map((item) => ({ label: `${item.name}`, value: item.id })))
const workflowNameMap = computed(() => {
  const map = new Map()
  workflowRecords.value.forEach((item) => {
    const id = Number(item.id)
    if (Number.isInteger(id) && id > 0) {
      map.set(id, String(item.name || '').trim())
    }
  })
  return map
})
const taskEnabledMap = computed(() => {
  const map = new Map()
  taskRecords.value.forEach((item) => {
    const id = Number(item.id)
    if (Number.isInteger(id) && id > 0) {
      map.set(id, item.enabled !== false)
    }
  })
  return map
})
const workflowEnabledMap = computed(() => {
  const map = new Map()
  workflowRecords.value.forEach((item) => {
    const id = Number(item.id)
    if (Number.isInteger(id) && id > 0) {
      map.set(id, item.enabled !== false)
    }
  })
  return map
})
const selectedNode = computed(() => flowNodes.value.find((item) => item.id === selectedNodeId.value))
const selectedEdge = computed(() => flowEdges.value.find((item) => item.id === selectedEdgeId.value))
const selectedEditableEdge = computed(() => {
  const target = selectedEdge.value
  if (!target || isEditorSystemEdge(target)) {
    return null
  }
  return target
})
const selectedEdgeCondition = computed(() => {
  if (!selectedEditableEdge.value) {
    return 'success'
  }
  return String(selectedEditableEdge.value.data?.condition || selectedEditableEdge.value.label || 'success')
})
const edgeQuickEditorStyle = computed(() => ({
  top: `${edgeQuickPosition.top}px`,
  left: `${edgeQuickPosition.left}px`,
}))
const routeWorkflowId = computed(() => {
  const raw = route.query.id
  const value = Number(raw)
  if (!Number.isInteger(value) || value <= 0) {
    return null
  }
  return value
})

const addNodeWizardVisible = ref(false)
const basicInfoVisible = ref(false)
const nodeConfigVisible = ref(false)
const basicLimitPrechecking = ref(false)
const basicLimitPrecheckOk = ref(false)
const basicLimitPrecheckMessage = ref('请选择 Inventory 后输入 Limit，系统将实时预检')
const basicLimitAllHosts = ref([])
const basicLimitMatchedHosts = ref([])
let basicLimitPrecheckTimer = null
let basicLimitPrecheckSeq = 0

const nodeConfigForm = reactive({
  id: '',
  name: '',
  node_type: 'task',
  task_id: undefined,
  workflow_id: undefined,
  convergence: 'any',
  run_type: 'success',
})

const basicLimitPrecheckText = computed(() => {
  if (basicLimitPrechecking.value && basicLimitPrecheckMessage.value === '正在预检...') {
    return '正在预检...'
  }
  return basicLimitPrecheckMessage.value
})

function clearBasicLimitPrecheckTimer() {
  if (basicLimitPrecheckTimer) {
    clearTimeout(basicLimitPrecheckTimer)
    basicLimitPrecheckTimer = null
  }
}

function scheduleBasicLimitPrecheck(delay = 300) {
  clearBasicLimitPrecheckTimer()
  basicLimitPrecheckTimer = setTimeout(() => {
    doBasicLimitPrecheck()
  }, delay)
}

async function doBasicLimitPrecheck() {
  if (!basicInfoVisible.value) {
    return
  }

  const inventoryId = Number(form.default_inventory)
  if (!Number.isInteger(inventoryId) || inventoryId <= 0) {
    basicLimitPrecheckOk.value = false
    basicLimitPrechecking.value = false
    basicLimitAllHosts.value = []
    basicLimitMatchedHosts.value = []
    basicLimitPrecheckMessage.value = '请选择 Inventory 后输入 Limit，系统将实时预检'
    return
  }

  const currentSeq = ++basicLimitPrecheckSeq
  basicLimitPrechecking.value = true
  try {
    const limitText = String(form.default_limit || '').trim()
    const baseRes = await precheckInventoryLimit(inventoryId, { limit: '' })
    if (currentSeq !== basicLimitPrecheckSeq) {
      return
    }

    let data = baseRes?.data?.data || {}
    if (limitText) {
      const narrowedRes = await precheckInventoryLimit(inventoryId, { limit: limitText })
      if (currentSeq !== basicLimitPrecheckSeq) {
        return
      }
      data = narrowedRes?.data?.data || {}
    }

    const baseData = baseRes?.data?.data || {}
    basicLimitPrecheckOk.value = !!data.ok
    basicLimitAllHosts.value = Array.isArray(baseData.matched_hosts_preview) ? baseData.matched_hosts_preview : []
    basicLimitMatchedHosts.value = Array.isArray(data.matched_hosts_preview) ? data.matched_hosts_preview : []
    basicLimitPrecheckMessage.value = data.message || '预检完成'
  } catch (error) {
    if (currentSeq !== basicLimitPrecheckSeq) {
      return
    }
    basicLimitPrecheckOk.value = false
    basicLimitAllHosts.value = []
    basicLimitMatchedHosts.value = []
    basicLimitPrecheckMessage.value = error?.message || '预检失败，请稍后重试'
  } finally {
    if (currentSeq === basicLimitPrecheckSeq) {
      basicLimitPrechecking.value = false
    }
  }
}

function handleBasicLimitHostClick(item) {
  goToAssetHost(router, message, item?.host_id, item?.host_name)
}

function handleBasicLimitLimitToggle(item) {
  const token = resolveMatchedHostLimitToken(item)
  form.default_limit = toggleLimitToken(form.default_limit, token)
}

function handleBasicLimitRemoveToken(token) {
  form.default_limit = removeLimitToken(form.default_limit, token)
}

const nodeConfigIncomingEdgeId = ref('')
const nodeConfigHasParentCondition = ref(false)
const nodeConfigRunTypeEditable = ref(false)
const nodeConfigRunTypeHint = ref('')

const addNodeWizardForm = reactive({
  parent_node_key: '',
  run_type: 'success',
  node_type: 'task',
  name: TASK_DEFAULT_NODE_NAME,
  task_id: undefined,
  workflow_id: undefined,
  convergence: 'any',
})

const addNodeWizardHasParentCondition = computed(() => Boolean(String(addNodeWizardForm.parent_node_key || '').trim()))

function createEditorStartNode() {
  return createStartNode(START_NODE_ID)
}

function isEditorSystemEdge(edge) {
  return isSystemEdge(edge, START_NODE_ID, START_EDGE_PREFIX)
}

function focusNode(nodeId) {
  if (!nodeId || nodeId === START_NODE_ID) {
    return
  }
  selectedNodeId.value = nodeId
  selectedEdgeId.value = ''
}

function openTaskInNewWindowByNode(node) {
  const taskId = Number(node?.data?.task_id)
  if (!Number.isInteger(taskId) || taskId <= 0) {
    return false
  }

  const target = router.resolve({
    path: '/sys/automation',
    query: {
      task_id: String(taskId),
    },
  })

  window.open(target.href, '_blank', 'noopener,noreferrer')
  return true
}

function openWorkflowInNewWindowByNode(node) {
  const workflowId = Number(node?.data?.workflow_id)
  if (!Number.isInteger(workflowId) || workflowId <= 0) {
    return false
  }

  const target = router.resolve({
    path: '/sys/automation/workflow/editor',
    query: {
      id: String(workflowId),
    },
  })

  window.open(target.href, '_blank', 'noopener,noreferrer')
  return true
}

function handleNodeCardClick(nodeId) {
  if (!nodeId || nodeId === START_NODE_ID) {
    return
  }
  focusNode(nodeId)
}

function openTaskDetailByNodeId(nodeId) {
  const target = flowNodes.value.find((item) => item.id === nodeId)
  if (!target) {
    return
  }
  focusNode(nodeId)
  openTaskInNewWindowByNode(target)
}

function openWorkflowDetailByNodeId(nodeId) {
  const target = flowNodes.value.find((item) => item.id === nodeId)
  if (!target) {
    return
  }
  focusNode(nodeId)
  openWorkflowInNewWindowByNode(target)
}

function resolveTaskNameByNodeData(nodeData) {
  return resolveTaskNameFromNodeData(nodeData, taskNameMap.value)
}

function resolveWorkflowNameByNodeData(nodeData) {
  return resolveWorkflowNameFromNodeData(nodeData, workflowNameMap.value)
}

function resolveNodeEnableStatus(nodeData) {
  return resolveNodeEnableStatusByData(nodeData, taskEnabledMap.value, workflowEnabledMap.value)
}

function openNodeConfigDialog(nodeId) {
  const target = flowNodes.value.find((item) => item.id === nodeId)
  if (!target || nodeId === START_NODE_ID) {
    return
  }
  focusNode(nodeId)
  nodeConfigForm.id = nodeId
  nodeConfigForm.name = String(target.data?.name || nodeId)
  nodeConfigForm.node_type = String(target.data?.node_type || 'task').toLowerCase() === 'workflow' ? 'workflow' : 'task'
  nodeConfigForm.task_id = target.data?.task_id
  nodeConfigForm.workflow_id = target.data?.workflow_id
  nodeConfigForm.convergence = String(target.data?.convergence || 'any').toLowerCase() === 'all' ? 'all' : 'any'

  const incomingBusinessEdges = flowEdges.value.filter(
    (item) => !isEditorSystemEdge(item) && item.target === nodeId,
  )

  if (incomingBusinessEdges.length === 1) {
    nodeConfigHasParentCondition.value = true
    const incoming = incomingBusinessEdges[0]
    nodeConfigIncomingEdgeId.value = String(incoming.id || '')
    nodeConfigForm.run_type = String(incoming.data?.condition || incoming.label || 'success')
    nodeConfigRunTypeEditable.value = true
    nodeConfigRunTypeHint.value = ''
  } else if (incomingBusinessEdges.length === 0) {
    nodeConfigHasParentCondition.value = false
    nodeConfigIncomingEdgeId.value = ''
    nodeConfigForm.run_type = 'success'
    nodeConfigRunTypeEditable.value = false
    nodeConfigRunTypeHint.value = ''
  } else {
    nodeConfigHasParentCondition.value = true
    nodeConfigIncomingEdgeId.value = ''
    nodeConfigForm.run_type = String(incomingBusinessEdges[0].data?.condition || incomingBusinessEdges[0].label || 'success')
    nodeConfigRunTypeEditable.value = false
    nodeConfigRunTypeHint.value = '当前节点存在多个父节点，请在连线上分别修改运行条件。'
  }

  nodeConfigVisible.value = true
}

function openNodeEditDialog(nodeId) {
  openNodeConfigDialog(nodeId)
}

function saveNodeConfig() {
  const target = flowNodes.value.find((item) => item.id === nodeConfigForm.id)
  if (!target || target.id === START_NODE_ID) {
    return
  }

  const nodeName = String(nodeConfigForm.name || '').trim()
  if (!nodeName) {
    message.error('节点名称不能为空')
    return
  }

  if (nodeConfigForm.node_type === 'workflow') {
    const workflowId = Number(nodeConfigForm.workflow_id)
    if (!Number.isInteger(workflowId) || workflowId <= 0) {
      message.error('请选择Workflow')
      return
    }
    target.data.node_type = 'workflow'
    target.data.workflow_id = workflowId
    target.data.task_id = undefined
  } else {
    const taskId = Number(nodeConfigForm.task_id)
    if (!Number.isInteger(taskId) || taskId <= 0) {
      message.error('请选择执行任务')
      return
    }
    target.data.node_type = 'task'
    target.data.task_id = taskId
    target.data.workflow_id = undefined
  }
  target.data.convergence = String(nodeConfigForm.convergence || 'any').toLowerCase() === 'all' ? 'all' : 'any'

  if (nodeConfigRunTypeEditable.value && nodeConfigIncomingEdgeId.value) {
    const incoming = flowEdges.value.find((item) => item.id === nodeConfigIncomingEdgeId.value)
    if (incoming && !isEditorSystemEdge(incoming)) {
      applyWorkflowEdgeVisual(incoming, nodeConfigForm.run_type)
    }
  }

  target.data.name = nodeName
  target.label = formatNodeLabel(target.data)
  flowNodes.value = [...flowNodes.value]
  flowEdges.value = [...flowEdges.value]
  nodeConfigVisible.value = false
}

function selectNodeConfigRunType(condition) {
  if (!nodeConfigRunTypeEditable.value) {
    return
  }
  nodeConfigForm.run_type = String(condition || 'success')
}

function makeEdgeFromConfig(config, index = 0) {
  const condition = normalizeEdgeCondition(config.condition)
  const pathOptions = resolveWorkflowEdgePathOptions(condition)
  return {
    id: `edge-${config.source_key}-${config.target_key}-${condition}-${index}`,
    type: resolveWorkflowEdgeType(condition),
    source: config.source_key,
    target: config.target_key,
    label: condition,
    data: { condition },
    ...(pathOptions ? { pathOptions } : {}),
    markerEnd: MarkerType.ArrowClosed,
    style: buildWorkflowEdgeStyle(condition),
    labelStyle: buildWorkflowEdgeLabelStyle(),
  }
}

function parseDefaultExtraVars() {
  const rawText = String(form.default_extra_vars_text || '').trim() || '{}'
  try {
    const parsed = JSON.parse(rawText)
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      throw new Error('默认变量必须是 JSON 对象')
    }
    return parsed
  } catch (error) {
    throw new Error('默认变量不是合法 JSON 对象')
  }
}

function generateNodeKey(nodeName) {
  const base = String(nodeName || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '') || 'node'

  let candidate = base
  let index = 2
  const keySet = new Set(flowNodes.value.filter((item) => item.id !== START_NODE_ID).map((item) => item.id))
  while (keySet.has(candidate)) {
    candidate = `${base}-${index}`
    index += 1
  }
  return candidate
}

function ensureStartEdges() {
  // START 辅助边只用于画布可视化入口，不参与后端编排数据。
  flowEdges.value = ensureStartEdgesForGraph({
    flowNodes: flowNodes.value,
    flowEdges: flowEdges.value,
    startNodeId: START_NODE_ID,
    startEdgePrefix: START_EDGE_PREFIX,
    startEdgeType: START_EDGE_TYPE,
    markerEnd: MarkerType.ArrowClosed,
    buildWorkflowEdgeStyle,
    buildWorkflowEdgeLabelStyle,
  })
}

function autoLayoutTree() {
  // Workflow 编排按树形布局：从左到右展开，同一层节点按从上到下排序。
  flowNodes.value = autoLayoutTreeNodes({
    flowNodes: flowNodes.value,
    flowEdges: flowEdges.value,
    startNodeId: START_NODE_ID,
    startEdgePrefix: START_EDGE_PREFIX,
  })
}

function resetBuilderGraph() {
  flowNodes.value = [createEditorStartNode()]
  flowEdges.value = []
  selectedNodeId.value = ''
  selectedEdgeId.value = ''
}

function resetForm() {
  editingId.value = null
  form.name = ''
  form.description = ''
  form.enabled = true
  form.default_inventory = undefined
  form.default_limit = ''
  form.default_extra_vars_text = '{}'
  form.remark = ''
  resetBuilderGraph()
}

function resetAddNodeWizardForm() {
  addNodeWizardForm.parent_node_key = ''
  addNodeWizardForm.run_type = 'success'
  addNodeWizardForm.node_type = 'task'
  addNodeWizardForm.name = TASK_DEFAULT_NODE_NAME
  addNodeWizardForm.task_id = taskOptions.value[0]?.value
  addNodeWizardForm.workflow_id = workflowOptions.value[0]?.value
  addNodeWizardForm.convergence = 'any'
}

function fillForm(record) {
  editingId.value = record.id
  form.name = record.name || ''
  form.description = record.description || ''
  form.enabled = Boolean(record.enabled)
  const savedInventoryId = Number(record.default_inventory || 0) > 0 ? Number(record.default_inventory) : undefined
  // 检查已保存的 inventory 是否仍存在（可能已被删除）
  if (savedInventoryId && inventoryRecords.value.length > 0) {
    const exists = inventoryRecords.value.some((item) => Number(item.id) === savedInventoryId)
    if (!exists) {
      message.warning('该 Workflow 绑定的 Inventory 已被删除，请重新选择')
      form.default_inventory = undefined
    } else {
      form.default_inventory = savedInventoryId
    }
  } else {
    form.default_inventory = savedInventoryId
  }
  form.default_limit = String(record.default_limit || '')
  form.default_extra_vars_text = JSON.stringify(record.default_extra_vars || {}, null, 2)
  form.remark = record.remark || ''

  const graphNodes = Array.isArray(record.nodes)
    ? record.nodes.map((item, index) => makeNodeFromConfig(item, index))
    : []
  const graphEdges = Array.isArray(record.edges)
    ? record.edges.map((item, index) => makeEdgeFromConfig(item, index))
    : []

  flowNodes.value = [createEditorStartNode(), ...graphNodes]
  flowEdges.value = graphEdges
  // 详情接口返回的是业务边，这里补回 START 系统边用于编辑体验。
  ensureStartEdges()
}

function resolveNodeNameByKey(nodeKey) {
  const target = flowNodes.value.find((item) => item.id === nodeKey)
  if (!target) {
    return nodeKey || '-'
  }
  if (target.id === START_NODE_ID) {
    return 'START'
  }
  return target.data?.name || target.id
}

function openAddNodeWizard(options = {}) {
  const runtimeNodes = flowNodes.value.filter((item) => item.id !== START_NODE_ID)
  const hasRuntimeNode = runtimeNodes.length > 0
  const forceRoot = Boolean(options.forceRoot)
  const selectedParentNode = selectedNode.value && selectedNode.value.id !== START_NODE_ID
    ? selectedNode.value.id
    : ''
  const incomingParentNodeId = String(options.parentNodeId || '').trim()
  const parentNodeId = forceRoot ? '' : (incomingParentNodeId || selectedParentNode)

  resetAddNodeWizardForm()
  addNodeWizardForm.node_type = String(options.presetNodeType || 'task') === 'workflow' ? 'workflow' : 'task'
  addNodeWizardForm.name = addNodeWizardForm.node_type === 'workflow' ? WORKFLOW_DEFAULT_NODE_NAME : TASK_DEFAULT_NODE_NAME
  addNodeWizardForm.parent_node_key = hasRuntimeNode ? parentNodeId : ''
  if (!addNodeWizardForm.parent_node_key) {
    addNodeWizardForm.run_type = 'success'
  }
  addNodeWizardVisible.value = true
}

function closeAddNodeWizard() {
  addNodeWizardVisible.value = false
}

function selectRunType(condition) {
  if (!addNodeWizardForm.parent_node_key) {
    return
  }
  addNodeWizardForm.run_type = String(condition || 'success')
}

function createNodeFromWizard() {
  const nodeName = String(addNodeWizardForm.name || '').trim()
  if (!nodeName) {
    message.error('请填写节点名称')
    return
  }

  const index = flowNodes.value.filter((item) => item.id !== START_NODE_ID).length
  const key = generateNodeKey(nodeName)
  let node = null

  if (addNodeWizardForm.node_type === 'workflow') {
    const workflowId = Number(addNodeWizardForm.workflow_id)
    if (!Number.isInteger(workflowId) || workflowId <= 0) {
      message.error('请选择Workflow')
      return
    }
    node = makeNodeFromConfig(
      {
        key,
        name: nodeName,
        node_type: 'workflow',
        workflow_id: workflowId,
        convergence: String(addNodeWizardForm.convergence || 'any').toLowerCase() === 'all' ? 'all' : 'any',
      },
      index,
    )
  } else {
    const taskId = Number(addNodeWizardForm.task_id)
    if (!Number.isInteger(taskId) || taskId <= 0) {
      message.error('请选择执行任务')
      return
    }
    node = makeNodeFromConfig(
      {
        key,
        name: nodeName,
        node_type: 'task',
        task_id: taskId,
        convergence: String(addNodeWizardForm.convergence || 'any').toLowerCase() === 'all' ? 'all' : 'any',
      },
      index,
    )
  }

  flowNodes.value.push(node)
  selectedNodeId.value = node.id
  selectedEdgeId.value = ''

  const runtimeNodes = flowNodes.value.filter((item) => item.id !== START_NODE_ID)

  const parentNodeKey = String(addNodeWizardForm.parent_node_key || '').trim()
  if (runtimeNodes.length > 1 && parentNodeKey && parentNodeKey !== node.id) {
    const condition = normalizeEdgeCondition(addNodeWizardForm.run_type)
    const pathOptions = resolveWorkflowEdgePathOptions(condition)
    const duplicate = flowEdges.value.some(
      (item) => item.source === parentNodeKey && item.target === node.id,
    )
    if (!duplicate) {
      flowEdges.value.push({
        id: `edge-${parentNodeKey}-${node.id}-${Date.now()}`,
        type: resolveWorkflowEdgeType(condition),
        source: parentNodeKey,
        target: node.id,
        label: condition,
        data: { condition },
        ...(pathOptions ? { pathOptions } : {}),
        markerEnd: MarkerType.ArrowClosed,
        style: buildWorkflowEdgeStyle(condition),
        labelStyle: buildWorkflowEdgeLabelStyle(),
      })
    }
  }

  autoLayoutTree()
  ensureStartEdges()

  addNodeWizardVisible.value = false
  message.success('节点已创建，请继续编排并保存')
}

function deleteNodesCascade(nodeIds) {
  if (!(nodeIds instanceof Set) || nodeIds.size === 0) {
    return
  }
  flowNodes.value = flowNodes.value.filter((item) => !nodeIds.has(item.id))
  flowEdges.value = flowEdges.value.filter((item) => !nodeIds.has(item.source) && !nodeIds.has(item.target))
  selectedNodeId.value = ''
  selectedEdgeId.value = ''
  autoLayoutTree()
  ensureStartEdges()
}

function removeNode(nodeId) {
  if (!nodeId || nodeId === START_NODE_ID) {
    return
  }

  const cascadeNodeIds = collectCascadeNodeIdsFromGraph(
    nodeId,
    flowEdges.value,
    START_NODE_ID,
    START_EDGE_PREFIX,
  )
  const removeCount = cascadeNodeIds.size
  const targetName = resolveNodeNameByKey(nodeId)
  const content = removeCount > 1
    ? `确认删除节点“${targetName}”以及其 ${removeCount - 1} 个子节点吗？`
    : `确认删除节点“${targetName}”吗？`

  Modal.confirm({
    title: '确认删除节点',
    content,
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    onOk: () => {
      deleteNodesCascade(cascadeNodeIds)
      message.success(removeCount > 1 ? '节点及其子节点已删除' : '节点已删除')
    },
  })
}

function removeEdge(edgeId) {
  if (!edgeId || String(edgeId).startsWith(START_EDGE_PREFIX)) {
    return
  }
  flowEdges.value = flowEdges.value.filter((item) => item.id !== edgeId)
  selectedEdgeId.value = ''
  ensureStartEdges()
}

function updateSelectedEdgeCondition(condition) {
  const target = selectedEdge.value
  if (!target || isEditorSystemEdge(target)) {
    return
  }
  applyWorkflowEdgeVisual(target, condition)
  flowEdges.value = [...flowEdges.value]
}

function handleConnect(params) {
  const source = String(params.source || '')
  const target = String(params.target || '')
  if (!source || !target || source === target) {
    return
  }
  if (source === START_NODE_ID || target === START_NODE_ID) {
    return
  }

  const duplicate = flowEdges.value.some(
    (item) => item.source === source && item.target === target,
  )
  if (duplicate) {
    return
  }

  const edge = {
    id: `edge-${source}-${target}-${Date.now()}`,
    type: resolveWorkflowEdgeType('success'),
    source,
    target,
    label: 'success',
    data: { condition: 'success' },
    ...(resolveWorkflowEdgePathOptions('success') ? { pathOptions: resolveWorkflowEdgePathOptions('success') } : {}),
    markerEnd: MarkerType.ArrowClosed,
    style: buildWorkflowEdgeStyle('success'),
    labelStyle: buildWorkflowEdgeLabelStyle(),
  }
  flowEdges.value.push(edge)
  ensureStartEdges()
  isConnecting.value = false
}

function handleConnectStart() {
  isConnecting.value = true
}

function handleConnectEnd() {
  isConnecting.value = false
}

function handleNodeClick(payload) {
  const targetNode = payload?.node
  selectedNodeId.value = targetNode?.id || ''
  selectedEdgeId.value = ''
}

function handleEdgeClick(payload) {
  const targetEdge = payload?.edge
  if (!targetEdge || isEditorSystemEdge(targetEdge)) {
    return
  }
  selectedEdgeId.value = targetEdge.id
  selectedNodeId.value = ''

  const mouseEvent = payload?.event
  const wrapEl = canvasWrapRef.value
  if (!wrapEl || !mouseEvent) {
    return
  }

  const rect = wrapEl.getBoundingClientRect()
  const menuWidth = 190
  const menuHeight = 190
  const padding = 10
  const maxLeft = Math.max(padding, rect.width - menuWidth - padding)
  const maxTop = Math.max(padding, rect.height - menuHeight - padding)

  const rawLeft = mouseEvent.clientX - rect.left + 10
  const rawTop = mouseEvent.clientY - rect.top + 10

  edgeQuickPosition.left = Math.min(maxLeft, Math.max(padding, rawLeft))
  edgeQuickPosition.top = Math.min(maxTop, Math.max(padding, rawTop))
}

function setSelectedEdgeCondition(condition) {
  updateSelectedEdgeCondition(String(condition || 'success'))
}

function removeSelectedEdge() {
  if (!selectedEditableEdge.value) {
    return
  }
  removeEdge(String(selectedEditableEdge.value.id || ''))
}

function clearSelection() {
  selectedNodeId.value = ''
  selectedEdgeId.value = ''
}

function buildPayloadFromGraph() {
  if (!String(form.name || '').trim()) {
    throw new Error('名称不能为空')
  }

  const runtimeNodes = flowNodes.value.filter((item) => item.id !== START_NODE_ID)

  const nodes = runtimeNodes.map((item) => {
    const data = item.data || {}
    const payload = {
      key: item.id,
      name: String(data.name || '').trim(),
      node_type: String(data.node_type || 'task').trim(),
      convergence: String(data.convergence || 'any').toLowerCase() === 'all' ? 'all' : 'any',
      x: Number(item.position?.x || 0),
      y: Number(item.position?.y || 0),
    }
    if (!payload.name) {
      throw new Error('存在未命名节点，请补充节点名称')
    }
    payload.node_type = String(data.node_type || 'task').toLowerCase() === 'workflow' ? 'workflow' : 'task'
    if (payload.node_type === 'workflow') {
      const workflowId = Number(data.workflow_id)
      if (!Number.isInteger(workflowId) || workflowId <= 0) {
        throw new Error(`节点【${payload.name}】未选择Workflow`)
      }
      payload.workflow_id = workflowId
    } else {
      const taskId = Number(data.task_id)
      if (!Number.isInteger(taskId) || taskId <= 0) {
        throw new Error(`节点【${payload.name}】未选择执行任务`)
      }
      payload.task_id = taskId
    }
    return payload
  })

  // 提交前剔除 START 系统边，避免污染后端 DAG 定义。
  const edges = flowEdges.value
    .filter((item) => item.source !== START_NODE_ID && !String(item.id || '').startsWith(START_EDGE_PREFIX))
    .map((item) => ({
      source_key: item.source,
      target_key: item.target,
      condition: String(item.data?.condition || item.label || 'success').toLowerCase(),
    }))

  return {
    name: String(form.name || '').trim(),
    description: String(form.description || '').trim(),
    enabled: Boolean(form.enabled),
    default_inventory: Number(form.default_inventory || 0) > 0 ? Number(form.default_inventory) : null,
    default_limit: String(form.default_limit || '').trim(),
    entry_node_key: '',
    nodes,
    edges,
    default_extra_vars: parseDefaultExtraVars(),
    remark: String(form.remark || '').trim(),
  }
}

async function loadTaskOptions() {
  const res = await getTaskList({ page: 1, page_size: 200 })
  const data = res?.data?.data || {}
  taskRecords.value = Array.isArray(data.results) ? data.results : []
}

async function loadWorkflowOptions() {
  // Workflow 节点下拉允许选择当前模板；递归在后端运行时拦截。
  const res = await getWorkflowList({ page: 1, page_size: 500, ordering: '-id' })
  const data = res?.data?.data || {}
  workflowRecords.value = Array.isArray(data.results) ? data.results : []
}

async function loadInventoryOptions() {
  const res = await getInventoryList({ page: 1, page_size: 500, ordering: '-id' })
  const data = res?.data?.data || {}
  inventoryRecords.value = Array.isArray(data.results) ? data.results : []
}

async function loadWorkflowDetail(id) {
  const res = await getWorkflowDetail(id)
  const data = res?.data?.data || {}
  fillForm(data)
}

async function initEditor() {
  pageLoading.value = true
  try {
    await Promise.all([loadTaskOptions(), loadInventoryOptions()])
    const id = routeWorkflowId.value
    if (id) {
      isCreateMode.value = false
      await loadWorkflowDetail(id)
      await loadWorkflowOptions()
      const isWizardMode = String(route.query.wizard || '').trim() === 'first-node'
      const hasRuntimeNode = flowNodes.value.some((item) => item.id !== START_NODE_ID)
      if (isWizardMode && !hasRuntimeNode) {
        openAddNodeWizard()
      }
      return
    }
    isCreateMode.value = true
    resetForm()
    await loadWorkflowOptions()
  } finally {
    pageLoading.value = false
  }
}

function goToTaskPage() {
  router.push('/sys/automation')
}

async function saveWorkflow() {
  let payload
  try {
    payload = buildPayloadFromGraph()
  } catch (error) {
    message.error(error.message || '编排内容不合法')
    return false
  }

  submitting.value = true
  try {
    if (isCreateMode.value) {
      const res = await createWorkflow(payload)
      const data = res?.data?.data || {}
      const createdId = Number(data.id || 0)
      message.success('Workflow 创建成功')
      if (createdId > 0) {
        await router.replace({ path: '/sys/automation/workflow/editor', query: { id: createdId } })
      }
    } else {
      await updateWorkflow(editingId.value, payload)
      message.success('Workflow 更新成功')
    }
    return true
  } finally {
    submitting.value = false
  }
}

async function handleBasicInfoConfirm() {
  const saved = await saveWorkflow()
  if (saved) {
    basicInfoVisible.value = false
  }
}

function goBack() {
  router.push('/sys/automation/workflow')
}

watch(routeWorkflowId, async () => {
  await initEditor()
}, { immediate: true })

watch(() => addNodeWizardForm.node_type, (nextType, prevType) => {
  const currentName = String(addNodeWizardForm.name || '').trim()
  const prevDefault = prevType === 'workflow' ? WORKFLOW_DEFAULT_NODE_NAME : TASK_DEFAULT_NODE_NAME
  const nextDefault = nextType === 'workflow' ? WORKFLOW_DEFAULT_NODE_NAME : TASK_DEFAULT_NODE_NAME

  // 仅在空值或仍是上一类型默认名时自动切换，避免覆盖用户手工输入。
  if (!currentName || currentName === prevDefault) {
    addNodeWizardForm.name = nextDefault
  }
})

watch(
  () => [form.default_inventory, form.default_limit, basicInfoVisible.value],
  ([, , visible]) => {
    if (!visible) {
      return
    }
    basicLimitPrecheckMessage.value = '正在预检...'
    scheduleBasicLimitPrecheck(300)
  },
)

onBeforeUnmount(() => {
  clearBasicLimitPrecheckTimer()
})

  return {
    getPopupContainer,
    pageLoading,
    submitting,
    form,
    flowNodes,
    flowEdges,
    canvasWrapRef,
    selectedNodeId,
    isConnecting,
    convergenceOptions,
    taskOptions,
    workflowOptions,
    inventoryOptions,
    selectedEditableEdge,
    selectedEdgeCondition,
    edgeQuickEditorStyle,
    addNodeWizardVisible,
    basicInfoVisible,
    nodeConfigVisible,
    basicLimitPrechecking,
    basicLimitPrecheckOk,
    basicLimitAllHosts,
    basicLimitMatchedHosts,
    nodeConfigForm,
    basicLimitPrecheckText,
    nodeConfigHasParentCondition,
    nodeConfigRunTypeEditable,
    nodeConfigRunTypeHint,
    addNodeWizardForm,
    addNodeWizardHasParentCondition,
    handleNodeCardClick,
    openTaskDetailByNodeId,
    openWorkflowDetailByNodeId,
    resolveTaskNameByNodeData,
    resolveWorkflowNameByNodeData,
    resolveNodeEnableStatus,
    openNodeEditDialog,
    removeNode,
    setSelectedEdgeCondition,
    removeSelectedEdge,
    closeAddNodeWizard,
    selectRunType,
    createNodeFromWizard,
    handleBasicInfoConfirm,
    handleBasicLimitHostClick,
    handleBasicLimitLimitToggle,
    handleBasicLimitRemoveToken,
    saveNodeConfig,
    selectNodeConfigRunType,
    openAddNodeWizard,
    goToTaskPage,
    goBack,
    saveWorkflow,
    handleConnect,
    handleConnectStart,
    handleConnectEnd,
    handleNodeClick,
    handleEdgeClick,
    clearSelection,
  }
}
