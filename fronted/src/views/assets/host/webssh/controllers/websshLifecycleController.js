export function createWebsshLifecycleController(options) {
    const {
        nextTick,
        route,
        state,
        actions,
        refs,
        apis,
    } = options

    const handleMounted = async () => {
        const routeHostId = route.query.host_id || route.query.id || route.params.host_id || route.params.id || 0
        state.hostId = Number(routeHostId)
        state.instanceName = String(route.query.instance_name || '')
        state.hostIp = String(route.query.ip || '')

        window.addEventListener('pagehide', actions.handlePageUnload)
        window.addEventListener('beforeunload', actions.handlePageUnload)
        window.addEventListener('click', actions.hideContextMenuByGlobalClick)
        window.addEventListener('resize', actions.hideContextMenuByGlobalClick)
        window.addEventListener('resize', actions.updateFileTableScrollY)
        window.addEventListener('resize', actions.syncTerminalFit)
        window.addEventListener('keydown', actions.hideContextMenuByEscape)
        window.addEventListener('dragover', actions.preventGlobalFileDrop)
        window.addEventListener('drop', actions.preventGlobalFileDrop)
        document.addEventListener('fullscreenchange', actions.updateFullscreenState)
        actions.startActiveUserPolling()

        if (state.hostId) {
            apis.getHostById(state.hostId)
                .then((res) => {
                    if (res.data?.code === 200 && res.data?.data) {
                        const host = res.data.data
                        state.instanceName = host.instance_name || state.instanceName
                        state.hostIp = host.ip || state.hostIp
                    }
                })
                .catch(() => {
                    // fallback to query display
                })
        }

        void actions.connectWebSsh()
        nextTick(() => {
            actions.setupFilePanelResizeObserver()
            actions.scheduleFileTableScrollYSync()
        })
    }

    const handleBeforeUnmount = () => {
        window.removeEventListener('pagehide', actions.handlePageUnload)
        window.removeEventListener('beforeunload', actions.handlePageUnload)
        window.removeEventListener('click', actions.hideContextMenuByGlobalClick)
        window.removeEventListener('resize', actions.hideContextMenuByGlobalClick)
        window.removeEventListener('resize', actions.updateFileTableScrollY)
        window.removeEventListener('resize', actions.syncTerminalFit)
        window.removeEventListener('keydown', actions.hideContextMenuByEscape)
        window.removeEventListener('dragover', actions.preventGlobalFileDrop)
        window.removeEventListener('drop', actions.preventGlobalFileDrop)
        document.removeEventListener('fullscreenchange', actions.updateFullscreenState)
        actions.stopDownloadProgressTicker()
        actions.closeFileContextMenu()
        actions.cancelActiveDownload()
        actions.cancelUpload()
        actions.stopResize()
        actions.stopTransferResize()
        actions.disconnectFilePanelResizeObserver()
        actions.closeSocket()
        actions.disposeTerminal()
        actions.stopActiveUserPolling()
        actions.stopActiveSessionsPolling()
    }

    return {
        handleMounted,
        handleBeforeUnmount,
    }
}
