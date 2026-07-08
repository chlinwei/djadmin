<template>
    <div ref="websshPageRef" class="webssh-page">
        <div class="webssh-header">
            <div class="header-left">
                <div class="title">Web SSH - {{ hostTitle }}</div>
                <div class="session-id">会话ID：{{ currentLogId || '-' }}</div>
                <div class="active-users">
                    <span>当前web ssh在线：{{ activeUserCount }} 人</span>
                    <a-button type="link" size="small" @click="openActiveSessionsModal">查看在线会话</a-button>
                </div>
            </div>
            <a-space>
                <a-tag :color="statusColor">{{ statusText }}</a-tag>
                <a-button size="small" :disabled="!currentLogId" :loading="downloadingLog" @click="downloadCurrentLog">下载日志</a-button>
                <a-button size="small" @click="toggleFullscreen">{{ isFullscreen ? '退出全屏' : '进入全屏' }}</a-button>
                <a-button size="small" @click="reconnect">重连</a-button>
                <a-button size="small" type="primary" @click="closeTab">关闭</a-button>
            </a-space>
        </div>

        <div v-if="downloadProgressVisible" class="page-download-progress">
            <div class="download-progress-text">
                <span>{{ downloadFileName || '下载中' }}</span>
                <span>{{ downloadProgressText }}</span>
            </div>
            <a-progress
                :percent="downloadProgressPercent"
                :status="downloadProgressStatus"
                size="small"
            />
            <div class="download-progress-actions">
                <a-button v-if="downloadRunning && canPauseCurrentDownload" size="small" type="link" @click="pauseDownload">暂停</a-button>
                <a-button v-if="downloadRunning" size="small" type="link" danger @click="cancelDownload">取消</a-button>
                <a-button v-if="downloadCanResume" size="small" type="link" @click="resumeDownload">继续下载</a-button>
            </div>
        </div>

        <div v-if="uploadProgressVisible" class="page-download-progress">
            <div class="download-progress-text">
                <span>{{ uploadFileName || '上传中' }}</span>
                <span>{{ uploadProgressText }}</span>
            </div>
            <a-progress
                :percent="uploadProgressPercent"
                :status="uploadProgressStatus"
                size="small"
            />
            <div class="download-progress-actions">
                <a-button v-if="uploadRunning" size="small" type="link" @click="pauseUpload">暂停</a-button>
                <a-button v-if="uploadRunning" size="small" type="link" danger @click="cancelUpload">取消</a-button>
                <a-button v-if="uploadCanResume" size="small" type="link" @click="resumeUpload">继续上传</a-button>
            </div>
        </div>

        <div ref="websshMainRef" class="webssh-main">
            <div
                v-if="showFilePanel"
                ref="filePanelRef"
                class="file-panel"
                :class="{ 'file-panel-offline': !fileOperationsEnabled }"
                :style="{ width: `${filePanelWidth}px` }"
            >
                <div class="file-toolbar">
                    <a-button size="small" :disabled="!fileOperationsEnabled || !previousDirectoryPath" @click="goParentDir">↑</a-button>
                    <a-button size="small" :loading="fileLoading" :disabled="!fileOperationsEnabled" @click="refreshFiles">刷新</a-button>
                    <a-input
                        v-model:value="fileFilterKeyword"
                        size="small"
                        allow-clear
                        class="file-filter-input"
                        placeholder="过滤当前目录"
                        :disabled="!fileOperationsEnabled"
                    />
                    <a-button size="small" :disabled="!fileOperationsEnabled" @click="createDirectory">新建目录</a-button>
                    <a-upload :show-upload-list="false" :disabled="!fileOperationsEnabled" :custom-request="uploadFile">
                        <a-button size="small" :disabled="!fileOperationsEnabled">上传文件</a-button>
                    </a-upload>
                </div>
                <div class="file-current-path">
                    <span>路径：</span>
                    <a-input
                        v-model:value="filePathInput"
                        size="small"
                        placeholder="输入目录路径后按 Enter"
                        :disabled="!fileOperationsEnabled"
                        @pressEnter="handlePathEnter"
                    />
                </div>
                <div v-if="fileErrorText" class="file-error">{{ fileErrorText }}</div>
                <div ref="fileTableWrapRef" class="file-table-wrap">
                    <a-table
                        :columns="fileColumns"
                        :data-source="filteredFileEntries"
                        :loading="fileLoading"
                        :pagination="false"
                        row-key="path"
                        size="small"
                        :scroll="{ y: fileTableScrollY }"
                        :custom-row="bindFileRowEvents"
                    >
                        <template #bodyCell="{ column, record }">
                            <template v-if="column.key === 'name'">
                                <a-button
                                    v-if="record.is_dir"
                                    type="link"
                                    size="small"
                                    class="file-link-btn"
                                    :disabled="!fileOperationsEnabled"
                                    @click="openDirectory(record.path)"
                                >
                                    📁 {{ record.name }}
                                </a-button>
                                <span v-else>📄 {{ record.name }}</span>
                            </template>
                            <template v-else-if="column.key === 'size'">
                                {{ formatFileSize(record.size) }}
                            </template>
                            <template v-else-if="column.key === 'mtime'">
                                {{ formatFileMtime(record.mtime) }}
                            </template>
                        </template>
                    </a-table>
                </div>
                <div
                    v-if="fileContextMenuVisible"
                    class="file-context-menu"
                    :style="fileContextMenuStyle"
                    @click.stop
                >
                    <div
                        v-for="action in fileContextMenuActions"
                        :key="action.key"
                        class="file-context-menu-item"
                        :class="{ danger: action.danger }"
                        @click="handleFileContextAction(action.key)"
                    >
                        {{ action.label }}
                    </div>
                </div>
            </div>

            <div v-if="showFilePanel" class="panel-resizer" @mousedown="startResize" />
            <div class="panel-toggle-handle" @click="toggleFilePanel">
                {{ showFilePanel ? '◀' : '▶' }}
            </div>

            <div class="terminal-panel">
                <a-alert
                    v-if="messageText"
                    :message="messageText"
                    :type="messageType"
                    show-icon
                    style="margin-bottom: 10px"
                />
                <div ref="terminalRef" class="terminal-wrapper" />
            </div>
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
    getHostWebSshDownloadTicket,
    getHostWebSshUploadTicket,
    uploadHostWebSshFileChunkByTicket,
    getHostWebSshFileUploadStatusByTicket,
    cancelHostWebSshFileUploadByTicket,
    renameHostWebSshFile,
    deleteHostWebSshFile,
    createHostWebSshDir,
} from '@/api/assets/host/index.js'
import { getToken } from '@/api/user/index.js'
import { downloadAuditWebSshSession } from '@/api/sys/audit.js'
import { formatTimeWithTimezone } from '@/util/timezone'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { getTransferServerUrl, getWebSocketBaseUrl } from '@/util/request'
import { listenUserTimezoneChanged } from '@/util/userTimezoneSync'
import { message } from 'ant-design-vue'

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
const userTimezone = ref(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
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
const fileContextMenuVisible = ref(false)
const fileContextMenuStyle = ref({})
const fileContextMenuRecord = ref(null)
const fileContextMenuRowPath = ref('')
const dragUploadDirPath = ref('')
const transferContextMenuVisible = ref(false)
const transferContextMenuStyle = ref({})
const transferContextMenuTarget = ref(null)
const transferContextMenuRowId = ref('')
const downloadProgressVisible = ref(false)
const downloadProgressPercent = ref(0)
const downloadProgressStatus = ref('active')
const downloadProgressText = ref('')
const downloadFileName = ref('')
const downloadCanResume = ref(false)
const downloadRunning = ref(false)
const uploadProgressVisible = ref(false)
const uploadProgressPercent = ref(0)
const uploadProgressStatus = ref('active')
const uploadProgressText = ref('')
const uploadFileName = ref('')
const uploadCanResume = ref(false)
const uploadRunning = ref(false)
const downloadQueue = ref([])
const uploadQueue = ref([])
const downloadHistory = ref([])
const uploadHistory = ref([])
let downloadQueueSeq = 0
let uploadQueueSeq = 0
let lastDownloadActionKey = ''
let lastDownloadActionAt = 0

let hostId = null
let hostName = ''
let hostIp = ''
let socket = null
let term = null
let fitAddon = null
let onDataDisposable = null
let onResizeDisposable = null
let activeCountTimer = null
let activeSessionsTimer = null
let stopListenTimezone = null
let filePanelResizeObserver = null
let resizing = false
let websshHeartbeatTimer = null
let websshTransferActivityTimer = null
let downloadAbortController = null
let pendingDownloadTask = null
const currentDownloadRecord = ref(null)
let downloadStopAction = null
let downloadProgressTicker = null
let downloadProgressActualBytes = 0
let downloadProgressDisplayBytes = 0
let downloadProgressTotalBytes = 0
let downloadProgressElapsedMs = 0
let downloadProgressTickAt = 0
let uploadAbortController = null
let pendingUploadTask = null
const currentUploadContext = ref(null)
let uploadStopAction = null
const UPLOAD_CHUNK_SIZE = 8 * 1024 * 1024
const TRANSFER_LIST_LIMIT = 5
const DOWNLOAD_ACTION_DEDUP_MS = 800
const DOWNLOAD_PROGRESS_TICK_MS = 120
const DOWNLOAD_PROGRESS_SMOOTH_FACTOR = 0.35
const DOWNLOAD_PROGRESS_MIN_STEP_BYTES = 256 * 1024
const DOWNLOAD_MODE_TICKET_STREAM = 'ticket-stream'

const canPauseCurrentDownload = computed(() => {
    return false
})

const fileOperationsEnabled = computed(() => statusText.value === '已连接')
let lastFileOpsOfflineNoticeAt = 0

const notifyFileOpsOffline = () => {
    const now = Date.now()
    if (now - lastFileOpsOfflineNoticeAt < 1500) {
        return
    }
    lastFileOpsOfflineNoticeAt = now
    message.warning('WebSSH 已离线，文件管理不可操作')
}

const ensureFileOperationsEnabled = (showTip = true) => {
    if (fileOperationsEnabled.value) {
        return true
    }
    if (showTip) {
        notifyFileOpsOffline()
    }
    return false
}

const getUploadResumeStorageKey = () => `webssh-upload-resume:${hostId || 'unknown'}`

const saveUploadResumeTask = () => {
    if (!pendingUploadTask) {
        window.localStorage.removeItem(getUploadResumeStorageKey())
        return
    }
    const payload = {
        uploadId: pendingUploadTask.uploadId,
        uploadTicket: pendingUploadTask.uploadTicket || '',
        targetPath: pendingUploadTask.targetPath,
        fileName: pendingUploadTask.fileName || pendingUploadTask.file?.name || '',
        totalSize: pendingUploadTask.totalSize || 0,
        totalChunks: pendingUploadTask.totalChunks || 0,
        nextChunkIndex: pendingUploadTask.nextChunkIndex || 0,
        uploadedBytes: pendingUploadTask.uploadedBytes || 0,
        chunkSize: pendingUploadTask.chunkSize || UPLOAD_CHUNK_SIZE,
        savedAt: Date.now(),
    }
    window.localStorage.setItem(getUploadResumeStorageKey(), JSON.stringify(payload))
}

const clearUploadResumeTask = () => {
    window.localStorage.removeItem(getUploadResumeStorageKey())
}

const pushTransferHistory = (historyRef, item) => {
    if (String(item?.status || '') === 'paused') {
        const nextPath = String(item?.path || '')
        const nextName = String(item?.name || '')
        const nextDirPath = String(item?.dirPath || '')
        historyRef.value = historyRef.value.filter((historyItem) => {
            if (String(historyItem?.status || '') !== 'paused') {
                return true
            }
            const historyPath = String(historyItem?.path || '')
            if (nextPath && historyPath) {
                return historyPath !== nextPath
            }
            return !(String(historyItem?.name || '') === nextName && String(historyItem?.dirPath || '') === nextDirPath)
        })
    }
    historyRef.value.unshift({
        id: `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
        ...item,
        at: Date.now(),
    })
    if (historyRef.value.length > TRANSFER_LIST_LIMIT) {
        historyRef.value = historyRef.value.slice(0, TRANSFER_LIST_LIMIT)
    }
}

const removeTransferHistoryItem = (historyRef, historyId) => {
    if (!historyId) return
    historyRef.value = historyRef.value.filter((item) => item.id !== historyId)
}

const trimDownloadQueueToLimit = () => {
    while (downloadQueue.value.length > TRANSFER_LIST_LIMIT) {
        const removedItem = downloadQueue.value.pop()
        if (removedItem?.record) {
            pushTransferHistory(downloadHistory, {
                name: removedItem.record.name || 'download.bin',
                path: removedItem.record.path || '',
                status: 'canceled',
                detail: `超出队列上限(${TRANSFER_LIST_LIMIT})，已自动取消`,
                dirPath: resolveFileParentDirectory(removedItem.record.path),
            })
        }
    }
}

const trimUploadQueueToLimit = () => {
    while (uploadQueue.value.length > TRANSFER_LIST_LIMIT) {
        const removedItem = uploadQueue.value.pop()
        if (removedItem?.task) {
            pushTransferHistory(uploadHistory, {
                name: removedItem.task.fileName || removedItem.task.file?.name || 'upload.bin',
                status: 'canceled',
                detail: `超出队列上限(${TRANSFER_LIST_LIMIT})，已自动取消`,
                dirPath: removedItem.task.targetPath || fileCurrentPath.value || '.',
            })
        }
    }
}

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
    const keyword = String(fileFilterKeyword.value || '').trim().toLowerCase()
    if (!keyword) return fileEntries.value
    return fileEntries.value.filter((entry) => String(entry?.name || '').toLowerCase().includes(keyword))
})

const fileContextMenuActions = computed(() => {
    const record = fileContextMenuRecord.value
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
    const target = transferContextMenuTarget.value
    if (!target) return []
    const itemType = target.type
    const item = target.item
    if (!item) return []
    if (itemType === 'download-queue') {
        return [
            { key: 'toggle', label: item.paused ? '继续' : '暂停' },
            { key: 'cancel', label: '取消', danger: true },
            { key: 'open-dir', label: '打开文件所在目录' },
        ]
    }
    if (itemType === 'upload-queue') {
        return [
            { key: 'toggle', label: item.paused ? '继续' : '暂停' },
            { key: 'cancel', label: '取消', danger: true },
            { key: 'open-dir', label: '打开文件所在目录' },
        ]
    }
    if (itemType === 'download-active') {
        const activePath = String(item?.record?.path || '')
        const actions = [
            { key: 'cancel', label: '取消', danger: true, disabled: !downloadRunning.value },
            { key: 'open-dir', label: '打开文件所在目录', disabled: !activePath },
        ]
        if (canPauseCurrentDownload.value || downloadCanResume.value) {
            actions.unshift({
                key: 'toggle',
                label: downloadRunning.value ? '暂停' : '继续',
                disabled: !downloadRunning.value && !downloadCanResume.value,
            })
        }
        return actions
    }
    if (itemType === 'upload-active') {
        return [
            { key: 'toggle', label: uploadRunning.value ? '暂停' : '继续', disabled: !uploadRunning.value && !uploadCanResume.value },
            { key: 'cancel', label: '取消', danger: true, disabled: !uploadRunning.value },
            { key: 'open-dir', label: '打开文件所在目录' },
        ]
    }
    if (itemType === 'download-history' || itemType === 'upload-history') {
        const canResumeDownload = itemType === 'download-history' && item.status === 'paused' && Boolean(item.path)
        const canResumeUpload = itemType === 'upload-history' && item.status === 'paused' && Boolean(pendingUploadTask)
        const canResume = canResumeDownload || canResumeUpload
        return [
            { key: 'toggle', label: item.status === 'paused' ? '继续' : '暂停', disabled: !canResume },
            { key: 'cancel', label: '取消', danger: true, disabled: true },
            { key: 'open-dir', label: '打开文件所在目录', disabled: !item.dirPath },
        ]
    }
    return []
})

const getTransferStatusMeta = (status) => {
    const normalized = String(status || '').toLowerCase()
    if (normalized === 'success') return { label: '成功', color: 'success' }
    if (normalized === 'error' || normalized === 'exception' || normalized === 'failed') return { label: '失败', color: 'error' }
    if (normalized === 'canceled' || normalized === 'cancelled') return { label: '已取消', color: 'default' }
    if (normalized === 'paused') return { label: '已暂停', color: 'warning' }
    if (normalized === 'queued') return { label: '排队中', color: 'processing' }
    return { label: '进行中', color: 'processing' }
}

const downloadRows = computed(() => {
    const rows = []
    if (downloadRunning.value) {
        const activeMeta = getTransferStatusMeta(downloadRunning.value ? 'running' : (downloadCanResume.value ? 'paused' : downloadProgressStatus.value))
        rows.push({
            id: 'download-active-row',
            type: 'download-active',
            isCurrent: true,
            name: downloadFileName.value || currentDownloadRecord.value?.name || '下载任务',
            detail: downloadProgressText.value || '-',
            statusLabel: activeMeta.label,
            tagColor: activeMeta.color,
            contextItem: {
                record: currentDownloadRecord.value || pendingDownloadTask?.record || null,
            },
        })
    }
    downloadQueue.value.forEach((item) => {
        const queueMeta = getTransferStatusMeta(item.paused ? 'paused' : 'queued')
        rows.push({
            id: item.id,
            type: 'download-queue',
            isCurrent: false,
            name: item?.record?.name || item?.record?.path || '未命名文件',
            detail: item?.record?.path || '-',
            statusLabel: queueMeta.label,
            tagColor: queueMeta.color,
            contextItem: item,
        })
    })
    downloadHistory.value.forEach((item) => {
        const historyMeta = getTransferStatusMeta(item.status)
        rows.push({
            id: `download-history-${item.id}`,
            type: 'download-history',
            isCurrent: false,
            name: item.name || '未命名文件',
            detail: item.detail || '-',
            statusLabel: historyMeta.label,
            tagColor: historyMeta.color,
            contextItem: item,
        })
    })
    return rows
})

const uploadRows = computed(() => {
    const rows = []
    if (uploadRunning.value) {
        const activeMeta = getTransferStatusMeta(uploadRunning.value ? 'running' : (uploadCanResume.value ? 'paused' : uploadProgressStatus.value))
        rows.push({
            id: 'upload-active-row',
            type: 'upload-active',
            isCurrent: true,
            name: uploadFileName.value || currentUploadContext.value?.fileName || '上传任务',
            detail: uploadProgressText.value || '-',
            statusLabel: activeMeta.label,
            tagColor: activeMeta.color,
            contextItem: {
                targetPath: currentUploadContext.value?.targetPath || pendingUploadTask?.targetPath || fileCurrentPath.value || '.',
            },
        })
    }
    uploadQueue.value.forEach((item) => {
        const queueMeta = getTransferStatusMeta(item.paused ? 'paused' : 'queued')
        rows.push({
            id: item.id,
            type: 'upload-queue',
            isCurrent: false,
            name: item?.task?.fileName || item?.task?.file?.name || '未命名文件',
            detail: item?.task?.targetPath || '-',
            statusLabel: queueMeta.label,
            tagColor: queueMeta.color,
            contextItem: item,
        })
    })
    uploadHistory.value.forEach((item) => {
        const historyMeta = getTransferStatusMeta(item.status)
        rows.push({
            id: `upload-history-${item.id}`,
            type: 'upload-history',
            isCurrent: false,
            name: item.name || '未命名文件',
            detail: item.detail || '-',
            statusLabel: historyMeta.label,
            tagColor: historyMeta.color,
            contextItem: item,
        })
    })
    return rows
})

const hasDownloadTask = computed(() => Boolean(downloadRows.value.length))
const hasUploadTask = computed(() => Boolean(uploadRows.value.length))
const displayedDownloadRows = computed(() => downloadRows.value.slice(0, TRANSFER_LIST_LIMIT))
const displayedUploadRows = computed(() => uploadRows.value.slice(0, TRANSFER_LIST_LIMIT))

const transferPanelDismissed = ref(false)
const transferPanelPinned = ref(false)
const transferPanelCollapsed = ref(false)
const transferPanelVisible = computed(
    () => !transferPanelDismissed.value && (transferPanelPinned.value || hasDownloadTask.value || hasUploadTask.value),
)
const transferPanelHeight = ref(230)
const fileTableWrapRef = ref(null)

const handlePageUnload = () => {
    closeSocket()
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

const syncTerminalFit = () => {
    nextTick(() => {
        fitAddon?.fit()
        scheduleFileTableScrollYSync()
    })
}

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

watch(
    () => [transferPanelVisible.value, transferPanelCollapsed.value],
    () => {
        syncTerminalFit()
    },
)

watch(
    () => [showFilePanel.value, fileErrorText.value],
    () => {
        scheduleFileTableScrollYSync()
    },
)

watch(
    () => [fileCurrentPath.value, filteredFileEntries.value.length, fileLoading.value],
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

const sendTerminalInput = (data) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
        return
    }
    socket.send(JSON.stringify({ type: 'input', data }))
}

const writeSystemLine = (text) => {
    term?.writeln(text)
}

const updateFullscreenState = () => {
    isFullscreen.value = Boolean(document.fullscreenElement)
    syncTerminalFit()
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

let transferResizing = false

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

const fetchActiveUserCount = async () => {
    if (!hostId) return
    try {
        const res = await getHostWebSshActiveCount(hostId)
        if (res?.data?.code === 200) {
            activeUserCount.value = Number(res.data?.data?.active_count || 0)
        }
    } catch (error) {
        // keep last value when fetch fails
    }
}

const createDirectory = async () => {
    if (!ensureFileOperationsEnabled()) return
    const name = window.prompt('请输入目录名', '')
    if (!name || !name.trim()) return
    try {
        await createHostWebSshDir(hostId, { path: fileCurrentPath.value || '.', name: name.trim() })
        message.success('目录创建成功')
        await loadFiles(fileCurrentPath.value)
    } catch (error) {
        message.error(error?.message || '目录创建失败')
    }
}

const startActiveUserPolling = () => {
    stopActiveUserPolling()
    fetchActiveUserCount()
    activeCountTimer = window.setInterval(fetchActiveUserCount, 5000)
}

const stopActiveUserPolling = () => {
    if (activeCountTimer) {
        window.clearInterval(activeCountTimer)
        activeCountTimer = null
    }
}

const startActiveSessionsPolling = () => {
    stopActiveSessionsPolling()
    fetchActiveSessions()
    activeSessionsTimer = window.setInterval(fetchActiveSessions, 5000)
}

const stopActiveSessionsPolling = () => {
    if (activeSessionsTimer) {
        window.clearInterval(activeSessionsTimer)
        activeSessionsTimer = null
    }
}

const hostTitle = computed(() => {
    const name = hostName || route.query.instance_name || `Host-${hostId || '-'}`
    const ip = hostIp || route.query.ip || '-'
    return `${name} (${ip})`
})

const statusColor = computed(() => {
    if (statusText.value === '已连接') return 'success'
    if (statusText.value === '连接中') return 'processing'
    if (statusText.value === '连接失败') return 'error'
    return 'default'
})

const normalizeUtcTime = (timeValue) => {
    if (!timeValue || typeof timeValue !== 'string') {
        return timeValue
    }
    const text = timeValue.trim()
    if (!text) {
        return timeValue
    }
    if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
        return text
    }
    return `${text.replace(' ', 'T')}Z`
}

const formatDateTime = (value) => {
    if (!value) {
        return '-'
    }
    return formatTimeWithTimezone(normalizeUtcTime(value), userTimezone.value, 'YYYY-MM-DD HH:mm:ss')
}

const formatFileMtime = (value) => {
    if (!value && value !== 0) return '-'
    return formatDateTime(new Date(Number(value) * 1000).toISOString())
}

const formatFileSize = (size) => {
    if (size === null || size === undefined) return '-'
    const bytes = Number(size)
    if (!Number.isFinite(bytes) || bytes < 0) return '-'
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
    return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

const loadUserTimezone = () => {
    getCurrentUserInfo()
        .then((res) => {
            const timezone = res?.data?.data?.timezone
            if (timezone) {
                userTimezone.value = timezone
            }
        })
        .catch(() => {})
}

const handleTimezoneChanged = (timezone) => {
    if (timezone) {
        userTimezone.value = timezone
    }
}

const buildWebSocketUrl = () => {
    const token = getToken() || ''
    return `${getWebSocketBaseUrl()}/ws/assets/hosts/${hostId}/webssh/?token=${encodeURIComponent(token)}`
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

const refreshFiles = () => {
    if (!ensureFileOperationsEnabled()) return
    loadFiles(fileCurrentPath.value)
}

const openDirectory = (path) => {
    if (!ensureFileOperationsEnabled()) return
    closeFileContextMenu()
    loadFiles(path, { historyFromPath: fileCurrentPath.value })
}

const resolveFileParentDirectory = (path) => {
    const rawPath = String(path || '').trim()
    if (!rawPath || rawPath === '.') return '.'
    if (rawPath === '/') return '/'
    const normalized = rawPath.endsWith('/') && rawPath.length > 1 ? rawPath.slice(0, -1) : rawPath
    const separatorIndex = normalized.lastIndexOf('/')
    if (separatorIndex < 0) return '.'
    if (separatorIndex === 0) return '/'
    return normalized.slice(0, separatorIndex)
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

const resetDownloadProgress = () => {
    stopDownloadProgressTicker()
    if (downloadAbortController) {
        downloadStopAction = 'cancel'
        downloadAbortController.abort()
    }
    pendingDownloadTask = null
    downloadProgressActualBytes = 0
    downloadProgressDisplayBytes = 0
    downloadProgressTotalBytes = 0
    downloadProgressElapsedMs = 0
    downloadProgressTickAt = 0
    downloadProgressVisible.value = false
    downloadProgressPercent.value = 0
    downloadProgressStatus.value = 'active'
    downloadProgressText.value = ''
    downloadFileName.value = ''
    downloadCanResume.value = false
    downloadRunning.value = false
    downloadQueue.value = []
    currentDownloadRecord.value = null
}

const resetUploadProgress = () => {
    if (uploadAbortController) {
        uploadStopAction = 'cancel'
        uploadAbortController.abort()
    }
    pendingUploadTask = null
    uploadProgressVisible.value = false
    uploadProgressPercent.value = 0
    uploadProgressStatus.value = 'active'
    uploadProgressText.value = ''
    uploadFileName.value = ''
    uploadCanResume.value = false
    uploadRunning.value = false
    uploadQueue.value = []
    currentUploadContext.value = null
    clearUploadResumeTask()
}

const formatBytes = (value) => {
    const bytes = Number(value || 0)
    if (!Number.isFinite(bytes) || bytes < 0) return '0 B'
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

const formatDuration = (ms) => {
    const totalSeconds = Math.max(0, Math.floor(Number(ms || 0) / 1000))
    const hours = Math.floor(totalSeconds / 3600)
    const minutes = Math.floor((totalSeconds % 3600) / 60)
    const seconds = totalSeconds % 60
    if (hours > 0) {
        return `${hours}h ${String(minutes).padStart(2, '0')}m ${String(seconds).padStart(2, '0')}s`
    }
    return `${minutes}m ${String(seconds).padStart(2, '0')}s`
}

const formatAverageSpeed = (downloaded, elapsedMs) => {
    const bytes = Number(downloaded || 0)
    const costMs = Number(elapsedMs || 0)
    if (!Number.isFinite(bytes) || !Number.isFinite(costMs) || bytes <= 0 || costMs <= 0) {
        return '0.00 B/s'
    }
    const speed = (bytes * 1000) / costMs
    if (speed < 1024) return `${speed.toFixed(2)} B/s`
    if (speed < 1024 * 1024) return `${(speed / 1024).toFixed(2)} KB/s`
    if (speed < 1024 * 1024 * 1024) return `${(speed / 1024 / 1024).toFixed(2)} MB/s`
    return `${(speed / 1024 / 1024 / 1024).toFixed(2)} GB/s`
}

const getDownloadModeLabel = (mode) => {
    void mode
    return '票据流'
}

const applyDownloadProgressDisplay = (downloaded, total, elapsedMs = 0) => {
    const totalSize = Number(total || 0)
    const doneSize = Number(downloaded || 0)
    if (!Number.isFinite(totalSize) || totalSize <= 0) {
        const avgSpeed = formatAverageSpeed(doneSize, elapsedMs)
        const totalTime = formatDuration(elapsedMs)
        // Directory tar stream has unknown total size; estimate a smooth progress curve and cap at 95% until completion.
        if (downloadRunning.value) {
            const doneMb = doneSize / (1024 * 1024)
            const estimated = Math.log2(1 + Math.max(doneMb, 0)) * 12
            downloadProgressPercent.value = Number(Math.min(95, Math.max(1, estimated)).toFixed(1))
        } else {
            downloadProgressPercent.value = 100
        }
        if (doneSize > 0) {
            downloadProgressText.value = `已下载 ${formatBytes(doneSize)} | 平均: ${avgSpeed} | 耗时: ${totalTime} | 总大小未知`
        } else {
            downloadProgressText.value = '正在下载（总大小未知）...'
        }
        return
    }
    const rawPercent = (doneSize / totalSize) * 100
    if (doneSize > 0 && rawPercent > 0 && rawPercent < 0.1) {
        downloadProgressPercent.value = 0.1
    } else {
        downloadProgressPercent.value = Number(Math.min(100, rawPercent).toFixed(1))
    }
    const avgSpeed = formatAverageSpeed(doneSize, elapsedMs)
    const totalTime = formatDuration(elapsedMs)
    downloadProgressText.value = `${formatBytes(doneSize)} / ${formatBytes(totalSize)} | 平均: ${avgSpeed} | 耗时: ${totalTime}`
}

const stopDownloadProgressTicker = () => {
    if (downloadProgressTicker) {
        window.clearInterval(downloadProgressTicker)
        downloadProgressTicker = null
    }
}

const tickDownloadProgress = () => {
    const now = performance.now()
    const tickDelta = Math.max(0, now - downloadProgressTickAt)
    downloadProgressTickAt = now
    if (downloadRunning.value) {
        downloadProgressElapsedMs += tickDelta
    }
    const totalSize = Number(downloadProgressTotalBytes || 0)
    const targetDone = Math.min(
        Number(downloadProgressActualBytes || 0),
        totalSize > 0 ? totalSize : Number(downloadProgressActualBytes || 0),
    )
    if (downloadProgressDisplayBytes < targetDone) {
        const gap = targetDone - downloadProgressDisplayBytes
        const step = Math.max(DOWNLOAD_PROGRESS_MIN_STEP_BYTES, gap * DOWNLOAD_PROGRESS_SMOOTH_FACTOR)
        downloadProgressDisplayBytes = Math.min(targetDone, downloadProgressDisplayBytes + step)
    } else if (downloadProgressDisplayBytes > targetDone) {
        downloadProgressDisplayBytes = targetDone
    }
    applyDownloadProgressDisplay(downloadProgressDisplayBytes, totalSize, downloadProgressElapsedMs)
    if (!downloadRunning.value && downloadProgressDisplayBytes >= targetDone) {
        stopDownloadProgressTicker()
    }
}

const startDownloadProgressTicker = () => {
    if (downloadProgressTicker) return
    downloadProgressTickAt = performance.now()
    downloadProgressTicker = window.setInterval(tickDownloadProgress, DOWNLOAD_PROGRESS_TICK_MS)
}

const updateDownloadProgress = (downloaded, total, elapsedMs = 0) => {
    downloadProgressActualBytes = Math.max(0, Number(downloaded || 0))
    downloadProgressTotalBytes = Math.max(0, Number(total || 0))
    downloadProgressElapsedMs = Math.max(0, Number(elapsedMs || 0))
    if (!Number.isFinite(downloadProgressDisplayBytes) || downloadProgressDisplayBytes < 0) {
        downloadProgressDisplayBytes = 0
    }
    if (downloadProgressDisplayBytes > downloadProgressActualBytes) {
        downloadProgressDisplayBytes = downloadProgressActualBytes
    }
    if (!downloadRunning.value) {
        downloadProgressDisplayBytes = downloadProgressActualBytes
        applyDownloadProgressDisplay(downloadProgressDisplayBytes, downloadProgressTotalBytes, downloadProgressElapsedMs)
        stopDownloadProgressTicker()
        return
    }
    startDownloadProgressTicker()
    tickDownloadProgress()
}

const supportsStreamFileDownload = () => {
    return Boolean(window.isSecureContext && typeof window.showSaveFilePicker === 'function')
}

const parseDownloadFilename = (contentDisposition, fallbackName = 'download.bin') => {
    const header = String(contentDisposition || '').trim()
    if (!header) {
        return fallbackName
    }

    const utf8Match = header.match(/filename\*=UTF-8''([^;]+)/i)
    if (utf8Match && utf8Match[1]) {
        try {
            return decodeURIComponent(utf8Match[1]).trim() || fallbackName
        } catch (error) {
            // ignore decode failure and continue fallback parse
        }
    }

    const quotedMatch = header.match(/filename="([^"]+)"/i)
    if (quotedMatch && quotedMatch[1]) {
        return quotedMatch[1].trim() || fallbackName
    }

    const plainMatch = header.match(/filename=([^;]+)/i)
    if (plainMatch && plainMatch[1]) {
        return plainMatch[1].trim().replace(/^"|"$/g, '') || fallbackName
    }

    return fallbackName
}

const parseResponseError = async (response) => {
    const contentType = response.headers.get('content-type') || ''
    if (contentType.includes('application/json')) {
        try {
            const payload = await response.json()
            if (payload?.msg) {
                return payload.msg
            }
        } catch (error) {
            // ignore parse failure
        }
        return null
    }
    if (contentType.includes('text/plain')) {
        try {
            const text = String(await response.text() || '').trim()
            return text || null
        } catch (error) {
            return null
        }
    }
    return null
}

const resolveDownloadUrlFromPayload = (payload) => {
    const ticket = String(payload?.ticket || '').trim()
    const ticketDownloadUrl = String(payload?.download_url || '').trim()
    if (!ticket && !ticketDownloadUrl) {
        throw new Error('下载票据无效')
    }
    if (ticketDownloadUrl) {
        return ticketDownloadUrl
    }
    if (ticket) {
        const transferBaseUrl = String(getTransferServerUrl() || '').replace(/\/$/, '')
        // Prefer stable legacy path for compatibility with not-yet-restarted transfer instances.
        return `${transferBaseUrl}/transfer/download/?ticket=${encodeURIComponent(ticket)}`
    }
    throw new Error('下载票据无效')
}

const issueDownloadTicketUrl = async (path) => {
    const ticketResponse = await getHostWebSshDownloadTicket(hostId, { path })
    if (ticketResponse?.data?.code !== 200) {
        throw new Error(ticketResponse?.data?.msg || '下载票据获取失败')
    }
    const payload = ticketResponse?.data?.data || {}
    return resolveDownloadUrlFromPayload(payload)
}

const requestDownloadSaveHandle = async (record) => {
    if (!supportsStreamFileDownload()) {
        return null
    }
    try {
        return await window.showSaveFilePicker({
            suggestedName: record?.targetFilename || record?.name || 'download.bin',
        })
    } catch (error) {
        if (error?.name === 'AbortError') {
            throw new Error('已取消选择保存位置')
        }
        throw error
    }
}

const startNextDownloadFromQueue = async () => {
    if (downloadRunning.value) return
    const nextIndex = downloadQueue.value.findIndex((item) => !item.paused)
    if (nextIndex < 0) return
    const [nextItem] = downloadQueue.value.splice(nextIndex, 1)
    await downloadFile(
        nextItem.record,
        {
            targetFilename: nextItem.record?.targetFilename || nextItem.record?.name || (nextItem.record?.is_dir ? 'directory.tar.gz' : 'download.bin'),
            downloadMode: nextItem.downloadMode || DOWNLOAD_MODE_TICKET_STREAM,
        },
        nextItem.saveHandle || null,
    )
}

const buildDownloadTargetFilename = (record) => {
    const baseName = String(record?.name || '').trim() || 'download'
    if (record?.is_dir) {
        return baseName.endsWith('.tar.gz') ? baseName : `${baseName}.tar.gz`
    }
    return baseName || 'download.bin'
}

const enqueueDownloadTask = async (record, downloadMode = DOWNLOAD_MODE_TICKET_STREAM) => {
    const recordPath = String(record?.path || '').trim()
    const actionKey = `${recordPath}|${downloadMode}`
    const now = Date.now()
    if (
        actionKey
        && actionKey === lastDownloadActionKey
        && now - lastDownloadActionAt < DOWNLOAD_ACTION_DEDUP_MS
    ) {
        return
    }
    lastDownloadActionKey = actionKey
    lastDownloadActionAt = now

    const targetFilename = buildDownloadTargetFilename(record)
    let saveHandle
    try {
        saveHandle = await requestDownloadSaveHandle({
            ...record,
            targetFilename,
        })
    } catch (error) {
        const text = error?.message || '选择保存位置失败'
        if (text !== '已取消选择保存位置') {
            message.error(text)
        }
        return
    }

    if (downloadRunning.value) {
        message.warning('当前有下载任务进行中，请稍后再试')
        return
    }
    await downloadFile(record, { targetFilename, downloadMode }, saveHandle)
}

const toggleDownloadQueueItem = (queueId) => {
    const queueItem = downloadQueue.value.find((item) => item.id === queueId)
    if (!queueItem) return
    queueItem.paused = !queueItem.paused
    if (!queueItem.paused && !downloadRunning.value) {
        void startNextDownloadFromQueue()
    }
}

const removeDownloadQueueItem = (queueId) => {
    const nextQueue = downloadQueue.value.filter((item) => item.id !== queueId)
    downloadQueue.value = nextQueue
}

const downloadFile = async (record, resumeState = null, selectedSaveHandle = null) => {
    if (!hostId) {
        message.error('缺少主机参数')
        return
    }
    currentDownloadRecord.value = record

    let downloadUrl = String(resumeState?.downloadUrl || '')
    const downloadMode = String(resumeState?.downloadMode || DOWNLOAD_MODE_TICKET_STREAM)
    const downloadModeLabel = getDownloadModeLabel(downloadMode)
    let useStreamingDownload = Boolean(selectedSaveHandle) || supportsStreamFileDownload()
    let useStreamWriter = false
    let fileWriter = null
    let saveHandle = selectedSaveHandle || resumeState?.saveHandle || null
    let targetFilename = String(resumeState?.targetFilename || buildDownloadTargetFilename(record))
    let fileSize = Number(resumeState?.fileSize || 0)
    let downloaded = Number(resumeState?.downloaded || 0)
    let elapsedMs = Number(resumeState?.elapsedMs || 0)
    const startedAt = performance.now()
    const getElapsedMs = () => elapsedMs + Math.max(0, performance.now() - startedAt)
    downloadStopAction = null
    downloadAbortController = new AbortController()
    downloadRunning.value = true

    try {
        downloadProgressVisible.value = true
        downloadProgressStatus.value = 'active'
        downloadCanResume.value = false
        downloadFileName.value = targetFilename
        downloadProgressText.value = `正在准备下载（${downloadModeLabel}）...`
        if (downloaded <= 0) {
            downloadProgressPercent.value = 0
        }
        await nextTick()
        message.info(`开始下载（${downloadModeLabel}），请查看页面顶部进度条`)

        if (!downloadUrl) {
            downloadUrl = await issueDownloadTicketUrl(record.path)
        }

        if (!fileSize) {
            const listedFileSize = Number(record.size || 0)
            if (Number.isFinite(listedFileSize) && listedFileSize > 0) {
                fileSize = listedFileSize
            }
        }

        if (!saveHandle) {
            saveHandle = await requestDownloadSaveHandle({
                ...record,
                targetFilename,
            })
            useStreamingDownload = Boolean(saveHandle)
        }
        if (!useStreamingDownload || !saveHandle) {
            downloadProgressText.value = '当前环境不支持选择本地路径，已使用浏览器默认下载目录'
            useStreamWriter = false
        } else {
            fileWriter = await saveHandle.createWritable()
            useStreamWriter = true
        }
        downloadProgressText.value = `正在建立流式传输通道（${downloadModeLabel}）...`
        const response = await fetch(downloadUrl, {
            method: 'GET',
            signal: downloadAbortController.signal,
        })
        if (response.status !== 200 && response.status !== 206) {
            const messageText = await parseResponseError(response)
            throw new Error(messageText || `下载失败（HTTP ${response.status}）`)
        }
        const contentRange = response.headers.get('content-range') || ''
        const contentLength = Number(response.headers.get('content-length') || '0')
        const totalMatch = contentRange.match(/\/(\d+)$/)
        const streamTotalSize = totalMatch?.[1] ? Number(totalMatch[1]) : (Number.isFinite(contentLength) ? contentLength : fileSize)
        if (streamTotalSize > 0) {
            fileSize = streamTotalSize
        }
        const reader = response.body?.getReader()
        if (!reader) {
            throw new Error('浏览器不支持流式下载')
        }
        const streamChunks = []
        downloaded = 0
        updateDownloadProgress(downloaded, fileSize, getElapsedMs())
        while (true) {
            const { done, value } = await reader.read()
            if (done) {
                break
            }
            if (!value || !value.length) {
                continue
            }
            if (useStreamWriter && fileWriter) {
                await fileWriter.write(value)
            } else {
                streamChunks.push(value)
            }
            downloaded += value.length
            updateDownloadProgress(downloaded, fileSize, getElapsedMs())
        }
        if (useStreamWriter && fileWriter) {
            await fileWriter.close()
            fileWriter = null
        } else {
            const blob = new Blob(streamChunks, { type: 'application/octet-stream' })
            triggerFileDownload(blob, targetFilename)
        }
        pendingDownloadTask = null
        downloadProgressStatus.value = 'success'
        elapsedMs = getElapsedMs()
        updateDownloadProgress(downloaded, fileSize, elapsedMs)
        stopDownloadProgressTicker()
        const avgSpeed = formatAverageSpeed(downloaded, elapsedMs)
        const totalTime = formatDuration(elapsedMs)
        downloadProgressPercent.value = 100
        downloadProgressText.value = `下载完成 | 平均: ${avgSpeed} | 总耗时: ${totalTime}`
        pushTransferHistory(downloadHistory, {
            name: targetFilename,
            path: record.path || '',
            status: 'success',
            detail: `${downloadModeLabel} | ${formatBytes(downloaded)} | ${avgSpeed}`,
            dirPath: resolveFileParentDirectory(record.path),
        })
        message.success('文件下载成功')
    } catch (error) {
        if (error?.name === 'AbortError') {
            if (downloadStopAction === 'pause') {
                if (useStreamingDownload) {
                    stopDownloadProgressTicker()
                    if (useStreamWriter && fileWriter) {
                        await fileWriter.close()
                        fileWriter = null
                    }
                    pendingDownloadTask = null
                    downloadProgressStatus.value = 'normal'
                    pendingDownloadTask = {
                        record,
                        saveHandle,
                        targetFilename,
                        downloadMode,
                        downloadUrl: '',
                        requestHeaders: {},
                    }
                    downloadCanResume.value = true
                    downloadProgressText.value = `已暂停（${downloadModeLabel}），可继续（将从头重新下载）`
                    pushTransferHistory(downloadHistory, {
                        name: targetFilename,
                        path: record.path || '',
                        status: 'paused',
                        detail: `${downloadModeLabel} | 已暂停，可继续（从头）`,
                        dirPath: resolveFileParentDirectory(record.path),
                    })
                    message.warning('下载已暂停，可继续（将从头开始）')
                    return
                }
                stopDownloadProgressTicker()
                pendingDownloadTask = null
                downloadProgressStatus.value = 'normal'
                downloadCanResume.value = false
                downloadProgressText.value = `已暂停（${downloadModeLabel}，流式下载不支持断点续传，请重新下载）`
                message.warning(`流式下载（${downloadModeLabel}）已暂停，需重新下载`)
            } else {
                stopDownloadProgressTicker()
                pendingDownloadTask = null
                if (useStreamWriter && fileWriter) {
                    await fileWriter.close()
                    fileWriter = null
                }
                downloadProgressVisible.value = false
                downloadProgressText.value = ''
                downloadFileName.value = ''
                downloadProgressPercent.value = 0
                downloadCanResume.value = false
                pushTransferHistory(downloadHistory, {
                    name: targetFilename,
                    path: record.path || '',
                    status: 'canceled',
                    detail: `${downloadModeLabel} | 已取消`,
                    dirPath: resolveFileParentDirectory(record.path),
                })
                message.info('已取消下载')
            }
            return
        }
        stopDownloadProgressTicker()
        pendingDownloadTask = null
        downloadProgressStatus.value = 'exception'
        downloadCanResume.value = false
        const errorText = error?.message || '下载失败'
        downloadProgressText.value = `下载失败：${errorText}`
        pushTransferHistory(downloadHistory, {
            name: targetFilename,
            path: record.path || '',
            status: 'error',
            detail: `${downloadModeLabel} | ${errorText}`,
            dirPath: resolveFileParentDirectory(record.path),
        })
        message.error(errorText || '文件下载失败')
    } finally {
        downloadRunning.value = false
        downloadStopAction = null
        downloadAbortController = null
        void startNextDownloadFromQueue()
    }
}

const resumeDownload = async () => {
    if (!pendingDownloadTask) return
    const { record } = pendingDownloadTask
    const task = pendingDownloadTask
    pendingDownloadTask = null
    await downloadFile(record, task, task?.saveHandle || null)
}

const pauseDownload = () => {
    if (!canPauseCurrentDownload.value) {
        message.info('票据流下载不支持暂停')
        return
    }
    cancelActiveDownload('pause')
}

const cancelDownload = () => {
    cancelActiveDownload('cancel')
}

const handleFileContextAction = async (actionKey) => {
    if (!ensureFileOperationsEnabled()) return
    const record = fileContextMenuRecord.value
    if (!record) return
    closeFileContextMenu()
    if (actionKey === 'open') {
        openDirectory(record.path)
        return
    }
    if (actionKey === 'download') {
        await enqueueDownloadTask(record, DOWNLOAD_MODE_TICKET_STREAM)
        return
    }
    if (actionKey === 'copy-dir-path') {
        const targetDir = resolveFileParentDirectory(record.path)
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

const cancelActiveDownload = (action = 'cancel') => {
    downloadStopAction = action
    if (downloadAbortController) {
        downloadAbortController.abort()
        return
    }
    if (action === 'cancel') {
        resetDownloadProgress()
    }
}

const createUploadId = () => {
    return `${Date.now()}_${Math.random().toString(36).slice(2, 10)}`
}

const issueUploadTicket = async (targetPath, fileName) => {
    const res = await getHostWebSshUploadTicket(hostId, {
        path: targetPath || '.',
        filename: fileName,
    })
    if (res?.data?.code !== 200) {
        throw new Error(res?.data?.msg || '上传票据获取失败')
    }
    const ticket = String(res?.data?.data?.ticket || '').trim()
    if (!ticket) {
        throw new Error('上传票据无效')
    }
    return ticket
}

const syncUploadTaskWithServer = async (task) => {
    if (!hostId || !task?.uploadId || !task?.fileName || !task?.uploadTicket) {
        return task
    }
    const res = await getHostWebSshFileUploadStatusByTicket({
        ticket: task.uploadTicket,
        upload_id: task.uploadId,
        chunk_size: task.chunkSize || UPLOAD_CHUNK_SIZE,
    })
    if (res?.data?.code !== 200) {
        return task
    }
    const payload = res?.data?.data || {}
    const nextChunkIndex = Number(payload.next_chunk_index || 0)
    const uploadedBytes = Number(payload.uploaded_size || 0)
    return {
        ...task,
        nextChunkIndex: Math.max(0, nextChunkIndex),
        uploadedBytes: Math.max(0, uploadedBytes),
    }
}

const restoreUploadResumeTask = async () => {
    if (!hostId) return
    const raw = window.localStorage.getItem(getUploadResumeStorageKey())
    if (!raw) return
    let parsed = null
    try {
        parsed = JSON.parse(raw)
    } catch (error) {
        clearUploadResumeTask()
        return
    }
    if (!parsed?.uploadId || !parsed?.fileName) {
        clearUploadResumeTask()
        return
    }
    let uploadTicket = String(parsed.uploadTicket || '').trim()
    if (!uploadTicket) {
        try {
            uploadTicket = await issueUploadTicket(parsed.targetPath || '.', parsed.fileName)
        } catch (error) {
            // keep empty ticket and let user retry via resume
        }
    }
    pendingUploadTask = {
        file: null,
        fileName: parsed.fileName,
        targetPath: parsed.targetPath || '.',
        uploadId: parsed.uploadId,
        uploadTicket,
        totalSize: Number(parsed.totalSize || 0),
        totalChunks: Number(parsed.totalChunks || 0),
        nextChunkIndex: Number(parsed.nextChunkIndex || 0),
        uploadedBytes: Number(parsed.uploadedBytes || 0),
        chunkSize: Number(parsed.chunkSize || UPLOAD_CHUNK_SIZE),
    }
    try {
        pendingUploadTask = await syncUploadTaskWithServer(pendingUploadTask)
    } catch (error) {
        // ignore sync error and use local cached state
    }
    uploadProgressVisible.value = true
    uploadProgressStatus.value = 'normal'
    uploadRunning.value = false
    uploadCanResume.value = true
    uploadFileName.value = pendingUploadTask.fileName
    uploadProgressPercent.value = Number(Math.min(100, (pendingUploadTask.uploadedBytes / Math.max(pendingUploadTask.totalSize || 1, 1)) * 100).toFixed(1))
    uploadProgressText.value = '检测到未完成上传，请重新选择同名文件后点击“继续上传”'
}

const startNextUploadFromQueue = async () => {
    if (uploadRunning.value || pendingUploadTask) return
    const nextIndex = uploadQueue.value.findIndex((item) => !item.paused)
    if (nextIndex < 0) return
    const [nextItem] = uploadQueue.value.splice(nextIndex, 1)
    if (!nextItem) return
    await startUpload(nextItem.task, nextItem.callbacks)
}

const enqueueUploadTask = (task, callbacks = {}) => {
    ensureTransferPanelVisible()
    uploadQueue.value.unshift({
        id: `upload-${Date.now()}-${uploadQueueSeq += 1}`,
        task,
        callbacks,
        paused: false,
    })
    trimUploadQueueToLimit()
    if (uploadRunning.value || pendingUploadTask) {
        message.info(`已加入上传队列（待处理 ${uploadQueue.value.length} 个）`)
        return
    }
    void startNextUploadFromQueue()
}

const toggleUploadQueueItem = (queueId) => {
    const queueItem = uploadQueue.value.find((item) => item.id === queueId)
    if (!queueItem) return
    queueItem.paused = !queueItem.paused
    if (!queueItem.paused && !uploadRunning.value && !pendingUploadTask) {
        void startNextUploadFromQueue()
    }
}

const removeUploadQueueItem = (queueId) => {
    const nextQueue = uploadQueue.value.filter((item) => item.id !== queueId)
    uploadQueue.value = nextQueue
}

const handleTransferContextAction = (action) => {
    if (!action || action.disabled) return
    const target = transferContextMenuTarget.value
    closeTransferContextMenu()
    if (!target?.item) return
    if (target.type === 'download-queue') {
        if (action.key === 'toggle') {
            toggleDownloadQueueItem(target.item.id)
            return
        }
        if (action.key === 'cancel') {
            removeDownloadQueueItem(target.item.id)
            return
        }
        if (action.key === 'open-dir') {
            const filePath = String(target.item?.record?.path || '')
            openDirectory(resolveFileParentDirectory(filePath))
        }
        return
    }
    if (target.type === 'upload-queue') {
        if (action.key === 'toggle') {
            toggleUploadQueueItem(target.item.id)
            return
        }
        if (action.key === 'cancel') {
            removeUploadQueueItem(target.item.id)
            return
        }
        if (action.key === 'open-dir') {
            const targetPath = String(target.item?.task?.targetPath || fileCurrentPath.value || '.')
            openDirectory(targetPath)
        }
        return
    }
    if (target.type === 'download-active') {
        if (action.key === 'toggle') {
            if (downloadRunning.value && canPauseCurrentDownload.value) {
                pauseDownload()
            } else if (downloadCanResume.value) {
                void resumeDownload()
            } else if (!canPauseCurrentDownload.value) {
                message.info('票据流下载不支持暂停')
            }
            return
        }
        if (action.key === 'cancel') {
            cancelDownload()
            return
        }
        if (action.key === 'open-dir') {
            const filePath = String(target.item?.record?.path || '')
            openDirectory(resolveFileParentDirectory(filePath))
        }
        return
    }
    if (target.type === 'upload-active') {
        if (action.key === 'toggle') {
            if (uploadRunning.value) {
                pauseUpload()
            } else if (uploadCanResume.value) {
                void resumeUpload()
            }
            return
        }
        if (action.key === 'cancel') {
            void cancelUpload()
            return
        }
        if (action.key === 'open-dir') {
            const targetPath = String(target.item?.targetPath || fileCurrentPath.value || '.')
            openDirectory(targetPath)
        }
        return
    }
    if (target.type === 'download-history' || target.type === 'upload-history') {
        if (action.key === 'toggle' && target.item?.status === 'paused') {
            if (target.type === 'download-history' && target.item?.path) {
                removeTransferHistoryItem(downloadHistory, target.item.id)
                if (
                    pendingDownloadTask?.record?.path
                    && String(pendingDownloadTask.record.path) === String(target.item.path)
                    && !downloadRunning.value
                    && downloadCanResume.value
                ) {
                    void resumeDownload()
                    return
                }
                void enqueueDownloadTask({
                    path: target.item.path,
                    name: target.item.name || 'download.bin',
                    is_dir: false,
                    size: 0,
                })
                return
            }
            if (target.type === 'upload-history') {
                if (!pendingUploadTask) {
                    message.warning('该上传暂停任务已失效，请重新选择文件后上传')
                    return
                }
                removeTransferHistoryItem(uploadHistory, target.item.id)
                void resumeUpload()
                return
            }
        }
        if (action.key === 'open-dir' && target.item?.dirPath) {
            openDirectory(target.item.dirPath)
        }
    }
}

const startUpload = async (task, callbacks = {}) => {
    const { onSuccess, onError, onProgress } = callbacks
    if (!task?.file) {
        message.error('无可上传文件')
        return
    }
    if (!hostId) {
        message.error('缺少主机参数')
        return
    }

    const rawFile = task.file
    const fileName = task.fileName || rawFile.name || 'upload.bin'
    const totalSize = Number(task.totalSize || rawFile.size || 0)
    const totalChunks = Number(task.totalChunks || Math.max(1, Math.ceil(totalSize / UPLOAD_CHUNK_SIZE)))
    const targetPath = task.targetPath || fileCurrentPath.value || '.'
    const uploadId = task.uploadId || createUploadId()
    let uploadTicket = String(task.uploadTicket || '').trim()
    const chunkSize = Number(task.chunkSize || UPLOAD_CHUNK_SIZE)
    let nextChunkIndex = Number(task.nextChunkIndex || 0)
    let uploadedBytes = Number(task.uploadedBytes || 0)
    currentUploadContext.value = {
        fileName,
        targetPath,
    }

    try {
        uploadTicket = await issueUploadTicket(targetPath, fileName)
        const normalizedTask = await syncUploadTaskWithServer({
            ...task,
            file: rawFile,
            fileName,
            totalSize,
            totalChunks,
            targetPath,
            uploadId,
            uploadTicket,
            chunkSize,
            nextChunkIndex,
            uploadedBytes,
        })
        nextChunkIndex = Number(normalizedTask.nextChunkIndex || 0)
        uploadedBytes = Number(normalizedTask.uploadedBytes || 0)

        uploadAbortController = new AbortController()
        uploadStopAction = null
        uploadRunning.value = true
        uploadProgressVisible.value = true
        uploadProgressStatus.value = 'active'
        uploadCanResume.value = false
        uploadFileName.value = fileName
        uploadProgressPercent.value = Number(Math.min(100, (uploadedBytes / Math.max(totalSize, 1)) * 100).toFixed(1))
        uploadProgressText.value = `${formatBytes(uploadedBytes)} / ${formatBytes(totalSize)}`

        while (nextChunkIndex < totalChunks) {
            const chunkStart = nextChunkIndex * chunkSize
            const chunkEnd = Math.min(totalSize, chunkStart + chunkSize)
            const chunkBlob = rawFile.slice(chunkStart, chunkEnd)
            const chunkBase = chunkStart

            const formData = new FormData()
            formData.append('ticket', uploadTicket)
            formData.append('upload_id', uploadId)
            formData.append('chunk_index', String(nextChunkIndex))
            formData.append('total_chunks', String(totalChunks))
            formData.append('chunk', chunkBlob, `${fileName}.part${nextChunkIndex}`)

            await uploadHostWebSshFileChunkByTicket(formData, {
                signal: uploadAbortController.signal,
                onUploadProgress: (event) => {
                    const loaded = Number(event?.loaded || 0)
                    const currentUploaded = Math.min(totalSize, chunkBase + loaded)
                    uploadProgressPercent.value = Number(Math.min(100, (currentUploaded / Math.max(totalSize, 1)) * 100).toFixed(1))
                    uploadProgressText.value = `${formatBytes(currentUploaded)} / ${formatBytes(totalSize)}`
                    if (onProgress) {
                        onProgress({ percent: uploadProgressPercent.value })
                    }
                },
            })

            uploadedBytes = chunkEnd
            nextChunkIndex += 1
            uploadProgressPercent.value = Number(Math.min(100, (uploadedBytes / Math.max(totalSize, 1)) * 100).toFixed(1))
            uploadProgressText.value = `${formatBytes(uploadedBytes)} / ${formatBytes(totalSize)}`
        }

        pendingUploadTask = null
        clearUploadResumeTask()
        uploadProgressPercent.value = 100
        uploadProgressStatus.value = 'success'
        uploadProgressText.value = '上传完成'
        pushTransferHistory(uploadHistory, {
            name: fileName,
            status: 'success',
            detail: `${formatBytes(uploadedBytes)} | 完成`,
            dirPath: targetPath,
        })
        message.success('文件上传成功')
        await loadFiles(fileCurrentPath.value)
        if (onSuccess) onSuccess()
    } catch (error) {
        const isCanceled = error?.name === 'CanceledError' || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED'
        if (isCanceled) {
            if (uploadStopAction === 'pause') {
                pendingUploadTask = {
                    file: rawFile,
                    fileName,
                    targetPath,
                    uploadId,
                    uploadTicket,
                    totalSize,
                    totalChunks,
                    nextChunkIndex,
                    uploadedBytes,
                    chunkSize,
                }
                saveUploadResumeTask()
                uploadProgressStatus.value = 'normal'
                uploadCanResume.value = nextChunkIndex > 0 && nextChunkIndex < totalChunks
                uploadProgressText.value = '已暂停'
                pushTransferHistory(uploadHistory, {
                    name: fileName,
                    status: 'paused',
                    detail: '已暂停',
                    dirPath: targetPath,
                })
                message.info('上传已暂停')
            } else {
                pendingUploadTask = null
                clearUploadResumeTask()
                if (uploadId && fileName) {
                    try {
                        await cancelHostWebSshFileUploadByTicket({ ticket: uploadTicket, upload_id: uploadId })
                    } catch (cancelError) {
                        // ignore cleanup failure, cancel should still complete
                    }
                }
                uploadProgressVisible.value = false
                uploadProgressText.value = ''
                uploadFileName.value = ''
                uploadProgressPercent.value = 0
                uploadCanResume.value = false
                pushTransferHistory(uploadHistory, {
                    name: fileName,
                    status: 'canceled',
                    detail: '已取消',
                    dirPath: targetPath,
                })
                message.info('已取消上传')
            }
            return
        }
        pendingUploadTask = {
            file: rawFile,
            fileName,
            targetPath,
            uploadId,
            uploadTicket,
            totalSize,
            totalChunks,
            nextChunkIndex,
            uploadedBytes,
            chunkSize,
        }
        saveUploadResumeTask()
        uploadProgressStatus.value = 'exception'
        uploadCanResume.value = nextChunkIndex > 0 && nextChunkIndex < totalChunks
        uploadProgressText.value = error?.message || '上传失败'
        pushTransferHistory(uploadHistory, {
            name: fileName,
            status: 'error',
            detail: error?.message || '上传失败',
            dirPath: targetPath,
        })
        message.error(error?.message || '文件上传失败')
        if (onError) onError(error)
    } finally {
        const finalStopAction = uploadStopAction
        uploadRunning.value = false
        uploadStopAction = null
        uploadAbortController = null
        if (finalStopAction !== 'pause') {
            void startNextUploadFromQueue()
        }
    }
}

const uploadRawFileToPath = async (rawFile, targetPath, callbacks = {}) => {
    const { onSuccess, onError, onProgress } = callbacks
    if (!rawFile) {
        message.error('请选择文件')
        return
    }
    if (pendingUploadTask && !pendingUploadTask.file) {
        const expectedName = String(pendingUploadTask.fileName || '')
        const expectedSize = Number(pendingUploadTask.totalSize || 0)
        if (rawFile.name !== expectedName || Number(rawFile.size || 0) !== expectedSize) {
            message.error(`请重新选择同一文件继续上传：${expectedName}`)
            return
        }
        pendingUploadTask.file = rawFile
        await startUpload(pendingUploadTask, { onSuccess, onError, onProgress })
        return
    }
    const totalSize = Number(rawFile?.size || 0)
    const totalChunks = Math.max(1, Math.ceil(totalSize / UPLOAD_CHUNK_SIZE))
    const task = {
        file: rawFile,
        fileName: rawFile.name,
        targetPath: targetPath || fileCurrentPath.value || '.',
        uploadId: createUploadId(),
        uploadTicket: '',
        totalSize,
        totalChunks,
        nextChunkIndex: 0,
        uploadedBytes: 0,
        chunkSize: UPLOAD_CHUNK_SIZE,
    }
    enqueueUploadTask(task, { onSuccess, onError, onProgress })
}

const uploadFile = async ({ file, onSuccess, onError, onProgress }) => {
    if (!ensureFileOperationsEnabled()) return
    const rawFile = file?.originFileObj || file
    await uploadRawFileToPath(rawFile, fileCurrentPath.value || '.', {
        onSuccess,
        onError,
        onProgress,
    })
}

const handleDirectoryDragOver = (path) => {
    dragUploadDirPath.value = path
}

const handleDirectoryDragLeave = (path) => {
    if (dragUploadDirPath.value === path) {
        dragUploadDirPath.value = ''
    }
}

const handleDirectoryDrop = async (event, targetPath) => {
    const normalizedPath = String(targetPath || '.')
    const droppedFiles = Array.from(event?.dataTransfer?.files || [])
    dragUploadDirPath.value = ''
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

const resumeUpload = async () => {
    if (!pendingUploadTask) return
    if (!pendingUploadTask.file) {
        message.warning('请先点击“上传文件”重新选择同名文件，再继续上传')
        return
    }
    const task = pendingUploadTask
    pendingUploadTask = null
    await startUpload(task)
}

const pauseUpload = () => {
    uploadStopAction = 'pause'
    if (uploadAbortController) {
        uploadAbortController.abort()
    }
}

const cancelUpload = async () => {
    uploadStopAction = 'cancel'
    if (uploadAbortController) {
        uploadAbortController.abort()
        return
    }
    if (pendingUploadTask?.uploadId && (pendingUploadTask?.fileName || pendingUploadTask?.file?.name)) {
        try {
            let uploadTicket = String(pendingUploadTask.uploadTicket || '').trim()
            if (!uploadTicket) {
                uploadTicket = await issueUploadTicket(
                    pendingUploadTask.targetPath || fileCurrentPath.value || '.',
                    pendingUploadTask.fileName || pendingUploadTask.file.name,
                )
            }
            await cancelHostWebSshFileUploadByTicket({
                ticket: uploadTicket,
                upload_id: pendingUploadTask.uploadId,
            })
        } catch (error) {
            // ignore cleanup failure when canceling local pending task
        }
    }
    resetUploadProgress()
    message.info('已取消上传')
}

const renameFile = async (record) => {
    const nextName = window.prompt('请输入新文件名', record.name || '')
    if (!nextName || !nextName.trim() || nextName.trim() === record.name) return
    try {
        await renameHostWebSshFile(hostId, { path: record.path, new_name: nextName.trim() })
        message.success('重命名成功')
        await loadFiles(fileCurrentPath.value)
    } catch (error) {
        message.error(error?.message || '重命名失败')
    }
}

const deleteFile = async (record) => {
    const confirmText = record.is_dir ? `确定删除目录 ${record.name}（含子文件）吗？` : `确定删除文件 ${record.name} 吗？`
    if (!window.confirm(confirmText)) return
    try {
        await deleteHostWebSshFile(hostId, { path: record.path, recursive: Boolean(record.is_dir) })
        message.success('删除成功')
        await loadFiles(fileCurrentPath.value)
    } catch (error) {
        message.error(error?.message || '删除失败')
    }
}

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

const disposeTerminal = () => {
    if (onDataDisposable) {
        onDataDisposable.dispose()
        onDataDisposable = null
    }
    if (onResizeDisposable) {
        onResizeDisposable.dispose()
        onResizeDisposable = null
    }
    if (term) {
        term.dispose()
        term = null
    }
    fitAddon = null
}

const closeSocket = () => {
    if (websshHeartbeatTimer) {
        window.clearInterval(websshHeartbeatTimer)
        websshHeartbeatTimer = null
    }
    if (websshTransferActivityTimer) {
        window.clearInterval(websshTransferActivityTimer)
        websshTransferActivityTimer = null
    }
    if (!socket) return
    try {
        if (socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'close' }))
        }
        socket.close()
    } catch (error) {
        // ignore close exception
    }
    socket = null
}

const startWebsshHeartbeat = () => {
    if (websshHeartbeatTimer) return
    websshHeartbeatTimer = window.setInterval(() => {
        if (!socket || socket.readyState !== WebSocket.OPEN) {
            return
        }
        try {
            socket.send(JSON.stringify({ type: 'ping' }))
        } catch (error) {
            // ignore heartbeat send failure
        }
    }, 20000)
}

const stopWebsshHeartbeat = () => {
    if (!websshHeartbeatTimer) return
    window.clearInterval(websshHeartbeatTimer)
    websshHeartbeatTimer = null
}

const sendWebsshTransferActivity = () => {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
        return
    }
    try {
        socket.send(JSON.stringify({ type: 'transfer_activity' }))
    } catch (error) {
        // ignore activity send failure
    }
}

const syncWebsshTransferActivityTimer = () => {
    const hasActiveTransfer = downloadRunning.value || uploadRunning.value
    if (!hasActiveTransfer) {
        if (websshTransferActivityTimer) {
            window.clearInterval(websshTransferActivityTimer)
            websshTransferActivityTimer = null
        }
        return
    }

    sendWebsshTransferActivity()
    if (websshTransferActivityTimer) {
        return
    }
    websshTransferActivityTimer = window.setInterval(() => {
        sendWebsshTransferActivity()
    }, 20000)
}

const initTerminal = async () => {
    await nextTick()
    if (!terminalRef.value) return

    disposeTerminal()
    term = new Terminal({
        cursorBlink: true,
        fontFamily: 'Consolas, Menlo, monospace',
        fontSize: 14,
        theme: {
            background: '#0b1220',
            foreground: '#e2e8f0',
        },
        convertEol: true,
        scrollback: 5000,
    })

    fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    terminalRef.value.innerHTML = ''
    term.open(terminalRef.value)
    fitAddon.fit()
    term.focus()

    onDataDisposable = term.onData((data) => {
        sendTerminalInput(data)
    })

    onResizeDisposable = term.onResize(({ cols, rows }) => {
        if (socket && socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'resize', cols, rows }))
        }
    })
}

const connectWebSsh = async () => {
    if (!hostId) {
        messageText.value = '缺少 host_id 参数'
        messageType.value = 'error'
        statusText.value = '连接失败'
        return
    }

    try {
        closeSocket()
        await initTerminal()

        statusText.value = '连接中'
        currentLogId.value = null
        messageText.value = '正在建立 SSH 连接...'
        messageType.value = 'info'
        writeSystemLine('Connecting...')

        socket = new WebSocket(buildWebSocketUrl())

        socket.onopen = () => {
            statusText.value = '连接中'
            messageText.value = 'WebSocket 已建立，等待 SSH 会话连接...'
            messageType.value = 'info'
            startWebsshHeartbeat()
            syncWebsshTransferActivityTimer()

            if (term) {
                term.focus()
                socket.send(JSON.stringify({ type: 'resize', cols: term.cols || 120, rows: term.rows || 32 }))
                // 连接阶段自动触发提示符。
                sendTerminalInput('\r')
            }
        }

        socket.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data)
                if (msg.type === 'output') {
                    term?.write(msg.data)
                } else if (msg.type === 'connected') {
                    statusText.value = '已连接'
                    messageText.value = ''
                    currentLogId.value = msg.log_id || null
                    // 使用返回的家目录，如果没有则使用默认的 /root
                    const homeDir = msg.home_dir || '/root'
                    fileCurrentPath.value = homeDir
                    filePathInput.value = homeDir
                    fetchActiveUserCount()
                    if (!fileEntries.value.length) {
                        loadFiles(fileCurrentPath.value)
                    }
                    writeSystemLine(`Connected to ${msg.host_name} (${msg.ip})`)
                } else if (msg.type === 'error') {
                    statusText.value = '连接失败'
                    messageText.value = msg.message || '连接失败'
                    messageType.value = 'error'
                    writeSystemLine(`[ERROR] ${msg.message || '连接失败'}`)
                } else if (msg.type === 'closed') {
                    statusText.value = '未连接'
                    messageText.value = msg.message || '会话已关闭'
                    messageType.value = 'warning'
                    stopWebsshHeartbeat()
                    fetchActiveUserCount()
                    writeSystemLine(`[CLOSED] ${msg.message || '会话已关闭'}`)
                    if (socket && socket.readyState === WebSocket.OPEN) {
                        socket.close()
                    }
                } else if (msg.type === 'pong') {
                    // heartbeat ack
                }
            } catch (error) {
                term?.write(event.data)
            }
        }

        socket.onerror = () => {
            stopWebsshHeartbeat()
            syncWebsshTransferActivityTimer()
            statusText.value = '连接失败'
            messageText.value = 'WebSocket 连接异常，请确认后端使用 ASGI/Daphne 方式启动并监听 9000 端口'
            messageType.value = 'error'
        }

        socket.onclose = () => {
            stopWebsshHeartbeat()
            syncWebsshTransferActivityTimer()
            const wasConnected = statusText.value === '已连接'
            if (statusText.value !== '连接失败') {
                statusText.value = '未连接'
            }
            if (wasConnected && !messageText.value) {
                messageText.value = 'SSH 会话已断开'
                messageType.value = 'warning'
            }
            fetchActiveUserCount()
        }
    } catch (error) {
        statusText.value = '连接失败'
        messageText.value = error?.message || '终端初始化失败'
        messageType.value = 'error'
    }
}

const reconnect = () => {
    connectWebSsh()
}

const fetchActiveSessions = async (showError = false) => {
    if (!hostId) return
    activeSessionsLoading.value = true
    try {
        const res = await getHostWebSshActiveSessions(hostId)
        if (res?.data?.code === 200) {
            const payload = res.data?.data || {}
            activeSessions.value = Array.isArray(payload.sessions) ? payload.sessions : []
            activeUserCount.value = Number(payload.active_count || activeSessions.value.length || 0)
        }
    } catch (error) {
        if (showError) {
            message.error(error?.message || '获取在线会话失败')
        }
    } finally {
        activeSessionsLoading.value = false
    }
}

const openActiveSessionsModal = () => {
    activeSessionsVisible.value = true
    startActiveSessionsPolling()
}

const closeActiveSessionsModal = () => {
    activeSessionsVisible.value = false
    stopActiveSessionsPolling()
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

onMounted(async () => {
    const routeHostId = route.query.host_id || route.query.id || route.params.host_id || route.params.id || 0
    hostId = Number(routeHostId)
    hostName = String(route.query.instance_name || route.query.name || '')
    hostIp = String(route.query.ip || '')

    window.addEventListener('pagehide', handlePageUnload)
    window.addEventListener('beforeunload', handlePageUnload)
    window.addEventListener('click', hideContextMenuByGlobalClick)
    window.addEventListener('resize', hideContextMenuByGlobalClick)
    window.addEventListener('resize', updateFileTableScrollY)
    window.addEventListener('keydown', hideContextMenuByEscape)
    window.addEventListener('dragover', preventGlobalFileDrop)
    window.addEventListener('drop', preventGlobalFileDrop)
    document.addEventListener('fullscreenchange', updateFullscreenState)
    stopListenTimezone = listenUserTimezoneChanged(handleTimezoneChanged)
    loadUserTimezone()
    startActiveUserPolling()

    if (hostId) {
        getHostById(hostId)
            .then((res) => {
                if (res.data?.code === 200 && res.data?.data) {
                    const host = res.data.data
                    hostName = host.instance_name || host.name || hostName
                    hostIp = host.ip || hostIp
                }
            })
            .catch(() => {
                // fallback to query display
            })
    }

    await restoreUploadResumeTask()
    void connectWebSsh()
    nextTick(() => {
        setupFilePanelResizeObserver()
        scheduleFileTableScrollYSync()
    })
})

onBeforeUnmount(() => {
    window.removeEventListener('pagehide', handlePageUnload)
    window.removeEventListener('beforeunload', handlePageUnload)
    window.removeEventListener('click', hideContextMenuByGlobalClick)
    window.removeEventListener('resize', hideContextMenuByGlobalClick)
    window.removeEventListener('resize', updateFileTableScrollY)
    window.removeEventListener('keydown', hideContextMenuByEscape)
    window.removeEventListener('dragover', preventGlobalFileDrop)
    window.removeEventListener('drop', preventGlobalFileDrop)
    document.removeEventListener('fullscreenchange', updateFullscreenState)
    stopDownloadProgressTicker()
    closeFileContextMenu()
    cancelActiveDownload()
    cancelUpload()
    stopResize()
    stopTransferResize()
    if (filePanelResizeObserver) {
        filePanelResizeObserver.disconnect()
        filePanelResizeObserver = null
    }
    closeSocket()
    disposeTerminal()
    stopActiveUserPolling()
    stopActiveSessionsPolling()
    if (stopListenTimezone) {
        stopListenTimezone()
        stopListenTimezone = null
    }
})
</script>

<style scoped>
.webssh-page {
    height: 100vh;
    display: flex;
    flex-direction: column;
    background: #f5f7fb;
    padding: 12px;
    box-sizing: border-box;
}

.webssh-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 10px;
}

.header-left {
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.title {
    font-size: 18px;
    font-weight: 600;
    color: #0f172a;
}

.active-users {
    display: flex;
    align-items: center;
    gap: 8px;
    color: #334155;
    font-size: 13px;
}

.header-download-progress {
    margin-top: 4px;
    max-width: 460px;
    padding: 8px;
    border: 1px solid #e2e8f0;
    border-radius: 6px;
    background: #f8fafc;
}

.active-session-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
}

.webssh-main {
    flex: 1;
    min-height: 0;
    display: flex;
    align-items: stretch;
    gap: 0;
}

.file-panel {
    min-width: 260px;
    background: #fff;
    border: 1px solid #d9e2ef;
    border-radius: 8px;
    padding: 10px;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    overflow: hidden;
}

.file-panel-offline {
    opacity: 0.68;
}

.panel-resizer {
    width: 10px;
    cursor: col-resize;
    position: relative;
    flex: 0 0 10px;
}

.panel-resizer::before {
    content: '';
    position: absolute;
    top: 0;
    bottom: 0;
    left: 4px;
    width: 2px;
    background: #d9e2ef;
    border-radius: 2px;
}

.panel-resizer:hover::before {
    background: #94a3b8;
}

.panel-toggle-handle {
    width: 18px;
    flex: 0 0 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #64748b;
    background: #f8fafc;
    border: 1px solid #d9e2ef;
    border-radius: 6px;
    cursor: pointer;
    user-select: none;
    margin: 0 6px 0 2px;
}

.panel-toggle-handle:hover {
    color: #2563eb;
    border-color: #93c5fd;
    background: #eff6ff;
}

.file-toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
}

.file-filter-input {
    width: 140px;
}

.file-current-path {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: #64748b;
    margin-bottom: 8px;
}

.file-error {
    font-size: 12px;
    color: #dc2626;
    margin-bottom: 8px;
    word-break: break-all;
}

.file-table-wrap {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
}

.file-table-wrap :deep(.ant-spin-nested-loading),
.file-table-wrap :deep(.ant-spin-container),
.file-table-wrap :deep(.ant-table-wrapper),
.file-table-wrap :deep(.ant-table),
.file-table-wrap :deep(.ant-table-container) {
    height: 100%;
    min-height: 0;
}

.file-table-wrap :deep(.ant-table) {
    display: flex;
    flex-direction: column;
}

.file-table-wrap :deep(.ant-table-container) {
    display: flex;
    flex-direction: column;
}

.file-table-wrap :deep(.ant-table-header) {
    flex: 0 0 auto;
}

.file-table-wrap :deep(.ant-table-body) {
    flex: 1 1 auto;
    min-height: 0;
    height: 100% !important;
    max-height: 100% !important;
}

.download-progress-text {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
    color: #334155;
    margin-bottom: 6px;
}

.download-progress-actions {
    margin-top: 4px;
    text-align: right;
}

.file-context-menu {
    position: fixed;
    z-index: 1200;
    min-width: 140px;
    background: #ffffff;
    border: 1px solid #d9e2ef;
    border-radius: 6px;
    box-shadow: 0 6px 20px rgba(15, 23, 42, 0.15);
    padding: 4px 0;
}

.file-context-menu-item {
    padding: 8px 12px;
    font-size: 13px;
    color: #0f172a;
    cursor: pointer;
    user-select: none;
}

.file-context-menu-item:hover {
    background: #f1f5f9;
}

.file-context-menu-item.danger {
    color: #dc2626;
}

.file-context-menu-item.disabled {
    color: #94a3b8;
    cursor: not-allowed;
}

.file-context-menu-item.disabled:hover {
    background: transparent;
}

.transfer-dock {
    margin-top: 10px;
    border: 1px solid #d9e2ef;
    border-radius: 8px;
    background: #fff;
    display: flex;
    flex-direction: column;
    min-height: 42px;
    overflow: hidden;
}

.transfer-dock-header {
    height: 42px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 10px;
    border-bottom: 1px solid #e2e8f0;
    background: #f8fafc;
    box-sizing: border-box;
    font-size: 13px;
    font-weight: 600;
    color: #0f172a;
}

.transfer-dock-resizer {
    position: relative;
    height: 10px;
    cursor: row-resize;
    background: #f8fafc;
    border-bottom: 1px solid #e2e8f0;
}

.transfer-dock-resizer::before {
    content: '';
    position: absolute;
    top: 4px;
    left: 50%;
    transform: translateX(-50%);
    width: 60px;
    height: 2px;
    border-radius: 999px;
    background: #94a3b8;
}

.transfer-dock-content {
    flex: 1;
    min-height: 0;
    display: flex;
    gap: 10px;
    padding: 10px;
    box-sizing: border-box;
}

.download-floating-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    width: 100%;
    background: #ffffff;
    border: 1px solid #d9e2ef;
    border-radius: 8px;
    padding: 10px;
    box-sizing: border-box;
    overflow: hidden;
}

.upload-floating-panel {
    margin-top: 0;
}

.download-floating-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 4px;
    font-size: 13px;
    font-weight: 600;
    color: #0f172a;
}

.download-empty-tip {
    font-size: 12px;
    color: #64748b;
    margin-top: 8px;
}

.queue-list {
    margin-top: 8px;
    flex: 1;
    min-height: 0;
    max-height: none;
    overflow: auto;
    border: 1px solid #e2e8f0;
    border-radius: 6px;
    background: #f8fafc;
    padding: 6px 8px;
}

.queue-title {
    font-size: 12px;
    color: #475569;
    margin-bottom: 4px;
}

.queue-item {
    font-size: 13px;
    color: #334155;
    line-height: 1.75;
    word-break: break-all;
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 8px;
    padding: 6px 0;
    cursor: context-menu;
}

.unified-transfer-list {
    flex: 1;
    min-height: 0;
}

.transfer-row {
    border-radius: 4px;
    padding: 8px 10px;
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
}

.transfer-row.is-current {
    background: #e8f4ff;
}

.transfer-row.is-context-row {
    outline: 1px solid #69b1ff;
    background: #f0f7ff;
}

.transfer-row-detail {
    color: #64748b;
    font-size: 12px;
}

.queue-name {
    flex: 1;
    min-width: 0;
}

.queue-item-actions {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
}

.queue-overflow-tip {
    margin-top: 4px;
    font-size: 12px;
    color: #64748b;
}

.file-link-btn {
    padding: 0;
}

:deep(.dir-drop-row > td) {
    background: #eff6ff !important;
}

:deep(.dir-drop-row .file-link-btn) {
    color: #2563eb !important;
}

:deep(.context-selected-row > td) {
    background: #e6f4ff !important;
}

.terminal-panel {
    flex: 1;
    min-width: 420px;
    display: flex;
    flex-direction: column;
}

.terminal-wrapper {
    flex: 1;
    min-height: 0;
    border: 1px solid #1f2937;
    border-radius: 8px;
    overflow: hidden;
    background: #0b1220;
}

.terminal-wrapper :deep(.xterm) {
    padding: 8px 10px;
    box-sizing: border-box;
}
</style>
