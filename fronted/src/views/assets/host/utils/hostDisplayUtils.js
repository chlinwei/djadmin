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

// formatBytes 字节数自适应单位（B/KB/MB/GB/TB）；与 formatSize（入参本身就是 GB 值）不同，
// 用于文件大小等以字节为单位的字段，如 Agent 安装包 size_bytes。
export const formatBytes = (value) => {
  if (value === null || value === undefined || value === '') {
    return '-'
  }
  const size = Number(value) || 0
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let index = 0
  let display = size
  while (display >= 1024 && index < units.length - 1) {
    display /= 1024
    index += 1
  }
  return `${index === 0 ? display : display.toFixed(2)} ${units[index]}`
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
