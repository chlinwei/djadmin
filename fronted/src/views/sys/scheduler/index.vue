<template>
  <div class="scheduler-page">
    <a-row class="tools" :gutter="16">
      <a-col :flex="1">
        <a-space wrap>
          <a-input-search
            class="tool-item"
            v-model:value="filterText"
            placeholder="搜索任务名称 / 任务编码"
            allow-clear
            enter-button
            @search="reload"
            style="width: 280px"
          />
          <a-select
            v-model:value="filterEnabled"
            :getPopupContainer="getPopupContainer"
            :options="enabledFilterOptions"
            allow-clear
            placeholder="启用状态"
            style="width: 120px"
            @change="reload"
          />
          <a-select
            v-model:value="filterRunning"
            :getPopupContainer="getPopupContainer"
            :options="runningFilterOptions"
            allow-clear
            placeholder="运行状态"
            style="width: 120px"
            @change="reload"
          />
        </a-space>
      </a-col>
      <a-col class="right-actions">
        <a-space>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :disabled="loading" @click="reload">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <a-card size="small" class="task-card">
      <a-table
        :columns="columns"
        :data-source="tasks"
        :loading="loading"
        :pagination="pagination"
        :scroll="{ x: 1850 }"
        rowKey="id"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">
              {{ record.enabled ? '启用' : '禁用' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'is_running'">
            <a-tag v-if="record.is_running" color="orange" icon="loading">
              运行中...
            </a-tag>
            <a-tag v-else color="green">
              就绪
            </a-tag>
          </template>
          <template v-else-if="column.key === 'menu_name'">
            <a-button
              v-if="record.menu && record.menu_name"
              type="link"
              size="small"
              @click="goToMenu(record)"
            >
              {{ record.menu_name }}
            </a-button>
            <span v-else>-</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑">
                <a-button size="small" type="primary" @click="openEditModal(record)">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="查看详情">
                <a-button size="small" @click="viewLogs(record)">
                  <FontAwesomeIcon :icon="['fas', 'eye']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="运行">
                <a-button
                  size="small"
                  type="primary"
                  ghost
                  @click="runNow(record)"
                  :loading="runningTaskId === record.id"
                  :disabled="record.is_running || runningTaskId === record.id"
                >
                  <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" />
                  <span>&nbsp;立即执行</span>
                </a-button>
              </a-tooltip>
              <a-tooltip v-if="record.enabled" title="禁用">
                <a-button
                  size="small"
                  danger
                  @click="toggleEnable(record, false)"
                  :disabled="record.is_running"
                >
                  禁用
                </a-button>
              </a-tooltip>
              <a-tooltip v-else title="启用">
                <a-button
                  size="small"
                  type="primary"
                  @click="toggleEnable(record, true)"
                  :disabled="record.is_running"
                >
                  启用
                </a-button>
              </a-tooltip>
            </a-space>
          </template>
          <template
            v-else-if="column.key === 'last_run_time' || column.key === 'next_run_time' || column.key === 'run_time'"
          >
            {{ formatTimeDisplay(record[column.dataIndex]) }}
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      title="编辑调度任务"
      :open="editVisible"
      :width="520"
      :confirmLoading="editLoading"
      @ok="submitTask"
      @cancel="editVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="任务名称" required>
          <a-input v-model:value="editForm.name" disabled />
        </a-form-item>
        <a-form-item label="任务编码" required>
          <a-input v-model:value="editForm.code" disabled />
        </a-form-item>
        <a-form-item label="关联菜单">
          <a-select
            v-model:value="editForm.menu"
            :getPopupContainer="getPopupContainer"
            :options="menuOptions"
            disabled
            placeholder="可选：关联一个菜单页面"
            allow-clear
            show-search
            optionFilterProp="label"
          />
        </a-form-item>
        <a-form-item label="启用状态">
          <a-switch v-model:checked="editForm.enabled" checked-children="启用" un-checked-children="禁用" />
        </a-form-item>
        <a-form-item label="Cron 表达式" required>
          <a-input
            v-model:value="editForm.cron_expression"
            placeholder="例如：*/15 * * * *"
          />
          <div class="cron-help">使用 5 段 Cron：分 时 日 月 周</div>
        </a-form-item>
        <a-form-item label="备注">
          <a-textarea v-model:value="editForm.description" rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="任务执行日志"
      :open="logsVisible"
      :width="1100"
      :footer="null"
      @cancel="logsVisible = false"
    >
      <div class="log-filter-toolbar">
        <a-space>
          <a-button @click="toggleLogFilterPanel">过滤</a-button>
          <a-tag v-if="hasLogFilters" color="blue">已启用筛选</a-tag>
          <a-button v-if="hasLogFilters" type="link" @click="resetLogFilters">重置</a-button>
        </a-space>
      </div>

      <a-card v-if="logFilterVisible" size="small" class="log-filter-panel">
        <a-form layout="inline">
          <a-form-item label="执行状态">
            <a-select
              v-model:value="logFilters.status"
              :getPopupContainer="getPopupContainer"
              :options="logStatusOptions"
              style="width: 140px"
              placeholder="全部"
              allow-clear
            />
          </a-form-item>
          <a-form-item label="日志内容">
            <a-input
              v-model:value="logFilters.content"
              style="width: 220px"
              placeholder="输入关键词匹配日志"
              allow-clear
            />
          </a-form-item>
          <a-form-item label="耗时最小(秒)">
            <a-input-number
              v-model:value="logFilters.durationMin"
              :min="0"
              :max="logFilters.durationMax ?? undefined"
              :precision="2"
              style="width: 140px"
              placeholder="不限"
              @change="handleDurationMinChange"
            />
          </a-form-item>
          <a-form-item label="耗时最大(秒)">
            <a-input-number
              v-model:value="logFilters.durationMax"
              :min="logFilters.durationMin ?? 0"
              :precision="2"
              style="width: 140px"
              placeholder="不限"
              @change="handleDurationMaxChange"
            />
          </a-form-item>
          <a-form-item>
            <a-space>
              <a-button type="primary" @click="applyLogFilters">应用</a-button>
              <a-button @click="resetLogFilters">清空</a-button>
            </a-space>
          </a-form-item>
        </a-form>
      </a-card>

      <a-table
        :columns="logColumns"
        :data-source="logs"
        :pagination="logsPagination"
        rowKey="id"
        size="small"
        :loading="logsLoading"
        :scroll="{ x: 1200 }"
        @change="handleLogTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'run_time'">
            {{ formatTimeDisplay(record.run_time) }}
          </template>
          <template v-if="column.key === 'action'">
            <a-tooltip title="查看详情">
              <a-button size="small" type="link" @click="viewLogDetail(record)">
                查看详情
              </a-button>
            </a-tooltip>
          </template>
        </template>
      </a-table>
    </a-modal>

    <a-modal
      title="任务执行详细日志"
      :open="logDetailVisible"
      :width="1100"
      :footer="null"
      @cancel="logDetailVisible = false"
    >
      <div v-if="currentLogDetail">
        <a-descriptions :column="1" size="small" class="mb-3">
          <a-descriptions-item label="执行时间">
            {{ formatTimeDisplay(currentLogDetail.run_time) }}
          </a-descriptions-item>
          <a-descriptions-item label="任务名称">
            {{ currentLogDetail.task_name }}
          </a-descriptions-item>
          <a-descriptions-item label="执行状态">
            <a-tag :color="currentLogDetail.status === '成功' ? 'green' : 'red'">
              {{ currentLogDetail.status }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="耗时(秒)">
            {{ currentLogDetail.duration_seconds }}
          </a-descriptions-item>
          <a-descriptions-item label="简述">
            {{ currentLogDetail.message }}
          </a-descriptions-item>
        </a-descriptions>

        <div class="log-detail-actions">
          <a-button type="primary" @click="downloadCurrentLog">
            下载日志
          </a-button>
        </div>

        <div v-if="currentLogDetail.output" class="log-output">
          <div class="log-title">执行日志输出：</div>
          <pre class="log-content">{{ currentLogDetail.output }}</pre>
        </div>
        <div v-else class="text-muted">暂无详细日志输出</div>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { message } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { getTaskList, getTaskLogList, enableTask, disableTask, updateTask, runTaskNow, getTaskStatus } from '@/api/sys/scheduler'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig'
import { getMenuTree } from '@/api/menu'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const filterText = ref('')
const filterEnabled = ref(undefined)
const filterRunning = ref(undefined)
const enabledFilterOptions = [
  { label: '已启用', value: 'true' },
  { label: '已禁用', value: 'false' },
]
const runningFilterOptions = [
  { label: '运行中', value: 'true' },
  { label: '就绪', value: 'false' },
]
const loading = ref(false)
const logsLoading = ref(false)
const logsVisible = ref(false)
const logDetailVisible = ref(false)
const editVisible = ref(false)
const editLoading = ref(false)
const runningTaskId = ref(null)
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const router = useRouter()
const currentTask = ref(null)
const currentLogDetail = ref(null)
const tasks = ref([])
const logs = ref([])
const menuOptions = ref([])
const logFilterVisible = ref(false)
const logFilters = reactive({
  status: undefined,
  content: '',
  durationMin: null,
  durationMax: null,
})
const logStatusOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
]
const editForm = reactive({
  id: null,
  name: '',
  code: '',
  menu: undefined,
  menu_name: '',
  enabled: true,
  cron_expression: '',
  interval_minutes: 15,
  description: '',
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  pageSizeOptions: ['10', '20', '30'],
  showTotal: (total) => `共有${total}条数据`,
  showQuickJumper: true,
})

const taskSort = reactive({
  field: null,
  order: null,
})

const logsPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  pageSizeOptions: ['10', '20', '30'],
  showTotal: (total) => `共有${total}条数据`,
  showQuickJumper: true,
})

