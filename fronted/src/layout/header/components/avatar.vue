<template>
    <div class="header-user-wrap">
        <div class="current-time-badge" title="当前时间（按用户时区显示）">
            <div class="current-time-date">{{ currentDate }}</div>
            <div class="current-time-weekday">{{ currentWeekday }}</div>
            <div class="current-time-clock">{{ currentClock }}</div>
        </div>

        <a-dropdown>
            <a class="ant-dropdown-link" @click.prevent>
                {{ currentUser?.username || '用户' }}
            </a>
            <template #overlay>
                <a-menu>
                    <a-menu-item>
                        <router-link class="hoverColoir" :to="{ name: '个人中心' }">个人中心</router-link>
                    </a-menu-item>
                    <a-menu-item>
                        <a class="hoverColoir" href="javascript:;" @click="logout">安全退出</a>
                    </a-menu-item>
                </a-menu>
            </template>
        </a-dropdown>
    </div>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { getCurrentUser } from '@/api/user'
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import router from '@/router'
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { formatTimeWithTimezone } from '@/util/timezone'
import { listenUserTimezoneChanged } from '@/util/userTimezoneSync'

console.log('avatar:')
console.log(useRouter().currentRoute.value.fullPath)

const currentUser = ref(getCurrentUser() || {})
const currentTime = ref('')
const userTimezone = ref(currentUser.value?.timezone || 'UTC')
let timeInterval = null
let stopListenTimezone = null

const currentDate = computed(() => currentTime.value.split(' ')[0] || '--')
const currentClock = computed(() => currentTime.value.split(' ')[1] || '--:--:--')
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
        const now = new Date()
        return formatTimeWithTimezone(now, userTimezone.value, 'YYYY-MM-DD HH:mm:ss')
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
.hoverColoir:hover {
    background-color: #1677ff;
    color: #fff !important;
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
</style>