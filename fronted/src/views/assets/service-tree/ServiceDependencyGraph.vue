<template>
  <Teleport to="body" :disabled="!isFullscreen">
  <div class="dependency-graph" :class="{ 'dependency-graph--fullscreen': isFullscreen }">
    <div class="graph-controls">
      <a-space wrap>
        <a-button :loading="layoutLoading" :disabled="!nodes.length" @click="arrange(true)">重新布局</a-button>
        <a-button :disabled="!layoutKey || !flowNodes.length || layoutLoading || (layoutSaved && !hasLayoutChanges)" @click="saveLayout"><FontAwesomeIcon :icon="['fas', 'floppy-disk']" /><span>保存布局</span></a-button>
        <a-button :disabled="!flowNodes.length || layoutLoading || !hasLayoutChanges" @click="restoreLayout">还原布局</a-button>
        <a-tag v-if="flowNodes.length && !layoutLoading" role="status" aria-live="polite" :color="hasLayoutChanges ? 'orange' : layoutSaved ? 'green' : 'default'">{{ hasLayoutChanges ? '有未保存的修改' : layoutSaved ? '已保存' : '尚未保存' }}</a-tag>
        <a-button :disabled="!flowNodes.length" @click="fit">适应画布</a-button>
        <a-button :loading="fullscreenPending" :aria-pressed="isFullscreen" @click="toggleFullscreen">
          <FullscreenExitOutlined v-if="isFullscreen" /><FullscreenOutlined v-else />
          <span>{{ isFullscreen ? '退出全屏' : '全屏显示' }}</span>
        </a-button>
        <a-button :disabled="!flowNodes.length || layoutLoading || !!imageAction" @click="exportSvg">导出 SVG</a-button>
        <a-button :loading="imageAction === 'png'" :disabled="!flowNodes.length || layoutLoading || !!imageAction" @click="exportPng">导出 PNG</a-button>
        <a-button :loading="imageAction === 'copy'" :disabled="!flowNodes.length || layoutLoading || !!imageAction" @click="copyImage">复制图片</a-button>
      </a-space>
      <span class="graph-legend">实线：强依赖　虚线：弱依赖　点线：待确认</span>
    </div>
    <a-alert v-if="layoutError" type="error" show-icon :message="layoutError" />
    <a-alert v-if="storageError" type="error" show-icon :message="storageError" />
    <!-- ASpin's class styles its indicator; wrapperClassName sizes the actual canvas parent. -->
    <a-spin wrapper-class-name="graph-spin" :spinning="layoutLoading">
      <div class="graph-canvas">
        <VueFlow
          v-if="flowNodes.length"
          v-model="flowElements"
          :nodes-connectable="false"
          :delete-key-code="null"
          :min-zoom="0.15"
          :max-zoom="2"
          @init="flowInstance = $event"
          @nodes-initialized="fit"
          @node-click="emit('select-node', $event.node.data.record)"
          @edge-click="emit('select-edge', $event.edge.data.record)"
        >
          <Background :gap="20" color="#dae3ef" />
          <template #node-service="{ data }">
            <div class="service-node" :class="{ 'service-node--external': data.record.external_to_selection }" :style="{ borderColor: runtimePresentation(data.record.runtime_summary).color }" role="button" tabindex="0" @keydown.enter="emit('select-node', data.record)" @keydown.space.prevent="emit('select-node', data.record)">
              <Handle type="target" :position="Position.Left" />
              <div class="node-title"><strong>{{ data.record.name }}</strong><a-tag v-if="!data.record.enabled">停用</a-tag><a-tag v-if="data.record.external_to_selection" color="blue">范围外</a-tag></div>
              <div class="node-code">{{ data.record.business_system_name }} · {{ data.record.code }}</div>
              <div class="node-status" :style="{ color: runtimePresentation(data.record.runtime_summary).color }">● {{ runtimePresentation(data.record.runtime_summary).label }}</div>
              <div class="node-summary">运行 {{ data.record.runtime_summary.running }} / 停止 {{ data.record.runtime_summary.stopped }} / 异常 {{ data.record.runtime_summary.error }} / 未知 {{ data.record.runtime_summary.unknown }}</div>
              <Handle type="source" :position="Position.Right" />
            </div>
          </template>
          <template #edge-dependency="edge">
            <BaseEdge :id="edge.id" :path="edgeGeometry(edge).path" :marker-end="edge.markerEnd" :style="edge.style" :interaction-width="20" />
            <path v-if="edgePlacement(edge).labelGuide" :d="edgePlacement(edge).labelGuide" fill="none" stroke="#b9c6d8" stroke-dasharray="3 3" pointer-events="none" />
            <EdgeLabelRenderer>
              <a-tooltip :title="`${edge.data.record.function_scope} · ${strengthLabels[edge.data.record.strength]}`" placement="top">
                <button class="edge-label nodrag nopan" :class="{ 'edge-label--dragging': draggingLabelId === edge.id }" :style="labelStyle(edge)" @pointerdown.stop="startLabelDrag($event, edge)" @lostpointercapture="cancelLabelDrag" @click.stop="clickLabel($event, edge)">{{ edge.data.record.function_scope }} · {{ strengthLabels[edge.data.record.strength] }}</button>
              </a-tooltip>
            </EdgeLabelRenderer>
          </template>
        </VueFlow>
        <a-empty v-else-if="!layoutLoading && !layoutError" description="当前范围暂无服务节点" />
      </div>
    </a-spin>
    <a-modal :open="!!copyPreviewUrl" title="右键复制图片" :width="960" centered :footer="null" @cancel="closeCopyPreview">
      <a-alert type="info" show-icon message="当前 HTTP 地址不支持一键复制。请在下方图片上点击鼠标右键，选择「复制图片」，再粘贴到微信或 Word；不要选择「复制图片地址」。若浏览器没有此菜单，请使用「导出 PNG」。" />
      <div class="copy-preview">
        <img v-if="copyPreviewUrl" :src="copyPreviewUrl" alt="服务依赖架构图，右键选择复制图片" @error="copyPreviewFailed" />
      </div>
    </a-modal>
  </div>
  </Teleport>
