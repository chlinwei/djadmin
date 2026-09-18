<template>
  <div class="host-target-panel">
    <div class="host-target-panel__batch-bar">
      <div class="host-target-panel__batch-head">
        <span class="host-target-panel__count">已选 {{ selectedCount }} 台主机</span>
        <slot name="batch-actions" />
      </div>
      <slot name="batch-hint" />
    </div>

    <div class="host-target-panel__layout">
      <div class="host-target-panel__tree">
        <a-input
          :value="groupKeyword"
          allow-clear
          size="small"
          placeholder="搜索分组"
          class="host-target-panel__tree-search"
          @update:value="$emit('update:groupKeyword', $event)"
        />
        <div class="host-target-panel__tree-body">
          <a-tree
            block-node
            :tree-data="groupTreeData"
            :selected-keys="selectedGroupKeys"
            :expanded-keys="groupExpandedKeys"
            :auto-expand-parent="true"
            @select="$emit('group-select', $event)"
            @expand="$emit('update:groupExpandedKeys', $event)"
          />
        </div>
      </div>

      <div class="host-target-panel__table">
        <div class="host-target-panel__filters">
          <a-input-search
            :value="keyword"
            allow-clear
            size="small"
            placeholder="搜索主机名 / IP"
            style="width: 200px"
            @update:value="$emit('update:keyword', $event)"
            @search="$emit('reload')"
          />
          <slot name="filters" />
        </div>
        <a-table
          row-key="host_id"
          :columns="columns"
          :data-source="rows"
          :loading="loading"
          :row-selection="rowSelection"
          size="small"
          :scroll="{ x: scrollX }"
          :locale="tableLocale"
          :pagination="pagination"
          @change="$emit('table-change', $event)"
        >
          <template #bodyCell="cell">
            <slot name="cell" v-bind="cell" />
          </template>
        </a-table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { tableLocale } from '@/util/tableStyle'

// 「纳管目标」主机表的共享外壳：左侧主机分组树 + 右侧筛选条/表格 + 顶部批量操作栏。
//
// 两个页面共用它（views/monitor/index.vue 的 Exporter 目标、views/monitor/log-collectors/index.vue
// 的日志采集），因为二者消费的是**同一份主机视角数据**（GET /monitor/targets/host-overview/）。
// 状态与加载逻辑在 @/util/hostTargetTable.js，本组件只管外壳与事件转发，不持有业务状态。
defineProps({
  rows: { type: Array, default: () => [] },
  columns: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  scrollX: { type: Number, default: undefined },
  pagination: { type: Object, required: true },
  rowSelection: { type: Object, default: () => ({}) },
  selectedCount: { type: Number, default: 0 },
  keyword: { type: String, default: '' },
  groupKeyword: { type: String, default: '' },
  groupTreeData: { type: Array, default: () => [] },
  selectedGroupKeys: { type: Array, default: () => [] },
  groupExpandedKeys: { type: Array, default: () => [] },
})

defineEmits([
  'update:keyword', 'update:groupKeyword', 'update:groupExpandedKeys',
  'reload', 'group-select', 'table-change',
])
</script>

<style scoped>
.host-target-panel__batch-bar {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 12px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}

.host-target-panel__batch-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.host-target-panel__count {
  color: #666;
  font-size: 13px;
  white-space: nowrap;
}

.host-target-panel__layout {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.host-target-panel__tree {
  flex: 0 0 200px;
  width: 200px;
  padding: 8px;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}

.host-target-panel__tree-search {
  margin-bottom: 8px;
}

.host-target-panel__tree-body {
  max-height: 520px;
  overflow: auto;
}

/* 表格区必须能收缩，否则 flex 子项默认 min-width:auto 会被宽表格撑破布局。 */
.host-target-panel__table {
  flex: 1;
  min-width: 0;
}

.host-target-panel__filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 8px;
}
</style>
