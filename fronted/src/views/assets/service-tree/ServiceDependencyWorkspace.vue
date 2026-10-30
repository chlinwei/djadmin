<template>
  <section class="dependency-workspace">
    <a-alert type="info" show-icon message="归属树与依赖图分开维护。允许同项目、同环境内跨业务依赖；双向依赖由两条独立关系组成。" />
    <a-alert v-if="catalogError || typesError" type="error" show-icon :message="catalogError || typesError" style="margin-top: 12px">
      <template #action><a-button size="small" @click="catalogError ? loadCatalog() : refreshTypes()">重试</a-button></template>
    </a-alert>
    <div class="workspace-toolbar">
      <a-space wrap>
        <a-select v-model:value="projectId" aria-label="项目" placeholder="选择项目" :options="projectOptions" :disabled="loadingCatalog || !!treeScope.project" :getPopupContainer="getPopupContainer" style="width: 170px" />
        <a-select v-model:value="environmentId" aria-label="环境" placeholder="选择环境" :options="environmentOptions" :disabled="loadingCatalog || !projectId || !!treeScope.environment" :getPopupContainer="getPopupContainer" style="width: 150px" />
        <a-button :loading="loadingSnapshot" :disabled="!ready" @click="refresh"><ReloadOutlined /><span>刷新快照</span></a-button>
      </a-space>
      <a-space wrap>
        <a-button v-if="canCreate" size="large" :disabled="!snapshot || loadingSnapshot || !!typesError" @click="editRelation(null)"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" /><span>&nbsp;新增依赖</span></a-button>
        <a-button :disabled="loadingCatalog" @click="typesOpen = true">交互类型管理</a-button>
      </a-space>
    </div>
    <a-empty v-if="!ready && !loadingCatalog && !catalogError" :description="selectionHint" />
    <a-alert v-if="snapshotError" type="error" show-icon :message="snapshotError" style="margin-bottom: 12px">
      <template #action><a-button size="small" @click="refresh">重试</a-button></template>
    </a-alert>
    <a-alert v-if="operationError" type="error" show-icon :message="operationError" style="margin-bottom: 12px" />
    <a-spin :spinning="loadingCatalog || loadingSnapshot">
      <template v-if="snapshot">
        <div class="snapshot-caption">
          {{ scopeTitle }} · {{ visibleTopology.nodes.length }} 个服务 / {{ visibleTopology.edges.length }} 条依赖
          <span>快照读取时间：{{ formatTime(snapshot.snapshot_time) }}（非实例检查时间）</span>
        </div>
        <div class="filter-toolbar">
          <a-space wrap>
            <a-input v-model:value="filters.search" allow-clear placeholder="搜索服务 / 功能 / 协议" aria-label="搜索依赖" style="width: 230px" />
            <a-select v-model:value="filters.strength" allow-clear placeholder="全部强度" :options="strengthOptions" :getPopupContainer="getPopupContainer" style="width: 135px" />
            <a-select v-model:value="filters.type" allow-clear placeholder="全部交互类型" :options="allTypeOptions" :getPopupContainer="getPopupContainer" style="width: 165px" />
          </a-space>
          <a-radio-group v-model:value="view" option-type="button" :options="[{ label: '架构图', value: 'graph' }, { label: '关系列表', value: 'list' }]" />
        </div>
        <ServiceDependencyGraph v-if="view === 'graph'" :nodes="visibleTopology.nodes" :edges="visibleTopology.edges" :title="scopeTitle" :layout-key="layoutKey" @select-node="showNode" @select-edge="showRelation" />
        <template v-else>
          <a-space class="batch-toolbar">
            <a-tooltip v-if="canDelete" title="删除" placement="top"><a-button class="delBtn" size="small" type="primary" danger :disabled="!selectedKeys.length || deleting" :loading="deleting" @click="deleteRelations(selectedRecords)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /><span>&nbsp;批量删除</span></a-button></a-tooltip>
            <span>已选 {{ selectedKeys.length }} 条；每个方向独立编辑、删除。</span>
          </a-space>
          <a-table :columns="columns" :data-source="visibleTopology.edges" row-key="id" size="small" :locale="tableLocale" :scroll="{ x: 1180 }" :row-selection="canDelete ? rowSelection : undefined" :pagination="pagination" @change="handleTableChange">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'source'"><a-button type="link" @click="showNodeById(record.source_service_id)">{{ record.source_name }}</a-button></template>
              <template v-else-if="column.key === 'target'"><a-button type="link" @click="showNodeById(record.target_service_id)">{{ record.target_name }}</a-button></template>
              <template v-else-if="column.key === 'type'">{{ record.interaction_type_name }}<a-tag v-if="!record.interaction_type_enabled">已停用</a-tag></template>
              <template v-else-if="column.key === 'strength'"><a-tag :color="record.strength === 'strong' ? 'blue' : record.strength === 'weak' ? 'orange' : 'default'">{{ strengthLabels[record.strength] }}</a-tag></template>
              <template v-else-if="column.key === 'function'"><a-button type="link" @click="showRelation(record)">{{ record.function_scope }}</a-button></template>
              <template v-else-if="column.key === 'action'">
                <a-space>
                  <a-tooltip v-if="canUpdate" title="编辑" placement="top"><a-button size="small" type="primary" :disabled="deleting || !!typesError" @click="editRelation(record)"><FontAwesomeIcon :icon="['fas', 'pen-to-square']" /></a-button></a-tooltip>
                  <a-tooltip v-if="canDelete" title="删除" placement="top"><a-button class="delBtn" size="small" type="primary" danger :disabled="deleting" @click="deleteRelations([record])"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip>
                </a-space>
              </template>
            </template>
          </a-table>
        </template>
      </template>
    </a-spin>
    <ServiceDependencyDialog v-if="dialogOpen" v-model:open="dialogOpen" :record="editingRelation" :services="snapshot?.nodes || []" :types="types" :initial-source="treeScope.service" @saved="onRelationSaved" />
    <ServiceDependencyTypes v-if="typesOpen" v-model:open="typesOpen" @changed="onTypesChanged" />
    <a-drawer :open="!!selectedNode" :title="selectedNode?.name || '服务状态'" :width="580" @close="selectedNode = null">
      <template v-if="selectedNode">
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="所属业务">{{ selectedNode.business_system_name }}</a-descriptions-item>
          <a-descriptions-item label="服务编码">{{ selectedNode.code }}</a-descriptions-item>
          <a-descriptions-item label="状态汇总">{{ runtimePresentation(selectedNode.runtime_summary).label }}</a-descriptions-item>
        </a-descriptions>
        <a-alert type="info" show-icon message="仅显示启用关联且启用部署的实例。运行不等于业务健康，检查异常不等于已停止；未知包含尚未检查。" style="margin: 16px 0" />
        <a-empty v-if="!selectedNode.instances.length" description="暂无启用实例" />
        <a-card v-for="instance in selectedNode.instances" :key="instance.id" size="small" :title="instance.instance_name" style="margin-bottom: 12px">
          <div>{{ instance.host_ip || `主机 #${instance.host_id}` }} · {{ instance.ha_role || '未设置角色' }}</div>
          <div>状态：{{ instance.last_status_check_time ? instanceStatus[instance.runtime_status] || '未知' : '未知（尚未检查）' }}</div>
          <div>上次检查：{{ instance.last_status_check_time ? formatTime(instance.last_status_check_time) : '尚未检查' }}</div>
          <pre v-if="instance.runtime_status_output" class="instance-output">{{ instance.runtime_status_output }}</pre>
        </a-card>
      </template>
    </a-drawer>
    <a-drawer :open="!!selectedRelation" title="依赖关系详情" :width="520" @close="selectedRelation = null">
      <template v-if="selectedRelation">
        <a-descriptions :column="1" bordered>
          <a-descriptions-item label="方向">{{ selectedRelation.source_name }} → {{ selectedRelation.target_name }}</a-descriptions-item>
          <a-descriptions-item label="交互类型">{{ selectedRelation.interaction_type_name }}</a-descriptions-item>
          <a-descriptions-item label="依赖强度">{{ strengthLabels[selectedRelation.strength] }}</a-descriptions-item>
          <a-descriptions-item label="功能范围">{{ selectedRelation.function_scope }}</a-descriptions-item>
          <a-descriptions-item label="协议 / 通道">{{ selectedRelation.protocol || '未填写' }}</a-descriptions-item>
          <a-descriptions-item label="故障影响">{{ selectedRelation.failure_impact || '未填写' }}</a-descriptions-item>
          <a-descriptions-item label="故障处理说明">{{ selectedRelation.fallback_plan || '未填写' }}</a-descriptions-item>
        </a-descriptions>
        <a-space style="margin-top: 16px">
          <a-tooltip v-if="canUpdate" title="编辑" placement="top"><a-button size="small" type="primary" :disabled="!!typesError" @click="editRelation(selectedRelation)"><FontAwesomeIcon :icon="['fas', 'pen-to-square']" /></a-button></a-tooltip>
          <a-tooltip v-if="canDelete" title="删除" placement="top"><a-button class="delBtn" size="small" type="primary" danger :disabled="deleting" @click="deleteRelations([selectedRelation])"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip>
        </a-space>
      </template>
    </a-drawer>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { getApplicationServiceList, getBusinessEnvironmentList, getBusinessSystemList, getProjectList } from '@/api/assets/application'
