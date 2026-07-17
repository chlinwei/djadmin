import { onBeforeUnmount, onMounted, reactive, ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { formatTimeWithTimezone } from '@/util/timezone'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { checkPermission } from '@/directives/permission/permission'
import store from '@/store'
import {
  getWorkflowList,
  getWorkflowDetail,
  createWorkflow,
  updateWorkflow,
  deleteWorkflow,
  launchWorkflow,
  precheckWorkflowLaunch,
} from '@/api/sys/automation'
import { buildAutomationInventoryRoute } from '../navigation'
import ScopePrecheckPanel from '../components/ScopePrecheckPanel.vue'
import ExecutionScopePreviewModal from '../components/ExecutionScopePreviewModal.vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import {
  LIMIT_INPUT_PLACEHOLDER,
  goToAssetHost,
  removeLimitToken,
  resolveMatchedHostLimitToken,
  toggleLimitToken,
} from '../utils/scopeHelpers'
import {
  WORKFLOW_ACTION_TOOLTIP_PROPS,
  WORKFLOW_COLUMNS,
  resolveWorkflowListOrdering,
  toCloneName,
  cloneDefaultExtraVars,
  cloneGraphItems,
  isWorkflowLaunchDisabled,
  buildLimitPayload,
  buildScopePreviewTitle,
  runWorkflowPrecheck,
  buildWorkflowPrecheckMessage,
} from '../utils/workflowControllerHelpers'


export function useAutomationWorkflowController() {
const router = useRouter()
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const loading = ref(false)
const workflowStatusUpdatingId = ref(null)
const launchingId = ref(null)
const cloningId = ref(null)
const cloneSubmitting = ref(false)
const cloneModalVisible = ref(false)
const cloneSourceRecord = ref(null)
const launchSubmitting = ref(false)
const launchModalVisible = ref(false)
const launchTarget = ref(null)
const launchPrecheckLoading = ref(false)
const launchPrecheckOk = ref(false)
const launchPrecheckMessage = ref('可选：为本次启动覆盖 Inventory 与 Limit')
const launchAllHosts = ref([])
const launchMatchedHosts = ref([])
const scopePreviewVisible = ref(false)
const scopePreviewLoading = ref(false)
const scopePreviewTitle = ref('执行范围主机预览')
const scopePreviewHosts = ref([])
const scopePreviewTotal = ref(0)
const keyword = ref('')
const canUpdateWorkflow = computed(() => checkPermission('automation:workflow:update'))
let launchPrecheckTimer = null


const actionTooltipProps = WORKFLOW_ACTION_TOOLTIP_PROPS

const records = ref([])
const launchScopeForm = reactive({
  limit: '',
})
const cloneForm = reactive({
  name: '',
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const listSort = reactive({
  field: null,
  order: null,
})

const columns = WORKFLOW_COLUMNS

function resolveListOrdering() {
  return resolveWorkflowListOrdering(listSort)
}

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
      ordering: resolveListOrdering(),
    })
    const data = res?.data?.data || {}
    records.value = Array.isArray(data.results) ? data.results : []
    pagination.total = Number(data.count || 0)
  } finally {
    loading.value = false
  }
}

function openBuilderForCreate() {
  router.push('/sys/automation/workflow/create')
}

function openBuilderForEdit(record) {
  router.push({ path: '/sys/automation/workflow/editor', query: { id: record.id } })
}

async function cloneRecord(record) {
  const workflowId = Number(record?.id || 0)
  if (!Number.isInteger(workflowId) || workflowId <= 0) {
    message.warning('无效的 Workflow 记录')
    return
  }

  cloningId.value = workflowId
  try {
    const detailRes = await getWorkflowDetail(workflowId)
    const detail = detailRes?.data?.data || {}
    const sourceName = String(detail.name || record?.name || 'Workflow').trim()
    const targetName = String(record?.clone_name || '').trim() || toCloneName(sourceName)
    const payload = {
      name: targetName,
      description: String(detail.description || '').trim(),
      enabled: Boolean(detail.enabled),
      default_inventory: Number(detail.default_inventory || 0) > 0 ? Number(detail.default_inventory) : null,
      default_limit: String(detail.default_limit || '').trim(),
      default_extra_vars: cloneDefaultExtraVars(detail.default_extra_vars),
      remark: String(detail.remark || '').trim(),
      entry_node_key: String(detail.entry_node_key || '').trim(),
      nodes: cloneGraphItems(detail.nodes),
      edges: cloneGraphItems(detail.edges),
    }

    const createRes = await createWorkflow(payload)
    const createdId = Number(createRes?.data?.data?.id || 0)
    message.success('复制成功，已创建 Workflow 副本')
    await loadWorkflows(false)
    if (Number.isInteger(createdId) && createdId > 0) {
      await router.push({ path: '/sys/automation/workflow/editor', query: { id: createdId } })
    }
  } catch (error) {
    const errMsg = error?.response?.data?.msg || error?.message || '复制失败'
    message.error(errMsg)
  } finally {
    cloningId.value = null
  }
}

