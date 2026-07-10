<template>
  <div class="automation-logs-page">
    <a-card title="任务运行记录列表" size="small" class="block-card jobs-card">
      <template #extra>
        <a-space>
          <a-input-search
            v-model:value="jobRecordId"
            placeholder="按执行记录ID搜索"
            allow-clear
            @search="onJobRecordIdSearch"
          />
          <a-input-search
            v-model:value="jobKeyword"
            placeholder="搜索任务ID/发起人"
            allow-clear
            @search="loadJobs(true)"
          />
          <a-select
            v-model:value="selectedJobStatus"
            :options="jobStatusOptions"
            allow-clear
            placeholder="按任务状态过滤"
            style="width: 180px"
            @change="loadJobs(true)"
          />
          <a-input-search
            v-model:value="jobOutputKeyword"
            placeholder="按统一日志过滤"
            allow-clear
            @search="loadJobs(true)"
          />
          <a-select
            v-model:value="selectedTaskId"
            :options="taskOptions"
            allow-clear
            show-search
            optionFilterProp="label"
            placeholder="按任务过滤"
            style="width: 260px"
            @change="onTaskFilterChange"
          />
          <a-range-picker
            v-model:value="jobTimeRange"
            show-time
            format="YYYY-MM-DD HH:mm:ss"
            @change="loadJobs(true)"
          />
          <a-tag v-if="selectedTaskName" color="blue">任务: {{ selectedTaskName }}</a-tag>
          <a-button v-if="selectedTaskId" type="link" @click="clearTaskFilter">清除筛选</a-button>
          <a-button type="primary" ghost :loading="jobLoading" @click="reloadPage">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="jobLoading" />
            <span>&nbsp;刷新</span>
          </a-button>
          <a-button type="primary" ghost @click="goTaskCenter">返回任务中心</a-button>
        </a-space>
      </template>
      <a-table
        :columns="jobColumns"
        :data-source="jobs"
        :loading="jobLoading"
        :pagination="jobPagination"
        rowKey="id"
        size="small"
        @change="handleJobTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">
              {{ record.status }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'start_time'">
            {{ formatDateTime(record.start_time) }}
          </template>
          <template v-else-if="column.key === 'duration_seconds'">
            {{ record.duration_seconds ? `${record.duration_seconds.toFixed(2)}s` : '-' }}
          </template>
          <template v-else-if="column.key === 'runtime_template'">
            <a-button type="link" size="small" class="runtime-template-link" @click="openRuntimeTemplateViewer(record)">
              {{ formatRuntimeTemplateLabel(record) }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'inventory_hosts'">
            <a-space v-if="getInventoryHostList(record).length > 0" size="small">
              <span>{{ getInventoryHostList(record).length }}台主机</span>
              <a-button type="link" size="small" class="job-host-list-preview" @click="openJobHostViewer(record)">查看</a-button>
            </a-space>
            <span v-else>0台主机</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button
                size="small"
                :disabled="!isJobFinished(record.status)"
                @click="showTargets(record)"
                v-permission="'automation:targets:view'"
              >
                详细日志
              </a-button>
              <a-button size="small" @click="openJobLogViewer(record)" v-permission="'automation:jobs:view'">
                统一日志
              </a-button>
              <a-button
                v-if="canDownloadJobLog(record)"
                size="small"
                :loading="downloadingJobLogId === record.id"
                @click="downloadJobLog(record)"
                v-permission="'automation:targets:view'"
              >
                下载日志
              </a-button>
              <a-button
                size="small"
                danger
                v-if="record.status === 'pending' || record.status === 'running'"
                @click="onCancelJob(record)"
                v-permission="'automation:jobs:cancel'"
              >
                取消
              </a-button>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer
      :title="`任务目标明细 - #${targetDrawerJobId || ''}`"
      :open="targetDrawerVisible"
      width="960"
      @close="closeTargetDrawer"
    >
      <a-table
        :columns="targetColumns"
        :data-source="targets"
        :loading="targetLoading"
        :pagination="false"
        rowKey="id"
        size="small"
        :scroll="{ x: 900, y: 560 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'log_viewer'">
            <a-button
              type="link"
              size="small"
              :disabled="!isJobFinished(targetDrawerJobStatus)"
              @click="openLogViewer(record)"
            >查看日志</a-button>
          </template>
        </template>
      </a-table>
    </a-drawer>

    <a-drawer
      :title="`日志查看 / ${logViewerHostLabel}`"
      :open="logViewerVisible"
      :width="'88vw'"
      @close="closeDetailLogViewer"
    >
      <a-space class="log-toolbar" wrap>
        <a-tag :color="streamStatusTagColor">{{ streamStatusLabel }}</a-tag>
        <span class="log-stream-meta">最后输出: {{ streamLastOutputText }}</span>
        <a-space size="small">
          <span>自动换行</span>
          <a-switch v-model:checked="logWrap" size="small" />
        </a-space>
        <a-space size="small">
          <a-button size="small" @click="decreaseLogFontSize">A-</a-button>
          <span>{{ logFontSize }}px</span>
          <a-button size="small" @click="increaseLogFontSize">A+</a-button>
        </a-space>
        <a-button size="small" @click="copyCurrentLog">复制</a-button>
        <a-button size="small" @click="downloadCurrentLog">下载</a-button>
      </a-space>
      <div class="log-viewer-shell">
        <pre
          :class="['log-viewer-content', { 'is-nowrap': !logWrap }]"
          :style="{ fontSize: `${logFontSize}px` }"
        >{{ currentLogText || '-' }}</pre>
      </div>
    </a-drawer>

    <a-drawer
      :title="`统一日志 / 作业 #${jobLogViewerJobId || ''}`"
      :open="jobLogViewerVisible"
      :width="'88vw'"
      @close="closeJobLogViewer"
    >
      <a-space class="log-toolbar" wrap>
        <a-tag :color="streamStatusTagColor">{{ streamStatusLabel }}</a-tag>
        <span class="log-stream-meta">最后输出: {{ streamLastOutputText }}</span>
        <a-space size="small">
          <span>自动换行</span>
          <a-switch v-model:checked="jobLogWrap" size="small" />
        </a-space>
        <a-space size="small">
          <span>自动追尾</span>
          <a-switch :checked="jobLogAutoFollowEnabled" size="small" @change="toggleJobLogAutoFollow" />
        </a-space>
        <a-space size="small">
          <a-button size="small" @click="decreaseJobLogFontSize">A-</a-button>
          <span>{{ jobLogFontSize }}px</span>
          <a-button size="small" @click="increaseJobLogFontSize">A+</a-button>
        </a-space>
        <a-button
          v-if="jobLogAutoFollowEnabled && jobLogAutoFollowSuspended"
          size="small"
          type="primary"
          ghost
          @click="resumeJobLogAutoFollow"
        >回到底部</a-button>
        <a-button
          size="small"
          danger
          :loading="cancellingJobId === Number(jobLogViewerJobId)"
          :disabled="!canCancelViewerJob"
          @click="onCancelViewerJob"
          v-permission="'automation:jobs:cancel'"
        >取消任务</a-button>
        <a-button size="small" @click="copyJobLog">复制</a-button>
        <a-button size="small" @click="downloadJobLogText">下载</a-button>
      </a-space>
      <div
        ref="jobLogViewerShellRef"
        class="log-viewer-shell"
        @scroll="handleJobLogViewerScroll"
      >
        <pre
          :class="['log-viewer-content', { 'is-nowrap': !jobLogWrap }]"
          :style="{ fontSize: `${jobLogFontSize}px` }"
        >{{ jobLogText || '-' }}</pre>
      </div>
    </a-drawer>

    <a-modal
      :open="jobHostViewerVisible"
      :title="jobHostViewerTitle"
      :width="980"
      :footer="null"
      @cancel="jobHostViewerVisible = false"
    >
      <div class="scope-editor__desc">仅展示当前作业运行范围（主机组与主机），未命中节点已隐藏</div>
      <div class="scope-viewer__summary">当前范围：{{ jobHostViewerHostCount }}台主机</div>
      <a-input
        v-model:value="jobHostViewerKeyword"
        allow-clear
        placeholder="搜索实例名/IP"
        class="job-host-viewer-search"
      />
      <div class="scope-editor__tree-wrap">
        <a-tree
          v-if="filteredJobHostViewerTreeData.length > 0"
          block-node
          :expanded-keys="jobHostViewerExpandedKeys"
          :auto-expand-parent="true"
          :tree-data="filteredJobHostViewerTreeData"
          :selectable="false"
          :show-line="{ showLeafIcon: false }"
        >
          <template #title="node">
            <span>{{ getTreeNodeTitle(node) }}</span>
          </template>
        </a-tree>
        <a-empty v-else description="未匹配到主机" />
      </div>
    </a-modal>

    <a-modal
      :open="runtimeTemplateVisible"
      :title="runtimeTemplateTitle"
      width="920px"
      :footer="null"
      @cancel="closeRuntimeTemplateViewer"
    >
      <div class="runtime-template-toolbar">
        <a-button size="small" @click="copyRuntimeTemplate">复制</a-button>
      </div>
      <pre class="runtime-template-content">{{ runtimeTemplateContent || '-' }}</pre>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { formatTimeWithTimezone, toUtcQueryISOString } from '@/util/timezone'
import { getToken } from '@/api/user'
import store from '@/store'
import { getWebSocketBaseUrl } from '@/util/request'
import { getJobList, cancelJob, getTargetList, getTaskList, getJobLog } from '@/api/sys/automation'
import {
  buildHostScopedLogText,
  copyTextWithFallback,
  formatInventoryHostLabel,
  normalizeUnifiedLogAliases,
} from './logHelpers'

const route = useRoute()
const router = useRouter()

const jobs = ref([])
const jobLoading = ref(false)
const jobRecordId = ref('')
const jobKeyword = ref('')
const selectedJobStatus = ref(null)
const jobOutputKeyword = ref('')
const jobTimeRange = ref([])
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

const jobStatusOptions = [
  { value: 'pending', label: '待执行' },
  { value: 'running', label: '执行中' },
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'cancelled', label: '已取消' },
]

const targets = ref([])
const targetLoading = ref(false)
const targetDrawerVisible = ref(false)
const targetDrawerJobId = ref(null)
const targetDrawerJobStatus = ref('')
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
const jobHostViewerKeyword = ref('')
const jobHostViewerHosts = ref([])

let pollTimer = null
let jobLogSocket = null
let jobLogSocketConnected = false
let jobLogReconnectTimer = null
let jobLogSocketJobId = null
let streamClockTimer = null

const jobColumns = [
  { title: '任务ID', dataIndex: 'job_id', key: 'job_id', width: 100 },
  { title: '任务名称', dataIndex: 'task_name', key: 'task_name', width: 140 },
  { title: '运行模板', key: 'runtime_template', width: 160 },
  { title: '运行主机', key: 'inventory_hosts', width: 260 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 100 },
  { title: '开始运行时间', dataIndex: 'start_time', key: 'start_time', width: 180 },
  { title: '耗时', dataIndex: 'duration_seconds', key: 'duration_seconds', width: 90 },
  { title: '操作', key: 'action', width: 220 },
]

const targetColumns = [
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 140 },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 120 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 90 },
  { title: '返回码', dataIndex: 'rc', key: 'rc', width: 80 },
  { title: '日志', key: 'log_viewer', width: 220 },
]

