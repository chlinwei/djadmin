import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message, Modal } from 'ant-design-vue'
import dayjs from 'dayjs'
import { AnsiUp } from 'ansi_up'
import { formatTimeWithTimezone } from '@/util/timezone'
import { buildUserTimezoneRangePresets, buildUserTimezoneShowTime, toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'
import { getToken } from '@/api/user'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import store from '@/store'
import { getWebSocketBaseUrl } from '@/util/request'
import { getJobList, getJobDetail, cancelJob, getTaskList, getJobLog } from '@/api/sys/automation'
import { cancelMonitorInstallHistory, getMonitorInstallHistoryDetail, getMonitorInstallHistoryList } from '@/api/monitor'
import {
  buildHostScopedLogText,
  copyTextWithFallback,
  normalizeUnifiedLogAliases,
} from '../../utils/logHelpers'
import { goToAssetHost } from '../../utils/scopeHelpers'
import {
  normalizeJobStatus,
  statusColor,
  isJobFinished,
  canDownloadJobLog,
  formatRuntimeTemplateLabel,
  getRuntimeTemplateContent,
  getInventoryHostList,
  getWorkflowRunHostList,
  buildMergedLogText,
  toSafeFileSegment,
  formatWorkflowDuration,
  getWorkflowRunStatusColor,
  canCancelWorkflowRunRecord,
  normalizeUtcTime,
} from '../../utils/logsControllerHelpers'
import ExecutionScopePreviewModal from '../../components/ExecutionScopePreviewModal.vue'

const ansiRenderer = new AnsiUp()
ansiRenderer.escape_for_html = true

function renderAnsiLogToHtml(text) {
  const source = String(text || '')
  if (!source) {
    return ''
  }
  return ansiRenderer.ansi_to_html(source)
}


export function useAutomationLogsController() {
const route = useRoute()
const router = useRouter()
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const activeRecordTab = ref('job')

function getActiveUserTimezone() {
  return String(store.state.user?.timezone || 'Asia/Shanghai')
}

const jobs = ref([])
const jobLoading = ref(false)
const jobRecordId = ref('')
const jobKeyword = ref('')
const selectedJobStatus = ref(null)
const jobOutputKeyword = ref('')
const jobTimeRange = ref([])
const logsTimeRangePresets = ref([])
const logsTimeRangeShowTime = buildUserTimezoneShowTime(getActiveUserTimezone())
const jobPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const selectedTaskId = ref(null)
const selectedTaskName = ref('')
const taskOptions = ref([])
const taskNameMap = ref({})

const workflowRuns = ref([])
const workflowRunLoading = ref(false)
const workflowRunCancelingId = ref(null)
const workflowRunKeyword = ref('')
const workflowRunStatus = ref(undefined)
const workflowRunTimeRange = ref([])
const workflowRunStatusOptions = [
  { label: '运行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'cancelled' },
  { label: '等待中', value: 'pending' },
]
const workflowRunPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})
const workflowRunSort = reactive({
  field: null,
  order: null,
})
const workflowRunColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80, sorter: true },
  { title: 'Workflow', dataIndex: 'workflow_name', key: 'workflow_name', width: 180 },
  { title: '运行主机', key: 'inventory_hosts', width: 260 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 140, sorter: true },
  { title: '触发人', dataIndex: 'requested_username', key: 'requested_username', width: 130 },
  { title: '开始运行', dataIndex: 'start_time', key: 'start_time', width: 160, sorter: true },
  { title: '耗时', dataIndex: 'duration_seconds', key: 'duration_seconds', width: 100, sorter: true },
  { title: '操作', key: 'action', width: 220, fixed: 'right' },
]
const monitorInstallHistoryRows = ref([])
const monitorInstallHistoryLoading = ref(false)
const monitorInstallHistoryKeyword = ref('')
const monitorInstallHistoryStatus = ref(undefined)
const monitorInstallHistoryAction = ref(undefined)
const monitorInstallHistoryTargetId = ref('')
const monitorInstallHistoryTargetType = ref('')
const monitorInstallHistoryTimeRange = ref([])
const monitorInstallHistoryPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})
const monitorInstallHistoryStatusOptions = [
  { label: '待执行', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'cancelled' },
]
const monitorInstallHistoryActionOptions = [
  { label: '安装', value: 'install' },
  { label: '卸载', value: 'uninstall' },
]
const monitorInstallHistoryColumns = [
  { title: '历史ID', dataIndex: 'id', key: 'id', width: 100 },
  { title: '纳管目标ID', dataIndex: 'managed_target_id', key: 'managed_target_id', width: 110 },
  { title: '动作', dataIndex: 'action', key: 'action', width: 100 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 180 },
  { title: '主机IP', dataIndex: 'host_ip', key: 'host_ip', width: 150 },
  { title: '监控项', dataIndex: 'target_exporter_type', key: 'target_exporter_type', width: 140 },
  { title: '摘要', dataIndex: 'summary_message', key: 'summary_message', width: 360 },
  { title: '创建时间', dataIndex: 'create_time', key: 'create_time', width: 170 },
  { title: '操作', key: 'action_col', width: 220, fixed: 'right' },
]
const monitorHistoryDetailVisible = ref(false)
const monitorHistoryDetailLoading = ref(false)
const monitorHistoryDetailRecord = ref(null)
const monitorHistoryDetailText = ref('')
const monitorHistoryDetailWrap = ref(true)
const monitorHistoryDetailFontSize = ref(13)
const monitorHistorySourceJobId = ref(null)
const monitorHistorySourceJobExists = ref(false)
const monitorHistorySourceJobChecking = ref(false)
const hasWorkflowRunFilters = computed(() => {
  return !!(
    workflowRunKeyword.value ||
    workflowRunStatus.value ||
    (workflowRunTimeRange.value && workflowRunTimeRange.value.length === 2)
  )
})

function refreshLogsTimeRangePresets() {
  const showTime = buildUserTimezoneShowTime(getActiveUserTimezone())
  logsTimeRangeShowTime.defaultValue = showTime.defaultValue
  logsTimeRangeShowTime.defaultOpenValue = showTime.defaultOpenValue
  logsTimeRangePresets.value = buildUserTimezoneRangePresets(getActiveUserTimezone())
}

function onLogsTimeRangeOpenChange(open) {
  if (open) {
    refreshLogsTimeRangePresets()
  }
}

const jobStatusOptions = [
  { value: 'pending', label: '待执行' },
  { value: 'running', label: '执行中' },
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'cancelled', label: '已取消' },
]

