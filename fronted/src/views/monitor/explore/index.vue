<template>
  <div class="monitor-explore-page">
    <a-card title="Explore" size="small">
      <template #extra>
        <a-space>
          <a-tooltip title="执行查询">
            <a-button type="primary" :loading="loading" @click="runQuery">执行</a-button>
          </a-tooltip>
        </a-space>
      </template>

      <a-space direction="vertical" style="width: 100%" :size="12">
        <PromQLEditor v-model="queryText" :completion-remote-url="completionRemoteUrl" @run="runQuery" />

        <a-space wrap>
          <a-radio-group v-model:value="queryMode" button-style="solid">
            <a-radio-button value="instant">表格</a-radio-button>
            <a-radio-button value="range">图形</a-radio-button>
          </a-radio-group>

          <a-input
            v-if="queryMode === 'range'"
            v-model:value="queryStep"
            style="width: 120px"
            placeholder="step，例如 30s"
          />

          <a-range-picker
            v-if="queryMode === 'range'"
            v-model:value="queryRange"
            :show-time="rangeShowTime"
            :presets="rangePresets"
            :placeholder="['开始时间', '结束时间']"
            :getPopupContainer="getPopupContainer"
            @openChange="onRangeOpenChange"
          />
        </a-space>

        <a-alert v-if="errorMessage" type="error" show-icon :message="errorMessage" />

        <!-- 图形模式：只显示图表，全宽 -->
        <div v-if="queryMode === 'range'">
          <PrometheusChart :result-data="rangeQueryResult" :height="500" />
        </div>

        <!-- Instant 查询模式：只显示表格 -->
        <div v-else>
          <a-table
            rowKey="rowKey"
            :columns="tableColumns"
            :data-source="tableRows"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="{ showSizeChanger: true, showQuickJumper: true, showTotal: (total) => `共有 ${total} 条数据` }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'metric'">
                <a-typography-text :content="record.metric" ellipsis style="max-width: 620px" />
              </template>
              <template v-else-if="column.key === 'value'">
                {{ record.value || '-' }}
              </template>
              <template v-else-if="column.key === 'time'">
                {{ formatTime(record.time) }}
              </template>
            </template>
          </a-table>
        </div>

      </a-space>
    </a-card>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import dayjs from 'dayjs'
import store from '@/store'
import { getServerUrl } from '@/util/request'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatTimeWithTimezone } from '@/util/timezone'
import { buildUserTimezoneRangePresets, buildUserTimezoneShowTime, toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'
import { queryPrometheusInstant, queryPrometheusRange } from '@/api/monitor'
import PromQLEditor from './components/PromQLEditor.vue'
import PrometheusChart from './components/PrometheusChart.vue'

defineOptions({
  name: 'MonitorExplorePage',
})

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const userTimezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')

const queryText = ref('')
const queryMode = ref('instant')

// 切换表格/图形时自动重新执行查询
watch(queryMode, () => {
  if (queryText.value) runQuery()
})
const queryStep = ref('30s')
const queryRange = ref([dayjs().subtract(1, 'hour'), dayjs()])
const rangePresets = ref([])
const rangeShowTime = buildUserTimezoneShowTime(userTimezone.value)

const loading = ref(false)
const errorMessage = ref('')
const resultType = ref('')
const rawResult = ref([])

// codemirror-promql 会请求 <remoteUrl>/api/v1/*，这里走后端同域代理避免跨域/鉴权问题。
const completionRemoteUrl = computed(() => `${getServerUrl()}/monitor/targets/prometheus/proxy`)

// Range 查询用结果，直接传给图表组件
const rangeQueryResult = computed(() => {
  if (resultType.value === 'matrix') {
    return { result: rawResult.value }
  }
  return { result: [] }
})

function onRangeOpenChange(open) {
  if (!open) {
    return
  }
  rangePresets.value = buildUserTimezoneRangePresets(userTimezone.value)
}

function parseApiData(res) {
  const payload = res?.data || res
  if (payload && typeof payload === 'object' && payload.data !== undefined) {
    return payload.data
  }
  return payload || {}
}

function formatTime(value) {
  if (!value) return '-'
  return formatTimeWithTimezone(value, userTimezone.value)
}

function formatMetric(metric) {
  if (!metric || typeof metric !== 'object') {
    return '-'
  }
  const keys = Object.keys(metric)
  if (!keys.length) {
    return '{}'
  }
  return keys
    .sort((a, b) => a.localeCompare(b))
    .map((key) => `${key}=${metric[key]}`)
    .join(', ')
}

function buildRangeQueryParams() {
  const [startTime, endTime] = Array.isArray(queryRange.value) ? queryRange.value : []
  return {
    query: queryText.value,
    start: toUtcQueryISOStringByUserTimezone(startTime, userTimezone.value),
    end: toUtcQueryISOStringByUserTimezone(endTime, userTimezone.value),
    step: queryStep.value || '30s',
  }
}

const tableColumns = computed(() => {
  if (resultType.value === 'matrix') {
    return [
      { title: '指标', key: 'metric', width: 620 },
      { title: '点数', dataIndex: 'points', key: 'points', width: 100 },
      { title: '最新值', dataIndex: 'value', key: 'value', width: 140 },
      { title: '最新时间', dataIndex: 'time', key: 'time', width: 220 },
    ]
  }
  return [
    { title: '指标', key: 'metric', width: 620 },
    { title: '值', dataIndex: 'value', key: 'value', width: 140 },
    { title: '时间', dataIndex: 'time', key: 'time', width: 220 },
  ]
})

const tableRows = computed(() => {
  const result = Array.isArray(rawResult.value) ? rawResult.value : []
  if (resultType.value === 'matrix') {
    return result.map((item, index) => {
      const values = Array.isArray(item?.values) ? item.values : []
      const lastPair = values.length ? values[values.length - 1] : []
      const ts = Array.isArray(lastPair) ? Number(lastPair[0] || 0) * 1000 : 0
      return {
        rowKey: `matrix-${index}`,
        metric: formatMetric(item?.metric),
        points: values.length,
        value: Array.isArray(lastPair) ? String(lastPair[1] || '') : '',
        time: ts ? new Date(ts).toISOString() : '',
      }
    })
  }

  return result.map((item, index) => {
    const pair = Array.isArray(item?.value) ? item.value : []
    const ts = Array.isArray(pair) ? Number(pair[0] || 0) * 1000 : 0
    return {
      rowKey: `vector-${index}`,
      metric: formatMetric(item?.metric),
      value: Array.isArray(pair) ? String(pair[1] || '') : '',
      time: ts ? new Date(ts).toISOString() : '',
    }
  })
})


async function runQuery() {
  const expression = String(queryText.value || '').trim()
  if (!expression) {
    errorMessage.value = '请输入 PromQL 表达式'
    return
  }

  loading.value = true
  errorMessage.value = ''
  try {
    const response = queryMode.value === 'range'
      ? await queryPrometheusRange(buildRangeQueryParams())
      : await queryPrometheusInstant({ query: expression })

    const data = parseApiData(response)
    if (String(data.status || '').toLowerCase() === 'error') {
      errorMessage.value = data.error || 'Prometheus 查询失败'
      resultType.value = ''
      rawResult.value = []
      return
    }

    resultType.value = String(data.result_type || '')
    rawResult.value = Array.isArray(data.result) ? data.result : []
  } catch (error) {
    errorMessage.value = error?.response?.data?.msg || error?.message || '执行查询失败'
    resultType.value = ''
    rawResult.value = []
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.monitor-explore-page {
  padding: 8px;
}
</style>
