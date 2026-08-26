<template>
  <aside class="service-tree">
    <header class="service-tree-header">
      <div>
        <div class="service-tree-title">服务树</div>
        <div class="service-tree-subtitle">业务系统 / 服务 / 实例</div>
      </div>
      <a-badge :count="systemCount" :number-style="{ backgroundColor: '#1677ff' }" />
    </header>

    <a-input-search
      v-model:value="searchText"
      allow-clear
      placeholder="搜索系统、环境、服务或实例"
      class="service-tree-search"
    />

    <div class="service-tree-filter">
      <span class="service-tree-filter-label">项目</span>
      <a-select
        v-model:value="selectedProjects"
        mode="multiple"
        allow-clear
        show-search
        max-tag-count="responsive"
        :max-tag-placeholder="selectedProjectPlaceholder"
        :options="projectOptions"
        placeholder="全部项目"
        class="service-tree-project-filter"
        :getPopupContainer="getPopupContainer"
        @change="handleProjectChange"
      />
    </div>
    <div class="service-tree-filter">
      <span class="service-tree-filter-label">环境</span>
      <a-select
        v-model:value="selectedEnvironments"
        mode="multiple"
        allow-clear
        show-search
        max-tag-count="responsive"
        :max-tag-placeholder="selectedEnvironmentPlaceholder"
        :options="environmentOptions"
        placeholder="全部环境"
        class="service-tree-environment-filter"
        :getPopupContainer="getPopupContainer"
        @change="handleEnvironmentChange"
      />
    </div>

    <div v-if="props.showStats" class="service-tree-stats">
      <div class="service-tree-stats-header">
        <span>资源占比</span>
        <a-segmented
          v-model:value="currentStatsDimension"
          :options="statsDimensionOptions"
          size="small"
        />
      </div>
      <div class="service-tree-stats-subtitle">按 CPU / 内存 资源汇总</div>
      <div v-if="cpuPieData.length || memoryPieData.length" class="service-tree-stats-grid">
        <div class="service-tree-chart-card">
          <div class="service-tree-chart-title">CPU 占比</div>
          <div ref="hostPieRef" class="service-tree-chart"></div>
        </div>
        <div class="service-tree-chart-card">
          <div class="service-tree-chart-title">内存占比</div>
          <div ref="deploymentPieRef" class="service-tree-chart"></div>
        </div>
      </div>
      <a-empty v-else :image="simpleImage" description="暂无统计数据" />
    </div>

    <a-spin :spinning="loading">
      <a-tree
        v-if="filteredTreeData.length"
        block-node
        show-line
        virtual
        :height="treeHeight"
        :tree-data="filteredTreeData"
        :selected-keys="selectedKeys"
        :expanded-keys="expandedKeys"
        @expand="handleExpand"
        @select="handleSelect"
      >
        <template #title="node">
          <div class="service-tree-node">
            <span class="service-tree-node-label">{{ node.title }}</span>
          </div>
        </template>
      </a-tree>
      <a-empty v-else :image="simpleImage" description="暂无服务" />
    </a-spin>
  </aside>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Empty, message } from 'ant-design-vue'
import * as echarts from 'echarts'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { fetchAllPages } from '@/util/fetchAllPages'
import {
  getApplicationDeploymentList,
  getApplicationServiceList,
  getBusinessEnvironmentList,
  getBusinessSystemList,
  getProjectList,
} from '@/api/assets/application'
import { getHostList } from '@/api/assets/host'

