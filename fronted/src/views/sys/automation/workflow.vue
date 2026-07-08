<template>
  <div class="workflow-page">
    <a-row class="tools" :gutter="12">
      <a-col :span="16">
        <a-input-search
          v-model:value="keyword"
          placeholder="搜索 Workflow 名称"
          allow-clear
          enter-button
          @search="loadWorkflows(true)"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-button size="large" @click="openBuilderForCreate" v-permission="'automation:workflow:create'">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp新增Workflow</span>
          </a-button>
          <a-button type="primary" ghost :loading="loading || runLoading" @click="reloadAll">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading || runLoading" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="Workflow 列表" size="small" class="block-card">
      <a-table
        :columns="columns"
        :data-source="records"
        :loading="loading"
        :pagination="pagination"
        :scroll="{ x: 1200 }"
        rowKey="id"
        size="small"
        @change="onTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '禁用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'node_count'">
            <span>{{ Number(record.node_count || 0) }} 节点 / {{ Number(record.edge_count || 0) }} 连线</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑编排">
                <a-button size="small" type="primary" @click="openBuilderForEdit(record)" v-permission="'automation:workflow:update'">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip :title="isWorkflowLaunchDisabled(record) ? '已禁用，无法启动' : '启动'">
                <a-button
                  size="small"
                  type="primary"
                  ghost
                  :disabled="isWorkflowLaunchDisabled(record)"
                  :loading="launchingId === record.id"
                  @click="launch(record)"
                  v-permission="'automation:workflow:launch'"
                >
                  <FontAwesomeIcon :icon="['fas', 'play']" />
                </a-button>
              </a-tooltip>
              <a-popconfirm title="确认删除该 Workflow 吗？" ok-text="确认" cancel-text="取消" @confirm="removeRecord(record)">
                <a-button size="small" type="primary" danger v-permission="'automation:workflow:delete'">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-card title="运行记录" size="small" class="block-card">
      <a-table
        :columns="runColumns"
        :data-source="runRecords"
        :loading="runLoading"
        :pagination="runPagination"
        :scroll="{ x: 1100 }"
        rowKey="id"
        size="small"
        @change="onRunTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="getRunStatusColor(record.status)">{{ record.status || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button size="small" type="primary" ghost @click="openRunStatus(record)">
                查看状态图
              </a-button>
              <a-popconfirm
                v-if="canCancelWorkflowRunRecord(record)"
                title="确认取消该 Workflow 运行吗？"
                ok-text="确认"
                cancel-text="取消"
                @confirm="cancelRunRecord(record)"
              >
                <a-button
                  size="small"
                  danger
                  :loading="runCancelingId === record.id"
                  v-permission="'automation:jobs:cancel'"
                >
                  取消运行
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      title="确认启动 Workflow"
      :open="launchModalVisible"
      :confirm-loading="launchSubmitting"
      ok-text="确认启动"
      cancel-text="取消"
      @ok="confirmLaunch"
      @cancel="closeLaunchModal"
    >
      <a-form layout="vertical">
        <a-form-item label="Workflow">
          <a-input :value="launchTarget?.name || '-'" readonly />
        </a-form-item>
      </a-form>
    </a-modal>

  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  getWorkflowList,
  deleteWorkflow,
  launchWorkflow,
  getWorkflowRunList,
  cancelWorkflowRun,
} from '@/api/sys/automation'

const router = useRouter()

const loading = ref(false)
const runLoading = ref(false)
const runCancelingId = ref(null)
const launchingId = ref(null)
const launchSubmitting = ref(false)
const launchModalVisible = ref(false)
const launchTarget = ref(null)
const keyword = ref('')

const records = ref([])
const runRecords = ref([])

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const runPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '图规模', dataIndex: 'node_count', key: 'node_count', width: 160 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 140 },
  { title: '操作', key: 'action', width: 240, fixed: 'right' },
]

const runColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: 'Workflow', dataIndex: 'workflow_name', key: 'workflow_name', width: 180 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 140 },
  { title: '触发人', dataIndex: 'requested_username', key: 'requested_username', width: 130 },
  { title: '创建时间', dataIndex: 'create_time', key: 'create_time', width: 140 },
  { title: '操作', key: 'action', width: 220, fixed: 'right' },
]

