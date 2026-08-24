<template>
  <a-modal
    :open="open"
    :title="`${application?.name || '应用'} - 版本管理`"
    :width="760"
    :footer="null"
    @cancel="emit('update:open', false)"
  >
    <a-form layout="inline" class="version-form" @submit.prevent="saveVersion">
      <a-form-item label="版本号" required>
        <a-input v-model:value="versionForm.version" placeholder="例如 9.0.93" />
      </a-form-item>
      <a-form-item label="停止支持日期">
        <a-date-picker v-model:value="versionForm.end_of_support" value-format="YYYY-MM-DD" />
      </a-form-item>
      <a-button type="primary" :loading="saving" @click="saveVersion">新增版本</a-button>
    </a-form>

    <a-table
      row-key="id"
      size="small"
      :columns="columns"
      :data-source="versions"
      :loading="loading"
      :pagination="false"
      :scroll="{ x: 640 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'enabled'">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '允许部署' : '已停用' }}</a-tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-tooltip title="删除">
            <a-button class="delBtn" size="small" type="primary" danger @click="confirmDelete(record)">
              <FontAwesomeIcon :icon="['fas', 'trash-can']" />
            </a-button>
          </a-tooltip>
        </template>
      </template>
    </a-table>
  </a-modal>
</template>

<script setup>
import { reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import {
  deleteApplicationVersion,
  getApplicationVersionList,
  saveApplicationVersion,
} from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, required: true },
  application: { type: Object, default: null },
})
const emit = defineEmits(['update:open', 'changed', 'created'])

const columns = [
  { title: '版本号', dataIndex: 'version', key: 'version' },
  { title: '停止支持日期', dataIndex: 'end_of_support', key: 'end_of_support', width: 150 },
  { title: '状态', key: 'enabled', width: 110 },
  { title: '备注', dataIndex: 'remark', key: 'remark' },
  { title: '操作', key: 'action', width: 90, fixed: 'right' },
]
const versions = ref([])
const loading = ref(false)
const saving = ref(false)
const versionForm = reactive({ version: '', end_of_support: null })

async function loadVersions() {
  if (!props.application?.id) return
  loading.value = true
  try {
    const response = await getApplicationVersionList({ application: props.application.id, page_size: 100 })
    versions.value = response?.data?.data?.results || []
  } finally {
    loading.value = false
  }
}

async function saveVersion() {
  const version = String(versionForm.version || '').trim()
  if (!version) {
    message.error('版本号为必填项')
    return
  }
  saving.value = true
  try {
    const response = await saveApplicationVersion({
      application: props.application.id,
      version,
      end_of_support: versionForm.end_of_support,
      enabled: true,
    })
    versionForm.version = ''
    versionForm.end_of_support = null
    message.success('版本新增成功')
    await loadVersions()
    emit('changed')
    emit('created', response?.data?.data)
  } finally {
    saving.value = false
  }
}

function confirmDelete(record) {
  openDeleteConfirm({
    title: '确认删除应用版本',
    summary: '已被部署实例引用的版本不能删除。',
    items: [`${props.application?.name || '应用'} ${record.version}`],
    onConfirm: async () => {
      await deleteApplicationVersion(record.id)
      message.success('版本删除成功')
      await loadVersions()
      emit('changed')
    },
  })
}

watch(() => props.open, (visible) => {
  if (visible) loadVersions()
})
</script>

<style scoped>
.version-form {
  margin-bottom: 16px;
  gap: 8px;
}
</style>
