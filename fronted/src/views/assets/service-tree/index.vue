<template>
  <div class="service-tree-page">
    <ServiceTree :selected-scope="serviceScope" @select="serviceScope = $event" />
    <main class="service-tree-content">
      <ServiceTreeNodeContent :scope="serviceScope" @navigate="serviceScope = $event" />
    </main>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import ServiceTree from '../application/components/ServiceTree.vue'
import ServiceTreeNodeContent from './ServiceTreeNodeContent.vue'

const serviceScope = ref({ nodeType: 'all', nodeTitle: '全部业务' })
</script>

<style scoped>
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