<template>
  <div>
    <a-tabs v-model:activeKey="activeKey" type="editable-card" @edit="onEdit" :hideAdd="true" class="app-tabs">
      <template #rightExtra>
        <a-tooltip title="关闭全部标签页（保留首页）">
          <a-button type="text" class="tabs-action-btn" @click="closeAllTabs">
            <template #icon>
              <CloseCircleOutlined />
            </template>
            关闭全部
          </a-button>
        </a-tooltip>
      </template>
      <a-tab-pane
        v-for="pane in panes"
        :key="pane.key"
        :tab="pane.title"
        :closable="pane.key == '/index' ? false : true"
      ></a-tab-pane>
    </a-tabs>
  </div>
</template>
<script setup>
import router from '@/router'
import store from '@/store/index.js'
import { watch, computed } from 'vue'
import { CloseCircleOutlined } from '@ant-design/icons-vue'
import { useRouter, useRoute } from 'vue-router'

const vueRouter = useRouter()
const route = useRoute()

const activeKey = computed({
  get: () => {
    return store.state.activeKey
  },
  set: (val) => {
    store.state.activeKey = val
  },
})

watch(activeKey, (newPath) => {
  router.push(newPath)
})

// 如果点击返回
watch(route, (newRoute) => {
  const tab = {
    title: newRoute.name,
    key: newRoute.path,
  }
  store.commit('add_tab', tab)
})

const panes = computed(() => {
  return store.state.tabs
})

const onEdit = (targetKey, action) => {
  const allRoutes = router.getRoutes()
  allRoutes.forEach((item) => {
    if (item.path == targetKey && item.meta) {
      item.meta.cached = false
    }
  })

  if (action == 'remove') {
    store.commit('remove_tab', targetKey)
    if (store.state.tabs.length == 0) {
      store.commit('add_tab', {
        title: '首页',
        key: '/index',
      })
    }
  }
}

function reset_tabs() {
  const tab = {
    title: vueRouter.currentRoute.value.name,
    key: vueRouter.currentRoute.value.fullPath,
  }
  store.commit('reset_tab', tab)
}

function closeAllTabs() {
  store.commit('close_all_tabs')
  router.push('/index')
}

reset_tabs()
</script>

<style scoped>
.app-tabs {
  margin-bottom: 6px;
}

.tabs-action-btn {
  height: 26px;
  padding: 0 10px;
  border-radius: 14px;
  color: #334155;
  font-weight: 500;
}

.tabs-action-btn:hover {
  background: #eef2ff;
  color: #1d4ed8;
}
</style>