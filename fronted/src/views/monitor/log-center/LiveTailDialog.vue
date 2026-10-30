<template>
  <a-modal :open="open" title="实时跟踪源文件 · 原始文件内容" width="92%" :footer="null" @cancel="closeAll">
    <a-tabs :active-key="activeKey" type="editable-card" hide-add @change="activate" @edit="removeTab">
      <a-tab-pane v-for="tab in tabs" :key="tab.key" :tab="tab.label" />
    </a-tabs>
    <div v-if="activeTab" class="tail-layout">
      <aside class="tail-sidebar" aria-label="跟踪信息与操作">
        <div class="tail-status">
          <span class="tail-section-title">跟踪状态</span>
          <a-tag :color="activeTab.status === '跟踪中' ? 'green' : 'default'">{{ activeTab.status }}</a-tag>
        </div>
        <a-space wrap>
          <a-tooltip v-if="activeTab.socket" title="取消" placement="top">
            <a-button @click="stop(activeTab)">停止跟踪</a-button>
          </a-tooltip>
          <a-tooltip v-else title="运行" placement="top">
            <a-button @click="resume(activeTab)">继续跟踪</a-button>
          </a-tooltip>
          <a-button @click="clearActive">清空显示</a-button>
          <a-checkbox v-model:checked="autoScroll">自动滚动</a-checkbox>
        </a-space>
        <div>
          <div class="tail-section-title">源文件</div>
          <div class="tail-path">{{ activeTab.source.log_path }}</div>
        </div>
        <a-alert type="info" show-icon message="显示文件现在的输出，不从历史记录回放；查询条件不适用。断线不自动重连；手动继续会重新读取文件末尾，可能有缺口或重复。" />
        <div v-if="activeTab.notice" class="tail-notice">{{ activeTab.notice }}</div>
        <div v-if="bufferState.evicted" class="tail-notice">页面缓存已淘汰较早内容，仅保留最近 5000 行 / 5 MiB（不是传输缺口）。</div>
        <div v-if="bufferState.truncated" class="tail-notice">超长行显示已截断为最多 64 KiB，余下内容不显示。</div>
      </aside>
      <section class="tail-output" aria-label="实时日志内容">
        <div class="tail-output-head">
          <span class="tail-section-title">实时内容</span>
          <div class="tail-output-actions">
            <a-button @click="copyActive">复制</a-button>
            <a-tooltip title="下载日志" placement="top">
              <a-button @click="downloadActive">下载</a-button>
            </a-tooltip>
          </div>
        </div>
        <div ref="viewport" class="tail-viewport" @scroll="scrollTop = $event.target.scrollTop">
          <div :style="{ height: `${bufferState.lines.length * TAIL_ROW_HEIGHT}px`, position: 'relative' }">
            <div
              v-for="(line, index) in visibleLines"
              :key="firstLine + index"
              class="tail-line"
              :style="{ top: `${(firstLine + index) * TAIL_ROW_HEIGHT}px` }"
            >{{ line || ' ' }}</div>
          </div>
        </div>
      </section>
    </div>
  </a-modal>
</template>

