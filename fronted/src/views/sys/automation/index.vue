<template>
  <div class="automation-page">
    <a-row :gutter="16">
      <a-col :span="24">
        <a-card title="执行控制台" size="small" class="block-card">
          <a-form layout="vertical">
            <a-row :gutter="12">
              <a-col :span="12">
                <a-form-item label="选择模板" required>
                  <a-select
                    v-model:value="runForm.templateId"
                    :options="playbookOptions"
                    placeholder="请选择模板"
                    show-search
                    optionFilterProp="label"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="主机选择（按实例名）">
                  <a-select
                    v-model:value="runForm.hostIds"
                    mode="multiple"
                    allow-clear
                    show-search
                    :filter-option="false"
                    :options="hostOptions"
                    :loading="hostLoading"
                    placeholder="输入实例名/主机名/IP 搜索并选择"
                    @search="onHostSearch"
                  />
                </a-form-item>
              </a-col>
            </a-row>

            <a-row :gutter="12">
              <a-col :span="12">
                <a-form-item label="主机分组（树形勾选，可选）">
                  <a-tree-select
                    v-model:value="runForm.groupIds"
                    multiple
                    tree-checkable
                    allow-clear
                    show-search
                    tree-default-expand-all
                    :tree-data="groupTreeData"
                    :loading="groupLoading"
                    :dropdown-style="{ maxHeight: '360px', overflow: 'auto' }"
                    placeholder="勾选主机分组"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="额外变量 JSON（可选）">
                  <a-input
                    v-model:value="runForm.extraVarsText"
                    placeholder='例如: {"env":"prod"}'
                  />
                </a-form-item>
              </a-col>
            </a-row>

            <a-space>
              <a-button type="primary" :loading="runningSubmit" @click="onRun" v-permission="'automation:jobs:create'">
                <FontAwesomeIcon :icon="['fas', 'play']" />
                <span>&nbsp;执行</span>
              </a-button>
              <a-button @click="resetRunForm">重置</a-button>
              <a-button type="primary" ghost :loading="playbookLoading" @click="reloadAll">
                <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="playbookLoading" />
                <span>&nbsp;刷新</span>
              </a-button>
            </a-space>
          </a-form>
        </a-card>

        <a-card title="执行任务" size="small" class="block-card jobs-card">
          <a-table
            :columns="jobColumns"
            :data-source="jobs"
            :loading="jobLoading"
            :pagination="jobPagination"
            rowKey="id"
            size="small"
            @change="handleJobTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'status'">
                <a-tag :color="statusColor(record.status)">
                  {{ record.status }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'duration_seconds'">
                {{ record.duration_seconds ? `${record.duration_seconds.toFixed(2)}s` : '-' }}
              </template>
              <template v-else-if="column.key === 'action'">
                <a-space>
                  <a-button size="small" @click="showTargets(record)" v-permission="'automation:targets:view'">
                    查看目标
                  </a-button>
                  <a-button
                    size="small"
                    danger
                    v-if="record.status === 'pending' || record.status === 'running'"
                    @click="onCancelJob(record)"
                    v-permission="'automation:jobs:cancel'"
                  >
                    取消
                  </a-button>
                </a-space>
              </template>
            </template>
          </a-table>
        </a-card>
      </a-col>
    </a-row>

    <a-drawer
      :title="`任务目标明细 - #${targetDrawerJobId || ''}`"
      :open="targetDrawerVisible"
      width="960"
      @close="targetDrawerVisible = false"
    >
      <a-table
        :columns="targetColumns"
        :data-source="targets"
        :loading="targetLoading"
        :pagination="false"
        rowKey="id"
        size="small"
        :scroll="{ x: 900, y: 560 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'stdout' || column.key === 'stderr'">
            <div class="log-cell">{{ record[column.dataIndex] || '-' }}</div>
          </template>
        </template>
      </a-table>
    </a-drawer>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  getPlaybookList,
  runPlaybook,
  getAutomationHostOptions,
  getAutomationGroupTree,
  getJobList,
  cancelJob,
  getTargetList,
} from '@/api/sys/automation'

const playbooks = ref([])
const playbookLoading = ref(false)

const jobs = ref([])
const jobLoading = ref(false)
const jobPagination = reactive({ current: 1, pageSize: 10, total: 0, showSizeChanger: true, showQuickJumper: true })

const targets = ref([])
const targetLoading = ref(false)
const targetDrawerVisible = ref(false)
const targetDrawerJobId = ref(null)

const runningSubmit = ref(false)
const hostLoading = ref(false)
const groupLoading = ref(false)
const hostOptions = ref([])
const groupTreeData = ref([])

const runForm = reactive({
  templateId: null,
  hostIds: [],
  groupIds: [],
  extraVarsText: '',
})

let pollTimer = null
let hostSearchTimer = null

const jobColumns = [
  { title: '任务ID', dataIndex: 'job_id', key: 'job_id', width: 220 },
  { title: '模板', dataIndex: 'template_name', key: 'template_name', width: 130 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 100 },
  { title: '耗时', dataIndex: 'duration_seconds', key: 'duration_seconds', width: 90 },
  { title: '操作', key: 'action', width: 140 },
]

const targetColumns = [
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 140 },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 120 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 90 },
  { title: '返回码', dataIndex: 'rc', key: 'rc', width: 80 },
  { title: 'stdout', dataIndex: 'stdout', key: 'stdout', width: 260 },
  { title: 'stderr', dataIndex: 'stderr', key: 'stderr', width: 260 },
]

const playbookOptions = computed(() => {
  return playbooks.value
    .filter((item) => item.enabled)
    .map((item) => ({ value: item.id, label: `${item.name} (${item.code})` }))
})

