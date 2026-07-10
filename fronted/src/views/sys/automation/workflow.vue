<template>
  <div class="workflow-page">
    <a-row class="tools" :gutter="12">
      <a-col :span="16">
        <a-input-search
          v-model:value="keyword"
          placeholder="搜索 Workflow 名称"
          allow-clear
          enter-button
          @search="loadWorkflows(true)"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-button size="large" @click="openBuilderForCreate" v-permission="'automation:workflow:create'">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp新增Workflow</span>
          </a-button>
          <a-button type="primary" ghost :loading="loading || runLoading" @click="reloadAll">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading || runLoading" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="Workflow 列表" size="small" class="block-card">
      <a-table
        :columns="columns"
        :data-source="records"
        :loading="loading"
        :pagination="pagination"
        :scroll="{ x: 1200 }"
        rowKey="id"
        size="small"
        @change="onTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '禁用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'node_count'">
            <span>{{ Number(record.node_count || 0) }} 节点 / {{ Number(record.edge_count || 0) }} 连线</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑编排">
                <a-button size="small" type="primary" @click="openBuilderForEdit(record)" v-permission="'automation:workflow:update'">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip :title="isWorkflowLaunchDisabled(record) ? '已禁用，无法启动' : '启动'">
                <a-button
                  size="small"
                  type="primary"
                  ghost
                  :disabled="isWorkflowLaunchDisabled(record)"
                  :loading="launchingId === record.id"
                  @click="launch(record)"
                  v-permission="'automation:workflow:launch'"
                >
                  <FontAwesomeIcon :icon="['fas', 'play']" />
                </a-button>
              </a-tooltip>
              <a-popconfirm title="确认删除该 Workflow 吗？" ok-text="确认" cancel-text="取消" @confirm="removeRecord(record)">
                <a-button size="small" type="primary" danger v-permission="'automation:workflow:delete'">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-card title="运行记录" size="small" class="block-card">
      <template #extra>
        <a-space wrap>
          <a-input-search
            v-model:value="runFilterKeyword"
            placeholder="Workflow 名称 / 触发人"
            allow-clear
            style="width: 220px"
            @search="loadWorkflowRuns(true)"
          />
          <a-select
            v-model:value="runFilterStatus"
            :options="runStatusOptions"
            allow-clear
            placeholder="运行状态"
            style="width: 120px"
            @change="loadWorkflowRuns(true)"
          />
          <a-range-picker
            v-model:value="runFilterTimeRange"
            show-time
            format="YYYY-MM-DD HH:mm"
            :placeholder="['开始时间', '结束时间']"
            style="width: 320px"
            @change="loadWorkflowRuns(true)"
          />
          <a-button type="link" size="small" :disabled="!hasRunFilters" @click="resetRunFilters">重置</a-button>
        </a-space>
      </template>
      <a-table
        :columns="runColumns"
        :data-source="runRecords"
        :loading="runLoading"
        :pagination="runPagination"
        :scroll="{ x: 1400 }"
        rowKey="id"
        size="small"
        @change="onRunTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="getRunStatusColor(record.status)">{{ record.status || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'start_time'">
            <span>{{ formatDateTime(record.start_time) }}</span>
          </template>
          <template v-else-if="column.key === 'duration_seconds'">
            <span>{{ formatDuration(record.duration_seconds) }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button size="small" type="primary" ghost @click="openRunStatus(record)">
                查看状态图
              </a-button>
              <a-popconfirm
                v-if="canCancelWorkflowRunRecord(record)"
                title="确认取消该 Workflow 运行吗？"
                ok-text="确认"
                cancel-text="取消"
                @confirm="cancelRunRecord(record)"
              >
                <a-button
                  size="small"
                  danger
                  :loading="runCancelingId === record.id"
                  v-permission="'automation:jobs:cancel'"
                >
                  取消运行
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      title="确认启动 Workflow"
      :open="launchModalVisible"
      :confirm-loading="launchSubmitting"
      ok-text="确认启动"
      cancel-text="取消"
      @ok="confirmLaunch"
      @cancel="closeLaunchModal"
    >
      <a-form layout="vertical">
        <a-form-item label="Workflow">
          <a-input :value="launchTarget?.name || '-'" readonly />
        </a-form-item>
        <a-form-item label="选择 Inventory（必填）" required>
          <a-select
            v-model:value="launchScopeForm.inventory_id"
            :options="inventoryOptions"
            show-search
            optionFilterProp="label"
            placeholder="请选择 Inventory"
          />
        </a-form-item>
        <a-form-item label="覆盖 Limit">
          <a-input
            v-model:value="launchScopeForm.limit"
            :placeholder="LIMIT_INPUT_PLACEHOLDER"
          />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button size="small" :loading="launchPrecheckLoading" @click="refreshLaunchPrecheck">预检范围</a-button>
          </a-space>
          <ScopePrecheckPanel
            :precheck-ok="launchPrecheckOk"
            :prechecking="launchPrecheckLoading"
            :message="launchPrecheckMessage"
            :hosts="launchAllHosts"
            :matched-hosts="launchMatchedHosts"
            :show-host-link="true"
            :show-limit-toggle="true"
            :show-target-filter="true"
            :limit-text="launchScopeForm.limit"
            @host-click="handleLaunchHostClick"
            @toggle-limit-host="handleLaunchLimitToggle"
            @remove-limit-token="handleLaunchRemoveLimitToken"
          />
        </a-form-item>
      </a-form>
    </a-modal>

  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, reactive, ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import {
  getWorkflowList,
  deleteWorkflow,
  launchWorkflow,
  getWorkflowRunList,
  cancelWorkflowRun,
  getInventoryList,
  precheckWorkflowLaunch,
} from '@/api/sys/automation'
import ScopePrecheckPanel from './components/ScopePrecheckPanel.vue'
import {
  LIMIT_INPUT_PLACEHOLDER,
  goToAssetHost,
  mapInventoryOptions,
  removeLimitToken,
  resolveMatchedHostLimitToken,
  toggleLimitToken,
} from './utils/scopeHelpers'

const router = useRouter()

const loading = ref(false)
const runLoading = ref(false)
const runCancelingId = ref(null)
const launchingId = ref(null)
const launchSubmitting = ref(false)
const launchModalVisible = ref(false)
const launchTarget = ref(null)
const launchPrecheckLoading = ref(false)
const launchPrecheckOk = ref(false)
const launchPrecheckMessage = ref('可选：为本次启动覆盖 Inventory 与 Limit')
const launchAllHosts = ref([])
const launchMatchedHosts = ref([])
const keyword = ref('')
let launchPrecheckTimer = null

const runFilterKeyword = ref('')
const runFilterStatus = ref(undefined)
const runFilterTimeRange = ref([])
const runStatusOptions = [
  { label: '运行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'cancelled' },
  { label: '等待中', value: 'pending' },
]
const hasRunFilters = computed(() => {
  return !!(runFilterKeyword.value || runFilterStatus.value || (runFilterTimeRange.value && runFilterTimeRange.value.length === 2))
})

function resetRunFilters() {
  runFilterKeyword.value = ''
  runFilterStatus.value = undefined
  runFilterTimeRange.value = []
  loadWorkflowRuns(true)
}

const records = ref([])
const runRecords = ref([])
const inventoryOptions = ref([])
const launchScopeForm = reactive({
  inventory_id: undefined,
  limit: '',
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const runPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '图规模', dataIndex: 'node_count', key: 'node_count', width: 160 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 140 },
  { title: '操作', key: 'action', width: 240, fixed: 'right' },
]

const runColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: 'Workflow', dataIndex: 'workflow_name', key: 'workflow_name', width: 180 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 140 },
  { title: '触发人', dataIndex: 'requested_username', key: 'requested_username', width: 130 },
  { title: '创建时间', dataIndex: 'create_time', key: 'create_time', width: 140 },
  { title: '开始运行', dataIndex: 'start_time', key: 'start_time', width: 160 },
  { title: '耗时', dataIndex: 'duration_seconds', key: 'duration_seconds', width: 100 },
  { title: '操作', key: 'action', width: 220, fixed: 'right' },
]

