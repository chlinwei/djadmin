<template>
  <div class="automation-page">
    <a-row class="tools" :gutter="12">
      <a-col :span="16">
        <a-input-search
          v-model:value="taskKeyword"
          placeholder="搜索任务名称 / 任务编码"
          allow-clear
          enter-button
          @search="loadTasks(true)"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-button size="large" @click="openCreateModal" v-permission="'automation:tasks:create'">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp新增任务</span>
          </a-button>
          <a-button @click="goToLogs" v-permission="'automation:jobs:view'">
            <FontAwesomeIcon :icon="['fas', 'list']" />
            <span>&nbsp;任务运行记录</span>
          </a-button>
          <a-button @click="goToWorkflow" v-permission="'automation:workflow:view'">
            <FontAwesomeIcon :icon="['fas', 'diagram-project']" />
            <span>&nbsp;Workflow编排</span>
          </a-button>
          <a-button type="primary" ghost :loading="taskLoading || playbookLoading" @click="reloadAll">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="taskLoading || playbookLoading" />
            <span>&nbsp;刷新</span>
          </a-button>
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
          <template v-else-if="column.key === 'code'">
            <span>{{ record.code || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">
              {{ record.enabled ? '启用' : '禁用' }}
            </a-tag>
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
            </div>
          </template>
          <template v-else-if="column.key === 'env_vars'">
            <div class="json-cell">{{ formatJsonCell(record.env_vars) }}</div>
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
              <a-popconfirm
                title="确认删除该任务吗？"
                ok-text="确认"
                cancel-text="取消"
                @confirm="removeTask(record)"
              >
                <a-button size="small" type="primary" danger v-permission="'automation:tasks:delete'">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-popconfirm>
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
          <a-col :span="12">
            <a-form-item label="任务编码" required>
              <a-input v-model:value="taskForm.code" placeholder="例如：prod-health-check" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-row :gutter="12">
          <a-col :span="8">
            <a-form-item label="选择模板" required>
              <a-select
                v-model:value="taskForm.template"
                :options="playbookOptions"
                show-search
                optionFilterProp="label"
                :getPopupContainer="getDropdownContainer"
                placeholder="请选择模板"
              />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="选择Inventory">
              <a-select
                v-model:value="taskForm.inventory"
                :options="inventoryOptions"
                show-search
                optionFilterProp="label"
                :getPopupContainer="getDropdownContainer"
                placeholder="可选"
                allow-clear
              />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="启用状态">
              <a-switch v-model:checked="taskForm.enabled" checked-children="启用" un-checked-children="禁用" />
            </a-form-item>
          </a-col>
        </a-row>

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

        <a-form-item label="环境变量 JSON（可选）">
          <a-textarea
            v-model:value="taskForm.env_vars_text"
            :rows="6"
            placeholder='例如: {"env":"prod","batch":20}'
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
                :getPopupContainer="getDropdownContainer"
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
                placeholder="root"
              />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="备注">
          <a-textarea v-model:value="taskForm.remark" :rows="2" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      :title="groupScopeViewTitle"
      :open="groupScopeViewVisible"
      :width="980"
      :footer="null"
      @cancel="groupScopeViewVisible = false"
    >
      <div class="group-scope-viewer">
        <div class="group-scope-editor__desc">仅展示当前任务已勾选的主机组/主机，未勾选节点已隐藏</div>
        <div class="group-scope-viewer__summary">{{ groupScopeViewSummary }}</div>
        <a-input
          v-model:value="groupScopeViewKeyword"
          allow-clear
          placeholder="搜索分组/主机/IP"
          class="group-scope-viewer__search"
        />
        <div class="group-scope-editor__tree-wrap">
          <a-tree
            v-if="filteredGroupScopeViewTreeData.length > 0"
            block-node
            :expanded-keys="groupScopeViewExpandedKeys"
            :auto-expand-parent="true"
            :tree-data="filteredGroupScopeViewTreeData"
            :selectable="false"
            :show-line="{ showLeafIcon: false }"
            class="group-scope-tree"
          >
            <template #title="node">
              <a-button
                v-if="isHostTreeNode(node)"
                type="link"
                size="small"
                class="scope-host-link-btn"
                @click.stop="goToAssetHostByNode(node)"
              >
                <span :class="getGroupTreeNodeClass(node)">{{ getScopeTreeNode(node).title }}</span>
              </a-button>
              <span v-else :class="getGroupTreeNodeClass(node)">{{ getScopeTreeNode(node).title }}</span>
            </template>
          </a-tree>
          <a-empty v-else description="暂无已勾选范围" />
        </div>
      </div>
    </a-modal>

    <a-modal
      title="运行任务"
      :open="runNowModalVisible"
      :confirmLoading="runNowSubmitting"
      ok-text="立即执行"
      cancel-text="取消"
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

    <a-modal
      :title="scopePreviewTitle"
      :open="scopePreviewModalVisible"
      :width="760"
      :footer="null"
      @cancel="closeScopePreviewModal"
    >
      <div class="group-scope-viewer">
        <div class="group-scope-editor__desc">按实例名 / IP / 主机组路径搜索</div>
        <div class="group-scope-viewer__summary">
          共 {{ scopePreviewTotal }} 台，当前展示 {{ scopePreviewVisibleHostCount }} 台
        </div>
        <a-input
          v-model:value="scopePreviewKeyword"
          allow-clear
          placeholder="搜索实例名 / IP / 主机组路径"
          class="group-scope-viewer__search"
        />
        <div class="group-scope-editor__tree-wrap scope-preview-list-wrap">
          <a-tree
            v-if="filteredScopePreviewTreeData.length > 0"
            block-node
            :expanded-keys="scopePreviewExpandedKeys"
            :auto-expand-parent="true"
            :tree-data="filteredScopePreviewTreeData"
            :selectable="false"
            :show-line="{ showLeafIcon: false }"
            class="group-scope-tree"
          >
            <template #title="node">
              <a-button
                v-if="isHostTreeNode(node)"
                type="link"
                size="small"
                class="scope-host-link-btn"
                @click.stop="goToAssetHostByNode(node)"
              >
                <span :class="getGroupTreeNodeClass(node)">{{ getScopeTreeNode(node).title }}</span>
              </a-button>
              <span v-else :class="getGroupTreeNodeClass(node)">{{ getScopeTreeNode(node).title }}</span>
            </template>
          </a-tree>
          <a-empty v-else description="暂无匹配主机" />
        </div>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import {
  getPlaybookList,
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
import { buildScopeSummaryText, flattenGroupPathMap } from './scopeSummary'
import { buildAutomationInventoryRoute, buildAutomationPlaybookRoute } from './navigation'
import ScopePrecheckPanel from './components/ScopePrecheckPanel.vue'
import {
  LIMIT_INPUT_PLACEHOLDER,
  removeLimitToken,
  resolveMatchedHostLimitToken,
  toggleLimitToken,
} from './utils/scopeHelpers'

const route = useRoute()
const router = useRouter()
const getDropdownContainer = (triggerNode) => {
  if (triggerNode && typeof triggerNode.closest === 'function') {
    const modal = triggerNode.closest('.ant-modal')
    if (modal) {
      return modal
    }
  }
  return triggerNode?.parentNode || document.body
}
const ASSET_HOST_ROUTE_CANDIDATES = ['/assets/hosts', '/assets/host', '/assets/hosts/index', '/assets/host/index']
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
const inventories = ref([])
const inventoryOptions = ref([])

const groupLoading = ref(false)
const groupTreeData = ref([])
const groupMap = ref({})
const groupPathMap = ref({})
const groupScopeCheckedKeys = ref([])
const ALL_GROUP_VALUE = '__all__'
const ALL_GROUP_TITLE = 'All（全部主机）'
const GROUP_TAG_PREVIEW_COUNT = 2

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
const runNowModalVisible = ref(false)
const runNowSubmitting = ref(false)
const runNowPrechecking = ref(false)
const runNowTask = ref(null)
const runNowLimit = ref('')
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
  code: '',
  template: null,
  inventory: null,
  default_limit: '',
  selected_host_ids: [],
  selected_group_ids: [],
  env_vars_text: '',
  enabled: true,
  remark: '',
  // 权限提升配置
  become_enabled: false,
  become_method: 'sudo',
  become_user: 'root',
})

const taskColumns = [
  { title: '任务名称', dataIndex: 'name', key: 'name', width: 160 },
  { title: '任务编码', dataIndex: 'code', key: 'code', width: 170 },
  { title: 'Inventory', dataIndex: 'inventory_name', key: 'inventory_name', width: 180 },
  { title: '模板', dataIndex: 'template_name', key: 'template_name', width: 140 },
  { title: '执行范围', dataIndex: 'selected_group_ids', key: 'selected_group_ids', width: 260 },
  { title: '环境变量', dataIndex: 'env_vars', key: 'env_vars', width: 220 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 120 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 160 },
  { title: '操作', key: 'action', width: 320, fixed: 'right' },
]

function parseJsonText(text, fieldLabel) {
  if (!text || !String(text).trim()) {
    return {}
  }
  try {
    const parsed = JSON.parse(text)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed
    }
    throw new Error('必须是 JSON 对象')
  } catch (error) {
    throw new Error(`${fieldLabel} 格式错误: ${error.message}`)
  }
}

