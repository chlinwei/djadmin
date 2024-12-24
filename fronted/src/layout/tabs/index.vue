<template>
<div>
    <a-tabs v-model:activeKey="activeKey" type="editable-card" @edit="onEdit" :hideAdd="true">
    <a-tab-pane v-for="pane in panes" :key="pane.key" :tab="pane.title" :closable="pane.closable" @click="remove_tab(pane)">
        <!-- <router-view></router-view> -->
    </a-tab-pane>
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
    console.log("activekey new:" + New)
    console.log("activekey old:" + Old)

    router.replace(New);
})
const panes = computed(()=>{
    return store.state.tabs;
});

const onEdit = (targetKey, action) => {
    if(action == "remove") {
      store.commit('remove_tab',targetKey)
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
console.log(store.state.tabs)
console.log(store.state.activeKey)
</script>
<style>

</style>