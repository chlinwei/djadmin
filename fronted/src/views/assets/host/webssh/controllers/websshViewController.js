export function createWebsshViewController(options) {
    const {
        computed,
        route,
        getHostId,
        getInstanceName,
        getHostIp,
        statusText,
        connectWebSsh,
        websshPageRef,
        currentLogId,
        downloadingLog,
        downloadAuditWebSshSession,
        parseDownloadFilename,
        message,
    } = options

    const hostTitle = computed(() => {
        const name = getInstanceName() || route.query.instance_name || `Host-${getHostId() || '-'}`
        const ip = getHostIp() || route.query.ip || '-'
        return `${name} (${ip})`
    })

    const statusColor = computed(() => {
        if (statusText.value === '已连接') return 'success'
        if (statusText.value === '连接中') return 'processing'
        if (statusText.value === '连接失败') return 'error'
        return 'default'
    })

    const triggerFileDownload = (blob, filename) => {
        const url = window.URL.createObjectURL(blob)
        const anchor = document.createElement('a')
        anchor.href = url
        anchor.download = filename
        document.body.appendChild(anchor)
        anchor.click()
        document.body.removeChild(anchor)
        window.URL.revokeObjectURL(url)
    }

    const reconnect = () => {
        connectWebSsh()
    }

    const toggleFullscreen = async () => {
        try {
            if (!document.fullscreenElement) {
                await (websshPageRef.value || document.documentElement).requestFullscreen()
            } else {
                await document.exitFullscreen()
            }
        } catch (error) {
            message.error('切换全屏失败')
        }
    }

    const downloadCurrentLog = async () => {
        if (!currentLogId.value) {
            message.warning('当前会话还未生成日志')
            return
        }

        downloadingLog.value = true
        try {
            const response = await downloadAuditWebSshSession(currentLogId.value)
            const fallbackName = `webssh-${currentLogId.value}.log`
            const filename = parseDownloadFilename(response.headers?.['content-disposition'], fallbackName)
            const blob = response.data instanceof Blob
                ? response.data
                : new Blob([response.data], { type: 'text/plain;charset=utf-8' })
            triggerFileDownload(blob, filename)
            message.success('日志下载成功')
        } catch (error) {
            message.error(error?.message || '日志下载失败')
        } finally {
            downloadingLog.value = false
        }
    }

    const closeTab = () => {
        window.close()
    }

    return {
        hostTitle,
        statusColor,
        reconnect,
        toggleFullscreen,
        downloadCurrentLog,
        closeTab,
    }
}
