<template>
  <a-modal :open="open" :title="record ? '编辑依赖关系' : '新增依赖关系'" :width="720" centered :mask-closable="false" :closable="!saving" @cancel="close">
    <a-alert type="info" show-icon message="A → B 表示 A 依赖 B；双向不是请求与响应，而是两个方向都存在业务依赖。" style="margin-bottom: 16px" />
    <a-form ref="formRef" :model="form" layout="vertical">
      <a-row :gutter="16">
        <a-col :span="12"><a-form-item label="依赖方（谁需要对方）" name="source_service_id" :rules="[{ required: true, message: '请选择依赖方' }]">
          <a-select v-model:value="form.source_service_id" show-search :options="serviceOptions" option-filter-prop="label" :getPopupContainer="getPopupContainer" placeholder="请选择服务" />
        </a-form-item></a-col>
        <a-col :span="12"><a-form-item label="被依赖方（依赖谁）" name="target_service_id" :rules="[{ required: true, message: '请选择被依赖方' }]">
          <a-select v-model:value="form.target_service_id" show-search :options="serviceOptions" option-filter-prop="label" :getPopupContainer="getPopupContainer" placeholder="请选择服务" />
        </a-form-item></a-col>
      </a-row>
      <DependencyFields :model="form.forward" prefix="forward" :type-options="typeOptions" />
      <a-form-item v-if="!record"><a-checkbox v-model:checked="form.bidirectional">同时新增反向依赖，分别设置强弱与功能范围</a-checkbox></a-form-item>
      <section v-if="!record && form.bidirectional" class="reverse-section">
        <h4>反向：{{ serviceName(form.target_service_id) }} → {{ serviceName(form.source_service_id) }}</h4>
        <DependencyFields :model="form.reverse" prefix="reverse" :type-options="enabledTypeOptions" />
      </section>
    </a-form>
    <a-alert v-if="error" type="error" show-icon :message="error" />
    <template #footer><a-button :disabled="saving" @click="close">取消</a-button><a-button type="primary" :loading="saving" @click="save">保存</a-button></template>
  </a-modal>
</template>

<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { saveServiceDependency } from '@/api/assets/serviceDependencies'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import DependencyFields from './DependencyFields.vue'
import { dependencyData } from './dependencyGraph'

const props = defineProps({
  open: Boolean,
  record: { type: Object, default: null },
  services: { type: Array, required: true },
  types: { type: Array, required: true },
  initialSource: { type: Number, default: undefined },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const freshDetails = () => ({ interaction_type_id: undefined, strength: 'unknown', function_scope: '', protocol: '', failure_impact: '', fallback_plan: '' })
const form = reactive({ source_service_id: undefined, target_service_id: undefined, bidirectional: false, forward: freshDetails(), reverse: freshDetails() })
const formRef = ref()
const saving = ref(false)
const error = ref('')
let generation = 0
const serviceOptions = computed(() => props.services.map((node) => ({ value: node.id, label: `${node.name} · ${node.business_system_name} / ${node.code}` })))
const enabledTypeOptions = computed(() => props.types.filter((item) => item.enabled).map((item) => ({ value: item.id, label: item.name })))
const typeOptions = computed(() => props.types.filter((item) => item.enabled || item.id === props.record?.interaction_type_id).map((item) => ({ value: item.id, label: `${item.name}${item.enabled ? '' : '（已停用，仅保留原关系）'}` })))
const serviceName = (id) => props.services.find((node) => node.id === id)?.name || '未选择服务'

watch(() => [props.open, props.record], () => {
  generation++
  if (!props.open) return
  const record = props.record
  Object.assign(form, {
    source_service_id: record?.source_service_id ?? props.initialSource,
    target_service_id: record?.target_service_id,
    bidirectional: false,
    forward: record ? Object.fromEntries(Object.keys(freshDetails()).map((key) => [key, record[key]])) : freshDetails(),
    reverse: freshDetails(),
  })
  error.value = ''
  formRef.value?.clearValidate()
}, { immediate: true })

function close() {
  if (!saving.value) emit('update:open', false)
}

async function save() {
  if (saving.value) return
  try { await formRef.value.validate() } catch (validation) {
    if (validation?.errorFields) return
    error.value = validation?.message || '表单校验失败'
    return
  }
  if (form.source_service_id === form.target_service_id) { error.value = '不能让服务依赖自身'; return }
  const payload = {
    source_service_id: form.source_service_id,
    target_service_id: form.target_service_id,
    ...form.forward,
    function_scope: form.forward.function_scope.trim(),
  }
  if (!props.record && form.bidirectional) payload.reverse = { ...form.reverse, function_scope: form.reverse.function_scope.trim() }
  const current = generation
  saving.value = true
  error.value = ''
  try {
    const data = dependencyData(await saveServiceDependency(props.record?.id, payload))
    if (!Array.isArray(data.results) || data.results.length !== (payload.reverse ? 2 : 1)) throw new Error('保存结果不完整，请刷新后核对关系')
    if (current !== generation) return
    message.success('依赖关系已保存')
    emit('saved')
    emit('update:open', false)
  } catch (failure) {
    if (current === generation) error.value = failure?.message || '保存依赖关系失败'
  } finally { saving.value = false }
}
onBeforeUnmount(() => { generation++ })
</script>

<style scoped>
.reverse-section { border: 1px solid #dce9fa; border-radius: 6px; padding: 14px; background: #f8fbff; }
.reverse-section h4 { margin: 0 0 14px; color: #426a9f; }
</style>
