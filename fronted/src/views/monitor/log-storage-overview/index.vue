<template>
  <div class="page-container">
    <a-card :bordered="false" style="margin-bottom: 12px">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap">
        <span style="font-weight: 600">日志存储水位</span>
        <a-select
          v-model:value="selectedClusterId"
          :options="clusterOptions"
          style="width: 280px"
          placeholder="选择 OpenSearch 集群"
          @change="loadOverview"
        />
        <a-button size="small" :loading="loading" :disabled="!selectedClusterId" @click="loadOverview">
          刷新
        </a-button>
        <span v-if="generatedAt" style="color: #999; font-size: 12px">数据时间：{{ generatedAt }}</span>
      </div>
    </a-card>

    <a-empty v-if="!clusters.length" description="尚未配置 OpenSearch 集群，请先到「日志存储」页添加" />
    <template v-else>
      <a-row :gutter="12">
        <a-col :span="8">
          <a-card :bordered="false" size="small" title="层级" style="min-height: 480px">
            <a-input-search v-model:value="treeSearch" placeholder="搜索" size="small" style="margin-bottom: 8px" />
            <a-spin v-if="loading" size="small" />
            <a-tree
              v-else
              v-model:selectedKeys="selectedKeys"
              :tree-data="treeData"
              :expanded-keys="expandedKeys"
              @select="selectNode"
              @expand="onExpand"
              block-node
            >
              <template #title="{ dataRef }">
                <span>{{ dataRef.title }}</span>
                <a-tag v-if="dataRef.meta && dataRef.meta.bytes" style="margin-left: 8px" :color="healthColor(dataRef.meta.health)">
                  {{ formatBytes(dataRef.meta.bytes) }}
                </a-tag>
                <a-tag v-if="dataRef.level === 'service' && dataRef.meta?.bytes" style="margin-left: 8px" :color="healthColor(dataRef.meta.health)">
                  {{ formatBytes(dataRef.meta.bytes) }}
                </a-tag>
              </template>
            </a-tree>
          </a-card>
        </a-col>
        <a-col :span="16">
          <a-card :bordered="false" size="small" style="min-height: 480px">
            <template #title>{{ selectedTitle }}</template>
            <a-spin v-if="loading" size="small" />
            <template v-else>
              <!-- 顶层：节点磁盘水位 + 汇总 -->
              <template v-if="selectedLevel === 'root'">
                <a-row :gutter="8" style="margin-bottom: 12px">
                  <a-col :span="6"><a-statistic title="总占用" :value="formatBytes(totalBytes)" /></a-col>
                  <a-col :span="6"><a-statistic title="总文档数" :value="totalDocs" /></a-col>
                  <a-col :span="6"><a-statistic title="data stream 数" :value="streams.length" /></a-col>
                  <a-col :span="6"><a-statistic title="健康异常流" :value="unhealthyStreams" /></a-col>
                </a-row>
                <a-table
                  size="small"
                  row-key="node"
                  :columns="allocationColumns"
                  :data-source="allocation"
                  :pagination="false"
                  :locale="tableLocale"
                />
                <div v-if="allocError" style="color: #faad14; margin-top: 8px">磁盘水位获取失败：{{ allocError }}</div>
              </template>

              <!-- 顶层下未识别流的容器 -->
              <template v-else-if="selectedLevel === 'unknown'">
                <a-alert type="warning" show-icon message="以下 data stream 无法按 <前缀>-<项目>-<环境>-<业务系统>-<档位编码> 解析（可能为手工创建或维度已删除）" style="margin-bottom: 12px" />
                <a-table
                  size="small"
                  row-key="name"
                  :columns="streamColumns"
                  :data-source="selectedStreams"
                  :pagination="false"
                  :locale="tableLocale"
                />
              </template>

              <!-- 项目 / 业务系统 / 环境：流明细一致，环境层额外展开后备索引 -->
              <template v-else-if="['project', 'bizsys', 'env', 'ungrouped'].includes(selectedLevel)">
                <a-row :gutter="8" style="margin-bottom: 12px">
                  <a-col :span="8"><a-statistic title="总占用" :value="formatBytes(sumBytes(selectedStreams))" /></a-col>
                  <a-col :span="8"><a-statistic title="总文档数" :value="sumDocs(selectedStreams)" /></a-col>
                  <a-col :span="8"><a-statistic title="data stream 数" :value="selectedStreams.length" /></a-col>
                </a-row>
                <a-table
                  size="small"
                  row-key="name"
                  :columns="streamColumns"
                  :data-source="selectedStreams"
                  :pagination="false"
                  :locale="tableLocale"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'bytes'">
                      <span>{{ formatBytes(record.bytes) }}</span>
                    </template>
                    <template v-else-if="column.key === 'docs'">
                      <span>{{ Number(record.docs || 0).toLocaleString() }}</span>
                    </template>
                    <template v-else-if="column.key === 'backing_count'">
                      <span>{{ (record.backing_indices || []).length }}</span>
                    </template>
                    <template v-else-if="column.key === 'ism'">
                      <a-tag v-if="record.ism_state" color="blue">{{ record.ism_state }}</a-tag>
                      <span v-else>-</span>
                    </template>
                    <template v-else-if="column.key === 'actions'">
                      <a-button type="link" size="small" style="padding: 0" @click="expandStream(record)">后备索引</a-button>
                    </template>
                  </template>
                </a-table>
                <template v-if="expandedStream">
                  <a-divider style="margin: 12px 0" />
                  <a-alert
                    type="info"
                    show-icon
                    :message="`data stream ${expandedStream.name} 的后备索引（这是真实磁盘占用的最细粒度）`"
                    style="margin-bottom: 8px"
                  />
                  <a-table
                    size="small"
                    row-key="index"
                    :columns="backingColumns"
                    :data-source="expandedStream.backing_indices"
                    :pagination="false"
                    :locale="tableLocale"
                  >
                    <template #bodyCell="{ column, record }">
                      <template v-if="column.key === 'bytes'">
                        <span>{{ formatBytes(record.bytes) }}</span>
                      </template>
                      <template v-else-if="column.key === 'docs'">
                        <span>{{ Number(record.docs || 0).toLocaleString() }}</span>
                      </template>
                      <template v-else-if="column.key === 'ism'">
                        <a-tag v-if="record.ism_state" color="blue">{{ record.ism_state }}</a-tag>
                        <span v-else>-</span>
                      </template>
                    </template>
                  </a-table>
                </template>
              </template>

              <!-- 逻辑服务：新命名下每个服务有自己的流，可显示真实占用；旧流只有写入量 -->
              <template v-else-if="selectedLevel === 'service'">
                <a-descriptions v-if="selectedService" :column="2" size="small" bordered style="margin-bottom: 12px">
                  <a-descriptions-item label="所属业务系统">{{ selectedService.bizsysName }}</a-descriptions-item>
                  <a-descriptions-item label="所属环境">{{ selectedService.envName }}</a-descriptions-item>
                  <a-descriptions-item label="data stream">{{ selectedService.streamName }}</a-descriptions-item>
                  <a-descriptions-item label="保留档位">
                    <a-tag v-if="selectedService.tier" color="blue">{{ selectedService.tier }}</a-tag>
                    <span v-else>-</span>
                  </a-descriptions-item>
                  <a-descriptions-item label="磁盘占用">
                    <span v-if="selectedService.bytes">{{ formatBytes(selectedService.bytes) }}（真实值）</span>
                    <a-tag v-else color="orange">未按服务分流的旧流，无独立占用</a-tag>
                  </a-descriptions-item>
                  <a-descriptions-item label="文档数">{{ Number(selectedService.docs || 0).toLocaleString() }}</a-descriptions-item>
                </a-descriptions>
                <a-table
                  size="small"
                  row-key="service"
                  :columns="[
                    { title: '逻辑服务', dataIndex: 'service', key: 'service' },
                    { title: `近 ${serviceUsageDays} 天写入文档数`, dataIndex: 'docs', key: 'docs', width: 180 },
                  ]"
                  :data-source="serviceUsage"
                  :pagination="false"
                  :locale="tableLocale"
                  :loading="serviceUsageLoading"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'docs'">
                      <span>{{ Number(record.docs).toLocaleString() }}</span>
                    </template>
                  </template>
                </a-table>
              </template>
            </template>
          </a-card>
        </a-col>
      </a-row>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { tableLocale } from '@/util/tableStyle'
