<template>
  <div>
    <div class="logo" />
    <a-menu theme="dark" mode="inline" v-model:openKeys="openKeys" v-model:selectedKeys="selectedKeys">
      <a-menu-item key="/index" @click="add_tab({ name: '首页', path: '/index' })">
        <FontAwesomeIcon :icon="'fa-home'" />
        <span>首页</span>
      </a-menu-item>
      <template v-for="menu in menuList">
        <SubMenu v-if="menu.location === 1" :menu="menu" />
      </template>

      <!-- <SubMenu v-for="menu in menuList" :menu="menu"/> -->
    </a-menu>
  </div>
</template>
<script setup>

import store from '@/store/index.js'
import { ref } from 'vue'
import { onMounted } from 'vue'
import { getMenuList } from '@/api/menu/index.js'
import { useRouter } from 'vue-router';
import { computed } from 'vue';
import SubMenu from '@/layout/menu/subMenu.vue';


const selectedKeys = computed({
  get: () => {
    return store.state.selectedKeys;
  },
  set: (val) => {
    store.state.selectedKeys = val;
  }
})

function init_selectedKeys() {
  // 刷新高亮当前选中的
  store.commit('set_selectedKeys', useRouter().currentRoute.value.path)
}
init_selectedKeys();
const menuList = getMenuList();
const openKeys = ref([])

onMounted(() => {
  menuList.forEach(menu => {
    openKeys.value.push(menu.path)
  });
})

const add_tab = (item) => {
  let tab = {
    title: item.name,
    key: item.path
  }
  store.commit('add_tab', tab);
}
</script>
<style scoped></style>