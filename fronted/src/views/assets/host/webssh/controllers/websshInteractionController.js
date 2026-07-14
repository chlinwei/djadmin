export function createWebsshInteractionController(options) {
    const {
        getHostId,
        message,
        refs,
        state,
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
    } = options

    const createDirectory = async () => {
        if (!ensureFileOperationsEnabled()) return
        const name = window.prompt('请输入目录名', '')
        if (!name || !name.trim()) return
        try {
            await createHostWebSshDir(getHostId(), { path: refs.fileCurrentPath.value || '.', name: name.trim() })
            message.success('目录创建成功')
            await loadFiles(refs.fileCurrentPath.value)
        } catch (error) {
            message.error(error?.message || '目录创建失败')
        }
    }

    const handleFileContextAction = async (actionKey) => {
        if (!ensureFileOperationsEnabled()) return
        const record = refs.fileContextMenuRecord.value
        if (!record) return
        closeFileContextMenu()
        if (actionKey === 'open') {
            openDirectory(record.path)
            return
        }
        if (actionKey === 'download') {
            await enqueueDownloadTask(record, options.DOWNLOAD_MODE_DIRECT)
            return
        }
        if (actionKey === 'copy-dir-path') {
            const targetDir = helpers.resolveFileParentDirectory(record.path)
            if (!targetDir) {
                message.error('目录路径无效，复制失败')
                return
            }
            try {
                if (navigator?.clipboard?.writeText) {
                    await navigator.clipboard.writeText(targetDir)
                    message.success(`已复制目录路径：${targetDir}`)
                    return
                }
                const textarea = document.createElement('textarea')
                textarea.value = targetDir
                textarea.setAttribute('readonly', 'readonly')
                textarea.style.position = 'fixed'
                textarea.style.left = '-9999px'
                document.body.appendChild(textarea)
                textarea.select()
                const copied = document.execCommand('copy')
                document.body.removeChild(textarea)
                if (!copied) {
                    throw new Error('浏览器复制命令失败')
                }
                message.success(`已复制目录路径：${targetDir}`)
            } catch (error) {
                message.error(error?.message || '复制目录路径失败')
            }
            return
        }
        if (actionKey === 'rename') {
            await renameFile(record)
            return
        }
        if (actionKey === 'delete') {
            await deleteFile(record)
        }
    }

    const hideContextMenuByGlobalClick = (event) => {
        const target = event?.target
        if (target?.closest && target.closest('.file-context-menu')) {
            return
        }
        closeFileContextMenu()
        closeTransferContextMenu()
    }

    const hideContextMenuByEscape = (event) => {
        if (event.key === 'Escape') {
            closeFileContextMenu()
            closeTransferContextMenu()
        }
    }

    const handleTransferContextAction = (action) => {
        if (!action || action.disabled) return
        const target = refs.transferContextMenuTarget.value
        closeTransferContextMenu()
        if (!target?.item) return
        if (target.type === 'upload-queue') {
            if (action.key === 'cancel') {
                removeUploadQueueItem(target.item.id)
                return
            }
            if (action.key === 'open-dir') {
                const targetPath = String(target.item?.task?.targetPath || refs.fileCurrentPath.value || '.')
                openDirectory(targetPath)
            }
            return
        }
        if (target.type === 'download-active') {
            if (action.key === 'cancel') {
                cancelDownload()
                return
            }
            if (action.key === 'open-dir') {
                const filePath = String(target.item?.record?.path || '')
                openDirectory(helpers.resolveFileParentDirectory(filePath))
            }
            return
        }
        if (target.type === 'upload-active') {
            if (action.key === 'cancel') {
                void cancelUpload()
                return
            }
            if (action.key === 'open-dir') {
                const targetPath = String(target.item?.targetPath || refs.fileCurrentPath.value || '.')
                openDirectory(targetPath)
            }
            return
        }
    }

    const handleDirectoryDragOver = (path) => {
        refs.dragUploadDirPath.value = path
    }

    const handleDirectoryDragLeave = (path) => {
        if (refs.dragUploadDirPath.value === path) {
            refs.dragUploadDirPath.value = ''
        }
    }

    const handleDirectoryDrop = async (event, targetPath) => {
        const normalizedPath = String(targetPath || '.')
        const droppedFiles = Array.from(event?.dataTransfer?.files || [])
        refs.dragUploadDirPath.value = ''
        if (!droppedFiles.length) {
            return
        }
        for (const droppedFile of droppedFiles) {
            await uploadRawFileToPath(droppedFile, normalizedPath)
        }
        message.success(`已将 ${droppedFiles.length} 个文件加入上传任务（目标目录：${normalizedPath}）`)
    }

    const preventGlobalFileDrop = (event) => {
        const hasFiles = Array.from(event?.dataTransfer?.types || []).includes('Files')
        if (!hasFiles) return
        event.preventDefault()
    }

    const renameFile = async (record) => {
        const nextName = window.prompt('请输入新文件名', record.name || '')
        if (!nextName || !nextName.trim() || nextName.trim() === record.name) return
        try {
            await renameHostWebSshFile(getHostId(), { path: record.path, new_name: nextName.trim() })
            message.success('重命名成功')
            await loadFiles(refs.fileCurrentPath.value)
        } catch (error) {
            message.error(error?.message || '重命名失败')
        }
    }

    const deleteFile = async (record) => {
        const confirmText = record.is_dir ? `确定删除目录 ${record.name}（含子文件）吗？` : `确定删除文件 ${record.name} 吗？`
        if (!window.confirm(confirmText)) return
        try {
            await deleteHostWebSshFile(getHostId(), { path: record.path, recursive: Boolean(record.is_dir) })
            message.success('删除成功')
            await loadFiles(refs.fileCurrentPath.value)
        } catch (error) {
            message.error(error?.message || '删除失败')
        }
    }

    return {
        createDirectory,
        handleFileContextAction,
        hideContextMenuByGlobalClick,
        hideContextMenuByEscape,
        handleTransferContextAction,
        handleDirectoryDragOver,
        handleDirectoryDragLeave,
        handleDirectoryDrop,
        preventGlobalFileDrop,
    }
}
