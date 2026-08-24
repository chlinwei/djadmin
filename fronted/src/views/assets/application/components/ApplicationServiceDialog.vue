<template>
  <a-modal
    :open="open"
    :title="serviceId ? `编辑逻辑服务：${form.name || '加载中'}` : clusterProfileId ? '创建集群' : '新增逻辑服务'"
    :width="760"
    :confirm-loading="saving"
    ok-text="保存"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12"><a-form-item name="name" label="服务名称"><a-input v-model:value="form.name" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="code" label="服务编码"><a-input v-model:value="form.code" placeholder="例如 order-cache-prod" /></a-form-item></a-col>
          <a-col :span="12">
            <a-form-item name="business_system" label="业务系统">
              <a-select v-model:value="form.business_system" show-search :filter-option="filterOption" :options="businessSystemOptions" :getPopupContainer="getPopupContainer">
                <template #notFoundContent>
                  <div class="inline-create-empty"><span>还没有可用业务系统</span><a-button type="link" @click.stop="businessSystemDialogOpen = true"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;新建业务系统</a-button></div>
                </template>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="environment" label="环境">
              <a-select v-model:value="form.environment" :options="environmentOptions" :disabled="!form.business_system" :placeholder="form.business_system ? '请选择环境' : '请先选择业务系统'" :getPopupContainer="getPopupContainer">
                <template #notFoundContent>
                  <div class="inline-create-empty"><span>当前业务系统还没有环境</span><a-button type="link" @click.stop="environmentDialogOpen = true"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;新建环境</a-button></div>
                </template>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col v-if="!clusterProfileId" :span="12"><a-form-item name="topology_type" label="部署形态"><a-segmented v-model:value="form.topology_type" :options="topologyOptions" /></a-form-item></a-col>
          <a-col v-if="form.topology_type === 'cluster'" :span="12">
            <a-form-item name="cluster_profile" label="集群模型">
              <a-select v-model:value="form.cluster_profile" show-search :filter-option="filterOption" :options="availableProfileOptions" :getPopupContainer="getPopupContainer">
                <template #notFoundContent>
                  <div class="inline-create-empty"><span>还没有可用集群模型</span><a-button type="link" @click.stop="clusterProfileDialogOpen = true"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;新建集群模型</a-button></div>
                </template>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col v-if="form.topology_type === 'standalone' || !profileHasFixedApplication" :span="12">
            <a-form-item name="application" label="应用">
              <a-select v-model:value="form.application" show-search :filter-option="filterOption" :options="applicationOptions" :getPopupContainer="getPopupContainer">
                <template #notFoundContent>
                  <div class="inline-create-empty"><span>还没有可用应用</span><a-button type="link" @click.stop="applicationDialogOpen = true"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;新建应用</a-button></div>
                </template>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col v-else :span="12"><a-form-item label="应用"><a-input :value="selectedProfile?.application_name || ''" disabled /></a-form-item></a-col>
          <a-col :span="12">
            <a-form-item name="application_version" label="应用版本">
              <a-select v-model:value="form.application_version" show-search :filter-option="filterOption" :options="versionOptions" :getPopupContainer="getPopupContainer">
                <template #notFoundContent>
                  <div class="inline-create-empty">
                    <span>{{ form.application ? '当前应用还没有可用版本' : '请先选择应用' }}</span>
                    <a-button type="link" :disabled="!form.application" @click.stop="openVersionCreator">
                      <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;新建应用版本
                    </a-button>
                  </div>
                </template>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item name="deployment_template" label="部署模板">
              <a-select v-model:value="form.deployment_template" show-search :filter-option="filterOption" :options="templateOptions" :getPopupContainer="getPopupContainer" :placeholder="form.application ? '请选择部署模板' : '请先选择应用'">
                <template #notFoundContent>
                  <div class="inline-create-empty">
                    <span>{{ form.application ? '当前应用还没有可用部署模板' : '请先选择应用' }}</span>
                    <a-button type="link" :disabled="!form.application" @click.stop="openTemplateCreator">
                      <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;新建部署模板
                    </a-button>
                  </div>
                </template>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col v-if="form.topology_type !== 'standalone'" :span="12"><a-form-item name="access_address" :label="isHaCluster ? 'VIP' : form.topology_type === 'load_balancer' ? '负载均衡地址' : '入口地址'"><a-input v-model:value="form.access_address" :placeholder="isHaCluster ? '请输入 HA 集群 VIP' : form.topology_type === 'load_balancer' ? '请输入负载均衡地址' : 'IP 或入口地址'" /></a-form-item></a-col>
          <a-col :span="24">
            <a-form-item :label="isHaCluster ? '成员实例（至少 2 个）' : form.topology_type === 'load_balancer' ? '后端成员实例' : '部署实例'" required>
              <a-button class="add-deployment-button" @click="openDeploymentDialog()">新增部署实例</a-button>
            </a-form-item>
          </a-col>
          <a-col v-for="deploymentId in selectedDeploymentIds" :key="deploymentId" :span="24">
            <div class="member-row">
              <span class="member-name">{{ deploymentLabel(deploymentId) }}</span>
              <a-switch v-model:checked="memberEnabled[deploymentId]" checked-children="启用" un-checked-children="停用" />
              <a-space>
                <a-tooltip title="编辑"><a-button size="small" @click="openDeploymentDialog(deploymentId)">编辑</a-button></a-tooltip>
                <a-tooltip title="删除"><a-button class="delBtn" danger size="small" @click="confirmDeleteDeployment(deploymentId)">删除</a-button></a-tooltip>
              </a-space>
            </div>
          </a-col>
          <a-col :span="12"><a-form-item label="启用"><a-switch v-model:checked="form.enabled" /></a-form-item></a-col>
          <a-col :span="24"><a-form-item label="备注"><a-textarea v-model:value="form.remark" :rows="3" /></a-form-item></a-col>
        </a-row>
      </a-form>
    </a-spin>
    <DeploymentDialog
      :open="deploymentDialogOpen"
      :deployment-id="selectedDeploymentId"
      :application-service-id="form.id"
      @update:open="deploymentDialogOpen = $event"
      @saved="handleDeploymentSaved"
    />
    <BusinessSystemDialog
      :open="businessSystemDialogOpen"
      @update:open="businessSystemDialogOpen = $event"
      @saved="handleBusinessSystemCreated"
    />
    <BusinessEnvironmentDialog
      :open="environmentDialogOpen"
      :business-system-id="form.business_system"
      @update:open="environmentDialogOpen = $event"
      @saved="handleEnvironmentCreated"
    />
    <ClusterProfileDialog
      :open="clusterProfileDialogOpen"
      :initial-application-id="form.application"
      @update:open="clusterProfileDialogOpen = $event"
      @saved="handleClusterProfileCreated"
    />
    <Dialog
      :open="applicationDialogOpen"
      :item_id="-1"
      title="新增应用"
      appname="应用"
      @update:open="applicationDialogOpen = $event"
      @saved="handleApplicationCreated"
    />
    <VersionDialog
      :open="versionDialogOpen"
      :application="selectedApplication"
      @update:open="versionDialogOpen = $event"
      @created="handleVersionCreated"
    />
    <TemplateDialog
      :open="templateDialogOpen"
      :initial-application-id="form.application"
      @update:open="templateDialogOpen = $event"
      @saved="handleTemplateCreated"
    />
  </a-modal>
