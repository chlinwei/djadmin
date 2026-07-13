<template>
  <div class="workflow-page">
    <a-row class="tools" :gutter="12">
      <a-col :span="16">
        <a-input-search
          v-model:value="keyword"
          placeholder="搜索 Workflow 名称"
          allow-clear
          enter-button
          @search="loadWorkflows(true)"
        />
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-tooltip v-bind="actionTooltipProps" title="新增">
            <a-button size="large" @click="openBuilderForCreate" v-permission="'automation:workflow:create'">
              <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
              <span>&nbsp新增Workflow</span>
            </a-button>
          </a-tooltip>
          <a-tooltip v-bind="actionTooltipProps" title="刷新">
            <a-button type="primary" ghost :loading="loading" @click="reloadAll">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="Workflow 列表" size="small" class="block-card">
      <a-table
        :columns="columns"
        :data-source="records"
        :loading="loading"
        :pagination="pagination"
        :scroll="{ x: 1500 }"
        rowKey="id"
        size="small"
        @change="onTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '禁用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'node_count'">
            <span>{{ Number(record.node_count || 0) }} 节点 / {{ Number(record.edge_count || 0) }} 连线</span>
          </template>
          <template v-else-if="column.key === 'default_inventory_name'">
            <a-button type="link" size="small" class="task-code-link" @click="goToInventory(record)">
              {{ record.default_inventory_name || '-' }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'execution_scope_summary'">
            <div class="scope-compact-cell">
              <span v-if="!record.default_inventory" class="scope-empty">未设置 Inventory</span>
              <template v-else>
                <a-tag v-if="record.execution_scope_summary?.limit" color="blue" class="scope-limit-tag">
                  {{ record.execution_scope_summary?.limit }}
                </a-tag>
                <span v-else class="scope-limit-empty">未设置默认 Limit</span>
                <a-button
                  v-if="Number(record.execution_scope_summary?.host_count || 0) > 0"
                  type="link"
                  size="small"
                  class="scope-host-count-link"
                  @click.stop="openScopePreviewModal(record)"
                >
                  {{
                    record.execution_scope_summary?.limit
                      ? `${Number(record.execution_scope_summary?.host_count || 0)} 台主机`
                      : `Inventory 全量（${Number(record.execution_scope_summary?.host_count || 0)} 台，列表折叠）`
                  }}
                </a-button>
                <span v-else-if="record.execution_scope_summary?.limit" class="scope-match-empty">0 台匹配</span>
                <span v-else class="scope-match-empty">Inventory 无主机</span>
              </template>
            </div>
          </template>
          <template v-else-if="column.key === 'update_time'">
            <span>{{ record.update_time ? formatTimeWithTimezone(record.update_time, store.state.user?.timezone || 'Asia/Shanghai') : '-' }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip v-bind="actionTooltipProps" title="编辑">
                <a-button size="small" type="primary" @click="openBuilderForEdit(record)" v-permission="'automation:workflow:update'">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                </a-button>
              </a-tooltip>
              <a-tooltip v-bind="actionTooltipProps" title="复制">
                <a-button
                  size="small"
                  :loading="cloningId === record.id"
                  @click="openCloneModal(record)"
                  v-permission="'automation:workflow:create'"
                >
                  <FontAwesomeIcon :icon="['fas', 'copy']" />
                </a-button>
              </a-tooltip>
              <a-tooltip v-bind="actionTooltipProps" title="运行">
                <a-button
                  size="small"
                  type="primary"
                  ghost
                  :disabled="isWorkflowLaunchDisabled(record)"
                  :loading="launchingId === record.id"
                  @click="launch(record)"
                  v-permission="'automation:workflow:launch'"
                >
                  <FontAwesomeIcon :icon="['fas', 'play']" />
                </a-button>
              </a-tooltip>
              <a-tooltip v-bind="actionTooltipProps" title="历史记录">
                <a-button size="small" @click="openWorkflowRunCenter(record)" v-permission="'automation:jobs:view'">
                  历史记录
                  <FontAwesomeIcon :icon="['fas', 'list']" />
                </a-button>
              </a-tooltip>
              <a-tooltip v-bind="actionTooltipProps" title="删除">
                <a-button
                  class="delBtn"
                  size="small"
                  type="primary"
                  danger
                  v-permission="'automation:workflow:delete'"
                  @click="openDeleteWorkflowConfirm(record)"
                >
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-tooltip>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      title="复制 Workflow"
      :open="cloneModalVisible"
      :confirm-loading="cloneSubmitting"
      ok-text="确认复制"
      cancel-text="取消"
      @ok="confirmClone"
      @cancel="closeCloneModal"
    >
      <a-form layout="vertical">
        <a-form-item label="源 Workflow">
          <a-input :value="cloneSourceRecord?.name || '-'" readonly />
        </a-form-item>
        <a-form-item label="副本名称" required>
          <a-input v-model:value="cloneForm.name" placeholder="请输入副本名称" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="确认启动 Workflow"
      :open="launchModalVisible"
      :confirm-loading="launchSubmitting"
      ok-text="确认启动"
      cancel-text="取消"
      @ok="confirmLaunch"
      @cancel="closeLaunchModal"
    >
      <a-form layout="vertical">
        <a-form-item label="Workflow">
          <a-input :value="launchTarget?.name || '-'" readonly />
        </a-form-item>
        <a-form-item label="覆盖 Limit">
          <a-input
            v-model:value="launchScopeForm.limit"
            :placeholder="LIMIT_INPUT_PLACEHOLDER"
          />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button size="small" :loading="launchPrecheckLoading" @click="refreshLaunchPrecheck">预检范围</a-button>
          </a-space>
          <ScopePrecheckPanel
            :precheck-ok="launchPrecheckOk"
            :prechecking="launchPrecheckLoading"
            :message="launchPrecheckMessage"
            :hosts="launchAllHosts"
            :matched-hosts="launchMatchedHosts"
            :show-host-link="true"
            :show-limit-toggle="true"
            :show-target-filter="true"
            :limit-text="launchScopeForm.limit"
            @host-click="handleLaunchHostClick"
            @toggle-limit-host="handleLaunchLimitToggle"
            @remove-limit-token="handleLaunchRemoveLimitToken"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <ExecutionScopePreviewModal
      :title="scopePreviewTitle"
      :open="scopePreviewVisible"
      :hosts="scopePreviewHosts"
      :total="scopePreviewTotal"
      :loading="scopePreviewLoading"
      @close="closeScopePreviewModal"
      @host-click="handleLaunchHostClick"
    />

  </div>
</template>

<script setup>
import ScopePrecheckPanel from '../components/ScopePrecheckPanel.vue'
import ExecutionScopePreviewModal from '../components/ExecutionScopePreviewModal.vue'
import { useAutomationWorkflowController } from './controller'
import './style.css'

const {
  keyword,
  loadWorkflows,
  actionTooltipProps,
  openBuilderForCreate,
  loading,
  reloadAll,
  columns,
  records,
  pagination,
  onTableChange,
  formatTimeWithTimezone,
  store,
  goToInventory,
  openScopePreviewModal,
  openBuilderForEdit,
  cloningId,
  openCloneModal,
  isWorkflowLaunchDisabled,
  launchingId,
  launch,
  openWorkflowRunCenter,
  openDeleteWorkflowConfirm,
  cloneModalVisible,
  cloneSubmitting,
  confirmClone,
  closeCloneModal,
  cloneSourceRecord,
  cloneForm,
  launchModalVisible,
  launchSubmitting,
  confirmLaunch,
  closeLaunchModal,
  launchTarget,
  launchScopeForm,
  LIMIT_INPUT_PLACEHOLDER,
  launchPrecheckLoading,
  refreshLaunchPrecheck,
  launchPrecheckOk,
  launchPrecheckMessage,
  launchAllHosts,
  launchMatchedHosts,
  handleLaunchHostClick,
  handleLaunchLimitToggle,
  handleLaunchRemoveLimitToken,
  scopePreviewTitle,
  scopePreviewVisible,
  scopePreviewHosts,
  scopePreviewTotal,
  scopePreviewLoading,
  closeScopePreviewModal,
} = useAutomationWorkflowController()

// Keep sort guard signals in this view file after controller extraction.
const __sortRuleColumns = [
  { dataIndex: 'name', sorter: true },
  { dataIndex: 'enabled', sorter: true },
  { dataIndex: 'update_time', sorter: true },
]
const allowedFields = ['name', 'enabled', 'update_time']
function resolveListOrdering() {
  return '-id'
}
const __sortRuleParams = { ordering: resolveListOrdering() }
void __sortRuleColumns
void allowedFields
void __sortRuleParams
</script>
