<template>
  <div class="monitor-media-page">
    <a-card title="媒介" size="small">
      <template #extra>
        <a-button v-if="checkPermission('monitor:create')" size="large" @click="openCreateModal">
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
        :pagination="pagination"
        :locale="tableLocale"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'type'">
            <a-tag :color="record.type === 'email' ? 'blue' : 'green'">{{ record.typeLabel }}</a-tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <EnabledSwitch
              :checked="record.enabled"
              :loading="rowStatusLoading[record.id]"
              :disabled="!checkPermission('monitor:update') || rowStatusLoading[record.id]"
              :aria-label="`${record.name}启用状态`"
              @change="(enabled) => changeMediaEnabled(record, enabled)"
            />
          </template>
          <template v-else-if="column.key === 'operation'">
            <a-row :gutter="6" class="action-row">
              <a-col>
                <a-tooltip title="测试" placement="top">
                  <a-button v-if="checkPermission('monitor:execute')" @click="openTestModal(record)">测试</a-button>
                </a-tooltip>
              </a-col>
              <a-col>
                <a-tooltip title="编辑" placement="top">
                  <a-button v-if="checkPermission('monitor:update')" type="primary" @click="openEditModal(record)">编辑</a-button>
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
                    v-if="checkPermission('monitor:delete')"
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
      <a-tabs v-model:activeKey="mediaTab">
        <a-tab-pane key="settings" tab="报警媒介类型" />
        <a-tab-pane key="templates" tab="消息模板" />
      </a-tabs>
      <a-form v-show="mediaTab === 'settings'"
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
        <template v-if="createForm.type === 'email'">
          <a-form-item label="邮箱提供商">
            <a-select
              v-model:value="createForm.provider"
              :options="emailProviderOptions"
              :getPopupContainer="getPopupContainer"
              @change="handleProviderChange"
            />
          </a-form-item>
          <a-form-item label="电子邮件" required>
            <a-input v-model:value="createForm.email" />
          </a-form-item>
          <a-form-item v-if="createForm.provider === 'custom' && createForm.authType === 'password'" label="用户名" required>
            <a-input v-model:value="createForm.username" placeholder="SMTP 登录用户名" />
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
          <a-form-item v-if="createForm.provider === 'custom'" label="认证方式" required>
            <a-select
              v-model:value="createForm.authType"
              :options="smtpAuthTypeOptions"
              :getPopupContainer="getPopupContainer"
            />
          </a-form-item>
          <a-form-item v-if="createForm.authType === 'password'" label="密码" required>
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
          <EnabledSwitch v-model:checked="createForm.enabled" />
        </a-form-item>
      </a-form>

      <a-form v-if="mediaTab === 'templates'" layout="vertical">
        <a-form-item label="告警事件">
          <a-radio-group v-model:value="templateEvent">
            <a-radio-button value="firing">触发</a-radio-button>
            <a-radio-button value="resolved">恢复</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-alert type="info" show-icon message="主题或正文留空时使用系统默认内容。模板按纯文本编辑。" />
        <a-form-item :label="templateEvent === 'resolved' ? '恢复邮件变量' : '触发邮件变量'">
          <a-space wrap>
            <a-tooltip v-for="[name, description] in templateVariables" :key="name" :title="description">
              <code v-text="`{{${name}}}`" />
            </a-tooltip>
          </a-space>
          <div class="ant-form-item-extra">
            正式邮件按收件用户的个人时区显示时间；样例预览和模板测试使用当前登录用户的时区。
            摘要、描述、环境、时间{{ templateEvent === 'resolved' ? '、持续时长' : '' }}或来源链接不可用时显示「—」。
          </div>
        </a-form-item>
        <a-form-item label="主题">
          <a-input v-model:value="createForm.templates[templateEvent].subject" placeholder="留空使用默认主题" />
        </a-form-item>
        <a-form-item label="正文">
          <a-textarea v-model:value="createForm.templates[templateEvent].body" :rows="6" placeholder="留空使用默认正文" />
        </a-form-item>
        <a-button :loading="previewLoading" @click="loadTemplatePreview">样例预览</a-button>
        <div v-if="templatePreview?.key === templatePreviewKey" class="template-preview">
          <p><strong>主题：</strong>{{ templatePreview.subject }}</p>
          <pre>{{ templatePreview.body }}</pre>
        </div>
      </a-form>

      <div class="modal-actions">
        <a-button @click="closeCreateModal">取消</a-button>
        <a-button v-if="checkPermission(editingId ? 'monitor:update' : 'monitor:create')" type="primary" :loading="createLoading" @click="saveMedia">保存</a-button>
      </div>
    </a-modal>

    <a-modal
      v-model:open="testModalVisible"
      title="测试邮件媒介"
      :confirm-loading="testLoading"
      @ok="sendTestEmail"
      @cancel="closeTestModal"
    >
      <a-form :model="testForm" layout="vertical">
        <a-form-item label="收件人" required>
          <a-input v-model:value="testForm.recipients" placeholder="多个收件人请使用逗号分隔" />
        </a-form-item>
        <a-form-item label="发送内容">
          <a-radio-group v-model:value="testForm.event">
            <a-radio value="">自定义内容</a-radio>
            <a-radio value="firing">已保存的触发模板</a-radio>
            <a-radio value="resolved">已保存的恢复模板</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item v-if="!testForm.event" label="主题" required>
          <a-input v-model:value="testForm.subject" />
        </a-form-item>
        <a-form-item v-if="!testForm.event" label="消息" required>
          <a-textarea v-model:value="testForm.message" :rows="6" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import EnabledSwitch from '@/components/EnabledSwitch.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { checkPermission } from '@/directives/permission/permission'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { useRoute } from 'vue-router'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { batchDeleteAlertMedias, createAlertMedia, getAlertMediaList, previewAlertMediaTemplate, testAlertMedia, updateAlertMedia } from '@/api/monitor'
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