function openCloneModal(record) {
  const workflowId = Number(record?.id || 0)
  if (!Number.isInteger(workflowId) || workflowId <= 0) {
    message.warning('无效的 Workflow 记录')
    return
  }
  cloneSourceRecord.value = record
  cloneForm.name = toCloneName(record?.name)
  cloneModalVisible.value = true
}

function closeCloneModal() {
  cloneModalVisible.value = false
  cloneSourceRecord.value = null
  cloneForm.name = ''
}

async function confirmClone() {
  const workflowId = Number(cloneSourceRecord.value?.id || 0)
  if (!Number.isInteger(workflowId) || workflowId <= 0) {
    message.warning('无效的 Workflow 记录')
    return
  }

  const cloneName = String(cloneForm.name || '').trim()
  if (!cloneName) {
    message.warning('副本名称不能为空')
    return
  }

  cloneSubmitting.value = true
  try {
    await cloneRecord({ ...cloneSourceRecord.value, clone_name: cloneName })
  } finally {
    cloneSubmitting.value = false
  }
  closeCloneModal()
}

function openWorkflowRunCenter(record = null) {
  const query = { tab: 'workflow' }
  const workflowName = String(record?.name || '').trim()
  if (workflowName) {
    query.keyword = workflowName
  }
  router.push({ path: '/sys/automation/logs', query })
}

function closeScopePreviewModal() {
  scopePreviewVisible.value = false
  scopePreviewLoading.value = false
  scopePreviewHosts.value = []
  scopePreviewTotal.value = 0
}

async function openScopePreviewModal(record) {
  const workflowId = Number(record?.id || 0)
  if (!Number.isInteger(workflowId) || workflowId <= 0) {
    message.warning('无效的 Workflow 记录')
    return
  }

  scopePreviewTitle.value = buildScopePreviewTitle({ ...record, id: workflowId })
  scopePreviewVisible.value = true
  scopePreviewLoading.value = true
  scopePreviewHosts.value = []
  scopePreviewTotal.value = 0

  const inventoryId = Number(record?.default_inventory || 0)
  const limit = String(record?.execution_scope_summary?.limit || record?.default_limit || '').trim()

  try {
    const payload = { limit }
    if (inventoryId > 0) {
      payload.inventory_id = inventoryId
    }
    const { data } = await runWorkflowPrecheck(precheckWorkflowLaunch, workflowId, payload)

    const ok = Boolean(data.ok)
    if (ok) {
      scopePreviewHosts.value = Array.isArray(data.matched_hosts_preview) ? data.matched_hosts_preview : []
      scopePreviewTotal.value = Number(data.matched_hosts_preview_total || data.resolved_host_count || 0)
    } else {
      scopePreviewHosts.value = []
      scopePreviewTotal.value = 0
      message.warning(String(data.message || '预检失败，请检查执行范围配置'))
    }
  } catch (error) {
    scopePreviewHosts.value = []
    scopePreviewTotal.value = 0
    message.error('预检失败，请稍后重试')
  } finally {
    scopePreviewLoading.value = false
  }
}

