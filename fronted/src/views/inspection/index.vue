<template>
  <div class="inspection-page">
    <header class="page-header">
      <div>
        <h1>巡检中心</h1>
        <p>按逻辑服务组织检查，实例巡检由 Agent 并发执行。</p>
      </div>
      <div class="summary-strip">
        <div><strong>{{ groups.length }}</strong><span>巡检组</span></div>
        <div><strong>{{ tasks.length }}</strong><span>巡检任务</span></div>
        <div><strong>{{ runningCount }}</strong><span>执行中</span></div>
      </div>
    </header>

    <a-tabs v-model:activeKey="activeTab" class="workspace-tabs" @change="handleTabChange">
      <a-tab-pane key="tasks" tab="巡检任务">
        <div class="toolbar">
          <a-button size="large" @click="openTaskModal()">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增任务</span>
          </a-button>
          <a-button size="large" @click="loadTasks">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="taskColumns"
          :data-source="tasks"
          :loading="taskLoading"
          :pagination="false"
          :scroll="{ x: 1050 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'scope'">
              <a-tag :color="record.scope === 'per_deployment' ? 'blue' : 'cyan'">
                {{ record.target_type === 'host_group' ? '每台主机' : scopeLabel(record.scope) }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'target'">
              <a-space>
                <a-tag :color="record.target_type === 'host_group' ? 'gold' : 'geekblue'">
                  {{ record.target_type === 'host_group' ? '主机组' : '逻辑服务' }}
                </a-tag>
                <span>{{ record.target_name }}</span>
              </a-space>
            </template>
            <template v-else-if="column.key === 'enabled'">
              <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-tooltip title="运行" placement="top">
                  <a-button type="primary" :loading="runningTaskIds.has(record.id)" @click="runTask(record)">
                    <FontAwesomeIcon :icon="['fas', 'play']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="编辑" placement="top">
                  <a-button @click="openTaskModal(record)">
                    <FontAwesomeIcon :icon="['fas', 'edit']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除" placement="top">
                  <a-button class="delBtn" danger @click="confirmDeleteTask(record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-tab-pane>

      <a-tab-pane key="groups" tab="巡检组">
        <div class="toolbar">
          <a-button size="large" @click="openGroupModal()">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增巡检组</span>
          </a-button>
          <a-button size="large" @click="loadGroups">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="groupColumns"
          :data-source="groups"
          :loading="groupLoading"
          :pagination="false"
          :scroll="{ x: 900 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'scope'">
              <a-tag :color="record.scope === 'per_deployment' ? 'blue' : 'cyan'">{{ scopeLabel(record.scope) }}</a-tag>
            </template>
            <template v-else-if="column.key === 'checks'">
              <a-space wrap>
                <a-tag v-for="check in record.checks" :key="check.id || check.name">
                  {{ check.name }} · {{ executorLabel(check.executor) }}
                </a-tag>
              </a-space>
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-tooltip title="编辑" placement="top">
                  <a-button @click="openGroupModal(record)"><FontAwesomeIcon :icon="['fas', 'edit']" /></a-button>
                </a-tooltip>
                <a-tooltip title="删除" placement="top">
                  <a-button class="delBtn" danger @click="confirmDeleteGroup(record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-tab-pane>

      <a-tab-pane key="executions" tab="执行记录">
        <div class="toolbar">
          <a-button size="large" @click="loadExecutions">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="executionColumns"
          :data-source="executions"
          :loading="executionLoading"
          :pagination="false"
          :scroll="{ x: 1000 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</a-tag>
            </template>
            <template v-else-if="column.key === 'summary'">
              {{ record.summary?.success || 0 }} 成功 / {{ record.summary?.failed || 0 }} 失败
            </template>
            <template v-else-if="column.key === 'create_time'">{{ formatTime(record.create_time) }}</template>
            <template v-else-if="column.key === 'action'">
              <a-tooltip title="详细日志" placement="top">
                <a-button @click="openExecution(record)"><FontAwesomeIcon :icon="['fas', 'list-check']" /></a-button>
              </a-tooltip>
            </template>
          </template>
        </a-table>
      </a-tab-pane>
    </a-tabs>

    <a-modal
      v-model:open="groupModalOpen"
      :title="groupForm.id ? '编辑巡检组' : '新增巡检组'"
      width="760px"
      centered
      :confirm-loading="savingGroup"
      @ok="submitGroup"
    >
      <a-form layout="vertical">
        <div class="form-grid">
          <a-form-item label="巡检组名称" required><a-input v-model:value="groupForm.name" /></a-form-item>
          <a-form-item label="执行范围" required>
            <a-select v-model:value="groupForm.scope" :getPopupContainer="getPopupContainer" @change="resetChecksForScope">
              <a-select-option value="per_deployment">每个部署实例</a-select-option>
              <a-select-option value="controller_once">控制端单次</a-select-option>
            </a-select>
          </a-form-item>
        </div>
        <a-form-item label="描述"><a-textarea v-model:value="groupForm.description" :rows="2" /></a-form-item>
        <div class="check-heading">
          <span>检查项</span>
          <a-button @click="addCheck"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;添加检查项</a-button>
        </div>
        <div v-for="(check, index) in groupForm.checks" :key="check.localKey" class="check-editor">
          <div class="check-editor-head">
            <strong>检查项 {{ index + 1 }}</strong>
            <a-button type="text" danger @click="removeCheck(index)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button>
          </div>
          <div class="form-grid">
            <a-form-item label="名称" required><a-input v-model:value="check.name" /></a-form-item>
            <a-form-item label="执行器" required>
              <a-select v-model:value="check.executor" :getPopupContainer="getPopupContainer">
                <a-select-option v-for="item in executorOptions" :key="item.value" :value="item.value">{{ item.label }}</a-select-option>
              </a-select>
            </a-form-item>
          </div>
          <template v-if="check.executor === 'shell'">
            <a-form-item label="Shell 命令" required><a-textarea v-model:value="check.config.command" :rows="2" /></a-form-item>
            <div class="form-grid">
              <a-form-item label="运行目录"><a-input v-model:value="check.config.work_directory" placeholder="${APP_HOME}" /></a-form-item>
              <a-form-item label="期望输出"><a-input v-model:value="check.config.expected_output" placeholder="留空表示仅校验退出码" /></a-form-item>
            </div>
          </template>
          <template v-else-if="check.executor === 'http'">
            <div class="form-grid">
              <a-form-item label="URL" required><a-input v-model:value="check.config.url" placeholder="https://service/health" /></a-form-item>
              <a-form-item label="期望状态码"><a-input-number v-model:value="check.config.expected_status" :min="100" :max="599" /></a-form-item>
            </div>
          </template>
          <template v-else>
            <div class="form-grid">
              <a-form-item label="主机" required><a-input v-model:value="check.config.host" /></a-form-item>
              <a-form-item label="端口" required><a-input-number v-model:value="check.config.port" :min="1" :max="65535" /></a-form-item>
            </div>
          </template>
        </div>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="taskModalOpen"
      :title="taskForm.id ? '编辑巡检任务' : '新增巡检任务'"
      centered
      :confirm-loading="savingTask"
      @ok="submitTask"
    >
      <a-form layout="vertical">
        <a-form-item label="任务名称" required><a-input v-model:value="taskForm.name" /></a-form-item>
        <a-form-item label="巡检组" required>
          <a-select v-model:value="taskForm.group" :getPopupContainer="getPopupContainer" show-search option-filter-prop="label" @change="handleTaskGroupChange">
            <a-select-option v-for="group in groups" :key="group.id" :value="group.id" :label="group.name">{{ group.name }}</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="目标类型" required>
          <a-segmented v-model:value="taskForm.target_type" :options="targetTypeOptions" block @change="handleTargetTypeChange" />
        </a-form-item>
        <a-form-item v-if="taskForm.target_type === 'logical_service'" label="逻辑服务" required>
          <a-tree-select
            v-model:value="taskForm.logical_service"
            :tree-data="serviceTreeData"
            :getPopupContainer="getPopupContainer"
            :loading="serviceTreeLoading"
            tree-default-expand-all
            tree-node-filter-prop="title"
            show-search
            allow-clear
            placeholder="请选择业务系统 / 环境 / 逻辑服务"
            :dropdown-style="{ maxHeight: '360px', overflow: 'auto' }"
          />
        </a-form-item>
        <a-form-item v-else label="主机组" required>
          <a-tree-select
            v-model:value="taskForm.host_group"
            :tree-data="hostGroupTreeData"
            :getPopupContainer="getPopupContainer"
            :loading="hostGroupTreeLoading"
            tree-default-expand-all
            tree-node-filter-prop="title"
            show-search
            allow-clear
            placeholder="请选择主机组（自动包含所有子组）"
            :dropdown-style="{ maxHeight: '360px', overflow: 'auto' }"
          />
        </a-form-item>
        <div class="form-grid">
          <a-form-item label="并发数"><a-input-number v-model:value="taskForm.concurrency" :min="1" :max="100" /></a-form-item>
          <a-form-item label="单目标超时（秒）"><a-input-number v-model:value="taskForm.timeout_seconds" :min="5" :max="3600" /></a-form-item>
        </div>
      </a-form>
    </a-modal>

    <a-drawer v-model:open="executionDrawerOpen" title="巡检执行详情" width="760">
      <a-descriptions v-if="selectedExecution" bordered size="small" :column="2">
        <a-descriptions-item label="任务">{{ selectedExecution.task_name }}</a-descriptions-item>
        <a-descriptions-item label="状态">{{ statusLabel(selectedExecution.status) }}</a-descriptions-item>
        <a-descriptions-item label="巡检目标">{{ selectedExecution.service_snapshot?.name }}</a-descriptions-item>
        <a-descriptions-item label="开始时间">{{ formatTime(selectedExecution.start_time) }}</a-descriptions-item>
      </a-descriptions>
      <a-collapse v-if="selectedExecution" class="target-results">
        <a-collapse-panel v-for="target in selectedExecution.targets" :key="target.id">
          <template #header>
            <a-space><a-badge :status="target.passed ? 'success' : 'error'" />{{ target.target_name }}</a-space>
          </template>
          <a-alert v-if="target.error_message" type="error" :message="target.error_message" show-icon />
          <a-table row-key="id" size="small" :pagination="false" :columns="resultColumns" :data-source="target.results">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'status'">
                <a-tag :color="record.status === 'pass' ? 'green' : record.status === 'skipped' ? 'default' : 'red'">{{ record.status }}</a-tag>
              </template>
              <template v-else-if="column.key === 'actual_value'"><pre>{{ formatValue(record.actual_value) }}</pre></template>
            </template>
          </a-table>
        </a-collapse-panel>
      </a-collapse>
    </a-drawer>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  getApplicationServiceList,
  getBusinessEnvironmentList,
  getBusinessSystemList,
} from '@/api/assets/application'
import { getHostGroupTree } from '@/api/assets/host'
import {
  deleteInspectionGroup,
  deleteInspectionTask,
  getInspectionExecution,
  getInspectionExecutions,
  getInspectionGroups,
  getInspectionTasks,
  runInspectionTask,
  saveInspectionGroup,
  saveInspectionTask,
} from '@/api/inspection'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const activeTab = ref('tasks')
const groups = ref([])
const tasks = ref([])
const businessSystems = ref([])
const businessEnvironments = ref([])
const services = ref([])
const hostGroups = ref([])
const executions = ref([])
const groupLoading = ref(false)
const taskLoading = ref(false)
const executionLoading = ref(false)
const serviceTreeLoading = ref(false)
const hostGroupTreeLoading = ref(false)
const groupModalOpen = ref(false)
const taskModalOpen = ref(false)
const executionDrawerOpen = ref(false)
const savingGroup = ref(false)
const savingTask = ref(false)
const selectedExecution = ref(null)
const runningTaskIds = reactive(new Set())
let executionPollTimer = null
let localKey = 0

