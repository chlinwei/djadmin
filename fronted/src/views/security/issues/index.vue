<template>
  <a-card title="安全问题" :bordered="false">
    <a-alert v-if="!canView" type="error" show-icon message="没有安全问题查看权限" />
    <template v-else>
      <a-alert type="info" show-icon message="仅汇总启用模块后的新基线扫描；没有问题记录不代表主机已检查或安全。"
        description="同一主机、同一策略 ID 只保留一条问题，修改策略不重复建单；有效复查通过后关闭，规则变更后通过单独标注，执行异常不会关闭。检查时间为扫描发起时间。" />
      <a-space wrap class="toolbar">
        <a-radio-group v-model:value="view" @change="changeView">
          <a-radio-button value="groups">按风险聚合</a-radio-button>
          <a-radio-button value="hosts">按主机查看</a-radio-button>
        </a-radio-group>
        <a-input-search v-model:value="search" placeholder="问题、主机名或 IP" allow-clear @search="searchIssues" />
        <a-select v-model:value="status" :getPopupContainer="getPopupContainer" :options="statusOptions" @change="searchIssues" />
        <a-select v-model:value="severity" :getPopupContainer="getPopupContainer" :options="severityOptions" @change="searchIssues" />
        <a-select v-model:value="environment" :getPopupContainer="getPopupContainer" :options="environmentOptions"
          placeholder="全部环境" allow-clear show-search option-filter-prop="label" @change="searchIssues" />
        <a-button :loading="loading" @click="load">
          <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" />&nbsp;刷新
        </a-button>
      </a-space>
      <a-alert v-if="ruleKey" class="notice" type="info" show-icon closable message="当前仅展示所选风险的主机问题" @close="clearRule" />
      <a-alert v-if="error" class="notice" type="error" show-icon :message="error" description="查询失败，未展示旧页数据。请点击刷新重试。" />
      <a-table :row-key="view === 'groups' ? 'rule_key' : 'id'" :loading="loading" :columns="columns"
        :data-source="rows" :pagination="pagination" :scroll="{ x: view === 'groups' ? 1200 : 1900 }"
        :locale="tableLocale" @change="changePage">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'title'">
            <a v-if="view === 'groups'" @click="showHosts(record)">{{ record.title }}</a>
            <span v-else>{{ record.title }}</span>
          </template>
          <template v-else-if="column.key === 'severity'">
            <a-tag :color="record.severity === 'high' ? 'red' : record.severity === 'medium' ? 'orange' : 'default'">
              {{ severityLabels[record.severity] || record.severity }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="record.status === 'open' ? 'red' : 'green'">{{ record.status === 'open' ? '未解决' : '已关闭' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'closure_reason'">{{ record.closure_reason === 'rule_changed' ? '规则变更后通过' : record.closure_reason === 'verified' ? '复查通过' : '-' }}</template>
          <template v-else-if="column.key === 'environment'">{{ environmentLabel(record.environment_id, record.environment_name) }}</template>
          <template v-else-if="column.key === 'last_result'">
            <a-tag :color="record.last_result === 'error' ? 'orange' : record.last_result === 'pass' ? 'green' : 'red'">
              {{ resultLabels[record.last_result] || record.last_result }}
            </a-tag>
          </template>
          <template v-else-if="['first_seen', 'last_seen', 'last_checked'].includes(column.key)">{{ formatTime(record[column.key]) }}</template>
          <template v-else-if="column.key === 'action'">
            <a-tooltip title="历史记录" placement="top">
              <a-button type="link" size="small" @click="openIssue(record)">历史记录</a-button>
            </a-tooltip>
          </template>
        </template>
      </a-table>
      <a-typography-paragraph type="secondary">
        聚合数量受当前筛选条件影响，同一主机可涉及多条风险；“无法验证”表示最近一次检查异常，不表示风险已消除。主机名称和 IP 为检查时快照，环境为主机当前所属环境，修改后刷新生效。
      </a-typography-paragraph>
    </template>
  </a-card>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { listSecurityIssueGroups, listSecurityIssues } from '@/api/securityIssues'
import { checkPermission } from '@/directives/permission/permission'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const router = useRouter()
const canView = computed(() => checkPermission('security:issues:view'))
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formatTime = (value) => value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
const severityLabels = { high: '高', medium: '中', low: '低' }
const resultLabels = { pass: '通过', fail: '不符合', error: '无法验证' }
const statusOptions = [{ value: 'open', label: '未解决' }, { value: 'closed', label: '已关闭' }, { value: '', label: '全部状态' }]
const severityOptions = [{ value: '', label: '全部级别' }, ...Object.entries(severityLabels).map(([value, label]) => ({ value, label }))]
const view = ref('groups')
const search = ref('')
const status = ref('open')
const severity = ref('')
const environment = ref(undefined)
const environmentOptions = ref([])
const environmentLabel = (id, name) => id == null ? '未设置环境' : name || `环境 #${id}（已删除或不可用）`
const ruleKey = ref('')
const rows = ref([])
const loading = ref(false)
const error = ref('')
const pagination = reactive(createPagination())
let requestVersion = 0
const commonColumns = [
  { title: '问题', key: 'title', width: 260 },
  { title: '风险级别', key: 'severity', width: 100 },
]
const columns = computed(() => view.value === 'groups' ? [
  ...commonColumns,
  { title: '涉及主机数', dataIndex: 'host_count', width: 120 },
  { title: '未解决', dataIndex: 'open_count', width: 100 },
  { title: '已关闭', dataIndex: 'closed_count', width: 100 },
  { title: '无法验证', dataIndex: 'unverified_count', width: 110 },
  { title: '首次发现', key: 'first_seen', width: 180 },
  { title: '最近检查', key: 'last_checked', width: 180 },
] : [
  ...commonColumns,
  { title: '主机', dataIndex: 'host_name', width: 180 },
  { title: 'IP', dataIndex: 'host_ip', width: 150 },
  { title: '环境', key: 'environment', width: 140 },
  { title: '状态', key: 'status', width: 100 },
  { title: '关闭依据', key: 'closure_reason', width: 160 },
  { title: '最近检查结果', key: 'last_result', width: 130 },
  { title: '首次发现', key: 'first_seen', width: 180 },
  { title: '最近发现', key: 'last_seen', width: 180 },
  { title: '最近检查', key: 'last_checked', width: 180 },
  { title: '操作', key: 'action', fixed: 'right', width: 110 },
])

async function load() {
  if (!canView.value) return
  const version = ++requestVersion
  loading.value = true
  error.value = ''
  rows.value = []
  pagination.total = 0
  try {
    const fetch = view.value === 'groups' ? listSecurityIssueGroups : listSecurityIssues
    const data = await fetch({
      page: pagination.current, page_size: pagination.pageSize, search: search.value,
      status: status.value, severity: severity.value, rule_key: ruleKey.value,
      environment_id: environment.value,
    })
    // 筛选/翻页后的迟到响应不能覆盖当前视图。
    if (version !== requestVersion) return
    if (!Array.isArray(data.environments)) throw new Error('安全问题环境筛选数据无效')
    environmentOptions.value = data.environments.map((item) => ({
      value: item.id == null ? 'unassigned' : item.id,
      label: environmentLabel(item.id, item.name),
    }))
    rows.value = data.results
    pagination.total = data.count
  } catch (failure) {
    if (version === requestVersion) error.value = failure.message || '安全问题加载失败'
  } finally {
    if (version === requestVersion) loading.value = false
  }
}
function searchIssues() {
  pagination.current = 1
  return load()
}
function changeView() {
  ruleKey.value = ''
  return searchIssues()
}
function showHosts(record) {
  view.value = 'hosts'
  ruleKey.value = record.rule_key
  return searchIssues()
}
function clearRule() {
  ruleKey.value = ''
  return searchIssues()
}
function changePage(page) {
  pagination.current = page.pageSize === pagination.pageSize ? page.current : 1
  pagination.pageSize = page.pageSize
  return load()
}
function openIssue(record) {
  router.push(`/sys/security/issues/${record.id}`)
}
watch(canView, (allowed) => {
  if (allowed) load()
  else {
    requestVersion++
    rows.value = []
    environmentOptions.value = []
    pagination.total = 0
    loading.value = false
  }
})
onMounted(load)
onBeforeUnmount(() => { requestVersion++ })
</script>

<style scoped>
.toolbar { margin: 16px 0; }
.toolbar :deep(.ant-input-search) { width: 260px; }
.toolbar :deep(.ant-select) { min-width: 130px; }
.notice { margin-bottom: 16px; }
</style>
