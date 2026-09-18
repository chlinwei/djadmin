<template>
  <div class="automation-logs-page">
    <JobRunRecordsTab />

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
  </div>
</template>

<script setup>
import { provide } from 'vue'
import ExecutionScopePreviewModal from '../../components/ExecutionScopePreviewModal.vue'
import LogViewerPanel from '../LogViewerPanel/index.vue'
import JobRunRecordsTab from '../tabs/JobRunRecordsTab/index.vue'
import { useAutomationLogsController } from './controller'
import './style.css'

const logsCtx = useAutomationLogsController()
provide('automationLogsCtx', logsCtx)

const {
  streamStatusTagColor,
  streamStatusLabel,
  streamLastOutputText,
  jobLogWrap,
  jobLogFontSize,
  jobLogHtml,
  jobLogAutoFollowEnabled,
  jobLogAutoFollowSuspended,
  cancellingJobId,
  jobLogViewerJobId,
  jobLogViewerVisible,
  canCancelViewerJob,
  toggleJobLogAutoFollow,
  resumeJobLogAutoFollow,
  decreaseJobLogFontSize,
  increaseJobLogFontSize,
  onCancelViewerJob,
  copyJobLog,
  downloadJobLogText,
  handleJobLogViewerScroll,
  bindJobLogViewerShell,
  closeJobLogViewer,
  jobHostViewerVisible,
  jobHostViewerTitle,
  jobHostViewerHosts,
  handleJobHostViewerHostClick,
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