async function loadWorkflows(resetPage = false) {
  if (resetPage) {
    pagination.current = 1
  }

  loading.value = true
  try {
    const res = await getWorkflowList({
      page: pagination.current,
      page_size: pagination.pageSize,
      search: String(keyword.value || '').trim(),
    })
    const data = res?.data?.data || {}
    records.value = Array.isArray(data.results) ? data.results : []
    pagination.total = Number(data.count || 0)
  } finally {
    loading.value = false
  }
}

async function loadWorkflowRuns(resetPage = false) {
  if (resetPage) {
    runPagination.current = 1
  }

  runLoading.value = true
  try {
    const params = {
      page: runPagination.current,
      page_size: runPagination.pageSize,
    }
    const kw = String(runFilterKeyword.value || '').trim()
    if (kw) {
      params.search = kw
    }
    if (runFilterStatus.value) {
      params.status = runFilterStatus.value
    }
    if (runFilterTimeRange.value && runFilterTimeRange.value.length === 2) {
      params.start_time_after = runFilterTimeRange.value[0].toISOString()
      params.start_time_before = runFilterTimeRange.value[1].toISOString()
    }
    const res = await getWorkflowRunList(params)
    const data = res?.data?.data || {}
    runRecords.value = Array.isArray(data.results) ? data.results : []
    runPagination.total = Number(data.count || 0)
  } finally {
    runLoading.value = false
  }
}

