<template>
<div>
    <a-tabs v-model:activeKey="activeKey" type="editable-card" @edit="onEdit" :hideAdd="true">
    <a-tab-pane v-for="pane in panes" :key="pane.key" :tab="pane.title" :closable="pane.key == '/index' ? false:true" @click="remove_tab(pane)"></a-tab-pane>
</a-tabs>
</div>
</template>
<script setup>

import router from '@/router'
import store from '@/store/index.js';
import {watch} from 'vue';
import { computed } from 'vue';

import {useRouter} from 'vue-router'



const activeKey = computed({
  get:()=>{
    return store.state.activeKey;
  },
  set:(val) => {
    store.state.activeKey = val;
  }
})

watch(activeKey,(New,Old)=>{
    router.push(New);
})

import {useRoute} from 'vue-router'
const route = useRoute();

// 如果点击返回
watch(route,(New) => {
    let tab = {
        title: New.name,
        key: New.path
    }
    store.commit('add_tab',tab);
})


const panes = computed(()=>{
    return store.state.tabs;
});

const onEdit = (targetKey, action) => {
  console.log("on edit")
     let all_routes = router.getRoutes()
     all_routes.forEach(e => {
      if(e.path == targetKey) {
        e.meta.cached = false;
        console.log(e)
      }
     })
    if(action == "remove") {
    store.commit('remove_tab',targetKey)
    console.log("================remove==============")
      if(store.state.tabs.length==0) {
        store.commit("add_tab", {
            title:'首页',
            key: '/index'
        })
      }
     
    }
};
function reset_tabs() {
    let tab = {
        'title': useRouter().currentRoute.value.name,
        'key':  useRouter().currentRoute.value.fullPath
    }
    store.commit('reset_tab',tab)
}
reset_tabs()
</script>
<style>

</style>