import { message } from 'ant-design-vue'
import { getLogServiceUsage, getLogStorageOverview, getOpenSearchClusterList } from '@/api/monitor.js'

// 存储水位：真实磁盘占用的原子粒度是 data stream，命名 = logs-<项目>-<环境>-<业务系统>-<档位编码>。
// 树的顶层/项目/业务系统/环境是流的真实聚合；逻辑服务层只有写入量（文档数）口径。

const clusters = ref([])
const selectedClusterId = ref(undefined)
const overview = ref(null)
const loading = ref(false)
const generatedAt = ref('')
const treeSearch = ref('')
const selectedKeys = ref([])
const expandedKeys = ref([])
const expandedStream = ref(null)
const serviceUsage = ref([])
const serviceUsageLoading = ref(false)
const serviceUsageDays = ref(30)

const clusterOptions = computed(() =>
  clusters.value.map((item) => ({ value: item.id, label: item.name || `集群 #${item.id}` }))
)

const streams = computed(() => overview.value?.data_streams || [])
const allocation = computed(() => overview.value?.allocation || [])
const allocError = computed(() => overview.value?.alloc_error || '')
const dims = computed(() => overview.value?.dims || {})

const totalBytes = computed(() => sumBytes(streams.value))
const totalDocs = computed(() => sumDocs(streams.value))
const unhealthyStreams = computed(() => streams.value.filter((item) => ['red', 'yellow'].includes(item.health)).length)

