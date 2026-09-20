import requestUtil from '@/util/request'

const prefix = 'monitor/'

export function getMonitorSummary() {
  return requestUtil.get(prefix + 'summary/')
}

export function getSoftwarePackages(params) {
  return requestUtil.get(prefix + 'packages/', params)
}

export function createSoftwarePackage(data) {
  return requestUtil.post(prefix + 'packages/', data)
}

export function uploadSoftwarePackageFile(id, formData) {
  return requestUtil.fileUpload(prefix + `packages/${id}/upload/`, formData)
}

export function updateSoftwarePackage(id, data) {
  return requestUtil.patch(prefix + `packages/${id}/`, data)
}

export function syncSoftwarePackageFromOfficial(id, version) {
  // 服务端需从 GitHub 下载官方 tarball，放宽超时避免网络较慢时误判失败
  return requestUtil.post(prefix + `packages/${id}/sync-official/`, { version }, 60000)
}

export function batchDeleteSoftwarePackages(ids) {
  return requestUtil.post(prefix + 'packages/batch-delete/', { ids })
}

export function getPrometheusOverview() {
  return requestUtil.get(prefix + 'targets/prometheus/overview/')
}

export function getPrometheusTargets() {
  return requestUtil.get(prefix + 'targets/prometheus/targets/')
}

export function getPrometheusTsdbStatus() {
  return requestUtil.get(prefix + 'targets/prometheus/tsdb-status/')
}

export function getPrometheusConfig() {
  return requestUtil.get(prefix + 'targets/prometheus/config/')
}

export function getPrometheusFlags() {
  return requestUtil.get(prefix + 'targets/prometheus/flags/')
}

export function queryPrometheusInstant(params) {
  return requestUtil.get(prefix + 'targets/prometheus/query/', params)
}

export function queryPrometheusRange(params) {
  return requestUtil.get(prefix + 'targets/prometheus/query-range/', params)
}

export function getPrometheusAlerts() {
  return requestUtil.get(prefix + 'targets/prometheus/alerts/')
}

// 告警规则改为只读展示 Prometheus 侧当前生效的规则，不再支持本地增删改/导出/部署，
// 详见 monitor.views.MonitorViewSet.prometheus_rules。
export function getPrometheusAlertRules() {
  return requestUtil.get(prefix + 'targets/prometheus/rules/')
}

// 历史告警：backend 替代 Alertmanager 接收 Prometheus 推送后落库的历史记录（只读查询）。
export function getAlertHistories(params) {
  return requestUtil.get(prefix + 'alert-histories/', params)
}

export function getAlertNotificationStatus(alertId) {
  return requestUtil.get(prefix + `alert-histories/${alertId}/notification-status/`)
}

// 用户视角的通知链路诊断（P1）：userId 为空表示当前登录用户。
export function getUserNotificationChain(userId) {
  return requestUtil.get(prefix + 'alert-notification/user-chain/', userId ? { user_id: userId } : {})
}

// 单条历史告警的完整通知链路（P2）：路由匹配 → 媒介 → 用户绑定 → 事件/投递明细。
export function getAlertNotificationChain(historyId) {
  return requestUtil.get(prefix + `alert-notification/chain/${historyId}/`)
}

export function getAlertMediaList(params) {
  return requestUtil.get(prefix + 'media/', params)
}

export function createAlertMedia(data) {
  return requestUtil.post(prefix + 'media/', data)
}

export function updateAlertMedia(id, data) {
  return requestUtil.patch(prefix + `media/${id}/`, data)
}

export function batchDeleteAlertMedias(ids) {
  return requestUtil.post(prefix + 'media/batch-delete/', { ids })
}

export function testAlertMedia(id, data) {
  return requestUtil.post(prefix + `media/${id}/test/`, data)
}

// 通知策略树（Grafana notification policy 模型）：根节点内置，子策略按 position 排序。
export function getNotificationPolicyList() {
  return requestUtil.get(prefix + 'notification-policies/')
}