const emptyGroupForm = () => ({ id: null, name: '', scope: 'per_deployment', description: '', enabled: true, checks: [] })
const emptyTaskForm = () => ({
  id: null,
  name: '',
  group: undefined,
  target_type: 'logical_service',
  logical_service: undefined,
  host_group: undefined,
  concurrency: 20,
  timeout_seconds: 60,
  enabled: true,
})
const groupForm = reactive(emptyGroupForm())
const taskForm = reactive(emptyTaskForm())

const taskColumns = [
  { title: '任务名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '巡检组', dataIndex: 'group_name', key: 'group_name', width: 160 },
  { title: '目标', dataIndex: 'target_name', key: 'target', width: 200 },
  { title: '范围', key: 'scope', width: 140 },
  { title: '并发 / 超时', key: 'limits', customRender: ({ record }) => `${record.concurrency} / ${record.timeout_seconds}s`, width: 130 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '操作', key: 'action', fixed: 'right', width: 170 },
]
const groupColumns = [
  { title: '巡检组', dataIndex: 'name', key: 'name', width: 180 },
  { title: '范围', key: 'scope', width: 140 },
  { title: '检查项', key: 'checks', width: 420 },
  { title: '操作', key: 'action', fixed: 'right', width: 120 },
]
const executionColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '任务', dataIndex: 'task_name', key: 'task_name', width: 200 },
  { title: '状态', key: 'status', width: 100 },
  { title: '结果', key: 'summary', width: 180 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 120 },
  { title: '创建时间', key: 'create_time', width: 180 },
  { title: '操作', key: 'action', fixed: 'right', width: 90 },
]
const resultColumns = [
  { title: '检查项', dataIndex: 'name', key: 'name', width: 170 },
  { title: '状态', key: 'status', width: 90 },
  { title: '实际值', key: 'actual_value', width: 220 },
  { title: '消息', dataIndex: 'message', key: 'message' },
]

