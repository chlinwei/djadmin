<template>
    <div class="metric-chart-shell">
        <div ref="chartRef" class="metric-chart"></div>
    </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { formatTimeWithTimezone } from '@/util/timezone'

const props = defineProps({
    title: { type: String, required: true },
    result: { type: Array, default: () => [] },
    timezone: { type: String, default: 'Asia/Shanghai' },
    seriesLabel: { type: Function, default: () => '使用率' },
    unit: { type: String, default: '%' },
    yAxisMax: { type: Number, default: 100 },
})

const chartRef = ref(null)
let chart = null
let resizeObserver = null

function buildOption() {
    const timestamps = props.result.flatMap((item) => (item.values || []).map(([timestamp]) => Number(timestamp) * 1000))
    const minTimestamp = timestamps.length ? Math.min(...timestamps) : 0
    const maxTimestamp = timestamps.length ? Math.max(...timestamps) : 0
    const rangeMilliseconds = Math.max(0, maxTimestamp - minTimestamp)
    const axisTimeFormat = rangeMilliseconds <= 24 * 60 * 60 * 1000 ? 'HH:mm' : 'MM-DD HH:mm'
    const series = props.result.map((item, index) => ({
        name: props.seriesLabel(item.metric || {}, index),
        type: 'line',
        data: (item.values || []).map(([timestamp, value]) => [Number(timestamp) * 1000, Number(value)]),
        showSymbol: false,
        connectNulls: false,
        smooth: false,
        lineStyle: { width: 2 },
        areaStyle: props.result.length === 1 ? { opacity: 0.08 } : undefined,
    }))

    return {
        color: ['#1677ff', '#52c41a', '#fa8c16', '#eb2f96', '#722ed1', '#13c2c2'],
        title: { text: props.title, left: 16, top: 10, textStyle: { fontSize: 14, fontWeight: 600 } },
        tooltip: {
            trigger: 'axis',
            formatter(params) {
                if (!Array.isArray(params) || !params.length) return ''
                const time = formatTimeWithTimezone(params[0].axisValue, props.timezone)
                const rows = params.map((item) => {
                    const unitSuffix = props.unit ? ` ${props.unit}` : ''
                    return `${item.marker}${item.seriesName}: <strong>${Number(item.value[1]).toFixed(2)}${unitSuffix}</strong>`
                })
                return `${time}<br/>${rows.join('<br/>')}`
            },
        },
        legend: { type: 'scroll', top: 10, right: 16, left: 120 },
        grid: { left: 52, right: 24, top: 58, bottom: 42 },
        xAxis: {
            type: 'time',
            splitNumber: 6,
            axisLabel: {
                hideOverlap: true,
                margin: 12,
                formatter: (value) => formatTimeWithTimezone(value, props.timezone, axisTimeFormat),
            },
        },
        yAxis: {
            type: 'value',
            min: 0,
            max: props.yAxisMax || undefined,
            axisLabel: { formatter: (value) => `${value}${props.unit === '%' ? '%' : ''}` },
            splitLine: { lineStyle: { color: '#f0f0f0' } },
        },
        series,
        graphic: series.length ? [] : [{
            type: 'text',
            left: 'center',
            top: 'middle',
            style: { text: '暂无数据', fill: '#8c8c8c', fontSize: 14 },
        }],
    }
}

function renderChart() {
    if (!chart) return
    chart.setOption(buildOption(), true)
}

onMounted(() => {
    chart = echarts.init(chartRef.value)
    resizeObserver = new ResizeObserver(() => chart?.resize())
    resizeObserver.observe(chartRef.value)
    renderChart()
})

watch(() => [props.result, props.timezone, props.title], () => nextTick(renderChart), { deep: true })

onBeforeUnmount(() => {
    resizeObserver?.disconnect()
    chart?.dispose()
    chart = null
})
</script>

<style scoped>
.metric-chart-shell {
    min-width: 0;
    border: 1px solid #e5e7eb;
    border-radius: 8px;
    background: #fff;
}

.metric-chart {
    width: 100%;
    height: 300px;
}
</style>