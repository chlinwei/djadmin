<template>
  <!-- 日志格式认证弹窗：选依据（默认按实例抽样）→ 后端取样例跑规则 pipeline 判定必备字段。
       认证是"提交时回答一次"的动作，所以结果不走表单保存流程，而是独立的小弹窗。
       抽成共享组件的原因：单条认证（日志中心按行的「发起认证」）与批量认证（勾选多条一起认证）
       是同一件事的两面，各写一份必然出现"两套提示文案、两种提交语义"。
       **认证入口只在日志中心**（2026-09-20 起逻辑服务编辑弹窗不再编辑逐条日志配置）。

       单条与批量是**同一个组件**：传 target = 认证这一条，传 targets = 勾选后一起认证（逐条串行、
       逐条进度、失败逐条列出）。这样"认证依据""认证范围""后果说明"只有一份文案。 -->
  <a-modal
    :open="open"
    :title="modalTitle"
    :width="560"
    :confirm-loading="submitting"
    ok-text="开始认证"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <!-- 批量认证是串行的（每条都要去主机取样例），所以要让人看见"跑到第几条"，
         而不是一个转圈的确定按钮让人怀疑卡死了。 -->
    <a-alert
      v-if="submitting && batchMode"
      type="info"
      show-icon
      class="verify-result"
      :message="`正在认证第 ${progress.current}/${progress.total} 条：${progress.name}`"
    />
    <a-alert
      v-if="failures.length"
      type="warning"
      show-icon
      class="verify-result"
      :message="failureMessage"
    >
      <template #description>
        <div class="verify-targets">
          <div v-for="(item, index) in failures" :key="index" class="verify-target">
            <div v-if="batchMode" class="verify-target-title">{{ item.name }}</div>
            <div v-if="item.message" class="verify-target-error">{{ item.message }}</div>
            <div v-else-if="item.missing_fields?.length" class="verify-target-error">
              缺必备字段：{{ item.missing_fields.join('、') }}
            </div>
            <template v-for="(target, targetIndex) in failedTargetsOf(item)" :key="targetIndex">
              <div class="verify-target-title">{{ targetLabel(target) }}</div>
              <div v-if="target.error" class="verify-target-error">{{ target.error }}</div>
              <div v-else-if="target.missing_fields?.length" class="verify-target-error">
                缺必备字段：{{ target.missing_fields.join('、') }}
              </div>
              <ul v-if="target.log_files?.length" class="verify-target-files">
                <li v-for="file in target.log_files" :key="file"><code>{{ file }}</code></li>
              </ul>
            </template>
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
    <div v-if="batchMode" class="field-hint">
      本次将对勾选的 {{ targetList.length }} 条日志**逐条**认证（每条都按上面的依据与范围）。
      部分不通过不影响其余的认证结果，失败的那几条会列出来。
    </div>
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
  // 批量认证的目标（日志行数组，每项至少带 log_definition 与 name）。非空时优先于 target：
  // 二者都传会让"要认证几条"变得说不清，所以调用方只传其中一个。
  targets: { type: Array, default: () => [] },
  // 候选部署实例（{label, value}）。由调用方给，因为"哪些实例可用"取决于它的服务绑定关系：
  // 认证在后端按库里的绑定取实例，不能拿表单里未保存的勾选。
  deploymentOptions: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:open', 'verified'])

const submitting = ref(false)
// 未通过的逐条结果（单条认证时就是那一条）：{name, log_definition, passed, message?, missing_fields?, targets?}。
const failures = ref([])
// 批量时的逐条进度：认证要去主机取样例，几十秒一轮，没有进度就没法判断"还在跑"还是"卡住了"。
const progress = reactive({ current: 0, total: 0, name: '' })
const form = reactive({ source: 'instance', all_deployments: false, deployment_id: null })
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const targetList = computed(() => (props.targets?.length ? props.targets : (props.target ? [props.target] : [])))
const batchMode = computed(() => targetList.value.length > 1)
const modalTitle = computed(() => (batchMode.value
  ? `格式认证：${targetList.value.length} 条日志`
  : `格式认证：${props.target?.name || ''}`))
// 单条认证的失败结果（沿用原来的口径：调用方/用例读它拿缺哪些必备字段）。
const result = computed(() => (batchMode.value ? null : (failures.value[0] || null)))
const failureMessage = computed(() => {
  if (batchMode.value) return `有 ${failures.value.length}/${targetList.value.length} 条日志未通过格式认证`
  const missing = failures.value[0]?.missing_fields || []
  if (missing.length) return `校验未通过：规则解析不出这些必备字段 —— ${missing.join('、')}`
  return '校验未通过：有实例取不到样例或解析不出记录'
})

// 某一条日志里"具体是哪台实例/哪个文件"的问题：实例自身的 Error 或缺失字段。全通过时为空。
function failedTargetsOf(item) {
  return (item?.targets || []).filter((target) => target.error || (target.missing_fields || []).length)
}

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
  failures.value = []
  progress.current = 0
  progress.total = 0
  progress.name = ''
  form.source = 'instance'
  form.all_deployments = false
  form.deployment_id = props.deploymentOptions.length ? props.deploymentOptions[0].value : null
}, { immediate: true })

async function submit() {
  const targets = targetList.value
  if (!targets.length) return
  if (form.source === 'instance' && !form.all_deployments && !form.deployment_id) {
    message.warning('按实例抽样认证需要先选择一个部署实例')
    return
  }
  submitting.value = true
  failures.value = []
  progress.total = targets.length
  try {
    for (let index = 0; index < targets.length; index += 1) {
      const target = targets[index]
      progress.current = index + 1
      progress.name = target.name || `日志 #${target.log_definition}`
      try {
        const response = await verifyApplicationServiceLogFormat(props.serviceId, {
          log_definition_id: target.log_definition,
          source: form.source,
          all_deployments: form.source === 'instance' ? form.all_deployments : false,
          deployment_id: form.source === 'instance' && !form.all_deployments ? form.deployment_id : 0,
        })
        const data = response?.data?.data || {}
        // 不通过不是接口错误：留在弹窗里展示缺哪几个字段、哪台实例有问题，允许换依据重试。
        if (!data.passed) {
          failures.value = [...failures.value, { ...data, name: target.name, log_definition: target.log_definition }]
        }
      } catch (error) {
        // 单条报错（主机离线、接口失败）不中断整批：记下来继续跑，最后一起列出来。
        failures.value = [...failures.value, {
          name: target.name, log_definition: target.log_definition, passed: false,
          message: error?.response?.data?.msg || error?.message || '认证失败，请稍后重试',
        }]
      }
    }
  } finally {
    submitting.value = false
    progress.total = 0
    progress.name = ''
  }
  if (!failures.value.length) {
    message.success(batchMode.value
      ? `${targets.length} 条日志的格式认证全部通过（依据：${form.source}）`
      : `格式认证通过（依据：${form.source}）`)
    emit('update:open', false)
    // 由调用方重载列表：状态/时间/操作人都由后端按指纹比对给出，前端不自己拼。
    emit('verified', batchMode.value ? { count: targets.length } : { passed: true })
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