</template>

<script setup>
import { computed, nextTick, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, shallowRef, watch } from 'vue'
import { message } from 'ant-design-vue'
import { FullscreenExitOutlined, FullscreenOutlined } from '@ant-design/icons-vue'
import { BaseEdge, EdgeLabelRenderer, Handle, Position, VueFlow } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { dependencyGraphBounds, dependencySvg, edgeLabelHeight, edgeLabelWidth, graphEdges, layoutDependencyNodes, nodeHeight, nodeWidth, positionedDependencyEdges, runtimePresentation, strengthColors, strengthLabels } from './dependencyGraph'
import { copyDependencyPng, dependencyPng, downloadDependencyImage } from './dependencyGraphExport'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'

const props = defineProps({
  nodes: { type: Array, required: true },
  edges: { type: Array, required: true },
  title: { type: String, default: '服务依赖架构' },
  layoutKey: { type: String, default: '' },
})
const emit = defineEmits(['select-node', 'select-edge'])
const flowElements = ref([])
const flowNodes = computed(() => flowElements.value.filter((element) => !('source' in element)))
const renderedRelations = computed(() => flowElements.value.filter((element) => 'source' in element).map((edge) => edge.data.record))
const labelOffsets = ref({})
const baselineLabelOffsets = ref({})
const draggingLabelId = ref('')
const edgePlacements = computed(() => new Map(positionedDependencyEdges(flowNodes.value, renderedRelations.value, labelOffsets.value).map((edge) => [edge.id, edge])))
const flowInstance = shallowRef(null)
const layoutLoading = ref(false)
const layoutError = ref('')
const storageError = ref('')
const imageAction = ref('')
const copyPreviewUrl = ref('')
const layoutSaved = ref(false)
const baselinePositions = shallowRef(new Map())
const initialPositions = shallowRef(new Map())
const hasLayoutChanges = computed(() => flowNodes.value.some((node) => {
  const baseline = baselinePositions.value.get(node.id)
  return !baseline || node.position.x !== baseline.x || node.position.y !== baseline.y
}) || renderedRelations.value.some((edge) => {
  const current = labelOffsets.value[edge.id], baseline = baselineLabelOffsets.value[edge.id]
  return current?.x !== baseline?.x || current?.y !== baseline?.y
}))
const isFullscreen = ref(false)
const fullscreenPending = ref(false)
let ownsFullscreen = false
let viewActive = true
let disposed = false
let generation = 0
let activeLayoutKey = null
let savedPositions = new Map()
let savedLabelOffsets = {}
let labelDrag = null
let suppressedLabelClick = ''
const storageKey = () => `service-dependency-layout:v1:${props.layoutKey}`