function flattenGroupTree(nodes, collector = {}) {
  (nodes || []).forEach((node) => {
    collector[node.id] = node.name
    flattenGroupTree(node.children || [], collector)
  })
  return collector
}

function getTaskScopeSummaryText(record) {
  return buildScopeSummaryText({
    selectedGroupIds: record?.selected_group_ids,
    selectedHostIds: record?.selected_host_ids,
    groupPathMap: groupPathMap.value,
    groupNameMap: groupMap.value,
  })
}

function normalizeGroupTree(nodes, hostChildrenMap = { groupedHosts: {}, standaloneHosts: [] }) {
  return (nodes || []).map((node) => ({
    key: `group-${node.id}`,
    value: `group-${node.id}`,
    title: node.name,
    node_type: 'group',
    children: [
      ...normalizeGroupTree(node.children || [], hostChildrenMap),
      ...(hostChildrenMap.groupedHosts[node.id] || []),
    ],
  }))
}

function buildGroupTreeWithAll(nodes, hosts = []) {
  const hostChildrenMap = buildHostChildrenMap(hosts)
  const normalizedGroups = normalizeGroupTree(nodes, hostChildrenMap)
  const prunedGroups = pruneGroupsWithoutHosts(normalizedGroups)
  return [{
    key: ALL_GROUP_VALUE,
    value: ALL_GROUP_VALUE,
    title: 'All',
    node_type: 'virtual',
    children: [...prunedGroups, ...hostChildrenMap.standaloneHosts],
  }]
}

