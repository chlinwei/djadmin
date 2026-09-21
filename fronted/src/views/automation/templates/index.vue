<template>
  <div class="template-page">
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
          <a-tooltip :title="shellcheck.ready ? `ShellCheck 已就绪（${shellcheck.source === 'uploaded' ? '平台上传' : '服务器安装'}${shellcheck.version ? ' v' + shellcheck.version : ''}）` : 'Shell 类模板需要 ShellCheck 校验，点此上传或安装'">
            <a-tag :color="shellcheck.ready ? 'green' : 'red'" class="shellcheck-tag" @click="openShellcheckModal">
              <FontAwesomeIcon :icon="['fas', shellcheck.ready ? 'circle-check' : 'triangle-exclamation']" />
              <span>&nbsp;ShellCheck {{ shellcheck.ready ? '已就绪' : '未就绪' }}</span>
            </a-tag>
          </a-tooltip>
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

    <a-card title="Playbook模板" size="small" class="block-card">
      <a-table
        :columns="columns"
        :data-source="rows"
        :loading="loading"
        :pagination="pagination"
        rowKey="id"
        size="small"
        :locale="tableLocale"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'category'">
            <a-tooltip v-if="record.category === 'software_package'" title="由监控软件仓库自动管理，如需修改内容请前往对应软件包编辑页">
              <a-tag color="purple">软件包安装/卸载专用</a-tag>
            </a-tooltip>
            <a-tooltip v-else-if="record.category === 'agent'" title="Agent 安装/更新专用模板（autoadmin 按分类读取，禁止删除，可编辑内容）">
              <a-tag color="geekblue">Agent 安装专用</a-tag>
            </a-tooltip>
            <a-tag v-else color="default">通用</a-tag>
          </template>
          <template v-else-if="column.key === 'content_format'">
            <a-tag :color="record.content_format === 'shell' ? 'orange' : 'blue'">
              {{ record.content_format === 'shell' ? 'Shell 脚本' : 'Playbook' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'update_time'">
            <span>{{ record.update_time ? formatTimeWithTimezone(record.update_time, store.state.user?.timezone || 'Asia/Shanghai') : '-' }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip :title="record.category === 'software_package' ? '由监控软件仓库自动管理，请前往对应软件包编辑页修改' : '编辑'">
                <a-button
                  v-if="hasPermission('update') && record.category !== 'software_package'"
                  size="small"
                  type="primary"
                  @click="openTemplateModal(record)"
                >
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="下载">
                <a-button
                  v-if="hasPermission('view')"
                  size="small"
                  :loading="downloadingTemplateId === record.id"
                  @click="downloadTemplate(record)"
                >
                  <FontAwesomeIcon :icon="['fas', 'download']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="删除">
                <a-button
                  v-if="hasPermission('delete') && !['software_package', 'agent'].includes(record.category)"
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
        <a-form-item label="模板名称" required>
          <a-input v-model:value="templateEdit.name" />
        </a-form-item>
        <a-form-item label="分类" required>
          <a-select v-model:value="templateEdit.category" :options="categoryOptions" :disabled="isAgentTemplate" :getPopupContainer="getPopupContainer" />
        </a-form-item>
        <a-form-item label="类型" required>
          <a-segmented v-model:value="templateEdit.content_format" :options="contentFormatOptions" block @change="handleContentFormatChange" />
          <div class="upload-tip">
            Playbook 存标准 YAML；Shell 脚本只写裸脚本，执行时由平台包装成单任务 playbook（以 root 运行，任务的 run_as 会切换用户）。
          </div>
        </a-form-item>
        <a-form-item label="描述">
          <a-input v-model:value="templateEdit.description" />
        </a-form-item>
        <a-form-item v-if="templateEdit.id && !isShellTemplate" label="模板文件导入">
          <a-space>
            <a-upload
              accept=".yml,.yaml"
              :show-upload-list="false"
              :custom-request="handleTemplateUpload"
            >
              <a-button type="primary" ghost :loading="uploadingTemplateId === templateEdit.id" :disabled="!hasPermission('update')">
                <UploadOutlined />
                <span>&nbsp;上传文件覆盖当前内容</span>
              </a-button>
            </a-upload>
            <span class="upload-tip">仅支持 .yml/.yaml，上传后将覆盖模板内容。</span>
          </a-space>
        </a-form-item>
        <a-form-item :label="isShellTemplate ? '脚本内容（Bash）' : '模板内容（YAML）'" required>
          <a-space style="margin-bottom: 8px;">
            <a-button
              type="primary"
              ghost
              :loading="playbookCheckingSyntax"
              @click="checkTemplateSyntax"
            >
              <span>检查语法</span>
            </a-button>
            <a-button v-if="isShellTemplate" type="link" @click="templateEdit.content = SHELL_EXAMPLE">填入内存检查示例</a-button>
            <span class="upload-tip">{{ isShellTemplate ? '保存前用 ShellCheck 校验脚本（需组件已就绪）' : '保存前可手动校验 YAML/Playbook 结构' }}</span>
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

    <!-- ShellCheck 组件管理：Shell 类模板校验依赖它。两种就绪方式——平台上传（离线内网）
         或服务器 yum/apt 安装（PATH）。上传的二进制存在服务器 <media>/shellcheck/shellcheck。 -->
    <a-modal
      v-model:open="shellcheckModalVisible"
      title="ShellCheck 组件"
      :footer="null"
      :width="620"
      centered
    >
      <a-descriptions :column="1" size="small" bordered>
        <a-descriptions-item label="状态">
          <a-tag :color="shellcheck.ready ? 'green' : 'red'">{{ shellcheck.ready ? '已就绪' : '未就绪' }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="来源">
          {{ shellcheck.ready ? (shellcheck.source === 'uploaded' ? '平台上传' : '服务器 PATH 安装') : '-' }}
        </a-descriptions-item>
        <a-descriptions-item label="版本">{{ shellcheck.version || '-' }}</a-descriptions-item>
      </a-descriptions>
      <div class="shellcheck-help">
        <p>Shell 类模板用 ShellCheck 做语法与静态检查，服务器需要满足其一：</p>
        <p>1）在下方上传对应的 shellcheck 静态二进制（离线内网推荐，架构需与服务器一致，通常 x86_64）；</p>
        <p>2）在服务器执行 <code>yum install shellcheck</code> 或 <code>apt install shellcheck</code>，然后点刷新。</p>
      </div>
      <a-space>
        <a-upload :show-upload-list="false" :custom-request="handleShellcheckUpload" :disabled="!hasPermission('update')">
          <a-button type="primary" :loading="shellcheckUploading">
            <UploadOutlined />
            <span>&nbsp;{{ shellcheck.ready && shellcheck.source === 'uploaded' ? '重新上传' : '上传 shellcheck 二进制' }}</span>
          </a-button>
        </a-upload>
        <a-button @click="loadShellcheckStatus" :loading="shellcheck.loading">刷新状态</a-button>
        <a-popconfirm
          v-if="shellcheck.ready && shellcheck.source === 'uploaded'"
          title="删除已上传的 ShellCheck 二进制？删除后将回退到服务器 PATH 安装。"
          ok-text="删除"
          cancel-text="取消"
          @confirm="removeShellcheckBinary"
        >
          <a-button danger :disabled="!hasPermission('update')">删除上传的二进制</a-button>
        </a-popconfirm>
      </a-space>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { message } from 'ant-design-vue'
import { UploadOutlined } from '@ant-design/icons-vue'
import { useRoute } from 'vue-router'
import { formatTimeWithTimezone } from '@/util/timezone'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { checkPermission } from '@/directives/permission/permission'
import store from '@/store'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import {
  batchDeletePlaybooks,
  createPlaybook,
  deleteShellcheckBinary,
  downloadPlaybookFile,
  getPlaybookList,
  getShellcheckStatus,
  uploadPlaybookFile,
  uploadShellcheckBinary,
  updatePlaybook,
  validatePlaybookContent,
} from '@/api/sys/automation'

const route = useRoute()

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

// “软件包安装/卸载专用”模板改由监控软件仓库（软件包编辑弹窗）内联编辑安装/卸载内容自动创建/更新，
// 不再支持在本页新建/把已有模板分类改成该值，避免同一份数据出现两个可编辑入口导致心智分裂。
const categoryOptions = [
  { label: '通用', value: 'general' },
]
const playbookConfig = {
    permPrefix: 'automation:playbooks',
    list: getPlaybookList,
    create: createPlaybook,
    update: updatePlaybook,
    remove: (id) => batchDeletePlaybooks([id]),
    upload: uploadPlaybookFile,
    download: downloadPlaybookFile,
    uploadExts: ['.yml', '.yaml'],
}

const keyword = ref('')
// 默认只看“通用”模板，避免和监控软件仓库专用的安装/卸载 playbook 混在一起；需要时可切换筛选查看。
const categoryFilter = ref('general')
const rows = ref([])
const loading = ref(false)
const submitting = ref(false)
const playbookCheckingSyntax = ref(false)
const templateModalVisible = ref(false)
const uploadingTemplateId = ref(null)
const downloadingTemplateId = ref(null)
const sortState = reactive({ field: null, order: null })
const pagination = reactive(createPagination())

const templateEdit = reactive({
  id: null,
  name: '',
  description: '',
  content: '',
  content_format: 'playbook',
  category: 'general',
})

// ---- ShellCheck 组件状态与上传（仅 Shell 类模板需要）----
const shellcheck = reactive({ ready: false, source: '', version: '', loading: false })
const shellcheckModalVisible = ref(false)
const shellcheckUploading = ref(false)

// Shell 脚本示例：内存使用率检查（超过阈值以非零退出码失败，任务结果即可判定）。
// 用 awk 只取一次 free 输出并计算，避免依赖 bc/其他外部命令。
const SHELL_EXAMPLE = `#!/bin/bash
# 内存使用率检查：超过阈值（默认 90%）退出码非 0，任务即判定失败。
THRESHOLD=\${THRESHOLD:-90}
total=\$(awk '/^MemTotal:/{print \$2}' /proc/meminfo)
available=\$(awk '/^MemAvailable:/{print \$2}' /proc/meminfo)
used_pct=\$(( (total - available) * 100 / total ))
echo "memory used: \${used_pct}% (threshold \${THRESHOLD}%)"
if [ "\$used_pct" -ge "\$THRESHOLD" ]; then
  echo "memory usage exceeds threshold"
  exit 1
fi
exit 0
`

async function loadShellcheckStatus() {
  shellcheck.loading = true
  try {
    const res = await getShellcheckStatus()
    const data = res?.data?.data || {}
    shellcheck.ready = Boolean(data.ready)
    shellcheck.source = data.source || ''
    shellcheck.version = data.version || ''
  } catch {
    shellcheck.ready = false
  } finally {
    shellcheck.loading = false
  }
}

function openShellcheckModal() {
  shellcheckModalVisible.value = true
  loadShellcheckStatus()
}

async function handleShellcheckUpload(options) {
  const file = options?.file
  if (!file) return
  const formData = new FormData()
  formData.append('file', file)
  shellcheckUploading.value = true
  try {
    const res = await uploadShellcheckBinary(formData)
    const data = res?.data?.data || {}
    shellcheck.ready = Boolean(data.ready)
    shellcheck.source = data.source || ''
    shellcheck.version = data.version || ''
    message.success('ShellCheck 已上传并启用')
    options?.onSuccess?.(data, file)
  } catch (error) {
    options?.onError?.(error)
    throw error
  } finally {
    shellcheckUploading.value = false
  }
}

async function removeShellcheckBinary() {
  await deleteShellcheckBinary()
  message.success('已删除上传的 ShellCheck 二进制')
  await loadShellcheckStatus()
}

// 校验结果的统一提示：warnings 是 shellcheck 的非 error 级告警（可保存但值得提示）。
function reportValidationWarnings(warnings) {
  const list = Array.isArray(warnings) ? warnings.filter(Boolean) : []
  if (!list.length) return
  const brief = list.slice(0, 3).map((item) => `第 ${item.line} 行 SC${item.code}: ${item.message}`).join('；')
  const suffix = list.length > 3 ? ` 等 ${list.length} 条` : ''
  message.warning(`脚本可保存，但存在 ${list.length} 条 ShellCheck 提示：${brief}${suffix}`, 8)
}

const lineNumberGutterRef = ref(null)

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', sorter: true },
  { title: '类型', key: 'content_format', width: 110 },
  { title: '分类', key: 'category', width: 150 },
  { title: '描述', dataIndex: 'description', key: 'description' },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 180, sorter: true },
  { title: '操作', key: 'action', width: 180 },
]
const categoryFilterOptions = [
  { label: '分类：通用', value: 'general' },
  { label: '分类：软件包安装/卸载专用', value: 'software_package' },
  { label: '分类：Agent 安装专用', value: 'agent' },
  { label: '分类：全部', value: '' },
]
const canCreateCurrentType = computed(() => hasPermission('create'))
// Agent 安装专用模板的分类锁定：它是 autoadmin 安装/更新 dj-agent 的唯一配置源（按分类定位），只允许改内容。
const isAgentTemplate = computed(() => templateEdit.category === 'agent')
const isShellTemplate = computed(() => templateEdit.content_format === 'shell')
const contentFormatOptions = [
  { label: 'Playbook YAML', value: 'playbook' },
  { label: 'Shell 脚本', value: 'shell' },
]
const templateLineNumbers = computed(() => {
  const lineCount = String(templateEdit.content || '').split('\n').length
  return Array.from({ length: Math.max(lineCount, 1) }, (_, index) => index + 1)
})

function hasPermission(action) {
  return checkPermission(`${playbookConfig.permPrefix}:${action}`)
}

function resolveTemplateOrdering() {
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
      ordering: resolveTemplateOrdering(),
    }
    if (categoryFilter.value) params.category = categoryFilter.value
    const res = await playbookConfig.list(params)

    const data = res?.data?.data || {}
    const records = Array.isArray(data.results) ? data.results : []
    rows.value = records
    pagination.total = Number(data.count || 0)
  } finally {
    loading.value = false
  }
}

