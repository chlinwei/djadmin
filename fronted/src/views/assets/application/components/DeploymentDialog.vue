<template>
  <a-modal
    :open="open"
    :title="deploymentId ? '编辑部署实例' : '登记部署实例'"
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
                <a-form-item name="application_version" label="应用版本">
                  <a-select
                    v-model:value="form.application_version"
                    show-search
                    :filter-option="filterOption"
                    :options="versionOptions"
                    :getPopupContainer="getPopupContainer"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item name="deployment_template" label="部署模板">
                  <a-select
                    v-model:value="form.deployment_template"
                    show-search
                    :filter-option="filterOption"
                    :options="availableTemplateOptions"
                    :getPopupContainer="getPopupContainer"
                    placeholder="请先选择应用版本"
                  />
                </a-form-item>
              </a-col>
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
        <a-alert v-if="form.application_version && !availableTemplateOptions.length" type="warning" show-icon message="该应用还没有可用部署模板，请先在应用定义中创建模板。" />
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { getHostList } from '@/api/assets/host'
import {
  getApplicationDeployment,
  getApplicationDeploymentTemplateList,
  getApplicationVersionList,
  saveApplicationDeployment,
} from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, required: true },
  deploymentId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const hostOptions = ref([])
const versionOptions = ref([])
const versionRecords = ref([])
const templateRecords = ref([])
const createInitialForm = () => ({
  application_version: null,
  deployment_template: null,
  host: null,
  instance_name: '',
  enabled: true,
  remark: '',
})
const form = reactive(createInitialForm())
const rules = {
  application_version: [{ required: true, message: '请选择应用版本' }],
  deployment_template: [{ required: true, message: '请选择部署模板' }],
  host: [{ required: true, message: '请选择主机' }],
  instance_name: [{ required: true, message: '请输入实例名称' }],
}

const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())
const availableTemplateOptions = computed(() => {
  const version = versionRecords.value.find((item) => item.id === form.application_version)
  if (!version) return []
  return templateRecords.value
    .filter((item) => item.application === version.application && (item.enabled || item.id === form.deployment_template))
    .map((item) => ({ label: item.name, value: item.id }))
})
function resetForm() {
  Object.assign(form, createInitialForm())
}

async function loadOptions() {
  const fetchAllPages = async (loader, params = {}) => {
    const firstResponse = await loader({ ...params, page: 1, page_size: 30 })
    const firstData = firstResponse?.data?.data || {}
    const records = [...(firstData.results || [])]
    const totalPages = Number(firstData.totalPages || 1)
    if (totalPages > 1) {
      const responses = await Promise.all(
        Array.from({ length: totalPages - 1 }, (_, index) => loader({ ...params, page: index + 2, page_size: 30 })),
      )
      for (const response of responses) records.push(...(response?.data?.data?.results || []))
    }
    return records
  }
  const [hostsResponse, versionsResponse, templatesResponse] = await Promise.all([
    fetchAllPages(getHostList),
    fetchAllPages(getApplicationVersionList, { enabled: true }),
    fetchAllPages(getApplicationDeploymentTemplateList),
  ])
  const hosts = hostsResponse
  const versions = versionsResponse
  versionRecords.value = versions
  templateRecords.value = templatesResponse
  hostOptions.value = hosts.map((item) => ({ label: `${item.instance_name || '-'} (${item.ip || '-'})`, value: item.id }))
  versionOptions.value = versions.map((item) => ({ label: `${item.application_name} ${item.version}`, value: item.id }))
}

async function initialize() {
  resetForm()
  loading.value = true
  try {
    await loadOptions()
    if (props.deploymentId) {
      const response = await getApplicationDeployment(props.deploymentId)
      const data = response?.data?.data || {}
      Object.assign(form, createInitialForm(), data)
    }
  } finally {
    loading.value = false
  }
}

async function submit() {
  await formRef.value?.validate()
  const payload = { ...form }
  payload.id = props.deploymentId || undefined
  delete payload.application_service
  delete payload.member_port

  saving.value = true
  try {
    await saveApplicationDeployment(payload)
    message.success(props.deploymentId ? '部署实例更新成功' : '部署实例新增成功')
    emit('saved')
    emit('update:open', false)
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => {
  if (visible) initialize()
})
watch(() => form.application_version, () => {
  if (form.deployment_template && !availableTemplateOptions.value.some((item) => item.value === form.deployment_template)) {
    form.deployment_template = null
  }
})
</script>

<style scoped>
</style>
