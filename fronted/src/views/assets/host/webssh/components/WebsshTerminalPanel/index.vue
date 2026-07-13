<template>
    <div class="terminal-panel">
        <a-alert
            v-if="shouldShowMessage(messageText)"
            :message="messageText"
            :type="resolveAlertType(messageType)"
            show-icon
            style="margin-bottom: 10px"
        />
        <div :ref="setTerminalRef" class="terminal-wrapper" />
    </div>
</template>

<script setup>
import { createWebsshTerminalPanelController, websshTerminalPanelProps } from './controller.js'

defineProps(websshTerminalPanelProps)

const { shouldShowMessage, resolveAlertType } = createWebsshTerminalPanelController()
</script>
