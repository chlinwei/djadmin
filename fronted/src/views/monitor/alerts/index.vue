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
      <!-- 用 a-tabs 划分当前告警/历史告警，两个主视图统一使用时间线展示。 -->
      <a-tabs v-model:activeKey="activeTabKey">
        <a-tab-pane key="current" tab="当前告警">
          <a-space style="margin-bottom: 12px" wrap>
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
            <a-select
              v-model:value="notificationFilter"
              :options="notificationFilterOptions"
              :getPopupContainer="getPopupContainer"
              style="width: 190px"
            />
            <a-input-search
              v-model:value="keyword"
              placeholder="按名称/实例/摘要搜索"
              allow-clear
              style="width: 260px"
            />
          </a-space>

          <a-alert v-if="loadError" type="error" show-icon :message="loadError" style="margin-bottom: 12px" />

          <a-spin :spinning="loading">
            <a-empty v-if="!filteredRows.length" description="暂无当前告警" />
            <a-table
              v-else
              row-key="key"
              :columns="currentAlertColumns"
              :data-source="currentTimelineEntries"
              :pagination="false"
              :scroll="{ x: 1250 }"
              size="small"
              :row-class-name="timelineRowClassName"
              :expandable="{ rowExpandable: isAlertEntryExpandable }"
            >
              <template #headerCell="{ column }">
                <a-button v-if="column.key === 'active_at'" type="link" size="small" class="time-sort-button" @click="toggleCurrentTimeOrder">
                  时间 {{ currentTimeOrder === 'desc' ? '↓' : '↑' }}
                </a-button>
              </template>
              <template #bodyCell="{ column, record }">
                <template v-if="record.type === 'separator'">
                  <span v-if="column.key === 'active_at'" class="timeline-date-node">{{ record.label }}</span>
                </template>
                <template v-else-if="column.key === 'active_at'">
                  {{ formatTimelineTime(record.record.active_at) }}
                </template>
                <template v-else-if="column.key === 'severity'">
                  <a-tag v-if="record.record.severity" :color="severityColor(record.record.severity)">{{ record.record.severity }}</a-tag>
                  <span v-else>-</span>
                </template>
                <template v-else-if="column.key === 'state'">
                  <a-tag :color="stateColor(record.record.state)">{{ record.record.state || 'unknown' }}</a-tag>
                </template>
                <template v-else-if="column.key === 'rule_group'">{{ record.record.rule_group || '-' }}</template>
                <template v-else-if="column.key === 'summary'">{{ record.record.summary || '-' }}</template>
                <template v-else-if="column.key === 'value'">{{ formatCurrentValue(record.record.value) }}</template>
                <template v-else-if="column.key === 'labels'">
                  <a-space v-if="alertLabelEntries(record.record.labels).length" size="small" wrap>
                    <a-tag v-for="[labelKey, labelValue] in alertLabelEntries(record.record.labels)" :key="labelKey" class="alert-label-tag">
                      {{ labelKey }}={{ labelValue }}
                    </a-tag>
                  </a-space>
                  <span v-else>-</span>
                </template>
                <template v-else-if="column.key === 'operation'">
                  <a-tooltip v-if="record.record.history_id && record.record.notification_count > 0" title="查看日志" placement="top">
                    <a-button type="link" size="small" :class="['notification-summary-action', `is-${record.record.notification_status}`]" @click="openNotificationStatus(record.record.history_id)">
                      <template #icon><EyeOutlined /></template>
                      <a-badge :status="notificationBadgeStatus(record.record.notification_status)" />
                      {{ notificationSummaryLabel(record.record.notification_status) }}
                    </a-button>
                  </a-tooltip>
                  <span v-else>-</span>
                </template>
                <template v-else>{{ record.record[column.dataIndex] || '-' }}</template>
              </template>
              <template #expandedRowRender="{ record }">
                <div v-if="record.type !== 'separator'" class="alert-rule-detail">
                  <p><strong>PromQL：</strong>{{ record.record.rule_details?.query || '-' }}</p>
                  <p v-if="Object.keys(record.record.rule_details?.labels || {}).length"><strong>标签：</strong>{{ formatKeyValues(record.record.rule_details.labels) }}</p>
                  <p v-if="record.record.rule_details?.annotations?.summary"><strong>summary：</strong>{{ record.record.rule_details.annotations.summary }}</p>
                  <p v-if="record.record.rule_details?.annotations?.description"><strong>description：</strong>{{ record.record.rule_details.annotations.description }}</p>
                </div>
              </template>
            </a-table>
            <a-pagination
              v-if="filteredRows.length"
              :current="currentPagination.current"
              :page-size="currentPagination.pageSize"
              :total="filteredRows.length"
              show-size-changer
              show-quick-jumper
              :show-total="currentPagination.showTotal"
              class="alert-table-pagination"
              @change="handleCurrentPaginationChange"
              @showSizeChange="handleCurrentPaginationChange"
            />
          </a-spin>
        </a-tab-pane>

        <a-tab-pane key="history" tab="历史告警" force-render>
          <a-space style="margin-bottom: 12px" wrap>
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
            <a-select
              v-model:value="historyNotificationStatus"
              :options="notificationFilterOptions"
              :getPopupContainer="getPopupContainer"
              style="width: 190px"
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
              :show-time="historyRangeShowTime"
              :presets="historyTimeRangePresets"
              :placeholder="['开始时间', '结束时间']"
              :getPopupContainer="getPopupContainer"
              @openChange="onHistoryRangeOpenChange"
              @change="onHistoryTimeRangeChange"
            />
          </a-space>

          <a-alert v-if="historyLoadError" type="error" show-icon :message="historyLoadError" style="margin-bottom: 12px" />

          <a-spin :spinning="historyLoading">
            <a-empty v-if="!historyRows.length" description="暂无历史告警" />
            <div v-else class="history-table-wrap">
              <a-table
                row-key="key"
                :columns="historyAlertColumns"
                :data-source="historyTimelineEntries"
                :pagination="false"
                :scroll="{ x: 1250 }"
                size="small"
                :row-class-name="timelineRowClassName"
                :expandable="{ rowExpandable: isAlertEntryExpandable }"
              >
                <template #headerCell="{ column }">
                  <a-button v-if="column.key === 'started_at'" type="link" size="small" class="time-sort-button" @click="toggleHistoryTimeOrder">
                    时间 {{ historyOrdering === '-started_at' ? '↓' : '↑' }}
                  </a-button>
                </template>
                <template #bodyCell="{ column, record }">
                  <template v-if="record.type === 'separator'">
                    <span v-if="column.key === 'started_at'" class="timeline-date-node">{{ record.label }}</span>
                  </template>
                  <template v-else-if="column.key === 'started_at'">{{ formatTimelineTime(record.record.started_at) }}</template>
                  <template v-else-if="column.key === 'severity'">
                    <a-tag v-if="record.record.severity" :color="severityColor(record.record.severity)">{{ record.record.severity }}</a-tag>
                    <span v-else>-</span>
                  </template>
                  <template v-else-if="column.key === 'state'"><a-tag :color="stateColor(record.record.state)">{{ record.record.state || 'unknown' }}</a-tag></template>
                  <template v-else-if="column.key === 'rule_group'">{{ record.record.rule_group || '-' }}</template>
                  <template v-else-if="column.key === 'labels'">
                    <a-space v-if="alertLabelEntries(record.record.labels).length" size="small" wrap>
                      <a-tag v-for="[labelKey, labelValue] in alertLabelEntries(record.record.labels)" :key="labelKey" class="alert-label-tag">
                        {{ labelKey }}={{ labelValue }}
                      </a-tag>
                    </a-space>
                    <span v-else>-</span>
                  </template>
                  <template v-else-if="column.key === 'resolved_at'">{{ record.record.resolved_at ? formatActiveAt(record.record.resolved_at) : '仍在告警中' }}</template>
                  <template v-else-if="column.key === 'operation'">
                    <a-tooltip v-if="record.record.notification_count > 0" title="查看日志" placement="top">
                      <a-button type="link" size="small" :class="['notification-summary-action', `is-${record.record.notification_status}`]" @click="openNotificationStatus(record.record.id)">
                        <template #icon><EyeOutlined /></template>
                        <a-badge :status="notificationBadgeStatus(record.record.notification_status)" />
                        {{ notificationSummaryLabel(record.record.notification_status) }}
                      </a-button>
                    </a-tooltip>
                    <span v-else>-</span>
                  </template>
                  <template v-else-if="column.key === 'duration'">{{ formatAlertDuration(record.record.started_at, record.record.resolved_at) }}</template>
                  <template v-else>{{ record.record[column.dataIndex] || '-' }}</template>
                </template>
                <template #expandedRowRender="{ record }">
                  <div v-if="record.type !== 'separator'" class="alert-rule-detail">
                    <p><strong>PromQL：</strong>{{ record.record.rule_details?.query || '-' }}</p>
                    <p v-if="Object.keys(record.record.rule_details?.labels || {}).length"><strong>标签：</strong>{{ formatKeyValues(record.record.rule_details.labels) }}</p>
                    <p v-if="record.record.rule_details?.annotations?.summary"><strong>summary：</strong>{{ record.record.rule_details.annotations.summary }}</p>
                    <p v-if="record.record.rule_details?.annotations?.description"><strong>description：</strong>{{ record.record.rule_details.annotations.description }}</p>
                  </div>
                </template>
              </a-table>
              <a-pagination
                :current="historyPagination.current"
                :page-size="historyPagination.pageSize"
                :total="historyPagination.total"
                show-size-changer
                show-quick-jumper
                :show-total="historyPagination.showTotal"
                class="history-timeline-pagination"
                @change="handleHistoryPaginationChange"
                @showSizeChange="handleHistoryPaginationChange"
              />
            </div>
          </a-spin>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <a-modal
      v-model:open="notificationModalVisible"
      title="通知记录"
      width="1100px"
      :footer="null"
      centered
    >
      <a-descriptions :column="2" size="small" bordered style="margin-bottom: 16px">
        <a-descriptions-item label="告警名称">{{ notificationDetail.alertname || '-' }}</a-descriptions-item>
        <a-descriptions-item label="实例">{{ notificationDetail.instance || '-' }}</a-descriptions-item>
      </a-descriptions>
      <a-table
        row-key="rowKey"
        :columns="notificationColumns"
        :data-source="notificationRows"
        :loading="notificationLoading"
        :pagination="false"
        :scroll="{ x: 1100 }"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'event_type'">
            <a-tag :color="record.event_type === 'firing' ? 'red' : 'green'">
              {{ record.event_type === 'firing' ? '告警' : '恢复' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="notificationStatusColor(record.status)">{{ notificationStatusLabel(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'time'">
            {{ formatActiveAt(record.sent_at || record.create_time) }}
          </template>
        </template>
      </a-table>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { EyeOutlined } from '@ant-design/icons-vue'
import dayjs from 'dayjs'
import { getAlertHistories, getAlertNotificationStatus, getPrometheusAlerts } from '@/api/monitor'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import { formatTimeWithTimezone } from '@/util/timezone'
import { buildUserTimezoneRangePresets, buildUserTimezoneShowTime, toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'
import store from '@/store'

defineOptions({
  name: 'MonitorAlertsPage',
})

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const userTimezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')

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
const firingCount = ref(0)
const resolvedCount = ref(0)
const rawRows = ref([])
const keyword = ref('')
const stateFilter = ref('all')
const severityFilter = ref('all')
const notificationFilter = ref('all')

const notificationFilterOptions = [
  { label: '全部通知状态', value: 'all' },
  { label: '无通知', value: 'none' },
  { label: '发送中', value: 'in_progress' },
  { label: '全部成功', value: 'success' },
  { label: '存在失败', value: 'failed' },
]

const stateFilterOptions = computed(() => {
  const values = Array.from(new Set(rawRows.value.map((row) => row.state).filter((v) => v)))
  return [{ label: '全部状态', value: 'all' }, ...values.map((v) => ({ label: v, value: v }))]
})

const severityFilterOptions = computed(() => {
  const values = Array.from(new Set(rawRows.value.map((row) => row.severity).filter((v) => v)))
  return [{ label: '全部级别', value: 'all' }, ...values.map((v) => ({ label: v, value: v }))]
})

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

function timelineColor(record) {
  if (record.state === 'firing') return 'red'
  if (record.severity === 'warning') return 'orange'
  return 'green'
}

const currentAlertColumns = [
  { title: '时间', dataIndex: 'active_at', key: 'active_at', width: 190 },
  { title: '严重性', key: 'severity', width: 100 },
  { title: '规则组', dataIndex: 'rule_group', key: 'rule_group', width: 180 },
  { title: '状态', key: 'state', width: 100 },
  { title: '主机/实例', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: '问题', key: 'summary', width: 360 },
  { title: '标签', key: 'labels', width: 320 },
  { title: '当前值', dataIndex: 'value', key: 'value', width: 120 },
  { title: '操作', key: 'operation', fixed: 'right', width: 150 },
]

const historyAlertColumns = [
  { title: '时间', dataIndex: 'started_at', key: 'started_at', width: 190 },
  { title: '严重性', key: 'severity', width: 100 },
  { title: '规则组', dataIndex: 'rule_group', key: 'rule_group', width: 180 },
  { title: '状态', key: 'state', width: 100 },
  { title: '主机/实例', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: '标签', key: 'labels', width: 320 },
  { title: '恢复时间', key: 'resolved_at', width: 190 },
  { title: '持续时间', key: 'duration', width: 140 },
  { title: '操作', key: 'operation', fixed: 'right', width: 150 },
]

function timelineRowClassName(record) {
  return record.type === 'separator' ? 'timeline-separator-row' : ''
}

// 按用户时区显示 Prometheus 返回的 activeAt（UTC RFC3339），与全站时间显示规范保持一致
function formatActiveAt(value) {
  if (!value) return '-'
  return formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai')
}

function getTimelineBucket(value, now = dayjs().tz(userTimezone.value)) {
  const date = dayjs(value).tz(userTimezone.value)
  if (!date.isValid()) return 'unknown'
  if (date.isSame(now, 'day')) return 'today'
  if (date.isSame(now.subtract(1, 'day'), 'day')) return 'yesterday'
  return date.format('YYYY-MM')
}

function formatTimelineTime(value) {
  const date = dayjs(value).tz(userTimezone.value)
  if (!date.isValid()) return '-'
  const bucket = getTimelineBucket(value)
  if (bucket === 'today') return date.format('HH:mm:ss')
  return date.format('YYYY年MM月DD日 HH:mm:ss')
}

function formatTimelineBucketLabel(bucket) {
  if (bucket === 'today') return '今天'
  if (bucket === 'yesterday') return '昨天'
  if (bucket === 'unknown') return '时间未知'
  const [year, month] = bucket.split('-')
  const currentYear = dayjs().tz(userTimezone.value).year()
  return Number(year) === currentYear ? `${Number(month)}月` : `${year}年${Number(month)}月`
}

function sortTimelineRows(rows, timeField, order = 'desc') {
  return [...rows].sort((left, right) => {
    const leftTime = dayjs(left[timeField]).valueOf()
    const rightTime = dayjs(right[timeField]).valueOf()
    const leftValue = Number.isFinite(leftTime) ? leftTime : 0
    const rightValue = Number.isFinite(rightTime) ? rightTime : 0
    return order === 'asc' ? leftValue - rightValue : rightValue - leftValue
  })
}

function buildTimelineEntries(rows, timeField, keyPrefix, order = 'desc') {
  const sortedRows = sortTimelineRows(rows, timeField, order)
  const entries = []
  sortedRows.forEach((record, index) => {
    const bucket = getTimelineBucket(record[timeField])
    entries.push({ type: 'record', record, key: `${keyPrefix}-record-${record.rowKey || record.id || index}` })
    const nextRecord = sortedRows[index + 1]
    const nextBucket = nextRecord ? getTimelineBucket(nextRecord[timeField]) : null
    if (bucket !== nextBucket) {
      entries.push({
        type: 'separator',
        label: formatTimelineBucketLabel(bucket),
        key: `${keyPrefix}-separator-${bucket}-${index}`,
      })
    }
  })
  return entries
}

function formatCurrentValue(value) {
  if (value === null || value === undefined || value === '') return '-'
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return String(value)
  return Number(numeric.toFixed(2)).toString()
}

function alertLabelEntries(labels) {
  if (!labels || typeof labels !== 'object' || Array.isArray(labels)) return []
  return Object.entries(labels)
    .map(([key, value]) => [String(key), String(value ?? '')])
    .sort(([leftKey], [rightKey]) => leftKey.localeCompare(rightKey))
}

function formatKeyValues(values) {
  return Object.entries(values || {}).map(([key, value]) => `${key}=${value}`).join(', ')
}

function isAlertEntryExpandable(record) {
  return record.type !== 'separator'
}

const filteredRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return rawRows.value.filter((row) => {
    if (stateFilter.value !== 'all' && row.state !== stateFilter.value) return false
    if (severityFilter.value !== 'all' && row.severity !== severityFilter.value) return false
    if (notificationFilter.value !== 'all' && row.notification_status !== notificationFilter.value) return false
    if (kw === '') return true
    return (
      String(row.name || '').toLowerCase().includes(kw) ||
      String(row.instance || '').toLowerCase().includes(kw) ||
      String(row.summary || '').toLowerCase().includes(kw)
    )
  })
})

const currentTimeOrder = ref('desc')
const currentPagination = reactive({
  current: 1,
  pageSize: 10,
  showTotal: (total) => `共有 ${total} 条数据`,
})
const currentSortedRows = computed(() => sortTimelineRows(filteredRows.value, 'active_at', currentTimeOrder.value))
const currentPageRows = computed(() => {
  const start = (currentPagination.current - 1) * currentPagination.pageSize
  return currentSortedRows.value.slice(start, start + currentPagination.pageSize)
})
const currentTimelineEntries = computed(() => buildTimelineEntries(currentPageRows.value, 'active_at', 'current', currentTimeOrder.value))

function toggleCurrentTimeOrder() {
  currentTimeOrder.value = currentTimeOrder.value === 'desc' ? 'asc' : 'desc'
}

function handleCurrentPaginationChange(page, pageSize) {
  currentPagination.current = page
  currentPagination.pageSize = pageSize
}

async function loadAlerts() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await getPrometheusAlerts()
    const data = parseApiData(res)
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
const historyNotificationStatus = ref('all')
const historyOrdering = ref('-started_at')
const historyTimeRange = ref([])
const historyPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const historyTimelineEntries = computed(() => buildTimelineEntries(
  historyRows.value,
  'started_at',
  'history',
  historyOrdering.value === 'started_at' ? 'asc' : 'desc',
))

function toggleHistoryTimeOrder() {
  historyOrdering.value = historyOrdering.value === '-started_at' ? 'started_at' : '-started_at'
  historyPagination.current = 1
  loadAlertHistory()
}

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

const historyTimeRangePresets = ref([])

function refreshHistoryTimeRangePresets() {
  // 面板打开时重算，确保“过去 N 分钟”基于点击当下，并以用户时区为基准。
  historyTimeRangePresets.value = buildUserTimezoneRangePresets(userTimezone.value)
}

function onHistoryRangeOpenChange(open) {
  if (open) refreshHistoryTimeRangePresets()
}

const historyRangeShowTime = buildUserTimezoneShowTime(userTimezone.value)

const notificationModalVisible = ref(false)
const notificationLoading = ref(false)
const notificationRows = ref([])
const notificationDetail = reactive({
  alertname: '',
  instance: '',
})

const notificationColumns = [
  { title: '事件', key: 'event_type', width: 80 },
  { title: '时间', key: 'time', width: 180 },
  { title: '用户', dataIndex: 'username', key: 'username', width: 130 },
  { title: '媒介', dataIndex: 'media_name', key: 'media_name', width: 150 },
  { title: '地址', dataIndex: 'address', key: 'address', width: 220 },
  { title: '状态', key: 'status', width: 100 },
  { title: '尝试', dataIndex: 'attempt_count', key: 'attempt_count', width: 70 },
]

function notificationStatusColor(status) {
  if (status === 'success') return 'green'
  if (status === 'failed') return 'red'
  if (status === 'sending') return 'blue'
  return 'orange'
}

function notificationStatusLabel(status) {
  return {
    pending: '等待发送',
    sending: '发送中',
    success: '已发送',
    failed: '发送失败',
  }[status] || status || '未知'
}

function notificationBadgeStatus(status) {
  return {
    success: 'success',
    failed: 'error',
    in_progress: 'processing',
  }[status] || 'default'
}

function notificationSummaryLabel(status) {
  return {
    success: '全部成功',
    failed: '存在失败',
    in_progress: '发送中',
  }[status] || '通知记录'
}

watch(notificationModalVisible, (open) => {
  document.body.classList.toggle('notification-modal-scroll-stable', open)
}, { flush: 'sync' })

async function openNotificationStatus(alertId) {
  if (!alertId) {
    message.info('该实时告警尚未生成本地通知记录')
    return
  }

  notificationModalVisible.value = true
  notificationLoading.value = true
  notificationRows.value = []
  try {
    const res = await getAlertNotificationStatus(alertId)
    const data = parseApiData(res)
    notificationDetail.alertname = data.alertname || ''
    notificationDetail.instance = data.instance || ''
    notificationRows.value = (data.events || []).flatMap((event) => {
      if (Array.isArray(event.deliveries) && event.deliveries.length) {
        return event.deliveries.map((delivery) => ({
          rowKey: `${event.id}-${delivery.id}`,
          event_type: event.event_type,
          ...delivery,
        }))
      }
      return [{
        rowKey: `${event.id}-event`,
        event_type: event.event_type,
        username: '未记录',
        media_name: '未记录',
        address: '未记录',
        status: event.status,
        attempt_count: event.attempt_count,
        error_message: event.error_message,
        sent_at: event.sent_at,
        create_time: event.create_time,
      }]
    })
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '加载通知记录失败')
  } finally {
    notificationLoading.value = false
  }
}

function formatAlertDuration(startedAt, resolvedAt) {
  if (!startedAt) return '-'
  const startMs = new Date(startedAt).getTime()
  const endMs = resolvedAt ? new Date(resolvedAt).getTime() : Date.now()
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs < startMs) return '-'

  let seconds = Math.floor((endMs - startMs) / 1000)
  const days = Math.floor(seconds / 86400)
  seconds -= days * 86400
  const hours = Math.floor(seconds / 3600)
  seconds -= hours * 3600
  const minutes = Math.floor(seconds / 60)
  seconds -= minutes * 60

  if (days > 0) return `${days}天 ${hours}小时 ${minutes}分`
  if (hours > 0) return `${hours}小时 ${minutes}分 ${seconds}秒`
  if (minutes > 0) return `${minutes}分 ${seconds}秒`
  return `${seconds}秒`
}

function normalizeDateTimeParam(value) {
  if (!value) return undefined
  return toUtcQueryISOStringByUserTimezone(value, userTimezone.value)
}

function buildHistoryQueryParams() {
  const [startTime, endTime] = historyTimeRange.value || []
  return {
    page: historyPagination.current,
    page_size: historyPagination.pageSize,
    keyword: historyKeyword.value || undefined,
    state: historyState.value !== 'all' ? historyState.value : undefined,
    severity: historySeverity.value !== 'all' ? historySeverity.value : undefined,
    notification_status: historyNotificationStatus.value !== 'all' ? historyNotificationStatus.value : undefined,
    ordering: historyOrdering.value,
    // 历史告警时间过滤统一按 started_at（开始时间）范围过滤。
    start_time: normalizeDateTimeParam(startTime),
    end_time: normalizeDateTimeParam(endTime),
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

function onHistoryTimeRangeChange(dates) {
  if (!Array.isArray(dates) || !dates[0] || !dates[1]) {
    historyTimeRange.value = []
    onHistoryFilterChange()
    return
  }

  // 保留用户或快捷项选择的精确时分秒（分钟/小时级快捷范围依赖该行为）。
  historyTimeRange.value = [dayjs(dates[0]), dayjs(dates[1])]
  onHistoryFilterChange()
}

function handleHistoryPaginationChange(page, pageSize) {
  historyPagination.current = page
  historyPagination.pageSize = pageSize
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
watch([keyword, stateFilter, severityFilter, notificationFilter], () => {
  currentPagination.current = 1
})
watch(() => filteredRows.value.length, (total) => {
  // 实时刷新可能减少告警数量，页码需收敛到仍然存在的最后一页。
  const lastPage = Math.max(1, Math.ceil(total / currentPagination.pageSize))
  if (currentPagination.current > lastPage) currentPagination.current = lastPage
})

useKeepAliveRefreshLifecycle(restartRefreshTimer, clearRefreshTimer)

onMounted(() => {
  refreshHistoryTimeRangePresets()
  loadAlerts()
  loadAlertHistory()
  restartRefreshTimer()
})

onBeforeUnmount(() => {
  clearRefreshTimer()
  document.body.classList.remove('notification-modal-scroll-stable')
})
</script>

<style scoped>
.alerts-page {
  padding: 12px;
}

.notification-summary-action.is-success {
  color: #389e0d;
}

.notification-summary-action.is-failed {
  color: #cf1322;
}

.notification-summary-action.is-in_progress {
  color: #1677ff;
}

.alert-timeline-wrap {
  width: min(100%, 1120px);
  margin: 0 auto;
  padding: 20px 16px 4px;
}

.alert-timeline-item {
  min-height: 112px;
  padding: 0 0 22px 8px;
  border-bottom: 1px solid #f0f0f0;
}

.alert-timeline-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
}

.alert-timeline-time {
  color: rgba(0, 0, 0, 0.65);
  font-variant-numeric: tabular-nums;
}

.alert-timeline-summary {
  margin-bottom: 10px;
  color: rgba(0, 0, 0, 0.88);
}

.alert-timeline-meta {
  display: grid;
  grid-template-columns: minmax(180px, 1.4fr) minmax(140px, 0.7fr);
  gap: 12px 24px;
  color: rgba(0, 0, 0, 0.88);
}

.alert-timeline-action {
  margin-top: 8px;
}

.timeline-date-node {
  display: inline-block;
  margin: 2px 0 18px;
  padding: 3px 10px;
  border: 1px solid #d9d9d9;
  border-radius: 3px;
  color: rgba(0, 0, 0, 0.65);
  background: #fafafa;
  font-size: 12px;
  font-weight: 600;
}

:deep(.timeline-separator-row > td) {
  height: 34px;
  padding-top: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid #d9d9d9;
  background: #fafafa;
}

:deep(.timeline-separator-row .ant-table-row-expand-icon) {
  visibility: hidden;
  pointer-events: none;
}

.history-table-wrap {
  width: 100%;
}

.time-sort-button {
  padding: 0;
  color: rgba(0, 0, 0, 0.88);
  font-weight: 600;
}

.alert-rule-detail {
  padding: 4px 16px;
  color: rgba(0, 0, 0, 0.88);
  background: #fafafa;
}

.alert-rule-detail p {
  margin: 4px 0;
  word-break: break-all;
}

.history-timeline-wrap {
  width: min(100%, 1120px);
  margin: 0 auto;
  padding: 20px 16px 4px;
}

.history-timeline-item {
  min-height: 112px;
  padding: 0 0 22px 8px;
  border-bottom: 1px solid #f0f0f0;
}

.history-timeline-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
}

.history-timeline-time {
  color: rgba(0, 0, 0, 0.65);
  font-variant-numeric: tabular-nums;
}

.history-timeline-meta {
  display: grid;
  grid-template-columns: minmax(180px, 1.4fr) minmax(220px, 1fr) minmax(140px, 0.7fr);
  gap: 12px 24px;
  color: rgba(0, 0, 0, 0.88);
}

.meta-label {
  margin-right: 8px;
  color: rgba(0, 0, 0, 0.45);
}

.history-timeline-action {
  margin-top: 8px;
}

.history-timeline-labels {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  margin-top: 10px;
}

.alert-label-tag {
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.alert-table-pagination,
.history-timeline-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

@media (max-width: 900px) {
  .history-timeline-meta,
  .alert-timeline-meta {
    grid-template-columns: 1fr;
  }
}

:global(html body.notification-modal-scroll-stable) {
  overflow-y: visible !important;
  width: auto !important;
}

</style>
