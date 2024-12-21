<template>
    <div>
        <div class="logo" />
            <a-menu theme="dark"  mode="inline" v-model:openKeys="openKeys">
          <a-menu-item key="index">
            <HomeOutlined />
                <span>首页</span>
          </a-menu-item>
          <a-sub-menu :key="menu.path" v-for="menu in menuList">
            <template #title>
                        <SvgIcon :name="menu.icon"></SvgIcon>
                        &nbsp; <span>{{menu.name}}{{ menu.path }}</span>
                    </template>
          
            <a-menu-item :key="subMen.id" v-for="subMen in menu.children">{{subMen.name}}</a-menu-item>
    
          </a-sub-menu>

        </a-menu>
    </div>
</template>
<script setup>
import {HomeOutlined} from '@ant-design/icons-vue';
import {ref} from 'vue'
import {onMounted} from 'vue'
const menuList=JSON.parse(sessionStorage.getItem("menuList"))
const  openKeys = ref([])
onMounted(() => {
    menuList.forEach(menu => {
        openKeys.value.push(menu.path)
    });
})
</script>
<style scoped>
</style>