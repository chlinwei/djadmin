<template>
  <span class="enabled-switch" @click.stop>
    <a-switch
      v-bind="$attrs"
      :checked="endpoint ? confirmed : checked"
      :disabled="disabled || readOnly || pending || !allowed ? true : undefined"
      :loading="loading || pending"
      checked-children="启用"
      un-checked-children="停用"
      @change="change"
    />
  </span>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { checkPermission } from '@/directives/permission/permission'

defineOptions({ inheritAttrs: false })
const props = defineProps({
  checked: { type: Boolean, default: false },
  disabled: Boolean,
  loading: Boolean,
  readOnly: Boolean,
  endpoint: { type: String, default: '' },
  permission: { type: [String, Array], default: '' },
  selfService: Boolean,
})
const emit = defineEmits(['update:checked', 'change', 'changed'])
const pending = ref(false)
const confirmed = ref(props.checked)
watch(() => props.checked, value => { confirmed.value = value })
// Remote updates require explicit action permissions; self-service only applies to own-profile endpoints.
const allowed = computed(() => {
  if (!props.endpoint) return true
  if (props.selfService) return true
  const permissions = Array.isArray(props.permission) ? props.permission : [props.permission]
  return permissions.length > 0 && permissions.every(code => code && checkPermission(code))
})
async function change(checked, event) {
  if (props.disabled || props.readOnly || props.loading || pending.value || !allowed.value) return
  if (!props.endpoint) {
    emit('update:checked', checked)
    emit('change', checked, event)
    return
  }
  const endpoint = props.endpoint
  pending.value = true
  try {
    const { default: request } = await import('@/util/request')
    const response = await request.patch(endpoint, { enabled: checked })
    if (response?.data?.code !== 200 || typeof response.data.data?.enabled !== 'boolean') {
      throw new Error(response?.data?.msg && response.data.msg !== 'success' ? response.data.msg : '状态更新响应异常')
    }
    // A reused row may now represent another resource; its state must not receive this older response.
    if (props.endpoint !== endpoint) return
    if (response.data.data.enabled !== checked) {
      throw new Error('状态未按请求更新，请刷新后重试')
    }
    confirmed.value = response.data.data.enabled
    emit('update:checked', confirmed.value)
    emit('change', confirmed.value, event)
    emit('changed', confirmed.value)
    message.success(confirmed.value ? '已启用' : '已停用')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '状态更新失败')
  } finally {
    pending.value = false
  }
}
</script>

<style scoped>
.enabled-switch {
  display: inline-flex;
  vertical-align: middle;
}
</style>
