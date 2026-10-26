<template>
  <div class="log-query-panel">
    <a-empty
      v-if="!queryReady"
      class="log-query-empty"
      :description="scope.applicationServiceId
        ? '请选择左侧逻辑服务或部署实例查看日志'
        : '当前范围内没有可查询的逻辑服务（先到部署模板/逻辑服务里创建）'"
    />

    <template v-else>
      <header class="log-query-header">
        <div>
          <div v-if="isMultiService" class="log-query-kicker">
            {{ scopeTrail.length ? scopeTrail.join(' / ') : '全部业务' }}
          </div>
          <div v-else class="log-query-kicker">{{ scope.businessSystemName || '-' }} / {{ scope.environmentName || '-' }}</div>
          <h2>{{ scope.nodeTitle }}<a-tag v-if="isMultiService" class="multi-service-tag" color="blue">跨服务查询</a-tag></h2>
        </div>
        <a-space>
          <a-tooltip v-if="queryReady" title="强制刷新当前查询范围内各服务采集中的 data stream 索引后再查询。ES 的 refresh_interval 是 10s，刚写进来的日志默认要等一会才可搜，点这个立即可见。">
            <a-button :loading="indexRefreshing" @click="refreshIndexAndReload">
              <FontAwesomeIcon :icon="['fas', 'database']" />
              <span>&nbsp;刷新索引并查询</span>
            </a-button>
          </a-tooltip>
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="activeLoading" @click="reload">
              <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="activeLoading" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </a-space>
      </header>

      <!-- 条件区（范围已归左侧树，面板不再放范围选择器）。
           时间 → 级别 → 关键词（按使用频率）→ 实例名。
           所有筛选改动**只更新状态**，统一由「查询」按钮/关键词回车触发（下钻链路除外，即查）——
           避免连环调整下拉时每次都打一轮 ES，也让"结果对应哪组条件"有唯一答案；
           条件改了未查询时按钮旁给出提示（见 queryDirty）。 -->
      <div class="log-query-filters">
        <div class="filter-field filter-time">
          <span class="filter-label">时间范围</span>
          <a-range-picker
            v-model:value="filters.timeRange"
            :show-time="timeRangeShowTime"
            :presets="timeRangePresets"
            format="YYYY-MM-DD HH:mm:ss"
            :placeholder="['开始时间', '结束时间']"
            :getPopupContainer="getPopupContainer"
            @openChange="onTimeRangeOpenChange"
          />
        </div>
        <div class="filter-field filter-narrow">
          <span class="filter-label">日志级别</span>
          <a-select
            v-model:value="filters.logLevels"
            mode="tags"
            allow-clear
            placeholder="全部级别"
            :options="logLevelOptions"
            :getPopupContainer="getPopupContainer"
            :virtual="false"
          />
        </div>
        <div class="filter-field filter-narrow">
          <span class="filter-label">实例名</span>
          <!-- 候选来自当前查询范围的分面统计（真实出现的实例名），支持手输精确匹配。 -->
          <a-select
            v-model:value="filters.instance"
            show-search
            allow-clear
            placeholder="精确匹配"
            :options="instanceOptions"
            :getPopupContainer="getPopupContainer"
            :virtual="false"
          />
        </div>
        <div class="filter-field filter-path">
          <span class="filter-label">日志路径</span>
          <a-select
            v-model:value="filters.logPath"
            allow-clear
            show-search
            option-filter-prop="label"
            :options="logPathOptions"
            :getPopupContainer="getPopupContainer"
            placeholder="选择日志路径"
          />
        </div>
        <div class="filter-field filter-keyword">
          <span class="filter-label">
            关键词
            <!-- 语法切换用**实心按钮组**：a-segmented 的选中态在这个主题下几乎看不出来
                 （白底盖浅灰），而实心按钮组的选中项是主色填充，一眼能看出哪个在用。 -->
            <a-tooltip
              title="正文：只在日志正文（log_message）里搜关键词，写 host_ip:xxx 这类字段写法不生效。Lucene：完整语法，支持 host_ip:&quot;1.2.3.4&quot;、log_level:ERROR AND timeout 等字段过滤。两种模式都只在你选中的服务范围内查。"
              placement="top"
            >
              <a-radio-group
                v-model:value="filters.keywordMode"
                size="small"
                button-style="solid"
                class="keyword-mode-switch"
              >
                <a-radio-button value="message">正文</a-radio-button>
                <a-radio-button value="lucene">Lucene</a-radio-button>
              </a-radio-group>
            </a-tooltip>
          </span>
          <a-input-search
            v-model:value="filters.keyword"
            :placeholder="keywordPlaceholder"
            allow-clear
            @search="runSearch"
          />
        </div>
        <div class="filter-field filter-submit">
          <span class="filter-label">&nbsp;</span>
          <div class="filter-submit-row">
            <a-button type="primary" :loading="activeLoading" @click="runSearch">查询</a-button>
            <a-tag v-if="queryDirty" color="orange">条件已改</a-tag>
          </div>
        </div>
      </div>

      <div class="log-query-range-hint">
        当前查询范围：{{ effectiveRangeLabel }}
        <template v-if="isMultiService">
          · 共 {{ effectiveServiceIds().length }} 个逻辑服务
        </template>
        <span v-if="queryDirty" class="keyword-mode-hint"> · 条件已修改，点「查询」生效</span>
      </div>

      <a-space v-if="hasDrillFilters" class="log-query-chip-row" wrap>
        <span class="chip-row-label">下钻过滤：</span>
        <a-tag v-if="filters.logLevels && filters.logLevels.length" closable @close="clearDrillFilter('logLevels')">日志级别 = {{ filters.logLevels.join('、') }}</a-tag>
        <a-tag v-if="filters.instance" closable @close="clearDrillFilter('instance')">实例 = {{ filters.instance }}</a-tag>
        <a-tag v-if="filters.hostIp" closable @close="clearDrillFilter('hostIp')">主机 = {{ filters.hostIp }}</a-tag>
        <a-tag v-if="filters.logName" closable @close="clearDrillFilter('logName')">日志文件 = {{ filters.logName }}</a-tag>
        <a-tag v-if="filters.logPath" closable @close="clearDrillFilter('logPath')">日志路径 = {{ filters.logPath }}</a-tag>
        <a-tag v-if="filters.errorFingerprint" closable @close="clearDrillFilter('errorFingerprint')">错误指纹 = {{ filters.errorFingerprint }}</a-tag>
        <a-tag v-if="filters.service" closable @close="clearDrillFilter('service')">逻辑服务 = {{ serviceNameByCode.get(filters.service) || filters.service }}</a-tag>
        <a-tag v-if="filters.project" closable @close="clearDrillFilter('project')">项目 = {{ dimensionName('projects', filters.project) }}</a-tag>
        <a-tag v-if="filters.businessSystem" closable @close="clearDrillFilter('businessSystem')">业务系统 = {{ dimensionName('businessSystems', filters.businessSystem) }}</a-tag>
        <a-tag v-if="filters.environment" closable @close="clearDrillFilter('environment')">环境 = {{ dimensionName('environments', filters.environment) }}</a-tag>
      </a-space>

      <a-tabs v-model:active-key="activeTab" @change="handleTabChange">
        <!-- 所有树节点使用同一个日志入口，只改变查询范围和来源列。 -->
        <a-tab-pane key="logs" :tab="innerTabs[0].label" force-render>
            <a-alert
              v-if="pagination.total >= MAX_RESULT_WINDOW"
              class="log-query-window-alert"
              type="warning"
              show-icon
              message="命中日志超过 2000 条上限，请缩小时间范围或增加过滤条件以查看更早的记录"
            />

            <a-table
              row-key="id"
              :columns="columns"
              :data-source="logs"
              :loading="loading"
              :pagination="pagination"
              :scroll="{ x: tableScrollX }"
              size="small"
              :locale="tableLocale"
              @change="handleTableChange"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'timestamp'">
                  {{ formatLogTime(record['@timestamp']) }}
                </template>
                <template v-else-if="column.key === 'log_level'">
                  <a-tag :color="LOG_LEVEL_COLOR[String(record.log_level || '').toUpperCase()] || 'default'">
                    {{ record.log_level || '-' }}
                  </a-tag>
                </template>
                <template v-else-if="column.key === 'log_path'">
                  <a-tooltip :title="record.log_path" placement="top">
                    <span>{{ record.log_path || '-' }}</span>
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'source'">
                  <a-tag v-if="record.shared_source_id" color="purple">共享源：{{ record.shared_source_name || record.shared_source_id }}</a-tag>
                  <span v-else>服务独占</span>
                </template>
                <template v-else-if="column.key === 'action'">
                  <a-tooltip title="查看详情">
                    <a-button size="small" type="link" @click="openDetail(record)">查看详情</a-button>
                  </a-tooltip>
                </template>
              </template>
            </a-table>
        </a-tab-pane>

        <!-- 统计作为辅助分析入口，分面结果可下钻回日志。 -->
        <a-tab-pane key="stats" :tab="innerTabs[1].label" force-render>
            <div class="stats-toolbar">
              <span class="stats-toolbar-label">统计维度</span>
              <a-select
                v-model:value="statsField"
                class="stats-field-select"
                :options="statsFieldOptions"
                :getPopupContainer="getPopupContainer"
                @change="onStatsFieldChange"
              />
              <span class="stats-toolbar-label">分桶粒度</span>
              <a-select
                v-model:value="statsIntervalOption"
                class="stats-interval-select"
                :options="statsIntervalOptions"
                :getPopupContainer="getPopupContainer"
                @change="loadStats"
              />
              <span class="stats-toolbar-hint">Top {{ statsBuckets.length }} · 按 {{ statsIntervalMinutes }} 分钟分桶</span>
            </div>

            <div ref="trendChartRef" class="stats-trend-chart"></div>

            <a-table
              row-key="value"
              :columns="statsColumns"
              :data-source="statsBuckets"
              :loading="statsLoading"
              :pagination="false"
              :scroll="{ x: 900 }"
              size="small"
              :locale="tableLocale"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'value'">{{ statsValueDisplay(record) }}</template>
                <template v-else-if="column.key === 'sample_message'">{{ record.sample?.log_message || '-' }}</template>
                <template v-else-if="column.key === 'action'">
                  <a-space :size="6">
                    <a-tooltip title="在明细里查看这一类日志">
                      <a-button size="small" type="link" @click="drillDown(record)">查看日志</a-button>
                    </a-tooltip>
                    <a-tooltip title="查看详情">
                      <a-button size="small" type="link" :disabled="!record.sample" @click="openDetail(record.sample)">查看详情</a-button>
                    </a-tooltip>
                  </a-space>
                </template>
              </template>
            </a-table>
        </a-tab-pane>
      </a-tabs>
    </template>

    <a-drawer v-model:open="detailOpen" title="日志详情" width="640" placement="right">
      <a-descriptions bordered :column="1" size="small">
        <a-descriptions-item label="时间">{{ formatLogTime(activeLog?.['@timestamp']) }}</a-descriptions-item>
        <a-descriptions-item label="级别">{{ activeLog?.log_level || '-' }}</a-descriptions-item>
        <a-descriptions-item label="项目">{{ dimensionName('projects', activeLog?.project) || '-' }}</a-descriptions-item>
        <a-descriptions-item label="业务">{{ dimensionName('businessSystems', activeLog?.business_system) || '-' }}</a-descriptions-item>
        <a-descriptions-item label="服务">{{ activeLog?.shared_source_id ? '共享源独立归属，不属于单个服务' : (activeLog?.service || '-') }}</a-descriptions-item>
        <a-descriptions-item v-if="activeLog?.shared_source_id" label="共享采集源">{{ activeLog.shared_source_name || activeLog.shared_source_id }}</a-descriptions-item>
        <a-descriptions-item label="实例">{{ activeLog?.instance || '-' }}</a-descriptions-item>
        <a-descriptions-item label="主机">{{ activeLog?.host_ip || '-' }}</a-descriptions-item>
        <a-descriptions-item label="日志名称">{{ activeLog?.log_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="日志路径">{{ activeLog?.log_path || '-' }}</a-descriptions-item>
        <a-descriptions-item label="错误指纹" v-if="activeLog?.error_fingerprint">{{ activeLog.error_fingerprint }}</a-descriptions-item>
      </a-descriptions>
      <div class="detail-section-head">
        <span class="detail-section-title">原始日志</span>
        <a-space :size="8">
          <a-button size="small" @click="openDetailTextPreview('原始日志', activeLog?.message)">
            <FontAwesomeIcon :icon="['fas', 'expand']" />
            <span>&nbsp;预览</span>
          </a-button>
          <a-button size="small" @click="copyDetailText(activeLog?.message)">复制</a-button>
        </a-space>
      </div>
      <pre class="detail-content">{{ activeLog?.message || '-' }}</pre>
      <div class="detail-section-head">
        <span class="detail-section-title">错误日志</span>
        <a-space :size="8">
          <a-button size="small" @click="openDetailTextPreview('错误日志', activeLog?.log_message)">
            <FontAwesomeIcon :icon="['fas', 'expand']" />
            <span>&nbsp;预览</span>
          </a-button>
          <a-button size="small" @click="copyDetailText(activeLog?.log_message)">复制</a-button>
        </a-space>
      </div>
      <pre class="detail-content">{{ activeLog?.log_message || '-' }}</pre>
      <template v-if="activeLog?.app_fields && Object.keys(activeLog.app_fields).length">
        <div class="detail-section-head">
          <span class="detail-section-title">应用私有字段</span>
          <a-space :size="8">
            <a-button size="small" @click="openDetailTextPreview('应用私有字段', formatJsonText(activeLog.app_fields))">
              <FontAwesomeIcon :icon="['fas', 'expand']" />
              <span>&nbsp;预览</span>
            </a-button>
            <a-button size="small" @click="copyDetailText(formatJsonText(activeLog.app_fields))">复制</a-button>
          </a-space>
        </div>
        <pre class="detail-content">{{ formatJsonText(activeLog.app_fields) }}</pre>
      </template>
    </a-drawer>

    <a-modal
      v-model:open="detailTextModal.open"
      :title="detailTextModal.title"
      width="92%"
      :style="{ top: '24px' }"
      :footer="null"
    >
      <pre class="detail-modal-content">{{ detailTextModal.content || '-' }}</pre>
      <div class="detail-modal-actions">
        <a-button type="primary" @click="copyDetailText(detailTextModal.content)">复制</a-button>
      </div>
    </a-modal>

  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { createPagination, tableLocale } from '@/util/tableStyle'
import * as echarts from 'echarts'
import dayjs from 'dayjs'
import { getElasticsearchClusterList, refreshElasticsearchLogIndex, searchElasticsearchLogFacetStats, searchElasticsearchLogs } from '@/api/monitor'
import { formatTimeWithTimezone } from '@/util/timezone'
import { buildUserTimezoneRangePresets, buildUserTimezoneShowTime, toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { copyTextWithFallback } from '@/util/clipboard'
import store from '@/store'

// 与后端 LOG_SEARCH_MAX_RESULT_WINDOW 保持一致：超过该上限需要用户缩小范围，而不是无限深翻页。
const MAX_RESULT_WINDOW = 2000
const LOG_LEVEL_COLOR = {
  ERROR: 'red', SEVERE: 'red', FATAL: 'red',
  WARN: 'orange', WARNING: 'orange',
  INFO: 'blue', DEBUG: 'default', TRACE: 'default',
}
// 与后端 LOG_FACET_ALLOWED_FIELDS 一一对应，用于把统计面板点击的分面值映射回过滤条件字段。
const FACET_FILTER_KEY = {
  error_fingerprint: 'errorFingerprint',
  log_level: 'logLevels',
  instance: 'instance',
  host_ip: 'hostIp',
  log_name: 'logName',
  log_path: 'logPath',
  // 范围维度（2026-09-28）：上层节点概览按这些维度分组后，点「查看日志」切到明细收窄。
  service: 'service',
  project: 'project',
  business_system: 'businessSystem',
  environment: 'environment',
}
// 统计维度 = **当前层级之下**的范围维度 + 结果侧维度。
// 范围维度只在"比当前节点更细"的层级上才有意义：环境节点按业务系统分组是空的（范围已限定在
// 某个业务系统），服务节点连逻辑服务分组都只有单一值——所以按 nodeType 裁剪，不再一股脑列全部。
// 结果侧维度（指纹/级别/实例/主机/文件/路径）任何层级都可用。
const RANGE_FACET_FIELDS = {
  all: [
    { label: '项目', value: 'project' },
    { label: '业务系统', value: 'business_system' },
    { label: '环境', value: 'environment' },
    { label: '逻辑服务', value: 'service' },
  ],
  project: [
    { label: '业务系统', value: 'business_system' },
    { label: '环境', value: 'environment' },
    { label: '逻辑服务', value: 'service' },
  ],
  businessSystem: [
    { label: '环境', value: 'environment' },
    { label: '逻辑服务', value: 'service' },
  ],
  environment: [
    { label: '逻辑服务', value: 'service' },
  ],
}
const RESULT_FACET_FIELDS = [
  { label: '错误指纹', value: 'error_fingerprint' },
  { label: '日志级别', value: 'log_level' },
  { label: '实例', value: 'instance' },
  { label: '主机', value: 'host_ip' },
  { label: '日志文件', value: 'log_name' },
  { label: '日志路径', value: 'log_path' },
]
const statsFieldOptions = computed(() => [
  ...(RANGE_FACET_FIELDS[props.scope.nodeType] || []),
  ...RESULT_FACET_FIELDS,
])
// 'auto' 交给后端按时间跨度自适应；其余选项是分钟数，后端会根据总桶数上限拒绝过细的选择。
const statsIntervalOptions = [
  { label: '自动', value: 'auto' },
  { label: '1 分钟', value: '1' },
  { label: '5 分钟', value: '5' },
  { label: '15 分钟', value: '15' },
  { label: '30 分钟', value: '30' },
  { label: '1 小时', value: '60' },
  { label: '6 小时', value: '360' },
  { label: '1 天', value: '1440' },
]

const props = defineProps({
  // 宿主（日志中心）当前选中的节点范围；nodeType 为 service/deployment（带 applicationServiceId）
  // 时查单个服务；其它层级进入**跨服务模式**：候选集由 candidateServices 给出，可多选一起查。
  scope: { type: Object, required: true },
  // 当前节点范围内的逻辑服务清单（util/logLevelOverview.servicesUnderScope 的产物）。
  // 空数组且没有 applicationServiceId 时显示空态（范围内没有服务可查）。
  candidateServices: { type: Array, default: () => [] },
  // 维度编码 → 名称的映射（跨服务模式的维度列显示名称）：
  // { projects: {code:name}, businessSystems: {code:name}, environments: {code:name} }。
  // 缺映射时显示编码（流文档上只有编码）。
  dimensionNames: { type: Object, default: () => ({}) },
  // 跨服务模式头部的层级路径（如 ['yilake','aos']）；空数组 = 不显示路径行
  //（全部节点没有可说的上级）。单服务模式仍用 scope 上的 businessSystemName/environmentName。
  scopeTrail: { type: Array, default: () => [] },
})

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

// 跨服务模式：非服务节点（业务系统/环境/项目/全部）。候选 = 范围内的全部逻辑服务。
const isMultiService = computed(() => !props.scope.applicationServiceId && props.candidateServices.length > 0)
// 是否具备查询条件：单服务（有 id）或多服务（范围内有服务）。
const queryReady = computed(() => Boolean(props.scope.applicationServiceId) || isMultiService.value)

// 树只决定范围，不改变查询入口，避免上层节点强迫用户先做聚合再找日志。
const innerTabs = [{ key: 'logs', label: '日志' }, { key: 'stats', label: '统计' }]

// 当前查询实际覆盖的逻辑服务 id（发请求与"刷新索引"共用）：
// 单服务 = scope 上的 id；跨服务 = 范围（左树选中节点）内的全部候选服务——面板不再二次收窄。
function effectiveServiceIds() {
  if (props.scope.applicationServiceId) return [props.scope.applicationServiceId]
  return props.candidateServices.map((service) => service.id)
}

// 文档上的 service 是**编码**：名称按 code 反查（同码服务在候选里取第一个命中，
// 范围内同码时显示同名不算错——它们本来就同名同码）。
const serviceNameByCode = computed(() => {
  const byCode = new Map()
  for (const service of props.candidateServices) {
    if (!byCode.has(service.code)) byCode.set(service.code, service.name)
  }
  return byCode
})

// 维度名称映射（业务系统/环境/项目）：文档上是编码，显示用名称。
const dimensionName = (mapping, code) => props.dimensionNames?.[mapping]?.[code] || code

// 跨服务模式的层级维度列：所在层级越高，列越多——
//   全部 → 项目+业务系统+环境+逻辑服务；项目 → 业务系统+环境+逻辑服务；
//   业务系统 → 环境+逻辑服务；环境 → 逻辑服务。
const dimensionColumns = computed(() => {
  const columns = []
  if (props.scope.nodeType === 'all') {
    columns.push({
      title: '项目', key: 'project_name', width: 130, ellipsis: true,
      customRender: ({ record }) => dimensionName('projects', record.project),
    })
  }
  if (['all', 'project'].includes(props.scope.nodeType)) {
    columns.push({
      title: '业务系统', key: 'business_system_name', width: 130, ellipsis: true,
      customRender: ({ record }) => dimensionName('businessSystems', record.business_system),
    })
  }
  if (['all', 'project', 'businessSystem'].includes(props.scope.nodeType)) {
    columns.push({
      title: '环境', key: 'environment_name', width: 110, ellipsis: true,
      customRender: ({ record }) => dimensionName('environments', record.environment),
    })
  }
  return columns
})
// 关键词框的两个模式（与后端 keyword_mode 一一对应）：正文=只搜日志正文（默认，安全）；
// Lucene=完整语法（可按字段过滤）。框里的提示词随模式变化，避免"写着 Lucene 却不支持 Lucene"。
// 提示词随模式变化：Lucene 模式给一个能直接照抄的例子（现场就是"不知道该怎么写"、
// 按框里的 Lucene 提示写字段过滤却查不到）。
const keywordPlaceholder = computed(() => (
  filters.keywordMode === 'lucene'
    ? 'Lucene 表达式，如 host_ip:"192.168.201.209" AND log_level:ERROR'
    : '搜索日志正文（关键词）'
))

const userTimezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const formatTime = (value) => (value ? formatTimeWithTimezone(value, userTimezone.value) : '-')
// 日志记录的时间显示到**毫秒**：ES 的 @timestamp 本就是毫秒精度，同一秒内多条记录靠它区分先后；
// 时间范围标签/趋势图轴仍用秒级 formatTime（选的是分钟级范围，显示毫秒只会变吵）。
const formatLogTime = (value) => (value ? formatTimeWithTimezone(value, userTimezone.value, 'YYYY-MM-DD HH:mm:ss.SSS') : '-')

const clusterId = ref(null)
const activeTab = ref('logs')
const loading = ref(false)
const indexRefreshing = ref(false)
const logs = ref([])
const detailOpen = ref(false)
const activeLog = ref(null)
// 原始日志 / 错误日志的放大预览：详情侧边栏窄，长文本（堆栈）在这里看不动，弹个大框。
const detailTextModal = reactive({ open: false, title: '', content: '' })


const statsField = ref('error_fingerprint')
const statsIntervalOption = ref('auto')
const statsLoading = ref(false)
const statsBuckets = ref([])
const statsIntervalMinutes = ref(1)
const trendChartRef = ref(null)
let trendChart = null

// 顶部刷新按钮的 loading 态跟随当前激活的 tab，避免切到统计页时按钮显示还停在日志查询的状态。
const activeLoading = computed(() => (activeTab.value === 'stats' ? statsLoading.value : loading.value))

const filters = reactive({
  // 与后端 _log_search_time_range 的默认窗口保持一致，避免选择器留白让人看不出实际生效的时间范围。
  timeRange: [dayjs().tz(userTimezone.value).subtract(1, 'hour'), dayjs().tz(userTimezone.value)],
  keyword: '',
  // 关键词框的模式：'message'（默认，只搜正文）/ 'lucene'（完整 Lucene 语法，可按字段过滤）。
  // 默认落在更安全的一侧；切换本身不触发查询，等用户按回车/点搜索。
  keywordMode: 'message',
  logLevels: [],
  instance: '',
  // 路径可直接输入；其他维度通过概览/统计面板“查看日志”下钻写入。
  hostIp: '',
  logName: '',
  logPath: '',
  errorFingerprint: '',
  // 范围维度下钻（概览按 逻辑服务/业务系统/环境/项目 分组后点进来看明细）。
  service: '',
  project: '',
  businessSystem: '',
  environment: '',
})

// 选择器一旦被清空，effectiveRangeLabel 兜底显示后端实际会生效的默认范围，而不是留白让人误以为“不限时间”。
const effectiveRangeLabel = computed(() => {
  const [startTime, endTime] = filters.timeRange || []
  if (!startTime || !endTime) return '最近 1 小时（未选择时的默认范围）'
  return `${formatTime(startTime)} ~ ${formatTime(endTime)}`
})

// 日志级别不同应用可能用不同的命名约定（如 Java SEVERE），不能硬编码；实际可选值改为从当前服务的真实数据中动态拉取（见 loadLogLevelOptions）。
const logLevelOptions = ref([])
// 实例名下拉候选：当前查询范围内真实出现的实例（分面统计拉取，见 loadLogLevelOptions）。
const instanceOptions = ref([])
const logPathOptions = ref([])

const timeRangePresets = ref([])
const timeRangeShowTime = buildUserTimezoneShowTime(userTimezone.value)

function refreshTimeRangePresets() {
  timeRangePresets.value = buildUserTimezoneRangePresets(userTimezone.value)
}

function onTimeRangeOpenChange(open) {
  if (open) refreshTimeRangePresets()
}

const pagination = reactive(createPagination(0, 100, { pageSizeOptions: ['50', '100', '200'] }))

// 跨服务模式多一列「逻辑服务」：记录上只有 service 编码，名字用候选清单反查
//（查不到显示编码——范围外的文档不该出现，出现编码也比空着好排查）。
const columns = computed(() => {
  const base = [
    { title: '时间', key: 'timestamp', width: 190, fixed: 'left' },
    { title: '级别', dataIndex: 'log_level', key: 'log_level', width: 90 },
  ]
  if (isMultiService.value) {
    base.push(...dimensionColumns.value)
    base.push({
      title: '逻辑服务', key: 'service_name', width: 150, ellipsis: true,
      customRender: ({ record }) => record.shared_source_id ? '共享源' : (serviceNameByCode.value.get(record.service) || record.service || '-'),
    })
  }
  base.push(
    { title: '采集来源', key: 'source', width: 190 },
    { title: '实例', dataIndex: 'instance', key: 'instance', width: 160, ellipsis: true },
    { title: '主机', dataIndex: 'host_ip', key: 'host_ip', width: 130 },
    { title: '日志路径', dataIndex: 'log_path', key: 'log_path', width: 280, ellipsis: true },
    { title: '错误日志', dataIndex: 'log_message', key: 'log_message', ellipsis: true },
    { title: '操作', key: 'action', width: 100, fixed: 'right' },
  )
  return base
})

// 路径列预留独立宽度；跨服务模式再增加维度列与逻辑服务列，避免挤压日志正文。
const tableScrollX = computed(() => {
  if (!isMultiService.value) return 1670
  const extra = dimensionColumns.value.reduce((sum, column) => sum + (column.width || 0), 0) + 150
  return 1670 + extra
})

const statsValueColumnTitle = computed(
  () => statsFieldOptions.value.find((item) => item.value === statsField.value)?.label || '值',
)
const statsColumns = computed(() => [
  { title: statsValueColumnTitle.value, key: 'value', width: 220, ellipsis: true },
  { title: '次数', dataIndex: 'count', key: 'count', width: 100 },
  { title: '样例错误日志', key: 'sample_message', ellipsis: true },
  { title: '操作', key: 'action', width: 160, fixed: 'right' },
])

function onStatsFieldChange() {
  loadStats()
}

// 日志中心左侧树上切换节点时，由父组件传入新的 scope，这里跟着重置查询状态。
// 关键词、级别、统计下钻写入的过滤项都代表"上一个服务的问题现场"，不能跨服务残留：
// 此前只重置了 instance，导致切到另一个逻辑服务时关键词/级别/下钻过滤仍带着上一个服务的值。
// 统一的查询入口：重置分页 + 刷新分面候选（级别/实例）+ 重放当前 tab 的查询。
// 筛选改动的所有出口（查询按钮、关键词回车、下钻标签、刷新）都汇到这里，行为只有一个。
// 筛选控件本身的改动**只更新状态**（即改即查会让连环调整下拉每次都打一轮 ES）。
//
// 期望条件签名 vs 上次已查询签名：不等就说明界面显示的筛选与结果是两组条件（queryDirty），
// 按钮旁给提示——否则用户改了条件没点查询，会误读结果。
const currentQuerySignature = computed(() => JSON.stringify(buildBaseParams()))
const lastQuerySignature = ref('')
const queryDirty = computed(() => queryReady.value && currentQuerySignature.value !== lastQuerySignature.value)

async function runSearch() {
  if (!queryReady.value) return
  pagination.current = 1
  lastQuerySignature.value = currentQuerySignature.value
  await Promise.all([loadLogLevelOptions(), reloadActiveTab()])
}

let logsRequestId = 0
let statsRequestId = 0
let optionsRequestId = 0

// 左树筛选和异步服务名单都定义查询范围；旧范围的响应不得回填新范围的界面。
watch([() => JSON.stringify(props.scope), () => effectiveServiceIds().join(',')], ([scopeKey], previous = []) => {
  logsRequestId++
  statsRequestId++
  optionsRequestId++
  loading.value = false
  statsLoading.value = false
  logs.value = []
  statsBuckets.value = []
  pagination.total = 0
  logLevelOptions.value = []
  instanceOptions.value = []
  logPathOptions.value = []
  if (scopeKey !== previous[0]) {
    filters.keyword = ''
    filters.logLevels = []
    filters.hostIp = ''
    filters.logName = ''
    filters.logPath = ''
    filters.errorFingerprint = ''
    filters.service = ''
    filters.project = ''
    filters.businessSystem = ''
    filters.environment = ''
    filters.instance = props.scope.nodeType === 'deployment' ? props.scope.nodeTitle : ''
    // 统计维度仍按范围初始化，但所有节点统一默认展示原始日志。
    statsField.value = isMultiService.value ? 'service' : 'error_fingerprint'
    activeTab.value = 'logs'
  } else if (isMultiService.value && !previous[1]) {
    // 首次异步加载服务名单时补全维度，不覆盖用户已经选择的 tab。
    statsField.value = 'service'
  }
  pagination.current = 1
  // 走统一查询入口：标记"已按当前条件查询"（否则一进来就显示"条件已改"）。
  runSearch()
}, { immediate: true })

// 当前有哪些下钻过滤（决定"下钻过滤"行是否显示）。
// 必须覆盖**全部**可下钻维度：只判断一部分会让按 逻辑服务/业务系统/环境/级别/实例 下钻后
// 整行不渲染，过滤条件看不见也叉不掉（现场反馈）。
const hasDrillFilters = computed(() => Boolean(
  filters.logLevels?.length || filters.instance || filters.hostIp || filters.logName
  || filters.logPath || filters.errorFingerprint
  || filters.service || filters.project || filters.businessSystem || filters.environment,
))

function clearDrillFilter(key) {
  // 下钻标签的清除属于"改完立刻想看到新结果"的链路，直接重查（不走查询按钮）。
  // 级别是多值（数组），其余是单值字符串——按当前值的类型清空，别把数组写成空串。
  filters[key] = Array.isArray(filters[key]) ? [] : ''
  runSearch()
}

function buildBaseParams() {
  const [startTime, endTime] = filters.timeRange || []
  const params = {
    start: toUtcQueryISOStringByUserTimezone(startTime, userTimezone.value),
    end: toUtcQueryISOStringByUserTimezone(endTime, userTimezone.value),
    keyword: filters.keyword || undefined,
    keyword_mode: filters.keyword || undefined ? filters.keywordMode : undefined,
    log_level: (filters.logLevels || []).join(',') || undefined,
    instance: filters.instance || undefined,
    host_ip: filters.hostIp || undefined,
    log_name: filters.logName || undefined,
    log_path: filters.logPath || undefined,
    error_fingerprint: filters.errorFingerprint || undefined,
    // 范围维度下钻（概览按服务/业务系统/环境/项目分组后点进来看明细）。
    project: filters.project || undefined,
    business_system: filters.businessSystem || undefined,
    environment: filters.environment || undefined,
    service: filters.service || undefined,
  }
  if (props.scope.applicationServiceId) {
    params.application_service_id = props.scope.applicationServiceId
  } else if (isMultiService.value) {
    // 跨服务检索：级联选中展开成服务 id（未选 = 范围内全部），后端按四元组收窄。
    params.application_service_ids = effectiveServiceIds().join(',')
  }
  return params
}

function buildQueryParams() {
  return {
    ...buildBaseParams(),
    size: pagination.pageSize,
    offset: (pagination.current - 1) * pagination.pageSize,
  }
}

function buildFacetParams() {
  return {
    ...buildBaseParams(),
    field: statsField.value,
    size: 20,
    interval_minutes: statsIntervalOption.value === 'auto' ? undefined : statsIntervalOption.value,
  }
}

async function ensureClusterId() {
  if (clusterId.value) return clusterId.value
  const response = await getElasticsearchClusterList({ page: 1, page_size: 100 })
  const results = response?.data?.data?.results || []
  const cluster = results.find((item) => item.is_default) || results[0]
  clusterId.value = cluster?.id || null
  return clusterId.value
}

async function loadLogs() {
  if (!queryReady.value) {
    logs.value = []
    pagination.total = 0
    return
  }
  const requestId = ++logsRequestId
  const params = buildQueryParams()
  loading.value = true
  try {
    const id = await ensureClusterId()
    if (requestId !== logsRequestId) return
    if (!id) {
      message.warning('尚未配置日志存储集群')
      logs.value = []
      pagination.total = 0
      return
    }
    const response = await searchElasticsearchLogs(id, params)
    if (requestId !== logsRequestId) return
    const payload = response?.data?.data || {}
    logs.value = payload.results || []
    pagination.total = Math.min(payload.count || 0, MAX_RESULT_WINDOW)
  } catch (error) {
    if (requestId !== logsRequestId) return
    message.error(error?.response?.data?.msg || error?.message || '查询日志失败')
  } finally {
    if (requestId === logsRequestId) loading.value = false
  }
}

async function loadStats() {
  if (!queryReady.value) {
    statsBuckets.value = []
    return
  }
  const requestId = ++statsRequestId
  const params = buildFacetParams()
  statsLoading.value = true
  try {
    const id = await ensureClusterId()
    if (requestId !== statsRequestId) return
    if (!id) {
      message.warning('尚未配置日志存储集群')
      statsBuckets.value = []
      return
    }
    const response = await searchElasticsearchLogFacetStats(id, params)
    if (requestId !== statsRequestId) return
    const payload = response?.data?.data || {}
    statsBuckets.value = payload.buckets || []
    statsIntervalMinutes.value = payload.interval_minutes || 1
  } catch (error) {
    if (requestId !== statsRequestId) return
    message.error(error?.response?.data?.msg || error?.message || '统计失败')
  } finally {
    if (requestId === statsRequestId) {
      statsLoading.value = false
      await nextTick()
      renderTrendChart()
    }
  }
}

function reloadActiveTab() {
  if (activeTab.value === 'stats') {
    return loadStats()
  } else {
    return loadLogs()
  }
}

function handleTableChange(pager) {
  pagination.current = pager.current
  pagination.pageSize = pager.pageSize
  loadLogs()
}

function handleTabChange(key) {
  activeTab.value = key
  reloadActiveTab()
  if (key === 'stats') {
    nextTick(() => {
      ensureTrendChart()
      trendChart?.resize()
    })
  }
}

function reload() {
  lastQuerySignature.value = currentQuerySignature.value
  loadLogLevelOptions()
  reloadActiveTab()
}

// 强制刷新该服务采集中的 data stream 索引，然后重新查询：ES refresh_interval=10s，
// 刚写入的日志默认要等一会才可搜，这个按钮把"已写入未刷新"的部分立即变得可查。
async function refreshIndexAndReload() {
  if (indexRefreshing.value) return
  indexRefreshing.value = true
  try {
    const id = await ensureClusterId()
    if (!id) {
      message.warning('尚未配置日志存储集群')
      return
    }
    // 跨服务模式：把当前查询覆盖的每个服务的 data stream 都刷新一遍（逐个调，服务数有上限 50）。
    let total = 0
    let skippedMessage = ''
    for (const serviceId of effectiveServiceIds()) {
      const response = await refreshElasticsearchLogIndex(id, { application_service_id: serviceId })
      const payload = response?.data?.data || {}
      if (payload.count > 0) {
        total += Number(payload.count) || 0
      } else if (!skippedMessage) {
        skippedMessage = payload.message || ''
      }
    }
    if (total > 0) {
      message.success(`已刷新 ${total} 条 data stream`)
    } else {
      message.info(skippedMessage || '当前范围内没有采集中的 data stream')
    }
    reload()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '刷新索引失败')
  } finally {
    indexRefreshing.value = false
  }
}

defineExpose({ reload, resize: resizeTrendChart })

// 复用分面统计接口拉当前服务+时间范围内实际出现过的级别值，避免下拉选项与真实数据脱节。
// 日志级别、实例名和路径候选都从当前查询范围的真实数据分面统计获取。
// 级别的命名随应用而异（Java SEVERE…）不能硬编码；实例名手写容易拼错，从真实数据里选才准。
// 候选加载失败不阻塞主查询。
async function loadLogLevelOptions() {
  const requestId = ++optionsRequestId
  if (!queryReady.value) {
    logLevelOptions.value = []
    instanceOptions.value = []
    logPathOptions.value = []
    return
  }
  const params = buildBaseParams()
  delete params.log_level
  delete params.instance
  const pathParams = buildBaseParams()
  // 不让当前路径选择收窄自身候选，用户仍能切换到同范围内其他文件。
  delete pathParams.log_path
  try {
    const id = await ensureClusterId()
    if (requestId !== optionsRequestId) return
    if (!id) return
    const [levelResponse, instanceResponse, pathResponse] = await Promise.all([
      searchElasticsearchLogFacetStats(id, { ...params, field: 'log_level', size: 50, options_only: true }),
      searchElasticsearchLogFacetStats(id, { ...params, field: 'instance', size: 50, options_only: true }),
      searchElasticsearchLogFacetStats(id, { ...pathParams, field: 'log_path', size: 50, options_only: true }),
    ])
    if (requestId !== optionsRequestId) return
    logLevelOptions.value = toFacetOptions(levelResponse)
    instanceOptions.value = toFacetOptions(instanceResponse)
    logPathOptions.value = toFacetOptions(pathResponse)
  } catch (error) {
    if (requestId !== optionsRequestId) return
    // 下拉可选值只是辅助提示，加载失败不阻塞主查询，mode="tags" 仍可手动输入级别。
    console.warn('[log_center] 加载日志筛选候选失败', error?.response?.data?.msg || error?.message)
    logLevelOptions.value = []
    instanceOptions.value = []
    logPathOptions.value = []
  }
}

function toFacetOptions(response) {
  const buckets = response?.data?.data?.buckets || []
  return buckets
    .filter((bucket) => bucket.value != null && bucket.value !== '')
    .map((bucket) => ({ label: String(bucket.value), value: String(bucket.value) }))
}

// 概览/统计表格的「值」列：范围维度显示名称（逻辑服务/业务系统/环境/项目），
// 其余维度原样显示值。下钻写入过滤条件的仍是**原始编码**（见 drillDown）。
function statsValueDisplay(record) {
  const value = record?.value == null ? '' : String(record.value)
  if (!value) return '-'
  if (statsField.value === 'service') return serviceNameByCode.value.get(value) || value
  if (statsField.value === 'business_system') return dimensionName('businessSystems', value)
  if (statsField.value === 'environment') return dimensionName('environments', value)
  if (statsField.value === 'project') return dimensionName('projects', value)
  return value
}

// 统计面板点击“查看日志”：把当前分面值写回对应过滤条件，切到明细实现下钻。
function drillDown(bucket) {
  const applyFilter = (field, rawValue) => {
    const key = FACET_FILTER_KEY[field]
    if (!key) return
    const value = rawValue == null ? '' : String(rawValue)
    if (key === 'logLevels') {
      filters.logLevels = value ? [value] : []
    } else {
      filters[key] = value
    }
  }
  applyFilter(statsField.value, bucket?.value)
  // 下钻即查：切到明细（上层节点下 key 仍是 logs）并重放查询。
  activeTab.value = 'logs'
  runSearch()
}

function openDetail(record) {
  if (!record) return
  activeLog.value = record
  detailOpen.value = true
}

function openDetailTextPreview(title, content) {
  detailTextModal.title = String(title || '查看')
  detailTextModal.content = String(content || '')
  detailTextModal.open = true
}

async function copyDetailText(text) {
  const content = String(text || '')
  if (!content) {
    message.warning('没有可复制的内容')
    return
  }
  const copied = await copyTextWithFallback(content)
  if (copied) {
    message.success('已复制')
    return
  }
  message.error('复制失败，请检查浏览器权限')
}

function formatJsonText(value) {
  try {
    return JSON.stringify(value, null, 2)
  } catch (error) {
    return String(value)
  }
}

function ensureTrendChart() {
  if (trendChartRef.value && !trendChart) {
    trendChart = echarts.init(trendChartRef.value)
  }
}

function renderTrendChart() {
  ensureTrendChart()
  if (!trendChart) return
  // 折线数最多画 8 条：分面值可能有 20 条，全部画上去图例会挤成一团，反而看不清趋势。
  const top = statsBuckets.value.slice(0, 8)
  const timestamps = (top[0]?.trend || []).map((point) => formatTime(point.timestamp))
  trendChart.setOption({
    tooltip: { trigger: 'axis' },
    legend: { type: 'scroll', bottom: 0 },
    grid: { left: 56, right: 24, top: 24, bottom: 56 },
    xAxis: { type: 'category', data: timestamps, axisLabel: { hideOverlap: true } },
    yAxis: { type: 'value' },
    series: top.map((bucket) => ({
      name: String(bucket.value ?? '-'),
      type: 'line',
      smooth: true,
      data: (bucket.trend || []).map((point) => point.count),
    })),
  }, true)
}

function resizeTrendChart() {
  trendChart?.resize()
}

onMounted(() => {
  refreshTimeRangePresets()
  window.addEventListener('resize', resizeTrendChart)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeTrendChart)
  trendChart?.dispose()
  trendChart = null
})
</script>

<style scoped>
.log-query-panel {
  min-width: 0;
}
.log-query-empty {
  margin-top: 80px;
}
.log-query-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.log-query-header h2 {
  margin: 0;
  font-size: 20px;
}
.multi-service-tag {
  margin-left: 10px;
  vertical-align: 3px;
}
.log-query-kicker {
  color: #7a8697;
  font-size: 13px;
}
/* 过滤区：流式布局。每个字段 = 标签 + 控件，同高（控件常规 size）、圆角一致；
   固定宽字段（时间/级别/实例/维度）不伸缩，弹性字段（关键词/逻辑服务）吃掉剩余宽度，
   一行放不下整体换行——对齐靠 flex-basis，不再拼栅格 span。 */
.log-query-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px 16px;
  margin-bottom: 4px;
}
/* 查询按钮与「条件已改」提示同排。 */
.filter-submit-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.filter-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.filter-field > :deep(.ant-select),
.filter-field > :deep(.ant-input),
.filter-field > :deep(.ant-input-search),
.filter-field > :deep(.ant-picker) {
  width: 100%;
}
.filter-label {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #7a8697;
  font-size: 12px;
  line-height: 18px;
}
.filter-time {
  flex: 0 0 350px;
}
.filter-keyword {
  flex: 1 1 340px;
}
.filter-narrow {
  flex: 0 0 200px;
}
.filter-path {
  flex: 1 1 280px;
  max-width: 100%;
}
.filter-path > :deep(.ant-input-affix-wrapper) {
  width: 100%;
  min-width: 0;
}
/* 逻辑服务是"主筛选"：比维度下拉略宽，但不弹性铺满（铺满会与其它框不对齐）。 */
.filter-wide {
  flex: 0 0 280px;
}
.keyword-mode-switch {
  /* 与标签文字同排，不把标签行撑高 */
  line-height: 18px;
}
.log-query-range-hint {
  margin: -4px 0 12px;
  color: #7a8697;
  font-size: 12px;
}
.keyword-mode-hint {
  color: #8c8c8c;
}
.log-query-window-alert {
  margin-bottom: 12px;
}
.log-query-chip-row {
  margin-bottom: 12px;
}
.chip-row-label {
  color: #7a8697;
  font-size: 13px;
}
.stats-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.stats-toolbar-label {
  color: #7a8697;
  font-size: 13px;
}
.stats-field-select {
  width: 160px;
}
.stats-interval-select {
  width: 120px;
}
.stats-toolbar-hint {
  margin-left: auto;
  color: #7a8697;
  font-size: 12px;
}
.stats-trend-chart {
  width: 100%;
  height: 280px;
  margin-bottom: 12px;
}
.detail-section-title {
  margin: 16px 0 8px;
  font-weight: 600;
}
.detail-section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin: 16px 0 8px;
}
.detail-section-head .detail-section-title {
  margin: 0;
}
.detail-content {
  white-space: pre-wrap;
  word-break: break-all;
  background: #f5f5f5;
  padding: 8px 12px;
  border-radius: 4px;
  max-height: 320px;
  overflow: auto;
}
.detail-modal-content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  background: #f5f5f5;
  padding: 12px 16px;
  border-radius: 4px;
  min-height: 45vh;
  max-height: 80vh;
  overflow: auto;
  font-size: 13px;
  line-height: 1.6;
}
.detail-modal-actions {
  margin-top: 12px;
  text-align: right;
}
</style>
