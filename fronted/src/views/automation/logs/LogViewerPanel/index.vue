<template>
  <div>
    <a-space class="log-toolbar" wrap>
      <a-tag :color="statusTagColor">{{ statusText || '-' }}</a-tag>
      <span class="log-stream-meta">最后输出: {{ lastOutputText || '-' }}</span>
      <a-space size="small">
        <span>自动换行</span>
        <a-switch :checked="wrap" size="small" @change="onWrapChange" />
      </a-space>
      <a-space v-if="showAutoFollow" size="small">
        <span>自动追尾</span>
        <a-switch :checked="autoFollowEnabled" size="small" @change="onAutoFollowChange" />
      </a-space>
      <a-space size="small">
        <a-tooltip title="减小字号">
          <a-button size="small" @click="$emit('decrease-font')">A-</a-button>
        </a-tooltip>
        <span>{{ fontSize }}px</span>
        <a-tooltip title="增大字号">
          <a-button size="small" @click="$emit('increase-font')">A+</a-button>
        </a-tooltip>
      </a-space>
      <a-tooltip
        v-if="showAutoFollow && autoFollowEnabled && autoFollowSuspended"
        title="回到底部"
      >
        <a-button
          size="small"
          type="primary"
          ghost
          @click="$emit('resume-auto-follow')"
        >回到底部</a-button>
      </a-tooltip>
      <slot name="actions">
        <a-tooltip v-if="showCancel" title="取消">
          <a-button
            size="small"
            danger
            :loading="cancelLoading"
            :disabled="cancelDisabled"
            @click="$emit('cancel')"
          >取消任务</a-button>
        </a-tooltip>
        <a-tooltip v-if="showCopy" title="复制">
          <a-button size="small" @click="$emit('copy')">复制</a-button>
        </a-tooltip>
        <a-tooltip v-if="showDownload" title="下载日志">
          <a-button size="small" @click="$emit('download')">下载</a-button>
        </a-tooltip>
      </slot>
    </a-space>
    <div
      ref="shellRef"
      class="log-viewer-shell"
      @scroll="$emit('scroll')"
    >
      <pre
        :class="['log-viewer-content', { 'is-nowrap': !wrap }]"
        :style="{ fontSize: `${fontSize}px` }"
        v-html="htmlContent || '-'"
      ></pre>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'

const props = defineProps({
  statusTagColor: {
    type: String,
    default: 'default',
  },
  statusText: {
    type: String,
    default: '-',
  },
  lastOutputText: {
    type: String,
    default: '-',
  },
  wrap: {
    type: Boolean,
    default: true,
  },
  fontSize: {
    type: Number,
    default: 13,
  },
  htmlContent: {
    type: String,
    default: '',
  },
  showAutoFollow: {
    type: Boolean,
    default: false,
  },
  autoFollowEnabled: {
    type: Boolean,
    default: false,
  },
  autoFollowSuspended: {
    type: Boolean,
    default: false,
  },
  showCancel: {
    type: Boolean,
    default: false,
  },
  cancelLoading: {
    type: Boolean,
    default: false,
  },
  cancelDisabled: {
    type: Boolean,
    default: false,
  },
  showCopy: {
    type: Boolean,
    default: true,
  },
  showDownload: {
    type: Boolean,
    default: true,
  },
})

const emit = defineEmits([
  'update:wrap',
  'decrease-font',
  'increase-font',
  'toggle-auto-follow',
  'resume-auto-follow',
  'cancel',
  'copy',
  'download',
  'scroll',
  'shell-ready',
])

const shellRef = ref(null)

function onWrapChange(checked) {
  emit('update:wrap', Boolean(checked))
}

function onAutoFollowChange(checked) {
  emit('toggle-auto-follow', Boolean(checked))
}

onMounted(() => {
  emit('shell-ready', shellRef.value)
})

watch(shellRef, (value) => {
  emit('shell-ready', value)
})

watch(
  () => props.wrap,
  () => {
    emit('shell-ready', shellRef.value)
  }
)
</script>