</template>

<script setup>
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import DeploymentDialog from './DeploymentDialog.vue'
import BusinessEnvironmentDialog from './BusinessEnvironmentDialog.vue'
import BusinessSystemDialog from './BusinessSystemDialog.vue'
import ClusterProfileDialog from './ClusterProfileDialog.vue'
import Dialog from './Dialog.vue'
import TemplateDialog from './TemplateDialog.vue'
import VersionDialog from './VersionDialog.vue'
import {
  getApplicationDeploymentList,
  getApplicationDeploymentTemplateList,
  getApplicationList,
  getApplicationService,
  getApplicationVersionList,
  getBusinessEnvironmentList,
  getBusinessSystemList,
  getClusterProfileList,
  saveApplicationService,
} from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, required: true },
  serviceId: { type: Number, default: null },
  clusterProfileId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const deploymentDialogOpen = ref(false)
const businessSystemDialogOpen = ref(false)
const environmentDialogOpen = ref(false)
const clusterProfileDialogOpen = ref(false)
const applicationDialogOpen = ref(false)
const versionDialogOpen = ref(false)
const templateDialogOpen = ref(false)
const selectedDeploymentId = ref(null)
const businessSystemOptions = ref([])
const applicationOptions = ref([])
const profileRecords = ref([])
const deploymentRecords = ref([])
const versionRecords = ref([])
const templateRecords = ref([])
const selectedDeploymentIds = ref([])
const memberEnabled = reactive({})
const environmentRecords = ref([])
const topologyOptions = [{ label: '单机', value: 'standalone' }, { label: '集群', value: 'cluster' }, { label: '负载均衡', value: 'load_balancer' }]
const initialForm = () => ({
  id: null, business_system: null, application: null, application_version: null, deployment_template: null, cluster_profile: null,
  name: '', code: '', environment: null, topology_type: 'standalone',
  access_address: '',
  enabled: true, remark: '',
})
const form = reactive(initialForm())
// business_system 仅用于缩小环境选项范围，实际提交的归属字段只有 environment。
const environmentOptions = computed(() => environmentRecords.value
  .filter((item) => item.enabled && item.business_system === form.business_system)
  .map((item) => ({ label: item.name, value: item.id })))
