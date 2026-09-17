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

// 存储水位总览：data stream 运行态（大小/docs/rollover/ILM）+ 节点磁盘 + 服务树维度数据
export function getLogStorageOverview(id) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/log-storage-overview/`)
}

// 逻辑服务写入量：terms 聚合文档数（非磁盘占用口径），params: business_system/environment/days
export function getLogServiceUsage(id, params) {
  return requestUtil.get(prefix + `elasticsearch-clusters/${id}/log-service-usage/`, params)
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

export function batchRetryLogCollectionTargets(ids) {
  return requestUtil.post(prefix + 'log-targets/batch-retry/', { ids })
}

export function batchStartLogCollectionTargets(ids) {
  return requestUtil.post(prefix + 'log-targets/batch-start-service/', { ids })
}

export function batchStopLogCollectionTargets(ids) {
  return requestUtil.post(prefix + 'log-targets/batch-stop-service/', { ids })
}

export function batchApplyLogCollectionTargets(ids) {
  return requestUtil.post(prefix + 'log-targets/batch-apply/', { ids })
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

