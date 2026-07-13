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
                <a-button size="small" :disabled="!currentLogId" :loading="downloadingLog" @click="downloadCurrentLog">下载日志</a-button>
                <a-button size="small" @click="toggleFullscreen">{{ getFullscreenButtonText(isFullscreen) }}</a-button>
                <a-button size="small" @click="reconnect">重连</a-button>
                <a-button size="small" type="primary" @click="closeTab">关闭</a-button>
            </a-space>
        </div>

        <div v-if="downloadProgressVisible" class="page-download-progress">
            <div class="download-progress-text">
                <span>{{ downloadFileName || '下载中' }}</span>
                <span>{{ downloadProgressText }}</span>
            </div>
            <a-progress :percent="downloadProgressPercent" :status="downloadProgressStatus" size="small" />
            <div class="download-progress-actions">
                <a-button v-if="downloadRunning" size="small" type="link" danger @click="cancelDownload">取消</a-button>
                <a-button v-else size="small" type="link" @click="dismissDownloadProgress">关闭</a-button>
            </div>
        </div>

        <div v-if="uploadProgressVisible" class="page-download-progress">
            <div class="download-progress-text">
                <span>{{ uploadFileName || '上传中' }}</span>
                <span>{{ uploadProgressText }}</span>
            </div>
            <a-progress :percent="uploadProgressPercent" :status="uploadProgressStatus" size="small" />
            <div class="download-progress-actions">
                <a-button v-if="uploadRunning" size="small" type="link" danger @click="cancelUpload">取消</a-button>
                <a-button v-else size="small" type="link" @click="dismissUploadProgress">关闭</a-button>
            </div>
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
    getFullscreenButtonText,
} = createWebsshHeaderSectionController(emit)
</script>
