<template>
  <div>
    <ServiceTreeLayout class="service-tree-page" navigation-id="service-tree-navigation" @resize="resizeCharts">
      <template #tree>
        <ServiceTree
          ref="serviceTreeRef"
          :selected-scope="serviceScope"
          :show-stats="false"
          stats-by-scope
          group-by-project
          @stats-change="handleStatsChange"
          @hosts-loaded="hostSnapshots = $event"
          @select="serviceScope = $event"
        />
      </template>
      <main class="service-tree-content">
        <a-tabs v-model:active-key="activePanel">
          <a-tab-pane key="info" tab="服务信息" />
          <a-tab-pane key="dependencies" tab="依赖关系" />
        </a-tabs>
        <div v-if="activePanel === 'info'" class="service-tree-detail-toolbar">
          <a-tooltip v-if="serviceScope.nodeType === 'all' || serviceScope.nodeType === 'project'" title="新增业务系统">
            <a-button v-permission="'assets:service-tree:manage'" type="primary" @click="openBusinessSystem()">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp;新增业务系统</span>
            </a-button>
          </a-tooltip>
          <a-tooltip v-if="canCreateService" title="新增逻辑服务">
            <a-button v-permission="'assets:service-tree:manage'" type="primary" @click="openApplicationService()">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp;新增逻辑服务</span>
            </a-button>
          </a-tooltip>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="refreshing" @click="refreshServiceTree">
              <ReloadOutlined />
              <span>刷新</span>
            </a-button>
          </a-tooltip>
        </div>
        <section v-show="activePanel === 'info' && showResourceStats" class="service-tree-right-stats">
          <div class="service-tree-right-stats-header">
            <span>资源占比</span>
          </div>
          <div class="service-tree-right-stats-subtitle">
            总容量按当前范围的主机去重；按{{ resourceDimensionLabel }}归集容量，组内去重、组间可能重叠。饼图百分比以各组容量之和为分母，不代表实际资源分配或实时使用率。
          </div>
          <div class="service-tree-right-stats-grid">
            <div class="service-tree-right-chart-card">
              <div class="service-tree-right-chart-title">
                <span>CPU 归集占比</span>
                <strong>去重总 CPU {{ formatResource(statsData.totalCpu, ' 核') }}</strong>
              </div>
              <div ref="hostPieRef" class="service-tree-right-chart"></div>
            </div>
            <div class="service-tree-right-chart-card">
              <div class="service-tree-right-chart-title">
                <span>内存归集占比</span>
                <strong>去重总内存 {{ formatResource(statsData.totalMemory, ' GB') }}</strong>
              </div>
              <div ref="deploymentPieRef" class="service-tree-right-chart"></div>
            </div>
          </div>
        </section>
        <!-- 卸载详情以停止状态检查轮询，依赖页只读已有快照。 -->
        <ServiceTreeNodeContent
          v-if="activePanel === 'info'"
          ref="nodeContentRef"
          :scope="serviceScope"
          :host-snapshots="hostSnapshots"
          @navigate="serviceScope = $event"
          @edit-business-system="openBusinessSystem"
          @delete-business-system="confirmDeleteBusinessSystem"
          @edit-service="openApplicationService"
          @copy-service="copyApplicationService"
          @delete-service="confirmDeleteService"
        />
        <template v-else>
          <ServiceDependencyWorkspace v-if="canViewDependencies" ref="dependencyWorkspaceRef" :scope="serviceScope" />
          <a-alert v-else type="warning" show-icon message="没有查看服务依赖的权限（assets:applications:view）。" />
        </template>
      </main>
    </ServiceTreeLayout>

    <BusinessSystemDialog
      :open="businessSystemDialogOpen"
      :system-id="selectedBusinessSystemId"
      :initial-project-id="businessSystemInitialProjectId"
      @update:open="businessSystemDialogOpen = $event"
      @saved="handleBusinessSystemSaved"
    />
    <ApplicationServiceDialog
      :open="serviceDialogOpen"
      :service-id="selectedServiceId"
      :copy-service-id="copyServiceId"
      :initial-business-system-id="serviceCreateInitialBusinessSystemId"
      :initial-environment-id="serviceCreateInitialEnvironmentId"
      @update:open="serviceDialogOpen = $event"
      @saved="handleApplicationServiceSaved"
    />
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import * as echarts from 'echarts'
import { Alert, message, Spin } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import ServiceTreeLayout from '@/components/ServiceTreeLayout.vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { batchDeleteBusinessSystems, batchDeleteApplicationServices } from '@/api/assets/application'
import { checkPermission } from '@/directives/permission/permission'
import ServiceTree from '../application/components/ServiceTree.vue'
import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'
import BusinessSystemDialog from '../application/components/BusinessSystemDialog.vue'
import ApplicationServiceDialog from '../application/components/ApplicationServiceDialog.vue'

