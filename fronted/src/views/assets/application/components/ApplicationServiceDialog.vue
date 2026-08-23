<template>
  <a-modal
    :open="open"
    :title="serviceId ? '编辑逻辑服务' : clusterProfileId ? '创建集群' : '新增逻辑服务'"
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
          <a-col :span="12"><a-form-item name="business_system" label="业务系统"><a-select v-model:value="form.business_system" show-search :filter-option="filterOption" :options="businessSystemOptions" :getPopupContainer="getPopupContainer" /></a-form-item></a-col>
          <a-col v-if="form.topology_type === 'standalone' || !profileHasFixedApplication" :span="12"><a-form-item name="application" label="应用"><a-select v-model:value="form.application" show-search :filter-option="filterOption" :options="applicationOptions" :getPopupContainer="getPopupContainer" /></a-form-item></a-col>
          <a-col v-else :span="12"><a-form-item label="应用"><a-input :value="selectedProfile?.application_name || ''" disabled /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="environment" label="环境"><a-select v-model:value="form.environment" :options="environmentOptions" :getPopupContainer="getPopupContainer" /></a-form-item></a-col>
          <a-col v-if="!clusterProfileId" :span="12"><a-form-item name="topology_type" label="部署形态"><a-segmented v-model:value="form.topology_type" :options="topologyOptions" /></a-form-item></a-col>
          <a-col v-if="form.topology_type === 'cluster'" :span="12"><a-form-item name="cluster_profile" label="集群模型"><a-select v-model:value="form.cluster_profile" show-search :filter-option="filterOption" :options="availableProfileOptions" :getPopupContainer="getPopupContainer" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="availability_mode" label="可用性"><a-select v-model:value="form.availability_mode" :options="availabilityOptions" :getPopupContainer="getPopupContainer" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="access_type" label="访问方式"><a-select v-model:value="form.access_type" :disabled="isHaCluster" :options="accessOptions" :getPopupContainer="getPopupContainer" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item name="access_address" :label="isHaCluster ? 'VIP' : '入口地址'"><a-input v-model:value="form.access_address" :placeholder="isHaCluster ? '请输入 HA 集群 VIP' : 'IP 或负载均衡地址'" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="入口端口"><a-input-number v-model:value="form.access_port" :min="1" :max="65535" style="width: 100%" /></a-form-item></a-col>
          <a-col v-if="form.topology_type === 'cluster'" :span="24">
            <a-form-item :label="isHaCluster ? '成员实例（至少 2 个）' : '成员实例'" required>
              <a-select v-model:value="selectedDeploymentIds" mode="multiple" show-search :filter-option="filterOption" :options="availableDeploymentOptions" :getPopupContainer="getPopupContainer" placeholder="选择加入集群的部署实例" />
            </a-form-item>
          </a-col>
          <a-col v-for="deploymentId in selectedDeploymentIds" :key="deploymentId" :span="24">
            <div class="member-row">
              <span class="member-name">{{ deploymentLabel(deploymentId) }}</span>
              <a-input-number v-if="!isHaCluster" v-model:value="memberPorts[deploymentId]" :min="1" :max="65535" placeholder="成员端口" />
              <a-radio
                v-else
                :checked="form.primary_deployment === deploymentId"
                @change="form.primary_deployment = deploymentId"
              >
                VIP 主节点
              </a-radio>
            </div>
          </a-col>
          <a-col :span="12"><a-form-item label="启用"><a-switch v-model:checked="form.enabled" /></a-form-item></a-col>
          <a-col :span="24"><a-form-item label="备注"><a-textarea v-model:value="form.remark" :rows="3" /></a-form-item></a-col>
        </a-row>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import {
  getApplicationDeploymentList,
  getApplicationList,
  getApplicationService,
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
const businessSystemOptions = ref([])
const applicationOptions = ref([])
const profileRecords = ref([])
const deploymentRecords = ref([])
const selectedDeploymentIds = ref([])
const memberPorts = reactive({})
const environmentOptions = [
  { label: '生产', value: 'production' }, { label: '测试', value: 'testing' },
  { label: '开发', value: 'development' }, { label: '其他', value: 'other' },
]
const topologyOptions = [{ label: '单机', value: 'standalone' }, { label: '集群', value: 'cluster' }]
const availabilityOptions = [
  { label: '无高可用', value: 'none' }, { label: '主备', value: 'active_standby' }, { label: '双活', value: 'active_active' },
]
const accessOptions = [
  { label: '节点地址', value: 'direct' }, { label: 'VIP', value: 'vip' }, { label: '负载均衡', value: 'load_balancer' },
]
const initialForm = () => ({
  id: null, business_system: null, application: null, cluster_profile: null,
  primary_deployment: null,
  name: '', code: '', environment: 'production', topology_type: 'standalone',
  availability_mode: 'none', access_type: 'direct', access_address: '', access_port: null,
  enabled: true, remark: '',
})
const form = reactive(initialForm())
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
  environment: [{ required: true, message: '请选择环境' }],
  topology_type: [{ required: true, message: '请选择部署形态' }],
  cluster_profile: form.topology_type === 'cluster' ? [{ required: true, message: '请选择集群模型' }] : [],
  access_address: isHaCluster.value ? [{ required: true, message: '请输入 HA 集群 VIP' }] : [],
}))
const availableProfileOptions = computed(() => profileRecords.value
  .filter((item) => item.enabled)
  .map((item) => ({ label: item.name, value: item.id })))
