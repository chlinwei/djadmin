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
      <ServiceTree ref="serviceTreeRef" :selected-scope="serviceScope" @select="serviceScope = $event" />
      <main class="service-tree-content">
        <ServiceTreeNodeContent ref="nodeContentRef" :scope="serviceScope" @navigate="serviceScope = $event" />
      </main>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import ServiceTree from '../application/components/ServiceTree.vue'
import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'

const serviceScope = ref({ nodeType: 'all', nodeTitle: '全部业务' })
const serviceTreeRef = ref(null)
const nodeContentRef = ref(null)
const refreshing = ref(false)

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
@media (max-width: 900px) {
  .service-tree-page { flex-direction: column; }
  .service-tree-page :deep(.service-tree) {
    width: 100%;
    height: auto;
    min-height: 0;
    max-height: 320px;
  }
  .service-tree-content { width: 100%; }
}
</style>