const ServiceDependencyWorkspace = defineAsyncComponent({
  loader: () => import('./ServiceDependencyWorkspace.vue'),
  loadingComponent: { render: () => h(Spin) },
  errorComponent: { render: () => h(Alert, { type: 'error', showIcon: true, message: '依赖工作台加载失败，请刷新页面重试' }) },
})
const activePanel = ref('info')
const dependencyWorkspaceRef = ref(null)
const canViewDependencies = computed(() => checkPermission('assets:applications:view'))
const serviceScope = ref({ nodeType: 'all', nodeTitle: '全部业务' })
const hostSnapshots = ref([])
const route = useRoute()
const serviceTreeRef = ref(null)
const nodeContentRef = ref(null)
const refreshing = ref(false)
const businessSystemDialogOpen = ref(false)
const selectedBusinessSystemId = ref(null)
const businessSystemInitialProjectId = ref(null)
const serviceDialogOpen = ref(false)
const selectedServiceId = ref(null)
const copyServiceId = ref(null)
const serviceCreateInitialBusinessSystemId = ref(null)
const serviceCreateInitialEnvironmentId = ref(null)
// 只有能定位到业务系统层级以上的节点才适合新增逻辑服务（service/deployment 自身就是详情，不再支持再新增）。
const canCreateService = computed(() => ['all', 'project', 'businessSystem', 'environment'].includes(serviceScope.value.nodeType))
const statsData = ref({ cpuRows: [], memoryRows: [], totalCpu: 0, totalMemory: 0 })
const resourceDimensionLabel = computed(() => ({
  all: '项目', project: '业务系统', businessSystem: '环境', environment: '逻辑服务',
}[serviceScope.value.nodeType] || '项目'))
const showResourceStats = computed(() => ['all', 'project', 'businessSystem', 'environment'].includes(serviceScope.value.nodeType))
const hostPieRef = ref(null)
const deploymentPieRef = ref(null)
let hostPieChart = null
let deploymentPieChart = null

function handleStatsChange(payload) {
  statsData.value = {
    cpuRows: Array.isArray(payload?.cpuRows) ? payload.cpuRows : [],
    memoryRows: Array.isArray(payload?.memoryRows) ? payload.memoryRows : [],
    totalCpu: Number(payload?.totalCpu || 0),
    totalMemory: Number(payload?.totalMemory || 0),
  }
}

function formatResource(value, unit) {
  return `${Number(value || 0).toFixed(2)}${unit}`
}

function buildPieOption(title, rows, unit) {
  return {
    tooltip: {
      trigger: 'item',
      formatter: (params) => `${params.name}: ${Number(params.value).toFixed(2)} ${unit} (归集占比 ${params.percent}%)`,
    },
    series: [
      {
        name: title,
        type: 'pie',
        radius: ['40%', '72%'],
        center: ['50%', '54%'],
        avoidLabelOverlap: true,
        label: {
          show: true,
          formatter: (params) => `${params.name}\n${Number(params.value).toFixed(2)}`,
        },
        labelLine: { length: 8, length2: 8 },
        data: rows,
      },
    ],
  }
}