const availableDeploymentOptions = computed(() => deploymentRecords.value
  .filter((item) => item.application_id === form.application)
  .map((item) => ({ label: `${item.instance_name} (${item.host_name || item.host_ip || '-'})`, value: item.id })))
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())

function deploymentLabel(deploymentId) {
  return availableDeploymentOptions.value.find((item) => item.value === deploymentId)?.label || `实例 ${deploymentId}`
}

async function fetchAll(loader, params = {}) {
  const response = await loader({ ...params, page: 1, page_size: 1000 })
  return response?.data?.data?.results || []
}

async function initialize() {
  Object.assign(form, initialForm())
  loading.value = true
  try {
    const [systems, applications, profiles, deployments] = await Promise.all([
      fetchAll(getBusinessSystemList, { enabled: true }),
      fetchAll(getApplicationList, { enabled: true }),
      fetchAll(getClusterProfileList, { enabled: true }),
      fetchAll(getApplicationDeploymentList),
    ])
    businessSystemOptions.value = systems.map((item) => ({ label: item.name, value: item.id }))
    applicationOptions.value = applications.map((item) => ({ label: item.name, value: item.id }))
    profileRecords.value = profiles
    deploymentRecords.value = deployments
    if (props.serviceId) {
      const response = await getApplicationService(props.serviceId)
      const data = response?.data?.data || {}
      Object.assign(form, initialForm(), data)
      selectedDeploymentIds.value = (data.member_instances || []).map((item) => item.deployment)
      for (const item of data.member_instances || []) memberPorts[item.deployment] = item.port
    } else if (props.clusterProfileId) {
      form.topology_type = 'cluster'
      form.cluster_profile = props.clusterProfileId
      form.application = selectedProfile.value?.application || null
    }
  } finally {
    loading.value = false
  }
}

async function submit() {
  await formRef.value.validate()
  const minimumMemberCount = isHaCluster.value ? 2 : 1
  if (form.topology_type === 'cluster' && selectedDeploymentIds.value.length < minimumMemberCount) {
    message.error(isHaCluster.value ? 'HA 集群至少需要两个成员实例' : '请选择至少一个成员实例')
    return
  }
  if (isHaCluster.value && !selectedDeploymentIds.value.includes(form.primary_deployment)) {
    message.error('请选择 VIP 所在的主节点')
    return
  }
  if (form.topology_type === 'cluster' && !isHaCluster.value && selectedDeploymentIds.value.some((id) => !memberPorts[id])) {
    message.error('请为每个集群成员填写端口')
    return
  }
  saving.value = true
  try {
    const payload = { ...form }
    if (payload.topology_type === 'standalone') payload.cluster_profile = null
    if (!isHaCluster.value) payload.primary_deployment = null
    if (payload.topology_type === 'cluster') {
      payload.member_configs = selectedDeploymentIds.value.map((deployment) => ({
        deployment,
        port: isHaCluster.value ? null : memberPorts[deployment],
      }))
    }
    await saveApplicationService(payload)
    message.success('保存成功')
    emit('update:open', false)
    emit('saved')
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (visible) => {
  if (visible) {
    selectedDeploymentIds.value = []
    for (const key of Object.keys(memberPorts)) delete memberPorts[key]
    initialize()
  }
})
watch(() => form.topology_type, (topology) => {
  if (topology === 'standalone') {
    form.cluster_profile = null
  } else if (selectedProfile.value?.application) {
    form.application = selectedProfile.value.application
  }
})
watch(() => form.cluster_profile, () => {
  if (form.topology_type === 'cluster' && selectedProfile.value?.application) {
    form.application = selectedProfile.value.application
  }
  selectedDeploymentIds.value = selectedDeploymentIds.value.filter((id) => availableDeploymentOptions.value.some((item) => item.value === id))
})
watch(selectedDeploymentIds, (deploymentIds) => {
  if (!deploymentIds.includes(form.primary_deployment)) form.primary_deployment = null
}, { deep: true })
watch(isHaCluster, (isHa) => {
  if (isHa) {
    form.access_type = 'vip'
  } else {
    form.primary_deployment = null
  }
})
</script>

<style scoped>
.member-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 180px;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fafafa;
}
.member-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
