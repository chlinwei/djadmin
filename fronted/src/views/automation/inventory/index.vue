<template>
  <div class="inventory-page">
    <a-row class="tools" :gutter="12">
      <a-col :span="16">
        <a-input-search
          v-model:value="keyword"
          placeholder="搜索 Inventory 名称"
          allow-clear
          enter-button
          @search="loadInventories(true)"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-button size="large" @click="openCreateModal" v-permission="'automation:inventory:create'">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp新增Inventory</span>
          </a-button>
          <a-button type="primary" ghost :loading="loading" @click="loadInventories(false)">刷新</a-button>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="Inventory列表" size="small" class="block-card">
      <a-table
        :columns="columns"
        :data-source="records"
        :loading="loading"
        :pagination="pagination"
        rowKey="id"
        size="small"
        @change="onTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-switch
              :checked="record.enabled === true"
              :disabled="!canUpdateInventory || loading || inventoryStatusUpdatingId === record.id"
              :loading="inventoryStatusUpdatingId === record.id"
              @change="(checked) => onChangeInventoryStatus(checked, record)"
            />
          </template>
          <template v-else-if="column.key === 'resolved_host_count'">
            <a-button size="small" type="link" @click.stop="openScopeViewer(record)">
              {{ Number(record.resolved_host_count || 0) }} 台
            </a-button>
          </template>
          <template v-else-if="column.key === 'health_status'">
            <a-tooltip :title="record.health_status?.message || ''">
              <a-tag :color="getHealthTagColor(record)">{{ record.health_status?.label || '未知' }}</a-tag>
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑">
                <a-button size="small" type="primary" @click="openEditModal(record)" v-permission="'automation:inventory:update'">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="删除">
                <a-button
                  class="delBtn"
                  size="small"
                  type="primary"
                  danger
                  v-permission="'automation:inventory:delete'"
                  @click="openDeleteInventoryConfirm(record)"
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
      :title="isCreateMode ? '新增Inventory' : '编辑Inventory'"
      :open="modalVisible"
      :confirmLoading="submitting"
      :width="860"
      @ok="submitForm"
      @cancel="modalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="名称" required>
          <a-input v-model:value="form.name" placeholder="例如：生产环境Inventory" />
        </a-form-item>

        <a-form-item label="启用状态">
          <a-switch v-model:checked="form.enabled" checked-children="启用" un-checked-children="禁用" />
        </a-form-item>

        <a-form-item label="范围预览">
          <a-input :value="scopePreviewText" readonly />
        </a-form-item>

        <a-form-item>
          <a-button @click="openScopeModal">编辑主机组范围</a-button>
        </a-form-item>

        <a-form-item label="备注">
          <a-textarea v-model:value="form.remark" :rows="2" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="编辑 Inventory 范围"
      :open="scopeModalVisible"
      :width="1080"
      @ok="scopeModalVisible = false"
      @cancel="scopeModalVisible = false"
    >
      <div class="scope-editor__desc">支持多层主机组。勾选分组会自动覆盖子分组，未勾选表示空范围（0台主机）。</div>
      <a-input
        v-model:value="scopeEditKeyword"
        allow-clear
        placeholder="搜索分组"
        class="scope-editor__search"
      />
      <div class="scope-editor__tree-wrap">
        <a-tree
          v-if="filteredScopeEditTreeData.length > 0"
          :checked-keys="checkedKeys"
          checkable
          block-node
          :expanded-keys="scopeEditExpandedKeys"
          :auto-expand-parent="true"
          :tree-data="filteredScopeEditTreeData"
          :selectable="false"
          :show-line="{ showLeafIcon: false }"
          @check="onScopeCheck"
        />
        <a-empty v-else description="未匹配到分组" />
      </div>
    </a-modal>

    <a-modal
      :title="scopeViewTitle"
      :open="scopeViewVisible"
      :width="980"
      :footer="null"
      @cancel="scopeViewVisible = false"
    >
      <div class="scope-editor__desc">仅展示当前 Inventory 执行范围（主机组与主机），未命中节点已隐藏</div>
      <div class="scope-viewer__summary">{{ scopeViewSummary }}</div>
      <a-input
        v-model:value="scopeViewKeyword"
        allow-clear
        placeholder="搜索分组/主机/IP"
        class="scope-viewer__search"
      />
      <div class="scope-editor__tree-wrap">
        <a-tree
          v-if="filteredScopeViewTreeData.length > 0"
          block-node
          :expanded-keys="scopeViewExpandedKeys"
          :auto-expand-parent="true"
          :tree-data="filteredScopeViewTreeData"
          :selectable="false"
          :show-line="{ showLeafIcon: false }"
        >
          <template #title="node">
            <a-button
              v-if="isHostTreeNode(node)"
              type="link"
              size="small"
              class="scope-host-link-btn"
              @click.stop="goToAssetHostByNode(node)"
            >
              {{ getTreeNodeTitle(node) }}
            </a-button>
            <span v-else>{{ getTreeNodeTitle(node) }}</span>
          </template>
        </a-tree>
        <a-empty v-else description="暂无已勾选范围" />
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { checkPermission } from '@/directives/permission/permission'
import {
  getInventoryList,
  createInventory,
  updateInventory,
  deleteInventory,
  getAutomationHostOptions,
  getAutomationGroupTree,
} from '@/api/sys/automation'
import { buildScopeSummaryText, flattenGroupPathMap } from '../scopeSummary'

