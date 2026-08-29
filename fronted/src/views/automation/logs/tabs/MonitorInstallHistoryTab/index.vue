<template>
  <a-card title="监控安装/卸载历史列表" size="small" class="block-card jobs-card">
      <template #extra>
        <a-space>
          <a-input-search
            v-model:value="monitorInstallHistoryTargetId"
            placeholder="按纳管目标ID过滤"
            allow-clear
            style="width: 180px"
            @search="loadMonitorInstallHistories(true)"
          />
          <a-input-search
            v-model:value="monitorInstallHistoryKeyword"
            placeholder="按主机/摘要搜索"
            allow-clear
            style="width: 240px"
            @search="loadMonitorInstallHistories(true)"
          />
          <a-select
            v-model:value="monitorInstallHistoryAction"
            :getPopupContainer="getPopupContainer"
            :options="monitorInstallHistoryActionOptions"
            allow-clear
            placeholder="动作"
            style="width: 120px"
            @change="loadMonitorInstallHistories(true)"
          />
          <a-select
            v-model:value="monitorInstallHistoryStatus"
            :getPopupContainer="getPopupContainer"
            :options="monitorInstallHistoryStatusOptions"
            allow-clear
            placeholder="状态"
            style="width: 140px"
            @change="loadMonitorInstallHistories(true)"
          />
          <a-range-picker
            v-model:value="monitorInstallHistoryTimeRange"
            :show-time="logsTimeRangeShowTime"
            :presets="logsTimeRangePresets"
            format="YYYY-MM-DD HH:mm:ss"
            :placeholder="['开始时间', '结束时间']"
            :getPopupContainer="getPopupContainer"
            @openChange="onLogsTimeRangeOpenChange"
            @change="loadMonitorInstallHistories(true)"
          />
          <a-button type="link" size="small" @click="clearMonitorInstallHistoryFilters">重置</a-button>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="monitorInstallHistoryLoading" @click="reloadPage">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="monitorInstallHistoryLoading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </template>

      <a-table
        :columns="monitorInstallHistoryColumns"
        :data-source="monitorInstallHistoryRows"
        :loading="monitorInstallHistoryLoading"
        :pagination="monitorInstallHistoryPagination"
        :scroll="{ x: 1700 }"
        rowKey="id"
        size="small"
        @change="handleMonitorInstallHistoryTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ record.status || '-' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-tag :color="record.action === 'install' ? 'blue' : 'purple'">
              {{ record.action === 'install' ? '安装' : '卸载' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'create_time'">
            {{ formatDateTime(record.create_time) }}
          </template>
          <template v-else-if="column.key === 'summary_message'">
            <a-tooltip v-if="record.summary_message" :title="record.summary_message" placement="topLeft">
              <a-typography-text
                :content="record.summary_message"
                :ellipsis="{ tooltip: false }"
                style="max-width: 330px"
              />
            </a-tooltip>
            <span v-else>-</span>
          </template>
          <template v-else-if="column.key === 'action_col'">
            <a-space>
              <a-tooltip title="查看日志">
                <a-button size="small" @click="openMonitorHistoryDetail(record)">查看日志</a-button>
              </a-tooltip>
              <a-tooltip v-if="record.status === 'pending' || record.status === 'running'" title="取消">
                <a-button danger size="small" @click="cancelMonitorHistory(record)">取消</a-button>
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
  monitorInstallHistoryRows,
  monitorInstallHistoryLoading,
  monitorInstallHistoryKeyword,
  monitorInstallHistoryStatus,
  monitorInstallHistoryAction,
  monitorInstallHistoryTargetId,
  monitorInstallHistoryTimeRange,
  logsTimeRangeShowTime,
  logsTimeRangePresets,
  onLogsTimeRangeOpenChange,
  monitorInstallHistoryStatusOptions,
  monitorInstallHistoryActionOptions,
  monitorInstallHistoryColumns,
  monitorInstallHistoryPagination,
  loadMonitorInstallHistories,
  clearMonitorInstallHistoryFilters,
  handleMonitorInstallHistoryTableChange,
  statusColor,
  formatDateTime,
  openMonitorHistoryDetail,
  cancelMonitorHistory,
  reloadPage,
} = ctx
</script>
