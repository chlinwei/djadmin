export function createWebsshPresenceController(options) {
    const {
        getHostId,
        state,
        activeSessionsVisible,
        activeSessionsLoading,
        activeSessions,
        activeUserCount,
        getHostWebSshActiveCount,
        getHostWebSshActiveSessions,
        message,
    } = options

    const fetchActiveUserCount = async () => {
        if (!getHostId()) return
        try {
            const res = await getHostWebSshActiveCount(getHostId())
            if (res?.data?.code === 200) {
                activeUserCount.value = Number(res.data?.data?.active_count || 0)
            }
        } catch (error) {
            // keep last value when fetch fails
        }
    }

    const stopActiveUserPolling = () => {
        if (state.activeCountTimer) {
            window.clearInterval(state.activeCountTimer)
            state.activeCountTimer = null
        }
    }

    const startActiveUserPolling = () => {
        stopActiveUserPolling()
        fetchActiveUserCount()
        state.activeCountTimer = window.setInterval(fetchActiveUserCount, 5000)
    }

    const fetchActiveSessions = async (showError = false) => {
        if (!getHostId()) return
        activeSessionsLoading.value = true
        try {
            const res = await getHostWebSshActiveSessions(getHostId())
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

    const stopActiveSessionsPolling = () => {
        if (state.activeSessionsTimer) {
            window.clearInterval(state.activeSessionsTimer)
            state.activeSessionsTimer = null
        }
    }

    const startActiveSessionsPolling = () => {
        stopActiveSessionsPolling()
        fetchActiveSessions()
        state.activeSessionsTimer = window.setInterval(fetchActiveSessions, 5000)
    }

    const openActiveSessionsModal = () => {
        activeSessionsVisible.value = true
        startActiveSessionsPolling()
    }

    const closeActiveSessionsModal = () => {
        activeSessionsVisible.value = false
        stopActiveSessionsPolling()
    }

    return {
        fetchActiveUserCount,
        startActiveUserPolling,
        stopActiveUserPolling,
        fetchActiveSessions,
        startActiveSessionsPolling,
        stopActiveSessionsPolling,
        openActiveSessionsModal,
        closeActiveSessionsModal,
    }
}