import { deleteServiceDependencies, getServiceDependencyTopology, getServiceDependencyTypes } from '@/api/assets/serviceDependencies'
import { checkPermission } from '@/directives/permission/permission'
import store from '@/store'
import { assertBatchDeleteSucceeded, openDeleteConfirm } from '@/util/deleteConfirm'
import { fetchAllPages } from '@/util/fetchAllPages'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatTimeWithTimezone } from '@/util/timezone'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { dependencyData, dependencyPage, resolveDependencyScope, runtimePresentation, selectDependencyTopology, strengthLabels, strengthOptions } from './dependencyGraph'
import ServiceDependencyDialog from './ServiceDependencyDialog.vue'
import ServiceDependencyGraph from './ServiceDependencyGraph.vue'
import ServiceDependencyTypes from './ServiceDependencyTypes.vue'

const props = defineProps({ scope: { type: Object, required: true } })
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const canCreate = computed(() => checkPermission('assets:applications:create'))
const canUpdate = computed(() => checkPermission('assets:applications:update'))
const canDelete = computed(() => checkPermission('assets:applications:delete'))
const formatTime = (value) => formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai')
const instanceStatus = { running: '运行', stopped: '停止', error: '检查异常', unknown: '未知' }
const projects = ref([])
const environments = ref([])
const businesses = ref([])
const services = ref([])
const types = ref([])
const projectId = ref()
const environmentId = ref()
const loadingCatalog = ref(false)
const catalogReady = ref(false)
const catalogError = ref('')
const typesError = ref('')
const loadingSnapshot = ref(false)
const snapshotError = ref('')
const operationError = ref('')
const snapshot = ref(null)
const filters = reactive({ search: '', strength: undefined, type: undefined })
const view = ref('graph')
const dialogOpen = ref(false)
const typesOpen = ref(false)
const editingRelation = ref(null)
const selectedNode = ref(null)
const selectedRelation = ref(null)
const selectedKeys = ref([])
const pagination = reactive(createPagination())
const deleting = ref(false)
let snapshotGeneration = 0
let typesGeneration = 0
let catalogGeneration = 0
let alive = true
const treeScope = computed(() => resolveDependencyScope(props.scope, businesses.value))
const layoutKey = computed(() => JSON.stringify([projectId.value, environmentId.value, treeScope.value.business_system || null, treeScope.value.service || null]))
const projectOptions = computed(() => projects.value.map((item) => ({ value: item.id, label: item.name })))
const environmentOptions = computed(() => {
  if (!projectId.value) return []
  const businessIds = new Set(businesses.value
    .filter((business) => Number(business.project) === projectId.value
      && (!treeScope.value.business_system || business.id === treeScope.value.business_system))
    .map((business) => Number(business.id)))
  // Like the ownership tree, derive environments from services, not the global dictionary or the selected environment.
  const environmentIds = new Set(services.value
    .filter((service) => businessIds.has(Number(service.business_system))
      && (!treeScope.value.service || Number(service.id) === Number(treeScope.value.service)))
    .map((service) => Number(service.environment)))
  return environments.value
    .filter((item) => item.enabled && environmentIds.has(Number(item.id)))
    .map((item) => ({ value: item.id, label: item.name }))
})
const ready = computed(() => catalogReady.value && !!projectId.value
  && environmentOptions.value.some((option) => option.value === environmentId.value) && !catalogError.value)