const props = defineProps({
  selectedScope: { type: Object, default: () => ({ nodeType: 'all' }) },
  showStats: { type: Boolean, default: true },
  statsDimension: { type: String, default: 'business' },
})
const emit = defineEmits(['select', 'stats-change', 'update:statsDimension'])
const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const loading = ref(false)
const searchText = ref('')
const selectedEnvironments = ref([])
const selectedProjects = ref([])
const projectRecords = ref([])
const environmentRecords = ref([])
const treeData = ref([])
const selectedKeys = ref(['all'])
const expandedKeys = ref([])
const treeHeight = ref(420)
const debouncedSearchText = ref('')
const searchDebounceTimer = ref(null)
const scopeByKey = new Map()
const lazyDeploymentChildrenByServiceKey = new Map()
const statsDimensionOptions = [
  { label: '按业务', value: 'business' },
  { label: '按项目', value: 'project' },
]
const hostPieRef = ref(null)
const deploymentPieRef = ref(null)
const hostPieChart = ref(null)
const deploymentPieChart = ref(null)
const chartServices = ref([])
const chartDeployments = ref([])
const chartHosts = ref([])
const chartProjects = ref([])
const chartSystems = ref([])
const currentStatsDimension = computed({
  get: () => (props.statsDimension === 'project' ? 'project' : 'business'),
  set: (value) => {
    emit('update:statsDimension', value)
  },
})
const SEARCH_DEBOUNCE_MS = 180
const MIN_TREE_HEIGHT = 320

function getDeploymentHostId(deployment) {
  const hostId = Number(deployment?.host_id ?? deployment?.host)
  return Number.isInteger(hostId) && hostId > 0 ? hostId : null
}

function buildTopNPieData(sourceRows, limit = 8) {
  const sorted = [...sourceRows]
    .filter((item) => Number(item.value) > 0)
    .sort((left, right) => right.value - left.value)

  if (sorted.length <= limit) {
    return sorted
  }
  const topRows = sorted.slice(0, limit)
  const otherValue = sorted.slice(limit).reduce((sum, item) => sum + Number(item.value || 0), 0)
  if (otherValue > 0) {
    topRows.push({ name: '其他', value: otherValue })
  }
  return topRows
}

const pieStats = computed(() => {
  const serviceList = chartServices.value
  const deploymentList = chartDeployments.value
  const hostList = chartHosts.value
  if (!serviceList.length || !deploymentList.length || !hostList.length) {
    return { cpuRows: [], memoryRows: [] }
  }

  const systemsById = new Map(
    chartSystems.value.map((item) => [String(item.id), String(item.name || `业务-${item.id}`)]),
  )
  const serviceById = new Map(serviceList.map((item) => [Number(item.id), item]))
  const projectRows = chartProjects.value
  const hostById = new Map(hostList.map((item) => [Number(item.id), item]))

  const buckets = new Map()

  function ensureBucket(groupId, groupName) {
    const key = String(groupId)
    if (!buckets.has(key)) {
      buckets.set(key, {
        id: key,
        name: groupName || `分组-${groupId}`,
        cpuTotal: 0,
        memoryTotal: 0,
      })
    }
    return buckets.get(key)
  }

  for (const deployment of deploymentList) {
    const hostId = getDeploymentHostId(deployment)
    const host = hostId != null ? hostById.get(hostId) : null
    if (!host) continue

    const linkedServiceIds = (deployment.application_service_ids || [])
      .map((item) => Number(item))
      .filter((item) => Number.isInteger(item) && item > 0)
    const linkedServices = linkedServiceIds
      .map((id) => serviceById.get(id))
      .filter((item) => item && typeof item === 'object')

    if (!linkedServices.length) {
      continue
    }

    const targetGroups = new Map()
    const hostCpu = Number(host.hardware?.cpu_cores ?? host.cpu_cores ?? 0) || 0
    const hostMemory = Number(host.hardware?.memory_gb ?? host.memory_gb ?? 0) || 0

    if (currentStatsDimension.value === 'project') {
      for (const service of linkedServices) {
        const businessId = String(service.business_system || '')
        if (!businessId) continue
        for (const project of projectRows) {
          const relatedSystems = Array.isArray(project.business_systems) ? project.business_systems : []
          if (relatedSystems.map((id) => String(id)).includes(businessId)) {
            targetGroups.set(String(project.id), String(project.name || `项目-${project.id}`))
          }
        }
      }
    } else {
      for (const service of linkedServices) {
        const businessId = service.business_system
        if (!businessId) continue
        const businessName = String(
          service.business_system_name || systemsById.get(String(businessId)) || `业务-${businessId}`,
        )
        targetGroups.set(String(businessId), businessName)
      }
    }

    for (const [groupId, groupName] of targetGroups.entries()) {
      const bucket = ensureBucket(groupId, groupName)
      bucket.cpuTotal += hostCpu
      bucket.memoryTotal += hostMemory
    }
  }

  const cpuRows = buildTopNPieData(
    [...buckets.values()].map((item) => ({ name: item.name, value: item.cpuTotal })),
  )
  const memoryRows = buildTopNPieData(
    [...buckets.values()].map((item) => ({ name: item.name, value: item.memoryTotal })),
  )

  const totalCpu = [...buckets.values()].reduce((sum, item) => sum + Number(item.cpuTotal || 0), 0)
  const totalMemory = [...buckets.values()].reduce((sum, item) => sum + Number(item.memoryTotal || 0), 0)

  return { cpuRows, memoryRows, totalCpu, totalMemory }
})

