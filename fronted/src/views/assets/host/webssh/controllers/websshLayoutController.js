export function createWebsshLayoutController(options) {
    const {
        nextTick,
        getFitAddon,
        websshMainRef,
        websshPageRef,
        filePanelRef,
        fileTableWrapRef,
        showFilePanel,
        transferPanelVisible,
        transferPanelDismissed,
        transferPanelPinned,
        transferPanelCollapsed,
        fileErrorText,
        fileCurrentPath,
        filteredFileEntries,
        fileLoading,
        fileTableScrollY,
        filePanelWidth,
        transferPanelHeight,
        isFullscreen,
    } = options

    let filePanelResizeObserver = null
    let resizing = false
    let transferResizing = false

    const updateFileTableScrollY = () => {
        const tableWrapEl = fileTableWrapRef.value
        if (!tableWrapEl) return
        const tableHeaderEl = tableWrapEl.querySelector('.ant-table-header')
            || tableWrapEl.querySelector('.ant-table-thead')
        const headerHeight = Number(tableHeaderEl?.getBoundingClientRect?.().height || 40)
        const available = tableWrapEl.clientHeight - headerHeight - 2
        fileTableScrollY.value = Math.max(120, Math.floor(available))
    }

    const scheduleFileTableScrollYSync = () => {
        nextTick(() => {
            if (typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function') {
                window.requestAnimationFrame(() => {
                    updateFileTableScrollY()
                })
                return
            }
            updateFileTableScrollY()
        })
    }

    const syncTerminalFit = () => {
        nextTick(() => {
            getFitAddon()?.fit()
            scheduleFileTableScrollYSync()
        })
    }

    const setupFilePanelResizeObserver = () => {
        if (!window.ResizeObserver) return
        if (filePanelResizeObserver) {
            filePanelResizeObserver.disconnect()
            filePanelResizeObserver = null
        }
        if (!filePanelRef.value) return
        filePanelResizeObserver = new ResizeObserver(() => {
            scheduleFileTableScrollYSync()
        })
        filePanelResizeObserver.observe(filePanelRef.value)
        if (fileTableWrapRef.value) {
            filePanelResizeObserver.observe(fileTableWrapRef.value)
        }
    }

    const disconnectFilePanelResizeObserver = () => {
        if (filePanelResizeObserver) {
            filePanelResizeObserver.disconnect()
            filePanelResizeObserver = null
        }
    }

    const ensureTransferPanelVisible = () => {
        transferPanelDismissed.value = false
        transferPanelPinned.value = true
    }

    const closeTransferPanel = () => {
        transferPanelDismissed.value = true
        transferPanelPinned.value = false
        syncTerminalFit()
    }

    const toggleTransferPanelCollapsed = () => {
        transferPanelCollapsed.value = !transferPanelCollapsed.value
        syncTerminalFit()
    }

    const toggleTransferPanelVisibility = () => {
        if (transferPanelVisible.value) {
            closeTransferPanel()
            return
        }
        transferPanelDismissed.value = false
        transferPanelPinned.value = true
        transferPanelCollapsed.value = false
        syncTerminalFit()
    }

    const updateFullscreenState = () => {
        isFullscreen.value = Boolean(document.fullscreenElement)
        syncTerminalFit()
    }

    const clampFilePanelWidth = (nextWidth) => {
        const containerWidth = websshMainRef.value?.clientWidth || 0
        const minFileWidth = 260
        const minTerminalWidth = 420
        if (!containerWidth) {
            filePanelWidth.value = Math.max(minFileWidth, Number(nextWidth) || filePanelWidth.value)
            return
        }
        const maxFileWidth = Math.max(minFileWidth, containerWidth - minTerminalWidth)
        filePanelWidth.value = Math.min(maxFileWidth, Math.max(minFileWidth, Number(nextWidth) || filePanelWidth.value))
    }

    const handleResizeMove = (event) => {
        if (!resizing || !websshMainRef.value) return
        const rect = websshMainRef.value.getBoundingClientRect()
        const nextWidth = event.clientX - rect.left
        clampFilePanelWidth(nextWidth)
        syncTerminalFit()
    }

    const stopResize = () => {
        if (!resizing) return
        resizing = false
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
        window.removeEventListener('mousemove', handleResizeMove)
        window.removeEventListener('mouseup', stopResize)
    }

    const startResize = (event) => {
        if (!websshMainRef.value) return
        resizing = true
        document.body.style.cursor = 'col-resize'
        document.body.style.userSelect = 'none'
        window.addEventListener('mousemove', handleResizeMove)
        window.addEventListener('mouseup', stopResize)
        handleResizeMove(event)
    }

    const clampTransferPanelHeight = (nextHeight) => {
        const minHeight = 150
        const pageHeight = websshPageRef.value?.clientHeight || window.innerHeight
        const maxHeight = Math.max(minHeight, pageHeight - 280)
        transferPanelHeight.value = Math.min(maxHeight, Math.max(minHeight, Number(nextHeight) || minHeight))
        syncTerminalFit()
    }

    const handleTransferResizeMove = (event) => {
        if (!transferResizing || !websshPageRef.value) return
        const rect = websshPageRef.value.getBoundingClientRect()
        const nextHeight = rect.bottom - event.clientY
        clampTransferPanelHeight(nextHeight)
    }

    const stopTransferResize = () => {
        if (!transferResizing) return
        transferResizing = false
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
        window.removeEventListener('mousemove', handleTransferResizeMove)
        window.removeEventListener('mouseup', stopTransferResize)
    }

    const startTransferResize = (event) => {
        if (!websshPageRef.value) return
        transferResizing = true
        document.body.style.cursor = 'row-resize'
        document.body.style.userSelect = 'none'
        window.addEventListener('mousemove', handleTransferResizeMove)
        window.addEventListener('mouseup', stopTransferResize)
        handleTransferResizeMove(event)
    }

    const toggleFilePanel = () => {
        showFilePanel.value = !showFilePanel.value
        stopResize()
        syncTerminalFit()
        nextTick(() => {
            setupFilePanelResizeObserver()
            scheduleFileTableScrollYSync()
        })
    }

    const watchTransferPanelDeps = () => [transferPanelVisible.value, transferPanelCollapsed.value]
    const watchFilePanelErrorDeps = () => [showFilePanel.value, fileErrorText.value]
    const watchFileListDeps = () => [fileCurrentPath.value, filteredFileEntries.value.length, fileLoading.value]

    return {
        ensureTransferPanelVisible,
        closeTransferPanel,
        toggleTransferPanelCollapsed,
        toggleTransferPanelVisibility,
        syncTerminalFit,
        updateFileTableScrollY,
        scheduleFileTableScrollYSync,
        setupFilePanelResizeObserver,
        disconnectFilePanelResizeObserver,
        updateFullscreenState,
        toggleFilePanel,
        startResize,
        stopResize,
        startTransferResize,
        stopTransferResize,
        watchTransferPanelDeps,
        watchFilePanelErrorDeps,
        watchFileListDeps,
    }
}