const selectionHint = computed(() => {
  if (projectId.value && !environmentOptions.value.length) return '当前范围暂无可用的服务环境，请先维护服务归属与环境配置。'
  if (treeScope.value.environment && !environmentOptions.value.some((option) => option.value === treeScope.value.environment)) return '当前节点环境不在可用范围，请刷新服务树并检查服务归属。'
  return '请先选择项目与环境，或在左侧选择具体环境 / 服务。'
})
const allTypeOptions = computed(() => types.value.map((item) => ({ value: item.id, label: `${item.name}${item.enabled ? '' : '（已停用）'}` })))
const visibleTopology = computed(() => snapshot.value ? selectDependencyTopology(snapshot.value, treeScope.value, filters) : { nodes: [], edges: [] })
const scopeTitle = computed(() => `${projects.value.find((item) => item.id === projectId.value)?.name || '项目'} / ${environments.value.find((item) => item.id === environmentId.value)?.name || '环境'} / ${props.scope.nodeTitle || '全部服务'}`)
const selectedRecords = computed(() => visibleTopology.value.edges.filter((edge) => selectedKeys.value.includes(edge.id)))
const rowSelection = computed(() => ({
  selectedRowKeys: selectedKeys.value,
  onChange: (keys) => {
    if (keys.length > 500) message.warning('一次最多删除 500 条关系')
    selectedKeys.value = keys.slice(0, 500)
  },
  getCheckboxProps: () => ({ disabled: deleting.value }),
}))
const columns = [
  { title: '依赖方', key: 'source', width: 170 },
  { title: '被依赖方', key: 'target', width: 170 },
  { title: '交互类型', key: 'type', width: 150 },
  { title: '依赖强度', key: 'strength', width: 100 },
  { title: '功能范围（点击查看）', key: 'function', width: 250 },
  { title: '协议 / 通道', dataIndex: 'protocol', width: 170 },
  { title: '操作', key: 'action', fixed: 'right', width: 120 },
]
const allPages = (loader) => fetchAllPages(async (params) => dependencyPage(await loader(params)))