const ASSET_HOST_ROUTE_CANDIDATES = ['/assets/hosts', '/assets/host', '/assets/hosts/index', '/assets/host/index']
const route = useRoute()
const router = useRouter()

const loading = ref(false)
const submitting = ref(false)
const inventoryStatusUpdatingId = ref(null)
const modalVisible = ref(false)
const scopeModalVisible = ref(false)
const scopeViewVisible = ref(false)
const scopeViewTitle = ref('查看 Inventory 范围')
const scopeViewSummary = ref('')
const scopeViewTreeData = ref([])
const scopeViewKeyword = ref('')
const scopeEditKeyword = ref('')
const isCreateMode = ref(true)
const editingId = ref(null)
const keyword = ref('')
const canUpdateInventory = computed(() => checkPermission('automation:inventory:update'))

const records = ref([])
const groupTreeData = ref([])
const groupNameMap = ref({})
const groupPathMap = ref({})
const hostOptions = ref([])
const checkedKeys = ref([])

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const form = reactive({
  name: '',
  enabled: true,
  selected_group_ids: [],
  selected_host_ids: [],
  remark: '',
})

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 260 },
  { title: '可用主机', dataIndex: 'resolved_host_count', key: 'resolved_host_count', width: 120 },
  { title: '健康状态', dataIndex: 'health_status', key: 'health_status', width: 120 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 100 },
  { title: '备注', dataIndex: 'remark', key: 'remark' },
  { title: '操作', key: 'action', width: 180, fixed: 'right' },
]

function getHealthTagColor(record) {
  const status = record?.health_status?.status || ''
  if (status === 'healthy') {
    return 'green'
  }
  if (status === 'empty') {
    return 'orange'
  }
  if (status === 'invalid') {
    return 'red'
  }
  return 'default'
}

const scopePreviewText = computed(() => {
  return buildScopeSummaryText({
    selectedGroupIds: form.selected_group_ids,
    selectedHostIds: form.selected_host_ids,
    groupPathMap: groupPathMap.value,
    groupNameMap: groupNameMap.value,
    emptyAsAllHosts: false,
  })
})

const scopeEditTreeData = computed(() => {
  return buildViewerTreeWithHosts(groupTreeData.value, hostOptions.value)
})

const filteredScopeEditTreeData = computed(() => {
  return filterTreeByKeyword(scopeEditTreeData.value, scopeEditKeyword.value)
})

function collectGroupTreeKeys(nodes) {
  const keys = []
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      if (!node || typeof node !== 'object') {
        return
      }
      const keyText = String(node.key || '')
      if (keyText.startsWith('group-')) {
        keys.push(keyText)
      }
      walk(node.children || [])
    })
  }
  walk(nodes)
  return keys
}

const scopeEditExpandedKeys = computed(() => collectGroupTreeKeys(filteredScopeEditTreeData.value))

function cloneGroupNodes(nodes) {
  return (Array.isArray(nodes) ? nodes : []).map((node) => ({
    ...node,
    children: cloneGroupNodes(node.children || []),
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

      const keyText = String(node.key || '')
      if (keyText.startsWith('host-')) {
        hostCount += 1
        return { ...node, children: undefined }
      }

      const children = Array.isArray(node.children) ? node.children : []
      const childResult = walk(children)
      const baseTitle = stripGroupCountSuffix(node.title || '-')
      const nextTitle = childResult.hostCount > 0 ? `${baseTitle}（${childResult.hostCount}台）` : baseTitle

      groupCount += 1
      hostCount += childResult.hostCount
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
      const keyText = String(node.key || '')
      if (keyText.startsWith('group-')) {
        stats.groupCount += 1
      }
      if (keyText.startsWith('host-')) {
        stats.hostCount += 1
      }
      walk(node.children || [])
    })
  }
  walk(nodes)
  return stats
}

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

