<template>
  <div class="audit-page">
    <a-row class="tools" :gutter="16">
      <a-col :span="8">
        <a-input-search
          v-model:value="filters.keyword"
          class="tool-item"
          placeholder="用户名 / 路径 / IP / 说明"
          allow-clear
          enter-button
          size="large"
          @search="loadLogs"
        />
      </a-col>
      <a-col :span="8">
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
          v-model:value="filters.method"
          class="tool-item"
          placeholder="请求方法"
          allow-clear
          size="large"
          :options="methodOptions"
          @change="loadLogs"
        />
      </a-col>
      <a-col :span="4" class="right-actions">
        <a-button size="large" type="primary" ghost class="refresh-btn" @click="reload" :disabled="loading">
          <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
          <span>&nbsp;刷新</span>
        </a-button>
      </a-col>
    </a-row>

    <a-card size="small" class="audit-card" title="操作日志">
      <a-table
        :columns="columns"
        :data-source="logs"
        :loading="loading"
        :pagination="pagination"
        rowKey="id"
        :scroll="{ x: 1300 }"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status_code'">
            <a-tag v-if="Number(record.status_code) < 400" color="success">{{ record.status_code }}</a-tag>
            <a-tag v-else color="error">{{ record.status_code }}</a-tag>
          </template>
          <template v-else-if="column.key === 'created_at'">
            {{ formatDateTime(record.created_at) }}
          </template>
          <template v-else-if="column.key === 'duration_ms'">
            {{ record.duration_ms ?? '-' }}
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button size="small" type="link" @click="openDetail(record)">查看详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="detailModalVisible"
      :title="`操作日志详情 - ${activeLog?.path || '-'}`"
      :footer="null"
      :width="1000"
      destroyOnClose
    >
      <a-descriptions bordered :column="1" size="small">
        <a-descriptions-item label="用户名">{{ activeLog?.username || '-' }}</a-descriptions-item>
        <a-descriptions-item label="方法">{{ activeLog?.method || '-' }}</a-descriptions-item>
        <a-descriptions-item label="路径">{{ activeLog?.path || '-' }}</a-descriptions-item>
        <a-descriptions-item label="路由">{{ activeLog?.route_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="状态码">{{ activeLog?.status_code ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="耗时(ms)">{{ activeLog?.duration_ms ?? '-' }}</a-descriptions-item>
      </a-descriptions>

      <a-row :gutter="16" style="margin-top: 16px">
        <a-col :span="12">
          <a-card size="small" title="请求参数" :bordered="false">
            <pre class="detail-content">{{ formatJsonText(activeLog?.request_data) }}</pre>
          </a-card>
        </a-col>
        <a-col :span="12">
          <a-card size="small" title="返回结果" :bordered="false">
            <pre class="detail-content">{{ formatJsonText(activeLog?.response_data) }}</pre>
          </a-card>
        </a-col>
      </a-row>
    </a-modal>
  </div>
</template>

<script setup>
defineOptions({
  name: 'operationAudit'
})

import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { getAuditOperationLogs } from '@/api/sys/audit.js'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { formatTimeWithTimezone, toUtcQueryISOString } from '@/util/timezone'
import { listenUserTimezoneChanged } from '@/util/userTimezoneSync'

const loading = ref(false)
const logs = ref([])
const detailModalVisible = ref(false)
const activeLog = ref(null)
const userTimezone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
let stopListenTimezone = null

const filters = reactive({
  keyword: '',
  timeRange: [],
  method: undefined,
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  pageSizeOptions: ['10', '20', '30'],
  showTotal: (total) => `共有${total}条数据`,
  showQuickJumper: true,
})

const methodOptions = [
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' },
  { label: 'PATCH', value: 'PATCH' },
  { label: 'DELETE', value: 'DELETE' },
]

const allowedMethods = new Set(methodOptions.map((item) => item.value))

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '用户名', dataIndex: 'username', key: 'username', width: 140 },
  { title: '方法', dataIndex: 'method', key: 'method', width: 90 },
  { title: '路径', dataIndex: 'path', key: 'path', width: 260, ellipsis: true },
  { title: '路由', dataIndex: 'route_name', key: 'route_name', width: 220, ellipsis: true },
  { title: '状态码', dataIndex: 'status_code', key: 'status_code', width: 110 },
  { title: '耗时(ms)', dataIndex: 'duration_ms', key: 'duration_ms', width: 100 },
  { title: '客户端IP', dataIndex: 'client_ip', key: 'client_ip', width: 140 },
  { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180 },
  { title: '说明', dataIndex: 'message', key: 'message', ellipsis: true },
  { title: '操作', key: 'action', width: 110, fixed: 'right' },
]

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

function formatDateTime(value) {
  if (!value) {
    return '-'
  }
  try {
    return formatTimeWithTimezone(normalizeUtcTime(value), userTimezone.value, 'YYYY-MM-DD HH:mm:ss')
  } catch (error) {
    return value
  }
}

function formatJsonText(value) {
  if (!value) {
    return '暂无内容'
  }
  try {
    const parsed = typeof value === 'string' ? JSON.parse(value) : value
    return JSON.stringify(parsed, null, 2)
  } catch (error) {
    return String(value)
  }
}

function openDetail(record) {
  activeLog.value = record
  detailModalVisible.value = true
}

function buildQueryParams() {
  const [startTime, endTime] = filters.timeRange || []
  return {
    pageNumber: pagination.current,
    pageSize: pagination.pageSize,
    keyword: filters.keyword || undefined,
    method: allowedMethods.has(filters.method) ? filters.method : undefined,
    created_at_from: toUtcQueryISOString(startTime),
    created_at_to: toUtcQueryISOString(endTime),
  }
}

async function loadLogs() {
  loading.value = true
  try {
    const res = await getAuditOperationLogs(buildQueryParams())
    const payload = res.data?.data || {}
    logs.value = payload.results || []
    pagination.total = payload.count || 0
    pagination.current = payload.pageNumber || pagination.current
    pagination.pageSize = payload.pageSize || pagination.pageSize
  } catch (error) {
    message.error(error?.message || '加载操作日志失败')
  } finally {
    loading.value = false
  }
}

async function loadUserTimezone() {
  try {
    const res = await getCurrentUserInfo()
    const timezone = res?.data?.data?.timezone
    if (timezone) {
      userTimezone.value = timezone
    }
  } catch (error) {
    // fallback: keep browser timezone
  }
}

function handleTableChange(pager) {
  pagination.current = pager.current
  pagination.pageSize = pager.pageSize
  loadLogs()
}

function handleTimeRangeChange() {
  pagination.current = 1
  loadLogs()
}

function reload() {
  pagination.current = 1
  loadLogs()
}

onMounted(() => {
  stopListenTimezone = listenUserTimezoneChanged((timezone) => {
    userTimezone.value = timezone
  })
  loadUserTimezone()
  loadLogs()
})

onUnmounted(() => {
  if (stopListenTimezone) {
    stopListenTimezone()
  }
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
}

.audit-card {
  min-height: 520px;
}

.detail-content {
  max-height: 360px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>