<script setup>
import { computed, markRaw, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { getToken } from '@/api/user'
import { getWebSocketBaseUrl } from '@/util/request'
import { copyTextWithFallback } from '@/util/clipboard'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import { createTailBuffer, liveSourceError, liveSourceFromLog, liveSourceKey, MAX_TAIL_TABS, TAIL_ROW_HEIGHT } from '@/util/liveTail'

const open = ref(false)
const emit = defineEmits(['state-change'])
const tabs = reactive([])
watch(() => tabs.map((tab) => [tab.key, Boolean(tab.socket), tab.status]), () => emit('state-change'))
const activeKey = ref('')
const activeTab = computed(() => tabs.find((tab) => tab.key === activeKey.value))
const viewport = ref(null)
const autoScroll = ref(true)
const scrollTop = ref(0)
const revision = ref(0)
let flushTimer = null
// Drawer focus can remain behind the modal, bypassing its local key handler.
// Capture Escape only while open so it closes all readers, not the drawer.
watch(open, (visible, _, onCleanup) => {
  if (!visible) return
  const onEscape = (event) => {
    if (event.key !== 'Escape') return
    event.preventDefault()
    event.stopPropagation()
    closeAll()
  }
  document.addEventListener('keydown', onEscape, true)
  onCleanup(() => document.removeEventListener('keydown', onEscape, true))
}, { flush: 'sync' })
const bufferState = computed(() => {
  void revision.value
  return activeTab.value ? { ...activeTab.value.buffer.state } : { lines: [], evicted: false, truncated: false }
})
const firstLine = computed(() => Math.max(0, Math.floor(scrollTop.value / TAIL_ROW_HEIGHT) - 5))
const visibleLines = computed(() => bufferState.value.lines.slice(firstLine.value, firstLine.value + 36))

function refresh() {
  if (flushTimer !== null) return
  // Rendering is batched, ingestion is already bounded even in hidden tabs.
  flushTimer = setTimeout(async () => {
    flushTimer = null
    revision.value++
    await nextTick()
    if (autoScroll.value && viewport.value) {
      viewport.value.scrollTop = viewport.value.scrollHeight
      scrollTop.value = viewport.value.scrollTop
    }
  }, 150)
}

function activate(key) {
  activeKey.value = key
  scrollTop.value = 0
  refresh()
}

function stop(tab) {
  const socket = tab.socket
  tab.socket = null
  if (socket) {
    try {
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'stop' }))
    } catch {
      // Stop is best-effort; closing must still release this and all other tabs.
    } finally {
      socket.close()
    }
  }
  tab.status = '已停止'
}

function removeTab(key, action) {
  if (action !== 'remove') return
  const index = tabs.findIndex((tab) => tab.key === key)
  if (index < 0) return
  stop(tabs[index])
  tabs.splice(index, 1)
  if (!tabs.length) closeAll()
  else if (activeKey.value === key) activate(tabs[Math.min(index, tabs.length - 1)].key)
}

function clearActive() {
  activeTab.value?.buffer.clear()
  scrollTop.value = 0
  refresh()
}

async function copyActive() {
  const content = activeTab.value?.buffer.state.lines.join('\n') || ''
  if (!content) {
    message.warning('没有可复制的内容')
    return
  }
  const copied = await copyTextWithFallback(content)
  if (copied) message.success('已复制')
  else message.error('复制失败，请检查浏览器权限')
}