async function loadInventoryOptions() {
  const res = await getInventoryList({ page: 1, page_size: 500, ordering: '-id' })
  const data = res?.data?.data || {}
  inventoryOptions.value = mapInventoryOptions(data.results)
}

function openBuilderForCreate() {
  router.push('/sys/automation/workflow/create')
}

function openBuilderForEdit(record) {
  router.push({ path: '/sys/automation/workflow/editor', query: { id: record.id } })
}

function handleLaunchHostClick(item) {
  goToAssetHost(router, message, item?.host_id, item?.host_name)
}

function handleLaunchLimitToggle(item) {
  const token = resolveMatchedHostLimitToken(item)
  launchScopeForm.limit = toggleLimitToken(launchScopeForm.limit, token)
}

function handleLaunchRemoveLimitToken(token) {
  launchScopeForm.limit = removeLimitToken(launchScopeForm.limit, token)
}

async function removeRecord(record) {
  try {
    await deleteWorkflow(record.id)
    message.success('删除成功')
    await loadWorkflows(false)
  } catch (err) {
    const errMsg = err?.response?.data?.msg || err?.message || '删除失败'
    message.error(errMsg)
  }
}

function launch(record) {
  if (isWorkflowLaunchDisabled(record)) {
    message.warning('该 Workflow 已禁用，无法启动')
    return
  }
  launchTarget.value = record || null
  const defaultInventoryId = Number(record?.default_inventory || 0) > 0 ? Number(record.default_inventory) : undefined
  // 检查默认 inventory 是否仍存在（可能已被删除）
  if (defaultInventoryId) {
    const exists = inventoryOptions.value.some((opt) => Number(opt.value) === defaultInventoryId)
    if (!exists) {
      message.warning('该 Workflow 绑定的 Inventory 已被删除，请重新选择')
      launchScopeForm.inventory_id = undefined
    } else {
      launchScopeForm.inventory_id = defaultInventoryId
    }
  } else {
    launchScopeForm.inventory_id = undefined
  }
  launchScopeForm.limit = String(record?.default_limit || '')
  launchPrecheckOk.value = false
  launchAllHosts.value = []
  launchMatchedHosts.value = []
  launchPrecheckMessage.value = '请选择 Inventory 后启动'
  launchModalVisible.value = true
  refreshLaunchPrecheck()
}

function isWorkflowLaunchDisabled(record) {
  return !Boolean(record?.enabled)
}

function closeLaunchModal() {
  clearLaunchPrecheckTimer()
  launchModalVisible.value = false
  launchSubmitting.value = false
  launchTarget.value = null
  launchScopeForm.inventory_id = undefined
  launchScopeForm.limit = ''
  launchPrecheckOk.value = false
  launchAllHosts.value = []
  launchMatchedHosts.value = []
  launchPrecheckMessage.value = '可选：为本次启动覆盖 Inventory 与 Limit'
}

function clearLaunchPrecheckTimer() {
  if (launchPrecheckTimer) {
    clearTimeout(launchPrecheckTimer)
    launchPrecheckTimer = null
  }
}

function scheduleLaunchPrecheck(delay = 250) {
  clearLaunchPrecheckTimer()
  launchPrecheckTimer = setTimeout(() => {
    refreshLaunchPrecheck()
  }, delay)
}

function buildLaunchPayload() {
  const payload = {}
  if (Number(launchScopeForm.inventory_id || 0) > 0) {
    payload.inventory_id = Number(launchScopeForm.inventory_id)
  }
  payload.limit = String(launchScopeForm.limit || '').trim()
  return payload
}

async function refreshLaunchPrecheck() {
  if (!launchTarget.value?.id) {
    return
  }

  launchPrecheckLoading.value = true
  try {
    const payload = buildLaunchPayload()
    const basePayload = { ...payload, limit: '' }
    const baseRes = await precheckWorkflowLaunch(launchTarget.value.id, basePayload)
    const baseData = baseRes?.data?.data || {}

    let data = baseData
    if (String(payload.limit || '').trim()) {
      const narrowedRes = await precheckWorkflowLaunch(launchTarget.value.id, payload)
      data = narrowedRes?.data?.data || {}
    }

    const ok = Boolean(data.ok)
    launchPrecheckOk.value = ok
    launchAllHosts.value = Array.isArray(baseData.matched_hosts_preview) ? baseData.matched_hosts_preview : []
    launchMatchedHosts.value = Array.isArray(data.matched_hosts_preview) ? data.matched_hosts_preview : []
    const scopeLabel = data.use_global_scope
      ? `Inventory: ${String(data.inventory_name || '-')}`
      : '未启用 Workflow 全局 Inventory'
    if (ok) {
      const hostCount = Number(data.resolved_host_count || 0)
      if (data.use_global_scope) {
        launchPrecheckMessage.value = `${scopeLabel}，匹配主机 ${hostCount} 台`
      } else {
        launchPrecheckMessage.value = `${scopeLabel}，将按各任务节点执行范围运行`
      }
    } else {
      launchPrecheckMessage.value = String(data.message || '预检失败，请检查范围配置')
    }
  } catch (error) {
    launchPrecheckOk.value = false
    launchAllHosts.value = []
    launchMatchedHosts.value = []
    launchPrecheckMessage.value = '预检失败，请稍后重试'
  } finally {
    launchPrecheckLoading.value = false
  }
}