const selectedLevel = ref('root')
const selectedTitle = ref('集群总览')
const selectedStreams = ref([])
const selectedService = ref(null)

function sumBytes(list) {
  return (list || []).reduce((sum, item) => sum + Number(item.bytes || 0), 0)
}
function sumDocs(list) {
  return (list || []).reduce((sum, item) => sum + Number(item.docs || 0), 0)
}
function formatBytes(value) {
  const size = Number(value) || 0
  if (!size) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let index = 0
  let display = size
  while (display >= 1024 && index < units.length - 1) {
    display /= 1024
    index += 1
  }
  return `${index === 0 ? display : display.toFixed(2)} ${units[index]}`
}
function healthColor(health) {
  if (health === 'red') return 'red'
  if (health === 'yellow') return 'orange'
  return 'green'
}
function onExpand(keys) {
  expandedKeys.value = keys
}

const allocationColumns = [
  { title: '节点', dataIndex: 'name', key: 'name' },
  { title: '地址', dataIndex: 'node', key: 'node' },
  { title: '分片数', dataIndex: 'shards', key: 'shards', width: 90 },
  { title: '磁盘已用', dataIndex: 'disk.used', key: 'disk.used', width: 110 },
  { title: '磁盘总量', dataIndex: 'disk.total', key: 'disk.total', width: 110 },
  { title: '使用率', dataIndex: 'disk.percent', key: 'disk.percent', width: 90 },
]