function downloadActive() {
  const tab = activeTab.value
  const content = tab?.buffer.state.lines.join('\n') || ''
  if (!content) {
    message.warning('没有可下载的内容')
    return
  }
  if (!tab.serviceName) {
    message.error('缺少服务名称，无法生成下载文件名，请重新查询后再试')
    return
  }
  let url
  let link
  try {
    const timestamp = formatTimeWithTimezone(new Date(), store.state.user?.timezone || 'Asia/Shanghai', 'YYYYMMDD_HHmmss')
    if (timestamp === '-') throw new Error('无法生成下载时间，请检查用户时区设置')
    const segments = [tab.source.host_ip, tab.serviceName, tab.source.log_name.replace(/\.log$/i, '')]
      .map((value) => String(value).replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_').replace(/[. ]+$/g, ''))
    const filename = `${segments.join('-')}-${timestamp}.log`
    url = window.URL.createObjectURL(new Blob([content], { type: 'text/plain;charset=utf-8' }))
    link = document.createElement('a')
    link.href = url
    link.download = filename
    document.body.appendChild(link)
    link.click()
  } catch (error) {
    message.error(`下载失败：${error.message}`)
  } finally {
    link?.remove()
    if (url) window.URL.revokeObjectURL(url)
  }
}

function closeAll() {
  tabs.forEach(stop)
  tabs.splice(0)
  open.value = false
  if (flushTimer !== null) clearTimeout(flushTimer)
  flushTimer = null
}

function getTabState(log) {
  if (!log?.host_ip || !log?.log_path) return null
  const tab = tabs.find((item) => item.key === liveSourceKey(log))
  return tab ? { tracking: Boolean(tab.socket), status: tab.status } : null
}

function start(log, serviceName = log?.service || '') {
  const key = log ? liveSourceKey(log) : ''
  const existing = tabs.find((tab) => tab.key === key)
  if (existing) {
    open.value = true
    activate(key)
    if (!existing.socket) message.info('此标签已停止，可点击“继续跟踪”重新连接；停止期间可能有缺口或重复。')
    return
  }
  const source = liveSourceFromLog(log)
  if (!source) {
    message.warning(liveSourceError(log))
    return
  }
  if (tabs.length >= MAX_TAIL_TABS) {
    message.warning('最多打开 5 个文件标签，请先关闭一个标签。')
    return
  }
  const tab = reactive({
    key, source, label: `${source.host_ip} · ${source.log_path.split('/').pop()}`,
    serviceName,
    status: '连接中', notice: '', socket: null, buffer: markRaw(createTailBuffer()),
  })
  if (!connect(tab)) return
  tabs.push(tab)
  open.value = true
  activate(key)
}

function connect(tab, continuing = false) {
  let socket
  try {
    socket = markRaw(new WebSocket(`${getWebSocketBaseUrl()}/ws/monitor/log-tail/`))
  } catch {
    message.error('无法建立实时日志连接，请检查 API 地址与浏览器安全策略。')
    return false
  }
  tab.socket = socket
  tab.status = '连接中'
  tab.notice = continuing ? '继续跟踪：重新读取文件当前末尾；停止期间可能有缺口，快照可能与上次重复。' : ''
  if (continuing) tab.buffer.beginSession(tab.notice)
  socket.onopen = () => {
    if (tab.socket !== socket) return socket.close()
    socket.send(JSON.stringify({ token: getToken(), source: tab.source }))
  }
  socket.onmessage = ({ data }) => {
    if (tab.socket !== socket) return
    try {
      const event = JSON.parse(data)
      if (event.type === 'data') tab.buffer.pushBase64(event.data)
      if (event.type === 'ready' || event.type === 'snapshot' || event.type === 'data') tab.status = '跟踪中'
      if (event.type === 'waiting') tab.status = '等待文件'
      if (event.message) tab.notice = event.message
      if (event.type === 'error' || event.type === 'end') {
        stop(tab)
        tab.status = event.type === 'error' ? '异常结束' : '已停止'
      }
      refresh()
    } catch {
      stop(tab)
      tab.status = '异常结束'
      tab.notice = '协议数据无效，已停止；可能有内容缺口。'
    }
  }
  socket.onerror = () => {
    if (tab.socket !== socket) return
    tab.notice = '连接失败：功能可能未启用、Origin 未获准或网络异常；不自动重连。'
  }
  socket.onclose = () => {
    if (tab.socket !== socket) return
    tab.socket = null
    tab.status = '异常结束'
    tab.notice ||= '连接已断开，可能有内容缺口；不自动重连。'
  }
  return true
}

function resume(tab) {
  // Reuse the tab, not the transport: each manual continuation must authenticate
  // again, and callbacks from the previous socket must not affect its successor.
  if (!tab.socket && connect(tab, true)) refresh()
}

// KeepAlive deactivation, cache eviction and logout/navigation all release
// readers; switching file tabs or pausing scroll does not stop ingestion.
useKeepAliveRefreshLifecycle(() => {}, closeAll)
onBeforeUnmount(closeAll)
defineExpose({ start, closeAll, getTabState })
</script>

<style scoped>
.tail-layout { display: grid; grid-template-columns: 280px minmax(0, 1fr); gap: 20px; align-items: start; }
.tail-sidebar { min-width: 0; display: flex; flex-direction: column; gap: 16px; padding-right: 20px; border-right: 1px solid #f0f0f0; }
.tail-status { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.tail-section-title { font-weight: 600; }
.tail-output { min-width: 0; }
.tail-output-head { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 12px; margin-bottom: 8px; }
.tail-output-actions { display: flex; align-items: center; gap: 8px; }
.tail-path { margin-top: 8px; overflow-wrap: anywhere; }
.tail-notice { color: #ad6800; overflow-wrap: anywhere; }
.tail-viewport { height: 520px; overflow: auto; background: #141414; color: #eee; font: 13px/22px monospace; }
.tail-line { position: absolute; white-space: pre; min-width: 100%; height: 22px; padding: 0 8px; }
@media (max-width: 900px) {
  .tail-layout { grid-template-columns: minmax(0, 1fr); }
  .tail-sidebar { padding-right: 0; padding-bottom: 16px; border-right: 0; border-bottom: 1px solid #f0f0f0; }
}
</style>