function readLayout() {
  savedPositions = new Map()
  savedLabelOffsets = {}
  labelOffsets.value = {}
  baselineLabelOffsets.value = {}
  layoutSaved.value = false
  storageError.value = ''
  if (!props.layoutKey) return
  try {
    const raw = localStorage.getItem(storageKey())
    if (raw === null) return
    const layout = JSON.parse(raw)
    if (layout?.version !== 1 || !layout.positions || typeof layout.positions !== 'object' || Array.isArray(layout.positions)) throw new Error('保存的坐标格式无效')
    const entries = Object.entries(layout.positions)
    if (entries.some(([id, position]) => !/^[1-9]\d*$/.test(id) || !position || !Number.isFinite(position.x) || !Number.isFinite(position.y))) throw new Error('保存的坐标无效')
    if (layout.labelOffsets !== undefined) {
      if (!layout.labelOffsets || typeof layout.labelOffsets !== 'object' || Array.isArray(layout.labelOffsets)
        || Object.entries(layout.labelOffsets).some(([id, offset]) => !/^[1-9]\d*$/.test(id) || !offset || !Number.isFinite(offset.x) || !Number.isFinite(offset.y))) throw new Error('保存的标签坐标无效')
      savedLabelOffsets = layout.labelOffsets
    }
    labelOffsets.value = { ...savedLabelOffsets }
    baselineLabelOffsets.value = { ...savedLabelOffsets }
    savedPositions = new Map(entries)
    layoutSaved.value = entries.length > 0
  } catch (error) {
    storageError.value = `读取布局失败：${error.message}。当前显示自动布局，可重新保存。`
  }
}

function saveLayout() {
  try {
    // Merge visible nodes so saving a filtered graph does not erase hidden services' coordinates.
    const positions = new Map(savedPositions)
    for (const node of flowNodes.value) {
      if (!Number.isFinite(node.position.x) || !Number.isFinite(node.position.y)) throw new Error('节点坐标无效，请重新布局')
      positions.set(node.id, { x: node.position.x, y: node.position.y })
    }
    const offsets = { ...savedLabelOffsets }
    for (const edge of renderedRelations.value) {
      const offset = labelOffsets.value[edge.id]
      if (offset) offsets[edge.id] = { ...offset }
      else delete offsets[edge.id]
    }
    localStorage.setItem(storageKey(), JSON.stringify({ version: 1, positions: Object.fromEntries(positions), labelOffsets: offsets }))
    savedPositions = positions
    layoutSaved.value = true
    baselinePositions.value = new Map(positions)
    savedLabelOffsets = offsets
    baselineLabelOffsets.value = { ...offsets }
    storageError.value = ''
    message.success('布局已保存到当前浏览器')
  } catch (error) {
    storageError.value = `保存布局失败：${error.message}。当前位置未丢失，请重试。`
  }
}

async function restoreLayout() {
  cancelLabelDrag()
  // Newly added services have no saved coordinates; restore their initial placement without overwriting storage.
  flowElements.value = flowElements.value.map((element) => {
    if ('source' in element) return element
    const position = baselinePositions.value.get(element.id) || initialPositions.value.get(element.id)
    return { ...element, position: { ...position } }
  })
  labelOffsets.value = Object.fromEntries(Object.entries(baselineLabelOffsets.value).map(([id, offset]) => [id, { ...offset }]))
  try {
    await fit()
  } catch (error) {
    message.error(`还原布局后适应画布失败：${error.message}`)
  }
}

function restorePositions(positions) {
  const saved = positions.filter((node) => savedPositions.has(node.id))
  if (!saved.length) return positions
  const added = positions.filter((node) => !savedPositions.has(node.id))
  // Place new services beside the saved layout instead of covering manually arranged cards.
  const shift = added.length
    ? Math.max(...saved.map((node) => savedPositions.get(node.id).x + nodeWidth)) + 180 - Math.min(...added.map((node) => node.position.x))
    : 0
  return positions.map((node) => ({ id: node.id, position: savedPositions.has(node.id) ? { ...savedPositions.get(node.id) } : { x: node.position.x + shift, y: node.position.y } }))
}

function updateFullscreenState() {
  isFullscreen.value = ownsFullscreen && viewActive && !disposed && document.fullscreenElement === document.documentElement
  if (document.fullscreenElement !== document.documentElement) ownsFullscreen = false
}

