<template>
  <a-modal
    title="运行任务"
    :open="open"
    :confirmLoading="confirmLoading"
    ok-text="立即执行"
    cancel-text="取消"
    :ok-button-props="{ disabled: !precheckOk }"
    @ok="emitConfirm"
    @cancel="emitCancel"
  >
    <a-form layout="vertical">
      <a-form-item label="本次 Limit（可选）">
        <a-input
          v-model:value="limitModel"
          allow-clear
          :placeholder="limitInputPlaceholder"
        />
      </a-form-item>
      <a-form-item v-if="isShellTask" label="本次参数字符串（可选）">
        <a-input
          v-model:value="shellArgsModel"
          allow-clear
          placeholder="例如: prod 8080 --force"
        />
      </a-form-item>
    </a-form>

    <ScopePrecheckPanel
      :precheck-ok="precheckOk"
      :prechecking="prechecking"
      :message="precheckText"
      :hosts="allHosts"
      :matched-hosts="matchedHosts"
      :show-host-link="true"
      :show-limit-toggle="true"
      :show-target-filter="true"
      :limit-text="runNowLimit"
      @host-click="onHostClick"
      @toggle-limit-host="onToggleLimitHost"
      @remove-limit-token="onRemoveLimitToken"
    />
  </a-modal>
</template>

<script setup>
import { computed } from 'vue'
import ScopePrecheckPanel from '../../../components/ScopePrecheckPanel.vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  confirmLoading: { type: Boolean, default: false },
  precheckOk: { type: Boolean, default: false },
  prechecking: { type: Boolean, default: false },
  precheckText: { type: String, default: '' },
  isShellTask: { type: Boolean, default: false },
  runNowLimit: { type: String, default: '' },
  runNowShellArgs: { type: String, default: '' },
  allHosts: { type: Array, default: () => [] },
  matchedHosts: { type: Array, default: () => [] },
  limitInputPlaceholder: { type: String, required: true },
})

const emit = defineEmits([
  'update:runNowLimit',
  'update:runNowShellArgs',
  'confirm',
  'cancel',
  'host-click',
  'toggle-limit-host',
  'remove-limit-token',
])

const limitModel = computed({
  get: () => props.runNowLimit,
  set: (value) => emit('update:runNowLimit', value),
})

const shellArgsModel = computed({
  get: () => props.runNowShellArgs,
  set: (value) => emit('update:runNowShellArgs', value),
})

function emitConfirm() {
  emit('confirm')
}

function emitCancel() {
  emit('cancel')
}

function onHostClick(item) {
  emit('host-click', item)
}

function onToggleLimitHost(item) {
  emit('toggle-limit-host', item)
}

function onRemoveLimitToken(token) {
  emit('remove-limit-token', token)
}
</script>
