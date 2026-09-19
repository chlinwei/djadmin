<template>
  <div ref="chartRef" class="storage-usage-pie" />
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'

// 容量占比饼图。只负责渲染：取哪些分组、零值怎么处理都在 util/storageUsagePie.js 里
// （纯函数，可直测——jsdom 没有真实 canvas，组件本身测不了渲染细节）。
const props = defineProps({
  option: { type: Object, default: null },
})

const chartRef = ref(null)
let chart = null

function render() {
  if (!chartRef.value) return
  if (!props.option) {
    // option 为 null 表示"没有可画的数据"（例如所有分组都是 0 占用），
    // 此时销毁实例而不是画一个空饼——空饼看着像加载失败。
    if (chart) {
      chart.dispose()
      chart = null
    }
    return
  }
  if (!chart) chart = echarts.init(chartRef.value)
  chart.setOption(props.option, true)
  chart.resize()
}

onMounted(render)
watch(() => props.option, render, { deep: true })
onBeforeUnmount(() => {
  if (chart) {
    chart.dispose()
    chart = null
  }
})
</script>

<style scoped>
.storage-usage-pie {
  width: 100%;
  height: 280px;
}
</style>