const runningCount = computed(() => executions.value.filter((item) => ['pending', 'running'].includes(item.status)).length)
const executorOptions = computed(() => groupForm.scope === 'per_deployment'
  ? [{ label: 'Agent Shell', value: 'shell' }]
  : [{ label: '控制端 HTTP', value: 'http' }, { label: '控制端 TCP', value: 'tcp' }])
const selectedTaskGroup = computed(() => groups.value.find((group) => group.id === taskForm.group))
const targetTypeOptions = computed(() => [
  { label: '逻辑服务', value: 'logical_service' },
  {
    label: '主机组',
    value: 'host_group',
    disabled: selectedTaskGroup.value?.scope === 'controller_once',
  },
])
const serviceTreeData = computed(() => {
  const environmentsBySystem = new Map()
  for (const environment of businessEnvironments.value) {
    if (!environmentsBySystem.has(environment.business_system)) environmentsBySystem.set(environment.business_system, [])
    environmentsBySystem.get(environment.business_system).push(environment)
  }
  const servicesByEnvironment = new Map()
  const orphanServices = []
  for (const service of services.value) {
    if (!service.environment) {
      orphanServices.push(service)
      continue
    }
    if (!servicesByEnvironment.has(service.environment)) servicesByEnvironment.set(service.environment, [])
    servicesByEnvironment.get(service.environment).push(service)
  }
  const serviceNodes = (records) => [...records]
    .sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
    .map((service) => ({
      title: service.name,
      value: service.id,
      key: `service:${service.id}`,
      isLeaf: true,
    }))
  const systemNodes = [...businessSystems.value]
    .sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
    .map((system) => ({
      title: system.name,
      value: `system:${system.id}`,
      key: `system:${system.id}`,
      disabled: true,
      children: [...(environmentsBySystem.get(system.id) || [])]
        .sort((left, right) => (left.order - right.order) || left.name.localeCompare(right.name, 'zh-CN'))
        .map((environment) => ({
          title: environment.name,
          value: `environment:${environment.id}`,
          key: `environment:${environment.id}`,
          disabled: true,
          children: serviceNodes(servicesByEnvironment.get(environment.id) || []),
        })),
    }))
  if (orphanServices.length) {
    systemNodes.push({
      title: '未归属业务系统',
      value: 'system:unassigned',
      key: 'system:unassigned',
      disabled: true,
      children: serviceNodes(orphanServices),
    })
  }
  return systemNodes
})
const hostGroupTreeData = computed(() => {
  const mapNodes = (nodes) => (Array.isArray(nodes) ? nodes : []).map((group) => ({
    title: `${group.name} (${group.host_count || 0})`,
    value: group.id,
    key: `host-group:${group.id}`,
    children: mapNodes(group.children),
  }))
  return mapNodes(hostGroups.value)
})

