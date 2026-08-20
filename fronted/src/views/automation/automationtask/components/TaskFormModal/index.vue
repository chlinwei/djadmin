<template>
  <a-modal
    :title="isCreateMode ? '新增任务' : '编辑任务'"
    :open="open"
    :width="820"
    :confirmLoading="confirmLoading"
    @ok="emitSubmit"
    @cancel="emitCancel"
  >
    <a-form layout="vertical">
      <a-row :gutter="12">
        <a-col :span="12">
          <a-form-item label="任务名称" required>
            <a-input v-model:value="taskForm.name" placeholder="例如：生产环境健康巡检" />
          </a-form-item>
        </a-col>
      </a-row>

      <a-row :gutter="12">
        <a-col :span="12">
          <a-form-item label="选择模板" required>
            <a-select
              v-model:value="taskForm.template"
              :options="taskTemplateOptions"
              :loading="taskTemplateLoading"
              show-search
              optionFilterProp="label"
              :getPopupContainer="getTaskModalPopupContainer"
              :placeholder="taskTemplatePlaceholder"
            />
          </a-form-item>
        </a-col>
        <a-col :span="12">
          <a-form-item label="选择Inventory（可选）">
            <a-select
              v-model:value="taskForm.inventory"
              :options="inventoryOptions"
              show-search
              optionFilterProp="label"
              :getPopupContainer="getTaskModalPopupContainer"
              placeholder="可选：未选择则按任务节点范围执行"
              allow-clear
            />
          </a-form-item>
        </a-col>
      </a-row>

      <a-form-item label="启用状态">
        <a-switch v-model:checked="taskForm.enabled" checked-children="启用" un-checked-children="禁用" />
      </a-form-item>

      <a-form-item>
        <a-alert
          type="info"
          show-icon
          message="任务执行范围由所选 Inventory 决定；主机组请在 Inventory 管理中维护"
        />
      </a-form-item>

      <a-form-item label="默认 Limit（可选）">
        <a-input
          v-model:value="taskForm.default_limit"
          :placeholder="limitInputPlaceholder"
        />
        <ScopePrecheckPanel
          :precheck-ok="taskLimitPrecheckOk"
          :prechecking="taskLimitPrechecking"
          :message="taskLimitPrecheckText"
          :hosts="taskLimitAllHosts"
          :matched-hosts="taskLimitMatchedHosts"
          :show-host-link="true"
          :show-limit-toggle="true"
          :show-target-filter="true"
          :limit-text="taskForm.default_limit"
          @host-click="onTaskLimitHostClick"
          @toggle-limit-host="onTaskLimitToggle"
          @remove-limit-token="onTaskLimitRemoveToken"
        />
      </a-form-item>

      <a-form-item label="执行超时（秒）" required>
        <a-input-number
          v-model:value="taskForm.execution_timeout_seconds"
          :min="1"
          :max="14400"
          :step="60"
          style="width: 100%"
          placeholder="默认 600"
        />
        <a-alert
          type="info"
          show-icon
          style="margin-top: 8px"
          message="任务总执行超时（秒），超过该时间后任务会被终止。"
        />
      </a-form-item>

      <a-form-item :label="taskEnvVarsLabel">
        <a-textarea
          v-model:value="taskForm.env_vars_text"
          :rows="6"
          :placeholder="taskEnvVarsPlaceholder"
        />
        <a-alert
          type="info"
          show-icon
          style="margin-top: 8px"
          message="填写 Playbook 执行时使用的 JSON 变量。"
        />
      </a-form-item>

      <a-divider orientation="left" style="margin: 16px 0">执行身份配置</a-divider>

      <a-row :gutter="12">
        <a-col :span="8">
          <a-form-item label="执行用户" required>
            <a-input
              v-model:value="taskForm.run_as_user"
              placeholder="必填，例如 node_exporter"
            />
            <a-alert
              type="info"
              show-icon
              message="dj-agent 以 root 运行，任务实际执行时会 setuid/setgid 降权到该用户，不再使用 sudo/su"
              style="margin-top: 8px"
            />
          </a-form-item>
        </a-col>
        <a-col :span="8">
          <a-form-item label="执行组（可选）">
            <a-input
              v-model:value="taskForm.run_as_group"
              placeholder="留空则使用执行用户的主组"
            />
          </a-form-item>
        </a-col>
        <a-col :span="8">
          <a-form-item label="工作目录">
            <a-input
              v-model:value="taskForm.work_directory"
              placeholder="默认为 /tmp"
            />
          </a-form-item>
        </a-col>
      </a-row>
    </a-form>
  </a-modal>
</template>

<script setup>
import ScopePrecheckPanel from '../../../components/ScopePrecheckPanel.vue'

defineProps({
  isCreateMode: { type: Boolean, default: true },
  open: { type: Boolean, default: false },
  confirmLoading: { type: Boolean, default: false },
  taskForm: { type: Object, required: true },
  getTaskModalPopupContainer: { type: Function, required: true },
  taskTemplateOptions: { type: Array, required: true },
  taskTemplateLoading: { type: Boolean, default: false },
  taskTemplatePlaceholder: { type: String, default: '' },
  inventoryOptions: { type: Array, required: true },
  limitInputPlaceholder: { type: String, required: true },
  taskLimitPrecheckOk: { type: Boolean, default: false },
  taskLimitPrechecking: { type: Boolean, default: false },
  taskLimitPrecheckText: { type: String, default: '' },
  taskLimitAllHosts: { type: Array, default: () => [] },
  taskLimitMatchedHosts: { type: Array, default: () => [] },
  taskEnvVarsLabel: { type: String, default: '' },
  taskEnvVarsPlaceholder: { type: String, default: '' },
})

const emit = defineEmits([
  'submit',
  'cancel',
  'task-limit-host-click',
  'task-limit-toggle',
  'task-limit-remove-token',
])

function emitSubmit() {
  emit('submit')
}

function emitCancel() {
  emit('cancel')
}

function onTaskLimitHostClick(item) {
  emit('task-limit-host-click', item)
}

function onTaskLimitToggle(item) {
  emit('task-limit-toggle', item)
}

function onTaskLimitRemoveToken(token) {
  emit('task-limit-remove-token', token)
}
</script>