export function createNotificationPolicy(data) {
  return requestUtil.post(prefix + 'notification-policies/create/', data)
}

export function updateNotificationPolicy(data) {
  return requestUtil.post(prefix + 'notification-policies/update/', data)
}

export function batchDeleteNotificationPolicies(ids) {
  return requestUtil.post(prefix + 'notification-policies/batch-delete/', { ids })
}

export function retryManagedTarget(id) {
  return requestUtil.post(prefix + `targets/${id}/retry/`)
}

export function cancelManagedTarget(id) {
  return requestUtil.post(prefix + `targets/${id}/cancel/`)
}

export function checkManagedTargetServiceStatus(id) {
  return requestUtil.post(prefix + `targets/${id}/check-service-status/`)
}

export function startManagedTargetService(id) {
  return requestUtil.post(prefix + `targets/${id}/start-service/`)
}

export function stopManagedTargetService(id) {
  return requestUtil.post(prefix + `targets/${id}/stop-service/`)
}

export function getMonitorInstallHistoryList(params) {
  return requestUtil.get(prefix + 'install-histories/', params)
}

export function getMonitorInstallHistoryDetail(id) {
  return requestUtil.get(prefix + `install-histories/${id}/`)
}

export function cancelMonitorInstallHistory(id) {
  return requestUtil.post(prefix + `install-histories/${id}/cancel/`)
}

export function getElasticsearchClusterList(params) {
  return requestUtil.get(prefix + 'elasticsearch-clusters/', params)
}

export function saveElasticsearchCluster(data) {
  return data.id
    ? requestUtil.patch(prefix + `elasticsearch-clusters/${data.id}/`, data)
    : requestUtil.post(prefix + 'elasticsearch-clusters/', data)
}

export function batchDeleteElasticsearchClusters(ids) {
  return requestUtil.post(prefix + 'elasticsearch-clusters/batch-delete/', { ids })
}

export function testElasticsearchCluster(id) {
  return requestUtil.post(prefix + `elasticsearch-clusters/${id}/test-connection/`, {})
}

// 日志采集链路逐层对账（只读）：索引模板 / 保留策略 / 解析规则 / 主机配置 / 采集进程 / 数据写入
export function getLogPipelineHealth(id) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/log-health/`)
}

// 展示集群实际的索引模板 mapping（字段名/类型），读不到时后端回退内置标准字段
export function getElasticsearchIndexTemplate(id) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/index-template/`)
}

export function simulateElasticsearchPipeline(id, payload) {
  return requestUtil.post(prefix + `elasticsearch-clusters/${id}/pipeline-simulate/`, payload)
}

// 按逻辑服务查询原始日志：params 仅支持后端白名单字段（application_service_id/start/end/keyword/log_level/instance/host_ip/log_name/error_fingerprint/size/offset）
export function searchElasticsearchLogs(id, params) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/log-search/`, params)
}

// 通用分面统计：params.field 必须是后端白名单字段之一，返回按该字段聚合的计数/样例/时间趋势
export function searchElasticsearchLogFacetStats(id, params) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/log-facet-stats/`, params)
}

// 强制刷新该逻辑服务"采集中的 data stream"的 ES 索引（refresh_interval=10s 导致新日志要等一会
// 才可查，这里点一下立即刷新）。params 只需 application_service_id。
export function refreshElasticsearchLogIndex(id, params) {
  return requestUtil.post(prefix + `elasticsearch-clusters/${id}/log-refresh/`, params)
}

// 存储水位总览：data stream 运行态（大小/docs/rollover/ILM）+ 节点磁盘 + 服务树维度数据
// 存储水位总览。params 支持 application_service_id：只取该逻辑服务的流，并且后端会直接把 ES
// 查询收窄到该服务的索引（不是取全量再前端过滤），供「日志中心 → 本服务水位」用。
// 用 id 而不是 service_code：逻辑服务编码允许跨业务/环境重复，只凭 code 会命中错的维度段。
// 不传 application_service_id 时是**全量视图**：「日志中心 → 存储水位」tab 未选中服务（或选的
// 是项目/业务系统/环境）时用，前端再按树的层级过滤。
export function getLogStorageOverview(id, params) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/log-storage-overview/`, params)
}

// 逻辑服务写入量：terms 聚合文档数（非磁盘占用口径），params: business_system/environment/days
export function getLogServiceUsage(id, params) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/log-service-usage/`, params)
}

