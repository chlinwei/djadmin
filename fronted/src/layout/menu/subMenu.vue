<template>
    <a-menu-item v-if="!menu.children" :key="menu.path" @click="add_tab(menu)">{{ menu.name }}</a-menu-item>
    <a-sub-menu v-else :key="menu.path">
        <template #title>
            <SvgIcon :name="menu.icon"></SvgIcon>
            &nbsp; <span style="vertical-align: middle;">{{ menu.name }}</span>
        </template>
        <subMenu v-for="children in menu.children" :menu="children" />
    </a-sub-menu>
</template>
<style scoped></style>
<script setup>
import store from '@/store/index.js';
import router from '@/router';
import { defineProps } from 'vue';
const props = defineProps(
    {
        menu: {
            type: Object,
            default: null
        }
    }
)
const add_tab = (item) => {
    let tab = {
        title: item.name,
        key: item.path
    }
    store.commit('add_tab', tab);
    let all_routes = router.getRoutes()
     all_routes.forEach(e => {
      if(e.path == item.path) {
        e.meta.cached = true;
        console.log(e)
      }
     })
}
</script>