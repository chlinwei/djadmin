export function createWebsshDownloadController(options) {
    const {
        nextTick,
        getHostId,
        constants,
        state,
        refs,
        message,
        helpers,
        deps,
    } = options

    const applyDownloadProgressDisplay = (downloaded, total, elapsedMs = 0) => {
        const totalSize = Number(total || 0)
        const doneSize = Number(downloaded || 0)
        if (!Number.isFinite(totalSize) || totalSize <= 0) {
            const avgSpeed = helpers.formatAverageSpeed(doneSize, elapsedMs)
            const totalTime = helpers.formatDuration(elapsedMs)
            // Directory tar stream has unknown total size; estimate a smooth progress curve and cap at 95% until completion.
            if (refs.downloadRunning.value) {
                const doneMb = doneSize / (1024 * 1024)
                const estimated = Math.log2(1 + Math.max(doneMb, 0)) * 12
                refs.downloadProgressPercent.value = Number(Math.min(95, Math.max(1, estimated)).toFixed(1))
            } else {
                refs.downloadProgressPercent.value = 100
            }
            if (doneSize > 0) {
                refs.downloadProgressText.value = `已下载 ${helpers.formatBytes(doneSize)} | 平均: ${avgSpeed} | 耗时: ${totalTime} | 总大小未知`
            } else {
                refs.downloadProgressText.value = '正在下载（总大小未知）...'
            }
            return
        }
        const rawPercent = (doneSize / totalSize) * 100
        if (doneSize > 0 && rawPercent > 0 && rawPercent < 0.1) {
            refs.downloadProgressPercent.value = 0.1
        } else {
            refs.downloadProgressPercent.value = Number(Math.min(100, rawPercent).toFixed(1))
        }
        const avgSpeed = helpers.formatAverageSpeed(doneSize, elapsedMs)
        const totalTime = helpers.formatDuration(elapsedMs)
        refs.downloadProgressText.value = `${helpers.formatBytes(doneSize)} / ${helpers.formatBytes(totalSize)} | 平均: ${avgSpeed} | 耗时: ${totalTime}`
    }

    const stopDownloadProgressTicker = () => {
        if (state.downloadProgressTicker) {
            window.clearInterval(state.downloadProgressTicker)
            state.downloadProgressTicker = null
        }
    }

    const tickDownloadProgress = () => {
        const now = performance.now()
        const tickDelta = Math.max(0, now - state.downloadProgressTickAt)
        state.downloadProgressTickAt = now
        if (refs.downloadRunning.value) {
            state.downloadProgressElapsedMs += tickDelta
        }
        const totalSize = Number(state.downloadProgressTotalBytes || 0)
        const targetDone = Math.min(
            Number(state.downloadProgressActualBytes || 0),
            totalSize > 0 ? totalSize : Number(state.downloadProgressActualBytes || 0),
        )
        if (state.downloadProgressDisplayBytes < targetDone) {
            const gap = targetDone - state.downloadProgressDisplayBytes
            const step = Math.max(constants.DOWNLOAD_PROGRESS_MIN_STEP_BYTES, gap * constants.DOWNLOAD_PROGRESS_SMOOTH_FACTOR)
            state.downloadProgressDisplayBytes = Math.min(targetDone, state.downloadProgressDisplayBytes + step)
        } else if (state.downloadProgressDisplayBytes > targetDone) {
            state.downloadProgressDisplayBytes = targetDone
        }
        applyDownloadProgressDisplay(state.downloadProgressDisplayBytes, totalSize, state.downloadProgressElapsedMs)
        if (!refs.downloadRunning.value && state.downloadProgressDisplayBytes >= targetDone) {
            stopDownloadProgressTicker()
        }
    }

    const startDownloadProgressTicker = () => {
        if (state.downloadProgressTicker) return
        state.downloadProgressTickAt = performance.now()
        state.downloadProgressTicker = window.setInterval(tickDownloadProgress, constants.DOWNLOAD_PROGRESS_TICK_MS)
    }

    const updateDownloadProgress = (downloaded, total, elapsedMs = 0) => {
        state.downloadProgressActualBytes = Math.max(0, Number(downloaded || 0))
        state.downloadProgressTotalBytes = Math.max(0, Number(total || 0))
        state.downloadProgressElapsedMs = Math.max(0, Number(elapsedMs || 0))
        if (!Number.isFinite(state.downloadProgressDisplayBytes) || state.downloadProgressDisplayBytes < 0) {
            state.downloadProgressDisplayBytes = 0
        }
        if (state.downloadProgressDisplayBytes > state.downloadProgressActualBytes) {
            state.downloadProgressDisplayBytes = state.downloadProgressActualBytes
        }
        if (!refs.downloadRunning.value) {
            state.downloadProgressDisplayBytes = state.downloadProgressActualBytes
            applyDownloadProgressDisplay(state.downloadProgressDisplayBytes, state.downloadProgressTotalBytes, state.downloadProgressElapsedMs)
            stopDownloadProgressTicker()
            return
        }
        startDownloadProgressTicker()
        tickDownloadProgress()
    }

    const issueDownloadTicketUrl = async (path) => {
        const ticketResponse = await deps.getHostWebSshDownloadTicket(getHostId(), { path })
        if (ticketResponse?.data?.code !== 200) {
            throw new Error(ticketResponse?.data?.msg || '下载票据获取失败')
        }
        const payload = ticketResponse?.data?.data || {}
        return helpers.resolveDownloadUrlFromPayload(payload, deps.getTransferServerUrl)
    }

    const requestDownloadSaveHandle = async (record) => {
        if (!helpers.supportsStreamFileDownload()) {
            return null
        }
        try {
            return await window.showSaveFilePicker({
                suggestedName: record?.targetFilename || record?.name || 'download.bin',
            })
        } catch (error) {
            if (error?.name === 'AbortError') {
                throw new Error('已取消选择保存位置')
            }
            throw error
        }
    }

    const startNextDownloadFromQueue = async () => {
        if (refs.downloadRunning.value) return
        const nextIndex = refs.downloadQueue.value.findIndex((item) => !item.paused)
        if (nextIndex < 0) return
        const [nextItem] = refs.downloadQueue.value.splice(nextIndex, 1)
        await downloadFile(
            nextItem.record,
            {
                targetFilename: nextItem.record?.targetFilename || nextItem.record?.name || (nextItem.record?.is_dir ? 'directory.tar.gz' : 'download.bin'),
                downloadMode: nextItem.downloadMode || constants.DOWNLOAD_MODE_TICKET_STREAM,
            },
            nextItem.saveHandle || null,
        )
    }

    const enqueueDownloadTask = async (record, downloadMode = constants.DOWNLOAD_MODE_TICKET_STREAM) => {
        const recordPath = String(record?.path || '').trim()
        const actionKey = `${recordPath}|${downloadMode}`
        const now = Date.now()
        if (
            actionKey
            && actionKey === state.lastDownloadActionKey
            && now - state.lastDownloadActionAt < constants.DOWNLOAD_ACTION_DEDUP_MS
        ) {
            return
        }
        state.lastDownloadActionKey = actionKey
        state.lastDownloadActionAt = now

        const targetFilename = helpers.buildDownloadTargetFilename(record)
        let saveHandle
        try {
            saveHandle = await requestDownloadSaveHandle({
                ...record,
                targetFilename,
            })
        } catch (error) {
            const text = error?.message || '选择保存位置失败'
            if (text !== '已取消选择保存位置') {
                message.error(text)
            }
            return
        }

        if (refs.downloadRunning.value) {
            message.warning('当前有下载任务进行中，请稍后再试')
            return
        }
        await downloadFile(record, { targetFilename, downloadMode }, saveHandle)
    }

    const removeDownloadQueueItem = (queueId) => {
        const nextQueue = refs.downloadQueue.value.filter((item) => item.id !== queueId)
        refs.downloadQueue.value = nextQueue
    }

    const clearDownloadProgressUi = () => {
        refs.downloadProgressVisible.value = false
        refs.downloadProgressStatus.value = 'active'
        refs.downloadProgressText.value = ''
        refs.downloadFileName.value = ''
        refs.downloadProgressPercent.value = 0
    }

    const downloadFile = async (record, resumeState = null, selectedSaveHandle = null) => {
        if (!getHostId()) {
            message.error('缺少主机参数')
            return
        }
        refs.currentDownloadRecord.value = record
        let finalStopAction = null

        let downloadUrl = String(resumeState?.downloadUrl || '')
        const downloadMode = String(resumeState?.downloadMode || constants.DOWNLOAD_MODE_TICKET_STREAM)
        const downloadModeLabel = helpers.getDownloadModeLabel(downloadMode)
        const targetFilename = String(resumeState?.targetFilename || helpers.buildDownloadTargetFilename(record))
        let useStreamingDownload = Boolean(selectedSaveHandle) || helpers.supportsStreamFileDownload()
        let useStreamWriter = false
        let fileWriter = null
        let saveHandle = selectedSaveHandle || resumeState?.saveHandle || null
        let fileSize = Number(resumeState?.fileSize || 0)
        let downloaded = Number(resumeState?.downloaded || 0)
        let elapsedMs = Number(resumeState?.elapsedMs || 0)
        const startedAt = performance.now()
        const getElapsedMs = () => elapsedMs + Math.max(0, performance.now() - startedAt)
        state.downloadStopAction = null
        state.downloadAbortController = new AbortController()
        refs.downloadRunning.value = true

        try {
            refs.downloadProgressVisible.value = true
            refs.downloadProgressStatus.value = 'active'
            refs.downloadFileName.value = targetFilename
            refs.downloadProgressText.value = `正在准备下载（${downloadModeLabel}）...`
            if (downloaded <= 0) {
                refs.downloadProgressPercent.value = 0
            }
            await nextTick()
            message.info(`开始下载（${downloadModeLabel}），请查看页面顶部进度条`)

            if (!downloadUrl) {
                downloadUrl = await issueDownloadTicketUrl(record.path)
            }

            if (!fileSize) {
                const listedFileSize = Number(record.size || 0)
                if (Number.isFinite(listedFileSize) && listedFileSize > 0) {
                    fileSize = listedFileSize
                }
            }

            if (!saveHandle) {
                saveHandle = await requestDownloadSaveHandle({
                    ...record,
                    targetFilename,
                })
                useStreamingDownload = Boolean(saveHandle)
            }
            if (!useStreamingDownload || !saveHandle) {
                refs.downloadProgressText.value = '当前环境不支持选择本地路径，已使用浏览器默认下载目录'
                useStreamWriter = false
            } else {
                fileWriter = await saveHandle.createWritable()
                useStreamWriter = true
            }
            refs.downloadProgressText.value = `正在建立流式传输通道（${downloadModeLabel}）...`
            const response = await fetch(downloadUrl, {
                method: 'GET',
                signal: state.downloadAbortController.signal,
            })
            if (response.status !== 200 && response.status !== 206) {
                const messageText = await helpers.parseResponseError(response)
                throw new Error(messageText || `下载失败（HTTP ${response.status}）`)
            }
            const contentRange = response.headers.get('content-range') || ''
            const contentLength = Number(response.headers.get('content-length') || '0')
            const totalMatch = contentRange.match(/\/(\d+)$/)
            const streamTotalSize = totalMatch?.[1] ? Number(totalMatch[1]) : (Number.isFinite(contentLength) ? contentLength : fileSize)
            if (streamTotalSize > 0) {
                fileSize = streamTotalSize
            }
            const reader = response.body?.getReader()
            if (!reader) {
                throw new Error('浏览器不支持流式下载')
            }
            const streamChunks = []
            downloaded = 0
            updateDownloadProgress(downloaded, fileSize, getElapsedMs())
            while (true) {
                if (state.downloadStopAction === 'cancel') {
                    const abortError = new Error('下载已取消')
                    abortError.name = 'AbortError'
                    throw abortError
                }
                const { done, value } = await reader.read()
                if (done) {
                    break
                }
                if (!value || !value.length) {
                    continue
                }
                if (useStreamWriter && fileWriter) {
                    await fileWriter.write(value)
                } else {
                    streamChunks.push(value)
                }
                downloaded += value.length
                updateDownloadProgress(downloaded, fileSize, getElapsedMs())
            }
            if (useStreamWriter && fileWriter) {
                await fileWriter.close()
                fileWriter = null
            } else {
                const blob = new Blob(streamChunks, { type: 'application/octet-stream' })
                helpers.triggerFileDownload(blob, targetFilename)
            }
            refs.downloadProgressStatus.value = 'success'
            elapsedMs = getElapsedMs()
            updateDownloadProgress(downloaded, fileSize, elapsedMs)
            stopDownloadProgressTicker()
            const avgSpeed = helpers.formatAverageSpeed(downloaded, elapsedMs)
            const totalTime = helpers.formatDuration(elapsedMs)
            refs.downloadProgressPercent.value = 100
            refs.downloadProgressText.value = `下载完成 | 平均: ${avgSpeed} | 总耗时: ${totalTime}`
            message.success('文件下载成功')
        } catch (error) {
            const cancelErrorText = String(error?.message || '')
            const isCanceled = error?.name === 'AbortError'
                || state.downloadStopAction === 'cancel'
                || /aborted|canceled|cancelled|取消/i.test(cancelErrorText)

            if (isCanceled) {
                stopDownloadProgressTicker()
                if (useStreamWriter && fileWriter) {
                    try {
                        await fileWriter.abort?.()
                    } catch (writerAbortError) {
                        // ignore abort errors during cancellation
                    }
                    fileWriter = null
                }
                clearDownloadProgressUi()
                message.info('已取消下载')
                return
            }
            stopDownloadProgressTicker()
            refs.downloadProgressStatus.value = 'exception'
            const errorText = error?.message || '下载失败'
            refs.downloadProgressText.value = `下载失败：${errorText}`
            message.error(errorText || '文件下载失败')
            clearDownloadProgressUi()
        } finally {
            finalStopAction = state.downloadStopAction
            refs.downloadRunning.value = false
            state.downloadStopAction = null
            state.downloadAbortController = null
            if (finalStopAction === 'cancel') {
                // Cancellation should always leave the header clean, even if browser errors are non-standard.
                clearDownloadProgressUi()
            }
            void startNextDownloadFromQueue()
        }
    }

    const cancelActiveDownload = () => {
        if (state.downloadStopAction === 'cancel') {
            return
        }
        state.downloadStopAction = 'cancel'
        if (state.downloadAbortController) {
            state.downloadAbortController.abort()
            return
        }
        resetDownloadProgress()
    }

    const resetDownloadProgress = () => {
        stopDownloadProgressTicker()
        if (state.downloadAbortController) {
            state.downloadStopAction = 'cancel'
            state.downloadAbortController.abort()
        }
        state.downloadProgressActualBytes = 0
        state.downloadProgressDisplayBytes = 0
        state.downloadProgressTotalBytes = 0
        state.downloadProgressElapsedMs = 0
        state.downloadProgressTickAt = 0
        refs.downloadProgressVisible.value = false
        refs.downloadProgressPercent.value = 0
        refs.downloadProgressStatus.value = 'active'
        refs.downloadProgressText.value = ''
        refs.downloadFileName.value = ''
        refs.downloadRunning.value = false
        refs.downloadQueue.value = []
        refs.currentDownloadRecord.value = null
    }

    const cancelDownload = () => {
        cancelActiveDownload()
    }

    return {
        resetDownloadProgress,
        applyDownloadProgressDisplay,
        stopDownloadProgressTicker,
        startDownloadProgressTicker,
        updateDownloadProgress,
        issueDownloadTicketUrl,
        requestDownloadSaveHandle,
        startNextDownloadFromQueue,
        enqueueDownloadTask,
        removeDownloadQueueItem,
        downloadFile,
        cancelDownload,
        cancelActiveDownload,
    }
}