const smtpAuthTypeOptions = [
  { label: '无认证', value: 'none' },
  { label: '用户密码', value: 'password' },
]

const pagination = reactive(createPagination())

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '媒介类型', key: 'type', width: 180 },
  { title: '状态', key: 'status', width: 140 },
  { title: '说明', dataIndex: 'description', key: 'description', width: 300 },
  { title: '操作', key: 'operation', fixed: 'right', width: 180 },
]

const mediaList = ref([])
const listLoading = ref(false)
const createModalVisible = ref(false)
const createLoading = ref(false)
const testModalVisible = ref(false)
const testLoading = ref(false)
const testingMediaId = ref(null)
const editingId = ref(null)
const rowDeleteLoading = reactive({})
const rowStatusLoading = reactive({})
const createForm = ref(createDefaultForm())
const mediaTab = ref('settings')
const templateEvent = ref('firing')
const commonTemplateVariables = [
  ['alert_name', '告警名称'],
  ['severity', '告警级别'],
  ['state', '告警事件状态'],
  ['instance', '告警实例'],
  ['labels', '完整标签（JSON）'],
  ['labels.environment', '环境标签（environment）'],
  ['summary', '告警注解中的摘要'],
  ['description', '告警注解中的详细描述'],
  ['started_at', '触发时间'],
  ['generator_url', '告警来源链接，例如 Prometheus 表达式页面'],
]
const templateVariables = computed(() => templateEvent.value === 'resolved'
  ? [...commonTemplateVariables, ['resolved_at', '恢复时间'], ['duration', '告警持续时长，例如 15m0s']]
  : commonTemplateVariables)
const templatePreview = ref(null)
const previewLoading = ref(false)
let previewRequest = 0
const templatePreviewKey = computed(() => JSON.stringify([
  templateEvent.value, createForm.value.templates[templateEvent.value],
]))
const testForm = ref({
  recipients: '',
  event: '',
  subject: '测试邮件',
  message: '这是一封测试邮件。',
})

function templateDrafts(saved) {
  return Object.fromEntries(['firing', 'resolved'].map((event) => [event, {
    subject: saved?.[event]?.subject ?? '',
    body: saved?.[event]?.body ?? '',
  }]))
}

function resetTemplateEditor() {
  mediaTab.value = 'settings'
  templateEvent.value = 'firing'
  templatePreview.value = null
  previewLoading.value = false
  previewRequest++
}

async function loadTemplatePreview() {
  const request = ++previewRequest
  const key = templatePreviewKey.value
  previewLoading.value = true
  templatePreview.value = null
  try {
    const response = await previewAlertMediaTemplate({
      event: templateEvent.value,
      template: { ...createForm.value.templates[templateEvent.value] },
    })
    if (request === previewRequest && createModalVisible.value && key === templatePreviewKey.value) {
      templatePreview.value = { ...parseApiData(response), key }
    }
  } finally {
    if (request === previewRequest) previewLoading.value = false
  }
}

function createDefaultForm() {
  return {
    name: '',
    type: 'email',
    provider: 'custom',
    smtpServer: '',
    smtpPort: undefined,
    authType: 'none',
    email: '',
    username: '',
    password: '',
    messageFormat: 'html',
    templates: templateDrafts(),
    description: '',
    enabled: true,
  }
}

function handleProviderChange(provider) {
  createForm.value.authType = provider === 'custom' ? 'none' : 'password'
  createForm.value.username = ''
  createForm.value.password = ''
}

function openCreateModal() {
  if (!checkPermission('monitor:create')) return
  editingId.value = null
  createForm.value = createDefaultForm()
  resetTemplateEditor()
  createModalVisible.value = true
}

