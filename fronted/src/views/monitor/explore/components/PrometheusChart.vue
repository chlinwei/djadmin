<template>
  <div class="prometheus-chart-wrapper">
    <div ref="chartRef" class="prometheus-chart"></div>
    <!-- legend 在图表外部渲染，避免 ECharts 内部 legend 分列问题 -->
    <div v-if="legendItems.length" class="prometheus-legend">
      <div
        v-for="item in legendItems"
        :key="item.name"
        class="prometheus-legend-item"
        :class="{ 'is-hidden': hiddenSeries.has(item.name) }"
        @click.exact="onLegendClick(item.name)"
        @click.ctrl.exact="onLegendCtrlClick(item.name)"
      >
        <span class="legend-dot" :style="{ background: item.color }"></span>
        <span class="legend-label">{{ item.name }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'

const props = defineProps({
  // 查询结果数据，格式来自 Prometheus 范围查询响应
  resultData: {
    type: Object,
    default: () => ({}),
  },
  // 图表容器高度（px）
  height: {
    type: Number,
    default: 400,
  },
})

const chartRef = ref(null)
let chartInstance = null

// 外部 legend 状态
const legendItems = ref([])   // [{ name, color }]
const hiddenSeries = ref(new Set())
let soloSeriesName = null

// 将 Prometheus 范围查询结果转换为 ECharts 格式
function buildChartOption(resultData) {
  if (!resultData || !resultData.result || !Array.isArray(resultData.result)) {
    return {
      title: { text: '暂无数据' },
      grid: { left: 60, right: 20, top: 40, bottom: 40 },
      xAxis: { type: 'time' },
      yAxis: { type: 'value' },
      series: [],
    }
  }

  const series = []
  const timeMap = new Set() // 收集所有时间戳

  // 遍历每个时间序列（metric）
  resultData.result.forEach((item) => {
    const metric = item.metric || {}
    const values = item.values || []

    // 构建 series 中该 metric 的名称（带上主要标签）
    const metricName = buildMetricLabel(metric)

    // 收集所有时间戳
    values.forEach(([ts]) => {
      timeMap.add(ts)
    })

    // 将 values 转换为 ECharts 数据格式，并在间隔过大处插入 null 断点
    const data = buildSeriesData(values)

    series.push({
      name: metricName,
      type: 'line',
      data: data,
      smooth: false,
      connectNulls: false,
      symbol: 'none',
      lineStyle: { width: 1.5 },
    })
  })

  // 如果没有数据就显示空状态
  if (series.length === 0) {
    return {
      title: { text: '暂无数据' },
      grid: { left: 60, right: 20, top: 40, bottom: 40 },
      xAxis: { type: 'time' },
      yAxis: { type: 'value' },
      series: [],
    }
  }

  // 提取 legend 项和颜色
  const colors = ['#5470c6', '#91419f', '#ee6666', '#73c0de', '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc']
  legendItems.value = series.map((s, i) => ({
    name: s.name,
    color: colors[i % colors.length],
  }))
  // 数据刷新时清空隐藏状态
  hiddenSeries.value.clear()
  soloSeriesName = null

  return {
    title: {
      text: '时间序列数据',
      left: 'center',
      textStyle: { fontSize: 12 },
    },
    tooltip: {
      trigger: 'axis',
      formatter: (params) => {
        if (!Array.isArray(params) || params.length === 0) {
          return ''
        }
        const time = new Date(params[0].axisValue).toLocaleString('zh-CN')
        let html = `<div style="padding: 4px; font-size: 12px;"><strong>${time}</strong><br/>`
        params.forEach((item) => {
          html += `<span style="color: ${item.color};">●</span> ${item.seriesName}: <strong>${item.value[1]}</strong><br/>`
        })
        html += '</div>'
        return html
      },
      axisPointer: {
        type: 'cross',
      },
    },
    legend: { show: false },  // legend 由外部 HTML 接管
    grid: {
      left: 60,
      right: 20,
      top: 50,
      bottom: 40,
      containLabel: false,
    },
    xAxis: {
      type: 'time',
      boundaryGap: false,
      axisLabel: {
        fontSize: 11,
        formatter: (value) => {
          const date = new Date(value)
          return date.toLocaleTimeString('zh-CN', {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
          })
        },
      },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        fontSize: 11,
      },
      splitLine: {
        lineStyle: {
          color: '#e8e8e8',
        },
      },
    },
    series: series.map((s, i) => ({
      ...s,
      lineStyle: { ...s.lineStyle, color: colors[i % colors.length] },
    })),
  }
}

