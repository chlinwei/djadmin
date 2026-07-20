<template>
  <div class="monitor-page">
    <a-row :gutter="12" class="tools">
      <a-col :span="16">
        <a-space>
          <a-tag color="blue">Prometheus</a-tag>
          <span class="prom-url">{{ prometheusBaseUrl || '-' }}</span>
          <a-tag v-if="lastRefreshAtText" color="default">刷新于 {{ lastRefreshAtText }}</a-tag>
        </a-space>
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-switch v-model:checked="autoRefreshEnabled" checked-children="自动刷新" un-checked-children="手动" />
          <a-select v-model:value="refreshIntervalSeconds" style="width: 120px" :options="refreshIntervalOptions" :disabled="!autoRefreshEnabled" />
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="loading" @click="loadAllData">
              刷新
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <div class="overview-grid">
      <a-card size="small" class="overview-card">
        <a-statistic title="监控目标总数" :value="overview.total" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="采集正常" :value="overview.up" :value-style="{ color: '#3f8600' }" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="采集异常" :value="overview.down" :value-style="{ color: '#cf1322' }" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="触发告警" :value="alertSummary.firing" :value-style="{ color: '#cf1322' }" />
      </a-card>
    </div>

    <a-card title="智能监控" size="small" class="monitor-card">
      <a-alert
        v-if="errorMessage"
        type="warning"
        show-icon
        :message="errorMessage"
        style="margin-bottom: 12px"
      />

      <a-tabs v-model:activeKey="activeTabKey">
        <a-tab-pane key="prom-targets" tab="Prometheus 采集目标">
          <a-table
            rowKey="instance"
            :columns="promTargetColumns"
            :data-source="promTargets"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="{ pageSize: 10, showSizeChanger: true }"
          />
        </a-tab-pane>

        <a-tab-pane key="alerts" tab="Prometheus 告警">
          <a-table
            :rowKey="alertRowKey"
            :columns="alertColumns"
            :data-source="alerts"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="{ pageSize: 10, showSizeChanger: true }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'state'">
                <a-tag :color="record.state === 'firing' ? 'red' : 'default'">{{ record.state || 'unknown' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'severity'">
                <a-tag :color="severityColor(record.severity)">{{ record.severity || '-' }}</a-tag>
              </template>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="managed-targets" tab="纳管目标">
          <a-table
            rowKey="id"
            :columns="managedColumns"
            :data-source="managedTargets"
            :loading="loading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="managedPagination"
            @change="handleManagedTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'managed_enabled'">
                <a-tag :color="record.managed_enabled ? 'green' : 'default'">{{ record.managed_enabled ? '启用' : '禁用' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'install_status'">
                <a-tooltip v-if="record.install_message" :title="record.install_message" placement="top">
                  <a-tag :color="statusColor(record.install_status)">{{ record.install_status || 'unknown' }}</a-tag>
                </a-tooltip>
                <a-tag v-else :color="statusColor(record.install_status)">{{ record.install_status || 'unknown' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'last_scrape_status'">
                <a-tag :color="scrapeColor(record.last_scrape_status)">{{ record.last_scrape_status || 'unknown' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <a-space>
                  <a-tooltip :title="record.install_status === 'failed' ? '重试' : '自动重试进行中或已成功，无需操作'">
                    <a-button
                      type="primary"
                      ghost
                      size="small"
                      :disabled="record.install_status !== 'failed'"
                      :loading="managedRetryLoading[record.id]"
                      @click="handleManagedRetry(record)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'rotate']" />
                      重试
                    </a-button>
                  </a-tooltip>
                  <a-tooltip :title="record.last_install_job_id ? '查看日志' : '暂无安装/卸载任务记录'">
                    <a-button
                      type="primary"
                      ghost
                      size="small"
                      :disabled="!record.last_install_job_id"
                      @click="openManagedTargetJobLog(record)"
                    >
                      <FontAwesomeIcon :icon="['fas', 'file-lines']" />
                    </a-button>
                  </a-tooltip>
                </a-space>
              </template>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="packages" tab="软件仓库">
          <a-table
            rowKey="id"
            :columns="packageColumns"
            :data-source="packages"
            :loading="packagesLoading"
            size="small"
            :scroll="{ x: 1200 }"
            :pagination="packagePagination"
            @change="handlePackageTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'enabled'">
                <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '禁用' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'size_bytes'">
                {{ formatSize(record.size_bytes) }}
              </template>
              <template v-else-if="column.key === 'synced'">
                <a-tag :color="record.synced ? 'green' : 'orange'">{{ record.synced ? '已同步' : '未同步' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <a-space>
                  <a-tooltip title="编辑">
                    <a-button type="primary" ghost @click="openPackageEditModal(record)">
                      <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="上传">
                    <a-upload
                      accept=".tar.gz,.tgz"
                      :show-upload-list="false"
                      :before-upload="(file) => beforePackageUpload(file, record)"
                      :custom-request="(options) => handlePackageUpload(options, record)"
                    >
                      <a-button type="primary" ghost :loading="packageUploadLoading[record.id]">
                        <FontAwesomeIcon :icon="['fas', 'upload']" />
                      </a-button>
                    </a-upload>
                  </a-tooltip>
                  <a-tooltip title="自动更新">
                    <a-button type="primary" ghost :loading="packageSyncLoading[record.id]" @click="openSyncOfficialModal(record)">
                      <FontAwesomeIcon :icon="['fas', 'rotate']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="下载">
                    <a-button type="primary" ghost :disabled="!record.synced" :href="record.download_url" target="_blank">
                      <FontAwesomeIcon :icon="['fas', 'download']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="删除">
                    <a-button class="delBtn" danger type="primary" :loading="packageRowLoading[record.id]" @click="openPackageDeleteConfirm(record)">
                      <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                    </a-button>
                  </a-tooltip>
                </a-space>
              </template>
            </template>
          </a-table>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <a-modal
      title="自动更新（从官方源下载）"
      :open="syncModalVisible"
      :confirm-loading="syncModalSubmitting"
      ok-text="确认更新"
      cancel-text="取消"
      @ok="submitSyncOfficial"
      @cancel="syncModalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="软件包">
          <span>{{ syncModalTarget ? `${syncModalTarget.name} (${syncModalTarget.os}-${syncModalTarget.arch})` : '-' }}</span>
        </a-form-item>
        <a-form-item label="目标版本" required>
          <a-input v-model:value="syncModalVersion" placeholder="例如 1.8.2" />
        </a-form-item>
        <a-alert type="info" show-icon message="将按官方 GitHub Release 命名规则拼接地址下载并覆盖当前文件" />
      </a-form>
    </a-modal>

    <a-modal
      title="编辑软件包"
      :open="packageEditModalVisible"
      :confirm-loading="packageEditModalSubmitting"
      ok-text="保存"
      cancel-text="取消"
      width="640px"
      @ok="submitPackageEdit"
      @cancel="packageEditModalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="软件包">
          <span>{{ packageEditTarget ? `${packageEditTarget.name} (${packageEditTarget.os}-${packageEditTarget.arch})` : '-' }}</span>
        </a-form-item>
        <a-form-item label="安装任务（Playbook）">
          <a-select
            v-model:value="packageEditForm.install_task"
            allow-clear
            show-search
            :loading="playbookTaskOptionsLoading"
            :options="playbookTaskOptions"
            :filter-option="filterTaskOption"
            :get-popup-container="getPopupContainer"
            placeholder="请选择安装该软件包使用的自动化任务"
          />
          <div class="form-item-hint">Playbook 内容在“模板 -&gt; Playbook 模板”中维护</div>
        </a-form-item>
        <a-form-item label="卸载任务（Playbook）">
          <a-select
            v-model:value="packageEditForm.uninstall_task"
            allow-clear
            show-search
            :loading="playbookTaskOptionsLoading"
            :options="playbookTaskOptions"
            :filter-option="filterTaskOption"
            :get-popup-container="getPopupContainer"
            placeholder="请选择卸载该软件包使用的自动化任务"
          />
        </a-form-item>
        <a-form-item label="systemd 服务名">
          <span>{{ packageEditTarget ? packageEditTarget.name + '.service' : '-' }}</span>
        </a-form-item>
        <a-form-item label="systemd unit 文件内容">
          <a-textarea
            v-model:value="packageEditForm.service_file_content"
            :rows="10"
            placeholder="安装 Playbook 中通过 extra_vars.service_file_content 拿到后写入 /usr/lib/systemd/system/<name>.service"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  deleteSoftwarePackage,
  getManagedTargets,
  getMonitorSummary,
  getPrometheusAlerts,
  getPrometheusOverview,
  getPrometheusTargets,
  getSoftwarePackages,
  retryManagedTarget,
  syncSoftwarePackageFromOfficial,
  updateSoftwarePackage,
  uploadSoftwarePackageFile,
} from '@/api/sys/monitor'
import { getTaskList } from '@/api/sys/automation'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const router = useRouter()
// a-select 弹层挂载容器统一复用公共工具，避免每个页面自行处理导致弹层时有时无法正常弹出。
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const loading = ref(false)
const errorMessage = ref('')
const activeTabKey = ref('prom-targets')

const lastRefreshAt = ref(null)
const lastRefreshAtText = computed(() => {
  if (!lastRefreshAt.value) return ''
  return lastRefreshAt.value.toLocaleTimeString('zh-CN', { hour12: false })
})

const prometheusBaseUrl = ref('')
const overview = reactive({ total: 0, up: 0, down: 0 })
const alertSummary = reactive({ firing: 0, resolved: 0 })

const promTargets = ref([])
const alerts = ref([])
const managedTargets = ref([])
const managedRetryLoading = reactive({})

const managedPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

const autoRefreshEnabled = ref(true)
const refreshIntervalSeconds = ref(15)
const refreshIntervalOptions = [
  { label: '5秒', value: 5 },
  { label: '10秒', value: 10 },
  { label: '15秒', value: 15 },
  { label: '30秒', value: 30 },
  { label: '60秒', value: 60 },
]
let refreshTimer = null

const promTargetColumns = [
  { title: 'Job', dataIndex: 'job', key: 'job', width: 140 },
  { title: 'Instance', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: 'Health', dataIndex: 'health', key: 'health', width: 100 },
  { title: 'Scrape Pool', dataIndex: 'scrape_pool', key: 'scrape_pool', width: 180 },
  { title: 'Last Scrape', dataIndex: 'last_scrape', key: 'last_scrape', width: 220 },
  { title: 'Scrape URL', dataIndex: 'scrape_url', key: 'scrape_url', width: 260 },
  { title: 'Last Error', dataIndex: 'last_error', key: 'last_error', width: 260 },
]

const alertColumns = [
  { title: '告警名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '级别', dataIndex: 'severity', key: 'severity', width: 100 },
  { title: '状态', dataIndex: 'state', key: 'state', width: 100 },
  { title: '实例', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: '摘要', dataIndex: 'summary', key: 'summary', width: 360 },
  { title: '激活时间', dataIndex: 'active_at', key: 'active_at', width: 220 },
]

const managedColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 180 },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 160 },
  { title: 'Exporter', dataIndex: 'exporter_type', key: 'exporter_type', width: 150 },
  { title: '纳管', dataIndex: 'managed_enabled', key: 'managed_enabled', width: 90 },
  { title: '安装状态', dataIndex: 'install_status', key: 'install_status', width: 120 },
  { title: '采集状态', dataIndex: 'last_scrape_status', key: 'last_scrape_status', width: 120 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
  { title: '操作', key: 'action', width: 160, fixed: 'right' },
]

const packageColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '名称', dataIndex: 'name', key: 'name', width: 150 },
  { title: '版本', dataIndex: 'version', key: 'version', width: 120 },
  { title: '系统', dataIndex: 'os', key: 'os', width: 100 },
  { title: '架构', dataIndex: 'arch', key: 'arch', width: 100 },
  { title: '大小', dataIndex: 'size_bytes', key: 'size_bytes', width: 110 },
  { title: 'sha256', dataIndex: 'sha256', key: 'sha256', width: 260, ellipsis: true },
  { title: '同步状态', dataIndex: 'synced', key: 'synced', width: 100 },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
  { title: '操作', key: 'action', width: 250, fixed: 'right' },
]

const packages = ref([])
const packagesLoading = ref(false)
const packageUploadLoading = reactive({})
const packageRowLoading = reactive({})
const packageSyncLoading = reactive({})
const syncModalVisible = ref(false)
const syncModalSubmitting = ref(false)
const syncModalTarget = ref(null)
const syncModalVersion = ref('')
const packageEditModalVisible = ref(false)
const packageEditModalSubmitting = ref(false)
const packageEditTarget = ref(null)
const packageEditForm = reactive({ install_task: undefined, uninstall_task: undefined, service_file_content: '' })
// 安装/卸载只能选择 Playbook 类型的自动化任务（dj-agent 仅对 Playbook 任务支持 become 提权，Shell 脚本任务不适用于安装/卸载）
const playbookTaskOptions = ref([])
const playbookTaskOptionsLoading = ref(false)
const packagePagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

function formatSize(bytes) {
  const value = Number(bytes || 0)
  if (value <= 0) return '-'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

// 统一提取接口错误提示：优先取后端 {code,msg,data} 中的 msg，404 单独提示为“记录不存在”
function resolvePackageErrorMessage(error, fallback) {
  if (Number(error?.response?.status) === 404) {
    return '记录不存在，可能已被删除，列表已刷新'
  }
  return error?.response?.data?.msg || error?.message || fallback
}

// 解析 node_exporter 官方 tarball 命名：node_exporter-<version>.<os>-<arch>.tar.gz
function parsePackageFilename(filename) {
  const match = /^node_exporter-([^.]+)\.([a-z0-9]+)-([a-z0-9]+)\.tar\.gz$/i.exec(filename || '')
  if (!match) return null
  return { version: match[1], os: match[2].toLowerCase(), arch: match[3].toLowerCase() }
}

async function loadPackages() {
  packagesLoading.value = true
  try {
    const res = await getSoftwarePackages({
      page: packagePagination.current,
      page_size: packagePagination.pageSize,
      ordering: '-id',
    })
    const data = parseApiData(res)
    packages.value = Array.isArray(data.results) ? data.results : []
    packagePagination.total = Number(data.count || 0)
  } catch (error) {
    message.warning(error?.message || '加载软件仓库失败')
  } finally {
    packagesLoading.value = false
  }
}

// 首次打开软件仓库页面时，默认软件包由后端一次性数据迁移预置，前端不再自动调用预置接口，
// 避免用户删除后因每次进页自动重建而“删不掉”。

async function loadPlaybookTaskOptions() {
  playbookTaskOptionsLoading.value = true
  try {
    const res = await getTaskList({ page: 1, page_size: 200, ordering: '-id' })
    const data = parseApiData(res)
    const records = Array.isArray(data.results) ? data.results : []
    // 只保留 playbook_template 非空的任务（Playbook 类型），与后端 dispatch_exporter_install_job/
    // uninstall_job 仅支持 playbook 任务的限制保持一致。
    playbookTaskOptions.value = records
      .filter((item) => item?.playbook_template)
      .map((item) => ({
        label: `${item.name}${item.enabled ? '' : '（已禁用）'}`,
        value: item.id,
      }))
  } catch (error) {
    message.warning(error?.response?.data?.msg || error?.message || '加载自动化任务列表失败')
  } finally {
    playbookTaskOptionsLoading.value = false
  }
}

function filterTaskOption(input, option) {
  return String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())
}

function openSyncOfficialModal(record) {
  syncModalTarget.value = record
  syncModalVersion.value = String(record.version || '')
  syncModalVisible.value = true
}

function openPackageEditModal(record) {
  packageEditTarget.value = record
  packageEditForm.install_task = record.install_task || undefined
  packageEditForm.uninstall_task = record.uninstall_task || undefined
  packageEditForm.service_file_content = record.service_file_content || ''
  packageEditModalVisible.value = true
  if (!playbookTaskOptions.value.length) {
    loadPlaybookTaskOptions()
  }
}

async function submitPackageEdit() {
  const record = packageEditTarget.value
  if (!record) return
  packageEditModalSubmitting.value = true
  try {
    await updateSoftwarePackage(record.id, {
      install_task: packageEditForm.install_task ?? null,
      uninstall_task: packageEditForm.uninstall_task ?? null,
      service_file_content: packageEditForm.service_file_content,
    })
    message.success('保存成功')
    packageEditModalVisible.value = false
    await loadPackages()
  } catch (error) {
    message.error(resolvePackageErrorMessage(error, '保存失败'))
  } finally {
    packageEditModalSubmitting.value = false
  }
}

async function submitSyncOfficial() {
  const record = syncModalTarget.value
  const version = syncModalVersion.value.trim()
  if (!record || !version) {
    message.error('请输入目标版本')
    return
  }
  syncModalSubmitting.value = true
  packageSyncLoading[record.id] = true
  try {
    await syncSoftwarePackageFromOfficial(record.id, version)
    message.success('自动更新成功')
    syncModalVisible.value = false
  } catch (error) {
    message.error(resolvePackageErrorMessage(error, '自动更新失败'))
  } finally {
    syncModalSubmitting.value = false
    packageSyncLoading[record.id] = false
    // 无论成功失败都刷新列表，清除本地过期数据（如记录已被删除导致 404）
    await loadPackages()
  }
}

function handlePackageTableChange(pagination) {
  packagePagination.current = Number(pagination?.current || 1)
  packagePagination.pageSize = Number(pagination?.pageSize || 10)
  loadPackages()
}

function beforePackageUpload(file, record) {
  const filename = String(file?.name || '')
  if (!filename.toLowerCase().endsWith('.tar.gz') && !filename.toLowerCase().endsWith('.tgz')) {
    message.error('仅支持上传 .tar.gz / .tgz 软件包')
    return false
  }
  const parsed = parsePackageFilename(filename)
  if (!parsed) {
    message.error('文件名需符合 node_exporter-<version>.<os>-<arch>.tar.gz 命名规范')
    return false
  }
  // 前端提前校验架构匹配，避免无谓上传后被后端拒绝
  if (parsed.os !== record.os || parsed.arch !== record.arch) {
    message.error(`文件架构（${parsed.os}-${parsed.arch}）与当前记录（${record.os}-${record.arch}）不一致`)
    return false
  }
  return true
}

async function handlePackageUpload(options, record) {
  const file = options?.file
  const parsed = parsePackageFilename(file?.name)
  if (!parsed) {
    options?.onError?.(new Error('invalid filename'))
    return
  }
  const formData = new FormData()
  formData.append('file', file)
  packageUploadLoading[record.id] = true
  try {
    await uploadSoftwarePackageFile(record.id, formData)
    message.success('软件包上传成功')
    await loadPackages()
    options?.onSuccess?.(null, file)
  } catch (error) {
    message.error(resolvePackageErrorMessage(error, '软件包上传失败'))
    options?.onError?.(error)
  } finally {
    packageUploadLoading[record.id] = false
  }
}

function openPackageDeleteConfirm(record) {
  openDeleteConfirm({
    title: '确认删除软件包',
    summary: '删除后将无法从本地仓库下发该软件包，agent 端安装会回退到官方下载源。',
    items: [`${record.name} ${record.version} (${record.os}-${record.arch})`],
    onConfirm: async () => {
      packageRowLoading[record.id] = true
      try {
        await deleteSoftwarePackage(record.id)
        message.success('删除成功')
      } catch (error) {
        // 删除失败（如记录已被其他会话删除导致 404）时只提示，不向上抛出，避免弹窗因 Promise 拒绝卡住/控制台报 Uncaught rejection
        message.error(resolvePackageErrorMessage(error, '删除失败'))
      } finally {
        packageRowLoading[record.id] = false
        // 无论成功失败都刷新列表，清除本地过期数据
        await loadPackages()
      }
    },
  })
}

function statusColor(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'success') return 'green'
  if (value === 'failed') return 'red'
  if (value === 'pending') return 'orange'
  return 'default'
}

function scrapeColor(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'up') return 'green'
  if (value === 'down') return 'red'
  return 'default'
}

function severityColor(severity) {
  const value = String(severity || '').toLowerCase()
  if (value === 'critical') return 'red'
  if (value === 'warning') return 'orange'
  return 'default'
}

function alertRowKey(record) {
  return `${record.name || '-'}:${record.instance || '-'}:${record.active_at || '-'}`
}

function parseApiData(resp) {
  return resp?.data?.data || {}
}

async function loadMonitorSummary() {
  const res = await getMonitorSummary()
  const data = parseApiData(res)
  const targets = data.targets || {}
  return {
    total: Number(targets.total || 0),
    managedEnabled: Number(targets.managed_enabled || 0),
    scrapeUp: Number(targets.scrape_up || 0),
  }
}

async function loadPromOverview() {
  const res = await getPrometheusOverview()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus overview 查询失败')
  }
  const targets = data.targets || {}
  return {
    baseUrl: String(data.prometheus_base_url || ''),
    total: Number(targets.total || 0),
    up: Number(targets.up || 0),
    down: Number(targets.down || 0),
  }
}

async function loadPromTargets() {
  const res = await getPrometheusTargets()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus targets 查询失败')
  }
  return Array.isArray(data.results) ? data.results : []
}

async function loadPromAlerts() {
  const res = await getPrometheusAlerts()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus alerts 查询失败')
  }
  return {
    firingCount: Number(data.firing_count || 0),
    resolvedCount: Number(data.resolved_count || 0),
    rows: Array.isArray(data.results) ? data.results : [],
  }
}

async function loadManagedTargets() {
  const res = await getManagedTargets({
    page: managedPagination.current,
    page_size: managedPagination.pageSize,
    ordering: '-id',
  })
  const data = parseApiData(res)
  managedTargets.value = Array.isArray(data.results) ? data.results : []
  managedPagination.total = Number(data.count || 0)
}

async function loadAllData() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [summaryPayload, overviewPayload, targetsPayload, alertsPayload] = await Promise.all([
      loadMonitorSummary(),
      loadPromOverview(),
      loadPromTargets(),
      loadPromAlerts(),
    ])

    overview.total = overviewPayload.total
    overview.up = overviewPayload.up
    overview.down = overviewPayload.down
    prometheusBaseUrl.value = overviewPayload.baseUrl

    promTargets.value = targetsPayload
    alerts.value = alertsPayload.rows
    alertSummary.firing = alertsPayload.firingCount
    alertSummary.resolved = alertsPayload.resolvedCount

    await loadManagedTargets()
    if (overview.total <= 0 && summaryPayload.total > 0) {
      overview.total = summaryPayload.total
      overview.up = summaryPayload.scrapeUp
      overview.down = Math.max(0, summaryPayload.total - summaryPayload.scrapeUp)
    }
    lastRefreshAt.value = new Date()
  } catch (error) {
    const msg = error?.message || '加载监控数据失败'
    errorMessage.value = msg
    message.warning(msg)
  } finally {
    loading.value = false
  }
}

