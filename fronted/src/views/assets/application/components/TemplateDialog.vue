<template>
  <a-modal
    :open="open"
    :title="templateId ? `编辑部署模板：${form.name || '加载中'}` : copyFromId ? `复制部署模板：${form.name || '加载中'}` : '新增部署模板'"
    :width="1180"
    :confirm-loading="saving"
    ok-text="保存模板"
    cancel-text="关闭"
    wrap-class-name="application-template-modal"
    centered
    destroy-on-close
    @ok="saveTemplate"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
        <a-tabs>
          <a-tab-pane key="base" tab="运行配置">
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item name="application" label="所属应用">
                  <a-select
                    v-model:value="form.application"
                    :options="applicationOptions"
                    :getPopupContainer="getPopupContainer"
                    show-search
                    option-filter-prop="label"
                    placeholder="请选择应用"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="12"><a-form-item name="name" label="模板名称"><a-input v-model:value="form.name" placeholder="例如 Tomcat Systemd 标准模板" /></a-form-item></a-col>
              <a-col :span="6"><a-form-item name="run_user" label="运行用户"><a-input v-model:value="form.run_user" /></a-form-item></a-col>
              <a-col :span="6"><a-form-item label="运行组"><a-input v-model:value="form.run_group" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="App Home"><a-input v-model:value="form.app_home" placeholder="例如 /home/${RUN_USER}/tomcat" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="工作目录"><a-input v-model:value="form.work_directory" placeholder="例如 ${APP_HOME}/bin" /></a-form-item></a-col>
              <a-col :span="24">
                <a-form-item label="宏" extra="模板提供默认值，逻辑服务可直接继承或覆盖">
                  <a-table :columns="macroColumns" :data-source="macroDefinitions" :pagination="false" row-key="name" size="small">
                    <template #bodyCell="{ column, record, index }">
                      <template v-if="column.key === 'name'"><a-input v-model:value="record.name" placeholder="例如 ORACLE_SID" /></template>
                      <template v-else-if="column.key === 'value'"><a-input v-model:value="record.value" placeholder="默认值，可为空" /></template>
                      <template v-else-if="column.key === 'description'"><a-input v-model:value="record.description" placeholder="说明" /></template>
                      <template v-else-if="column.key === 'action'"><a-tooltip title="删除"><a-button class="delBtn" danger size="small" @click="macroDefinitions.splice(index, 1)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip></template>
                    </template>
                  </a-table>
                  <a-button class="macro-add-button" @click="addMacro"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" /><span>&nbsp;新增宏</span></a-button>
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item name="control_type" label="控制方式">
                  <a-select
                    v-model:value="form.control_type"
                    :options="controlTypeOptions"
                    :getPopupContainer="getPopupContainer"
                    placeholder="请选择控制方式"
                  />
                </a-form-item>
              </a-col>
            </a-row>

            <template v-if="form.control_type === 'systemd'">
              <a-row :gutter="16">
                <a-col :span="12">
                  <a-form-item name="service_name" label="Systemd 服务名">
                    <a-input v-model:value="form.service_name" placeholder="例如 tomcat 或 tomcat.service" />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item name="systemd_scope" label="Systemd 作用域">
                    <a-segmented v-model:value="form.systemd_scope" :options="systemdScopeOptions" block />
                  </a-form-item>
                </a-col>
              </a-row>
              <a-alert
                v-if="form.systemd_scope === 'user'"
                type="info"
                show-icon
                message="用户服务会切换到运行用户，并设置该用户的 XDG_RUNTIME_DIR 后执行 systemctl --user；跨用户执行要求 dj-agent 以 root 运行。"
              />
            </template>
            <template v-else-if="form.control_type === 'external_ha'">
              <a-row :gutter="16">
                <a-col :span="8"><a-form-item label="HA 系统"><a-input v-model:value="form.ha_system_name" /></a-form-item></a-col>
                <a-col :span="8"><a-form-item label="集群名称"><a-input v-model:value="form.ha_cluster_name" /></a-form-item></a-col>
                <a-col :span="8"><a-form-item label="资源名称"><a-input v-model:value="form.ha_resource_name" /></a-form-item></a-col>
              </a-row>
              <a-alert
                type="info"
                show-icon
                message="状态命令由目标主机上的运行用户执行；退出码 0 表示运行中，非 0 表示已停止。"
              />
              <a-form-item label="状态检查 Shell（必填）" class="ha-status-command">
                <a-textarea
                  v-model:value="commandValues.status"
                  :rows="4"
                  placeholder="例如 pgrep -f '[o]rg.apache.catalina.startup.Bootstrap' >/dev/null"
                />
              </a-form-item>
            </template>
            <a-row v-else-if="form.control_type === 'docker'" :gutter="16">
              <a-col :span="12"><a-form-item label="容器名称" required><a-input v-model:value="form.docker_config.container_name" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="Docker Host"><a-input v-model:value="form.docker_config.docker_host" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="期望镜像"><a-input v-model:value="form.docker_config.expected_image" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="期望 Tag"><a-input v-model:value="form.docker_config.expected_image_tag" /></a-form-item></a-col>
            </a-row>
            <a-row v-else-if="form.control_type === 'docker_compose'" :gutter="16">
              <a-col :span="8"><a-form-item label="项目名称" required><a-input v-model:value="form.compose_config.project_name" /></a-form-item></a-col>
              <a-col :span="8"><a-form-item label="服务名称" required><a-input v-model:value="form.compose_config.service_name" /></a-form-item></a-col>
              <a-col :span="8"><a-form-item label="工作目录" required><a-input v-model:value="form.compose_config.working_directory" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="Compose 文件" required><a-input v-model:value="form.compose_config.compose_file_path" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="环境变量文件"><a-input v-model:value="form.compose_config.env_file" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="期望镜像"><a-input v-model:value="form.compose_config.expected_image" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="期望 Tag"><a-input v-model:value="form.compose_config.expected_image_tag" /></a-form-item></a-col>
            </a-row>
            <template v-else-if="form.control_type === 'command'">
              <a-alert type="warning" show-icon message="start、stop、status 必填；可引用 ${APP_HOME} 和 ${RUN_USER}。" />
              <a-row :gutter="16" class="command-grid">
                <a-col v-for="action in commandActions" :key="action.value" :span="action.required ? 8 : 12">
                  <a-form-item :label="`${action.label}${action.required ? '（必填）' : ''}`">
                    <a-textarea v-model:value="commandValues[action.value]" :rows="3" :placeholder="commandPlaceholder(action.value)" />
                  </a-form-item>
                </a-col>
              </a-row>
            </template>
            <a-form-item label="允许新部署"><a-switch v-model:checked="form.enabled" /></a-form-item>
          </a-tab-pane>

          <a-tab-pane key="ports" tab="端口">
            <div v-for="(item, index) in form.ports" :key="`port-${index}`" class="repeat-row port-row">
              <a-input v-model:value="item.name" placeholder="名称" />
              <a-select v-model:value="item.protocol" :options="protocolOptions" :getPopupContainer="getPopupContainer" />
              <a-input-number v-model:value="item.port" :min="1" :max="65535" placeholder="端口" />
              <a-input v-model:value="item.bind_address" placeholder="监听地址" />
              <a-checkbox v-model:checked="item.external_access">外部访问</a-checkbox>
              <a-button danger @click="form.ports.splice(index, 1)">移除</a-button>
            </div>
            <a-button type="dashed" block @click="addPort">新增端口</a-button>
          </a-tab-pane>

          <a-tab-pane key="paths" tab="路径与文件">
            <h4>应用路径</h4>
            <div v-for="(item, index) in form.paths" :key="`path-${index}`" class="repeat-row path-row">
              <a-input v-model:value="item.name" placeholder="名称" />
              <a-select v-model:value="item.path_type" :options="pathTypeOptions" :getPopupContainer="getPopupContainer" />
              <a-input v-model:value="item.path" placeholder="例如 ${APP_HOME}/logs" />
              <a-button danger @click="form.paths.splice(index, 1)">移除</a-button>
            </div>
            <a-button type="dashed" block @click="addPath">新增路径</a-button>
            <a-divider />
            <h4>配置文件</h4>
            <div v-for="(item, index) in form.config_files" :key="`config-${index}`" class="repeat-row config-row">
              <a-input v-model:value="item.name" placeholder="名称" />
              <a-input v-model:value="item.path" placeholder="例如 ${APP_HOME}/conf/server.xml" />
              <a-select v-model:value="item.file_format" :options="fileFormatOptions" :getPopupContainer="getPopupContainer" />
              <a-button danger @click="form.config_files.splice(index, 1)">移除</a-button>
            </div>
            <a-button type="dashed" block @click="addConfigFile">新增配置文件</a-button>
            <a-divider />
            <h4>日志</h4>
            <div v-for="(item, index) in form.logs" :key="`log-${index}`" class="repeat-row log-row">
              <a-input v-model:value="item.name" placeholder="名称" />
              <a-input v-model:value="item.path_pattern" placeholder="例如 ${APP_HOME}/logs/*.log" />
              <a-select
                v-model:value="item.encoding"
                :options="encodingOptions"
                :getPopupContainer="getPopupContainer"
                placeholder="字符编码"
              />
              <a-select
                v-model:value="item.processing_rule"
                :options="processingRuleOptions"
                :getPopupContainer="getPopupContainer"
                allow-clear
                show-search
                option-filter-prop="label"
                placeholder="日志处理规则"
              />
              <a-checkbox v-model:checked="item.collection_enabled">启用采集</a-checkbox>
              <a-button danger @click="form.logs.splice(index, 1)">移除</a-button>
            </div>
            <a-button type="dashed" block @click="addLog">新增日志</a-button>
          </a-tab-pane>
        </a-tabs>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import {
  getApplicationList,
  getApplicationDeploymentTemplate,
  saveApplicationDeploymentTemplate,
} from '@/api/assets/application'
import { getLogProcessingRules } from '@/api/monitor'