const logViewerVisible = ref(false)
const logViewerRecord = ref(null)
const logViewerJobOutput = ref('')
const jobLogViewerVisible = ref(false)
const jobLogViewerJobId = ref(null)
const jobLogText = ref('')
const jobLogViewerShellRef = ref(null)
const jobLogAutoFollowEnabled = ref(true)
const jobLogAutoFollowSuspended = ref(false)
const logWrap = ref(true)
const jobLogWrap = ref(true)
const logFontSize = ref(13)
const jobLogFontSize = ref(13)
const downloadingJobLogId = ref(null)
const cancellingJobId = ref(null)
const streamConnectionState = ref('idle')
const streamJobStatus = ref('')
const streamLastOutputAt = ref(0)
const streamLastOutputServerTime = ref('')
const streamClockTick = ref(Date.now())
const runtimeTemplateVisible = ref(false)
const runtimeTemplateTitle = ref('运行模板')
const runtimeTemplateContent = ref('')
const jobHostViewerVisible = ref(false)
const jobHostViewerTitle = ref('运行主机')
const jobHostViewerHosts = ref([])
const automationLogsRefreshIntervalSeconds = ref(5)

const MIN_AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS = 1
const MAX_AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS = 600

let pollTimer = null
let jobLogSocket = null
let jobLogSocketConnected = false
let jobLogReconnectTimer = null
let jobLogSocketJobId = null
let streamClockTimer = null

const jobSort = reactive({
  field: null,
  order: null,
})

function resolveJobOrdering() {
  if (!jobSort.field || !jobSort.order) {
    return '-id'
  }

  const orderingFieldMap = {
    // 前端展示 job_id，后端未开放该字段排序，退化为按主键排序。
    job_id: 'id',
    status: 'status',
    start_time: 'start_time',
    duration_seconds: 'duration_seconds',
  }

  const mappedField = orderingFieldMap[jobSort.field]
  if (!mappedField) {
    return '-id'
  }

  const prefix = jobSort.order === 'descend' ? '-' : ''
  return `${prefix}${mappedField}`
}

function resolveWorkflowRunOrdering() {
  if (!workflowRunSort.field || !workflowRunSort.order) {
    return '-id'
  }

  const orderingFieldMap = {
    id: 'id',
    status: 'status',
    start_time: 'start_time',
    duration_seconds: 'duration_seconds',
  }

  const mappedField = orderingFieldMap[workflowRunSort.field]
  if (!mappedField) {
    return '-id'
  }

  const prefix = workflowRunSort.order === 'descend' ? '-' : ''
  return `${prefix}${mappedField}`
}

function resolveConfigIntValue(response, fallbackValue) {
  const rawValue = response?.data?.value ?? response?.data?.data?.value
  const parsed = Number(rawValue)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallbackValue
  }
  return Math.floor(parsed)
}

async function loadAutomationLogsRefreshIntervalConfig() {
  try {
    const res = await getConfigByKey(CONFIG_KEYS.AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS)
    const configValue = resolveConfigIntValue(res, 5)
    automationLogsRefreshIntervalSeconds.value = Math.max(
      MIN_AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS,
      Math.min(MAX_AUTOMATION_LOGS_REFRESH_INTERVAL_SECONDS, configValue)
    )
  } catch (error) {
    automationLogsRefreshIntervalSeconds.value = 5
  }
}

const jobColumns = [
  { title: '任务ID', dataIndex: 'job_id', key: 'job_id', width: 100, sorter: true },
  { title: '任务名称', dataIndex: 'task_name', key: 'task_name', width: 140 },
  { title: '运行模板', key: 'runtime_template', width: 160 },
  { title: '运行主机', key: 'inventory_hosts', width: 260 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100, sorter: true },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 100 },
  { title: '开始运行时间', dataIndex: 'start_time', key: 'start_time', width: 180, sorter: true },
  { title: '耗时', dataIndex: 'duration_seconds', key: 'duration_seconds', width: 90, sorter: true },
  { title: '操作', key: 'action', width: 300, fixed: 'right' },
]

const currentLogText = computed(() => {
  const record = logViewerRecord.value
  if (!record) {
    return ''
  }
  const hostScoped = buildHostScopedLogText(logViewerJobOutput.value, record)
  if (hostScoped) {
    return hostScoped
  }
  return buildMergedLogText(record)
})

const currentLogHtml = computed(() => renderAnsiLogToHtml(currentLogText.value))

const jobLogHtml = computed(() => renderAnsiLogToHtml(jobLogText.value))
const monitorHistoryDetailHtml = computed(() => renderAnsiLogToHtml(monitorHistoryDetailText.value))
const monitorHistoryDetailStatus = computed(() => String(monitorHistoryDetailRecord.value?.status || '-'))
const monitorHistoryDetailLastOutput = computed(() => {
  const sourceTime = monitorHistoryDetailRecord.value?.end_time || monitorHistoryDetailRecord.value?.update_time || monitorHistoryDetailRecord.value?.create_time
  return formatDateTime(sourceTime)
})

function bindJobLogViewerShell(element) {
  jobLogViewerShellRef.value = element
}

const logViewerHostLabel = computed(() => {
  const record = logViewerRecord.value
  if (!record) {
    return '-'
  }
  return `${record.host_name || '-'} (${record.host_ip || '-'})`
})

const streamStatusTagColor = computed(() => {
  const status = streamStatusLabel.value
  if (status.startsWith('实时输出中')) {
    return 'green'
  }
  if (status.startsWith('等待新输出')) {
    return 'gold'
  }
  if (status.startsWith('连接中') || status.startsWith('重连中')) {
    return 'processing'
  }
  if (status.startsWith('连接异常') || status.startsWith('连接断开')) {
    return 'red'
  }
  if (status.startsWith('已结束')) {
    return 'blue'
  }
  return 'default'
})

const streamStatusLabel = computed(() => {
  // Keep this dependency to refresh idle-check every second.
  const now = streamClockTick.value
  const currentStatus = String(streamJobStatus.value || '').toLowerCase()
  const outputAgeMs = streamLastOutputAt.value ? now - streamLastOutputAt.value : Number.POSITIVE_INFINITY

  if (currentStatus === 'success') {
    return '已结束（成功）'
  }
  if (currentStatus === 'failed') {
    return '已结束（失败）'
  }
  if (currentStatus === 'cancelled') {
    return '已结束（已取消）'
  }

  if (streamConnectionState.value === 'connecting') {
    return '连接中'
  }
  if (streamConnectionState.value === 'reconnecting') {
    return '重连中'
  }
  if (streamConnectionState.value === 'error') {
    return '连接异常'
  }
  if (streamConnectionState.value === 'disconnected') {
    return '连接断开'
  }

  if (streamConnectionState.value === 'connected') {
    if (outputAgeMs <= 6000) {
      return '实时输出中'
    }
    return '等待新输出'
  }

  return '未连接'
})