function statusColor(status) {
  if (status === 'success') return 'green'
  if (status === 'running') return 'processing'
  if (status === 'pending') return 'gold'
  if (status === 'cancelled' || status === 'skipped') return 'default'
  return 'red'
}

function parseJsonText(text, fieldLabel) {
  if (!text || !String(text).trim()) {
    return {}
  }
  try {
    const parsed = JSON.parse(text)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed
    }
    throw new Error('必须是 JSON 对象')
  } catch (error) {
    throw new Error(`${fieldLabel} 格式错误: ${error.message}`)
  }
}

function normalizeHostOptions(records) {
  return (records || []).map((item) => {
    const instanceName = item.instance_name || '-'
    const hostName = item.hostname || '-'
    const hostIp = item.ip || '-'
    return {
      value: item.id,
      label: `${instanceName} / ${hostName} (${hostIp})`,
    }
  })
}

function normalizeGroupTree(nodes) {
  return (nodes || []).map((node) => ({
    key: node.id,
    value: node.id,
    title: node.name,
    children: normalizeGroupTree(node.children || []),
  }))
}

async function loadHostOptions(searchKeyword = '') {
  hostLoading.value = true
  try {
    const res = await getAutomationHostOptions({
      page: 1,
      page_size: 30,
      search: searchKeyword,
    })
    const data = res?.data?.data || {}
    hostOptions.value = normalizeHostOptions(data.results || [])
  } finally {
    hostLoading.value = false
  }
}

async function loadGroupTree() {
  groupLoading.value = true
  try {
    const res = await getAutomationGroupTree()
    const data = res?.data?.data || []
    groupTreeData.value = normalizeGroupTree(data)
  } finally {
    groupLoading.value = false
  }
}

function onHostSearch(keyword) {
  if (hostSearchTimer) {
    window.clearTimeout(hostSearchTimer)
  }
  hostSearchTimer = window.setTimeout(() => {
    loadHostOptions(keyword)
  }, 250)
}

async function loadPlaybooks() {
  playbookLoading.value = true
  try {
    const res = await getPlaybookList({
      page: 1,
      page_size: 200,
    })
    const data = res?.data?.data || {}
    playbooks.value = data.results || []
  } finally {
    playbookLoading.value = false
  }
}

async function loadJobs() {
  jobLoading.value = true
  try {
    const res = await getJobList({
      page: jobPagination.current,
      page_size: jobPagination.pageSize,
      ordering: '-id',
    })
    const data = res?.data?.data || {}
    jobs.value = data.results || []
    jobPagination.total = data.count || 0
  } finally {
    jobLoading.value = false
  }
}

async function showTargets(jobRecord) {
  targetDrawerVisible.value = true
  targetDrawerJobId.value = jobRecord.id
  targetLoading.value = true
  try {
    const res = await getTargetList({ job_id: jobRecord.id, page_size: 1000 })
    const data = res?.data?.data || {}
    targets.value = data.results || data || []
  } finally {
    targetLoading.value = false
  }
}

async function onRun() {
  if (!runForm.templateId) {
    message.error('请先选择模板')
    return
  }

  const hostIds = Array.isArray(runForm.hostIds) ? runForm.hostIds.map((item) => Number(item)).filter((item) => Number.isInteger(item) && item > 0) : []
  const groupIds = Array.isArray(runForm.groupIds) ? runForm.groupIds.map((item) => Number(item)).filter((item) => Number.isInteger(item) && item > 0) : []
  if (hostIds.length === 0 && groupIds.length === 0) {
    message.error('至少选择一台主机或一个分组')
    return
  }

  let extraVars = {}
  try {
    extraVars = parseJsonText(runForm.extraVarsText, '额外变量 JSON')
  } catch (error) {
    message.error(error.message)
    return
  }

  runningSubmit.value = true
  try {
    await runPlaybook(runForm.templateId, {
      host_ids: hostIds,
      group_ids: groupIds,
      extra_vars: extraVars,
    })
    message.success('任务已提交，正在后台执行')
    await loadJobs()
  } finally {
    runningSubmit.value = false
  }
}

function resetRunForm() {
  runForm.templateId = null
  runForm.hostIds = []
  runForm.groupIds = []
  runForm.extraVarsText = ''
}

async function onCancelJob(record) {
  await cancelJob(record.id)
  message.success('任务已取消')
  await loadJobs()
  if (targetDrawerVisible.value && targetDrawerJobId.value === record.id) {
    await showTargets(record)
  }
}

function handleJobTableChange(page) {
  jobPagination.current = page.current
  jobPagination.pageSize = page.pageSize
  loadJobs()
}

function reloadAll() {
  loadPlaybooks()
  loadJobs()
}

function startPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
  }
  pollTimer = window.setInterval(() => {
    loadJobs()
    if (targetDrawerVisible.value && targetDrawerJobId.value) {
      getTargetList({ job_id: targetDrawerJobId.value, page_size: 1000 }).then((res) => {
        const data = res?.data?.data || {}
        targets.value = data.results || data || []
      })
    }
  }, 5000)
}

onMounted(async () => {
  await loadPlaybooks()
  await loadJobs()
  await loadHostOptions('')
  await loadGroupTree()
  startPolling()
})

onBeforeUnmount(() => {
  if (pollTimer) {
    window.clearInterval(pollTimer)
  }
  if (hostSearchTimer) {
    window.clearTimeout(hostSearchTimer)
  }
})
</script>

<style scoped>
.automation-page {
  padding: 2px;
}

.block-card {
  margin-bottom: 12px;
}

.jobs-card {
  margin-top: 12px;
}

.log-cell {
  white-space: pre-wrap;
  max-height: 160px;
  overflow: auto;
  font-size: 12px;
  color: #444;
}
</style>
