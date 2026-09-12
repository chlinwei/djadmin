<template>
  <div class="scan-detail-page">
    <a-card :bordered="false" class="detail-shell">
      <template #title>
        <a-space>
          <a-button @click="goBack">
            <FontAwesomeIcon :icon="['fas', 'arrow-left']" />
            <span>&nbsp;返回</span>
          </a-button>
          <span class="detail-title">扫描详情</span>
          <a-tag v-if="scan?.id" color="blue">ID: {{ scan.id }}</a-tag>
          <a-tag v-if="scan?.status" :color="statusColor(scan.status)">{{ statusLabel(scan.status) }}</a-tag>
        </a-space>
      </template>
      <template #extra>
        <a-button type="primary" ghost :loading="loading" @click="loadDetail">
          <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" />
          <span>&nbsp;刷新</span>
        </a-button>
      </template>

      <a-spin :spinning="loading">
        <a-alert v-if="!loading && !scan" type="warning" show-icon message="未找到扫描记录" description="请返回列表重试，或确认该扫描是否已被删除。" />

        <template v-else-if="scan">
          <a-descriptions bordered :column="3" size="small" class="scan-summary">
            <a-descriptions-item label="基线">{{ scan.baseline }}</a-descriptions-item>
            <a-descriptions-item label="发起人">{{ scan.requested_username || '-' }}</a-descriptions-item>
            <a-descriptions-item label="开始时间">{{ scan.start_time || '-' }}</a-descriptions-item>
            <a-descriptions-item label="结束时间">{{ scan.end_time || '-' }}</a-descriptions-item>
            <a-descriptions-item label="主机汇总">{{ summaryText }}</a-descriptions-item>
            <a-descriptions-item label="状态"><a-tag :color="statusColor(scan.status)">{{ statusLabel(scan.status) }}</a-tag></a-descriptions-item>
          </a-descriptions>

          <h4 class="section-title">主机符合率</h4>
          <a-table row-key="host_ip" :columns="targetColumns" :data-source="targets" :pagination="false" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'status'">
                <a-tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</a-tag>
              </template>
              <template v-else-if="column.key === 'compliance_rate'">
                <a-progress :percent="Number(record.compliance_rate)" size="small" :status="Number(record.compliance_rate) === 100 ? 'success' : 'exception'" />
              </template>
            </template>
          </a-table>

          <h4 class="section-title">条目明细</h4>
          <div class="toolbar">
            <a-space wrap>
              <a-input-search
                v-model:value="itemSearchText"
                class="filter-item"
                placeholder="搜索条目名称"
                allow-clear
              />
              <a-select
                v-model:value="hostFilter"
                class="filter-item"
                show-search
                option-filter-prop="label"
                placeholder="全部主机"
                allow-clear
                :options="hostOptions"
              />
              <a-select
                v-model:value="chapterFilter"
                class="filter-item"
                show-search
                option-filter-prop="label"
                placeholder="全部章节"
                allow-clear
                :options="chapterOptions"
              />
              <a-select
                v-model:value="severityFilter"
                class="filter-item"
                placeholder="全部级别"
                allow-clear
                :options="[{ label: '高', value: 'high' }, { label: '中', value: 'medium' }, { label: '低', value: 'low' }]"
              />
              <a-radio-group v-model:value="statusFilter" button-style="solid">
                <a-radio-button value="all">全部（{{ items.length }}）</a-radio-button>
                <a-radio-button value="fail">不符合（{{ failedCount }}）</a-radio-button>
                <a-radio-button value="pass">通过（{{ items.length - failedCount }}）</a-radio-button>
              </a-radio-group>
            </a-space>
          </div>
          <a-table row-key="idx" :columns="itemColumns" :data-source="filteredItems" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'severity'">
                <a-tag :color="record.severity === 'high' ? 'red' : record.severity === 'medium' ? 'orange' : 'default'">{{ severityLabel(record.severity) }}</a-tag>
              </template>
              <template v-else-if="column.key === 'status'">
                <a-tag :color="record.status === 'pass' ? 'green' : 'red'">{{ record.status === 'pass' ? '通过' : '不符合' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'expected'">
                <template v-if="assertionsOf(record).length">
                  <div v-for="(assertion, i) in assertionsOf(record)" :key="i" class="assertion-line">
                    <span class="assertion-name">{{ assertion.title }}</span>
                    <span class="assertion-value">应为 {{ assertion.expected }}</span>
                  </div>
                </template>
                <a-tooltip v-else placement="topLeft" :title="formatExpected(record.expected)">
                  <div class="cell-pre">{{ formatExpected(record.expected) }}</div>
                </a-tooltip>
              </template>
              <template v-else-if="column.key === 'actual'">
                <template v-if="assertionsOf(record).length">
                  <div v-for="(assertion, i) in assertionsOf(record)" :key="i" class="assertion-line">
                    <span class="assertion-name">{{ assertion.title }}</span>
                    <span class="assertion-value" :class="assertion.successful ? 'is-pass' : 'is-fail'">
                      {{ assertion.successful ? '✓' : '✗' }} 实际 {{ assertion.actual }}
                    </span>
                  </div>
                </template>
                <a-popover v-else placement="topLeft" trigger="click">
                  <template #content><pre class="actual-json">{{ JSON.stringify(record.actual, null, 2) }}</pre></template>
                  <div class="cell-pre">{{ formatActual(record.actual) }}</div>
                </a-popover>
              </template>
              <template v-else-if="column.key === 'remediation'">
                <span v-if="record.status === 'pass'" class="remediation-none">—</span>
                <div v-else-if="record.remediation" class="cell-pre">{{ record.remediation }}</div>
                <span v-else class="remediation-none">未填写</span>
              </template>
            </template>
          </a-table>
        </template>
      </a-spin>
    </a-card>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import requestUtil from '@/util/request'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const scan = ref(null)
const targets = ref([])
const items = ref([])
const statusFilter = ref('all')
const itemSearchText = ref('')
const hostFilter = ref(undefined)
const chapterFilter = ref(undefined)
const severityFilter = ref(undefined)

const targetColumns = [
  { title: '主机', dataIndex: 'host_name', key: 'host_name' },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 140 },
  { title: '状态', key: 'status', width: 100 },
  { title: '通过 / 不符合', key: 'items', width: 130, customRender: ({ record }) => `${record.passed_items} / ${record.failed_items}` },
  { title: '符合率', key: 'compliance_rate', width: 180 },
  { title: '备注', dataIndex: 'error_message', key: 'error_message' },
]
const itemColumns = [
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 110 },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 120 },
  { title: '章节', dataIndex: 'chapter', key: 'chapter', width: 100 },
  { title: '条目', dataIndex: 'item_name', key: 'item_name', width: 180 },
  { title: '级别', key: 'severity', width: 70 },
  { title: '结果', key: 'status', width: 80 },
  { title: '预期', key: 'expected', width: 200 },
  { title: '实际', key: 'actual', width: 200 },
  { title: '消息', dataIndex: 'message', key: 'message', ellipsis: true },
  { title: '修复建议', key: 'remediation', width: 240 },
]

