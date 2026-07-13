export function createWebsshUploadController(options) {
    const {
        getHostId,
        UPLOAD_CHUNK_SIZE,
        state,
        refs,
        message,
        ensureFileOperationsEnabled,
        ensureTransferPanelVisible,
        trimUploadQueueToLimit,
        formatBytes,
        saveUploadResumeTask,
        clearUploadResumeTask,
        getHostWebSshUploadTicket,
        getHostWebSshFileUploadStatusByTicket,
        uploadHostWebSshFileChunkByTicket,
        cancelHostWebSshFileUploadByTicket,
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
            state.uploadStopAction = 'cancel'
            state.uploadAbortController.abort()
        }
        state.pendingUploadTask = null
        refs.uploadProgressVisible.value = false
        refs.uploadProgressPercent.value = 0
        refs.uploadProgressStatus.value = 'active'
        refs.uploadProgressText.value = ''
        refs.uploadFileName.value = ''
        refs.uploadCanResume.value = false
        refs.uploadRunning.value = false
        refs.uploadQueue.value = []
        refs.currentUploadContext.value = null
        clearUploadResumeTask()
    }

    const createUploadId = () => {
        return `${Date.now()}_${Math.random().toString(36).slice(2, 10)}`
    }

    const issueUploadTicket = async (targetPath, fileName) => {
        const res = await getHostWebSshUploadTicket(getHostId(), {
            path: targetPath || '.',
            filename: fileName,
        })
        if (res?.data?.code !== 200) {
            throw new Error(res?.data?.msg || '上传票据获取失败')
        }
        const ticket = String(res?.data?.data?.ticket || '').trim()
        if (!ticket) {
            throw new Error('上传票据无效')
        }
        return ticket
    }

    const syncUploadTaskWithServer = async (task) => {
        if (!getHostId() || !task?.uploadId || !task?.fileName || !task?.uploadTicket) {
            return task
        }
        const res = await getHostWebSshFileUploadStatusByTicket({
            ticket: task.uploadTicket,
            upload_id: task.uploadId,
            chunk_size: task.chunkSize || UPLOAD_CHUNK_SIZE,
        })
        if (res?.data?.code !== 200) {
            return task
        }
        const payload = res?.data?.data || {}
        const nextChunkIndex = Number(payload.next_chunk_index || 0)
        const uploadedBytes = Number(payload.uploaded_size || 0)
        return {
            ...task,
            nextChunkIndex: Math.max(0, nextChunkIndex),
            uploadedBytes: Math.max(0, uploadedBytes),
        }
    }

    const restoreUploadResumeTask = async () => {
        if (!getHostId()) return
        const raw = window.localStorage.getItem(refs.getUploadResumeStorageKey(getHostId()))
        if (!raw) return
        let parsed = null
        try {
            parsed = JSON.parse(raw)
        } catch (error) {
            clearUploadResumeTask()
            return
        }
        if (!parsed?.uploadId || !parsed?.fileName) {
            clearUploadResumeTask()
            return
        }
        let uploadTicket = String(parsed.uploadTicket || '').trim()
        if (!uploadTicket) {
            try {
                uploadTicket = await issueUploadTicket(parsed.targetPath || '.', parsed.fileName)
            } catch (error) {
                // keep empty ticket and let user retry via resume
            }
        }
        state.pendingUploadTask = {
            file: null,
            fileName: parsed.fileName,
            targetPath: parsed.targetPath || '.',
            uploadId: parsed.uploadId,
            uploadTicket,
            totalSize: Number(parsed.totalSize || 0),
            totalChunks: Number(parsed.totalChunks || 0),
            nextChunkIndex: Number(parsed.nextChunkIndex || 0),
            uploadedBytes: Number(parsed.uploadedBytes || 0),
            chunkSize: Number(parsed.chunkSize || UPLOAD_CHUNK_SIZE),
        }
        try {
            state.pendingUploadTask = await syncUploadTaskWithServer(state.pendingUploadTask)
        } catch (error) {
            // ignore sync error and use local cached state
        }
        refs.uploadProgressVisible.value = true
        refs.uploadProgressStatus.value = 'normal'
        refs.uploadRunning.value = false
        refs.uploadCanResume.value = true
        refs.uploadFileName.value = state.pendingUploadTask.fileName
        refs.uploadProgressPercent.value = Number(
            Math.min(100, (state.pendingUploadTask.uploadedBytes / Math.max(state.pendingUploadTask.totalSize || 1, 1)) * 100).toFixed(1),
        )
        refs.uploadProgressText.value = '检测到未完成上传，请重新选择同名文件后点击“继续上传”'
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
        const totalChunks = Number(task.totalChunks || Math.max(1, Math.ceil(totalSize / UPLOAD_CHUNK_SIZE)))
        const targetPath = task.targetPath || refs.fileCurrentPath.value || '.'
        const uploadId = task.uploadId || createUploadId()
        let uploadTicket = String(task.uploadTicket || '').trim()
        const chunkSize = Number(task.chunkSize || UPLOAD_CHUNK_SIZE)
        let nextChunkIndex = Number(task.nextChunkIndex || 0)
        let uploadedBytes = Number(task.uploadedBytes || 0)
        refs.currentUploadContext.value = {
            fileName,
            targetPath,
        }

        try {
            uploadTicket = await issueUploadTicket(targetPath, fileName)
            const normalizedTask = await syncUploadTaskWithServer({
                ...task,
                file: rawFile,
                fileName,
                totalSize,
                totalChunks,
                targetPath,
                uploadId,
                uploadTicket,
                chunkSize,
                nextChunkIndex,
                uploadedBytes,
            })
            nextChunkIndex = Number(normalizedTask.nextChunkIndex || 0)
            uploadedBytes = Number(normalizedTask.uploadedBytes || 0)

            state.uploadAbortController = new AbortController()
            state.uploadStopAction = null
            refs.uploadRunning.value = true
            refs.uploadProgressVisible.value = true
            refs.uploadProgressStatus.value = 'active'
            refs.uploadCanResume.value = false
            refs.uploadFileName.value = fileName
            refs.uploadProgressPercent.value = Number(Math.min(100, (uploadedBytes / Math.max(totalSize, 1)) * 100).toFixed(1))
            refs.uploadProgressText.value = `${formatBytes(uploadedBytes)} / ${formatBytes(totalSize)}`

            while (nextChunkIndex < totalChunks) {
                const chunkStart = nextChunkIndex * chunkSize
                const chunkEnd = Math.min(totalSize, chunkStart + chunkSize)
                const chunkBlob = rawFile.slice(chunkStart, chunkEnd)
                const chunkBase = chunkStart

                const formData = new FormData()
                formData.append('ticket', uploadTicket)
                formData.append('upload_id', uploadId)
                formData.append('chunk_index', String(nextChunkIndex))
                formData.append('total_chunks', String(totalChunks))
                formData.append('chunk', chunkBlob, `${fileName}.part${nextChunkIndex}`)

                await uploadHostWebSshFileChunkByTicket(formData, {
                    signal: state.uploadAbortController.signal,
                    onUploadProgress: (event) => {
                        const loaded = Number(event?.loaded || 0)
                        const currentUploaded = Math.min(totalSize, chunkBase + loaded)
                        refs.uploadProgressPercent.value = Number(Math.min(100, (currentUploaded / Math.max(totalSize, 1)) * 100).toFixed(1))
                        refs.uploadProgressText.value = `${formatBytes(currentUploaded)} / ${formatBytes(totalSize)}`
                        if (onProgress) {
                            onProgress({ percent: refs.uploadProgressPercent.value })
                        }
                    },
                })

                uploadedBytes = chunkEnd
                nextChunkIndex += 1
                refs.uploadProgressPercent.value = Number(Math.min(100, (uploadedBytes / Math.max(totalSize, 1)) * 100).toFixed(1))
                refs.uploadProgressText.value = `${formatBytes(uploadedBytes)} / ${formatBytes(totalSize)}`
            }

            state.pendingUploadTask = null
            clearUploadResumeTask()
            refs.uploadProgressPercent.value = 100
            refs.uploadProgressStatus.value = 'success'
            refs.uploadProgressText.value = '上传完成'
            message.success('文件上传成功')
            await loadFiles(refs.fileCurrentPath.value)
            if (onSuccess) onSuccess()
        } catch (error) {
            const isCanceled = error?.name === 'CanceledError' || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED'
            if (isCanceled) {
                if (state.uploadStopAction === 'pause') {
                    state.pendingUploadTask = {
                        file: rawFile,
                        fileName,
                        targetPath,
                        uploadId,
                        uploadTicket,
                        totalSize,
                        totalChunks,
                        nextChunkIndex,
                        uploadedBytes,
                        chunkSize,
                    }
                    saveUploadResumeTask()
                    refs.uploadProgressStatus.value = 'normal'
                    refs.uploadCanResume.value = nextChunkIndex > 0 && nextChunkIndex < totalChunks
                    refs.uploadProgressText.value = '已暂停'
                    message.info('上传已暂停')
                } else {
                    state.pendingUploadTask = null
                    clearUploadResumeTask()
                    if (uploadId && fileName) {
                        try {
                            await cancelHostWebSshFileUploadByTicket({ ticket: uploadTicket, upload_id: uploadId })
                        } catch (cancelError) {
                            // ignore cleanup failure, cancel should still complete
                        }
                    }
                    refs.uploadProgressVisible.value = false
                    refs.uploadProgressText.value = ''
                    refs.uploadFileName.value = ''
                    refs.uploadProgressPercent.value = 0
                    refs.uploadCanResume.value = false
                    message.info('已取消上传')
                }
                return
            }
            state.pendingUploadTask = {
                file: rawFile,
                fileName,
                targetPath,
                uploadId,
                uploadTicket,
                totalSize,
                totalChunks,
                nextChunkIndex,
                uploadedBytes,
                chunkSize,
            }
            saveUploadResumeTask()
            refs.uploadProgressStatus.value = 'exception'
            refs.uploadCanResume.value = nextChunkIndex > 0 && nextChunkIndex < totalChunks
            refs.uploadProgressText.value = error?.message || '上传失败'
            message.error(error?.message || '文件上传失败')
            if (onError) onError(error)
        } finally {
            const finalStopAction = state.uploadStopAction
            refs.uploadRunning.value = false
            state.uploadStopAction = null
            state.uploadAbortController = null
            if (finalStopAction !== 'pause') {
                void startNextUploadFromQueue()
            }
        }
    }

    const startNextUploadFromQueue = async () => {
        if (refs.uploadRunning.value || state.pendingUploadTask) return
        const nextIndex = refs.uploadQueue.value.findIndex((item) => !item.paused)
        if (nextIndex < 0) return
        const [nextItem] = refs.uploadQueue.value.splice(nextIndex, 1)
        if (!nextItem) return
        await startUpload(nextItem.task, nextItem.callbacks)
    }

    const enqueueUploadTask = (task, callbacks = {}) => {
        ensureTransferPanelVisible()
        refs.uploadQueue.value.unshift({
            id: `upload-${Date.now()}-${state.nextUploadSeq()}`,
            task,
            callbacks,
            paused: false,
        })
        trimUploadQueueToLimit()
        if (refs.uploadRunning.value || state.pendingUploadTask) {
            message.info(`已加入上传队列（待处理 ${refs.uploadQueue.value.length} 个）`)
            return
        }
        void startNextUploadFromQueue()
    }

    const toggleUploadQueueItem = (queueId) => {
        const queueItem = refs.uploadQueue.value.find((item) => item.id === queueId)
        if (!queueItem) return
        queueItem.paused = !queueItem.paused
        if (!queueItem.paused && !refs.uploadRunning.value && !state.pendingUploadTask) {
            void startNextUploadFromQueue()
        }
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
        if (state.pendingUploadTask && !state.pendingUploadTask.file) {
            const expectedName = String(state.pendingUploadTask.fileName || '')
            const expectedSize = Number(state.pendingUploadTask.totalSize || 0)
            if (rawFile.name !== expectedName || Number(rawFile.size || 0) !== expectedSize) {
                message.error(`请重新选择同一文件继续上传：${expectedName}`)
                return
            }
            state.pendingUploadTask.file = rawFile
            await startUpload(state.pendingUploadTask, { onSuccess, onError, onProgress })
            return
        }
        const totalSize = Number(rawFile?.size || 0)
        const totalChunks = Math.max(1, Math.ceil(totalSize / UPLOAD_CHUNK_SIZE))
        const task = {
            file: rawFile,
            fileName: rawFile.name,
            targetPath: targetPath || refs.fileCurrentPath.value || '.',
            uploadId: createUploadId(),
            uploadTicket: '',
            totalSize,
            totalChunks,
            nextChunkIndex: 0,
            uploadedBytes: 0,
            chunkSize: UPLOAD_CHUNK_SIZE,
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

    const resumeUpload = async () => {
        if (!state.pendingUploadTask) return
        if (!state.pendingUploadTask.file) {
            message.warning('请先点击“上传文件”重新选择同名文件，再继续上传')
            return
        }
        const task = state.pendingUploadTask
        state.pendingUploadTask = null
        await startUpload(task)
    }

    const pauseUpload = () => {
        state.uploadStopAction = 'pause'
        if (state.uploadAbortController) {
            state.uploadAbortController.abort()
        }
    }

    const cancelUpload = async () => {
        state.uploadStopAction = 'cancel'
        if (state.uploadAbortController) {
            state.uploadAbortController.abort()
            return
        }
        if (state.pendingUploadTask?.uploadId && (state.pendingUploadTask?.fileName || state.pendingUploadTask?.file?.name)) {
            try {
                let uploadTicket = String(state.pendingUploadTask.uploadTicket || '').trim()
                if (!uploadTicket) {
                    uploadTicket = await issueUploadTicket(
                        state.pendingUploadTask.targetPath || refs.fileCurrentPath.value || '.',
                        state.pendingUploadTask.fileName || state.pendingUploadTask.file.name,
                    )
                }
                await cancelHostWebSshFileUploadByTicket({
                    ticket: uploadTicket,
                    upload_id: state.pendingUploadTask.uploadId,
                })
            } catch (error) {
                // ignore cleanup failure when canceling local pending task
            }
        }
        resetUploadProgress()
        message.info('已取消上传')
    }

    return {
        triggerFileDownload,
        resetUploadProgress,
        restoreUploadResumeTask,
        startNextUploadFromQueue,
        enqueueUploadTask,
        toggleUploadQueueItem,
        removeUploadQueueItem,
        uploadRawFileToPath,
        uploadFile,
        resumeUpload,
        pauseUpload,
        cancelUpload,
    }
}
