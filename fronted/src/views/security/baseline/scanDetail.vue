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
            <a-descriptions-item label="主机环境">{{ environmentSummary }}</a-descriptions-item>
            <a-descriptions-item label="发起人">{{ scan.requested_username || '-' }}</a-descriptions-item>
            <a-descriptions-item label="开始时间">{{ scan.start_time || '-' }}</a-descriptions-item>
            <a-descriptions-item label="结束时间">{{ scan.end_time || '-' }}</a-descriptions-item>
            <a-descriptions-item label="主机汇总">{{ summaryText }}</a-descriptions-item>
            <a-descriptions-item label="状态"><a-tag :color="statusColor(scan.status)">{{ statusLabel(scan.status) }}</a-tag></a-descriptions-item>
          </a-descriptions>

          <a-alert
            v-if="scan.environment_source === 'current_assets'"
            type="info" show-icon
            message="环境按主机当前所属环境展示和筛选，非扫描时快照；主机环境变更后刷新即可更新，不改变扫描结果。"
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
              <a-select
                v-model:value="environmentFilter"
                class="filter-item" placeholder="全部环境" allow-clear
                show-search option-filter-prop="label"
                :options="environmentOptions"
                :getPopupContainer="getPopupContainer"
              />
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
                <a-radio-button value="pass">通过（{{ passedCount }}）</a-radio-button>
                <a-radio-button value="error">执行异常（{{ errorCount }}）</a-radio-button>
              </a-radio-group>
            </a-space>
          </div>
          <p class="detail-hint">展开条目可查看完整检查断言、修复建议和原始证据。</p>
          <a-table row-key="idx" table-layout="fixed" :columns="visibleItemColumns" :data-source="filteredItems" :pagination="itemPagination" :scroll="{ x: itemTableWidth }" size="small" :locale="tableLocale" @change="handleItemTableChange">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'severity'">
                <a-tag :color="record.severity === 'high' ? 'red' : record.severity === 'medium' ? 'orange' : 'default'">{{ severityLabel(record.severity) }}</a-tag>
              </template>
              <template v-else-if="column.key === 'status'">
                <a-tag :color="record.status === 'pass' ? 'green' : record.status === 'error' ? 'orange' : 'red'">{{ record.status === 'pass' ? '通过' : record.status === 'error' ? '执行异常' : '不符合' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'message'">
                <div class="cell-pre">{{ resultSummary(record) }}</div>
              </template>
              <template v-else-if="column.key === 'remediation'">
                <span v-if="record.status === 'pass'" class="remediation-none">—</span>
                <div v-else-if="record.remediation" class="cell-pre">{{ record.remediation }}</div>
                <span v-else class="remediation-none">未填写</span>
              </template>
            </template>
            <template #expandedRowRender="{ record }">
              <div class="result-detail">
                <a-descriptions bordered :column="1" size="small">
                  <a-descriptions-item label="条目">{{ record.item_name }}</a-descriptions-item>
                  <a-descriptions-item label="消息"><div class="detail-text">{{ record.message || '—' }}</div></a-descriptions-item>
                  <a-descriptions-item v-if="record.status !== 'pass'" label="修复建议">
                    <div class="detail-text">{{ record.remediation || '未填写' }}</div>
                  </a-descriptions-item>
                </a-descriptions>
                <a-alert v-if="record.status === 'error'" class="evidence-section" type="warning" show-icon
                  message="执行异常，无法得出合规结论。原始证据仅供排障，不作为检查通过的依据。" />
                <template v-else-if="assertionsOf(record).length">
                  <h4 class="evidence-section">检查断言</h4>
                  <a-table row-key="idx" table-layout="fixed" :columns="assertionColumns" :data-source="assertionsOf(record)"
                    :pagination="false" :scroll="{ x: 1000 }" size="small">
                    <template #bodyCell="{ column, record: assertion }">
                      <template v-if="column.key === 'expected' || column.key === 'actual'">
                        <pre class="evidence-value">{{ formatEvidence(assertion[column.key]) }}</pre>
                      </template>
                      <template v-else-if="column.key === 'result'">
                        <a-tag :color="assertion.successful ? 'green' : 'red'">{{ assertion.successful ? '通过' : '不符合' }}</a-tag>
                        <div v-if="assertion.message" class="detail-text">{{ assertion.message }}</div>
                      </template>
                    </template>
                  </a-table>
                </template>
                <a-collapse class="evidence-section" ghost>
                  <a-collapse-panel key="evidence" header="原始证据">
                    <div class="evidence-grid">
                      <section>
                        <h4>预期证据</h4>
                        <pre class="evidence-value">{{ formatEvidence(record.expected) }}</pre>
                      </section>
                      <section>
                        <h4>实际证据</h4>
                        <pre class="evidence-value">{{ formatEvidence(record.actual) }}</pre>
                      </section>
                    </div>
                  </a-collapse-panel>
                </a-collapse>
              </div>
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
import { baselineAssertions, formatEvidence } from '@/util/baselineEvidence'

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
const environmentText = (row) => row.environments?.length ? row.environments.map(environmentLabel).join('、') : '未设置环境'
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
  { title: '条目', dataIndex: 'item_name', key: 'item_name', width: 220, ellipsis: true },
  { title: '级别', key: 'severity', width: 70 },
  { title: '结果', key: 'status', width: 100 },
  { title: '检查摘要', key: 'message', width: 320 },
  { title: '修复建议', key: 'remediation', width: 240 },
]
const visibleItemColumns = computed(() => statusFilter.value === 'pass'
  ? itemColumns.filter((column) => column.key !== 'message' && column.key !== 'remediation')
  : itemColumns)
// 横向滚动宽度包含展开列，避免窗口变窄时挤压证据摘要。
const itemTableWidth = computed(() => visibleItemColumns.value.reduce((width, column) => width + column.width, 48))
const assertionColumns = [
  { title: '检查项', dataIndex: 'title', key: 'title', width: 260 },
  { title: '预期', key: 'expected', width: 220 },
  { title: '实际', key: 'actual', width: 380 },
  { title: '结论', key: 'result', width: 140 },
]

const assertionsOf = (record) => baselineAssertions(record?.actual)
const resultSummary = (record) => {
  // 异常的部分断言不能构成有效合规结论。
  if (record.status === 'error') return record.message || '检查异常，无法得出合规结论'
  const assertions = assertionsOf(record)
  if (assertions.length) {
    const failed = assertions.filter((assertion) => !assertion.successful)
    if (!failed.length) return `共 ${assertions.length} 项检查，全部通过`
    return `共 ${assertions.length} 项检查，${failed.length} 项不符合：${failed.slice(0, 2).map((assertion) => assertion.title).join('；')}${failed.length > 2 ? '…' : ''}`
  }
  return record.message || '展开查看检查证据'
}

const statusLabel = (status) => ({ pending: '等待中', running: '扫描中', success: '完成', failed: '检查未全部通过', skipped: '已跳过', canceled: '已取消' }[status] || status)
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
  return '未设置环境'
})
const environmentOptions = computed(() => {
  const options = (scan.value?.environments || []).map((environment) => ({ label: environmentLabel(environment), value: environment.id }))
  if (targets.value.some((target) => !target.environments?.length)) options.push({ label: '未设置环境', value: 'unknown' })
  return options
})
// 接口保留环境数组结构，每台主机最多一个当前环境；未设置环境可单独筛选。
const matchesEnvironment = (row) => {
  if (environmentFilter.value == null) return true
  if (environmentFilter.value === 'unknown') return !row.environments?.length
  return row.environments?.some((environment) => environment.id === environmentFilter.value) === true
}
const filteredTargets = computed(() => targets.value.filter(matchesEnvironment))
const environmentItems = computed(() => items.value.filter(matchesEnvironment))
const failedCount = computed(() => environmentItems.value.filter((item) => item.status === 'fail').length)
const passedCount = computed(() => environmentItems.value.filter((item) => item.status === 'pass').length)
const errorCount = computed(() => environmentItems.value.filter((item) => item.status === 'error').length)
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
.detail-hint { color: #66727d; margin-bottom: 8px; }
.cell-pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.result-detail { padding: 8px; min-width: 0; }
.detail-text { white-space: pre-wrap; overflow-wrap: anywhere; }
.evidence-section { margin-top: 16px; }
.evidence-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.evidence-grid section { min-width: 0; }
.evidence-value {
  margin: 0;
  max-height: 280px;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 12px;
}
.remediation-none { color: #bfbfbf; }
@media (max-width: 768px) {
  .evidence-grid { grid-template-columns: minmax(0, 1fr); }
}
</style>
