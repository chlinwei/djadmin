import { formatTimeWithTimezone } from '@/util/timezone'

/**
 * 将不带时区标识的 UTC 时间字符串补全为带 Z 后缀的 ISO 格式。
 * 已带时区标识（Z / +HH:MM）的字符串原样返回。
 */
export const normalizeUtcTime = (timeValue) => {
    if (!timeValue || typeof timeValue !== 'string') {
        return timeValue
    }
    const text = timeValue.trim()
    if (!text) {
        return timeValue
    }
    if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
        return text
    }
    return `${text.replace(' ', 'T')}Z`
}

/**
 * 将字节数格式化为可读字符串（B / KB / MB / GB）。
 */
export const formatFileSize = (size) => {
    if (size === null || size === undefined) return '-'
    const bytes = Number(size)
    if (!Number.isFinite(bytes) || bytes < 0) return '-'
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
    return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

/**
 * 根据路径计算父目录路径。
 */
export const resolveFileParentDirectory = (path) => {
    const rawPath = String(path || '').trim()
    if (!rawPath || rawPath === '.') return '.'
    if (rawPath === '/') return '/'
    const normalized = rawPath.endsWith('/') && rawPath.length > 1 ? rawPath.slice(0, -1) : rawPath
    const separatorIndex = normalized.lastIndexOf('/')
    if (separatorIndex < 0) return '.'
    if (separatorIndex === 0) return '/'
    return normalized.slice(0, separatorIndex)
}

/**
 * 将 UTC 时间字符串按指定时区格式化。
 * @param {string} value - UTC 时间字符串
 * @param {string} timezone - IANA 时区名称
 */
export const formatDateTime = (value, timezone) => {
    if (!value) return '-'
    return formatTimeWithTimezone(
        normalizeUtcTime(value),
        timezone || 'Asia/Shanghai',
        'YYYY-MM-DD HH:mm:ss',
    )
}

/**
 * 将 SFTP st_mtime（Unix 秒级时间戳）按指定时区格式化。
 * @param {number} value - Unix 时间戳（秒）
 * @param {string} timezone - IANA 时区名称
 */
export const formatFileMtime = (value, timezone) => {
    if (!value && value !== 0) return '-'
    return formatDateTime(new Date(Number(value) * 1000).toISOString(), timezone)
}
