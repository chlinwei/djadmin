<template>
  <div class="automation-page">
    <a-row class="tools" :gutter="12">
      <a-col :span="16">
        <a-input-search
          v-model:value="taskKeyword"
          placeholder="搜索任务名称"
          allow-clear
          enter-button
          @search="loadTasks(true)"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-tooltip title="新增">
            <a-button size="large" @click="openCreateModal" v-permission="'automation:tasks:create'">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp新增任务</span>
            </a-button>
          </a-tooltip>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="taskLoading || playbookLoading" @click="reloadAll">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="taskLoading || playbookLoading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="任务列表" size="small" class="block-card">
      <a-table
        :columns="taskColumns"
        :data-source="tasks"
        :loading="taskLoading"
        :pagination="taskPagination"
        rowKey="id"
        size="small"
        :scroll="{ x: 1700 }"
        @change="handleTaskTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <a-button
              v-if="canEditTask"
              type="link"
              size="small"
              class="task-name-link"
              @click="openEditModal(record)"
            >
              {{ record.name || '-' }}
            </a-button>
            <span v-else>{{ record.name || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'enabled'">
            <a-switch
              :checked="record.enabled === true"
              :disabled="!canEditTask || taskLoading || taskStatusUpdatingId === record.id"
              :loading="taskStatusUpdatingId === record.id"
              @change="(checked) => onChangeTaskStatus(checked, record)"
            />
          </template>
          <template v-else-if="column.key === 'template_name'">
            <a-button type="link" size="small" class="task-code-link" @click="goToPlaybookTemplate(record)">
              {{ record.template_name || '-' }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'inventory_name'">
            <a-button type="link" size="small" class="task-code-link" @click="goToInventory(record)">
              {{ record.inventory_name || '-' }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'selected_group_ids'">
            <div class="scope-compact-cell">
              <span v-if="!record.inventory" class="scope-limit-empty">未设置 Inventory</span>

              <template v-else>
              <a-tag v-if="record.limit_preview_limit" color="blue" class="scope-limit-tag">
                {{ record.limit_preview_limit }}
              </a-tag>
              <span v-else class="scope-limit-empty">未设置默认 Limit</span>

              <a-button
                v-if="Number(record.limit_preview_total || 0) > 0"
                type="link"
                size="small"
                class="scope-host-count-link"
                @click.stop="openScopePreviewModal(record)"
              >
                {{
                  record.limit_preview_limit
                    ? `${Number(record.limit_preview_total || 0)} 台主机`
                    : `Inventory 全量（${Number(record.limit_preview_total || 0)} 台，列表折叠）`
                }}
              </a-button>

              <span v-else-if="record.limit_preview_limit" class="scope-match-empty">0 台匹配</span>
              <span v-else class="scope-match-empty">Inventory 无主机</span>
              </template>
            </div>
          </template>
          <template v-else-if="column.key === 'env_vars'">
            <a-tooltip :title="formatEnvVarCellFullText(record.env_vars)">
              <div class="json-cell">{{ formatEnvVarCell(record.env_vars) }}</div>
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'update_time'">
            <span>{{ record.update_time ? formatTimeWithTimezone(record.update_time, store.state.user?.timezone || 'Asia/Shanghai') : '-' }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑">
                <a-button size="small" type="primary" @click="openEditModal(record)" v-permission="'automation:tasks:update'">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="运行">
                <a-button
                  size="small"
                  type="primary"
                  ghost
                  :loading="runningTaskId === record.id"
                  :disabled="!record.enabled || runningTaskId === record.id"
                  @click="openRunNowModal(record)"
                  v-permission="'automation:jobs:create'"
                >
                  <FontAwesomeIcon :icon="['fas', 'play']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="历史记录">
                <a-button size="small" @click="viewTaskLogs(record)" v-permission="'automation:jobs:view'">
                  历史记录
                  <FontAwesomeIcon :icon="['fas', 'list']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="删除">
                <a-button
                  class="delBtn"
                  size="small"
                  type="primary"
                  danger
                  v-permission="'automation:tasks:delete'"
                  @click="openDeleteTaskConfirm(record)"
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
      :title="isCreateMode ? '新增任务' : '编辑任务'"
      :open="taskModalVisible"
      :width="820"
      :confirmLoading="modalSubmitting"
      @ok="submitTask"
      @cancel="taskModalVisible = false"
    >
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="任务名称" required>
              <a-input v-model:value="taskForm.name" placeholder="例如：生产环境健康巡检" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-row :gutter="12">
          <a-col :span="8">
            <a-form-item label="模板类型" required>
              <a-select
                v-model:value="taskForm.template_type"
                :options="[
                  { label: 'Playbook', value: 'playbook' },
                  { label: 'Shell脚本', value: 'shell_script' }
                ]"
                :getPopupContainer="getTaskModalPopupContainer"
                @change="handleTemplateTypeChange"
              />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="选择模板" required>
              <a-select
                v-model:value="taskForm.template"
                :options="taskTemplateOptions"
                :loading="taskTemplateLoading"
                show-search
                optionFilterProp="label"
                :getPopupContainer="getTaskModalPopupContainer"
                :placeholder="taskTemplatePlaceholder"
              />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="选择Inventory（可选）">
              <a-select
                v-model:value="taskForm.inventory"
                :options="inventoryOptions"
                show-search
                optionFilterProp="label"
                :getPopupContainer="getTaskModalPopupContainer"
                placeholder="可选：未选择则按任务节点范围执行"
                allow-clear
              />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="启用状态">
          <a-switch v-model:checked="taskForm.enabled" checked-children="启用" un-checked-children="禁用" />
        </a-form-item>

        <a-form-item>
          <a-alert
            type="info"
            show-icon
            message="任务执行范围由所选 Inventory 决定；主机组请在 Inventory 管理中维护"
          />
        </a-form-item>

        <a-form-item label="默认 Limit（可选）">
          <a-input
            v-model:value="taskForm.default_limit"
            :placeholder="LIMIT_INPUT_PLACEHOLDER"
          />
          <ScopePrecheckPanel
            :precheck-ok="taskLimitPrecheckOk"
            :prechecking="taskLimitPrechecking"
            :message="taskLimitPrecheckText"
            :hosts="taskLimitAllHosts"
            :matched-hosts="taskLimitMatchedHosts"
            :show-host-link="true"
            :show-limit-toggle="true"
            :show-target-filter="true"
            :limit-text="taskForm.default_limit"
            @host-click="handleTaskLimitHostClick"
            @toggle-limit-host="handleTaskLimitToggle"
            @remove-limit-token="handleTaskLimitRemoveToken"
          />
        </a-form-item>

        <a-form-item label="执行超时（秒）" required>
          <a-input-number
            v-model:value="taskForm.execution_timeout_seconds"
            :min="1"
            :max="14400"
            :step="60"
            style="width: 100%"
            placeholder="默认 600"
          />
          <a-alert
            type="info"
            show-icon
            style="margin-top: 8px"
            message="任务总执行超时（秒）。超过该时间后 dj-agent 会终止执行进程（保底退出）。"
          />
        </a-form-item>

        <a-form-item :label="taskEnvVarsLabel">
          <a-textarea
            v-model:value="taskForm.env_vars_text"
            :rows="6"
            :placeholder="taskEnvVarsPlaceholder"
          />
          <a-alert
            v-if="taskForm.template_type === 'shell_script'"
            type="info"
            show-icon
            style="margin-top: 8px"
            message="这里填写的是环境变量（key=value），执行时会以 export 注入脚本运行环境。"
          />
        </a-form-item>

        <a-form-item
          v-if="taskForm.template_type === 'shell_script'"
          label="Shell 参数字符串（可选）"
        >
          <a-input
            v-model:value="taskForm.shell_args_text"
            :placeholder="taskShellArgsPlaceholder"
          />
          <a-alert
            type="info"
            show-icon
            style="margin-top: 8px"
            message="按空格分割的参数字符串，执行时按顺序映射到 $1、$2...（示例：prod 8080 --force）"
          />
        </a-form-item>

        <a-divider orientation="left" style="margin: 16px 0">权限提升配置</a-divider>

        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="权限提升">
              <a-switch
                v-model:checked="taskForm.become_enabled"
                checked-children="启用"
                un-checked-children="禁用"
              />
              <span style="margin-left: 8px; color: #666; font-size: 12px">
                在目标主机上以指定用户（如 root）身份执行任务
              </span>
            </a-form-item>
          </a-col>
        </a-row>

        <a-row :gutter="12" v-if="taskForm.become_enabled">
          <a-col :span="12">
            <a-form-item label="提升方式">
              <a-select
                v-model:value="taskForm.become_method"
                :options="[
                  { label: 'sudo', value: 'sudo' },
                  { label: 'su', value: 'su' }
                ]"
                :getPopupContainer="getTaskModalPopupContainer"
                placeholder="选择权限提升方式"
              />
              <a-alert
                type="info"
                show-icon
                message="sudo: 安全且可配置免密，推荐使用；su: 需要目标用户密码"
                style="margin-top: 8px"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="目标用户">
              <a-input
                v-model:value="taskForm.become_user"
                placeholder="默认为 root"
              />
              <a-alert
                type="warning"
                show-icon
                message="请确保该用户有执行权限，建议在凭证中配置免密sudo"
                style="margin-top: 8px"
              />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>

    <a-modal
      title="运行任务"
      :open="runNowModalVisible"
      :confirmLoading="runNowSubmitting"
      ok-text="立即执行"
      cancel-text="取消"
      :ok-button-props="{ disabled: !runNowPrecheckOk }"
      @ok="confirmRunNow"
      @cancel="closeRunNowModal"
    >
      <a-form layout="vertical">
        <a-form-item label="本次 Limit（可选）">
          <a-input
            v-model:value="runNowLimit"
            allow-clear
            :placeholder="LIMIT_INPUT_PLACEHOLDER"
          />
        </a-form-item>
        <a-form-item v-if="runNowIsShellTask" label="本次参数字符串（可选）">
          <a-input
            v-model:value="runNowShellArgs"
            allow-clear
            placeholder="例如: prod 8080 --force"
          />
        </a-form-item>
      </a-form>

      <ScopePrecheckPanel
        :precheck-ok="runNowPrecheckOk"
        :prechecking="runNowPrechecking"
        :message="runNowPrecheckText"
        :hosts="runNowAllHosts"
        :matched-hosts="runNowMatchedHosts"
        :show-host-link="true"
        :show-limit-toggle="true"
        :show-target-filter="true"
        :limit-text="runNowLimit"
        @host-click="handleRunNowHostClick"
        @toggle-limit-host="handleRunNowLimitToggle"
        @remove-limit-token="handleRunNowRemoveToken"
      />
    </a-modal>

    <ExecutionScopePreviewModal
      :title="scopePreviewTitle"
      :open="scopePreviewModalVisible"
      :hosts="scopePreviewHosts"
      :total="scopePreviewTotal"
      @close="closeScopePreviewModal"
      @host-click="handleScopePreviewHostClick"
    />
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { formatTimeWithTimezone } from '@/util/timezone'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import store from '@/store'
import {
  getPlaybookList,
  getShellScriptTemplateList,
  getInventoryList,
  getTaskList,
  createTask,
  updateTask,
  deleteTask,
  precheckTaskRun,
  runTaskNow,
  precheckInventoryLimit,
  getAutomationHostOptions,
  getAutomationGroupTree,
} from '@/api/sys/automation'
import { checkPermission } from '@/directives/permission/permission'
import { buildScopeSummaryText, flattenGroupPathMap } from '../scopeSummary'
import { buildAutomationInventoryRoute, buildAutomationTemplateRoute } from '../navigation'
import ScopePrecheckPanel from '../components/ScopePrecheckPanel.vue'
import ExecutionScopePreviewModal from '../components/ExecutionScopePreviewModal.vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import {
  LIMIT_INPUT_PLACEHOLDER,
  removeLimitToken,
  resolveMatchedHostLimitToken,
  toggleLimitToken,
} from '../utils/scopeHelpers'
import {
  appendGroupHostCount,
  buildGroupTreeWithAll,
  cloneTreeNodes,
  collectCheckedHostCountByNode,
  collectCheckedScope,
  collectExpandedGroupKeys,
  collectTreeScopeStats,
  filterTreeByKeyword,
  pruneTreeToCheckedNodes,
  stripGroupCountSuffix,
  toNumericSet,
} from '../utils/taskScopeTree'
import {
  ASSET_HOST_ROUTE_CANDIDATES,
  ALL_GROUP_TITLE,
  ALL_GROUP_VALUE,
  GROUP_TAG_PREVIEW_COUNT,
  TASK_COLUMNS,
  resolveTaskListOrdering,
  parseJsonObjectText,
  flattenGroupNameMap,
  formatEnvVarCellText,
  getScopeTreeNode,
} from '../utils/automationTaskHelpers'

const route = useRoute()
const router = useRouter()
const getTaskModalPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const canEditTask = computed(() => checkPermission('automation:tasks:update'))
const tasks = ref([])
const taskLoading = ref(false)
const taskKeyword = ref('')
const taskPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const playbooks = ref([])
const playbookLoading = ref(false)
const playbookOptions = ref([])
const shellScriptTemplates = ref([])
const shellScriptLoading = ref(false)
const shellScriptOptions = ref([])
const inventories = ref([])
const inventoryOptions = ref([])

const groupLoading = ref(false)
const groupTreeData = ref([])
const groupMap = ref({})
const groupPathMap = ref({})
const groupScopeCheckedKeys = ref([])

const taskModalVisible = ref(false)
const groupScopeEditorVisible = ref(false)
const groupScopeEditorTreeWrapRef = ref(null)
const groupScopeViewVisible = ref(false)
const groupScopeViewTitle = ref('查看主机组范围')
const groupScopeViewTreeData = ref([])
const groupScopeViewSummary = ref('')
const groupScopeViewKeyword = ref('')
const scopePreviewModalVisible = ref(false)
const scopePreviewTitle = ref('执行范围主机预览')
const scopePreviewKeyword = ref('')
const scopePreviewHosts = ref([])
const scopePreviewTreeData = ref([])
const scopePreviewTotal = ref(0)
const modalSubmitting = ref(false)
const isCreateMode = ref(true)
const editingTaskId = ref(null)
const runningTaskId = ref(null)
const taskStatusUpdatingId = ref(null)
const runNowModalVisible = ref(false)
const runNowSubmitting = ref(false)
const runNowPrechecking = ref(false)
const runNowTask = ref(null)
const runNowLimit = ref('')
const runNowShellArgs = ref('')
const runNowHostCount = ref(0)
const runNowEffectiveLimit = ref('')
const runNowAllHosts = ref([])
const runNowMatchedHosts = ref([])
const runNowPrecheckOk = ref(false)
const runNowPrecheckMessage = ref('请输入 Limit，系统将实时预检匹配结果')
let runNowPrecheckTimer = null
let runNowPrecheckSeq = 0
const taskLimitPrechecking = ref(false)
const taskLimitPrecheckOk = ref(false)
const taskLimitPrecheckMessage = ref('请选择 Inventory 后输入 Limit，系统将实时预检')
const taskLimitAllHosts = ref([])
const taskLimitMatchedHosts = ref([])
let taskLimitPrecheckTimer = null
let taskLimitPrecheckSeq = 0

const taskForm = reactive({
  name: '',
  template_type: 'playbook',
  template: null,
  inventory: null,
  default_limit: '',
  execution_timeout_seconds: 600,
  selected_host_ids: [],
  selected_group_ids: [],
  env_vars_text: '',
  shell_args_text: '',
  enabled: true,
  remark: '',
  // 权限提升配置
  become_enabled: false,
  become_method: 'sudo',
  become_user: 'root',
})

const taskColumns = TASK_COLUMNS

const taskTemplateOptions = computed(() => {
  if (taskForm.template_type === 'shell_script') {
    return shellScriptOptions.value
  }
  return playbookOptions.value
})

const taskTemplateLoading = computed(() => {
  if (taskForm.template_type === 'shell_script') {
    return shellScriptLoading.value
  }
  return playbookLoading.value
})

const taskTemplatePlaceholder = computed(() => {
  if (taskForm.template_type === 'shell_script') {
    return '请选择 Shell 脚本模板'
  }
  return '请选择 Playbook 模板'
})

const taskEnvVarsLabel = computed(() => {
  if (taskForm.template_type === 'shell_script') {
    return 'Shell 环境变量（可选）'
  }
  return '环境变量 JSON（可选）'
})

const taskEnvVarsPlaceholder = computed(() => {
  if (taskForm.template_type === 'shell_script') {
    return '例如: var1=a;var2=b'
  }
  return '{"env":"prod","batch":20}'
})

const taskShellArgsPlaceholder = computed(() => '例如: prod 8080 --force')

function parseShellEnvText(text) {
  const input = String(text || '').trim()
  if (!input) return {}
  const result = {}
  for (const item of input.split(/[;\n]+/).map((s) => s.trim()).filter(Boolean)) {
    const i = item.indexOf('=')
    if (i <= 0) throw new Error(`Shell 环境变量格式错误: ${item}`)
    const key = item.slice(0, i).trim()
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) throw new Error(`Shell 环境变量名不合法: ${key}`)
    result[key] = item.slice(i + 1).trim()
  }
  return result
}

function formatShellEnvText(envVars) {
  if (!envVars || typeof envVars !== 'object' || Array.isArray(envVars)) return ''
  return Object.entries(envVars).map(([k, v]) => `${k}=${String(v ?? '')}`).join(';')
}

function resolveTaskSubmitErrorMessage(error) {
  const response = error?.response?.data
  const fieldErrors = response?.data
  if (fieldErrors && typeof fieldErrors === 'object' && !Array.isArray(fieldErrors)) {
    const labels = { name: '任务名称', inventory: 'Inventory', shell_parameters: 'Shell 参数字符串', env_vars: '环境变量' }
    const messages = []
    for (const [field, value] of Object.entries(fieldErrors)) {
      const text = (Array.isArray(value) ? value.join('；') : String(value || '')).trim()
      if (!text) continue
      if (field === 'name' && /already exists/i.test(text)) { messages.push('任务名称已存在'); continue }
      messages.push(field === 'non_field_errors' ? text : `${labels[field] || field}: ${text}`)
    }
    if (messages.length) return messages.join('；')
  }
  return response?.msg || error?.message || '任务保存失败'
}

function buildRunNowPayload(limit, isShellTask, shellArgs) {
  const runtimeLimit = String(limit || '').trim()
  return isShellTask
    ? { limit: runtimeLimit, shell_parameters: String(shellArgs || '').trim() }
    : { limit: runtimeLimit }
}

const taskSort = reactive({
  field: null,
  order: null,
})

const runNowIsShellTask = computed(() => Number(runNowTask.value?.shell_script_template || 0) > 0)

function resolveTaskOrdering() {
  return resolveTaskListOrdering(taskSort)
}

function getTaskScopeSummaryText(record) {
  return buildScopeSummaryText({
    selectedGroupIds: record?.selected_group_ids,
    selectedHostIds: record?.selected_host_ids,
    groupPathMap: groupPathMap.value,
    groupNameMap: groupMap.value,
  })
}

function formatEnvVarCell(value) {
  return formatEnvVarCellText(value)
}

function formatEnvVarCellFullText(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return '-'
  }
  const entries = Object.entries(value)
  if (!entries.length) {
    return '{}'
  }
  const isFlat = entries.every(([key, item]) => {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(String(key))) {
      return false
    }
    const itemType = typeof item
    return item == null || itemType === 'string' || itemType === 'number' || itemType === 'boolean'
  })
  return isFlat
    ? entries.map(([key, item]) => `${key}=${String(item ?? '')}`).join('; ')
    : JSON.stringify(value)
}

