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

export function getManagedTargets(params) {
  return requestUtil.get(prefix + 'targets/', params)
}

export function getManagedTargetDetail(id) {
  return requestUtil.get(prefix + `targets/${id}/`)
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

export function updateMonitorTarget(id, data) {
  return requestUtil.patch(prefix + `targets/${id}/`, data)
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

// 日志存储管理面：一键初始化 index template + ISM policy（架构文档 §11 阶段 1）
export function bootstrapOpenSearchCluster(id) {
  return requestUtil.post(prefix + `opensearch-clusters/${id}/bootstrap/`, {})
}

// 解析规则调试：支持未保存定义直接试跑（架构文档 §5.4）
export function simulateOpenSearchPipeline(id, payload) {
  return requestUtil.post(prefix + `opensearch-clusters/${id}/pipeline-simulate/`, payload)
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

// 日志洞察（架构文档 §9）：index 必填，hours 默认 1
export function getLogErrorPatterns(id, index, hours = 1) {
  return requestUtil.get(prefix + `opensearch-clusters/${id}/insight/error-patterns/`, { index, hours })
}

export function getLogNewErrors(id, index, hours = 1) {
  return requestUtil.get(prefix + `opensearch-clusters/${id}/insight/new-errors/`, { index, hours })
}

export function getLogErrorSpikes(id, index, hours = 1) {
  return requestUtil.get(prefix + `opensearch-clusters/${id}/insight/error-spikes/`, { index, hours })
}

export function getLogErrorByInstance(id, index, hours = 1) {
  return requestUtil.get(prefix + `opensearch-clusters/${id}/insight/error-by-instance/`, { index, hours })
}

// 主机级日志采集目标（架构文档 §7/§8）
export function getLogCollectionTargetList(params) {
  return requestUtil.get(prefix + 'log-targets/', params)
}

export function createLogCollectionTarget(data) {
  return requestUtil.post(prefix + 'log-targets/', data)
}

export function renderLogCollectionConfig(id) {
  return requestUtil.get(prefix + `log-targets/${id}/render-config/`)
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

export function getLogInstanceTail(id, instanceName, logName, lines = 100) {
  return requestUtil.get(prefix + `log-targets/${id}/log-tail/`, {
    instance_name: instanceName, log_name: logName, lines,
  })
}
