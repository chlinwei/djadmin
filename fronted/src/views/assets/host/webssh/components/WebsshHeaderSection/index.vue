<template>
    <div class="webssh-header-block">
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
                <a-tooltip title="缩小终端字体（Ctrl/⌘ + 滚轮也可）">
                    <a-button size="small" @click="decreaseFontSize">A-</a-button>
                </a-tooltip>
                <a-tooltip title="放大终端字体（Ctrl/⌘ + 滚轮也可）">
                    <a-button size="small" @click="increaseFontSize">A+</a-button>
                </a-tooltip>
                <a-button size="small" :disabled="!currentLogId" :loading="downloadingLog" @click="downloadCurrentLog">下载日志</a-button>
                <a-button size="small" @click="toggleFullscreen">{{ getFullscreenButtonText(isFullscreen) }}</a-button>
                <a-button v-if="!isTemporaryCredential" size="small" @click="reconnect">重连</a-button>
                <a-button size="small" type="primary" @click="closeTab">关闭</a-button>
            </a-space>
        </div>

        <div v-if="downloadProgressVisible" class="page-transfer-progress">
            <div class="transfer-progress-meta">
                <span class="transfer-progress-name">{{ downloadFileName || '下载中' }}</span>
                <span class="transfer-progress-info">{{ downloadProgressText }}</span>
                <span class="transfer-progress-percent">{{ downloadProgressPercent }}%</span>
                <a-button v-if="downloadRunning" size="small" type="link" danger class="transfer-progress-action" @click="cancelDownload">取消</a-button>
                <a-button v-else size="small" type="link" class="transfer-progress-action" @click="dismissDownloadProgress">关闭</a-button>
            </div>
            <a-progress
                :percent="downloadProgressPercent"
                :show-info="false"
                :stroke-color="downloadProgressStatus === 'exception' ? '#ff4d4f' : '#52c41a'"
                size="small"
            />
        </div>

        <div v-if="uploadProgressVisible" class="page-transfer-progress">
            <div class="transfer-progress-meta">
                <span class="transfer-progress-name">{{ uploadFileName || '上传中' }}</span>
                <span class="transfer-progress-info">{{ uploadProgressText }}</span>
                <span class="transfer-progress-percent">{{ uploadProgressPercent }}%</span>
                <a-button v-if="uploadRunning" size="small" type="link" danger class="transfer-progress-action" @click="cancelUpload">取消</a-button>
                <a-button v-else size="small" type="link" class="transfer-progress-action" @click="dismissUploadProgress">关闭</a-button>
            </div>
            <a-progress
                :percent="uploadProgressPercent"
                :show-info="false"
                :stroke-color="uploadProgressStatus === 'exception' ? '#ff4d4f' : '#52c41a'"
                size="small"
            />
        </div>
    </div>
</template>

<script setup>
import {
    createWebsshHeaderSectionController,
    websshHeaderSectionEvents,
    websshHeaderSectionProps,
} from './controller.js'

const emit = defineEmits(websshHeaderSectionEvents)
defineProps(websshHeaderSectionProps)

const {
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
} = createWebsshHeaderSectionController(emit)
</script>