// 将 Prometheus values 数组转为 ECharts 数据，相邻点间隔超过 2.5 倍 step 时插入 null 断点
function buildSeriesData(values) {
  if (!values || values.length === 0) return []

  // 自动推断 step：取前几对点间隔的中位数
  let inferredStep = 0
  if (values.length >= 2) {
    const gaps = []
    for (let i = 1; i < Math.min(values.length, 10); i++) {
      gaps.push(values[i][0] - values[i - 1][0])
    }
    gaps.sort((a, b) => a - b)
    inferredStep = gaps[Math.floor(gaps.length / 2)]
  }

  const result = []
  for (let i = 0; i < values.length; i++) {
    const [ts, value] = values[i]
    // 与上一个点间隔超过 2.5 倍 step，视为数据断裂，插入 null
    if (i > 0 && inferredStep > 0) {
      const gap = ts - values[i - 1][0]
      if (gap > inferredStep * 2.5) {
        result.push([ts * 1000 - 1, null])  // 断点前插 null
      }
    }
    result.push([ts * 1000, parseFloat(value)])
  }
  return result
}

// 根据 metric 对象生成友好标签（取关键标签）
function buildMetricLabel(metric) {
  if (!metric) return '未命名'

  // 优先显示 __name__ 作为指标名，然后追加重要的标签
  const name = metric.__name__ || ''
  const importantLabels = []

  // 这些标签通常是标识不同线条的关键标签
  const priorityLabels = ['instance', 'job', 'service', 'device', 'pod', 'node', 'hostname']

  priorityLabels.forEach((label) => {
    if (metric[label]) {
      importantLabels.push(`${label}="${metric[label]}"`)
    }
  })

  if (importantLabels.length === 0) {
    // 如果没有找到优先标签，随机选几个
    Object.keys(metric).forEach((key) => {
      if (key !== '__name__' && importantLabels.length < 2) {
        importantLabels.push(`${key}="${metric[key]}"`)
      }
    })
  }

  return importantLabels.length > 0 ? `${name}{${importantLabels.join(', ')}}` : name
}

function resizeChart() {
  if (chartInstance && chartRef.value) {
    chartInstance.resize()
  }
}

// 单击 legend 项：只显示该系列；再次单击同一项 → 恢复全部显示
function onLegendClick(seriesName) {
  if (soloSeriesName === seriesName) {
    // 已是单选状态，再点一次恢复全部
    soloSeriesName = null
    hiddenSeries.value.clear()
  } else {
    // 单选该系列，其余隐藏
    soloSeriesName = seriesName
    hiddenSeries.value.clear()
    // 隐藏除了 seriesName 之外的所有系列
    legendItems.value.forEach((item) => {
      if (item.name !== seriesName) {
        hiddenSeries.value.add(item.name)
      }
    })
  }
  updateChartVisibility()
}

// CTRL+单击 legend 项：多选模式，切换该系列的显示状态
function onLegendCtrlClick(seriesName) {
  // 如果当前处于单选模式，切换到多选模式
  if (soloSeriesName !== null) {
    soloSeriesName = null
  }
  // 切换该系列的隐藏状态
  if (hiddenSeries.value.has(seriesName)) {
    hiddenSeries.value.delete(seriesName)
  } else {
    hiddenSeries.value.add(seriesName)
  }
  updateChartVisibility()
}

// 根据 hiddenSeries 状态更新图表显示
function updateChartVisibility() {
  if (!chartInstance) return

  // 构建 legend.selected 对象：隐藏的系列设为 false，其余设为 true
  const selected = {}
  legendItems.value.forEach((item) => {
    selected[item.name] = !hiddenSeries.value.has(item.name)
  })
  
  // 通过更新 legend.selected 来控制系列显示隐藏
  chartInstance.setOption({ legend: { selected } })
}

onMounted(() => {
  if (!chartRef.value) return

  // 初始化图表实例
  chartInstance = echarts.init(chartRef.value, null, { renderer: 'canvas' })

  // 监听窗口大小变化
  window.addEventListener('resize', resizeChart)

  // 设置初始选项
  chartInstance.setOption(buildChartOption(props.resultData))
})

watch(
  () => props.resultData,
  (newData) => {
    if (chartInstance) {
      soloSeriesName = null  // 数据刷新时重置单选状态
      chartInstance.setOption(buildChartOption(newData), true)
    }
  },
  { deep: true },
)

watch(
  () => props.height,
  () => {
    nextTick(() => {
      resizeChart()
    })
  },
)

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeChart)
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
})
</script>

<style scoped>
.prometheus-chart-wrapper {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  height: v-bind('height + "px"');
}

.prometheus-chart {
  flex: 1;
  min-width: 0;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  background: #fafafa;
}

.prometheus-legend {
  width: 100%;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  background: #fff;
  padding: 8px 0;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
}

.prometheus-legend-item {
  display: flex;
  align-items: center;
  padding: 4px 12px;
  cursor: pointer;
  user-select: none;
  transition: background-color 0.2s;
  flex-shrink: 0;
}

.prometheus-legend-item:hover {
  background-color: #f5f5f5;
}

.prometheus-legend-item.is-hidden {
  opacity: 0.4;
  text-decoration: line-through;
}

.legend-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 2px;
  margin-right: 8px;
  flex-shrink: 0;
}

.legend-label {
  font-size: 12px;
}
</style>
