<template>
<div>
    <a-tabs v-model:activeKey="activeKey" type="editable-card" @edit="onEdit">
     
    <a-tab-pane v-for="pane in panes" :key="pane.key" :tab="pane.title" :closable="pane.closable" @click="remove_tab(pane)">
      {{ pane.content }}
    </a-tab-pane>
  </a-tabs>
</div>
</template>
<script setup>
// const panes = ref([
//   {
//     title: 'Tab 1',
//     content: 'Content of Tab 1',
//     key: '1',
//   },
//   {
//     title: 'Tab 2',
//     content: 'Content of Tab 2',
//     key: '2',
//   },
//   {
//     title: 'Tab 3',
//     content: 'Content of Tab 3',
//     key: '3',
//   },
// ]);
// const activeKey = ref(panes.value[0].key);
// const newTabIndex = ref(0);
// const add = () => {
//   activeKey.value = `newTab${++newTabIndex.value}`;
//   panes.value.push({
//     title: 'New Tab',
//     content: 'Content of new Tab',
//     key: activeKey.value,
//   });
// };




import store from '@/store/index.js';
import {ref} from 'vue';


const panes = ref(store.state.tabs);
const activeKey = ref(store.state.activeKey);


const remove = targetKey => {
  let lastIndex = 0;
  panes.value.forEach((pane, i) => {
    if (pane.key === targetKey) {
        // 表示删除的那个tab的前面一个的tab的key的index
      lastIndex = i - 1;
        console.log("lastindex:" + lastIndex)
    }
  });
//   删除被删除的key
  panes.value = panes.value.filter(pane => pane.key !== targetKey);
   //如果被删除的那个正好和活动的tab是同一个，且lastindex存在，则选择被删的的tab的前一个tab
  if (panes.value.length && activeKey.value === targetKey) {
    if (lastIndex >= 0) {
        console.log(lastIndex)
      activeKey.value = panes.value[lastIndex].key;
    } else {
        //否则选择第一个作为活动tab
      activeKey.value = panes.value[0].key;
    }
  }
};
const onEdit = (targetKey, action) => {
    if(action == "remove") {
        remove(targetKey)
    }
};
</script>
<style>

</style>