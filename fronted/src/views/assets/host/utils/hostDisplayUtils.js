// OS 类型与版本合并为 "类型:版本" 单列展示，缺一侧时只展示存在的一侧
export const formatOsInfo = (record) => {
  const type = String(record?.os_type || record?.system?.os_type || '').trim()
  const version = String(record?.os_version || record?.system?.os_version || '').trim()
  if (type && version) {
    return `${type}:${version}`
  }
  return type || version || '-'
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
