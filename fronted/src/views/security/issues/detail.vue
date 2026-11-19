<template>
  <a-card :title="fullHistory ? '安全问题检查记录' : '安全问题详情'" :bordered="false">
    <template #extra>
      <a-space>
        <a-button @click="router.push(fullHistory ? detailPath : '/sys/security/issues')">返回</a-button>
        <a-button :loading="loading" @click="load">
          <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" />&nbsp;刷新
        </a-button>
      </a-space>
    </template>
    <a-alert v-if="!canView" type="error" show-icon message="没有安全问题查看权限" />
    <template v-else>
      <a-alert v-if="error" type="error" show-icon :message="error" description="请点击刷新重试。" />
      <a-spin :spinning="loading">
        <template v-if="issue">
          <a-descriptions v-if="!fullHistory" bordered :column="2">
            <a-descriptions-item label="问题">{{ issue.title }}</a-descriptions-item>
            <a-descriptions-item label="状态">{{ issue.status === 'open' ? '未解决' : '已关闭' }}</a-descriptions-item>
            <a-descriptions-item label="稳定策略 ID">基线策略 #{{ issue.item_id }}</a-descriptions-item>
            <a-descriptions-item label="关闭依据">{{ issue.closure_reason === 'rule_changed' ? '规则变更后通过' : issue.closure_reason === 'verified' ? '复查通过' : '-' }}</a-descriptions-item>
            <a-descriptions-item label="主机">{{ issue.host_name }}（{{ issue.host_ip }}）</a-descriptions-item>
            <a-descriptions-item label="主机环境（当前）">{{ issue.environment_id == null ? '未设置环境' : issue.environment_name || `环境 #${issue.environment_id}（已删除或不可用）` }}</a-descriptions-item>
            <a-descriptions-item label="来源">基线扫描 / {{ issue.chapter }}</a-descriptions-item>
            <a-descriptions-item label="风险级别">{{ severityLabels[issue.severity] || issue.severity }}</a-descriptions-item>
            <a-descriptions-item label="最近检查结果">{{ resultLabels[issue.last_result] || issue.last_result }}</a-descriptions-item>
            <a-descriptions-item label="首次发现">{{ formatTime(issue.first_seen) }}</a-descriptions-item>
            <a-descriptions-item label="最近发现">{{ formatTime(issue.last_seen) }}</a-descriptions-item>
            <a-descriptions-item label="最近检查">{{ formatTime(issue.last_checked) }}</a-descriptions-item>
            <a-descriptions-item label="关闭时间">{{ formatTime(issue.closed_at) }}</a-descriptions-item>
            <a-descriptions-item label="累计不符合次数（仅生效结果）">{{ issue.occurrence_count }}</a-descriptions-item>
            <a-descriptions-item label="复发次数">{{ issue.reopen_count }}</a-descriptions-item>
            <a-descriptions-item label="最近检查版本"><code>{{ issue.last_rule_version }}</code></a-descriptions-item>
            <a-descriptions-item label="最近生效不符合版本"><code>{{ issue.failure_rule_version }}</code></a-descriptions-item>
          </a-descriptions>
          <a-alert v-if="!fullHistory && issue.closure_reason === 'rule_changed'" class="notice" type="warning" show-icon
            message="该问题在规则变更后复查通过，仅表示符合此次检查标准，不能证明主机已修复。请对比历史证据确认是策略纠错还是检查标准变化。" />
          <a-alert v-if="fullHistory" class="notice" type="info" show-icon
            message="同一主机和策略 ID 跨版本归并；仅修改策略不会关闭问题，仍需有效复查通过。迟到旧结果保留证据，不覆盖较新结论。删除原报告不删除历史证据。" />
          <div class="history-heading">
            <h3>{{ fullHistory ? `${issue.title} · ${issue.host_name}` : '关键事件' }}</h3>
            <router-link v-if="!fullHistory" :to="`${detailPath}/history`">查看全部检查记录</router-link>
          </div>
          <p>{{ fullHistory ? '按扫描发起时间列出全部检查记录。' : '连续不符合只展示每轮最后一条；关闭后连续通过不重复列出。' }}点击行首可查看证据。</p>
          <a-table row-key="id" :data-source="historyWithEvidence" :columns="columns" :pagination="pagination"
            :locale="tableLocale" :scroll="{ x: 900 }" size="small" @change="changePage">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'checked_at'">{{ formatTime(record.checked_at) }}</template>
              <template v-else-if="column.key === 'result'">
                <a-tag :color="record.result === 'error' ? 'orange' : record.result === 'pass' ? 'green' : 'red'">{{ resultLabels[record.result] || record.result }}</a-tag>
              </template>
              <template v-else-if="column.key === 'transition'">
                {{ !fullHistory && record.transition === 'persistent' ? '本轮最后一次不符合' : transitionLabels[record.transition] || record.transition }}
              </template>
              <template v-else-if="column.key === 'scan_id'">
                <router-link v-if="canReadScan" :to="`/sys/security/baseline/scans/${record.scan_id}`">扫描 #{{ record.scan_id }}</router-link>
                <span v-else>扫描 #{{ record.scan_id }}</span>
              </template>
              <template v-else-if="column.key === 'summary'">{{ record.summary }}</template>
            </template>
            <template #expandedRowRender="{ record }">
              <a-alert v-if="record.result === 'error'" type="warning" show-icon
                message="检查异常，无法得出合规结论" />
              <template v-else-if="record.assertions.length">
                <h4>{{ record.result === 'fail' ? '不符合项' : '检查项' }}</h4>
                <a-table row-key="idx" :columns="evidenceColumns" :data-source="record.assertions"
                  :pagination="false" :scroll="{ x: 520 }" size="small">
                  <template #bodyCell="{ column, record: assertion }">
                    <span v-if="column.key === 'title'" class="evidence-text">{{ assertion.title || '—' }}</span>
                    <pre v-else>{{ formatEvidence(assertion[column.key]) }}</pre>
                  </template>
                </a-table>
              </template>
              <p v-else>未记录结构化检查项，请查看下方检查消息和原始证据。</p>
              <a-descriptions class="evidence-meta" bordered :column="1" size="small">
                <a-descriptions-item label="修复建议"><span class="evidence-text">{{ record.remediation || '未填写' }}</span></a-descriptions-item>
                <a-descriptions-item label="检查消息"><span class="evidence-text">{{ record.message || '—' }}</span></a-descriptions-item>
                <a-descriptions-item label="规则版本"><code>{{ record.rule_version }}</code></a-descriptions-item>
              </a-descriptions>
              <a-collapse class="evidence-meta" ghost>
                <a-collapse-panel key="raw" header="原始证据">
                  <a-descriptions bordered :column="1" size="small">
                    <a-descriptions-item label="实际证据"><pre>{{ formatEvidence(record.actual_value) }}</pre></a-descriptions-item>
                    <a-descriptions-item label="预期证据"><pre>{{ formatEvidence(record.expected_value) }}</pre></a-descriptions-item>
                  </a-descriptions>
                </a-collapse-panel>
              </a-collapse>
            </template>
          </a-table>
        </template>
      </a-spin>
    </template>
  </a-card>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { getSecurityIssue, listSecurityIssueHistory } from '@/api/securityIssues'
