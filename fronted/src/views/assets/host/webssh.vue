<template>
    <div class="webssh-page">
        <div class="webssh-header">
            <div class="header-left">
                <div class="title">Web SSH - {{ hostTitle }}</div>
                <div class="session-id">会话ID：{{ currentSessionId || '-' }}</div>
            </div>
            <a-space>
                <a-tag :color="statusColor">{{ statusText }}</a-tag>
                <a-button size="small" @click="reconnect">重连</a-button>
                <a-button size="small" type="primary" @click="closeTab">关闭</a-button>
            </a-space>
        </div>

        <a-alert
            v-if="messageText"
            :message="messageText"
            :type="messageType"
            show-icon
            style="margin-bottom: 10px"
        />

        <div ref="terminalRef" class="terminal-wrapper" />
    </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { getHostById } from '@/api/assets/host/index.js'
import { getToken } from '@/api/user/index.js'

const route = useRoute()
const terminalRef = ref(null)
const messageText = ref('')
const messageType = ref('info')
const statusText = ref('未连接')
const currentSessionId = ref('')

let hostId = null
let hostName = ''
let hostIp = ''
let socket = null
let term = null
let fitAddon = null
let onDataDisposable = null
let onResizeDisposable = null

const handlePageUnload = () => {
    closeSocket()
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

const buildWebSocketUrl = () => {
    const token = getToken() || ''
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const wsHost = `${window.location.hostname}:8000`
    return `${protocol}://${wsHost}/ws/assets/hosts/${hostId}/webssh/?token=${encodeURIComponent(token)}`
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

const initTerminal = async () => {
    await nextTick()
    if (!terminalRef.value) return

    disposeTerminal()
    term = new Terminal({
        cursorBlink: true,
        fontFamily: 'Consolas, Menlo, monospace',
        fontSize: 13,
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
        if (socket && socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'input', data }))
        }
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

    closeSocket()
    await initTerminal()

    statusText.value = '连接中'
    currentSessionId.value = ''
    messageText.value = '正在建立 SSH 连接...'
    messageType.value = 'info'
    term?.writeln('Connecting...')

    socket = new WebSocket(buildWebSocketUrl())

    socket.onopen = () => {
        statusText.value = '连接中'
        messageText.value = 'WebSocket 已建立，等待 SSH 会话连接...'
        messageType.value = 'info'

        if (term) {
            term.focus()
            socket.send(JSON.stringify({ type: 'resize', cols: term.cols || 120, rows: term.rows || 32 }))
            socket.send(JSON.stringify({ type: 'input', data: '\r' }))
        }
    }

    socket.onmessage = (event) => {
        try {
            const msg = JSON.parse(event.data)
            if (msg.type === 'output') {
                term?.write(msg.data)
            } else if (msg.type === 'connected') {
                statusText.value = '已连接'
                messageText.value = '连接已建立'
                messageType.value = 'success'
                currentSessionId.value = msg.session_id || ''
                term?.writeln(`Connected to ${msg.host_name} (${msg.ip})`)
                if (currentSessionId.value) {
                    term?.writeln(`Session ID: ${currentSessionId.value}`)
                }
            } else if (msg.type === 'error') {
                statusText.value = '连接失败'
                messageText.value = msg.message || '连接失败'
                messageType.value = 'error'
                term?.writeln(`\r\n[ERROR] ${msg.message || '连接失败'}`)
            } else if (msg.type === 'closed') {
                statusText.value = '未连接'
                messageText.value = msg.message || '会话已关闭'
                messageType.value = 'warning'
                term?.writeln(`\r\n[CLOSED] ${msg.message || '会话已关闭'}`)
                if (socket && socket.readyState === WebSocket.OPEN) {
                    socket.close()
                }
            }
        } catch (error) {
            term?.write(event.data)
        }
    }

    socket.onerror = () => {
        statusText.value = '连接失败'
        messageText.value = 'WebSocket 连接异常，请确认后端使用 ASGI/Daphne 方式启动并监听 8000 端口'
        messageType.value = 'error'
    }

    socket.onclose = () => {
        if (statusText.value !== '连接失败') {
            statusText.value = '未连接'
        }
        if (messageType.value === 'success') {
            messageText.value = 'SSH 会话已断开'
            messageType.value = 'warning'
        }
    }
}

const reconnect = () => {
    connectWebSsh()
}

const closeTab = () => {
    window.close()
}

onMounted(async () => {
    hostId = Number(route.query.host_id || 0)
    hostName = String(route.query.instance_name || '')
    hostIp = String(route.query.ip || '')

    window.addEventListener('pagehide', handlePageUnload)
    window.addEventListener('beforeunload', handlePageUnload)

    if (hostId) {
        try {
            const res = await getHostById(hostId)
            if (res.data?.code === 200 && res.data?.data) {
                const host = res.data.data
                hostName = host.instance_name || host.name || hostName
                hostIp = host.ip || hostIp
            }
        } catch (error) {
            // fallback to query display
        }
    }

    await connectWebSsh()
})

onBeforeUnmount(() => {
    window.removeEventListener('pagehide', handlePageUnload)
    window.removeEventListener('beforeunload', handlePageUnload)
    closeSocket()
    disposeTerminal()
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

.session-id {
    font-size: 12px;
    color: #64748b;
    font-family: Consolas, Menlo, monospace;
}

.terminal-wrapper {
    flex: 1;
    min-height: 0;
    border: 1px solid #1f2937;
    border-radius: 8px;
    overflow: hidden;
    background: #0b1220;
}
</style>