async function exitFullscreen() {
  if (!ownsFullscreen || document.fullscreenElement !== document.documentElement) return
  try {
    await document.exitFullscreen()
    updateFullscreenState()
  } catch (error) {
    message.error(`退出全屏失败：${error.message || '请按 Esc 退出'}`)
  }
}

async function toggleFullscreen() {
  if (fullscreenPending.value) return
  fullscreenPending.value = true
  try {
    if (isFullscreen.value) {
      await exitFullscreen()
    } else {
      if (typeof document.documentElement.requestFullscreen !== 'function') throw new Error('当前浏览器不支持全屏显示')
      if (document.fullscreenElement) throw new Error('请先退出当前全屏，再打开依赖图全屏')
      // Keep body-mounted drawers/modals visible; only the graph is maximized above the page content.
      ownsFullscreen = true
      await document.documentElement.requestFullscreen()
      if (disposed || !viewActive) { await exitFullscreen(); return }
      updateFullscreenState()
    }
  } catch (error) {
    ownsFullscreen = false
    message.error(`进入全屏失败：${error.message || '浏览器拒绝了全屏请求'}`)
  } finally { fullscreenPending.value = false }
}

watch(isFullscreen, async (_, __, onCleanup) => {
  let cancelled = false
  let frame
  onCleanup(() => { cancelled = true; if (frame !== undefined) cancelAnimationFrame(frame) })
  await nextTick()
  if (cancelled) return
  // Wait for the teleported viewport's ResizeObserver before fitting against its new dimensions.
  frame = requestAnimationFrame(() => {
    frame = requestAnimationFrame(() => {
      if (!cancelled) fit().catch((error) => message.error(`全屏画布适配失败：${error.message}`))
    })
  })
})

function edgeGeometry(edge) {
  return edgePlacement(edge).geometry
}

function startLabelDrag(event, edge) {
  if (event.button !== 0 || labelDrag || layoutLoading.value) return
  const zoom = flowInstance.value?.getViewport().zoom
  if (!Number.isFinite(zoom) || zoom <= 0) {
    message.error('画布尚未就绪，请稍后重试拖动标签')
    return
  }
  suppressedLabelClick = ''
  const placed = edgePlacement(edge)
  const original = labelOffsets.value[edge.id]
  const center = { x: placed.label.x + edgeLabelWidth / 2, y: placed.label.y + edgeLabelHeight / 2 }
  const automatic = original ? { x: center.x - original.x, y: center.y - original.y } : placed.geometry
  // Keep the release/click on this label even when the pointer leaves it, rather than selecting another edge.
  event.currentTarget.setPointerCapture(event.pointerId)
  labelDrag = { id: edge.id, element: event.currentTarget, pointerId: event.pointerId, x: event.clientX, y: event.clientY, zoom, center, automatic, original, moved: false }
  window.addEventListener('pointermove', moveLabelDrag)
  window.addEventListener('pointerup', finishLabelDrag)
  window.addEventListener('pointercancel', cancelLabelDrag)
  window.addEventListener('blur', cancelLabelDrag)
}

function moveLabelDrag(event) {
  const drag = labelDrag
  if (!drag || event.pointerId !== drag.pointerId) return
  const dx = event.clientX - drag.x, dy = event.clientY - drag.y
  if (!drag.moved && Math.hypot(dx, dy) < 4) return
  event.preventDefault()
  drag.moved = true
  draggingLabelId.value = drag.id
  const offsets = { ...labelOffsets.value }
  if (dx === 0 && dy === 0) {
    if (drag.original) offsets[drag.id] = drag.original
    else delete offsets[drag.id]
  } else {
    offsets[drag.id] = { x: drag.center.x + dx / drag.zoom - drag.automatic.x, y: drag.center.y + dy / drag.zoom - drag.automatic.y }
  }
  labelOffsets.value = offsets
}

function finishLabelDrag(event) {
  if (!labelDrag || event.pointerId !== labelDrag.pointerId) return
  moveLabelDrag(event)
  if (labelDrag.moved) {
    const id = labelDrag.id
    suppressedLabelClick = id
    if (!baselineLabelOffsets.value[id]) {
      const offsets = { ...labelOffsets.value }
      delete offsets[id]
      const automatic = positionedDependencyEdges(flowNodes.value, renderedRelations.value, offsets).find((edge) => edge.id === id)
      const current = edgePlacements.value.get(id)
      if (automatic.label.x === current.label.x && automatic.label.y === current.label.y) labelOffsets.value = offsets
    }
  }
  stopLabelDrag()
}

