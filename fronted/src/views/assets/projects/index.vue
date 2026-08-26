<template>
  <section class="project-page">
    <a-row :gutter="12" class="tools">
      <a-col flex="360px"><a-input-search v-model:value="keyword" placeholder="搜索项目、编码或业务系统" allow-clear enter-button @search="loadProjects" /></a-col>
      <a-col flex="auto" class="right-tools">
        <a-space>
          <a-button v-permission="'assets:projects:create'" size="large" @click="openProject()"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" /><span>&nbsp;新增项目</span></a-button>
          <a-tooltip title="刷新"><a-button type="primary" ghost :loading="loading" @click="loadProjects"><FontAwesomeIcon :icon="['fas', 'arrows-rotate']" /><span>&nbsp;刷新</span></a-button></a-tooltip>
        </a-space>
      </a-col>
    </a-row>
    <a-table row-key="id" :columns="columns" :data-source="projects" :loading="loading" :pagination="false" :scroll="{ x: 1100 }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'business_system_names'">
          <a-space wrap><a-tag v-for="name in record.business_system_names || []" :key="name">{{ name }}</a-tag><span v-if="!record.business_system_names?.length">-</span></a-space>
        </template>
        <template v-else-if="column.key === 'enabled'"><a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" /></template>
        <template v-else-if="column.key === 'action'"><a-space>
          <a-tooltip title="编辑"><a-button v-permission="'assets:projects:update'" size="small" type="primary" @click="openProject(record)"><FontAwesomeIcon :icon="['fa', 'edit']" /></a-button></a-tooltip>
          <a-tooltip title="删除"><a-button v-permission="'assets:projects:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDelete(record)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip>
        </a-space></template>
      </template>
    </a-table>
    <ProjectDialog :open="dialogOpen" :project-id="selectedId" @update:open="dialogOpen = $event" @saved="loadProjects" />
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { deleteProject, getProjectList } from '@/api/assets/application'
import ProjectDialog from '../application/components/ProjectDialog.vue'

const projects = ref([])
const keyword = ref('')
const loading = ref(false)
const dialogOpen = ref(false)
const selectedId = ref(null)
const columns = [
  { title: '项目名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '项目编码', dataIndex: 'code', key: 'code', width: 180 },
  { title: '关联业务系统', dataIndex: 'business_system_names', key: 'business_system_names', width: 360 },
  { title: '负责人', dataIndex: 'owner', key: 'owner', width: 160 },
  { title: '状态', key: 'enabled', width: 100 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 240 },
  { title: '操作', key: 'action', fixed: 'right', width: 120 },
]

async function loadProjects() {
  loading.value = true
  try {
    const response = await getProjectList({ page: 1, page_size: 1000, search: keyword.value })
    projects.value = response?.data?.data?.results || []
  } finally {
    loading.value = false
  }
}
function openProject(record = null) {
  selectedId.value = record?.id || null
  dialogOpen.value = true
}
function confirmDelete(record) {
  openDeleteConfirm({
    title: '删除项目',
    summary: '删除项目只会解除项目关联，不会删除业务系统和服务。',
    items: [record.name || record.code || record.id],
    onConfirm: async () => { await deleteProject(record.id); message.success('删除成功'); await loadProjects() },
  })
}
onMounted(loadProjects)
</script>
