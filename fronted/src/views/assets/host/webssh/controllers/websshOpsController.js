export function createWebsshOpsController(options) {
    const {
        computed,
        message,
        statusText,
        uploadQueue,
        trimUploadQueueToLimitByHelper,
        transferListLimit,
    } = options
    const fileOperationsEnabled = computed(() => statusText.value === '已连接')
    let lastFileOpsOfflineNoticeAt = 0

    const notifyFileOpsOffline = () => {
        const now = Date.now()
        if (now - lastFileOpsOfflineNoticeAt < 1500) {
            return
        }
        lastFileOpsOfflineNoticeAt = now
        message.warning('WebSSH 已离线，文件管理不可操作')
    }

    const ensureFileOperationsEnabled = (showTip = true) => {
        if (fileOperationsEnabled.value) {
            return true
        }
        if (showTip) {
            notifyFileOpsOffline()
        }
        return false
    }

    const trimUploadQueueToLimit = () => {
        trimUploadQueueToLimitByHelper({
            uploadQueue,
            transferListLimit,
        })
    }

    return {
        fileOperationsEnabled,
        ensureFileOperationsEnabled,
        trimUploadQueueToLimit,
    }
}