function resetTemplateEdit() {
  templateEdit.id = null
  templateEdit.name = ''
  templateEdit.description = ''
  templateEdit.content = ''
  templateEdit.content_format = 'playbook'
  templateEdit.category = 'general'
}

function openTemplateModal(record = null) {
  resetTemplateEdit()
  if (record) {
    templateEdit.id = record.id
    templateEdit.name = record.name || ''
    templateEdit.description = record.description || ''
    templateEdit.content = record.content || ''
    templateEdit.content_format = record.content_format || 'playbook'
    templateEdit.category = record.category || 'general'
  }
  templateModalVisible.value = true
  if (templateEdit.content_format === 'shell') loadShellcheckStatus()
}

// 切换到 Shell 时若内容为空，自动填入内存检查示例，降低上手成本。
function handleContentFormatChange(value) {
  if (value === 'shell') {
    if (!String(templateEdit.content || '').trim()) templateEdit.content = SHELL_EXAMPLE
    loadShellcheckStatus()
  }
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
  if (!String(templateEdit.content || '').trim()) {
    message.error('请先输入模板内容')
    return
  }
  if (isShellTemplate.value && !shellcheck.ready) {
    message.warning('服务器未就绪 ShellCheck，请先上传二进制或安装后再校验')
    openShellcheckModal()
    return
  }

  playbookCheckingSyntax.value = true
  try {
    const res = await validatePlaybookContent({
      content: templateEdit.content,
      content_format: templateEdit.content_format,
    })
    reportValidationWarnings(res?.data?.data?.warnings)
    message.success('语法检查通过')
  } finally {
    playbookCheckingSyntax.value = false
  }
}

