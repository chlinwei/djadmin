<template>
  <div class="alert-rules-page">
    <a-card title="告警规则" size="small">
      <a-space style="margin-bottom: 12px" wrap>
        <a-tooltip title="刷新">
          <a-button type="primary" ghost :loading="loading" @click="loadRules">刷新</a-button>
        </a-tooltip>
        <a-select
          v-model:value="typeFilter"
          :options="typeFilterOptions"
          :getPopupContainer="getPopupContainer"
          style="width: 140px"
        />
        <a-select
          v-model:value="groupFilter"
          :options="groupFilterOptions"
          :getPopupContainer="getPopupContainer"
          show-search
          option-filter-prop="label"
          style="width: 200px"
        />
        <a-select
          v-model:value="severityFilter"
          :options="severityFilterOptions"
          :getPopupContainer="getPopupContainer"
          style="width: 140px"
        />
        <a-select
          v-model:value="stateFilter"
          :options="stateFilterOptions"
          :getPopupContainer="getPopupContainer"
          style="width: 140px"
        />
        <a-input-search
          v-model:value="keyword"
          placeholder="按名称/表达式搜索"
          allow-clear
          style="width: 260px"
        />
      </a-space>

      <a-alert v-if="loadError" type="error" show-icon :message="loadError" style="margin-bottom: 12px" />

      <a-table
        rowKey="rowKey"
        :columns="columns"
        :data-source="filteredRows"
        :loading="loading"
        size="small"
        :scroll="{ x: 1600 }"
        :pagination="{ showSizeChanger: true, showQuickJumper: true, showTotal: (total) => `共有 ${total} 条数据` }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'type'">
            <a-tag :color="record.type === 'alerting' ? 'purple' : 'blue'">
              {{ record.type === 'alerting' ? '告警规则' : '记录规则' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'severity'">
            <a-tag v-if="record.severity" :color="severityColor(record.severity)">{{ record.severity }}</a-tag>
            <span v-else>-</span>
          </template>
          <template v-else-if="column.key === 'query'">
            <a-typography-text ellipsis style="max-width: 420px">{{ record.query }}</a-typography-text>
          </template>
          <template v-else-if="column.key === 'duration'">
            {{ formatDuration(record.duration) }}
          </template>
          <template v-else-if="column.key === 'state'">
            <a-tag v-if="record.type === 'alerting'" :color="stateColor(record.state)">{{ record.state || 'unknown' }}</a-tag>
            <span v-else>-</span>
          </template>
          <template v-else-if="column.key === 'health'">
            <a-tag :color="record.health === 'ok' ? 'green' : (record.health ? 'red' : 'default')">{{ record.health || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'last_evaluation'">
            {{ formatLastEvaluation(record.last_evaluation) }}
          </template>
        </template>

        <template #expandedRowRender="{ record }">
          <div class="rule-detail">
            <p><strong>PromQL：</strong>{{ record.query }}</p>
            <p v-if="record.labels && Object.keys(record.labels).length"><strong>标签：</strong>{{ formatKeyValues(record.labels) }}</p>
            <p v-if="record.annotations && record.annotations.summary"><strong>summary：</strong>{{ record.annotations.summary }}</p>
            <p v-if="record.annotations && record.annotations.description"><strong>description：</strong>{{ record.annotations.description }}</p>
            <p v-if="record.last_error"><strong>lastError：</strong>{{ record.last_error }}</p>
          </div>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { getPrometheusAlertRules } from '@/api/monitor'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const loading = ref(false)
const loadError = ref('')
const groups = ref([])
const keyword = ref('')
const typeFilter = ref('all')
const groupFilter = ref('all')
const severityFilter = ref('all')
const stateFilter = ref('all')

const typeFilterOptions = [
  { label: '全部类型', value: 'all' },
  { label: '仅告警规则', value: 'alerting' },
  { label: '仅记录规则', value: 'recording' },
]

// 规则组下拉选项：由当前数据里出现过的规则组名称去重生成，随数据刷新自动更新
const groupFilterOptions = computed(() => {
  const names = Array.from(new Set(groups.value.map((group) => group.name || '').filter((name) => name !== '')))
  return [{ label: '全部分组', value: 'all' }, ...names.map((name) => ({ label: name, value: name }))]
})

// 级别下拉选项：来自告警规则 labels.severity（记录规则没有级别，不参与生成）
const severityFilterOptions = computed(() => {
  const values = Array.from(new Set(flatRows.value.map((row) => row.severity).filter((v) => v)))
  return [{ label: '全部级别', value: 'all' }, ...values.map((v) => ({ label: v, value: v }))]
})

// 状态下拉选项：来自告警规则的 state（inactive/pending/firing），记录规则没有状态
const stateFilterOptions = computed(() => {
  const values = Array.from(new Set(flatRows.value.filter((row) => row.type === 'alerting').map((row) => row.state).filter((v) => v)))
  return [{ label: '全部状态', value: 'all' }, ...values.map((v) => ({ label: v, value: v }))]
})

const columns = [
  { title: '规则组', dataIndex: 'group_name', key: 'group_name', width: 160 },
  { title: '类型', key: 'type', width: 100 },
  { title: '名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '表达式', key: 'query', width: 420 },
  { title: 'for', key: 'duration', width: 90 },
  { title: '级别', key: 'severity', width: 100 },
  { title: '状态', key: 'state', width: 100 },
  { title: '健康', key: 'health', width: 90 },
  { title: '最近执行时间', dataIndex: 'last_evaluation', key: 'last_evaluation', width: 200 },
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

function formatDuration(seconds) {
  const value = Number(seconds)
  if (!Number.isFinite(value) || value < 0) return '-'
  if (value === 0) return '0s'
  if (value % 3600 === 0) return `${value / 3600}h`
  if (value % 60 === 0) return `${value / 60}m`
  return `${value}s`
}

function formatKeyValues(obj) {
  return Object.entries(obj || {}).map(([k, v]) => `${k}=${v}`).join(', ')
}

// 按用户时区显示 Prometheus 返回的 lastEvaluation（UTC RFC3339），与全站时间显示规范保持一致
function formatLastEvaluation(value) {
  if (!value) return '-'
  return formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai')
}

const flatRows = computed(() => {
  const rows = []
  groups.value.forEach((group) => {
    const groupName = group.name || ''
    ;(group.rules || []).forEach((rule) => {
      const labels = rule.labels || {}
      rows.push({
        rowKey: `${groupName}-${rule.type}-${rule.name}-${rows.length}`,
        group_name: groupName,
        type: rule.type,
        name: rule.name,
        query: rule.query,
        duration: rule.duration,
        severity: labels.severity || '',
        state: rule.state,
        health: rule.health,
        last_evaluation: rule.last_evaluation,
        labels,
        annotations: rule.annotations || {},
        last_error: rule.last_error,
      })
    })
  })
  return rows
})

const filteredRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return flatRows.value.filter((row) => {
    if (typeFilter.value !== 'all' && row.type !== typeFilter.value) return false
    if (groupFilter.value !== 'all' && row.group_name !== groupFilter.value) return false
    if (severityFilter.value !== 'all' && row.severity !== severityFilter.value) return false
    if (stateFilter.value !== 'all' && row.state !== stateFilter.value) return false
    if (kw === '') return true
    return (
      String(row.name || '').toLowerCase().includes(kw) ||
      String(row.query || '').toLowerCase().includes(kw)
    )
  })
})

async function loadRules() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await getPrometheusAlertRules()
    const data = parseApiData(res)
    if (data.status === 'error') {
      loadError.value = data.error || '查询 Prometheus 规则失败'
      groups.value = []
      return
    }
    groups.value = Array.isArray(data.groups) ? data.groups : []
  } catch (error) {
    loadError.value = error?.response?.data?.msg || error?.message || '加载告警规则失败'
    message.warning(loadError.value)
    groups.value = []
  } finally {
    loading.value = false
    // 刷新后原选中的筛选值可能已在 Prometheus 侧不再存在（规则组被删除/改名、告警不再触发等），回退到对应的“全部”避免过滤条件失效导致列表长期为空
    if (groupFilter.value !== 'all' && !groups.value.some((group) => group.name === groupFilter.value)) {
      groupFilter.value = 'all'
    }
    if (severityFilter.value !== 'all' && !severityFilterOptions.value.some((option) => option.value === severityFilter.value)) {
      severityFilter.value = 'all'
    }
    if (stateFilter.value !== 'all' && !stateFilterOptions.value.some((option) => option.value === stateFilter.value)) {
      stateFilter.value = 'all'
    }
  }
}

onMounted(() => {
  loadRules()
})
</script>

<style scoped>
.alert-rules-page {
  padding: 12px;
}

.rule-detail p {
  margin: 4px 0;
  word-break: break-all;
}
</style>
