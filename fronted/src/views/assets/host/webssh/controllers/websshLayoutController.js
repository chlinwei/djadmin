export function createWebsshLayoutController(options) {
    const {
        nextTick,
        getFitAddon,
        websshMainRef,
        websshPageRef,
        filePanelRef,
        fileTableWrapRef,
        showFilePanel,
        fileErrorText,
        fileCurrentPath,
        filteredFileEntries,
        fileLoading,
        fileTableScrollY,
        filePanelWidth,
        isFullscreen,
    } = options

    let filePanelResizeObserver = null
    let resizing = false

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

    const toggleFilePanel = () => {
        showFilePanel.value = !showFilePanel.value
        stopResize()
        syncTerminalFit()
        nextTick(() => {
            setupFilePanelResizeObserver()
            scheduleFileTableScrollYSync()
        })
    }

    const watchFilePanelErrorDeps = () => [showFilePanel.value, fileErrorText.value]
    const watchFileListDeps = () => [fileCurrentPath.value, filteredFileEntries.value.length, fileLoading.value]

    return {
        syncTerminalFit,
        updateFileTableScrollY,
        scheduleFileTableScrollYSync,
        setupFilePanelResizeObserver,
        disconnectFilePanelResizeObserver,
        updateFullscreenState,
        toggleFilePanel,
        startResize,
        stopResize,
        watchFilePanelErrorDeps,
        watchFileListDeps,
    }
}
