<template>
  <div class="alert-rules-page">
    <a-card title="告警规则" size="small">
      <a-alert
        type="info"
        show-icon
        message="先维护规则模型（字段化），再导出 Prometheus YAML；导出后同步到 Prometheus rule_files 并 reload。"
        style="margin-bottom: 12px"
      />

      <a-space style="margin-bottom: 12px">
        <a-tooltip title="新增">
          <a-button type="primary" @click="openCreateModal">新增规则</a-button>
        </a-tooltip>
        <a-tooltip title="刷新">
          <a-button type="primary" ghost :loading="loading" @click="loadRules">刷新</a-button>
        </a-tooltip>
        <a-tooltip title="导出 YAML">
          <a-button type="primary" ghost :loading="exportingYaml" @click="handleExportYaml">导出 YAML</a-button>
        </a-tooltip>
        <a-tooltip title="一键部署并 reload">
          <a-button type="primary" ghost :loading="deploying" @click="handleDeployRules">一键部署</a-button>
        </a-tooltip>
        <a-tooltip title="部署记录">
          <a-button type="primary" ghost @click="openDeployHistoryModal">部署记录</a-button>
        </a-tooltip>
        <a-tooltip title="规则组管理">
          <a-button type="primary" ghost @click="goToGroupPage">规则组管理</a-button>
        </a-tooltip>
      </a-space>

      <a-table
        rowKey="id"
        :columns="columns"
        :data-source="ruleList"
        :loading="loading"
        size="small"
        :scroll="{ x: 1600 }"
        :pagination="pagination"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '禁用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'severity'">
            <a-tag :color="severityColor(record.severity)">{{ record.severity || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'expr'">
            <a-typography-text ellipsis style="max-width: 420px">{{ record.expr }}</a-typography-text>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑">
                <a-button type="primary" ghost size="small" @click="openEditModal(record)">编辑</a-button>
              </a-tooltip>
              <a-tooltip title="删除">
                <a-button class="delBtn" danger type="primary" size="small" :loading="deleteLoading[record.id]" @click="openDeleteRuleConfirm(record)">删除</a-button>
              </a-tooltip>
            </a-space>
          </template>
        </template>
      </a-table>

      <div class="footer-hint">
        <a-typography-text type="secondary">
          建议流程：维护规则模型 -> 导出 YAML -> 一键部署并触发 Prometheus reload。
        </a-typography-text>
      </div>
    </a-card>

    <a-modal
      :title="isEdit ? '编辑告警规则' : '新增告警规则'"
      :open="modalVisible"
      :confirm-loading="submitting"
      width="900px"
      ok-text="保存"
      cancel-text="取消"
      @ok="submitForm"
      @cancel="modalVisible = false"
    >
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="规则名称" required>
              <a-input v-model:value="formModel.name" placeholder="例如 HostDown" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="规则组" required>
              <a-select
                v-model:value="formModel.group"
                :options="groupOptions"
                :getPopupContainer="getPopupContainer"
                placeholder="请选择规则组"
              />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="PromQL 表达式" required>
          <a-textarea v-model:value="formModel.expr" :rows="4" placeholder='例如 up{job="node_exporter"} == 0' />
        </a-form-item>

        <a-row :gutter="12">
          <a-col :span="8">
            <a-form-item label="for" required>
              <a-input v-model:value="formModel.duration_for" placeholder="例如 2m" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="keep_firing_for">
              <a-input v-model:value="formModel.keep_firing_for" placeholder="例如 5m（可空）" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="严重级别" required>
              <a-select v-model:value="formModel.severity" :options="severityOptions" :getPopupContainer="getPopupContainer" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="顺序">
              <a-input-number v-model:value="formModel.order_num" :min="1" :precision="0" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="是否启用">
              <a-switch v-model:checked="formModel.enabled" checked-children="启用" un-checked-children="禁用" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="附加标签（JSON 对象）">
          <a-textarea v-model:value="formModel.extra_labels_text" :rows="3" placeholder='例如 {"service":"host","team":"ops"}' />
        </a-form-item>

        <a-form-item label="summary 模板">
          <a-input v-model:value="formModel.summary_template" placeholder="例如 主机不可达 {{ $labels.instance }}" />
        </a-form-item>

        <a-form-item label="description 模板">
          <a-textarea v-model:value="formModel.description_template" :rows="3" placeholder="例如 {{ $labels.instance }} 连续 2 分钟不可达" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="导出的 Prometheus YAML"
      :open="yamlModalVisible"
      :footer="null"
      width="900px"
      @cancel="yamlModalVisible = false"
    >
      <a-space style="margin-bottom: 8px">
        <a-button type="primary" ghost @click="copyYaml">复制 YAML</a-button>
      </a-space>
      <a-textarea
        :value="exportedYaml"
        :rows="22"
        readonly
        class="rules-editor"
      />
    </a-modal>

    <a-modal
      title="部署审计记录"
      :open="deployHistoryModalVisible"
      :footer="null"
      width="960px"
      @cancel="deployHistoryModalVisible = false"
    >
      <a-table
        rowKey="id"
        :columns="deployHistoryColumns"
        :data-source="deployHistories"
        :loading="deployHistoriesLoading"
        size="small"
        :pagination="{ pageSize: 5, showSizeChanger: false }"
        :scroll="{ x: 920 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="record.status === 'success' ? 'green' : 'red'">{{ record.status }}</a-tag>
          </template>
        </template>
      </a-table>
    </a-modal>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  deployAlertRules,
  createAlertRule,
  getAlertRuleDeployHistories,
  getAlertRuleGroups,
  deleteAlertRule,
  exportAlertRulesYaml,
  getAlertRules,
  updateAlertRule,
} from '@/api/sys/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const router = useRouter()
const loading = ref(false)
const submitting = ref(false)
const exportingYaml = ref(false)
const deploying = ref(false)
const ruleList = ref([])
const groupList = ref([])
const deleteLoading = reactive({})
const modalVisible = ref(false)
const isEdit = ref(false)
const currentRuleId = ref(null)
const yamlModalVisible = ref(false)
const exportedYaml = ref('')
const deployHistoryModalVisible = ref(false)
const deployHistoriesLoading = ref(false)
const deployHistories = ref([])

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const severityOptions = [
  { label: 'critical', value: 'critical' },
  { label: 'warning', value: 'warning' },
  { label: 'info', value: 'info' },
]

const groupOptions = ref([])

const deployHistoryColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '操作人', dataIndex: 'requested_username_snapshot', key: 'requested_username_snapshot', width: 140 },
  { title: '规则文件', dataIndex: 'deployed_file_path', key: 'deployed_file_path', width: 260 },
  { title: 'reload URL', dataIndex: 'reload_url', key: 'reload_url', width: 210 },
  { title: '消息', dataIndex: 'message', key: 'message', width: 220 },
  { title: '时间', dataIndex: 'create_time', key: 'create_time', width: 180 },
]

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '规则组', dataIndex: 'group_name', key: 'group_name', width: 160 },
  { title: '规则名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '表达式', dataIndex: 'expr', key: 'expr', width: 440 },
  { title: 'for', dataIndex: 'duration_for', key: 'duration_for', width: 100 },
  { title: '级别', dataIndex: 'severity', key: 'severity', width: 100 },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: '顺序', dataIndex: 'order_num', key: 'order_num', width: 90 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 150 },
  { title: '操作', key: 'action', width: 180, fixed: 'right' },
]