const cpuPieData = computed(() => pieStats.value.cpuRows)
const memoryPieData = computed(() => pieStats.value.memoryRows)
const totalCpu = computed(() => pieStats.value.totalCpu || 0)
const totalMemory = computed(() => pieStats.value.totalMemory || 0)

function buildPieOption(title, sourceRows) {
  return {
    tooltip: {
      trigger: 'item',
      formatter: (params) => `${params.name}: ${Number(params.value).toFixed(2)} (${params.percent}%)`,
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
        labelLine: {
          length: 8,
          length2: 8,
        },
        data: sourceRows,
      },
    ],
  }
}

function ensureCharts() {
  if (hostPieRef.value && !hostPieChart.value) {
    hostPieChart.value = echarts.init(hostPieRef.value)
  }
  if (deploymentPieRef.value && !deploymentPieChart.value) {
    deploymentPieChart.value = echarts.init(deploymentPieRef.value)
  }
}

function renderPieCharts() {
  ensureCharts()
  if (hostPieChart.value) {
    hostPieChart.value.setOption(buildPieOption('去重主机占比', hostPieData.value), true)
  }
  if (deploymentPieChart.value) {
    deploymentPieChart.value.setOption(buildPieOption('部署实例占比', deploymentPieData.value), true)
  }
}

function resizeCharts() {
  hostPieChart.value?.resize()
  deploymentPieChart.value?.resize()
}

function disposeCharts() {
  if (hostPieChart.value) {
    hostPieChart.value.dispose()
    hostPieChart.value = null
  }
  if (deploymentPieChart.value) {
    deploymentPieChart.value.dispose()
    deploymentPieChart.value = null
  }
}
const systemCount = computed(() => treeData.value[0]?.count || 0)
const environmentOptions = computed(() => environmentRecords.value
  .filter((environment) => environment.enabled)
  .map((environment) => ({ label: environment.name, value: environment.id })))
const selectedEnvironmentPlaceholder = () => `已选 ${selectedEnvironments.value.length} 个环境`
const projectOptions = computed(() => projectRecords.value
  .filter((project) => project.enabled)
  .map((project) => ({ label: project.name, value: project.id })))
const selectedProjectPlaceholder = () => `已选 ${selectedProjects.value.length} 个项目`
const currentFilterScope = () => ({
  nodeType: 'all',
  nodeTitle: '全部业务',
  projectIds: [...selectedProjects.value],
  environmentIds: [...selectedEnvironments.value],
})

