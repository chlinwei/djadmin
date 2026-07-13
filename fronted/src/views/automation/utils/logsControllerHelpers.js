export function normalizeJobStatus(status) {
  return String(status || '').trim().toLowerCase()
}

export function statusColor(status) {
  if (status === 'success') return 'green'
  if (status === 'running') return 'processing'
  if (status === 'pending') return 'gold'
  if (status === 'cancelled' || status === 'skipped') return 'default'
  return 'red'
}

export function isJobFinished(status) {
  const normalized = String(status || '').toLowerCase()
  return normalized === 'success' || normalized === 'failed' || normalized === 'cancelled'
}

export function canDownloadJobLog(record) {
  return String(record?.status || '').toLowerCase() !== 'pending'
}

export function formatRuntimeTemplateLabel(record) {
  const templateName = String(record?.template_name_snapshot || record?.template_name || '-').trim() || '-'
  const runtimeTaskName = String(record?.task_name_snapshot || record?.task_name || '').trim()
  return templateName || runtimeTaskName || '-'
}

export function getRuntimeTemplateContent(record) {
  const snapshotContent = String(record?.template_content_snapshot || '').trim()
  if (snapshotContent) {
    return snapshotContent
  }
  return ''
}

export function getInventoryHostList(record) {
  const snapshot = record?.inventory_snapshot
  if (!snapshot || typeof snapshot !== 'object') {
    return []
  }
  const hosts = snapshot.hosts
  return Array.isArray(hosts) ? hosts : []
}

export function getWorkflowRunHostList(record) {
  const hosts = record?.runtime_hosts_preview
  return Array.isArray(hosts) ? hosts : []
}

export function buildMergedLogText(record) {
  const stdout = String(record?.stdout || '').trim()
  const stderr = String(record?.stderr || '').trim()

  if (stdout && stderr) {
    return `===== STDOUT =====\n${stdout}\n\n===== STDERR =====\n${stderr}`
  }
  if (stdout) {
    return stdout
  }
  if (stderr) {
    return stderr
  }
  return ''
}

export function toSafeFileSegment(value) {
  return String(value || '')
    .replace(/[\\/:*?"<>|\s]+/g, '_')
    .replace(/^_+|_+$/g, '')
}

export function formatWorkflowDuration(seconds) {
  if (seconds === null || seconds === undefined) return '-'
  const num = Number(seconds)
  if (!Number.isFinite(num) || num < 0) return '-'
  if (num < 1) return `${(num * 1000).toFixed(0)}ms`
  if (num < 60) return `${num.toFixed(2)}s`
  if (num < 3600) {
    const mins = Math.floor(num / 60)
    const secs = (num % 60).toFixed(0)
    return `${mins}m ${secs}s`
  }
  const hours = Math.floor(num / 3600)
  const mins = Math.floor((num % 3600) / 60)
  return `${hours}h ${mins}m`
}

export function getWorkflowRunStatusColor(status) {
  if (status === 'success') return 'green'
  if (status === 'failed') return 'red'
  if (status === 'running') return 'blue'
  return 'default'
}

export function normalizeWorkflowRunStatus(record) {
  return String(record?.runtime_status || record?.status || '').toLowerCase()
}

export function canCancelWorkflowRunRecord(record) {
  return ['pending', 'running'].includes(normalizeWorkflowRunStatus(record))
}

export function normalizeUtcTime(timeValue) {
  if (!timeValue || typeof timeValue !== 'string') {
    return timeValue
  }

  const text = timeValue.trim()
  if (!text) {
    return timeValue
  }

  // Backend naive time should be interpreted as UTC before formatting to user timezone.
  if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
    return text
  }
  return `${text.replace(' ', 'T')}Z`
}
