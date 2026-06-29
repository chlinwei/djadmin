<template>

    <div style="display: flex; gap: 24px; align-items: center;">
        <span class="current-time">{{ currentTime }}</span>
        
        <a-dropdown>
            <a class="ant-dropdown-link" @click.prevent>
                {{ currentUser?.username || '用户' }}
            </a>
            <template #overlay>
                <a-menu>
                    <a-menu-item >
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
import { useRouter } from 'vue-router';
import { getCurrentUser } from '@/api/user';
import { getCurrentUserInfo } from '@/api/sys/userTimezone'
import router from '@/router'
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { formatTimeWithTimezone } from '@/util/timezone'

console.log("avatar:")
console.log(useRouter().currentRoute.value.fullPath)
const currentUser = ref(getCurrentUser() || {})
const currentTime = ref('')
const userTimezone = ref('UTC')
let timeInterval = null

const formatTime = () => {
    try {
        const now = new Date()
        // 使用用户选择的时区格式化时间
        return formatTimeWithTimezone(now, userTimezone.value, 'YYYY-MM-DD HH:mm:ss')
    } catch (error) {
        // 如果时区格式化失败，使用本地时间
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
    // 从API加载当前用户的时区，用于时间显示
    try {
        getCurrentUserInfo().then(res => {
            if (res.data && res.data.data && res.data.data.timezone) {
                userTimezone.value = res.data.data.timezone
            }
        }).catch(error => {
            console.error('获取用户时区失败:', error)
            // 降级：使用默认时区
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
    if (timeInterval) {
        clearInterval(timeInterval)
    }
})

const logout = () => {
    window.sessionStorage.clear()
    window.localStorage.clear()
    router.replace("/login")
}

</script>
<style scoped>
.hoverColoir:hover {
    background-color: #1677ff;
    color: #fff !important;
}

.current-time {
    font-size: 12px;
    color: #666;
    font-family: 'Courier New', monospace;
    white-space: nowrap;
}
</style>