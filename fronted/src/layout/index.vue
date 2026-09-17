<template>
    <a-layout style="min-height: 100vh">
        <a-layout-sider v-model:collapsed="collapsed" collapsible>
            <!-- 左边menu -->
            <Menu />
        </a-layout-sider>
        <a-layout>
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

// 关闭当前标签、关闭全部标签时 store 会先于路由跳转更新，若此时 include 立刻剔除当前
// 正在挂载的组件名，KeepAlive.pruneCache 会去卸载挂载中的实例，Vue 在移除 DOM 时读到
// null 的 parentNode 而抛错。把当前路由的缓存名始终并入 include，等路由真正切换后再释放。
const currentRouteCacheName = computed(() => cacheNameOfMatched(route.matched))

let cachedIncludes = []
const tab_includes = computed(()=>{
    const names = []
    const currentName = currentRouteCacheName.value
    if (currentName) names.push(currentName)
    store.state.tabs.forEach((tab) => {
        const name = resolveTabCacheName(tab.key)
        if (name && !names.includes(name)) names.push(name)
    })
    // include 的引用一变就会触发 KeepAlive 的 pruneCache 并卸载缓存实例；
    // 内容没变时必须复用同一个数组引用，否则每次路由跳转都会重跑一遍卸载流程。
    const unchanged = names.length === cachedIncludes.length
        && names.every((name, index) => name === cachedIncludes[index])
    if (!unchanged) cachedIncludes = names
    return cachedIncludes
})


const collapsed = ref(false)
</script>


<style scoped>
</style>