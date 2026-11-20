<template>
    <div class="header-user-wrap">
        <div class="current-time-badge" title="当前时间（按用户时区显示）">
            <div class="current-time-date">{{ currentDate }}</div>
            <div class="current-time-weekday">{{ currentWeekday }}</div>
            <div class="current-time-clock">{{ currentClock }}</div>
        </div>
        <div class="current-time-timezone">{{ currentTimezoneLabel }}</div>

        <a-dropdown
            v-model:open="userMenuOpen"
            placement="bottomRight"
            :align="{ offset: [0, 0] }"
            :mouse-leave-delay="0.25"
        >
            <a class="ant-dropdown-link" @click.prevent>
                <a-avatar :size="28" :src="avatarUrl || undefined" class="header-avatar">
                    <template #icon>
                        <SvgIcon name="user" />
                    </template>
                </a-avatar>
                {{ currentUser?.username || '用户' }}
            </a>
            <template #overlay>
                <a-menu class="header-user-menu" :selectable="false" @click="userMenuOpen = false">
                    <a-menu-item key="profile">
                        <router-link class="header-user-menu-entry" :to="{ name: '个人中心' }">
                            <UserOutlined aria-hidden="true" />
                            <span>个人中心</span>
                        </router-link>
                    </a-menu-item>
                    <a-menu-divider />
                    <a-menu-item key="logout" danger @click="logout">
                        <span class="header-user-menu-entry">
                            <LogoutOutlined aria-hidden="true" />
                            <span>安全退出</span>
                        </span>
                    </a-menu-item>
                </a-menu>
            </template>
        </a-dropdown>
    </div>
</template>

<script setup>
import { LogoutOutlined, UserOutlined } from '@ant-design/icons-vue'
import { getCurrentUser } from '@/api/user'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import { getMediaUrl } from '@/util/request'
import router from '@/router'
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { formatTimeWithTimezone } from '@/util/timezone'
import { listenUserTimezoneChanged } from '@/util/userTimezoneSync'
import store from '@/store'

const cachedUser = getCurrentUser() || {}
// 头像上传已通过 saveCurrentUser 更新 store；顶栏订阅同一状态，缓存仅用于刷新后的初始回显。
const currentUser = computed(() => store.state.user || cachedUser)
const avatarUrl = computed(() => {
    const raw = currentUser.value?.avatar || currentUser.value?.user?.avatar || ''
    return raw ? getMediaUrl(raw) : ''
})
const currentTime = ref('')
// Control visibility so clicks on popup padding don't act like menu selections.
const userMenuOpen = ref(false)
const userTimezone = ref(currentUser.value?.timezone || 'UTC')
let timeInterval = null
let stopListenTimezone = null

const currentDate = computed(() => currentTime.value.split(' ')[0] || '--')
const currentClock = computed(() => currentTime.value.split(' ')[1] || '--:--:--')
const currentTimezoneLabel = computed(() => `时区: ${userTimezone.value}`)
const currentWeekday = computed(() => {
    try {
        return new Intl.DateTimeFormat('zh-CN', {
            weekday: 'long',
            timeZone: userTimezone.value,
        }).format(new Date())
    } catch (error) {
        return '--'
    }
})

const formatTime = () => {
    try {
        return formatTimeWithTimezone(new Date(), userTimezone.value, 'YYYY-MM-DD HH:mm:ss')
    } catch (error) {
        const now = new Date()
        const year = now.getFullYear()
        const month = String(now.getMonth() + 1).padStart(2, '0')
        const day = String(now.getDate()).padStart(2, '0')
        const hours = String(now.getHours()).padStart(2, '0')
        const minutes = String(now.getMinutes()).padStart(2, '0')
        const seconds = String(now.getSeconds()).padStart(2, '0')
        return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
    }
}

onMounted(() => {
    stopListenTimezone = listenUserTimezoneChanged((timezone) => {
        userTimezone.value = timezone
        currentTime.value = formatTime()
    })

    try {
        getCurrentUserInfo()
            .then((res) => {
                if (res.data && res.data.data && res.data.data.timezone) {
                    userTimezone.value = res.data.data.timezone
                }
            })
            .catch((error) => {
                console.error('获取用户时区失败:', error)
                userTimezone.value = 'UTC'
            })
    } catch (error) {
        console.error('API 调用异常:', error)
        userTimezone.value = 'UTC'
    }

    currentTime.value = formatTime()
    timeInterval = setInterval(() => {
        currentTime.value = formatTime()
    }, 1000)
})

onBeforeUnmount(() => {
    if (stopListenTimezone) {
        stopListenTimezone()
    }
    if (timeInterval) {
        clearInterval(timeInterval)
    }
})

const logout = () => {
    window.sessionStorage.clear()
    window.localStorage.clear()
    router.replace('/login')
}
</script>

<style scoped>
.header-user-menu {
    min-width: 144px;
}

.header-user-menu-entry {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 28px;
    white-space: nowrap;
}

.header-user-menu-entry .anticon {
    flex-shrink: 0;
    font-size: 15px;
}

.header-avatar {
    margin-right: 8px;
    vertical-align: middle;
}

.header-user-wrap {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: flex-end;
    padding-right: 8px;
    white-space: nowrap;
}

.current-time-badge {
    display: inline-flex;
    flex-direction: column;
    align-items: flex-end;
    justify-content: center;
    min-width: 180px;
    padding: 6px 10px;
    border-radius: 10px;
    border: 1px solid #d9e6f6;
    background: linear-gradient(135deg, #f8fbff 0%, #eef5ff 100%);
    box-shadow: 0 2px 8px rgba(15, 23, 42, 0.06);
}

.current-time-date {
    font-size: 12px;
    color: #4b5563;
    line-height: 1.2;
}

.current-time-weekday {
    margin-top: 2px;
    font-size: 12px;
    color: #64748b;
    line-height: 1.2;
}

.current-time-clock {
    margin-top: 2px;
    font-size: 17px;
    font-weight: 700;
    color: #0f172a;
    line-height: 1.2;
    letter-spacing: 0.4px;
    font-family: 'Courier New', monospace;
}

.current-time-timezone {
    margin-top: 2px;
    font-size: 11px;
    color: #475569;
    line-height: 1.2;
}
</style>