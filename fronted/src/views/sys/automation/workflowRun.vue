<template>
  <div class="workflow-editor-page">
    <div class="editor-toolbar">
      <div class="editor-title">
        <span class="name">{{ workflowName }}</span>
        <a-tag :color="runStatusColor">{{ runStatusLabel }}</a-tag>
        <a-tag color="blue">运行ID: #{{ runId || '-' }}</a-tag>
        <a-tag color="cyan">{{ runTotalDurationLabel }}</a-tag>
      </div>
      <a-space>
        <a-button @click="goBack">返回列表</a-button>
        <a-popconfirm
          v-if="canCancelCurrentRun"
          title="确认取消当前 Workflow 运行吗？"
          ok-text="确认"
          cancel-text="取消"
          @confirm="cancelCurrentRun"
        >
          <a-button danger :loading="cancelingRun">取消运行</a-button>
        </a-popconfirm>
        <a-button type="primary" ghost :loading="runLoading" @click="reloadNow">刷新</a-button>
      </a-space>
    </div>

    <a-alert type="info" show-icon :message="summaryText" class="run-summary-alert" />

    <div class="run-count-row">
      <a-tag color="default">待执行 {{ pendingCount }}</a-tag>
      <a-tag color="gold">等待中 {{ waitingCount }}</a-tag>
      <a-tag color="processing">运行中 {{ runningCount }}</a-tag>
      <a-tag color="green">成功 {{ successCount }}</a-tag>
      <a-tag color="default">已跳过 {{ skippedCount }}</a-tag>
      <a-tag color="orange">已取消 {{ cancelledCount }}</a-tag>
      <a-tag color="red">失败 {{ failedCount }}</a-tag>
    </div>

    <a-skeleton active :loading="runLoading && flowNodes.length === 0">
      <div class="canvas-wrap">
        <VueFlow
          v-model:nodes="flowNodes"
          v-model:edges="flowEdges"
          :fit-view-on-init="true"
          :nodes-draggable="false"
          :nodes-connectable="false"
          :elements-selectable="false"
          class="workflow-canvas"
          @node-click="handleNodeClick"
        >
          <template #node-workflow-start-node>
            <div class="workflow-start-node-card">
              <Handle type="source" :position="Position.Right" :connectable="false" />
              START
            </div>
          </template>
          <template #node-workflow-node="nodeProps">
            <div class="workflow-node-wrap">
              <a-tooltip
                :title="nodeProps.data?.nodeMessage || ''"
                :visible="!!(nodeProps.data?.nodeMessage)"
                placement="topLeft"
              >
                <div
                  class="workflow-node-card"
                  :class="`status-${normalizeNodeStatus(nodeProps.data?.status)}`"
                >
                  <Handle type="target" :position="Position.Left" :connectable="false" />
                  <Handle type="source" :position="Position.Right" :connectable="false" />

                <div class="workflow-node-state-icon" :class="`icon-${normalizeNodeStatus(nodeProps.data?.status)}`">
                  {{ getNodeStatusIcon(nodeProps.data?.status) }}
                </div>

                <div
                  class="workflow-node-type-icon"
                  :class="String(nodeProps.data?.nodeType || '').toLowerCase() === 'workflow' ? 'is-workflow' : 'is-task'"
                  :title="String(nodeProps.data?.nodeType || '').toLowerCase() === 'workflow' ? 'Workflow节点' : '任务节点'"
                >
                  <ApartmentOutlined v-if="String(nodeProps.data?.nodeType || '').toLowerCase() === 'workflow'" />
                  <ToolOutlined v-else />
                </div>
                <div
                  v-if="String(nodeProps.data?.convergence || 'any').toLowerCase() === 'all'"
                  class="workflow-node-convergence-tag"
                >
                  ALL
                </div>
                <div v-if="String(nodeProps.data?.nodeType || '').toLowerCase() === 'task'" class="workflow-node-quick-actions">
                  <span class="node-quick-ref-name" :title="resolveTaskRefLabel(nodeProps.data)">{{ resolveTaskRefLabel(nodeProps.data) }}</span>
                  <button
                    type="button"
                    class="node-quick-btn"
                    @click.stop="handleTaskDetailAction(nodeProps.data, nodeProps.id)"
                  >详细</button>
                  <button
                    v-if="hasNodeLogs(nodeProps.data)"
                    type="button"
                    class="node-quick-btn"
                    @click.stop="handleTaskLogAction(nodeProps.data, nodeProps.id)"
                  >日志</button>
                  <a-popconfirm
                    v-if="canCancelNodeJob(nodeProps.data)"
                    title="确认取消当前运行中的任务吗？"
                    ok-text="确认"
                    cancel-text="取消"
                    @confirm="cancelNodeJob(nodeProps.data, nodeProps.id)"
                  >
                    <button
                      type="button"
                      class="node-quick-btn node-quick-btn-danger"
                      :disabled="cancelingNodeJobId === Number(nodeProps.data?.jobId)"
                      @click.stop="suppressNextNodeClick"
                    >
                      {{ cancelingNodeJobId === Number(nodeProps.data?.jobId) ? '取消中' : '取消' }}
                    </button>
                  </a-popconfirm>
                </div>
                <div v-else-if="String(nodeProps.data?.nodeType || '').toLowerCase() === 'workflow'" class="workflow-node-quick-actions">
                  <span class="node-quick-ref-name" :title="resolveWorkflowRefLabel(nodeProps.data)">{{ resolveWorkflowRefLabel(nodeProps.data) }}</span>
                  <button
                    type="button"
                    class="node-quick-btn"
                    @click.stop="handleWorkflowDetailAction(nodeProps.data, nodeProps.id)"
                  >详细</button>
                  <button
                    v-if="canOpenWorkflowRunGraph(nodeProps.data)"
                    type="button"
                    class="node-quick-btn"
                    @click.stop="handleWorkflowRunGraphAction(nodeProps.data, nodeProps.id)"
                  >运行图</button>
                </div>
                <div class="workflow-node-runtime">{{ toNodeStatusLabel(nodeProps.data?.status) }}</div>
              </div>
              </a-tooltip>
              <div
                class="workflow-node-name"
              >
                <span class="workflow-node-name-text" :title="nodeProps.data?.name || nodeProps.id">{{ nodeProps.data?.name || nodeProps.id }}</span>
                <span
                  v-if="getNodeDurationText(nodeProps.data)"
                  class="workflow-node-duration"
                  :title="getNodeDurationTitle(nodeProps.data)"
                >耗时 {{ getNodeDurationText(nodeProps.data) }}</span>
              </div>
            </div>
          </template>
          <Background pattern-color="#d9d9d9" :gap="18" />
        </VueFlow>
      </div>
    </a-skeleton>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { ApartmentOutlined, ToolOutlined } from '@ant-design/icons-vue'
