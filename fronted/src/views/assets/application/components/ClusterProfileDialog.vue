<template>
  <a-modal
    :open="open"
    :title="profileId ? `${locked ? '查看' : '编辑'}集群模型：${form.name || '加载中'}` : '新增自定义集群模型'"
    :width="680"
    :confirm-loading="saving"
    ok-text="保存"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <template v-if="locked" #footer><a-button @click="emit('update:open', false)">关闭</a-button></template>
    <a-spin :spinning="loading">
      <a-form :disabled="locked" ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12"><a-form-item name="name" label="模型名称"><a-input v-model:value="form.name" placeholder="例如 Redis Sentinel" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="code" label="模型编码"><a-input v-model:value="form.code" placeholder="例如 redis-sentinel" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="application" label="应用"><a-select v-model:value="form.application" show-search :filter-option="filterOption" :options="applicationOptions" :getPopupContainer="getPopupContainer" :virtual="false" placeholder="请选择该集群对应的应用" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="模型类型"><a-input :value="form.profile_type === 'builtin' ? '内置集群' : '自定义集群'" disabled /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="启用"><EnabledSwitch :read-only="locked" v-model:checked="form.enabled" /></a-form-item></a-col>
          <a-col :span="24"><a-form-item label="备注"><a-textarea v-model:value="form.remark" :rows="3" /></a-form-item></a-col>
        </a-row>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import EnabledSwitch from '@/components/EnabledSwitch.vue'
import { formFields } from './assetFormValues'
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import {
  getApplicationList,
  getClusterProfile,
  saveClusterProfile,
} from '@/api/assets/application'

const props = defineProps({
  readOnly: Boolean,
  open: { type: Boolean, required: true },
  profileId: { type: Number, default: null },
  initialApplicationId: { type: Number, default: null },
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
// 以详情为准：即使列表还认为模型可编辑，内置模型也不能保存。
const locked = computed(() => props.readOnly || form.profile_type !== 'custom' || (props.profileId != null && props.profileId !== form.id))
const rules = {
  name: [{ required: true, message: '请输入模型名称' }],
  code: [
    { required: true, message: '请输入模型编码' },
    { pattern: /^[A-Za-z0-9][A-Za-z0-9_-]*$/, message: '编码仅支持大小写字母、数字、下划线和连字符' },
  ],
  application: [{ required: true, message: '请选择应用' }],
}
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())

async function initialize() {
  Object.assign(form, initialForm())
  loading.value = true
  try {
    applicationOptions.value = []
    let applicationName
    if (props.profileId) {
      const response = await getClusterProfile(props.profileId)
      const data = response?.data?.data || {}
      Object.assign(form, formFields(initialForm(), data))
      applicationName = data.application_name
    } else if (props.initialApplicationId) {
      form.application = props.initialApplicationId
    }
    if (!locked.value) {
      const response = await getApplicationList({ enabled: true, page: 1, page_size: 1000 })
      applicationOptions.value = (response?.data?.data?.results || []).map(item => ({ label: item.name, value: item.id }))
    }
    if (form.application && !applicationOptions.value.some(item => item.value === form.application)) {
      applicationOptions.value.push({ value: form.application, label: applicationName || `应用 ${form.application}` })
    }
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (locked.value || loading.value) return
  try {
    await formRef.value.validate()
  } catch {
    // 表单校验失败时 antd 已在字段上标红，不再弹全局提示
    return
  }
  if (locked.value) return
  saving.value = true
  try {
    const response = await saveClusterProfile({ ...form })
    message.success('保存成功')
    emit('update:open', false)
    emit('saved', response?.data?.data)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存集群模型失败')
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => {
  if (visible) initialize()
}, { immediate: true })
</script>
