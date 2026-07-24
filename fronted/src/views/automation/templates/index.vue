<template>
  <div class="template-page">
    <a-tabs v-model:activeKey="currentType" class="template-type-tabs" @change="handleTypeChange">
      <a-tab-pane key="playbook" tab="Playbook模板" />
      <a-tab-pane key="shell_script" tab="Shell脚本模板" />
    </a-tabs>

    <a-row :gutter="12" class="top-tools">
      <a-col :span="16">
        <a-space>
          <a-input-search
            v-model:value="keyword"
            placeholder="搜索模板名称"
            allow-clear
            enter-button
            @search="loadTemplates(true)"
          />
          <a-select
            v-model:value="categoryFilter"
            :options="categoryFilterOptions"
            style="width: 200px"
            :getPopupContainer="getPopupContainer"
            @change="loadTemplates(true)"
          />
        </a-space>
      </a-col>
      <a-col :span="8" class="right-tools">
        <a-space>
          <a-tooltip title="新增">
            <a-button v-if="canCreateCurrentType" size="large" @click="openTemplateModal()">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp新增模板</span>
            </a-button>
          </a-tooltip>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="loading" @click="loadTemplates(false)">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <a-card :title="currentType === 'playbook' ? 'Playbook模板' : 'Shell脚本模板'" size="small" class="block-card">
      <a-table
        :columns="columns"
        :data-source="rows"
        :loading="loading"
        :pagination="pagination"
        rowKey="id"
        size="small"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'type'">
            <a-tag :color="record.template_type === 'playbook' ? 'blue' : 'gold'">
              {{ record.template_type === 'playbook' ? 'Playbook' : 'Shell脚本' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'category'">
            <a-tooltip v-if="record.category === 'software_package'" title="由监控软件仓库自动管理，如需修改内容请前往对应软件包编辑页">
              <a-tag color="purple">软件包安装/卸载专用</a-tag>
            </a-tooltip>
            <a-tag v-else color="default">通用</a-tag>
          </template>
          <template v-else-if="column.key === 'update_time'">
            <span>{{ record.update_time ? formatTimeWithTimezone(record.update_time, store.state.user?.timezone || 'Asia/Shanghai') : '-' }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip :title="record.category === 'software_package' ? '由监控软件仓库自动管理，请前往对应软件包编辑页修改' : '编辑'">
                <a-button
                  v-if="hasPermission(record.template_type, 'update') && record.category !== 'software_package'"
                  size="small"
                  type="primary"
                  @click="openTemplateModal(record)"
                >
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="下载">
                <a-button
                  v-if="hasPermission(record.template_type, 'view')"
                  size="small"
                  :loading="downloadingTemplateId === record.id"
                  @click="downloadTemplate(record)"
                >
                  <FontAwesomeIcon :icon="['fas', 'download']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="删除">
                <a-button
                  v-if="hasPermission(record.template_type, 'delete') && record.category !== 'software_package'"
                  class="delBtn"
                  size="small"
                  type="primary"
                  danger
                  @click="openDeleteTemplateConfirm(record)"
                >
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-tooltip>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      :title="templateEdit.id ? '编辑模板' : '新增模板'"
      :open="templateModalVisible"
      :confirmLoading="submitting"
      :width="900"
      ok-text="保存"
      cancel-text="取消"
      @ok="submitTemplate"
      @cancel="templateModalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="模板类型" required>
          <a-select v-model:value="templateEdit.template_type" :options="typeOptions" :disabled="!!templateEdit.id" :getPopupContainer="getPopupContainer" />
        </a-form-item>
        <a-form-item label="模板名称" required>
          <a-input v-model:value="templateEdit.name" />
        </a-form-item>
        <a-form-item label="分类" required>
          <a-select v-model:value="templateEdit.category" :options="categoryOptions" :getPopupContainer="getPopupContainer" />
        </a-form-item>
        <a-form-item label="描述">
          <a-input v-model:value="templateEdit.description" />
        </a-form-item>
        <a-form-item v-if="templateEdit.id" :label="templateEdit.template_type === 'playbook' ? '模板文件导入' : '脚本文件导入'">
          <a-space>
            <a-upload
              :accept="templateEdit.template_type === 'playbook' ? '.yml,.yaml' : '.sh'"
              :show-upload-list="false"
              :custom-request="handleTemplateUpload"
            >
              <a-button type="primary" ghost :loading="uploadingTemplateId === templateEdit.id" :disabled="!hasPermission(templateEdit.template_type, 'update')">
                <UploadOutlined />
                <span>&nbsp;上传文件覆盖当前内容</span>
              </a-button>
            </a-upload>
            <span class="upload-tip">{{ templateEdit.template_type === 'playbook' ? '仅支持 .yml/.yaml，上传后将覆盖模板内容。' : '仅支持 .sh，上传后将覆盖脚本内容。' }}</span>
          </a-space>
        </a-form-item>
        <a-form-item :label="templateEdit.template_type === 'playbook' ? '模板内容（YAML）' : '脚本内容'" required>
          <a-space style="margin-bottom: 8px;">
            <a-button
              type="primary"
              ghost
              :loading="templateEdit.template_type === 'playbook' ? playbookCheckingSyntax : shellCheckingSyntax"
              @click="checkTemplateSyntax"
            >
              <span>检查语法</span>
            </a-button>
            <span class="upload-tip">{{ templateEdit.template_type === 'playbook' ? '保存前可手动校验 YAML/Playbook 结构' : '保存前可手动校验 Shell 语法结构' }}</span>
          </a-space>
          <div class="template-editor-wrap">
            <div ref="lineNumberGutterRef" class="template-editor-gutter" aria-hidden="true">
              <div v-for="line in templateLineNumbers" :key="line" class="template-editor-line-number">{{ line }}</div>
            </div>
            <textarea
              v-model="templateEdit.content"
              class="template-editor-textarea"
              rows="14"
              spellcheck="false"
              @scroll="syncTemplateLineNumberScroll"
            />
          </div>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { UploadOutlined } from '@ant-design/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import { formatTimeWithTimezone } from '@/util/timezone'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { checkPermission } from '@/directives/permission/permission'
import store from '@/store'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import {
  createPlaybook,
  createShellScriptTemplate,
  deletePlaybook,
  deleteShellScriptTemplate,
  downloadPlaybookFile,
  downloadShellScriptTemplateFile,
  getPlaybookList,
  getShellScriptTemplateList,
  uploadPlaybookFile,
  uploadShellScriptTemplateFile,
  updatePlaybook,
  updateShellScriptTemplate,
  validatePlaybookContent,
  validateShellScriptContent,
} from '@/api/sys/automation'

const route = useRoute()
const router = useRouter()

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const typeOptions = [
  { label: 'Playbook', value: 'playbook' },
  { label: 'Shell脚本', value: 'shell_script' },
]
// “软件包安装/卸载专用”模板改由监控软件仓库（软件包编辑弹窗）内联编辑安装/卸载内容自动创建/更新，
// 不再支持在本页新建/把已有模板分类改成该值，避免同一份数据出现两个可编辑入口导致心智分裂。
const categoryOptions = [
  { label: '通用', value: 'general' },
]
const typeConfig = {
  playbook: {
    permPrefix: 'automation:playbooks',
    list: getPlaybookList,
    create: createPlaybook,
    update: updatePlaybook,
    remove: deletePlaybook,
    upload: uploadPlaybookFile,
    download: downloadPlaybookFile,
    uploadExts: ['.yml', '.yaml'],
  },
  shell_script: {
    permPrefix: 'automation:shell_scripts',
    list: getShellScriptTemplateList,
    create: createShellScriptTemplate,
    update: updateShellScriptTemplate,
    remove: deleteShellScriptTemplate,
    upload: uploadShellScriptTemplateFile,
    download: downloadShellScriptTemplateFile,
    uploadExts: ['.sh'],
  },
}

const currentType = ref('playbook')
const keyword = ref('')
// 默认只看“通用”模板，避免和监控软件仓库专用的安装/卸载 playbook 混在一起；需要时可切换筛选查看。
const categoryFilter = ref('general')
const rows = ref([])
const loading = ref(false)
const submitting = ref(false)
const playbookCheckingSyntax = ref(false)
const shellCheckingSyntax = ref(false)
const templateModalVisible = ref(false)
const uploadingTemplateId = ref(null)
const downloadingTemplateId = ref(null)
const sortState = reactive({ field: null, order: null })
const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const templateEdit = reactive({
  id: null,
  template_type: 'playbook',
  name: '',
  description: '',
  content: '',
  category: 'general',
})

const lineNumberGutterRef = ref(null)

const columns = [
  { title: '类型', key: 'type', width: 110 },
  { title: '名称', dataIndex: 'name', key: 'name', sorter: true },
  { title: '分类', key: 'category', width: 150 },
  { title: '描述', dataIndex: 'description', key: 'description' },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 180, sorter: true },
  { title: '操作', key: 'action', width: 180 },
]
const categoryFilterOptions = [
  { label: '分类：通用', value: 'general' },
  { label: '分类：软件包安装/卸载专用', value: 'software_package' },
  { label: '分类：全部', value: '' },
]
const canCreateCurrentType = computed(() => hasPermission(currentType.value, 'create'))
const templateLineNumbers = computed(() => {
  const lineCount = String(templateEdit.content || '').split('\n').length
  return Array.from({ length: Math.max(lineCount, 1) }, (_, index) => index + 1)
})

function hasPermission(type, action) {
  const prefix = typeConfig[type]?.permPrefix || typeConfig.playbook.permPrefix
  return checkPermission(`${prefix}:${action}`)
}

function resolveOrdering() {
  if (!sortState.field || !sortState.order) return '-id'
  return `${sortState.order === 'descend' ? '-' : ''}${sortState.field}`
}

function syncTemplateLineNumberScroll(event) {
  if (!lineNumberGutterRef.value) return
  lineNumberGutterRef.value.scrollTop = event.target.scrollTop
}

async function loadTemplates(resetPage = false) {
  if (resetPage) pagination.current = 1
  loading.value = true
  try {
    const params = {
      page: pagination.current,
      page_size: pagination.pageSize,
      search: keyword.value,
      ordering: resolveOrdering(),
    }
    if (categoryFilter.value) params.category = categoryFilter.value
    const res = await typeConfig[currentType.value].list(params)

    const data = res?.data?.data || {}
    const records = Array.isArray(data.results) ? data.results : []
    rows.value = records.map((item) => ({ ...item, template_type: currentType.value }))
    pagination.total = Number(data.count || 0)
  } finally {
    loading.value = false
  }
}

function resetTemplateEdit() {
  templateEdit.id = null
  templateEdit.template_type = currentType.value
  templateEdit.name = ''
  templateEdit.description = ''
  templateEdit.content = ''
  templateEdit.category = 'general'
}

function openTemplateModal(record = null) {
  resetTemplateEdit()
  if (record) {
    templateEdit.id = record.id
    templateEdit.template_type = record.template_type
    templateEdit.name = record.name || ''
    templateEdit.description = record.description || ''
    templateEdit.content = record.content || ''
    templateEdit.category = record.category || 'general'
  }
  templateModalVisible.value = true
}

function parseDownloadFilename(contentDisposition, fallbackName) {
  if (!contentDisposition) return fallbackName
  const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch {
      return fallbackName
    }
  }
  const plainMatch = contentDisposition.match(/filename="?([^";]+)"?/i)
  if (plainMatch?.[1]) return plainMatch[1]
  return fallbackName
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

async function checkTemplateSyntax() {
  const isPlaybook = templateEdit.template_type === 'playbook'
  if (!String(templateEdit.content || '').trim()) {
    message.error(isPlaybook ? '请先输入模板内容' : '请先输入脚本内容')
    return
  }

  if (isPlaybook) {
    playbookCheckingSyntax.value = true
    try {
      await validatePlaybookContent({ content: templateEdit.content })
      message.success('语法检查通过')
    } finally {
      playbookCheckingSyntax.value = false
    }
    return
  }

  shellCheckingSyntax.value = true
  try {
    await validateShellScriptContent({ content: templateEdit.content })
    message.success('语法检查通过')
  } finally {
    shellCheckingSyntax.value = false
  }
}

async function handleTemplateUpload(options) {
  const file = options?.file
  if (!templateEdit.id || !file) {
    options?.onError?.(new Error('invalid upload request'))
    return
  }

  const cfg = typeConfig[templateEdit.template_type]
  const filename = String(file.name || '')
  const lower = filename.toLowerCase()
  const allowed = cfg.uploadExts
  if (!allowed.some((ext) => lower.endsWith(ext))) {
    message.error(`仅支持上传 ${allowed.join(' / ')} 文件`)
    options?.onError?.(new Error('invalid file type'))
    return
  }

  const formData = new FormData()
  formData.append('file', file)
  uploadingTemplateId.value = templateEdit.id
  try {
    const res = await cfg.upload(templateEdit.id, formData)
    const data = res?.data?.data
    templateEdit.content = String(data?.content || '')
    message.success('文件上传成功，内容已更新')
    await loadTemplates(false)
    options?.onSuccess?.(data, file)
  } catch (error) {
    options?.onError?.(error)
    throw error
  } finally {
    uploadingTemplateId.value = null
  }
}

async function downloadTemplate(record) {
  downloadingTemplateId.value = record.id
  try {
    const cfg = typeConfig[record.template_type]
    const res = await cfg.download(record.id)
    const headers = res?.headers || {}
    const contentDisposition = headers['content-disposition'] || headers['Content-Disposition']
    const fallbackExt = record.template_type === 'playbook' ? '.yml' : '.sh'
    const fallbackName = `${record.name || 'template'}${fallbackExt}`
    const filename = parseDownloadFilename(contentDisposition, fallbackName)
    triggerFileDownload(res.data, filename)
  } finally {
    downloadingTemplateId.value = null
  }
}

async function submitTemplate() {
  if (!String(templateEdit.name || '').trim()) {
    message.error('模板名称为必填项')
    return
  }
  if (!String(templateEdit.content || '').trim()) {
    message.error('模板内容为必填项')
    return
  }

  const payload = {
    name: String(templateEdit.name).trim(),
    description: String(templateEdit.description || ''),
    content: templateEdit.content,
    category: templateEdit.category || 'general',
  }

  submitting.value = true
  try {
    try {
      if (templateEdit.template_type === 'playbook') {
        await validatePlaybookContent({ content: templateEdit.content })
      } else {
        await validateShellScriptContent({ content: templateEdit.content })
      }
    } catch (error) {
      return
    }

    const cfg = typeConfig[templateEdit.template_type]
    if (templateEdit.id) await cfg.update(templateEdit.id, payload)
    else await cfg.create(payload)

    message.success(templateEdit.id ? '模板更新成功' : '模板创建成功')
    templateModalVisible.value = false
    await loadTemplates(false)
  } finally {
    submitting.value = false
  }
}

async function onDeleteTemplate(record) {
  await typeConfig[record.template_type].remove(record.id)
  message.success('模板删除成功')
  await loadTemplates(false)
}

function openDeleteTemplateConfirm(record) {
  const name = String(record?.name || '').trim() || `#${record?.id || '-'}`
  openDeleteConfirm({
    title: '确认删除模板',
    summary: '删除后不可恢复，请确认影响清单。',
    items: [`模板: ${name}`],
    onConfirm: () => onDeleteTemplate(record),
  })
}

function handleTableChange(page, _filters, sorter) {
  pagination.current = page.current
  pagination.pageSize = page.pageSize

  const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
  const allowed = ['name', 'update_time']
  if (nextSorter?.field && allowed.includes(nextSorter.field) && nextSorter.order) {
    sortState.field = nextSorter.field
    sortState.order = nextSorter.order
  } else {
    sortState.field = null
    sortState.order = null
  }

  loadTemplates(false)
}

async function handleTypeChange(value) {
  pagination.current = 1
  const search = String(keyword.value || '').trim()
  await router.replace({
    path: '/sys/automation/templates',
    query: { ...(search ? { search } : {}), type: value },
  })
  await loadTemplates(false)
}

onMounted(async () => {
  const queryType = String(route.query.type || '').trim()
  currentType.value = queryType === 'shell_script' ? 'shell_script' : 'playbook'
  keyword.value = String(route.query.search || route.query.keyword || '').trim()
  pagination.current = 1
  await loadTemplates(false)
})
</script>

<style scoped>
.template-page {
  padding: 2px;
}

.template-type-tabs {
  margin-bottom: 8px;
}

:deep(.template-type-tabs .ant-tabs-nav) {
  margin-bottom: 8px;
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
  color: #8c8c8c;
  font-size: 12px;
}

.template-editor-wrap {
  display: flex;
  width: 100%;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  overflow: hidden;
}

.template-editor-gutter {
  width: 54px;
  min-width: 54px;
  max-height: 338px;
  overflow: hidden;
  padding: 12px 8px;
  text-align: right;
  background: #fafafa;
  border-right: 1px solid #f0f0f0;
  color: #8c8c8c;
  font-family: Menlo, Monaco, Consolas, 'Courier New', monospace;
  font-size: 13px;
  line-height: 22px;
  user-select: none;
}

.template-editor-line-number {
  height: 22px;
}

.template-editor-textarea {
  flex: 1;
  max-height: 338px;
  padding: 12px;
  border: none;
  outline: none;
  resize: vertical;
  font-family: Menlo, Monaco, Consolas, 'Courier New', monospace;
  font-size: 13px;
  line-height: 22px;
}

.template-editor-textarea:focus {
  box-shadow: none;
}
</style>
