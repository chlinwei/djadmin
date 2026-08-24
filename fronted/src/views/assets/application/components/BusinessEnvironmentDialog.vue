<template>
  <a-modal
    :open="open"
    :title="environmentId ? `编辑环境：${form.name || '加载中'}` : '新增环境'"
    :confirm-loading="saving"
    ok-text="保存"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item name="business_system" label="业务系统">
              <a-select
                v-model:value="form.business_system"
                show-search
                :filter-option="filterOption"
                :options="businessSystemOptions"
                :getPopupContainer="getPopupContainer"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="name" label="环境名称">
              <a-input v-model:value="form.name" placeholder="例如 生产环境" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="code" label="环境编码">
              <a-input v-model:value="form.code" placeholder="例如 production" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="order" label="展示顺序">
              <a-input-number v-model:value="form.order" :min="0" :max="999" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="owner" label="负责人">
              <a-input v-model:value="form.owner" placeholder="负责人或团队" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="启用">
              <a-switch v-model:checked="form.enabled" />
            </a-form-item>
          </a-col>
          <a-col :span="24">
            <a-form-item label="备注">
              <a-textarea v-model:value="form.remark" :rows="3" />
            </a-form-item>
          </a-col>
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
  getBusinessEnvironment,
  getBusinessSystemList,
  saveBusinessEnvironment,
} from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, required: true },
  environmentId: { type: Number, default: null },
  businessSystemId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const businessSystemOptions = ref([])
const initialForm = () => ({
  id: null, business_system: null, name: '', code: '', order: 0, owner: '', enabled: true, remark: '',
})
const form = reactive(initialForm())
const rules = {
  business_system: [{ required: true, message: '请选择业务系统' }],
  name: [{ required: true, message: '请输入环境名称' }],
  code: [
    { required: true, message: '请输入环境编码' },
    { pattern: /^[a-z0-9][a-z0-9_-]*$/, message: '编码仅支持小写字母、数字、下划线和连字符' },
  ],
}
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())

async function initialize() {
  Object.assign(form, initialForm())
  loading.value = true
  try {
    const response = await getBusinessSystemList({ page: 1, page_size: 1000, enabled: true })
    businessSystemOptions.value = (response?.data?.data?.results || []).map((item) => ({ label: item.name, value: item.id }))
    if (props.environmentId) {
      const detail = await getBusinessEnvironment(props.environmentId)
      Object.assign(form, initialForm(), detail?.data?.data || {})
    } else if (props.businessSystemId) {
      form.business_system = props.businessSystemId
    }
  } finally {
    loading.value = false
  }
}

async function submit() {
  await formRef.value.validate()
  saving.value = true
  try {
    const response = await saveBusinessEnvironment({ ...form })
    message.success('保存成功')
    emit('update:open', false)
    emit('saved', response?.data?.data)
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => {
  if (visible) initialize()
})
</script>
