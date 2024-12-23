<template>
<div>
    <a-tabs v-model:activeKey="activeKey" type="editable-card" @edit="onEdit" :hideAdd="true">
     
    <a-tab-pane v-for="pane in panes" :key="pane.key" :tab="pane.title" :closable="pane.closable" @click="remove_tab(pane)">
        <component :is="com.default.value"> 
     </component>
    </a-tab-pane>
    
</a-tabs>
</div>
</template>
<script setup>
import {getMenuList} from '@/api/user/index.js';
import { useRouter } from 'vue-router';
import {ref} from 'vue'
import store from '@/store/index.js';

import { computed } from 'vue';
import router from '@/router';
console.log("===========")

const routes = useRouter().getRoutes()
function get11(routes,path){
  let tmp = ""
  for( var index in routes) {
    if(routes[index].path === path) {
        tmp = routes[index].components
        break
    }
  }
  return tmp

}
const com = ref(get11(routes,"/sys/user"));

const activeKey = computed({
  get:()=>{
    return store.state.activeKey;
  },
  set:(val) => {
    store.state.activeKey = val;
  }
})
const panes = computed(()=>{
    return store.state.tabs;
});

const onEdit = (targetKey, action) => {
    if(action == "remove") {
      store.commit('remove_tab',targetKey)
    }
};
</script>
<style>

</style>