const jobHostViewerHostCount = computed(() => {
  return (Array.isArray(jobHostViewerHosts.value) ? jobHostViewerHosts.value : []).length
})

const jobHostViewerTreeData = computed(() => {
  const rawTree = buildJobHostViewerTree(jobHostViewerHosts.value)
  return appendGroupHostCount(rawTree)
})

const filteredJobHostViewerTreeData = computed(() => {
  const keyword = String(jobHostViewerKeyword.value || '').trim().toLowerCase()
  if (!keyword) {
    return jobHostViewerTreeData.value
  }
  return filterTreeByKeyword(jobHostViewerTreeData.value, keyword)
})

const jobHostViewerExpandedKeys = computed(() => collectExpandedGroupKeys(filteredJobHostViewerTreeData.value))

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

function normalizeJobStatus(status) {
  return String(status || '').trim().toLowerCase()
}

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

function statusColor(status) {
  if (status === 'success') return 'green'
  if (status === 'running') return 'processing'
  if (status === 'pending') return 'gold'
  if (status === 'cancelled' || status === 'skipped') return 'default'
  return 'red'
}

function isJobFinished(status) {
  const normalized = String(status || '').toLowerCase()
  return normalized === 'success' || normalized === 'failed' || normalized === 'cancelled'
}

function canDownloadJobLog(record) {
  return String(record?.status || '').toLowerCase() !== 'pending'
}