function pruneGroupsWithoutHosts(nodes) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const result = []

  normalizedNodes.forEach((node) => {
    if (!node || typeof node !== 'object') {
      return
    }

    if (node.node_type === 'host') {
      result.push({ ...node, children: undefined })
      return
    }

    const children = pruneGroupsWithoutHosts(node.children || [])
    if (children.length > 0) {
      result.push({ ...node, children })
    }
  })

  return result
}

function buildHostChildrenMap(hosts) {
  const groupedHosts = {}
  const standaloneHosts = []
  ;(hosts || []).forEach((item) => {
    if (!item || typeof item !== 'object') {
      return
    }
    const hostId = Number(item.id)
    if (!Number.isInteger(hostId) || hostId <= 0) {
      return
    }
    const hostNode = {
      key: `host-${hostId}`,
      value: `host-${hostId}`,
      title: formatHostTreeTitle(item),
      node_type: 'host',
      isLeaf: true,
      host_id: hostId,
      group_id: item.group_id || null,
    }
    if (item.group_id) {
      if (!groupedHosts[item.group_id]) {
        groupedHosts[item.group_id] = []
      }
      groupedHosts[item.group_id].push(hostNode)
      return
    }
    standaloneHosts.push(hostNode)
  })
  return { groupedHosts, standaloneHosts }
}