function ensureCharts() {
  if (hostPieRef.value && !hostPieChart) {
    hostPieChart = echarts.init(hostPieRef.value)
  }
  if (deploymentPieRef.value && !deploymentPieChart) {
    deploymentPieChart = echarts.init(deploymentPieRef.value)
  }
}

function renderCharts() {
  ensureCharts()
  if (hostPieChart) {
    hostPieChart.setOption(buildPieOption('CPU 归集占比', statsData.value.cpuRows || [], '核'), true)
  }
  if (deploymentPieChart) {
    deploymentPieChart.setOption(buildPieOption('内存归集占比', statsData.value.memoryRows || [], 'GB'), true)
  }
}

function resizeCharts() {
  hostPieChart?.resize()
  deploymentPieChart?.resize()
}

function destroyCharts() {
  hostPieChart?.dispose()
  deploymentPieChart?.dispose()
  hostPieChart = null
  deploymentPieChart = null
}

async function refreshServiceTree() {
  refreshing.value = true
  try {
    await Promise.all([
      serviceTreeRef.value?.refresh(),
      activePanel.value === 'dependencies'
        ? dependencyWorkspaceRef.value?.refresh()
        : nodeContentRef.value?.refresh(),
    ])
  } finally {
    refreshing.value = false
  }
}

function openBusinessSystem(record = null) {
  selectedBusinessSystemId.value = record?.id || null
  // 新增且当前选中的是项目节点时预填所属项目，不用用户再选一遍；编辑时不需要(字段会从记录本身加载)。
  businessSystemInitialProjectId.value = record
    ? null
    : (serviceScope.value.nodeType === 'project' ? serviceScope.value.projectId : null)
  businessSystemDialogOpen.value = true
}

async function handleBusinessSystemSaved() {
  await refreshServiceTree()
}

function confirmDeleteBusinessSystem(record) {
  openDeleteConfirm({
    title: '删除业务系统',
    summary: '仍包含逻辑服务的业务系统不能删除。',
    items: [record.name || record.code || record.id],
    onConfirm: async () => {
      await batchDeleteBusinessSystems([record.id])
      message.success('删除成功')
      // 删的正是当前选中的节点时，节点已经不存在了，退回全部业务，避免详情停留在一个已删除的节点上。
      if (serviceScope.value.nodeType === 'businessSystem' && serviceScope.value.businessSystemId === record.id) {
        serviceScope.value = { nodeType: 'all', nodeTitle: '全部业务' }
      }
      await refreshServiceTree()
    },
  })
}

function openApplicationService(record = null) {
  copyServiceId.value = null
  selectedServiceId.value = record?.id || null
  // 新增时按当前树选中的业务系统/环境预填，不用用户再选一遍；编辑时不需要(字段会从记录本身加载)。
  serviceCreateInitialBusinessSystemId.value = record ? null : (serviceScope.value.businessSystemId ?? null)
  serviceCreateInitialEnvironmentId.value = record ? null : (serviceScope.value.environment ?? null)
  serviceDialogOpen.value = true
}

async function handleApplicationServiceSaved() {
  await refreshServiceTree()
}

function copyApplicationService(record) {
  selectedServiceId.value = null
  copyServiceId.value = record.id
  serviceCreateInitialBusinessSystemId.value = null
  serviceCreateInitialEnvironmentId.value = null
  serviceDialogOpen.value = true
}