const filteredScopePreviewTreeData = computed(() => {
  return filterTreeByKeyword(scopePreviewTreeData.value, scopePreviewKeyword.value)
})

const scopePreviewVisibleHostCount = computed(() => {
  const stats = collectTreeScopeStats(filteredScopePreviewTreeData.value)
  return stats.hostCount
})

function openScopePreviewModal(record) {
  scopePreviewHosts.value = Array.isArray(record?.limit_preview_hosts) ? [...record.limit_preview_hosts] : []
  scopePreviewTotal.value = Number(record?.limit_preview_total || 0)
  scopePreviewTitle.value = `执行范围主机预览 / ${record?.name || '-'}`
  scopePreviewModalVisible.value = true
}

function closeScopePreviewModal() {
  scopePreviewModalVisible.value = false
}

function handleScopePreviewHostClick(item) {
  goToAssetHost(item?.host_id, item?.host_name)
}

function getGroupLabels(groupIds) {
  if (!Array.isArray(groupIds) || groupIds.length === 0) {
    return []
  }
  if (groupIds.includes(ALL_GROUP_VALUE)) {
    return [ALL_GROUP_TITLE]
  }
  return groupIds.map((id) => groupMap.value[id] || `分组#${id}`)
}

function getRecordGroupIds(record) {
  const selectedGroupIds = Array.isArray(record?.selected_group_ids) ? record.selected_group_ids : []
  const selectedHostIds = Array.isArray(record?.selected_host_ids) ? record.selected_host_ids : []
  if (selectedGroupIds.length === 0 && selectedHostIds.length === 0) {
    return [ALL_GROUP_VALUE]
  }
  return selectedGroupIds
}

