<template>
  <div class="notification-policy-page">
    <a-card title="通知策略" size="small">
      <template #extra>
        <a-button size="large" @click="openCreateModal(rootPolicy?.id)">
          <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
          <span>&nbsp;新增子策略</span>
        </a-button>
      </template>

      <a-alert type="info" show-icon style="margin-bottom: 12px" message="路由语义（对齐 Grafana）">
        <template #description>
          从根节点向下逐层匹配：每层按排序取第一条命中的子策略继续下钻，最深命中策略决定出口媒介与事件开关；
          出口媒介留空继承父节点，显式设为「静音」则不投递。用户在个人中心绑定的只是收件邮箱。
        </template>
      </a-alert>

      <a-spin :spinning="listLoading">
        <a-empty v-if="!listLoading && !treeData.length" description="策略树为空" />
        <a-tree
          v-else
          :tree-data="treeData"
          block-node
          default-expand-all
          :selectable="false"
        >
          <template #title="wrapper">
            <div class="policy-node">
              <span class="policy-node-name">
                <FontAwesomeIcon v-if="wrapper.raw.isRoot" :icon="['fas', 'house']" class="policy-root-icon" />
                {{ wrapper.raw.name }}
              </span>
              <a-tag v-if="wrapper.raw.isRoot" color="purple">默认策略（恒命中）</a-tag>
              <a-tag v-if="wrapper.raw.position" color="default">#{{ wrapper.raw.position }}</a-tag>
              <template v-if="wrapper.raw.matchers.length">
                <a-tag v-for="(matcher, index) in wrapper.raw.matchers" :key="index" color="blue">{{ formatMatcher(matcher) }}</a-tag>
              </template>
              <a-tag v-else-if="!wrapper.raw.isRoot" color="cyan">无条件命中</a-tag>
              <a-tag :color="wrapper.raw.notifyOnFiring ? 'red' : 'default'">firing {{ wrapper.raw.notifyOnFiring ? '开' : '关' }}</a-tag>
              <a-tag :color="wrapper.raw.notifyOnResolved ? 'green' : 'default'">resolved {{ wrapper.raw.notifyOnResolved ? '开' : '关' }}</a-tag>
              <a-tag v-if="wrapper.raw.mediaInherited" color="default">出口：继承</a-tag>
              <template v-else-if="wrapper.raw.mediaIds.length">
                <a-tag v-for="mediaId in wrapper.raw.mediaIds" :key="mediaId">{{ mediaNameMap[mediaId] || `媒介 #${mediaId}` }}</a-tag>
              </template>
              <a-tag v-else color="orange">静音（不投递）</a-tag>
              <span class="policy-node-actions">
                <a-tooltip title="添加子策略" placement="top">
                  <a-button size="small" @click="openCreateModal(wrapper.raw.id)">
                    <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="编辑" placement="top">
                  <a-button size="small" type="primary" @click="openEditModal(wrapper.raw)">编辑</a-button>
                </a-tooltip>
                <a-tooltip v-if="!wrapper.raw.isRoot" title="删除（含子树）" placement="top">
                  <a-button size="small" type="primary" danger class="delBtn" @click="handleDelete(wrapper.raw)">删除</a-button>
                </a-tooltip>
              </span>
            </div>
          </template>
        </a-tree>
      </a-spin>
    </a-card>

    <a-modal
      v-model:open="modalVisible"
      :title="modalTitle"
      width="760px"
      :footer="null"
      @cancel="closeModal"
    >
      <a-form :model="policyForm" :label-col="{ span: 5 }" :wrapper-col="{ span: 17 }">
        <a-form-item label="父策略" required>
          <a-select
            v-model:value="policyForm.parent_id"
            :options="parentOptions"
            :disabled="Boolean(editingId) && editingIsRoot"
            :getPopupContainer="getPopupContainer"
            placeholder="选择父策略"
          />
        </a-form-item>
        <a-form-item label="名称" required>
          <a-input v-model:value="policyForm.name" placeholder="例如：生产环境严重告警" />
        </a-form-item>
        <a-form-item label="排序">
          <a-input-number v-model:value="policyForm.position" :min="0" style="width: 160px" />
          <span class="form-tip">同层按排序值从小到大取第一条命中的策略。</span>
        </a-form-item>
        <a-form-item label="匹配条件">
          <div class="matcher-list">
            <div v-for="(matcher, index) in policyForm.matchers" :key="matcher.id" class="matcher-row">
              <a-select v-model:value="matcher.type" :options="matcherTypeOptions" style="width: 96px" :getPopupContainer="getPopupContainer" />
              <template v-if="matcher.type === 'label'">
                <a-input v-model:value="matcher.label" placeholder="标签名，例如 severity" />
                <a-select v-model:value="matcher.operator" :options="operatorOptions" style="width: 76px" :getPopupContainer="getPopupContainer" />
                <a-input v-model:value="matcher.value" placeholder="标签值 / 正则" />
              </template>
              <template v-else>
                <a-tree-select
                  v-model:value="matcher.treeNode"
                  show-search
                  allow-clear
                  :tree-data="matcherTreeData(matcher)"
                  :tree-node-filter-prop="'title'"
                  :getPopupContainer="getPopupContainer"
                  placeholder="选择服务树节点"
                  style="flex: 1"
                />
              </template>
              <a-button danger @click="removeMatcher(index)">移除</a-button>
            </div>
            <a-button @click="addMatcher">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp;添加条件</span>
            </a-button>
            <div class="matcher-tip">不添加条件时匹配全部告警；多个条件必须同时满足（AND）。= / != 精确匹配，=~ / !~ 正则匹配，缺失标签按空串参与匹配。</div>
          </div>
        </a-form-item>
        <a-form-item label="出口媒介" required>
          <a-checkbox v-model:checked="policyForm.mediaInherited" :disabled="editingIsRoot">继承父节点出口</a-checkbox>
          <a-select
            v-if="!policyForm.mediaInherited"
            v-model:value="policyForm.mediaIds"
            mode="multiple"
            allow-clear
            :options="mediaOptions"
            :getPopupContainer="getPopupContainer"
            placeholder="不选 = 静音（命中也不投递）"
            style="margin-top: 8px"
          />
          <div v-else class="form-tip" style="margin-top: 4px">沿父链取最近一个显式出口；根节点出口必须显式配置。</div>
        </a-form-item>
        <a-form-item label="接收组">
          <a-checkbox v-model:checked="policyForm.userGroupInherited">继承父节点的接收组限制</a-checkbox>
          <a-select
            v-if="!policyForm.userGroupInherited"
            v-model:value="policyForm.userGroupIds"
            mode="multiple"
            allow-clear
            :options="userGroupOptions"
            :getPopupContainer="getPopupContainer"
            placeholder="不选 = 不限组（媒介上全部绑定都可收）"
            style="margin-top: 8px"
          />
          <div v-else class="form-tip" style="margin-top: 4px">限制后仅出口用户组成员的绑定会收到；成员在「系统管理 &gt; 用户组」维护。</div>
        </a-form-item>
        <a-form-item label="通知事件" required>
          <a-checkbox-group v-model:value="policyForm.eventTypes" :options="eventTypeOptions" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="policyForm.remark" :rows="2" />
        </a-form-item>
      </a-form>

      <div class="modal-actions">
        <a-button @click="closeModal">取消</a-button>
        <a-button type="primary" :loading="saveLoading" @click="savePolicy">保存</a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { tableLocale } from '@/util/tableStyle'
