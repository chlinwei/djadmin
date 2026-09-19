<template>
  <a-modal
    :open="open"
    :title="title"
    :confirm-loading="submitting"
    ok-text="确认清理"
    cancel-text="取消"
    @update:open="(value) => emit('update:open', value)"
    @ok="submit"
  >
    <!-- 清理是**唯一会真删日志数据**的动作，所以：范围（保留多久）必须由用户显式选，
         并且把"清理哪条流"和"清理不会停采"写在同一屏里。 -->
    <a-alert
      type="warning"
      show-icon
      message="清理不可恢复"
      :description="alertDescription"
    />
    <div class="cleanup-target" v-if="targetLabel">
      <span class="cleanup-target-label">清理对象</span>
      <span class="cleanup-target-value">{{ targetLabel }}</span>
    </div>
    <a-form layout="vertical" style="margin-top: 16px">
      <a-form-item label="清理范围">
        <a-radio-group v-model:value="mode">
          <a-radio value="hours">保留最近 N 小时</a-radio>
          <a-radio value="days">保留最近 N 天</a-radio>
          <a-radio value="all">全部清空</a-radio>
        </a-radio-group>
      </a-form-item>
      <a-form-item v-if="mode !== 'all'" :label="mode === 'hours' ? '保留小时数' : '保留天数'">
        <a-input-number v-model:value="amount" :min="1" :max="maxAmount" style="width: 180px" />
      </a-form-item>
      <div class="field-hint">
        按数据流的 <code>@timestamp</code> 判断，Elasticsearch 后台异步执行；
        <strong>只删已有文档，不会停止采集</strong>（之后新写入的日志仍会进入这条流），流对象保留。
      </div>
    </a-form>
  </a-modal>
</template>

<script setup>
// 数据流清理弹窗（2026-09-19）：与「日志查询」里原来的清理入口**合并成一个**——
// 同一个动作在两处各写一套，文案和范围选项迟早会分叉。现在：
//   · 日志查询面板不再提供清理按钮（清理入口统一在**数据流列表**：日志中心 → 存储水位，每条流一个）；
//   · 本组件支持两种作用域：按逻辑服务（可带 tier 收窄到某条流）/ 按数据流名（未识别流的兜底路径）。
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'

import { cleanupLogDataStream, cleanupLogDataStreamByStream } from '@/api/monitor'

const props = defineProps({
  open: { type: Boolean, default: false },
  // { kind: 'service'|'stream', serviceId?, tier?, stream?, label? }
  scope: { type: Object, default: null },
})
const emit = defineEmits(['update:open', 'cleaned'])

const mode = ref('days')
const amount = ref(7)
const submitting = ref(false)

const maxAmount = computed(() => (mode.value === 'hours' ? 87600 : 3650))
const title = computed(() => (props.scope?.kind === 'stream' ? '清理数据流' : '清理日志数据'))
const targetLabel = computed(() => props.scope?.label || props.scope?.stream || '')
const alertDescription = computed(() => (
  props.scope?.kind === 'stream'
    ? '这是一条未识别流（不归属任何逻辑服务）：按时间清理只删除早于所选时间的数据；全部清空会删除这条数据流的所有文档。'
    : '按时间清理只删除早于所选时间的数据；全部清空会删除该逻辑服务数据流的所有文档（不限于当前档位）。'
))

// 每次打开都回到默认范围（保留 7 天），避免残留上一次的选择被"手快点掉"。
watch(() => props.open, (visible) => {
  if (!visible) return
  mode.value = 'days'
  amount.value = 7
})

async function submit() {
  const scope = props.scope
  if (!scope || (scope.kind === 'service' && !scope.serviceId) || (scope.kind === 'stream' && !scope.stream)) return
  const selectedMode = mode.value
  const selectedAmount = selectedMode === 'all' ? 0 : Number(amount.value || 0)
  if (selectedMode !== 'all' && selectedAmount < 1) {
    message.warning('请填写大于 0 的保留数量')
    return
  }
  submitting.value = true
  try {
    const response = scope.kind === 'stream'
      ? await cleanupLogDataStreamByStream({ stream: scope.stream, mode: selectedMode, amount: selectedAmount })
      : await cleanupLogDataStream({
        service_id: scope.serviceId, mode: selectedMode, amount: selectedAmount, tier: scope.tier || '',
      })
    const data = response?.data?.data || {}
    if (data.matched === false) {
      message.info('没有匹配到这条数据流，无需清理')
    } else {
      message.success('清理任务已提交，Elasticsearch 后台执行中')
    }
    emit('update:open', false)
    emit('cleaned', data)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '清理日志数据失败')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.cleanup-target {
  margin-top: 12px;
  padding: 8px 12px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  word-break: break-all;
}
.cleanup-target-label {
  color: rgba(0, 0, 0, 0.45);
  margin-right: 8px;
}
.cleanup-target-value {
  font-family: Menlo, Consolas, monospace;
}
</style>