function formatRuntimeTemplateLabel(record) {
  const templateName = String(record?.template_name_snapshot || record?.template_name || '-').trim() || '-'
  const runtimeTaskName = String(record?.task_name_snapshot || record?.task_name || '').trim()
  const displayName = templateName || runtimeTaskName || '-'
  return displayName
}

function getRuntimeTemplateContent(record) {
  const snapshotContent = String(record?.template_content_snapshot || '').trim()
  if (snapshotContent) {
    return snapshotContent
  }
  return ''
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

function getInventoryHostList(record) {
  const snapshot = record?.inventory_snapshot
  if (!snapshot || typeof snapshot !== 'object') {
    return []
  }
  const hosts = snapshot.hosts
  return Array.isArray(hosts) ? hosts : []
}

function openJobHostViewer(record) {
  const hosts = getInventoryHostList(record)
  jobHostViewerHosts.value = hosts
  jobHostViewerKeyword.value = ''
  jobHostViewerTitle.value = `运行主机 / 作业 #${record?.job_id || record?.id || '-'}`
  jobHostViewerVisible.value = true
}

function stripGroupCountSuffix(title) {
  return String(title || '').replace(/\s*[（(](?:\d+台)[）)]\s*$/, '')
}

function appendGroupHostCount(nodes) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []

  const walk = (items) => {
    let hostCount = 0
    const mapped = items.map((node) => {
      if (!node || typeof node !== 'object') {
        return node
      }

      const keyText = String(node.key || '')
      if (keyText.startsWith('host-')) {
        hostCount += 1
        return { ...node, children: undefined }
      }

      const children = Array.isArray(node.children) ? node.children : []
      const childResult = walk(children)
      const baseTitle = stripGroupCountSuffix(node.title || '-')
      const nextTitle = childResult.hostCount > 0 ? `${baseTitle}（${childResult.hostCount}台）` : baseTitle

      hostCount += childResult.hostCount
      return {
        ...node,
        title: nextTitle,
        children: childResult.nodes,
      }
    })

    return { nodes: mapped, hostCount }
  }

  return walk(normalizedNodes).nodes
}