const responseData = (response) => response?.data?.data || {}
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formatTime = (value) => value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
const scopeLabel = (scope) => scope === 'per_deployment' ? '每个部署实例' : '控制端单次'
const executorLabel = (executor) => ({ shell: 'Agent Shell', http: 'HTTP', tcp: 'TCP' }[executor] || executor)
const statusLabel = (status) => ({ pending: '等待中', running: '执行中', success: '成功', failed: '失败' }[status] || status)
const statusColor = (status) => ({ pending: 'default', running: 'processing', success: 'green', failed: 'red' }[status] || 'default')
const formatValue = (value) => typeof value === 'object' && value !== null ? JSON.stringify(value, null, 2) : String(value ?? '-')

async function fetchAll(loader, params = {}) {
  const firstData = responseData(await loader({ ...params, page: 1, page_size: 30 }))
  const records = [...(firstData.results || [])]
  const totalPages = Number(firstData.totalPages || 1)
  if (totalPages > 1) {
    const responses = await Promise.all(
      Array.from({ length: totalPages - 1 }, (_, index) => loader({ ...params, page: index + 2, page_size: 30 })),
    )
    for (const response of responses) records.push(...(responseData(response).results || []))
  }
  return records
}

async function loadGroups() {
  groupLoading.value = true
  try { groups.value = responseData(await getInspectionGroups({ page_size: 30 })).results || [] } finally { groupLoading.value = false }
}
async function loadTasks() {
  taskLoading.value = true
  try { tasks.value = responseData(await getInspectionTasks({ page_size: 30 })).results || [] } finally { taskLoading.value = false }
}
async function loadServiceTree() {
  serviceTreeLoading.value = true
  try {
    const [systems, environments, serviceRecords] = await Promise.all([
      fetchAll(getBusinessSystemList),
      fetchAll(getBusinessEnvironmentList),
      fetchAll(getApplicationServiceList),
    ])
    businessSystems.value = systems
    businessEnvironments.value = environments
    services.value = serviceRecords
  } finally {
    serviceTreeLoading.value = false
  }
}
async function loadHostGroupTree() {
  hostGroupTreeLoading.value = true
  try {
    hostGroups.value = responseData(await getHostGroupTree()) || []
  } finally {
    hostGroupTreeLoading.value = false
  }
}
async function loadExecutions() {
  executionLoading.value = true
  try { executions.value = responseData(await getInspectionExecutions({ page_size: 30 })).results || [] } finally { executionLoading.value = false }
}

