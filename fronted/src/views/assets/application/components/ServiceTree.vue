<template>
  <aside class="service-tree">
    <header class="service-tree-header">
      <div>
        <div class="service-tree-title">服务树</div>
        <div class="service-tree-subtitle">业务系统 / 环境 / 服务 / 实例</div>
      </div>
      <a-badge :count="systemCount" :number-style="{ backgroundColor: '#1677ff' }" />
    </header>

    <a-input-search
      v-model:value="searchText"
      allow-clear
      placeholder="搜索系统、环境、服务或实例"
      class="service-tree-search"
    />

    <a-spin :spinning="loading">
      <a-tree
        v-if="filteredTreeData.length"
        block-node
        show-line
        :tree-data="filteredTreeData"
        :selected-keys="selectedKeys"
        :expanded-keys="expandedKeys"
        @expand="expandedKeys = $event"
        @select="handleSelect"
      >
        <template #title="node">
          <div class="service-tree-node">
            <span class="service-tree-node-label">{{ node.title }}</span>
            <span v-if="node.count !== undefined" class="service-tree-node-count">{{ node.count }}</span>
          </div>
        </template>
      </a-tree>
      <a-empty v-else :image="simpleImage" description="暂无服务" />
    </a-spin>
  </aside>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { Empty, message } from 'ant-design-vue'
import {
  getApplicationDeploymentList,
  getApplicationServiceList,
  getBusinessSystemList,
} from '@/api/assets/application'

const props = defineProps({
  selectedScope: { type: Object, default: () => ({ nodeType: 'all' }) },
})
const emit = defineEmits(['select'])
const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE
const environmentLabels = {
  production: '生产环境',
  testing: '测试环境',
  development: '开发环境',
  other: '其他环境',
}
const topologyLabels = { standalone: '单机', cluster: '集群' }

const loading = ref(false)
const searchText = ref('')
const treeData = ref([])
const selectedKeys = ref(['all'])
const expandedKeys = ref([])
const scopeByKey = new Map()
const systemCount = computed(() => treeData.value[0]?.count || 0)

async function fetchAll(loader, params = {}) {
  const firstResponse = await loader({ ...params, page: 1, page_size: 100 })
  const firstData = firstResponse?.data?.data || {}
  const records = [...(firstData.results || [])]
  const totalPages = Number(firstData.totalPages || 1)
  if (totalPages > 1) {
    const responses = await Promise.all(
      Array.from({ length: totalPages - 1 }, (_, index) => loader({ ...params, page: index + 2, page_size: 100 })),
    )
    for (const response of responses) records.push(...(response?.data?.data?.results || []))
  }
  return records
}