function confirmDeleteService(record) {
  openDeleteConfirm({
    title: '删除逻辑服务',
    summary: '将撤销日志采集，并永久删除该服务所有档位的数据流（data stream）、底层索引及全部日志，删除服务配置、成员关联及专属部署实例记录；共享实例、主机、实际应用和原始日志文件保留。删除失败会保留服务记录，请重试删除。',
    items: [record.name || record.code || record.id],
    onConfirm: async () => {
      const response = await batchDeleteApplicationServices([record.id])
      const result = response?.data?.data?.results?.find((item) => Number(item.id) === Number(record.id))
      if (!result?.ok) {
        const reason = result?.message || '删除未完成，请重试'
        message.error(reason)
        await refreshServiceTree()
        throw new Error(reason)
      }
      message.success('删除成功')
      if (serviceScope.value.nodeType === 'service' && serviceScope.value.applicationServiceId === record.id) {
        serviceScope.value = { nodeType: 'all', nodeTitle: '全部业务' }
      }
      // 先让子组件收到新范围，再刷新，避免仍按已删除的服务 ID 请求详情。
      await nextTick()
      await refreshServiceTree()
    },
  })
}

watch(
  () => [statsData.value.cpuRows, statsData.value.memoryRows, showResourceStats.value, activePanel.value],
  async () => {
    await nextTick()
    renderCharts()
    if (showResourceStats.value) resizeCharts()
  },
  { deep: true },
)

onMounted(() => {
  window.addEventListener('resize', resizeCharts)
  applyQueryScope()
})

// 深链定位：日志处理规则页「影响服务数」弹窗点服务名会带 query 跳过来
// （application_service_id 必填，其余用于面包屑）。只设 serviceScope——树组件会按
// scopeKey 反推选中态高亮对应节点。
function applyQueryScope() {
  const query = route.query
  const serviceId = Number(query.application_service_id)
  if (!Number.isInteger(serviceId) || serviceId <= 0) return
  const environmentId = Number(query.environment_id)
  serviceScope.value = {
    nodeType: 'service',
    applicationServiceId: serviceId,
    nodeTitle: String(query.service_name || `服务 ${serviceId}`),
    businessSystemId: Number(query.business_system_id) || undefined,
    environment: Number.isInteger(environmentId) && environmentId > 0 ? environmentId : null,
    environmentName: String(query.environment_name || ''),
    projectId: Number(query.project_id) || undefined,
    projectName: String(query.project_name || ''),
    businessSystemName: String(query.business_system_name || ''),
    serviceName: String(query.service_name || `服务 ${serviceId}`),
  }
  const deploymentId = Number(query.deployment_id)
  if (Number.isInteger(deploymentId) && deploymentId > 0) {
    serviceScope.value = {
      ...serviceScope.value, nodeType: 'deployment', deploymentId,
      nodeTitle: `部署 ${deploymentId}`,
    }
  }
}

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeCharts)
  destroyCharts()
})
</script>

<style scoped>
.service-tree-detail-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 12px;
}
.service-tree-detail-toolbar .ant-btn { display: inline-flex; align-items: center; gap: 6px; }
.service-tree-content {
  flex: 1;
  min-width: 0;
}
.service-tree-right-stats {
  margin-bottom: 12px;
  border: 1px solid #edf1f6;
  border-radius: 8px;
  background: #fafcff;
  padding: 10px;
}
.service-tree-right-stats-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
  color: #172033;
  font-size: 13px;
  font-weight: 600;
}
.service-tree-right-stats-subtitle {
  margin-bottom: 10px;
  color: #7a8697;
  font-size: 12px;
}
.service-tree-right-stats-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.service-tree-right-chart-card {
  border: 1px solid #eef2f7;
  border-radius: 8px;
  background: #fff;
  padding: 8px;
}
.service-tree-right-chart-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: #425066;
  font-size: 12px;
  margin-bottom: 4px;
}
.service-tree-right-chart-title strong {
  color: #172033;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
.service-tree-right-chart {
  width: 100%;
  height: 220px;
}
@media (max-width: 900px) {
  .service-tree-content { width: 100%; }
  .service-tree-right-stats-grid {
    grid-template-columns: 1fr;
  }
}
</style>