function formatHostTreeTitle(host) {
  if (!host || typeof host !== 'object') {
    return '-'
  }
  const instanceName = String(host.instance_name || '').trim()
  const ip = host.ip || '-'
  if (instanceName && instanceName !== ip) {
    return `${instanceName}(${ip})`
  }
  return ip
}

function formatMatchedHostTitle(host) {
  const displayName = String(host?.host_name || '').trim()
  const ip = String(host?.host_ip || '').trim()
  if (displayName && ip && displayName !== ip) {
    return `${displayName}(${ip})`
  }
  return displayName || ip || '-'
}

function formatJsonCell(value) {
  if (!value || typeof value !== 'object') {
    return '-'
  }
  const text = JSON.stringify(value)
  return text.length > 64 ? `${text.slice(0, 64)}...` : text
}

const filteredScopePreviewTreeData = computed(() => {
  return filterTreeByKeyword(scopePreviewTreeData.value, scopePreviewKeyword.value)
})

const scopePreviewVisibleHostCount = computed(() => {
  const stats = collectTreeScopeStats(filteredScopePreviewTreeData.value)
  return stats.hostCount
})

function buildScopePreviewTreeData(hosts) {
  const rootNodes = []
  const groupNodeMap = {}
  const rootHosts = []

  const ensureGroupNode = (pathParts) => {
    if (!Array.isArray(pathParts) || pathParts.length === 0) {
      return null
    }
    const pathKey = pathParts.join('/')
    const groupKey = `scope-preview-group-${pathKey}`
    if (groupNodeMap[groupKey]) {
      return groupNodeMap[groupKey]
    }

    const parentParts = pathParts.slice(0, -1)
    const parentNode = ensureGroupNode(parentParts)
    const label = pathParts[pathParts.length - 1]
    const nextNode = {
      key: groupKey,
      value: groupKey,
      title: label,
      node_type: 'group',
      children: [],
    }

    groupNodeMap[groupKey] = nextNode
    if (parentNode) {
      parentNode.children.push(nextNode)
    } else {
      rootNodes.push(nextNode)
    }

    return nextNode
  }

  ;(Array.isArray(hosts) ? hosts : []).forEach((item) => {
    if (!item || typeof item !== 'object') {
      return
    }
    const hostId = Number(item.host_id)
    if (!Number.isInteger(hostId) || hostId <= 0) {
      return
    }
    const groupPathText = String(item.group_path || item.group_name || '').trim()
    const hostTitle = formatMatchedHostTitle(item)
    const hostNode = {
      key: `scope-preview-host-${hostId}`,
      value: `scope-preview-host-${hostId}`,
      title: hostTitle,
      node_type: 'host',
      host_id: hostId,
      isLeaf: true,
    }

    if (!groupPathText) {
      rootHosts.push(hostNode)
      return
    }

    const pathParts = groupPathText
      .split('/')
      .map((part) => String(part || '').trim())
      .filter(Boolean)
    if (pathParts.length === 0) {
      rootHosts.push(hostNode)
      return
    }

    const leafGroupNode = ensureGroupNode(pathParts)
    if (!leafGroupNode) {
      rootHosts.push(hostNode)
      return
    }
    leafGroupNode.children.push(hostNode)
  })

  const sortNodes = (nodes) => {
    return (Array.isArray(nodes) ? nodes : [])
      .map((node) => {
        if (!node || typeof node !== 'object') {
          return node
        }
        const children = Array.isArray(node.children) ? sortNodes(node.children) : []
        return { ...node, children }
      })
      .sort((a, b) => {
        const aType = a?.node_type || ''
        const bType = b?.node_type || ''
        if (aType === bType) {
          return String(a?.title || '').localeCompare(String(b?.title || ''))
        }
        if (aType === 'group') {
          return -1
        }
        if (bType === 'group') {
          return 1
        }
        return String(a?.title || '').localeCompare(String(b?.title || ''))
      })
  }

  return appendGroupHostCount(sortNodes([...rootNodes, ...rootHosts]))
}