function getRecordGroupLabels(record) {
  return getGroupLabels(getRecordGroupIds(record))
}

function getVisibleGroupLabels(groupIds) {
  return getGroupLabels(groupIds).slice(0, GROUP_TAG_PREVIEW_COUNT)
}

function getHiddenGroupLabels(groupIds) {
  return getGroupLabels(groupIds).slice(GROUP_TAG_PREVIEW_COUNT)
}

function getGroupScopePreviewText() {
  const selectedGroupIds = Array.isArray(taskForm.selected_group_ids) ? taskForm.selected_group_ids : []
  if (selectedGroupIds.includes(ALL_GROUP_VALUE)) {
    return ALL_GROUP_TITLE
  }
  if (selectedGroupIds.length === 0) {
    return '全部主机（未指定主机组）'
  }
  const labels = selectedGroupIds.map((id) => groupMap.value[id] || `分组#${id}`)
  const preview = labels.slice(0, 2).join('，')
  return labels.length > 2 ? `${preview} 等${labels.length}组` : preview
}

async function loadPlaybooks() {
  playbookLoading.value = true
  try {
    const res = await getPlaybookList({ page: 1, page_size: 200, ordering: '-id' })
    const data = res?.data?.data || {}
    playbooks.value = data.results || []
    playbookOptions.value = playbooks.value
      .map((item) => ({ value: item.id, label: item.name }))
  } finally {
    playbookLoading.value = false
  }
}

