<template>
  <div class="monitor-media-page">
    <a-card title="媒介" size="small">
      <template #extra>
        <a-button size="large" @click="openCreateModal">
          <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
          <span>&nbsp;新增</span>
        </a-button>
      </template>

      <a-table
        rowKey="id"
        :columns="columns"
        :data-source="mediaList"
        :loading="listLoading"
        size="small"
        :scroll="{ x: 1100 }"
        :pagination="{ showSizeChanger: true, showQuickJumper: true }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'type'">
            <a-tag :color="record.type === 'email' ? 'blue' : 'green'">{{ record.typeLabel }}</a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '已启用' : '已停用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'operation'">
            <a-row :gutter="6" class="action-row">
              <a-col>
                <a-tooltip title="编辑" placement="top">
                  <a-button type="primary" @click="openEditModal(record)">编辑</a-button>
                </a-tooltip>
              </a-col>
              <a-col>
                <a-tooltip title="删除" placement="top">
                  <a-button
                    class="delBtn"
                    danger
                    type="primary"
                    :loading="rowDeleteLoading[record.id]"
                    @click="handleDelete(record)"
                  >删除</a-button>
                </a-tooltip>
              </a-col>
            </a-row>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="createModalVisible"
      :title="editingId ? '编辑报警媒介类型' : '新增报警媒介类型'"
      width="720px"
      :footer="null"
      cancel-text="取消"
      :confirm-loading="createLoading"
      @cancel="closeCreateModal"
    >
      <div class="media-tabs">
        <span class="media-tab active">报警媒介类型</span>
        <span class="media-tab">消息模板</span>
        <span class="media-tab">选项</span>
      </div>

      <a-form
        class="media-form"
        :model="createForm"
        :label-col="{ span: 5 }"
        :wrapper-col="{ span: 15 }"
      >
        <a-form-item label="名称" required>
          <a-input v-model:value="createForm.name" />
        </a-form-item>
        <a-form-item label="类型" required>
          <a-select
            v-model:value="createForm.type"
            :options="mediaTypeOptions"
            :getPopupContainer="getPopupContainer"
          />
        </a-form-item>
        <a-form-item label="静态收件邮箱">
          <a-select
            v-model:value="createForm.recipientEmails"
            mode="tags"
            :getPopupContainer="getPopupContainer"
            placeholder="输入邮箱后回车，可填写多个"
          />
        </a-form-item>
        <template v-if="createForm.type === 'email'">
          <a-form-item label="邮箱提供商">
            <a-select
              v-model:value="createForm.provider"
              :options="emailProviderOptions"
              :getPopupContainer="getPopupContainer"
            />
          </a-form-item>
          <template v-if="createForm.provider === 'custom'">
            <a-form-item label="SMTP服务器" required>
              <a-input v-model:value="createForm.smtpServer" placeholder="例如 smtp.example.com" />
            </a-form-item>
            <a-form-item label="SMTP服务器端口" required>
              <a-input-number
                v-model:value="createForm.smtpPort"
                :min="1"
                :max="65535"
                style="width: 100%"
                placeholder="例如 587"
              />
            </a-form-item>
          </template>
          <a-form-item label="电子邮件" required>
            <a-input v-model:value="createForm.email" />
          </a-form-item>
          <a-form-item label="密码" required>
            <a-input-password v-model:value="createForm.password" />
          </a-form-item>
          <a-form-item label="消息格式">
            <a-radio-group v-model:value="createForm.messageFormat" button-style="solid">
              <a-radio-button value="html">HTML</a-radio-button>
              <a-radio-button value="text">文本</a-radio-button>
            </a-radio-group>
          </a-form-item>
        </template>
        <a-form-item label="描述">
          <a-textarea v-model:value="createForm.description" :rows="4" />
        </a-form-item>
        <a-form-item label="已启用">
          <a-checkbox v-model:checked="createForm.enabled" />
        </a-form-item>
      </a-form>

      <div class="modal-actions">
        <a-button @click="closeCreateModal">取消</a-button>
        <a-button type="primary" :loading="createLoading" @click="saveMedia">保存</a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { createAlertMedia, deleteAlertMedia, getAlertMediaList, updateAlertMedia } from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const route = useRoute()

const mediaTypeOptions = [
  { label: 'Email', value: 'email' },
]

const emailProviderOptions = [
  { label: 'Gmail', value: 'gmail' },
  { label: '自定义 SMTP', value: 'custom' },
]

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '媒介类型', key: 'type', width: 180 },
  { title: '配置状态', key: 'status', width: 140 },
  { title: '说明', dataIndex: 'description', key: 'description', width: 300 },
  { title: '操作', key: 'operation', fixed: 'right', width: 180 },
]