const streamLastOutputText = computed(() => {
  if (streamLastOutputServerTime.value) {
    return `${streamLastOutputServerTime.value} (后端)`
  }
  if (!streamLastOutputAt.value) {
    return '-'
  }
  const date = new Date(streamLastOutputAt.value)
  const hh = String(date.getHours()).padStart(2, '0')
  const mm = String(date.getMinutes()).padStart(2, '0')
  const ss = String(date.getSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
})

const canCancelViewerJob = computed(() => {
  const jobId = Number(jobLogViewerJobId.value)
  if (!Number.isInteger(jobId) || jobId <= 0) {
    return false
  }
  const status = normalizeJobStatus(streamJobStatus.value)
  return status === 'pending' || status === 'running'
})

function updateStreamJobStatus(status) {
  const normalized = normalizeJobStatus(status)
  if (!normalized) {
    return
  }
  streamJobStatus.value = normalized
}

function markStreamOutputUpdated() {
  streamLastOutputAt.value = Date.now()
}

function updateServerOutputTimeFromText(textChunk) {
  const text = String(textChunk || '')
  if (!text) {
    return
  }

  // Timestamp format from backend log lines:
  // [YYYY-MM-DD HH:mm:ss][host][stdout|stderr] message
  const timePattern = /\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\]\[[^\]]+\]\[(?:stdout|stderr)\]/g
  let match = null
  let lastTimestamp = ''
  while (true) {
    match = timePattern.exec(text)
    if (!match) {
      break
    }
    lastTimestamp = match[1] || ''
  }

  if (lastTimestamp) {
    streamLastOutputServerTime.value = lastTimestamp
  }
}

function resetStreamStatus() {
  streamConnectionState.value = 'idle'
  streamJobStatus.value = ''
  streamLastOutputAt.value = 0
  streamLastOutputServerTime.value = ''
}

function applyJobStatusFromList(jobId) {
  const targetJobId = Number(jobId)
  if (!Number.isFinite(targetJobId) || targetJobId <= 0) {
    return
  }
  const matched = (jobs.value || []).find((item) => Number(item.id) === targetJobId)
  if (matched?.status) {
    updateStreamJobStatus(matched.status)
  }
}

function openRuntimeTemplateViewer(record) {
  runtimeTemplateTitle.value = `运行模板 / 作业 #${record?.job_id || record?.id || '-'}`
  runtimeTemplateContent.value = getRuntimeTemplateContent(record)
  runtimeTemplateVisible.value = true
}

function closeRuntimeTemplateViewer() {
  runtimeTemplateVisible.value = false
}

async function copyRuntimeTemplate() {
  const text = runtimeTemplateContent.value || ''
  const copied = await copyTextWithFallback(text)
  if (copied) {
    message.success('运行模板已复制')
  } else {
    message.error('复制失败，请检查浏览器权限')
  }
}

function openJobHostViewer(record) {
  const hosts = getInventoryHostList(record)
  jobHostViewerHosts.value = hosts
  jobHostViewerTitle.value = `运行主机 / 作业 #${record?.job_id || record?.id || '-'}`
  jobHostViewerVisible.value = true
}

function openWorkflowRunHostViewer(record) {
  const hosts = getWorkflowRunHostList(record)
  jobHostViewerHosts.value = hosts
  jobHostViewerTitle.value = `运行主机 / Workflow运行 #${record?.id || '-'}`
  jobHostViewerVisible.value = true
}

function handleJobHostViewerHostClick(item) {
  const hostId = Number(item?.host_id || 0)
  if (Number.isInteger(hostId) && hostId > 0) {
    goToAssetHost(router, message, hostId, item?.host_name || item?.instance_name || '')
  }
}

async function openLogViewer(record) {
  if (!isJobFinished(targetDrawerJobStatus.value)) {
    message.warning('请等待日志输出完成后再查看日志')
    return
  }
  logViewerRecord.value = record
  logViewerVisible.value = true
  if (targetDrawerJobId.value) {
    applyJobStatusFromList(targetDrawerJobId.value)
    await loadJobLog(targetDrawerJobId.value)
  }
}

async function openJobLogViewer(record) {
  const targetJobId = Number(record?.id)
  if (!Number.isInteger(targetJobId) || targetJobId <= 0) {
    message.error('作业ID无效，无法打开日志')
    return
  }
  jobLogViewerJobId.value = targetJobId
  jobLogViewerVisible.value = true
  jobLogAutoFollowEnabled.value = true
  jobLogAutoFollowSuspended.value = false
  updateStreamJobStatus(record?.status)
  connectJobLogSocket(targetJobId)
  await loadJobLog(targetJobId)
  await nextTick()
  scrollJobLogToBottom(true)
}

async function openJobLogViewerById(jobId) {
  const targetJobId = Number(jobId)
  if (!Number.isInteger(targetJobId) || targetJobId <= 0) {
    return
  }
  const matched = (jobs.value || []).find((item) => Number(item.id) === targetJobId)
  await openJobLogViewer({
    id: targetJobId,
    status: matched?.status || '',
  })
}

function closeJobLogViewer() {
  jobLogViewerVisible.value = false
  clearJobIdQueryFromRoute()
  if (!logViewerVisible.value) {
    closeJobLogSocket()
  }
}

function clearJobIdQueryFromRoute() {
  const query = route.query || {}
  const currentJobId = Array.isArray(query.job_id) ? query.job_id[0] : query.job_id
  if (!currentJobId) {
    return
  }

  const nextQuery = { ...query }
  delete nextQuery.job_id
  router.replace({ path: route.path, query: nextQuery }).catch(() => {})
}

function isNearBottom(element, threshold = 24) {
  if (!element) {
    return true
  }
  const distance = element.scrollHeight - element.scrollTop - element.clientHeight
  return distance <= threshold
}

function scrollJobLogToBottom(force = false) {
  const shell = jobLogViewerShellRef.value
  if (!shell) {
    return
  }
  if (!force && (!jobLogAutoFollowEnabled.value || jobLogAutoFollowSuspended.value)) {
    return
  }
  shell.scrollTop = shell.scrollHeight
}

function handleJobLogViewerScroll() {
  const shell = jobLogViewerShellRef.value
  if (!shell || !jobLogViewerVisible.value || !jobLogAutoFollowEnabled.value) {
    return
  }
  jobLogAutoFollowSuspended.value = !isNearBottom(shell)
}

function toggleJobLogAutoFollow(checked) {
  jobLogAutoFollowEnabled.value = Boolean(checked)
  if (jobLogAutoFollowEnabled.value) {
    jobLogAutoFollowSuspended.value = false
    nextTick(() => {
      scrollJobLogToBottom(true)
    })
  }
}

function resumeJobLogAutoFollow() {
  jobLogAutoFollowEnabled.value = true
  jobLogAutoFollowSuspended.value = false
  nextTick(() => {
    scrollJobLogToBottom(true)
  })
}

function closeDetailLogViewer() {
  logViewerVisible.value = false
  if (!jobLogViewerVisible.value) {
    closeJobLogSocket()
  }
}

