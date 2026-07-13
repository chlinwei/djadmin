export function createWebsshFilePanelController(options) {
    const {
        ensureFileOperationsEnabled,
        loadFiles,
        fileCurrentPath,
        filePathInput,
        previousDirectoryPath,
        fileOperationsEnabled,
        fileContextMenuVisible,
        fileContextMenuRecord,
        fileContextMenuRowPath,
        transferContextMenuVisible,
        transferContextMenuTarget,
        transferContextMenuRowId,
        fileContextMenuStyle,
        transferContextMenuStyle,
        dragUploadDirPath,
        resolveFileParentDirectory,
        handleDirectoryDragOver,
        handleDirectoryDragLeave,
        handleDirectoryDrop,
    } = options

    const refreshFiles = () => {
        if (!ensureFileOperationsEnabled()) return
        loadFiles(fileCurrentPath.value)
    }

    const closeFileContextMenu = () => {
        fileContextMenuVisible.value = false
        fileContextMenuRecord.value = null
        fileContextMenuRowPath.value = ''
    }

    const closeTransferContextMenu = () => {
        transferContextMenuVisible.value = false
        transferContextMenuTarget.value = null
        transferContextMenuRowId.value = ''
    }

    const openDirectory = (path) => {
        if (!ensureFileOperationsEnabled()) return
        closeFileContextMenu()
        loadFiles(path, { historyFromPath: fileCurrentPath.value })
    }

    const handlePathEnter = () => {
        if (!ensureFileOperationsEnabled()) return
        const targetPath = String(filePathInput.value || '').trim()
        if (!targetPath) {
            filePathInput.value = fileCurrentPath.value
            return
        }
        loadFiles(targetPath, { historyFromPath: fileCurrentPath.value })
    }

    const goParentDir = () => {
        if (!ensureFileOperationsEnabled()) return
        closeFileContextMenu()
        if (!previousDirectoryPath.value) return
        const targetPath = previousDirectoryPath.value
        loadFiles(targetPath, { historyFromPath: fileCurrentPath.value })
    }

    const openFileContextMenu = (event, record) => {
        if (!fileOperationsEnabled.value) {
            closeFileContextMenu()
            return
        }
        event.preventDefault()
        closeTransferContextMenu()
        fileContextMenuRecord.value = record
        fileContextMenuRowPath.value = String(record?.path || '')
        fileContextMenuStyle.value = {
            left: `${event.clientX}px`,
            top: `${event.clientY}px`,
        }
        fileContextMenuVisible.value = true
    }

    const openTransferContextMenu = (event, type, item, rowId = '') => {
        if (!item) return
        event.preventDefault()
        closeFileContextMenu()
        transferContextMenuTarget.value = { type, item }
        transferContextMenuRowId.value = rowId
        transferContextMenuStyle.value = {
            left: `${event.clientX}px`,
            top: `${event.clientY}px`,
        }
        transferContextMenuVisible.value = true
    }

    const resolveDropTargetPath = (record) => {
        if (!record?.path) return ''
        return record.is_dir ? String(record.path) : resolveFileParentDirectory(record.path)
    }

    const bindFileRowEvents = (record) => ({
        class: [
            dragUploadDirPath.value === resolveDropTargetPath(record) ? 'dir-drop-row' : '',
            fileContextMenuRowPath.value && fileContextMenuRowPath.value === String(record?.path || '') ? 'context-selected-row' : '',
        ].filter(Boolean).join(' '),
        onContextmenu: (event) => {
            if (!fileOperationsEnabled.value) {
                return
            }
            openFileContextMenu(event, record)
        },
        onClick: () => closeFileContextMenu(),
        onDragover: (event) => {
            if (!fileOperationsEnabled.value) return
            const targetPath = resolveDropTargetPath(record)
            if (!targetPath) return
            event.preventDefault()
            handleDirectoryDragOver(targetPath)
        },
        onDragleave: () => {
            if (!fileOperationsEnabled.value) return
            const targetPath = resolveDropTargetPath(record)
            if (!targetPath) return
            handleDirectoryDragLeave(targetPath)
        },
        onDrop: (event) => {
            if (!fileOperationsEnabled.value) return
            const targetPath = resolveDropTargetPath(record)
            if (!targetPath) return
            event.preventDefault()
            void handleDirectoryDrop(event, targetPath)
        },
    })

    return {
        refreshFiles,
        openDirectory,
        handlePathEnter,
        goParentDir,
        closeFileContextMenu,
        closeTransferContextMenu,
        openFileContextMenu,
        openTransferContextMenu,
        resolveDropTargetPath,
        bindFileRowEvents,
    }
}
