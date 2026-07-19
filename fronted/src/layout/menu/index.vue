<template>
  <div>
    <div class="logo" />
    <a-menu
      theme="dark"
      mode="inline"
      v-model:openKeys="openKeys"
      v-model:selectedKeys="selectedKeys"
      :items="menuItems"
      @click="handleMenuClick"
    />
  </div>
</template>
<script setup>

import store from '@/store/index.js'
import { h, ref, resolveComponent } from 'vue'
import { onMounted } from 'vue'
import { getMenuList } from '@/api/menu/index.js'
import { useRouter } from 'vue-router';
import { computed } from 'vue';


const selectedKeys = computed({
  get: () => {
    return store.state.selectedKeys;
  },
  set: (val) => {
    store.state.selectedKeys = val;
  }
})

const menuPathNameMap = new Map([['/index', '首页']])
const FontAwesomeIconComp = resolveComponent('FontAwesomeIcon')

const normalizeMenuKey = (menu, prefix) => {
  const rawPath = String(menu?.path || '').trim()
  if (rawPath) {
    return rawPath
  }
  return `${prefix}-${menu?.id || 'no-id'}`
}

const buildMenuItems = (menus = [], parentPrefix = 'menu') => {
  const rows = []
  ;(Array.isArray(menus) ? menus : []).forEach((menu, index) => {
    const menuType = String(menu?.menu_type || '').trim()
    if (menuType !== 'M' && menuType !== 'C') {
      return
    }

    const key = normalizeMenuKey(menu, `${parentPrefix}-${index}`)
    const title = String(menu?.name || key)
    const iconName = String(menu?.icon || '').trim() || 'fa-folder'
    const iconVNode = () => h('span', { class: 'menu-icon-wrap' }, [h(FontAwesomeIconComp, { icon: iconName })])

    if (menuType === 'C') {
      menuPathNameMap.set(key, title)
      rows.push({
        key,
        label: title,
        icon: iconVNode,
      })
      return
    }

    const children = buildMenuItems(menu?.children, `${parentPrefix}-${index}`)
    rows.push({
      key,
      label: title,
      icon: iconVNode,
      children,
    })
  })
  return rows
}

const visibleMenuList = computed(() => {
  return (Array.isArray(menuList) ? menuList : []).filter((menu) => Number(menu?.location || 0) === 1)
})

const menuItems = computed(() => {
  menuPathNameMap.clear()
  menuPathNameMap.set('/index', '首页')
  return [
    {
      key: '/index',
      label: '首页',
      icon: () => h('span', { class: 'menu-icon-wrap' }, [h(FontAwesomeIconComp, { icon: 'fa-home' })]),
    },
    ...buildMenuItems(visibleMenuList.value, 'root'),
  ]
})

function init_selectedKeys() {
  // 刷新高亮当前选中的
  store.commit('set_selectedKeys', useRouter().currentRoute.value.path)
}
init_selectedKeys();
const menuList = getMenuList();
const openKeys = ref([])

function collectParentPathsByCurrentRoute(nodes, currentPath, parents = [], result = []) {
  for (const menu of Array.isArray(nodes) ? nodes : []) {
    const nextParents = menu?.menu_type === 'M' && menu?.path ? [...parents, menu.path] : parents
    if (menu?.path === currentPath) {
      result.push(...parents)
      return true
    }
    if (Array.isArray(menu?.children) && menu.children.length > 0) {
      const found = collectParentPathsByCurrentRoute(menu.children, currentPath, nextParents, result)
      if (found) {
        return true
      }
    }
  }
  return false
}

onMounted(() => {
  const nextOpenKeys = []
  const walk = (nodes) => {
    ;(Array.isArray(nodes) ? nodes : []).forEach((menu) => {
      if (menu?.menu_type === 'M' && menu?.is_expanded !== false && menu?.path) {
        nextOpenKeys.push(menu.path)
      }
      if (Array.isArray(menu?.children) && menu.children.length > 0) {
        walk(menu.children)
      }
    })
  }
  walk(menuList)

  // 仅初始化时把当前路由的父级目录展开，后续展开/折叠以用户当前操作状态为准。
  const currentPath = useRouter().currentRoute.value.path
  const parentPaths = []
  collectParentPathsByCurrentRoute(menuList, currentPath, [], parentPaths)
  openKeys.value = Array.from(new Set([...nextOpenKeys, ...parentPaths]))
})

const add_tab = (item) => {
  let tab = {
    title: item.name,
    key: item.path
  }
  store.commit('add_tab', tab);
}

const handleMenuClick = ({ key }) => {
  const keyText = String(key || '').trim()
  if (!keyText) {
    return
  }
  add_tab({
    name: menuPathNameMap.get(keyText) || keyText,
    path: keyText,
  })
}
</script>
<style scoped></style>