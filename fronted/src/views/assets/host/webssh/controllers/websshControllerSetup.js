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
        refs,
        message,
        ensureFileOperationsEnabled,
        ensureTransferPanelVisible,
        trimUploadQueueToLimit,
        formatBytes,
        deps,
    } = ctx

    return {
        getHostId: () => hostState.hostId,
        state: {
            get uploadAbortController() { return hostState.uploadAbortController },
            set uploadAbortController(value) { hostState.uploadAbortController = value },
            nextUploadSeq: hostState.nextUploadSeq,
        },
        refs,
        message,
        ensureFileOperationsEnabled,
        ensureTransferPanelVisible,
        trimUploadQueueToLimit,
        formatBytes,
        ...deps,
    }
}

export function createWebsshInteractionSetup(ctx) {
    const {
        hostState,
        DOWNLOAD_MODE_DIRECT,
        message,
        refs,
        ensureFileOperationsEnabled,
        closeFileContextMenu,
        closeTransferContextMenu,
        openDirectory,
        enqueueDownloadTask,
        cancelDownload,
        removeUploadQueueItem,
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
        DOWNLOAD_MODE_DIRECT,
        message,
        refs,
        ensureFileOperationsEnabled,
        closeFileContextMenu,
        closeTransferContextMenu,
        openDirectory,
        enqueueDownloadTask,
        cancelDownload,
        removeUploadQueueItem,
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
