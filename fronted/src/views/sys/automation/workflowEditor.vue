<template>
  <div class="workflow-editor-page">
    <div class="editor-toolbar">
      <div class="editor-title">
        <span class="name">{{ form.name || 'Workflow 编排' }}</span>
        <a-tag :color="form.enabled ? 'green' : 'default'">{{ form.enabled ? '启用' : '禁用' }}</a-tag>
      </div>
      <a-space>
        <a-button @click="goBack">返回列表</a-button>
        <a-button @click="basicInfoVisible = true">基础信息</a-button>
        <a-button @click="openAddNodeWizard({ presetNodeType: 'task', forceRoot: true })">添加任务节点</a-button>
        <a-button type="primary" :loading="submitting" @click="saveWorkflow">保存</a-button>
      </a-space>
    </div>

    <a-skeleton active :loading="pageLoading">
      <div ref="canvasWrapRef" class="canvas-wrap">
        <VueFlow
          v-model:nodes="flowNodes"
          v-model:edges="flowEdges"
          :fit-view-on-init="true"
          :nodes-draggable="true"
          :nodes-connectable="true"
          :elements-selectable="true"
          :connection-mode="ConnectionMode.Strict"
          :class="['workflow-canvas', { 'is-connecting': isConnecting }]"
          @connect="handleConnect"
          @connect-start="handleConnectStart"
          @connect-end="handleConnectEnd"
          @node-click="handleNodeClick"
          @edge-click="handleEdgeClick"
          @pane-click="clearSelection"
        >
          <template #node-workflow-start-node>
            <div class="workflow-start-node-card">
              <Handle
                type="source"
                :position="Position.Right"
                :connectable="false"
                :connectable-start="false"
                :connectable-end="false"
              />
              START
            </div>
          </template>
          <template #node-workflow-node="nodeProps">
            <div class="workflow-node-wrap">
              <div class="workflow-node-card" @click.stop="handleNodeCardClick(nodeProps.id)">
                <Handle
                  type="target"
                  :position="Position.Left"
                  class="workflow-target-handle"
                  :connectable-start="false"
                  :connectable-end="true"
                />
                <Handle
                  type="source"
                  :position="Position.Right"
                  class="workflow-source-handle"
                  :connectable-start="true"
                  :connectable-end="false"
                />

                <div
                  class="workflow-node-type-icon"
                  :class="nodeProps.data?.node_type === 'workflow' ? 'is-workflow' : 'is-task'"
                  :title="nodeProps.data?.node_type === 'workflow' ? 'Workflow 节点' : '任务节点'"
                >
                  <ApartmentOutlined v-if="nodeProps.data?.node_type === 'workflow'" />
                  <ToolOutlined v-else />
                </div>
                <div
                  v-if="String(nodeProps.data?.convergence || 'any').toLowerCase() === 'all'"
                  class="workflow-node-convergence-tag"
                >
                  ALL
                </div>

                <div v-if="nodeProps.data?.node_type === 'task'" class="workflow-node-quick-actions">
                  <span class="node-quick-ref-name" :title="resolveTaskNameByNodeData(nodeProps.data)">
                    {{ resolveTaskNameByNodeData(nodeProps.data) }}
                  </span>
                  <button
                    type="button"
                    class="node-quick-btn"
                    :title="resolveTaskNameByNodeData(nodeProps.data)"
                    @click.stop="openTaskDetailByNodeId(nodeProps.id)"
                  >
                    详细
                  </button>
                </div>
                <div v-else-if="nodeProps.data?.node_type === 'workflow'" class="workflow-node-quick-actions">
                  <span class="node-quick-ref-name" :title="resolveWorkflowNameByNodeData(nodeProps.data)">
                    {{ resolveWorkflowNameByNodeData(nodeProps.data) }}
                  </span>
                  <button
                    type="button"
                    class="node-quick-btn"
                    :title="resolveWorkflowNameByNodeData(nodeProps.data)"
                    @click.stop="openWorkflowDetailByNodeId(nodeProps.id)"
                  >
                    详细
                  </button>
                </div>

                <div class="workflow-node-tools" :class="{ visible: selectedNodeId === nodeProps.id }">
                  <button type="button" class="tool-btn" title="添加后续节点" @click.stop="openAddNodeWizard({ parentNodeId: nodeProps.id })">+</button>
                  <button type="button" class="tool-btn" title="编辑节点配置" @click.stop="openNodeEditDialog(nodeProps.id)">✎</button>
                  <button type="button" class="tool-btn danger" title="删除节点" @click.stop="removeNode(nodeProps.id)">🗑</button>
                </div>
              </div>
              <div class="workflow-node-name" @click.stop="handleNodeCardClick(nodeProps.id)">{{ nodeProps.data?.name || nodeProps.id }}</div>
            </div>
          </template>
          <Background pattern-color="#d9d9d9" :gap="18" />
        </VueFlow>

        <div
          v-if="selectedEditableEdge"
          class="edge-quick-editor"
          :style="edgeQuickEditorStyle"
        >
          <button type="button" class="edge-action-item" @click="setSelectedEdgeCondition('success')">
            <span class="edge-action-dot dot-success">✓</span>
            <span class="edge-action-label">Run on success</span>
            <span v-if="selectedEdgeCondition === 'success'" class="edge-action-check">✓</span>
          </button>
          <button type="button" class="edge-action-item" @click="setSelectedEdgeCondition('always')">
            <span class="edge-action-dot dot-always"></span>
            <span class="edge-action-label">Run always</span>
            <span v-if="selectedEdgeCondition === 'always'" class="edge-action-check">✓</span>
          </button>
          <button type="button" class="edge-action-item" @click="setSelectedEdgeCondition('failure')">
            <span class="edge-action-dot dot-failure">!</span>
            <span class="edge-action-label">Run on fail</span>
            <span v-if="selectedEdgeCondition === 'failure'" class="edge-action-check">✓</span>
          </button>
          <div class="edge-action-divider"></div>
          <button type="button" class="edge-action-item edge-action-delete" @click="removeSelectedEdge">
            <span class="edge-action-dot dot-delete">🗑</span>
            <span class="edge-action-label">Remove link</span>
          </button>
        </div>
      </div>
    </a-skeleton>

    <a-modal
      title="Add Node"
      :open="addNodeWizardVisible"
      :mask-closable="false"
      :closable="true"
      ok-text="创建节点"
      cancel-text="Cancel"
      @ok="createNodeFromWizard"
      @cancel="closeAddNodeWizard"
    >
      <div class="add-node-wizard-layout">
        <div class="add-node-wizard-content">
          <div v-if="addNodeWizardHasParentCondition" class="wizard-section">
            <div class="run-type-title">运行条件</div>
            <div class="run-type-desc">设置新节点在父节点什么状态下执行。</div>
            <a-alert
              v-if="!addNodeWizardForm.parent_node_key"
              type="info"
              show-icon
              class="run-type-alert"
              message="当前是首个节点，无父节点，执行条件固定为 success。"
            />
            <div class="run-type-grid">
              <div
                class="run-type-card"
                :class="{ active: addNodeWizardForm.run_type === 'success', disabled: !addNodeWizardForm.parent_node_key }"
                @click="selectRunType('success')"
              >
                <div class="card-title">On Success</div>
                <div class="card-desc">Execute when the parent node results in a successful state.</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: addNodeWizardForm.run_type === 'failure', disabled: !addNodeWizardForm.parent_node_key }"
                @click="selectRunType('failure')"
              >
                <div class="card-title">On Failure</div>
                <div class="card-desc">Execute when the parent node results in a failed state.</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: addNodeWizardForm.run_type === 'always', disabled: !addNodeWizardForm.parent_node_key }"
                @click="selectRunType('always')"
              >
                <div class="card-title">Always</div>
                <div class="card-desc">Execute regardless of the parent node's final state.</div>
              </div>
            </div>
          </div>

          <div class="wizard-section">
            <a-form layout="vertical">
              <a-form-item label="节点类型" required>
                <a-radio-group v-model:value="addNodeWizardForm.node_type">
                  <a-radio-button value="task">任务节点</a-radio-button>
                  <a-radio-button value="workflow">Workflow节点</a-radio-button>
                </a-radio-group>
              </a-form-item>

              <a-form-item label="节点名称" required>
                <a-input v-model:value="addNodeWizardForm.name" placeholder="例如：任务节点" />
              </a-form-item>

              <a-form-item v-if="addNodeWizardForm.node_type === 'task'" label="执行任务" required>
                <a-select
                  v-model:value="addNodeWizardForm.task_id"
                  :options="taskOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择任务"
                />
                <div v-if="taskOptions.length === 0" class="wizard-help-link">
                  当前暂无可选任务，请先去
                  <a @click="goToTaskPage">任务管理</a>
                  创建任务。
                </div>
              </a-form-item>

              <a-form-item v-else label="选择Workflow" required>
                <a-select
                  v-model:value="addNodeWizardForm.workflow_id"
                  :options="workflowOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择Workflow"
                />
              </a-form-item>

              <a-form-item label="Convergence">
                <a-select v-model:value="addNodeWizardForm.convergence" :options="convergenceOptions" />
              </a-form-item>

            </a-form>
          </div>
        </div>
      </div>
    </a-modal>

    <a-modal
      title="基础信息"
      :open="basicInfoVisible"
      ok-text="确认"
      cancel-text="取消"
      @ok="basicInfoVisible = false"
      @cancel="basicInfoVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="名称" required>
          <a-input v-model:value="form.name" placeholder="例如：生产发布编排" />
        </a-form-item>
        <a-form-item label="启用状态">
          <a-switch v-model:checked="form.enabled" checked-children="启用" un-checked-children="禁用" />
        </a-form-item>
        <a-form-item label="描述">
          <a-input v-model:value="form.description" placeholder="可选" />
        </a-form-item>
        <a-form-item label="默认变量 JSON">
          <a-textarea v-model:value="form.default_extra_vars_text" :rows="3" placeholder='例如：{"env":"prod"}' />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="节点配置"
      :open="nodeConfigVisible"
      ok-text="保存"
      cancel-text="取消"
      @ok="saveNodeConfig"
      @cancel="nodeConfigVisible = false"
    >
      <div class="add-node-wizard-layout">
        <div class="add-node-wizard-content">
          <div v-if="nodeConfigHasParentCondition" class="wizard-section">
            <div class="run-type-title">运行条件</div>
            <div class="run-type-desc">设置当前节点在父节点什么状态下执行。</div>
            <a-alert
              v-if="!nodeConfigRunTypeEditable"
              type="info"
              show-icon
              class="run-type-alert"
              :message="nodeConfigRunTypeHint"
            />
            <div class="run-type-grid">
              <div
                class="run-type-card"
                :class="{ active: nodeConfigForm.run_type === 'success', disabled: !nodeConfigRunTypeEditable }"
                @click="selectNodeConfigRunType('success')"
              >
                <div class="card-title">On Success</div>
                <div class="card-desc">Execute when the parent node results in a successful state.</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: nodeConfigForm.run_type === 'failure', disabled: !nodeConfigRunTypeEditable }"
                @click="selectNodeConfigRunType('failure')"
              >
                <div class="card-title">On Failure</div>
                <div class="card-desc">Execute when the parent node results in a failed state.</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: nodeConfigForm.run_type === 'always', disabled: !nodeConfigRunTypeEditable }"
                @click="selectNodeConfigRunType('always')"
              >
                <div class="card-title">Always</div>
                <div class="card-desc">Execute regardless of the parent node's final state.</div>
              </div>
            </div>
          </div>

          <div class="wizard-section">
            <a-form layout="vertical">
              <a-form-item label="节点类型" required>
                <a-radio-group v-model:value="nodeConfigForm.node_type">
                  <a-radio-button value="task">任务节点</a-radio-button>
                  <a-radio-button value="workflow">Workflow节点</a-radio-button>
                </a-radio-group>
              </a-form-item>

              <a-form-item label="节点名称" required>
                <a-input v-model:value="nodeConfigForm.name" />
              </a-form-item>

              <a-form-item v-if="nodeConfigForm.node_type === 'task'" label="执行任务" required>
                <a-select
                  v-model:value="nodeConfigForm.task_id"
                  :options="taskOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择任务"
                />
              </a-form-item>

              <a-form-item v-else label="选择Workflow" required>
                <a-select
                  v-model:value="nodeConfigForm.workflow_id"
                  :options="workflowOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择Workflow"
                />
              </a-form-item>

              <a-form-item label="Convergence">
                <a-select v-model:value="nodeConfigForm.convergence" :options="convergenceOptions" />
              </a-form-item>

            </a-form>
          </div>
        </div>
      </div>
    </a-modal>

  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Modal, message } from 'ant-design-vue'
