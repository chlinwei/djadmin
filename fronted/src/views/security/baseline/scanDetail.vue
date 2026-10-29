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
        <a-space>
          <a-popconfirm
            v-if="['pending', 'running'].includes(scan?.status)"
            title="确定取消该扫描吗？"
            @confirm="handleCancel"
          >
            <a-button danger :loading="canceling">取消扫描</a-button>
          </a-popconfirm>
          <a-button type="primary" ghost :loading="loading" @click="loadDetail">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-space>
      </template>

      <a-spin :spinning="loading">
        <a-alert v-if="!loading && !scan" type="warning" show-icon message="未找到扫描记录" description="请返回列表重试，或确认该扫描是否已被删除。" />

        <template v-else-if="scan">
          <a-descriptions bordered :column="3" size="small" class="scan-summary">
            <a-descriptions-item label="基线">{{ scan.baseline }}</a-descriptions-item>
            <a-descriptions-item label="扫描方式">{{ scan.mount_type === 'host' ? '按主机' : '按项目' }}</a-descriptions-item>
            <a-descriptions-item label="项目">{{ scan.mount_type === 'host' ? '不限定项目' : scan.project_name || (scan.project_id ? `项目 #${scan.project_id}（已删除或不可用）` : '-') }}</a-descriptions-item>
            <a-descriptions-item label="环境">{{ scan.mount_type === 'host' ? '不限定环境' : environmentSummary }}</a-descriptions-item>
            <a-descriptions-item label="发起人">{{ scan.requested_username || '-' }}</a-descriptions-item>
            <a-descriptions-item label="开始时间">{{ scan.start_time || '-' }}</a-descriptions-item>
            <a-descriptions-item label="结束时间">{{ scan.end_time || '-' }}</a-descriptions-item>
            <a-descriptions-item label="主机汇总">{{ summaryText }}</a-descriptions-item>
            <a-descriptions-item label="状态"><a-tag :color="statusColor(scan.status)">{{ statusLabel(scan.status) }}</a-tag></a-descriptions-item>
          </a-descriptions>

          <a-alert
            v-if="scan.environment_source === 'current_assets'"
            type="info" show-icon
            message="此记录未保存范围快照，项目名称和环境归属按当前资产关联展示，非扫描时快照。"
          />
          <h4 class="section-title">主机符合率</h4>
          <div class="toolbar">
            <a-select
              v-model:value="environmentFilter"
              class="filter-item" placeholder="全部环境" allow-clear
              show-search option-filter-prop="label"
              :options="environmentOptions"
              :getPopupContainer="getPopupContainer"
            />
          </div>
          <a-table row-key="host_id" :columns="targetColumns" :data-source="filteredTargets" :pagination="targetPagination" :scroll="{ x: 1100 }" size="small" :locale="tableLocale" @change="handleTargetTableChange">
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
                :getPopupContainer="getPopupContainer"
                class="filter-item"
                show-search
                option-filter-prop="label"
                placeholder="全部主机"
                allow-clear
                :options="hostOptions"
              />
              <a-select
                v-model:value="chapterFilter"
                :getPopupContainer="getPopupContainer"
                class="filter-item"
                show-search
                option-filter-prop="label"
                placeholder="全部章节"
                allow-clear
                :options="chapterOptions"
              />
              <a-select
                v-model:value="severityFilter"
                :getPopupContainer="getPopupContainer"
                class="filter-item"
                placeholder="全部级别"
                allow-clear
                :options="[{ label: '高', value: 'high' }, { label: '中', value: 'medium' }, { label: '低', value: 'low' }]"
              />
              <a-radio-group v-model:value="statusFilter" button-style="solid">
                <a-radio-button value="all">全部（{{ environmentItems.length }}）</a-radio-button>
                <a-radio-button value="fail">不符合（{{ failedCount }}）</a-radio-button>
                <a-radio-button value="pass">通过（{{ environmentItems.length - failedCount }}）</a-radio-button>
              </a-radio-group>
            </a-space>
          </div>
          <a-table row-key="idx" :columns="visibleItemColumns" :data-source="filteredItems" :pagination="itemPagination" :scroll="{ x: statusFilter === 'pass' ? 1400 : 1800 }" size="small" :locale="tableLocale" @change="handleItemTableChange">
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
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import requestUtil from '@/util/request'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const route = useRoute()
const router = useRouter()
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const loading = ref(false)
const scan = ref(null)
const targets = ref([])
// 必须是 reactive：分页对象作为受控参数传给 a-table，翻页只在 @change 里改 current 才生效。
const targetPagination = reactive(createPagination())
const itemPagination = reactive(createPagination())
const items = ref([])
const statusFilter = ref('all')
const itemSearchText = ref('')
const hostFilter = ref(undefined)
const chapterFilter = ref(undefined)
const severityFilter = ref(undefined)
const environmentFilter = ref(undefined)
const environmentLabel = (environment) => environment.name || `环境 #${environment.id}（已删除或不可用）`
const environmentText = (row) => row.environments?.length ? row.environments.map(environmentLabel).join('、') : '未记录环境'
const environmentColumn = { title: '环境', key: 'environments', width: 160, customRender: ({ record }) => environmentText(record) }