import { message } from 'ant-design-vue'
import {
  batchDeleteNotificationPolicies,
  createNotificationPolicy,
  getAlertMediaList,
  getNotificationPolicyList,
  updateNotificationPolicy,
} from '@/api/monitor'
import { getUserGroupList } from '@/api/userGroup'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { fetchAllPages } from '@/util/fetchAllPages'
import {
  getApplicationServiceList,
  getBusinessEnvironmentList,
  getBusinessSystemList,
  getProjectList,
} from '@/api/assets/application'
import { buildAlertScopeTreeData } from '@/util/alertScope'

defineOptions({
  name: 'NotificationPoliciesPage',
})

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const matcherTypeOptions = [
  { label: '标签', value: 'label' },
  { label: '服务树', value: 'tree' },
]
const operatorOptions = [
  { label: '=', value: '=' },
  { label: '!=', value: '!=' },
  { label: '=~', value: '=~' },
  { label: '!~', value: '!~' },
]
const eventTypeOptions = [
  { label: '告警触发', value: 'firing' },
  { label: '告警恢复', value: 'resolved' },
]

const policyItems = ref([])
const mediaList = ref([])
const userGroupList = ref([])
const listLoading = ref(false)
const saveLoading = ref(false)
const modalVisible = ref(false)
const editingId = ref(null)
const editingIsRoot = ref(false)
let matcherSequence = 0
const policyForm = ref(createDefaultForm())
const scopeTreeData = ref([])
const scopeNodeNameMap = ref({})