async function refreshTypes() {
  const current = ++typesGeneration
  typesError.value = ''
  try {
    const records = await allPages(getServiceDependencyTypes)
    if (current === typesGeneration) types.value = records
  } catch (failure) {
    if (current === typesGeneration) {
      types.value = []
      typesError.value = failure.message || '加载交互类型失败'
    }
  }
}
async function loadCatalog() {
  const current = ++catalogGeneration
  loadingCatalog.value = true
  catalogReady.value = false
  catalogError.value = ''
  try {
    const [projectRows, environmentRows, businessRows, serviceRows] = await Promise.all([
      allPages(getProjectList), allPages(getBusinessEnvironmentList), allPages(getBusinessSystemList), allPages(getApplicationServiceList), refreshTypes(),
    ])
    if (current !== catalogGeneration) return
    projects.value = projectRows
    environments.value = environmentRows
    businesses.value = businessRows
    services.value = serviceRows
    catalogReady.value = true
    syncTreeScope()
  } catch (failure) {
    if (current === catalogGeneration) {
      catalogReady.value = false
      catalogError.value = failure.message || '加载项目与环境失败'
    }
  } finally { if (current === catalogGeneration) loadingCatalog.value = false }
}

function syncTreeScope() {
  dialogOpen.value = false
  selectedNode.value = null
  selectedRelation.value = null
  if (!catalogReady.value) return
  projectId.value = treeScope.value.project
  environmentId.value = treeScope.value.environment
  reconcileEnvironment()
}

async function refresh(prefix = '', preservePositions = true) {
  const current = ++snapshotGeneration
  if (!preservePositions) snapshot.value = null
  snapshotError.value = ''
  operationError.value = ''
  selectedKeys.value = []
  selectedNode.value = null
  selectedRelation.value = null
  if (!ready.value) { snapshot.value = null; loadingSnapshot.value = false; return }
  loadingSnapshot.value = true
  const params = { project: projectId.value, environment: environmentId.value }
  try {
    if (props.scope.businessSystemId && !treeScope.value.business_system) throw new Error('当前业务不在可访问的业务列表中，请刷新服务树')
    if (treeScope.value.business_system && Number(businesses.value.find((item) => item.id === treeScope.value.business_system)?.project) !== params.project) throw new Error('当前业务尚未关联该项目，请先完善业务归属')
    const data = dependencyData(await getServiceDependencyTopology(params))
    if (!Array.isArray(data.nodes) || !Array.isArray(data.edges) || data.state_source !== 'stored_runtime_snapshot' || !data.snapshot_time) throw new Error('服务拓扑返回格式异常')
    if (current !== snapshotGeneration) return
    if (treeScope.value.service && !data.nodes.some((node) => node.id === Number(treeScope.value.service))) throw new Error('当前服务已不存在或不属于所选项目与环境，请刷新服务树')
    snapshot.value = data
  } catch (failure) {
    if (current === snapshotGeneration) {
      snapshot.value = null
      snapshotError.value = `${typeof prefix === 'string' ? prefix : ''}${failure.message || '读取拓扑快照失败'}`
    }
  } finally { if (current === snapshotGeneration) loadingSnapshot.value = false }
}