function buildTree(systems, services, deployments) {
  scopeByKey.clear()
  lazyDeploymentChildrenByServiceKey.clear()
  scopeByKey.set('all', currentFilterScope())
  const servicesBySystem = new Map()
  const orphanServices = []
  for (const service of services) {
    if (!service.business_system) {
      orphanServices.push(service)
      continue
    }
    if (!servicesBySystem.has(service.business_system)) servicesBySystem.set(service.business_system, [])
    servicesBySystem.get(service.business_system).push(service)
  }
  const deploymentsByService = new Map()
  for (const deployment of deployments) {
    for (const serviceId of deployment.application_service_ids || []) {
      if (!deploymentsByService.has(serviceId)) deploymentsByService.set(serviceId, [])
      deploymentsByService.get(serviceId).push(deployment)
    }
  }

  function buildServiceNodes(systemServices, system) {
    return [...systemServices]
      .sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
      .map((service) => {
        const serviceKey = `service:${service.id}`
        scopeByKey.set(serviceKey, {
          nodeType: 'service', applicationServiceId: service.id, nodeTitle: service.name,
          businessSystemId: system.id, businessSystemName: system.name,
          environment: service.environment || null, environmentName: service.environment_name || '',
        })
        const serviceDeployments = (deploymentsByService.get(service.id) || [])
          .sort((left, right) => left.instance_name.localeCompare(right.instance_name, 'zh-CN'))
        const deploymentNodes = serviceDeployments.map((deployment) => {
          const deploymentKey = `deployment:${deployment.id}`
          const hostLabel = deployment.host_name || deployment.host_ip || `主机-${deployment.host_id ?? deployment.host ?? deployment.id}`
          scopeByKey.set(deploymentKey, {
            nodeType: 'deployment', deploymentId: deployment.id, nodeTitle: deployment.instance_name,
            businessSystemId: system.id, businessSystemName: system.name,
            environment: service.environment || null, environmentName: service.environment_name || '',
            applicationServiceId: service.id, serviceName: service.name,
          })
          return {
            key: deploymentKey,
            title: `${deployment.instance_name} [主机: ${hostLabel}]`,
            searchText: `${deployment.instance_name} ${service.name} ${hostLabel}`.toLowerCase(),
            isLeaf: true,
          }
        })
        lazyDeploymentChildrenByServiceKey.set(serviceKey, deploymentNodes)
        return {
          key: serviceKey,
          title: `${service.name} [${service.environment_name || '未指定环境'}]`,
          searchText: `${service.name} ${service.environment_name || '未指定环境'}`.toLowerCase(),
          count: serviceDeployments.length,
          children: [],
          hasLazyChildren: deploymentNodes.length > 0,
        }
      })
  }

  const allSystems = [...systems]
  if (orphanServices.length) {
    allSystems.push({ id: 'unassigned', name: '未归属业务系统', enabled: false })
  }
  const hasResourceFilter = selectedProjects.value.length > 0 || selectedEnvironments.value.length > 0
  const systemNodes = allSystems.map((system) => {
    const systemKey = `system:${system.id}`
    if (system.id !== 'unassigned') {
      scopeByKey.set(systemKey, {
        nodeType: 'businessSystem', businessSystemId: system.id, nodeTitle: system.name,
      })
    }
    if (system.id === 'unassigned') {
      const serviceNodes = buildServiceNodes(orphanServices, system)
      return {
        key: systemKey,
        title: system.name,
        searchText: system.name.toLowerCase(),
        count: serviceNodes.length,
        children: serviceNodes,
        selectable: false,
      }
    }
    const children = buildServiceNodes(servicesBySystem.get(system.id) || [], system)
    return {
      key: systemKey,
      title: system.name,
      searchText: system.name.toLowerCase(),
      count: children.length,
      children,
      selectable: true,
    }
  }).filter((node) => !hasResourceFilter || node.children.length)

  treeData.value = [{ key: 'all', title: '全部业务', searchText: '全部业务', count: systemNodes.length, children: systemNodes }]

  const baseExpandedKeys = ['all', ...systemNodes.map((node) => node.key)]
  expandedKeys.value = baseExpandedKeys
}

