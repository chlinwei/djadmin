<template>
  <a-modal
    :open="open"
    :title="profileId ? '编辑自定义集群模型' : '新增自定义集群模型'"
    :width="680"
    :confirm-loading="saving"
    ok-text="保存"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12"><a-form-item name="name" label="模型名称"><a-input v-model:value="form.name" placeholder="例如 Redis Sentinel" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="code" label="模型编码"><a-input v-model:value="form.code" placeholder="例如 redis-sentinel" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="application" label="应用"><a-select v-model:value="form.application" show-search :filter-option="filterOption" :options="applicationOptions" :getPopupContainer="getPopupContainer" placeholder="请选择该集群对应的应用" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="模型类型"><a-input value="自定义集群" disabled /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="启用"><a-switch v-model:checked="form.enabled" /></a-form-item></a-col>
          <a-col :span="24"><a-form-item label="备注"><a-textarea v-model:value="form.remark" :rows="3" /></a-form-item></a-col>
        </a-row>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import { reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import {
  getApplicationList,
  getClusterProfile,
  saveClusterProfile,
} from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, required: true },
  profileId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const applicationOptions = ref([])
const initialForm = () => ({
  id: null, name: '', code: '', application: null, profile_type: 'custom',
  cluster_type: 'custom', enabled: true, remark: '',
})
const form = reactive(initialForm())
const rules = {
  name: [{ required: true, message: '请输入模型名称' }],
  code: [
    { required: true, message: '请输入模型编码' },
    { pattern: /^[a-z0-9][a-z0-9_-]*$/, message: '编码仅支持小写字母、数字、下划线和连字符' },
  ],
  application: [{ required: true, message: '请选择应用' }],
}
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())

async function initialize() {
  Object.assign(form, initialForm())
  loading.value = true
  try {
    const applicationResponse = await getApplicationList({ enabled: true, page: 1, page_size: 1000 })
    applicationOptions.value = (applicationResponse?.data?.data?.results || []).map((item) => ({ label: item.name, value: item.id }))
    if (props.profileId) {
      const response = await getClusterProfile(props.profileId)
      const data = response?.data?.data || {}
      Object.assign(form, initialForm(), data)
    }
  } finally {
    loading.value = false
  }
}

async function submit() {
  await formRef.value.validate()
  saving.value = true
  try {
    await saveClusterProfile({ ...form })
    message.success('保存成功')
    emit('update:open', false)
    emit('saved')
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => {
  if (visible) initialize()
})
</script>