async function loadShellScriptTemplates() {
  shellScriptLoading.value = true
  try {
    const res = await getShellScriptTemplateList({ page: 1, page_size: 200, ordering: '-id' })
    const data = res?.data?.data || {}
    shellScriptTemplates.value = data.results || []
    shellScriptOptions.value = shellScriptTemplates.value
      .map((item) => ({ value: item.id, label: item.name }))
  } finally {
    shellScriptLoading.value = false
  }
}

async function loadInventories() {
  const res = await getInventoryList({ page: 1, page_size: 500, ordering: '-id' })
  const data = res?.data?.data || {}
  inventories.value = data.results || []
  inventoryOptions.value = inventories.value
    .map((item) => ({ value: item.id, label: item.name }))
}

async function loadTasks(resetPage = false) {
  if (resetPage) {
    taskPagination.current = 1
  }
  taskLoading.value = true
  try {
    const normalizedKeyword = String(taskKeyword.value || '').trim()
    const exactTaskId = /^\d+$/.test(normalizedKeyword) ? Number(normalizedKeyword) : 0
    const res = await getTaskList({
      page: taskPagination.current,
      page_size: taskPagination.pageSize,
      ordering: resolveTaskOrdering(),
      search: exactTaskId > 0 ? undefined : (normalizedKeyword || undefined),
      task_id: exactTaskId > 0 ? String(exactTaskId) : undefined,
    })
    const data = res?.data?.data || {}
    tasks.value = data.results || []
    taskPagination.total = data.count || 0
  } finally {
    taskLoading.value = false
  }
}

async function findTaskRecordById(taskId) {
  const direct = tasks.value.find((item) => Number(item?.id) === taskId)
  if (direct) {
    return direct
  }

  const pageSize = 200
  let page = 1

  while (page <= 50) {
    const res = await getTaskList({ page, page_size: pageSize, ordering: '-id' })
    const data = res?.data?.data || {}
    const records = Array.isArray(data.results) ? data.results : []
    const hit = records.find((item) => Number(item?.id) === taskId)
    if (hit) {
      return hit
    }
    if (!data.next) {
      break
    }
    page += 1
  }

  return null
}

async function openTaskFromRouteQuery() {
  const rawTaskId = Array.isArray(route.query.task_id) ? route.query.task_id[0] : route.query.task_id
  const taskId = Number(rawTaskId)
  if (!Number.isInteger(taskId) || taskId <= 0) {
    return
  }

  taskKeyword.value = String(taskId)
  await loadTasks(true)

  const target = await findTaskRecordById(taskId)
  if (target) {
    openEditModal(target)
  } else {
    message.warning(`未找到任务 #${taskId}`)
  }

  const nextQuery = { ...route.query }
  delete nextQuery.task_id
  await router.replace({ path: route.path, query: nextQuery })
}

async function loadGroupTree() {
  groupLoading.value = true
  try {
    const [groupRes, hostRecords] = await Promise.all([
      getAutomationGroupTree(),
      loadAllHostOptions(),
    ])
    const groupData = groupRes?.data?.data || []
    groupTreeData.value = appendGroupHostCount(buildGroupTreeWithAll(groupData, hostRecords))
    groupMap.value = flattenGroupNameMap(groupData, {})
    groupPathMap.value = flattenGroupPathMap(groupData, {})
  } finally {
    groupLoading.value = false
  }
}

async function loadAllHostOptions() {
  const pageSize = 500
  let page = 1
  let total = null
  let next = null
  const records = []

  while (true) {
    const res = await getAutomationHostOptions({ page, page_size: pageSize })
    const data = res?.data?.data || {}
    const pageRecords = Array.isArray(data.results) ? data.results : []
    if (typeof data.count === 'number') {
      total = data.count
    }
    next = data.next || null

    records.push(...pageRecords)

    if (!next) {
      break
    }
    if (typeof total === 'number' && records.length >= total) {
      break
    }

    page += 1
    if (page > 100) {
      break
    }
  }

  return records
}

function getGroupTreeNodeClass(node) {
  const target = getScopeTreeNode(node)
  return target?.node_type === 'host' ? 'group-scope-tree__host' : 'group-scope-tree__group'
}

function isHostTreeNode(node) {
  const target = getScopeTreeNode(node)
  return target?.node_type === 'host' && Number(target?.host_id || 0) > 0
}

function resolveAssetHostListPath() {
  for (const path of ASSET_HOST_ROUTE_CANDIDATES) {
    const resolved = router.resolve({ path })
    if (Array.isArray(resolved?.matched) && resolved.matched.length > 0) {
      return path
    }
  }
  return ''
}