import { VueFlow, Handle, MarkerType, Position } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { cancelJob, cancelWorkflowRun, getWorkflowRunDetail } from '@/api/sys/automation'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'

function normalizeEdgeCondition(condition) {
  const text = String(condition || 'success').trim().toLowerCase()
  if (text === 'failure' || text === 'always') {
    return text
  }
  return 'success'
}

function resolveEdgeColor(condition) {
  const normalized = normalizeEdgeCondition(condition)
  if (normalized === 'failure') {
    return '#ff4d4f'
  }
  if (normalized === 'always') {
    return '#1677ff'
  }
  return '#52c41a'
}

const route = useRoute()
const router = useRouter()

const runLoading = ref(false)
const cancelingRun = ref(false)
const cancelingNodeJobId = ref(null)
const runDetail = ref(null)
const flowNodes = ref([])
const flowEdges = ref([])
const skipNextNodeClick = ref(false)

let pollTimer = null

const runId = computed(() => {
  const raw = route.query.run_id
  const parsed = Number(Array.isArray(raw) ? raw[0] : raw)
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return null
  }
  return parsed
})

const runStatus = computed(() => String(runDetail.value?.runtime_status || runDetail.value?.status || '').toLowerCase())
const workflowName = computed(() => String(runDetail.value?.workflow_name || runDetail.value?.workflow_name_snapshot || 'Workflow 运行状态'))

const runTotalDurationSeconds = computed(() => {
  const explicitDuration = Number(runDetail.value?.duration_seconds)
  if (Number.isFinite(explicitDuration) && explicitDuration >= 0) {
    return explicitDuration
  }

  const startTimeText = String(runDetail.value?.start_time || '').trim()
  if (!startTimeText) {
    return null
  }

  const startTimestamp = Date.parse(startTimeText)
  if (!Number.isFinite(startTimestamp)) {
    return null
  }

  const endTimeText = String(runDetail.value?.end_time || '').trim()
  const endTimestamp = endTimeText ? Date.parse(endTimeText) : Date.now()
  if (!Number.isFinite(endTimestamp)) {
    return null
  }

  return Math.max(0, (endTimestamp - startTimestamp) / 1000)
})

