<template>
    <a-layout class="app-layout" style="min-height: 100vh">
        <a-layout-sider v-model:collapsed="collapsed" collapsible>
            <!-- 左边menu -->
            <Menu />
        </a-layout-sider>
        <a-layout class="app-main" :style="{ marginLeft: collapsed ? '80px' : '200px' }">
            <!-- header -->
            <a-layout-header style="background: #fff; padding: 0">
                <Header />
            </a-layout-header>
            <!-- content -->
            <a-layout-content style="margin: 0 16px">
               <Tabs />

                <router-view v-slot="{Component,route}" >
                    <keep-alive :include="tab_includes">
                          <component :is="Component" :key="$route.path" />
                    </keep-alive>
               </router-view>


            </a-layout-content>

            <!-- footer -->
            <a-layout-footer style="text-align: center">
                <Footer />
            </a-layout-footer>
        </a-layout>

    </a-layout>
</template>
<script setup>
import Menu from "@/layout/menu/index.vue"
import Header from "@/layout/header/index.vue"
import Footer from "@/layout/footer/index.vue"
import Tabs from "@/layout/tabs/index.vue"
import {ref} from 'vue'
import store from '@/store/index.js';
import router, {keepAliveNameOfRecord} from '@/router/index.js';
import {computed} from 'vue';
import {useRoute} from 'vue-router';

const route = useRoute();

function cacheNameOfMatched(matched) {
    const record = matched && matched.length ? matched[matched.length - 1] : null
    return keepAliveNameOfRecord(record)
}

// tab.key 存的是 fullPath（可能带 query），路由表里只有 path，必须先解析成匹配到的路由记录再取缓存名。
function resolveTabCacheName(tabKey) {
    const path = String(tabKey || '').split(/[?#]/)[0]
    if (!path) return ''
    try {
        return cacheNameOfMatched(router.resolve(path).matched)
    } catch {
        return ''
    }
}

// 标签先关闭、路由后切换；在导航完成前保留当前页的缓存资格，切换后再释放。
const currentRouteCacheName = computed(() => cacheNameOfMatched(route.matched))

const tab_includes = computed(()=>{
    const names = []
    const currentName = currentRouteCacheName.value
    if (currentName) names.push(currentName)
    store.state.tabs.forEach((tab) => {
        const name = resolveTabCacheName(tab.key)
        if (name && !names.includes(name)) names.push(name)
    })
    return names
})


const collapsed = ref(false)
</script>


<style scoped>
/* 页面内容可能溢出布局容器，sticky 会受容器底部限制而被带离视口。
   固定侧栏并由主区域让出宽度，仅菜单内部滚动；保留底部按钮占位。 */
.app-layout :deep(.ant-layout-sider) {
    position: fixed;
    top: 0;
    left: 0;
    z-index: 100;
    height: 100vh;
    overflow: hidden;
}

.app-main {
    transition: margin-left 0.2s;
}

.app-layout :deep(.ant-layout-sider-children) {
    overflow-y: auto;
    overflow-x: hidden;
}
</style>