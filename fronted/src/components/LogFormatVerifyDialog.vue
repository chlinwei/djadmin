<template>
  <!-- 日志格式认证弹窗：选依据（默认按实例抽样）→ 后端取样例跑规则 pipeline 判定必备字段。
       认证是"提交时回答一次"的动作，所以结果不走表单保存流程，而是独立的小弹窗。
       抽成共享组件的原因：逻辑服务编辑弹窗与日志中心页都要给同一条 (服务×日志定义) 发起认证，
       两处各写一份必然出现"两套提示文案、两种提交语义"。 -->
  <a-modal
    :open="open"
    :title="`格式认证：${target?.name || ''}`"
    :width="560"
    :confirm-loading="submitting"
    ok-text="开始认证"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <a-alert
      v-if="result && !result.passed"
      type="warning"
      show-icon
      class="verify-result"
      :message="failureMessage"
    >
      <template #description>
        <div v-if="failedTargets.length" class="verify-targets">
          <div v-for="(target, index) in failedTargets" :key="index" class="verify-target">
            <div class="verify-target-title">{{ targetLabel(target) }}</div>
            <div v-if="target.error" class="verify-target-error">{{ target.error }}</div>
            <div v-else-if="target.missing_fields?.length" class="verify-target-error">
              缺必备字段：{{ target.missing_fields.join('、') }}
            </div>
            <ul v-if="target.log_files?.length" class="verify-target-files">
              <li v-for="file in target.log_files" :key="file"><code>{{ file }}</code></li>
            </ul>
          </div>
        </div>
        <div>可以换个依据重试（实例抽样最贴近真实采集），或调整模板上的解析规则后重新认证；确认格式没问题时可以走人工豁免。</div>
      </template>
    </a-alert>
    <a-form layout="vertical" class="verify-form">
      <a-form-item label="认证依据">
        <a-radio-group v-model:value="form.source">
          <a-radio value="instance">实例抽样（取该实例最近 50 行真实日志）</a-radio>
          <a-radio value="sample_log">规则样例（用解析规则里保存的样例日志）</a-radio>
          <a-radio value="waiver">人工豁免（不做校验，记录确认人）</a-radio>
        </a-radio-group>
      </a-form-item>
      <template v-if="form.source === 'instance'">
        <a-form-item label="认证范围">
          <a-radio-group v-model:value="form.all_deployments">
            <a-radio :value="false">单个实例</a-radio>
            <a-radio :value="true">全部实例</a-radio>
          </a-radio-group>
          <div class="field-hint">
            每个实例上**所有匹配到的日志文件**都会逐一认证；任一实例或任一文件解析不出必备字段即不通过。
          </div>
        </a-form-item>
        <a-form-item v-if="!form.all_deployments" label="从哪个实例取样例">
          <a-select
            v-model:value="form.deployment_id"
            :options="deploymentOptions"
            :getPopupContainer="getPopupContainer"
            placeholder="选择一个已绑定的部署实例"
            allow-clear
          />
          <div v-if="!deploymentOptions.length" class="field-hint">
            这个逻辑服务还没有绑定部署实例，按实例抽样无从获取：可以先绑定实例并保存，或改用规则样例。
          </div>
        </a-form-item>
        <a-form-item v-else label="认证实例">
          <span class="field-hint">将对已绑定的全部部署实例逐一认证（共 {{ deploymentOptions.length }} 个）。</span>
        </a-form-item>
      </template>
    </a-form>
    <div class="field-hint">
      认证是抽样通过，不证明文件里每一行都合规（同一文件可能混着启动横幅、堆栈续行）。
      通过后不再持续检查，只有模板日志定义、解析规则、服务宏或应用版本变化时才要求重新认证。
    </div>
  </a-modal>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { verifyApplicationServiceLogFormat } from '@/api/assets/application'

const props = defineProps({
  open: { type: Boolean, default: false },
  // 认证按"库里已保存的服务"执行：没有 serviceId 就没法认证（调用方负责禁用入口）。
  serviceId: { type: [Number, String], default: null },
  // 被认证的日志行：至少要带 log_definition 与 name。
  target: { type: Object, default: null },
  // 候选部署实例（{label, value}）。由调用方给，因为"哪些实例可用"取决于它的服务绑定关系：
  // 认证在后端按库里的绑定取实例，不能拿表单里未保存的勾选。
  deploymentOptions: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:open', 'verified'])

const submitting = ref(false)
const result = ref(null)
const form = reactive({ source: 'instance', all_deployments: false, deployment_id: null })
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

// 不通过的实例明细：实例自身的 Error 或缺失字段。全部通过时为空（弹窗会关闭）。
const failedTargets = computed(() => (result.value?.targets || []).filter(
  (target) => target.error || (target.missing_fields || []).length,
))

const failureMessage = computed(() => {
  const missing = result.value?.missing_fields || []
  if (missing.length) return `校验未通过：规则解析不出这些必备字段 —— ${missing.join('、')}`
  return '校验未通过：有实例取不到样例或解析不出记录'
})

function targetLabel(target) {
  const name = target.deployment_instance_name || target.host_instance_name || `实例#${target.deployment_id}`
  const host = target.host_instance_name && target.host_instance_name !== name
    ? `（主机 ${target.host_instance_name}）`
    : ''
  return name + host
}

// 每次打开都重置：上一次的"未通过"提示与选中的依据不该带到下一条日志上。
// immediate 是必须的——调用方可能直接以 open=true 挂载（不经过 false→true 的切换），
// 少了它表单会停在未初始化状态（实例依据没有预选实例，提交直接被拦下）。
watch(() => props.open, (open) => {
  if (!open) return
  result.value = null
  form.source = 'instance'
  form.all_deployments = false
  form.deployment_id = props.deploymentOptions.length ? props.deploymentOptions[0].value : null
}, { immediate: true })

async function submit() {
  if (!props.target) return
  if (form.source === 'instance' && !form.all_deployments && !form.deployment_id) {
    message.warning('按实例抽样认证需要先选择一个部署实例')
    return
  }
  submitting.value = true
  try {
    const response = await verifyApplicationServiceLogFormat(props.serviceId, {
      log_definition_id: props.target.log_definition,
      source: form.source,
      all_deployments: form.source === 'instance' ? form.all_deployments : false,
      deployment_id: form.source === 'instance' && !form.all_deployments ? form.deployment_id : 0,
    })
    const data = response?.data?.data || {}
    if (data.passed) {
      message.success(`格式认证通过（依据：${data.format_verified_source || form.source}）`)
      emit('update:open', false)
      // 由调用方重载列表：状态/时间/操作人都由后端按指纹比对给出，前端不自己拼。
      emit('verified', data)
    } else {
      // 不通过不是接口错误：留在弹窗里展示缺哪几个字段、哪台实例有问题，允许换依据重试。
      result.value = data
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '认证失败，请稍后重试')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.field-hint {
  margin-top: 4px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
.verify-result {
  margin-bottom: 12px;
}
.verify-form :deep(.ant-form-item) {
  margin-bottom: 12px;
}
.verify-targets {
  margin-bottom: 8px;
}
.verify-target {
  margin-bottom: 6px;
}
.verify-target-title {
  color: rgba(0, 0, 0, 0.75);
  font-size: 12px;
  font-weight: 600;
}
.verify-target-error {
  color: #d4380d;
  font-size: 12px;
}
.verify-target-files {
  margin: 2px 0 0;
  padding-left: 16px;
  font-size: 12px;
}
</style>
