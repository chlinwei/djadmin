export function createWebsshSessionController(options) {
    const {
        nextTick,
        Terminal,
        FitAddon,
        terminalRef,
        state,
        getHostId,
        statusText,
        messageText,
        messageType,
        currentLogId,
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
    } = options

    const disposeTerminal = () => {
        if (state.onDataDisposable) {
            state.onDataDisposable.dispose()
            state.onDataDisposable = null
        }
        if (state.onResizeDisposable) {
            state.onResizeDisposable.dispose()
            state.onResizeDisposable = null
        }
        if (state.term) {
            state.term.dispose()
            state.term = null
        }
        state.fitAddon = null
    }

    const startWebsshHeartbeat = () => {
        if (state.websshHeartbeatTimer) return
        state.websshHeartbeatTimer = window.setInterval(() => {
            if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
                return
            }
            try {
                state.socket.send(JSON.stringify({ type: 'ping' }))
            } catch (error) {
                // ignore heartbeat send failure
            }
        }, 20000)
    }

    const stopWebsshHeartbeat = () => {
        if (!state.websshHeartbeatTimer) return
        window.clearInterval(state.websshHeartbeatTimer)
        state.websshHeartbeatTimer = null
    }

    const sendWebsshTransferActivity = () => {
        if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
            return
        }
        try {
            state.socket.send(JSON.stringify({ type: 'transfer_activity' }))
        } catch (error) {
            // ignore activity send failure
        }
    }

    const syncWebsshTransferActivityTimer = () => {
        const hasActiveTransfer = downloadRunning.value || uploadRunning.value
        if (!hasActiveTransfer) {
            if (state.websshTransferActivityTimer) {
                window.clearInterval(state.websshTransferActivityTimer)
                state.websshTransferActivityTimer = null
            }
            return
        }

        sendWebsshTransferActivity()
        if (state.websshTransferActivityTimer) {
            return
        }
        state.websshTransferActivityTimer = window.setInterval(() => {
            sendWebsshTransferActivity()
        }, 20000)
    }

    const closeSocket = () => {
        if (state.websshHeartbeatTimer) {
            window.clearInterval(state.websshHeartbeatTimer)
            state.websshHeartbeatTimer = null
        }
        if (state.websshTransferActivityTimer) {
            window.clearInterval(state.websshTransferActivityTimer)
            state.websshTransferActivityTimer = null
        }
        if (!state.socket) return
        try {
            if (state.socket.readyState === WebSocket.OPEN) {
                state.socket.send(JSON.stringify({ type: 'close' }))
            }
            state.socket.close()
        } catch (error) {
            // ignore close exception
        }
        state.socket = null
    }

    const initTerminal = async () => {
        await nextTick()
        if (!terminalRef.value) return

        disposeTerminal()
        state.term = new Terminal({
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

        state.fitAddon = new FitAddon()
        state.term.loadAddon(state.fitAddon)
        terminalRef.value.innerHTML = ''
        state.term.open(terminalRef.value)
        state.fitAddon.fit()
        state.term.focus()

        state.onDataDisposable = state.term.onData((data) => {
            sendTerminalInput(data)
        })

        state.onResizeDisposable = state.term.onResize(({ cols, rows }) => {
            if (state.socket && state.socket.readyState === WebSocket.OPEN) {
                state.socket.send(JSON.stringify({ type: 'resize', cols, rows }))
            }
        })
    }

    const connectWebSsh = async () => {
        if (!getHostId()) {
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

            state.socket = new WebSocket(buildWebSocketUrl())

            state.socket.onopen = () => {
                statusText.value = '连接中'
                messageText.value = 'WebSocket 已建立，等待 SSH 会话连接...'
                messageType.value = 'info'
                startWebsshHeartbeat()
                syncWebsshTransferActivityTimer()

                if (state.term) {
                    state.term.focus()
                    state.socket.send(JSON.stringify({ type: 'resize', cols: state.term.cols || 120, rows: state.term.rows || 32 }))
                    sendTerminalInput('\r')
                }
            }

            state.socket.onmessage = (event) => {
                try {
                    const msg = JSON.parse(event.data)
                    if (msg.type === 'output') {
                        state.term?.write(msg.data)
                    } else if (msg.type === 'connected') {
                        statusText.value = '已连接'
                        messageText.value = ''
                        currentLogId.value = msg.log_id || null
                        const homeDir = msg.home_dir || '/root'
                        const instanceName = String(msg.instance_name || '').trim() || 'unknown'
                        fileCurrentPath.value = homeDir
                        filePathInput.value = homeDir
                        fetchActiveUserCount()
                        if (!fileEntries.value.length) {
                            loadFiles(fileCurrentPath.value)
                        }
                        writeSystemLine(`Connected to ${instanceName} (${msg.ip})`)
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
                        if (state.socket && state.socket.readyState === WebSocket.OPEN) {
                            state.socket.close()
                        }
                    } else if (msg.type === 'pong') {
                        // heartbeat ack
                    }
                } catch (error) {
                    state.term?.write(event.data)
                }
            }

            state.socket.onerror = () => {
                stopWebsshHeartbeat()
                syncWebsshTransferActivityTimer()
                statusText.value = '连接失败'
                messageText.value = 'WebSocket 连接异常，请确认后端使用 ASGI/Daphne 方式启动并监听 9000 端口'
                messageType.value = 'error'
            }

            state.socket.onclose = () => {
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

    return {
        getFitAddon: () => state.fitAddon,
        getSocket: () => state.socket,
        getTerm: () => state.term,
        disposeTerminal,
        closeSocket,
        startWebsshHeartbeat,
        stopWebsshHeartbeat,
        sendWebsshTransferActivity,
        syncWebsshTransferActivityTimer,
        initTerminal,
        connectWebSsh,
    }
}