const streamColumns = [
  { title: 'data stream', dataIndex: 'name', key: 'name' },
  { title: '健康', dataIndex: 'health', key: 'health', width: 80 },
  { title: '占用', dataIndex: 'bytes', key: 'bytes', width: 110 },
  { title: '文档数', dataIndex: 'docs', key: 'docs', width: 110 },
  { title: '后备索引数', dataIndex: 'backing_count', key: 'backing_count', width: 110 },
  { title: 'ISM 状态', dataIndex: 'ism_state', key: 'ism', width: 120 },
  { title: '详情', key: 'actions', width: 90 },
]

const backingColumns = [
  { title: '索引', dataIndex: 'index', key: 'index' },
  { title: '健康', dataIndex: 'health', key: 'health', width: 80 },
  { title: '占用', dataIndex: 'bytes', key: 'bytes', width: 110 },
  { title: '文档数', dataIndex: 'docs', key: 'docs', width: 110 },
  { title: '创建时间', dataIndex: 'create_at', key: 'create_at', width: 180 },
  { title: 'ISM 状态', dataIndex: 'ism_state', key: 'ism', width: 120 },
]

// ---- 树构建 ----
const treeData = computed(() => {
  const search = treeSearch.value.trim().toLowerCase()
  const projects = dims.value.projects || []
  const bizsystems = dims.value.business_systems || []
  const services = dims.value.services || []

  const match = (text) => !search || String(text || '').toLowerCase().includes(search)

  const projectNodes = []
  const usedBizsys = new Set()
  projects.forEach((project) => {
    const children = []
    bizsystems
      .filter((item) => item.project_id === project.id)
      .forEach((bizsys) => {
        usedBizsys.add(bizsys.code)
        children.push(bizsysNode(bizsys, match))
      })
    if (!search || match(project.name) || match(project.code) || children.length) {
      projectNodes.push({
        key: `project:${project.code || project.id}`,
        title: `项目：${project.name}`,
        level: 'project',
        meta: {},
        children,
      })
    }
  })

  // 未挂项目的业务系统
  const orphan = bizsystems.filter((item) => !usedBizsys.has(item.code))
  if (orphan.length) {
    projectNodes.push({
      key: 'ungrouped',
      title: '未分组业务系统',
      level: 'ungrouped',
      meta: {},
      children: orphan.map((bizsys) => bizsysNode(bizsys, match)),
    })
  }

  // 无法解析维度的流
  const unknownStreams = streams.value.filter((item) => !item.recognized)
  const nodes = [
    {
      key: 'root',
      title: '集群总览',
      level: 'root',
      meta: { bytes: totalBytes.value, health: unhealthyStreams.value ? 'yellow' : 'green' },
      children: [...projectNodes, ...(unknownStreams.length ? [unknownNode(unknownStreams)] : [])],
    },
  ]
  return nodes
})

function bizsysNode(bizsys, match) {
  const bizsysStreams = streams.value.filter((item) => item.recognized && item.business_system === bizsys.code)
  const environments = dims.value.environments || []
  const children = environments
    .map((env) => {
      const envStreams = bizsysStreams.filter((item) => item.environment === env.code)
      return {
        key: `bizsys:${bizsys.code}:env:${env.code}`,
        title: `环境：${env.name}`,
        level: 'env',
        bizsysCode: bizsys.code,
        bizsysName: bizsys.name,
        envCode: env.code,
        envName: env.name,
        meta: { bytes: sumBytes(envStreams), health: worstHealth(envStreams) },
        streams: envStreams,
        children: [],
        isLeaf: false,
      }
    })
    .filter((node) => node.streams.length)
  return {
    key: `bizsys:${bizsys.code}`,
    title: `业务系统：${bizsys.name}`,
    level: 'bizsys',
    bizsysCode: bizsys.code,
    bizsysName: bizsys.name,
    meta: { bytes: sumBytes(bizsysStreams), health: worstHealth(bizsysStreams) },
    children: match(bizsys.name) || match(bizsys.code) || children.length ? children : [],
  }
}


function unknownNode(list) {
  return {
    key: 'unknown',
    title: `未识别 (${list.length})`,
    level: 'unknown',
    meta: { bytes: sumBytes(list), health: worstHealth(list) },
    streams: list,
    children: [],
  }
}

