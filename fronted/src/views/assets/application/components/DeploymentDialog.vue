<template>
  <a-modal
    :open="open"
    :title="deploymentId ? `编辑部署实例：${form.instance_name || '加载中'}` : '登记部署实例'"
    :width="820"
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
              <a-col :span="12"><a-form-item name="instance_name" label="实例名称"><a-input v-model:value="form.instance_name" placeholder="例如 tomcat-order-prod" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="启用"><a-switch v-model:checked="form.enabled" /></a-form-item></a-col>
              <a-col :span="24"><a-form-item label="备注"><a-textarea v-model:value="form.remark" :rows="3" /></a-form-item></a-col>
        </a-row>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import { nextTick, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { fetchAllPages } from '@/util/fetchAllPages'
import { getHostList } from '@/api/assets/host'
import {
  getApplicationDeployment,
  getApplicationDeploymentList,
  getApplicationService,
  saveApplicationDeployment,
} from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, required: true },
  deploymentId: { type: Number, default: null },
  applicationServiceId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const hostOptions = ref([])
const serviceEnvironmentId = ref(null)
const createInitialForm = () => ({
  host: null,
  application_service: null,
  instance_name: '',
  enabled: true,
  remark: '',
})
const form = reactive(createInitialForm())
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
  Object.assign(form, createInitialForm())
}

async function loadOptions() {
  const hosts = await fetchAllPages(getHostList, serviceEnvironmentId.value ? { environment: serviceEnvironmentId.value } : {}, 30)
  hostOptions.value = hosts.map((item) => ({
    label: `${item.instance_name || '-'} (${item.ip || '-'})`,
    value: item.id,
    instanceName: item.instance_name || '',
  }))
}

async function initialize() {
  resetForm()
  serviceEnvironmentId.value = null
  form.application_service = props.applicationServiceId
  loading.value = true
  applyingRecord.value = true
  try {
    if (props.applicationServiceId) {
      const serviceResponse = await getApplicationService(props.applicationServiceId)
      serviceEnvironmentId.value = serviceResponse?.data?.data?.environment || null
    } else if (props.deploymentId) {
      const deploymentResponse = await getApplicationDeployment(props.deploymentId)
      serviceEnvironmentId.value = deploymentResponse?.data?.data?.environment || null
    }
    await loadOptions()
    if (props.deploymentId) {
      const response = await getApplicationDeployment(props.deploymentId)
      const data = response?.data?.data || {}
      Object.assign(form, createInitialForm(), data)
      if (!form.instance_name) {
        form.instance_name = data.host_name || getHostInstanceName(form.host)
      }
    }
    await nextTick()
  } finally {
    applyingRecord.value = false
    loading.value = false
  }
}

async function submit() {
  await formRef.value?.validate()
  const payload = { ...form }
  payload.id = props.deploymentId || undefined

  saving.value = true
  try {
    if (!payload.id && payload.host && payload.instance_name) {
      const response = await getApplicationDeploymentList({ host: payload.host, page: 1, page_size: 1000 })
      const existingDeployment = (response?.data?.data?.results || []).find(
        (item) => String(item.instance_name || '').trim() === String(payload.instance_name).trim(),
      )
      if (existingDeployment?.id) payload.id = existingDeployment.id
    }
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

watch(() => props.open, (visible) => {
  if (visible) initialize()
})
watch(() => form.host, (hostId, previousHostId) => {
  if (applyingRecord.value) return
  // 仅在名称为空或仍是上一台主机的默认值时跟随主机，保留用户自定义名称。
  if (form.instance_name && form.instance_name !== getHostInstanceName(previousHostId)) return
  form.instance_name = getHostInstanceName(hostId)
})
</script>

<style scoped>
</style>