// 采集配置差异：期望片段 vs 主机上**已下发**的 inputs.d 片段（逐文件 added/removed/changed/unchanged
// + 两侧完整内容，行级 diff 由前端算）。params 可带 application_service_id：带上就再给该服务在这台
// 主机上的子指纹与"本服务是否待下发"。
// 差异内容必须读主机（库里只落指纹），所以主机离线时响应里 read_error 有话说、files 只含期望侧。
export function getLogTargetConfigDiff(id, params) {
  return requestUtil.get(prefix + `log-targets/${id}/config-diff/`, params, 60000)
}

export function getLogProcessingRules(params) {
  return requestUtil.get(prefix + 'log-processing-rules/', params)
}

export function saveLogProcessingRule(data) {
  return data.id
    ? requestUtil.patch(prefix + `log-processing-rules/${data.id}/`, data)
    : requestUtil.post(prefix + 'log-processing-rules/', data)
}

export function batchDeleteLogProcessingRules(ids) {
  return requestUtil.post(prefix + 'log-processing-rules/batch-delete/', { ids })
}

export function getLogCollectionFilterRules(params) {
  return requestUtil.get(prefix + 'log-collection-filter-rules/', params)
}

export function saveLogCollectionFilterRule(data) {
  return data.id
    ? requestUtil.patch(prefix + `log-collection-filter-rules/${data.id}/`, data)
    : requestUtil.post(prefix + 'log-collection-filter-rules/', data)
}

export function batchDeleteLogCollectionFilterRules(ids) {
  return requestUtil.post(prefix + 'log-collection-filter-rules/batch-delete/', { ids })
}

// 日志保留档位：档位即 data stream 后缀，保存后后端会把 ILM policy 重新下发到集群
export function getLogRetentionTiers(params) {
  return requestUtil.get(prefix + 'log-retention-tiers/', params)
}

export function saveLogRetentionTier(data) {
  return data.id
    ? requestUtil.patch(prefix + `log-retention-tiers/${data.id}/`, data)
    : requestUtil.post(prefix + 'log-retention-tiers/', data)
}

export function batchDeleteLogRetentionTiers(ids) {
  return requestUtil.post(prefix + 'log-retention-tiers/batch-delete/', { ids })
}

export function applyLogCollectionConfig(id) {
  return requestUtil.post(prefix + `log-targets/${id}/apply/`, {})
}

export function checkLogCollectionStatus(id) {
  return requestUtil.post(prefix + `log-targets/${id}/check-status/`, {})
}

export function retryLogCollectionTarget(id) {
  return requestUtil.post(prefix + `log-targets/${id}/retry/`, {})
}

export function startLogCollectionService(id) {
  return requestUtil.post(prefix + `log-targets/${id}/start-service/`, {})
}

export function stopLogCollectionService(id) {
  return requestUtil.post(prefix + `log-targets/${id}/stop-service/`, {})
}

export function cancelLogCollectionTarget(id) {
  return requestUtil.post(prefix + `log-targets/${id}/cancel/`, {})
}

// 批量动作（下发配置 / 安装重试）不再同步跑完：后端建作业 + 入队，前端轮询作业进度。
// ids 省略且 action=apply 表示"全部待下发"（由后端实时算）。
// 服务级下发状态：承载该服务的**主机清单**（区分已纳管/未纳管）+ 各主机配置态聚合。
// 聚合而不是服务级指纹：config_fingerprint 是主机级的（同一服务在不同主机上因实例级
// runtime_variables 不同，渲染结果本就不同）。这个接口不查 ES，只做一次渲染比对。
export function getServiceLogConfigState(applicationServiceId) {
  return requestUtil.get(prefix + 'log-targets/service-config-state/', { application_service_id: applicationServiceId })
}

