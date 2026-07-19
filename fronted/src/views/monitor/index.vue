<template>
  <div class="monitor-page">
    <a-row :gutter="12" class="tools">
      <a-col :span="16">
        <a-space>
          <a-tag color="blue">Prometheus</a-tag>
          <span class="prom-url">{{ prometheusBaseUrl || '-' }}</span>
          <a-tag v-if="lastRefreshAtText" color="default">刷新于 {{ lastRefreshAtText }}</a-tag>
        </a-space>
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-switch v-model:checked="autoRefreshEnabled" checked-children="自动刷新" un-checked-children="手动" />
          <a-select v-model:value="refreshIntervalSeconds" style="width: 120px" :options="refreshIntervalOptions" :disabled="!autoRefreshEnabled" />
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="loading" @click="loadAllData">
              刷新
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <div class="overview-grid">
      <a-card size="small" class="overview-card">
        <a-statistic title="监控目标总数" :value="overview.total" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="采集正常" :value="overview.up" :value-style="{ color: '#3f8600' }" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="采集异常" :value="overview.down" :value-style="{ color: '#cf1322' }" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="触发告警" :value="alertSummary.firing" :value-style="{ color: '#cf1322' }" />
      </a-card>
    </div>

    <a-card title="智能监控" size="small" class="monitor-card">
      <a-alert
        v-if="errorMessage"
        type="warning"
        show-icon
        :message="errorMessage"
        style="margin-bottom: 12px"
      />

      <a-tabs v-model:activeKey="activeTabKey">
        <a-tab-pane key="prom-targets" tab="Prometheus 采集目标">
          <a-table
            rowKey="instance"
            :columns="promTargetColumns"
            :data-source="promTargets"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="{ pageSize: 10, showSizeChanger: true }"
          />
        </a-tab-pane>

        <a-tab-pane key="alerts" tab="Prometheus 告警">
          <a-table
            :rowKey="alertRowKey"
            :columns="alertColumns"
            :data-source="alerts"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="{ pageSize: 10, showSizeChanger: true }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'state'">
                <a-tag :color="record.state === 'firing' ? 'red' : 'default'">{{ record.state || 'unknown' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'severity'">
                <a-tag :color="severityColor(record.severity)">{{ record.severity || '-' }}</a-tag>
              </template>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="managed-targets" tab="纳管目标">
          <a-table
            rowKey="id"
            :columns="managedColumns"
            :data-source="managedTargets"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="managedPagination"
            @change="handleManagedTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'managed_enabled'">
                <a-tag :color="record.managed_enabled ? 'green' : 'default'">{{ record.managed_enabled ? '启用' : '禁用' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'install_status'">
                <a-tag :color="statusColor(record.install_status)">{{ record.install_status || 'unknown' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'last_scrape_status'">
                <a-tag :color="scrapeColor(record.last_scrape_status)">{{ record.last_scrape_status || 'unknown' }}</a-tag>
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
import {
  getManagedTargets,
  getMonitorSummary,
  getPrometheusAlerts,
  getPrometheusOverview,
  getPrometheusTargets,
} from '@/api/sys/monitor'

const loading = ref(false)
const errorMessage = ref('')
const activeTabKey = ref('prom-targets')

const lastRefreshAt = ref(null)
const lastRefreshAtText = computed(() => {
  if (!lastRefreshAt.value) return ''
  return lastRefreshAt.value.toLocaleTimeString('zh-CN', { hour12: false })
})

const prometheusBaseUrl = ref('')
const overview = reactive({ total: 0, up: 0, down: 0 })
const alertSummary = reactive({ firing: 0, resolved: 0 })

const promTargets = ref([])
const alerts = ref([])
const managedTargets = ref([])

const managedPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

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

const promTargetColumns = [
  { title: 'Job', dataIndex: 'job', key: 'job', width: 140 },
  { title: 'Instance', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: 'Health', dataIndex: 'health', key: 'health', width: 100 },
  { title: 'Scrape Pool', dataIndex: 'scrape_pool', key: 'scrape_pool', width: 180 },
  { title: 'Last Scrape', dataIndex: 'last_scrape', key: 'last_scrape', width: 220 },
  { title: 'Scrape URL', dataIndex: 'scrape_url', key: 'scrape_url', width: 260 },
  { title: 'Last Error', dataIndex: 'last_error', key: 'last_error', width: 260 },
]

const alertColumns = [
  { title: '告警名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '级别', dataIndex: 'severity', key: 'severity', width: 100 },
  { title: '状态', dataIndex: 'state', key: 'state', width: 100 },
  { title: '实例', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: '摘要', dataIndex: 'summary', key: 'summary', width: 360 },
  { title: '激活时间', dataIndex: 'active_at', key: 'active_at', width: 220 },
]

const managedColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 180 },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 160 },
  { title: 'Exporter', dataIndex: 'exporter_type', key: 'exporter_type', width: 150 },
  { title: '端口', dataIndex: 'exporter_port', key: 'exporter_port', width: 90 },
  { title: '纳管', dataIndex: 'managed_enabled', key: 'managed_enabled', width: 90 },
  { title: '安装状态', dataIndex: 'install_status', key: 'install_status', width: 120 },
  { title: '采集状态', dataIndex: 'last_scrape_status', key: 'last_scrape_status', width: 120 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
]

function statusColor(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'success') return 'green'
  if (value === 'failed') return 'red'
  if (value === 'pending') return 'orange'
  return 'default'
}

function scrapeColor(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'up') return 'green'
  if (value === 'down') return 'red'
  return 'default'
}

function severityColor(severity) {
  const value = String(severity || '').toLowerCase()
  if (value === 'critical') return 'red'
  if (value === 'warning') return 'orange'
  return 'default'
}

function alertRowKey(record) {
  return `${record.name || '-'}:${record.instance || '-'}:${record.active_at || '-'}`
}

function parseApiData(resp) {
  return resp?.data?.data || {}
}

async function loadMonitorSummary() {
  const res = await getMonitorSummary()
  const data = parseApiData(res)
  const targets = data.targets || {}
  return {
    total: Number(targets.total || 0),
    managedEnabled: Number(targets.managed_enabled || 0),
    scrapeUp: Number(targets.scrape_up || 0),
  }
}

async function loadPromOverview() {
  const res = await getPrometheusOverview()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus overview 查询失败')
  }
  const targets = data.targets || {}
  return {
    baseUrl: String(data.prometheus_base_url || ''),
    total: Number(targets.total || 0),
    up: Number(targets.up || 0),
    down: Number(targets.down || 0),
  }
}

async function loadPromTargets() {
  const res = await getPrometheusTargets()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus targets 查询失败')
  }
  return Array.isArray(data.results) ? data.results : []
}

async function loadPromAlerts() {
  const res = await getPrometheusAlerts()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus alerts 查询失败')
  }
  return {
    firingCount: Number(data.firing_count || 0),
    resolvedCount: Number(data.resolved_count || 0),
    rows: Array.isArray(data.results) ? data.results : [],
  }
}

