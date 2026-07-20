<template>
    <div ref="websshPageRef" class="webssh-page">
        <WebsshHeaderSection
            :host-title="hostTitle"
            :current-log-id="currentLogId"
            :active-user-count="activeUserCount"
            :status-color="statusColor"
            :status-text="statusText"
            :downloading-log="downloadingLog"
            :is-fullscreen="isFullscreen"
            :is-temporary-credential="isTemporaryCredential"
            :download-progress-visible="downloadProgressVisible"
            :download-progress-percent="downloadProgressPercent"
            :download-progress-status="downloadProgressStatus"
            :download-progress-text="downloadProgressText"
            :download-file-name="downloadFileName"
            :download-running="downloadRunning"
            :upload-progress-visible="uploadProgressVisible"
            :upload-progress-percent="uploadProgressPercent"
            :upload-progress-status="uploadProgressStatus"
            :upload-progress-text="uploadProgressText"
            :upload-file-name="uploadFileName"
            :upload-running="uploadRunning"
            @open-active-sessions-modal="openActiveSessionsModal"
            @download-current-log="downloadCurrentLog"
            @toggle-fullscreen="toggleFullscreen"
            @reconnect="reconnect"
            @close-tab="closeTab"
            @cancel-download="cancelDownload"
            @cancel-upload="cancelUpload"
            @dismiss-download-progress="dismissDownloadProgress"
            @dismiss-upload-progress="dismissUploadProgress"
            @increase-font-size="increaseTerminalFontSize"
            @decrease-font-size="decreaseTerminalFontSize"
        />

        <div ref="websshMainRef" class="webssh-main">
            <WebsshFilePanel
                v-if="supportsFileOps"
                :show-file-panel="showFilePanel"
                :set-file-panel-ref="(el) => (filePanelRef = el)"
                :file-operations-enabled="fileOperationsEnabled"
                :file-panel-width="filePanelWidth"
                :file-filter-keyword="fileFilterKeyword"
                :file-loading="fileLoading"
                :previous-directory-path="previousDirectoryPath"
                :file-path-input="filePathInput"
                :file-error-text="fileErrorText"
                :set-file-table-wrap-ref="(el) => (fileTableWrapRef = el)"
                :file-columns="fileColumns"
                :filtered-file-entries="filteredFileEntries"
                :file-table-scroll-y="fileTableScrollY"
                :on-bind-file-row-events="bindFileRowEvents"
                :on-format-file-size="formatFileSize"
                :on-format-file-mtime="formatFileMtime"
                :file-context-menu-visible="fileContextMenuVisible"
                :file-context-menu-style="fileContextMenuStyle"
                :file-context-menu-actions="fileContextMenuActions"
                @update:file-filter-keyword="fileFilterKeyword = $event"
                @update:file-path-input="filePathInput = $event"
                @go-parent-dir="goParentDir"
                @refresh-files="refreshFiles"
                @create-directory="createDirectory"
                @upload-file="uploadFile"
                @handle-path-enter="handlePathEnter"
                @open-directory="openDirectory"
                @handle-file-context-action="handleFileContextAction"
            />

            <div v-if="supportsFileOps && showFilePanel" class="panel-resizer" @mousedown="startResize" />
            <div v-if="supportsFileOps" class="panel-toggle-handle" @click="toggleFilePanel">
                {{ showFilePanel ? '◀' : '▶' }}
            </div>

            <WebsshTerminalPanel
                :message-text="messageText"
                :message-type="messageType"
                :set-terminal-ref="(el) => (terminalRef = el)"
            />
        </div>

        <a-modal
            v-model:open="activeSessionsVisible"
            title="当前在线 Web SSH 会话"
            width="680px"
            :footer="null"
            @cancel="closeActiveSessionsModal"
        >
            <div class="active-session-toolbar">
                <div>当前在线：{{ activeUserCount }} 人</div>
                <a-button size="small" :loading="activeSessionsLoading" @click="fetchActiveSessions(true)">刷新</a-button>
            </div>
            <a-table
                :columns="activeSessionColumns"
                :data-source="activeSessions"
                :loading="activeSessionsLoading"
                :pagination="false"
                row-key="id"
                size="small"
            >
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'start_time'">
                        {{ formatDateTime(record.start_time) }}
                    </template>
                </template>
            </a-table>
        </a-modal>
    </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import {
    getHostById,
    getHostWebSshActiveCount,
    getHostWebSshActiveSessions,
    getHostWebSshFiles,
    uploadHostWebSshFile,
    renameHostWebSshFile,
    deleteHostWebSshFile,
    createHostWebSshDir,
} from '@/api/assets/host/index.js'
import { getToken } from '@/api/user/index.js'
import { downloadAuditWebSshSession } from '@/api/sys/audit.js'
import { getServerUrl, getTransferServerUrl, getWebSocketBaseUrl } from '@/util/request'
import {
    formatFileSize,
    resolveFileParentDirectory,
    formatDateTime as _formatDateTime,
    formatFileMtime as _formatFileMtime,
} from '@/views/assets/host/webssh/helpers/websshUtils.js'
import {
    DOWNLOAD_ACTION_DEDUP_MS,
    DOWNLOAD_MODE_DIRECT,
    formatBytes,
    formatDuration,
    getDownloadModeLabel,
    supportsStreamFileDownload,
    parseDownloadFilename,
    parseResponseError,
    buildDownloadTargetFilename,
} from '@/views/assets/host/webssh/helpers/websshTransferHelpers.js'
import { createWebsshLayoutController } from '@/views/assets/host/webssh/controllers/websshLayoutController.js'
import { createWebsshSessionController } from '@/views/assets/host/webssh/controllers/websshSessionController.js'
import { createWebsshPresenceController } from '@/views/assets/host/webssh/controllers/websshPresenceController.js'
import { createWebsshViewController } from '@/views/assets/host/webssh/controllers/websshViewController.js'
import { createWebsshUploadController } from '@/views/assets/host/webssh/controllers/websshUploadController.js'
import { createWebsshDownloadController } from '@/views/assets/host/webssh/controllers/websshDownloadController.js'
import { createWebsshInteractionController } from '@/views/assets/host/webssh/controllers/websshInteractionController.js'
import { createWebsshLifecycleController } from '@/views/assets/host/webssh/controllers/websshLifecycleController.js'
import { createWebsshOpsController } from '@/views/assets/host/webssh/controllers/websshOpsController.js'
import { createWebsshDataController } from '@/views/assets/host/webssh/controllers/websshDataController.js'
import {
    createWebsshDownloadSetup,
    createWebsshUploadSetup,
    createWebsshInteractionSetup,
    createWebsshLifecycleSetup,
} from '@/views/assets/host/webssh/controllers/websshControllerSetup.js'
import { createWebsshFilePanelController } from '@/views/assets/host/webssh/components/WebsshFilePanel/controller.js'
import WebsshHeaderSection from './components/WebsshHeaderSection/index.vue'
import WebsshFilePanel from './components/WebsshFilePanel/index.vue'
import WebsshTerminalPanel from './components/WebsshTerminalPanel/index.vue'
import { message } from 'ant-design-vue'
import store from '@/store'

