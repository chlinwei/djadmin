export const getGroupName = (record) => {
  return record.group_name || record.group?.name || '-'
}

export const getCredentialName = (record) => {
  return record.credential_name || record.credential?.name || '-'
}

export const hasHostCredential = (record) => {
  const directName = String(record?.credential_name || '').trim()
  if (directName) {
    return true
  }
  const nestedName = String(record?.credential?.name || '').trim()
  if (nestedName) {
    return true
  }
  const nestedId = Number(record?.credential?.id || 0)
  return Number.isFinite(nestedId) && nestedId > 0
}

export const getDisks = (record) => {
  return record.disks || []
}

export const formatSize = (value) => {
  if (value === null || value === undefined || value === '') {
    return '-'
  }
  return `${value} GB`
}

export const formatPercent = (value) => {
  if (value === null || value === undefined || value === '') {
    return '-'
  }
  return `${Number(value).toFixed(2)}%`
}

export const normalizeUtcTime = (value) => {
  if (!value || typeof value !== 'string') {
    return value
  }
  const text = value.trim()
  if (!text) {
    return value
  }
  if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
    return text
  }
  return `${text.replace(' ', 'T')}Z`
}

export const formatDateTimeWithTimezone = (value, formatTimeWithTimezone, timezone) => {
  if (!value) {
    return '-'
  }
  try {
    return formatTimeWithTimezone(normalizeUtcTime(value), timezone, 'YYYY-MM-DD HH:mm:ss')
  } catch (error) {
    return value
  }
}
