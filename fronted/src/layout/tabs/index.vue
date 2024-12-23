<template>
<div>
    <a-tabs v-model:activeKey="activeKey" type="editable-card" @edit="onEdit" :hideAdd="true">
     
    <a-tab-pane v-for="pane in panes" :key="pane.key" :tab="pane.title" :closable="pane.closable" @click="remove_tab(pane)">
     
    </a-tab-pane>
  </a-tabs>
</div>
</template>
<script setup>


import store from '@/store/index.js';



import { computed } from 'vue';
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