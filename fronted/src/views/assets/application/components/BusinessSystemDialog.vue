<template>
  <a-modal
    :open="open"
    :title="systemId ? '编辑业务系统' : '新增业务系统'"
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
            <a-form-item name="name" label="业务系统名称">
              <a-input v-model:value="form.name" placeholder="例如 KUL-TIB" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="code" label="业务系统编码">
              <a-input v-model:value="form.code" placeholder="例如 kul-tib" />
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
import { getBusinessSystem, saveBusinessSystem } from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, required: true },
  systemId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const initialForm = () => ({ id: null, name: '', code: '', owner: '', enabled: true, remark: '' })
const form = reactive(initialForm())
const rules = {
  name: [{ required: true, message: '请输入业务系统名称' }],
  code: [
    { required: true, message: '请输入业务系统编码' },
    { pattern: /^[a-z0-9][a-z0-9_-]*$/, message: '编码仅支持小写字母、数字、下划线和连字符' },
  ],
}

async function initialize() {
  Object.assign(form, initialForm())
  if (!props.systemId) return
  loading.value = true
  try {
    const response = await getBusinessSystem(props.systemId)
    Object.assign(form, response?.data?.data || {})
  } finally {
    loading.value = false
  }
}

async function submit() {
  await formRef.value.validate()
  saving.value = true
  try {
    await saveBusinessSystem({ ...form })
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
