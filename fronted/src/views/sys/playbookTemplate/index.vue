<template>
  <div class="playbook-template-page">
    <a-row :gutter="16" class="top-tools">
      <a-col :span="14">
        <a-input-search
          v-model:value="keyword"
          placeholder="搜索模板名称"
          allow-clear
          enter-button
          @search="loadPlaybooks"
        />
      </a-col>
      <a-col :span="10" class="right-tools">
        <a-space>
          <a-button size="large" @click="openPlaybookModal()" v-permission="'automation:playbooks:create'">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp新增模板</span>
          </a-button>
          <a-button type="primary" ghost :loading="playbookLoading" @click="loadPlaybooks">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="playbookLoading" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="Playbook 模板" size="small" class="block-card">
      <a-table
        :columns="playbookColumns"
        :data-source="playbooks"
        :loading="playbookLoading"
        :pagination="playbookPagination"
        rowKey="id"
        size="small"
        @change="handlePlaybookTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'update_time'">
            {{ formatRecentUpdateTime(record.update_time) }}
          </template>
          <template v-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑">
                <a-button size="small" type="primary" @click="openPlaybookModal(record)" v-permission="'automation:playbooks:update'">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="下载">
                <a-button size="small" @click="downloadPlaybook(record)" :loading="downloadingPlaybookId === record.id" v-permission="'automation:playbooks:view'">
                  <FontAwesomeIcon :icon="['fas', 'download']" />
                </a-button>
              </a-tooltip>
              <a-popconfirm
                title="确认删除该模板吗？"
                ok-text="确认"
                cancel-text="取消"
                @confirm="onDeletePlaybook(record)"
              >
                <a-button size="small" type="primary" danger v-permission="'automation:playbooks:delete'">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      :title="playbookEdit.id ? '编辑模板' : '新增模板'"
      :open="playbookModalVisible"
      :confirmLoading="playbookSubmitting"
      :width="840"
      ok-text="保存"
      cancel-text="取消"
      @ok="submitPlaybook"
      @cancel="playbookModalVisible = false"
    >
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="24">
            <a-form-item label="模板名称" required>
              <a-input v-model:value="playbookEdit.name" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="描述">
          <a-input v-model:value="playbookEdit.description" />
        </a-form-item>
        <a-form-item v-if="playbookEdit.id" label="模板文件导入">
          <a-space>
            <a-upload
              accept=".yml,.yaml"
              :show-upload-list="false"
              :custom-request="handleModalPlaybookUpload"
              v-permission="'automation:playbooks:update'"
            >
              <a-button type="primary" ghost :loading="uploadingPlaybookId === playbookEdit.id">
                <UploadOutlined />
                <span>&nbsp;上传 YAML 文件覆盖当前内容</span>
              </a-button>
            </a-upload>
            <span class="upload-tip">上传后会直接回填到当前模板内容</span>
          </a-space>
        </a-form-item>
        <a-form-item label="模板内容（YAML）" required>
          <a-space style="margin-bottom: 8px;">
            <a-button type="primary" ghost :loading="playbookCheckingSyntax" @click="checkPlaybookSyntax">
              <span>检查语法</span>
            </a-button>
            <span class="upload-tip">保存前可手动校验 YAML/Playbook 结构</span>
          </a-space>
          <a-textarea
            v-model:value="playbookEdit.content"
            class="playbook-content-textarea"
            :rows="14"
            placeholder="---&#10;- hosts: all&#10;  gather_facts: false&#10;  tasks:&#10;    - name: ping&#10;      ping:"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { reactive, ref, watch } from 'vue'
import { UploadOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { useRoute } from 'vue-router'
import {
  getPlaybookList,
  createPlaybook,
  updatePlaybook,
  deletePlaybook,
  validatePlaybookContent,
  uploadPlaybookFile,
  downloadPlaybookFile,
} from '@/api/sys/automation'

const route = useRoute()
const keyword = ref('')
const playbooks = ref([])
const playbookLoading = ref(false)
const playbookModalVisible = ref(false)
const playbookSubmitting = ref(false)
const playbookCheckingSyntax = ref(false)
const uploadingPlaybookId = ref(null)
const downloadingPlaybookId = ref(null)
const playbookPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const playbookEdit = reactive({
  id: null,
  name: '',
  description: '',
  content: '',
})

const playbookColumns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '描述', dataIndex: 'description', key: 'description' },
  { title: '最近更新时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
  { title: '操作', key: 'action', width: 220 },
]

function formatRecentUpdateTime(value) {
  const text = String(value || '').trim()
  if (!text) {
    return '-'
  }
  const date = new Date(text)
  if (Number.isNaN(date.getTime())) {
    return text
  }

  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  const second = String(date.getSeconds()).padStart(2, '0')
  return `${year}-${month}-${day} ${hour}:${minute}:${second}`
}

function parseDownloadFilename(contentDisposition, fallbackName) {
  if (!contentDisposition) {
    return fallbackName
  }

  const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch (error) {
      return fallbackName
    }
  }

  const plainMatch = contentDisposition.match(/filename="?([^";]+)"?/i)
  if (plainMatch?.[1]) {
    return plainMatch[1]
  }

  return fallbackName
}

async function checkPlaybookSyntax() {
  if (!playbookEdit.content || !playbookEdit.content.trim()) {
    message.error('请先输入模板内容')
    return
  }
  playbookCheckingSyntax.value = true
  try {
    await validatePlaybookContent({ content: playbookEdit.content })
    message.success('语法检查通过')
  } finally {
    playbookCheckingSyntax.value = false
  }
}

function triggerFileDownload(blob, filename) {
  const url = window.URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  window.URL.revokeObjectURL(url)
}

