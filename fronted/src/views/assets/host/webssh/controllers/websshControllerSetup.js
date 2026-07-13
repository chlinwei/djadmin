export function createWebsshDownloadSetup(ctx) {
    const {
        nextTick,
        hostState,
        constants,
        refs,
        message,
        helpers,
        deps,
    } = ctx

    return {
        nextTick,
        getHostId: () => hostState.hostId,
        constants,
        state: {
            get downloadAbortController() { return hostState.downloadAbortController },
            set downloadAbortController(value) { hostState.downloadAbortController = value },
            get downloadStopAction() { return hostState.downloadStopAction },
            set downloadStopAction(value) { hostState.downloadStopAction = value },
            get downloadProgressTicker() { return hostState.downloadProgressTicker },
            set downloadProgressTicker(value) { hostState.downloadProgressTicker = value },
            get downloadProgressActualBytes() { return hostState.downloadProgressActualBytes },
            set downloadProgressActualBytes(value) { hostState.downloadProgressActualBytes = value },
            get downloadProgressDisplayBytes() { return hostState.downloadProgressDisplayBytes },
            set downloadProgressDisplayBytes(value) { hostState.downloadProgressDisplayBytes = value },
            get downloadProgressTotalBytes() { return hostState.downloadProgressTotalBytes },
            set downloadProgressTotalBytes(value) { hostState.downloadProgressTotalBytes = value },
            get downloadProgressElapsedMs() { return hostState.downloadProgressElapsedMs },
            set downloadProgressElapsedMs(value) { hostState.downloadProgressElapsedMs = value },
            get downloadProgressTickAt() { return hostState.downloadProgressTickAt },
            set downloadProgressTickAt(value) { hostState.downloadProgressTickAt = value },
            get lastDownloadActionKey() { return hostState.lastDownloadActionKey },
            set lastDownloadActionKey(value) { hostState.lastDownloadActionKey = value },
            get lastDownloadActionAt() { return hostState.lastDownloadActionAt },
            set lastDownloadActionAt(value) { hostState.lastDownloadActionAt = value },
        },
        refs,
        message,
        helpers,
        deps,
    }
}

export function createWebsshUploadSetup(ctx) {
    const {
        hostState,
        UPLOAD_CHUNK_SIZE,
        refs,
        message,
        ensureFileOperationsEnabled,
        ensureTransferPanelVisible,
        trimUploadQueueToLimit,
        formatBytes,
        saveUploadResumeTask,
        clearUploadResumeTask,
        deps,
    } = ctx

    return {
        getHostId: () => hostState.hostId,
        UPLOAD_CHUNK_SIZE,
        state: {
            get uploadAbortController() { return hostState.uploadAbortController },
            set uploadAbortController(value) { hostState.uploadAbortController = value },
            get pendingUploadTask() { return hostState.pendingUploadTask },
            set pendingUploadTask(value) { hostState.pendingUploadTask = value },
            get uploadStopAction() { return hostState.uploadStopAction },
            set uploadStopAction(value) { hostState.uploadStopAction = value },
            nextUploadSeq: hostState.nextUploadSeq,
        },
        refs,
        message,
        ensureFileOperationsEnabled,
        ensureTransferPanelVisible,
        trimUploadQueueToLimit,
        formatBytes,
        saveUploadResumeTask,
        clearUploadResumeTask,
        ...deps,
    }
}

export function createWebsshInteractionSetup(ctx) {
    const {
        hostState,
        DOWNLOAD_MODE_TICKET_STREAM,
        message,
        refs,
        ensureFileOperationsEnabled,
        closeFileContextMenu,
        closeTransferContextMenu,
        openDirectory,
        enqueueDownloadTask,
        cancelDownload,
        removeDownloadQueueItem,
        toggleUploadQueueItem,
        removeUploadQueueItem,
        pauseUpload,
        resumeUpload,
        cancelUpload,
        uploadRawFileToPath,
        renameHostWebSshFile,
        deleteHostWebSshFile,
        createHostWebSshDir,
        loadFiles,
        helpers,
    } = ctx

    return {
        getHostId: () => hostState.hostId,
        DOWNLOAD_MODE_TICKET_STREAM,
        message,
        refs,
        state: {
            get pendingUploadTask() { return hostState.pendingUploadTask },
        },
        ensureFileOperationsEnabled,
        closeFileContextMenu,
        closeTransferContextMenu,
        openDirectory,
        enqueueDownloadTask,
        cancelDownload,
        removeDownloadQueueItem,
        toggleUploadQueueItem,
        removeUploadQueueItem,
        pauseUpload,
        resumeUpload,
        cancelUpload,
        uploadRawFileToPath,
        renameHostWebSshFile,
        deleteHostWebSshFile,
        createHostWebSshDir,
        loadFiles,
        helpers,
    }
}

export function createWebsshLifecycleSetup(ctx) {
    const {
        nextTick,
        route,
        hostState,
        actions,
        apis,
    } = ctx

    return {
        nextTick,
        route,
        state: {
            get hostId() { return hostState.hostId },
            set hostId(value) { hostState.hostId = value },
            get instanceName() { return hostState.instanceName },
            set instanceName(value) { hostState.instanceName = value },
            get hostIp() { return hostState.hostIp },
            set hostIp(value) { hostState.hostIp = value },
        },
        actions,
        refs: {
            fileCurrentPath: ctx.fileCurrentPath,
        },
        apis,
    }
}
