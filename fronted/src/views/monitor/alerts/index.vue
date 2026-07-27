<template>
  <div class="alerts-page">
    <a-card title="告警" size="small">
      <!-- 右上角自动刷新控制（对齐智能监控页）：开关 + 间隔 + 手动刷新 + 上次刷新时间，
           定时器只刷新当前激活的 tab，避免后台两个 tab 都请求。 -->
      <template #extra>
        <a-space>
          <a-tag v-if="lastRefreshAtText" color="default">刷新于 {{ lastRefreshAtText }}</a-tag>
          <a-switch v-model:checked="autoRefreshEnabled" checked-children="自动刷新" un-checked-children="手动" />
          <a-select v-model:value="refreshIntervalSeconds" style="width: 120px" :options="refreshIntervalOptions" :disabled="!autoRefreshEnabled" :getPopupContainer="getPopupContainer" />
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="loading || historyLoading" @click="refreshActiveTab">刷新</a-button>
          </a-tooltip>
        </a-space>
      </template>
      <!-- 用 a-tabs 划分“当前告警/历史告警”层级：当前告警读取 Prometheus 实时数据，
           历史告警后续接入时作为同级 tab-pane 新增，避免与当前告警的展示逻辑耦合。 -->
      <a-tabs v-model:activeKey="activeTabKey">
        <a-tab-pane key="current" tab="当前告警">
          <a-space style="margin-bottom: 12px" wrap>
            <a-tooltip title="刷新">
              <a-button type="primary" ghost :loading="loading" @click="loadAlerts">刷新</a-button>
            </a-tooltip>
            <a-tag color="red">firing：{{ firingCount }}</a-tag>
            <a-tag color="default">resolved：{{ resolvedCount }}</a-tag>
            <a-select
              v-model:value="stateFilter"
              :options="stateFilterOptions"
              :getPopupContainer="getPopupContainer"
              style="width: 140px"
            />
            <a-select
              v-model:value="severityFilter"
              :options="severityFilterOptions"
              :getPopupContainer="getPopupContainer"
              style="width: 140px"
            />
            <a-input-search
              v-model:value="keyword"
              placeholder="按名称/实例/摘要搜索"
              allow-clear
              style="width: 260px"
            />
            <a-typography-text type="secondary">Prometheus 地址：{{ prometheusBaseUrl || '-' }}</a-typography-text>
          </a-space>

          <a-alert v-if="loadError" type="error" show-icon :message="loadError" style="margin-bottom: 12px" />

          <a-table
            rowKey="rowKey"
            :columns="columns"
            :data-source="filteredRows"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="{ showSizeChanger: true, showQuickJumper: true, showTotal: (total) => `共有 ${total} 条数据` }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'severity'">
                <a-tag v-if="record.severity" :color="severityColor(record.severity)">{{ record.severity }}</a-tag>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.key === 'state'">
                <a-tag :color="stateColor(record.state)">{{ record.state || 'unknown' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'summary'">
                <a-typography-text ellipsis style="max-width: 360px">{{ record.summary || '-' }}</a-typography-text>
              </template>
              <template v-else-if="column.key === 'active_at'">
                {{ formatActiveAt(record.active_at) }}
              </template>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="history" tab="历史告警" force-render>
          <a-space style="margin-bottom: 12px" wrap>
            <a-tooltip title="刷新">
              <a-button type="primary" ghost :loading="historyLoading" @click="loadAlertHistory">刷新</a-button>
            </a-tooltip>
            <a-select
              v-model:value="historyState"
              :options="historyStateOptions"
              :getPopupContainer="getPopupContainer"
              style="width: 140px"
              @change="onHistoryFilterChange"
            />
            <a-select
              v-model:value="historySeverity"
              :options="historySeverityOptions"
              :getPopupContainer="getPopupContainer"
              style="width: 140px"
              @change="onHistoryFilterChange"
            />
            <a-input-search
              v-model:value="historyKeyword"
              placeholder="按名称/实例搜索"
              allow-clear
              style="width: 260px"
              @search="onHistoryFilterChange"
            />
            <a-range-picker
              v-model:value="historyTimeRange"
              show-time
              :getPopupContainer="getPopupContainer"
              @change="onHistoryFilterChange"
            />
          </a-space>

          <a-alert v-if="historyLoadError" type="error" show-icon :message="historyLoadError" style="margin-bottom: 12px" />

          <a-table
            rowKey="id"
            :columns="historyColumns"
            :data-source="historyRows"
            :loading="historyLoading"
            size="small"
            :scroll="{ x: 1300 }"
            :pagination="historyPagination"
            @change="handleHistoryTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'severity'">
                <a-tag v-if="record.severity" :color="severityColor(record.severity)">{{ record.severity }}</a-tag>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.key === 'state'">
                <a-tag :color="stateColor(record.state)">{{ record.state || 'unknown' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'started_at'">
                {{ formatActiveAt(record.started_at) }}
              </template>
              <template v-else-if="column.key === 'resolved_at'">
                <span v-if="!record.resolved_at">仍在告警中</span>
                <a-tooltip v-else-if="record.resolved_by_reconciliation" title="该恢复时间由每日对账兜底订正，非 Prometheus 精确推送">
                  {{ formatActiveAt(record.resolved_at) }}（对账）
                </a-tooltip>
                <span v-else>{{ formatActiveAt(record.resolved_at) }}</span>
              </template>
            </template>
          </a-table>
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { getAlertHistories, getPrometheusAlerts } from '@/api/sys/monitor'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const activeTabKey = ref('current')

// 自动刷新（对齐智能监控页）：定时器只刷新当前激活的 tab，避免后台无谓请求。
const autoRefreshEnabled = ref(true)
const refreshIntervalSeconds = ref(15)
const refreshIntervalOptions = [
  { label: '5秒', value: 5 },
  { label: '10秒', value: 10 },
  { label: '15秒', value: 15 },
  { label: '30秒', value: 30 },
  { label: '60秒', value: 60 },
]
let refreshTimer = null
const lastRefreshAt = ref(null)
const lastRefreshAtText = computed(() => {
  if (!lastRefreshAt.value) return ''
  // “刷新于”展示本次刷新的时间，按用户时区格式化（与全站时间显示规范一致）
  return formatTimeWithTimezone(lastRefreshAt.value, store.state.user?.timezone || 'Asia/Shanghai', 'HH:mm:ss')
})
const loading = ref(false)
const loadError = ref('')
const prometheusBaseUrl = ref('')
const firingCount = ref(0)
const resolvedCount = ref(0)
const rawRows = ref([])
const keyword = ref('')
const stateFilter = ref('all')
const severityFilter = ref('all')

const stateFilterOptions = computed(() => {
  const values = Array.from(new Set(rawRows.value.map((row) => row.state).filter((v) => v)))
  return [{ label: '全部状态', value: 'all' }, ...values.map((v) => ({ label: v, value: v }))]
})

const severityFilterOptions = computed(() => {
  const values = Array.from(new Set(rawRows.value.map((row) => row.severity).filter((v) => v)))
  return [{ label: '全部级别', value: 'all' }, ...values.map((v) => ({ label: v, value: v }))]
})

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '级别', key: 'severity', width: 100 },
  { title: '状态', key: 'state', width: 100 },
  { title: '实例', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: '摘要', key: 'summary', width: 360 },
  { title: '触发时间', key: 'active_at', width: 200 },
  { title: '当前值', dataIndex: 'value', key: 'value', width: 120 },
]

function parseApiData(res) {
  const payload = res?.data || res
  if (payload && typeof payload === 'object' && payload.data !== undefined) {
    return payload.data
  }
  return payload || {}
}

function severityColor(severity) {
  if (severity === 'critical') return 'red'
  if (severity === 'warning') return 'orange'
  if (severity === 'info') return 'blue'
  return 'default'
}

function stateColor(state) {
  if (state === 'firing') return 'red'
  if (state === 'pending') return 'orange'
  return 'default'
}

// 按用户时区显示 Prometheus 返回的 activeAt（UTC RFC3339），与全站时间显示规范保持一致
function formatActiveAt(value) {
  if (!value) return '-'
  return formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai')
}

const filteredRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return rawRows.value.filter((row) => {
    if (stateFilter.value !== 'all' && row.state !== stateFilter.value) return false
    if (severityFilter.value !== 'all' && row.severity !== severityFilter.value) return false
    if (kw === '') return true
    return (
      String(row.name || '').toLowerCase().includes(kw) ||
      String(row.instance || '').toLowerCase().includes(kw) ||
      String(row.summary || '').toLowerCase().includes(kw)
    )
  })
})

