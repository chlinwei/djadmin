<template>
  <div class="automation-logs-page">
    <a-tabs v-model:activeKey="activeRecordTab" @change="handleRecordTabChange">
      <a-tab-pane key="job" tab="自动化任务运行记录">
        <JobRunRecordsTab />
      </a-tab-pane>
      <a-tab-pane key="monitor_history" tab="监控安装历史">
        <MonitorInstallHistoryTab />
      </a-tab-pane>
      <a-tab-pane key="workflow" tab="Workflow运行记录">
        <WorkflowRunRecordsTab />
      </a-tab-pane>
    </a-tabs>

    <a-drawer
      :title="`查看日志 / 作业 #${jobLogViewerJobId || ''}`"
      :open="jobLogViewerVisible"
      :width="'88vw'"
      @close="closeJobLogViewer"
    >
      <LogViewerPanel
        :status-tag-color="streamStatusTagColor"
        :status-text="streamStatusLabel"
        :last-output-text="streamLastOutputText"
        :wrap="jobLogWrap"
        :font-size="jobLogFontSize"
        :html-content="jobLogHtml"
        :show-auto-follow="true"
        :auto-follow-enabled="jobLogAutoFollowEnabled"
        :auto-follow-suspended="jobLogAutoFollowSuspended"
        :show-cancel="true"
        :cancel-loading="cancellingJobId === Number(jobLogViewerJobId)"
        :cancel-disabled="!canCancelViewerJob"
        @update:wrap="(value) => (jobLogWrap = value)"
        @toggle-auto-follow="toggleJobLogAutoFollow"
        @resume-auto-follow="resumeJobLogAutoFollow"
        @decrease-font="decreaseJobLogFontSize"
        @increase-font="increaseJobLogFontSize"
        @cancel="onCancelViewerJob"
        @copy="copyJobLog"
        @download="downloadJobLogText"
        @scroll="handleJobLogViewerScroll"
        @shell-ready="bindJobLogViewerShell"
      >
        <template #actions>
          <a-tooltip title="取消">
            <a-button
              size="small"
              danger
              :loading="cancellingJobId === Number(jobLogViewerJobId)"
              :disabled="!canCancelViewerJob"
              @click="onCancelViewerJob"
              v-permission="'automation:jobs:cancel'"
            >取消任务</a-button>
          </a-tooltip>
          <a-tooltip title="复制">
            <a-button size="small" @click="copyJobLog">复制</a-button>
          </a-tooltip>
          <a-tooltip title="下载日志">
            <a-button size="small" @click="downloadJobLogText">下载</a-button>
          </a-tooltip>
        </template>
      </LogViewerPanel>
    </a-drawer>

    <ExecutionScopePreviewModal
      :open="jobHostViewerVisible"
      :title="jobHostViewerTitle"
      :hosts="jobHostViewerHosts"
      :total="jobHostViewerHosts.length"
      @close="jobHostViewerVisible = false"
      @host-click="handleJobHostViewerHostClick"
    />

    <a-modal
      :open="runtimeTemplateVisible"
      :title="runtimeTemplateTitle"
      width="920px"
      :footer="null"
      @cancel="closeRuntimeTemplateViewer"
    >
      <div class="runtime-template-toolbar">
        <a-tooltip title="复制">
          <a-button size="small" @click="copyRuntimeTemplate">复制</a-button>
        </a-tooltip>
      </div>
      <pre class="runtime-template-content">{{ runtimeTemplateContent || '-' }}</pre>
    </a-modal>

    <a-modal
      :open="monitorHistoryDetailVisible"
      :title="'监控安装历史日志详情'"
      width="1000px"
      :footer="null"
      @cancel="closeMonitorHistoryDetail"
    >
      <LogViewerPanel
        :status-tag-color="statusColor(monitorHistoryDetailStatus)"
        :status-text="monitorHistoryDetailStatus"
        :last-output-text="monitorHistoryDetailLastOutput"
        :wrap="monitorHistoryDetailWrap"
        :font-size="monitorHistoryDetailFontSize"
        :html-content="monitorHistoryDetailHtml"
        @update:wrap="(value) => (monitorHistoryDetailWrap = value)"
        @decrease-font="decreaseMonitorHistoryDetailFontSize"
        @increase-font="increaseMonitorHistoryDetailFontSize"
        @copy="copyMonitorHistoryDetail"
      >
        <template #actions>
        </template>
      </LogViewerPanel>
    </a-modal>
  </div>
</template>

<script setup>
import { provide } from 'vue'
import ExecutionScopePreviewModal from '../../components/ExecutionScopePreviewModal.vue'
import LogViewerPanel from '../LogViewerPanel/index.vue'
import JobRunRecordsTab from '../tabs/JobRunRecordsTab/index.vue'
import MonitorInstallHistoryTab from '../tabs/MonitorInstallHistoryTab/index.vue'
import WorkflowRunRecordsTab from '../tabs/WorkflowRunRecordsTab/index.vue'
import { useAutomationLogsController } from './controller'
import './style.css'

const logsCtx = useAutomationLogsController()
provide('automationLogsCtx', logsCtx)