function buildTree(systems, services, deployments) {
  scopeByKey.clear()
  scopeByKey.set('all', { nodeType: 'all', nodeTitle: '全部业务' })
  const servicesBySystem = new Map()
  for (const service of services) {
    const systemKey = service.business_system || 'unassigned'
    if (!servicesBySystem.has(systemKey)) servicesBySystem.set(systemKey, [])
    servicesBySystem.get(systemKey).push(service)
  }
  const deploymentsByService = new Map()
  for (const deployment of deployments) {
    if (!deploymentsByService.has(deployment.application_service)) {
      deploymentsByService.set(deployment.application_service, [])
    }
    deploymentsByService.get(deployment.application_service).push(deployment)
  }

  const allSystems = [...systems]
  if (servicesBySystem.has('unassigned')) {
    allSystems.push({ id: 'unassigned', name: '未归属业务系统', enabled: false })
  }
  const systemNodes = allSystems.map((system) => {
    const systemServices = servicesBySystem.get(system.id) || []
    const systemKey = `system:${system.id}`
    if (system.id !== 'unassigned') {
      scopeByKey.set(systemKey, {
        nodeType: 'businessSystem', businessSystemId: system.id, nodeTitle: system.name,
      })
    }
    const servicesByEnvironment = new Map()
    for (const service of systemServices) {
      const environment = service.environment || 'other'
      if (!servicesByEnvironment.has(environment)) servicesByEnvironment.set(environment, [])
      servicesByEnvironment.get(environment).push(service)
    }
    const children = [...servicesByEnvironment.entries()]
      .sort(([left], [right]) => (environmentLabels[left] || left).localeCompare(environmentLabels[right] || right, 'zh-CN'))
      .map(([environment, environmentServices]) => {
        const environmentKey = `environment:${system.id}:${environment}`
        if (system.id !== 'unassigned') {
          scopeByKey.set(environmentKey, {
            nodeType: 'environment', businessSystemId: system.id, businessSystemName: system.name,
            environment, nodeTitle: environmentLabels[environment] || environment,
          })
        }
        const serviceNodes = environmentServices
          .sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
          .map((service) => {
            const serviceKey = `service:${service.id}`
            scopeByKey.set(serviceKey, {
              nodeType: 'service', applicationServiceId: service.id, nodeTitle: service.name,
              businessSystemId: system.id, businessSystemName: system.name,
              environment, environmentName: environmentLabels[environment] || environment,
            })
            const serviceDeployments = (deploymentsByService.get(service.id) || [])
              .sort((left, right) => left.instance_name.localeCompare(right.instance_name, 'zh-CN'))
            const deploymentNodes = serviceDeployments.map((deployment) => {
              const deploymentKey = `deployment:${deployment.id}`
              scopeByKey.set(deploymentKey, {
                nodeType: 'deployment', deploymentId: deployment.id, nodeTitle: deployment.instance_name,
                businessSystemId: system.id, businessSystemName: system.name,
                environment, environmentName: environmentLabels[environment] || environment,
                applicationServiceId: service.id, serviceName: service.name,
              })
              return {
                key: deploymentKey,
                title: deployment.is_primary ? `${deployment.instance_name} · VIP 主节点` : deployment.instance_name,
                isLeaf: true,
              }
            })
            return {
              key: serviceKey,
              title: service.name,
              count: deploymentNodes.length,
              children: deploymentNodes,
            }
          })
        return {
          key: environmentKey,
          title: environmentLabels[environment] || environment,
          count: serviceNodes.length,
          children: serviceNodes,
          selectable: system.id !== 'unassigned',
        }
      })
    return {
      key: systemKey,
      title: system.name,
      count: systemServices.length,
      children,
      selectable: system.id !== 'unassigned',
    }
  })

  treeData.value = [{ key: 'all', title: '全部业务', count: systems.length, children: systemNodes }]
  expandedKeys.value = [
    'all',
    ...systemNodes.map((node) => node.key),
    ...systemNodes.flatMap((node) => node.children.map((child) => child.key)),
    ...systemNodes.flatMap((node) => node.children.flatMap((child) => child.children.map((service) => service.key))),
  ]
}

function filterNodes(nodes, keyword) {
  if (!keyword) return nodes
  return nodes.reduce((result, node) => {
    const children = filterNodes(node.children || [], keyword)
    if (String(node.title).toLowerCase().includes(keyword) || children.length) {
      result.push({ ...node, children })
    }
    return result
  }, [])
}

const filteredTreeData = computed(() => filterNodes(treeData.value, searchText.value.trim().toLowerCase()))

function handleSelect(keys) {
  const key = keys[0] || 'all'
  selectedKeys.value = [key]
  emit('select', scopeByKey.get(key) || {})
}

function scopeKey(scope) {
  if (scope?.nodeType === 'businessSystem') return `system:${scope.businessSystemId}`
  if (scope?.nodeType === 'environment') return `environment:${scope.businessSystemId}:${scope.environment}`
  if (scope?.nodeType === 'service') return `service:${scope.applicationServiceId}`
  if (scope?.nodeType === 'deployment') return `deployment:${scope.deploymentId}`
  return 'all'
}

watch(
  () => props.selectedScope,
  (scope) => { selectedKeys.value = [scopeKey(scope)] },
  { deep: true, immediate: true },
)

async function refresh() {
  loading.value = true
  try {
    const [systems, services, deployments] = await Promise.all([
      fetchAll(getBusinessSystemList),
      fetchAll(getApplicationServiceList),
      fetchAll(getApplicationDeploymentList),
    ])
    buildTree(systems, services, deployments)
    if (selectedKeys.value[0] === 'all') emit('select', scopeByKey.get('all'))
  } catch (error) {
    message.error(error?.message || '服务树加载失败')
  } finally {
    loading.value = false
  }
}

defineExpose({ refresh })
onMounted(refresh)
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