const rootPolicy = computed(() => policyItems.value.find((item) => item.is_root) || null)
const mediaNameMap = computed(() => Object.fromEntries(mediaList.value.map((item) => [item.id, item.name])))
const userGroupOptions = computed(() => userGroupList.value.map((group) => ({
  label: group.name,
  value: group.id,
})))
const mediaOptions = computed(() => mediaList.value
  .filter((item) => item.media_type === 'email')
  .map((item) => ({ label: item.enabled ? item.name : `${item.name}（已停用）`, value: item.id })))
const parentOptions = computed(() => policyItems.value.map((item) => ({
  label: item.is_root ? `${item.name}（根）` : item.name,
  value: item.id,
})))
const modalTitle = computed(() => (editingId.value ? '编辑策略' : '新增策略'))

// 扁平列表 → 树控件数据。
const treeData = computed(() => {
  const nodeMap = new Map()
  const nodes = policyItems.value.map((item) => {
    const node = {
      id: item.id,
      parentId: item.parent_id || 0,
      isRoot: Boolean(item.is_root),
      name: item.name,
      position: item.position,
      matchers: Array.isArray(item.matchers) ? item.matchers : [],
      mediaInherited: item.media_ids == null,
      mediaIds: item.media_ids || [],
      userGroupInherited: item.user_group_ids == null,
      userGroupIds: item.user_group_ids || [],
      userGroupNames: item.user_group_names || [],
      notifyOnFiring: item.notify_on_firing,
      notifyOnResolved: item.notify_on_resolved,
      children: [],
    }
    nodeMap.set(node.id, node)
    return node
  })
  const roots = []
  for (const node of nodes) {
    const parent = nodeMap.get(node.parentId)
    if (parent && node.parentId !== node.id) {
      parent.children.push(node)
    } else {
      roots.push(node)
    }
  }
  const toTree = (node) => ({
    key: node.id,
    title: node.name,
    raw: node,
    children: node.children
      .sort((left, right) => left.position - right.position || left.id - right.id)
      .map(toTree),
  })
  return roots.map(toTree)
})

function createDefaultForm() {
  return {
    parent_id: undefined,
    name: '',
    position: 0,
    matchers: [],
    mediaInherited: false,
    mediaIds: [],
    userGroupInherited: true,
    userGroupIds: [],
    eventTypes: ['firing', 'resolved'],
    remark: '',
  }
}

function formatMatcher(matcher) {
  if (matcher.type === 'tree') {
    const key = `${matcher.node_type}:${matcher.id}`
    const name = scopeNodeNameMap.value[key]
    const typeLabel = { service: '服务', environment: '环境', business: '业务', project: '项目' }[matcher.node_type] || matcher.node_type
    return `服务树 ${typeLabel}：${name || `#${matcher.id}`}`
  }
  return `${matcher.label} ${matcher.operator} ${matcher.value}`
}