function openScopePreviewModal(record) {
  scopePreviewKeyword.value = ''
  scopePreviewHosts.value = Array.isArray(record?.limit_preview_hosts) ? [...record.limit_preview_hosts] : []
  scopePreviewTreeData.value = buildScopePreviewTreeData(scopePreviewHosts.value)
  scopePreviewTotal.value = Number(record?.limit_preview_total || 0)
  scopePreviewTitle.value = `执行范围主机预览 / ${record?.name || '-'} (${record?.code || '-'})`
  scopePreviewModalVisible.value = true
}

function closeScopePreviewModal() {
  scopePreviewModalVisible.value = false
  scopePreviewKeyword.value = ''
}

function getScopeTreeNode(node) {
  if (!node || typeof node !== 'object') {
    return {}
  }
  return node.dataRef || node
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
    const res = await getTaskList({
      page: taskPagination.current,
      page_size: taskPagination.pageSize,
      ordering: '-id',
      search: taskKeyword.value || undefined,
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
    groupMap.value = flattenGroupTree(groupData, {})
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

function collectCheckedHostCountByNode(nodes, checkedKeySet, collector = {}, inheritedSelected = false) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  let subtreeHostCount = 0

  normalizedNodes.forEach((node) => {
    if (!node || typeof node !== 'object') {
      return
    }

    const key = String(node.key || '')
    const nodeType = node.node_type || ''
    const selfSelected = inheritedSelected || checkedKeySet.has(key)

    if (nodeType === 'host') {
      if (selfSelected) {
        subtreeHostCount += 1
      }
      return
    }

    const childCount = collectCheckedHostCountByNode(node.children || [], checkedKeySet, collector, selfSelected)
    collector[key] = childCount
    subtreeHostCount += childCount
  })

  return subtreeHostCount
}

const checkedHostCountByNodeKey = computed(() => {
  const checkedKeySet = new Set((Array.isArray(groupScopeCheckedKeys.value) ? groupScopeCheckedKeys.value : []).map((item) => String(item)))
  const collector = {}
  collectCheckedHostCountByNode(groupTreeData.value, checkedKeySet, collector, false)
  return collector
})

function filterTreeByKeyword(nodes, keyword) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const kw = String(keyword || '').trim().toLowerCase()
  if (!kw) {
    return normalizedNodes
  }

  const walk = (items, keepAllDescendants = false) => {
    const result = []
    items.forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const rawChildren = Array.isArray(node.children) ? node.children : []
      const titleText = String(stripGroupCountSuffix(node.title || '')).toLowerCase()
      const selfMatched = keepAllDescendants || titleText.includes(kw)
      const children = walk(rawChildren, selfMatched)
      if (selfMatched || children.length > 0) {
        result.push({ ...node, children })
      }
    })
    return result
  }

  return walk(normalizedNodes, false)
}

function collectExpandedGroupKeys(nodes) {
  const keys = []
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const keyText = String(node.key || '')
      const nodeType = String(node.node_type || '')
      const isHostNode = nodeType === 'host' || keyText.includes('host-')
      if (!isHostNode && keyText) {
        keys.push(keyText)
      }
      walk(node.children || [])
    })
  }
  walk(nodes)
  return keys
}

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

function cloneTreeNodes(nodes) {
  return (nodes || []).map((item) => ({
    ...item,
    children: cloneTreeNodes(item.children || []),
  }))
}

function stripGroupCountSuffix(title) {
  return String(title || '').replace(/\s*[（(](?:\d+个|\d+台|\d+组，\d+台)[）)]\s*$/, '')
}