function increaseLogFontSize() {
  logFontSize.value = Math.min(20, logFontSize.value + 1)
}

function decreaseLogFontSize() {
  logFontSize.value = Math.max(11, logFontSize.value - 1)
}

function increaseJobLogFontSize() {
  jobLogFontSize.value = Math.min(20, jobLogFontSize.value + 1)
}

function decreaseJobLogFontSize() {
  jobLogFontSize.value = Math.max(11, jobLogFontSize.value - 1)
}

async function copyCurrentLog() {
  const text = currentLogText.value || ''
  const copied = await copyTextWithFallback(text)
  if (copied) {
    message.success('日志已复制')
  } else {
    message.error('复制失败，请检查浏览器权限')
  }
}

async function copyJobLog() {
  const text = jobLogText.value || ''
  const copied = await copyTextWithFallback(text)
  if (copied) {
    message.success('日志已复制')
  } else {
    message.error('复制失败，请检查浏览器权限')
  }
}

function downloadCurrentLog() {
  const text = currentLogText.value || ''
  const record = logViewerRecord.value || {}
  const hostName = String(record.host_name || 'host').replace(/[^\w.-]+/g, '_')
  const hostIp = String(record.host_ip || 'unknown').replace(/[^\w.-]+/g, '_')
  const jobId = String(targetDrawerJobId.value || 'job')
  const filename = `job_${jobId}_${hostName}_${hostIp}_log.log`
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  window.URL.revokeObjectURL(url)
}

function downloadJobLogText() {
  const text = jobLogText.value || ''
  const jobId = String(jobLogViewerJobId.value || 'job')
  const taskName = toSafeFileSegment(resolveTaskNameForJob(jobLogViewerJobId.value) || 'task')
  const filename = `job_${jobId}_${taskName}.log`
  triggerTextDownload(filename, text)
}

function triggerTextDownload(filename, text) {
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  window.URL.revokeObjectURL(url)
}

function resolveTaskNameForJob(jobRecordOrId) {
  if (jobRecordOrId && typeof jobRecordOrId === 'object') {
    return String(jobRecordOrId.task_name || '').trim()
  }

  const jobId = Number(jobRecordOrId)
  if (!Number.isInteger(jobId)) {
    return ''
  }

  const matched = (jobs.value || []).find((item) => Number(item.id) === jobId)
  return String(matched?.task_name || '').trim()
}

async function loadJobLog(jobId) {
  if (!jobId) {
    jobLogText.value = ''
    logViewerJobOutput.value = ''
    return
  }
  const res = await getJobLog(jobId)
  const output = normalizeUnifiedLogAliases(res?.data?.data?.job_output || '')
  const oldLength = String(jobLogText.value || '').length
  jobLogText.value = output
  logViewerJobOutput.value = output
  updateStreamJobStatus(res?.data?.data?.status)
  updateServerOutputTimeFromText(output)
  if (String(output).length > oldLength) {
    markStreamOutputUpdated()
  }
}

function closeJobLogSocket() {
  if (jobLogReconnectTimer) {
    window.clearTimeout(jobLogReconnectTimer)
    jobLogReconnectTimer = null
  }
  if (jobLogSocket) {
    jobLogSocket.onopen = null
    jobLogSocket.onmessage = null
    jobLogSocket.onerror = null
    jobLogSocket.onclose = null
    jobLogSocket.close()
    jobLogSocket = null
  }
  jobLogSocketConnected = false
  jobLogSocketJobId = null
  if (jobLogViewerVisible.value || logViewerVisible.value) {
    streamConnectionState.value = 'disconnected'
  } else {
    resetStreamStatus()
  }
}

function shouldStreamForJob(jobId) {
  if (!jobId) {
    return false
  }
  const unifiedMatch = jobLogViewerVisible.value && Number(jobLogViewerJobId.value) === Number(jobId)
  const detailMatch = logViewerVisible.value && Number(targetDrawerJobId.value) === Number(jobId)
  return unifiedMatch || detailMatch
}

function getFallbackStreamJobId() {
  if (jobLogViewerVisible.value && jobLogViewerJobId.value) {
    return jobLogViewerJobId.value
  }
  if (logViewerVisible.value && targetDrawerJobId.value) {
    return targetDrawerJobId.value
  }
  return null
}

function scheduleJobLogReconnect(jobId) {
  if (!shouldStreamForJob(jobId)) {
    return
  }
  if (jobLogReconnectTimer) {
    return
  }
  jobLogReconnectTimer = window.setTimeout(() => {
    jobLogReconnectTimer = null
    connectJobLogSocket(jobId)
  }, 1200)
}

function connectJobLogSocket(jobId) {
  if (!jobId) {
    return
  }
  if (jobLogSocket && jobLogSocketConnected && Number(jobLogSocketJobId) === Number(jobId)) {
    return
  }
  closeJobLogSocket()
  jobLogSocketJobId = Number(jobId)
  streamConnectionState.value = 'connecting'
  applyJobStatusFromList(jobId)

  const token = (getToken() || '').trim()
  if (!token) {
    streamConnectionState.value = 'error'
    jobLogSocketJobId = null
    return
  }

  const wsUrl = `${getWebSocketBaseUrl()}/ws/automation/jobs/${jobId}/logs/?token=${encodeURIComponent(token)}`
  const socket = new WebSocket(wsUrl)
  jobLogSocket = socket

  socket.onopen = () => {
    jobLogSocketConnected = true
    streamConnectionState.value = 'connected'
  }

  socket.onmessage = (event) => {
    try {
      const messageData = JSON.parse(event.data || '{}')
      const type = messageData?.type
      const payload = messageData?.data || {}

      if (type === 'snapshot') {
        const snapshot = normalizeUnifiedLogAliases(payload.data || '')
        jobLogText.value = snapshot
        logViewerJobOutput.value = snapshot
        updateServerOutputTimeFromText(snapshot)
        if (snapshot.trim()) {
          markStreamOutputUpdated()
        }
      } else if (type === 'output') {
        const delta = String(payload.data || '')
        const merged = normalizeUnifiedLogAliases(`${jobLogText.value}${delta}`)
        jobLogText.value = merged
        logViewerJobOutput.value = merged
        updateServerOutputTimeFromText(merged)
        if (delta.trim()) {
          markStreamOutputUpdated()
        }
      } else if (type === 'status') {
        updateStreamJobStatus(payload.status)
      } else if (type === 'completed') {
        updateStreamJobStatus(payload.status)
      }
    } catch (error) {
      // Ignore malformed websocket payloads.
    }
  }

  socket.onerror = () => {
    jobLogSocketConnected = false
    streamConnectionState.value = 'error'
  }

  socket.onclose = () => {
    jobLogSocketConnected = false
    const status = normalizeJobStatus(streamJobStatus.value)
    // 如果任务已完成（success/failed/cancelled），则不重新连接
    const isJobCompleted = ['success', 'failed', 'cancelled'].includes(status)
    streamConnectionState.value = !isJobCompleted && shouldStreamForJob(jobId) ? 'reconnecting' : 'disconnected'
    if (!isJobCompleted && shouldStreamForJob(jobId)) {
      scheduleJobLogReconnect(jobId)
    }
  }
}

