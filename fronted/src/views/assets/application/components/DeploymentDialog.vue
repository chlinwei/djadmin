<template>
  <a-modal
    :open="open"
    :title="deploymentId ? `${readOnly ? '查看' : '编辑'}部署实例：${form.instance_name || '加载中'}` : '登记部署实例'"
    :width="820"
    :confirm-loading="saving"
    ok-text="保存"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <template v-if="readOnly" #footer><a-button @click="emit('update:open', false)">关闭</a-button></template>
    <a-spin :spinning="loading">
      <a-form :disabled="readOnly" ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item name="host" label="主机">
                  <a-select
                    v-model:value="form.host"
                    show-search
                    :filter-option="filterOption"
                    :options="hostOptions"
                    :getPopupContainer="getPopupContainer"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="12"><a-form-item name="instance_name" label="实例名称" extra="同一主机的不同程序应登记为不同实例。"><a-input v-model:value="form.instance_name" placeholder="例如 tomcat-order-prod" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="启用"><EnabledSwitch :read-only="readOnly" v-model:checked="form.enabled" /></a-form-item></a-col>
              <a-col :span="24"><a-form-item label="备注"><a-textarea v-model:value="form.remark" :rows="3" /></a-form-item></a-col>
        </a-row>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import EnabledSwitch from '@/components/EnabledSwitch.vue'
import { formFields } from './assetFormValues'
import { nextTick, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { fetchAllPages } from '@/util/fetchAllPages'
import { getHostById, getHostList } from '@/api/assets/host'
import {
  getApplicationDeployment,
  getApplicationService,
  saveApplicationDeployment,
} from '@/api/assets/application'

const props = defineProps({
  readOnly: Boolean,
  open: { type: Boolean, required: true },
  deploymentId: { type: Number, default: null },
  applicationServiceId: { type: Number, default: null },
  environmentId: { type: Number, default: undefined },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const hostOptions = ref([])
const serviceEnvironmentId = ref(null)
// 注意：这里**不提交逻辑服务关联**。实例与服务的绑定由服务侧维护（服务编辑弹窗的成员列表 →
// member_configs），实例侧改它会与"服务侧全量重建关联"的语义冲突（同一实例可能同时属于多个服务）。
// 原先这里带了一个 application_service 字段，后端结构体并不接收 → 静默忽略，
// 看起来"绑定成功"其实没有写库（2026-09-18 排查 yilake nginx 时发现）。
// 从服务弹窗点「新增部署实例」创建的场景仍会绑定：保存后由服务侧 handleDeploymentSaved
// 把新实例加入成员列表，再随服务一起保存。
const createInitialForm = () => ({
  host: null,
  instance_name: '',
  enabled: true,
  remark: '',
})
const form = reactive(createInitialForm())
// 实例更新接口整行保存；仅在编辑时保留不可见的运行字段，避免清空实例变量或把它们带入新增表单。
let preservedRuntime = {}
// 回填已有记录时会改写 host，需抑制主机联动，避免覆盖已保存的实例名。
const applyingRecord = ref(false)
const rules = {
  host: [{ required: true, message: '请选择主机' }],
  instance_name: [{ required: true, message: '请输入实例名称' }],
}

const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())
const getHostInstanceName = (hostId) => hostOptions.value.find(
  (item) => String(item.value) === String(hostId),
)?.instanceName || ''
function resetForm() {
  preservedRuntime = {}
  Object.assign(form, createInitialForm())
}

async function loadOptions(environment) {
  const hosts = await fetchAllPages(getHostList, environment ? { environment } : {}, 30)
  return hosts.map((item) => ({
    label: `${item.instance_name || '-'} (${item.ip || '-'})`,
    value: item.id,
    instanceName: item.instance_name || '',
  }))
}

async function initialize(isCurrent) {
  resetForm()
  hostOptions.value = []
  serviceEnvironmentId.value = null
  loading.value = true
  applyingRecord.value = true
  try {
    const response = props.deploymentId ? await getApplicationDeployment(props.deploymentId) : null
    if (!isCurrent()) return
    const data = response?.data?.data
    if (props.readOnly && data) {
      // 实例详情已有主机地址；查看不读取主机清单或携带运行变量进入表单。
      Object.assign(form, formFields(createInitialForm(), data))
      hostOptions.value = [{ value: data.host, label: [data.host_name, data.host_ip].filter(Boolean).join(' / ') || `主机 ${data.host}` }]
      return
    }
    if (props.readOnly) return
    // 服务表单的环境可能尚未保存，必须优先使用父表单值，不能重新读回旧环境。
    if (props.environmentId !== undefined) {
      serviceEnvironmentId.value = props.environmentId
    } else if (props.applicationServiceId) {
      const serviceResponse = await getApplicationService(props.applicationServiceId)
      if (!isCurrent()) return
      serviceEnvironmentId.value = serviceResponse?.data?.data?.environment || null
    } else if (data) {
      serviceEnvironmentId.value = data.environment || null
    }
    const hosts = await loadOptions(serviceEnvironmentId.value)
    if (!isCurrent()) return
    hostOptions.value = hosts
    if (data) {
      preservedRuntime = Object.fromEntries(['runtime_variables', 'runtime_status', 'runtime_status_output', 'ha_role']
        .filter(key => Object.hasOwn(data, key)).map(key => [key, data[key]]))
      Object.assign(form, formFields(createInitialForm(), data))
      if (data.host && !hostOptions.value.some((item) => String(item.value) === String(data.host))) {
        // 已绑定主机可能已移出当前环境：保留准确回显，但不把它作为新增候选。
        const hostResponse = await getHostById(data.host)
        if (!isCurrent()) return
        const host = hostResponse?.data?.data
        if (host?.id) hostOptions.value.push({
          value: host.id,
          label: `${host.instance_name || '-'} (${host.ip || '-'})`,
          instanceName: host.instance_name || '',
          disabled: true,
        })
      }
      if (!form.instance_name) {
        form.instance_name = data.host_name || getHostInstanceName(form.host)
      }
    }
    await nextTick()
  } finally {
    if (isCurrent()) {
      applyingRecord.value = false
      loading.value = false
    }
  }
}

async function submit() {
  if (props.readOnly || loading.value) return
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  const payload = { ...preservedRuntime, ...form }
  payload.id = props.deploymentId || undefined

  if (props.readOnly) return
  saving.value = true
  try {
    const response = await saveApplicationDeployment(payload)
    message.success(payload.id ? '部署实例更新成功' : '部署实例新增成功')
    emit('saved', response?.data?.data)
    emit('update:open', false)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存部署实例失败')
  } finally {
    saving.value = false
  }
}

watch([() => props.open, () => props.deploymentId, () => props.readOnly], ([visible], _, onCleanup) => {
  // 只接受本次打开的响应，慢查询不能把旧实例的主机或运行变量写进另一条记录。
  let current = true
  onCleanup(() => { current = false })
  if (visible) initialize(() => current)
}, { immediate: true })
watch(() => form.host, (hostId, previousHostId) => {
  if (applyingRecord.value) return
  // 仅在名称为空或仍是上一台主机的默认值时跟随主机，保留用户自定义名称。
  if (form.instance_name && form.instance_name !== getHostInstanceName(previousHostId)) return
  form.instance_name = getHostInstanceName(hostId)
})
</script>

<style scoped>
</style>