const selectedApplication = computed(() => {
  const option = applicationOptions.value.find((item) => item.value === form.application)
  return option ? { id: option.value, name: option.label } : null
})
const selectedProfile = computed(() => profileRecords.value.find((item) => item.id === form.cluster_profile))
const profileHasFixedApplication = computed(() => Boolean(selectedProfile.value?.application))
const isHaCluster = computed(() => selectedProfile.value?.cluster_type === 'ha')
const rules = computed(() => ({
  name: [{ required: true, message: '请输入服务名称' }],
  code: [
    { required: true, message: '请输入服务编码' },
    { pattern: /^[a-z0-9][a-z0-9_-]*$/, message: '编码仅支持小写字母、数字、下划线和连字符' },
  ],
  business_system: [{ required: true, message: '请选择业务系统' }],
  application: form.topology_type === 'standalone' || !profileHasFixedApplication.value ? [{ required: true, message: '请选择应用' }] : [],
  application_version: [{ required: true, message: '请选择应用版本' }],
  deployment_template: [{ required: true, message: '请选择部署模板' }],
  environment: [{ required: true, message: '请选择环境' }],
  topology_type: [{ required: true, message: '请选择部署形态' }],
  cluster_profile: form.topology_type === 'cluster' ? [{ required: true, message: '请选择集群模型' }] : [],
  access_address: ['cluster', 'load_balancer'].includes(form.topology_type) && (isHaCluster.value || form.topology_type === 'load_balancer')
    ? [{ required: true, message: isHaCluster.value ? '请输入 VIP' : '请输入负载均衡地址' }]
    : [],
}))
const availableProfileOptions = computed(() => profileRecords.value
  .filter((item) => item.enabled)
  .map((item) => ({ label: item.name, value: item.id })))
const versionOptions = computed(() => versionRecords.value
  .filter((item) => item.application === form.application)
  .map((item) => ({ label: item.version, value: item.id })))
const templateOptions = computed(() => templateRecords.value
  .filter((item) => item.application === form.application && item.enabled)
  .filter((item) => isHaCluster.value ? item.control_type === 'external_ha' : item.control_type !== 'external_ha')
  .map((item) => ({ label: item.name, value: item.id })))
const availableDeploymentOptions = computed(() => deploymentRecords.value
  .filter((item) => item.application_id === form.application)
  .map((item) => ({ label: `${item.instance_name} (${item.host_name || item.host_ip || '-'})`, value: item.id })))
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())

function deploymentLabel(deploymentId) {
  return availableDeploymentOptions.value.find((item) => item.value === deploymentId)?.label || `实例 ${deploymentId}`
}

function openVersionCreator() {
  if (!form.application) {
    message.warning('请先选择应用')
    return
  }
  versionDialogOpen.value = true
}

function handleBusinessSystemCreated(system) {
  if (!system?.id) return
  businessSystemOptions.value = [...businessSystemOptions.value.filter((item) => item.value !== system.id), { label: system.name, value: system.id }]
  form.business_system = system.id
  businessSystemDialogOpen.value = false
  nextTick(() => formRef.value?.clearValidate('business_system'))
}

