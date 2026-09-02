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

export function deleteSoftwarePackage(id) {
  return requestUtil.del(prefix + `packages/${id}/`)
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

export function getAlertMediaList(params) {
  return requestUtil.get(prefix + 'media/', params)
}

export function createAlertMedia(data) {
  return requestUtil.post(prefix + 'media/', data)
}

export function updateAlertMedia(id, data) {
  return requestUtil.patch(prefix + `media/${id}/`, data)
}

export function deleteAlertMedia(id) {
  return requestUtil.del(prefix + `media/${id}/`)
}

export function testAlertMedia(id, data) {
  return requestUtil.post(prefix + `media/${id}/test/`, data)
}

export function getAlertRouteList(params) {
  return requestUtil.get(prefix + 'alert-routes/', params)
}

export function createAlertRoute(data) {
  return requestUtil.post(prefix + 'alert-routes/', data)
}

export function updateAlertRoute(id, data) {
  return requestUtil.patch(prefix + `alert-routes/${id}/`, data)
}

export function deleteAlertRoute(id) {
  return requestUtil.del(prefix + `alert-routes/${id}/`)
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


export function deleteManagedTarget(id) {
  return requestUtil.del(prefix + `targets/${id}/`)
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

export function getOpenSearchClusterList(params) {
  return requestUtil.get(prefix + 'opensearch-clusters/', params)
}

export function saveOpenSearchCluster(data) {
  return data.id
    ? requestUtil.patch(prefix + `opensearch-clusters/${data.id}/`, data)
    : requestUtil.post(prefix + 'opensearch-clusters/', data)
}

export function deleteOpenSearchCluster(id) {
  return requestUtil.del(prefix + `opensearch-clusters/${id}/`)
}

export function testOpenSearchCluster(id) {
  return requestUtil.post(prefix + `opensearch-clusters/${id}/test-connection/`, {})
}

// 日志采集链路逐层对账（只读）：索引模板 / 保留策略 / 解析规则 / 主机配置 / 采集进程 / 数据写入
export function getLogPipelineHealth(id) {
  return requestUtil.get(prefix + `opensearch-clusters/${id}/log-health/`)
}

export function simulateOpenSearchPipeline(id, payload) {
  return requestUtil.post(prefix + `opensearch-clusters/${id}/pipeline-simulate/`, payload)
}

// 按逻辑服务查询原始日志：params 仅支持后端白名单字段（application_service_id/start/end/keyword/log_level/instance/host_ip/log_name/error_fingerprint/size/offset）
export function searchOpenSearchLogs(id, params) {
  return requestUtil.get(prefix + `opensearch-clusters/${id}/log-search/`, params)
}

// 通用分面统计：params.field 必须是后端白名单字段之一，返回按该字段聚合的计数/样例/时间趋势
export function searchOpenSearchLogFacetStats(id, params) {
  return requestUtil.get(prefix + `opensearch-clusters/${id}/log-facet-stats/`, params)
}

export function getLogProcessingRules(params) {
  return requestUtil.get(prefix + 'log-processing-rules/', params)
}

export function saveLogProcessingRule(data) {
  return data.id
    ? requestUtil.patch(prefix + `log-processing-rules/${data.id}/`, data)
    : requestUtil.post(prefix + 'log-processing-rules/', data)
}

export function deleteLogProcessingRule(id) {
  return requestUtil.del(prefix + `log-processing-rules/${id}/`)
}

export function getLogCollectionFilterRules(params) {
  return requestUtil.get(prefix + 'log-collection-filter-rules/', params)
}

export function saveLogCollectionFilterRule(data) {
  return data.id
    ? requestUtil.patch(prefix + `log-collection-filter-rules/${data.id}/`, data)
    : requestUtil.post(prefix + 'log-collection-filter-rules/', data)
}

export function deleteLogCollectionFilterRule(id) {
  return requestUtil.del(prefix + `log-collection-filter-rules/${id}/`)
}

// 日志保留档位：档位即 data stream 后缀，保存后后端会把 ISM policy 重新下发到集群
export function getLogRetentionTiers(params) {
  return requestUtil.get(prefix + 'log-retention-tiers/', params)
}

export function saveLogRetentionTier(data) {
  return data.id
    ? requestUtil.patch(prefix + `log-retention-tiers/${data.id}/`, data)
    : requestUtil.post(prefix + 'log-retention-tiers/', data)
}

export function deleteLogRetentionTier(id) {
  return requestUtil.del(prefix + `log-retention-tiers/${id}/`)
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

export function deleteLogCollectionTarget(id) {
  return requestUtil.del(prefix + `log-targets/${id}/`)
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