const runTotalDurationLabel = computed(() => {
  const seconds = runTotalDurationSeconds.value
  if (!Number.isFinite(seconds) || seconds < 0) {
    return '总耗时 --:--:--'
  }
  return `总耗时 ${formatDuration(seconds)}`
})

const runStatusLabel = computed(() => {
  if (runStatus.value === 'running') {
    return '运行中'
  }
  if (runStatus.value === 'pending') {
    return '待执行'
  }
  if (runStatus.value === 'success') {
    return '已成功'
  }
  if (runStatus.value === 'failed') {
    return '已失败'
  }
  return runStatus.value || '-'
})

const runStatusColor = computed(() => {
  if (runStatus.value === 'running') {
    return 'processing'
  }
  if (runStatus.value === 'pending') {
    return 'gold'
  }
  if (runStatus.value === 'success') {
    return 'green'
  }
  if (runStatus.value === 'failed') {
    return 'red'
  }
  return 'default'
})

const canCancelCurrentRun = computed(() => ['pending', 'running'].includes(runStatus.value))

const nodeStatuses = computed(() => {
  const source = Array.isArray(runDetail.value?.node_results_runtime)
    ? runDetail.value.node_results_runtime
    : []

  return source
    .filter((item) => item && typeof item === 'object')
    .map((item) => String(item.status || '').toLowerCase())
})

const pendingCount = computed(() => nodeStatuses.value.filter((item) => item === 'queued' || item === 'pending').length)
const waitingCount = computed(() => nodeStatuses.value.filter((item) => item === 'waiting').length)
const runningCount = computed(() => nodeStatuses.value.filter((item) => item === 'running').length)
const successCount = computed(() => nodeStatuses.value.filter((item) => item === 'success').length)
const skippedCount = computed(() => nodeStatuses.value.filter((item) => item === 'skipped').length)
const cancelledCount = computed(() => nodeStatuses.value.filter((item) => item === 'cancelled').length)
const failedCount = computed(() => nodeStatuses.value.filter((item) => item === 'failed').length)

const summaryText = computed(() => {
  const userName = String(runDetail.value?.requested_username || '-')
  return `只读运行视图：不可编辑、不可保存。触发人: ${userName} ｜ 当前状态: ${runStatusLabel.value}`
})

function normalizeNodeStatus(status) {
  const text = String(status || '').toLowerCase()
  if (['queued', 'pending', 'waiting', 'running', 'success', 'failed', 'cancelled', 'skipped'].includes(text)) {
    return text
  }
  return 'pending'
}

function toNodeStatusLabel(status) {
  const normalized = normalizeNodeStatus(status)
  if (normalized === 'running') {
    return '运行中'
  }
  if (normalized === 'success') {
    return '成功'
  }
  if (normalized === 'skipped') {
    return '已跳过（条件不满足）'
  }
  if (normalized === 'failed') {
    return '失败'
  }
  if (normalized === 'cancelled') {
    return '已取消'
  }
  if (normalized === 'queued') {
    return '排队中'
  }
  if (normalized === 'pending') {
    return 'pending中'
  }
  if (normalized === 'waiting') {
    return '等待前置节点'
  }
  return '待执行'
}

function formatDuration(seconds) {
  const totalSeconds = Math.max(0, Math.floor(Number(seconds) || 0))
  const hours = String(Math.floor(totalSeconds / 3600)).padStart(2, '0')
  const minutes = String(Math.floor((totalSeconds % 3600) / 60)).padStart(2, '0')
  const remainingSeconds = String(totalSeconds % 60).padStart(2, '0')
  return `${hours}:${minutes}:${remainingSeconds}`
}

function getNodeDurationText(nodeData) {
  const durationSeconds = Number(nodeData?.durationSeconds)
  if (!Number.isFinite(durationSeconds) || durationSeconds < 0) {
    return ''
  }
  return formatDuration(durationSeconds)
}

