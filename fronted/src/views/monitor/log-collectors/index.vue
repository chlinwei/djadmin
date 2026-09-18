<template>
  <div class="log-collectors-page">
    <div class="page-head">
      <div>
        <h3>日志采集</h3>
        <p class="page-head__hint">
          纳管主机上的 Filebeat：安装/卸载、启停、下发采集配置，并展示「配置状态」（主机上的配置是否为当前期望的那份）。
          采集配置的内容来自逻辑服务的日志设置与日志定义，Filebeat 安装包在「智能监控 → 软件仓库」维护。
        </p>
      </div>
    </div>

    <HostTargetPanel
      v-model:keyword="table.keyword.value"
      v-model:group-keyword="table.groupKeyword.value"
      v-model:group-expanded-keys="table.groupExpandedKeys.value"
      :rows="table.hosts.value"
      :columns="COLUMNS"
      :loading="table.loading.value"
      :scroll-x="scrollX"
      :pagination="table.pagination"
      :row-selection="table.rowSelection.value"
      :selected-count="table.selectedHostIds.value.length"
      :group-tree-data="table.groupTreeData.value"
      :selected-group-keys="table.selectedGroupKeys.value"
      @reload="table.reload()"
      @group-select="table.handleGroupSelect"
      @table-change="table.handleTableChange"
    >
      <template #filters>
        <a-radio-group v-model:value="managedFilter" size="small" @change="table.reload()">
          <a-radio-button value="">全部 Filebeat</a-radio-button>
          <a-radio-button value="true">已安装</a-radio-button>
          <a-radio-button value="false">未安装</a-radio-button>
        </a-radio-group>
        <a-select
          v-model:value="configStateFilter"
          size="small"
          style="width: 150px"
          placeholder="全部配置状态"
          allow-clear
          :options="configStateFilterOptions"
          :getPopupContainer="getPopupContainer"
          @change="table.reload()"
        />
        <a-tooltip v-if="configStateError" :title="configStateError" placement="top">
          <a-typography-text type="warning">
            <FontAwesomeIcon :icon="['fas', 'triangle-exclamation']" />
            &nbsp;配置状态无法计算
          </a-typography-text>
        </a-tooltip>
      </template>

      <template #batch-actions>
        <a-tooltip title="为选中的未纳管主机批量安装 Filebeat" placement="top">
          <a-button
            type="primary"
            size="small"
            :disabled="!selectedUnmanaged.length"
            :loading="batchLoading === 'create'"
            @click="handleBatchCreate"
          >
            <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
            &nbsp;装（{{ selectedUnmanaged.length }}）
          </a-button>
        </a-tooltip>
        <a-tooltip title="批量重新安装 Filebeat" placement="top">
          <a-button
            type="primary"
            ghost
            size="small"
            :disabled="!selectedManagedIds.length"
            :loading="batchLoading === 'retry'"
            @click="handleBatch('retry')"
          >
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            &nbsp;重新安装（{{ selectedManagedIds.length }}）
          </a-button>
        </a-tooltip>
        <a-tooltip title="运行" placement="top">
          <a-button
            type="primary"
            ghost
            size="small"
            :disabled="!selectedManagedIds.length"
            :loading="batchLoading === 'start'"
            @click="handleBatch('start')"
          >
            <FontAwesomeIcon :icon="['fas', 'play']" />
            &nbsp;启动
          </a-button>
        </a-tooltip>
        <a-tooltip title="批量停止 Filebeat" placement="top">
          <a-button
            danger
            ghost
            size="small"
            :disabled="!selectedManagedIds.length"
            :loading="batchLoading === 'stop'"
            @click="handleBatch('stop')"
          >
            <FontAwesomeIcon :icon="['fas', 'stop']" />
            &nbsp;停止
          </a-button>
        </a-tooltip>
        <a-tooltip :title="selectedManagedIds.length ? `为选中的 ${selectedManagedIds.length} 台下发采集配置` : '请先选择已纳管的主机'" placement="top">
          <a-button
            type="primary"
            ghost
            size="small"
            :disabled="!selectedManagedIds.length"
            :loading="batchLoading === 'batch-apply'"
            @click="handleSelectedApply"
          >
            <FontAwesomeIcon :icon="['fas', 'paper-plane']" />
            &nbsp;下发配置
          </a-button>
        </a-tooltip>
        <a-tooltip :title="pendingTooltip" placement="top">
          <a-button
            type="primary"
            size="small"
            :disabled="!pendingSummary.pending"
            :loading="batchLoading === 'batch-apply'"
            @click="handleApplyAllPending"
          >
            <FontAwesomeIcon :icon="['fas', 'cloud-arrow-down']" />
            &nbsp;一键下发全部待变更（{{ pendingSummary.pending }}）
          </a-button>
        </a-tooltip>
        <a-tooltip :title="selectedManagedIds.length ? `为选中的 ${selectedManagedIds.length} 台重新安装 Filebeat` : '请先选择已纳管的主机'" placement="top">
          <a-button
            size="small"
            :disabled="!selectedManagedIds.length"
            :loading="batchLoading === 'batch-install'"
            @click="handleBatchInstall"
          >
            <FontAwesomeIcon :icon="['fas', 'rotate-right']" />
            &nbsp;批量重新安装
          </a-button>
        </a-tooltip>
        <a-tooltip title="删除" placement="top">
          <a-button
            class="delBtn"
            danger
            type="primary"
            size="small"
            :disabled="!selectedManagedIds.length"
            :loading="batchLoading === 'delete'"
            @click="openBatchDeleteConfirm"
          >
            <FontAwesomeIcon :icon="['fas', 'trash-can']" />
            &nbsp;删除
          </a-button>
        </a-tooltip>
      </template>

      <template #cell="{ column, record }">
        <template v-if="column.key === 'filebeat_status'">
          <a-tooltip v-if="filebeatStatusTooltip(record.filebeat)" :title="filebeatStatusTooltip(record.filebeat)" placement="top">
            <a-tag :color="filebeatStatusColor(record.filebeat)">
              {{ filebeatStatusText(record.filebeat) }}
            </a-tag>
          </a-tooltip>
          <a-tag v-else :color="filebeatStatusColor(record.filebeat)">
            {{ filebeatStatusText(record.filebeat) }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'filebeat_config_state'">
          <a-tooltip v-if="configStateTooltip(record.filebeat)" :title="configStateTooltip(record.filebeat)" placement="top">
            <a-tag :color="configStateColor(record.filebeat)">
              {{ configStateText(record.filebeat) }}
            </a-tag>
          </a-tooltip>
          <a-tag v-else :color="configStateColor(record.filebeat)">
            {{ configStateText(record.filebeat) }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'last_applied_time'">
          {{ record.filebeat.managed ? formatTimelineTime(record.filebeat.last_applied_time) : '-' }}
        </template>
        <template v-else-if="column.key === 'last_error'">
          <a-tooltip v-if="record.filebeat.last_error" :title="record.filebeat.last_error" placement="top">
            <a-typography-text type="danger" :content="record.filebeat.last_error" ellipsis />
          </a-tooltip>
          <span v-else>-</span>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space :size="6">
            <a-tooltip
              v-if="!record.filebeat.managed"
              :title="record.host_agent_online ? '纳管并安装 Filebeat' : 'dj-agent 离线，操作不可用'"
              placement="top"
            >
              <a-button
                type="primary"
                ghost
                size="small"
                :disabled="!record.host_agent_online"
                :loading="createLoading[record.host_id]"
                @click="handleCreateOne(record.filebeat)"
              >
                <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
                &nbsp;Filebeat
              </a-button>
            </a-tooltip>
            <a-dropdown v-else trigger="click" :getPopupContainer="getPopupContainer">
              <a-button type="primary" ghost size="small" title="Filebeat 操作">
                Filebeat&nbsp;<FontAwesomeIcon :icon="['fas', 'angle-down']" />
              </a-button>
              <template #overlay>
                <div class="row-action-menu">
                  <a-tooltip :title="record.host_agent_online ? '重新安装' : 'dj-agent 离线，操作不可用'" placement="left">
                    <a-button
                      block
                      type="primary"
                      ghost
                      size="small"
                      :disabled="!record.host_agent_online"
                      :loading="retryLoading[record.filebeat.id]"
                      @click="openRetryConfirm(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'rotate']" />
                      &nbsp;重新安装
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="查看日志" placement="left">
                    <a-button
                      block
                      type="primary"
                      ghost
                      size="small"
                      :disabled="retryLoading[record.filebeat.id]"
                      @click="openJobLog(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'file-lines']" />
                      &nbsp;查看日志
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="运行" placement="left">
                    <a-button
                      block
                      type="primary"
                      ghost
                      size="small"
                      :disabled="!record.host_agent_online || !record.filebeat.agent_installed"
                      :loading="startLoading[record.filebeat.id]"
                      @click="handleStartService(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'play']" />
                      &nbsp;运行
                    </a-button>
                  </a-tooltip>
                  <a-tooltip :title="record.filebeat.agent_installed ? '停止服务' : 'Filebeat 尚未安装，无法停止'" placement="left">
                    <a-button
                      block
                      danger
                      ghost
                      size="small"
                      :disabled="!record.host_agent_online || !record.filebeat.agent_installed"
                      :loading="stopLoading[record.filebeat.id]"
                      @click="handleStopService(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'stop']" />
                      &nbsp;停止
                    </a-button>
                  </a-tooltip>
                  <a-tooltip :title="applyTooltip(record.filebeat)" placement="left">
                    <a-button
                      block
                      type="primary"
                      ghost
                      size="small"
                      :disabled="!canApplyConfig(record.filebeat)"
                      :loading="applyLoading[record.filebeat.id]"
                      @click="handleApplyConfig(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'paper-plane']" />
                      &nbsp;下发配置
                    </a-button>
                  </a-tooltip>
                  <a-tooltip :title="record.host_agent_online ? '查看状态图' : 'dj-agent 离线，操作不可用'" placement="left">
                    <a-button
                      block
                      type="primary"
                      ghost
                      size="small"
                      :disabled="!record.host_agent_online"
                      :loading="statusLoading[record.filebeat.id]"
                      @click="handleCheckStatus(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'rotate']" />
                      &nbsp;查看状态图
                    </a-button>
                  </a-tooltip>
                  <a-tooltip :title="canCancelTarget(record.filebeat) ? '取消' : '当前任务已结束，无需取消'" placement="left">
                    <a-button
                      block
                      danger
                      ghost
                      size="small"
                      :disabled="!canCancelTarget(record.filebeat)"
                      :loading="cancelLoading[record.filebeat.id]"
                      @click="handleCancelTarget(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'ban']" />
                      &nbsp;取消
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="删除" placement="left">
                    <a-button
                      block
                      class="delBtn"
                      danger
                      type="primary"
                      size="small"
                      :disabled="record.filebeat.install_status === 'pending' || (!record.host_agent_online && record.filebeat.agent_installed)"
                      :loading="deleteLoading[record.filebeat.id]"
                      @click="openDeleteConfirmFor(record.filebeat)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                      &nbsp;删除
                    </a-button>
                  </a-tooltip>
                </div>
              </template>
            </a-dropdown>
          </a-space>
        </template>
      </template>
    </HostTargetPanel>

    <a-modal
      v-model:open="batchJobVisible"
      :title="batchJobTitle"
      :footer="null"
      width="640px"
    >
      <div v-if="batchJob" class="batch-job">
        <a-progress :percent="batchJobPercent" :status="batchJob.is_running ? 'active' : (batchJob.failed_count ? 'exception' : 'success')" />
        <div class="batch-job__counts">
          <span>共 {{ batchJob.total_count }} 台</span>
          <span class="ok">成功 {{ batchJob.success_count }}</span>
          <span class="fail">失败 {{ batchJob.failed_count }}</span>
          <span>待处理 {{ batchJob.pending_count }}</span>
          <span v-if="batchJob.is_running">并发 {{ batchJob.concurrency }} 台</span>
        </div>
        <a-alert
          v-if="batchJob.is_running"
          type="info"
          show-icon
          :message="`作业在后台按并发上限推进，关掉这个窗口不会中断；失败的主机会在下面逐台列出。`"
        />
        <a-alert
          v-else
          :type="batchJob.failed_count ? 'warning' : 'success'"
          show-icon
          :message="batchJob.message || '作业已结束'"
        />
        <div v-if="batchJobFailedItems.length" class="batch-job__failures">
          <p class="batch-job__failures-title">失败主机（{{ batchJobFailedItems.length }}）</p>
          <ul>
            <li v-for="item in batchJobFailedItems" :key="item.id">
              <strong>{{ item.host_name || item.host_ip || `Host-${item.host_id}` }}</strong>
              <span class="reason">{{ item.message || '未返回原因' }}</span>
            </li>
          </ul>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'

import {
  applyLogCollectionConfig,
  batchCreateLogCollectionTargets,
  batchDeleteLogCollectionTargets,
  batchStartLogCollectionTargets,
  batchStopLogCollectionTargets,
  cancelLogCollectionTarget,
  checkLogCollectionStatus,
  createLogBatchJob,
  getActiveLogBatchJob,
  getLogBatchJob,
  getLogPendingSummary,
  getMonitorInstallHistoryList,
  retryLogCollectionTarget,
  startLogCollectionService,
  stopLogCollectionService,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import { formatTimeWithTimezone } from '@/util/timezone'
import { useHostTargetTable } from '@/util/hostTargetTable'
import store from '@/store'
import HostTargetPanel from '../components/HostTargetPanel.vue'

// 「日志管理 → 日志采集」：Filebeat 纳管目标的独立页面。
//
// 从「智能监控 → 纳管目标」拆出来的原因：那是 exporter 与 Filebeat 混在一张表里用 segmented 切换，
// 而日志采集的完整闭环（安装 → 下发配置 → 看配置状态）本就属于日志管理，与日志存储/处理规则/
// 保留档位同属一条链路；拆开后监控页只留 exporter。两者消费的是同一份主机视角数据
// （GET /monitor/targets/host-overview/），主机树/分页/行选择/状态刷新走 @/util/hostTargetTable 共享。

const router = useRouter()
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const COLUMNS = [
  { title: '主机名', dataIndex: 'host_name', key: 'host_name', width: 180, fixed: 'left' },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 140 },
  { title: 'Filebeat 状态', key: 'filebeat_status', width: 150 },
  { title: '配置状态', key: 'filebeat_config_state', width: 130 },
  { title: 'Filebeat 下发', key: 'last_applied_time', width: 170 },
  { title: 'Filebeat 错误', key: 'last_error', width: 200 },
  { title: '操作', key: 'action', width: 250, fixed: 'right' },
]
const scrollX = computed(() => COLUMNS.reduce((total, column) => total + Number(column.width || 0), 0))

const managedFilter = ref('')
const configStateFilter = ref('')
const configStateError = ref('')
const configStateFilterOptions = [
  { value: 'synced', label: '已同步' },
  { value: 'drift', label: '待下发（已变更）' },
  { value: 'never', label: '从未下发' },
  { value: 'unknown', label: '未知' },
]

const batchLoading = ref('')
const createLoading = reactive({})
const applyLoading = reactive({})
const retryLoading = reactive({})
const startLoading = reactive({})
const stopLoading = reactive({})
const cancelLoading = reactive({})
const deleteLoading = reactive({})
const statusLoading = reactive({})
const realStatusMap = reactive({})

const table = useHostTargetTable({
  extraQuery: () => ({
    filebeat_managed: managedFilter.value || undefined,
    config_state: configStateFilter.value || undefined,
  }),
  // 列表加载后为当前页已纳管且 agent 在线的 Filebeat 查一次真实运行态（后端执行 systemctl status
  // 并回写 runtime_status）。不查的话界面会一直显示上次安装/下发时落库的旧状态。
  refreshRowStatuses: async (hosts) => {
    const ids = hosts
      .filter((item) => item.host_agent_online && item.filebeat && item.filebeat.managed)
      .map((item) => item.filebeat.id)
      .filter(Boolean)
    await Promise.all(ids.map(async (id) => {
      try {
        const job = parseApiData(await checkLogCollectionStatus(id))
        if (job && job.exit_code !== undefined && job.exit_code !== null) {
          realStatusMap[id] = { exitCode: Number(job.exit_code), checkedAt: new Date().toISOString() }
        }
      } catch (error) {
        console.warn('[filebeat_status] 查询运行状态失败', id, error?.response?.data?.msg || error?.message)
      }
    }))
  },
  onLoaded: (data) => {
    configStateError.value = data.config_state_error || ''
  },
})

function parseApiData(resp) {
  return resp?.data?.data || {}
}

// ---- 选项与选中集合 ----

const selectedRows = computed(() => table.selectedRows.value)
const selectedManagedIds = computed(() =>
  selectedRows.value.filter((item) => item.filebeat?.managed).map((item) => item.filebeat.id).filter(Boolean),
)
const selectedUnmanaged = computed(() =>
  selectedRows.value.filter((item) => item.filebeat && !item.filebeat.managed),
)

// 全量「待下发」台数（drift + never）。算的是**全部纳管目标**而不是当前页：一键下发要跨页，
// 分页无关的全局计数才有意义（后端一次批量渲染，见 pending-summary 接口的说明）。
const pendingSummary = ref({ total: 0, synced: 0, drift: 0, never: 0, unknown: 0, pending: 0, error: '' })

async function loadPendingSummary() {
  try {
    pendingSummary.value = parseApiData(await getLogPendingSummary())
  } catch (error) {
    // 计数取不到时不能显示"0 台待下发"（那是在谎报"全都同步了"），把原因带出来。
    pendingSummary.value = {
      total: 0, synced: 0, drift: 0, never: 0, unknown: 0, pending: 0,
      error: error?.response?.data?.msg || error?.message || '待下发台数获取失败',
    }
  }
}

const pendingTooltip = computed(() => {
  const summary = pendingSummary.value
  if (configStateError.value) return `配置状态无法计算：${configStateError.value}`
  if (summary.error) return `配置状态无法计算：${summary.error}`
  if (!summary.pending) return '所有已纳管主机的采集配置都是最新的'
  return `对全部 ${summary.pending} 台配置已变更/从未下发的主机下发采集配置`
  + `（已变更 ${summary.drift} 台、从未下发 ${summary.never} 台）`
})

// ---- 批量作业进度 ----
// 作业在后台按有界并发推进（安装 20 / 下发 5 台并发，见 §9 第 9 条），这里只轮询作业详情：
// 进度、成功/失败台数、失败主机与原因。关掉弹窗不停作业——作业是服务端的事实，与页面无关。
const LOG_BATCH_LABELS = { apply: '下发采集配置', install: '重新安装 Filebeat' }
const LOG_BATCH_POLL_INTERVAL = 2000
const batchJobVisible = ref(false)
const batchJob = ref(null)
const batchJobAction = ref('apply')
let batchJobTimer = null

const batchJobTitle = computed(() => `${LOG_BATCH_LABELS[batchJobAction.value] || '批量作业'}进度`)
const batchJobFailedItems = computed(() =>
  (batchJob.value?.items || []).filter((item) => item.status === 'failed'),
)
const batchJobPercent = computed(() => {
  const job = batchJob.value
  if (!job?.total_count) return 0
  return Math.round(((job.success_count + job.failed_count) / job.total_count) * 100)
})

function openBatchJobProgress(jobId, action) {
  batchJobAction.value = action
  batchJob.value = { id: jobId, total_count: 0, success_count: 0, failed_count: 0, pending_count: 0, status: 'pending', items: [] }
  batchJobVisible.value = true
  startBatchJobPolling(jobId)
}

function startBatchJobPolling(jobId) {
  stopBatchJobPolling()
  const poll = async () => {
    try {
      const job = parseApiData(await getLogBatchJob(jobId))
      batchJob.value = job
      if (!job.is_running) {
        stopBatchJobPolling()
        await loadPendingSummary()
        await table.load()
        return
      }
    } catch (error) {
      stopBatchJobPolling()
      return
    }
    batchJobTimer = setTimeout(poll, LOG_BATCH_POLL_INTERVAL)
  }
  poll()
}

function stopBatchJobPolling() {
  if (batchJobTimer) {
    clearTimeout(batchJobTimer)
    batchJobTimer = null
  }
}

// 刷新页面后接着看还在跑的作业：作业在服务端，页面只是它的视图。
async function resumeActiveBatchJob() {
  for (const action of ['apply', 'install']) {
    try {
      const job = parseApiData(await getActiveLogBatchJob(action))
      if (job?.id) {
        openBatchJobProgress(job.id, action)
        return
      }
    } catch (error) {
      // 查询失败不打扰用户：没有进行中的作业才是常态。
    }
  }
}

// ---- 运行态与配置状态展示 ----

// systemctl status 退出码语义：0=运行中，3=inactive/已停止，其余视为异常。
function realStatusInfo(targetId) {
  const cached = realStatusMap[targetId]
  if (!cached || !cached.checkedAt) return null
  const exitCode = Number(cached.exitCode)
  if (exitCode === 0) return { status: 'running', text: '运行中', color: 'success', tooltip: 'Filebeat 正常运行' }
  if (exitCode === 3) return { status: 'stopped', text: '已停止', color: 'warning', tooltip: 'Filebeat 服务已停止' }
  return { status: 'error', text: '异常', color: 'error', tooltip: 'Filebeat 运行异常' }
}

function filebeatStatusText(record) {
  const real = realStatusInfo(record?.id)
  if (real) return real.text
  if (!record || !record.managed) return '未安装'
  if (record.install_status === 'pending') return '安装中'
  if (record.install_status === 'failed') return '安装失败'
  if (record.agent_installed) {
    if (record.runtime_status === 'running') return '运行中'
    if (record.runtime_status === 'stopped') return '已停止'
    if (record.runtime_status === 'error') return '异常'
    return '已安装'
  }
  return '未安装'
}

function filebeatStatusColor(record) {
  const real = realStatusInfo(record?.id)
  if (real) return real.color
  if (!record || !record.managed) return 'default'
  if (record.install_status === 'pending') return 'processing'
  if (record.install_status === 'failed') return 'error'
  if (record.agent_installed) {
    if (record.runtime_status === 'running') return 'success'
    if (record.runtime_status === 'stopped') return 'warning'
    if (record.runtime_status === 'error') return 'error'
    return 'success'
  }
  return 'default'
}

function filebeatStatusTooltip(record) {
  const real = realStatusInfo(record?.id)
  if (real) return real.tooltip
  if (!record || !record.managed) return '未纳管 Filebeat 日志采集'
  if (record.install_status === 'pending') return '任务执行中，请稍候'
  if (record.install_status === 'failed') return record.last_error || '安装失败，请点击重试'
  if (record.agent_installed) {
    if (record.runtime_status === 'running') return 'Filebeat 正常运行'
    if (record.runtime_status === 'stopped') return 'Filebeat 服务已停止'
    if (record.runtime_status === 'error') return record.last_error || 'Filebeat 运行异常'
  }
  return ''
}

// 配置状态与运行态正交：配置一致不代表进程在跑，进程在跑也不代表配置是最新的。
function configStateText(record) {
  if (!record || !record.managed) return '未纳管'
  switch (record.config_state) {
    case 'synced':
      return '已同步'
    case 'drift':
      return '待下发'
    case 'never':
      return '从未下发'
    default:
      return '未知'
  }
}

function configStateColor(record) {
  if (!record || !record.managed) return 'default'
  switch (record.config_state) {
    case 'synced':
      return 'success'
    case 'drift':
    case 'never':
      return 'warning'
    default:
      return 'default'
  }
}

function configStateTooltip(record) {
  if (!record || !record.managed) return ''
  if (record.config_state === 'unknown') {
    return configStateError.value || '配置状态暂时无法计算'
  }
  const lines = []
  if (record.config_state === 'synced') {
    lines.push(`主机上的采集配置与当前期望一致（覆盖 ${record.config_service_num || 0} 个服务）`)
  } else if (record.config_state === 'drift') {
    lines.push('配置自上次下发后已变更（路径/宏/档位/日志开关，或默认集群的输出段），需重新下发')
  } else if (record.config_state === 'never') {
    lines.push('该主机从未下发过采集配置')
  }
  if (record.expected_fingerprint && record.expected_fingerprint !== record.config_fingerprint) {
    lines.push(`期望指纹：${record.expected_fingerprint.slice(0, 12)}…`)
  }
  const warnings = Array.isArray(record.config_warnings) ? record.config_warnings : []
  if (warnings.length) lines.push(...warnings.slice(0, 3))
  return lines.join('\n')
}

function formatTimelineTime(value) {
  if (!value) return '-'
  return formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai')
}

// 下发门槛只要求「agent 在线 + Filebeat 已装」：下发本身会写 filebeat.yml 与 inputs.d 并重启
// Filebeat（agent 侧 restart），所以"服务当前没在跑"不构成阻碍——反过来，要求先 running
// 会让停机待修的主机永远无法通过下发恢复。runtime_status 只在提示里说明。
function canApplyConfig(record) {
  return Boolean(record?.host_agent_online) && Boolean(record?.agent_installed)
}

function applyTooltip(record) {
  if (!record?.host_agent_online) return 'dj-agent 离线，操作不可用'
  if (!record?.agent_installed) return 'Filebeat 未安装，请先完成离线安装'
  if (record?.runtime_status !== 'running') return 'Filebeat 当前未运行，下发后会一并启动'
  return '运行'
}

function canCancelTarget(record) {
  return ['pending', 'running'].includes(String(record?.install_status || '').toLowerCase())
}

// ---- 批量与单台动作 ----

// 同步批量：单台只是一次 30 秒超时的 agent 调用，逐台串行的代价可接受。
const BATCH_ACTIONS = {
  start: { label: '批量启动', request: batchStartLogCollectionTargets },
  stop: { label: '批量停止', request: batchStopLogCollectionTargets },
  delete: { label: '批量删除', request: batchDeleteLogCollectionTargets },
}

async function handleBatch(action) {
  const config = BATCH_ACTIONS[action]
  const ids = [...selectedManagedIds.value]
  if (!config || !ids.length) return
  batchLoading.value = action
  try {
    const data = parseApiData(await config.request(ids))
    reportBatchResult(config.label, data)
    table.clearSelection()
    await Promise.all([table.load(), table.loadGroupTree()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || `${config.label}失败`)
  } finally {
    batchLoading.value = ''
  }
}

// 纳管并安装：安装部分由后端建了批量作业，把进度弹窗接上（失败原因逐台可见）。
function followCreatedInstallBatch(data) {
  if (data?.install_batch?.id) {
    openBatchJobProgress(data.install_batch.id, 'install')
    return
  }
  if (data?.install_error) message.warning(`纳管成功，但安装作业创建失败：${data.install_error}`)
}

// 异步批量作业：下发配置（apply）与安装重试（install）都要跑分钟级操作，1000 台时不可能在
// 一个请求里跑完。后端只建作业 + 入队，前端按作业号轮询进度（见 log_batch_job.go）。
async function handleStartLogBatch(action, ids) {
  batchLoading.value = `batch-${action}`
  try {
    const job = parseApiData(await createLogBatchJob(action, ids))
    message.success(`${LOG_BATCH_LABELS[action] || '批量作业'}已入队，共 ${job.total_count} 台`)
    openBatchJobProgress(job.id, action)
    table.clearSelection()
    await table.load()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '批量作业创建失败')
  } finally {
    batchLoading.value = ''
  }
}

// 「下发配置」：只作用于选中的已纳管主机。
function handleSelectedApply() {
  const ids = [...selectedManagedIds.value]
  if (ids.length) handleStartLogBatch('apply', ids)
}

// 「一键下发全部待变更」：ids 省略 → 后端按配置态实时算出全部 drift/never 的主机。
function handleApplyAllPending() {
  if (!pendingSummary.value.pending) return
  handleStartLogBatch('apply', null)
}

function handleBatchInstall() {
  const ids = [...selectedManagedIds.value]
  if (ids.length) handleStartLogBatch('install', ids)
}

async function handleBatchCreate() {
  const hostIds = selectedUnmanaged.value.map((item) => item.host_id)
  if (!hostIds.length) return
  batchLoading.value = 'create'
  try {
    const data = parseApiData(await batchCreateLogCollectionTargets(hostIds, true))
    reportBatchResult('纳管', data)
    // 安装走批量作业（后端入队），前台不再逐台等；进度在作业弹窗里看。
    followCreatedInstallBatch(data)
    table.clearSelection()
    await Promise.all([table.load(), table.loadGroupTree()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '纳管失败')
  } finally {
    batchLoading.value = ''
  }
}

async function handleCreateOne(record) {
  createLoading[record.host_id] = true
  try {
    const data = parseApiData(await batchCreateLogCollectionTargets([record.host_id], true))
    reportBatchResult('纳管', data)
    followCreatedInstallBatch(data)
    await Promise.all([table.load(), table.loadGroupTree()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '纳管失败')
  } finally {
    createLoading[record.host_id] = false
  }
}

function openBatchDeleteConfirm() {
  const selected = selectedRows.value.filter((item) => item.filebeat?.managed)
  if (!selected.length) return
  openDeleteConfirm({
    title: '确认批量删除 Filebeat 目标',
    summary: '已安装的主机会先下发卸载任务，卸载成功后自动删除纳管记录；卸载失败则保留记录和安装日志。',
    items: selected.map((item) => `${item.host_name || item.host_ip || `Host-${item.host_id}`} - Filebeat`),
    onConfirm: () => handleBatch('delete'),
  })
}

function openDeleteConfirmFor(record) {
  const hostLabel = record.host_name || record.host_ip || String(record.host || '-')
  openDeleteConfirm({
    title: '确认删除 Filebeat 目标',
    summary: record.agent_installed
      ? '会先下发卸载任务，卸载成功后自动删除纳管记录；卸载失败则保留记录和安装日志。'
      : '目标删除后，如需继续采集日志必须重新创建并安装。',
    items: [`${hostLabel} - Filebeat`],
    onConfirm: async () => {
      deleteLoading[record.id] = true
      try {
        const data = parseApiData(await batchDeleteLogCollectionTargets([record.id]))
        message.success(data?.pending_uninstall
          ? '已下发卸载任务，卸载成功后自动删除'
          : 'Filebeat 目标已删除')
      } catch (error) {
        message.error(error?.response?.data?.msg || error?.message || 'Filebeat 目标删除失败')
      } finally {
        deleteLoading[record.id] = false
        await table.load()
      }
    },
  })
}

async function handleRetry(record) {
  retryLoading[record.id] = true
  try {
    await retryLogCollectionTarget(record.id)
    message.success('已下发 Filebeat 重新安装任务，请稍后刷新查看结果')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Filebeat 重新安装失败')
  } finally {
    retryLoading[record.id] = false
    await table.load()
  }
}

async function openRetryConfirm(record) {
  if (!record?.host_agent_online) return
  const hostLabel = record.host_name || record.host_ip || String(record.host || '-')
  await openDeleteConfirm({
    title: '确认重新安装',
    okText: '确认',
    summary: '将从 djadmin 本地仓库选择与目标系统架构匹配的 Filebeat tar.gz 包并重新安装。',
    items: [`${hostLabel} - Filebeat`],
    onConfirm: () => {
      void handleRetry(record)
    },
  })
}

async function handleCheckStatus(record) {
  statusLoading[record.id] = true
  try {
    const job = parseApiData(await checkLogCollectionStatus(record.id))
    if (job && job.exit_code !== undefined && job.exit_code !== null) {
      realStatusMap[record.id] = { exitCode: Number(job.exit_code), checkedAt: new Date().toISOString() }
    }
    message.success('Filebeat 状态已更新')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Filebeat 状态检查失败')
  } finally {
    statusLoading[record.id] = false
  }
}

async function handleApplyConfig(record) {
  applyLoading[record.id] = true
  try {
    const result = parseApiData(await applyLogCollectionConfig(record.id))
    message.success(result?.skipped ? '配置未变化，无需重复下发' : 'Filebeat 配置已下发')
    reportApplyWarnings(result?.warnings)
    await table.load()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Filebeat 配置下发失败')
  } finally {
    applyLoading[record.id] = false
  }
}

async function handleStartService(record) {
  startLoading[record.id] = true
  try {
    const result = parseApiData(await startLogCollectionService(record.id))
    if (result?.status === 'success' && result?.exit_code === 0) {
      message.success('Filebeat 启动成功')
    } else {
      message.error(`Filebeat 启动失败：${result?.stderr || result?.error_message || result?.status}`)
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Filebeat 启动失败')
  } finally {
    startLoading[record.id] = false
    await table.load()
  }
}

async function handleStopService(record) {
  stopLoading[record.id] = true
  try {
    const result = parseApiData(await stopLogCollectionService(record.id))
    if (result?.status === 'success' && result?.exit_code === 0) {
      message.success('Filebeat 停止成功')
    } else {
      message.error(`Filebeat 停止失败：${result?.stderr || result?.error_message || result?.status}`)
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Filebeat 停止失败')
  } finally {
    stopLoading[record.id] = false
    await table.load()
  }
}

async function handleCancelTarget(record) {
  if (!canCancelTarget(record)) return
  cancelLoading[record.id] = true
  try {
    await cancelLogCollectionTarget(record.id)
    message.success('任务已取消')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '取消任务失败')
  } finally {
    cancelLoading[record.id] = false
    await table.load()
  }
}

// ---- 作业日志与提示 ----

// 安装历史不再单独占运行记录中心的 tab：取该目标最新一条历史关联的自动化作业 id，跳到作业日志。
async function openInstallHistoryJob(filter) {
  let jobId = null
  try {
    const historyData = parseApiData(await getMonitorInstallHistoryList({
      page: 1, page_size: 1, ordering: '-id', ...filter,
    }))
    const rows = Array.isArray(historyData?.results) ? historyData.results : []
    const parsed = Number(rows[0]?.automation_job_id_snapshot)
    if (Number.isInteger(parsed) && parsed > 0) jobId = parsed
  } catch (_error) {
    // 查询失败不阻断入口，下面统一提示。
  }
  if (!jobId) {
    message.warning('未找到该目标对应的自动化作业日志')
    return
  }
  router.push({ path: '/sys/automation/logs', query: { job_id: String(jobId) } })
}

async function openJobLog(record) {
  await openInstallHistoryJob({ log_collection_target_id: String(record.id) })
}

function reportBatchResult(label, data) {
  const results = Array.isArray(data.results) ? data.results : []
  const failed = results.filter((item) => !item.ok)
  const warnings = results.flatMap((item) => item?.detail?.warnings || [])
  if (warnings.length) reportApplyWarnings(warnings)
  if (failed.length === 0) {
    message.success(`${label}成功：${data.success} 台`)
    return
  }
  // 逐台执行，部分失败是常态；把失败主机和原因摊开，避免只报一个笼统错误。
  const detail = failed.slice(0, 3).map((item) => `${item.host}：${item.message}`).join('；')
  const suffix = failed.length > 3 ? ` 等 ${failed.length} 台` : ''
  message.warning(`${label}完成：成功 ${data.success} 台，失败 ${data.failed} 台。${detail}${suffix}`)
}

// 下发/预览返回的 warnings（未展开宏、未关联处理规则等）以前被丢弃，导致"下发成功但日志不解析"
// 无从发现；这里统一提示，最多展开 2 条。
function reportApplyWarnings(warnings) {
  const list = Array.isArray(warnings) ? warnings.filter(Boolean) : []
  if (!list.length) return
  const suffix = list.length > 2 ? ` 等 ${list.length} 条` : ''
  message.warning(`下发成功，但存在告警：${list.slice(0, 2).join('；')}${suffix}`, 8)
}

// ---- 刷新生命周期 ----

let refreshTimer = null

async function loadAll() {
  await Promise.all([table.load(), table.loadGroupTree(), loadPendingSummary()])
}

function startRefresh() {
  stopRefresh()
  refreshTimer = window.setInterval(() => {
    if (table.loading.value) return
    loadAll()
  }, 30000)
}

function stopRefresh() {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
}

// 页面切走时暂停轮询，避免后台持续请求（与其它页面同一约定）。
useKeepAliveRefreshLifecycle(startRefresh, stopRefresh)

onMounted(async () => {
  await loadAll()
  // 回到页面时接着看还没跑完的批量作业（作业在服务端，与页面生命周期无关）。
  await resumeActiveBatchJob()
})
onBeforeUnmount(() => {
  stopRefresh()
  stopBatchJobPolling()
})
</script>

<style scoped>
.batch-job__counts {
  display: flex;
  gap: 16px;
  margin: 12px 0;
  color: #595959;
}

.batch-job__counts .ok {
  color: #52c41a;
}

.batch-job__counts .fail {
  color: #ff4d4f;
}

.batch-job__failures {
  margin-top: 12px;
  max-height: 240px;
  overflow-y: auto;
}

.batch-job__failures-title {
  margin: 0 0 6px;
  color: #595959;
}

.batch-job__failures ul {
  margin: 0;
  padding-left: 18px;
}

.batch-job__failures .reason {
  margin-left: 8px;
  color: #8c8c8c;
}

.page-head {
  margin-bottom: 12px;
}

.page-head h3 {
  margin: 0 0 4px;
}

.page-head__hint {
  margin: 0;
  color: #8c8c8c;
  font-size: 13px;
}

/* 行内动作收进下拉面板，避免操作列被十来个按钮撑到不可用。 */
.row-action-menu {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 150px;
  padding: 8px;
  background: #fff;
  border-radius: 6px;
  box-shadow: 0 3px 6px -4px rgb(0 0 0 / 12%), 0 6px 16px 0 rgb(0 0 0 / 8%);
}
</style>