function addCheck() {
  const executor = groupForm.scope === 'per_deployment' ? 'shell' : 'http'
  const config = executor === 'shell'
    ? { command: '', work_directory: '${APP_HOME}', expected_output: '' }
    : { url: '', expected_status: 200 }
  groupForm.checks.push({ localKey: ++localKey, name: '', executor, config, enabled: true, order: groupForm.checks.length })
}
function removeCheck(index) { groupForm.checks.splice(index, 1) }
function resetChecksForScope() { groupForm.checks = []; addCheck() }
function openGroupModal(record) {
  Object.assign(groupForm, emptyGroupForm(), record ? JSON.parse(JSON.stringify(record)) : {})
  groupForm.checks = (groupForm.checks || []).map((check) => ({ ...check, localKey: ++localKey, config: { ...(check.config || {}) } }))
  if (!groupForm.checks.length) addCheck()
  groupModalOpen.value = true
}
function openTaskModal(record) {
  Object.assign(taskForm, emptyTaskForm(), record ? JSON.parse(JSON.stringify(record)) : {})
  if (!taskForm.target_type) taskForm.target_type = 'logical_service'
  taskModalOpen.value = true
}
function handleTaskGroupChange() {
  if (selectedTaskGroup.value?.scope === 'controller_once' && taskForm.target_type === 'host_group') {
    taskForm.target_type = 'logical_service'
    taskForm.host_group = undefined
  }
}
function handleTargetTypeChange(targetType) {
  if (targetType === 'host_group') taskForm.logical_service = undefined
  else taskForm.host_group = undefined
}