const formModel = reactive({
  group: undefined,
  group_name: 'host-baseline',
  name: '',
  expr: '',
  duration_for: '2m',
  keep_firing_for: '',
  severity: 'warning',
  enabled: true,
  order_num: 100,
  extra_labels_text: '{}',
  summary_template: '',
  description_template: '',
})

function parseApiData(res) {
  const payload = res?.data || res
  if (payload && typeof payload === 'object' && payload.data !== undefined) {
    return payload.data
  }
  return payload || {}
}

function severityColor(severity) {
  if (severity === 'critical') return 'red'
  if (severity === 'warning') return 'orange'
  if (severity === 'info') return 'blue'
  return 'default'
}

function resetForm() {
  formModel.group = groupList.value.length > 0 ? groupList.value[0].id : undefined
  formModel.group_name = 'host-baseline'
  formModel.name = ''
  formModel.expr = ''
  formModel.duration_for = '2m'
  formModel.keep_firing_for = ''
  formModel.severity = 'warning'
  formModel.enabled = true
  formModel.order_num = 100
  formModel.extra_labels_text = '{}'
  formModel.summary_template = ''
  formModel.description_template = ''
}

function openCreateModal() {
  isEdit.value = false
  currentRuleId.value = null
  resetForm()
  modalVisible.value = true
}

function openEditModal(record) {
  isEdit.value = true
  currentRuleId.value = record.id
  formModel.group = record.group || undefined
  formModel.group_name = record.group_name || 'host-baseline'
  formModel.name = record.name || ''
  formModel.expr = record.expr || ''
  formModel.duration_for = record.duration_for || '2m'
  formModel.keep_firing_for = record.keep_firing_for || ''
  formModel.severity = record.severity || 'warning'
  formModel.enabled = Boolean(record.enabled)
  formModel.order_num = Number(record.order_num || 100)
  formModel.extra_labels_text = JSON.stringify(record.extra_labels || {}, null, 2)
  formModel.summary_template = record.summary_template || ''
  formModel.description_template = record.description_template || ''
  modalVisible.value = true
}