function getNodeDurationTitle(nodeData) {
  const statusText = toNodeStatusLabel(nodeData?.status)
  return `耗时：从开始执行到当前时刻或结束时刻的总耗时，格式为 HH:MM:SS。当前状态：${statusText}`
}

function getNodeStatusIcon(status) {
  const normalized = normalizeNodeStatus(status)
  if (normalized === 'success') {
    return 'OK'
  }
  if (normalized === 'skipped') {
    return 'S'
  }
  if (normalized === 'failed') {
    return 'X'
  }
  if (normalized === 'cancelled') {
    return 'C'
  }
  if (normalized === 'running') {
    return '...'
  }
  if (normalized === 'queued') {
    return 'Q'
  }
  if (normalized === 'waiting') {
    return 'W'
  }
  return 'P'
}

function resolveTaskRefLabel(nodeData) {
  const runtimeTaskName = String(nodeData?.runtimeTaskName || '').trim()
  const runtimeTemplateName = String(nodeData?.runtimeTemplateName || '').trim()
  if (runtimeTaskName && runtimeTemplateName) {
    return `${runtimeTaskName} @ ${runtimeTemplateName}`
  }
  if (runtimeTaskName) {
    return runtimeTaskName
  }
  const taskName = String(nodeData?.taskName || '').trim()
  if (taskName) {
    return taskName
  }
  const taskId = Number(nodeData?.taskId)
  if (Number.isInteger(taskId) && taskId > 0) {
    return `任务#${taskId}`
  }
  return '未绑定任务'
}

function resolveWorkflowRefLabel(nodeData) {
  const workflowName = String(nodeData?.workflowName || '').trim()
  if (workflowName) {
    return workflowName
  }
  const workflowId = Number(nodeData?.workflowId)
  if (Number.isInteger(workflowId) && workflowId > 0) {
    return `Workflow#${workflowId}`
  }
  return '未绑定Workflow'
}