function parseApiData(response) {
  return response?.data?.data || {}
}

// 服务树选择数据：已被其他条件行选中的节点置 disabled，且其整个子树一并禁用——
// 服务树条件命中告警主机的全部归属节点（含祖先），选父节点已覆盖子树，再选子节点属于冗余条件。
function matcherTreeData(current) {
  const selected = new Set(
    policyForm.value.matchers
      .filter((matcher) => matcher !== current && matcher.type === 'tree')
      .map((matcher) => matcher.treeNode),
  )
  if (!selected.size) return scopeTreeData.value
  const clone = (nodes, covered) => (nodes || []).map((node) => {
    const isCovered = covered || (node.value && selected.has(node.value))
    const next = {
      ...node,
      disabled: isCovered || node.disabled,
    }
    if (node.children?.length) next.children = clone(node.children, isCovered)
    return next
  })
  return clone(scopeTreeData.value, false)
}

function addMatcher() {
  matcherSequence += 1
  policyForm.value.matchers.push({ id: matcherSequence, type: 'label', label: '', operator: '=', value: '', treeNode: undefined })
}

function removeMatcher(index) {
  policyForm.value.matchers.splice(index, 1)
}

async function loadScopeTreeData() {
  if (scopeTreeData.value.length) return
  const [projects, systems, services, environments] = await Promise.all([
    fetchAllPages(getProjectList),
    fetchAllPages(getBusinessSystemList),
    fetchAllPages(getApplicationServiceList),
    fetchAllPages(getBusinessEnvironmentList),
  ])
  const environmentNames = new Map(environments.map((item) => [String(item.id), item.name]))
  scopeTreeData.value = buildAlertScopeTreeData({ projects, systems, services, environmentNames })
  scopeNodeNameMap.value = {
    ...Object.fromEntries(projects.map((item) => [`project:${item.id}`, item.name])),
    ...Object.fromEntries(systems.map((item) => [`business:${item.id}`, item.name])),
    ...Object.fromEntries(environments.map((item) => [`environment:${item.id}`, item.name])),
    ...Object.fromEntries(services.map((item) => [`service:${item.id}`, item.name])),
  }
}

function openCreateModal(parentId) {
  editingId.value = null
  editingIsRoot.value = false
  policyForm.value = createDefaultForm()
  policyForm.value.parent_id = parentId || rootPolicy.value?.id
  modalVisible.value = true
  loadScopeTreeData().catch((error) => console.error('加载服务树失败:', error))
}

function openEditModal(node) {
  editingId.value = node.id
  editingIsRoot.value = node.isRoot
  const item = policyItems.value.find((entry) => entry.id === node.id) || {}
  policyForm.value = {
    parent_id: item.parent_id || undefined,
    name: item.name,
    position: item.position,
    matchers: (Array.isArray(item.matchers) ? item.matchers : []).map((matcher) => {
      matcherSequence += 1
      return {
        id: matcherSequence,
        type: matcher.type,
        label: matcher.label || '',
        operator: matcher.operator || '=',
        value: matcher.value || '',
        treeNode: matcher.type === 'tree' ? `${matcher.node_type}:${matcher.id}` : undefined,
      }
    }),
    mediaInherited: item.media_ids == null,
    mediaIds: [...(item.media_ids || [])],
    userGroupInherited: item.user_group_ids == null,
    userGroupIds: [...(item.user_group_ids || [])],
    eventTypes: [
      ...(item.notify_on_firing ? ['firing'] : []),
      ...(item.notify_on_resolved ? ['resolved'] : []),
    ],
    remark: item.remark || '',
  }
  modalVisible.value = true
  loadScopeTreeData().catch((error) => console.error('加载服务树失败:', error))
}

function closeModal() {
  modalVisible.value = false
}

