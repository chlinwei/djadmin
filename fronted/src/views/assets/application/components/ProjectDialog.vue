<template>
  <a-modal
    :open="open"
    :title="projectId ? `编辑项目：${form.name || '加载中'}` : '新增项目'"
    :confirm-loading="saving"
    ok-text="保存"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-form-item name="name" label="项目名称"><a-input v-model:value="form.name" /></a-form-item>
        <a-form-item name="code" label="项目编码"><a-input v-model:value="form.code" /></a-form-item>
        <a-form-item name="business_systems" label="关联业务系统">
          <a-select
            v-model:value="form.business_systems"
            mode="multiple"
            show-search
            :filter-option="filterOption"
            :options="businessSystemOptions"
            :getPopupContainer="getPopupContainer"
            placeholder="请选择关联业务系统"
          />
        </a-form-item>
        <a-form-item name="owner" label="负责人"><a-input v-model:value="form.owner" /></a-form-item>
        <a-form-item label="启用"><a-switch v-model:checked="form.enabled" /></a-form-item>
        <a-form-item label="备注"><a-textarea v-model:value="form.remark" :rows="3" /></a-form-item>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import { reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { getBusinessSystemList, getProject, saveProject } from '@/api/assets/application'

const props = defineProps({ open: { type: Boolean, required: true }, projectId: { type: Number, default: null } })
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const businessSystemOptions = ref([])
const initialForm = () => ({ id: null, name: '', code: '', business_systems: [], owner: '', enabled: true, remark: '' })
const form = reactive(initialForm())
const rules = {
  name: [{ required: true, message: '请输入项目名称' }],
  code: [{ required: true, message: '请输入项目编码' }, { pattern: /^[a-z0-9][a-z0-9_-]*$/, message: '编码仅支持小写字母、数字、下划线和连字符' }],
}
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())

async function initialize() {
  Object.assign(form, initialForm())
  loading.value = true
  try {
    const systemsResponse = await getBusinessSystemList({ page: 1, page_size: 1000, enabled: true })
    businessSystemOptions.value = (systemsResponse?.data?.data?.results || []).map((item) => ({ label: item.name, value: item.id }))
    if (props.projectId) {
      const response = await getProject(props.projectId)
      Object.assign(form, initialForm(), response?.data?.data || {})
    }
  } finally {
    loading.value = false
  }
}

async function submit() {
  await formRef.value.validate()
  saving.value = true
  try {
    const response = await saveProject({ ...form })
    message.success('保存成功')
    emit('update:open', false)
    emit('saved', response?.data?.data)
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => { if (visible) initialize() })
</script>