function filterTreeByKeyword(nodes, keyword) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const kw = String(keyword || '').trim().toLowerCase()
  if (!kw) {
    return normalizedNodes
  }

  const walk = (items, keepAllDescendants = false) => {
    const result = []
    items.forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const rawChildren = Array.isArray(node.children) ? node.children : []
      const titleText = String(stripGroupCountSuffix(node.title || '')).toLowerCase()
      const selfMatched = keepAllDescendants || titleText.includes(kw)
      const children = walk(rawChildren, selfMatched)
      if (selfMatched || children.length > 0) {
        result.push({ ...node, children })
      }
    })
    return result
  }

  return walk(normalizedNodes, false)
}

function collectExpandedGroupKeys(nodes) {
  const keys = []
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const keyText = String(node.key || '')
      if (keyText && !keyText.startsWith('host-')) {
        keys.push(keyText)
      }
      walk(node.children || [])
    })
  }
  walk(nodes)
  return keys
}

function buildJobHostViewerTree(hosts) {
  const normalizedHosts = Array.isArray(hosts) ? hosts : []
  const roots = []
  const groupNodeMap = new Map()

  const ensureGroupNode = (groupKey, groupTitle, container) => {
    const existing = groupNodeMap.get(groupKey)
    if (existing) {
      return existing
    }
    const node = {
      key: `group-${groupKey}`,
      title: groupTitle,
      children: [],
    }
    container.push(node)
    groupNodeMap.set(groupKey, node)
    return node
  }

  normalizedHosts.forEach((host, index) => {
    const groupPathRaw = String(host?.group_path || host?.group_name || '').trim()
    const groupSegments = (groupPathRaw ? groupPathRaw : '未分组')
      .split('/')
      .map((item) => String(item || '').trim())
      .filter((item) => item)

    let container = roots
    let groupPathKey = ''
    groupSegments.forEach((segment) => {
      groupPathKey = groupPathKey ? `${groupPathKey}/${segment}` : segment
      const groupNode = ensureGroupNode(groupPathKey, segment, container)
      container = groupNode.children
    })

    const hostKey = Number(host?.host_id)
    const safeHostKey = Number.isInteger(hostKey) && hostKey > 0 ? hostKey : `${index}`
    container.push({
      key: `host-${safeHostKey}`,
      title: formatInventoryHostLabel(host),
      isLeaf: true,
    })
  })

  return roots
}