function buildPayloadFromForm() {
  let labels = {}
  const text = String(formModel.extra_labels_text || '').trim()
  if (text !== '') {
    try {
      const parsed = JSON.parse(text)
      if (typeof parsed !== 'object' || Array.isArray(parsed) || parsed === null) {
        throw new Error('附加标签必须是 JSON 对象')
      }
      labels = parsed
    } catch (error) {
      throw new Error('附加标签不是合法 JSON 对象')
    }
  }

  return {
    group: formModel.group,
    group_name: String(formModel.group_name || '').trim(),
    name: String(formModel.name || '').trim(),
    expr: String(formModel.expr || '').trim(),
    duration_for: String(formModel.duration_for || '').trim(),
    keep_firing_for: String(formModel.keep_firing_for || '').trim(),
    severity: formModel.severity,
    enabled: Boolean(formModel.enabled),
    order_num: Number(formModel.order_num || 100),
    extra_labels: labels,
    summary_template: String(formModel.summary_template || '').trim(),
    description_template: String(formModel.description_template || '').trim(),
  }
}

async function loadGroups() {
  try {
    const res = await getAlertRuleGroups({ page: 1, page_size: 200, ordering: 'order_num' })
    const data = parseApiData(res)
    const rows = Array.isArray(data.results) ? data.results : []
    groupList.value = rows
    groupOptions.value = rows.map((item) => ({ label: `${item.name} (${item.interval})`, value: item.id }))
    if (!formModel.group && rows.length > 0) {
      formModel.group = rows[0].id
    }
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '加载规则组失败')
  }
}

async function loadRules() {
  loading.value = true
  try {
    const res = await getAlertRules({
      page: pagination.current,
      page_size: pagination.pageSize,
      ordering: 'order_num',
    })
    const data = parseApiData(res)
    ruleList.value = Array.isArray(data.results) ? data.results : []
    pagination.total = Number(data.count || 0)
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '加载规则失败')
  } finally {
    loading.value = false
  }
}

async function submitForm() {
  let payload = null
  try {
    payload = buildPayloadFromForm()
  } catch (error) {
    message.warning(error?.message || '请检查表单')
    return
  }

  submitting.value = true
  try {
    if (isEdit.value && currentRuleId.value) {
      await updateAlertRule(currentRuleId.value, payload)
      message.success('规则更新成功')
    } else {
      await createAlertRule(payload)
      message.success('规则创建成功')
    }
    modalVisible.value = false
    await loadRules()
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '保存规则失败')
  } finally {
    submitting.value = false
  }
}

function openDeleteRuleConfirm(record) {
  openDeleteConfirm({
    title: '删除告警规则',
    contentTitle: '确认删除以下告警规则吗？',
    items: [{ name: `${record.name} (${record.group_name})` }],
    async onConfirm() {
      deleteLoading[record.id] = true
      try {
        await deleteAlertRule(record.id)
        message.success('删除成功')
        await loadRules()
      } catch (error) {
        message.warning(error?.response?.data?.msg || error?.message || '删除规则失败')
      } finally {
        deleteLoading[record.id] = false
      }
    },
  })
}

function handleTableChange(nextPagination) {
  pagination.current = Number(nextPagination?.current || 1)
  pagination.pageSize = Number(nextPagination?.pageSize || 10)
  loadRules()
}

async function handleExportYaml() {
  exportingYaml.value = true
  try {
    const res = await exportAlertRulesYaml()
    const data = parseApiData(res)
    exportedYaml.value = String(data.yaml || '')
    yamlModalVisible.value = true
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '导出 YAML 失败')
  } finally {
    exportingYaml.value = false
  }
}

async function handleDeployRules() {
  deploying.value = true
  try {
    const res = await deployAlertRules()
    const data = parseApiData(res)
    message.success(`部署成功：${data.file_path || ''}`)
    await loadDeployHistories()
  } catch (error) {
    const serverMsg = error?.response?.data?.msg
    const rawMsg = String(error?.message || '')
    const isNetworkLike = /network|timeout|failed to fetch|ecconn|econn/i.test(rawMsg)
    // 业务错误码会被 request 拦截器统一 message.error，这里避免再次重复弹窗。
    if (serverMsg) {
      message.warning(serverMsg)
    } else if (isNetworkLike) {
      message.warning(rawMsg || '部署失败')
    }
    await loadDeployHistories()
  } finally {
    deploying.value = false
  }
}

async function loadDeployHistories() {
  deployHistoriesLoading.value = true
  try {
    const res = await getAlertRuleDeployHistories({ page: 1, page_size: 20, ordering: '-id' })
    const data = parseApiData(res)
    deployHistories.value = Array.isArray(data.results) ? data.results : []
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '加载部署记录失败')
  } finally {
    deployHistoriesLoading.value = false
  }
}

async function openDeployHistoryModal() {
  deployHistoryModalVisible.value = true
  await loadDeployHistories()
}

function goToGroupPage() {
  router.push('/monitor/alert-rule-groups')
}

async function copyYaml() {
  try {
    await navigator.clipboard.writeText(exportedYaml.value || '')
    message.success('YAML 已复制')
  } catch (error) {
    message.warning('复制失败，请手动复制')
  }
}

onMounted(() => {
  loadGroups()
  loadRules()
})
</script>

<style scoped>
.alert-rules-page {
  padding: 12px;
}

.rules-editor {
  font-family: Menlo, Monaco, Consolas, 'Courier New', monospace;
}

.footer-hint {
  margin-top: 10px;
}
</style>