function handleEnvironmentCreated(environment) {
  if (!environment?.id) return
  environmentRecords.value = [...environmentRecords.value.filter((item) => item.id !== environment.id), environment]
  form.environment = environment.id
  environmentDialogOpen.value = false
  nextTick(() => formRef.value?.clearValidate('environment'))
}

function handleApplicationCreated(application) {
  if (!application?.id) return
  applicationOptions.value = [...applicationOptions.value.filter((item) => item.value !== application.id), { label: application.name, value: application.id }]
  form.application = application.id
  applicationDialogOpen.value = false
  nextTick(() => formRef.value?.clearValidate('application'))
}

function handleClusterProfileCreated(profile) {
  if (!profile?.id) return
  profileRecords.value = [...profileRecords.value.filter((item) => item.id !== profile.id), profile]
  form.cluster_profile = profile.id
  if (profile.application) form.application = profile.application
  clusterProfileDialogOpen.value = false
  nextTick(() => formRef.value?.clearValidate('cluster_profile'))
}

function openTemplateCreator() {
  if (!form.application) {
    message.warning('请先选择应用')
    return
  }
  templateDialogOpen.value = true
}

function handleVersionCreated(version) {
  if (!version?.id) return
  versionRecords.value = [...versionRecords.value.filter((item) => item.id !== version.id), version]
  form.application_version = version.id
  versionDialogOpen.value = false
  nextTick(() => formRef.value?.clearValidate('application_version'))
}

function handleTemplateCreated(template) {
  if (!template?.id) return
  templateRecords.value = [...templateRecords.value.filter((item) => item.id !== template.id), template]
  form.deployment_template = template.id
  templateDialogOpen.value = false
  nextTick(() => formRef.value?.clearValidate('deployment_template'))
}

async function openDeploymentDialog(deploymentId = null) {
  // 部署实例强依赖服务上的版本与模板，缺失时先在本表单补齐，避免子弹窗提交必然失败。
  await formRef.value.validate(['application_version', 'deployment_template'])
  if (!form.id) {
    await formRef.value.validate([
      'name', 'code', 'business_system', 'application', 'environment',
      'topology_type', 'application_version', 'deployment_template',
    ])
    const payload = { ...form, draft: true }
    delete payload.business_system
    payload.cluster_profile = payload.topology_type === 'cluster' ? payload.cluster_profile : null
    const response = await saveApplicationService(payload)
    Object.assign(form, response?.data?.data || {})
    emit('saved')
  }
  await nextTick()
  formRef.value?.clearValidate()
  selectedDeploymentId.value = deploymentId
  deploymentDialogOpen.value = true
}

function handleDeploymentSaved(deployment) {
  if (!deployment?.id) return
  if (deploymentRecords.value.some((item) => item.id === deployment.id)) {
    deploymentRecords.value = deploymentRecords.value.map((item) => item.id === deployment.id ? { ...item, ...deployment } : item)
  } else {
    deploymentRecords.value = [...deploymentRecords.value, deployment]
    selectedDeploymentIds.value = [...selectedDeploymentIds.value, deployment.id]
  }
  memberEnabled[deployment.id] = deployment.enabled !== false
  selectedDeploymentId.value = null
}

function confirmDeleteDeployment(deploymentId) {
  const deployment = deploymentRecords.value.find((item) => item.id === deploymentId)
  openDeleteConfirm({
    title: '删除部署实例',
    summary: '删除后该实例将从当前逻辑服务中移除。',
    items: [deploymentLabel(deploymentId)],
    onConfirm: async () => {
      selectedDeploymentIds.value = selectedDeploymentIds.value.filter((id) => id !== deploymentId)
      delete memberEnabled[deploymentId]
      message.success(`${deployment?.instance_name || '部署实例'}已从当前逻辑服务移除`)
    },
  })
}

async function fetchAll(loader, params = {}) {
  const response = await loader({ ...params, page: 1, page_size: 1000 })
  return response?.data?.data?.results || []
}