// 服务级「采集链路」诊断：查不到日志时按层回答断在哪（agent 在线 / Filebeat 进程 / 配置下发 /
// 解析规则是否已发布 / 最近有没有在写）。判定与链路体检同源，只是按服务收窄。
export function getServiceCollectionChain(applicationServiceId) {
  return requestUtil.get(prefix + 'log-targets/service-collection-chain/', { application_service_id: applicationServiceId })
}

// 服务级下发：对承载该服务的全部**已纳管**主机重下发（后端逐台全量——agent 侧 apply 是
// "交付即该主机 inputs.d 全量，未交付的 .yml 删除"，所以只能整台来）。返回批量作业，进度另查。
export function applyLogTargetsForService(applicationServiceId) {
  return requestUtil.post(prefix + 'log-targets/service-apply/', { application_service_id: applicationServiceId })
}

export function createLogBatchJob(action, ids) {
  const body = { action }
  if (Array.isArray(ids) && ids.length) body.ids = ids
  return requestUtil.post(prefix + 'log-targets/batch-jobs/', body)
}

export function getLogBatchJob(id) {
  return requestUtil.get(prefix + `log-targets/batch-jobs/${id}/`)
}

// 页面上还在跑的批量作业（刷新页面后仍能接着看进度）；没有时 data 为 null。
export function getActiveLogBatchJob(action) {
  return requestUtil.get(prefix + 'log-targets/batch-jobs/active/', { action })
}

// "N 台待下发"的全量口径。
export function getLogPendingSummary() {
  return requestUtil.get(prefix + 'log-targets/pending-summary/')
}

export function batchStartLogCollectionTargets(ids) {
  return requestUtil.post(prefix + 'log-targets/batch-start-service/', { ids })
}

export function batchStopLogCollectionTargets(ids) {
  return requestUtil.post(prefix + 'log-targets/batch-stop-service/', { ids })
}

export function batchDeleteLogCollectionTargets(ids) {
  return requestUtil.post(prefix + 'log-targets/batch-delete/', { ids })
}

export function batchCreateLogCollectionTargets(hostIds, installNow = false) {
  return requestUtil.post(prefix + 'log-targets/batch-create/', { host_ids: hostIds, install_now: installNow })
}

export function getMonitorHostGroupTree() {
  return requestUtil.get(prefix + 'targets/host-group-tree/')
}

export function getMonitorHostOverview(params) {
  return requestUtil.get(prefix + 'targets/host-overview/', params)
}

export function getMonitorExporterOptions() {
  return requestUtil.get(prefix + 'targets/exporter-options/')
}

export function batchCreateMonitorTargets(payload) {
  return requestUtil.post(prefix + 'targets/batch-create/', payload)
}

export function batchDeleteMonitorTargets(ids) {
  return requestUtil.post(prefix + 'targets/batch-delete/', { ids })
}

export function batchStartMonitorTargets(ids) {
  return requestUtil.post(prefix + 'targets/batch-start-service/', { ids })
}

export function batchStopMonitorTargets(ids) {
  return requestUtil.post(prefix + 'targets/batch-stop-service/', { ids })
}

// 清理逻辑服务的数据流数据：mode=all|hours|days，amount 为小时/天数（all 可省略）。
// tier 可选：把范围从"这个服务的所有档位"收窄到某一条流（回收换档位后留下的历史流）。
export function cleanupLogDataStream(payload) {
  return requestUtil.post(prefix + 'log-datastreams/cleanup/', payload)
}

// 按**数据流名**清理（未识别流 / 档位段不在档位表里的历史流的兜底路径）。
// 服务端只收一条具体的数据流名：通配符、后备索引、前缀之外的名字一律拒绝。
export function cleanupLogDataStreamByStream(payload) {
  return requestUtil.post(prefix + 'log-datastreams/cleanup-stream/', payload)
}

