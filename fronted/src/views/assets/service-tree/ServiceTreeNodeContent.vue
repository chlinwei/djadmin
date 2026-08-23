<template>
  <section class="node-content">
    <a-breadcrumb class="node-breadcrumb">
      <a-breadcrumb-item v-for="item in breadcrumbs" :key="item">{{ item }}</a-breadcrumb-item>
    </a-breadcrumb>
    <header class="node-content-header">
      <div>
        <div class="node-content-kicker">{{ levelLabel }}</div>
        <h2>{{ scope.nodeTitle || '全部业务' }}</h2>
      </div>
      <a-tag :color="summaryStatus.color">{{ summaryStatus.label }}</a-tag>
    </header>

    <a-spin :spinning="loading">
      <div v-if="metrics.length" class="metric-band">
        <div v-for="metric in metrics" :key="metric.label" class="metric-item">
          <span class="metric-label">{{ metric.label }}</span>
          <strong :class="{ 'metric-danger': metric.danger }">{{ metric.value }}</strong>
        </div>
      </div>

      <a-descriptions v-if="scope.nodeType === 'businessSystem' && entity" bordered :column="{ xs: 1, sm: 2 }" size="small" class="node-summary">
        <a-descriptions-item label="系统编码">{{ entity.code || '-' }}</a-descriptions-item>
        <a-descriptions-item label="负责人">{{ entity.owner || '-' }}</a-descriptions-item>
        <a-descriptions-item label="状态"><a-badge :status="entity.enabled ? 'success' : 'default'" :text="entity.enabled ? '启用' : '停用'" /></a-descriptions-item>
        <a-descriptions-item label="备注">{{ entity.remark || '-' }}</a-descriptions-item>
      </a-descriptions>

      <a-descriptions v-else-if="scope.nodeType === 'environment'" bordered :column="{ xs: 1, sm: 2 }" size="small" class="node-summary">
        <a-descriptions-item label="业务系统">{{ scope.businessSystemName || '-' }}</a-descriptions-item>
        <a-descriptions-item label="环境">{{ environmentLabels[scope.environment] || scope.environment || '-' }}</a-descriptions-item>
      </a-descriptions>

      <a-descriptions v-else-if="scope.nodeType === 'service' && entity" bordered :column="{ xs: 1, sm: 2 }" size="small" class="node-summary">
        <a-descriptions-item label="业务系统">{{ entity.business_system_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="环境">{{ environmentLabels[entity.environment] || entity.environment || '-' }}</a-descriptions-item>
        <a-descriptions-item label="应用">{{ entity.application_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="部署形态">{{ entity.topology_type === 'cluster' ? '集群' : '单机' }}</a-descriptions-item>
        <a-descriptions-item label="集群模型">{{ entity.cluster_profile_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="可用性">{{ availabilityLabels[entity.availability_mode] || entity.availability_mode || '-' }}</a-descriptions-item>
        <a-descriptions-item label="访问入口">{{ formatAccessEndpoint(entity) }}</a-descriptions-item>
        <a-descriptions-item label="备注">{{ entity.remark || '-' }}</a-descriptions-item>
      </a-descriptions>

      <a-descriptions v-else-if="scope.nodeType === 'deployment' && detail" bordered :column="{ xs: 1, sm: 2 }" size="small" class="node-summary">
        <a-descriptions-item label="实例名称">{{ detail.instance_name }}</a-descriptions-item>
        <a-descriptions-item label="逻辑服务">{{ detail.service_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="业务系统">{{ detail.business_system_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="环境">{{ environmentLabels[detail.environment] || detail.environment || '-' }}</a-descriptions-item>
        <a-descriptions-item label="应用">{{ detail.application_name }} {{ detail.version }}</a-descriptions-item>
        <a-descriptions-item label="成员端口">{{ detail.member_port || '-' }}</a-descriptions-item>
        <a-descriptions-item label="主机">{{ detail.host_name || '-' }}（{{ detail.host_ip || '-' }}）</a-descriptions-item>
        <a-descriptions-item label="部署模板">{{ detail.template_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="运行状态"><a-badge :status="runtimeStatus[detail.runtime_status]?.status || 'default'" :text="runtimeStatus[detail.runtime_status]?.label || '未知'" /></a-descriptions-item>
        <a-descriptions-item label="健康状态">{{ healthStatus[detail.health_status] || '未检查' }}</a-descriptions-item>
        <a-descriptions-item label="备注">{{ detail.remark || '-' }}</a-descriptions-item>
      </a-descriptions>

      <template v-if="scope.nodeType !== 'deployment'">
        <div class="child-section-title"><span>{{ childSectionTitle }}</span><span>{{ rows.length }} 项</span></div>
        <a-table row-key="key" :columns="columns" :data-source="rows" :pagination="false" :scroll="{ x: tableWidth }" :custom-row="getChildRowProps" size="middle">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'child_link'">
              <a class="child-navigation-link" href="#" @click.prevent.stop="navigateToChild(record)">
                <span>{{ childLabel(record) }}</span>
                <RightOutlined />
              </a>
            </template>
            <template v-else-if="column.key === 'topology_type'"><a-tag :color="record.topology_type === 'cluster' ? 'blue' : 'default'">{{ record.topology_type === 'cluster' ? '集群' : '单机' }}</a-tag></template>
            <template v-else-if="column.key === 'enabled'"><a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" /></template>
            <template v-else-if="column.key === 'runtime_status'"><a-badge :status="runtimeStatus[record.runtime_status]?.status || 'default'" :text="runtimeStatus[record.runtime_status]?.label || '未知'" /></template>
          </template>
        </a-table>
      </template>

      <div v-else-if="detail" class="deployment-sections">
        <section>
          <div class="child-section-title"><span>监听端口</span><span>{{ detail.ports?.length || 0 }} 项</span></div>
          <a-space v-if="detail.ports?.length" wrap>
            <a-tag v-for="port in detail.ports" :key="`${port.protocol}:${port.port}`">{{ port.name || '端口' }} · {{ String(port.protocol || '').toUpperCase() }} {{ port.port }}</a-tag>
          </a-space>
          <a-empty v-else :image="simpleImage" description="未配置端口" />
        </section>
        <section>
          <div class="child-section-title"><span>最近基线检查</span><span>{{ baselineHistory.length }} 条</span></div>
          <a-table row-key="id" :columns="historyColumns" :data-source="baselineHistory" :pagination="false" :scroll="{ x: 790 }" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'status'">{{ historyStatusLabels[record.status] || record.status }}</template>
              <template v-else-if="column.key === 'passed'">
                <a-tag v-if="record.passed === true" color="success">通过</a-tag>
                <a-tag v-else-if="record.passed === false" color="error">未通过</a-tag>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.key === 'start_time'">{{ formatDateTime(record.start_time) }}</template>
            </template>
          </a-table>
        </section>
      </div>
      <a-empty v-if="!loading && scope.nodeType === 'deployment' && !detail" :image="simpleImage" description="实例不存在或已删除" />
    </a-spin>
  </section>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Empty, message } from 'ant-design-vue'
import { RightOutlined } from '@ant-design/icons-vue'
import store from '@/store'
import { formatTimeWithTimezone } from '@/util/timezone'
import {
  getApplicationService,
  getApplicationDeployment,
  getApplicationDeploymentBaselineHistory,
  getApplicationDeploymentList,
  getApplicationServiceList,
  getBusinessSystem,
  getBusinessSystemList,
} from '@/api/assets/application'

const props = defineProps({
  scope: { type: Object, required: true },
})
const emit = defineEmits(['navigate'])
const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE
const environmentLabels = { production: '生产环境', testing: '测试环境', development: '开发环境', other: '其他环境' }
const availabilityLabels = { none: '无高可用', active_standby: '主备', active_active: '双活' }
const runtimeStatus = {
  unknown: { label: '未知', status: 'default' }, running: { label: '运行中', status: 'success' },
  stopped: { label: '已停止', status: 'error' }, error: { label: '检查失败', status: 'warning' },
}
const healthStatus = { unknown: '未检查', checking: '检查中', healthy: '正常', unhealthy: '异常', error: '检查失败' }
const historyStatusLabels = { queued: '等待中', running: '检查中', completed: '已完成', failed: '执行失败' }
const loading = ref(false)
const rows = ref([])
const entity = ref(null)
const detail = ref(null)
const descendants = ref([])
const services = ref([])
const baselineHistory = ref([])
let loadSequence = 0

const systemColumns = [
  { title: '业务系统', dataIndex: 'name', key: 'child_link', width: 220 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 180 },
  { title: '负责人', dataIndex: 'owner', key: 'owner', width: 160 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 110 },
  { title: '状态', key: 'enabled', width: 100 },
]
const environmentColumns = [
  { title: '环境', dataIndex: 'environment', key: 'child_link', width: 180 },
  { title: '逻辑服务', dataIndex: 'service_count', key: 'service_count', width: 120 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 120 },
]
const serviceColumns = [
  { title: '逻辑服务', dataIndex: 'name', key: 'child_link', width: 220 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 180 },
  { title: '部署形态', key: 'topology_type', width: 110 },
  { title: '集群模型', dataIndex: 'cluster_profile_name', key: 'cluster_profile_name', width: 180 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 110 },
  { title: '状态', key: 'enabled', width: 100 },
]
const deploymentColumns = [
  { title: '部署实例', dataIndex: 'instance_name', key: 'child_link', width: 220 },
  { title: '成员端口', dataIndex: 'member_port', key: 'member_port', width: 120 },
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 180 },
  { title: '地址', dataIndex: 'host_ip', key: 'host_ip', width: 150 },
  { title: '版本', dataIndex: 'version', key: 'version', width: 120 },
  { title: '运行状态', key: 'runtime_status', width: 120 },
]
const historyColumns = [
  { title: '状态', key: 'status', width: 110 },
  { title: '结论', key: 'passed', width: 90 },
  { title: '通过项', dataIndex: 'passed_count', key: 'passed_count', width: 90 },
  { title: '检查项', dataIndex: 'total_count', key: 'total_count', width: 90 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 120 },
  { title: '开始时间', key: 'start_time', width: 170 },
]
const columns = computed(() => ({
  all: systemColumns,
  businessSystem: environmentColumns,
  environment: serviceColumns,
  service: deploymentColumns,
}[props.scope.nodeType] || []))
const childSectionTitle = computed(() => ({
  all: '业务系统', businessSystem: '环境', environment: '逻辑服务', service: '部署实例',
}[props.scope.nodeType] || '请选择节点'))
const levelLabel = computed(() => ({
  all: '服务树根节点', businessSystem: '业务系统', environment: props.scope.businessSystemName || '环境',
  service: '逻辑服务', deployment: '部署实例',
}[props.scope.nodeType] || '服务树'))
const tableWidth = computed(() => ({ all: 800, businessSystem: 520, environment: 1000, service: 950 }[props.scope.nodeType] || 800))
const breadcrumbs = computed(() => [
  '全部业务',
  props.scope.businessSystemName || (props.scope.nodeType === 'businessSystem' ? props.scope.nodeTitle : null),
  props.scope.environmentName,
  props.scope.serviceName || (props.scope.nodeType === 'service' ? props.scope.nodeTitle : null),
  props.scope.nodeType === 'deployment' ? props.scope.nodeTitle : null,
].filter(Boolean).filter((item, index, values) => values.indexOf(item) === index))
const abnormalCount = computed(() => descendants.value.filter((item) => (
  ['stopped', 'error'].includes(item.runtime_status) || ['unhealthy', 'error'].includes(item.health_status)
)).length)
const runningCount = computed(() => descendants.value.filter((item) => item.runtime_status === 'running').length)
const metrics = computed(() => {
  if (props.scope.nodeType === 'all') return [
    { label: '业务系统', value: rows.value.length },
    { label: '逻辑服务', value: services.value.length },
    { label: '部署实例', value: descendants.value.length },
    { label: '异常实例', value: abnormalCount.value, danger: abnormalCount.value > 0 },
  ]
  if (props.scope.nodeType === 'businessSystem') return [
    { label: '环境', value: rows.value.length },
    { label: '逻辑服务', value: services.value.length },
    { label: '部署实例', value: descendants.value.length },
    { label: '异常实例', value: abnormalCount.value, danger: abnormalCount.value > 0 },
  ]
  if (props.scope.nodeType === 'environment') return [
    { label: '逻辑服务', value: rows.value.length },
    { label: '部署实例', value: descendants.value.length },
    { label: '运行中', value: runningCount.value },
    { label: '异常实例', value: abnormalCount.value, danger: abnormalCount.value > 0 },
  ]
  if (props.scope.nodeType === 'service') return [
    { label: '部署实例', value: rows.value.length },
    { label: '运行中', value: runningCount.value },
    { label: '停止', value: descendants.value.filter((item) => item.runtime_status === 'stopped').length },
    { label: '异常实例', value: abnormalCount.value, danger: abnormalCount.value > 0 },
  ]
  if (props.scope.nodeType === 'deployment' && detail.value) return [
    { label: '运行状态', value: runtimeStatus[detail.value.runtime_status]?.label || '未知' },
    { label: '健康状态', value: healthStatus[detail.value.health_status] || '未检查' },
    { label: '监听端口', value: detail.value.ports?.length || 0 },
    { label: '基线通过率', value: detail.value.baseline_pass_rate == null ? '-' : `${detail.value.baseline_pass_rate}%` },
  ]
  return []
})
const summaryStatus = computed(() => {
  if (props.scope.nodeType === 'deployment' && detail.value) {
    if (['error', 'unhealthy'].includes(detail.value.health_status) || ['error', 'stopped'].includes(detail.value.runtime_status)) {
      return { label: '需要关注', color: 'error' }
    }
    if (detail.value.runtime_status === 'running') return { label: '运行正常', color: 'success' }
    return { label: '状态未知', color: 'default' }
  }
  if (abnormalCount.value > 0) return { label: `${abnormalCount.value} 个异常实例`, color: 'error' }
  if (descendants.value.length > 0) return { label: '未发现异常', color: 'success' }
  return { label: '暂无实例', color: 'default' }
})

function formatAccessEndpoint(service) {
  if (!service.access_address) return service.access_type === 'direct' ? '实例节点地址' : '-'
  return service.access_port ? `${service.access_address}:${service.access_port}` : service.access_address
}

function formatDateTime(value) {
  return value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
}

function childLabel(record) {
  if (props.scope.nodeType === 'businessSystem') {
    return environmentLabels[record.environment] || record.environment
  }
  if (props.scope.nodeType === 'service') return record.instance_name
  return record.name
}

function buildChildScope(record) {
  if (props.scope.nodeType === 'all') {
    return {
      nodeType: 'businessSystem', businessSystemId: record.id,
      businessSystemName: record.name, nodeTitle: record.name,
    }
  }
  if (props.scope.nodeType === 'businessSystem') {
    return {
      nodeType: 'environment', businessSystemId: props.scope.businessSystemId,
      businessSystemName: props.scope.businessSystemName || props.scope.nodeTitle,
      environment: record.environment,
      environmentName: environmentLabels[record.environment] || record.environment,
      nodeTitle: environmentLabels[record.environment] || record.environment,
    }
  }
  if (props.scope.nodeType === 'environment') {
    return {
      nodeType: 'service', applicationServiceId: record.id, nodeTitle: record.name,
      businessSystemId: props.scope.businessSystemId,
      businessSystemName: props.scope.businessSystemName,
      environment: props.scope.environment,
      environmentName: props.scope.environmentName || props.scope.nodeTitle,
    }
  }
  if (props.scope.nodeType === 'service') {
    return {
      nodeType: 'deployment', deploymentId: record.id, nodeTitle: record.instance_name,
      businessSystemId: props.scope.businessSystemId,
      businessSystemName: props.scope.businessSystemName,
      environment: props.scope.environment,
      environmentName: props.scope.environmentName,
      applicationServiceId: props.scope.applicationServiceId,
      serviceName: props.scope.serviceName || props.scope.nodeTitle,
    }
  }
  return null
}

function navigateToChild(record) {
  const childScope = buildChildScope(record)
  if (childScope) emit('navigate', childScope)
}

function getChildRowProps(record) {
  return {
    class: 'navigable-row',
    tabindex: 0,
    onClick: () => navigateToChild(record),
    onKeydown: (event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault()
        navigateToChild(record)
      }
    },
  }
}

async function fetchAll(loader, params = {}) {
  const firstResponse = await loader({ ...params, page: 1, page_size: 100 })
  const firstData = firstResponse?.data?.data || {}
  const records = [...(firstData.results || [])]
  const totalPages = Number(firstData.totalPages || 1)
  if (totalPages > 1) {
    const responses = await Promise.all(Array.from(
      { length: totalPages - 1 },
      (_, index) => loader({ ...params, page: index + 2, page_size: 100 }),
    ))
    for (const response of responses) records.push(...(response?.data?.data?.results || []))
  }
  return records
}

function summarizeEnvironments(services) {
  const summaries = new Map()
  for (const service of services) {
    const current = summaries.get(service.environment) || {
      key: service.environment, environment: service.environment, service_count: 0, deployment_count: 0,
    }
    current.service_count += 1
    current.deployment_count += Number(service.deployment_count || 0)
    summaries.set(service.environment, current)
  }
  return [...summaries.values()].sort((left, right) => String(left.environment).localeCompare(String(right.environment)))
}

async function loadNode() {
  const sequence = ++loadSequence
  loading.value = true
  rows.value = []
  entity.value = null
  detail.value = null
  descendants.value = []
  services.value = []
  baselineHistory.value = []
  try {
    let nextRows = []
    let nextEntity = null
    let nextDetail = null
    let nextDescendants = []
    let nextServices = []
    let nextHistory = []
    if (props.scope.nodeType === 'all') {
      const [systemsResult, servicesResult, deploymentsResult] = await Promise.all([
        fetchAll(getBusinessSystemList),
        fetchAll(getApplicationServiceList),
        fetchAll(getApplicationDeploymentList),
      ])
      nextRows = systemsResult.map((item) => ({ ...item, key: item.id }))
      nextServices = servicesResult
      nextDescendants = deploymentsResult
    } else if (props.scope.nodeType === 'businessSystem') {
      const [entityResponse, servicesResult, deploymentsResult] = await Promise.all([
        getBusinessSystem(props.scope.businessSystemId),
        fetchAll(getApplicationServiceList, { business_system: props.scope.businessSystemId }),
        fetchAll(getApplicationDeploymentList, { application_service__business_system: props.scope.businessSystemId }),
      ])
      nextEntity = entityResponse?.data?.data || null
      nextServices = servicesResult
      nextDescendants = deploymentsResult
      nextRows = summarizeEnvironments(servicesResult)
    } else if (props.scope.nodeType === 'environment') {
      const [servicesResult, deploymentsResult] = await Promise.all([
        fetchAll(getApplicationServiceList, {
          business_system: props.scope.businessSystemId,
          environment: props.scope.environment,
        }),
        fetchAll(getApplicationDeploymentList, {
          application_service__business_system: props.scope.businessSystemId,
          application_service__environment: props.scope.environment,
        }),
      ])
      nextServices = servicesResult
      nextDescendants = deploymentsResult
      nextRows = servicesResult.map((item) => ({ ...item, key: item.id }))
    } else if (props.scope.nodeType === 'service') {
      const [entityResponse, deploymentsResult] = await Promise.all([
        getApplicationService(props.scope.applicationServiceId),
        fetchAll(getApplicationDeploymentList, { application_service: props.scope.applicationServiceId }),
      ])
      nextEntity = entityResponse?.data?.data || null
      nextDescendants = deploymentsResult
      nextRows = deploymentsResult.map((item) => ({ ...item, key: item.id }))
    } else if (props.scope.nodeType === 'deployment') {
      const [detailResponse, historyResponse] = await Promise.all([
        getApplicationDeployment(props.scope.deploymentId),
        getApplicationDeploymentBaselineHistory(props.scope.deploymentId),
      ])
      nextDetail = detailResponse?.data?.data || null
      nextHistory = (historyResponse?.data?.data || []).slice(0, 5)
    }
    if (sequence !== loadSequence) return
    rows.value = nextRows
    entity.value = nextEntity
    detail.value = nextDetail
    descendants.value = nextDescendants
    services.value = nextServices
    baselineHistory.value = nextHistory
  } catch (error) {
    if (sequence === loadSequence) message.error(error?.message || '节点信息加载失败')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

watch(() => props.scope, loadNode, { deep: true, immediate: true })
</script>

<style scoped>
.node-content { min-width: 0; }
.node-breadcrumb { margin-bottom: 12px; }
.node-content-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid #e5e7eb;
}
.node-content-header h2 { margin: 3px 0 0; color: #172033; font-size: 20px; }
.node-content-kicker { color: #7b8494; font-size: 12px; }
.metric-band {
  display: grid;
  grid-template-columns: repeat(4, minmax(110px, 1fr));
  margin-bottom: 18px;
  border-block: 1px solid #e5e7eb;
  background: #fafbfc;
}
.metric-item {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
  padding: 14px 18px;
  border-right: 1px solid #e5e7eb;
}
.metric-item:last-child { border-right: 0; }
.metric-label { color: #687386; font-size: 12px; }
.metric-item strong { color: #172033; font-size: 22px; font-weight: 600; }
.metric-item strong.metric-danger { color: #cf1322; }
.node-summary { margin-bottom: 20px; }
.child-section-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 18px 0 10px;
  color: #172033;
  font-size: 14px;
  font-weight: 600;
}
.child-section-title span:last-child { color: #8c95a5; font-size: 12px; font-weight: 400; }
.child-navigation-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #1677ff;
  font-weight: 500;
}
.child-navigation-link:hover { color: #0958d9; text-decoration: underline; }
.child-navigation-link :deep(.anticon) { font-size: 11px; }
.node-content :deep(.navigable-row) { cursor: pointer; }
.node-content :deep(.navigable-row:hover > td),
.node-content :deep(.navigable-row:focus > td) { background: #f0f5ff; }
.node-content :deep(.navigable-row:focus) { outline: 2px solid #91caff; outline-offset: -2px; }
.deployment-sections { display: grid; gap: 8px; }
@media (max-width: 760px) {
  .metric-band { grid-template-columns: repeat(2, minmax(100px, 1fr)); }
  .metric-item:nth-child(2) { border-right: 0; }
  .metric-item:nth-child(-n + 2) { border-bottom: 1px solid #e5e7eb; }
}
</style>