const route = useRoute()
const websshPageRef = ref(null)
const websshMainRef = ref(null)
const filePanelRef = ref(null)
const terminalRef = ref(null)
const messageText = ref('')
const messageType = ref('info')
const statusText = ref('未连接')
const currentLogId = ref(null)
const downloadingLog = ref(false)
const isFullscreen = ref(false)
const activeUserCount = ref(0)
const activeSessionsVisible = ref(false)
const activeSessionsLoading = ref(false)
const activeSessions = ref([])
const fileEntries = ref([])
const fileFilterKeyword = ref('')
const fileCurrentPath = ref('')
const filePathInput = ref('')
const previousDirectoryPath = ref('')
const fileLoading = ref(false)
const fileErrorText = ref('')
const fileTableScrollY = ref(520)
const filePanelWidth = ref(380)
const showFilePanel = ref(true)
const supportsFileOps = ref(false)
const isTemporaryCredential = ref(false)  // 临时凭证会话：隐藏重连按钮（凭证已删除，重连必失败）
const fileContextMenuVisible = ref(false)
const fileContextMenuStyle = ref({})
const fileContextMenuRecord = ref(null)
const fileContextMenuRowPath = ref('')
const dragUploadDirPath = ref('')
const downloadProgressVisible = ref(false)
const downloadProgressPercent = ref(0)
const downloadProgressStatus = ref('active')
const downloadProgressText = ref('')
const downloadFileName = ref('')
const downloadRunning = ref(false)
const uploadProgressVisible = ref(false)
const uploadProgressPercent = ref(0)
const uploadProgressStatus = ref('active')
const uploadProgressText = ref('')
const uploadFileName = ref('')
const uploadRunning = ref(false)
let lastDownloadActionKey = ''
let lastDownloadActionAt = 0