function goToAssetHost(hostId, hostName = '') {
  const normalizedHostId = Number(hostId)
  if (!Number.isInteger(normalizedHostId) || normalizedHostId <= 0) {
    return
  }

  const hostListPath = resolveAssetHostListPath()
  if (!hostListPath) {
    message.warning('未找到资产主机页面路由')
    return
  }

  const searchText = String(hostName || '').trim()
  router.push({
    path: hostListPath,
    query: {
      host_id: String(normalizedHostId),
      ...(searchText ? { search: searchText } : {}),
    },
  })
}

function goToAssetHostByNode(node) {
  const target = getScopeTreeNode(node)
  goToAssetHost(target?.host_id, target?.title)
}

const checkedHostCountByNodeKey = computed(() => {
  const checkedKeySet = new Set((Array.isArray(groupScopeCheckedKeys.value) ? groupScopeCheckedKeys.value : []).map((item) => String(item)))
  const collector = {}
  collectCheckedHostCountByNode(groupTreeData.value, checkedKeySet, collector, false)
  return collector
})

const filteredGroupScopeViewTreeData = computed(() => {
  return filterTreeByKeyword(groupScopeViewTreeData.value, groupScopeViewKeyword.value)
})

const groupScopeViewExpandedKeys = computed(() => collectExpandedGroupKeys(filteredGroupScopeViewTreeData.value))

const scopePreviewExpandedKeys = computed(() => collectExpandedGroupKeys(filteredScopePreviewTreeData.value))

function getEditTreeNodeTitle(node) {
  const target = getScopeTreeNode(node)
  const rawTitle = stripGroupCountSuffix(target?.title || '-')
  const nodeType = target?.node_type || ''
  if (nodeType === 'group' || nodeType === 'virtual') {
    const key = String(target?.key || '')
    const count = Number(checkedHostCountByNodeKey.value[key] || 0)
    return count > 0 ? `${rawTitle}（${count}台）` : rawTitle
  }
  return rawTitle
}

function openGroupScopeViewer(record) {
  groupScopeViewKeyword.value = ''
  const executionTree = Array.isArray(record?.execution_scope_tree) ? record.execution_scope_tree : []
  if (executionTree.length > 0) {
    groupScopeViewTitle.value = `查看执行范围 / ${record?.name || '-'}`
    groupScopeViewTreeData.value = appendGroupHostCount(cloneTreeNodes(executionTree))
    const summaryHostCount = Number(record?.execution_scope_summary?.host_count || 0)
    groupScopeViewSummary.value = `当前范围：${summaryHostCount}台主机`
    groupScopeViewVisible.value = true
    return
  }

  const selectedGroupIds = Array.isArray(record?.selected_group_ids) ? record.selected_group_ids : []
  const selectedHostIds = Array.isArray(record?.selected_host_ids) ? record.selected_host_ids : []
  const isAllHostsScope = selectedGroupIds.length === 0 && selectedHostIds.length === 0

  groupScopeViewTitle.value = `查看主机组范围 / ${record?.name || '-'}`

  if (isAllHostsScope) {
    // 全主机场景：展示全树，但会自动去掉没有任何主机节点的空分组。
    const allTree = cloneTreeNodes(groupTreeData.value || [])
    const selectedAllGroups = new Set()
    const selectedAllHosts = new Set()
    const prunedTree = pruneTreeToCheckedNodes(allTree, selectedAllGroups, selectedAllHosts, true)
    groupScopeViewTreeData.value = appendGroupHostCount(prunedTree)
  } else {
    // 非全主机场景：仅展示勾选落点（勾选主机 + 勾选分组下的主机），并移除空分组。
    const fullTree = cloneTreeNodes(groupTreeData.value || [])
    const selectedGroupSet = toNumericSet(selectedGroupIds)
    const selectedHostSet = toNumericSet(selectedHostIds)
    const prunedTree = pruneTreeToCheckedNodes(fullTree, selectedGroupSet, selectedHostSet, false)
    groupScopeViewTreeData.value = appendGroupHostCount(prunedTree)
  }

  const scopeStats = collectTreeScopeStats(groupScopeViewTreeData.value)
  groupScopeViewSummary.value = `当前范围：${scopeStats.hostCount}台主机`

  groupScopeViewVisible.value = true
}

function syncGroupScopeCheckedKeys() {
  const selectedGroupIds = Array.isArray(taskForm.selected_group_ids) ? taskForm.selected_group_ids : []
  const selectedHostIds = Array.isArray(taskForm.selected_host_ids) ? taskForm.selected_host_ids : []
  // 后端约定：group/host 都为空时表示“全部主机”。
  if (selectedGroupIds.includes(ALL_GROUP_VALUE) || (selectedGroupIds.length === 0 && selectedHostIds.length === 0)) {
    taskForm.selected_group_ids = [ALL_GROUP_VALUE]
    taskForm.selected_host_ids = []
    groupScopeCheckedKeys.value = [ALL_GROUP_VALUE]
    return
  }
  groupScopeCheckedKeys.value = [
    ...selectedGroupIds.map((id) => `group-${id}`),
    ...selectedHostIds.map((id) => `host-${id}`),
  ]
}

function onGroupScopeCheck(checkedKeys) {
  const nextKeys = Array.isArray(checkedKeys) ? checkedKeys : checkedKeys?.checked || []
  if (nextKeys.includes(ALL_GROUP_VALUE)) {
    taskForm.selected_group_ids = [ALL_GROUP_VALUE]
    taskForm.selected_host_ids = []
    groupScopeCheckedKeys.value = [ALL_GROUP_VALUE]
    return
  }

  const { checkedGroupIds, checkedHostIds } = collectCheckedScope(nextKeys, ALL_GROUP_VALUE)
  taskForm.selected_group_ids = checkedGroupIds
  taskForm.selected_host_ids = checkedHostIds
  groupScopeCheckedKeys.value = nextKeys
}

function resetTaskForm() {
  taskForm.name = ''
  taskForm.template_type = 'playbook'
  taskForm.template = null
  taskForm.inventory = null
  taskForm.default_limit = ''
  taskForm.execution_timeout_seconds = 600
  taskForm.selected_host_ids = []
  taskForm.selected_group_ids = []
  taskForm.env_vars_text = ''
  taskForm.shell_args_text = ''
  taskForm.enabled = true
  taskForm.remark = ''
  // 权限提升配置
  taskForm.become_enabled = false
  taskForm.become_method = 'sudo'
  taskForm.become_user = 'root'
  groupScopeEditorVisible.value = false
  groupScopeCheckedKeys.value = []
  clearTaskLimitPrecheckTimer()
  taskLimitPrecheckSeq += 1
  taskLimitPrechecking.value = false
  taskLimitPrecheckOk.value = false
  taskLimitAllHosts.value = []
  taskLimitMatchedHosts.value = []
  taskLimitPrecheckMessage.value = '请选择 Inventory 后输入 Limit，系统将实时预检'
}

function handleTemplateTypeChange() {
  taskForm.template = null
  taskForm.shell_args_text = ''
}