const targetColumns = [
  { title: '主机', dataIndex: 'host_name', key: 'host_name' },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 140 },
  environmentColumn,
  { title: '状态', key: 'status', width: 100 },
  { title: '通过 / 不符合', key: 'items', width: 130, customRender: ({ record }) => `${record.passed_items} / ${record.failed_items}` },
  { title: '符合率', key: 'compliance_rate', width: 180 },
  { title: '备注', dataIndex: 'error_message', key: 'error_message' },
]
const itemColumns = [
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 110 },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 120 },
  environmentColumn,
  { title: '章节', dataIndex: 'chapter', key: 'chapter', width: 100 },
  { title: '条目', dataIndex: 'item_name', key: 'item_name', width: 180 },
  { title: '级别', key: 'severity', width: 70 },
  { title: '结果', key: 'status', width: 80 },
  { title: '预期', key: 'expected', width: 200 },
  { title: '实际', key: 'actual', width: 200 },
  { title: '消息', dataIndex: 'message', key: 'message', ellipsis: true },
  { title: '修复建议', key: 'remediation', width: 240 },
]
const visibleItemColumns = computed(() => statusFilter.value === 'pass'
  ? itemColumns.filter((column) => column.key !== 'message' && column.key !== 'remediation')
  : itemColumns)

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

const statusLabel = (status) => ({ pending: '等待中', running: '扫描中', success: '完成', failed: '存在不符合', skipped: '已跳过', canceled: '已取消' }[status] || status)
const statusColor = (status) => ({ pending: 'default', running: 'processing', success: 'green', failed: 'red', skipped: 'default', canceled: 'orange' }[status] || 'default')

const canceling = ref(false)
const handleCancel = async () => {
  canceling.value = true
  try {
    await requestUtil.post(`sys/security/scans/${Number(route.params.id)}/cancel/`)
    message.success('扫描已取消')
    await loadDetail()
  } finally {
    canceling.value = false
  }
}
const severityLabel = (severity) => ({ high: '高', medium: '中', low: '低' }[severity] || severity)

const summaryText = computed(() => {
  const summary = scan.value?.summary
  if (!summary?.total) return '-'
  return `${summary.total} 台：${summary.success} 成功 / ${summary.failed} 失败 / ${summary.skipped ?? 0} 跳过`
})
const environmentSummary = computed(() => {
  const names = (scan.value?.environments || []).map(environmentLabel)
  if (names.length) return names.join('、')
  if (scan.value?.environment_id) return environmentLabel({ id: scan.value.environment_id, name: scan.value.environment_name })
  return '未记录环境'
})
const environmentOptions = computed(() => {
  const options = (scan.value?.environments || []).map((environment) => ({ label: environmentLabel(environment), value: environment.id }))
  if (targets.value.some((target) => !target.environments?.length)) options.push({ label: '未记录环境', value: 'unknown' })
  return options
})
// 主机可同时属于多个环境；按环境筛选不复制主机或结果，只检查其归属集合。
const matchesEnvironment = (row) => {
  if (environmentFilter.value == null) return true
  if (environmentFilter.value === 'unknown') return !row.environments?.length
  return row.environments?.some((environment) => environment.id === environmentFilter.value) === true
}
const filteredTargets = computed(() => targets.value.filter(matchesEnvironment))
const environmentItems = computed(() => items.value.filter(matchesEnvironment))
const failedCount = computed(() => environmentItems.value.filter((item) => item.status !== 'pass').length)
const hostOptions = computed(() => {
  const seen = new Map()
  filteredTargets.value.forEach((target) => seen.set(target.host_id, { label: `${target.host_name} / ${target.host_ip}`, value: target.host_id }))
  return [...seen.values()]
})
const chapterOptions = computed(() => {
  const seen = new Set(items.value.map((item) => item.chapter).filter(Boolean))
  return [...seen].map((chapter) => ({ label: chapter, value: chapter }))
})
const filteredItems = computed(() => {
  const keyword = itemSearchText.value.trim().toLowerCase()
  return environmentItems.value.filter((item) => {
    if (statusFilter.value !== 'all' && item.status !== statusFilter.value) return false
    if (hostFilter.value && item.host_id !== hostFilter.value) return false
    if (chapterFilter.value && item.chapter !== chapterFilter.value) return false
    if (severityFilter.value && item.severity !== severityFilter.value) return false
    if (keyword && !String(item.item_name || '').toLowerCase().includes(keyword)) return false
    return true
  })
})

// 受控分页：a-table 传入 current/total 后，翻页必须在这里回写，否则点第 2 页无反应。
const handleTargetTableChange = (pagination) => {
  targetPagination.current = pagination.current
  targetPagination.pageSize = pagination.pageSize
}
const handleItemTableChange = (pagination) => {
  itemPagination.current = pagination.current
  itemPagination.pageSize = pagination.pageSize
}

watch(filteredItems, (rows) => {
  itemPagination.total = rows.length
  itemPagination.current = 1
}, { immediate: true })
watch(filteredTargets, (rows) => {
  targetPagination.total = rows.length
  targetPagination.current = 1
  if (hostFilter.value != null && !rows.some((row) => row.host_id === hostFilter.value)) hostFilter.value = undefined
}, { immediate: true })
watch(environmentOptions, (options) => {
  if (environmentFilter.value != null && !options.some((option) => option.value === environmentFilter.value)) environmentFilter.value = undefined
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
