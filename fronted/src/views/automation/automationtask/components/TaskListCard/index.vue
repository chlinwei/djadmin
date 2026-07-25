<template>
  <div>
    <a-row class="tools" :gutter="12">
      <a-col :span="16">
        <a-input-search
          v-model:value="keywordModel"
          placeholder="搜索任务名称"
          allow-clear
          enter-button
          @search="emitSearch"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-tooltip title="新增">
            <a-button size="large" @click="emitCreate" v-permission="'automation:tasks:create'">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp新增任务</span>
            </a-button>
          </a-tooltip>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="taskLoading || playbookLoading" @click="emitReload">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="taskLoading || playbookLoading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="任务列表" size="small" class="block-card">
      <a-table
        :columns="taskColumns"
        :data-source="tasks"
        :loading="taskLoading"
        :pagination="taskPagination"
        rowKey="id"
        size="small"
        :scroll="{ x: 1700 }"
        @change="emitTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <a-button
              v-if="canEditTask"
              type="link"
              size="small"
              class="task-name-link"
              @click="emitEdit(record)"
            >
              {{ record.name || '-' }}
            </a-button>
            <span v-else>{{ record.name || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'enabled'">
            <a-switch
              :checked="record.enabled === true"
              :disabled="!canEditTask || taskLoading || taskStatusUpdatingId === record.id"
              :loading="taskStatusUpdatingId === record.id"
              @change="(checked) => emitStatusChange(checked, record)"
            />
          </template>
          <template v-else-if="column.key === 'template_name'">
            <a-button type="link" size="small" class="task-code-link" @click="emitGotoTemplate(record)">
              {{ record.template_name || '-' }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'inventory_name'">
            <a-button type="link" size="small" class="task-code-link" @click="emitGotoInventory(record)">
              {{ record.inventory_name || '-' }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'selected_group_ids'">
            <div class="scope-compact-cell">
              <span v-if="!record.inventory" class="scope-limit-empty">未设置 Inventory</span>

              <template v-else>
                <a-tag v-if="record.limit_preview_limit" color="blue" class="scope-limit-tag">
                  {{ record.limit_preview_limit }}
                </a-tag>
                <span v-else class="scope-limit-empty">未设置默认 Limit</span>

                <a-button
                  v-if="Number(record.limit_preview_total || 0) > 0"
                  type="link"
                  size="small"
                  class="scope-host-count-link"
                  @click.stop="emitOpenScopePreview(record)"
                >
                  {{
                    record.limit_preview_limit
                      ? `${Number(record.limit_preview_total || 0)} 台主机`
                      : `Inventory 全量（${Number(record.limit_preview_total || 0)} 台，列表折叠）`
                  }}
                </a-button>

                <span v-else-if="record.limit_preview_limit" class="scope-match-empty">0 台匹配</span>
                <span v-else class="scope-match-empty">Inventory 无主机</span>
              </template>
            </div>
          </template>
          <template v-else-if="column.key === 'env_vars'">
            <a-tooltip :title="formatEnvVarCellFullText(record.env_vars)">
              <div class="json-cell">{{ formatEnvVarCell(record.env_vars) }}</div>
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'update_time'">
            <span>{{ formatUpdateTime(record.update_time, timezone) }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="编辑">
                <a-button size="small" type="primary" @click="emitEdit(record)" v-permission="'automation:tasks:update'">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="运行">
                <a-button
                  size="small"
                  type="primary"
                  ghost
                  :loading="runningTaskId === record.id"
                  :disabled="!record.enabled || runningTaskId === record.id"
                  @click="emitRunNow(record)"
                  v-permission="'automation:jobs:create'"
                >
                  <FontAwesomeIcon :icon="['fas', 'play']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="历史记录">
                <a-button size="small" @click="emitLogs(record)" v-permission="'automation:jobs:view'">
                  历史记录
                  <FontAwesomeIcon :icon="['fas', 'list']" />
                </a-button>
              </a-tooltip>
              <a-tooltip title="删除">
                <a-button
                  class="delBtn"
                  size="small"
                  type="primary"
                  danger
                  v-permission="'automation:tasks:delete'"
                  @click="emitDelete(record)"
                >
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-tooltip>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  taskKeyword: { type: String, default: '' },
  taskLoading: { type: Boolean, default: false },
  playbookLoading: { type: Boolean, default: false },
  taskColumns: { type: Array, required: true },
  tasks: { type: Array, required: true },
  taskPagination: { type: Object, required: true },
  canEditTask: { type: Boolean, default: false },
  taskStatusUpdatingId: { type: Number, default: null },
  runningTaskId: { type: Number, default: null },
  timezone: { type: String, default: 'Asia/Shanghai' },
  formatEnvVarCell: { type: Function, required: true },
  formatEnvVarCellFullText: { type: Function, required: true },
  formatUpdateTime: { type: Function, required: true },
})

const emit = defineEmits([
  'update:taskKeyword',
  'search',
  'create',
  'reload',
  'table-change',
  'edit',
  'status-change',
  'goto-template',
  'goto-inventory',
  'open-scope-preview',
  'run-now',
  'logs',
  'delete',
])

const keywordModel = computed({
  get: () => props.taskKeyword,
  set: (value) => emit('update:taskKeyword', value),
})

function emitSearch() {
  emit('search')
}

function emitCreate() {
  emit('create')
}

function emitReload() {
  emit('reload')
}

function emitTableChange(...args) {
  emit('table-change', ...args)
}

function emitEdit(record) {
  emit('edit', record)
}

function emitStatusChange(checked, record) {
  emit('status-change', checked, record)
}

function emitGotoTemplate(record) {
  emit('goto-template', record)
}

function emitGotoInventory(record) {
  emit('goto-inventory', record)
}

function emitOpenScopePreview(record) {
  emit('open-scope-preview', record)
}

function emitRunNow(record) {
  emit('run-now', record)
}

function emitLogs(record) {
  emit('logs', record)
}

function emitDelete(record) {
  emit('delete', record)
}
</script>
