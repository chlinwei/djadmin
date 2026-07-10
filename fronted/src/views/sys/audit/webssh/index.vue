<template>
  <div class="audit-page">
    <a-row class="tools" :gutter="16">
      <a-col :span="5">
        <a-input-search
          v-model:value="filters.keyword"
          class="tool-item"
          placeholder="ID / 实例名 / IP / 用户"
          allow-clear
          enter-button
          size="large"
          @search="handleKeywordSearch"
        />
      </a-col>
      <a-col :span="5">
        <a-input-search
          v-model:value="filters.outputKeyword"
          class="tool-item"
          placeholder="输出记录关键字"
          allow-clear
          enter-button
          size="large"
          @search="handleKeywordSearch"
        />
      </a-col>
      <a-col :span="6">
        <a-range-picker
          v-model:value="filters.timeRange"
          class="tool-item"
          show-time
          size="large"
          format="YYYY-MM-DD HH:mm:ss"
          :placeholder="['开始时间', '结束时间']"
          @change="handleTimeRangeChange"
        />
      </a-col>
      <a-col :span="4">
        <a-select
          v-model:value="filters.status"
          class="tool-item"
          placeholder="会话状态"
          allow-clear
          size="large"
          :options="statusOptions"
          @change="loadSessions"
        />
      </a-col>
      <a-col :span="4" class="right-actions">
        <a-space wrap>
          <a-button size="large" type="primary" ghost @click="downloadFilteredSessions" :disabled="loading">
            <FontAwesomeIcon :icon="['fas', 'download']" />
            <span>&nbsp;{{ selectedCount > 0 ? `下载已选(${selectedCount})` : '批量下载日志' }}</span>
          </a-button>
          <a-button size="large" type="primary" ghost class="refresh-btn" @click="reload" :disabled="loading">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-space>
      </a-col>
    </a-row>

    <a-card size="small" class="audit-card" title="Web SSH 操作审计">
      <a-table
        :columns="columns"
        :data-source="sessions"
        :loading="loading"
        :pagination="pagination"
        :row-selection="rowSelection"
        rowKey="id"
        :scroll="{ x: 1300 }"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'host_name'">
            <a-space direction="vertical" :size="0">
              <span>{{ record.host_name || '-' }}</span>
              <span class="host-ip">{{ record.host_ip || '-' }}</span>
            </a-space>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag v-if="record.status === 'connected'" color="processing">连接中</a-tag>
            <a-tag v-else-if="record.status === 'closed'" color="success">已关闭</a-tag>
            <a-tag v-else color="error">失败</a-tag>
          </template>
          <template v-else-if="column.key === 'start_time' || column.key === 'end_time'">
            {{ formatDateTime(record[column.dataIndex]) }}
          </template>
          <template v-else-if="column.key === 'duration_seconds'">
            {{ record.duration_seconds ?? '-' }}
          </template>
          <template v-else-if="column.key === 'command_count'">
            {{ record.command_count ?? 0 }}
          </template>
          <template v-else-if="column.key === 'input_bytes'">
            {{ record.input_bytes ?? 0 }}
          </template>
          <template v-else-if="column.key === 'recorded_content_bytes'">
            {{ formatSize(record.recorded_content_bytes || 0) }}
            <a-tag v-if="record.is_content_truncated" color="orange" style="margin-left: 6px">已截断</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space :size="8" class="action-links" wrap>
              <a-button size="small" type="link" @click="openSessionContent(record)">
                查看内容
              </a-button>
              <a-button size="small" type="link" @click="downloadSessionLog(record)">
                下载日志
              </a-button>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="contentModalVisible"
      :title="`会话内容 - ${activeSessionId || '-'}`"
      :footer="null"
      :width="1000"
      destroyOnClose
    >
      <a-spin :spinning="contentLoading">
        <a-tabs>
          <a-tab-pane key="output" tab="输出记录">
            <pre class="session-log-content">{{ contentOutput || '暂无输出内容' }}</pre>
          </a-tab-pane>
          <a-tab-pane key="input" tab="输入记录">
            <div class="session-log-content">
              <div v-if="readableInputCommands.length" class="command-list">
                <div v-for="(cmd, index) in readableInputCommands" :key="`${index}-${cmd}`" class="command-item">
                  <span class="command-index">{{ index + 1 }}.</span>
                  <span class="command-text">{{ cmd }}</span>
                </div>
              </div>
              <pre v-else>{{ contentInput || '暂无输入内容' }}</pre>
            </div>
          </a-tab-pane>
        </a-tabs>
      </a-spin>
    </a-modal>
  </div>
</template>

<script setup>
defineOptions({
  name: 'websshAudit'
})