import { ApartmentOutlined, ToolOutlined } from '@ant-design/icons-vue'
import { VueFlow, Handle, Position, MarkerType, ConnectionMode } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import {
  getTaskList,
  getWorkflowList,
  getWorkflowDetail,
  createWorkflow,
  updateWorkflow,
} from '@/api/sys/automation'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'

const START_NODE_ID = 'start'
const START_EDGE_PREFIX = 'start-edge-'
const WORKFLOW_EDGE_TYPE = 'smoothstep'
const WORKFLOW_EDGE_PATH_OPTIONS = { borderRadius: 18, offset: 20 }
const TASK_DEFAULT_NODE_NAME = '任务节点'
const WORKFLOW_DEFAULT_NODE_NAME = 'Workflow节点'

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

function buildWorkflowEdgeStyle(condition = 'success') {
  return { stroke: resolveEdgeColor(condition) }
}

function buildWorkflowEdgeLabelStyle() {
  return { fill: '#333', fontSize: '12px' }
}

const route = useRoute()
const router = useRouter()

const pageLoading = ref(false)
const submitting = ref(false)
const isCreateMode = ref(true)
const editingId = ref(null)

const taskRecords = ref([])
const workflowRecords = ref([])

const form = reactive({
  name: '',
  description: '',
  enabled: true,
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
const selectedNode = computed(() => flowNodes.value.find((item) => item.id === selectedNodeId.value))
const selectedEdge = computed(() => flowEdges.value.find((item) => item.id === selectedEdgeId.value))
const selectedEditableEdge = computed(() => {
  const target = selectedEdge.value
  if (!target || isSystemEdge(target)) {
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

const nodeConfigForm = reactive({
  id: '',
  name: '',
  node_type: 'task',
  task_id: undefined,
  workflow_id: undefined,
  convergence: 'any',
  run_type: 'success',
})

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

function createStartNode() {
  return {
    id: START_NODE_ID,
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

function formatNodeLabel(data) {
  const typeLabel = data.node_type === 'workflow' ? 'Workflow' : '任务'
  return `${data.name || '未命名节点'} (${typeLabel})`
}

function makeNodeFromConfig(config, index = 0) {
  const x = Number(config.x)
  const y = Number(config.y)
  const position = {
    x: Number.isFinite(x) ? x : 340 + (index % 4) * 260,
    y: Number.isFinite(y) ? y : 220 + Math.floor(index / 4) * 140,
  }
  const data = {
    key: config.key,
    name: config.name,
    node_type: String(config.node_type || 'task').toLowerCase() === 'workflow' ? 'workflow' : 'task',
    task_id: config.task_id,
    workflow_id: config.workflow_id,
    convergence: String(config.convergence || 'any').toLowerCase() === 'all' ? 'all' : 'any',
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
  const taskId = Number(nodeData?.task_id)
  if (!Number.isInteger(taskId) || taskId <= 0) {
    return '未选择任务'
  }
  return taskNameMap.value.get(taskId) || `任务#${taskId}`
}

function resolveWorkflowNameByNodeData(nodeData) {
  const workflowId = Number(nodeData?.workflow_id)
  if (!Number.isInteger(workflowId) || workflowId <= 0) {
    return '未选择Workflow'
  }
  return workflowNameMap.value.get(workflowId) || `Workflow#${workflowId}`
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
    (item) => !isSystemEdge(item) && item.target === nodeId,
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
    if (incoming && !isSystemEdge(incoming)) {
      const condition = normalizeEdgeCondition(nodeConfigForm.run_type)
      incoming.data = { ...(incoming.data || {}), condition }
      incoming.label = condition
      incoming.style = buildWorkflowEdgeStyle(condition)
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
  return {
    id: `edge-${config.source_key}-${config.target_key}-${condition}-${index}`,
    type: WORKFLOW_EDGE_TYPE,
    source: config.source_key,
    target: config.target_key,
    label: condition,
    data: { condition },
    pathOptions: WORKFLOW_EDGE_PATH_OPTIONS,
    markerEnd: MarkerType.ArrowClosed,
    style: buildWorkflowEdgeStyle(condition),
    labelStyle: buildWorkflowEdgeLabelStyle(),
  }
}

function isSystemEdge(edge) {
  if (!edge || typeof edge !== 'object') {
    return false
  }
  return edge.source === START_NODE_ID || String(edge.id || '').startsWith(START_EDGE_PREFIX)
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
  const runtimeNodes = flowNodes.value.filter((item) => item.id !== START_NODE_ID)
  const runtimeNodeIds = new Set(runtimeNodes.map((item) => item.id))

  const incomingCount = {}
  runtimeNodes.forEach((item) => {
    incomingCount[item.id] = 0
  })

  flowEdges.value.forEach((item) => {
    if (!item || item.source === START_NODE_ID) {
      return
    }
    if (runtimeNodeIds.has(item.target)) {
      incomingCount[item.target] = (incomingCount[item.target] || 0) + 1
    }
  })

  const rootKeys = runtimeNodes.filter((item) => (incomingCount[item.id] || 0) === 0).map((item) => item.id)

  flowEdges.value = flowEdges.value.filter((item) => item.source !== START_NODE_ID && !String(item.id || '').startsWith(START_EDGE_PREFIX))
  rootKeys.forEach((nodeKey) => {
    flowEdges.value.push({
      id: `${START_EDGE_PREFIX}${nodeKey}`,
      type: WORKFLOW_EDGE_TYPE,
      source: START_NODE_ID,
      target: nodeKey,
      label: 'always',
      data: { condition: 'always' },
      pathOptions: WORKFLOW_EDGE_PATH_OPTIONS,
      markerEnd: MarkerType.ArrowClosed,
      style: buildWorkflowEdgeStyle('always'),
      labelStyle: buildWorkflowEdgeLabelStyle(),
      selectable: false,
      updatable: false,
      focusable: false,
    })
  })
}

function autoLayoutTree() {
  // Workflow 编排按树形布局：从左到右展开，同一层节点按从上到下排序。
  const START_X = 60
  const START_Y = 220
  const START_NODE_HEIGHT = 64
  const RUNTIME_NODE_HEIGHT = 72
  const RUNTIME_BASE_Y = START_Y + START_NODE_HEIGHT / 2 - RUNTIME_NODE_HEIGHT / 2
  const LAYER_X_GAP = 280
  const ROW_Y_GAP = 120

  const runtimeNodes = flowNodes.value.filter((item) => item.id !== START_NODE_ID)
  if (runtimeNodes.length === 0) {
    return
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

  // 自动布局只基于业务边，忽略 START 系统边。
  const businessEdges = flowEdges.value.filter((edge) => !isSystemEdge(edge))
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
        x: START_X + depth * LAYER_X_GAP,
        y: count === 1 ? RUNTIME_BASE_Y : yStart + index * ROW_Y_GAP,
      }
      placedY[key] = node.position.y
    })
  })

  flowNodes.value = [createStartNode(), ...runtimeNodes]
}

function resetBuilderGraph() {
  flowNodes.value = [createStartNode()]
  flowEdges.value = []
  selectedNodeId.value = ''
  selectedEdgeId.value = ''
}

function resetForm() {
  editingId.value = null
  form.name = ''
  form.description = ''
  form.enabled = true
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
  form.default_extra_vars_text = JSON.stringify(record.default_extra_vars || {}, null, 2)
  form.remark = record.remark || ''

  const graphNodes = Array.isArray(record.nodes)
    ? record.nodes.map((item, index) => makeNodeFromConfig(item, index))
    : []
  const graphEdges = Array.isArray(record.edges)
    ? record.edges.map((item, index) => makeEdgeFromConfig(item, index))
    : []

  flowNodes.value = [createStartNode(), ...graphNodes]
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
    const duplicate = flowEdges.value.some(
      (item) => item.source === parentNodeKey && item.target === node.id,
    )
    if (!duplicate) {
      flowEdges.value.push({
        id: `edge-${parentNodeKey}-${node.id}-${Date.now()}`,
        type: WORKFLOW_EDGE_TYPE,
        source: parentNodeKey,
        target: node.id,
        label: condition,
        data: { condition },
        pathOptions: WORKFLOW_EDGE_PATH_OPTIONS,
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

function collectCascadeNodeIds(rootNodeId) {
  const collected = new Set()
  const queue = [rootNodeId]

  while (queue.length > 0) {
    const currentNodeId = String(queue.shift() || '').trim()
    if (!currentNodeId || currentNodeId === START_NODE_ID || collected.has(currentNodeId)) {
      continue
    }

    collected.add(currentNodeId)
    flowEdges.value.forEach((edge) => {
      if (isSystemEdge(edge)) {
        return
      }
      if (String(edge.source || '') !== currentNodeId) {
        return
      }
      const targetNodeId = String(edge.target || '').trim()
      if (targetNodeId && targetNodeId !== START_NODE_ID && !collected.has(targetNodeId)) {
        queue.push(targetNodeId)
      }
    })
  }

  return collected
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

  const cascadeNodeIds = collectCascadeNodeIds(nodeId)
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
  if (!target || isSystemEdge(target)) {
    return
  }
  const normalized = normalizeEdgeCondition(condition)
  target.data = { ...(target.data || {}), condition: normalized }
  target.label = normalized
  target.style = buildWorkflowEdgeStyle(normalized)
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
    type: WORKFLOW_EDGE_TYPE,
    source,
    target,
    label: 'success',
    data: { condition: 'success' },
    pathOptions: WORKFLOW_EDGE_PATH_OPTIONS,
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
  if (!targetEdge || isSystemEdge(targetEdge)) {
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
  if (runtimeNodes.length === 0) {
    throw new Error('请至少添加一个节点')
  }

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

async function loadWorkflowDetail(id) {
  const res = await getWorkflowDetail(id)
  const data = res?.data?.data || {}
  fillForm(data)
}

async function initEditor() {
  pageLoading.value = true
  try {
    await loadTaskOptions()
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
    return
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
  } finally {
    submitting.value = false
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
  justify-content: space-between;
  gap: 12px;
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

.canvas-wrap {
  position: relative;
  border: 1px solid #e8e8e8;
  border-radius: 8px;
  overflow: hidden;
}

.edge-quick-editor {
  position: absolute;
  width: 190px;
  padding: 6px 0;
  border: 1px solid #d9d9d9;
  border-radius: 2px;
  background: #fff;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
  z-index: 9;
}

.edge-action-item {
  width: 100%;
  border: none;
  background: transparent;
  cursor: pointer;
  padding: 10px 14px;
  display: flex;
  align-items: center;
  gap: 10px;
  color: #262626;
  font-size: 15px;
  text-align: left;
}

.edge-action-item:hover {
  background: #fafafa;
}

.edge-action-dot {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  color: #fff;
  flex-shrink: 0;
}

.dot-success {
  background: #389e0d;
}

.dot-always {
  background: #1890ff;
}

.dot-failure {
  background: #cf1322;
}

.dot-delete {
  background: transparent;
  color: #ff4d4f;
  width: auto;
  height: auto;
}

.edge-action-label {
  flex: 1;
}

.edge-action-check {
  color: #389e0d;
  font-size: 14px;
}

.edge-action-divider {
  height: 1px;
  margin: 6px 0;
  background: #f0f0f0;
}

.edge-action-delete {
  color: #ff4d4f;
}

.workflow-canvas {
  height: calc(100vh - 220px);
  min-height: 620px;
  background: #fafafa;
}

.workflow-node-wrap {
  width: 220px;
}

.workflow-node-card {
  position: relative;
  width: 220px;
  height: 72px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 10px 14px;
  border: 1px solid #6f6f6f;
  border-radius: 4px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
}

:deep(.workflow-node-card .workflow-target-handle) {
  left: 2px;
  top: 2px;
  width: calc(100% - 26px);
  height: calc(100% - 4px);
  transform: none;
  border: none;
  border-radius: 4px;
  background: transparent;
  opacity: 0;
  z-index: 1;
  pointer-events: none;
}

:deep(.workflow-canvas.is-connecting .workflow-node-card .workflow-target-handle) {
  pointer-events: auto;
}

:deep(.workflow-node-card .workflow-source-handle) {
  right: -7px;
  top: 50%;
  width: 12px;
  height: 12px;
  transform: translateY(-50%);
  border: 1px solid #1677ff;
  border-radius: 50%;
  background: #fff;
  cursor: crosshair;
  opacity: 1;
  z-index: 3;
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

.workflow-node-name {
  margin-top: 6px;
  font-size: 16px;
  color: #1f1f1f;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
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
  max-width: 174px;
}

.node-quick-ref-name {
  display: inline-block;
  max-width: 104px;
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

.workflow-node-tools {
  position: absolute;
  right: -52px;
  top: 50%;
  transform: translateY(-50%);
  width: 44px;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  background: #fff;
  display: none;
  flex-direction: column;
  overflow: hidden;
}

.workflow-node-tools.visible {
  display: flex;
}

.tool-btn {
  border: none;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
  cursor: pointer;
  padding: 7px 0;
  font-size: 16px;
  line-height: 1;
}

.tool-btn:last-child {
  border-bottom: none;
}

.tool-btn:hover {
  background: #f5f5f5;
}

.tool-btn.danger:hover {
  background: #fff1f0;
}

.add-node-wizard-layout {
  display: block;
}

.add-node-wizard-content {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.wizard-section {
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  padding: 12px;
}

.run-type-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 4px;
}

.run-type-desc {
  color: #8c8c8c;
  font-size: 12px;
  margin-bottom: 10px;
}

.run-type-alert {
  margin-bottom: 10px;
}

.run-type-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.run-type-card {
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  padding: 10px;
  cursor: pointer;
  background: #fff;
  transition: all 0.2s;
}

.run-type-card .card-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 4px;
}

.run-type-card .card-desc {
  font-size: 12px;
  color: #8c8c8c;
  line-height: 1.4;
}

.run-type-card.active {
  border-color: #1677ff;
  box-shadow: inset 0 0 0 1px #1677ff;
}

.run-type-card.disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.wizard-help-link {
  margin-top: 6px;
  color: #8c8c8c;
  font-size: 12px;
}

@media (max-width: 1200px) {
  .workflow-canvas {
    height: 500px;
  }
}

@media (max-width: 992px) {
  .editor-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