// 预期展示 Rego 策略摘要；实际展示违规数与违规项摘要，悬浮可看完整 JSON。
// 断言级结构化结果（agent OPA 检查的 actual.details）：逐条展示「应为 X / 实际 Y」。
// 历史数据或非 OPA 结果没有 details 时，回退到策略源码/JSON 摘要展示。
const assertionsOf = (record) => {
  const details = record?.actual?.details
  if (!Array.isArray(details)) return []
  return details.filter((detail) => detail && detail.property === 'assertion')
    .map((detail) => ({
      title: detail.title || detail.name || '',
      expected: detail.expected,
      actual: detail.actual,
      successful: !!detail.successful,
      message: detail.message || '',
    }))
}
const formatExpected = (value) => {
  if (!value) return '-'
  if (typeof value === 'object' && value.policy) return String(value.policy)
  return JSON.stringify(value)
}
const formatActual = (value) => {
  if (!value) return '-'
  if (typeof value === 'object') {
    const parts = []
    if (value.violation_count !== undefined) parts.push(`违规 ${value.violation_count} 项`)
    const violations = value.violations || []
    violations.slice(0, 3).forEach((violation) => parts.push(`· ${violation?.msg || violation?.item?.message || JSON.stringify(violation?.item?.actual ?? violation?.item ?? violation)}`))
    if (violations.length > 3) parts.push(`…等 ${violations.length} 项`)
    if (!parts.length) parts.push(JSON.stringify(value))
    return parts.join('\n')
  }
  return String(value)
}

