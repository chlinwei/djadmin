<template>
  <div class="log-parser-page">
    <div class="page-title-row">
      <h2>日志处理规则</h2>
      <a-select
        v-model:value="selectedClusterId"
        class="cluster-select"
        placeholder="选择 OpenSearch 集群"
        :getPopupContainer="getPopupContainer"
        @change="loadRules"
      >
        <a-select-option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">
          {{ cluster.name }}
          <span v-if="cluster.is_default">（默认）</span>
        </a-select-option>
      </a-select>
    </div>

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
      :data-source="processingRules"
      :loading="loading"
      :pagination="false"
      :scroll="{ x: 1050 }"
      :locale="{ emptyText: selectedClusterId ? '暂无解析规则' : '请先选择 OpenSearch 集群' }"
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
            <a-form-item v-if="simulationText" label="运行结果" class="result-field">
              <a-textarea
                v-model:value="simulationText"
                class="json-editor result-editor"
                :rows="10"
                readonly
              />
            </a-form-item>
          </a-form>
        </a-tab-pane>
      </a-tabs>
    </a-modal>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  deleteLogProcessingRule,
  getOpenSearchClusterList,
  getOpenSearchPipelineDefault,
  getLogProcessingRules,
  saveLogProcessingRule,
  simulateOpenSearchPipeline,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const clusters = ref([])
const selectedClusterId = ref(null)
const processingRules = ref([])
const loading = ref(false)
const saving = ref(false)
const simulating = ref(false)
const editorOpen = ref(false)
const editingOriginalName = ref('')
const activeTab = ref('preprocess')
const formRef = ref(null)
const sampleMode = ref('raw')
const sampleText = ref('2026-08-27 13:00:00 INFO service started')
const simulationText = ref('')
const form = reactive({
  id: null,
  name: '',
  description: '',
  input_format: 'text',
  multiline_enabled: false,
  start_pattern: '',
  continuation_pattern: '',
  flush_timeout: 1000,
  bodyText: '',
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

async function loadClusters() {
  const response = await getOpenSearchClusterList({ page: 1, page_size: 100 })
  clusters.value = (response?.data?.data?.results || []).filter((item) => item.enabled)
  const preferred = clusters.value.find((item) => item.is_default) || clusters.value[0]
  selectedClusterId.value = preferred?.id || null
  await loadRules()
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

function resetEditor() {
  Object.assign(form, {
    id: null,
    name: '',
    description: '',
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
}

async function openCreate() {
  resetEditor()
  try {
    const response = await getOpenSearchPipelineDefault(selectedClusterId.value)
    form.bodyText = JSON.stringify(response?.data?.data || { processors: [] }, null, 2)
    editorOpen.value = true
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '获取默认规则失败')
  }
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

async function simulate() {
  let body
  let sample
  try {
    body = parseJson(form.bodyText, 'Pipeline JSON')
    if (!sampleText.value.trim()) {
      throw new Error(sampleMode.value === 'raw' ? '请输入原始日志' : '请输入样例文档 JSON')
    }
    sample = sampleMode.value === 'raw'
      ? { log: sampleText.value }
      : parseJson(sampleText.value, '样例文档')
  } catch (error) {
    message.error(error.message)
    return
  }
  simulating.value = true
  try {
    const response = await simulateOpenSearchPipeline(selectedClusterId.value, {
      pipeline: body,
      docs: [sample],
    })
    simulationText.value = JSON.stringify(response?.data?.data || {}, null, 2)
    message.success('运行成功')
  } catch (error) {
    simulationText.value = ''
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

onMounted(loadClusters)
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
.cluster-select {
  width: min(360px, 50vw);
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
.result-editor {
  background: #f7f8fa;
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