function handleEnvironmentChange(values) {
  selectedEnvironments.value = values || []
  selectedKeys.value = ['all']
  emit('select', currentFilterScope())
  refresh()
}

function handleProjectChange(values) {
  selectedProjects.value = values || []
  selectedKeys.value = ['all']
  emit('select', currentFilterScope())
  refresh()
}

function filterNodes(nodes, keyword) {
  if (!keyword) return nodes
  return nodes.reduce((result, node) => {
    let sourceChildren = node.children || []
    if (!sourceChildren.length && node.hasLazyChildren) {
      sourceChildren = lazyDeploymentChildrenByServiceKey.get(node.key) || []
    }
    const children = filterNodes(sourceChildren, keyword)
    if ((node.searchText || String(node.title).toLowerCase()).includes(keyword) || children.length) {
      result.push({ ...node, children })
    }
    return result
  }, [])
}

const filteredTreeData = computed(() => filterNodes(treeData.value, debouncedSearchText.value.trim().toLowerCase()))

function materializeServiceChildren(serviceKey) {
  const lazyChildren = lazyDeploymentChildrenByServiceKey.get(serviceKey)
  if (!lazyChildren?.length) return

  const visit = (nodes) => {
    for (const node of nodes || []) {
      if (node.key === serviceKey) {
        if (!node.children?.length) node.children = lazyChildren
        return true
      }
      if (node.children?.length && visit(node.children)) return true
    }
    return false
  }

  visit(treeData.value)
}

function updateTreeHeight() {
  treeHeight.value = Math.max(MIN_TREE_HEIGHT, window.innerHeight - 360)
  resizeCharts()
}

function handleExpand(keys, info) {
  expandedKeys.value = keys
  if (info?.expanded && String(info?.node?.key || '').startsWith('service:')) {
    // 服务节点展开时再挂载实例子节点，降低大规模树初始渲染开销。
    materializeServiceChildren(info.node.key)
  }
}

function handleSelect(keys) {
  const key = keys[0] || 'all'
  selectedKeys.value = [key]
  emit('select', scopeByKey.get(key) || {})
}

function scopeKey(scope) {
  if (scope?.nodeType === 'businessSystem') return `system:${scope.businessSystemId}`
  if (scope?.nodeType === 'service') return `service:${scope.applicationServiceId}`
  if (scope?.nodeType === 'deployment') return `deployment:${scope.deploymentId}`
  return 'all'
}

watch(
  () => props.selectedScope,
  (scope) => { selectedKeys.value = [scopeKey(scope)] },
  { deep: true, immediate: true },
)

watch(searchText, (value) => {
  if (searchDebounceTimer.value) clearTimeout(searchDebounceTimer.value)
  searchDebounceTimer.value = setTimeout(() => {
    debouncedSearchText.value = value || ''
  }, SEARCH_DEBOUNCE_MS)
})

watch([cpuPieData, memoryPieData, totalCpu, totalMemory, currentStatsDimension], () => {
  emit('stats-change', {
    dimension: currentStatsDimension.value,
    cpuRows: cpuPieData.value,
    memoryRows: memoryPieData.value,
    totalCpu: totalCpu.value,
    totalMemory: totalMemory.value,
  })
  renderPieCharts()
})