async function downloadJobLog(jobRecord) {
  downloadingJobLogId.value = jobRecord.id
  try {
    const res = await getJobLog(jobRecord.id)
    const content = res?.data?.data?.job_output || 'No unified logs.'
    const taskName = toSafeFileSegment(resolveTaskNameForJob(jobRecord) || 'task')
    const filename = `job_${jobRecord.id}_${taskName}.log`
    triggerTextDownload(filename, content)
    message.success('任务日志下载成功')
  } catch (error) {
    message.error(error?.message || '任务日志下载失败')
  } finally {
    downloadingJobLogId.value = null
  }
}

function formatDateTime(value) {
  if (!value) {
    return '-'
  }
  return formatTimeWithTimezone(normalizeUtcTime(value), store.state.user?.timezone || 'Asia/Shanghai', 'YYYY-MM-DD HH:mm:ss')
}

function openWorkflowRunStatus(record) {
  const runId = Number(record?.id)
  if (!Number.isInteger(runId) || runId <= 0) {
    message.warning('无效的运行记录ID')
    return
  }
  router.push({ path: '/sys/automation/workflow/run', query: { run_id: String(runId) } })
}

async function cancelWorkflowRunRecord(record) {
  const runId = Number(record?.id)
  if (!Number.isInteger(runId) || runId <= 0) {
    return
  }
  workflowRunCancelingId.value = runId
  try {
    await cancelWorkflowRun(runId)
    message.success('Workflow 运行已取消')
    await loadWorkflowRuns(false)
  } catch (error) {
    message.error(error?.message || '取消 Workflow 运行失败')
  } finally {
    workflowRunCancelingId.value = null
  }
}

async function loadWorkflowRuns(resetPage = false) {
  if (resetPage) {
    workflowRunPagination.current = 1
  }
  workflowRunLoading.value = true
  try {
    const params = {
      page: workflowRunPagination.current,
      page_size: workflowRunPagination.pageSize,
    }
    const kw = String(workflowRunKeyword.value || '').trim()
    if (kw) {
      params.search = kw
    }
    if (workflowRunStatus.value) {
      params.status = workflowRunStatus.value
    }
    if (workflowRunTimeRange.value && workflowRunTimeRange.value.length === 2) {
      params.start_time_after = toUtcQueryISOStringByUserTimezone(workflowRunTimeRange.value[0], getActiveUserTimezone())
      params.start_time_before = toUtcQueryISOStringByUserTimezone(workflowRunTimeRange.value[1], getActiveUserTimezone())
    }
    params.ordering = resolveWorkflowRunOrdering()
    const res = await getWorkflowRunList(params)
    const data = res?.data?.data || {}
    workflowRuns.value = Array.isArray(data.results) ? data.results : []
    workflowRunPagination.total = Number(data.count || 0)
  } finally {
    workflowRunLoading.value = false
  }
}

function handleWorkflowRunTableChange(page, _filters, sorter) {
  workflowRunPagination.current = page.current
  workflowRunPagination.pageSize = page.pageSize

  const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
  const allowedFields = ['id', 'status', 'start_time', 'duration_seconds']
  if (nextSorter?.field && allowedFields.includes(nextSorter.field) && nextSorter.order) {
    workflowRunSort.field = nextSorter.field
    workflowRunSort.order = nextSorter.order
  } else {
    workflowRunSort.field = null
    workflowRunSort.order = null
  }

  loadWorkflowRuns(false)
}

function resetWorkflowRunFilters() {
  workflowRunKeyword.value = ''
  workflowRunStatus.value = undefined
  workflowRunTimeRange.value = []
  loadWorkflowRuns(true)
}

async function loadMonitorInstallHistories(resetPage = false) {
  if (resetPage) {
    monitorInstallHistoryPagination.current = 1
  }
  monitorInstallHistoryLoading.value = true
  try {
    const startTime = Array.isArray(monitorInstallHistoryTimeRange.value) && monitorInstallHistoryTimeRange.value[0]
      ? toUtcQueryISOStringByUserTimezone(monitorInstallHistoryTimeRange.value[0], getActiveUserTimezone())
      : undefined
    const endTime = Array.isArray(monitorInstallHistoryTimeRange.value) && monitorInstallHistoryTimeRange.value[1]
      ? toUtcQueryISOStringByUserTimezone(monitorInstallHistoryTimeRange.value[1], getActiveUserTimezone())
      : undefined

    const params = {
      page: monitorInstallHistoryPagination.current,
      page_size: monitorInstallHistoryPagination.pageSize,
      ordering: '-id',
      keyword: String(monitorInstallHistoryKeyword.value || '').trim() || undefined,
      status: monitorInstallHistoryStatus.value || undefined,
      action: monitorInstallHistoryAction.value || undefined,
      target_id: monitorInstallHistoryTargetType.value === 'fluent_bit'
        ? undefined
        : String(monitorInstallHistoryTargetId.value || '').trim() || undefined,
      log_collection_target_id: monitorInstallHistoryTargetType.value === 'fluent_bit'
        ? String(monitorInstallHistoryTargetId.value || '').trim() || undefined
        : undefined,
      start_time: startTime,
      end_time: endTime,
    }
    const res = await getMonitorInstallHistoryList(params)
    const data = res?.data?.data || {}
    monitorInstallHistoryRows.value = Array.isArray(data.results) ? data.results : []
    monitorInstallHistoryPagination.total = Number(data.count || 0)
  } catch (error) {
    message.error(error?.message || '加载监控安装历史失败')
    monitorInstallHistoryRows.value = []
  } finally {
    monitorInstallHistoryLoading.value = false
  }
}

function handleMonitorInstallHistoryTableChange(page) {
  monitorInstallHistoryPagination.current = Number(page?.current || 1)
  monitorInstallHistoryPagination.pageSize = Number(page?.pageSize || 10)
  loadMonitorInstallHistories(false)
}