function cancelLabelDrag(event) {
  if (event?.pointerId !== undefined && labelDrag && event.pointerId !== labelDrag.pointerId) return
  if (labelDrag?.moved) {
    const offsets = { ...labelOffsets.value }
    if (labelDrag.original) offsets[labelDrag.id] = labelDrag.original
    else delete offsets[labelDrag.id]
    labelOffsets.value = offsets
    suppressedLabelClick = labelDrag.id
  }
  stopLabelDrag()
}

function stopLabelDrag() {
  const drag = labelDrag
  labelDrag = null
  if (drag?.element.hasPointerCapture(drag.pointerId)) drag.element.releasePointerCapture(drag.pointerId)
  draggingLabelId.value = ''
  window.removeEventListener('pointermove', moveLabelDrag)
  window.removeEventListener('pointerup', finishLabelDrag)
  window.removeEventListener('pointercancel', cancelLabelDrag)
  window.removeEventListener('blur', cancelLabelDrag)
}

function clickLabel(event, edge) {
  // Native mouse clicks follow pointerup; keyboard activation (detail=0) must still open details.
  if (suppressedLabelClick === edge.id && event.detail !== 0) {
    suppressedLabelClick = ''
    return
  }
  emit('select-edge', edge.data.record)
}

function labelStyle(edge) {
  const { x, y } = edgePlacement(edge).label
  return { transform: `translate(${x}px, ${y}px)`, width: `${edgeLabelWidth}px`, height: `${edgeLabelHeight}px`, color: strengthColors[edge.data.record.strength] }
}

function edgePlacement(edge) {
  return edgePlacements.value.get(edge.id)
}

async function fit() {
  await nextTick()
  // D3 only resolves an animated fit on transition end, not interruption; initialization can request fit twice.
  if (flowNodes.value.length) await flowInstance.value?.fitBounds(dependencyGraphBounds(flowNodes.value, renderedRelations.value, labelOffsets.value), { padding: 0.2, duration: 0 })
}

async function arrange(force = false) {
  cancelLabelDrag()
  const current = ++generation
  const records = props.nodes
  const relations = props.edges
  const scopeChanged = activeLayoutKey !== props.layoutKey
  if (scopeChanged) {
    activeLayoutKey = props.layoutKey
    readLayout()
  }
  const old = new Map(flowNodes.value.map((node) => [node.id, node.position]))
  const sameNodes = !scopeChanged && records.length === old.size && records.every((node) => old.has(String(node.id)))
  layoutError.value = ''
  layoutLoading.value = true
  try {
    // Snapshot refreshes preserve the user's positions; only new node sets or explicit relayout move them.
    let positions = !force && sameNodes ? [...old].map(([id, position]) => ({ id, position })) : await layoutDependencyNodes(records, relations)
    if (current !== generation) return
    if (force) {
      const offsets = { ...labelOffsets.value }
      for (const edge of relations) delete offsets[edge.id]
      labelOffsets.value = offsets
    }
    if (!force && !sameNodes) positions = restorePositions(positions)
    if (!force && !sameNodes) {
      initialPositions.value = new Map(positions.map((node) => [node.id, { ...node.position }]))
      // Vue Flow mutates coordinates while dragging; the comparison baseline must not share those objects.
      baselinePositions.value = layoutSaved.value
        ? new Map(savedPositions)
        : new Map(positions.map((node) => [node.id, { ...node.position }]))
    }
    const byId = new Map(positions.map((node) => [node.id, node.position]))
    const nodes = records.map((record) => ({ id: String(record.id), type: 'service', position: byId.get(String(record.id)), width: nodeWidth, height: nodeHeight, data: { record } }))
    // Vue Flow's combined model registers nodes before edges; separate prop watchers can drop edges during async layout.
    flowElements.value = [...nodes, ...graphEdges(relations)]
    if (force || !sameNodes) await fit()
  } catch (error) {
    if (current !== generation) return
    flowElements.value = []
    layoutError.value = `自动布局失败：${error.message}`
  } finally {
    if (current === generation) layoutLoading.value = false
  }
}

