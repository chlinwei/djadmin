<template>
  <a-tabs v-model:activeKey="activeMode" class="mode-tabs">
    <a-tab-pane key="agent" tab="Agent 共享 Token" />
    <a-tab-pane key="api" tab="API 绑定 Token" />
  </a-tabs>

  <a-row class="tools" :gutter="16">
    <a-col class="AddBtn tool-item" v-permission.remove="'system:api_token:create'">
      <a-button size="large" @click="openCreateModal">
        <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
        <span>&nbsp;新增</span>
      </a-button>
    </a-col>
    <a-col class="tool-item">
      <a-button size="large" @click="loadList">刷新</a-button>
    </a-col>
  </a-row>

  <a-table
    :scroll="{ x: 1600 }"
    rowKey="id"
    :columns="columns"
    :data-source="displayList"
    :loading="loading"
    :pagination="false"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'is_active'">
        <a-tag :color="record.is_active ? 'green' : 'red'">{{ record.is_active ? '启用' : '禁用' }}</a-tag>
      </template>
      <template v-else-if="column.key === 'expires_at'">
        <span>{{ record.bind_mode === 'agent' ? '永不过期' : formatDateTime(record.expires_at) }}</span>
      </template>
      <template v-else-if="column.key === 'last_used_at'">
        <span>{{ formatDateTime(record.last_used_at) }}</span>
      </template>
      <template v-else-if="column.key === 'create_time'">
        <span>{{ formatDateTime(record.create_time) }}</span>
      </template>
      <template v-else-if="column.key === 'operation'">
        <a-row :gutter="6" class="action_row">
          <a-col v-permission.remove="'system:api_token:rotate'">
            <a-tooltip title="轮换">
              <a-button type="primary" :loading="rowRotateLoading[record.id]" @click="handleRotate(record)">
                轮换
              </a-button>
            </a-tooltip>
          </a-col>
          <a-col v-permission.remove="'system:api_token:disable'">
            <a-tooltip title="禁用">
              <a-button
                class="delBtn"
                danger
                type="primary"
                :disabled="!record.is_active"
                :loading="rowDisableLoading[record.id]"
                @click="handleDisable(record)"
              >
                禁用
              </a-button>
            </a-tooltip>
          </a-col>
          <a-col v-permission.remove="'system:api_token:delete'">
            <a-tooltip title="删除">
              <a-button
                class="delBtn"
                danger
                type="primary"
                :loading="rowDeleteLoading[record.id]"
                @click="handleDelete(record)"
              >
                删除
              </a-button>
            </a-tooltip>
          </a-col>
        </a-row>
      </template>
    </template>
  </a-table>

  <a-modal v-model:open="createVisible" :title="createModalTitle" :confirm-loading="createLoading" @ok="submitCreate">
    <a-form :model="createForm" layout="vertical">
      <a-form-item label="Agent ID">
        <a-input v-model:value="createForm.agent_id" :disabled="createForm.bind_mode === 'agent'" placeholder="api 模式必填" />
      </a-form-item>
      <a-form-item label="名称">
        <a-input v-model:value="createForm.name" placeholder="可选" />
      </a-form-item>
      <a-form-item v-if="createForm.bind_mode === 'api'" label="过期时间">
        <a-date-picker
          v-model:value="createForm.expires_at"
          show-time
          format="YYYY-MM-DD HH:mm:ss"
          value-format="YYYY-MM-DDTHH:mm:ss"
          style="width: 100%"
          :getPopupContainer="getPopupContainer"
          :disabled-date="disabledPastDate"
          :disabled-time="disabledPastTime"
        />
      </a-form-item>
      <a-form-item v-else label="过期时间">
        <a-input value="永不过期" disabled />
      </a-form-item>
      <a-form-item label="备注">
        <a-input v-model:value="createForm.remark" placeholder="可选" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import dayjs from 'dayjs'
import { message, Modal } from 'ant-design-vue'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { createApiToken, deleteApiToken, disableApiToken, getApiTokenList, rotateApiToken } from '@/api/user/apiToken'
import { openDeleteConfirm } from '@/util/deleteConfirm'

defineOptions({
  name: 'apiToken',
})

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const activeMode = ref('agent')