function appendGroupHostCount(nodes) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []

  const walk = (items) => {
    let hostCount = 0
    let groupCount = 0
    const mapped = items.map((node) => {
      if (!node || typeof node !== 'object') {
        return node
      }

      const nodeType = node.node_type || ''
      if (nodeType === 'host') {
        hostCount += 1
        return { ...node, children: undefined }
      }

      const children = Array.isArray(node.children) ? node.children : []
      const childResult = walk(children)
      const baseTitle = stripGroupCountSuffix(node.title || '-')
      let nextTitle = baseTitle

      if (nodeType === 'group') {
        groupCount += 1
        nextTitle = childResult.hostCount > 0
          ? `${baseTitle}（${childResult.hostCount}台）`
          : baseTitle
      }

      if (nodeType === 'virtual') {
        nextTitle = `${baseTitle}（${childResult.groupCount}组，${childResult.hostCount}台）`
      }

      hostCount += childResult.hostCount
      groupCount += childResult.groupCount
      return {
        ...node,
        title: nextTitle,
        children: childResult.nodes,
      }
    })

    return { nodes: mapped, hostCount, groupCount }
  }

  return walk(normalizedNodes).nodes
}

function collectTreeScopeStats(nodes) {
  const stats = { groupCount: 0, hostCount: 0 }

  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const nodeType = node.node_type || ''
      if (nodeType === 'group') {
        stats.groupCount += 1
      }
      if (nodeType === 'host') {
        stats.hostCount += 1
      }
      walk(node.children || [])
    })
  }

  walk(nodes)
  return stats
}

function toNumericSet(values) {
  const set = new Set()
  ;(Array.isArray(values) ? values : []).forEach((item) => {
    const num = Number(item)
    if (Number.isInteger(num) && num > 0) {
      set.add(num)
    }
  })
  return set
}

function parseGroupIdFromNode(node) {
  if (!node || typeof node !== 'object') {
    return null
  }
  if (Number.isInteger(Number(node.group_id)) && Number(node.group_id) > 0) {
    return Number(node.group_id)
  }
  const keyText = String(node.key || '')
  if (keyText.startsWith('group-')) {
    const groupId = Number(keyText.slice('group-'.length))
    return Number.isInteger(groupId) && groupId > 0 ? groupId : null
  }
  return null
}

function pruneTreeToCheckedNodes(nodes, selectedGroupSet, selectedHostSet, inheritedGroupSelected = false) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const result = []

  normalizedNodes.forEach((node) => {
    if (!node || typeof node !== 'object') {
      return
    }

    const nodeType = node.node_type || ''
    if (nodeType === 'host') {
      const hostId = Number(node.host_id)
      const keepByHost = Number.isInteger(hostId) && selectedHostSet.has(hostId)
      if (keepByHost || inheritedGroupSelected) {
        result.push({ ...node, children: undefined })
      }
      return
    }

    const groupId = parseGroupIdFromNode(node)
    const groupSelected = inheritedGroupSelected || (groupId !== null && selectedGroupSet.has(groupId))
    const children = pruneTreeToCheckedNodes(node.children || [], selectedGroupSet, selectedHostSet, groupSelected)

    // 仅展示有勾选主机落在其子树中的分组；空分组直接隐藏。
    if (children.length > 0) {
      result.push({ ...node, children })
    }
  })

  return result
}

