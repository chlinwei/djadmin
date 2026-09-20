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
      <a-alert
        v-if="loadError"
        type="warning"
        show-icon
        :message="loadError"
        class="dialog-load-error"
      >
        <template #action>
          <a-button size="small" type="link" @click="initialize">重试</a-button>
        </template>
      </a-alert>
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
              <a-select v-model:value="form.environment" :options="environmentOptions" placeholder="请选择环境" :getPopupContainer="getPopupContainer">
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
          <a-col v-if="templateMacros.length" :span="24">
            <a-form-item label="模板宏">
              <a-alert
                v-if="missingMacros.length"
                type="warning"
                show-icon
                class="service-macro-alert"
                :message="`模板没给默认值的宏必须在这里填：${missingMacrosText}`"
                description="空着保存不了：下发时路径里的 ${VAR} 展不开，那台主机要么被跳过、要么拼出坏路径。"
              />
              <a-table
                :columns="macroTableColumns"
                :data-source="templateMacros"
                :pagination="false"
                row-key="name"
                size="small"
                :locale="tableLocale"
                class="service-macro-table"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'name'"><code>{{ macroKeyLabel(record.name) }}</code></template>
                  <template v-else-if="column.key === 'value'">
                    <a-input
                      :value="macroValue(record)"
                      :status="isMacroMissing(record) ? 'error' : ''"
                      :placeholder="record.value ? '' : '必填（模板未给默认值）'"
                      @update:value="setMacroValue(record.name, $event)"
                    />
                  </template>
                  <template v-else-if="column.key === 'description'">
                    <span>{{ record.description || '-' }}</span>
                    <!-- 模板没给值时不能显示"继承"——那是在骗人：根本没有可继承的东西。 -->
                    <a-tag v-if="isMacroMissing(record)" color="red">必填</a-tag>
                    <a-tag v-else-if="hasMacroOverride(record.name)" color="blue">已覆盖</a-tag>
                    <a-tag v-else color="default">继承</a-tag>
                  </template>
                  <template v-else-if="column.key === 'action'">
                    <a-button
                      type="link"
                      size="small"
                      @click="resetMacroValue(record.name)"
                    >
                      重置为默认
                    </a-button>
                  </template>
                </template>
              </a-table>
            </a-form-item>
          </a-col>
          <a-col v-if="form.topology_type !== 'standalone'" :span="12"><a-form-item name="access_address" :label="isHaCluster ? 'VIP' : form.topology_type === 'load_balancer' ? '负载均衡地址' : '入口地址'"><a-input v-model:value="form.access_address" :placeholder="isHaCluster ? '请输入 HA 集群 VIP' : form.topology_type === 'load_balancer' ? '请输入负载均衡地址' : 'IP 或入口地址'" /></a-form-item></a-col>
          <a-col :span="24">
            <a-form-item :label="isHaCluster ? '成员实例（至少 2 个）' : form.topology_type === 'load_balancer' ? '后端成员实例' : '部署实例'" required>
              <a-space wrap>
                <a-button class="add-deployment-button" @click="openDeploymentDialog()">新增部署实例</a-button>
                <a-select
                  v-model:value="pickedDeploymentId"
                  show-search
                  allow-clear
                  placeholder="从已有实例中选择"
                  style="width: 260px"
                  :options="addableDeploymentOptions"
                  :filter-option="filterOption"
                  :getPopupContainer="getPopupContainer"
                  @change="addPickedDeployment"
                />
              </a-space>
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
          <a-col :span="24">
            <a-divider orientation="left">日志</a-divider>
          </a-col>
          <a-col :span="12">
            <a-form-item label="开启日志采集">
              <a-switch v-model:checked="form.log_collection_enabled" />
              <div class="field-hint">关闭时该服务下所有实例均不采集。</div>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="默认保留档位">
              <a-select
                v-model:value="form.log_retention_tier"
                allow-clear
                placeholder="请选择保留档位"
                :options="retentionTierOptions"
                :getPopupContainer="getPopupContainer"
              />
              <div class="field-hint">
                档位决定写入哪个 data stream 及其过期策略。改档位会写入新流（旧流停止写入、按原档位保留到期，
                不迁移数据）；改动需要重新下发采集配置后主机才会走新流。
              </div>
            </a-form-item>
          </a-col>
          <!-- 日志的逐条配置（采集开关 / 保留档位 / 采集过滤 / 格式认证）**统一在日志中心
               「日志配置」里做**：那边有勾选批量、有配置差异、有下发入口，也被更多页面复用。
               这个弹窗只保留服务级的两个日志默认值（上面：开启采集、默认保留档位），
               保存时**不提交 log_settings**——那条字段是整表替换语义，提交空集合会把已有覆盖值删掉。 -->
          <a-col :span="24">
            <a-form-item label="逐条日志配置">
              <div class="field-hint">
                日志的路径与解析规则在部署模板的日志定义里维护；<b>每条日志采不采、保留档位、采集过滤、格式认证</b>
                都在「<b>日志管理 → 日志中心 → 日志配置</b>」里按行调整（支持勾选批量改，改完在那里下发）。
                <br />
                这里保存**不会改动**任何逐条覆盖值（服务级的两个默认值除外）。
              </div>
            </a-form-item>
          </a-col>
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
import { tableLocale } from '@/util/tableStyle'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { fetchAllPages } from '@/util/fetchAllPages'
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
import { getLogRetentionTiers } from '@/api/monitor'
import { retentionText } from '@/util/logRetention'