function openCreateModal() {
  isCreateMode.value = true
  editingTaskId.value = null
  resetTaskForm()
  taskModalVisible.value = true
  scheduleTaskLimitPrecheck(0)
}

function openEditModal(record, options = {}) {
  const showTaskModal = options.showTaskModal !== false
  isCreateMode.value = false
  editingTaskId.value = record.id
  taskForm.name = record.name || ''
  if (Number(record?.shell_script_template) > 0) {
    taskForm.template_type = 'shell_script'
    taskForm.template = Number(record.shell_script_template)
    taskForm.shell_args_text = String(record.shell_parameters || '').trim()
    taskForm.env_vars_text = formatShellEnvText(record.env_vars || {})
  } else {
    taskForm.template_type = 'playbook'
    taskForm.template = Number(record.playbook_template || 0) || null
    taskForm.env_vars_text = JSON.stringify(record.env_vars || {}, null, 2)
    taskForm.shell_args_text = ''
  }
  taskForm.inventory = record.inventory || null
  // 检查已保存的 inventory 是否仍存在（可能已被删除或尚未配置）
  if (!taskForm.inventory) {
    if (record.id) {
      message.warning('该任务未配置 Inventory 或已被删除，请重新选择')
    }
  } else if (inventoryOptions.value.length > 0) {
    const exists = inventoryOptions.value.some((opt) => Number(opt.value) === Number(taskForm.inventory))
    if (!exists) {
      message.warning('该任务绑定的 Inventory 已被删除，请重新选择')
      taskForm.inventory = null
    }
  }
  taskForm.default_limit = record.default_limit || ''
  taskForm.execution_timeout_seconds = Number(record.execution_timeout_seconds) > 0
    ? Number(record.execution_timeout_seconds)
    : 600
  const selectedGroupIds = Array.isArray(record.selected_group_ids) ? [...record.selected_group_ids] : []
  taskForm.selected_host_ids = Array.isArray(record.selected_host_ids) ? [...record.selected_host_ids] : []
  taskForm.selected_group_ids = selectedGroupIds
  taskForm.enabled = !!record.enabled
  taskForm.remark = record.remark || ''
  // 权限提升配置
  taskForm.become_enabled = !!record.become_enabled
  taskForm.become_method = record.become_method || 'sudo'
  taskForm.become_user = record.become_user || 'root'
  taskModalVisible.value = showTaskModal
  groupScopeEditorVisible.value = false
  syncGroupScopeCheckedKeys()
  if (showTaskModal) {
    scheduleTaskLimitPrecheck(0)
  }
}

async function openGroupScopeEditor(record) {
  openEditModal(record, { showTaskModal: false })
  await nextTick()
  groupScopeEditorVisible.value = true
}

async function onGroupScopeEditorConfirm() {
  if (taskModalVisible.value) {
    groupScopeEditorVisible.value = false
    return
  }
  await submitTask()
  groupScopeEditorVisible.value = false
}

async function submitTask() {
  if (!taskForm.name || !String(taskForm.name).trim()) {
    message.error('请输入任务名称')
    return
  }
  if (!taskForm.template) {
    message.error('请选择模板')
    return
  }

  let envVars = {}
  let shellParameters = ''
  try {
    if (taskForm.template_type === 'shell_script') {
      envVars = parseShellEnvText(taskForm.env_vars_text)
      shellParameters = String(taskForm.shell_args_text || '').trim()
    } else {
      envVars = parseJsonObjectText(taskForm.env_vars_text, '环境变量 JSON')
    }
  } catch (error) {
    message.error(error.message)
    return
  }

  const timeoutSeconds = Number(taskForm.execution_timeout_seconds)
  if (!Number.isInteger(timeoutSeconds) || timeoutSeconds < 1 || timeoutSeconds > 14400) {
    message.error('执行超时必须是 1-14400 之间的整数秒')
    return
  }

  const payload = {
    name: String(taskForm.name).trim(),
    playbook_template: taskForm.template_type === 'playbook' ? Number(taskForm.template) : null,
    shell_script_template: taskForm.template_type === 'shell_script' ? Number(taskForm.template) : null,
    inventory: Number(taskForm.inventory) > 0 ? Number(taskForm.inventory) : null,
    default_limit: String(taskForm.default_limit || '').trim(),
    execution_timeout_seconds: timeoutSeconds,
    selected_host_ids: [],
    selected_group_ids: [],
    env_vars: envVars,
    shell_parameters: shellParameters,
    enabled: !!taskForm.enabled,
    remark: taskForm.remark || '',
    // 权限提升配置
    become_enabled: !!taskForm.become_enabled,
    become_method: taskForm.become_method || 'sudo',
    become_user: taskForm.become_user || 'root',
  }

  modalSubmitting.value = true
  try {
    if (isCreateMode.value) {
      await createTask(payload)
      message.success('任务创建成功')
    } else {
      await updateTask(editingTaskId.value, payload)
      message.success('任务更新成功')
    }
    taskModalVisible.value = false
    await loadTasks(false)
  } catch (error) {
    message.error(resolveTaskSubmitErrorMessage(error))
  } finally {
    modalSubmitting.value = false
  }
}

async function removeTask(record) {
  await deleteTask(record.id)
  message.success('任务已删除')
  await loadTasks(false)
}

async function onChangeTaskStatus(checked, record) {
  if (!record?.id) {
    return
  }

  if (!canEditTask.value) {
    message.warning('没有状态修改权限')
    return
  }

  const targetEnabled = checked === true
  const originalEnabled = record.enabled === true
  if (targetEnabled === originalEnabled) {
    return
  }

  taskStatusUpdatingId.value = record.id
  record.enabled = targetEnabled
  try {
    await updateTask(record.id, { enabled: targetEnabled })
    message.success('状态修改成功')
  } catch (error) {
    record.enabled = originalEnabled
    message.error(error?.response?.data?.msg || error?.message || '状态修改失败')
  } finally {
    taskStatusUpdatingId.value = null
  }
}

function openDeleteTaskConfirm(record) {
  const taskName = String(record?.name || '').trim() || `#${record?.id || '-'}`
  openDeleteConfirm({
    title: '确认删除任务',
    summary: '删除后不可恢复，请确认影响清单。',
    items: [`任务: ${taskName}`],
    onConfirm: () => removeTask(record),
  })
}

function clearRunNowPrecheckTimer() {
  if (runNowPrecheckTimer) {
    clearTimeout(runNowPrecheckTimer)
    runNowPrecheckTimer = null
  }
}

function clearTaskLimitPrecheckTimer() {
  if (taskLimitPrecheckTimer) {
    clearTimeout(taskLimitPrecheckTimer)
    taskLimitPrecheckTimer = null
  }
}

const taskLimitPrecheckText = computed(() => {
  if (taskLimitPrechecking.value && taskLimitPrecheckMessage.value === '正在预检...') {
    return '正在预检...'
  }
  return taskLimitPrecheckMessage.value
})