async function submitGroup() {
  if (!groupForm.name.trim() || !groupForm.checks.length || groupForm.checks.some((check) => !check.name.trim())) {
    message.warning('请完整填写巡检组和检查项')
    return
  }
  savingGroup.value = true
  try {
    const payload = JSON.parse(JSON.stringify(groupForm))
    payload.checks.forEach((check, index) => { delete check.localKey; delete check.id; check.order = index })
    await saveInspectionGroup(payload)
    groupModalOpen.value = false
    message.success('巡检组已保存')
    await loadGroups()
  } finally { savingGroup.value = false }
}
async function submitTask() {
  const hasTarget = taskForm.target_type === 'host_group' ? taskForm.host_group : taskForm.logical_service
  if (!taskForm.name.trim() || !taskForm.group || !hasTarget) {
    message.warning('请完整填写任务信息')
    return
  }
  savingTask.value = true
  try {
    const payload = { ...taskForm }
    if (payload.target_type === 'host_group') payload.logical_service = null
    else payload.host_group = null
    await saveInspectionTask(payload)
    taskModalOpen.value = false
    message.success('巡检任务已保存')
    await loadTasks()
  } finally { savingTask.value = false }
}
async function runTask(record) {
  runningTaskIds.add(record.id)
  try {
    await runInspectionTask(record.id)
    message.success('巡检任务已提交')
    activeTab.value = 'executions'
    await loadExecutions()
    startExecutionPolling()
  } finally { runningTaskIds.delete(record.id) }
}
async function openExecution(record) {
  selectedExecution.value = responseData(await getInspectionExecution(record.id))
  executionDrawerOpen.value = true
}
function confirmDeleteGroup(record) {
  openDeleteConfirm({ title: '删除巡检组', summary: '删除后无法恢复。', items: [record.name], onConfirm: async () => { await deleteInspectionGroup(record.id); await loadGroups() } })
}
function confirmDeleteTask(record) {
  openDeleteConfirm({ title: '删除巡检任务', summary: '历史执行记录会保留。', items: [record.name], onConfirm: async () => { await deleteInspectionTask(record.id); await loadTasks() } })
}
function handleTabChange(key) {
  if (key === 'executions') loadExecutions()
}
function startExecutionPolling() {
  if (executionPollTimer) return
  executionPollTimer = window.setInterval(async () => {
    if (activeTab.value !== 'executions') return
    await loadExecutions()
    if (!executions.value.some((item) => ['pending', 'running'].includes(item.status))) stopExecutionPolling()
  }, 3000)
}
function stopExecutionPolling() {
  if (executionPollTimer) window.clearInterval(executionPollTimer)
  executionPollTimer = null
}

onMounted(async () => { await Promise.all([loadGroups(), loadTasks(), loadServiceTree(), loadHostGroupTree(), loadExecutions()]) })
onBeforeUnmount(stopExecutionPolling)
</script>

<style scoped>
.inspection-page { min-height: 100%; padding: 24px; background: #f4f6f8; color: #17212b; }
.page-header { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; padding: 8px 4px 22px; border-bottom: 1px solid #d9e0e6; }
.page-header h1 { margin: 0 0 6px; font-family: "Noto Sans SC", "Microsoft YaHei", sans-serif; font-size: 28px; font-weight: 700; letter-spacing: 0; }
.page-header p { margin: 0; color: #66727d; }
.summary-strip { display: flex; gap: 1px; border: 1px solid #d9e0e6; background: #d9e0e6; }
.summary-strip div { min-width: 92px; padding: 10px 16px; background: #fff; text-align: center; }
.summary-strip strong, .summary-strip span { display: block; }
.summary-strip strong { color: #126e82; font-size: 20px; }
.summary-strip span { color: #66727d; font-size: 12px; }
.workspace-tabs { margin-top: 18px; padding: 0 20px 20px; background: #fff; border: 1px solid #e1e6ea; }
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.check-heading, .check-editor-head { display: flex; align-items: center; justify-content: space-between; }
.check-heading { margin: 4px 0 12px; font-weight: 600; }
.check-editor { margin-bottom: 12px; padding: 14px 16px 2px; border-left: 3px solid #126e82; background: #f6f8fa; }
.target-results { margin-top: 18px; }
pre { max-width: 240px; margin: 0; white-space: pre-wrap; word-break: break-word; font-size: 12px; }
@media (max-width: 760px) {
  .inspection-page { padding: 12px; }
  .page-header { align-items: flex-start; flex-direction: column; }
  .summary-strip { width: 100%; }
  .summary-strip div { flex: 1; min-width: 0; padding: 8px; }
  .form-grid { grid-template-columns: 1fr; gap: 0; }
}
</style>