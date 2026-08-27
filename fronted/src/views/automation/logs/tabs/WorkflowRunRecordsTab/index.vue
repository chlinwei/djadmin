<template>
  <a-card title="Workflow运行记录列表" size="small" class="block-card jobs-card">
      <template #extra>
        <a-space>
          <a-input-search
            v-model:value="workflowRunKeyword"
            placeholder="Workflow 名称 / 触发人"
            allow-clear
            @search="loadWorkflowRuns(true)"
          />
          <a-select
            v-model:value="workflowRunStatus"
            :getPopupContainer="getPopupContainer"
            :options="workflowRunStatusOptions"
            allow-clear
            placeholder="运行状态"
            style="width: 140px"
            @change="loadWorkflowRuns(true)"
          />
          <a-range-picker
            v-model:value="workflowRunTimeRange"
            :show-time="logsTimeRangeShowTime"
            :presets="logsTimeRangePresets"
            format="YYYY-MM-DD HH:mm:ss"
            :placeholder="['开始时间', '结束时间']"
            style="width: 320px"
            :getPopupContainer="getPopupContainer"
            @openChange="onLogsTimeRangeOpenChange"
            @change="loadWorkflowRuns(true)"
          />
          <a-button type="link" size="small" :disabled="!hasWorkflowRunFilters" @click="resetWorkflowRunFilters">重置</a-button>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="workflowRunLoading" @click="reloadPage">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="workflowRunLoading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </template>

      <a-table
        :columns="workflowRunColumns"
        :data-source="workflowRuns"
        :loading="workflowRunLoading"
        :pagination="workflowRunPagination"
        :scroll="{ x: 1400 }"
        rowKey="id"
        size="small"
        @change="handleWorkflowRunTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="getWorkflowRunStatusColor(record.status)">{{ record.status || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'start_time'">
            <span>{{ formatDateTime(record.start_time) }}</span>
          </template>
          <template v-else-if="column.key === 'duration_seconds'">
            <span>{{ formatWorkflowDuration(record.duration_seconds) }}</span>
          </template>
          <template v-else-if="column.key === 'inventory_hosts'">
            <a-space v-if="getWorkflowRunHostList(record).length > 0" size="small">
              <span>{{ getWorkflowRunHostList(record).length }}台主机</span>
              <a-button type="link" size="small" class="job-host-list-preview" @click="openWorkflowRunHostViewer(record)">查看</a-button>
            </a-space>
            <span v-else>0台主机</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="查看状态图">
                <a-button size="small" type="primary" ghost @click="openWorkflowRunStatus(record)">
                  查看状态图
                </a-button>
              </a-tooltip>
              <a-popconfirm
                v-if="canCancelWorkflowRunRecord(record)"
                title="确认取消该 Workflow 运行吗？"
                ok-text="确认"
                cancel-text="取消"
                @confirm="cancelWorkflowRunRecord(record)"
              >
                <a-tooltip title="取消运行">
                  <a-button
                    size="small"
                    danger
                    :loading="workflowRunCancelingId === record.id"
                    v-permission="'automation:jobs:cancel'"
                  >
                    取消运行
                  </a-button>
                </a-tooltip>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
  </a-card>
</template>

<script setup>
import { inject } from 'vue'

const ctx = inject('automationLogsCtx')
if (!ctx) {
  throw new Error('automationLogsCtx is required')
}

const {
  getPopupContainer,
  workflowRunKeyword,
  loadWorkflowRuns,
  workflowRunStatus,
  workflowRunStatusOptions,
  workflowRunTimeRange,
  logsTimeRangeShowTime,
  logsTimeRangePresets,
  onLogsTimeRangeOpenChange,
  hasWorkflowRunFilters,
  resetWorkflowRunFilters,
  workflowRunLoading,
  reloadPage,
  workflowRunColumns,
  workflowRuns,
  workflowRunPagination,
  handleWorkflowRunTableChange,
  getWorkflowRunStatusColor,
  formatDateTime,
  formatWorkflowDuration,
  getWorkflowRunHostList,
  openWorkflowRunHostViewer,
  openWorkflowRunStatus,
  canCancelWorkflowRunRecord,
  cancelWorkflowRunRecord,
  workflowRunCancelingId,
} = ctx
</script>
