import requestUtil from '@/util/request'

const prefix = 'sys/monitor/'

export function getMonitorSummary() {
  return requestUtil.get(prefix + 'summary/')
}

export function getSoftwarePackages(params) {
  return requestUtil.get(prefix + 'packages/', params)
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

export function getPrometheusAlerts() {
  return requestUtil.get(prefix + 'targets/prometheus/alerts/')
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