function getTreeNodeTitle(node) {
  const target = node?.dataRef || node
  return target?.title || '-'
}

function getTreeNodeHostId(node) {
  const target = node?.dataRef || node
  const hostId = Number(target?.host_id)
  if (Number.isInteger(hostId) && hostId > 0) {
    return hostId
  }

  const keyText = String(target?.key || '')
  if (keyText.startsWith('host-')) {
    const parsed = Number(keyText.slice('host-'.length))
    return Number.isInteger(parsed) && parsed > 0 ? parsed : null
  }
  return null
}

function isHostTreeNode(node) {
  return getTreeNodeHostId(node) !== null
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

function goToAssetHost(hostId, hostTitle = '') {
  const normalizedHostId = Number(hostId)
  if (!Number.isInteger(normalizedHostId) || normalizedHostId <= 0) {
    return
  }

  const hostListPath = resolveAssetHostListPath()
  if (!hostListPath) {
    message.warning('未找到资产主机页面路由')
    return
  }

  const searchText = String(hostTitle || '').trim().replace(/\s*\([^)]*\)\s*$/, '')
  router.push({
    path: hostListPath,
    query: {
      host_id: String(normalizedHostId),
      ...(searchText ? { search: searchText } : {}),
    },
  })
}

function goToAssetHostByNode(node) {
  const hostId = getTreeNodeHostId(node)
  if (hostId === null) {
    return
  }
  goToAssetHost(hostId, getTreeNodeTitle(node))
}

const filteredScopeViewTreeData = computed(() => {
  return filterTreeByKeyword(scopeViewTreeData.value, scopeViewKeyword.value)
})

const scopeViewExpandedKeys = computed(() => collectGroupTreeKeys(filteredScopeViewTreeData.value))

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
  const keyText = String(node?.key || '')
  if (!keyText.startsWith('group-')) {
    return null
  }
  const groupId = Number(keyText.slice('group-'.length))
  return Number.isInteger(groupId) && groupId > 0 ? groupId : null
}

function pruneTreeToCheckedNodes(nodes, selectedGroupSet, selectedHostSet, inheritedGroupSelected = false) {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  const result = []

  normalizedNodes.forEach((node) => {
    if (!node || typeof node !== 'object') {
      return
    }

    const keyText = String(node.key || '')
    if (keyText.startsWith('host-')) {
      const hostId = Number(keyText.slice('host-'.length))
      const keepByHost = Number.isInteger(hostId) && selectedHostSet.has(hostId)
      if (keepByHost || inheritedGroupSelected) {
        result.push({ ...node, children: undefined })
      }
      return
    }

    const groupId = parseGroupIdFromNode(node)
    const groupSelected = inheritedGroupSelected || (groupId !== null && selectedGroupSet.has(groupId))
    const children = pruneTreeToCheckedNodes(node.children || [], selectedGroupSet, selectedHostSet, groupSelected)
    if (children.length > 0) {
      result.push({ ...node, children })
    }
  })

  return result
}

function getGroupIdByNode(node) {
  const key = String(node?.key || '')
  if (!key.startsWith('group-')) {
    return null
  }
  const groupId = Number(key.replace('group-', ''))
  return Number.isInteger(groupId) && groupId > 0 ? groupId : null
}

function attachHostsToGroupNodes(nodes, hostsByGroup) {
  ;(Array.isArray(nodes) ? nodes : []).forEach((node) => {
    attachHostsToGroupNodes(node.children || [], hostsByGroup)
    const groupId = getGroupIdByNode(node)
    if (groupId === null) {
      return
    }

    const hostChildren = hostsByGroup.get(groupId) || []
    if (hostChildren.length > 0) {
      node.children = [...(node.children || []), ...hostChildren]
    }
  })
}