let hostId = null
let instanceName = ''
let hostIp = ''
const selectedCredentialId = ref(null)
let socket = null
let term = null
let fitAddon = null
let onDataDisposable = null
let onResizeDisposable = null
let activeCountTimer = null
let activeSessionsTimer = null
let websshHeartbeatTimer = null
let websshTransferActivityTimer = null
let downloadAbortController = null
const currentDownloadRecord = ref(null)
let downloadStopAction = null
let downloadProgressTicker = null
let downloadProgressActualBytes = 0
let downloadProgressDisplayBytes = 0
let downloadProgressTotalBytes = 0
let downloadProgressElapsedMs = 0
let downloadProgressTickAt = 0
let uploadAbortController = null
const currentUploadContext = ref(null)

const websshOpsController = createWebsshOpsController({
    computed,
    message,
    statusText,
    supportsFileOps,
})

const fileOperationsEnabled = websshOpsController.fileOperationsEnabled
const ensureFileOperationsEnabled = websshOpsController.ensureFileOperationsEnabled

const websshDataController = createWebsshDataController({
    computed,
    refs: {
        fileFilterKeyword,
        fileEntries,
        fileContextMenuRecord,
    },
})

const activeSessionColumns = websshDataController.activeSessionColumns
const fileColumns = websshDataController.fileColumns
const filteredFileEntries = websshDataController.filteredFileEntries
const fileContextMenuActions = websshDataController.fileContextMenuActions
const fileTableWrapRef = ref(null)

const layoutController = createWebsshLayoutController({
    nextTick,
    getFitAddon: () => fitAddon,
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
})

const syncTerminalFit = layoutController.syncTerminalFit
const updateFileTableScrollY = layoutController.updateFileTableScrollY
const scheduleFileTableScrollYSync = layoutController.scheduleFileTableScrollYSync
const setupFilePanelResizeObserver = layoutController.setupFilePanelResizeObserver
const disconnectFilePanelResizeObserver = layoutController.disconnectFilePanelResizeObserver
const updateFullscreenState = layoutController.updateFullscreenState
const toggleFilePanel = layoutController.toggleFilePanel
const startResize = layoutController.startResize
const stopResize = layoutController.stopResize

const handlePageUnload = () => {
    closeSocket()
}

watch(
    layoutController.watchFilePanelErrorDeps,
    () => {
        scheduleFileTableScrollYSync()
    },
)

watch(
    layoutController.watchFileListDeps,
    () => {
        scheduleFileTableScrollYSync()
    },
)

watch(
    () => fileOperationsEnabled.value,
    (enabled) => {
        if (!enabled) {
            closeFileContextMenu()
        }
    },
)

watch(
    () => [downloadRunning.value, uploadRunning.value],
    () => {
        syncWebsshTransferActivityTimer()
    },
)

watch(
    () => [downloadProgressVisible.value, uploadProgressVisible.value],
    () => {
        // Progress UI visibility changes can affect terminal viewport perception; refit rows immediately.
        syncTerminalFit()
    },
)

const sendTerminalInput = (data) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
        return
    }
    socket.send(JSON.stringify({ type: 'input', data }))
}

const writeSystemLine = (text) => {
    term?.writeln(text)
}

const handleHostOffline = (host) => {
    const displayName = String(
        host?.instance_name || host?.system?.hostname || hostIp || hostId || '当前主机',
    )
    const warningText = `检测到 ${displayName} 已离线，WebSSH 会话已断开`
    messageText.value = warningText
    messageType.value = 'warning'
    writeSystemLine(`[CLOSED] ${warningText}`)
    closeSocket()
    message.warning(warningText)
}

const websshPresenceController = createWebsshPresenceController({
    getHostId: () => hostId,
    getHostById,
    state: {
        get activeCountTimer() {
            return activeCountTimer
        },
        set activeCountTimer(value) {
            activeCountTimer = value
        },
        get activeSessionsTimer() {
            return activeSessionsTimer
        },
        set activeSessionsTimer(value) {
            activeSessionsTimer = value
        },
    },
    activeSessionsVisible,
    activeSessionsLoading,
    activeSessions,
    activeUserCount,
    getHostWebSshActiveCount,
    getHostWebSshActiveSessions,
    message,
    onHostOffline: handleHostOffline,
})