function goToInventory(record) {
  router.push(
    buildAutomationInventoryRoute({
      inventory: record?.default_inventory,
      inventory_name: record?.default_inventory_name,
    }),
  )
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

async function onChangeWorkflowStatus(checked, record) {
  if (!record?.id) {
    return
  }
  if (!canUpdateWorkflow.value) {
    message.warning('没有状态修改权限')
    return
  }

  const targetEnabled = checked === true
  const originalEnabled = record.enabled === true
  if (targetEnabled === originalEnabled) {
    return
  }

  workflowStatusUpdatingId.value = record.id
  record.enabled = targetEnabled
  try {
    await updateWorkflow(record.id, { enabled: targetEnabled })
    message.success('状态修改成功')
  } catch (error) {
    record.enabled = originalEnabled
    const errMsg = error?.response?.data?.msg || error?.message || '状态修改失败'
    message.error(errMsg)
  } finally {
    workflowStatusUpdatingId.value = null
  }
}

function openDeleteWorkflowConfirm(record) {
  const workflowName = String(record?.name || '').trim() || `#${record?.id || '-'}`
  openDeleteConfirm({
    title: '确认删除 Workflow',
    summary: '删除后不可恢复，请确认影响范围。',
    items: [`Workflow: ${workflowName}`],
    onConfirm: () => removeRecord(record),
  })
}

function launch(record) {
  if (isWorkflowLaunchDisabled(record)) {
    message.warning('该 Workflow 已禁用，无法启动')
    return
  }
  if (!Number(record?.default_inventory || 0)) {
    message.warning('该 Workflow 未配置默认 Inventory，无法启动')
    return
  }
  launchTarget.value = record || null
  launchScopeForm.limit = String(record?.default_limit || '')
  launchPrecheckOk.value = false
  launchAllHosts.value = []
  launchMatchedHosts.value = []
  launchPrecheckMessage.value = '将使用 Workflow 默认 Inventory 进行预检'
  launchModalVisible.value = true
  refreshLaunchPrecheck()
}

function closeLaunchModal() {
  clearLaunchPrecheckTimer()
  launchModalVisible.value = false
  launchSubmitting.value = false
  launchTarget.value = null
  launchScopeForm.limit = ''
  launchPrecheckOk.value = false
  launchAllHosts.value = []
  launchMatchedHosts.value = []
  launchPrecheckMessage.value = '可选：为本次启动覆盖 Limit'
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
  return buildLimitPayload(launchScopeForm.limit)
}

async function refreshLaunchPrecheck() {
  if (!launchTarget.value?.id) {
    return
  }

  launchPrecheckLoading.value = true
  try {
    const payload = buildLaunchPayload()
    const { baseData, data } = await runWorkflowPrecheck(precheckWorkflowLaunch, launchTarget.value.id, payload)

    const ok = Boolean(data.ok)
    launchPrecheckOk.value = ok
    launchAllHosts.value = Array.isArray(baseData.matched_hosts_preview) ? baseData.matched_hosts_preview : []
    launchMatchedHosts.value = Array.isArray(data.matched_hosts_preview) ? data.matched_hosts_preview : []
    launchPrecheckMessage.value = buildWorkflowPrecheckMessage(data)
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
  } finally {
    launchSubmitting.value = false
    launchingId.value = null
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

function onTableChange(page, _filters, sorter) {
  pagination.current = page.current
  pagination.pageSize = page.pageSize

  const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
  const allowedFields = ['name', 'enabled', 'update_time']
  if (nextSorter?.field && allowedFields.includes(nextSorter.field) && nextSorter.order) {
    listSort.field = nextSorter.field
    listSort.order = nextSorter.order
  } else {
    listSort.field = null
    listSort.order = null
  }

  loadWorkflows(false)
}

function reloadAll() {
  loadWorkflows(false)
}

onMounted(async () => {
  await loadWorkflows(true)
})

watch(
  () => [launchScopeForm.limit, launchModalVisible.value],
  ([, visible]) => {
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

  return {
    keyword,
    loadWorkflows,
    actionTooltipProps,
    openBuilderForCreate,
    canUpdateWorkflow,
    loading,
    workflowStatusUpdatingId,
    reloadAll,
    columns,
    records,
    pagination,
    onTableChange,
    formatTimeWithTimezone,
    store,
    goToInventory,
    openScopePreviewModal,
    openBuilderForEdit,
    cloningId,
    openCloneModal,
    isWorkflowLaunchDisabled,
    launchingId,
    launch,
    openWorkflowRunCenter,
    openDeleteWorkflowConfirm,
    onChangeWorkflowStatus,
    cloneModalVisible,
    cloneSubmitting,
    confirmClone,
    closeCloneModal,
    cloneSourceRecord,
    cloneForm,
    launchModalVisible,
    launchSubmitting,
    confirmLaunch,
    closeLaunchModal,
    launchTarget,
    launchScopeForm,
    LIMIT_INPUT_PLACEHOLDER,
    launchPrecheckLoading,
    refreshLaunchPrecheck,
    launchPrecheckOk,
    launchPrecheckMessage,
    launchAllHosts,
    launchMatchedHosts,
    handleLaunchHostClick,
    handleLaunchLimitToggle,
    handleLaunchRemoveLimitToken,
    scopePreviewTitle,
    scopePreviewVisible,
    scopePreviewHosts,
    scopePreviewTotal,
    scopePreviewLoading,
    closeScopePreviewModal,
  }
}