function buildViewerTreeWithHosts(groupNodes, hosts) {
  const groups = cloneGroupNodes(groupNodes)
  const hostsByGroup = new Map()

  ;(Array.isArray(hosts) ? hosts : []).forEach((host) => {
    const groupId = Number(host?.group_id)
    if (!Number.isInteger(groupId) || groupId <= 0) {
      return
    }
    const current = hostsByGroup.get(groupId) || []
    const hostName = formatInventoryHostDisplayName(host)
    const hostIp = String(host?.ip || '').trim()
    const hostTitle = hostName && hostIp && hostName !== hostIp ? `${hostName}(${hostIp})` : (hostName || hostIp || '-')
    current.push({
      key: `host-${host.id}`,
      value: `host-${host.id}`,
      title: hostTitle,
      host_id: host.id,
      node_type: 'host',
      isLeaf: true,
    })
    hostsByGroup.set(groupId, current)
  })

  attachHostsToGroupNodes(groups, hostsByGroup)
  return groups
}

function formatInventoryHostDisplayName(host) {
  if (!host || typeof host !== 'object') {
    return '-'
  }

  const instanceName = String(host.instance_name || '').trim()
  if (instanceName) {
    return instanceName
  }

  return String(host.ip || '').trim() || `host-${host?.id || ''}`
}

function openScopeViewer(record) {
  scopeViewKeyword.value = ''
  const selectedGroupIds = Array.isArray(record?.selected_group_ids) ? record.selected_group_ids : []
  const selectedHostIds = Array.isArray(record?.selected_host_ids) ? record.selected_host_ids : []
  const hasScopeSelection = selectedGroupIds.length > 0 || selectedHostIds.length > 0

  scopeViewTitle.value = `查看 Inventory 范围 - ${record?.name || ''}`

  const treeWithHosts = buildViewerTreeWithHosts(groupTreeData.value, hostOptions.value)

  if (hasScopeSelection) {
    const selectedGroupSet = toNumericSet(selectedGroupIds)
    const selectedHostSet = toNumericSet(selectedHostIds)
    const prunedTree = pruneTreeToCheckedNodes(treeWithHosts, selectedGroupSet, selectedHostSet, false)
    scopeViewTreeData.value = appendGroupHostCount(prunedTree)
  } else {
    scopeViewTreeData.value = []
  }

  const scopeStats = collectTreeScopeStats(scopeViewTreeData.value)
  scopeViewSummary.value = `当前范围：${scopeStats.hostCount}台主机`

  scopeViewVisible.value = true
}

function openScopeModal() {
  scopeEditKeyword.value = ''
  scopeModalVisible.value = true
}

function resetForm() {
  form.name = ''
  form.enabled = true
  form.selected_group_ids = []
  form.selected_host_ids = []
  form.remark = ''
  checkedKeys.value = []
}

function syncCheckedKeysFromForm() {
  const selectedGroupIds = Array.isArray(form.selected_group_ids) ? form.selected_group_ids : []
  const selectedHostIds = Array.isArray(form.selected_host_ids) ? form.selected_host_ids : []
  checkedKeys.value = [
    ...selectedGroupIds.map((id) => `group-${id}`),
    ...selectedHostIds.map((id) => `host-${id}`),
  ]
}

function onScopeCheck(nextChecked) {
  const keys = Array.isArray(nextChecked) ? nextChecked : nextChecked?.checked || []
  checkedKeys.value = keys
  form.selected_group_ids = keys
    .filter((item) => typeof item === 'string' && item.startsWith('group-'))
    .map((item) => Number(item.replace('group-', '')))
    .filter((item) => Number.isInteger(item) && item > 0)
  form.selected_host_ids = keys
    .filter((item) => typeof item === 'string' && item.startsWith('host-'))
    .map((item) => Number(item.replace('host-', '')))
    .filter((item) => Number.isInteger(item) && item > 0)
}

function buildGroupTree(nodes) {
  return (nodes || []).map((node) => ({
    key: `group-${node.id}`,
    value: `group-${node.id}`,
    title: node.name,
    children: buildGroupTree(node.children || []),
  }))
}

function flattenGroupMap(nodes, collector = {}) {
  ;(nodes || []).forEach((node) => {
    collector[node.id] = node.name
    flattenGroupMap(node.children || [], collector)
  })
  return collector
}

async function loadGroupTree() {
  const res = await getAutomationGroupTree()
  const raw = res?.data?.data || []
  groupNameMap.value = flattenGroupMap(raw, {})
  groupPathMap.value = flattenGroupPathMap(raw, {})
  groupTreeData.value = buildGroupTree(raw)
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

  hostOptions.value = records
}

async function loadInventories(resetPage = false) {
  if (resetPage) {
    pagination.current = 1
  }
  loading.value = true
  try {
    const res = await getInventoryList({
      page: pagination.current,
      page_size: pagination.pageSize,
      ordering: '-id',
      search: keyword.value || undefined,
    })
    const data = res?.data?.data || {}
    records.value = data.results || []
    pagination.total = data.count || 0
  } finally {
    loading.value = false
  }
}