async function initialize() {
  Object.assign(form, initialForm())
  loading.value = true
  try {
    const [systems, environments, applications, versions, templates, profiles, deployments] = await Promise.all([
      fetchAll(getBusinessSystemList, { enabled: true }),
      fetchAll(getBusinessEnvironmentList, { enabled: true }),
      fetchAll(getApplicationList, { enabled: true }),
      fetchAll(getApplicationVersionList, { enabled: true }),
      fetchAll(getApplicationDeploymentTemplateList, { enabled: true }),
      fetchAll(getClusterProfileList, { enabled: true }),
      fetchAll(getApplicationDeploymentList),
    ])
    businessSystemOptions.value = systems.map((item) => ({ label: item.name, value: item.id }))
    environmentRecords.value = environments
    applicationOptions.value = applications.map((item) => ({ label: item.name, value: item.id }))
    versionRecords.value = versions
    templateRecords.value = templates
    profileRecords.value = profiles
    deploymentRecords.value = deployments
    if (props.serviceId) {
      const response = await getApplicationService(props.serviceId)
      const data = response?.data?.data || {}
      Object.assign(form, initialForm(), data)
      selectedDeploymentIds.value = (data.member_instances || []).map((item) => item.deployment)
      for (const item of data.member_instances || []) {
        memberEnabled[item.deployment] = item.enabled !== false
      }
    } else if (props.clusterProfileId) {
      form.topology_type = 'cluster'
      form.cluster_profile = props.clusterProfileId
      form.application = selectedProfile.value?.application || null
    }
    await nextTick()
    formRef.value?.clearValidate()
  } finally {
    loading.value = false
  }
}

async function submit() {
  await formRef.value.validate()
  const minimumMemberCount = isHaCluster.value ? 2 : 1
  if (selectedDeploymentIds.value.length < minimumMemberCount) {
    message.error(isHaCluster.value ? 'HA 集群至少需要两个成员实例' : form.topology_type === 'standalone' ? '请选择部署实例' : '请选择至少一个后端成员实例')
    return
  }
  saving.value = true
  try {
    const payload = { ...form }
    delete payload.business_system
    if (payload.topology_type === 'standalone') {
      payload.cluster_profile = null
    }
    if (payload.topology_type === 'load_balancer') {
      payload.cluster_profile = null
    }
    payload.member_configs = selectedDeploymentIds.value.map((deployment) => ({
      deployment,
      enabled: memberEnabled[deployment] !== false,
    }))
    await saveApplicationService(payload)
    message.success('保存成功')
    emit('update:open', false)
    emit('saved')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存逻辑服务失败')
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => {
  if (visible) {
    deploymentDialogOpen.value = false
    selectedDeploymentIds.value = []
    for (const key of Object.keys(memberEnabled)) delete memberEnabled[key]
    initialize()
  }
})
watch(() => form.business_system, () => {
  // 切换业务系统后原环境已不在可选范围内，必须清空避免提交到别的系统下的环境。
  if (form.environment && !environmentOptions.value.some((item) => item.value === form.environment)) {
    form.environment = null
  }
})
watch(() => form.application, () => {
  if (form.application_version && !versionOptions.value.some((item) => item.value === form.application_version)) {
    form.application_version = null
  }
  if (form.deployment_template && !templateOptions.value.some((item) => item.value === form.deployment_template)) {
    form.deployment_template = null
  }
  selectedDeploymentIds.value = selectedDeploymentIds.value.filter((id) => availableDeploymentOptions.value.some((item) => item.value === id))
})
watch(() => form.topology_type, (topology) => {
  if (topology === 'standalone') {
    form.cluster_profile = null
    form.access_address = ''
  } else if (topology === 'load_balancer') {
    form.cluster_profile = null
  } else if (selectedProfile.value?.application) {
    form.application = selectedProfile.value.application
  }
  if (form.deployment_template && !templateOptions.value.some((item) => item.value === form.deployment_template)) {
    form.deployment_template = null
  }
})
watch(() => form.cluster_profile, () => {
  if (form.topology_type === 'cluster' && selectedProfile.value?.application) {
    form.application = selectedProfile.value.application
  }
  selectedDeploymentIds.value = selectedDeploymentIds.value.filter((id) => availableDeploymentOptions.value.some((item) => item.value === id))
})
</script>

<style scoped>
.inline-create-empty {
  display: flex;
  min-height: 72px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: rgba(0, 0, 0, 0.45);
}
.member-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto 180px auto;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fafafa;
}
.member-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
