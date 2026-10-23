<template>
  <div ref="pageRef" class="tree-panel-layout">
    <aside class="tree-panel" :class="{ 'tree-panel--collapsed': treeCollapsed }" :style="{ '--tree-panel-width': `${displayedTreeWidth}px` }">
      <div class="tree-panel-tools">
        <span v-if="!treeCollapsed">树面板</span>
        <a-tooltip v-if="!treeCollapsed" :title="treeEnlarged ? '还原宽度' : '放大面板'" placement="top">
          <a-button size="small" :aria-label="treeEnlarged ? '还原宽度' : '放大面板'" :aria-pressed="treeEnlarged" @click="treeEnlarged = !treeEnlarged">
            <FullscreenExitOutlined v-if="treeEnlarged" />
            <FullscreenOutlined v-else />
          </a-button>
        </a-tooltip>
        <a-tooltip :title="treeCollapsed ? '展开树面板' : '收起树面板'" placement="top">
          <a-button size="small" :aria-label="treeCollapsed ? '展开树面板' : '收起树面板'" :aria-expanded="!treeCollapsed" :aria-controls="navigationId" @click="treeCollapsed = !treeCollapsed">
            <MenuUnfoldOutlined v-if="treeCollapsed" />
            <MenuFoldOutlined v-else />
          </a-button>
        </a-tooltip>
      </div>
      <!-- 隐藏而不卸载，保留树内的搜索、筛选和节点展开状态。 -->
      <div :id="navigationId" v-show="!treeCollapsed">
        <slot name="tree" />
      </div>
    </aside>
    <div
      v-if="!treeCollapsed"
      class="tree-panel-divider"
      role="separator"
      tabindex="0"
      aria-label="调整服务树宽度"
      aria-orientation="vertical"
      :aria-valuemin="MIN_TREE_WIDTH"
      :aria-valuemax="maxTreeWidth"
      :aria-valuenow="displayedTreeWidth"
      @pointerdown="startTreeResize"
      @pointermove="moveTreeResize"
      @pointerup="stopTreeResize"
      @pointercancel="stopTreeResize"
      @lostpointercapture="stopTreeResize"
      @keydown.left.prevent="adjustTreeWidth(-20)"
      @keydown.right.prevent="adjustTreeWidth(20)"
      @keydown.home.prevent="setTreeWidth(MIN_TREE_WIDTH)"
      @keydown.end.prevent="setTreeWidth(maxTreeWidth)"
    />
    <div ref="contentRef" class="tree-panel-content">
      <slot />
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { FullscreenExitOutlined, FullscreenOutlined, MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons-vue'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'

defineProps({ navigationId: { type: String, required: true } })
const emit = defineEmits(['resize'])
const MIN_TREE_WIDTH = 260
const pageRef = ref(null)
const contentRef = ref(null)
const treeWidth = ref(MIN_TREE_WIDTH)
const treeCollapsed = ref(false)
const treeEnlarged = ref(false)
const pageWidth = ref(0)
const maxTreeWidth = computed(() => Math.max(MIN_TREE_WIDTH, Math.min(640, pageWidth.value - 376)))
const displayedTreeWidth = computed(() => treeEnlarged.value ? maxTreeWidth.value : Math.min(treeWidth.value, maxTreeWidth.value))
let panelObserver = null
let resizeTarget = null
let resizePointerId = null
let resizeStartX = 0
let resizeStartWidth = 0

function setTreeWidth(width) {
  treeEnlarged.value = false
  treeWidth.value = Math.max(MIN_TREE_WIDTH, Math.min(width, maxTreeWidth.value))
}

function adjustTreeWidth(delta) {
  setTreeWidth(displayedTreeWidth.value + delta)
}

function startTreeResize(event) {
  if (event.button !== 0 || resizeTarget) return
  event.preventDefault()
  resizeStartX = event.clientX
  resizeStartWidth = displayedTreeWidth.value
  resizeTarget = event.currentTarget
  resizePointerId = event.pointerId
  resizeTarget.setPointerCapture(resizePointerId)
}

function moveTreeResize(event) {
  if (!resizeTarget || event.pointerId !== resizePointerId) return
  setTreeWidth(resizeStartWidth + event.clientX - resizeStartX)
}

function stopTreeResize() {
  const target = resizeTarget
  const pointerId = resizePointerId
  resizeTarget = null
  resizePointerId = null
  if (target?.hasPointerCapture(pointerId)) target.releasePointerCapture(pointerId)
}

useKeepAliveRefreshLifecycle(undefined, stopTreeResize)

onMounted(() => {
  pageWidth.value = pageRef.value.clientWidth
  // 两页共用尺寸通知，图表也能响应侧栏和树面板调整，而不只响应窗口缩放。
  panelObserver = new ResizeObserver(() => {
    pageWidth.value = pageRef.value.clientWidth
    emit('resize')
  })
  panelObserver.observe(pageRef.value)
  panelObserver.observe(contentRef.value)
})

onBeforeUnmount(() => {
  stopTreeResize()
  panelObserver?.disconnect()
})
</script>

<style scoped>
.tree-panel-layout {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
}
.tree-panel {
  flex: 0 0 var(--tree-panel-width);
  width: var(--tree-panel-width);
  min-width: 0;
}
.tree-panel--collapsed { flex-basis: 36px; width: 36px; }
.tree-panel-tools {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}
.tree-panel-tools > span:first-child { flex: 1; color: #687386; white-space: nowrap; }
.tree-panel-tools .ant-btn { display: inline-flex; align-items: center; justify-content: center; }
.tree-panel :deep(.service-tree) {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}
.tree-panel-divider {
  align-self: stretch;
  flex: 0 0 6px;
  min-height: 320px;
  cursor: col-resize;
  touch-action: none;
  user-select: none;
  border-radius: 3px;
  background: #edf1f6;
}
.tree-panel-divider:hover, .tree-panel-divider:focus-visible { background: #91caff; }
.tree-panel-content { flex: 1; min-width: 0; }
@media (max-width: 900px) {
  .tree-panel-layout { flex-direction: column; }
  .tree-panel { flex: none; width: 100%; }
  .tree-panel-divider { display: none; }
  .tree-panel :deep(.service-tree) {
    height: auto;
    min-height: 0;
    max-height: 320px;
  }
  .tree-panel-content { width: 100%; }
}
</style>