async function loadAlerts() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await getPrometheusAlerts()
    const data = parseApiData(res)
    prometheusBaseUrl.value = data.prometheus_base_url || ''
    if (String(data.status || '').toLowerCase() === 'error') {
      loadError.value = data.error || '查询 Prometheus 告警失败'
      rawRows.value = []
      firingCount.value = 0
      resolvedCount.value = 0
      return
    }
    firingCount.value = Number(data.firing_count || 0)
    resolvedCount.value = Number(data.resolved_count || 0)
    const results = Array.isArray(data.results) ? data.results : []
    rawRows.value = results.map((item, index) => ({ rowKey: `${item.name}-${item.instance}-${index}`, ...item }))
  } catch (error) {
    loadError.value = error?.response?.data?.msg || error?.message || '加载告警失败'
    message.warning(loadError.value)
    rawRows.value = []
  } finally {
    loading.value = false
    lastRefreshAt.value = new Date()
    if (stateFilter.value !== 'all' && !stateFilterOptions.value.some((option) => option.value === stateFilter.value)) {
      stateFilter.value = 'all'
    }
    if (severityFilter.value !== 'all' && !severityFilterOptions.value.some((option) => option.value === severityFilter.value)) {
      severityFilter.value = 'all'
    }
  }
}

// ---- 历史告警（后端落库，服务端分页/筛选）----
const historyLoading = ref(false)
const historyLoadError = ref('')
const historyRows = ref([])
const historyKeyword = ref('')
const historyState = ref('all')
const historySeverity = ref('all')
const historyTimeRange = ref([])
const historyPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const historyStateOptions = [
  { label: '全部状态', value: 'all' },
  { label: 'firing', value: 'firing' },
  { label: 'resolved', value: 'resolved' },
]

