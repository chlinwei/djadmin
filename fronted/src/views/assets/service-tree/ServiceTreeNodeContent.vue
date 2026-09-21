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
      <a-space>
        <template v-if="scope.nodeType === 'businessSystem' && entity">
          <a-tooltip title="编辑">
            <a-button v-permission="'assets:service-tree:manage'" size="small" type="primary" @click="emit('edit-business-system', entity)">
              <FontAwesomeIcon :icon="['fa', 'edit']" />
            </a-button>
          </a-tooltip>
          <a-tooltip title="删除">
            <a-button v-permission="'assets:service-tree:manage'" class="delBtn" size="small" type="primary" danger @click="emit('delete-business-system', entity)">
              <FontAwesomeIcon :icon="['fas', 'trash-can']" />
            </a-button>
          </a-tooltip>
        </template>
        <template v-else-if="scope.nodeType === 'service' && entity">
          <a-tooltip title="编辑">
            <a-button v-permission="'assets:service-tree:manage'" size="small" type="primary" @click="emit('edit-service', entity)">
              <FontAwesomeIcon :icon="['fa', 'edit']" />
            </a-button>
          </a-tooltip>
          <a-tooltip title="删除">
            <a-button v-permission="'assets:service-tree:manage'" class="delBtn" size="small" type="primary" danger @click="emit('delete-service', entity)">
              <FontAwesomeIcon :icon="['fas', 'trash-can']" />
            </a-button>
          </a-tooltip>
        </template>
        <a-tag :color="summaryStatus.color">{{ summaryStatus.label }}</a-tag>
      </a-space>
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

      <a-descriptions v-else-if="scope.nodeType === 'service' && entity" bordered :column="{ xs: 1, sm: 2 }" size="small" class="node-summary">
        <a-descriptions-item label="业务系统">{{ entity.business_system_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="环境">{{ entity.environment_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="部署形态">{{ entity.topology_type === 'cluster' ? '集群' : entity.topology_type === 'load_balancer' ? '负载均衡' : '单机' }}</a-descriptions-item>
        <a-descriptions-item v-if="entity.topology_type === 'cluster'" label="集群模型">{{ entity.cluster_profile_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="应用">{{ entity.application_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="应用版本">{{ entity.application_version_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="部署模板">{{ entity.deployment_template_name || '-' }}</a-descriptions-item>
        <a-descriptions-item v-if="entity.topology_type !== 'standalone'" :label="entity.cluster_type === 'ha' ? 'HA VIP' : entity.topology_type === 'load_balancer' ? '负载均衡地址' : '入口地址'">{{ entity.access_address || '-' }}</a-descriptions-item>
        <a-descriptions-item label="备注">{{ entity.remark || '-' }}</a-descriptions-item>
      </a-descriptions>

      <a-descriptions v-else-if="scope.nodeType === 'deployment' && detail" bordered :column="{ xs: 1, sm: 2 }" size="small" class="node-summary">
        <a-descriptions-item label="实例名称">{{ detail.instance_name }}</a-descriptions-item>
        <a-descriptions-item label="主机">{{ detail.host_name || '-' }}（{{ detail.host_ip || '-' }}）</a-descriptions-item>
        <a-descriptions-item label="运行状态">
          <a-tooltip v-if="detail.runtime_status === 'error' && detail.runtime_status_output" :title="detail.runtime_status_output" placement="top">
            <a-badge :status="runtimeStatus[detail.runtime_status]?.status || 'default'" :text="runtimeStatus[detail.runtime_status]?.label || '未知'" />
          </a-tooltip>
          <a-badge v-else :status="runtimeStatus[detail.runtime_status]?.status || 'default'" :text="runtimeStatus[detail.runtime_status]?.label || '未知'" />
        </a-descriptions-item>
        <a-descriptions-item v-if="detail.cluster_type === 'ha'" label="主备状态">{{ haRoleLabels[detail.ha_role] || '未知' }}</a-descriptions-item>
        <a-descriptions-item label="备注">{{ detail.remark || '-' }}</a-descriptions-item>
      </a-descriptions>

      <!-- 主机基础信息：来自资产主机详情接口（agent 采集的硬件/系统/磁盘快照）。
           失败（最常见是服务树用户没有 assets:hosts:view 权限）或从未采集时降级成一行提示，
           不阻塞实例信息本身。WebSSH 直接带 host_id 跳资产页的 webssh 终端（新开标签页）。 -->
      <section v-if="scope.nodeType === 'deployment' && detail && detail.host" class="host-info-section">
        <div class="child-section-title">
          <span>主机信息</span>
          <a-tooltip title="打开 WebSSH 终端（新标签页）">
            <a-button v-permission="'assets:hosts:view'" size="small" @click="openWebSSH">
              <FontAwesomeIcon :icon="['fas', 'terminal']" />
            </a-button>
          </a-tooltip>
        </div>
        <a-descriptions v-if="hostInfo" bordered :column="{ xs: 1, sm: 2 }" size="small">
          <a-descriptions-item label="主机名">{{ formatHostValue(hostInfo.hostname) }}</a-descriptions-item>
          <a-descriptions-item label="操作系统">{{ hostOsText }}</a-descriptions-item>
          <a-descriptions-item label="CPU">{{ hostCpuText }}</a-descriptions-item>
          <a-descriptions-item label="内存">{{ hostMemoryText }}</a-descriptions-item>
          <a-descriptions-item label="磁盘总量">{{ hostDiskTotalText }}</a-descriptions-item>
          <a-descriptions-item label="磁盘使用率">
            <span v-if="hostInfo.disk_used_percent != null" :class="{ 'host-disk-danger': Number(hostInfo.disk_used_percent) > 85 }">{{ hostInfo.disk_used_percent }}%</span>
            <span v-else>-</span>
          </a-descriptions-item>
        </a-descriptions>
        <template v-if="hostInfo && visibleDisks.length">
          <div class="host-disk-title">磁盘明细（{{ hostInfo.disks.length }} 块）</div>
          <a-table size="small" row-key="device" :columns="hostDiskColumns" :data-source="visibleDisks" :pagination="false" :locale="{ emptyText: '无磁盘数据' }">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'usage'">
                <span v-if="record.usage_percent != null" :class="{ 'host-disk-danger': Number(record.usage_percent) > 85 }">{{ record.usage_percent }}%</span>
                <span v-else>-</span>
              </template>
            </template>
          </a-table>
          <a-button v-if="hostInfo.disks.length > 8" type="link" size="small" class="host-disk-toggle" @click="showAllDisks = !showAllDisks">
            {{ showAllDisks ? '收起' : `展开全部 ${hostInfo.disks.length} 块` }}
          </a-button>
        </template>
        <div v-else-if="!hostInfoLoading" class="host-info-missing">主机信息不可见或尚未采集（需要资产查看权限且 agent 已采集过数据）</div>
      </section>

      <section v-if="scope.nodeType === 'service' && entity" class="service-ports-section">
        <div class="child-section-title"><span>监听端口</span><span>{{ entity.ports?.length || 0 }} 项</span></div>
        <a-space v-if="entity.ports?.length" wrap>
          <a-tag v-for="port in entity.ports" :key="`${port.protocol}:${port.port}`">{{ port.name || '端口' }} · {{ String(port.protocol || '').toUpperCase() }} {{ port.port }}</a-tag>
        </a-space>
        <div v-else class="section-empty">未配置端口</div>
      </section>

      <!-- 日志文件：模板日志定义（名 + 路径），路径用后端 resolved_path（服务层宏已尽力展开），
           仍含实例级宏时标注出来，不猜值。失败静默展示空态，不阻塞节点信息。 -->
      <section v-if="scope.nodeType === 'service' && entity" class="service-ports-section">
        <div class="child-section-title"><span>日志文件</span><span>{{ serviceLogs.length }} 项</span></div>
        <a-space v-if="serviceLogs.length" direction="vertical" :size="6" class="service-log-list">
          <div v-for="log in serviceLogs" :key="log.log_definition" class="service-log-item">
            <a-tag color="blue">{{ log.name }}</a-tag>
            <span class="service-log-path">{{ log.resolved_path || log.path_pattern }}</span>
            <a-tooltip v-if="log.pending_macros?.length" :title="`实例级宏待展开：${log.pending_macros.join('、')}`">
              <a-tag color="orange">{{ log.pending_macros.join('、') }} 待展开</a-tag>
            </a-tooltip>
          </div>
        </a-space>
        <div v-else class="section-empty">模板未配置日志</div>
      </section>

      <template v-if="scope.nodeType !== 'deployment'">
        <div class="child-section-title"><span>{{ childSectionTitle }}</span><span>{{ rows.length }} 项</span></div>
        <a-table row-key="key" :columns="columns" :data-source="rows" :pagination="false" :scroll="{ x: tableWidth }" :custom-row="getChildRowProps" size="small" :locale="tableLocale">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'child_link'">
              <a class="child-navigation-link" href="#" @click.prevent.stop="navigateToChild(record)">
                <span>{{ childLabel(record) }}</span>
                <RightOutlined />
              </a>
            </template>
            <template v-else-if="column.key === 'topology_type'"><a-tag :color="record.topology_type === 'cluster' ? 'blue' : record.topology_type === 'load_balancer' ? 'green' : 'default'">{{ record.topology_type === 'cluster' ? '集群' : record.topology_type === 'load_balancer' ? '负载均衡' : '单机' }}</a-tag></template>
            <template v-else-if="column.key === 'enabled'"><a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" /></template>
            <template v-else-if="column.key === 'runtime_status'">
              <a-tooltip v-if="record.runtime_status === 'error' && record.runtime_status_output" :title="record.runtime_status_output" placement="top">
                <a-badge :status="runtimeStatus[record.runtime_status]?.status || 'default'" :text="runtimeStatus[record.runtime_status]?.label || '未知'" />
              </a-tooltip>
              <a-badge v-else :status="runtimeStatus[record.runtime_status]?.status || 'default'" :text="runtimeStatus[record.runtime_status]?.label || '未知'" />
            </template>
            <template v-else-if="column.key === 'ha_role'"><a-tag :color="haRoleColors[record.ha_role] || 'default'">{{ haRoleLabels[record.ha_role] || '未知' }}</a-tag></template>
            <template v-else-if="column.key === 'business_system_action'">
              <a-space :size="6">
                <a-tooltip title="编辑">
                  <a-button v-permission="'assets:service-tree:manage'" size="small" type="primary" @click.stop="emit('edit-business-system', record)">
                    <FontAwesomeIcon :icon="['fa', 'edit']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除">
                  <a-button v-permission="'assets:service-tree:manage'" class="delBtn" size="small" type="primary" danger @click.stop="emit('delete-business-system', record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
            <template v-else-if="column.key === 'service_action'">
              <a-space :size="6">
                <a-tooltip title="编辑">
                  <a-button v-permission="'assets:service-tree:manage'" size="small" type="primary" @click.stop="emit('edit-service', record)">
                    <FontAwesomeIcon :icon="['fa', 'edit']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除">
                  <a-button v-permission="'assets:service-tree:manage'" class="delBtn" size="small" type="primary" danger @click.stop="emit('delete-service', record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
      </template>

      <a-empty v-if="!loading && scope.nodeType === 'deployment' && !detail" :image="simpleImage" description="实例不存在或已删除" />
    </a-spin>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Empty, message } from 'ant-design-vue'
import { RightOutlined } from '@ant-design/icons-vue'
import { tableLocale } from '@/util/tableStyle'
import store from '@/store'
import { formatTimeWithTimezone } from '@/util/timezone'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import {
  getApplicationService,
  getApplicationServiceLogConfig,
  getApplicationDeployment,
  getApplicationDeploymentList,
  getApplicationServiceList,
  getBusinessEnvironmentList,
  getBusinessSystem,
  getBusinessSystemList,
  getProjectList,
  refreshApplicationServiceRuntimeStatus,
} from '@/api/assets/application'
import { getHostById } from '@/api/assets/host'

const props = defineProps({
  scope: { type: Object, required: true },
})
const emit = defineEmits(['navigate', 'edit-business-system', 'delete-business-system', 'edit-service', 'delete-service'])
const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE
const runtimeStatus = {
  unknown: { label: '未知', status: 'default' }, running: { label: '运行中', status: 'success' },
  stopped: { label: '已停止', status: 'error' }, error: { label: '检查失败', status: 'warning' },
}
const haRoleLabels = { unknown: '未知', primary: '主', standby: '备' }
const haRoleColors = { primary: 'green', standby: 'blue', unknown: 'default' }
const loading = ref(false)
const rows = ref([])
const router = useRouter()
const entity = ref(null)
const serviceLogs = ref([])
// 部署实例节点的主机基础信息（资产主机详情接口）：null = 不可见/未采集（界面降级成一行提示）。
const hostInfo = ref(null)
const hostInfoLoading = ref(false)
const showAllDisks = ref(false)
const hostDiskColumns = [
  { title: '设备', dataIndex: 'device', key: 'device', width: 120 },
  { title: '挂载点', dataIndex: 'mount_point', key: 'mount_point', width: 140 },
  { title: '文件系统', dataIndex: 'filesystem', key: 'filesystem', width: 110 },
  { title: '容量 (GB)', dataIndex: 'size_gb', key: 'size_gb', width: 100, align: 'right' },
  { title: '已用 (GB)', dataIndex: 'used_gb', key: 'used_gb', width: 100, align: 'right' },
  { title: '使用率', key: 'usage', width: 90, align: 'right' },
]
const detail = ref(null)
const descendants = ref([])
const services = ref([])
let loadSequence = 0
let runtimeRefreshTimer = null
let runtimeRefreshInFlight = false
const runtimeRefreshInterval = 15000

const systemColumns = [
  { title: '业务系统', dataIndex: 'name', key: 'child_link', width: 220 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 180 },
  { title: '负责人', dataIndex: 'owner', key: 'owner', width: 160 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 110 },
  { title: '状态', key: 'enabled', width: 100 },
  { title: '操作', key: 'business_system_action', width: 110, fixed: 'right' },
]
const serviceColumns = [
  { title: '逻辑服务', dataIndex: 'name', key: 'child_link', width: 220 },
  { title: '环境', dataIndex: 'environment_name', key: 'environment_name', width: 140 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 180 },
  { title: '部署形态', key: 'topology_type', width: 110 },
  { title: '集群模型', dataIndex: 'cluster_profile_name', key: 'cluster_profile_name', width: 180 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 110 },
  { title: '状态', key: 'enabled', width: 100 },
  { title: '操作', key: 'service_action', width: 110, fixed: 'right' },
]
// 业务系统下先按环境分一层，跟左侧树的层级（项目/业务系统/环境/服务）保持一致，选中环境后才展开具体逻辑服务。
const environmentSummaryColumns = [
  { title: '环境', dataIndex: 'name', key: 'child_link', width: 220 },
  { title: '逻辑服务', dataIndex: 'service_count', key: 'service_count', width: 120 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 120 },
]
const deploymentColumns = [
  { title: '部署实例', dataIndex: 'instance_name', key: 'child_link', width: 220 },
  { title: '主备状态', key: 'ha_role', width: 110 },
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 180 },
  { title: '地址', dataIndex: 'host_ip', key: 'host_ip', width: 150 },
  { title: '版本', dataIndex: 'version', key: 'version', width: 120 },
  { title: '运行状态', key: 'runtime_status', width: 120 },
]
const columns = computed(() => {
  const baseColumns = ({
    all: systemColumns,
    project: systemColumns,
    businessSystem: environmentSummaryColumns,
    environment: serviceColumns,
    service: deploymentColumns,
  }[props.scope.nodeType] || [])
  if (props.scope.nodeType === 'service' && entity.value?.cluster_type !== 'ha') {
    return baseColumns.filter((column) => column.key !== 'ha_role')
  }
  return baseColumns
})
const childSectionTitle = computed(() => ({
  all: '业务系统', project: '业务系统', businessSystem: '环境', environment: '逻辑服务', service: '部署实例',
}[props.scope.nodeType] || '请选择节点'))
const levelLabel = computed(() => ({
  all: '服务树根节点', project: '项目', businessSystem: '业务系统', environment: '环境',
  service: '逻辑服务', deployment: '部署实例',
}[props.scope.nodeType] || '服务树'))
const tableWidth = computed(() => ({ all: 920, project: 920, businessSystem: 460, environment: 1210, service: 950 }[props.scope.nodeType] || 800))
const breadcrumbs = computed(() => [
  '全部业务',
  props.scope.projectName || (props.scope.nodeType === 'project' ? props.scope.nodeTitle : null),
  props.scope.businessSystemName || (props.scope.nodeType === 'businessSystem' ? props.scope.nodeTitle : null),
  props.scope.environmentName || (props.scope.nodeType === 'environment' ? props.scope.nodeTitle : null),
  props.scope.serviceName || (props.scope.nodeType === 'service' ? props.scope.nodeTitle : null),
  props.scope.nodeType === 'deployment' ? props.scope.nodeTitle : null,
].filter(Boolean).filter((item, index, values) => values.indexOf(item) === index))
const abnormalCount = computed(() => descendants.value.filter((item) => (
  ['stopped', 'error'].includes(item.runtime_status)
)).length)
const runningCount = computed(() => descendants.value.filter((item) => item.runtime_status === 'running').length)
const metrics = computed(() => {
  if (props.scope.nodeType === 'all' || props.scope.nodeType === 'project') return [
    { label: '业务系统', value: rows.value.length },
    { label: '逻辑服务', value: services.value.length },
    { label: '部署实例', value: descendants.value.length },
    { label: '异常实例', value: abnormalCount.value, danger: abnormalCount.value > 0 },
  ]
  if (props.scope.nodeType === 'businessSystem' || props.scope.nodeType === 'environment') return [
    { label: '逻辑服务', value: services.value.length },
    { label: '部署实例', value: descendants.value.length },
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
    { label: '主备状态', value: haRoleLabels[detail.value.ha_role] || '未知' },
    { label: '监听端口', value: detail.value.ports?.length || 0 },
  ]
  return []
})
const summaryStatus = computed(() => {
  if (props.scope.nodeType === 'deployment' && detail.value) {
    if (detail.value.runtime_status === 'error' || detail.value.runtime_status === 'stopped') {
      return { label: '需要关注', color: 'error' }
    }
    if (detail.value.runtime_status === 'running') return { label: '运行正常', color: 'success' }
    return { label: '状态未知', color: 'default' }
  }
  if (abnormalCount.value > 0) return { label: `${abnormalCount.value} 个异常实例`, color: 'error' }
  if (descendants.value.length > 0) return { label: '未发现异常', color: 'success' }
  return { label: '暂无实例', color: 'default' }
})

function formatDateTime(value) {
  return value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
}

function childLabel(record) {
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
  if (props.scope.nodeType === 'project') {
    return {
      nodeType: 'businessSystem', businessSystemId: record.id,
      businessSystemName: record.name, nodeTitle: record.name,
      projectId: props.scope.projectId, projectName: props.scope.nodeTitle,
    }
  }
  if (props.scope.nodeType === 'businessSystem') {
    // 业务系统下先跳转到环境层级，跟左侧树的层级顺序保持一致，具体逻辑服务留到选中环境后再展开。
    return {
      nodeType: 'environment',
      businessSystemId: props.scope.businessSystemId,
      businessSystemName: props.scope.businessSystemName || props.scope.nodeTitle,
      environment: record.environment,
      environmentName: record.name,
      nodeTitle: record.name,
    }
  }
  if (props.scope.nodeType === 'environment') {
    return {
      nodeType: 'service', applicationServiceId: record.id, nodeTitle: record.name,
      businessSystemId: props.scope.businessSystemId,
      businessSystemName: props.scope.businessSystemName,
      environment: record.environment,
      environmentName: record.environment_name,
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

// 手动刷新到服务节点时顺带实时查询一次 Agent；切换节点的自动加载不触发，避免浏览即发起远程调用。
async function refreshRuntimeStatus(silent = false) {
  const serviceId = props.scope.nodeType === 'service' ? props.scope.applicationServiceId : null
  if (!serviceId || runtimeRefreshInFlight) return
  runtimeRefreshInFlight = true
  try {
    const response = await refreshApplicationServiceRuntimeStatus(serviceId)
    if (!silent) {
      const summary = response?.data?.data?.summary || {}
      message.success(`运行状态已刷新：运行中 ${summary.running || 0} / 已停止 ${summary.stopped || 0} / 检查失败 ${summary.error || 0}`)
    }
  } catch (error) {
    if (!silent) {
      message.error(error?.response?.data?.msg || error?.message || '查询运行状态失败')
    }
  } finally {
    runtimeRefreshInFlight = false
  }
}

async function refresh() {
  await refreshRuntimeStatus()
  await loadNode()
}

async function refreshAutomatically() {
  if (props.scope.nodeType !== 'service') return
  await refreshRuntimeStatus(true)
  await loadNode()
}

function startRuntimeRefresh() {
  if (runtimeRefreshTimer) clearInterval(runtimeRefreshTimer)
  runtimeRefreshTimer = setInterval(() => { void refreshAutomatically() }, runtimeRefreshInterval)
}

function stopRuntimeRefresh() {
  if (runtimeRefreshTimer) clearInterval(runtimeRefreshTimer)
  runtimeRefreshTimer = null
}

function formatHostValue(value) {
  if (value == null || value === '') return '-'
  return value
}

// 磁盘明细超过 8 块默认折叠（排障高频看前几块就够，全部展开会把详情页拉得很长）。
const visibleDisks = computed(() => {
  const disks = Array.isArray(hostInfo.value?.disks) ? hostInfo.value.disks : []
  return showAllDisks.value ? disks : disks.slice(0, 8)
})
const hostOsText = computed(() => {
  const os = hostInfo.value
  if (!os) return '-'
  return [formatHostValue(os.os_type), formatHostValue(os.os_version)].filter((part) => part !== '-').join(' ') || '-'
})
const hostCpuText = computed(() => {
  const info = hostInfo.value
  if (!info) return '-'
  const cores = info.cpu_cores != null ? `${info.cpu_cores} 核` : ''
  const model = formatHostValue(info.cpu_model)
  return [cores, model].filter(Boolean).join(' · ') || '-'
})
const hostMemoryText = computed(() => (hostInfo.value?.memory_gb != null ? `${hostInfo.value.memory_gb} GB` : '-'))
const hostDiskTotalText = computed(() => (hostInfo.value?.disk_total_gb != null ? `${hostInfo.value.disk_total_gb} GB` : '-'))

async function fetchHostInfo(hostId) {
  hostInfo.value = null
  if (!hostId) return
  hostInfoLoading.value = true
  try {
    // 接口失败（权限/未采集）保持 null，界面降级成提示行；不打断实例详情展示。
    const response = await getHostById(hostId)
    hostInfo.value = response?.data?.data || null
  } catch {
    hostInfo.value = null
  } finally {
    hostInfoLoading.value = false
  }
}

// WebSSH 复用资产主机页的入口：新标签页打开终端，query 语义与其保持一致
// （target_user 不传，由 webssh 页自己走默认用户/用户选择）。
function openWebSSH() {
  const currentDetail = detail.value || {}
  const routeData = router.resolve({
    path: '/assets/hosts/webssh',
    query: {
      host_id: String(currentDetail.host),
      instance_name: currentDetail.instance_name || '',
      ip: currentDetail.host_ip || '',
    },
  })
  window.open(routeData.href, '_blank', 'noopener,noreferrer,width=1280,height=820')
}

async function loadNode() {
  const sequence = ++loadSequence
  loading.value = true
  rows.value = []
  entity.value = null
  serviceLogs.value = []
  hostInfo.value = null
  showAllDisks.value = false
  detail.value = null
  descendants.value = []
  services.value = []
  try {
    let nextRows = []
    let nextEntity = null
    let nextDetail = null
    let nextDescendants = []
    let nextServices = []
    let nextServiceLogs = []
    if (props.scope.nodeType === 'all') {
      const [systemsResult, projectsResult, servicesResult, deploymentsResult] = await Promise.all([
        fetchAll(getBusinessSystemList),
        fetchAll(getProjectList),
        fetchAll(getApplicationServiceList),
        fetchAll(getApplicationDeploymentList),
      ])
      const projectIds = new Set((props.scope.projectIds || []).map((id) => String(id)))
      const projectSystemIds = projectIds.size
        ? new Set(projectsResult
          .filter((project) => projectIds.has(String(project.id)))
          .flatMap((project) => project.business_systems || [])
          .map((id) => String(id)))
        : null
      const environmentIds = new Set(props.scope.environmentIds || [])
      nextServices = servicesResult.filter((service) => (
        (!projectSystemIds || projectSystemIds.has(String(service.business_system)))
        && (!environmentIds.size || environmentIds.has(service.environment))
      ))
      const serviceIds = new Set(nextServices.map((service) => service.id))
      nextDescendants = deploymentsResult.filter((deployment) => (
        (deployment.application_service_ids || []).some((serviceId) => serviceIds.has(serviceId))
      ))
      const visibleSystemIds = new Set(nextServices.map((service) => service.business_system))
      nextRows = systemsResult
        .filter((system) => !projectSystemIds && !environmentIds.size ? true : visibleSystemIds.has(system.id))
        .map((item) => ({ ...item, key: item.id }))
    } else if (props.scope.nodeType === 'project') {
      // 服务树勾了 groupByProject 才会出现这一层：项目下没有直接过滤参数，只能先查该项目的业务系统，
      // 再用全量服务/实例数据按业务系统 ID 收敛，跟“全部业务”根节点算总量的方式保持一致。
      const [systemsResult, servicesResult, deploymentsResult] = await Promise.all([
        fetchAll(getBusinessSystemList, { project: props.scope.projectId }),
        fetchAll(getApplicationServiceList),
        fetchAll(getApplicationDeploymentList),
      ])
      const systemIds = new Set(systemsResult.map((system) => system.id))
      nextServices = servicesResult.filter((service) => systemIds.has(service.business_system))
      const serviceIds = new Set(nextServices.map((service) => service.id))
      nextDescendants = deploymentsResult.filter((deployment) => (
        (deployment.application_service_ids || []).some((serviceId) => serviceIds.has(serviceId))
      ))
      nextRows = systemsResult.map((item) => ({ ...item, key: item.id }))
    } else if (props.scope.nodeType === 'businessSystem') {
      const [entityResponse, servicesResult, deploymentsResult] = await Promise.all([
        getBusinessSystem(props.scope.businessSystemId),
        fetchAll(getApplicationServiceList, { business_system: props.scope.businessSystemId }),
        fetchAll(getApplicationDeploymentList, { application_service__business_system: props.scope.businessSystemId }),
      ])
      nextEntity = entityResponse?.data?.data || null
      nextServices = servicesResult
      nextDescendants = deploymentsResult
      // 业务系统下先按环境分组展示，跟左侧树的层级保持一致；具体服务留到选中某个环境后再展开。
      const environmentGroups = new Map()
      const environmentOrder = []
      servicesResult.forEach((service) => {
        const envKey = service.environment ?? 'unassigned'
        if (!environmentGroups.has(envKey)) {
          environmentGroups.set(envKey, {
            key: `environment:${envKey}`,
            environment: service.environment ?? null,
            name: service.environment_name || '未配置环境',
            serviceIds: new Set(),
          })
          environmentOrder.push(envKey)
        }
        environmentGroups.get(envKey).serviceIds.add(service.id)
      })
      nextRows = environmentOrder.map((envKey) => {
        const group = environmentGroups.get(envKey)
        const deploymentCount = deploymentsResult.filter((deployment) => (
          (deployment.application_service_ids || []).some((serviceId) => group.serviceIds.has(serviceId))
        )).length
        return {
          key: group.key,
          environment: group.environment,
          name: group.name,
          service_count: group.serviceIds.size,
          deployment_count: deploymentCount,
        }
      })
    } else if (props.scope.nodeType === 'environment') {
      // 未配置环境的分组 scope.environment 是 null，后端过滤参数不方便传 null，改成整业务系统拉回来再按值比对。
      const [servicesResult, deploymentsResult] = await Promise.all([
        fetchAll(getApplicationServiceList, { business_system: props.scope.businessSystemId }),
        fetchAll(getApplicationDeploymentList, { application_service__business_system: props.scope.businessSystemId }),
      ])
      const matchesEnvironment = (value) => (value ?? null) === (props.scope.environment ?? null)
      nextServices = servicesResult.filter((service) => matchesEnvironment(service.environment))
      nextDescendants = deploymentsResult.filter((deployment) => matchesEnvironment(deployment.environment))
      nextRows = nextServices.map((item) => ({ ...item, key: item.id }))
    } else if (props.scope.nodeType === 'service') {
      const [entityResponse, deploymentsResult, logConfigResponse] = await Promise.all([
        getApplicationService(props.scope.applicationServiceId),
        fetchAll(getApplicationDeploymentList, { application_service: props.scope.applicationServiceId }),
        getApplicationServiceLogConfig(props.scope.applicationServiceId).catch(() => null),
      ])
      nextEntity = entityResponse?.data?.data || null
      // 模板日志定义用于「日志文件」一节；接口失败时留空，不阻塞节点其他信息。
      nextServiceLogs = logConfigResponse?.data?.data?.logs || []
      const rolesByDeployment = new Map(
        (nextEntity?.member_instances || []).map((item) => [item.deployment, item.ha_role]),
      )
      nextDescendants = deploymentsResult.map((item) => ({
        ...item,
        ha_role: rolesByDeployment.get(item.id) || item.ha_role,
      }))
      nextRows = deploymentsResult.map((item) => ({ ...item, key: item.id }))
    } else if (props.scope.nodeType === 'deployment') {
      const detailResponse = await getApplicationDeployment(props.scope.deploymentId)
      nextDetail = detailResponse?.data?.data || null
      // 主机基础信息不阻塞实例详情：序列校验内并行拉取（失败/无权限 → null 降级提示）。
      void fetchHostInfo(nextDetail?.host)
    }
    if (sequence !== loadSequence) return
    rows.value = nextRows
    entity.value = nextEntity
    serviceLogs.value = nextServiceLogs
    detail.value = nextDetail
    descendants.value = nextDescendants
    services.value = nextServices
  } catch (error) {
    if (sequence === loadSequence) message.error(error?.message || '节点信息加载失败')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

watch(() => props.scope, (scope) => {
  void (scope.nodeType === 'service' ? refreshAutomatically() : loadNode())
}, { deep: true, immediate: true })

onBeforeUnmount(stopRuntimeRefresh)
// 这个组件嵌在被 keep-alive 缓存的页面里，切 tab 只会触发 onDeactivated，不会真正 unmount，
// 必须用共享的 keep-alive 生命周期处理失活时停轮询；onActivated 首次挂载也会触发一次，不用再单独 onMounted。
useKeepAliveRefreshLifecycle(startRuntimeRefresh, stopRuntimeRefresh)

defineExpose({ refresh })
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
.service-log-item { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.service-log-path {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-size: 12px;
  color: #172033;
  word-break: break-all;
}
.service-log-list { width: 100%; }
.host-info-section { margin-bottom: 4px; }
.host-info-section .ant-descriptions { margin-bottom: 12px; }
.host-disk-title {
  margin: 0 0 8px;
  color: #687386;
  font-size: 12px;
}
.host-disk-toggle { padding: 0; }
.host-disk-danger { color: #cf1322; font-weight: 600; }
.host-info-missing { color: #8c95a5; font-size: 12px; line-height: 20px; }
/* 空态用一行灰色文字而不是 a-empty：默认 empty 会撑出约 80px 高度，把「监听端口」与
   「日志文件」两节推得很远、白占空间（2026-09-21 现场）。 */
.section-empty {
  padding: 2px 0 6px;
  color: #8c95a5;
  font-size: 12px;
  line-height: 18px;
}
/* 服务详情两节（监听端口/日志文件）用小标题间距，避免连续两个空态把版面拉长。 */
.service-ports-section .child-section-title { margin: 12px 0 6px; }
@media (max-width: 760px) {
  .metric-band { grid-template-columns: repeat(2, minmax(100px, 1fr)); }
  .metric-item:nth-child(2) { border-right: 0; }
  .metric-item:nth-child(-n + 2) { border-bottom: 1px solid #e5e7eb; }
}
</style>
