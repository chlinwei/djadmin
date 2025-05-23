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
import router from '@/router/index.js';
import {computed} from 'vue';



const tab_includes = computed(()=>{
    var tmp_tab_includes = []
    store.state.tabs.forEach(element => {
        console.log(element.key)
        router.getRoutes().forEach(e2 => {
            if(e2.path == element.key) {
                if(e2.components) {
                    if(e2.components.default.name) {
                        tmp_tab_includes.push(e2.components.default.name)
                    }
                }
            }
        })

    });
    return tmp_tab_includes;
})


const collapsed = ref(false)
</script>


<style scoped>
</style>