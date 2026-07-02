import requestUtil from '@/util/request'

const prefix = 'sys/audit/'

function normalizeAuditListParams(params = {}) {
  const normalized = { ...params }
  if (normalized.pageNumber != null) {
    normalized.page = normalized.pageNumber
  }
  if (normalized.pageSize != null) {
    normalized.page_size = normalized.pageSize
  }
  return normalized
}

export function getAuditWebSshSessions(params) {
  return requestUtil.get(prefix + 'webssh-sessions/', normalizeAuditListParams(params))
}

export function getAuditWebSshSessionContent(id) {
  return requestUtil.get(prefix + `webssh-sessions/${id}/content/`)
}

export function downloadAuditWebSshSession(id) {
  return requestUtil.download(prefix + `webssh-sessions/${id}/download/`)
}

export function downloadAuditWebSshSessions(params) {
  return requestUtil.download(prefix + 'webssh-sessions/download-all/', normalizeAuditListParams(params))
}

export function getAuditLoginLogs(params) {
  return requestUtil.get(prefix + 'login-logs/', normalizeAuditListParams(params))
}

export function getAuditOperationLogs(params) {
  return requestUtil.get(prefix + 'operation-logs/', normalizeAuditListParams(params))
}
