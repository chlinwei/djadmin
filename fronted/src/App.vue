<template>
  <RouterView />
</template>
<script setup>
import {  RouterView, useRoute } from 'vue-router';
import {watch} from 'vue';
const route = useRoute();
import store from '@/store'
watch(route,(to,from,next) => {
    if(to.name == '个人中心') {
        let obj = {
            title: to.name,
            key: to.path
        }
        console.log("==========")
        console.log(obj);
        store.commit('add_tab',obj)
        // next(false);
    }
}, {deep: true, immediate: true})
</script>

<style>
html,body,#app {
  height: 100%;
}
</style>