async function handleTemplateUpload(options) {
  const file = options?.file
  if (!templateEdit.id || !file) {
    options?.onError?.(new Error('invalid upload request'))
    return
  }

  const cfg = playbookConfig
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
    const res = await playbookConfig.download(record.id)
    const headers = res?.headers || {}
    const contentDisposition = headers['content-disposition'] || headers['Content-Disposition']
    const fallbackName = `${record.name || 'template'}.yml`
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
    content_format: templateEdit.content_format,
    category: templateEdit.category || 'general',
  }

  if (isShellTemplate.value && !shellcheck.ready) {
    message.warning('服务器未就绪 ShellCheck，请先上传二进制或安装后再保存')
    openShellcheckModal()
    return
  }

  submitting.value = true
  try {
    try {
      const res = await validatePlaybookContent({
        content: templateEdit.content,
        content_format: templateEdit.content_format,
      })
      reportValidationWarnings(res?.data?.data?.warnings)
    } catch (error) {
      return
    }

    if (templateEdit.id) await playbookConfig.update(templateEdit.id, payload)
    else await playbookConfig.create(payload)

    message.success(templateEdit.id ? '模板更新成功' : '模板创建成功')
    templateModalVisible.value = false
    await loadTemplates(false)
  } finally {
    submitting.value = false
  }
}

async function onDeleteTemplate(record) {
  await playbookConfig.remove(record.id)
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
  const allowedFields = ['name', 'update_time']
  if (nextSorter?.field && allowedFields.includes(nextSorter.field) && nextSorter.order) {
    sortState.field = nextSorter.field
    sortState.order = nextSorter.order
  } else {
    sortState.field = null
    sortState.order = null
  }

  loadTemplates(false)
}

onMounted(async () => {
  keyword.value = String(route.query.search || route.query.keyword || '').trim()
  pagination.current = 1
  await Promise.all([loadTemplates(false), loadShellcheckStatus()])
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

.shellcheck-tag {
  cursor: pointer;
  user-select: none;
  padding: 4px 10px;
  font-size: 13px;
}

.shellcheck-help {
  margin: 12px 0;
  color: #595959;
  font-size: 13px;
  line-height: 22px;
}

.shellcheck-help p {
  margin: 0;
}

.shellcheck-help code {
  padding: 1px 5px;
  background: #f5f5f5;
  border-radius: 4px;
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