function buildGraph(data) {
  // 运行图使用 run 快照数据，避免模板后续变更影响历史运行回放。
  // 运行图也按同一棵树理解：从左到右分层，同一层按从上到下的顺序摆放。
  const workflowNodes = Array.isArray(data?.workflow_nodes) ? data.workflow_nodes : []
  const workflowEdges = Array.isArray(data?.workflow_edges) ? data.workflow_edges : []
  const nodeResults = Array.isArray(data?.node_results_runtime)
    ? data.node_results_runtime
    : []

  const nodeStatusMap = {}
  const nodeJobIdMap = {}
  const nodeChildRunIdMap = {}
  const nodeDurationSecondsMap = {}
  const nodeJobTaskNameMap = {}
  const nodeJobTemplateNameMap = {}
  const nodeJobTaskIdMap = {}
  const nodeJobTemplateIdMap = {}
  const nodeMessageMap = {}
  nodeResults.forEach((item) => {
    const key = String(item?.node_key || '').trim()
    if (!key) {
      return
    }
    nodeStatusMap[key] = normalizeNodeStatus(item?.status)
    const message = String(item?.message || '').trim()
    if (message) {
      nodeMessageMap[key] = message
    }
    const jobId = Number(item?.job_id)
    if (Number.isInteger(jobId) && jobId > 0) {
      nodeJobIdMap[key] = jobId
    }
    const childRunId = Number(item?.child_run_id)
    if (Number.isInteger(childRunId) && childRunId > 0) {
      nodeChildRunIdMap[key] = childRunId
    }
    const durationSeconds = Number(item?.duration_seconds)
    if (Number.isFinite(durationSeconds) && durationSeconds >= 0) {
      nodeDurationSecondsMap[key] = durationSeconds
    }
    const jobTaskName = String(item?.job_task_name_snapshot || '').trim()
    if (jobTaskName) {
      nodeJobTaskNameMap[key] = jobTaskName
    }
    const jobTemplateName = String(item?.job_template_name_snapshot || '').trim()
    if (jobTemplateName) {
      nodeJobTemplateNameMap[key] = jobTemplateName
    }
    const jobTaskId = Number(item?.job_task_id)
    if (Number.isInteger(jobTaskId) && jobTaskId > 0) {
      nodeJobTaskIdMap[key] = jobTaskId
    }
    const jobTemplateId = Number(item?.job_template_id)
    if (Number.isInteger(jobTemplateId) && jobTemplateId > 0) {
      nodeJobTemplateIdMap[key] = jobTemplateId
    }
  })

  const incomingCount = {}
  workflowNodes.forEach((item) => {
    const key = String(item?.key || '').trim()
    if (key) {
      incomingCount[key] = 0
    }
  })
  workflowEdges.forEach((item) => {
    const targetKey = String(item?.target_key || '').trim()
    if (targetKey && Object.prototype.hasOwnProperty.call(incomingCount, targetKey)) {
      incomingCount[targetKey] += 1
    }
  })

  const nodeOrderMap = {}
  workflowNodes.forEach((item, index) => {
    const key = String(item?.key || '').trim()
    if (key) {
      nodeOrderMap[key] = index
    }
  })

  let rootNodeKeys = Object.keys(incomingCount).filter((key) => Number(incomingCount[key] || 0) === 0)
  if (rootNodeKeys.length === 0 && workflowNodes.length > 0) {
    const fallbackRoot = String(workflowNodes[0]?.key || '').trim()
    rootNodeKeys = fallbackRoot ? [fallbackRoot] : []
  }

  rootNodeKeys = [...rootNodeKeys].sort((a, b) => Number(nodeOrderMap[a] || 0) - Number(nodeOrderMap[b] || 0))

  const runtimeNodes = workflowNodes.map((item, index) => {
    const key = String(item?.key || `node-${index}`)
    const status = nodeStatusMap[key] || 'pending'
    const x = Number(item?.x)
    const y = Number(item?.y)
    const position = {
      x: Number.isFinite(x) ? x : 340 + (index % 4) * 260,
      y: Number.isFinite(y) ? y : 220 + Math.floor(index / 4) * 140,
    }

    return {
      id: key,
      type: 'workflow-node',
      position,
      draggable: false,
      selectable: false,
      connectable: false,
      data: {
        status,
        name: item?.name || key,
        nodeType: String(item?.node_type || '').toLowerCase(),
        nodeTypeLabel: String(item?.node_type || '').toLowerCase() === 'workflow'
          ? 'Workflow节点'
          : '任务节点',
        taskId: Number(item?.task_id) || null,
        taskName: String(item?.task_name || '').trim(),
        runtimeTaskId: nodeJobTaskIdMap[key] || null,
        runtimeTemplateId: nodeJobTemplateIdMap[key] || null,
        runtimeTaskName: String(nodeJobTaskNameMap[key] || '').trim(),
        runtimeTemplateName: String(nodeJobTemplateNameMap[key] || '').trim(),
        workflowId: Number(item?.workflow_id) || null,
        workflowName: String(item?.workflow_name || '').trim(),
        childRunId: nodeChildRunIdMap[key] || null,
        durationSeconds: Object.prototype.hasOwnProperty.call(nodeDurationSecondsMap, key)
          ? nodeDurationSecondsMap[key]
          : null,
        convergence: String(item?.convergence || 'any').toLowerCase() === 'all' ? 'all' : 'any',
        jobId: nodeJobIdMap[key] || null,
        nodeMessage: nodeMessageMap[key] || null,
      },
      style: {
        border: 'none',
        background: 'transparent',
        padding: '0',
        boxShadow: 'none',
      },
    }
  })

  const startNode = {
    id: 'start',
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

  const startEdges = rootNodeKeys.map((targetKey, index) => ({
    id: `start-edge-${index}`,
    source: 'start',
    target: targetKey,
    type: 'smoothstep',
    label: 'always',
    data: { condition: 'always' },
    pathOptions: { borderRadius: 18, offset: 20 },
    markerEnd: MarkerType.ArrowClosed,
    style: { stroke: resolveEdgeColor('always') },
    labelStyle: { fill: '#333', fontSize: '12px' },
  }))

  flowNodes.value = [startNode, ...runtimeNodes]

  const runtimeEdges = workflowEdges.map((item, index) => {
    const condition = normalizeEdgeCondition(item?.condition)
    return {
      id: `edge-${index}`,
      source: String(item?.source_key || ''),
      target: String(item?.target_key || ''),
      label: condition,
      type: 'smoothstep',
      animated: false,
      data: { condition },
      pathOptions: { borderRadius: 18, offset: 20 },
      markerEnd: MarkerType.ArrowClosed,
      style: { stroke: resolveEdgeColor(condition) },
      labelStyle: { fill: '#333', fontSize: '12px' },
    }
  })

  flowEdges.value = [...startEdges, ...runtimeEdges]
}

function openTaskInNewWindowByNodeData(nodeData) {
  const isTaskNode = String(nodeData?.nodeType || '').toLowerCase() === 'task'
  if (!isTaskNode) {
    return false
  }

  const jobId = Number(nodeData?.jobId)
  if (!Number.isInteger(jobId) || jobId <= 0) {
    return false
  }

  const runtimeTaskName = String(nodeData?.runtimeTaskName || '').trim()
  const staticNodeName = String(nodeData?.name || '').trim()
  const resolvedTaskName = runtimeTaskName || staticNodeName

  const target = router.resolve({
    path: '/sys/automation/logs',
    query: {
      job_id: String(jobId),
    },
  })

  // 运行态“详细”打开任务运行记录列表，并按当前运行记录 ID 精确过滤。
  window.open(target.href, '_blank', 'noopener,noreferrer')
  return true
}

function openWorkflowInNewWindowByNodeData(nodeData) {
  const isWorkflowNode = String(nodeData?.nodeType || '').toLowerCase() === 'workflow'
  if (!isWorkflowNode) {
    return false
  }

  const workflowId = Number(nodeData?.workflowId)
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

function suppressNextNodeClick() {
  skipNextNodeClick.value = true
}

function handleTaskDetailAction(nodeData) {
  suppressNextNodeClick()
  return openTaskInNewWindowByNodeData(nodeData)
}

function handleWorkflowDetailAction(nodeData) {
  suppressNextNodeClick()
  const opened = openWorkflowInNewWindowByNodeData(nodeData)
  if (!opened) {
    message.info('当前 Workflow 节点未绑定可打开的 Workflow')
  }
  return opened
}

function canOpenWorkflowRunGraph(nodeData) {
  const isWorkflowNode = String(nodeData?.nodeType || '').toLowerCase() === 'workflow'
  if (!isWorkflowNode) {
    return false
  }
  const childRunId = Number(nodeData?.childRunId)
  return Number.isInteger(childRunId) && childRunId > 0
}

function openWorkflowRunGraphByNodeData(nodeData) {
  if (!canOpenWorkflowRunGraph(nodeData)) {
    return false
  }

  const childRunId = Number(nodeData?.childRunId)
  const target = router.resolve({
    path: '/sys/automation/workflow/run',
    query: {
      run_id: String(childRunId),
    },
  })
  window.open(target.href, '_blank', 'noopener,noreferrer')
  return true
}

function handleWorkflowRunGraphAction(nodeData) {
  suppressNextNodeClick()
  const opened = openWorkflowRunGraphByNodeData(nodeData)
  if (!opened) {
    message.info('当前 Workflow 节点暂无可查看的子流程运行图')
  }
  return opened
}

function hasNodeLogs(nodeData) {
  const isTaskNode = String(nodeData?.nodeType || '').toLowerCase() === 'task'
  if (!isTaskNode) {
    return false
  }
  // 没有 jobId 说明节点未实际派生任务（如 skipped/递归拦截），不显示日志按钮。
  const jobId = Number(nodeData?.jobId)
  return Number.isInteger(jobId) && jobId > 0
}

function canCancelNodeJob(nodeData) {
  const status = normalizeNodeStatus(nodeData?.status)
  return hasNodeLogs(nodeData) && status === 'running'
}

function openTaskLogsByNodeData(nodeData, nodeId = '') {
  if (!hasNodeLogs(nodeData)) {
    return false
  }

  const taskId = Number(nodeData?.taskId)
  if (!Number.isInteger(taskId) || taskId <= 0) {
    return false
  }

  const query = {
    task_id: String(taskId),
    task_name: String(nodeData?.name || nodeId || ''),
  }

  const jobId = Number(nodeData?.jobId)
  if (Number.isInteger(jobId) && jobId > 0) {
    query.job_id = String(jobId)
  }

  const target = router.resolve({
    path: '/sys/automation/logs',
    query,
  })

  window.open(target.href, '_blank', 'noopener,noreferrer')
  return true
}

function handleTaskLogAction(nodeData, nodeId = '') {
  suppressNextNodeClick()
  return openTaskLogsByNodeData(nodeData, nodeId)
}

async function cancelNodeJob(nodeData) {
  suppressNextNodeClick()
  const jobId = Number(nodeData?.jobId)
  if (!Number.isInteger(jobId) || jobId <= 0) {
    message.info('当前节点没有可取消的运行任务')
    return
  }

  cancelingNodeJobId.value = jobId
  try {
    await cancelJob(jobId)
    message.success('任务已取消')
    await loadRunDetail(true)
    schedulePolling()
  } finally {
    cancelingNodeJobId.value = null
  }
}

function handleNodeClick(event) {
  if (skipNextNodeClick.value) {
    skipNextNodeClick.value = false
    return
  }

  // 运行页点击节点卡片不触发跳转，所有跳转都必须通过显式按钮触发。
  return
}

async function loadRunDetail(showError = false) {
  if (!runId.value) {
    return
  }
  runLoading.value = true
  try {
    const res = await getWorkflowRunDetail(runId.value)
    const data = res?.data?.data || null
    runDetail.value = data
    buildGraph(data)
  } catch (error) {
    if (showError) {
      message.error(error?.message || '加载运行状态失败')
    }
  } finally {
    runLoading.value = false
  }
}

function isFinalRunStatus() {
  return ['success', 'failed', 'cancelled'].includes(runStatus.value)
}

function getPollDelayMs() {
  const hidden = typeof document !== 'undefined' && document.hidden
  let baseDelay = 5000

  if (runStatus.value === 'running') {
    baseDelay = 2000
  } else if (runStatus.value === 'pending') {
    baseDelay = 3000
  }

  return hidden ? Math.max(baseDelay, 8000) : baseDelay
}

function clearPollTimer() {
  if (pollTimer) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
}

function schedulePolling() {
  clearPollTimer()
  if (!runId.value || isFinalRunStatus()) {
    return
  }

  pollTimer = window.setTimeout(async () => {
    await loadRunDetail(false)
    schedulePolling()
  }, getPollDelayMs())
}

async function reloadNow() {
  await loadRunDetail(true)
  schedulePolling()
}

async function cancelCurrentRun() {
  if (!runId.value) {
    return
  }

  cancelingRun.value = true
  try {
    await cancelWorkflowRun(runId.value)
    message.success('Workflow 运行已取消')
    await loadRunDetail(true)
    schedulePolling()
  } finally {
    cancelingRun.value = false
  }
}

function handleVisibilityChange() {
  if (!runId.value || isFinalRunStatus()) {
    return
  }
  schedulePolling()
}

function goBack() {
  router.push('/sys/automation/workflow')
}

onMounted(async () => {
  if (!runId.value) {
    message.warning('缺少运行ID，请从 Workflow 页面进入')
    goBack()
    return
  }
  document.addEventListener('visibilitychange', handleVisibilityChange)
  await loadRunDetail(true)
  schedulePolling()
})

onBeforeUnmount(() => {
  clearPollTimer()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<style scoped>
.workflow-editor-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.editor-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: space-between;
  flex-wrap: wrap;
  padding: 8px 12px;
  border: 1px solid #e8e8e8;
  border-radius: 8px;
  background: #fff;
}

.editor-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.editor-title .name {
  font-size: 16px;
  font-weight: 600;
}

.run-summary-alert {
  margin-bottom: 0;
}

.run-count-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.canvas-wrap {
  border: 1px solid #e8e8e8;
  border-radius: 10px;
  overflow: hidden;
}

.workflow-canvas {
  height: calc(100vh - 280px);
  min-height: 620px;
  background: #fafafa;
}

.workflow-node-wrap {
  width: 220px;
}

.workflow-start-node-card {
  position: relative;
  min-width: 112px;
  min-height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  border: 1px solid #1677ff;
  background: #1677ff;
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 1px;
}

.workflow-node-card {
  position: relative;
  width: 220px;
  height: 86px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 10px 14px;
  border: 1px solid #6f6f6f;
  border-radius: 4px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
}

.workflow-node-state-icon {
  position: absolute;
  left: -10px;
  top: -10px;
  min-width: 22px;
  height: 22px;
  padding: 0 4px;
  border-radius: 999px;
  border: 2px solid #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.18);
  z-index: 2;
  letter-spacing: 0.2px;
}

.workflow-node-state-icon.icon-pending {
  background: #d9d9d9;
  color: #434343;
}

.workflow-node-state-icon.icon-queued {
  background: #91caff;
  color: #003a8c;
}

.workflow-node-state-icon.icon-waiting {
  background: #faad14;
  color: #fff;
}

.workflow-node-state-icon.icon-running {
  background: #1677ff;
  color: #fff;
  animation: running-pulse 1.2s ease-in-out infinite;
}

.workflow-node-state-icon.icon-success {
  background: #52c41a;
  color: #fff;
}

.workflow-node-state-icon.icon-skipped {
  background: #8c8c8c;
  color: #fff;
}

.workflow-node-state-icon.icon-failed,
.workflow-node-state-icon.icon-cancelled {
  background: #faad14;
  color: #fff;
}

.workflow-node-state-icon.icon-failed {
  background: #ff4d4f;
  color: #fff;
}

.workflow-node-card.status-pending,
.workflow-node-card.status-queued {
  border-color: #d9d9d9;
  background: #fafafa;
}

.workflow-node-card.status-waiting {
  border-color: #faad14;
  background: #fffbe6;
}

.workflow-node-card.status-running {
  border-color: #1677ff;
  background: #e6f7ff;
}

.workflow-node-card.status-success {
  border-color: #52c41a;
  background: #f6ffed;
}

.workflow-node-card.status-skipped {
  border-color: #bfbfbf;
  background: #f5f5f5;
}

.workflow-node-card.status-cancelled {
  border-color: #faad14;
  background: #fff7e6;
}

.workflow-node-card.status-failed {
  border-color: #ff4d4f;
  background: #fff2f0;
}

.workflow-node-name {
  margin-top: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 16px;
  color: #1f1f1f;
  line-height: 1.2;
  white-space: nowrap;
  cursor: pointer;
}

.workflow-node-name-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: middle;
}

.workflow-node-duration {
  flex: none;
  display: inline-block;
  padding: 0 6px;
  border-radius: 999px;
  background: #f0f5ff;
  color: #1677ff;
  font-size: 12px;
  line-height: 20px;
  vertical-align: middle;
}

.workflow-node-type-icon {
  position: absolute;
  left: 14px;
  bottom: 8px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  border: 1px solid transparent;
}

.workflow-node-type-icon.is-task {
  color: #1677ff;
  background: #e6f4ff;
  border-color: #91caff;
}

.workflow-node-type-icon.is-workflow {
  color: #722ed1;
  background: #f9f0ff;
  border-color: #d3adf7;
}

.workflow-node-convergence-tag {
  position: absolute;
  right: 10px;
  bottom: 28px;
  padding: 1px 6px;
  border-radius: 10px;
  border: 1px solid #faad14;
  background: #fffbe6;
  color: #ad6800;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.2px;
}

.workflow-node-quick-actions {
  position: absolute;
  right: 10px;
  bottom: 6px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 224px;
}

.node-quick-ref-name {
  display: inline-block;
  max-width: 82px;
  color: #8c8c8c;
  font-size: 11px;
  line-height: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-quick-btn {
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  background: #fff;
  color: #595959;
  font-size: 11px;
  line-height: 1;
  padding: 3px 6px;
  cursor: pointer;
}

.node-quick-btn:hover {
  border-color: #1677ff;
  color: #1677ff;
  background: #f0f7ff;
}

.node-quick-btn.node-quick-btn-danger {
  border-color: #ffccc7;
  color: #cf1322;
  background: #fff1f0;
}

.node-quick-btn.node-quick-btn-danger:hover {
  border-color: #ff4d4f;
  color: #a8071a;
  background: #fff1f0;
}

.workflow-node-runtime {
  position: absolute;
  right: 10px;
  top: 8px;
  font-size: 12px;
  color: #595959;
}

@keyframes running-pulse {
  0% {
    transform: scale(1);
    box-shadow: 0 1px 4px rgba(22, 119, 255, 0.35);
  }
  50% {
    transform: scale(1.08);
    box-shadow: 0 1px 8px rgba(22, 119, 255, 0.45);
  }
  100% {
    transform: scale(1);
    box-shadow: 0 1px 4px rgba(22, 119, 255, 0.35);
  }
}

@media (max-width: 1200px) {
  .workflow-canvas {
    height: calc(100vh - 300px);
    min-height: 560px;
  }
}

@media (max-width: 992px) {
  .editor-toolbar {
    align-items: flex-start;
  }

  .workflow-canvas {
    min-height: 500px;
  }
}
</style>