async function openMonitorHistoryDetail(record) {
  const historyId = Number(record?.id)
  if (!Number.isInteger(historyId) || historyId <= 0) {
    message.warning('历史记录ID无效')
    return
  }
  monitorHistoryDetailVisible.value = true
  monitorHistoryDetailLoading.value = true
  try {
    const res = await getMonitorInstallHistoryDetail(historyId)
    const data = res?.data?.data || {}
    monitorHistoryDetailRecord.value = data
    const chunks = []
    if (data.summary_message) {
      chunks.push(`[summary]\n${String(data.summary_message).trimEnd()}`)
    }
    if (data.stdout_snapshot) {
      chunks.push(`[stdout]\n${String(data.stdout_snapshot).trimEnd()}`)
    }
    if (data.stderr_snapshot) {
      chunks.push(`[stderr]\n${String(data.stderr_snapshot).trimEnd()}`)
    }
    if (data.error_message_snapshot) {
      chunks.push(`[error]\n${String(data.error_message_snapshot).trimEnd()}`)
    }
    monitorHistoryDetailText.value = chunks.filter(Boolean).join('\n\n')
  } catch (error) {
    monitorHistoryDetailRecord.value = null
    monitorHistoryDetailText.value = ''
    message.error(error?.message || '加载历史详情失败')
  } finally {
    monitorHistoryDetailLoading.value = false
  }
}

function cancelMonitorHistory(record) {
  const historyId = Number(record?.id)
  if (!Number.isInteger(historyId) || !['pending', 'running'].includes(String(record?.status || '').toLowerCase())) {
    return
  }

  Modal.confirm({
    title: '确认取消监控任务？',
    content: `主机 ${record?.host_name || record?.host_ip || '-'} 的${record?.action === 'install' ? '安装' : '卸载'}任务将停止继续执行。`,
    okText: '确认取消',
    cancelText: '返回',
    onOk: async () => {
      try {
        await cancelMonitorInstallHistory(historyId)
        message.success('监控任务已取消')
        await loadMonitorInstallHistories(false)
      } catch (error) {
        message.error(error?.message || '取消监控任务失败')
      }
    },
  })
}

function resolveMonitorHistorySourceJobId(detail) {
  const rawValue = detail?.automation_job_id_snapshot
  const parsed = Number(rawValue)
  if (Number.isInteger(parsed) && parsed > 0) {
    return parsed
  }
  return null
}

async function probeMonitorHistorySourceJob(detail) {
  monitorHistorySourceJobId.value = resolveMonitorHistorySourceJobId(detail)
  monitorHistorySourceJobExists.value = false
  if (!monitorHistorySourceJobId.value) {
    return
  }

  monitorHistorySourceJobChecking.value = true
  try {
    const res = await getJobDetail(monitorHistorySourceJobId.value)
    if (Number(res?.data?.code) === 200 && res?.data?.data) {
      monitorHistorySourceJobExists.value = true
    }
  } catch (_error) {
    monitorHistorySourceJobExists.value = false
  } finally {
    monitorHistorySourceJobChecking.value = false
  }
}

function jumpToMonitorHistorySourceJob() {
  const sourceJobId = Number(monitorHistorySourceJobId.value)
  if (!Number.isInteger(sourceJobId) || sourceJobId <= 0 || !monitorHistorySourceJobExists.value) {
    return
  }

  closeMonitorHistoryDetail()
  activeRecordTab.value = 'job'
  // 跳回自动化任务记录时，按作业ID精确过滤并清空其他条件，避免命中过去筛选导致看不到目标记录。
  jobRecordId.value = String(sourceJobId)
  jobKeyword.value = ''
  selectedJobStatus.value = null
  jobOutputKeyword.value = ''
  selectedTaskId.value = null
  selectedTaskName.value = ''
  jobTimeRange.value = []
  loadJobs(true)
  router.replace({ path: route.path, query: { tab: 'job', job_id: String(sourceJobId) } }).catch(() => {})
}

function closeMonitorHistoryDetail() {
  monitorHistoryDetailVisible.value = false
  monitorHistoryDetailRecord.value = null
  monitorHistoryDetailText.value = ''
}

function increaseMonitorHistoryDetailFontSize() {
  monitorHistoryDetailFontSize.value = Math.min(20, monitorHistoryDetailFontSize.value + 1)
}

function decreaseMonitorHistoryDetailFontSize() {
  monitorHistoryDetailFontSize.value = Math.max(11, monitorHistoryDetailFontSize.value - 1)
}

async function copyMonitorHistoryDetail() {
  const copied = await copyTextWithFallback(monitorHistoryDetailText.value || '')
  if (copied) {
    message.success('历史日志已复制')
  } else {
    message.error('复制失败，请检查浏览器权限')
  }
}

function clearMonitorInstallHistoryFilters() {
  monitorInstallHistoryKeyword.value = ''
  monitorInstallHistoryStatus.value = undefined
  monitorInstallHistoryAction.value = undefined
  monitorInstallHistoryTargetId.value = ''
  monitorInstallHistoryTargetType.value = ''
  monitorInstallHistoryTimeRange.value = []
  loadMonitorInstallHistories(true)
}

async function loadTaskOptions() {
  const res = await getTaskList({ page: 1, page_size: 300, ordering: '-id' })
  const data = res?.data?.data || {}
  const records = data.results || []
  const nextNameMap = {}
  taskOptions.value = records.map((item) => {
    const label = `${item.name}`
    nextNameMap[item.id] = item.name
    return {
      value: item.id,
      label,
    }
  })
  taskNameMap.value = nextNameMap
}

function onJobRecordIdSearch(value) {
  // 按执行记录ID精确搜索，清空其他过滤条件
  jobKeyword.value = ''
  selectedJobStatus.value = null
  jobOutputKeyword.value = ''
  selectedTaskId.value = null
  selectedTaskName.value = ''
  jobRecordId.value = value.trim()
  loadJobs(true)
}

function onTaskFilterChange(value) {
  if (value) {
    selectedTaskName.value = taskNameMap.value[value] || ''
  } else {
    selectedTaskName.value = ''
  }
  loadJobs(true)
}

async function loadJobs(resetPage = false) {
  if (resetPage) {
    jobPagination.current = 1
  }
  jobLoading.value = true
  try {
    const startTimeFrom = Array.isArray(jobTimeRange.value) && jobTimeRange.value[0]
      ? toUtcQueryISOStringByUserTimezone(jobTimeRange.value[0], getActiveUserTimezone())
      : undefined
    const startTimeTo = Array.isArray(jobTimeRange.value) && jobTimeRange.value[1]
      ? toUtcQueryISOStringByUserTimezone(jobTimeRange.value[1], getActiveUserTimezone())
      : undefined

    const res = await getJobList({
      page: jobPagination.current,
      page_size: jobPagination.pageSize,
      ordering: resolveJobOrdering(),
      ...(jobRecordId.value ? { job_id: jobRecordId.value } : {}),
      ...(jobKeyword.value && !jobRecordId.value ? { keyword: jobKeyword.value } : {}),
      status: selectedJobStatus.value || undefined,
      output_keyword: jobOutputKeyword.value || undefined,
      task_id: selectedTaskId.value || undefined,
      start_time_from: startTimeFrom,
      start_time_to: startTimeTo,
    })
    const data = res?.data?.data || {}
    jobs.value = data.results || []
    jobPagination.total = data.count || 0
    const fallbackJobId = getFallbackStreamJobId()
    if (fallbackJobId) {
      applyJobStatusFromList(fallbackJobId)
    }
  } finally {
    jobLoading.value = false
  }
}