const props = defineProps({
  open: { type: Boolean, required: true },
  templateId: { type: Number, default: null },
  copyFromId: { type: Number, default: null },
  initialApplicationId: { type: Number, default: null },
})
const emit = defineEmits(['update:open', 'saved'])
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formRef = ref(null)
const loading = ref(false)
const saving = ref(false)
const applications = ref([])
const processingRules = ref([])
let loadToken = 0
const commandValues = reactive({ start: '', stop: '', status: '', restart: '', reload: '' })

const controlTypeOptions = [
  { label: 'Systemd', value: 'systemd' }, { label: '命令行', value: 'command' },
  { label: '外部 HA', value: 'external_ha' }, { label: 'Docker', value: 'docker' },
  { label: 'Docker Compose', value: 'docker_compose' },
]
const systemdScopeOptions = [
  { label: '系统服务', value: 'system' },
  { label: '用户服务 (--user)', value: 'user' },
]
const protocolOptions = [{ label: 'TCP', value: 'tcp' }, { label: 'UDP', value: 'udp' }]
const pathTypeOptions = [
  { label: 'Home', value: 'home' }, { label: 'Bin', value: 'bin' },
  { label: '配置', value: 'config' }, { label: '数据', value: 'data' },
  { label: '日志', value: 'log' }, { label: 'PID', value: 'pid' },
  { label: '可执行文件', value: 'executable' }, { label: '其他', value: 'other' },
]
const fileFormatOptions = ['xml', 'yaml', 'json', 'ini', 'properties', 'text'].map((value) => ({ label: value.toUpperCase(), value }))
const encodingOptions = ['utf-8', 'utf-16le', 'utf-16be', 'latin1'].map((value) => ({ label: value.toUpperCase(), value }))
const commandActions = [
  { label: '启动命令', value: 'start', required: true },
  { label: '停止命令', value: 'stop', required: true },
  { label: '状态命令', value: 'status', required: true },
  { label: '重启命令', value: 'restart', required: false },
  { label: '重载命令', value: 'reload', required: false },
]
const commandPlaceholder = (action) => action === 'start'
  ? '${APP_HOME}/bin/startup.sh'
  : `\${APP_HOME}/bin/${action}.sh`
