<template>
  <div class="log-query-panel">
    <a-empty
      v-if="!scope.applicationServiceId"
      class="log-query-empty"
      description="请选择左侧逻辑服务或部署实例查看日志"
    />

    <template v-else>
      <header class="log-query-header">
        <div>
          <div class="log-query-kicker">{{ scope.businessSystemName || '-' }} / {{ scope.environmentName || '-' }}</div>
          <h2>{{ scope.nodeTitle }}</h2>
        </div>
        <a-tooltip title="刷新">
          <a-button type="primary" ghost :loading="activeLoading" @click="reload">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="activeLoading" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-tooltip>
      </header>

      <a-row class="log-query-filters" :gutter="16">
        <a-col :span="8">
          <a-range-picker
            v-model:value="filters.timeRange"
            class="filter-item"
            :show-time="timeRangeShowTime"
            :presets="timeRangePresets"
            size="large"
            format="YYYY-MM-DD HH:mm:ss"
            :placeholder="['开始时间', '结束时间']"
            :getPopupContainer="getPopupContainer"
            @openChange="onTimeRangeOpenChange"
            @change="handleFilterChange"
          />
        </a-col>
        <a-col :span="6">
          <a-input-search
            v-model:value="filters.keyword"
            class="filter-item"
            placeholder="搜索日志内容"
            allow-clear
            size="large"
            @search="handleFilterChange"
          />
        </a-col>
        <a-col :span="5">
          <a-select
            v-model:value="filters.logLevels"
            class="filter-item"
            mode="tags"
            size="large"
            allow-clear
            placeholder="全部级别"
            :options="logLevelOptions"
            :getPopupContainer="getPopupContainer"
            @change="handleFilterChange"
          />
        </a-col>
        <a-col :span="5">
          <a-input
            v-model:value="filters.instance"
            class="filter-item"
            size="large"
            placeholder="按实例名精确过滤"
            allow-clear
            :disabled="scope.nodeType === 'deployment'"
            @press-enter="handleFilterChange"
            @change="(event) => { if (!event.target.value) handleFilterChange() }"
          />
        </a-col>
      </a-row>

      <div class="log-query-range-hint">当前查询范围：{{ effectiveRangeLabel }}</div>

      <a-space v-if="filters.hostIp || filters.logName || filters.errorFingerprint" class="log-query-chip-row" wrap>
        <span class="chip-row-label">下钻过滤：</span>
        <a-tag v-if="filters.hostIp" closable @close="clearDrillFilter('hostIp')">主机 = {{ filters.hostIp }}</a-tag>
        <a-tag v-if="filters.logName" closable @close="clearDrillFilter('logName')">日志文件 = {{ filters.logName }}</a-tag>
        <a-tag v-if="filters.errorFingerprint" closable @close="clearDrillFilter('errorFingerprint')">错误指纹 = {{ filters.errorFingerprint }}</a-tag>
      </a-space>

      <a-tabs v-model:active-key="activeTab" @change="handleTabChange">
        <a-tab-pane key="logs" tab="日志查询">
          <a-alert
            v-if="pagination.total >= MAX_RESULT_WINDOW"
            class="log-query-window-alert"
            type="warning"
            show-icon
            message="命中日志超过 2000 条上限，请缩小时间范围或增加过滤条件以查看更早的记录"
          />

          <a-table
            row-key="id"
            :columns="columns"
            :data-source="logs"
            :loading="loading"
            :pagination="pagination"
            :scroll="{ x: 1200 }"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'timestamp'">
                {{ formatTime(record['@timestamp']) }}
              </template>
              <template v-else-if="column.key === 'log_level'">
                <a-tag :color="LOG_LEVEL_COLOR[String(record.log_level || '').toUpperCase()] || 'default'">
                  {{ record.log_level || '-' }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <a-tooltip title="查看详情">
                  <a-button size="small" type="link" @click="openDetail(record)">查看详情</a-button>
                </a-tooltip>
              </template>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="stats" tab="统计" force-render>
          <div class="stats-toolbar">
            <span class="stats-toolbar-label">统计维度</span>
            <a-select
              v-model:value="statsField"
              class="stats-field-select"
              :options="statsFieldOptions"
              :getPopupContainer="getPopupContainer"
              @change="loadStats"
            />
            <span class="stats-toolbar-label">分桶粒度</span>
            <a-select
              v-model:value="statsIntervalOption"
              class="stats-interval-select"
              :options="statsIntervalOptions"
              :getPopupContainer="getPopupContainer"
              @change="loadStats"
            />
            <span class="stats-toolbar-hint">Top {{ statsBuckets.length }} · 按 {{ statsIntervalMinutes }} 分钟分桶</span>
          </div>

          <div ref="trendChartRef" class="stats-trend-chart"></div>

          <a-table
            row-key="value"
            :columns="statsColumns"
            :data-source="statsBuckets"
            :loading="statsLoading"
            :pagination="false"
            :scroll="{ x: 900 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'value'">{{ record.value ?? '-' }}</template>
              <template v-else-if="column.key === 'sample_message'">{{ record.sample?.log_message || '-' }}</template>
              <template v-else-if="column.key === 'action'">
                <a-space :size="6">
                  <a-tooltip title="查看日志">
                    <a-button size="small" type="link" @click="drillDown(record)">查看日志</a-button>
                  </a-tooltip>
                  <a-tooltip title="查看详情">
                    <a-button size="small" type="link" :disabled="!record.sample" @click="openDetail(record.sample)">查看详情</a-button>
                  </a-tooltip>
                </a-space>
              </template>
            </template>
          </a-table>
        </a-tab-pane>
      </a-tabs>
    </template>

    <a-drawer v-model:open="detailOpen" title="日志详情" width="640" placement="right">
      <a-descriptions bordered :column="1" size="small">
        <a-descriptions-item label="时间">{{ formatTime(activeLog?.['@timestamp']) }}</a-descriptions-item>
        <a-descriptions-item label="级别">{{ activeLog?.log_level || '-' }}</a-descriptions-item>
        <a-descriptions-item label="服务">{{ activeLog?.service || '-' }}</a-descriptions-item>
        <a-descriptions-item label="实例">{{ activeLog?.instance || '-' }}</a-descriptions-item>
        <a-descriptions-item label="主机">{{ activeLog?.host_ip || '-' }}</a-descriptions-item>
        <a-descriptions-item label="日志名称">{{ activeLog?.log_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="日志路径">{{ activeLog?.log_path || '-' }}</a-descriptions-item>
        <a-descriptions-item label="错误指纹" v-if="activeLog?.error_fingerprint">{{ activeLog.error_fingerprint }}</a-descriptions-item>
      </a-descriptions>
      <div class="detail-section-title">原始消息</div>
      <pre class="detail-content">{{ activeLog?.log_message || '-' }}</pre>
      <template v-if="activeLog?.app_fields && Object.keys(activeLog.app_fields).length">
        <div class="detail-section-title">应用私有字段</div>
        <pre class="detail-content">{{ formatJsonText(activeLog.app_fields) }}</pre>
      </template>
    </a-drawer>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import * as echarts from 'echarts'
import dayjs from 'dayjs'
import { getOpenSearchClusterList, searchOpenSearchLogFacetStats, searchOpenSearchLogs } from '@/api/monitor'
import { formatTimeWithTimezone } from '@/util/timezone'
import { buildUserTimezoneRangePresets, buildUserTimezoneShowTime, toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import store from '@/store'

// 与后端 LOG_SEARCH_MAX_RESULT_WINDOW 保持一致：超过该上限需要用户缩小范围，而不是无限深翻页。
const MAX_RESULT_WINDOW = 2000
const LOG_LEVEL_COLOR = {
  ERROR: 'red', SEVERE: 'red', FATAL: 'red',
  WARN: 'orange', WARNING: 'orange',
  INFO: 'blue', DEBUG: 'default', TRACE: 'default',
}
// 与后端 LOG_FACET_ALLOWED_FIELDS 一一对应，用于把统计面板点击的分面值映射回过滤条件字段。
const FACET_FILTER_KEY = {
  error_fingerprint: 'errorFingerprint',
  log_level: 'logLevels',
  instance: 'instance',
  host_ip: 'hostIp',
  log_name: 'logName',
}
const statsFieldOptions = [
  { label: '错误指纹', value: 'error_fingerprint' },
  { label: '日志级别', value: 'log_level' },
  { label: '实例', value: 'instance' },
  { label: '主机', value: 'host_ip' },
  { label: '日志文件', value: 'log_name' },
]
// 'auto' 交给后端按时间跨度自适应；其余选项是分钟数，后端会根据总桶数上限拒绝过细的选择。
const statsIntervalOptions = [
  { label: '自动', value: 'auto' },
  { label: '1 分钟', value: '1' },
  { label: '5 分钟', value: '5' },
  { label: '15 分钟', value: '15' },
  { label: '30 分钟', value: '30' },
  { label: '1 小时', value: '60' },
  { label: '6 小时', value: '360' },
  { label: '1 天', value: '1440' },
]

const props = defineProps({
  // 服务树当前选中的节点范围；只有 nodeType 为 service/deployment（带 applicationServiceId）时才能查日志。
  scope: { type: Object, required: true },
})

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const userTimezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const formatTime = (value) => (value ? formatTimeWithTimezone(value, userTimezone.value) : '-')

const clusterId = ref(null)
const activeTab = ref('logs')
const loading = ref(false)
const logs = ref([])
const detailOpen = ref(false)
const activeLog = ref(null)

const statsField = ref('error_fingerprint')
const statsIntervalOption = ref('auto')
const statsLoading = ref(false)
const statsBuckets = ref([])
const statsIntervalMinutes = ref(1)
const trendChartRef = ref(null)
let trendChart = null

// 顶部刷新按钮的 loading 态跟随当前激活的 tab，避免切到统计页时按钮显示还停在日志查询的状态。
const activeLoading = computed(() => (activeTab.value === 'stats' ? statsLoading.value : loading.value))

const filters = reactive({
  // 与后端 _log_search_time_range 的默认窗口保持一致，避免选择器留白让人看不出实际生效的时间范围。
  timeRange: [dayjs().tz(userTimezone.value).subtract(1, 'hour'), dayjs().tz(userTimezone.value)],
  keyword: '',
  logLevels: [],
  instance: '',
  // 以下三项只通过统计面板“查看日志”下钻写入，普通场景不提供输入框，避免筛选面板过于臃肿。
  hostIp: '',
  logName: '',
  errorFingerprint: '',
})

// 选择器一旦被清空，effectiveRangeLabel 兜底显示后端实际会生效的默认范围，而不是留白让人误以为“不限时间”。
const effectiveRangeLabel = computed(() => {
  const [startTime, endTime] = filters.timeRange || []
  if (!startTime || !endTime) return '最近 1 小时（未选择时的默认范围）'
  return `${formatTime(startTime)} ~ ${formatTime(endTime)}`
})

// 日志级别不同应用可能用不同的命名约定（如 Java SEVERE），不能硬编码；实际可选值改为从当前服务的真实数据中动态拉取（见 loadLogLevelOptions）。
const logLevelOptions = ref([])

const timeRangePresets = ref([])
const timeRangeShowTime = buildUserTimezoneShowTime(userTimezone.value)

function refreshTimeRangePresets() {
  timeRangePresets.value = buildUserTimezoneRangePresets(userTimezone.value)
}

function onTimeRangeOpenChange(open) {
  if (open) refreshTimeRangePresets()
}

const pagination = reactive({
  current: 1,
  pageSize: 100,
  total: 0,
  showSizeChanger: true,
  pageSizeOptions: ['50', '100', '200'],
  showTotal: (total) => `共有${total}条数据`,
})

const columns = [
  { title: '时间', key: 'timestamp', width: 190, fixed: 'left' },
  { title: '级别', dataIndex: 'log_level', key: 'log_level', width: 90 },
  { title: '实例', dataIndex: 'instance', key: 'instance', width: 160, ellipsis: true },
  { title: '主机', dataIndex: 'host_ip', key: 'host_ip', width: 130 },
  { title: '消息', dataIndex: 'log_message', key: 'log_message', ellipsis: true },
  { title: '操作', key: 'action', width: 100, fixed: 'right' },
]

const statsValueColumnTitle = computed(
  () => statsFieldOptions.find((item) => item.value === statsField.value)?.label || '值',
)
const statsColumns = computed(() => [
  { title: statsValueColumnTitle.value, key: 'value', width: 220, ellipsis: true },
  { title: '次数', dataIndex: 'count', key: 'count', width: 100 },
  { title: '样例消息', key: 'sample_message', ellipsis: true },
  { title: '操作', key: 'action', width: 160, fixed: 'right' },
])

// 服务树上切换节点时（不同于日志查询独立页自带树），由父组件传入新的 scope，这里跟着重置查询状态。
watch(() => props.scope, () => {
  filters.instance = props.scope.nodeType === 'deployment' ? props.scope.nodeTitle : ''
  pagination.current = 1
  loadLogLevelOptions()
  reloadActiveTab()
}, { deep: true, immediate: true })

function clearDrillFilter(key) {
  filters[key] = ''
  reloadActiveTab()
}

function buildBaseParams() {
  const [startTime, endTime] = filters.timeRange || []
  return {
    application_service_id: props.scope.applicationServiceId,
    start: toUtcQueryISOStringByUserTimezone(startTime, userTimezone.value),
    end: toUtcQueryISOStringByUserTimezone(endTime, userTimezone.value),
    keyword: filters.keyword || undefined,
    log_level: (filters.logLevels || []).join(',') || undefined,
    instance: filters.instance || undefined,
    host_ip: filters.hostIp || undefined,
    log_name: filters.logName || undefined,
    error_fingerprint: filters.errorFingerprint || undefined,
  }
}

function buildQueryParams() {
  return {
    ...buildBaseParams(),
    size: pagination.pageSize,
    offset: (pagination.current - 1) * pagination.pageSize,
  }
}

function buildFacetParams() {
  return {
    ...buildBaseParams(),
    field: statsField.value,
    size: 20,
    interval_minutes: statsIntervalOption.value === 'auto' ? undefined : statsIntervalOption.value,
  }
}

async function ensureClusterId() {
  if (clusterId.value) return clusterId.value
  const response = await getOpenSearchClusterList({ page: 1, page_size: 100 })
  const results = response?.data?.data?.results || []
  const cluster = results.find((item) => item.is_default) || results[0]
  clusterId.value = cluster?.id || null
  return clusterId.value
}

async function loadLogs() {
  if (!props.scope.applicationServiceId) {
    logs.value = []
    pagination.total = 0
    return
  }
  loading.value = true
  try {
    const id = await ensureClusterId()
    if (!id) {
      message.warning('尚未配置日志存储集群')
      logs.value = []
      pagination.total = 0
      return
    }
    const response = await searchOpenSearchLogs(id, buildQueryParams())
    const payload = response?.data?.data || {}
    logs.value = payload.results || []
    pagination.total = Math.min(payload.count || 0, MAX_RESULT_WINDOW)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '查询日志失败')
  } finally {
    loading.value = false
  }
}

async function loadStats() {
  if (!props.scope.applicationServiceId) {
    statsBuckets.value = []
    return
  }
  statsLoading.value = true
  try {
    const id = await ensureClusterId()
    if (!id) {
      message.warning('尚未配置日志存储集群')
      statsBuckets.value = []
      return
    }
    const response = await searchOpenSearchLogFacetStats(id, buildFacetParams())
    const payload = response?.data?.data || {}
    statsBuckets.value = payload.buckets || []
    statsIntervalMinutes.value = payload.interval_minutes || 1
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '统计失败')
  } finally {
    statsLoading.value = false
    await nextTick()
    renderTrendChart()
  }
}

function reloadActiveTab() {
  if (activeTab.value === 'stats') {
    loadStats()
  } else {
    loadLogs()
  }
}

function handleFilterChange() {
  pagination.current = 1
  // 级别下拉的可选项是按当前时间范围/关键字等条件统计出来的，筛选条件一变就必须联动重新拉取，
  // 否则会出现“时间范围内已经有日志，但级别下拉仍是最初空结果”的不一致。
  loadLogLevelOptions()
  reloadActiveTab()
}

function handleTableChange(pager) {
  pagination.current = pager.current
  pagination.pageSize = pager.pageSize
  loadLogs()
}

function handleTabChange(key) {
  activeTab.value = key
  reloadActiveTab()
  if (key === 'stats') {
    nextTick(() => {
      ensureTrendChart()
      trendChart?.resize()
    })
  }
}

function reload() {
  loadLogLevelOptions()
  reloadActiveTab()
}

defineExpose({ reload })

// 复用分面统计接口拉当前服务+时间范围内实际出现过的级别值，避免下拉选项与真实数据脱节。
async function loadLogLevelOptions() {
  if (!props.scope.applicationServiceId) {
    logLevelOptions.value = []
    return
  }
  try {
    const id = await ensureClusterId()
    if (!id) return
    const params = buildBaseParams()
    delete params.log_level
    const response = await searchOpenSearchLogFacetStats(id, { ...params, field: 'log_level', size: 50 })
    const buckets = response?.data?.data?.buckets || []
    logLevelOptions.value = buckets
      .filter((bucket) => bucket.value != null && bucket.value !== '')
      .map((bucket) => ({ label: String(bucket.value), value: String(bucket.value) }))
  } catch (error) {
    // 下拉可选值只是辅助提示，加载失败不阻塞主查询，mode="tags" 仍可手动输入级别。
    logLevelOptions.value = []
  }
}

// 统计面板点击“查看日志”：把当前分面值写回对应过滤条件，切到日志查询 tab 实现下钻。
function drillDown(bucket) {
  const key = FACET_FILTER_KEY[statsField.value]
  if (!key) return
  const value = bucket?.value != null ? String(bucket.value) : ''
  if (key === 'logLevels') {
    filters.logLevels = value ? [value] : []
  } else {
    filters[key] = value
  }
  activeTab.value = 'logs'
  pagination.current = 1
  loadLogs()
}

function openDetail(record) {
  if (!record) return
  activeLog.value = record
  detailOpen.value = true
}

function formatJsonText(value) {
  try {
    return JSON.stringify(value, null, 2)
  } catch (error) {
    return String(value)
  }
}

function ensureTrendChart() {
  if (trendChartRef.value && !trendChart) {
    trendChart = echarts.init(trendChartRef.value)
  }
}

function renderTrendChart() {
  ensureTrendChart()
  if (!trendChart) return
  // 折线数最多画 8 条：分面值可能有 20 条，全部画上去图例会挤成一团，反而看不清趋势。
  const top = statsBuckets.value.slice(0, 8)
  const timestamps = (top[0]?.trend || []).map((point) => formatTime(point.timestamp))
  trendChart.setOption({
    tooltip: { trigger: 'axis' },
    legend: { type: 'scroll', bottom: 0 },
    grid: { left: 56, right: 24, top: 24, bottom: 56 },
    xAxis: { type: 'category', data: timestamps, axisLabel: { hideOverlap: true } },
    yAxis: { type: 'value' },
    series: top.map((bucket) => ({
      name: String(bucket.value ?? '-'),
      type: 'line',
      smooth: true,
      data: (bucket.trend || []).map((point) => point.count),
    })),
  }, true)
}

function resizeTrendChart() {
  trendChart?.resize()
}

onMounted(() => {
  refreshTimeRangePresets()
  window.addEventListener('resize', resizeTrendChart)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeTrendChart)
  trendChart?.dispose()
  trendChart = null
})
</script>

<style scoped>
.log-query-panel {
  min-width: 0;
}
.log-query-empty {
  margin-top: 80px;
}
.log-query-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.log-query-header h2 {
  margin: 0;
  font-size: 20px;
}
.log-query-kicker {
  color: #7a8697;
  font-size: 13px;
}
.log-query-filters {
  margin-bottom: 12px;
}
.filter-item {
  width: 100%;
}
.log-query-range-hint {
  margin: -4px 0 12px;
  color: #7a8697;
  font-size: 12px;
}
.log-query-window-alert {
  margin-bottom: 12px;
}
.log-query-chip-row {
  margin-bottom: 12px;
}
.chip-row-label {
  color: #7a8697;
  font-size: 13px;
}
.stats-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.stats-toolbar-label {
  color: #7a8697;
  font-size: 13px;
}
.stats-field-select {
  width: 160px;
}
.stats-interval-select {
  width: 120px;
}
.stats-toolbar-hint {
  margin-left: auto;
  color: #7a8697;
  font-size: 12px;
}
.stats-trend-chart {
  width: 100%;
  height: 280px;
  margin-bottom: 12px;
}
.detail-section-title {
  margin: 16px 0 8px;
  font-weight: 600;
}
.detail-content {
  white-space: pre-wrap;
  word-break: break-all;
  background: #f5f5f5;
  padding: 8px 12px;
  border-radius: 4px;
  max-height: 320px;
  overflow: auto;
}
</style>