async function confirmLaunch() {
  if (!launchTarget.value?.id) {
    return
  }
  if (!Number(launchScopeForm.inventory_id || 0)) {
    message.error('请选择 Inventory')
    return
  }
  launchingId.value = launchTarget.value.id
  launchSubmitting.value = true
  try {
    const res = await launchWorkflow(launchTarget.value.id, buildLaunchPayload())
    const runId = Number(res?.data?.data?.id || 0)
    message.success('Workflow 已启动')
    closeLaunchModal()
    if (Number.isInteger(runId) && runId > 0) {
      router.push({
        path: '/sys/automation/workflow/run',
        query: { run_id: String(runId) },
      })
      return
    }
    await loadWorkflowRuns(true)
  } finally {
    launchSubmitting.value = false
    launchingId.value = null
  }
}

function openRunStatus(record) {
  const runId = Number(record?.id)
  if (!Number.isInteger(runId) || runId <= 0) {
    message.warning('无效的运行记录ID')
    return
  }
  router.push({
    path: '/sys/automation/workflow/run',
    query: { run_id: String(runId) },
  })
}

function normalizeRunStatus(record) {
  return String(record?.runtime_status || record?.status || '').toLowerCase()
}

function canCancelWorkflowRunRecord(record) {
  return ['pending', 'running'].includes(normalizeRunStatus(record))
}

async function cancelRunRecord(record) {
  const runId = Number(record?.id)
  if (!Number.isInteger(runId) || runId <= 0) {
    message.warning('无效的运行记录ID')
    return
  }

  runCancelingId.value = runId
  try {
    await cancelWorkflowRun(runId)
    message.success('Workflow 运行已取消')
    await loadWorkflowRuns(false)
  } finally {
    runCancelingId.value = null
  }
}

function formatDateTime(dateStr) {
  if (!dateStr) return '-'
  try {
    const date = new Date(dateStr)
    if (Number.isNaN(date.getTime())) return '-'
    const tz = store.state.user?.timezone || 'Asia/Shanghai'
    return formatTimeWithTimezone(date, tz)
  } catch {
    return '-'
  }
}

function formatDuration(seconds) {
  if (seconds === null || seconds === undefined) return '-'
  const num = Number(seconds)
  if (!Number.isFinite(num) || num < 0) return '-'
  
  // 如果少于1秒，显示毫秒
  if (num < 1) {
    return `${(num * 1000).toFixed(0)}ms`
  }
  // 如果少于60秒
  if (num < 60) {
    return `${num.toFixed(2)}s`
  }
  // 如果少于3600秒（1小时）
  if (num < 3600) {
    const mins = Math.floor(num / 60)
    const secs = (num % 60).toFixed(0)
    return `${mins}m ${secs}s`
  }
  // 超过1小时
  const hours = Math.floor(num / 3600)
  const mins = Math.floor((num % 3600) / 60)
  return `${hours}h ${mins}m`
}

function getRunStatusColor(status) {
  if (status === 'success') {
    return 'green'
  }
  if (status === 'failed') {
    return 'red'
  }
  if (status === 'running') {
    return 'blue'
  }
  return 'default'
}

function onTableChange(page) {
  pagination.current = page.current
  pagination.pageSize = page.pageSize
  loadWorkflows(false)
}

function onRunTableChange(page) {
  runPagination.current = page.current
  runPagination.pageSize = page.pageSize
  loadWorkflowRuns(false)
}

function reloadAll() {
  loadWorkflows(false)
  loadWorkflowRuns(false)
}

onMounted(async () => {
  await Promise.all([loadWorkflows(true), loadWorkflowRuns(true), loadInventoryOptions()])
})

watch(
  () => [launchScopeForm.inventory_id, launchScopeForm.limit, launchModalVisible.value],
  ([, , visible]) => {
    if (!visible) {
      return
    }
    launchPrecheckMessage.value = '正在预检...'
    scheduleLaunchPrecheck(250)
  },
)

onBeforeUnmount(() => {
  clearLaunchPrecheckTimer()
})
</script>

<style scoped>
.workflow-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.right-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.block-card :deep(.ant-card-body) {
  padding-top: 12px;
}

@media (max-width: 992px) {
  .right-actions {
    justify-content: flex-start;
    margin-top: 8px;
  }
}
</style>
