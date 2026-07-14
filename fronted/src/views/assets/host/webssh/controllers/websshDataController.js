export function createWebsshDataController(options) {
    const {
        computed,
        refs,
        state,
        helpers,
        TRANSFER_LIST_LIMIT,
    } = options

    const activeSessionColumns = [
        { title: '会话ID', dataIndex: 'id', key: 'id', width: 120 },
        { title: '用户名', dataIndex: 'username', key: 'username', width: 180 },
        { title: '会话开始时间', dataIndex: 'start_time', key: 'start_time', width: 280 },
    ]

    const fileColumns = [
        { title: '名称', dataIndex: 'name', key: 'name', width: 220 },
        { title: '大小', dataIndex: 'size', key: 'size', width: 90 },
        { title: '修改时间', dataIndex: 'mtime', key: 'mtime', width: 160 },
    ]

    const filteredFileEntries = computed(() => {
        const keyword = String(refs.fileFilterKeyword.value || '').trim().toLowerCase()
        if (!keyword) return refs.fileEntries.value
        return refs.fileEntries.value.filter((entry) => String(entry?.name || '').toLowerCase().includes(keyword))
    })

    const fileContextMenuActions = computed(() => {
        const record = refs.fileContextMenuRecord.value
        if (!record) return []
        const actions = []
        if (record.is_dir) {
            actions.push({ key: 'open', label: '打开目录' })
            actions.push({ key: 'download', label: '下载' })
        } else {
            actions.push({ key: 'download', label: '下载' })
        }
        actions.push({ key: 'copy-dir-path', label: '复制目录路径' })
        actions.push({ key: 'rename', label: '重命名' })
        actions.push({ key: 'delete', label: '删除', danger: true })
        return actions
    })

    const transferContextMenuActions = computed(() => {
        const target = refs.transferContextMenuTarget.value
        if (!target) return []
        const itemType = target.type
        const item = target.item
        if (!item) return []
        if (itemType === 'upload-queue') {
            return [
                { key: 'cancel', label: '取消', danger: true },
                { key: 'open-dir', label: '打开文件所在目录' },
            ]
        }
        if (itemType === 'download-active') {
            const activePath = String(item?.record?.path || '')
            return [
                { key: 'cancel', label: '取消', danger: true, disabled: !refs.downloadRunning.value },
                { key: 'open-dir', label: '打开文件所在目录', disabled: !activePath },
            ]
        }
        if (itemType === 'upload-active') {
            return [
                { key: 'cancel', label: '取消', danger: true, disabled: !refs.uploadRunning.value },
                { key: 'open-dir', label: '打开文件所在目录' },
            ]
        }
        return []
    })

    const downloadRows = computed(() => {
        return helpers.buildDownloadRows({
            downloadRunning: refs.downloadRunning,
            downloadProgressStatus: refs.downloadProgressStatus,
            downloadFileName: refs.downloadFileName,
            currentDownloadRecord: refs.currentDownloadRecord,
            downloadProgressText: refs.downloadProgressText,
            getStatusMeta: helpers.getTransferStatusMeta,
        })
    })

    const uploadRows = computed(() => {
        return helpers.buildUploadRows({
            uploadRunning: refs.uploadRunning,
            uploadProgressStatus: refs.uploadProgressStatus,
            uploadFileName: refs.uploadFileName,
            currentUploadContext: refs.currentUploadContext,
            uploadProgressText: refs.uploadProgressText,
            fileCurrentPath: refs.fileCurrentPath,
            uploadQueue: refs.uploadQueue,
            getStatusMeta: helpers.getTransferStatusMeta,
        })
    })

    // Auto-show transfer panel only for active/queued tasks; history alone should not occupy layout space.
    const hasDownloadTask = computed(() => refs.downloadRunning.value)
    const hasUploadTask = computed(() => refs.uploadRunning.value || refs.uploadQueue.value.length > 0)
    const displayedDownloadRows = computed(() => downloadRows.value.slice(0, TRANSFER_LIST_LIMIT))
    const displayedUploadRows = computed(() => uploadRows.value.slice(0, TRANSFER_LIST_LIMIT))

    return {
        activeSessionColumns,
        fileColumns,
        filteredFileEntries,
        fileContextMenuActions,
        transferContextMenuActions,
        downloadRows,
        uploadRows,
        hasDownloadTask,
        hasUploadTask,
        displayedDownloadRows,
        displayedUploadRows,
    }
}