const hasLogFilters = computed(() => {
  return !!(
    logFilters.status ||
    (logFilters.content && String(logFilters.content).trim()) ||
    logFilters.durationMin !== null ||
    logFilters.durationMax !== null
  )
})

const columns = [
  { title: '任务名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '任务编码', dataIndex: 'code', key: 'code', width: 180 },
  { title: '关联菜单', dataIndex: 'menu_name', key: 'menu_name', width: 180 },
  { title: 'Cron 表达式', dataIndex: 'effective_cron_expression', key: 'effective_cron_expression', width: 180 },
  { title: '启用状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '运行状态', dataIndex: 'is_running', key: 'is_running', width: 100 },
  { title: '最近执行时间', dataIndex: 'last_run_time', key: 'last_run_time', width: 180, sorter: true },
  { title: '下次运行时间', dataIndex: 'next_run_time', key: 'next_run_time', width: 180, sorter: true },
  { title: '最近结果', dataIndex: 'last_status', key: 'last_status', width: 120 },
  { title: '备注', dataIndex: 'description', key: 'description' },
  { title: '操作', key: 'action', fixed: 'right', width: 340 },
]

const resolveTaskOrdering = () => {
  if (!taskSort.field || !taskSort.order) {
    return undefined
  }
  const sortPrefix = taskSort.order === 'descend' ? '-' : ''
  return `${sortPrefix}${taskSort.field}`
}

const buildMenuOptions = (nodes, collector = []) => {
  nodes.forEach((item) => {
    if (item.menu_type === 'C') {
      collector.push({
        label: item.name,
        value: item.id,
      })
    }
    if (item.children && item.children.length) {
      buildMenuOptions(item.children, collector)
    }
  })
  return collector
}

const loadMenuOptions = () => {
  getMenuTree()
    .then((res) => {
      const tree = res?.data?.data || []
      menuOptions.value = buildMenuOptions(tree)
    })
    .catch(() => {
      menuOptions.value = []
    })
}

const logColumns = ref([
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '执行时间', dataIndex: 'run_time', key: 'run_time', width: 200 },
  { title: '任务名称', dataIndex: 'task_name', key: 'task_name', width: 180 },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    width: 100,
    sorter: true,
    sortOrder: null,
  },
  {
    title: '耗时(s)',
    dataIndex: 'duration_seconds',
    key: 'duration_seconds',
    width: 100,
    sorter: true,
    sortOrder: null,
  },
  { title: '信息', dataIndex: 'message', key: 'message', width: 150 },
  { title: '操作', key: 'action', fixed: 'right', width: 80 },
])