const fetchActiveUserCount = websshPresenceController.fetchActiveUserCount

const startActiveUserPolling = websshPresenceController.startActiveUserPolling
const stopActiveUserPolling = websshPresenceController.stopActiveUserPolling
const startActiveSessionsPolling = websshPresenceController.startActiveSessionsPolling
const stopActiveSessionsPolling = websshPresenceController.stopActiveSessionsPolling

const formatDateTime = (value) => _formatDateTime(value, store.state.user?.timezone)
const formatFileMtime = (value) => _formatFileMtime(value, store.state.user?.timezone)

const buildWebSocketUrl = () => {
    const token = getToken() || ''
    const credentialQuery = selectedCredentialId.value ? `&credential_id=${encodeURIComponent(selectedCredentialId.value)}` : ''
    return `${getWebSocketBaseUrl()}/ws/assets/hosts/${hostId}/webssh/?token=${encodeURIComponent(token)}${credentialQuery}`
}

const loadFiles = async (path = fileCurrentPath.value, options = {}) => {
    const { allowFallback = true, historyFromPath = '' } = options
    if (!hostId) return
    fileLoading.value = true
    try {
        const res = await getHostWebSshFiles(hostId, path)
        if (res?.data?.code === 200) {
            const payload = res.data?.data || {}
            const resolvedCurrentPath = payload.current_path || path
            fileCurrentPath.value = resolvedCurrentPath
            filePathInput.value = fileCurrentPath.value
            fileEntries.value = Array.isArray(payload.entries) ? payload.entries : []
            fileErrorText.value = ''
            fileErrorText.value = ''
            if (historyFromPath && historyFromPath !== resolvedCurrentPath) {
                previousDirectoryPath.value = historyFromPath
            }
        }
    } catch (error) {
        const errorText = error?.message || '加载文件列表失败'
        fileErrorText.value = errorText
        if (allowFallback && path === '/root') {
            await loadFiles('/', { allowFallback: false })
            return
        }
        message.error(errorText)
    } finally {
        fileLoading.value = false
        scheduleFileTableScrollYSync()
    }
}

const websshSessionController = createWebsshSessionController({
    nextTick,
    Terminal,
    FitAddon,
    terminalRef,
    state: {
        get socket() {
            return socket
        },
        set socket(value) {
            socket = value
        },
        get term() {
            return term
        },
        set term(value) {
            term = value
        },
        get fitAddon() {
            return fitAddon
        },
        set fitAddon(value) {
            fitAddon = value
        },
        get onDataDisposable() {
            return onDataDisposable
        },
        set onDataDisposable(value) {
            onDataDisposable = value
        },
        get onResizeDisposable() {
            return onResizeDisposable
        },
        set onResizeDisposable(value) {
            onResizeDisposable = value
        },
        get websshHeartbeatTimer() {
            return websshHeartbeatTimer
        },
        set websshHeartbeatTimer(value) {
            websshHeartbeatTimer = value
        },
        get websshTransferActivityTimer() {
            return websshTransferActivityTimer
        },
        set websshTransferActivityTimer(value) {
            websshTransferActivityTimer = value
        },
    },
    getHostId: () => hostId,
    statusText,
    messageText,
    messageType,
    currentLogId,
    supportsFileOps,
    isTemporaryCredential,
    fileCurrentPath,
    filePathInput,
    fileEntries,
    downloadRunning,
    uploadRunning,
    buildWebSocketUrl,
    sendTerminalInput,
    writeSystemLine,
    fetchActiveUserCount,
    loadFiles,
})

const disposeTerminal = websshSessionController.disposeTerminal
const closeSocket = websshSessionController.closeSocket
const syncWebsshTransferActivityTimer = websshSessionController.syncWebsshTransferActivityTimer
const connectWebSsh = websshSessionController.connectWebSsh
const increaseTerminalFontSize = websshSessionController.increaseTerminalFontSize
const decreaseTerminalFontSize = websshSessionController.decreaseTerminalFontSize

