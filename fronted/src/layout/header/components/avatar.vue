<template>



    <a-dropdown>
        <a class="ant-dropdown-link" @click.prevent>
            {{ currentUser.username }}
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

</template>

<script setup>
import { useRouter } from 'vue-router';
import { getCurrentUser } from '@/api/user';
import router from '@/router'

console.log("avatar:")
console.log(useRouter().currentRoute.value.fullPath)
const currentUser = getCurrentUser()
const logout = () => {
    window.sessionStorage.clear()
    router.replace("/login")
}

</script>
<style scoped>
.hoverColoir:hover {
    background-color: #1677ff;
    color: #fff !important;
}



</style>