import { checkPermission } from '@/directives/permission/permission'
import { baselineAssertions, formatEvidence } from '@/util/baselineEvidence'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const router = useRouter()
const props = defineProps({ fullHistory: Boolean })
// 工作台按完整地址缓存实例，固定本实例身份，避免其他标签的路由参数触发读取。
const issueId = useRoute().params.id
const detailPath = `/sys/security/issues/${issueId}`
const canView = computed(() => checkPermission('security:issues:view'))
const canReadScan = computed(() => checkPermission('baseline:view'))
const formatTime = (value) => value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
const severityLabels = { high: '高', medium: '中', low: '低' }
const resultLabels = { pass: '通过', fail: '不符合', error: '无法验证' }
const transitionLabels = { discovered: '首次发现', persistent: '持续不符合', reopened: '复发重开', closed: '验证关闭', closed_rule_changed: '规则变更后通过', verified: '复查通过', check_error: '检查异常', stale: '迟到结果（未覆盖当前状态）' }
const issue = ref(null)
const history = ref([])
const error = ref('')
const loading = ref(false)
const pagination = reactive(createPagination())
const columns = [
  { title: '扫描发起时间', key: 'checked_at', width: 180 },
  { title: '扫描', key: 'scan_id', width: 110 },
  { title: '检查结果', key: 'result', width: 110 },
  { title: '状态变化', key: 'transition', width: 220 },
  { title: '证据摘要', key: 'summary' },
]
const evidenceColumns = [
  { title: '检查项', key: 'title' },
  { title: '实际值', key: 'actual', width: 150 },
  { title: '期望值', key: 'expected', width: 150 },
]
const historyWithEvidence = computed(() => history.value.map((record) => {
  // 检查异常可能携带部分断言，但不能用于合规计数。
  const assertions = record.result === 'error' ? [] : baselineAssertions(record.actual_value)
    .filter((assertion) => record.result === 'fail' ? assertion.successful === false : assertion.successful === true)
  let summary = '展开查看证据'
  if (record.result === 'error') summary = '无法验证，展开查看原因'
  else if (assertions.length) {
    summary = `${assertions.length} 项${record.result === 'fail' ? '不符合' : '通过'}`
  }
  return { ...record, assertions, summary }
}))
let requestVersion = 0
async function load() {
  if (!canView.value) return
  const version = ++requestVersion
  loading.value = true
  error.value = ''
  issue.value = null
  history.value = []
  pagination.total = 0
  try {
    const [detail, observations] = await Promise.all([
      getSecurityIssue(issueId),
      listSecurityIssueHistory(issueId, {
        page: pagination.current, page_size: pagination.pageSize,
        scope: props.fullHistory ? 'all' : 'milestones',
      }),
    ])
    if (version !== requestVersion) return
    issue.value = detail
    history.value = observations.results
    pagination.total = observations.count
  } catch (failure) {
    if (version === requestVersion) error.value = failure.message || '安全问题详情加载失败'
  } finally {
    if (version === requestVersion) loading.value = false
  }
}
function changePage(page) {
  pagination.current = page.pageSize === pagination.pageSize ? page.current : 1
  pagination.pageSize = page.pageSize
  return load()
}
watch(canView, (allowed) => {
  if (allowed) load()
  else {
    requestVersion++
    issue.value = null
    history.value = []
    pagination.total = 0
    loading.value = false
  }
})
onMounted(load)
onBeforeUnmount(() => { requestVersion++ })
</script>

<style scoped>
.notice { margin: 16px 0; }
.history-heading { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 8px; margin-top: 16px; }
.evidence-meta { margin-top: 12px; }
.evidence-text { white-space: pre-wrap; overflow-wrap: anywhere; }
pre { margin: 0; max-height: 360px; max-width: 100%; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; }
code { overflow-wrap: anywhere; }
</style>