const statusLabel = (status) => ({ pending: '等待中', running: '扫描中', success: '完成', failed: '存在不符合', skipped: '已跳过' }[status] || status)
const statusColor = (status) => ({ pending: 'default', running: 'processing', success: 'green', failed: 'red', skipped: 'default' }[status] || 'default')
const severityLabel = (severity) => ({ high: '高', medium: '中', low: '低' }[severity] || severity)

const summaryText = computed(() => {
  const summary = scan.value?.summary
  if (!summary?.total) return '-'
  return `${summary.total} 台：${summary.success} 成功 / ${summary.failed} 失败 / ${summary.skipped ?? 0} 跳过`
})
const failedCount = computed(() => items.value.filter((item) => item.status !== 'pass').length)
const hostOptions = computed(() => {
  const seen = new Map()
  targets.value.forEach((target) => seen.set(target.host_ip, { label: `${target.host_name} / ${target.host_ip}`, value: target.host_ip }))
  return [...seen.values()]
})
const chapterOptions = computed(() => {
  const seen = new Set(items.value.map((item) => item.chapter).filter(Boolean))
  return [...seen].map((chapter) => ({ label: chapter, value: chapter }))
})
const filteredItems = computed(() => {
  const keyword = itemSearchText.value.trim().toLowerCase()
  return items.value.filter((item) => {
    if (statusFilter.value !== 'all' && item.status !== statusFilter.value) return false
    if (hostFilter.value && item.host_ip !== hostFilter.value) return false
    if (chapterFilter.value && item.chapter !== chapterFilter.value) return false
    if (severityFilter.value && item.severity !== severityFilter.value) return false
    if (keyword && !String(item.item_name || '').toLowerCase().includes(keyword)) return false
    return true
  })
})

const loadDetail = async () => {
  const scanId = Number(route.params.id)
  if (!Number.isFinite(scanId) || scanId <= 0) return
  loading.value = true
  try {
    const response = await requestUtil.get(`sys/security/scans/${scanId}/`)
    const data = response?.data?.data || {}
    scan.value = data.scan || null
    targets.value = data.targets || []
    items.value = (data.items || []).map((item, index) => ({ ...item, idx: index }))
  } finally {
    loading.value = false
  }
}

const goBack = () => {
  if (window.history.length > 1) {
    router.back()
  } else {
    router.push('/sys/security/baseline')
  }
}

onMounted(loadDetail)

defineOptions({ name: 'ViewSecurityBaselineScanDetail' })
</script>

<style scoped>
.scan-detail-page { padding: 8px; }
.detail-title { font-size: 16px; font-weight: 600; }
.scan-summary { margin-bottom: 16px; }
.section-title { margin: 18px 0 10px; font-weight: 600; }
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.filter-item { width: 180px; }
.cell-pre {
  white-space: pre-wrap;
  word-break: break-all;
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 12px;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.actual-json { max-width: 480px; max-height: 320px; overflow: auto; font-size: 12px; margin: 0; }
.assertion-line { display: flex; align-items: baseline; gap: 8px; line-height: 1.7; }
.assertion-name { color: #66727d; }
.assertion-value { font-family: "JetBrains Mono", "Cascadia Code", monospace; font-size: 12px; word-break: break-all; }
.assertion-value.is-pass { color: #389e0d; }
.assertion-value.is-fail { color: #cf1322; font-weight: 600; }
.remediation-none { color: #bfbfbf; }
</style>