import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { downloadAuditWebSshSession, downloadAuditWebSshSessions, getAuditWebSshSessionContent, getAuditWebSshSessions } from '@/api/sys/audit.js'
import { formatTimeWithTimezone, toUtcQueryISOString } from '@/util/timezone'
import store from '@/store'

const loading = ref(false)
const sessions = ref([])
const contentModalVisible = ref(false)
const contentLoading = ref(false)
const activeSessionId = ref('')
const contentInput = ref('')
const contentOutput = ref('')
const readableInputCommands = ref([])
const selectedLogIds = ref([])
const selectedRecords = ref([])

const selectedCount = computed(() => selectedLogIds.value.length)

const filters = reactive({
  keyword: '',
  outputKeyword: '',
  timeRange: [],
  status: undefined,
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有${total}条数据`,
})

const statusOptions = [
  { label: '连接中', value: 'connected' },
  { label: '已关闭', value: 'closed' },
  { label: '失败', value: 'failed' },
]

const rowSelection = reactive({
  selectedRowKeys: [],
  onChange: (selectedKeys, selectedRows) => {
    rowSelection.selectedRowKeys = selectedKeys
    selectedRecords.value = selectedRows
    selectedLogIds.value = selectedRows.map((row) => row.id).filter((id) => id != null)
  },
})

const columns = [
  {
    title: 'ID',
    dataIndex: 'id',
    key: 'id',
    width: 80,
  },
  {
    title: '实例名',
    dataIndex: 'host_name',
    key: 'host_name',
    width: 220,
  },
  {
    title: '操作用户',
    dataIndex: 'username',
    key: 'username',
    width: 130,
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    width: 100,
  },
  {
    title: '操作',
    key: 'action',
    width: 180,
  },
  {
    title: '开始时间',
    dataIndex: 'start_time',
    key: 'start_time',
    width: 180,
  },
  {
    title: '结束时间',
    dataIndex: 'end_time',
    key: 'end_time',
    width: 180,
  },
  {
    title: '时长(秒)',
    dataIndex: 'duration_seconds',
    key: 'duration_seconds',
    width: 100,
  },
  {
    title: '命令数',
    dataIndex: 'command_count',
    key: 'command_count',
    width: 90,
  },
  {
    title: '输入字节',
    dataIndex: 'input_bytes',
    key: 'input_bytes',
    width: 110,
  },
  {
    title: '客户端IP',
    dataIndex: 'client_ip',
    key: 'client_ip',
    width: 140,
  },
  {
    title: '内容大小',
    dataIndex: 'recorded_content_bytes',
    key: 'recorded_content_bytes',
    width: 160,
  },
]

function formatDateTime(value) {
  if (!value) {
    return '-'
  }
  const date = new Date(normalizeUtcTime(value))
  if (Number.isNaN(date.getTime())) {
    return '-'
  }
  return formatTimeWithTimezone(normalizeUtcTime(value), store.state.user?.timezone || 'Asia/Shanghai', 'YYYY-MM-DD HH:mm:ss')
}

function normalizeUtcTime(value) {
  if (!value || typeof value !== 'string') {
    return value
  }
  const text = value.trim()
  if (!text) {
    return value
  }
  if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
    return text
  }
  return `${text.replace(' ', 'T')}Z`
}

function formatSize(bytes) {
  const size = Number(bytes) || 0
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`
  return `${(size / (1024 * 1024)).toFixed(2)} MB`
}

function buildQueryParams() {
  const [startTime, endTime] = filters.timeRange || []
  return {
    pageNumber: pagination.current,
    pageSize: pagination.pageSize,
    keyword: filters.keyword || undefined,
    output_keyword: filters.outputKeyword || undefined,
    start_time_from: toUtcQueryISOString(startTime),
    start_time_to: toUtcQueryISOString(endTime),
    status: filters.status || undefined,
  }
}

function buildDownloadQueryParams() {
  const [startTime, endTime] = filters.timeRange || []
  const hasSelected = selectedLogIds.value.length > 0
  return {
    keyword: filters.keyword || undefined,
    output_keyword: filters.outputKeyword || undefined,
    start_time_from: toUtcQueryISOString(startTime),
    start_time_to: toUtcQueryISOString(endTime),
    status: filters.status || undefined,
    ids: hasSelected ? selectedLogIds.value.join(',') : undefined,
  }
}

function clearSelection() {
  rowSelection.selectedRowKeys = []
  selectedRecords.value = []
  selectedLogIds.value = []
}

function formatFilenameTime(value) {
  if (!value) {
    return 'unknown-start-time'
  }
  return formatDateTime(value).replace(/[\/:\s]/g, '-').replace(/-+/g, '-').replace(/\.$/, '')
}

function buildSingleLogFallbackName(record) {
  const hostName = record?.host_name || 'unknown-host'
  const hostIp = record?.host_ip || 'unknown-ip'
  const startTime = formatFilenameTime(record?.start_time)
  return `webssh-${record?.id ?? 'unknown-id'}-${hostName}(${hostIp})-${startTime}.log`
}

function buildZipFallbackName() {
  const date = new Date()
  const pad = (num) => `${num}`.padStart(2, '0')
  const timestamp = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}-${pad(date.getHours())}-${pad(date.getMinutes())}-${pad(date.getSeconds())}`
  return `webssh-${timestamp}.zip`
}

async function loadSessions() {
  loading.value = true
  try {
    const res = await getAuditWebSshSessions(buildQueryParams())
    const payload = res.data?.data || {}
    sessions.value = payload.results || []
    clearSelection()
    pagination.total = payload.count || 0
    pagination.current = payload.pageNumber || pagination.current
    pagination.pageSize = payload.pageSize || pagination.pageSize
  } catch (error) {
    message.error(error?.message || '加载 Web SSH 审计日志失败')
  } finally {
    loading.value = false
  }
}


function handleTableChange(pager) {
  pagination.current = pager.current
  pagination.pageSize = pager.pageSize
  loadSessions()
}

function handleTimeRangeChange() {
  pagination.current = 1
  loadSessions()
}

function handleKeywordSearch() {
  pagination.current = 1
  loadSessions()
}

function reload() {
  pagination.current = 1
  loadSessions()
}

async function openSessionContent(record) {
  contentModalVisible.value = true
  contentLoading.value = true
  activeSessionId.value = String(record.id || '')
  contentInput.value = ''
  contentOutput.value = ''
  readableInputCommands.value = []

  try {
    const res = await getAuditWebSshSessionContent(record.id)
    const payload = res.data?.data || {}
    contentInput.value = payload.readable_input_content || payload.input_content || ''
    contentOutput.value = payload.output_content || ''
    readableInputCommands.value = payload.readable_input_commands || []
  } catch (error) {
    message.error(error?.message || '加载会话内容失败')
  } finally {
    contentLoading.value = false
  }
}

function parseDownloadFilename(contentDisposition, fallbackName) {
  if (!contentDisposition) {
    return fallbackName
  }

  const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch (error) {
      return fallbackName
    }
  }

  const plainMatch = contentDisposition.match(/filename="?([^";]+)"?/i)
  if (plainMatch?.[1]) {
    return plainMatch[1]
  }

  return fallbackName
}

async function downloadSessionLog(record) {
  try {
    const response = await downloadAuditWebSshSession(record.id)
    const fallbackName = buildSingleLogFallbackName(record)
    const filename = parseDownloadFilename(response.headers?.['content-disposition'], fallbackName)
    const blob = response.data instanceof Blob
      ? response.data
      : new Blob([response.data], { type: 'text/plain;charset=utf-8' })

    const url = window.URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    document.body.appendChild(anchor)
    anchor.click()
    document.body.removeChild(anchor)
    window.URL.revokeObjectURL(url)
    message.success('日志下载成功')
  } catch (error) {
    message.error(error?.message || '下载日志失败')
  }
}

async function downloadFilteredSessions() {
  try {
    const response = await downloadAuditWebSshSessions(buildDownloadQueryParams())
    const fallbackName = selectedCount.value === 1
      ? buildSingleLogFallbackName(selectedRecords.value[0])
      : buildZipFallbackName()
    const filename = parseDownloadFilename(response.headers?.['content-disposition'], fallbackName)
    const blob = response.data instanceof Blob
      ? response.data
      : new Blob([response.data], { type: 'text/plain;charset=utf-8' })

    const url = window.URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    document.body.appendChild(anchor)
    anchor.click()
    document.body.removeChild(anchor)
    window.URL.revokeObjectURL(url)
    message.success(selectedCount.value > 0 ? `已下载 ${selectedCount.value} 条勾选记录` : '批量下载日志成功')
  } catch (error) {
    message.error(error?.message || '批量下载日志失败')
  }
}

onMounted(() => {
  loadSessions()
})
</script>

<style scoped>
.audit-page {
  margin-top: 12px;
}

.tools {
  margin-bottom: 12px;
}

.tool-item {
  width: 100%;
}

.right-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.audit-card {
  min-height: 520px;
}

.host-ip {
  color: #888;
  font-size: 12px;
}

.action-links {
  white-space: nowrap;
}

.session-log-content {
  max-height: 500px;
  overflow: auto;
  padding: 10px;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  background: #fafafa;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
}

.command-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.command-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  line-height: 1.6;
  font-family: Consolas, 'Courier New', monospace;
}

.command-index {
  color: #999;
  min-width: 28px;
}

.command-text {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
