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
        <a-form-item label="关联业务系统">
          <a-space v-if="form.business_system_names?.length" wrap>
            <a-tag v-for="name in form.business_system_names" :key="name">{{ name }}</a-tag>
          </a-space>
          <span v-else class="empty-hint">暂无</span>
          <div class="field-hint">归属关系在「业务系统」中维护，此处仅展示。</div>
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
import { getProject, saveProject } from '@/api/assets/application'

const props = defineProps({ open: { type: Boolean, required: true }, projectId: { type: Number, default: null } })
const emit = defineEmits(['update:open', 'saved'])
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const initialForm = () => ({ id: null, name: '', code: '', business_system_names: [], owner: '', enabled: true, remark: '' })
const form = reactive(initialForm())
const rules = {
  name: [{ required: true, message: '请输入项目名称' }],
  code: [{ required: true, message: '请输入项目编码' }, { pattern: /^[a-z0-9][a-z0-9_-]*$/, message: '编码仅支持小写字母、数字、下划线和连字符' }],
}

async function initialize() {
  Object.assign(form, initialForm())
  if (!props.projectId) return
  loading.value = true
  try {
    const response = await getProject(props.projectId)
    Object.assign(form, initialForm(), response?.data?.data || {})
  } finally {
    loading.value = false
  }
}

async function submit() {
  try {
    await formRef.value.validate()
  } catch {
    // 表单校验失败时 antd 已在字段上标红，不再弹全局提示
    return
  }
  saving.value = true
  try {
    const response = await saveProject({ ...form })
    message.success('保存成功')
    emit('update:open', false)
    emit('saved', response?.data?.data)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存项目失败')
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => { if (visible) initialize() })
</script>

<style scoped>
.empty-hint {
  color: rgba(0, 0, 0, 0.45);
}

.field-hint {
  margin-top: 4px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