async function onCancelJob(record) {
  const jobId = Number(record?.id)
  if (!Number.isInteger(jobId) || jobId <= 0) {
    return
  }

  cancellingJobId.value = jobId
  try {
    await cancelJob(jobId)
    message.success('任务已取消')
    await loadJobs(false)
  } catch (error) {
    message.error(error?.message || '取消任务失败')
  } finally {
    cancellingJobId.value = null
  }
}

async function onCancelViewerJob() {
  const jobId = Number(jobLogViewerJobId.value)
  if (!Number.isInteger(jobId) || jobId <= 0 || !canCancelViewerJob.value) {
    return
  }
  await onCancelJob({ id: jobId })
  updateStreamJobStatus('cancelled')
}

function handleJobTableChange(page, _filters, sorter) {
  jobPagination.current = page.current
  jobPagination.pageSize = page.pageSize

  const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
  const allowedFields = ['job_id', 'status', 'start_time', 'duration_seconds']
  if (nextSorter?.field && allowedFields.includes(nextSorter.field) && nextSorter.order) {
    jobSort.field = nextSorter.field
    jobSort.order = nextSorter.order
  } else {
    jobSort.field = null
    jobSort.order = null
  }

  loadJobs(false)
}

function clearTaskFilter() {
  selectedTaskId.value = null
  selectedTaskName.value = ''
  loadJobs(true)
}

function reloadPage() {
  if (activeRecordTab.value === 'monitor_history') {
    loadMonitorInstallHistories(false)
    return
  }
  if (activeRecordTab.value === 'workflow') {
    loadWorkflowRuns(false)
    return
  }
  loadJobs(false)
}

function stopPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function goTaskCenter() {
  router.push('/sys/automation')
}

function goWorkflowCenter() {
  router.push('/sys/automation/workflow')
}

function handleRecordTabChange(key) {
  const activeKey = String(key || 'job')
  if (activeKey === 'monitor_history' && monitorInstallHistoryRows.value.length === 0) {
    loadMonitorInstallHistories(true)
  }
  if (activeKey === 'workflow' && workflowRuns.value.length === 0) {
    loadWorkflowRuns(true)
  }
  // 切换标签后重置轮询，确保两类记录都按统一配置间隔自动刷新。
  startPolling()
}



function startPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
  }
  const pollingIntervalMs = automationLogsRefreshIntervalSeconds.value * 1000
  pollTimer = window.setInterval(() => {
    streamClockTick.value = Date.now()
    if (activeRecordTab.value === 'monitor_history') {
      loadMonitorInstallHistories(false)
      return
    }
    if (activeRecordTab.value === 'workflow') {
      loadWorkflowRuns(false)
      return
    }
    
    // 如果通过 job_id 精确搜索且任务已完成，停止轮询
    if (jobRecordId.value && jobs.value.length > 0) {
      const job = jobs.value[0]
      if (isJobFinished(job?.status)) {
        stopPolling()
        return
      }
    }
    
    loadJobs(false)
  }, pollingIntervalMs)

  if (streamClockTimer) {
    window.clearInterval(streamClockTimer)
  }
  streamClockTimer = window.setInterval(() => {
    streamClockTick.value = Date.now()
  }, 1000)
}

watch(jobLogText, async () => {
  if (!jobLogViewerVisible.value) {
    return
  }
  await nextTick()
  scrollJobLogToBottom(false)
})

watch(jobLogViewerVisible, async (visible) => {
  if (!visible) {
    return
  }
  await nextTick()
  scrollJobLogToBottom(true)
})

watch(() => route.query.job_id, (jobID, previousJobID) => {
  if (route.path !== '/sys/automation/logs' || !jobID || jobID === previousJobID) {
    return
  }
  // This view is cached by keep-alive, so a Task run can navigate here without
  // re-running onMounted. Apply the exact Job filter and load it immediately.
  jobRecordId.value = String(Array.isArray(jobID) ? jobID[0] : jobID).trim()
  jobKeyword.value = ''
  selectedJobStatus.value = null
  jobOutputKeyword.value = ''
  selectedTaskId.value = null
  selectedTaskName.value = ''
  loadJobs(true)
})

onMounted(async () => {
  refreshLogsTimeRangePresets()
  const queryTab = String(route.query.tab || '').trim()
  if (queryTab === 'monitor_history') {
    activeRecordTab.value = 'monitor_history'
  }
  if (queryTab === 'workflow') {
    activeRecordTab.value = 'workflow'
  }

  const queryTaskId = route.query.task_id
  const queryTaskName = route.query.task_name
  const queryKeyword = route.query.keyword
  const queryJobId = route.query.job_id
  const queryOpenLog = String(route.query.open_log || '').trim().toLowerCase()

  if (activeRecordTab.value === 'monitor_history') {
    const queryTargetId = route.query.target_id
    const queryLogCollectionTargetId = route.query.log_collection_target_id
    const queryHistoryId = route.query.history_id
    if (queryLogCollectionTargetId) {
      monitorInstallHistoryTargetType.value = 'fluent_bit'
      monitorInstallHistoryTargetId.value = String(
        Array.isArray(queryLogCollectionTargetId) ? queryLogCollectionTargetId[0] : queryLogCollectionTargetId,
      ).trim()
    } else if (queryTargetId) {
      monitorInstallHistoryTargetType.value = 'exporter'
      monitorInstallHistoryTargetId.value = String(Array.isArray(queryTargetId) ? queryTargetId[0] : queryTargetId).trim()
    }
    if (queryKeyword) {
      monitorInstallHistoryKeyword.value = String(queryKeyword).trim()
    }
    await loadAutomationLogsRefreshIntervalConfig()
    await loadMonitorInstallHistories(true)
    const parsedHistoryId = Number(Array.isArray(queryHistoryId) ? queryHistoryId[0] : queryHistoryId)
    if (Number.isInteger(parsedHistoryId) && parsedHistoryId > 0) {
      await openMonitorHistoryDetail({ id: parsedHistoryId })
    }
    startPolling()
    return
  }

  if (activeRecordTab.value === 'workflow') {
    if (queryKeyword) {
      workflowRunKeyword.value = String(queryKeyword).trim()
    }
    await loadAutomationLogsRefreshIntervalConfig()
    await loadWorkflowRuns(true)
    startPolling()
    return
  }
  
  // 优先级：job_id > keyword > 其他参数
  if (queryJobId) {
    jobRecordId.value = String(queryJobId).trim()
    selectedTaskId.value = null
    selectedTaskName.value = ''
  } else if (queryKeyword) {
    jobKeyword.value = String(queryKeyword).trim()
  } else {
    // 否则按原有逻辑处理其他参数
    if (queryTaskId && String(queryTaskId).trim()) {
      const parsedId = Number(queryTaskId)
      selectedTaskId.value = Number.isInteger(parsedId) && parsedId > 0 ? parsedId : null
    }
    if (queryTaskName) {
      selectedTaskName.value = String(queryTaskName)
    }
  }

  await loadTaskOptions()
  await loadAutomationLogsRefreshIntervalConfig()
  if (selectedTaskId.value && !selectedTaskName.value) {
    selectedTaskName.value = taskNameMap.value[selectedTaskId.value] || ''
  }
  await loadJobs(true)
  
  // 兼容两种自动打开日志场景：
  // 1) 旧逻辑：带 job_id + task_id（原日志按钮）；
  // 2) 新逻辑：带 job_id + open_log=1（纳管目标“查看日志”优先直达）。
  const shouldAutoOpenLog = Boolean(
    queryJobId && (
      (queryTaskId && String(queryTaskId).trim())
      || queryOpenLog === '1'
      || queryOpenLog === 'true'
    )
  )
  if (activeRecordTab.value === 'job' && shouldAutoOpenLog) {
    const parsedJobId = Number(Array.isArray(queryJobId) ? queryJobId[0] : queryJobId)
    if (Number.isInteger(parsedJobId) && parsedJobId > 0) {
      await openJobLogViewerById(parsedJobId)
    }
  }
  
  startPolling()
})

