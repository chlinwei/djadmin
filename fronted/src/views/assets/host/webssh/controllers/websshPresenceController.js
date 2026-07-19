export function createWebsshPresenceController(options) {
    const {
        getHostId,
        getHostById,
        state,
        activeSessionsVisible,
        activeSessionsLoading,
        activeSessions,
        activeUserCount,
        getHostWebSshActiveCount,
        getHostWebSshActiveSessions,
        message,
        onHostOffline,
    } = options

    let hostOfflineNotified = false

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

    const checkHostOnlineStatus = async () => {
        if (!getHostId() || typeof getHostById !== 'function') return
        try {
            const res = await getHostById(getHostId())
            if (res?.data?.code !== 200) {
                return
            }
            const host = res?.data?.data || {}
            const isOnline = Boolean(host?.system?.agent_online)
            if (isOnline) {
                hostOfflineNotified = false
                return
            }

            if (!hostOfflineNotified && typeof onHostOffline === 'function') {
                hostOfflineNotified = true
                onHostOffline(host)
            }
        } catch (error) {
            // keep previous state when fetch fails
        }
    }

    const pollPresenceAndHostStatus = () => {
        void fetchActiveUserCount()
        void checkHostOnlineStatus()
    }

    const startActiveUserPolling = () => {
        stopActiveUserPolling()
        pollPresenceAndHostStatus()
        state.activeCountTimer = window.setInterval(pollPresenceAndHostStatus, 5000)
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