const columns = [
  { title: 'Api ID', dataIndex: 'agent_id', key: 'agent_id', width: 180, fixed: 'left' },
  { title: '绑定模式', dataIndex: 'bind_mode', key: 'bind_mode', width: 130 },
  { title: '名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '状态', dataIndex: 'is_active', key: 'is_active', width: 100 },
  { title: '创建人', dataIndex: 'created_by_username', key: 'created_by_username', width: 140 },
  { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', width: 220 },
  { title: '最后使用时间', dataIndex: 'last_used_at', key: 'last_used_at', width: 220 },
  { title: '创建时间', dataIndex: 'create_time', key: 'create_time', width: 220 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 220 },
  { title: '操作', key: 'operation', fixed: 'right', width: 260 },
]

const loading = ref(false)
const sourceList = ref([])
const rowRotateLoading = reactive({})
const rowDisableLoading = reactive({})
const rowDeleteLoading = reactive({})

const displayList = computed(() => {
  return sourceList.value.filter((item) => item.bind_mode === activeMode.value)
})

const createModalTitle = computed(() => (activeMode.value === 'agent' ? '创建 Agent 共享 Token' : '创建 API 绑定 Token'))

const createVisible = ref(false)
const createLoading = ref(false)
const createForm = reactive({
  bind_mode: 'agent',
  agent_id: '',
  name: '',
  expires_at: '',
  remark: '',
})

const normalizeUtcTime = (value) => {
  if (!value || typeof value !== 'string') {
    return value
  }
  const text = value.trim()
  if (!text) {
    return value
  }
  if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
    return text
  }
  return `${text.replace(' ', 'T')}Z`
}

const formatDateTime = (value) => {
  if (!value) {
    return '-'
  }
  try {
    return formatTimeWithTimezone(normalizeUtcTime(value), store.state.user?.timezone || 'Asia/Shanghai', 'YYYY-MM-DD HH:mm:ss')
  } catch (error) {
    return value
  }
}

const setRowRotateLoading = (id, status) => {
  rowRotateLoading[id] = status
}

const setRowDisableLoading = (id, status) => {
  rowDisableLoading[id] = status
}

const setRowDeleteLoading = (id, status) => {
  rowDeleteLoading[id] = status
}

const disabledPastDate = (current) => {
  if (!current) {
    return false
  }
  return current.isBefore(dayjs().startOf('day'))
}

const disabledPastTime = (current) => {
  if (!current) {
    return {}
  }

  const now = dayjs()
  if (!current.isSame(now, 'day')) {
    return {}
  }

  const currentHour = now.hour()
  const currentMinute = now.minute()
  const currentSecond = now.second()

  return {
    disabledHours: () => Array.from({ length: currentHour }, (_, i) => i),
    disabledMinutes: (selectedHour) => {
      if (selectedHour !== currentHour) {
        return []
      }
      return Array.from({ length: currentMinute }, (_, i) => i)
    },
    disabledSeconds: (selectedHour, selectedMinute) => {
      if (selectedHour !== currentHour || selectedMinute !== currentMinute) {
        return []
      }
      return Array.from({ length: currentSecond }, (_, i) => i)
    },
  }
}

const loadList = async () => {
  loading.value = true
  try {
    const res = await getApiTokenList()
    sourceList.value = res?.data?.data?.results || []
  } finally {
    loading.value = false
  }
}

const resetCreateForm = () => {
  createForm.bind_mode = activeMode.value
  createForm.agent_id = ''
  createForm.name = ''
  createForm.expires_at = ''
  createForm.remark = ''
}

const openCreateModal = () => {
  resetCreateForm()
  createVisible.value = true
}

const submitCreate = async () => {
  if (createForm.bind_mode === 'api' && !createForm.agent_id.trim()) {
    message.error('api 模式下 agent_id 不能为空')
    return
  }

  const payload = {
    bind_mode: createForm.bind_mode,
    name: createForm.name.trim(),
    remark: createForm.remark.trim(),
  }

  if (createForm.bind_mode === 'api') {
    payload.agent_id = createForm.agent_id.trim()
  }
  if (createForm.bind_mode === 'api' && createForm.expires_at.trim()) {
    payload.expires_at = createForm.expires_at.trim()
  }

  createLoading.value = true
  try {
    const res = await createApiToken(payload)
    const data = res?.data?.data || {}
    createVisible.value = false
    Modal.info({
      title: 'Api Token 创建成功',
      content: `请立即保存明文 Token：${data.token || ''}`,
    })
    await loadList()
  } finally {
    createLoading.value = false
  }
}

const handleRotate = async (record) => {
  setRowRotateLoading(record.id, true)
  try {
    const res = await rotateApiToken(record.id)
    const data = res?.data?.data || {}
    Modal.info({
      title: 'Api Token 轮换成功',
      content: `请立即保存新 Token：${data.token || ''}`,
    })
    await loadList()
  } finally {
    setRowRotateLoading(record.id, false)
  }
}

const handleDisable = (record) => {
  Modal.confirm({
    title: '确认禁用 Api Token',
    content: `将禁用 ${record.agent_id}，禁用后无法继续用于 Agent 鉴权。`,
    okText: '确认',
    cancelText: '取消',
    onOk: async () => {
      setRowDisableLoading(record.id, true)
      try {
        await disableApiToken(record.id)
        message.success('Api Token 已禁用')
        await loadList()
      } finally {
        setRowDisableLoading(record.id, false)
      }
    },
  })
}

const handleDelete = async (record) => {
  await openDeleteConfirm({
    title: '确认删除 Api Token',
    summary: '删除后该 Token 将不可恢复，请确认影响范围。',
    items: [`${record.agent_id} (${record.bind_mode})`],
    onConfirm: async () => {
      setRowDeleteLoading(record.id, true)
      try {
        await deleteApiToken(record.id)
        message.success('Api Token 已删除')
        await loadList()
      } finally {
        setRowDeleteLoading(record.id, false)
      }
    },
  })
}

watch(activeMode, (mode) => {
  createForm.bind_mode = mode
  if (mode === 'agent') {
    createForm.agent_id = ''
    createForm.expires_at = ''
  }
})

loadList()
</script>

<style scoped>
.mode-tabs {
  margin-bottom: 8px;
}

.tools {
  margin-bottom: 16px;
}

.tool-item {
  margin-bottom: 8px;
}
</style>