function syncPlaybookContent(record) {
  const index = playbooks.value.findIndex((item) => item.id === record.id)
  if (index >= 0) {
    playbooks.value[index] = record
  }
  if (playbookEdit.id === record.id) {
    playbookEdit.content = record.content || ''
  }
}

async function loadPlaybooks() {
  playbookLoading.value = true
  try {
    const res = await getPlaybookList({
      page: playbookPagination.current,
      page_size: playbookPagination.pageSize,
      search: keyword.value,
      ordering: '-id',
    })
    const data = res?.data?.data || {}
    playbooks.value = data.results || []
    playbookPagination.total = data.count || 0
  } finally {
    playbookLoading.value = false
  }
}

function resetPlaybookEdit() {
  playbookEdit.id = null
  playbookEdit.name = ''
  playbookEdit.description = ''
  playbookEdit.content = ''
}

function openPlaybookModal(record = null) {
  resetPlaybookEdit()
  if (record) {
    playbookEdit.id = record.id
    playbookEdit.name = record.name || ''
    playbookEdit.description = record.description || ''
    playbookEdit.content = record.content || ''
  }
  playbookModalVisible.value = true
}

async function submitPlaybook() {
  if (!playbookEdit.name || !playbookEdit.content) {
    message.error('名称、模板内容为必填项')
    return
  }

  playbookSubmitting.value = true
  const payload = {
    name: playbookEdit.name,
    description: playbookEdit.description,
    content: playbookEdit.content,
  }

  try {
    if (playbookEdit.id) {
      await updatePlaybook(playbookEdit.id, payload)
      message.success('模板更新成功')
    } else {
      await createPlaybook(payload)
      message.success('模板创建成功')
    }
    playbookModalVisible.value = false
    await loadPlaybooks()
  } finally {
    playbookSubmitting.value = false
  }
}

async function onDeletePlaybook(record) {
  await deletePlaybook(record.id)
  message.success('模板删除成功')
  await loadPlaybooks()
}

async function handlePlaybookUpload(record, options) {
  const file = options?.file
  const filename = file?.name || ''
  const isYaml = /\.(yml|yaml)$/i.test(filename)
  if (!isYaml) {
    message.error('仅支持上传 .yml 或 .yaml 文件')
    options?.onError?.(new Error('invalid file type'))
    return
  }

  const formData = new FormData()
  formData.append('file', file)
  uploadingPlaybookId.value = record.id

  try {
    const res = await uploadPlaybookFile(record.id, formData)
    const updatedRecord = res?.data?.data
    syncPlaybookContent(updatedRecord)
    message.success('模板上传成功')
    options?.onSuccess?.(updatedRecord)
  } catch (error) {
    message.error(error?.message || '模板上传失败')
    options?.onError?.(error)
  } finally {
    uploadingPlaybookId.value = null
  }
}

function handleModalPlaybookUpload(options) {
  if (!playbookEdit.id) {
    message.warning('请先保存模板，再上传 YAML 文件')
    options?.onError?.(new Error('playbook not saved'))
    return
  }
  return handlePlaybookUpload({ id: playbookEdit.id }, options)
}

async function downloadPlaybook(record) {
  downloadingPlaybookId.value = record.id
  try {
    const response = await downloadPlaybookFile(record.id)
    const fallbackName = `${record.name || `playbook-${record.id}`}.yml`
    const filename = parseDownloadFilename(response.headers?.['content-disposition'], fallbackName)
    const blob = response.data instanceof Blob
      ? response.data
      : new Blob([response.data], { type: 'text/yaml;charset=utf-8' })

    triggerFileDownload(blob, filename)
    message.success('模板下载成功')
  } catch (error) {
    message.error(error?.message || '模板下载失败')
  } finally {
    downloadingPlaybookId.value = null
  }
}

function handlePlaybookTableChange(page) {
  playbookPagination.current = page.current
  playbookPagination.pageSize = page.pageSize
  loadPlaybooks()
}

watch(
  () => [route.query.search, route.query.keyword],
  async () => {
    keyword.value = String(route.query.search || route.query.keyword || '').trim()
    playbookPagination.current = 1
    await loadPlaybooks()
  },
  { immediate: true }
)
</script>

<style scoped>
.playbook-template-page {
  padding: 2px;
}

.top-tools {
  margin-bottom: 12px;
}

.right-tools {
  display: flex;
  justify-content: flex-end;
}

.block-card {
  margin-bottom: 12px;
}

.upload-tip {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}

:deep(textarea.playbook-content-textarea.ant-input),
:deep(.playbook-content-textarea.ant-input),
:deep(.playbook-content-textarea textarea.ant-input),
:deep(.playbook-content-textarea .ant-input) {
  background: #141414 !important;
  color: #f5f5f5 !important;
  border-radius: 8px !important;
  border-color: #303030 !important;
  font-family: Menlo, Monaco, Consolas, "Courier New", monospace;
  line-height: 1.5;
}

:deep(textarea.playbook-content-textarea.ant-input::placeholder),
:deep(.playbook-content-textarea textarea.ant-input::placeholder),
:deep(.playbook-content-textarea .ant-input::placeholder) {
  color: rgba(255, 255, 255, 0.45);
}

:deep(textarea.playbook-content-textarea.ant-input:hover),
:deep(textarea.playbook-content-textarea.ant-input:focus),
:deep(.playbook-content-textarea textarea.ant-input:hover),
:deep(.playbook-content-textarea textarea.ant-input:focus),
:deep(.playbook-content-textarea .ant-input:hover),
:deep(.playbook-content-textarea .ant-input:focus) {
  border-color: #1677ff !important;
}
</style>