const createInitialForm = () => ({
  application: null, name: '', control_type: 'systemd', run_user: '', run_group: '', app_home: '', work_directory: '',
  service_name: '', systemd_scope: 'system', ha_system_name: '', ha_cluster_name: '', ha_resource_name: '', enabled: true, remark: '',
  ports: [], paths: [], config_files: [], logs: [],
  docker_config: { container_name: '', docker_host: 'unix:///var/run/docker.sock', expected_image: '', expected_image_tag: '' },
  compose_config: { project_name: '', service_name: '', compose_file_path: '', working_directory: '', env_file: '', expected_image: '', expected_image_tag: '' },
})
const form = reactive(createInitialForm())
const macroDefinitions = ref([])
const macroColumns = [
  { title: '宏 Key', key: 'name', width: 260 },
  { title: '值', key: 'value', width: 240 },
  { title: '说明', key: 'description' },
  { title: '操作', key: 'action', width: 70 },
]
const rules = {
  application: [{ required: true, message: '请选择所属应用' }],
  name: [{ required: true, message: '请输入模板名称' }],
  control_type: [{ required: true, message: '请选择控制方式' }],
  run_user: [{ required: true, message: '请输入运行用户' }],
}
const applicationOptions = computed(() => applications.value.map((item) => ({ label: item.name, value: item.id })))
// 只列出属于当前应用的规则和不限应用的通用规则，避免把别的应用的解析规则挂到这个模板上。
const processingRuleOptions = computed(() => processingRules.value
  .filter((item) => !item.application || item.application === form.application)
  .map((item) => ({
    label: item.application ? item.name : `${item.name}（通用）`,
    value: item.id,
  })))