async function doTaskLimitPrecheck() {
  if (!taskModalVisible.value) {
    return
  }

  const inventoryId = Number(taskForm.inventory)
  if (!Number.isInteger(inventoryId) || inventoryId <= 0) {
    taskLimitPrecheckOk.value = false
    taskLimitPrechecking.value = false
    taskLimitAllHosts.value = []
    taskLimitMatchedHosts.value = []
    taskLimitPrecheckMessage.value = '请选择 Inventory 后输入 Limit，系统将实时预检'
    return
  }

  const currentSeq = ++taskLimitPrecheckSeq
  taskLimitPrechecking.value = true
  try {
    const limitText = String(taskForm.default_limit || '').trim()
    const baseRes = await precheckInventoryLimit(inventoryId, { limit: '' })
    if (currentSeq !== taskLimitPrecheckSeq) {
      return
    }

    let data = baseRes?.data?.data || {}
    if (limitText) {
      const narrowedRes = await precheckInventoryLimit(inventoryId, { limit: limitText })
      if (currentSeq !== taskLimitPrecheckSeq) {
        return
      }
      data = narrowedRes?.data?.data || {}
    }

    const baseData = baseRes?.data?.data || {}
    taskLimitPrecheckOk.value = !!data.ok
    taskLimitAllHosts.value = Array.isArray(baseData.matched_hosts_preview) ? baseData.matched_hosts_preview : []
    taskLimitMatchedHosts.value = Array.isArray(data.matched_hosts_preview) ? data.matched_hosts_preview : []
    taskLimitPrecheckMessage.value = data.message || '预检完成'
  } catch (error) {
    if (currentSeq !== taskLimitPrecheckSeq) {
      return
    }
    taskLimitPrecheckOk.value = false
    taskLimitAllHosts.value = []
    taskLimitMatchedHosts.value = []
    taskLimitPrecheckMessage.value = error?.message || '预检失败，请稍后重试'
  } finally {
    if (currentSeq === taskLimitPrecheckSeq) {
      taskLimitPrechecking.value = false
    }
  }
}

function scheduleTaskLimitPrecheck(delay = 300) {
  clearTaskLimitPrecheckTimer()
  taskLimitPrecheckTimer = setTimeout(() => {
    doTaskLimitPrecheck()
  }, delay)
}

function handleTaskLimitHostClick(item) {
  goToAssetHost(item?.host_id, item?.host_name)
}

function handleTaskLimitToggle(item) {
  const token = resolveMatchedHostLimitToken(item)
  taskForm.default_limit = toggleLimitToken(taskForm.default_limit, token)
}

function handleTaskLimitRemoveToken(token) {
  taskForm.default_limit = removeLimitToken(taskForm.default_limit, token)
}

async function doRunNowPrecheck() {
  if (!runNowModalVisible.value || !runNowTask.value?.id) {
    return
  }

  // 检查任务是否有inventory，没有则不能预检
  if (!runNowTask.value.inventory) {
    runNowPrecheckOk.value = false
    runNowHostCount.value = 0
    runNowAllHosts.value = []
    runNowMatchedHosts.value = []
    runNowPrechecking.value = false
    runNowPrecheckMessage.value = '任务未配置 Inventory，无法执行'
    return
  }

  const currentSeq = ++runNowPrecheckSeq
  runNowPrechecking.value = true

  try {
    const limitText = String(runNowLimit.value || '').trim()
    const baseRes = await precheckTaskRun(runNowTask.value.id, { limit: '' })
    if (currentSeq !== runNowPrecheckSeq) {
      return
    }

    let precheckData = baseRes?.data?.data || {}
    if (limitText) {
      const narrowedRes = await precheckTaskRun(runNowTask.value.id, { limit: limitText })
      if (currentSeq !== runNowPrecheckSeq) {
        return
      }
      precheckData = narrowedRes?.data?.data || {}
    }

    const baseData = baseRes?.data?.data || {}
    const hostCount = Number(precheckData.resolved_host_count || 0)
    runNowHostCount.value = hostCount
    runNowEffectiveLimit.value = String(precheckData.effective_limit || limitText)
    runNowAllHosts.value = Array.isArray(baseData.matched_hosts_preview)
      ? baseData.matched_hosts_preview
      : []
    runNowMatchedHosts.value = Array.isArray(precheckData.matched_hosts_preview)
      ? precheckData.matched_hosts_preview
      : []

    if (precheckData.ok) {
      runNowPrecheckOk.value = true
      // 直接使用后端 message，包含离线主机等警告信息
      runNowPrecheckMessage.value = precheckData.message || `预检通过，可执行主机 ${hostCount} 台`
    } else {
      runNowPrecheckOk.value = false
      runNowPrecheckMessage.value = precheckData.message || '任务预检未通过'
    }
  } catch (error) {
    if (currentSeq !== runNowPrecheckSeq) {
      return
    }
    runNowPrecheckOk.value = false
    runNowHostCount.value = 0
    runNowAllHosts.value = []
    runNowMatchedHosts.value = []
    runNowPrecheckMessage.value = error?.message || '预检失败，请稍后重试'
  } finally {
    if (currentSeq === runNowPrecheckSeq) {
      runNowPrechecking.value = false
    }
  }
}

function scheduleRunNowPrecheck(delay = 300) {
  clearRunNowPrecheckTimer()
  runNowPrecheckTimer = setTimeout(() => {
    doRunNowPrecheck()
  }, delay)
}

function handleRunNowHostClick(item) {
  goToAssetHost(item?.host_id, item?.host_name)
}

function handleRunNowLimitToggle(item) {
  const token = resolveMatchedHostLimitToken(item)
  runNowLimit.value = toggleLimitToken(runNowLimit.value, token)
}

function handleRunNowRemoveToken(token) {
  runNowLimit.value = removeLimitToken(runNowLimit.value, token)
}

function openRunNowModal(record) {
  runNowTask.value = record
  runNowLimit.value = String(record?.default_limit || '').trim()
  runNowShellArgs.value = Number(record?.shell_script_template || 0) > 0
    ? String(record?.shell_parameters || '').trim()
    : ''
  runNowHostCount.value = 0
  runNowEffectiveLimit.value = ''
  runNowAllHosts.value = []
  runNowMatchedHosts.value = []
  runNowPrecheckOk.value = false
  runNowPrecheckMessage.value = '正在预检...'
  runNowModalVisible.value = true
  scheduleRunNowPrecheck(0)
}

function closeRunNowModal() {
  runNowModalVisible.value = false
  runNowSubmitting.value = false
  runNowTask.value = null
  runNowShellArgs.value = ''
  runNowAllHosts.value = []
  runNowMatchedHosts.value = []
  clearRunNowPrecheckTimer()
  runNowPrecheckSeq += 1
}

const runNowPrecheckText = computed(() => {
  if (runNowPrechecking.value && runNowPrecheckMessage.value === '正在预检...') {
    return '正在预检...'
  }
  return runNowPrecheckMessage.value
})