const logsSort = reactive({
  field: null,
  order: null,
})

const normalizeUtcTime = (timeValue) => {
  if (!timeValue || typeof timeValue !== 'string') {
    return timeValue
  }

  const text = timeValue.trim()
  if (!text) {
    return timeValue
  }

  // 后端若返回无时区字符串（如 2026-06-29 08:00:00），按 UTC 解释后再转用户时区。
  if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
    return text
  }
  return `${text.replace(' ', 'T')}Z`
}

const formatTimeDisplay = (timeValue) => {
  if (!timeValue) {
    return '-'
  }

  try {
    return formatTimeWithTimezone(
      normalizeUtcTime(timeValue),
      store.state.user?.timezone || 'Asia/Shanghai',
      'YYYY-MM-DD HH:mm:ss'
    )
  } catch (error) {
    return timeValue
  }
}

const loadTasks = () => {
  loading.value = true
  const params = {
    search: filterText.value,
    page: pagination.current,
    page_size: pagination.pageSize,
  }
  if (filterEnabled.value !== undefined && filterEnabled.value !== null && filterEnabled.value !== '') {
    params.enabled = filterEnabled.value
  }
  if (filterRunning.value !== undefined && filterRunning.value !== null && filterRunning.value !== '') {
    params.is_running = filterRunning.value
  }
  const ordering = resolveTaskOrdering()
  if (ordering) {
    params.ordering = ordering
  }
  getTaskList(params)
    .then((res) => {
      const data = res?.data?.data
      if (data && typeof data === 'object') {
        tasks.value = data.results || data || []
        pagination.total = data.count || data.length || 0
      } else {
        tasks.value = []
        pagination.total = 0
      }
    })
    .catch((error) => {
      message.error('加载定时任务列表失败: ' + (error.message || '未知错误'))
      tasks.value = []
      pagination.total = 0
    })
    .finally(() => {
      loading.value = false
    })
}

