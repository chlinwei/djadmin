<template>
  <a-card title="任务运行记录列表" size="small" class="block-card jobs-card">
      <template #extra>
        <a-space>
          <a-input-search
            v-model:value="jobRecordId"
            placeholder="按执行记录ID搜索"
            allow-clear
            @search="onJobRecordIdSearch"
          />
          <a-input-search
            v-model:value="jobKeyword"
            placeholder="按发起人搜索"
            allow-clear
            @search="loadJobs(true)"
          />
          <a-select
            v-model:value="selectedJobStatus"
            :getPopupContainer="getPopupContainer"
            :options="jobStatusOptions"
            allow-clear
            placeholder="按任务状态过滤"
            style="width: 180px"
            @change="loadJobs(true)"
          />
          <a-input-search
            v-model:value="jobOutputKeyword"
            placeholder="按日志过滤"
            allow-clear
            @search="loadJobs(true)"
          />
          <a-select
            v-model:value="selectedTaskId"
            :getPopupContainer="getPopupContainer"
            :options="taskOptions"
            allow-clear
            show-search
            optionFilterProp="label"
            placeholder="按任务过滤"
            style="width: 260px"
            @change="onTaskFilterChange"
          />
          <a-range-picker
            v-model:value="jobTimeRange"
            :show-time="logsTimeRangeShowTime"
            :presets="logsTimeRangePresets"
            format="YYYY-MM-DD HH:mm:ss"
            :placeholder="['开始时间', '结束时间']"
            :getPopupContainer="getPopupContainer"
            @openChange="onLogsTimeRangeOpenChange"
            @change="loadJobs(true)"
          />
          <a-tag v-if="selectedTaskName" color="blue">任务: {{ selectedTaskName }}</a-tag>
          <a-tooltip v-if="selectedTaskId" title="清除筛选">
            <a-button type="link" @click="clearTaskFilter">清除筛选</a-button>
          </a-tooltip>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="jobLoading" @click="reloadPage">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="jobLoading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </template>
      <a-table
        :columns="jobColumns"
        :data-source="jobs"
        :loading="jobLoading"
        :pagination="jobPagination"
        :scroll="{ x: 1500 }"
        rowKey="id"
        size="small"
        @change="handleJobTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">
              {{ record.status }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'start_time'">
            {{ formatDateTime(record.start_time) }}
          </template>
          <template v-else-if="column.key === 'duration_seconds'">
            {{ record.duration_seconds ? `${record.duration_seconds.toFixed(2)}s` : '-' }}
          </template>
          <template v-else-if="column.key === 'runtime_template'">
            <a-button type="link" size="small" class="runtime-template-link" @click="openRuntimeTemplateViewer(record)">
              {{ formatRuntimeTemplateLabel(record) }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'inventory_hosts'">
            <a-space v-if="getInventoryHostList(record).length > 0" size="small">
              <span>{{ getInventoryHostList(record).length }}台主机</span>
              <a-button type="link" size="small" class="job-host-list-preview" @click="openJobHostViewer(record)">查看</a-button>
            </a-space>
            <span v-else>0台主机</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-tooltip title="查看日志">
                <a-button size="small" @click="openJobLogViewer(record)" v-permission="'automation:jobs:view'">
                  查看日志
                </a-button>
              </a-tooltip>
              <a-tooltip v-if="canDownloadJobLog(record)" title="下载日志">
                <a-button
                  size="small"
                  :loading="downloadingJobLogId === record.id"
                  @click="downloadJobLog(record)"
                  v-permission="'automation:jobs:view'"
                >
                  下载日志
                </a-button>
              </a-tooltip>
              <a-tooltip v-if="record.status === 'pending' || record.status === 'running'" title="取消">
                <a-button
                  size="small"
                  danger
                  @click="onCancelJob(record)"
                  v-permission="'automation:jobs:cancel'"
                >
                  取消
                </a-button>
              </a-tooltip>
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
  jobRecordId,
  onJobRecordIdSearch,
  jobKeyword,
  loadJobs,
  selectedJobStatus,
  jobStatusOptions,
  jobOutputKeyword,
  selectedTaskId,
  taskOptions,
  onTaskFilterChange,
  jobTimeRange,
  logsTimeRangeShowTime,
  logsTimeRangePresets,
  onLogsTimeRangeOpenChange,
  selectedTaskName,
  clearTaskFilter,
  jobLoading,
  reloadPage,
  jobColumns,
  jobs,
  jobPagination,
  handleJobTableChange,
  statusColor,
  formatDateTime,
  formatRuntimeTemplateLabel,
  openRuntimeTemplateViewer,
  getInventoryHostList,
  openJobHostViewer,
  openJobLogViewer,
  canDownloadJobLog,
  downloadingJobLogId,
  downloadJobLog,
  onCancelJob,
} = ctx
</script>