function normalizeInventorySearchQuery(query) {
  const searchText = String(query?.search || query?.inventory_name || query?.keyword || '').trim()
  return searchText
}

function applyRouteSearchQuery() {
  const nextKeyword = normalizeInventorySearchQuery(route.query)
  if (nextKeyword === String(keyword.value || '').trim()) {
    return false
  }
  keyword.value = nextKeyword
  return true
}

function openCreateModal() {
  isCreateMode.value = true
  editingId.value = null
  resetForm()
  modalVisible.value = true
}

function openEditModal(record) {
  isCreateMode.value = false
  editingId.value = record.id
  form.name = record.name || ''
  form.enabled = !!record.enabled
  form.selected_group_ids = Array.isArray(record.selected_group_ids) ? [...record.selected_group_ids] : []
  form.selected_host_ids = Array.isArray(record.selected_host_ids) ? [...record.selected_host_ids] : []
  form.remark = record.remark || ''
  syncCheckedKeysFromForm()
  modalVisible.value = true
}

async function submitForm() {
  if (!form.name || !String(form.name).trim()) {
    message.error('请输入名称')
    return
  }

  const selectedGroupIds = Array.isArray(form.selected_group_ids) ? form.selected_group_ids : []
  const selectedHostIds = Array.isArray(form.selected_host_ids) ? form.selected_host_ids : []
  if (selectedGroupIds.length === 0 && selectedHostIds.length === 0) {
    message.error('请至少选择一个主机组后再保存')
    return
  }

  const payload = {
    name: String(form.name).trim(),
    enabled: !!form.enabled,
    selected_group_ids: selectedGroupIds,
    selected_host_ids: selectedHostIds,
    remark: form.remark || '',
  }

  submitting.value = true
  try {
    if (isCreateMode.value) {
      await createInventory(payload)
      message.success('Inventory 创建成功')
    } else {
      await updateInventory(editingId.value, payload)
      message.success('Inventory 更新成功')
    }
    modalVisible.value = false
    await loadInventories(false)
  } finally {
    submitting.value = false
  }
}

async function removeRecord(record) {
  await deleteInventory(record.id)
  message.success('Inventory 已删除')
  await loadInventories(false)
}

async function onChangeInventoryStatus(checked, record) {
  if (!record?.id) {
    return
  }
  if (!canUpdateInventory.value) {
    message.warning('没有状态修改权限')
    return
  }

  const targetEnabled = checked === true
  const originalEnabled = record.enabled === true
  if (targetEnabled === originalEnabled) {
    return
  }

  inventoryStatusUpdatingId.value = record.id
  record.enabled = targetEnabled
  try {
    await updateInventory(record.id, { enabled: targetEnabled })
    message.success('状态修改成功')
  } catch (error) {
    record.enabled = originalEnabled
    message.error(error?.response?.data?.msg || error?.message || '状态修改失败')
  } finally {
    inventoryStatusUpdatingId.value = null
  }
}

function openDeleteInventoryConfirm(record) {
  const inventoryName = String(record?.name || '').trim() || `#${record?.id || '-'}`
  openDeleteConfirm({
    title: '确认删除 Inventory',
    summary: '删除后不可恢复，请确认影响清单。',
    items: [`Inventory: ${inventoryName}`],
    onConfirm: () => removeRecord(record),
  })
}

function onTableChange(page) {
  pagination.current = page.current
  pagination.pageSize = page.pageSize
  loadInventories(false)
}

onMounted(async () => {
  await Promise.all([loadGroupTree(), loadAllHostOptions()])
  applyRouteSearchQuery()
  await loadInventories(true)
})

watch(
  () => [route.query.search, route.query.inventory_name, route.query.keyword],
  async () => {
    const changed = applyRouteSearchQuery()
    if (!changed) {
      return
    }
    await loadInventories(true)
  }
)
</script>

<style scoped>
.inventory-page {
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

.scope-editor__desc {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.45);
  margin-bottom: 10px;
}

.scope-editor__tree-wrap {
  width: 100%;
  min-height: 420px;
  max-height: 620px;
  overflow: auto;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  padding: 12px 16px;
  background: #fff;
}

.scope-editor__search {
  max-width: 360px;
  margin-bottom: 10px;
}

.scope-viewer__search {
  max-width: 360px;
  margin-bottom: 10px;
}
</style>
