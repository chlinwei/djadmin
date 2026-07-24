export function createWebsshDataController(options) {
    const {
        computed,
        refs,
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
        } else {
            actions.push({ key: 'download', label: '下载' })
        }
        // 右键上传：无论文件或文件夹，都上传到该项所在的目录（父目录）
        actions.push({ key: 'upload', label: '上传到此目录' })
        actions.push({ key: 'copy-dir-path', label: '复制目录路径' })
        actions.push({ key: 'rename', label: '重命名' })
        actions.push({ key: 'delete', label: '删除', danger: true })
        return actions
    })

    return {
        activeSessionColumns,
        fileColumns,
        filteredFileEntries,
        fileContextMenuActions,
    }
}