async function loadManagedTargets() {
  const res = await getManagedTargets({
    page: managedPagination.current,
    page_size: managedPagination.pageSize,
    ordering: '-id',
  })
  const data = parseApiData(res)
  managedTargets.value = Array.isArray(data.results) ? data.results : []
  managedPagination.total = Number(data.count || 0)
}

async function loadAllData() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [summaryPayload, overviewPayload, targetsPayload, alertsPayload] = await Promise.all([
      loadMonitorSummary(),
      loadPromOverview(),
      loadPromTargets(),
      loadPromAlerts(),
    ])

    overview.total = overviewPayload.total
    overview.up = overviewPayload.up
    overview.down = overviewPayload.down
    prometheusBaseUrl.value = overviewPayload.baseUrl

    promTargets.value = targetsPayload
    alerts.value = alertsPayload.rows
    alertSummary.firing = alertsPayload.firingCount
    alertSummary.resolved = alertsPayload.resolvedCount

    await loadManagedTargets()
    if (overview.total <= 0 && summaryPayload.total > 0) {
      overview.total = summaryPayload.total
      overview.up = summaryPayload.scrapeUp
      overview.down = Math.max(0, summaryPayload.total - summaryPayload.scrapeUp)
    }
    lastRefreshAt.value = new Date()
  } catch (error) {
    const msg = error?.message || '加载监控数据失败'
    errorMessage.value = msg
    message.warning(msg)
  } finally {
    loading.value = false
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
  if (!autoRefreshEnabled.value) {
    return
  }
  const intervalMs = Number(refreshIntervalSeconds.value || 15) * 1000
  refreshTimer = window.setInterval(() => {
    if (loading.value) return
    loadAllData()
  }, intervalMs)
}

function handleManagedTableChange(pagination) {
  managedPagination.current = Number(pagination?.current || 1)
  managedPagination.pageSize = Number(pagination?.pageSize || 10)
  loadManagedTargets()
}

watch(() => autoRefreshEnabled.value, restartRefreshTimer)
watch(() => refreshIntervalSeconds.value, restartRefreshTimer)

onMounted(async () => {
  await loadAllData()
  restartRefreshTimer()
})

onBeforeUnmount(() => {
  clearRefreshTimer()
})
</script>

<style scoped>
.monitor-page {
  padding: 0;
}

.tools {
  margin-bottom: 12px;
}

.right-actions {
  text-align: right;
}

.prom-url {
  color: #1f1f1f;
}

.monitor-card {
  border-radius: 12px;
}

.overview-grid {
  margin-bottom: 12px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
}

.overview-card {
  border-radius: 12px;
}
</style>