function exportSvg() {
  try {
    const content = dependencySvg(flowNodes.value, renderedRelations.value, props.title, labelOffsets.value)
    downloadDependencyImage(new Blob([content], { type: 'image/svg+xml;charset=utf-8' }), 'svg')
  } catch (error) {
    message.error(error.message || '导出架构图失败')
  }
}

async function exportPng() {
  imageAction.value = 'png'
  try {
    const content = dependencySvg(flowNodes.value, renderedRelations.value, props.title, labelOffsets.value)
    downloadDependencyImage(await dependencyPng(content), 'png')
  } catch (error) {
    message.error(`导出 PNG 失败：${error.message}`)
  } finally { imageAction.value = '' }
}

async function copyImage() {
  imageAction.value = 'copy'
  try {
    const content = dependencySvg(flowNodes.value, renderedRelations.value, props.title, labelOffsets.value)
    // HTTP cannot write the image clipboard; expose a real PNG for the browser's native context menu.
    if (!window.isSecureContext) {
      const png = await dependencyPng(content)
      if (disposed || !viewActive) return
      closeCopyPreview()
      copyPreviewUrl.value = URL.createObjectURL(png)
      return
    }
    await copyDependencyPng(content)
    message.success('图片已复制，可粘贴到微信或 Word')
  } catch (error) {
    message.error(`复制图片失败：${error.message}`)
  } finally { imageAction.value = '' }
}

function closeCopyPreview() {
  if (copyPreviewUrl.value) URL.revokeObjectURL(copyPreviewUrl.value)
  copyPreviewUrl.value = ''
}

function copyPreviewFailed() {
  closeCopyPreview()
  message.error('图片预览加载失败，请重试或使用导出 PNG')
}

watch(() => [props.nodes, props.edges, props.layoutKey], () => arrange(), { immediate: true })
onMounted(() => document.addEventListener('fullscreenchange', updateFullscreenState))
onActivated(() => { viewActive = true })
onDeactivated(() => { viewActive = false; cancelLabelDrag(); closeCopyPreview(); void exitFullscreen() })
onBeforeUnmount(() => {
  disposed = true
  generation++
  cancelLabelDrag()
  closeCopyPreview()
  document.removeEventListener('fullscreenchange', updateFullscreenState)
  void exitFullscreen()
})
</script>

<style scoped>
.dependency-graph { border: 1px solid #e8edf4; border-radius: 8px; overflow: hidden; }
.dependency-graph--fullscreen { position: fixed; inset: 0; width: 100vw; height: 100vh; box-sizing: border-box; z-index: 999; display: flex; flex-direction: column; border-radius: 0; background: #fff; }
.dependency-graph--fullscreen .graph-controls { flex-shrink: 0; }
.dependency-graph--fullscreen .graph-spin { flex: 1; min-height: 0; width: 100%; }
.dependency-graph--fullscreen .graph-spin :deep(.ant-spin-container) { height: 100%; width: 100%; }
.dependency-graph--fullscreen .graph-canvas { height: 100%; min-height: 0; }
.graph-controls { padding: 12px 16px; display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.graph-legend { color: #78889d; font-size: 12px; }
.copy-preview { margin-top: 16px; max-height: 65vh; overflow: auto; }
.copy-preview img { display: block; max-width: 100%; height: auto; }
.graph-canvas { width: 100%; height: 540px; background: #f8faff; display: grid; align-items: center; }
.service-node { box-sizing: border-box; width: 250px; height: 114px; background: white; border: 1.5px solid; border-radius: 8px; padding: 10px 14px; box-shadow: 0 3px 12px #203c6010; text-align: left; }
.service-node--external { border-style: dashed; }
.node-title { display: flex; align-items: center; gap: 5px; }
.node-title strong { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; flex: 1; }
.node-title :deep(.ant-tag) { font-size: 10px; padding: 0 3px; margin: 0; }
.node-code { color: #8290a4; font-size: 11px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.node-status { font-size: 12px; margin-top: 6px; }
.node-summary { font-size: 10px; color: #8290a4; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.edge-label { position: absolute; box-sizing: border-box; background: #f8faff; border: 1px solid #e2e8f1; border-radius: 4px; padding: 2px 6px; font-size: 11px; line-height: 18px; pointer-events: all; cursor: grab; touch-action: none; user-select: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.edge-label--dragging { cursor: grabbing; }
</style>