function worstHealth(list) {
  if ((list || []).some((item) => item.health === 'red')) return 'red'
  if ((list || []).some((item) => item.health === 'yellow')) return 'yellow'
  return 'green'
}

function selectNode(selectedKeysValue, { node }) {
  const meta = node.meta || {}
  expandedStream.value = null
  selectedLevel.value = node.level || 'root'
  selectedTitle.value = node.title
  if (node.level === 'service') {
    selectedService.value = {
      bizsysName: node.bizsysName,
      envName: node.envName,
      streamName: node.streamName,
      tier: node.tier,
      bytes: node.meta?.bytes || 0,
      docs: node.meta?.docs || 0,
    }
    loadServiceUsage(node.bizsysCode, node.envCode, node.serviceCode)
    return
  }
  if (node.level === 'root') {
    selectedStreams.value = []
    return
  }
  if (node.level === 'project') {
    // 项目的流 = 下属业务系统（含未分组切换后仍按 key 收集）——直接汇总子树已挂的流由子节点聚合，这里收集项目下全部 bizsys 的流
    const bizsysCodes = (node.children || [])
      .flatMap((child) => collectBizsysCodes(child))
    selectedStreams.value = streams.value.filter((item) => item.recognized && bizsysCodes.includes(item.business_system))
    return
  }
  if (node.level === 'ungrouped') {
    selectedStreams.value = streams.value.filter((item) => item.recognized && !bizsysHasProject(item.business_system))
    return
  }
  if (node.streams) {
    selectedStreams.value = node.streams
  }
}

function collectBizsysCodes(node) {
  if (node.level === 'bizsys') return [node.bizsysCode]
  return (node.children || []).flatMap((child) => collectBizsysCodes(child))
}

function bizsysHasProject(code) {
  const bizsystems = dims.value.business_systems || []
  const found = bizsystems.find((item) => item.code === code)
  return Boolean(found && found.project_id)
}

function expandStream(record) {
  expandedStream.value = record
}

async function loadServiceUsage(bizsysCode, envCode, serviceCode) {
  serviceUsageLoading.value = true
  try {
    const response = await getLogServiceUsage(selectedClusterId.value, {
      business_system: bizsysCode,
      environment: envCode,
      days: serviceUsageDays.value,
    })
    const payload = response?.data?.data || {}
    let items = payload.items || []
    if (serviceCode) {
      items = items.filter((item) => item.service === serviceCode)
    }
    serviceUsage.value = items
    if (!items.length) {
      message.info('该范围内近期没有日志写入')
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '获取服务写入量失败')
  } finally {
    serviceUsageLoading.value = false
  }
}

async function loadClusters() {
  try {
    const response = await getOpenSearchClusterList({ page: 1, page_size: 100 })
    const payload = response?.data?.data || {}
    clusters.value = Array.isArray(payload.results) ? payload.results : []
    if (clusters.value.length && !selectedClusterId.value) {
      selectedClusterId.value = clusters.value[0].id
      await loadOverview()
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '获取集群列表失败')
  }
}

async function loadOverview() {
  if (!selectedClusterId.value) {
    return
  }
  loading.value = true
  try {
    const response = await getLogStorageOverview(selectedClusterId.value)
    if (response?.data?.code !== 200) {
      message.error(response?.data?.msg || '获取存储水位失败')
      return
    }
    selectedKeys.value = ['root']
    expandedKeys.value = []
    overview.value = response.data.data || {}
    generatedAt.value = (overview.value.generated_at || '').replace('T', ' ').slice(0, 19)
    selectedLevel.value = 'root'
    selectedTitle.value = '集群总览'
    selectedStreams.value = []
    expandedStream.value = null
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '获取存储水位失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadClusters)
</script>

<style scoped>
.page-container {
  padding: 12px;
}
</style>