const historySeverityOptions = [
  { label: '全部级别', value: 'all' },
  { label: 'critical', value: 'critical' },
  { label: 'warning', value: 'warning' },
  { label: 'info', value: 'info' },
]

const historyColumns = [
  { title: '名称', dataIndex: 'alertname', key: 'alertname', width: 200 },
  { title: '级别', key: 'severity', width: 100 },
  { title: '状态', key: 'state', width: 100 },
  { title: '实例', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: '开始时间', key: 'started_at', width: 180 },
  { title: '恢复时间', key: 'resolved_at', width: 220 },
]

function buildHistoryQueryParams() {
  const [startTime, endTime] = historyTimeRange.value || []
  return {
    page: historyPagination.current,
    page_size: historyPagination.pageSize,
    keyword: historyKeyword.value || undefined,
    state: historyState.value !== 'all' ? historyState.value : undefined,
    severity: historySeverity.value !== 'all' ? historySeverity.value : undefined,
    start_time: startTime ? startTime.toISOString() : undefined,
    end_time: endTime ? endTime.toISOString() : undefined,
  }
}

async function loadAlertHistory() {
  historyLoading.value = true
  historyLoadError.value = ''
  try {
    const res = await getAlertHistories(buildHistoryQueryParams())
    const payload = parseApiData(res)
    historyRows.value = Array.isArray(payload.results) ? payload.results : []
    historyPagination.total = payload.count || 0
    historyPagination.current = payload.pageNumber || historyPagination.current
  } catch (error) {
    historyLoadError.value = error?.response?.data?.msg || error?.message || '加载历史告警失败'
    message.warning(historyLoadError.value)
    historyRows.value = []
  } finally {
    historyLoading.value = false
    lastRefreshAt.value = new Date()
  }
}

function onHistoryFilterChange() {
  historyPagination.current = 1
  loadAlertHistory()
}

function handleHistoryTableChange(pager) {
  historyPagination.current = pager.current
  historyPagination.pageSize = pager.pageSize
  loadAlertHistory()
}

// 只刷新当前激活的 tab：当前告警走 Prometheus 实时，历史告警走后端分页接口。
function refreshActiveTab() {
  if (activeTabKey.value === 'history') {
    if (historyLoading.value) return
    loadAlertHistory()
  } else {
    if (loading.value) return
    loadAlerts()
  }
}

function clearRefreshTimer() {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
}

function restartRefreshTimer() {
  clearRefreshTimer()
  if (!autoRefreshEnabled.value) return
  const intervalMs = Number(refreshIntervalSeconds.value || 15) * 1000
  refreshTimer = window.setInterval(refreshActiveTab, intervalMs)
}

// 开关/间隔/切 tab 变化时重建定时器，保证只定时刷新当前激活的 tab。
watch(() => autoRefreshEnabled.value, restartRefreshTimer)
watch(() => refreshIntervalSeconds.value, restartRefreshTimer)
watch(() => activeTabKey.value, restartRefreshTimer)

onMounted(() => {
  loadAlerts()
  loadAlertHistory()
  restartRefreshTimer()
})

onBeforeUnmount(() => {
  clearRefreshTimer()
})
</script>

<style scoped>
.alerts-page {
  padding: 12px;
}
</style>
