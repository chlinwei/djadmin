<template>
    <a-menu-item v-if="menu.menu_type === 'C'" :key="menu.path || menu.id" @click="add_tab(menu)"><FontAwesomeIcon :icon="menu.icon" />&nbsp;{{ menu.name }}</a-menu-item>
    <a-sub-menu v-else-if="menu.menu_type === 'M'" :key="menu.path || menu.id">
        <!-- 下面的template的title就是目录名称,例如系统管理 -->
        <template #title>
            <FontAwesomeIcon :icon="menu.icon" />
            <span>&nbsp;{{ menu.name }}</span>
        </template>
        <SubMenu
            v-for="children in normalizedChildren"
            :key="children.path || children.id"
            :menu="children"
        />
    </a-sub-menu>

    <!-- 传统的从文件加载图标 -->
        <!-- <template #title>
            <SvgIcon :name="menu.icon"></SvgIcon>
            &nbsp; <span style="vertical-align: middle;">{{ menu.name }}</span>
        </template> -->
    
</template>
<style scoped></style>
<script setup>
import store from '@/store/index.js';
import router from '@/router';
import { computed, defineProps } from 'vue';

defineOptions({
    name: 'SubMenu',
})

const props = defineProps(
    {
        menu: {
            type: Object,
            default: null
        }
    }
)

const normalizedChildren = computed(() => {
    return Array.isArray(props.menu?.children) ? props.menu.children : []
})

const add_tab = (item) => {
    let tab = {
        title: item.name,
        key: item.path
    }
    store.commit('add_tab', tab);
    let all_routes = router.getRoutes()
     all_routes.forEach(e => {
      if(e.path == item.path) {
        e.meta.cached = true;
        console.log(e)
      }
     })
}
</script>