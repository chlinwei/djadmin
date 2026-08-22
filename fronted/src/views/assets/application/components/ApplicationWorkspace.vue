<template>
  <div class="application-workspace">
    <a-tabs v-model:activeKey="activeTab" @change="handleTabChange">
      <a-tab-pane key="applications" tab="应用定义" />
      <a-tab-pane key="templates" tab="部署模板" />
      <a-tab-pane key="deployments" tab="部署实例" />
    </a-tabs>

    <a-row :gutter="12" class="tools">
      <a-col flex="360px">
        <a-input-search v-model:value="keyword" :placeholder="searchPlaceholder" allow-clear enter-button @search="reload(true)" />
      </a-col>
      <a-col flex="auto" class="right-tools">
        <a-space>
          <a-button v-permission="'assets:applications:create'" size="large" @click="openCurrentTabCreateDialog">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;{{ createButtonLabel }}</span>
          </a-button>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="loading" @click="reload(false)">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <a-table
      :row-key="getWorkspaceRowKey"
      :columns="currentColumns"
      :data-source="rows"
      :loading="loading"
      :pagination="pagination"
      :scroll="currentTableScroll"
      @change="handleTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="activeTab === 'applications' && column.key === 'category'">
          <a-tag>{{ categoryLabels[record.category] || record.category }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'applications' && column.key === 'versions'">
          <a-space wrap>
            <a-tag v-for="version in record.versions" :key="version.id" color="blue">{{ version.version }}</a-tag>
            <span v-if="!record.versions?.length">-</span>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'applications' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'applications' && column.key === 'action'">
          <a-space>
            <a-tooltip title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openApplication(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="版本管理">
              <a-button size="small" @click="openVersions(record)">
                <FontAwesomeIcon :icon="['fas', 'code-branch']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteApplication(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'templates' && column.key === 'control_type'">
          <a-tag :color="controlTypeColors[record.control_type]">{{ controlTypeLabels[record.control_type] || record.control_type }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'templates' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'templates' && column.key === 'action'">
          <a-space>
            <a-tooltip title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openTemplate(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteTemplate(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'host'">
          <div>{{ record.host_name || '-' }}</div>
          <span class="secondary">{{ record.host_ip || '-' }}</span>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'control_type'">
          <a-tag :color="controlTypeColors[record.control_type]">{{ controlTypeLabels[record.control_type] || record.control_type }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'health_status'">
          <div><a-badge :status="healthStatusMap[record.health_status]?.badge || 'default'" :text="healthStatusMap[record.health_status]?.label || '未检查'" /></div>
          <span class="secondary">{{ formatPassRate(record.baseline_pass_rate) }}</span>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'last_check_time'">
          <span>{{ formatDateTime(record.last_check_time) }}</span>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'action'">
          <a-space>
            <a-tooltip v-if="record.control_type !== 'external_ha'" title="启动">
              <a-button
                v-permission="'assets:applications:update'"
                data-control-action="start"
                size="small"
                type="primary"
                ghost
                :loading="isControlLoading(record.id, 'start')"
                :disabled="isDeploymentBusy(record.id)"
                @click="confirmApplicationControl(record, 'start')"
              >
                <FontAwesomeIcon :icon="['fas', 'play']" />
              </a-button>
            </a-tooltip>
            <a-tooltip v-if="record.control_type !== 'external_ha'" title="停止">
              <a-button
                v-permission="'assets:applications:update'"
                data-control-action="stop"
                size="small"
                danger
                :loading="isControlLoading(record.id, 'stop')"
                :disabled="isDeploymentBusy(record.id)"
                @click="confirmApplicationControl(record, 'stop')"
              >
                <FontAwesomeIcon :icon="['fas', 'stop']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="查看状态">
              <a-button
                v-permission="'assets:applications:update'"
                data-control-action="status"
                size="small"
                :loading="isControlLoading(record.id, 'status')"
                :disabled="isDeploymentBusy(record.id)"
                @click="runApplicationControl(record, 'status')"
              >
                <FontAwesomeIcon :icon="['fas', 'circle-info']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="基线检查">
              <a-button
                v-permission="'assets:applications:update'"
                data-control-action="baseline"
                size="small"
                type="primary"
                :loading="checkingDeploymentId === record.id"
                :disabled="record.health_status === 'checking' || isDeploymentBusy(record.id)"
                @click="runBaselineCheck(record)"
              >
                <FontAwesomeIcon :icon="['fas', 'clipboard-check']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="历史记录">
              <a-button v-permission="'assets:applications:view'" size="small" @click="openBaselineHistory(record)">
                <FontAwesomeIcon :icon="['fas', 'list']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openDeployment(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteDeployment(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
      </template>
    </a-table>

    <Dialog
      :open="applicationDialogOpen"
      :item_id="selectedApplication?.id || -1"
      :title="selectedApplication ? `编辑-${selectedApplication.name}` : '新增应用'"
      appname="应用"
      @update:open="applicationDialogOpen = $event"
      @initList="reload(false)"
    />
    <VersionDialog
      :open="versionDialogOpen"
      :application="selectedApplication"
      @update:open="versionDialogOpen = $event"
      @changed="reload(false)"
    />
    <TemplateDialog
      :open="templateDialogOpen"
      :template-id="selectedTemplateId"
      @update:open="templateDialogOpen = $event"
      @saved="reload(false)"
    />
    <DeploymentDialog
      :open="deploymentDialogOpen"
      :deployment-id="selectedDeployment?.id || null"
      @update:open="deploymentDialogOpen = $event"
      @saved="reload(false)"
    />
    <a-modal
      v-model:open="historyDialogOpen"
      :title="`${historyDeployment?.instance_name || ''} - 基线检查历史`"
      width="1080px"
      :footer="null"
      destroy-on-close
    >
      <a-table
        row-key="id"
        :columns="historyColumns"
        :data-source="baselineHistory"
        :loading="historyLoading"
        :pagination="false"
        :scroll="{ x: 920 }"
        :expand-row-by-click="true"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="executionStatusMap[record.status]?.color || 'default'">
              {{ executionStatusMap[record.status]?.label || record.status }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'passed'">
            <a-tag v-if="record.passed === true" color="success">通过</a-tag>
            <a-tag v-else-if="record.passed === false" color="error">未通过</a-tag>
            <span v-else>-</span>
          </template>
          <template v-else-if="column.key === 'count'">
            {{ record.passed_count }}/{{ record.total_count }}
          </template>
          <template v-else-if="column.key === 'create_time'">
            {{ formatDateTime(record.create_time) }}
          </template>
        </template>
        <template #expandedRowRender="{ record }">
          <a-alert v-if="record.error_message" type="error" :message="record.error_message" show-icon class="history-error" />
          <a-table
            v-else
            row-key="id"
            size="small"
            :columns="resultColumns"
            :data-source="record.results || []"
            :pagination="false"
            :scroll="{ x: 860 }"
          >
            <template #bodyCell="{ column, record: result }">
              <template v-if="column.key === 'status'">
                <a-tag :color="resultStatusMap[result.status]?.color || 'default'">
                  {{ resultStatusMap[result.status]?.label || result.status }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'expected_value'">{{ formatCheckValue(result.expected_value) }}</template>
              <template v-else-if="column.key === 'actual_value'">{{ formatCheckValue(result.actual_value) }}</template>
            </template>
          </a-table>
        </template>
      </a-table>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import store from '@/store'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { formatTimeWithTimezone } from '@/util/timezone'
import {
  batchDeleteApplication,
  checkApplicationDeploymentBaseline,
  controlApplicationDeployment,
  deleteApplicationDeployment,
  deleteApplicationDeploymentTemplate,
  getApplicationDeploymentBaselineHistory,
  getApplicationDeploymentList,
  getApplicationDeploymentTemplateList,
  getApplicationList,
} from '@/api/assets/application'
import Dialog from './Dialog.vue'
import VersionDialog from './VersionDialog.vue'
import TemplateDialog from './TemplateDialog.vue'
import DeploymentDialog from './DeploymentDialog.vue'

const activeTab = ref('applications')
const keyword = ref('')
const rows = ref([])
const loading = ref(false)
const applicationDialogOpen = ref(false)
const versionDialogOpen = ref(false)
const templateDialogOpen = ref(false)
const deploymentDialogOpen = ref(false)
const historyDialogOpen = ref(false)
const historyLoading = ref(false)
const baselineHistory = ref([])
const historyDeployment = ref(null)
const checkingDeploymentId = ref(null)
const selectedApplication = ref(null)
const selectedDeployment = ref(null)
const selectedTemplateId = ref(null)
const deploymentControlLoading = reactive({})
const paginationState = reactive({ current: 1, pageSize: 10, total: 0 })

const categoryLabels = { web_container: 'Web 容器', database: '数据库', middleware: '中间件', business: '业务应用', other: '其他' }
const controlTypeLabels = { systemd: 'Systemd', command: '命令行', external_ha: '外部 HA', docker: 'Docker', docker_compose: 'Docker Compose' }
const controlTypeColors = { systemd: 'blue', command: 'orange', external_ha: 'gold', docker: 'cyan', docker_compose: 'geekblue' }
const healthStatusMap = {
  unknown: { label: '未检查', badge: 'default' },
  checking: { label: '检查中', badge: 'processing' },
  healthy: { label: '正常', badge: 'success' },
  unhealthy: { label: '异常', badge: 'error' },
  error: { label: '检查失败', badge: 'warning' },
}
const executionStatusMap = {
  queued: { label: '等待中', color: 'default' },
  running: { label: '检查中', color: 'processing' },
  completed: { label: '已完成', color: 'blue' },
  failed: { label: '执行失败', color: 'error' },
}
const resultStatusMap = {
  pass: { label: '通过', color: 'success' },
  fail: { label: '失败', color: 'error' },
  error: { label: '错误', color: 'warning' },
  skipped: { label: '跳过', color: 'default' },
}
const applicationColumns = [
  { title: '应用名称', dataIndex: 'name', key: 'name', sorter: true, width: 180 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 150 },
  { title: '类别', key: 'category', width: 120 },
  { title: '厂商', dataIndex: 'vendor', key: 'vendor', width: 150 },
  { title: '版本', key: 'versions', width: 240 },
  { title: '部署数', dataIndex: 'deployment_count', key: 'deployment_count', width: 90 },
  { title: '模板数', dataIndex: 'deployment_template_count', key: 'deployment_template_count', width: 90 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '备注', dataIndex: 'remark', key: 'remark' },
  { title: '操作', key: 'action', width: 300, fixed: 'right' },
]
const deploymentColumns = [
  { title: '实例名称', dataIndex: 'instance_name', key: 'instance_name', sorter: true, width: 180 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 160 },
  { title: '版本', dataIndex: 'version', key: 'version', width: 120 },
  { title: '主机', key: 'host', width: 190 },
  { title: '环境', dataIndex: 'environment', key: 'environment', width: 100 },
  { title: '部署模板', dataIndex: 'template_name', key: 'template_name', width: 180 },
  { title: '控制方式', key: 'control_type', width: 150 },
  { title: '健康状态', key: 'health_status', width: 130 },
  { title: '最后检查', key: 'last_check_time', width: 170 },
  { title: '操作', key: 'action', width: 190, fixed: 'right' },
]
const templateColumns = [
  { title: '模板名称', dataIndex: 'name', key: 'name', sorter: true, width: 220 },
  { title: '所属应用', dataIndex: 'application_name', key: 'application_name', width: 180 },
  { title: '控制方式', key: 'control_type', width: 150 },
  { title: '运行用户', dataIndex: 'run_user', key: 'run_user', width: 130 },
  { title: 'App Home', dataIndex: 'app_home', key: 'app_home', width: 280 },
  { title: '服务名称', dataIndex: 'service_name', key: 'service_name', width: 180 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 220 },
  { title: '操作', key: 'action', width: 120, fixed: 'right' },
]
const historyColumns = [
  { title: '状态', key: 'status', width: 110 },
  { title: '结论', key: 'passed', width: 90 },
  { title: '通过项', key: 'count', width: 100 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 120 },
  { title: '任务 ID', dataIndex: 'job_id', key: 'job_id', width: 230 },
  { title: '检查时间', key: 'create_time', width: 170 },
]
const resultColumns = [
  { title: '检查项', dataIndex: 'name', key: 'name', width: 180 },
  { title: '类型', dataIndex: 'check_type', key: 'check_type', width: 150 },
  { title: '状态', key: 'status', width: 90 },
  { title: '期望值', key: 'expected_value', width: 150 },
  { title: '实际值', key: 'actual_value', width: 150 },
  { title: '说明', dataIndex: 'message', key: 'message', width: 220 },
]
const currentColumns = computed(() => ({ applications: applicationColumns, templates: templateColumns, deployments: deploymentColumns }[activeTab.value]))
const currentTableScroll = computed(() => ({ x: ({ applications: 1100, templates: 1570, deployments: 1560 }[activeTab.value]) }))
const createButtonLabel = computed(() => ({ applications: '新增应用', templates: '新增模板', deployments: '登记实例' }[activeTab.value]))
const searchPlaceholder = computed(() => ({ applications: '搜索应用、编码或厂商', templates: '搜索模板、应用或服务名', deployments: '搜索应用、版本、主机或实例' }[activeTab.value]))
let baselinePollTimer = null
let reloadSequence = 0
const pagination = computed(() => ({
  current: paginationState.current,
  pageSize: paginationState.pageSize,
  total: paginationState.total,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
}))

async function reload(resetPage = false) {
  if (resetPage) paginationState.current = 1
  const requestedTab = activeTab.value
  const currentSequence = ++reloadSequence
  loading.value = true
  try {
    const params = { page: paginationState.current, page_size: paginationState.pageSize, search: keyword.value }
    const listRequests = {
      applications: getApplicationList,
      templates: getApplicationDeploymentTemplateList,
      deployments: getApplicationDeploymentList,
    }
    const response = await listRequests[requestedTab](params)
    if (currentSequence !== reloadSequence || activeTab.value !== requestedTab) return
    const data = response?.data?.data || {}
    rows.value = data.results || []
    paginationState.total = Number(data.count || 0)
  } finally {
    if (currentSequence === reloadSequence) loading.value = false
  }
}

function handleTabChange(nextTab) {
  activeTab.value = nextTab
  keyword.value = ''
  rows.value = []
  paginationState.total = 0
  reload(true)
}
function getWorkspaceRowKey(record) {
  return `${activeTab.value}-${record.id}`
}
function handleTableChange(page) {
  paginationState.current = page.current
  paginationState.pageSize = page.pageSize
  reload(false)
}
function openApplication(record = null) {
  selectedApplication.value = record
  applicationDialogOpen.value = true
}
function openVersions(record) {
  selectedApplication.value = record
  versionDialogOpen.value = true
}
function openTemplate(record = null) {
  selectedTemplateId.value = record?.id || null
  templateDialogOpen.value = true
}
function openDeployment(record = null) {
  selectedDeployment.value = record
  deploymentDialogOpen.value = true
}
function openCurrentTabCreateDialog() {
  if (activeTab.value === 'applications') openApplication()
  else if (activeTab.value === 'templates') openTemplate()
  else openDeployment()
}
function formatDateTime(value) {
  return value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
}
function formatPassRate(value) {
  return value === null || value === undefined ? '通过率 -' : `通过率 ${Number(value).toFixed(1)}%`
}
function formatCheckValue(value) {
  if (value === null || value === undefined || value === '') return '-'
  return typeof value === 'object' ? JSON.stringify(value) : String(value)
}
async function runBaselineCheck(record) {
  checkingDeploymentId.value = record.id
  try {
    await checkApplicationDeploymentBaseline(record.id)
    message.success('基线检查任务已提交')
    await reload(false)
    scheduleBaselinePoll(record.id, 0)
  } finally {
    checkingDeploymentId.value = null
  }
}
function controlLoadingKey(deploymentId, action) {
  return `${deploymentId}:${action}`
}
function isControlLoading(deploymentId, action) {
  return Boolean(deploymentControlLoading[controlLoadingKey(deploymentId, action)])
}
function isDeploymentBusy(deploymentId) {
  return ['start', 'stop', 'status'].some((action) => isControlLoading(deploymentId, action))
}
async function runApplicationControl(record, action) {
  const loadingKey = controlLoadingKey(record.id, action)
  deploymentControlLoading[loadingKey] = true
  try {
    const response = await controlApplicationDeployment(record.id, action)
    const data = response?.data?.data || {}
    const actionLabel = { start: '启动', stop: '停止', status: '状态' }[action]
    if (action === 'status') {
      Modal.info({
        title: `${record.instance_name} - 应用状态`,
        content: data.output || '命令执行成功，无输出',
        okText: '关闭',
      })
    } else {
      message.success(`${record.instance_name} ${actionLabel}成功`)
    }
  } finally {
    delete deploymentControlLoading[loadingKey]
  }
}
function confirmApplicationControl(record, action) {
  const actionLabel = action === 'start' ? '启动' : '停止'
  Modal.confirm({
    title: `确认${actionLabel}应用`,
    content: `${record.application_name} / ${record.instance_name} / ${record.host_name || record.host_ip}`,
    okText: `确认${actionLabel}`,
    cancelText: '取消',
    okButtonProps: action === 'stop' ? { danger: true } : {},
    onOk: () => runApplicationControl(record, action),
  })
}
function scheduleBaselinePoll(deploymentId, attempt) {
  if (baselinePollTimer) clearTimeout(baselinePollTimer)
  if (attempt >= 30) return
  baselinePollTimer = setTimeout(async () => {
    await reload(false)
    const deployment = rows.value.find((item) => item.id === deploymentId)
    if (deployment?.health_status === 'checking') scheduleBaselinePoll(deploymentId, attempt + 1)
  }, 2000)
}
async function openBaselineHistory(record) {
  historyDeployment.value = record
  historyDialogOpen.value = true
  historyLoading.value = true
  try {
    const response = await getApplicationDeploymentBaselineHistory(record.id)
    baselineHistory.value = response?.data?.data || []
  } finally {
    historyLoading.value = false
  }
}
function confirmDeleteApplication(record) {
  openDeleteConfirm({
    title: '确认删除应用',
    summary: '应用版本和未被保护的关联资产将一并删除。',
    items: [`应用: ${record.name}`],
    onConfirm: async () => {
      await batchDeleteApplication([record.id])
      message.success('应用删除成功')
      reload(false)
    },
  })
}
function confirmDeleteDeployment(record) {
  openDeleteConfirm({
    title: '确认删除部署实例',
    summary: '仅删除该主机上的实例登记，不会删除应用版本或部署模板。',
    items: [`${record.application_name} / ${record.instance_name} / ${record.host_ip}`],
    onConfirm: async () => {
      await deleteApplicationDeployment(record.id)
      message.success('部署实例删除成功')
      reload(false)
    },
  })
}
function confirmDeleteTemplate(record) {
  openDeleteConfirm({
    title: '确认删除部署模板',
    summary: '已被部署实例引用的模板不能删除。',
    items: [`${record.application_name} / ${record.name}`],
    onConfirm: async () => {
      await deleteApplicationDeploymentTemplate(record.id)
      message.success('部署模板删除成功')
      reload(false)
    },
  })
}

onMounted(() => reload(true))
onBeforeUnmount(() => {
  if (baselinePollTimer) clearTimeout(baselinePollTimer)
})
</script>

<style scoped>
.application-workspace { padding: 0 2px; }
.tools { margin-bottom: 16px; }
.right-tools { display: flex; justify-content: flex-end; }
.secondary { color: rgba(0, 0, 0, 0.45); font-size: 12px; }
.history-error { margin-bottom: 8px; }
</style>