const loadLogs = (taskId) => {
  logsLoading.value = true
  const params = {
    task_id: taskId,
    page: logsPagination.current,
    size: logsPagination.pageSize,
  }
  if (logFilters.status) {
    params.status = logFilters.status
  }
  if (logFilters.content && String(logFilters.content).trim()) {
    params.content = String(logFilters.content).trim()
  }
  if (logFilters.durationMin !== null) {
    params.duration_min = logFilters.durationMin
  }
  if (logFilters.durationMax !== null) {
    params.duration_max = logFilters.durationMax
  }
  if (logsSort.field && logsSort.order) {
    const sortPrefix = logsSort.order === 'descend' ? '-' : ''
    params.ordering = `${sortPrefix}${logsSort.field}`
  }

  getTaskLogList({
    ...params,
  })
    .then((res) => {
      const data = res?.data?.data
      if (data && typeof data === 'object') {
        logs.value = data.results || data || []
        logsPagination.total = data.count || data.length || 0
      } else {
        logs.value = []
        logsPagination.total = 0
      }
    })
    .catch((error) => {
      message.error('加载执行日志失败: ' + (error.message || '未知错误'))
      logs.value = []
      logsPagination.total = 0
    })
    .finally(() => {
      logsLoading.value = false
    })
}

const reload = () => {
  pagination.current = 1
  loadTasks()
}

const resetFilters = () => {
  filterText.value = ''
  filterEnabled.value = undefined
  filterRunning.value = undefined
  reload()
}

const handleTableChange = (paginationInfo, _filters, sorter) => {
  pagination.current = paginationInfo.current
  pagination.pageSize = paginationInfo.pageSize

  const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
  const sortableFields = ['last_run_time', 'next_run_time']
  if (nextSorter?.field && sortableFields.includes(nextSorter.field) && nextSorter.order) {
    taskSort.field = nextSorter.field
    taskSort.order = nextSorter.order
  } else {
    taskSort.field = null
    taskSort.order = null
  }

  loadTasks()
}

const handleLogTableChange = (paginationInfo, _filters, sorter) => {
  logsPagination.current = paginationInfo.current
  logsPagination.pageSize = paginationInfo.pageSize

  const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
  const sortableFields = ['status', 'duration_seconds']
  if (nextSorter?.field && sortableFields.includes(nextSorter.field) && nextSorter.order) {
    logsSort.field = nextSorter.field
    logsSort.order = nextSorter.order
  } else {
    logsSort.field = null
    logsSort.order = null
  }

  logColumns.value.forEach((column) => {
    if (column.key === logsSort.field) {
      column.sortOrder = logsSort.order
    } else if (column.key === 'status' || column.key === 'duration_seconds') {
      column.sortOrder = null
    }
  })

  if (currentTask.value?.id) {
    loadLogs(currentTask.value.id)
  }
}

const viewLogs = (record) => {
  currentTask.value = record
  logsVisible.value = true
  logsPagination.current = 1
  loadLogs(record.id)
}

const toggleLogFilterPanel = () => {
  logFilterVisible.value = !logFilterVisible.value
}

