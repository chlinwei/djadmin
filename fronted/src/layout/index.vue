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
               <router-view></router-view>
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


const collapsed = ref(false)
</script>


<style scoped>
</style>