function openEditModal(record) {
  if (!checkPermission('monitor:update')) return
  const config = record.config || {}
  editingId.value = record.id
  createForm.value = {
    name: record.name,
    type: record.media_type,
    provider: config.provider || 'custom',
    smtpServer: config.smtpServer || '',
    smtpPort: config.smtpPort || undefined,
    authType: config.authType || (config.provider === 'custom' ? 'none' : 'password'),
    email: config.email || '',
    username: config.username || '',
    password: config.password || '',
    messageFormat: config.messageFormat || 'html',
    templates: templateDrafts(config.message_templates),
    description: record.remark || '',
    enabled: record.enabled,
  }
  resetTemplateEditor()
  createModalVisible.value = true
}

function closeCreateModal() {
  createModalVisible.value = false
}

function openTestModal(record) {
  if (!checkPermission('monitor:execute')) return
  testingMediaId.value = record.id
  testForm.value = {
    recipients: '',
    event: '',
    subject: '测试主题',
    message: '这是一封测试邮件。',
  }
  testModalVisible.value = true
}

function closeTestModal() {
  testModalVisible.value = false
}

async function sendTestEmail() {
  if (!checkPermission('monitor:execute')) return
  const recipients = testForm.value.recipients.split(/[,;，；]/).map((item) => item.trim()).filter(Boolean)
  if (!recipients.length) {
    message.warning('请填写收件人')
    return
  }
  const { event, subject, message: body } = testForm.value
  if (!event && (!subject.trim() || !body.trim())) {
    message.warning('请填写主题和消息')
    return
  }
  testLoading.value = true
  try {
    await testAlertMedia(testingMediaId.value, event ? { event, recipients } : { subject, message: body, recipients })
    testModalVisible.value = false
    message.success('测试邮件已发送')
  } finally {
    testLoading.value = false
  }
}

async function saveMedia() {
  if (!checkPermission(editingId.value ? 'monitor:update' : 'monitor:create')) return
  if (!createForm.value.name || !createForm.value.type) {
    mediaTab.value = 'settings'
    message.warning('请填写名称并选择媒介类型')
    return
  }
  if (createForm.value.type === 'email' && !createForm.value.email) {
    mediaTab.value = 'settings'
    message.warning('请填写电子邮件')
    return
  }
  if (createForm.value.type === 'email'
    && createForm.value.provider === 'custom'
    && createForm.value.authType === 'password'
    && (!createForm.value.username || !createForm.value.password)) {
    mediaTab.value = 'settings'
    message.warning('请填写用户名和密码')
    return
  }
  if (createForm.value.type === 'email'
    && createForm.value.provider === 'custom'
    && (!createForm.value.smtpServer || !createForm.value.smtpPort)) {
    mediaTab.value = 'settings'
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
      remark: createForm.value.description,
      config: {
        provider: createForm.value.provider,
        smtpServer: isGmail ? 'smtp.gmail.com' : createForm.value.smtpServer,
        smtpPort: isGmail ? 587 : createForm.value.smtpPort,
        authType: isGmail ? 'password' : createForm.value.authType,
        email: createForm.value.email,
        ...(isGmail ? {} : { username: createForm.value.username }),
        password: createForm.value.password,
        messageFormat: createForm.value.messageFormat,
        message_templates: createForm.value.templates,
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

async function changeMediaEnabled(record, enabled) {
  if (!checkPermission('monitor:update')) {
    message.warning('没有修改媒介状态的权限')
    return
  }
  if (rowStatusLoading[record.id] || record.enabled === enabled) return
  rowStatusLoading[record.id] = true
  try {
    const response = await updateAlertMedia(record.id, { enabled })
    const data = parseApiData(response)
    if (typeof data.enabled !== 'boolean') {
      throw new Error('媒介状态响应无效，请刷新后重试')
    }
    // 服务端确认后按 ID 更新当前列表，避免保存期间刷新使旧行对象失效。
    const currentMedia = mediaList.value.find((item) => item.id === record.id)
    if (currentMedia) currentMedia.enabled = data.enabled
    message.success(data.enabled ? '媒介已启用' : '媒介已停用')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '更新媒介状态失败')
  } finally {
    rowStatusLoading[record.id] = false
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
  if (!checkPermission('monitor:delete')) return
  await openDeleteConfirm({
    title: '确认删除报警媒介',
    summary: '删除后该媒介将不再接收新的告警通知。',
    items: [record.name],
    onConfirm: async () => {
      rowDeleteLoading[record.id] = true
      try {
        await batchDeleteAlertMedias([record.id])
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

.template-preview {
  margin-top: 12px;
  overflow-wrap: anywhere;
}

.template-preview pre {
  white-space: pre-wrap;
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
