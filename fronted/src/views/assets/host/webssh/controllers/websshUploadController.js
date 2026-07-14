export function createWebsshUploadController(options) {
    const {
        getHostId,
        state,
        refs,
        message,
        ensureFileOperationsEnabled,
        ensureTransferPanelVisible,
        trimUploadQueueToLimit,
        formatBytes,
        uploadHostWebSshFile,
        loadFiles,
    } = options

    const triggerFileDownload = (blob, filename) => {
        const url = window.URL.createObjectURL(blob)
        const anchor = document.createElement('a')
        anchor.href = url
        anchor.download = filename
        document.body.appendChild(anchor)
        anchor.click()
        document.body.removeChild(anchor)
        window.URL.revokeObjectURL(url)
    }

    const resetUploadProgress = () => {
        if (state.uploadAbortController) {
            state.uploadAbortController.abort()
        }
        refs.uploadProgressVisible.value = false
        refs.uploadProgressPercent.value = 0
        refs.uploadProgressStatus.value = 'active'
        refs.uploadProgressText.value = ''
        refs.uploadFileName.value = ''
        refs.uploadRunning.value = false
        refs.uploadQueue.value = []
        refs.currentUploadContext.value = null
    }

    const formatDuration = (ms) => {
        const totalSeconds = Math.max(0, Math.floor(Number(ms || 0) / 1000))
        const minutes = Math.floor(totalSeconds / 60)
        const seconds = totalSeconds % 60
        return `${minutes}m ${String(seconds).padStart(2, '0')}s`
    }

    const formatUploadSpeed = (bytes, elapsedMs) => {
        const value = Number(bytes || 0)
        const cost = Number(elapsedMs || 0)
        if (!Number.isFinite(value) || !Number.isFinite(cost) || value <= 0 || cost <= 0) {
            return '0 B/s'
        }
        const speed = (value * 1000) / cost
        return `${formatBytes(speed)}/s`
    }

    const startUpload = async (task, callbacks = {}) => {
        const { onSuccess, onError, onProgress } = callbacks
        if (!task?.file) {
            message.error('无可上传文件')
            return
        }
        if (!getHostId()) {
            message.error('缺少主机参数')
            return
        }

        const rawFile = task.file
        const fileName = task.fileName || rawFile.name || 'upload.bin'
        const totalSize = Number(task.totalSize || rawFile.size || 0)
        const targetPath = task.targetPath || refs.fileCurrentPath.value || '.'
        let uploadedBytes = 0
        const startedAt = performance.now()
        let lastProgressUiAt = 0
        let hasShownServerConfirmStage = false
        const getElapsedMs = () => Math.max(0, performance.now() - startedAt)
        refs.currentUploadContext.value = {
            fileName,
            targetPath,
        }

        try {
            state.uploadAbortController = new AbortController()
            refs.uploadRunning.value = true
            refs.uploadProgressVisible.value = true
            refs.uploadProgressStatus.value = 'active'
            refs.uploadFileName.value = fileName
            refs.uploadProgressPercent.value = Number(Math.min(100, (uploadedBytes / Math.max(totalSize, 1)) * 100).toFixed(1))
            refs.uploadProgressText.value = '正在准备上传...'

            const formData = new FormData()
            formData.append('path', targetPath)
            formData.append('filename', fileName)
            formData.append('file', rawFile, fileName)

            refs.uploadProgressText.value = '正在连接目标主机...'
            await uploadHostWebSshFile(getHostId(), formData, {
                timeout: 15 * 60 * 1000,
                signal: state.uploadAbortController.signal,
                onUploadProgress: (event) => {
                    uploadedBytes = Math.min(totalSize, Number(event?.loaded || 0))
                    const elapsedMs = getElapsedMs()
                    const transferPercent = (uploadedBytes / Math.max(totalSize, 1)) * 100
                    const now = performance.now()
                    const reachedServerConfirmStage = uploadedBytes >= totalSize && totalSize > 0
                    const shouldRefreshUi = reachedServerConfirmStage
                        ? !hasShownServerConfirmStage
                        : (now - lastProgressUiAt) >= 120

                    if (!shouldRefreshUi) {
                        return
                    }

                    lastProgressUiAt = now
                    hasShownServerConfirmStage = reachedServerConfirmStage
                    // Keep progress below 100% until server confirms remote write success.
                    refs.uploadProgressPercent.value = Number(Math.min(99, transferPercent).toFixed(1))
                    if (reachedServerConfirmStage) {
                        refs.uploadProgressText.value = `${formatBytes(uploadedBytes)} / ${formatBytes(totalSize)} | 正在等待服务端确认...`
                    } else {
                        refs.uploadProgressText.value = `${formatBytes(uploadedBytes)} / ${formatBytes(totalSize)} | 平均: ${formatUploadSpeed(uploadedBytes, elapsedMs)} | 耗时: ${formatDuration(elapsedMs)}`
                    }
                    if (onProgress) {
                        onProgress({ percent: refs.uploadProgressPercent.value })
                    }
                },
            })

            refs.uploadProgressPercent.value = 100
            refs.uploadProgressStatus.value = 'success'
            const elapsedMs = getElapsedMs()
            refs.uploadProgressText.value = `上传完成 | 平均: ${formatUploadSpeed(totalSize, elapsedMs)} | 总耗时: ${formatDuration(elapsedMs)}`
            message.success('文件上传成功')
            await loadFiles(refs.fileCurrentPath.value)
            if (onSuccess) onSuccess()
        } catch (error) {
            const isCanceled = error?.name === 'CanceledError' || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED'
            if (isCanceled) {
                refs.uploadProgressVisible.value = false
                refs.uploadProgressText.value = ''
                refs.uploadFileName.value = ''
                refs.uploadProgressPercent.value = 0
                refs.uploadProgressStatus.value = 'active'
                message.info('已取消上传')
                return
            }
            refs.uploadProgressStatus.value = 'exception'
            refs.uploadProgressText.value = error?.message || '上传失败'
            message.error(error?.message || '文件上传失败')
            if (onError) onError(error)
        } finally {
            refs.uploadRunning.value = false
            state.uploadAbortController = null
            void startNextUploadFromQueue()
        }
    }

    const startNextUploadFromQueue = async () => {
        if (refs.uploadRunning.value) return
        const nextItem = refs.uploadQueue.value.shift()
        if (!nextItem) return
        await startUpload(nextItem.task, nextItem.callbacks)
    }

    const enqueueUploadTask = (task, callbacks = {}) => {
        ensureTransferPanelVisible()
        refs.uploadQueue.value.unshift({
            id: `upload-${Date.now()}-${state.nextUploadSeq()}`,
            task,
            callbacks,
        })
        trimUploadQueueToLimit()
        if (refs.uploadRunning.value) {
            message.info(`已加入上传队列（待处理 ${refs.uploadQueue.value.length} 个）`)
            return
        }
        void startNextUploadFromQueue()
    }

    const removeUploadQueueItem = (queueId) => {
        const nextQueue = refs.uploadQueue.value.filter((item) => item.id !== queueId)
        refs.uploadQueue.value = nextQueue
    }

    const uploadRawFileToPath = async (rawFile, targetPath, callbacks = {}) => {
        const { onSuccess, onError, onProgress } = callbacks
        if (!rawFile) {
            message.error('请选择文件')
            return
        }
        const totalSize = Number(rawFile?.size || 0)
        const task = {
            file: rawFile,
            fileName: rawFile.name,
            targetPath: targetPath || refs.fileCurrentPath.value || '.',
            totalSize,
        }
        enqueueUploadTask(task, { onSuccess, onError, onProgress })
    }

    const uploadFile = async ({ file, onSuccess, onError, onProgress }) => {
        if (!ensureFileOperationsEnabled()) return
        const rawFile = file?.originFileObj || file
        await uploadRawFileToPath(rawFile, refs.fileCurrentPath.value || '.', {
            onSuccess,
            onError,
            onProgress,
        })
    }

    const cancelUpload = async () => {
        if (state.uploadAbortController) {
            state.uploadAbortController.abort()
            return
        }
        resetUploadProgress()
        message.info('已取消上传')
    }

    return {
        triggerFileDownload,
        resetUploadProgress,
        startNextUploadFromQueue,
        enqueueUploadTask,
        removeUploadQueueItem,
        uploadRawFileToPath,
        uploadFile,
        cancelUpload,
    }
}