const applyLogFilters = () => {
  if (
    logFilters.durationMin !== null &&
    logFilters.durationMax !== null &&
    Number(logFilters.durationMin) > Number(logFilters.durationMax)
  ) {
    message.warning('耗时最小值不能大于最大值')
    return
  }

  logsPagination.current = 1
  if (currentTask.value?.id) {
    loadLogs(currentTask.value.id)
  }
}

const handleDurationMinChange = (value) => {
  if (value === null || value === undefined || logFilters.durationMax === null || logFilters.durationMax === undefined) {
    return
  }
  if (Number(value) > Number(logFilters.durationMax)) {
    logFilters.durationMax = value
  }
}

const handleDurationMaxChange = (value) => {
  if (value === null || value === undefined || logFilters.durationMin === null || logFilters.durationMin === undefined) {
    return
  }
  if (Number(value) < Number(logFilters.durationMin)) {
    logFilters.durationMin = value
  }
}

const resetLogFilters = () => {
  logFilters.status = undefined
  logFilters.content = ''
  logFilters.durationMin = null
  logFilters.durationMax = null
  logsPagination.current = 1
  if (currentTask.value?.id) {
    loadLogs(currentTask.value.id)
  }
}

const viewLogDetail = (record) => {
  currentLogDetail.value = record
  logDetailVisible.value = true
}

