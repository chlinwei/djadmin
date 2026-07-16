export const DOWNLOAD_ACTION_DEDUP_MS = 800
export const DOWNLOAD_MODE_DIRECT = 'direct'

export const formatBytes = (value) => {
    const bytes = Number(value || 0)
    if (!Number.isFinite(bytes) || bytes < 0) return '0 B'
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

export const formatDuration = (ms) => {
    const totalSeconds = Math.max(0, Math.floor(Number(ms || 0) / 1000))
    const hours = Math.floor(totalSeconds / 3600)
    const minutes = Math.floor((totalSeconds % 3600) / 60)
    const seconds = totalSeconds % 60
    if (hours > 0) {
        return `${hours}h ${String(minutes).padStart(2, '0')}m ${String(seconds).padStart(2, '0')}s`
    }
    return `${minutes}m ${String(seconds).padStart(2, '0')}s`
}

export const formatAverageSpeed = (downloaded, elapsedMs) => {
    const bytes = Number(downloaded || 0)
    const costMs = Number(elapsedMs || 0)
    if (!Number.isFinite(bytes) || !Number.isFinite(costMs) || bytes <= 0 || costMs <= 0) {
        return '0.00 B/s'
    }
    const speed = (bytes * 1000) / costMs
    if (speed < 1024) return `${speed.toFixed(2)} B/s`
    if (speed < 1024 * 1024) return `${(speed / 1024).toFixed(2)} KB/s`
    if (speed < 1024 * 1024 * 1024) return `${(speed / 1024 / 1024).toFixed(2)} MB/s`
    return `${(speed / 1024 / 1024 / 1024).toFixed(2)} GB/s`
}

export const getDownloadModeLabel = (mode) => {
    return String(mode || '') === DOWNLOAD_MODE_DIRECT ? '直连' : '下载'
}

export const supportsStreamFileDownload = () => {
    return Boolean(window.isSecureContext && typeof window.showSaveFilePicker === 'function')
}

export const parseDownloadFilename = (contentDisposition, fallbackName = 'download.bin') => {
    const header = String(contentDisposition || '').trim()
    if (!header) {
        return fallbackName
    }

    const utf8Match = header.match(/filename\*=UTF-8''([^;]+)/i)
    if (utf8Match && utf8Match[1]) {
        try {
            return decodeURIComponent(utf8Match[1]).trim() || fallbackName
        } catch (error) {
            // ignore decode failure and continue fallback parse
        }
    }

    const quotedMatch = header.match(/filename="([^"]+)"/i)
    if (quotedMatch && quotedMatch[1]) {
        return quotedMatch[1].trim() || fallbackName
    }

    const plainMatch = header.match(/filename=([^;]+)/i)
    if (plainMatch && plainMatch[1]) {
        return plainMatch[1].trim().replace(/^"|"$/g, '') || fallbackName
    }

    return fallbackName
}

export const parseResponseError = async (response) => {
    const contentType = response.headers.get('content-type') || ''
    if (contentType.includes('application/json')) {
        try {
            const payload = await response.json()
            if (payload?.msg) {
                return payload.msg
            }
        } catch (error) {
            // ignore parse failure
        }
        return null
    }
    if (contentType.includes('text/plain')) {
        try {
            const text = String(await response.text() || '').trim()
            return text || null
        } catch (error) {
            return null
        }
    }
    return null
}

export const buildDownloadTargetFilename = (record) => {
    const baseName = String(record?.name || '').trim() || 'download'
    return baseName || 'download.bin'
}