useKeepAliveRefreshLifecycle(() => {
  startPolling()
  // 日志弹窗还开着且任务没结束时，恢复实时日志的 WebSocket 推送；close 时同理必须断开，
  // 否则切走 tab 后连接仍在后台收消息、占着服务端资源。
  const resumeJobId = getFallbackStreamJobId()
  if (resumeJobId) {
    connectJobLogSocket(resumeJobId)
  }
}, () => {
  stopPolling()
  if (streamClockTimer) {
    window.clearInterval(streamClockTimer)
    streamClockTimer = null
  }
  closeJobLogSocket()
})

onBeforeUnmount(() => {
  closeJobLogSocket()
  if (pollTimer) {
    window.clearInterval(pollTimer)
  }
  if (streamClockTimer) {
    window.clearInterval(streamClockTimer)
  }
})

  return {
    getPopupContainer,
    activeRecordTab,
    handleRecordTabChange,
    jobRecordId,
    onJobRecordIdSearch,
    jobKeyword,
    loadJobs,
    selectedJobStatus,
    jobStatusOptions,
    jobOutputKeyword,
    selectedTaskId,
    taskOptions,
    onTaskFilterChange,
    jobTimeRange,
    logsTimeRangeShowTime,
    logsTimeRangePresets,
    onLogsTimeRangeOpenChange,
    selectedTaskName,
    clearTaskFilter,
    jobLoading,
    reloadPage,
    goTaskCenter,
    jobColumns,
    jobs,
    jobPagination,
    handleJobTableChange,
    statusColor,
    formatDateTime,
    formatRuntimeTemplateLabel,
    openRuntimeTemplateViewer,
    getInventoryHostList,
    openJobHostViewer,
    isJobFinished,
    openJobLogViewer,
    canDownloadJobLog,
    downloadingJobLogId,
    downloadJobLog,
    onCancelJob,
    workflowRunKeyword,
    loadWorkflowRuns,
    workflowRunStatus,
    workflowRunStatusOptions,
    workflowRunTimeRange,
    hasWorkflowRunFilters,
    resetWorkflowRunFilters,
    workflowRunLoading,
    goWorkflowCenter,
    workflowRunColumns,
    workflowRuns,
    workflowRunPagination,
    handleWorkflowRunTableChange,
    getWorkflowRunStatusColor,
    formatWorkflowDuration,
    getWorkflowRunHostList,
    openWorkflowRunHostViewer,
    openWorkflowRunStatus,
    canCancelWorkflowRunRecord,
    cancelWorkflowRunRecord,
    workflowRunCancelingId,
    monitorInstallHistoryRows,
    monitorInstallHistoryLoading,
    monitorInstallHistoryKeyword,
    monitorInstallHistoryStatus,
    monitorInstallHistoryAction,
    monitorInstallHistoryTargetId,
    monitorInstallHistoryTimeRange,
    monitorInstallHistoryStatusOptions,
    monitorInstallHistoryActionOptions,
    monitorInstallHistoryColumns,
    monitorInstallHistoryPagination,
    loadMonitorInstallHistories,
    clearMonitorInstallHistoryFilters,
    handleMonitorInstallHistoryTableChange,
    openMonitorHistoryDetail,
    cancelMonitorHistory,
    monitorHistoryDetailVisible,
    monitorHistoryDetailLoading,
    monitorHistoryDetailHtml,
    monitorHistoryDetailStatus,
    monitorHistoryDetailLastOutput,
    monitorHistoryDetailWrap,
    monitorHistoryDetailFontSize,
    monitorHistorySourceJobExists,
    monitorHistorySourceJobChecking,
    closeMonitorHistoryDetail,
    increaseMonitorHistoryDetailFontSize,
    decreaseMonitorHistoryDetailFontSize,
    copyMonitorHistoryDetail,
    jumpToMonitorHistorySourceJob,
    openLogViewer,
    logViewerHostLabel,
    logViewerVisible,
    closeDetailLogViewer,
    streamStatusTagColor,
    streamStatusLabel,
    streamLastOutputText,
    logWrap,
    logFontSize,
    decreaseLogFontSize,
    increaseLogFontSize,
    copyCurrentLog,
    downloadCurrentLog,
    currentLogText,
    currentLogHtml,
    jobLogViewerJobId,
    jobLogViewerVisible,
    bindJobLogViewerShell,
    closeJobLogViewer,
    jobLogViewerShellRef,
    handleJobLogViewerScroll,
    jobLogWrap,
    jobLogAutoFollowEnabled,
    toggleJobLogAutoFollow,
    jobLogAutoFollowSuspended,
    resumeJobLogAutoFollow,
    canCancelViewerJob,
    cancellingJobId,
    onCancelViewerJob,
    copyJobLog,
    downloadJobLogText,
    jobLogFontSize,
    decreaseJobLogFontSize,
    increaseJobLogFontSize,
    jobLogText,
    jobLogHtml,
    jobHostViewerVisible,
    jobHostViewerTitle,
    jobHostViewerHosts,
    handleJobHostViewerHostClick,
    runtimeTemplateVisible,
    runtimeTemplateTitle,
    closeRuntimeTemplateViewer,
    copyRuntimeTemplate,
    runtimeTemplateContent,
  }
}