async function refresh() {
  loading.value = true
  try {
    const [systems, environments, projects, services, deployments, hosts] = await Promise.all([
      fetchAllPages(getBusinessSystemList),
      fetchAllPages(getBusinessEnvironmentList),
      fetchAllPages(getProjectList),
      fetchAllPages(getApplicationServiceList),
      fetchAllPages(getApplicationDeploymentList),
      fetchAllPages(getHostList),
    ])
    environmentRecords.value = environments
    projectRecords.value = projects
    const projectIds = new Set(selectedProjects.value.map((id) => String(id)))
    const projectSystemIds = projectIds.size
      ? new Set(projects
        .filter((project) => projectIds.has(String(project.id)))
        .flatMap((project) => project.business_systems || [])
        .map((id) => String(id)))
      : null
    const environmentIds = new Set(selectedEnvironments.value)
    const visibleServices = environmentIds.size
      ? services.filter((service) => environmentIds.has(service.environment))
      : services
    const projectServices = projectSystemIds
      ? visibleServices.filter((service) => projectSystemIds.has(String(service.business_system)))
      : visibleServices
    const visibleServiceIdSet = new Set(projectServices.map((item) => Number(item.id)).filter((item) => Number.isInteger(item) && item > 0))
    const visibleDeployments = deployments.filter((deployment) => {
      const linkedServiceIds = (deployment.application_service_ids || [])
        .map((item) => Number(item))
        .filter((item) => Number.isInteger(item) && item > 0)
      return linkedServiceIds.some((serviceId) => visibleServiceIdSet.has(serviceId))
    })
    chartSystems.value = systems
    chartProjects.value = projects
    chartServices.value = projectServices
    chartDeployments.value = visibleDeployments
    chartHosts.value = hosts || []
    buildTree(systems, projectServices, deployments)
    if (selectedKeys.value[0] === 'all') emit('select', scopeByKey.get('all'))
  } catch (error) {
    message.error(error?.message || '服务树加载失败')
  } finally {
    loading.value = false
  }
}

defineExpose({ refresh })
onMounted(() => {
  updateTreeHeight()
  window.addEventListener('resize', updateTreeHeight)
  refresh()
})

onBeforeUnmount(() => {
  if (searchDebounceTimer.value) clearTimeout(searchDebounceTimer.value)
  window.removeEventListener('resize', updateTreeHeight)
  disposeCharts()
})
</script>

<style scoped>
.service-tree {
  width: 260px;
  min-width: 260px;
  height: calc(100vh - 168px);
  min-height: 520px;
  padding: 16px 12px;
  overflow: auto;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fff;
}
.service-tree-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 4px 14px;
}
.service-tree-title {
  color: #172033;
  font-size: 16px;
  font-weight: 600;
}
.service-tree-subtitle {
  margin-top: 2px;
  color: #8c95a5;
  font-size: 12px;
}
.service-tree-search { margin-bottom: 12px; }
.service-tree-filter {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.service-tree-filter-label {
  flex: 0 0 auto;
  color: #687386;
  font-size: 12px;
}
.service-tree-environment-filter {
  flex: 1 1 auto;
  min-width: 0;
  width: 0;
}
.service-tree-project-filter {
  flex: 1 1 auto;
  min-width: 0;
  width: 0;
}
.service-tree-stats {
  margin: 2px 0 12px;
  padding: 10px 8px;
  border: 1px solid #edf1f6;
  border-radius: 8px;
  background: #fafcff;
}
.service-tree-stats-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
  color: #172033;
  font-size: 12px;
  font-weight: 600;
}
.service-tree-stats-subtitle {
  margin-bottom: 8px;
  color: #7a8697;
  font-size: 11px;
}
.service-tree-stats-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
}
.service-tree-chart-card {
  border: 1px solid #eef2f7;
  border-radius: 8px;
  background: #fff;
  padding: 6px;
}
.service-tree-chart-title {
  color: #425066;
  font-size: 11px;
  margin-bottom: 2px;
}
.service-tree-chart {
  width: 100%;
  height: 160px;
}
.service-tree-node {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.service-tree-node-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.service-tree-node-count {
  min-width: 20px;
  padding: 0 6px;
  border-radius: 10px;
  color: #687386;
  background: #f1f3f6;
  font-size: 11px;
  line-height: 20px;
  text-align: center;
}
</style>
