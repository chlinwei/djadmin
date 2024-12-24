<template>
    <div>
        <div class="logo" />
            <a-menu theme="dark"  mode="inline" v-model:openKeys="openKeys" v-model:selectedKeys="selectedKeys">
          <a-menu-item key="/index" @click="add_tab({name:'首页',path:'/index'})">
            <HomeOutlined />
                <span>首页</span>
          </a-menu-item>
          <a-sub-menu :key="menu.path" v-for="menu in menuList">
            <template #title>
                        <SvgIcon :name="menu.icon"></SvgIcon>
                        &nbsp; <span>{{menu.name}}</span>
                    </template>
            <a-menu-item :key="subMen.path" v-for="subMen in menu.children" @click="add_tab(subMen)">{{subMen.name}}</a-menu-item>
          </a-sub-menu>

        </a-menu>
    </div>
</template>
<script setup>
import {HomeOutlined} from '@ant-design/icons-vue';
import store from '@/store/index.js'
import {ref} from 'vue'
import {onMounted} from 'vue'
import {getMenuList} from '@/api/user/index.js'
import { useRouter } from 'vue-router';
import { computed } from 'vue';

const selectedKeys = computed({
  get:()=>{
    return store.state.selectedKeys;
  },
  set:(val) => {
    store.state.selectedKeys = val;
  }
})

function init_selectedKeys() {
    // 刷新高亮当前选中的
    store.commit('set_selectedKeys',useRouter().currentRoute.value.path)
}
console.log("menu:")
console.log(useRouter().currentRoute.value.path)
init_selectedKeys();
const menuList = getMenuList();
const  openKeys = ref([])

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
    store.commit('add_tab',tab);;
}
</script>
<style scoped>
</style>