async function loadWorkflows(resetPage = false) {
  if (resetPage) {
    pagination.current = 1
  }

  loading.value = true
  try {
    const res = await getWorkflowList({
      page: pagination.current,
      page_size: pagination.pageSize,
      search: String(keyword.value || '').trim(),
    })
    const data = res?.data?.data || {}
    records.value = Array.isArray(data.results) ? data.results : []
    pagination.total = Number(data.count || 0)
  } finally {
    loading.value = false
  }
}

async function loadWorkflowRuns(resetPage = false) {
  if (resetPage) {
    runPagination.current = 1
  }

  runLoading.value = true
  try {
    const res = await getWorkflowRunList({
      page: runPagination.current,
      page_size: runPagination.pageSize,
    })
    const data = res?.data?.data || {}
    runRecords.value = Array.isArray(data.results) ? data.results : []
    runPagination.total = Number(data.count || 0)
  } finally {
    runLoading.value = false
  }
}

function openBuilderForCreate() {
  router.push('/sys/automation/workflow/create')
}

function openBuilderForEdit(record) {
  router.push({ path: '/sys/automation/workflow/editor', query: { id: record.id } })
}

async function removeRecord(record) {
  await deleteWorkflow(record.id)
  message.success('删除成功')
  await loadWorkflows(false)
}

function launch(record) {
  if (isWorkflowLaunchDisabled(record)) {
    message.warning('该 Workflow 已禁用，无法启动')
    return
  }
  launchTarget.value = record || null
  launchModalVisible.value = true
}

function isWorkflowLaunchDisabled(record) {
  return !Boolean(record?.enabled)
}

function closeLaunchModal() {
  launchModalVisible.value = false
  launchSubmitting.value = false
  launchTarget.value = null
}

async function confirmLaunch() {
  if (!launchTarget.value?.id) {
    return
  }
  launchingId.value = launchTarget.value.id
  launchSubmitting.value = true
  try {
    const res = await launchWorkflow(launchTarget.value.id, {})
    const runId = Number(res?.data?.data?.id || 0)
    message.success('Workflow 已启动')
    closeLaunchModal()
    if (Number.isInteger(runId) && runId > 0) {
      router.push({
        path: '/sys/automation/workflow/run',
        query: { run_id: String(runId) },
      })
      return
    }
    await loadWorkflowRuns(true)
  } finally {
    launchSubmitting.value = false
    launchingId.value = null
  }
}

function openRunStatus(record) {
  const runId = Number(record?.id)
  if (!Number.isInteger(runId) || runId <= 0) {
    message.warning('无效的运行记录ID')
    return
  }
  router.push({
    path: '/sys/automation/workflow/run',
    query: { run_id: String(runId) },
  })
}

function normalizeRunStatus(record) {
  return String(record?.runtime_status || record?.status || '').toLowerCase()
}

function canCancelWorkflowRunRecord(record) {
  return ['pending', 'running'].includes(normalizeRunStatus(record))
}

async function cancelRunRecord(record) {
  const runId = Number(record?.id)
  if (!Number.isInteger(runId) || runId <= 0) {
    message.warning('无效的运行记录ID')
    return
  }

  runCancelingId.value = runId
  try {
    await cancelWorkflowRun(runId)
    message.success('Workflow 运行已取消')
    await loadWorkflowRuns(false)
  } finally {
    runCancelingId.value = null
  }
}

function getRunStatusColor(status) {
  if (status === 'success') {
    return 'green'
  }
  if (status === 'failed') {
    return 'red'
  }
  if (status === 'running') {
    return 'blue'
  }
  return 'default'
}

function onTableChange(page) {
  pagination.current = page.current
  pagination.pageSize = page.pageSize
  loadWorkflows(false)
}

function onRunTableChange(page) {
  runPagination.current = page.current
  runPagination.pageSize = page.pageSize
  loadWorkflowRuns(false)
}

function reloadAll() {
  loadWorkflows(false)
  loadWorkflowRuns(false)
}

onMounted(async () => {
  await Promise.all([loadWorkflows(true), loadWorkflowRuns(true)])
})
</script>

<style scoped>
.workflow-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.right-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.block-card :deep(.ant-card-body) {
  padding-top: 12px;
}

@media (max-width: 992px) {
  .right-actions {
    justify-content: flex-start;
    margin-top: 8px;
  }
}
</style>