function getTreeNodeTitle(node) {
  const target = node?.dataRef || node
  return target?.title || '-'
}

async function openLogViewer(record) {
  if (!isJobFinished(targetDrawerJobStatus.value)) {
    message.warning('请等待统一日志输出完成后再查看详细日志')
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
    message.error('作业ID无效，无法打开统一日志')
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
    message.success('统一日志已复制')
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

function buildMergedLogText(record) {
  const stdout = String(record?.stdout || '').trim()
  const stderr = String(record?.stderr || '').trim()

  if (stdout && stderr) {
    return `===== STDOUT =====\n${stdout}\n\n===== STDERR =====\n${stderr}`
  }
  if (stdout) {
    return stdout
  }
  if (stderr) {
    return stderr
  }
  return ''
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

function toSafeFileSegment(value) {
  return String(value || '')
    .replace(/[\\/:*?"<>|\s]+/g, '_')
    .replace(/^_+|_+$/g, '')
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

function normalizeUtcTime(timeValue) {
  if (!timeValue || typeof timeValue !== 'string') {
    return timeValue
  }

  const text = timeValue.trim()
  if (!text) {
    return timeValue
  }

  // 后端若返回无时区字符串（如 2026-07-02 08:00:00），按 UTC 解释后再转用户时区。
  if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
    return text
  }
  return `${text.replace(' ', 'T')}Z`
}

async function loadTaskOptions() {
  const res = await getTaskList({ page: 1, page_size: 300, ordering: '-id' })
  const data = res?.data?.data || {}
  const records = data.results || []
  const nextNameMap = {}
  taskOptions.value = records.map((item) => {
    const label = `${item.name} (${item.code})`
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
      ? toUtcQueryISOString(jobTimeRange.value[0])
      : undefined
    const startTimeTo = Array.isArray(jobTimeRange.value) && jobTimeRange.value[1]
      ? toUtcQueryISOString(jobTimeRange.value[1])
      : undefined

    const res = await getJobList({
      page: jobPagination.current,
      page_size: jobPagination.pageSize,
      ordering: '-id',
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
    if (targetDrawerVisible.value && targetDrawerJobId.value) {
      const matched = (jobs.value || []).find((item) => Number(item.id) === Number(targetDrawerJobId.value))
      targetDrawerJobStatus.value = String(matched?.status || targetDrawerJobStatus.value || '')
    }
    const fallbackJobId = getFallbackStreamJobId()
    if (fallbackJobId) {
      applyJobStatusFromList(fallbackJobId)
    }
  } finally {
    jobLoading.value = false
  }
}

async function showTargets(jobRecord) {
  if (!isJobFinished(jobRecord?.status)) {
    message.warning('请等待统一日志输出完成后再查看详细日志')
    return
  }
  targetDrawerVisible.value = true
  targetDrawerJobId.value = jobRecord.id
  targetDrawerJobStatus.value = String(jobRecord.status || '')
  targetLoading.value = true
  try {
    const res = await getTargetList({ job_id: jobRecord.id, page_size: 1000 })
    const data = res?.data?.data || {}
    targets.value = data.results || data || []
  } finally {
    targetLoading.value = false
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

    if (targetDrawerVisible.value && Number(targetDrawerJobId.value) === jobId) {
      targetDrawerJobStatus.value = 'cancelled'
      const res = await getTargetList({ job_id: jobId, page_size: 1000 })
      const data = res?.data?.data || {}
      targets.value = data.results || data || []
    }
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

function handleJobTableChange(page) {
  jobPagination.current = page.current
  jobPagination.pageSize = page.pageSize
  loadJobs(false)
}

function clearTaskFilter() {
  selectedTaskId.value = null
  selectedTaskName.value = ''
  loadJobs(true)
}

function reloadPage() {
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

function closeTargetDrawer() {
  targetDrawerVisible.value = false
  targetDrawerJobId.value = null
  targetDrawerJobStatus.value = ''
}

function startPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
  }
  pollTimer = window.setInterval(() => {
    streamClockTick.value = Date.now()
    
    // 如果通过 job_id 精确搜索且任务已完成，停止轮询
    if (jobRecordId.value && jobs.value.length > 0) {
      const job = jobs.value[0]
      if (isJobFinished(job?.status)) {
        stopPolling()
        return
      }
    }
    
    loadJobs(false)
    if (targetDrawerVisible.value && targetDrawerJobId.value) {
      getTargetList({ job_id: targetDrawerJobId.value, page_size: 1000 }).then((res) => {
        const data = res?.data?.data || {}
        targets.value = data.results || data || []
      })
    }
  }, 5000)

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

onMounted(async () => {
  const queryTaskId = route.query.task_id
  const queryTaskName = route.query.task_name
  const queryKeyword = route.query.keyword
  const queryJobId = route.query.job_id
  
  // 优先级：job_id > keyword > 其他参数
  if (queryJobId) {
    jobRecordId.value = String(queryJobId).trim()
    selectedTaskId.value = null
    selectedTaskName.value = ''
  } else if (queryKeyword) {
    jobRecordId.value = String(queryKeyword).trim()
    selectedTaskId.value = null
    selectedTaskName.value = ''
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
  if (selectedTaskId.value && !selectedTaskName.value) {
    selectedTaskName.value = taskNameMap.value[selectedTaskId.value] || ''
  }
  await loadJobs(true)
  
  // 只有同时有 job_id 和 task_id 参数（从"日志"按钮跳转），才自动打开日志界面
  // "详细"按钮只传 job_id，不传 task_id，所以不会自动打开
  if (queryJobId && queryTaskId) {
    const parsedJobId = Number(Array.isArray(queryJobId) ? queryJobId[0] : queryJobId)
    if (Number.isInteger(parsedJobId) && parsedJobId > 0) {
      await openJobLogViewerById(parsedJobId)
    }
  }
  
  startPolling()
})

onBeforeUnmount(() => {
  targetDrawerJobStatus.value = ''
  closeJobLogSocket()
  if (pollTimer) {
    window.clearInterval(pollTimer)
  }
  if (streamClockTimer) {
    window.clearInterval(streamClockTimer)
  }
})
</script>

<style scoped>
.automation-logs-page {
  padding: 2px;
}

.block-card {
  margin-bottom: 12px;
}

.jobs-card {
  margin-top: 12px;
}

.log-cell {
  white-space: pre-wrap;
  max-height: 160px;
  overflow: auto;
  font-size: 12px;
  color: #444;
}

.log-toolbar {
  display: flex;
  width: 100%;
  padding-bottom: 12px;
  border-bottom: 1px solid #f0f0f0;
}

.log-stream-meta {
  color: #666;
  font-size: 12px;
}

.job-host-list-preview {
  color: #1677ff;
  cursor: pointer;
}

.job-host-list-popover {
  max-width: 420px;
  max-height: 260px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.job-host-viewer-search {
  margin-bottom: 10px;
}

.job-host-viewer-list {
  max-width: 100%;
  max-height: 52vh;
  padding-right: 4px;
}

.job-host-viewer-item {
  padding: 4px 0;
  border-bottom: 1px solid #f5f5f5;
  word-break: break-all;
}

.scope-editor__desc {
  margin-bottom: 8px;
  color: #8c8c8c;
  font-size: 13px;
}

.scope-viewer__summary {
  margin-bottom: 8px;
  color: #262626;
  font-size: 14px;
  font-weight: 500;
}

.scope-editor__tree-wrap {
  max-height: 60vh;
  overflow: auto;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  padding: 8px 10px;
}

.runtime-template-link {
  padding-left: 0;
}

.runtime-template-toolbar {
  margin-bottom: 16px;
  display: block;
}

.runtime-template-content {
  margin: 0;
  max-height: 60vh;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  background: #141414;
  color: #f5f5f5;
  border-radius: 8px;
  padding: 14px;
  font-size: 12px;
  line-height: 1.5;
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
}

.log-viewer-shell {
  margin-top: 12px;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  background: #141414;
  height: calc(100vh - 220px);
  overflow: auto;
}

.log-viewer-content {
  margin: 0;
  padding: 14px;
  color: #f5f5f5;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
}

.log-viewer-content.is-nowrap {
  white-space: pre;
  word-break: normal;
}

</style>
