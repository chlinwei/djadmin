export const TRANSFER_LIST_LIMIT = 5
export const DOWNLOAD_ACTION_DEDUP_MS = 800
export const DOWNLOAD_PROGRESS_TICK_MS = 120
export const DOWNLOAD_PROGRESS_SMOOTH_FACTOR = 0.35
export const DOWNLOAD_PROGRESS_MIN_STEP_BYTES = 256 * 1024
export const DOWNLOAD_MODE_DIRECT = 'direct'

export const trimUploadQueueToLimit = ({
    uploadQueue,
    transferListLimit = TRANSFER_LIST_LIMIT,
}) => {
    while (uploadQueue.value.length > transferListLimit) {
        uploadQueue.value.pop()
    }
}

export const getTransferStatusMeta = (status) => {
    const normalized = String(status || '').toLowerCase()
    if (normalized === 'success') return { label: '成功', color: 'success' }
    if (normalized === 'error' || normalized === 'exception' || normalized === 'failed') return { label: '失败', color: 'error' }
    if (normalized === 'canceled' || normalized === 'cancelled') return { label: '已取消', color: 'default' }
    if (normalized === 'paused') return { label: '已暂停', color: 'warning' }
    if (normalized === 'queued') return { label: '排队中', color: 'processing' }
    return { label: '进行中', color: 'processing' }
}

export const buildDownloadRows = ({
    downloadRunning,
    downloadProgressStatus,
    downloadFileName,
    currentDownloadRecord,
    downloadProgressText,
    getStatusMeta = getTransferStatusMeta,
}) => {
    const rows = []
    if (downloadRunning.value) {
        const activeMeta = getStatusMeta(downloadRunning.value ? 'running' : downloadProgressStatus.value)
        rows.push({
            id: 'download-active-row',
            type: 'download-active',
            isCurrent: true,
            name: downloadFileName.value || currentDownloadRecord.value?.name || '下载任务',
            detail: downloadProgressText.value || '-',
            statusLabel: activeMeta.label,
            tagColor: activeMeta.color,
            contextItem: {
                record: currentDownloadRecord.value || null,
            },
        })
    }
    return rows
}

export const buildUploadRows = ({
    uploadRunning,
    uploadProgressStatus,
    uploadFileName,
    currentUploadContext,
    uploadProgressText,
    fileCurrentPath,
    uploadQueue,
    getStatusMeta = getTransferStatusMeta,
}) => {
    const rows = []
    if (uploadRunning.value) {
        const activeMeta = getStatusMeta(uploadRunning.value ? 'running' : uploadProgressStatus.value)
        rows.push({
            id: 'upload-active-row',
            type: 'upload-active',
            isCurrent: true,
            name: uploadFileName.value || currentUploadContext.value?.fileName || '上传任务',
            detail: uploadProgressText.value || '-',
            statusLabel: activeMeta.label,
            tagColor: activeMeta.color,
            contextItem: {
                targetPath: currentUploadContext.value?.targetPath || fileCurrentPath.value || '.',
            },
        })
    }
    uploadQueue.value.forEach((item) => {
        const queueMeta = getStatusMeta(item.paused ? 'paused' : 'queued')
        rows.push({
            id: item.id,
            type: 'upload-queue',
            isCurrent: false,
            name: item?.task?.fileName || item?.task?.file?.name || '未命名文件',
            detail: item?.task?.targetPath || '-',
            statusLabel: queueMeta.label,
            tagColor: queueMeta.color,
            contextItem: item,
        })
    })
    return rows
}

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

    if (record?.is_dir) {
        return baseName.endsWith('.tar.gz') ? baseName : `${baseName}.tar.gz`
    }
    return baseName || 'download.bin'
}