function resetForm() {
  Object.assign(form, createInitialForm())
  macroDefinitions.value = []
  Object.keys(commandValues).forEach((key) => { commandValues[key] = '' })
}
async function loadApplications() {
  const response = await getApplicationList({ page_size: 100 })
  applications.value = response?.data?.data?.results || []
  if (!props.templateId && !props.copyFromId && props.initialApplicationId) {
    form.application = props.initialApplicationId
  }
}
async function loadProcessingRules() {
  const response = await getLogProcessingRules({ page_size: 100 })
  processingRules.value = response?.data?.data?.results || []
}
async function loadTemplate(id) {
  if (!id) return
  const currentLoadToken = ++loadToken
  loading.value = true
  try {
    const response = await getApplicationDeploymentTemplate(id)
    if (currentLoadToken !== loadToken) return
    const data = response?.data?.data || {}
    const initial = createInitialForm()
    Object.assign(form, initial, data)
    macroDefinitions.value = (data.macro_definitions || []).map((item) => ({
      name: item.name || '', value: item.value || item.default || '', description: item.description || '',
    }))
    if (props.copyFromId) form.name = `${form.name || '部署模板'}-副本`
    if (!form.docker_config) form.docker_config = initial.docker_config
    if (!form.compose_config) form.compose_config = initial.compose_config
    Object.keys(commandValues).forEach((key) => { commandValues[key] = '' })
    for (const item of data.control_actions || []) commandValues[item.action] = item.command || ''
  } finally {
    if (currentLoadToken === loadToken) loading.value = false
  }
}
function validateControlConfig() {
  if (form.control_type === 'systemd' && !String(form.service_name || '').trim()) return '请输入 Systemd 服务名'
  if (form.control_type === 'docker' && !String(form.docker_config?.container_name || '').trim()) return '请输入 Docker 容器名称'
  if (form.control_type === 'docker_compose') {
    const config = form.compose_config || {}
    if (![config.project_name, config.service_name, config.compose_file_path, config.working_directory].every((value) => String(value || '').trim())) return '请完整填写 Docker Compose 配置'
  }
  if (form.control_type === 'external_ha') {
    if (!String(form.ha_resource_name || '').trim()) return '请输入 HA 资源名称'
    if (!String(commandValues.status || '').trim()) return '请填写 HA 状态检查 Shell'
  }
  if (form.control_type === 'command' && !['start', 'stop', 'status'].every((action) => String(commandValues[action] || '').trim())) return '请填写启动、停止和状态命令'
  return ''
}
async function saveTemplate() {
  try {
    await formRef.value?.validate()
  } catch {
    // 表单校验失败时 antd 已在字段上标红，不再弹全局提示
    return
  }
  const validationMessage = validateControlConfig()
  if (validationMessage) {
    message.error(validationMessage)
    return
  }
  const payload = { ...form, id: props.copyFromId ? undefined : props.templateId || undefined, macro_definitions: macroDefinitions.value }
  payload.control_actions = form.control_type === 'command'
    ? commandActions.filter((item) => String(commandValues[item.value] || '').trim()).map((item) => ({ action: item.value, command: commandValues[item.value], timeout_seconds: 60, success_exit_codes: [0] }))
    : form.control_type === 'external_ha'
      ? [{ action: 'status', command: commandValues.status, timeout_seconds: 30, success_exit_codes: [0] }]
      : []
  payload.docker_config = form.control_type === 'docker' ? form.docker_config : null
  payload.compose_config = form.control_type === 'docker_compose' ? form.compose_config : null
  saving.value = true
  try {
    const response = await saveApplicationDeploymentTemplate(payload)
    message.success(props.templateId ? '部署模板保存成功' : props.copyFromId ? '部署模板复制成功' : '部署模板新增成功')
    emit('saved', response?.data?.data)
    emit('update:open', false)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存部署模板失败')
  } finally {
    saving.value = false
  }
}
const addPort = () => form.ports.push({ name: '', protocol: 'tcp', port: null, bind_address: '0.0.0.0', required: true, external_access: false, check_enabled: true })
const addMacro = () => macroDefinitions.value.push({ name: '', value: '', description: '' })
const addPath = () => form.paths.push({ name: '', path_type: 'other', path: '', required: true, expected_owner: '', expected_group: '', expected_mode: '', check_enabled: true })
const addConfigFile = () => form.config_files.push({ name: '', path: '', file_format: 'text', required: true })
const addLog = () => form.logs.push({ name: '', path_pattern: '', encoding: 'utf-8', collection_enabled: false, processing_rule: null, extra_fields: {} })

watch(() => props.open, (visible) => {
  if (!visible) {
    loadToken += 1
    return
  }
  resetForm()
  Promise.all([
    loadApplications(),
    loadProcessingRules(),
    props.templateId || props.copyFromId
      ? loadTemplate(props.templateId || props.copyFromId)
      : Promise.resolve(),
  ])
})
</script>

<style scoped>
.ha-status-command {
  margin-top: 16px;
}
.command-grid,
.repeat-row { margin-top: 16px; }
.repeat-row { display: grid; gap: 8px; align-items: center; margin-bottom: 8px; }
.port-row { grid-template-columns: 1.1fr 110px 130px 1.2fr 110px 72px; }
.path-row { grid-template-columns: 1fr 150px 2fr 72px; }
.config-row { grid-template-columns: 1fr 2fr 130px 72px; }
.log-row { grid-template-columns: 1fr 2fr 150px 110px 72px; }
:global(.application-template-modal .ant-modal) {
  width: min(1180px, calc(100vw - 240px)) !important;
  max-width: none;
  margin-right: 24px;
}
@media (max-width: 768px) {
  :global(.application-template-modal .ant-modal) {
    width: calc(100vw - 32px) !important;
    margin: 8px auto;
  }
}
</style>
