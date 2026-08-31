<template>
  <a-modal
    :open="open"
    :title="`${application?.name || '应用'} - 部署模板管理`"
    :width="1000"
    :footer="null"
    @cancel="emit('update:open', false)"
  >
    <div class="template-manager-toolbar">
      <a-button v-permission="'assets:applications:create'" type="primary" @click="openCreate">
        <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
        <span>&nbsp;新增模板</span>
      </a-button>
    </div>

    <a-table
      row-key="id"
      size="small"
      :columns="columns"
      :data-source="templates"
      :loading="loading"
      :pagination="false"
      :scroll="{ x: 1160 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'control_type'">
          <a-tag :color="controlTypeColors[record.control_type]">{{ controlTypeLabels[record.control_type] || record.control_type }}</a-tag>
        </template>
        <template v-else-if="column.key === 'service_count'">
          <a-tooltip :title="record.service_count ? '已被逻辑服务引用，需先解除引用才能删除' : '无逻辑服务引用，可直接删除'">
            <a-tag :color="record.service_count ? 'blue' : 'default'">{{ record.service_count ?? 0 }}</a-tag>
          </a-tooltip>
        </template>
        <template v-else-if="column.key === 'enabled'">
          <a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" />
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-tooltip title="编辑">
              <a-button v-permission="'assets:applications:update'" size="small" type="primary" @click="openEdit(record)">
                <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="复制">
              <a-button v-permission="'assets:applications:create'" size="small" @click="openCopy(record)">
                <FontAwesomeIcon :icon="['fas', 'copy']" />
              </a-button>
            </a-tooltip>
            <a-tooltip title="删除">
              <a-button v-permission="'assets:applications:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDelete(record)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
      </template>
    </a-table>

    <TemplateDialog
      :open="templateDialogOpen"
      :template-id="selectedTemplateId"
      :copy-from-id="selectedTemplateCopyId"
      :initial-application-id="application?.id"
      @update:open="templateDialogOpen = $event"
      @saved="handleTemplateSaved"
    />
  </a-modal>
</template>

<script setup>
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import {
  deleteApplicationDeploymentTemplate,
  getApplicationDeploymentTemplateList,
} from '@/api/assets/application'
import TemplateDialog from './TemplateDialog.vue'

const props = defineProps({
  open: { type: Boolean, required: true },
  application: { type: Object, default: null },
})
const emit = defineEmits(['update:open', 'changed'])

const controlTypeLabels = { systemd: 'Systemd', command: '命令行', external_ha: '外部 HA', docker: 'Docker', docker_compose: 'Docker Compose' }
const controlTypeColors = { systemd: 'blue', command: 'orange', external_ha: 'gold', docker: 'cyan', docker_compose: 'geekblue' }
const columns = [
  { title: '模板名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '控制方式', key: 'control_type', width: 130 },
  { title: '关联逻辑服务', key: 'service_count', width: 120 },
  { title: '运行用户', dataIndex: 'run_user', key: 'run_user', width: 120 },
  { title: 'App Home', dataIndex: 'app_home', key: 'app_home', width: 260 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '备注', dataIndex: 'remark', key: 'remark', width: 200 },
  { title: '操作', key: 'action', width: 130, fixed: 'right' },
]
const templates = ref([])
const loading = ref(false)
const templateDialogOpen = ref(false)
const selectedTemplateId = ref(null)
const selectedTemplateCopyId = ref(null)

async function loadTemplates() {
  if (!props.application?.id) return
  loading.value = true
  try {
    const response = await getApplicationDeploymentTemplateList({ application: props.application.id, page_size: 100 })
    templates.value = response?.data?.data?.results || []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  selectedTemplateId.value = null
  selectedTemplateCopyId.value = null
  templateDialogOpen.value = true
}
function openEdit(record) {
  selectedTemplateId.value = record.id
  selectedTemplateCopyId.value = null
  templateDialogOpen.value = true
}
function openCopy(record) {
  selectedTemplateId.value = null
  selectedTemplateCopyId.value = record.id
  templateDialogOpen.value = true
}

async function handleTemplateSaved() {
  await loadTemplates()
  emit('changed')
}

function confirmDelete(record) {
  openDeleteConfirm({
    title: '确认删除部署模板',
    summary: '已被部署实例引用的模板不能删除。',
    items: [`${props.application?.name || '应用'} / ${record.name}`],
    onConfirm: async () => {
      await deleteApplicationDeploymentTemplate(record.id)
      message.success('部署模板删除成功')
      await loadTemplates()
      emit('changed')
    },
  })
}

watch(() => props.open, (visible) => {
  if (visible) loadTemplates()
})
</script>

<style scoped>
.template-manager-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}
</style>
