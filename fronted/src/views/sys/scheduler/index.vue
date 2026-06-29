<template>
  <div class="scheduler-page">
    <a-row class="tools" :gutter="16">
      <a-col :span="16">
        <a-input-search
          v-model:value="filterText"
          placeholder="搜索任务名称 / 任务编码"
          allow-clear
          enter-button
          @search="loadTasks"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-button type="primary" @click="reload" :loading="loading">
          刷新
        </a-button>
      </a-col>
    </a-row>

    <a-card size="small" class="task-card">
      <a-table
        :columns="columns"
        :data-source="tasks"
        :loading="loading"
        :pagination="pagination"
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
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button size="small" @click="openEditModal(record)">编辑</a-button>
              <a-button size="small" type="primary" @click="viewLogs(record)">日志</a-button>
              <a-button
                size="small"
                type="primary"
                @click="runNow(record)"
                :loading="runningTaskId === record.id"
                :disabled="record.is_running || runningTaskId === record.id"
              >
                立即执行
              </a-button>
              <a-button
                size="small"
                v-if="record.enabled"
                danger
                @click="toggleEnable(record, false)"
                :disabled="record.is_running"
              >
                禁用
              </a-button>
              <a-button
                size="small"
                v-else
                type="primary"
                @click="toggleEnable(record, true)"
                :disabled="record.is_running"
              >
                启用
              </a-button>
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
      width="520"
      :confirmLoading="editLoading"
      @ok="submitEdit"
      @cancel="editVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="任务名称">
          <a-input v-model:value="editForm.name" disabled />
        </a-form-item>
        <a-form-item label="任务编码">
          <a-input v-model:value="editForm.code" disabled />
        </a-form-item>
        <a-form-item label="关联菜单">
          <a-input v-model:value="editForm.menu_name" disabled />
        </a-form-item>
        <a-form-item label="间隔(分钟)" required>
          <a-input-number
            v-model:value="editForm.interval_minutes"
            :min="1"
            style="width: 100%"
          />
        </a-form-item>
        <a-form-item label="备注">
          <a-textarea v-model:value="editForm.remark" rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="任务执行日志"
      :open="logsVisible"
      width="1000"
      :footer="null"
      @cancel="logsVisible = false"
    >
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
          <template v-if="column.key === 'action'">
            <a-button size="small" type="link" @click="viewLogDetail(record)">
              查看详情
            </a-button>
          </template>
        </template>
      </a-table>
    </a-modal>

    <a-modal
      title="任务执行详细日志"
      :open="logDetailVisible"
      width="1000"
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
import { ref, reactive, onMounted } from 'vue'
import { message } from 'ant-design-vue'
import { getTaskList, getTaskLogList, enableTask, disableTask, updateTask, runTaskNow, getTaskStatus } from '@/api/sys/scheduler'
import { getConfigByKey, CONFIG_KEYS } from '@/api/sys/sysconfig'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { formatTimeWithTimezone } from '@/util/timezone'

const filterText = ref('')
const loading = ref(false)
const logsLoading = ref(false)
const logsVisible = ref(false)
const logDetailVisible = ref(false)
const editVisible = ref(false)
const editLoading = ref(false)
const runningTaskId = ref(null)
const userTimezone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
const currentTask = ref(null)
const currentLogDetail = ref(null)
const tasks = ref([])
const logs = ref([])
const editForm = reactive({
  id: null,
  name: '',
  code: '',
  menu_name: '',
  interval_minutes: 15,
  remark: '',
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showTotal: (total) => `共 ${total} 条`,
  showQuickJumper: true,
})

const logsPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showTotal: (total) => `共 ${total} 条`,
  showQuickJumper: true,
})

const columns = [
  { title: '任务名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '任务编码', dataIndex: 'code', key: 'code', width: 180 },
  { title: '关联菜单', dataIndex: 'menu_name', key: 'menu_name', width: 180 },
  { title: '间隔(分钟)', dataIndex: 'interval_minutes', key: 'interval_minutes', width: 120 },
  { title: '启用状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '运行状态', dataIndex: 'is_running', key: 'is_running', width: 100 },
  { title: '最近执行时间', dataIndex: 'last_run_time', key: 'last_run_time', width: 180 },
  { title: '下次运行时间', dataIndex: 'next_run_time', key: 'next_run_time', width: 180 },
  { title: '最近结果', dataIndex: 'last_status', key: 'last_status', width: 120 },
  { title: '备注', dataIndex: 'remark', key: 'remark' },
  { title: '操作', key: 'action', fixed: 'right', width: 260 },
]

const logColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '执行时间', dataIndex: 'run_time', key: 'run_time', width: 200 },
  { title: '任务名称', dataIndex: 'task_name', key: 'task_name', width: 180 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '耗时(s)', dataIndex: 'duration_seconds', key: 'duration_seconds', width: 100 },
  { title: '信息', dataIndex: 'message', key: 'message', width: 150 },
  { title: '操作', key: 'action', fixed: 'right', width: 80 },
]

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
      userTimezone.value,
      'YYYY-MM-DD HH:mm:ss'
    )
  } catch (error) {
    return timeValue
  }
}

const loadUserTimezone = () => {
  getCurrentUserInfo()
    .then((res) => {
      const timezone = res?.data?.data?.timezone
      if (timezone) {
        userTimezone.value = timezone
      }
    })
    .catch(() => {})
}

const loadTasks = () => {
  loading.value = true
  getTaskList({
    search: filterText.value,
    page: pagination.current,
    page_size: pagination.pageSize,
  })
    .then((res) => {
      const data = res.data.data
      tasks.value = data.results || data
      pagination.total = data.count || data.length || 0
    })
    .finally(() => {
      loading.value = false
    })
}

const loadLogs = (taskId) => {
  logsLoading.value = true
  getTaskLogList({
    task_id: taskId,
    page: logsPagination.current,
    size: logsPagination.pageSize,
  })
    .then((res) => {
      const data = res.data.data
      logs.value = data.results || data
      logsPagination.total = data.count || data.length || 0
    })
    .finally(() => {
      logsLoading.value = false
    })
}

const reload = () => {
  pagination.current = 1
  loadTasks()
}

const handleTableChange = (paginationInfo) => {
  pagination.current = paginationInfo.current
  pagination.pageSize = paginationInfo.pageSize
  loadTasks()
}

const handleLogTableChange = (paginationInfo) => {
  logsPagination.current = paginationInfo.current
  logsPagination.pageSize = paginationInfo.pageSize
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

const viewLogDetail = (record) => {
  currentLogDetail.value = record
  logDetailVisible.value = true
}

const openEditModal = (record) => {
  editForm.id = record.id
  editForm.name = record.name
  editForm.code = record.code
  editForm.menu_name = record.menu_name || ''
  editForm.interval_minutes = record.interval_minutes || 15
  editForm.remark = record.remark || ''
  editVisible.value = true
}

const submitEdit = () => {
  if (!editForm.id) {
    return
  }
  if (!editForm.interval_minutes || editForm.interval_minutes < 1) {
    message.error('请输入有效的间隔分钟数')
    return
  }
  editLoading.value = true
  updateTask(editForm.id, {
    interval_minutes: editForm.interval_minutes,
    remark: editForm.remark,
  })
    .then((res) => {
      message.success('保存成功')
      editVisible.value = false
      loadTasks()
    })
    .catch(() => {
      message.error('保存失败')
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
  loadUserTimezone()
  loadTasks()
})
</script>

<style scoped>
.scheduler-page {
  padding: 16px;
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
.text-muted {
  color: #999;
  font-style: italic;
}
</style>
