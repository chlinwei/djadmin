<template>
  <div>
    <div class="service-tree-toolbar">
      <a-tooltip title="刷新">
        <a-button type="primary" ghost :loading="refreshing" @click="refreshServiceTree">
          <ReloadOutlined />
          <span>刷新</span>
        </a-button>
      </a-tooltip>
    </div>
    <div class="service-tree-page">
      <ServiceTree
        ref="serviceTreeRef"
        :selected-scope="serviceScope"
        :show-stats="false"
        :stats-dimension="statsDimension"
        @update:statsDimension="statsDimension = $event"
        @stats-change="handleStatsChange"
        @select="serviceScope = $event"
      />
      <main class="service-tree-content">
        <section v-show="serviceScope.nodeType === 'all'" class="service-tree-right-stats">
          <div class="service-tree-right-stats-header">
            <span>资源占比</span>
            <a-segmented
              v-model:value="statsDimension"
              :options="statsDimensionOptions"
              size="small"
            />
          </div>
          <div class="service-tree-right-stats-subtitle">按 CPU / 内存 资源汇总</div>
          <div class="service-tree-right-stats-grid">
            <div class="service-tree-right-chart-card">
              <div class="service-tree-right-chart-title">
                <span>CPU 占比</span>
                <strong>总 CPU {{ formatResource(statsData.totalCpu, ' 核') }}</strong>
              </div>
              <div ref="hostPieRef" class="service-tree-right-chart"></div>
            </div>
            <div class="service-tree-right-chart-card">
              <div class="service-tree-right-chart-title">
                <span>内存占比</span>
                <strong>总内存 {{ formatResource(statsData.totalMemory, ' GB') }}</strong>
              </div>
              <div ref="deploymentPieRef" class="service-tree-right-chart"></div>
            </div>
          </div>
        </section>
        <ServiceTreeNodeContent ref="nodeContentRef" :scope="serviceScope" @navigate="serviceScope = $event" />
      </main>
    </div>
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { ReloadOutlined } from '@ant-design/icons-vue'
import ServiceTree from '../application/components/ServiceTree.vue'
import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'

const serviceScope = ref({ nodeType: 'all', nodeTitle: '全部业务' })
const serviceTreeRef = ref(null)
const nodeContentRef = ref(null)
const refreshing = ref(false)
const statsDimension = ref('business')
const statsDimensionOptions = [
  { label: '按业务', value: 'business' },
  { label: '按项目', value: 'project' },
]
const statsData = ref({ cpuRows: [], memoryRows: [], totalCpu: 0, totalMemory: 0 })
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

function buildPieOption(title, rows) {
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
    hostPieChart.setOption(buildPieOption('CPU 占比', statsData.value.cpuRows || []), true)
  }
  if (deploymentPieChart) {
    deploymentPieChart.setOption(buildPieOption('内存占比', statsData.value.memoryRows || []), true)
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
      nodeContentRef.value?.refresh(),
    ])
  } finally {
    refreshing.value = false
  }
}

watch(
  () => [statsData.value.cpuRows, statsData.value.memoryRows],
  async () => {
    await nextTick()
    renderCharts()
  },
  { deep: true },
)

onMounted(() => {
  window.addEventListener('resize', resizeCharts)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeCharts)
  destroyCharts()
})
</script>

<style scoped>
.service-tree-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}
.service-tree-toolbar .ant-btn { display: inline-flex; align-items: center; gap: 6px; }
.service-tree-page {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  min-width: 0;
}
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
  grid-template-columns: repeat(2, minmax(260px, 1fr));
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
  .service-tree-page { flex-direction: column; }
  .service-tree-page :deep(.service-tree) {
    width: 100%;
    height: auto;
    min-height: 0;
    max-height: 320px;
  }
  .service-tree-content { width: 100%; }
  .service-tree-right-stats-grid {
    grid-template-columns: 1fr;
  }
}
</style>