function editRelation(record) {
  editingRelation.value = record
  selectedRelation.value = null
  dialogOpen.value = true
}
function showNode(node) { selectedRelation.value = null; selectedNode.value = node }
function showNodeById(id) { const node = snapshot.value?.nodes.find((item) => item.id === id); if (node) showNode(node) }
function showRelation(edge) { selectedNode.value = null; selectedRelation.value = edge }
function onRelationSaved() { refresh('关系已保存，但刷新失败：') }
function onTypesChanged() {
  // Edges embed type labels and enabled flags, so updating only dropdown options would leave the list stale.
  return Promise.all([refreshTypes(), refresh('类型更新已处理，但拓扑刷新失败：')])
}
function handleTableChange(page) { Object.assign(pagination, { current: page.current, pageSize: page.pageSize }) }

function deleteRelations(records) {
  const ids = records.map((record) => record.id)
  if (!ids.length || deleting.value) return
  const currentScope = snapshotGeneration
  openDeleteConfirm({
    title: '删除依赖关系',
    summary: '只删除清单中的有向关系，不删除服务或实例；另一个方向的依赖保留。',
    items: records.map((record) => `${record.source_name} → ${record.target_name} · ${record.function_scope}（#${record.id}）`),
    onConfirm: async () => {
      deleting.value = true
      let failure
      try { assertBatchDeleteSucceeded(await deleteServiceDependencies(ids), ids) } catch (reason) { failure = reason }
      if (alive && currentScope === snapshotGeneration) {
        await refresh('删除请求已处理，但刷新失败：')
        if (failure) operationError.value = failure.message || '删除依赖关系失败'
        else message.success('依赖关系已删除')
      }
      deleting.value = false
      if (failure) throw failure
    },
  })
}

// Scope changes invalidate outstanding requests before their responses can be painted in a different environment.
watch(() => props.scope, syncTreeScope, { deep: true })
// Resolve the cascade before loading a snapshot; an explicit tree environment must never be replaced by another one.
function reconcileEnvironment() {
  const options = environmentOptions.value
  if (options.some((option) => option.value === environmentId.value)) return
  const next = !treeScope.value.environment && options.length === 1 ? options[0].value : undefined
  if (environmentId.value !== next) environmentId.value = next
}
watch(environmentOptions, reconcileEnvironment, { flush: 'sync' })
watch(environmentId, reconcileEnvironment)
watch([projectId, environmentId, () => treeScope.value.business_system, () => treeScope.value.service, ready], () => {
  dialogOpen.value = false
  Object.assign(filters, { search: '', strength: undefined, type: undefined })
  refresh('', false)
})
watch(visibleTopology, () => {
  selectedKeys.value = selectedKeys.value.filter((id) => visibleTopology.value.edges.some((edge) => edge.id === id))
  Object.assign(pagination, { current: 1, total: visibleTopology.value.edges.length })
})
onMounted(loadCatalog)
onBeforeUnmount(() => { alive = false; snapshotGeneration++; typesGeneration++; catalogGeneration++ })
defineExpose({ refresh })
</script>

<style scoped>
.dependency-workspace { padding: 8px 0 16px; }
.workspace-toolbar, .filter-toolbar { display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin: 16px 0; }
.snapshot-caption { color: #61728a; font-size: 12px; display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; }
.batch-toolbar { margin-bottom: 12px; color: #8190a3; font-size: 12px; }
.instance-output { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 200px; overflow: auto; margin: 10px 0 0; background: #f5f7fa; padding: 8px; }
</style>
