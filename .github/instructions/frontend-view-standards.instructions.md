---
description: "Auto-apply Vue 3 + Ant Design conventions to all frontend view components. Enforces tooltip consistency, button styling uniformity, table operation column layout, delete interaction patterns, timezone handling, and popup container usage."
name: "Frontend View Component Standards"
applyTo: "fronted/src/views/**/*.vue"
---

# Frontend View Component Standards

When reviewing or editing Vue 3 components in `fronted/src/views/`, apply these standards automatically.

## Tooltip Requirements
Every operation button (edit, delete, run, etc.) in action columns must be wrapped in `<a-tooltip>` with a standardized action word from this list:
- 编辑 (edit)
- 运行 (run)
- 删除 (delete)
- 历史记录 (history)
- 详细日志 (detailed log)
- 查看日志 (view log)
- 下载日志 (download log)
- 取消 (cancel)
- 查看状态图 (view status graph)

**Pattern**:
```vue
<a-tooltip title="编辑">
  <a-button ... @click="handleEdit">...</a-button>
</a-tooltip>
```

## Button Styling Consistency
All buttons performing the same action across pages must use identical icon and style:
- **Add buttons**: Use `plus-circle` icon + default (primary) button style + text label
- **Edit buttons**: Use consistent icon style across all pages
- **Delete buttons**: Use `delete` icon + danger style + class `delBtn` (prevents width jitter)

Reference baseline: `fronted/src/views/assets/credential/index.vue` (add button pattern)

## Table Operation Column Layout
**Required for all tables with action columns**:
1. Operation column must have `fixed: 'right'` and explicit `width`
2. Parent `a-table` must have `:scroll="{ x: number }"` configured for horizontal scrolling
3. Verify main content can scroll without crushing operation buttons

Example:
```vue
<a-table
  :scroll="{ x: 1200 }"
  :columns="columns"
  ...
>
  <!-- Columns include: -->
  <!-- { title: 'Action', key: 'action', fixed: 'right', width: 200, ... } -->
</a-table>
```

## Delete Interaction Pattern
All delete operations (single or batch) must:
1. Use the centralized utility: `import { openDeleteConfirm } from '@/util/deleteConfirm'`
2. Apply class `delBtn` to the delete button (prevents width change during loading)
3. Never create custom `a-popconfirm` delete flows
4. Display the affected items/IDs in the confirmation dialog

```vue
<a-button class="delBtn" danger @click="handleDelete">删除</a-button>

// In handler:
openDeleteConfirm({
  title: '删除',
  content: `确认删除 ${selectedCount} 项?`,
  items: selectedItems.map(i => i.name),
  onOk: async () => { /* delete logic */ }
})
```

## Popup Container for a-select
All `<a-select>` components must have `:getPopupContainer` bound to handle dropdown positioning in modals and scrollable contexts:

```vue
<template>
  <a-select :getPopupContainer="getPopupContainer" ... />
</template>

<script setup>
import { resolvePopupContainerByContext } from '@/util/popupContainer'
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
</script>
```

## Timezone-Aware Time Display
**All time fields must use user timezone**, never hardcoded locale:

```vue
<script setup>
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'

const timezone = store.state.user?.timezone || 'Asia/Shanghai'
</script>

<template>
  <!-- ❌ WRONG: -->
  <!-- <span>{{ new Date(time).toLocaleString('zh-CN') }}</span> -->
  
  <!-- ✅ CORRECT: -->
  <span>{{ formatTimeWithTimezone(time, timezone) }}</span>
</template>
```

## Time Range Picker Defaults
For `<a-range-picker show-time>` components:
- Default start time: `00:00:00`
- Default end time: `23:59:59`
- On submit, normalize date range to natural day boundaries

```vue
<a-range-picker
  v-model:value="dateRange"
  show-time
  :value-format="'YYYY-MM-DD HH:mm:ss'"
  placeholder="['开始日期', '结束日期']"
  @change="handleRangeChange"
/>

function handleRangeChange(dates) {
  if (dates && dates.length === 2) {
    // Ensure start is T00:00:00 and end is T23:59:59
    dateRange.value = [
      dates[0].startOf('day'),
      dates[1].endOf('day')
    ]
  }
}
```

## Avoid Element Plus
Do not introduce Element Plus components in new or modified views. djadmin uses Ant Design Vue exclusively.
- ❌ `<el-button>`, `<el-select>`, `<el-table>`, etc.
- ✅ `<a-button>`, `<a-select>`, `<a-table>`, etc.

## Review Checklist
When editing a view component, verify:
- [ ] All operation buttons have `<a-tooltip>` with standardized action word
- [ ] Add/edit/delete buttons match style pattern across pages
- [ ] Operation column has `fixed: 'right'` and `width`
- [ ] Parent table has `:scroll="{ x: ... }"`
- [ ] Delete buttons use `delBtn` class and `openDeleteConfirm` utility
- [ ] All `<a-select>` have `:getPopupContainer` binding
- [ ] Time display uses `formatTimeWithTimezone`, not `toLocaleString()`
- [ ] For `a-range-picker show-time`, defaults and normalization are correct
- [ ] No Element Plus components used