function openGroupScopeViewer(record) {
  groupScopeViewKeyword.value = ''
  const executionTree = Array.isArray(record?.execution_scope_tree) ? record.execution_scope_tree : []
  if (executionTree.length > 0) {
    groupScopeViewTitle.value = `查看执行范围 / ${record?.name || '-'} (${record?.code || '-'})`
    groupScopeViewTreeData.value = appendGroupHostCount(cloneTreeNodes(executionTree))
    const summaryHostCount = Number(record?.execution_scope_summary?.host_count || 0)
    groupScopeViewSummary.value = `当前范围：${summaryHostCount}台主机`
    groupScopeViewVisible.value = true
    return
  }

  const selectedGroupIds = Array.isArray(record?.selected_group_ids) ? record.selected_group_ids : []
  const selectedHostIds = Array.isArray(record?.selected_host_ids) ? record.selected_host_ids : []
  const isAllHostsScope = selectedGroupIds.length === 0 && selectedHostIds.length === 0

  groupScopeViewTitle.value = `查看主机组范围 / ${record?.name || '-'} (${record?.code || '-'})`

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

function collectCheckedScope(keys) {
  const checkedGroupIds = []
  const checkedHostIds = []
  ;(Array.isArray(keys) ? keys : []).forEach((key) => {
    if (key === ALL_GROUP_VALUE) {
      checkedGroupIds.push(ALL_GROUP_VALUE)
      return
    }
    if (typeof key !== 'string') {
      return
    }
    if (key.startsWith('group-')) {
      const groupId = Number(key.replace('group-', ''))
      if (Number.isInteger(groupId) && groupId > 0) {
        checkedGroupIds.push(groupId)
      }
      return
    }
    if (key.startsWith('host-')) {
      const hostId = Number(key.replace('host-', ''))
      if (Number.isInteger(hostId) && hostId > 0) {
        checkedHostIds.push(hostId)
      }
    }
  })
  return { checkedGroupIds, checkedHostIds }
}

function onGroupScopeCheck(checkedKeys) {
  const nextKeys = Array.isArray(checkedKeys) ? checkedKeys : checkedKeys?.checked || []
  if (nextKeys.includes(ALL_GROUP_VALUE)) {
    taskForm.selected_group_ids = [ALL_GROUP_VALUE]
    taskForm.selected_host_ids = []
    groupScopeCheckedKeys.value = [ALL_GROUP_VALUE]
    return
  }

  const { checkedGroupIds, checkedHostIds } = collectCheckedScope(nextKeys)
  taskForm.selected_group_ids = checkedGroupIds
  taskForm.selected_host_ids = checkedHostIds
  groupScopeCheckedKeys.value = nextKeys
}

function resetTaskForm() {
  taskForm.name = ''
  taskForm.code = ''
  taskForm.template = null
  taskForm.inventory = null
  taskForm.default_limit = ''
  taskForm.selected_host_ids = []
  taskForm.selected_group_ids = []
  taskForm.env_vars_text = ''
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
  taskForm.code = record.code || ''
  taskForm.template = record.template || null
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
  const selectedGroupIds = Array.isArray(record.selected_group_ids) ? [...record.selected_group_ids] : []
  taskForm.selected_host_ids = Array.isArray(record.selected_host_ids) ? [...record.selected_host_ids] : []
  taskForm.selected_group_ids = selectedGroupIds
  taskForm.env_vars_text = JSON.stringify(record.env_vars || {}, null, 2)
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
  if (!taskForm.code || !String(taskForm.code).trim()) {
    message.error('请输入任务编码')
    return
  }
  if (!taskForm.template) {
    message.error('请选择模板')
    return
  }

  let envVars = {}
  try {
    envVars = parseJsonText(taskForm.env_vars_text, '环境变量 JSON')
  } catch (error) {
    message.error(error.message)
    return
  }

  const payload = {
    name: String(taskForm.name).trim(),
    code: String(taskForm.code).trim(),
    template: Number(taskForm.template),
    inventory: Number(taskForm.inventory) > 0 ? Number(taskForm.inventory) : null,
    default_limit: String(taskForm.default_limit || '').trim(),
    selected_host_ids: [],
    selected_group_ids: [],
    env_vars: envVars,
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
  } finally {
    modalSubmitting.value = false
  }
}

async function removeTask(record) {
  await deleteTask(record.id)
  message.success('任务已删除')
  await loadTasks(false)
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
      runNowPrecheckMessage.value = `预检通过，可执行主机 ${hostCount} 台（Limit: ${runNowEffectiveLimit.value || '（空）'}）`
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

  const runtimeLimit = String(runNowLimit.value || '').trim()
  runningTaskId.value = runNowTask.value.id
  runNowSubmitting.value = true
  try {
    const res = await runTaskNow(runNowTask.value.id, { limit: runtimeLimit })
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
  router.push(buildAutomationPlaybookRoute(record))
}

function goToInventory(record) {
  router.push(buildAutomationInventoryRoute(record))
}

function goToWorkflow() {
  router.push('/sys/automation/workflow')
}

function viewTaskLogs(record) {
  goToLogs(record)
}

function handleTaskTableChange(page) {
  taskPagination.current = page.current
  taskPagination.pageSize = page.pageSize
  loadTasks(false)
}

function reloadAll() {
  loadPlaybooks()
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
