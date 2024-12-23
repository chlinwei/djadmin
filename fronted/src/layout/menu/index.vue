<template>
    <div>
        <div class="logo" />
            <a-menu theme="dark"  mode="inline" v-model:openKeys="openKeys">
          <a-menu-item key="index" @click="add_tab({name:'首页',path:'/index'})">
            <HomeOutlined />
                <span>首页</span>
          </a-menu-item>
          <a-sub-menu :key="menu.path" v-for="menu in menuList">
            <template #title>
                        <SvgIcon :name="menu.icon"></SvgIcon>
                        &nbsp; <span>{{menu.name}}</span>
                    </template>
          
            <a-menu-item :key="subMen.id" v-for="subMen in menu.children" @click="add_tab(subMen)">{{subMen.name}}</a-menu-item>
          </a-sub-menu>

        </a-menu>
    </div>
</template>
<script setup>
import {HomeOutlined} from '@ant-design/icons-vue';
import store from '@/store/index.js'
import {ref} from 'vue'
import {onMounted} from 'vue'
import router from '@/router'
import {getMenuList} from '@/api/user/index.js'
const menuList = getMenuList();
// const menuList=JSON.parse(sessionStorage.getItem("menuList"))
// function resolve(dir) {
//     return resolve.join(__dirname, dir)
// }
// function set_dynamic_routes() {
   
    
//     menuList.forEach((e)=>{
//         tempPath = e.path || ''
//         tempList = []
//         if('children' in e) {
//             let children = e.children
//             children.forEach((e1)=>{
//                 if(menuList.find(o => o.path && o.path == e1.path)) {
//                     tempList.push(e1)
//                 }
            
//             })
//         }
//         console.log(tempList)
//     })
//     console.log(router.getRoutes())
// }
// set_dynamic_routes()

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
    store.commit('add_tab',tab);
    router.replace(item.path);
}
</script>
<style scoped>
</style>