function buildMatchersPayload() {
  const matchers = []
  for (const matcher of policyForm.value.matchers) {
    if (matcher.type === 'label') {
      const label = matcher.label.trim()
      const value = matcher.value.trim()
      if (!label) {
        message.warning('标签条件的标签名不能为空')
        return null
      }
      if (!value && (matcher.operator === '=' || matcher.operator === '!=')) {
        message.warning(`标签 ${label} 的值不能为空`)
        return null
      }
      matchers.push({ type: 'label', label, operator: matcher.operator, value })
    } else {
      const match = String(matcher.treeNode || '').match(/^(service|environment|business|project):(\d+)$/)
      if (!match) {
        message.warning('服务树条件请选择一个节点')
        return null
      }
      matchers.push({ type: 'tree', node_type: match[1], id: Number(match[2]) })
    }
  }
  return matchers
}

async function savePolicy() {
  const name = policyForm.value.name.trim()
  if (!name) {
    message.warning('请填写策略名称')
    return
  }
  if (!editingIsRoot.value && !policyForm.value.parent_id) {
    message.warning('请选择父策略')
    return
  }
  if (!policyForm.value.eventTypes.length) {
    message.warning('至少选择一种通知事件')
    return
  }
  const matchers = buildMatchersPayload()
  if (matchers === null) return

  const payload = {
    id: editingId.value || undefined,
    parent_id: editingIsRoot.value ? 0 : policyForm.value.parent_id,
    name,
    position: policyForm.value.position,
    matchers,
    // 根节点出口必须显式：前端根编辑时不允许勾继承。
    media_ids: policyForm.value.mediaInherited && !editingIsRoot.value ? null : [...policyForm.value.mediaIds],
    user_group_ids: policyForm.value.userGroupInherited ? null : [...policyForm.value.userGroupIds],
    notify_on_firing: policyForm.value.eventTypes.includes('firing'),
    notify_on_resolved: policyForm.value.eventTypes.includes('resolved'),
    remark: policyForm.value.remark.trim(),
  }

  saveLoading.value = true
  try {
    if (editingId.value) {
      await updateNotificationPolicy(payload)
    } else {
      await createNotificationPolicy(payload)
    }
    modalVisible.value = false
    message.success(editingId.value ? '策略已更新' : '策略已添加')
    await loadPolicies()
  } finally {
    saveLoading.value = false
  }
}

async function loadPolicies() {
  listLoading.value = true
  try {
    const response = await getNotificationPolicyList()
    const data = parseApiData(response)
    policyItems.value = Array.isArray(data.items) ? data.items : []
  } finally {
    listLoading.value = false
  }
}

async function loadUserGroups() {
  const response = await getUserGroupList()
  userGroupList.value = parseApiData(response).items || []
}

async function loadMedia() {
  const response = await getAlertMediaList({ page: 1, page_size: 100 })
  const data = parseApiData(response)
  mediaList.value = data.results || data || []
}

async function handleDelete(node) {
  await openDeleteConfirm({
    title: '确认删除策略',
    summary: '删除后其子策略一并删除，命中该分支的告警将回退到上级策略路由。',
    items: [node.name],
    onConfirm: async () => {
      await batchDeleteNotificationPolicies([node.id])
      message.success('策略已删除')
      await loadPolicies()
    },
  })
}

onMounted(async () => {
  try {
    await Promise.all([loadPolicies(), loadMedia(), loadUserGroups()])
    loadScopeTreeData().catch((error) => console.error('加载服务树失败:', error))
  } catch (error) {
    message.error(error?.response?.data?.msg || '通知策略数据加载失败')
  }
})
</script>

<style scoped>
.notification-policy-page {
  padding: 8px;
}

.policy-node {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.policy-node-name {
  color: rgba(0, 0, 0, 0.88);
  font-weight: 600;
}

.policy-root-icon {
  margin-right: 4px;
  color: #722ed1;
}

.policy-node-actions {
  display: inline-flex;
  gap: 6px;
  margin-left: 8px;
}

.matcher-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.matcher-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.matcher-row > .ant-input,
.matcher-row > .ant-select,
.matcher-row > .ant-tree-select {
  flex: 1;
  min-width: 0;
}

.matcher-tip {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}

.form-tip {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
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