const websshViewController = createWebsshViewController({
    computed,
    route,
    getHostId: () => hostId,
    getInstanceName: () => instanceName,
    getHostIp: () => hostIp,
    statusText,
    connectWebSsh,
    websshPageRef,
    currentLogId,
    downloadingLog,
    downloadAuditWebSshSession,
    parseDownloadFilename,
    message,
})

const hostTitle = websshViewController.hostTitle
const statusColor = websshViewController.statusColor
const reconnect = websshViewController.reconnect
const toggleFullscreen = websshViewController.toggleFullscreen
const downloadCurrentLog = websshViewController.downloadCurrentLog
const closeTab = websshViewController.closeTab

const websshFilePanelController = createWebsshFilePanelController({
    ensureFileOperationsEnabled,
    loadFiles,
    fileCurrentPath,
    filePathInput,
    previousDirectoryPath,
    fileOperationsEnabled,
    fileContextMenuVisible,
    fileContextMenuRecord,
    fileContextMenuRowPath,
    fileContextMenuStyle,
    dragUploadDirPath,
    resolveFileParentDirectory,
    handleDirectoryDragOver: (path) => handleDirectoryDragOver(path),
    handleDirectoryDragLeave: (path) => handleDirectoryDragLeave(path),
    handleDirectoryDrop: (event, targetPath) => handleDirectoryDrop(event, targetPath),
})

const refreshFiles = websshFilePanelController.refreshFiles
const openDirectory = websshFilePanelController.openDirectory
const handlePathEnter = websshFilePanelController.handlePathEnter
const goParentDir = websshFilePanelController.goParentDir
const closeFileContextMenu = websshFilePanelController.closeFileContextMenu
const openFileContextMenu = websshFilePanelController.openFileContextMenu
const bindFileRowEvents = websshFilePanelController.bindFileRowEvents
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

const hostState = {
    get hostId() { return hostId },
    set hostId(value) { hostId = value },
    get instanceName() { return instanceName },
    set instanceName(value) { instanceName = value },
    get hostIp() { return hostIp },
    set hostIp(value) { hostIp = value },
    get selectedCredentialId() { return selectedCredentialId.value },
    set selectedCredentialId(value) { selectedCredentialId.value = value },
    get downloadAbortController() { return downloadAbortController },
    set downloadAbortController(value) { downloadAbortController = value },
    get downloadStopAction() { return downloadStopAction },
    set downloadStopAction(value) { downloadStopAction = value },
    get downloadProgressTicker() { return downloadProgressTicker },
    set downloadProgressTicker(value) { downloadProgressTicker = value },
    get downloadProgressActualBytes() { return downloadProgressActualBytes },
    set downloadProgressActualBytes(value) { downloadProgressActualBytes = value },
    get downloadProgressDisplayBytes() { return downloadProgressDisplayBytes },
    set downloadProgressDisplayBytes(value) { downloadProgressDisplayBytes = value },
    get downloadProgressTotalBytes() { return downloadProgressTotalBytes },
    set downloadProgressTotalBytes(value) { downloadProgressTotalBytes = value },
    get downloadProgressElapsedMs() { return downloadProgressElapsedMs },
    set downloadProgressElapsedMs(value) { downloadProgressElapsedMs = value },
    get downloadProgressTickAt() { return downloadProgressTickAt },
    set downloadProgressTickAt(value) { downloadProgressTickAt = value },
    get lastDownloadActionKey() { return lastDownloadActionKey },
    set lastDownloadActionKey(value) { lastDownloadActionKey = value },
    get lastDownloadActionAt() { return lastDownloadActionAt },
    set lastDownloadActionAt(value) { lastDownloadActionAt = value },
    get uploadAbortController() { return uploadAbortController },
    set uploadAbortController(value) { uploadAbortController = value },
}

const websshDownloadController = createWebsshDownloadController(createWebsshDownloadSetup({
    nextTick,
    hostState,
    constants: {
        DOWNLOAD_ACTION_DEDUP_MS,
        DOWNLOAD_MODE_DIRECT,
    },
    refs: {
        downloadRunning,
        downloadProgressVisible,
        downloadProgressPercent,
        downloadProgressStatus,
        downloadProgressText,
        downloadFileName,
        currentDownloadRecord,
    },
    message,
    helpers: {
        formatBytes,
        formatDuration,
        supportsStreamFileDownload,
        buildDownloadTargetFilename,
        getDownloadModeLabel,
        parseResponseError,
        triggerFileDownload,
    },
    deps: {
        getServerUrl,
        getTransferServerUrl,
        getToken,
    },
}))

