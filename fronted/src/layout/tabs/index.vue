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
import {ref} from 'vue'
import store from '@/store/index.js';
import {watch} from 'vue';
import { computed } from 'vue';




const activeKey = computed({
  get:()=>{
    return store.state.activeKey;
  },
  set:(val) => {
    store.state.activeKey = val;
  }
})

watch(activeKey,(New,Old)=>{
    router.replace(New);
})
const panes = computed(()=>{
    return store.state.tabs;
});



// const route = useRoute();
// watch(route,(to,from) => {
//     if(to.name == '个人中心') {
//         let obj = {
//             title: to.name,
//             key: to.path
//         }
//         console.log("==========")
//         console.log(obj);
//         store.commit('add_tab',obj)
        
//     }
// })
// , {deep: true, immediate: true}
const onEdit = (targetKey, action) => {
    if(action == "remove") {
      store.commit('remove_tab',targetKey)
    }
};
console.log(store.state.tabs)
</script>
<style>

</style>