const mediaList = ref([])
const listLoading = ref(false)
const createModalVisible = ref(false)
const createLoading = ref(false)
const editingId = ref(null)
const rowDeleteLoading = reactive({})
const createForm = ref(createDefaultForm())

function createDefaultForm() {
  return {
    name: '',
    type: 'email',
    provider: 'gmail',
    smtpServer: '',
    smtpPort: undefined,
    email: '',
    password: '',
    messageFormat: 'html',
    description: '',
    enabled: true,
    recipientEmails: [],
  }
}

function openCreateModal() {
  editingId.value = null
  createForm.value = createDefaultForm()
  createModalVisible.value = true
}

function openEditModal(record) {
  const config = record.config || {}
  editingId.value = record.id
  createForm.value = {
    name: record.name,
    type: record.media_type,
    provider: config.provider || 'custom',
    smtpServer: config.smtpServer || '',
    smtpPort: config.smtpPort || undefined,
    email: config.email || '',
    password: config.password || '',
    messageFormat: config.messageFormat || 'html',
    description: record.remark || '',
    enabled: record.enabled,
    recipientEmails: record.recipient_emails || [],
  }
  createModalVisible.value = true
}

function closeCreateModal() {
  createModalVisible.value = false
}

async function saveMedia() {
  if (!createForm.value.name || !createForm.value.type) {
    message.warning('请填写名称并选择媒介类型')
    return
  }
  if (createForm.value.type === 'email' && (!createForm.value.email || !createForm.value.password)) {
    message.warning('请填写电子邮件和密码')
    return
  }
  if (createForm.value.type === 'email'
    && createForm.value.provider === 'custom'
    && (!createForm.value.smtpServer || !createForm.value.smtpPort)) {
    message.warning('请填写 SMTP 服务器和端口')
    return
  }

  createLoading.value = true
  try {
    const isGmail = createForm.value.provider === 'gmail'
    const payload = {
      name: createForm.value.name,
      media_type: createForm.value.type,
      enabled: createForm.value.enabled,
      recipient_emails: createForm.value.recipientEmails,
      remark: createForm.value.description,
      config: {
        provider: createForm.value.provider,
        smtpServer: isGmail ? 'smtp.gmail.com' : createForm.value.smtpServer,
        smtpPort: isGmail ? 587 : createForm.value.smtpPort,
        email: createForm.value.email,
        password: createForm.value.password,
        messageFormat: createForm.value.messageFormat,
      },
    }
    if (editingId.value) {
      await updateAlertMedia(editingId.value, payload)
    } else {
      await createAlertMedia(payload)
    }
    createModalVisible.value = false
    message.success(editingId.value ? '媒介已更新' : '媒介已添加')
    await loadMedia()
  } finally {
    createLoading.value = false
  }
}

function parseApiData(response) {
  return response?.data?.data || {}
}

async function loadMedia() {
  listLoading.value = true
  try {
    const response = await getAlertMediaList({ page: 1, pageSize: 100 })
    const data = parseApiData(response)
    mediaList.value = (data.results || data || []).map((item) => ({
      ...item,
      type: item.media_type,
      typeLabel: mediaTypeOptions.find((option) => option.value === item.media_type)?.label || item.media_type,
      description: item.remark || '-',
    }))
  } finally {
    listLoading.value = false
  }
}

async function handleDelete(record) {
  await openDeleteConfirm({
    title: '确认删除报警媒介',
    summary: '删除后该媒介将不再接收新的告警通知。',
    items: [record.name],
    onConfirm: async () => {
      rowDeleteLoading[record.id] = true
      try {
        await deleteAlertMedia(record.id)
        message.success('媒介已删除')
        await loadMedia()
      } finally {
        rowDeleteLoading[record.id] = false
      }
    },
  })
}

onMounted(async () => {
  try {
    await loadMedia()
    if (String(route.query.create || '') === '1') {
      openCreateModal()
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || '媒介数据加载失败')
  }
})
</script>

<style scoped>
.monitor-media-page {
  padding: 8px;
}

.media-tabs {
  display: flex;
  gap: 24px;
  margin: -8px 0 18px;
  border-bottom: 1px solid #e5e7eb;
}

.media-tab {
  padding: 0 10px 10px;
  color: #1677ff;
  font-size: 13px;
}

.media-tab.active {
  border-bottom: 3px solid #1677ff;
  color: #111827;
}

.media-form :deep(.ant-form-item) {
  margin-bottom: 14px;
}

.action-row {
  flex-wrap: nowrap;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 22px;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
}
</style>