const resetDownloadProgress = websshDownloadController.resetDownloadProgress
const enqueueDownloadTask = websshDownloadController.enqueueDownloadTask
const cancelDownload = websshDownloadController.cancelDownload
const cancelActiveDownload = websshDownloadController.cancelActiveDownload

const dismissDownloadProgress = () => {
    downloadProgressVisible.value = false
    downloadProgressStatus.value = 'active'
    downloadProgressText.value = ''
    downloadFileName.value = ''
    downloadProgressPercent.value = 0
}

const websshUploadController = createWebsshUploadController(createWebsshUploadSetup({
    hostState,
    refs: {
        uploadRunning,
        uploadProgressVisible,
        uploadProgressStatus,
        uploadProgressText,
        uploadProgressPercent,
        uploadFileName,
        fileCurrentPath,
        currentUploadContext,
    },
    message,
    ensureFileOperationsEnabled,
    formatBytes,
    formatDuration,
    deps: {
        uploadHostWebSshFile,
        loadFiles,
    },
}))

const enqueueUploadTask = websshUploadController.enqueueUploadTask
const uploadRawFileToPath = websshUploadController.uploadRawFileToPath
const uploadFile = websshUploadController.uploadFile
const cancelUpload = websshUploadController.cancelUpload

const dismissUploadProgress = () => {
    uploadProgressVisible.value = false
    uploadProgressStatus.value = 'active'
    uploadProgressText.value = ''
    uploadFileName.value = ''
    uploadProgressPercent.value = 0
}

const websshInteractionController = createWebsshInteractionController(createWebsshInteractionSetup({
    hostState,
    DOWNLOAD_MODE_DIRECT,
    message,
    refs: {
        fileCurrentPath,
        fileContextMenuRecord,
        downloadRunning,
        uploadRunning,
        dragUploadDirPath,
    },
    ensureFileOperationsEnabled,
    closeFileContextMenu,
    openDirectory,
    enqueueDownloadTask,
    uploadRawFileToPath,
    renameHostWebSshFile,
    deleteHostWebSshFile,
    createHostWebSshDir,
    loadFiles,
    helpers: {
        resolveFileParentDirectory,
    },
}))

const createDirectory = websshInteractionController.createDirectory
const handleFileContextAction = websshInteractionController.handleFileContextAction
const hideContextMenuByGlobalClick = websshInteractionController.hideContextMenuByGlobalClick
const hideContextMenuByEscape = websshInteractionController.hideContextMenuByEscape
const handleDirectoryDragOver = websshInteractionController.handleDirectoryDragOver
const handleDirectoryDragLeave = websshInteractionController.handleDirectoryDragLeave
const handleDirectoryDrop = websshInteractionController.handleDirectoryDrop
const preventGlobalFileDrop = websshInteractionController.preventGlobalFileDrop

const websshLifecycleController = createWebsshLifecycleController(createWebsshLifecycleSetup({
    nextTick,
    route,
    hostState,
    fileCurrentPath,
    actions: {
        handlePageUnload,
        hideContextMenuByGlobalClick,
        hideContextMenuByEscape,
        updateFileTableScrollY,
        syncTerminalFit,
        preventGlobalFileDrop,
        updateFullscreenState,
        startActiveUserPolling,
        connectWebSsh,
        setupFilePanelResizeObserver,
        scheduleFileTableScrollYSync,
        closeFileContextMenu,
        cancelActiveDownload,
        cancelUpload,
        stopResize,
        disconnectFilePanelResizeObserver,
        closeSocket,
        disposeTerminal,
        stopActiveUserPolling,
        stopActiveSessionsPolling,
    },
    apis: {
        getHostById,
    },
}))

const fetchActiveSessions = websshPresenceController.fetchActiveSessions
const openActiveSessionsModal = websshPresenceController.openActiveSessionsModal
const closeActiveSessionsModal = websshPresenceController.closeActiveSessionsModal

onMounted(websshLifecycleController.handleMounted)

onBeforeUnmount(websshLifecycleController.handleBeforeUnmount)
</script>

<style src="./webssh.css"></style>
