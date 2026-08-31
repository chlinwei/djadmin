<template>
  <div class="log-parser-page">
    <div class="page-title-row">
      <h2>日志处理规则</h2>
    </div>

    <div class="parser-layout">
      <div class="application-pane">
        <div class="pane-title">应用</div>
        <a-menu
          class="application-menu"
          mode="inline"
          :selected-keys="[selectedApplicationKey]"
          @select="({ key }) => (selectedApplicationKey = key)"
        >
          <a-menu-item v-for="item in applicationGroups" :key="item.key">
            <span class="application-item">
              <span class="application-label" :title="item.label">{{ item.label }}</span>
              <a-badge
                :count="item.count"
                :number-style="{ backgroundColor: item.count ? '#1677ff' : '#bfbfbf' }"
                :show-zero="true"
              />
            </span>
          </a-menu-item>
        </a-menu>
      </div>

      <div class="rule-pane">
        <a-tabs v-model:active-key="catalogTab">
          <a-tab-pane key="processing" tab="解析规则">
            <div class="toolbar">
          <a-button size="large" :disabled="!selectedClusterId" @click="openCreate">
            <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
            <span>&nbsp;新增规则</span>
          </a-button>
          <a-tooltip title="刷新">
            <a-button size="large" :disabled="!selectedClusterId" @click="loadRules">
              <FontAwesomeIcon :icon="['fas', 'rotate']" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </div>

        <a-table
          row-key="name"
          :columns="columns"
          :data-source="visibleRules"
          :loading="loading"
          :pagination="false"
          :scroll="{ x: 1150 }"
          :locale="{ emptyText: selectedClusterId ? '该应用暂无解析规则' : '尚未配置日志存储集群，请先在「日志存储集群」页面添加' }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'name'">
              <span class="pipeline-name">{{ record.name }}</span>
            </template>
            <template v-else-if="column.key === 'description'">
              <span>{{ record.description || '-' }}</span>
            </template>
            <template v-else-if="column.key === 'input_format'">
              <a-tag :color="record.input_format === 'json' ? 'cyan' : 'default'">
                {{ record.input_format === 'json' ? 'JSON' : '文本' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'multiline'">
              <a-tag :color="record.multiline_enabled ? 'blue' : 'default'">
                {{ record.multiline_enabled ? '多行' : '单行' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'processors'">
              <a-tag color="green">{{ record.pipeline_body?.processors?.length || 0 }}</a-tag>
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space :size="6">
                <a-tooltip title="编辑">
                  <a-button type="primary" @click="openEdit(record)">
                    <FontAwesomeIcon :icon="['fa', 'edit']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="运行">
                  <a-button @click="openTest(record)">
                    <FontAwesomeIcon :icon="['fas', 'play']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除">
                  <a-button class="delBtn" danger type="primary" @click="confirmDelete(record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
          </a-tab-pane>
          <a-tab-pane key="filter" tab="采集过滤规则">
            <div class="toolbar">
              <a-button size="large" @click="openFilterCreate"><FontAwesomeIcon :icon="['fas', 'plus-circle']" /><span>&nbsp;新增过滤规则</span></a-button>
              <a-tooltip title="刷新"><a-button size="large" @click="loadFilterRules"><FontAwesomeIcon :icon="['fas', 'rotate']" /><span>&nbsp;刷新</span></a-button></a-tooltip>
            </div>
            <a-table row-key="id" :columns="filterColumns" :data-source="visibleFilterRules" :loading="filterLoading" :pagination="false" :scroll="{ x: 1000 }" :locale="{ emptyText: '当前应用暂无采集过滤规则' }">
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'name'"><span class="pipeline-name">{{ record.name }}</span></template>
                <template v-else-if="column.key === 'description'"><span>{{ record.description || '-' }}</span></template>
                <template v-else-if="column.key === 'pattern'"><code>{{ record.pattern }}</code></template>
                <template v-else-if="column.key === 'enabled'"><a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag></template>
                <template v-else-if="column.key === 'action'"><a-space :size="6"><a-tooltip title="编辑"><a-button type="primary" @click="openFilterEdit(record)"><FontAwesomeIcon :icon="['fa', 'edit']" /></a-button></a-tooltip><a-tooltip title="删除"><a-button class="delBtn" danger type="primary" @click="confirmDeleteFilter(record)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip></a-space></template>
              </template>
            </a-table>
          </a-tab-pane>
        </a-tabs>
      </div>
    </div>

    <a-modal
      v-model:open="editorOpen"
      :title="editingOriginalName ? `编辑处理规则：${editingOriginalName}` : '新增日志处理规则'"
      :width="960"
      centered
      :confirm-loading="saving"
      ok-text="发布"
      cancel-text="取消"
      @ok="publishPipeline"
    >
      <a-tabs v-model:activeKey="activeTab">
        <a-tab-pane key="preprocess" tab="发送前处理（Fluent Bit）">
          <a-form ref="formRef" :model="form" :rules="formRules" layout="vertical">
            <a-form-item name="name" label="规则名称">
              <a-input
                v-model:value="form.name"
                :disabled="Boolean(editingOriginalName)"
                placeholder="例如 logs-tomcat-access"
              />
            </a-form-item>
            <a-form-item label="说明">
              <a-input v-model:value="form.description" placeholder="描述该规则适用的日志格式" />
            </a-form-item>
            <a-form-item name="application" label="所属应用">
              <a-select
                v-model:value="form.application"
                allow-clear
                placeholder="留空表示不限应用的通用规则"
                :options="applicationOptions"
                :getPopupContainer="getPopupContainer"
              />
            </a-form-item>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item name="input_format" label="日志格式">
                  <a-segmented v-model:value="form.input_format" :options="inputFormatOptions" block />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="多行合并">
                  <a-switch v-model:checked="form.multiline_enabled" />
                </a-form-item>
              </a-col>
            </a-row>
            <template v-if="form.multiline_enabled">
              <a-row :gutter="16">
                <a-col :span="12">
                  <a-form-item name="start_pattern" label="首行正则">
                    <a-input v-model:value="form.start_pattern" placeholder="例如 ^\d{4}-\d{2}-\d{2}" />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item name="continuation_pattern" label="续行正则">
                    <a-input v-model:value="form.continuation_pattern" placeholder="例如 ^(?!\d{4}-\d{2}-\d{2})" />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item name="flush_timeout" label="合并超时（毫秒）">
                    <a-input-number v-model:value="form.flush_timeout" :min="100" :max="60000" style="width: 100%" />
                  </a-form-item>
                </a-col>
              </a-row>
            </template>
          </a-form>
        </a-tab-pane>
        <a-tab-pane key="ingest" tab="字段解析（OpenSearch Ingest）">
          <a-form layout="vertical">
            <a-form-item label="Pipeline JSON">
              <a-textarea
                v-model:value="form.bodyText"
                class="json-editor"
                :rows="18"
                spellcheck="false"
              />
            </a-form-item>
          </a-form>
        </a-tab-pane>
        <a-tab-pane key="test" tab="在线调试">
          <a-form layout="vertical">
            <a-form-item label="样例格式">
              <a-segmented v-model:value="sampleMode" :options="sampleModeOptions" />
            </a-form-item>
            <a-form-item :label="sampleMode === 'raw' ? '原始日志' : '样例文档 JSON'">
              <a-textarea
                v-model:value="sampleText"
                class="json-editor"
                :rows="12"
                :placeholder="sampleMode === 'raw' ? '直接粘贴包含换行的完整日志' : '输入包含 message 或 log 字段的 JSON 对象'"
                spellcheck="false"
              />
            </a-form-item>
            <a-button type="primary" :loading="simulating" @click="simulate">
              <FontAwesomeIcon :icon="['fas', 'play']" />
              <span>&nbsp;运行</span>
            </a-button>
            <a-alert
              v-if="schemaViolations.length"
              class="schema-violation-alert"
              type="error"
              show-icon
              message="存在不符合标准字段的输出"
            >
              <template #description>
                <div>以下字段不在标准字段列表内，写入 OpenSearch 时会被丢弃（不报错但无法检索和聚合），请改写到 <code>app_fields.&lt;字段名&gt;</code> 下：</div>
                <a-space wrap class="schema-violation-tags">
                  <a-tag v-for="field in schemaViolations" :key="field" color="red">{{ field }}</a-tag>
                </a-space>
              </template>
            </a-alert>
            <div v-if="simulationText" class="result-field">
              <div class="result-header">
                <span class="result-title">运行结果</span>
                <a-space>
                  <a-tooltip title="展开日志换行">
                    <a-switch
                      v-model:checked="expandNewline"
                      checked-children="换行"
                      un-checked-children="原始"
                    />
                  </a-tooltip>
                  <a-tooltip title="复制">
                    <a-button size="small" @click="copyResult">
                      <FontAwesomeIcon :icon="['fas', 'copy']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="全屏查看">
                    <a-button size="small" @click="resultFullscreen = true">
                      <FontAwesomeIcon :icon="['fas', 'expand']" />
                    </a-button>
                  </a-tooltip>
                </a-space>
              </div>
              <pre class="json-editor result-viewer">{{ resultDisplayText }}</pre>
            </div>
          </a-form>
        </a-tab-pane>
      </a-tabs>
    </a-modal>

    <a-modal v-model:open="filterEditorOpen" :title="filterForm.id ? `编辑过滤规则：${filterForm.name}` : '新增采集过滤规则'" :confirm-loading="filterSaving" ok-text="保存" cancel-text="取消" @ok="saveFilterRule">
      <a-form ref="filterFormRef" :model="filterForm" :rules="filterFormRules" layout="vertical">
        <a-form-item name="name" label="规则名称"><a-input v-model:value="filterForm.name" placeholder="例如 error-critical-only" /></a-form-item>
        <a-form-item name="application" label="所属应用"><a-select v-model:value="filterForm.application" allow-clear placeholder="留空表示通用规则" :options="applicationOptions" :getPopupContainer="getPopupContainer" /></a-form-item>
        <a-form-item label="说明"><a-input v-model:value="filterForm.description" placeholder="例如 仅采集错误、失败和严重级别日志" /></a-form-item>
        <a-form-item name="pattern" label="匹配正则"><a-textarea v-model:value="filterForm.pattern" :rows="3" placeholder="例如 (?i)(error|failed|critical|fatal)" spellcheck="false" /></a-form-item>
        <a-form-item label="启用"><a-switch v-model:checked="filterForm.enabled" /></a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="resultFullscreen"
      title="运行结果"
      class="result-fullscreen-modal"
      :width="'96vw'"
      :footer="null"
      centered
      destroy-on-close
    >
      <div class="result-header">
        <a-space>
          <a-tooltip title="展开日志换行">
            <a-switch
              v-model:checked="expandNewline"
              checked-children="换行"
              un-checked-children="原始"
            />
          </a-tooltip>
          <a-tooltip title="复制">
            <a-button size="small" @click="copyResult">
              <FontAwesomeIcon :icon="['fas', 'copy']" />
            </a-button>
          </a-tooltip>
        </a-space>
      </div>
      <pre class="json-editor result-viewer result-viewer-full">{{ resultDisplayText }}</pre>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  deleteLogCollectionFilterRule,
  deleteLogProcessingRule,
  getLogCollectionFilterRules,
  getOpenSearchClusterList,
  getLogProcessingRules,
  saveLogCollectionFilterRule,
  saveLogProcessingRule,
  simulateOpenSearchPipeline,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { getApplicationList } from '@/api/assets/application'
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const selectedClusterId = ref(null)
const applications = ref([])
const selectedApplicationKey = ref('all')
const processingRules = ref([])
const collectionFilterRules = ref([])
const loading = ref(false)
const filterLoading = ref(false)
const saving = ref(false)
const filterSaving = ref(false)
const simulating = ref(false)
const editorOpen = ref(false)
const editingOriginalName = ref('')
const activeTab = ref('preprocess')
const catalogTab = ref('processing')
const formRef = ref(null)
const filterFormRef = ref(null)
const filterEditorOpen = ref(false)
const sampleMode = ref('raw')
const sampleText = ref('2026-08-27 13:00:00 INFO service started')
const simulationText = ref('')
const schemaViolations = ref([])
const resultFullscreen = ref(false)
const expandNewline = ref(true)

// 模拟结果里的多行日志会被 JSON 序列化成 \n 转义串，展开成真实换行才能直接阅读堆栈
const resultDisplayText = computed(() => {
  if (!expandNewline.value) return simulationText.value
  return simulationText.value.replace(/\\r\\n|\\n/g, '\n').replace(/\\t/g, '  ')
})

async function copyResult() {
  try {
    await navigator.clipboard.writeText(resultDisplayText.value)
    message.success('已复制运行结果')
  } catch {
    message.error('复制失败，请手动选中复制')
  }
}
const form = reactive({
  id: null,
  name: '',
  description: '',
  application: null,
  input_format: 'text',
  multiline_enabled: false,
  start_pattern: '',
  continuation_pattern: '',
  flush_timeout: 1000,
  bodyText: '',
})
const filterForm = reactive({ id: null, name: '', application: null, description: '', pattern: '', enabled: true })
const applicationOptions = computed(() =>
  applications.value.map((item) => ({ label: item.name, value: item.id })),
)

// 左侧分组：数量统一由当前集群的规则列表当场统计，避免每个应用再发一次请求。
// 除已建应用外额外给出“全部”与“通用”（application 为空，不限应用的规则）两个入口。
const applicationGroups = computed(() => {
  const countByApplication = new Map()
  let genericCount = 0
  for (const rule of processingRules.value) {
    if (rule.application) {
      countByApplication.set(rule.application, (countByApplication.get(rule.application) || 0) + 1)
    } else {
      genericCount += 1
    }
  }
  return [
    { key: 'all', label: '全部规则', count: processingRules.value.length },
    ...applications.value.map((item) => ({
      key: String(item.id),
      label: item.name,
      count: countByApplication.get(item.id) || 0,
    })),
    { key: 'generic', label: '通用（不限应用）', count: genericCount },
  ]
})

const visibleRules = computed(() => {
  if (selectedApplicationKey.value === 'all') return processingRules.value
  if (selectedApplicationKey.value === 'generic') {
    return processingRules.value.filter((rule) => !rule.application)
  }
  return processingRules.value.filter((rule) => String(rule.application) === selectedApplicationKey.value)
})
const visibleFilterRules = computed(() => {
  if (selectedApplicationKey.value === 'all') return collectionFilterRules.value
  if (selectedApplicationKey.value === 'generic') return collectionFilterRules.value.filter((rule) => !rule.application)
  return collectionFilterRules.value.filter((rule) => String(rule.application) === selectedApplicationKey.value)
})
const inputFormatOptions = [
  { label: '文本', value: 'text' },
  { label: 'JSON', value: 'json' },
]
const sampleModeOptions = [
  { label: '原始日志', value: 'raw' },
  { label: '文档 JSON', value: 'json' },
]

const columns = [
  { title: '规则名称', key: 'name', width: 260, fixed: 'left' },
  { title: '说明', key: 'description', width: 430 },
  { title: '发送前格式', key: 'input_format', width: 120, align: 'center' },
  { title: '发送前行处理', key: 'multiline', width: 130, align: 'center' },
  { title: 'Ingest 处理器', key: 'processors', width: 130, align: 'center' },
  { title: '操作', key: 'action', width: 160, fixed: 'right' },
]
const filterColumns = [
  { title: '规则名称', key: 'name', width: 240, fixed: 'left' },
  { title: '说明', key: 'description', width: 300 },
  { title: '匹配正则', key: 'pattern', width: 300 },
  { title: '状态', key: 'enabled', width: 100, align: 'center' },
  { title: '操作', key: 'action', width: 140, fixed: 'right' },
]

const formRules = {
  name: [
    { required: true, message: '请输入规则名称' },
    { pattern: /^[a-z0-9][a-z0-9._-]*$/, message: '仅支持小写字母、数字、点、下划线和连字符' },
  ],
  start_pattern: [{ required: true, message: '请输入首行正则' }],
  continuation_pattern: [{ required: true, message: '请输入续行正则' }],
}

function parseJson(text, label) {
  try {
    const value = JSON.parse(text)
    if (!value || Array.isArray(value) || typeof value !== 'object') {
      throw new Error(`${label}必须是 JSON 对象`)
    }
    return value
  } catch (error) {
    if (error instanceof SyntaxError) {
      throw new Error(`${label}格式错误：${error.message}`)
    }
    throw error
  }
}

// 平台只对接一套 OpenSearch，集群不给用户选，直接取默认（或唯一启用）集群。
async function loadClusters() {
  const response = await getOpenSearchClusterList({ page: 1, page_size: 100 })
  const enabledClusters = (response?.data?.data?.results || []).filter((item) => item.enabled)
  const preferred = enabledClusters.find((item) => item.is_default) || enabledClusters[0]
  selectedClusterId.value = preferred?.id || null
  await loadRules()
}

async function loadApplications() {
  try {
    const response = await getApplicationList({ page_size: 100 })
    applications.value = response?.data?.data?.results || []
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '获取应用列表失败')
  }
}

async function loadRules() {
  if (!selectedClusterId.value) {
    processingRules.value = []
    return
  }
  loading.value = true
  try {
    const response = await getLogProcessingRules({ cluster: selectedClusterId.value, page_size: 100 })
    processingRules.value = response?.data?.data?.results || []
  } catch (error) {
    processingRules.value = []
    message.error(error?.response?.data?.msg || error?.message || '获取解析规则失败')
  } finally {
    loading.value = false
  }
}

async function loadFilterRules() {
  filterLoading.value = true
  try {
    const response = await getLogCollectionFilterRules({ page_size: 100 })
    collectionFilterRules.value = response?.data?.data?.results || []
  } catch (error) {
    collectionFilterRules.value = []
    message.error(error?.response?.data?.msg || error?.message || '获取采集过滤规则失败')
  } finally {
    filterLoading.value = false
  }
}

function resetEditor() {
  Object.assign(form, {
    id: null,
    name: '',
    description: '',
    application: null,
    input_format: 'text',
    multiline_enabled: false,
    start_pattern: '',
    continuation_pattern: '',
    flush_timeout: 1000,
    bodyText: '',
  })
  editingOriginalName.value = ''
  activeTab.value = 'preprocess'
  simulationText.value = ''
  schemaViolations.value = []
}

function openCreate() {
  resetEditor()
  // 从左侧已选应用入口新建时直接带入归属，避免再手选一次
  const selectedApplicationId = Number(selectedApplicationKey.value)
  form.application = Number.isNaN(selectedApplicationId) ? null : selectedApplicationId
  form.bodyText = JSON.stringify({ processors: [] }, null, 2)
  editorOpen.value = true
}

function openEdit(record) {
  resetEditor()
  editingOriginalName.value = record.name
  Object.assign(form, {
    ...record,
    bodyText: JSON.stringify(record.pipeline_body || { processors: [] }, null, 2),
  })
  editorOpen.value = true
}

function openTest(record) {
  openEdit(record)
  activeTab.value = 'test'
}

function openFilterCreate() {
  Object.assign(filterForm, { id: null, name: '', application: null, description: '', pattern: '', enabled: true })
  const selectedApplicationId = Number(selectedApplicationKey.value)
  filterForm.application = Number.isNaN(selectedApplicationId) ? null : selectedApplicationId
  filterEditorOpen.value = true
}

function openFilterEdit(record) {
  Object.assign(filterForm, record)
  filterEditorOpen.value = true
}

const filterFormRules = {
  name: [{ required: true, message: '请输入规则名称' }],
  pattern: [{ required: true, message: '请输入匹配正则' }],
}

async function saveFilterRule() {
  await filterFormRef.value?.validate()
  filterSaving.value = true
  try {
    await saveLogCollectionFilterRule({ ...filterForm })
    message.success('过滤规则保存成功')
    filterEditorOpen.value = false
    await loadFilterRules()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '过滤规则保存失败')
  } finally {
    filterSaving.value = false
  }
}

async function publishPipeline() {
  await formRef.value?.validate()
  let body
  try {
    body = parseJson(form.bodyText, 'Pipeline JSON')
  } catch (error) {
    message.error(error.message)
    activeTab.value = 'ingest'
    return
  }
  saving.value = true
  try {
    await saveLogProcessingRule({
      id: form.id,
      cluster: selectedClusterId.value,
      name: form.name,
      description: form.description,
      input_format: form.input_format,
      multiline_enabled: form.multiline_enabled,
      start_pattern: form.multiline_enabled ? form.start_pattern : '',
      continuation_pattern: form.multiline_enabled ? form.continuation_pattern : '',
      flush_timeout: form.flush_timeout,
      pipeline_body: body,
    })
    message.success('发布成功')
    editorOpen.value = false
    await loadRules()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '发布失败')
  } finally {
    saving.value = false
  }
}

// 还原 Fluent Bit 采集侧行为：tail 默认逐行成记录，开启多行合并后按首行/续行正则聚合成一个事件。
// 调试时若不做这一步，粘贴多条日志会被当成单条文档送进 pipeline，结果与真实采集不一致。
function buildRawDocs(text) {
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  if (!form.multiline_enabled) {
    return lines.filter((line) => line.trim()).map((line) => ({ log: line }))
  }
  if (!form.start_pattern.trim() || !form.continuation_pattern.trim()) {
    throw new Error('已启用多行合并，请先在“发送前处理”填写首行和续行正则')
  }
  let startRe
  let continuationRe
  try {
    startRe = new RegExp(form.start_pattern)
    continuationRe = new RegExp(form.continuation_pattern)
  } catch (error) {
    throw new Error(`多行正则不合法：${error.message}`)
  }
  const docs = []
  let buffer = []
  const flush = () => {
    if (buffer.length) {
      docs.push({ log: buffer.join('\n') })
      buffer = []
    }
  }
  for (const line of lines) {
    if (startRe.test(line)) {
      flush()
      buffer = [line]
    } else if (buffer.length && continuationRe.test(line)) {
      buffer.push(line)
    } else {
      // 两条规则都没命中：Fluent Bit 会把该行单独输出，不并入上一个事件
      flush()
      if (line.trim()) docs.push({ log: line })
    }
  }
  flush()
  if (!docs.length) throw new Error('样例日志为空或未命中首行正则')
  return docs
}

async function simulate() {
  let body
  let docs
  try {
    body = parseJson(form.bodyText, 'Pipeline JSON')
    if (!sampleText.value.trim()) {
      throw new Error(sampleMode.value === 'raw' ? '请输入原始日志' : '请输入样例文档 JSON')
    }
    docs = sampleMode.value === 'raw'
      ? buildRawDocs(sampleText.value)
      : [parseJson(sampleText.value, '样例文档')]
  } catch (error) {
    message.error(error.message)
    return
  }
  simulating.value = true
  try {
    const response = await simulateOpenSearchPipeline(selectedClusterId.value, {
      pipeline: body,
      docs,
    })
    const result = response?.data?.data || {}
    schemaViolations.value = result.schema_violations || []
    simulationText.value = JSON.stringify(result, null, 2)
    if (schemaViolations.value.length) {
      message.warning(`运行成功，但有 ${schemaViolations.value.length} 个字段不符合标准字段规范`)
    } else {
      message.success(`运行成功，共 ${docs.length} 条事件`)
    }
  } catch (error) {
    simulationText.value = ''
    schemaViolations.value = []
    message.error(error?.response?.data?.msg || error?.message || '运行失败')
  } finally {
    simulating.value = false
  }
}

function confirmDelete(record) {
  openDeleteConfirm({
    title: '删除解析规则',
    summary: '删除后，引用该 Pipeline 的日志将无法完成解析。',
    items: [`解析规则: ${record.name}`],
    onConfirm: async () => {
      await deleteLogProcessingRule(record.id)
      message.success('删除成功')
      await loadRules()
    },
  })
}

function confirmDeleteFilter(record) {
  openDeleteConfirm({
    title: '删除过滤规则',
    summary: '删除后，逻辑服务将不能再选择该规则。',
    items: [`过滤规则: ${record.name}`],
    onConfirm: async () => {
      await deleteLogCollectionFilterRule(record.id)
      message.success('删除成功')
      await loadFilterRules()
    },
  })
}

onMounted(() => {
  loadApplications()
  loadClusters()
  loadFilterRules()
})
</script>

<style scoped>
.log-parser-page {
  padding: 16px;
}
.page-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.page-title-row h2 {
  margin: 0;
  font-size: 20px;
}
.parser-layout {
  display: flex;
  align-items: flex-start;
  gap: 16px;
}
.application-pane {
  flex: 0 0 240px;
  width: 240px;
  background: #fff;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  overflow: hidden;
}
.pane-title {
  padding: 10px 16px;
  font-weight: 600;
  border-bottom: 1px solid #f0f0f0;
}
.application-menu {
  border-inline-end: none;
  max-height: calc(100vh - 240px);
  overflow: auto;
}
.application-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.application-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rule-pane {
  flex: 1;
  min-width: 0;
}
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.toolbar .ant-btn {
  display: inline-flex;
  align-items: center;
}
.pipeline-name {
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 13px;
}
.json-editor {
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 13px;
  line-height: 1.55;
  tab-size: 2;
}
.result-field {
  margin-top: 16px;
}
.schema-violation-alert {
  margin-top: 16px;
}
.schema-violation-tags {
  margin-top: 8px;
}
.result-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.result-title {
  color: rgba(0, 0, 0, 0.88);
}
.result-viewer {
  margin: 0;
  padding: 12px;
  background: #f7f8fa;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  max-height: 46vh;
  overflow: auto;
  white-space: pre;
  resize: vertical;
}
.result-viewer-full {
  max-height: none;
  height: calc(100vh - 220px);
}
@media (max-width: 720px) {
  .page-title-row {
    align-items: stretch;
    flex-direction: column;
  }
  .cluster-select {
    width: 100%;
  }
}
</style>
