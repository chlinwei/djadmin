<template>
  <div class="application-workspace">
    <a-tabs v-model:activeKey="activeTab">
      <a-tab-pane key="systems" tab="业务系统" />
      <a-tab-pane key="environments" tab="环境" />
      <a-tab-pane key="services" tab="逻辑服务" />
      <a-tab-pane key="profiles" tab="集群模型" />
      <a-tab-pane key="applications" tab="应用定义" />
      <a-tab-pane key="templates" tab="部署模板" />
    </a-tabs>

    <a-row :gutter="12" class="tools">
      <a-col flex="360px">
        <a-input-search v-model:value="keyword" :placeholder="searchPlaceholder" allow-clear enter-button @search="reload(true)" />
      </a-col>
      <a-col flex="auto" class="right-tools">
        <a-space>
          <a-button v-permission="'assets:applications:create'" size="large" @click="openCurrentTabCreateDialog">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;{{ createButtonLabel }}</span>
          </a-button>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="loading || runtimeRefreshing" @click="handleManualRefresh">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="loading || runtimeRefreshing" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <a-row v-show="activeTab === 'services'" :gutter="12" class="service-filters">
      <a-col :xs="24" :sm="12" :md="6">
        <a-select v-model:value="serviceFilters.business_system" allow-clear show-search placeholder="业务系统" :options="serviceBusinessSystemOptions" :filter-option="filterOption" :getPopupContainer="getPopupContainer" />
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-select v-model:value="serviceFilters.application" allow-clear show-search placeholder="应用" :options="serviceApplicationOptions" :filter-option="filterOption" :getPopupContainer="getPopupContainer" />
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-select v-model:value="serviceFilters.environment" allow-clear show-search placeholder="环境" :options="serviceEnvironmentOptions" :filter-option="filterOption" :getPopupContainer="getPopupContainer" />
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-select v-model:value="serviceFilters.topology_type" allow-clear placeholder="部署形态" :options="topologyFilterOptions" :getPopupContainer="getPopupContainer" />
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-select v-model:value="serviceFilters.cluster_profile" allow-clear show-search placeholder="集群模型" :options="serviceClusterProfileOptions" :filter-option="filterOption" :getPopupContainer="getPopupContainer" />
      </a-col>
      <a-col :xs="24" :sm="12" :md="6">
        <a-select v-model:value="serviceFilters.enabled" allow-clear placeholder="状态" :options="enabledFilterOptions" :getPopupContainer="getPopupContainer" />
      </a-col>
      <a-col :xs="24" :sm="12" :md="6" class="service-filter-actions">
        <a-button type="primary" @click="reload(true)">查询</a-button>
        <a-button @click="resetServiceFilters">重置</a-button>
      </a-col>
    </a-row>

    <a-table
      :row-key="getWorkspaceRowKey"
      :columns="currentColumns"
      :data-source="rows"
      :loading="loading"
      :pagination="pagination"
      :scroll="currentTableScroll"
      @change="handleTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="activeTab === 'systems' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'systems' && column.key === 'action'">
          <a-space>
            <a-tooltip title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openBusinessSystem(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteBusinessSystem(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'environments' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'environments' && column.key === 'action'">
          <a-space>
            <a-tooltip title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openBusinessEnvironment(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteBusinessEnvironment(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'services' && column.key === 'topology_type'">
          <a-tag :color="record.topology_type === 'cluster' ? 'blue' : record.topology_type === 'load_balancer' ? 'green' : 'default'">{{ record.topology_type === 'cluster' ? '集群' : record.topology_type === 'load_balancer' ? '负载均衡' : '单机' }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'services' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'services' && column.key === 'health_status'">
          <div><a-badge :status="healthStatusMap[record.health_status]?.badge || 'default'" :text="healthStatusMap[record.health_status]?.label || '未检查'" /></div>
          <span class="secondary">{{ formatPassRate(record.baseline_pass_rate) }}</span>
        </template>
        <template v-else-if="activeTab === 'services' && column.key === 'last_check_time'">
          <span>{{ formatDateTime(record.last_check_time) }}</span>
        </template>
        <template v-else-if="activeTab === 'services' && column.key === 'action'">
          <a-space>
            <a-tooltip v-if="record.topology_type === 'cluster'" title="运行">
              <a-button
                v-permission="'assets:applications:update'"
                data-service-baseline
                size="small"
                :loading="checkingServiceId === record.id"
                @click="runClusterBaselineCheck(record)"
              >
                <FontAwesomeIcon :icon="['fas', 'clipboard-check']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="编辑"><a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openService(record)"><FontAwesomeIcon :icon="['fa', 'edit']" /></a-button></a-tooltip>
            <a-tooltip title="删除"><a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteService(record)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'profiles' && column.key === 'profile_type'">
          <a-tag :color="record.profile_type === 'builtin' ? 'blue' : 'orange'">{{ record.profile_type === 'builtin' ? '内置' : '自定义' }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'profiles' && column.key === 'cluster_type'">
          <a-tag>{{ clusterTypeLabels[record.cluster_type] || record.cluster_type }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'profiles' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'profiles' && column.key === 'action'">
          <a-space>
            <a-tooltip title="创建集群">
              <a-button v-permission="'assets:applications:create'" data-create-cluster-profile size="small" @click="createClusterFromProfile(record)">
                <FontAwesomeIcon :icon="['fas', 'diagram-project']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="查看集群">
              <a-button size="small" @click="viewClustersForProfile(record)">
                <FontAwesomeIcon :icon="['fas', 'list']" />
              </a-button>
            </a-tooltip>
            <a-tooltip v-if="record.profile_type === 'custom'" title="编辑"><a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openClusterProfile(record)"><FontAwesomeIcon :icon="['fa', 'edit']" /></a-button></a-tooltip>
            <a-tooltip v-if="record.profile_type === 'custom'" title="删除"><a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteClusterProfile(record)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'applications' && column.key === 'category'">
          <a-tag>{{ categoryLabels[record.category] || record.category }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'applications' && column.key === 'versions'">
          <a-space wrap>
            <a-tag v-for="version in record.versions" :key="version.id" color="blue">{{ version.version }}</a-tag>
            <span v-if="!record.versions?.length">-</span>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'applications' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'applications' && column.key === 'action'">
          <a-space>
            <a-tooltip key="application-edit" title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openApplication(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="application-versions" title="版本管理">
              <a-button size="small" @click="openVersions(record)">
                <FontAwesomeIcon :icon="['fas', 'code-branch']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="application-delete" title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteApplication(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'templates' && column.key === 'control_type'">
          <a-tag :color="controlTypeColors[record.control_type]">{{ controlTypeLabels[record.control_type] || record.control_type }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'templates' && column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="activeTab === 'templates' && column.key === 'action'">
          <a-space>
            <a-tooltip key="template-edit" title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openTemplate(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="template-copy" title="复制">
              <a-button v-permission="'assets:applications:create'" size="small" @click="copyTemplate(record)">
                <FontAwesomeIcon :icon="['fas', 'copy']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="template-delete" title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteTemplate(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'host'">
          <div>{{ record.host_name || '-' }}</div>
          <span class="secondary">{{ record.host_ip || '-' }}</span>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'control_type'">
          <a-tag :color="controlTypeColors[record.control_type]">{{ controlTypeLabels[record.control_type] || record.control_type }}</a-tag>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'runtime_status'">
          <div class="runtime-status-line">
            <a-badge :status="runtimeStatusMap[record.runtime_status]?.badge || 'default'" :text="runtimeStatusMap[record.runtime_status]?.label || '未知'" />
            <a-tooltip v-if="record.runtime_status === 'error' && record.runtime_status_output" placement="top" :title="record.runtime_status_output">
              <InfoCircleOutlined class="runtime-error-detail" aria-label="查看检查失败原因" />
            </a-tooltip>
          </div>
          <span class="secondary">{{ formatDateTime(record.last_status_check_time) }}</span>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'health_status'">
          <div><a-badge :status="healthStatusMap[record.health_status]?.badge || 'default'" :text="healthStatusMap[record.health_status]?.label || '未检查'" /></div>
          <span class="secondary">{{ formatPassRate(record.baseline_pass_rate) }}</span>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'last_check_time'">
          <span>{{ formatDateTime(record.last_check_time) }}</span>
        </template>
        <template v-else-if="activeTab === 'deployments' && column.key === 'action'">
          <a-space>
            <a-tooltip v-if="record.control_type !== 'external_ha'" key="deployment-start" title="启动">
              <a-button
                v-permission="'assets:applications:update'"
                data-control-action="start"
                size="small"
                type="primary"
                ghost
                :loading="isControlLoading(record.id, 'start')"
                :disabled="isDeploymentBusy(record.id)"
                @click="confirmApplicationControl(record, 'start')"
              >
                <FontAwesomeIcon :icon="['fas', 'play']" />
              </a-button>
            </a-tooltip>
            <a-tooltip v-if="record.control_type !== 'external_ha'" key="deployment-stop" title="停止">
              <a-button
                v-permission="'assets:applications:update'"
                data-control-action="stop"
                size="small"
                danger
                :loading="isControlLoading(record.id, 'stop')"
                :disabled="isDeploymentBusy(record.id)"
                @click="confirmApplicationControl(record, 'stop')"
              >
                <FontAwesomeIcon :icon="['fas', 'stop']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="deployment-baseline" title="基线检查">
              <a-button
                v-permission="'assets:applications:update'"
                data-control-action="baseline"
                size="small"
                type="primary"
                :loading="checkingDeploymentId === record.id"
                :disabled="record.health_status === 'checking' || isDeploymentBusy(record.id)"
                @click="runBaselineCheck(record)"
              >
                <FontAwesomeIcon :icon="['fas', 'clipboard-check']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="deployment-history" title="历史记录">
              <a-button v-permission="'assets:applications:view'" size="small" @click="openBaselineHistory(record)">
                <FontAwesomeIcon :icon="['fas', 'list']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="deployment-edit" title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openDeployment(record)">
                <FontAwesomeIcon :icon="['fa', 'edit']" />
              </a-button>
            </a-tooltip>
            <a-tooltip key="deployment-delete" title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteDeployment(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
      </template>
    </a-table>

    <BusinessSystemDialog
      :open="businessSystemDialogOpen"
      :system-id="selectedBusinessSystemId"
      @update:open="businessSystemDialogOpen = $event"
      @saved="handleBusinessSystemSaved"
    />
    <BusinessEnvironmentDialog
      :open="businessEnvironmentDialogOpen"
      :environment-id="selectedBusinessEnvironmentId"
      :business-system-id="serviceScope.businessSystemId || null"
      @update:open="businessEnvironmentDialogOpen = $event"
      @saved="handleBusinessSystemSaved"
    />
    <ApplicationServiceDialog
      :open="serviceDialogOpen"
      :service-id="selectedServiceId"
      :cluster-profile-id="selectedServiceClusterProfileId"
      @update:open="serviceDialogOpen = $event"
      @saved="handleServiceSaved"
    />
    <ClusterProfileDialog
      :open="clusterProfileDialogOpen"
      :profile-id="selectedClusterProfileId"
      @update:open="clusterProfileDialogOpen = $event"
      @saved="handleTopologySaved"
    />
    <Dialog
      :open="applicationDialogOpen"
      :item_id="selectedApplication?.id || -1"
      :title="selectedApplication ? `编辑-${selectedApplication.name}` : '新增应用'"
      appname="应用"
      @update:open="applicationDialogOpen = $event"
      @initList="reload(false)"
    />
    <VersionDialog
      :open="versionDialogOpen"
      :application="selectedApplication"
      @update:open="versionDialogOpen = $event"
      @changed="reload(false)"
    />
    <TemplateDialog
      :open="templateDialogOpen"
      :template-id="selectedTemplateId"
      :copy-from-id="selectedTemplateCopyId"
      @update:open="templateDialogOpen = $event"
      @saved="reload(false)"
    />
    <DeploymentDialog
      :open="deploymentDialogOpen"
      :deployment-id="selectedDeployment?.id || null"
      @update:open="deploymentDialogOpen = $event"
      @saved="handleDeploymentSaved"
    />
    <a-modal
      v-model:open="historyDialogOpen"
      :title="`${historyDeployment?.instance_name || ''} - 基线检查历史`"
      width="1080px"
      :footer="null"
      destroy-on-close
    >
      <a-table
        row-key="id"
        :columns="historyColumns"
        :data-source="baselineHistory"
        :loading="historyLoading"
        :pagination="false"
        :scroll="{ x: 920 }"
        :expand-row-by-click="true"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="executionStatusMap[record.status]?.color || 'default'">
              {{ executionStatusMap[record.status]?.label || record.status }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'passed'">
            <a-tag v-if="record.passed === true" color="success">通过</a-tag>
            <a-tag v-else-if="record.passed === false" color="error">未通过</a-tag>
            <span v-else>-</span>
          </template>
          <template v-else-if="column.key === 'count'">
            {{ record.passed_count }}/{{ record.total_count }}
          </template>
          <template v-else-if="column.key === 'create_time'">
            {{ formatDateTime(record.create_time) }}
          </template>
        </template>
        <template #expandedRowRender="{ record }">
          <a-alert v-if="record.error_message" type="error" :message="record.error_message" show-icon class="history-error" />
          <a-table
            v-else
            row-key="id"
            size="small"
            :columns="resultColumns"
            :data-source="record.results || []"
            :pagination="false"
            :scroll="{ x: 940 }"
          >
            <template #bodyCell="{ column, record: result }">
              <template v-if="column.key === 'status'">
                <a-tag :color="resultStatusMap[result.status]?.color || 'default'">
                  {{ resultStatusMap[result.status]?.label || result.status }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'expected_value'"><pre class="check-value">{{ formatCheckValue(result.expected_value) }}</pre></template>
              <template v-else-if="column.key === 'actual_value'">
                <div v-if="getMissingCheckValues(result.actual_value).length" class="check-missing">
                  <div v-for="item in getMissingCheckValues(result.actual_value)" :key="item">缺少 {{ item }}</div>
                </div>
                <pre class="check-value">{{ formatActualCheckValue(result) }}</pre>
              </template>
            </template>
          </a-table>
        </template>
      </a-table>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { InfoCircleOutlined } from '@ant-design/icons-vue'
import store from '@/store'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import { formatTimeWithTimezone } from '@/util/timezone'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatActualCheckValue, formatCheckValue } from './baselineValue'
import {
  batchDeleteApplication,
  checkApplicationServiceBaseline,
  checkApplicationDeploymentBaseline,
  controlApplicationDeployment,
  deleteBusinessSystem,
  deleteBusinessEnvironment,
  deleteApplicationService,
  deleteClusterProfile,
  deleteApplicationDeployment,
  deleteApplicationDeploymentTemplate,
  getBusinessSystemList,
  getBusinessEnvironmentList,
  getApplicationServiceList,
  getClusterProfileList,
  getApplicationDeploymentBaselineHistory,
  getApplicationDeploymentList,
  getApplicationDeploymentTemplateList,
  getApplicationList,
} from '@/api/assets/application'
import Dialog from './Dialog.vue'
import VersionDialog from './VersionDialog.vue'
import TemplateDialog from './TemplateDialog.vue'
import DeploymentDialog from './DeploymentDialog.vue'
import BusinessSystemDialog from './BusinessSystemDialog.vue'
import BusinessEnvironmentDialog from './BusinessEnvironmentDialog.vue'
import ApplicationServiceDialog from './ApplicationServiceDialog.vue'
import ClusterProfileDialog from './ClusterProfileDialog.vue'

const props = defineProps({
  serviceScope: { type: Object, default: () => ({}) },
})
const emit = defineEmits(['data-changed'])
const activeTab = ref('applications')
const keyword = ref('')
const serviceFilters = reactive({ business_system: undefined, application: undefined, environment: undefined, topology_type: undefined, cluster_profile: undefined, enabled: undefined })
const serviceFilterRecords = reactive({ businessSystems: [], applications: [], environments: [], profiles: [] })
const rows = ref([])
const loading = ref(false)
const businessSystemDialogOpen = ref(false)
const selectedBusinessSystemId = ref(null)
const businessEnvironmentDialogOpen = ref(false)
const selectedBusinessEnvironmentId = ref(null)
const serviceDialogOpen = ref(false)
const selectedServiceId = ref(null)
const selectedServiceClusterProfileId = ref(null)
const clusterProfileDialogOpen = ref(false)
const selectedClusterProfileId = ref(null)
const applicationDialogOpen = ref(false)
const versionDialogOpen = ref(false)
const templateDialogOpen = ref(false)
const deploymentDialogOpen = ref(false)
const historyDialogOpen = ref(false)
const historyLoading = ref(false)
const baselineHistory = ref([])
const historyDeployment = ref(null)
const checkingDeploymentId = ref(null)
const checkingServiceId = ref(null)
const selectedApplication = ref(null)
const selectedDeployment = ref(null)
const selectedTemplateId = ref(null)
const selectedTemplateCopyId = ref(null)
const runtimeRefreshing = ref(false)
const deploymentControlLoading = reactive({})
const paginationState = reactive({ current: 1, pageSize: 10, total: 0 })
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const categoryLabels = { web_container: 'Web 容器', database: '数据库', middleware: '中间件', business: '业务应用', other: '其他' }
const controlTypeLabels = { systemd: 'Systemd', command: '命令行', external_ha: '外部 HA', docker: 'Docker', docker_compose: 'Docker Compose' }
const clusterTypeLabels = { mysql: 'MySQL 集群', redis: 'Redis 集群', nacos: 'Nacos 集群', elasticsearch: 'Elasticsearch 集群', ha: 'HA 集群', custom: '自定义集群' }
const controlTypeColors = { systemd: 'blue', command: 'orange', external_ha: 'gold', docker: 'cyan', docker_compose: 'geekblue' }
const healthStatusMap = {
  unknown: { label: '未检查', badge: 'default' },
  checking: { label: '检查中', badge: 'processing' },
  healthy: { label: '正常', badge: 'success' },
  unhealthy: { label: '异常', badge: 'error' },
  error: { label: '检查失败', badge: 'warning' },
}
const runtimeStatusMap = {
  unknown: { label: '未知', badge: 'default' },
  running: { label: '运行中', badge: 'success' },
  stopped: { label: '已停止', badge: 'error' },
  error: { label: '检查失败', badge: 'warning' },
}
const executionStatusMap = {
  queued: { label: '等待中', color: 'default' },
  running: { label: '检查中', color: 'processing' },
  completed: { label: '已完成', color: 'blue' },
  failed: { label: '执行失败', color: 'error' },
}
const resultStatusMap = {
  pass: { label: '通过', color: 'success' },
  fail: { label: '失败', color: 'error' },
  error: { label: '错误', color: 'warning' },
  skipped: { label: '跳过', color: 'default' },
}
const businessSystemColumns = [
  { title: '业务系统名称', dataIndex: 'name', key: 'name', sorter: true, width: 200 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 180 },
  { title: '负责人', dataIndex: 'owner', key: 'owner', width: 150 },
  { title: '部署数', dataIndex: 'deployment_count', key: 'deployment_count', width: 100 },
  { title: '状态', key: 'enabled', width: 100 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 240 },
  { title: '操作', key: 'action', width: 120, fixed: 'right' },
]
const businessEnvironmentColumns = [
  { title: '环境名称', dataIndex: 'name', key: 'name', sorter: true, width: 180 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 160 },
  { title: '业务系统', dataIndex: 'business_system_name', key: 'business_system_name', width: 180 },
  { title: '顺序', dataIndex: 'order', key: 'order', width: 90 },
  { title: '负责人', dataIndex: 'owner', key: 'owner', width: 140 },
  { title: '逻辑服务', dataIndex: 'service_count', key: 'service_count', width: 110 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 110 },
  { title: '状态', key: 'enabled', width: 100 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 220 },
  { title: '操作', key: 'action', width: 120, fixed: 'right' },
]
const serviceColumns = [
  { title: '服务名称', dataIndex: 'name', key: 'name', sorter: true, width: 190 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 170 },
  { title: '业务系统', dataIndex: 'business_system_name', key: 'business_system_name', width: 160 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 160 },
  { title: '环境', dataIndex: 'environment_name', key: 'environment_name', width: 120 },
  { title: '形态', key: 'topology_type', width: 90 },
  { title: '集群模型', dataIndex: 'cluster_profile_name', key: 'cluster_profile_name', width: 160 },
  { title: '实例数', dataIndex: 'deployment_count', key: 'deployment_count', width: 90 },
  { title: '集群健康', key: 'health_status', width: 130 },
  { title: '最后检查', key: 'last_check_time', width: 170 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '操作', key: 'action', width: 160, fixed: 'right' },
]
const clusterProfileColumns = [
  { title: '模型名称', dataIndex: 'name', key: 'name', sorter: true, width: 190 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 170 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 160 },
  { title: '类型', key: 'profile_type', width: 100 },
  { title: '集群类型', key: 'cluster_type', width: 180 },
  { title: '已建集群', dataIndex: 'service_count', key: 'service_count', width: 100 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '操作', key: 'action', width: 160, fixed: 'right' },
]
const applicationColumns = [
  { title: '应用名称', dataIndex: 'name', key: 'name', sorter: true, width: 180 },
  { title: '编码', dataIndex: 'code', key: 'code', width: 150 },
  { title: '类别', key: 'category', width: 120 },
  { title: '厂商', dataIndex: 'vendor', key: 'vendor', width: 150 },
  { title: '版本', key: 'versions', width: 240 },
  { title: '部署数', dataIndex: 'deployment_count', key: 'deployment_count', width: 90 },
  { title: '模板数', dataIndex: 'deployment_template_count', key: 'deployment_template_count', width: 90 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '备注', dataIndex: 'remark', key: 'remark' },
  { title: '操作', key: 'action', width: 300, fixed: 'right' },
]
const deploymentColumns = [
  { title: '实例名称', dataIndex: 'instance_name', key: 'instance_name', sorter: true, width: 180 },
  { title: '业务系统', dataIndex: 'business_system_name', key: 'business_system_name', width: 160 },
  { title: '应用', dataIndex: 'application_name', key: 'application_name', width: 160 },
  { title: '版本', dataIndex: 'version', key: 'version', width: 120 },
  { title: '主机', key: 'host', width: 190 },
  { title: '环境', dataIndex: 'environment_name', key: 'environment_name', width: 120 },
  { title: '部署模板', dataIndex: 'template_name', key: 'template_name', width: 180 },
  { title: '控制方式', key: 'control_type', width: 150 },
  { title: '运行状态', key: 'runtime_status', width: 160 },
  { title: '健康状态', key: 'health_status', width: 130 },
  { title: '最后检查', key: 'last_check_time', width: 170 },
  { title: '操作', key: 'action', width: 190, fixed: 'right' },
]
const templateColumns = [
  { title: '模板名称', dataIndex: 'name', key: 'name', sorter: true, width: 220 },
  { title: '所属应用', dataIndex: 'application_name', key: 'application_name', width: 180 },
  { title: '控制方式', key: 'control_type', width: 150 },
  { title: '运行用户', dataIndex: 'run_user', key: 'run_user', width: 130 },
  { title: 'App Home', dataIndex: 'app_home', key: 'app_home', width: 280 },
  { title: '服务名称', dataIndex: 'service_name', key: 'service_name', width: 180 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 220 },
  { title: '操作', key: 'action', width: 120, fixed: 'right' },
]
const historyColumns = [
  { title: '状态', key: 'status', width: 110 },
  { title: '结论', key: 'passed', width: 90 },
  { title: '通过项', key: 'count', width: 100 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 120 },
  { title: '任务 ID', dataIndex: 'job_id', key: 'job_id', width: 230 },
  { title: '检查时间', key: 'create_time', width: 170 },
]
const resultColumns = [
  { title: '检查项', dataIndex: 'name', key: 'name', width: 180 },
  { title: '类型', dataIndex: 'check_type', key: 'check_type', width: 150 },
  { title: '状态', key: 'status', width: 90 },
  { title: '期望值', key: 'expected_value', width: 240 },
  { title: '实际值', key: 'actual_value', width: 240 },
  { title: '说明', dataIndex: 'message', key: 'message', width: 220 },
]
const currentColumns = computed(() => ({ systems: businessSystemColumns, environments: businessEnvironmentColumns, services: serviceColumns, profiles: clusterProfileColumns, applications: applicationColumns, templates: templateColumns, deployments: deploymentColumns }[activeTab.value]))
const currentTableScroll = computed(() => ({ x: ({ systems: 1050, environments: 1410, services: 1800, profiles: 1220, applications: 1100, templates: 1570, deployments: 1740 }[activeTab.value]) }))
const createButtonLabel = computed(() => ({ systems: '新增业务系统', environments: '新增环境', services: '新增逻辑服务', profiles: '新增自定义集群', applications: '新增应用', templates: '新增模板', deployments: '登记实例' }[activeTab.value]))
const searchPlaceholder = computed(() => ({ systems: '搜索业务系统、编码或负责人', environments: '搜索环境、编码或业务系统', services: '搜索服务、系统或应用', profiles: '搜索集群模型、编码或应用', applications: '搜索应用、编码或厂商', templates: '搜索模板、应用或服务名', deployments: '搜索应用、版本、主机或实例' }[activeTab.value]))
const serviceBusinessSystemOptions = computed(() => serviceFilterRecords.businessSystems.map((item) => ({ label: item.name, value: item.id })))
const serviceApplicationOptions = computed(() => serviceFilterRecords.applications.map((item) => ({ label: item.name, value: item.id })))
const serviceEnvironmentOptions = computed(() => serviceFilterRecords.environments
  .filter((item) => !serviceFilters.business_system || item.business_system === serviceFilters.business_system)
  .map((item) => ({ label: item.name, value: item.id })))
const serviceClusterProfileOptions = computed(() => serviceFilterRecords.profiles.map((item) => ({ label: item.name, value: item.id })))
const topologyFilterOptions = [{ label: '单机', value: 'standalone' }, { label: '集群', value: 'cluster' }, { label: '负载均衡', value: 'load_balancer' }]
const enabledFilterOptions = [{ label: '启用', value: true }, { label: '停用', value: false }]
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())
let baselinePollTimer = null
let runtimePollTimer = null
let runtimePollInFlight = false
let reloadSequence = 0
const runtimePollIntervalMs = 10000
const pagination = computed(() => ({
  current: paginationState.current,
  pageSize: paginationState.pageSize,
  total: paginationState.total,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
}))

async function reload(resetPage = false) {
  if (resetPage) paginationState.current = 1
  const requestedTab = activeTab.value
  const currentSequence = ++reloadSequence
  loading.value = true
  try {
    const params = { page: paginationState.current, page_size: paginationState.pageSize, search: keyword.value }
    if (requestedTab === 'services') {
      Object.entries(serviceFilters).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') params[key] = value
      })
    }
    if (requestedTab === 'systems' && props.serviceScope.nodeType === 'businessSystem') {
      params.id = props.serviceScope.businessSystemId
    }
    if (requestedTab === 'environments' && props.serviceScope.businessSystemId) {
      params.business_system = props.serviceScope.businessSystemId
    }
    if (requestedTab === 'deployments') {
      if (props.serviceScope.deploymentId) params.id = props.serviceScope.deploymentId
      else if (props.serviceScope.applicationServiceId) params.application_service = props.serviceScope.applicationServiceId
      else if (props.serviceScope.businessSystemId) {
        params.application_service__business_system = props.serviceScope.businessSystemId
      }
      if (props.serviceScope.environment) {
        params.application_service__environment = props.serviceScope.environment
      }
    }
    const listRequests = {
      systems: getBusinessSystemList,
      environments: getBusinessEnvironmentList,
      services: getApplicationServiceList,
      profiles: getClusterProfileList,
      applications: getApplicationList,
      templates: getApplicationDeploymentTemplateList,
      deployments: getApplicationDeploymentList,
    }
    const response = await listRequests[requestedTab](params)
    if (currentSequence !== reloadSequence || activeTab.value !== requestedTab) return
    const data = response?.data?.data || {}
    rows.value = data.results || []
    paginationState.total = Number(data.count || 0)
  } finally {
    if (currentSequence === reloadSequence) loading.value = false
  }
}

function stopRuntimePolling() {
  if (runtimePollTimer) clearInterval(runtimePollTimer)
  runtimePollTimer = null
}
function scheduleRuntimePolling() {
  stopRuntimePolling()
  if (activeTab.value !== 'deployments') return
  runtimePollTimer = setInterval(() => {
    void refreshVisibleRuntimeStatuses({ background: true })
  }, runtimePollIntervalMs)
}
async function refreshVisibleRuntimeStatuses({ background = false } = {}) {
  if (activeTab.value !== 'deployments' || (background && runtimePollInFlight)) return
  const deployments = rows.value.filter((record) => !isDeploymentBusy(record.id))
  if (deployments.length === 0) return
  if (background) runtimePollInFlight = true
  else runtimeRefreshing.value = true
  try {
    for (let index = 0; index < deployments.length; index += 3) {
      if (activeTab.value !== 'deployments') return
      const batch = deployments.slice(index, index + 3)
      await Promise.allSettled(batch.map((record) => controlApplicationDeployment(
        record.id,
        'status',
        { suppressBusinessErrorMessage: true },
      )))
    }
    if (activeTab.value === 'deployments') await reload(false)
  } finally {
    if (background) runtimePollInFlight = false
    else runtimeRefreshing.value = false
  }
}
async function startRuntimePolling() {
  stopRuntimePolling()
  await refreshVisibleRuntimeStatuses({ background: true })
  scheduleRuntimePolling()
}
async function handleManualRefresh() {
  if (activeTab.value === 'deployments') {
    stopRuntimePolling()
    await refreshVisibleRuntimeStatuses()
    scheduleRuntimePolling()
    return
  }
  await reload(false)
}
async function loadActiveTab(nextTab) {
  keyword.value = ''
  if (nextTab === 'services') await loadServiceFilterOptions()
  rows.value = []
  paginationState.total = 0
  await reload(true)
  if (nextTab === 'deployments') startRuntimePolling()
}
async function loadServiceFilterOptions() {
  const [systems, applications, environments, profiles] = await Promise.all([
    getBusinessSystemList({ page: 1, page_size: 1000, enabled: true }),
    getApplicationList({ page: 1, page_size: 1000, enabled: true }),
    getBusinessEnvironmentList({ page: 1, page_size: 1000, enabled: true }),
    getClusterProfileList({ page: 1, page_size: 1000, enabled: true }),
  ])
  serviceFilterRecords.businessSystems = systems?.data?.data?.results || []
  serviceFilterRecords.applications = applications?.data?.data?.results || []
  serviceFilterRecords.environments = environments?.data?.data?.results || []
  serviceFilterRecords.profiles = profiles?.data?.data?.results || []
}
function resetServiceFilters() {
  Object.keys(serviceFilters).forEach((key) => { serviceFilters[key] = undefined })
  reload(true)
}
function getWorkspaceRowKey(record) {
  return `${activeTab.value}-${record.id}`
}
async function handleTableChange(page) {
  paginationState.current = page.current
  paginationState.pageSize = page.pageSize
  await reload(false)
  if (activeTab.value === 'deployments') startRuntimePolling()
}
function openApplication(record = null) {
  selectedApplication.value = record
  applicationDialogOpen.value = true
}
function openBusinessSystem(record = null) {
  selectedBusinessSystemId.value = record?.id || null
  businessSystemDialogOpen.value = true
}
function openBusinessEnvironment(record = null) {
  selectedBusinessEnvironmentId.value = record?.id || null
  businessEnvironmentDialogOpen.value = true
}
function openService(record = null) {
  selectedServiceId.value = record?.id || null
  selectedServiceClusterProfileId.value = null
  serviceDialogOpen.value = true
}
function createClusterFromProfile(record) {
  selectedServiceId.value = null
  selectedServiceClusterProfileId.value = record.id
  serviceDialogOpen.value = true
}
function viewClustersForProfile(record) {
  serviceFilters.cluster_profile = record.id
  activeTab.value = 'services'
}
function openClusterProfile(record = null) {
  selectedClusterProfileId.value = record?.id || null
  clusterProfileDialogOpen.value = true
}
async function handleTopologySaved() {
  await reload(false)
  emit('data-changed')
}
async function handleServiceSaved() {
  emit('data-changed')
  if (selectedServiceClusterProfileId.value) {
    selectedServiceClusterProfileId.value = null
    activeTab.value = 'services'
    return
  }
  await reload(false)
}
async function runClusterBaselineCheck(record) {
  checkingServiceId.value = record.id
  try {
    const response = await checkApplicationServiceBaseline(record.id)
    const passRate = response?.data?.data?.baseline_pass_rate
    message.success(`集群基线检查完成，通过率 ${Number(passRate || 0).toFixed(1)}%`)
    await reload(false)
  } finally {
    checkingServiceId.value = null
  }
}
async function handleBusinessSystemSaved() {
  await reload(false)
  emit('data-changed')
}
async function handleDeploymentSaved() {
  await reload(false)
  emit('data-changed')
}
function openVersions(record) {
  selectedApplication.value = record
  versionDialogOpen.value = true
}
function openTemplate(record = null) {
  selectedTemplateId.value = record?.id || null
  selectedTemplateCopyId.value = null
  templateDialogOpen.value = true
}
function copyTemplate(record) {
  selectedTemplateId.value = null
  selectedTemplateCopyId.value = record?.id || null
  templateDialogOpen.value = true
}
function openDeployment(record = null) {
  selectedDeployment.value = record
  deploymentDialogOpen.value = true
}
function openCurrentTabCreateDialog() {
  if (activeTab.value === 'systems') openBusinessSystem()
  else if (activeTab.value === 'environments') openBusinessEnvironment()
  else if (activeTab.value === 'services') openService()
  else if (activeTab.value === 'profiles') openClusterProfile()
  else if (activeTab.value === 'applications') openApplication()
  else if (activeTab.value === 'templates') openTemplate()
  // 部署实例必须归属逻辑服务才能拿到版本与模板，因此只能从逻辑服务内新增。
  else openService()
}
function confirmDeleteService(record) {
  openDeleteConfirm({
    title: '删除逻辑服务',
    summary: '仍包含部署实例的逻辑服务不能删除。',
    items: [record.name || record.code || record.id],
    onConfirm: async () => {
      await deleteApplicationService(record.id)
      message.success('删除成功')
      await handleTopologySaved()
    },
  })
}
function confirmDeleteClusterProfile(record) {
  openDeleteConfirm({
    title: '删除集群模型',
    summary: '仍被逻辑服务引用的集群模型不能删除。',
    items: [record.name || record.code || record.id],
    onConfirm: async () => {
      await deleteClusterProfile(record.id)
      message.success('删除成功')
      await handleTopologySaved()
    },
  })
}
function confirmDeleteBusinessSystem(record) {
  openDeleteConfirm({
    title: '删除业务系统',
    summary: '系统下的环境会一并删除，仍包含逻辑服务的系统不能删除。',
    items: [record.name || record.code || record.id],
    onConfirm: async () => {
      await deleteBusinessSystem(record.id)
      message.success('删除成功')
      await reload(false)
      emit('data-changed')
    },
  })
}
function confirmDeleteBusinessEnvironment(record) {
  openDeleteConfirm({
    title: '删除环境',
    summary: '仍包含逻辑服务的环境不能删除。',
    items: [`${record.business_system_name || '-'} / ${record.name || record.code || record.id}`],
    onConfirm: async () => {
      await deleteBusinessEnvironment(record.id)
      message.success('删除成功')
      await reload(false)
      emit('data-changed')
    },
  })
}
function formatDateTime(value) {
  return value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
}
function formatPassRate(value) {
  return value === null || value === undefined ? '通过率 -' : `通过率 ${Number(value).toFixed(1)}%`
}
function getMissingCheckValues(value) {
  if (!value || typeof value !== 'object' || !Array.isArray(value.elements)) return []
  return value.elements.flatMap((element) => Object.entries(element || {}).flatMap(([attribute, details]) => {
    if (!Array.isArray(details?.missing) || details.missing.length === 0) return []
    return [`${attribute}: ${details.missing.join(', ')}`]
  }))
}
async function runBaselineCheck(record) {
  checkingDeploymentId.value = record.id
  try {
    await checkApplicationDeploymentBaseline(record.id)
    message.success('基线检查任务已提交')
    await reload(false)
    scheduleBaselinePoll(record.id, 0)
  } finally {
    checkingDeploymentId.value = null
  }
}
function controlLoadingKey(deploymentId, action) {
  return `${deploymentId}:${action}`
}
function isControlLoading(deploymentId, action) {
  return Boolean(deploymentControlLoading[controlLoadingKey(deploymentId, action)])
}
function isDeploymentBusy(deploymentId) {
  return ['start', 'stop', 'status'].some((action) => isControlLoading(deploymentId, action))
}
async function runApplicationControl(record, action) {
  const loadingKey = controlLoadingKey(record.id, action)
  deploymentControlLoading[loadingKey] = true
  try {
    const response = await controlApplicationDeployment(record.id, action)
    const data = response?.data?.data || {}
    const actionLabel = { start: '启动', stop: '停止', status: '状态' }[action]
    if (action === 'status') {
      await reload(false)
      Modal.info({
        title: `${record.instance_name} - 应用状态`,
        content: `${runtimeStatusMap[data.runtime_status]?.label || '未知'}${data.output ? `：${data.output}` : ''}`,
        okText: '关闭',
      })
    } else {
      message.success(`${record.instance_name} ${actionLabel}成功`)
      try {
        const statusResponse = await controlApplicationDeployment(record.id, 'status')
        const statusData = statusResponse?.data?.data || {}
        const expectedStatus = action === 'start' ? 'running' : 'stopped'
        if (statusData.runtime_status !== expectedStatus) {
          message.warning(
            `${actionLabel}命令成功，但状态复查为${runtimeStatusMap[statusData.runtime_status]?.label || '未知'}`,
          )
        }
      } catch {
        message.warning(`${actionLabel}命令成功，但自动状态复查失败`)
      }
      await reload(false)
    }
  } finally {
    delete deploymentControlLoading[loadingKey]
  }
}
function confirmApplicationControl(record, action) {
  const actionLabel = action === 'start' ? '启动' : '停止'
  Modal.confirm({
    title: `确认${actionLabel}应用`,
    content: `${record.application_name} / ${record.instance_name} / ${record.host_name || record.host_ip}`,
    okText: `确认${actionLabel}`,
    cancelText: '取消',
    okButtonProps: action === 'stop' ? { danger: true } : {},
    onOk: () => runApplicationControl(record, action),
  })
}
function scheduleBaselinePoll(deploymentId, attempt) {
  if (baselinePollTimer) clearTimeout(baselinePollTimer)
  if (attempt >= 30) return
  baselinePollTimer = setTimeout(async () => {
    await reload(false)
    const deployment = rows.value.find((item) => item.id === deploymentId)
    if (deployment?.health_status === 'checking') scheduleBaselinePoll(deploymentId, attempt + 1)
  }, 2000)
}
async function openBaselineHistory(record) {
  historyDeployment.value = record
  historyDialogOpen.value = true
  historyLoading.value = true
  try {
    const response = await getApplicationDeploymentBaselineHistory(record.id)
    baselineHistory.value = response?.data?.data || []
  } finally {
    historyLoading.value = false
  }
}
function confirmDeleteApplication(record) {
  openDeleteConfirm({
    title: '确认删除应用',
    summary: '应用版本和未被保护的关联资产将一并删除。',
    items: [`应用: ${record.name}`],
    onConfirm: async () => {
      await batchDeleteApplication([record.id])
      message.success('应用删除成功')
      reload(false)
    },
  })
}
function confirmDeleteDeployment(record) {
  openDeleteConfirm({
    title: '确认删除部署实例',
    summary: '仅删除该主机上的实例登记，不会删除应用版本或部署模板。',
    items: [`${record.application_name} / ${record.instance_name} / ${record.host_ip}`],
    onConfirm: async () => {
      await deleteApplicationDeployment(record.id)
      message.success('部署实例删除成功')
      await reload(false)
      emit('data-changed')
    },
  })
}
function confirmDeleteTemplate(record) {
  openDeleteConfirm({
    title: '确认删除部署模板',
    summary: '已被部署实例引用的模板不能删除。',
    items: [`${record.application_name} / ${record.name}`],
    onConfirm: async () => {
      await deleteApplicationDeploymentTemplate(record.id)
      message.success('部署模板删除成功')
      reload(false)
    },
  })
}

useKeepAliveRefreshLifecycle(
  () => {
    if (activeTab.value === 'deployments') startRuntimePolling()
  },
  stopRuntimePolling,
)

watch(activeTab, (nextTab) => {
  stopRuntimePolling()
  void loadActiveTab(nextTab)
})

watch(() => serviceFilters.business_system, (businessSystemId) => {
  if (businessSystemId && !serviceFilterRecords.environments.some((item) => item.id === serviceFilters.environment && item.business_system === businessSystemId)) {
    serviceFilters.environment = undefined
  }
})

watch(() => props.serviceScope, () => {
  stopRuntimePolling()
  const targetTab = ['all', 'businessSystem'].includes(props.serviceScope.nodeType) ? 'systems' : 'deployments'
  if (activeTab.value !== targetTab) {
    activeTab.value = targetTab
    return
  }
  void reload(true).then(() => {
    if (activeTab.value === 'deployments') startRuntimePolling()
  })
}, { deep: true })

onMounted(async () => {
  await reload(true)
  if (activeTab.value === 'deployments') startRuntimePolling()
})
onBeforeUnmount(() => {
  if (baselinePollTimer) clearTimeout(baselinePollTimer)
  stopRuntimePolling()
})
</script>

<style scoped>
.application-workspace { padding: 0 2px; }
.tools { margin-bottom: 16px; }
.service-filters { margin-bottom: 16px; }
.service-filters .ant-select { width: 100%; }
.service-filter-actions { display: flex; gap: 8px; align-items: center; }
.right-tools { display: flex; justify-content: flex-end; }
.secondary { color: rgba(0, 0, 0, 0.45); font-size: 12px; }
.runtime-status-line { display: inline-flex; align-items: center; gap: 6px; }
.runtime-error-detail { color: #d48806; cursor: help; }
.history-error { margin-bottom: 8px; }
.check-value {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font: inherit;
}
.check-missing {
  margin-bottom: 6px;
  color: #cf1322;
  font-weight: 600;
  line-height: 1.5;
}
</style>
