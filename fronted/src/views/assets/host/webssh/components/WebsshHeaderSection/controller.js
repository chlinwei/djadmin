const WEBSSH_HEADER_EVENTS = [
    'open-active-sessions-modal',
    'download-current-log',
    'toggle-fullscreen',
    'reconnect',
    'close-tab',
    'cancel-download',
    'cancel-upload',
    'dismiss-download-progress',
    'dismiss-upload-progress',
    'increase-font-size',
    'decrease-font-size',
]

export const websshHeaderSectionProps = {
    hostTitle: { type: String, default: '' },
    currentLogId: { type: [Number, String], default: null },
    activeUserCount: { type: Number, default: 0 },
    statusColor: { type: String, default: 'default' },
    statusText: { type: String, default: '' },
    downloadingLog: { type: Boolean, default: false },
    isFullscreen: { type: Boolean, default: false },
    downloadProgressVisible: { type: Boolean, default: false },
    downloadProgressPercent: { type: Number, default: 0 },
    downloadProgressStatus: { type: String, default: 'active' },
    downloadProgressText: { type: String, default: '' },
    downloadFileName: { type: String, default: '' },
    downloadRunning: { type: Boolean, default: false },
    uploadProgressVisible: { type: Boolean, default: false },
    uploadProgressPercent: { type: Number, default: 0 },
    uploadProgressStatus: { type: String, default: 'active' },
    uploadProgressText: { type: String, default: '' },
    uploadFileName: { type: String, default: '' },
    uploadRunning: { type: Boolean, default: false },
}

export const createWebsshHeaderSectionController = (emit) => {
    const openActiveSessionsModal = () => emit('open-active-sessions-modal')
    const downloadCurrentLog = () => emit('download-current-log')
    const toggleFullscreen = () => emit('toggle-fullscreen')
    const reconnect = () => emit('reconnect')
    const closeTab = () => emit('close-tab')
    const cancelDownload = () => emit('cancel-download')
    const cancelUpload = () => emit('cancel-upload')
    const dismissDownloadProgress = () => emit('dismiss-download-progress')
    const dismissUploadProgress = () => emit('dismiss-upload-progress')
    const increaseFontSize = () => emit('increase-font-size')
    const decreaseFontSize = () => emit('decrease-font-size')

    const getFullscreenButtonText = (isFullscreen) => (isFullscreen ? '退出全屏' : '进入全屏')

    return {
        openActiveSessionsModal,
        downloadCurrentLog,
        toggleFullscreen,
        reconnect,
        closeTab,
        cancelDownload,
        cancelUpload,
        dismissDownloadProgress,
        dismissUploadProgress,
        increaseFontSize,
        decreaseFontSize,
        getFullscreenButtonText,
    }
}

export const websshHeaderSectionEvents = WEBSSH_HEADER_EVENTS
