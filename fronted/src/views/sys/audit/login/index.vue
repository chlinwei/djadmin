<template>
  <div class="audit-page">
    <a-row class="tools" :gutter="16">
      <a-col :span="8">
        <a-input-search
          v-model:value="filters.keyword"
          class="tool-item"
          placeholder="用户名 / 客户端IP / 提示信息"
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
          v-model:value="filters.status"
          class="tool-item"
          placeholder="登录状态"
          allow-clear
          size="large"
          :options="statusOptions"
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

    <a-card size="small" class="audit-card" title="登录日志">
      <a-table
        :columns="columns"
        :data-source="logs"
        :loading="loading"
        :pagination="pagination"
        rowKey="id"
        :scroll="{ x: 1100 }"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag v-if="record.status === 'success'" color="success">成功</a-tag>
            <a-tag v-else color="error">失败</a-tag>
          </template>
          <template v-else-if="column.key === 'login_time'">
            {{ formatDateTime(record.login_time) }}
          </template>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup>
defineOptions({
  name: 'loginAudit'
})

import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { getAuditLoginLogs } from '@/api/sys/audit.js'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { formatTimeWithTimezone, toUtcQueryISOString } from '@/util/timezone'
import { listenUserTimezoneChanged } from '@/util/userTimezoneSync'

const loading = ref(false)
const logs = ref([])
const userTimezone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
let stopListenTimezone = null

const filters = reactive({
  keyword: '',
  timeRange: [],
  status: undefined,
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共 ${total} 条`,
})

const statusOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
]

const columns = [
  { title: '用户名', dataIndex: 'username', key: 'username', width: 180 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '登录时间', dataIndex: 'login_time', key: 'login_time', width: 200 },
  { title: '客户端IP', dataIndex: 'client_ip', key: 'client_ip', width: 180 },
  { title: 'User-Agent', dataIndex: 'user_agent', key: 'user_agent', ellipsis: true },
  { title: '说明', dataIndex: 'message', key: 'message', width: 220 },
]

function formatDateTime(value) {
  if (!value) {
    return '-'
  }
  const date = new Date(normalizeUtcTime(value))
  if (Number.isNaN(date.getTime())) {
    return '-'
  }
  return formatTimeWithTimezone(normalizeUtcTime(value), userTimezone.value, 'YYYY-MM-DD HH:mm:ss')
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

function handleTimezoneChanged(timezone) {
  if (timezone) {
    userTimezone.value = timezone
  }
}

function buildQueryParams() {
  const [startTime, endTime] = filters.timeRange || []
  return {
    pageNumber: pagination.current,
    pageSize: pagination.pageSize,
    keyword: filters.keyword || undefined,
    login_time_from: toUtcQueryISOString(startTime),
    login_time_to: toUtcQueryISOString(endTime),
    status: filters.status || undefined,
  }
}

async function loadLogs() {
  loading.value = true
  try {
    const res = await getAuditLoginLogs(buildQueryParams())
    const payload = res.data?.data || {}
    logs.value = payload.results || []
    pagination.total = payload.count || 0
    pagination.current = payload.pageNumber || pagination.current
    pagination.pageSize = payload.pageSize || pagination.pageSize
  } catch (error) {
    message.error(error?.message || '加载登录日志失败')
  } finally {
    loading.value = false
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
  stopListenTimezone = listenUserTimezoneChanged(handleTimezoneChanged)
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
</style>