async function confirmRunNow() {
  if (!runNowTask.value?.id) {
    return
  }
  if (!runNowPrecheckOk.value) {
    message.error(runNowPrecheckMessage.value || '任务预检未通过，无法执行')
    return
  }

  runningTaskId.value = runNowTask.value.id
  runNowSubmitting.value = true
  try {
    const payload = buildRunNowPayload(runNowLimit.value, runNowIsShellTask.value, runNowShellArgs.value)

    const res = await runTaskNow(runNowTask.value.id, payload)
    const createdJobId = Number(res?.data?.data?.id || 0)
    message.success('任务已提交，正在后台执行')
    goToLogs(runNowTask.value, createdJobId)
    closeRunNowModal()
  } finally {
    runningTaskId.value = null
    runNowSubmitting.value = false
  }
}

function goToLogs(record = null, jobId = null) {
  if (record && record.id) {
    const query = {
      task_id: String(record.id),
      task_name: record.name || '',
    }
    if (Number.isInteger(Number(jobId)) && Number(jobId) > 0) {
      query.job_id = String(Number(jobId))
    }
    router.push({ path: '/sys/automation/logs', query })
    return
  }
  if (Number.isInteger(Number(jobId)) && Number(jobId) > 0) {
    router.push({ path: '/sys/automation/logs', query: { job_id: String(Number(jobId)) } })
    return
  }
  router.push('/sys/automation/logs')
}

function goToPlaybookTemplate(record) {
  router.push(buildAutomationTemplateRoute(record))
}

function goToInventory(record) {
  router.push(buildAutomationInventoryRoute(record))
}

function viewTaskLogs(record) {
  goToLogs(record)
}

function handleTaskTableChange(page, _filters, sorter) {
  taskPagination.current = page.current
  taskPagination.pageSize = page.pageSize

  const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
  const allowedFields = ['name', 'enabled', 'update_time']
  if (nextSorter?.field && allowedFields.includes(nextSorter.field) && nextSorter.order) {
    taskSort.field = nextSorter.field
    taskSort.order = nextSorter.order
  } else {
    taskSort.field = null
    taskSort.order = null
  }

  loadTasks(false)
}

function reloadAll() {
  loadPlaybooks()
  loadShellScriptTemplates()
  loadTasks(false)
}

watch(groupScopeEditorVisible, (visible) => {
  if (!visible) {
    return
  }
  nextTick(() => {
    if (groupScopeEditorTreeWrapRef.value) {
      groupScopeEditorTreeWrapRef.value.scrollTop = 0
    }
  })
})

watch(runNowLimit, () => {
  if (!runNowModalVisible.value) {
    return
  }
  runNowPrecheckMessage.value = '正在预检...'
  scheduleRunNowPrecheck(300)
})

watch(
  () => [taskForm.default_limit, taskForm.inventory, taskModalVisible.value],
  () => {
    if (!taskModalVisible.value) {
      return
    }
    taskLimitPrecheckMessage.value = '正在预检...'
    scheduleTaskLimitPrecheck(300)
  }
)

onBeforeUnmount(() => {
  clearRunNowPrecheckTimer()
  clearTaskLimitPrecheckTimer()
})

onMounted(async () => {
  await loadPlaybooks()
  await loadShellScriptTemplates()
  await loadInventories()
  await loadTasks(true)
  await loadGroupTree()
  await openTaskFromRouteQuery()
})
</script>

<style scoped>
.automation-page {
  padding: 2px;
}

.tools {
  margin-bottom: 12px;
}

.right-actions {
  display: flex;
  justify-content: flex-end;
}

.block-card {
  margin-bottom: 12px;
}

.json-cell {
  max-width: 220px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.group-list-cell {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}

.scope-compact-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  max-width: 240px;
}

.scope-limit-tag {
  margin-inline-end: 0;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.scope-limit-empty {
  color: rgba(0, 0, 0, 0.45);
}

.scope-match-empty {
  color: rgba(0, 0, 0, 0.45);
  line-height: 1.2;
}

.scope-host-count-link {
  padding: 0;
  height: auto;
}

.scope-popover-list {
  max-width: 460px;
  max-height: 260px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.scope-popover-item {
  font-size: 12px;
  line-height: 1.4;
  word-break: break-all;
}

.scope-popover-more {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
  margin-top: 2px;
}

.group-list-edit-btn {
  padding-inline: 4px;
}

.group-list-view-btn {
  padding-inline: 4px;
}

.group-list-tag {
  margin-inline-end: 0;
}

.group-list-more {
  margin-inline-end: 0;
  cursor: pointer;
  color: #1677ff;
  border-color: #91caff;
  background: #f0f8ff;
}

.group-list-popover {
  max-width: 360px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.scope-link-button {
  padding: 0;
}

.scope-tree-link {
  padding: 0;
  height: auto;
}

.task-code-link {
  padding: 0;
  height: auto;
}

.group-scope-editor {
  width: 100%;
  min-height: 520px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.group-scope-viewer {
  width: 100%;
  min-height: 520px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.group-scope-viewer__summary {
  font-size: 13px;
  color: rgba(0, 0, 0, 0.75);
  background: #f7faff;
  border: 1px solid #d6e4ff;
  border-radius: 6px;
  padding: 6px 10px;
}

.group-scope-viewer__search {
  max-width: 360px;
}

.group-scope-editor__header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.group-scope-editor__title {
  font-size: 14px;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.88);
}

.group-scope-editor__desc {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.45);
}

.group-scope-editor__tree-wrap {
  width: 100%;
  min-height: 460px;
  max-height: 620px;
  overflow: auto;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  padding: 12px 16px;
  background: #fff;
}

.group-scope-tree {
  width: 100%;
}

:deep(.group-scope-tree .ant-tree) {
  width: 100%;
}

:deep(.group-scope-tree .ant-tree-node-content-wrapper) {
  width: 100%;
}

.group-scope-tree__group {
  font-weight: 500;
}

.group-scope-tree__host {
  color: rgba(0, 0, 0, 0.75);
}

:deep(.execution-scope-popover .ant-popover-inner) {
  max-width: 520px;
}

.execution-scope-popover-content {
  max-width: 480px;
  max-height: 360px;
  overflow: auto;
}

.run-now-preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  color: rgba(0, 0, 0, 0.75);
}

.run-now-preview-body {
  max-height: 240px;
  overflow: auto;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  padding: 8px 10px;
  background: #fafafa;
}

.run-now-preview-list {
  margin: 0;
  padding-left: 18px;
}

.run-now-preview-list li {
  line-height: 1.6;
  word-break: break-all;
}

.task-limit-preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 8px;
  margin-bottom: 8px;
  color: rgba(0, 0, 0, 0.75);
}

.task-limit-preview-body {
  max-height: 180px;
  overflow: auto;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  padding: 8px 10px;
  background: #fafafa;
}

.task-limit-preview-list {
  margin: 0;
  padding-left: 18px;
}

.task-limit-preview-list li {
  line-height: 1.6;
  word-break: break-all;
}
</style>
