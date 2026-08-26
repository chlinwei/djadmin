<template>
  <section class="environment-page">
    <a-row :gutter="12" class="tools">
      <a-col flex="360px"><a-input-search v-model:value="keyword" placeholder="搜索环境、编码" allow-clear enter-button @search="loadEnvironments" /></a-col>
      <a-col flex="auto" class="right-tools">
        <a-space>
          <a-button v-permission="'assets:applications:create'" size="large" @click="openEnvironment()"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" /><span>&nbsp;新增环境</span></a-button>
          <a-tooltip title="刷新"><a-button type="primary" ghost :loading="loading" @click="loadEnvironments"><FontAwesomeIcon :icon="['fas', 'arrows-rotate']" /><span>&nbsp;刷新</span></a-button></a-tooltip>
        </a-space>
      </a-col>
    </a-row>
    <a-table row-key="id" :columns="columns" :data-source="environments" :loading="loading" :pagination="false" :scroll="{ x: 1200 }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'enabled'"><a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" /></template>
        <template v-else-if="column.key === 'action'"><a-space>
          <a-tooltip title="编辑"><a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openEnvironment(record)"><FontAwesomeIcon :icon="['fa', 'edit']" /></a-button></a-tooltip>
          <a-tooltip title="删除"><a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDelete(record)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip>
        </a-space></template>
      </template>
    </a-table>
    <BusinessEnvironmentDialog :open="dialogOpen" :environment-id="selectedId" @update:open="dialogOpen = $event" @saved="loadEnvironments" />
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { deleteBusinessEnvironment, getBusinessEnvironmentList } from '@/api/assets/application'
import BusinessEnvironmentDialog from '../application/components/BusinessEnvironmentDialog.vue'

const environments = ref([])
const keyword = ref('')
const loading = ref(false)
const dialogOpen = ref(false)
const selectedId = ref(null)
const columns = [
  { title: '环境名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '环境编码', dataIndex: 'code', key: 'code', width: 180 },
  { title: '顺序', dataIndex: 'order', key: 'order', width: 100 },
  { title: '负责人', dataIndex: 'owner', key: 'owner', width: 180 },
  { title: '逻辑服务', dataIndex: 'service_count', key: 'service_count', width: 120 },
  { title: '部署实例', dataIndex: 'deployment_count', key: 'deployment_count', width: 120 },
  { title: '状态', key: 'enabled', width: 100 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 260 },
  { title: '操作', key: 'action', fixed: 'right', width: 120 },
]
async function loadEnvironments() {
  loading.value = true
  try {
    const response = await getBusinessEnvironmentList({ page: 1, page_size: 1000, search: keyword.value })
    environments.value = response?.data?.data?.results || []
  } finally {
    loading.value = false
  }
}
function openEnvironment(record = null) {
  selectedId.value = record?.id || null
  dialogOpen.value = true
}
function confirmDelete(record) {
  openDeleteConfirm({
    title: '删除环境',
    summary: '仍被逻辑服务或部署实例引用的环境不能删除。',
    items: [record.name || record.code || record.id],
    onConfirm: async () => { await deleteBusinessEnvironment(record.id); message.success('删除成功'); await loadEnvironments() },
  })
}
onMounted(loadEnvironments)
</script>