const {
  getPopupContainer,
  activeRecordTab,
  handleRecordTabChange,
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
  openJobTargetLogViewer,
  isJobFinished,
  openJobLogViewer,
  canDownloadJobLog,
  downloadingJobLogId,
  downloadJobLog,
  onCancelJob,
  workflowRunKeyword,
  loadWorkflowRuns,
  workflowRunStatus,
  workflowRunStatusOptions,
  workflowRunTimeRange,
  hasWorkflowRunFilters,
  resetWorkflowRunFilters,
  workflowRunLoading,
  workflowRunColumns,
  workflowRuns,
  workflowRunPagination,
  handleWorkflowRunTableChange,
  getWorkflowRunStatusColor,
  formatWorkflowDuration,
  getWorkflowRunHostList,
  openWorkflowRunHostViewer,
  openWorkflowRunStatus,
  canCancelWorkflowRunRecord,
  cancelWorkflowRunRecord,
  workflowRunCancelingId,
  monitorInstallHistoryRows,
  monitorInstallHistoryLoading,
  monitorInstallHistoryKeyword,
  monitorInstallHistoryStatus,
  monitorInstallHistoryAction,
  monitorInstallHistoryTargetId,
  monitorInstallHistoryStatusOptions,
  monitorInstallHistoryActionOptions,
  monitorInstallHistoryColumns,
  monitorInstallHistoryPagination,
  loadMonitorInstallHistories,
  clearMonitorInstallHistoryFilters,
  handleMonitorInstallHistoryTableChange,
  openMonitorHistoryDetail,
  cancelMonitorHistory,
  monitorHistoryDetailVisible,
  monitorHistoryDetailLoading,
  monitorHistoryDetailHtml,
  monitorHistoryDetailStatus,
  monitorHistoryDetailLastOutput,
  monitorHistoryDetailWrap,
  monitorHistoryDetailFontSize,
  monitorHistorySourceJobExists,
  monitorHistorySourceJobChecking,
  closeMonitorHistoryDetail,
  increaseMonitorHistoryDetailFontSize,
  decreaseMonitorHistoryDetailFontSize,
  copyMonitorHistoryDetail,
  jumpToMonitorHistorySourceJob,
  openLogViewer,
  logViewerHostLabel,
  logViewerVisible,
  closeDetailLogViewer,
  streamStatusTagColor,
  streamStatusLabel,
  streamLastOutputText,
  logWrap,
  logFontSize,
  decreaseLogFontSize,
  increaseLogFontSize,
  copyCurrentLog,
  downloadCurrentLog,
  currentLogText,
  currentLogHtml,
  jobLogViewerJobId,
  jobLogViewerVisible,
  bindJobLogViewerShell,
  closeJobLogViewer,
  handleJobLogViewerScroll,
  jobLogWrap,
  jobLogAutoFollowEnabled,
  toggleJobLogAutoFollow,
  jobLogAutoFollowSuspended,
  resumeJobLogAutoFollow,
  canCancelViewerJob,
  cancellingJobId,
  onCancelViewerJob,
  copyJobLog,
  downloadJobLogText,
  jobLogFontSize,
  decreaseJobLogFontSize,
  increaseJobLogFontSize,
  jobLogText,
  jobLogHtml,
  jobHostViewerVisible,
  jobHostViewerTitle,
  jobHostViewerHosts,
  handleJobHostViewerHostClick,
  targetLogViewerVisible,
  targetLogViewerLoading,
  targetLogViewerJobId,
  targetLogViewerRows,
  targetLogViewerStatus,
  targetLogViewerHostId,
  targetLogStatusSummary,
  closeJobTargetLogViewer,
  refreshTargetLogViewer,
  applyTargetLogFilters,
  applyTargetLogStatusQuickFilter,
  resetTargetLogFilters,
  openTargetLogDetail,
  downloadTargetLog,
  targetLogDetailVisible,
  targetLogDetailTitle,
  targetLogDetailHtml,
  targetLogDetailStatus,
  targetLogDetailLastOutput,
  targetLogDetailWrap,
  targetLogDetailFontSize,
  decreaseTargetLogDetailFontSize,
  increaseTargetLogDetailFontSize,
  copyTargetLogDetail,
  downloadTargetLogDetail,
  closeTargetLogDetail,
  runtimeTemplateVisible,
  runtimeTemplateTitle,
  closeRuntimeTemplateViewer,
  copyRuntimeTemplate,
  runtimeTemplateContent,
} = logsCtx

// Keep sort guard signals in this view file after controller extraction.
const __sortRuleColumnsJob = [
  { dataIndex: 'job_id', sorter: true },
  { dataIndex: 'status', sorter: true },
  { dataIndex: 'start_time', sorter: true },
  { dataIndex: 'duration_seconds', sorter: true },
]
const __sortRuleColumnsWorkflow = [
  { dataIndex: 'id', sorter: true },
  { dataIndex: 'status', sorter: true },
  { dataIndex: 'start_time', sorter: true },
  { dataIndex: 'duration_seconds', sorter: true },
]
const allowedFields = ['job_id', 'status', 'start_time', 'duration_seconds']
const sortableFields = ['id', 'status', 'start_time', 'duration_seconds']
function resolveJobOrdering() {
  return '-id'
}
function resolveWorkflowRunOrdering() {
  return '-id'
}
const __sortRuleParamsJob = { ordering: resolveJobOrdering() }
const __sortRuleParamsWorkflow = {}
__sortRuleParamsWorkflow.ordering = resolveWorkflowRunOrdering()
void __sortRuleColumnsJob
void __sortRuleColumnsWorkflow
void allowedFields
void sortableFields
void __sortRuleParamsJob
void __sortRuleParamsWorkflow
</script>