const props = defineProps({
  open: { type: Boolean, required: true },
  serviceId: { type: Number, default: null },
  clusterProfileId: { type: Number, default: null },
  initialBusinessSystemId: { type: Number, default: null },
  initialEnvironmentId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const loadError = ref('')
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
const pickedDeploymentId = ref(null)
const memberEnabled = reactive({})
const environmentRecords = ref([])
const retentionTierRecords = ref([])
const retentionTierOptions = computed(() => retentionTierRecords.value
  .filter((item) => item.enabled)
  .map((item) => ({ label: item.retention_value ? `${item.name}（${retentionText(item)}）` : item.name, value: item.id })))

// 「继承服务默认」到底继承的是哪一档：继承链是 **服务默认档位（上面那个下拉，未保存的以表单为准）
// → 平台默认档（is_default）→ std**。只写"继承服务默认"用户没法知道这条日志实际保留多久。
// 解析规则名的展示：优先用后端随行返回的名字（保存过的服务），没带就用规则列表按 id 反查
// （新建/换模板时只有 id）。

// 当前选中的部署模板：宏定义与 app_home 都从它取（路径预览要用，见 rowResolvedPath）。
const topologyOptions = [{ label: '单机', value: 'standalone' }, { label: '集群', value: 'cluster' }, { label: '负载均衡', value: 'load_balancer' }]
const initialForm = () => ({
  id: null, business_system: null, application: null, application_version: null, deployment_template: null, cluster_profile: null,
  name: '', code: '', environment: null, topology_type: 'standalone',
  macro_values: {},
  access_address: '',
  log_collection_enabled: false, log_retention_tier: null,
  enabled: true, remark: '',
})
const form = reactive(initialForm())
const environmentOptions = computed(() => environmentRecords.value
  .filter((item) => item.enabled)
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
const selectedTemplate = computed(() => templateRecords.value.find((item) => item.id === form.deployment_template) || null)
const templateMacros = computed(() => selectedTemplate.value?.macro_definitions || [])
const macroTableColumns = [
  { title: '宏 Key', key: 'name', width: 220 },
  { title: '值', key: 'value', width: 280 },
  { title: '说明', key: 'description' },
  { title: '操作', key: 'action', width: 110 },
]
const macroKeyLabel = (name) => `\${${name}}`
// 取服务级覆盖值。**用"取值是否 undefined"判断有没有覆盖，不要用 hasOwnProperty**：
// Vue 3.5 里 hasOwnProperty 建立的依赖在"新增键"时不失效（取值依赖会失效），用它会让
// "模板宏必填"的红标在用户填好值之后一直不消失（computed 缓存着旧结论，表单却已经能保存了）。
const macroOverride = (name) => (form.macro_values || {})[name]
const macroValue = (macro) => {
  const override = macroOverride(macro.name)
  return override === undefined ? (macro.value || '') : override
}
const hasMacroOverride = (name) => macroOverride(name) !== undefined
// 模板宏必须"有值"才能保存：要么模板给了默认值（继承），要么本服务自己填。
//
// 为什么不能空着：下发时 `${VAR}` 按"模板默认 → 服务覆盖 → 实例变量"的顺序找值，一个都没有时
// 要么整台实例被跳过（下发告警里只说"路径含未定义宏"），要么被替换成空串拼出坏路径
// （如 `/catalina.out`），Filebeat 监听不到文件、采集静默为空。两者都是"事后才发现"的坑，
// 所以在保存这一步就拦住——**模板没给默认值 = 本服务必填**。
const missingMacros = computed(() => templateMacros.value
  .filter((macro) => !String(macroValue(macro) ?? '').trim()))
const isMacroMissing = (macro) => missingMacros.value.some((item) => item.name === macro.name)
const missingMacrosText = computed(() => missingMacros.value.map((macro) => macroKeyLabel(macro.name)).join('、'))
function setMacroValue(name, value) {
  if (value === '' || value === undefined || value === null) {
    delete form.macro_values[name]
    return
  }
  form.macro_values[name] = value
}
function resetMacroValue(name) {
  delete form.macro_values[name]
}
const deploymentOptionLabel = (item) => `${item.instance_name} (${item.host_name || item.host_ip || '-'})`
// 可选的实例候选 = 当前应用的实例 + **当前已绑定的实例**。
//
// 必须并上已绑定的那些：后端返回的 `application_id` 是"该实例**最早**绑定的那个服务所属的应用"
// （不是实例自己的属性），而一个实例可以同时属于多个应用下的服务——例如 yilake-nginx-105 既在
// redis(应用 8) 也在 nginx(应用 15) 下，它的 application_id 就是 8。旧版只用
// `application_id === form.application` 过滤，而下面两个 watcher 又拿这份列表去**删**
// selectedDeploymentIds，于是已绑定的实例一进编辑弹窗就被静默剔除（2026-09-18 yilake nginx 现场：
// 库里关联还在，弹窗里却是空的）。
//
// 因此这份候选只用来收敛"新添加"的范围，绝不作为"删除已有成员"的依据：要移除成员请走成员行的
// 删除按钮（confirmDeleteDeployment）。
// "从已有实例中选择"的候选 = 全部实例刨掉已绑定的（关联表上有 (service_id, deployment_id)
// 唯一键，重复提交会直接失败）。同应用的排在前面，但**不**把别的应用排除在外。
//
// 为什么不用 `application_id` 过滤：后端返回的它是"该实例**最早**绑定的那个服务所属的应用"，
// 不是实例自己的属性——一个实例可以同时挂在多个应用下的服务上（yilake-nginx-105 既在
// redis(应用 8) 也在 nginx(应用 15) 下，它是 8；106 只在 mgmt(应用 28) 下，它是 28）。
// 拿它当"能不能绑到本服务"的判据，正好会把该绑的实例挡在外面（2026-09-18 yilake nginx 现场：
// 库里关联还在、弹窗里成员却是空的、重新绑定也无路可走）。这里只用来排序。
//
// 已绑定成员**不进候选**，也不在任何 watcher 里被自动剔除：成员列表以服务端返回的
// member_instances 为准，要移除只走成员行的删除按钮（confirmDeleteDeployment）。
const addableDeploymentOptions = computed(() => {
  const memberIds = new Set(selectedDeploymentIds.value)
  return deploymentRecords.value
    .filter((item) => !memberIds.has(item.id))
    .map((item) => ({
      label: deploymentOptionLabel(item),
      value: item.id,
      sameApplication: item.application_id === form.application,
    }))
    .sort((left, right) => Number(right.sameApplication) - Number(left.sameApplication))
})

// 绑定一台已存在的实例：这一步同时解决了"实例已在别的服务下、无法再绑到本服务"的场景
// （此前只能走「新增部署实例」→ 前端按主机+实例名去重转成编辑，路径绕且容易误解）。
function addPickedDeployment(deploymentId) {
  if (!deploymentId) return
  if (!selectedDeploymentIds.value.includes(deploymentId)) {
    selectedDeploymentIds.value = [...selectedDeploymentIds.value, deploymentId]
    memberEnabled[deploymentId] = true
  }
  pickedDeploymentId.value = null
}
const filterOption = (input, option) => String(option?.label || '').toLowerCase().includes(String(input || '').toLowerCase())

// 已关联实例要按全量记录取名，按当前应用过滤会让跨应用实例回退成「实例 ID」。
function deploymentLabel(deploymentId) {
  const deployment = deploymentRecords.value.find((item) => item.id === deploymentId)
  return deployment ? deploymentOptionLabel(deployment) : `实例 ${deploymentId}`
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
  businessSystemOptions.value = [
    ...businessSystemOptions.value.filter((item) => item.value !== system.id),
    { label: system.project_name ? `${system.project_name} / ${system.name}` : system.name, value: system.id },
  ]
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
  }
  if (!selectedDeploymentIds.value.includes(deployment.id)) {
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

async function initialize() {
  Object.assign(form, initialForm())
  loadError.value = ''
  loading.value = true
  try {
    const loaders = [
      ['业务系统', () => fetchAllPages(getBusinessSystemList, { enabled: true })],
      ['环境', () => fetchAllPages(getBusinessEnvironmentList, { enabled: true })],
      ['应用', () => fetchAllPages(getApplicationList, { enabled: true })],
      ['应用版本', () => fetchAllPages(getApplicationVersionList, { enabled: true })],
      ['部署模板', () => fetchAllPages(getApplicationDeploymentTemplateList, { enabled: true })],
      ['集群模型', () => fetchAllPages(getClusterProfileList, { enabled: true })],
      ['部署实例', () => fetchAllPages(getApplicationDeploymentList)],
      // 保留档位：服务级"默认保留档位"下拉的选项（逐条档位在日志中心改）。
      ['保留档位', () => fetchAllPages(getLogRetentionTiers, { enabled: true })],
    ]
    const results = await Promise.all(loaders.map(async ([label, loader]) => {
      try {
        return { label, records: await loader() }
      } catch (error) {
        return { label, error }
      }
    }))
    const failedLabels = results.filter((result) => result.error).map((result) => result.label)
    if (failedLabels.length) loadError.value = `${failedLabels.join('、')}加载失败，请点击重试`
    const records = Object.fromEntries(results.map((result) => [result.label, result.records || []]))
    const systems = records['业务系统']
    const environments = records['环境']
    const applications = records['应用']
    const versions = records['应用版本']
    const templates = records['部署模板']
    const profiles = records['集群模型']
    const deployments = records['部署实例']
    businessSystemOptions.value = systems.map((item) => ({ label: item.name, value: item.id }))
    environmentRecords.value = environments
    applicationOptions.value = applications.map((item) => ({ label: item.name, value: item.id }))
    versionRecords.value = versions
    templateRecords.value = templates
    profileRecords.value = profiles
    deploymentRecords.value = deployments
    retentionTierRecords.value = records['保留档位']
    if (props.serviceId) {
      const response = await getApplicationService(props.serviceId)
      const data = response?.data?.data || {}
      Object.assign(form, initialForm(), data)
      form.macro_values = { ...(data.macro_values || {}) }
      // 后端 member_instances 为部署 ID 数组（无覆盖记录时无 enabled 信息，默认启用）
      selectedDeploymentIds.value = (data.member_instances || [])
        .map((item) => (item && typeof item === 'object' ? item.deployment : item))
        .filter((id) => id !== undefined && id !== null)
      for (const item of data.member_instances || []) {
        if (item && typeof item === 'object') {
          memberEnabled[item.deployment] = item.enabled !== false
        } else {
          memberEnabled[item] = true
        }
      }
    } else if (props.clusterProfileId) {
      form.topology_type = 'cluster'
      form.cluster_profile = props.clusterProfileId
      form.application = selectedProfile.value?.application || null
    } else {
      // 纯新增且调用方已经带了左侧树选中的业务系统/环境时直接预填，避免重复选一遍。
      if (props.initialBusinessSystemId && businessSystemOptions.value.some((item) => item.value === props.initialBusinessSystemId)) {
        form.business_system = props.initialBusinessSystemId
      }
      if (props.initialEnvironmentId && environmentOptions.value.some((item) => item.value === props.initialEnvironmentId)) {
        form.environment = props.initialEnvironmentId
      }
    }
    await nextTick()
    formRef.value?.clearValidate()
  } finally {
    loading.value = false
  }
}

async function submit() {
  // 校验失败是正常交互（红字提示补字段），catch 住避免未处理的 Promise 拒绝刷控制台
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  const minimumMemberCount = isHaCluster.value ? 2 : 1
  if (selectedDeploymentIds.value.length < minimumMemberCount) {
    message.error(isHaCluster.value ? 'HA 集群至少需要两个成员实例' : form.topology_type === 'standalone' ? '请选择部署实例' : '请选择至少一个后端成员实例')
    return
  }
  // 模板没给默认值的宏必须在本服务填：空着下发时路径展不开（跳过主机或拼出坏路径），
  // 这是"采集静默为空"的常见原因，所以在保存这一步拦住（表格里也用红框与"必填"标出来了）。
  if (missingMacros.value.length) {
    message.error(`模板宏还没填：${missingMacrosText.value}（模板未给默认值，必须在本服务设置）`)
    return
  }
  saving.value = true
  try {
    const payload = { ...form }
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
    // **不提交 log_settings**（逐条日志的覆盖值）：这个弹窗不再编辑它们（统一在日志中心
    // 「日志配置」里改），而那个字段是**整表替换**语义——提交空集合会把该服务已有的
    // 采集开关/档位/过滤覆盖值全部删掉。字段缺省（null）= 后端整块跳过，一行都不动。
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
// 改应用 / 改集群模型时只清理不再适用的版本与模板。
// **不要**顺手过滤 selectedDeploymentIds：成员属于这个逻辑服务，且可能同时挂在别的应用下的服务上，
// 按 application_id 过滤会把已绑定的实例静默剔除出成员列表（2026-09-18 yilake nginx 现场：
// 库里关联还在，编辑弹窗里成员却是空的，保存时又把空成员写回去）。
// 移除成员只走成员行的删除按钮（confirmDeleteDeployment），那是显式动作。
watch(() => form.application, () => {
  if (form.application_version && !versionOptions.value.some((item) => item.value === form.application_version)) {
    form.application_version = null
  }
  if (form.deployment_template && !templateOptions.value.some((item) => item.value === form.deployment_template)) {
    form.deployment_template = null
  }
})
watch(() => form.deployment_template, (templateId) => {
  const definitions = templateRecords.value.find((item) => item.id === templateId)?.macro_definitions || []
  const values = {}
  for (const definition of definitions) {
    if (Object.prototype.hasOwnProperty.call(form.macro_values || {}, definition.name)) values[definition.name] = form.macro_values[definition.name]
  }
  form.macro_values = values
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
})
</script>

<style scoped>
.path-toggle {
  margin-left: 6px;
}
.path-macro-tag {
  margin-left: 6px;
  font-size: 11px;
}
/* 模板宏缺值时提示与表格贴在一起，别让"保存不了"变成只闪一下的 toast。 */
.service-macro-alert {
  margin-bottom: 8px;
}
.field-hint {
  margin-top: 4px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
/* 模板没给这条日志挂解析规则时提示：不会采集，且服务侧改不了。 */
.log-rule-missing {
  color: #d4380d;
}
/* 日志配置加载失败（接口报错）与"确实没有日志定义"必须能区分开。 */
.log-config-error {
  margin-bottom: 8px;
}
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