const toSafeFileSegment = (value) => {
  return String(value || '')
    .replace(/[\\/:*?"<>|\s]+/g, '_')
    .replace(/^_+|_+$/g, '')
}

const downloadCurrentLog = () => {
  if (!currentLogDetail.value) {
    message.warning('暂无可下载的日志')
    return
  }

  const log = currentLogDetail.value
  const displayRunTime = formatTimeDisplay(log.run_time)
  const fileName = [
    'task_log',
    toSafeFileSegment(log.task_name || 'unknown'),
    toSafeFileSegment(displayRunTime === '-' ? Date.now() : displayRunTime),
  ].join('_') + '.txt'

  const content = [
    `任务名称: ${log.task_name || '-'}`,
    `执行时间: ${formatTimeDisplay(log.run_time)}`,
    `执行状态: ${log.status || '-'}`,
    `耗时(秒): ${log.duration_seconds ?? '-'}`,
    `简述: ${log.message || '-'}`,
    '',
    '执行日志输出:',
    log.output || '(无输出)',
  ].join('\n')

  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = fileName
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

const goToMenu = (record) => {
  if (!record.menu_path) {
    message.warning('该任务未配置可跳转的菜单路由')
    return
  }
  router.push({ path: record.menu_path })
}

const openEditModal = (record) => {
  editForm.id = record.id
  editForm.name = record.name
  editForm.code = record.code
  editForm.menu = record.menu || undefined
  editForm.menu_name = record.menu_name || ''
  editForm.enabled = !!record.enabled
  editForm.cron_expression = record.effective_cron_expression || record.cron_expression || ''
  editForm.interval_minutes = record.interval_minutes || 15
  editForm.description = record.description || ''
  editVisible.value = true
}

const submitTask = () => {
  if (!editForm.name || !String(editForm.name).trim()) {
    message.error('请输入任务名称')
    return
  }
  if (!editForm.code || !String(editForm.code).trim()) {
    message.error('请输入任务编码')
    return
  }
  if (!editForm.cron_expression || !String(editForm.cron_expression).trim()) {
    message.error('请输入有效的 Cron 表达式')
    return
  }

  const payload = {
    name: String(editForm.name).trim(),
    code: String(editForm.code).trim(),
    menu: editForm.menu || null,
    enabled: !!editForm.enabled,
    cron_expression: String(editForm.cron_expression).trim(),
    description: editForm.description,
  }

  editLoading.value = true
  updateTask(editForm.id, payload)
    .then((res) => {
      message.success('保存成功')
      editVisible.value = false
      loadTasks()
    })
    .catch((error) => {
      message.error(error?.response?.data?.msg || error?.response?.data?.error || error?.message || '保存失败')
    })
    .finally(() => {
      editLoading.value = false
    })
}

const toggleEnable = (record, enable) => {
  const action = enable ? enableTask : disableTask
  action(record.id)
    .then((res) => {
      if (res.data.code === 200) {
        message.success(enable ? '已启用' : '已禁用')
        loadTasks()
      } else {
        message.error(res.data.msg || '操作失败')
      }
    })
    .catch(() => {
      message.error('操作失败')
    })
}

const pollIntervals = ref({})  // Store poll intervals by task ID
const pollInterval = ref(3000)   // default 3s, overridden by sys config
const pollMaxCount = ref(1200)   // default 1200, overridden by sys config

// Load poll config from system parameters
getConfigByKey(CONFIG_KEYS.TASK_POLL_INTERVAL).then(res => {
  const v = parseInt(res.data.data?.value)
  if (!isNaN(v) && v > 0) pollInterval.value = v
}).catch(() => {})
getConfigByKey(CONFIG_KEYS.TASK_POLL_MAX_COUNT).then(res => {
  const v = parseInt(res.data.data?.value)
  if (!isNaN(v) && v > 0) pollMaxCount.value = v
}).catch(() => {})

const startPollingTaskStatus = (taskId, hideLoading) => {
  let pollCount = 0
  const maxPolls = pollMaxCount.value
  
  const pollFn = async () => {
    pollCount++
    
    if (pollCount > maxPolls) {
      clearInterval(pollIntervals.value[taskId])
      delete pollIntervals.value[taskId]
      hideLoading()
      message.warning('任务执行超时（>20分钟），请手动检查日志')
      runningTaskId.value = null
      loadTasks()
      return
    }
    
    try {
      const res = await getTaskStatus(taskId)
      const taskData = res.data.data
      
      // Find and update the task in the table
      const task = tasks.value.find(t => t.id === taskId)
      if (task) {
        task.is_running = taskData.is_running
        task.last_status = taskData.last_status
        task.last_message = taskData.last_message
        task.last_run_time = taskData.last_run_time
      }
      
      if (!taskData.is_running) {
        // Task completed
        clearInterval(pollIntervals.value[taskId])
        delete pollIntervals.value[taskId]
        hideLoading()
        
        if (taskData.last_status === 'success') {
          message.success('任务执行成功')
        } else if (taskData.last_status === 'failed') {
          message.error(`任务失败: ${taskData.last_message || '未知错误'}`)
        } else {
          message.warning(`任务完成，状态: ${taskData.last_status}`)
        }
        
        runningTaskId.value = null
        loadTasks()  // Refresh task list to get final data
      }
    } catch (error) {
      console.error('轮询失败:', error)
      // Continue polling even if one poll fails
    }
  }
  
  // Poll interval from sys config
  pollIntervals.value[taskId] = setInterval(pollFn, pollInterval.value)
}

const runNow = (record) => {
  if (!record.enabled) {
    message.error('任务已禁用，请先启用')
    return
  }
  
  runningTaskId.value = record.id
  const hideLoading = message.loading('任务执行中，请稍候...', 0)
  
  runTaskNow(record.id)
    .then((res) => {
      if (res.data.code === 200 && res.data.data.status === 'submitted') {
        // Start polling for task status
        startPollingTaskStatus(record.id, hideLoading)
      } else {
        hideLoading()
        message.success('任务执行成功')
        runningTaskId.value = null
        loadTasks()
      }
    })
    .catch((error) => {
      hideLoading()
      console.error(error)
      runningTaskId.value = null
      if (error.response?.data?.error) {
        message.error(error.response.data.error)
      } else {
        message.error('任务提交失败: ' + (error.message || '未知错误'))
      }
    })
}

onMounted(() => {
  loadMenuOptions()
  loadTasks()
})
</script>

<style scoped>
.scheduler-page {
  padding: 16px;
  position: relative;
}
.tools {
  margin-bottom: 20px;
}
.right-actions {
  text-align: right;
}
.task-card {
  margin-top: 12px;
}
.mb-3 {
  margin-bottom: 16px;
}
.log-output {
  margin-top: 16px;
  padding: 12px;
  background-color: #f5f5f5;
  border-radius: 4px;
  border: 1px solid #ddd;
}
.log-title {
  font-weight: 500;
  margin-bottom: 8px;
  color: #333;
}
.log-content {
  margin: 0;
  padding: 12px;
  background-color: #1e1e1e;
  color: #d4d4d4;
  border-radius: 4px;
  overflow-x: auto;
  font-family: 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  max-height: 400px;
}
.log-detail-actions {
  margin-bottom: 12px;
  text-align: right;
}
.text-muted {
  color: #999;
  font-style: italic;
}
.log-filter-toolbar {
  margin-bottom: 12px;
}
.log-filter-panel {
  margin-bottom: 12px;
}

.cron-help {
  margin-top: 6px;
  color: #888;
  font-size: 12px;
}
</style>