function clearRefreshTimer() {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
}

function restartRefreshTimer() {
  clearRefreshTimer()
  if (!autoRefreshEnabled.value) {
    return
  }
  const intervalMs = Number(refreshIntervalSeconds.value || 15) * 1000
  refreshTimer = window.setInterval(() => {
    if (loading.value) return
    loadAllData()
  }, intervalMs)
}

function handleManagedTableChange(pagination) {
  managedPagination.current = Number(pagination?.current || 1)
  managedPagination.pageSize = Number(pagination?.pageSize || 10)
  loadManagedTargets()
}

async function handleManagedRetry(record) {
  managedRetryLoading[record.id] = true
  try {
    await retryManagedTarget(record.id)
    message.success('已重新下发任务')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '重试失败')
  } finally {
    managedRetryLoading[record.id] = false
    await loadManagedTargets()
  }
}

function openManagedTargetJobLog(record) {
  if (!record.last_install_job_id) return
  // 安装/卸载执行日志统一由“自动化任务”的执行记录页承载，跳转时通过 job_id 定位到具体记录，
  // 与 automation/logs 页面已有的 route.query.job_id 打开逻辑保持一致。
  router.push({ path: '/sys/automation/logs', query: { job_id: record.last_install_job_id } })
}

watch(() => autoRefreshEnabled.value, restartRefreshTimer)
watch(() => refreshIntervalSeconds.value, restartRefreshTimer)

onMounted(async () => {
  await loadAllData()
  await loadPackages()
  restartRefreshTimer()
})

onBeforeUnmount(() => {
  clearRefreshTimer()
})
</script>

<style scoped>
.monitor-page {
  padding: 0;
}

.tools {
  margin-bottom: 12px;
}

.right-actions {
  text-align: right;
}

.prom-url {
  color: #1f1f1f;
}

.monitor-card {
  border-radius: 12px;
}

.overview-grid {
  margin-bottom: 12px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
}

.overview-card {
  border-radius: 12px;
}

.form-item-hint {
  margin-top: 4px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
