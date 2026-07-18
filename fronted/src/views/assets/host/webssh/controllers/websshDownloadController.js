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
        const elapsedText = helpers.formatDuration(Math.max(0, Number(elapsedMs || 0)))
        
        // 空文件的特殊处理：总大小为0
        if (totalSize === 0) {
            refs.downloadProgressPercent.value = 100
            refs.downloadProgressText.value = `文件为空 | 已用时 ${elapsedText}`
            return
        }
        
        if (!Number.isFinite(totalSize) || totalSize < 0) {
            // 总大小未知的情况
            refs.downloadProgressPercent.value = refs.downloadRunning.value ? 0 : 100
            if (doneSize > 0) {
                refs.downloadProgressText.value = `已下载 ${helpers.formatBytes(doneSize)} | 总大小未知 | 已用时 ${elapsedText}`
            } else {
                refs.downloadProgressText.value = `正在下载（总大小未知）... | 已用时 ${elapsedText}`
            }
            return
        }
        const rawPercent = (doneSize / totalSize) * 100
        if (doneSize > 0 && rawPercent > 0 && rawPercent < 0.1) {
            refs.downloadProgressPercent.value = 0.1
        } else {
            refs.downloadProgressPercent.value = Number(Math.min(100, rawPercent).toFixed(1))
        }
        refs.downloadProgressText.value = `${helpers.formatBytes(doneSize)} / ${helpers.formatBytes(totalSize)} | 已用时 ${elapsedText}`
    }

    const updateDownloadProgress = (downloaded, total, elapsedMs = 0) => {
        state.downloadProgressActualBytes = Math.max(0, Number(downloaded || 0))
        state.downloadProgressTotalBytes = Math.max(0, Number(total || 0))
        state.downloadProgressElapsedMs = Math.max(0, Number(elapsedMs || 0))
        state.downloadProgressDisplayBytes = state.downloadProgressActualBytes
        applyDownloadProgressDisplay(
            state.downloadProgressDisplayBytes,
            state.downloadProgressTotalBytes,
            state.downloadProgressElapsedMs,
        )
    }

    const issueDirectDownloadUrls = (path) => {
        const baseCandidates = [
            String(deps.getServerUrl?.() || ''),
            String(deps.getTransferServerUrl?.() || ''),
        ]
            .map((item) => item.trim().replace(/\/$/, ''))
            .filter(Boolean)

        const uniqueBases = [...new Set(baseCandidates)]
        const encodedPath = encodeURIComponent(String(path || '').trim())
        if (!encodedPath) {
            throw new Error('下载路径不能为空')
        }
        return uniqueBases.map((baseUrl) => `${baseUrl}/assets/hosts/${getHostId()}/files/download/?path=${encodedPath}`)
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

    const enqueueDownloadTask = async (record, downloadMode = constants.DOWNLOAD_MODE_DIRECT) => {
        if (record?.is_dir) {
            message.error('目录下载功能已关闭，请改为逐个下载文件')
            return
        }
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
        const downloadMode = String(resumeState?.downloadMode || constants.DOWNLOAD_MODE_DIRECT)
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

            const downloadUrlCandidates = downloadUrl
                ? [downloadUrl]
                : issueDirectDownloadUrls(record.path)

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
            refs.downloadProgressText.value = `正在连接目标主机（${downloadModeLabel}）...`
            const requestHeaders = {}
            const token = String(deps.getToken?.() || '').trim()
            if (token) {
                requestHeaders.AUTHORIZATION = token
            }

            let response = null
            let lastFetchError = null
            for (const candidateUrl of downloadUrlCandidates) {
                try {
                    response = await fetch(candidateUrl, {
                        method: 'GET',
                        headers: requestHeaders,
                        signal: state.downloadAbortController.signal,
                    })
                    downloadUrl = candidateUrl
                    if (response.status === 200 || response.status === 206) {
                        break
                    }

                    const messageText = await helpers.parseResponseError(response)
                    lastFetchError = new Error(messageText || `下载失败（HTTP ${response.status}）`)

                    // 除 404/5xx 之外的错误通常是业务错误，不再切换地址重试。
                    if (response.status < 500 && response.status !== 404) {
                        throw lastFetchError
                    }
                } catch (fetchError) {
                    lastFetchError = fetchError
                    // 用户取消下载时，不再尝试下一个地址。
                    if (fetchError?.name === 'AbortError') {
                        throw fetchError
                    }
                }
            }

            if (!response || (response.status !== 200 && response.status !== 206)) {
                throw lastFetchError || new Error('下载失败')
            }
            refs.downloadProgressText.value = `目标主机已连接，正在读取远端文件（${downloadModeLabel}）...`
            const contentRange = response.headers.get('content-range') || ''
            const contentLength = Number(response.headers.get('content-length') || '0')
            const totalMatch = contentRange.match(/\/(\d+)$/)
            const streamTotalSize = totalMatch?.[1] ? Number(totalMatch[1]) : (Number.isFinite(contentLength) ? contentLength : fileSize)
            // 后端响应头中获取的真实文件大小应该覆盖列表中的值（包括空文件的 0 字节）
            if (Number.isFinite(streamTotalSize)) {
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
            refs.downloadProgressPercent.value = 100
            refs.downloadProgressText.value = `下载完成 | 总耗时 ${helpers.formatDuration(elapsedMs)}`
            message.success('文件下载成功')
        } catch (error) {
            const cancelErrorText = String(error?.message || '')
            const isCanceled = error?.name === 'AbortError'
                || state.downloadStopAction === 'cancel'
                || /aborted|canceled|cancelled|取消/i.test(cancelErrorText)

            if (isCanceled) {
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
        refs.currentDownloadRecord.value = null
    }

    const cancelDownload = () => {
        cancelActiveDownload()
    }

    return {
        resetDownloadProgress,
        applyDownloadProgressDisplay,
        updateDownloadProgress,
        requestDownloadSaveHandle,
        enqueueDownloadTask,
        downloadFile,
        cancelDownload,
        cancelActiveDownload,
    }
}
