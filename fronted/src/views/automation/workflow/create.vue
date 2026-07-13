<template>
  <div class="workflow-create-page">
    <a-card title="创建 Workflow - 基础信息" size="small" class="block-card">
      <template #extra>
        <a-space>
          <a-button @click="goBack">返回列表</a-button>
          <a-button type="primary" :loading="submitting" @click="submitCreate">保存并进入编排</a-button>
        </a-space>
      </template>

      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="名称" required>
              <a-input v-model:value="form.name" placeholder="例如：生产发布编排" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item label="启用状态">
              <a-switch v-model:checked="form.enabled" checked-children="启用" un-checked-children="禁用" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="描述">
          <a-input v-model:value="form.description" placeholder="可选" />
        </a-form-item>

        <a-form-item label="选择Inventory（可选）">
          <a-select
            v-model:value="form.default_inventory"
            :getPopupContainer="getPopupContainer"
            :options="inventoryOptions"
            show-search
            allow-clear
            optionFilterProp="label"
            placeholder="未选择则按任务节点各自范围执行"
          />
        </a-form-item>

        <a-form-item>
          <a-alert
            type="info"
            show-icon
            message="Workflow 全局范围由所选 Inventory 决定；主机组请在 Inventory 管理中维护"
          />
        </a-form-item>

        <a-form-item label="默认 Limit（可选）">
          <a-input
            v-model:value="form.default_limit"
            :placeholder="LIMIT_INPUT_PLACEHOLDER"
          />
          <ScopePrecheckPanel
            :precheck-ok="limitPrecheckOk"
            :prechecking="limitPrechecking"
            :message="limitPrecheckText"
            :hosts="limitAllHosts"
            :matched-hosts="limitMatchedHosts"
            :show-limit-toggle="true"
            :show-target-filter="true"
            :limit-text="form.default_limit"
            @toggle-limit-host="handleCreateLimitToggle"
            @remove-limit-token="handleCreateRemoveLimitToken"
          />
        </a-form-item>

        <a-form-item label="默认变量 JSON">
          <a-textarea v-model:value="form.default_extra_vars_text" :rows="3" placeholder='例如：{"env":"prod"}' />
        </a-form-item>

        <a-alert
          type="info"
          show-icon
          message="先保存基础信息，再进入下一步向导创建第一个节点。"
        />
      </a-form>
    </a-card>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { createWorkflow, getInventoryList, precheckInventoryLimit } from '@/api/sys/automation'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import ScopePrecheckPanel from '../components/ScopePrecheckPanel.vue'
import {
  LIMIT_INPUT_PLACEHOLDER,
  mapInventoryOptions,
  removeLimitToken,
  resolveMatchedHostLimitToken,
  toggleLimitToken,
} from '../utils/scopeHelpers'

const router = useRouter()
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const submitting = ref(false)
const inventoryOptions = ref([])
const limitPrechecking = ref(false)
const limitPrecheckOk = ref(false)
const limitPrecheckMessage = ref('请选择 Inventory 后输入 Limit，系统将实时预检')
const limitAllHosts = ref([])
const limitMatchedHosts = ref([])
let limitPrecheckTimer = null
let limitPrecheckSeq = 0

const form = reactive({
  name: '',
  description: '',
  enabled: true,
  default_inventory: undefined,
  default_limit: '',
  default_extra_vars_text: '{}',
  remark: '',
})

const limitPrecheckText = computed(() => {
  if (limitPrechecking.value && limitPrecheckMessage.value === '正在预检...') {
    return '正在预检...'
  }
  return limitPrecheckMessage.value
})

function clearLimitPrecheckTimer() {
  if (limitPrecheckTimer) {
    clearTimeout(limitPrecheckTimer)
    limitPrecheckTimer = null
  }
}

async function loadInventoryOptions() {
  const res = await getInventoryList({ page: 1, page_size: 500, ordering: '-id' })
  const data = res?.data?.data || {}
  inventoryOptions.value = mapInventoryOptions(data.results)
}

async function doLimitPrecheck() {
  const inventoryId = Number(form.default_inventory)
  if (!Number.isInteger(inventoryId) || inventoryId <= 0) {
    limitPrecheckOk.value = false
    limitPrechecking.value = false
    limitAllHosts.value = []
    limitMatchedHosts.value = []
    limitPrecheckMessage.value = '请选择 Inventory 后输入 Limit，系统将实时预检'
    return
  }

  const currentSeq = ++limitPrecheckSeq
  limitPrechecking.value = true
  try {
    const limitText = String(form.default_limit || '').trim()
    const baseRes = await precheckInventoryLimit(inventoryId, { limit: '' })
    if (currentSeq !== limitPrecheckSeq) {
      return
    }

    let data = baseRes?.data?.data || {}
    if (limitText) {
      const narrowedRes = await precheckInventoryLimit(inventoryId, { limit: limitText })
      if (currentSeq !== limitPrecheckSeq) {
        return
      }
      data = narrowedRes?.data?.data || {}
    }

    const baseData = baseRes?.data?.data || {}
    limitPrecheckOk.value = !!data.ok
    limitAllHosts.value = Array.isArray(baseData.matched_hosts_preview) ? baseData.matched_hosts_preview : []
    limitMatchedHosts.value = Array.isArray(data.matched_hosts_preview) ? data.matched_hosts_preview : []
    limitPrecheckMessage.value = data.message || '预检完成'
  } catch (error) {
    if (currentSeq !== limitPrecheckSeq) {
      return
    }
    limitPrecheckOk.value = false
    limitAllHosts.value = []
    limitMatchedHosts.value = []
    limitPrecheckMessage.value = error?.message || '预检失败，请稍后重试'
  } finally {
    if (currentSeq === limitPrecheckSeq) {
      limitPrechecking.value = false
    }
  }
}

function scheduleLimitPrecheck(delay = 300) {
  clearLimitPrecheckTimer()
  limitPrecheckTimer = setTimeout(() => {
    doLimitPrecheck()
  }, delay)
}

function parseDefaultExtraVars() {
  const rawText = String(form.default_extra_vars_text || '').trim() || '{}'
  try {
    const parsed = JSON.parse(rawText)
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      throw new Error('默认变量必须是 JSON 对象')
    }
    return parsed
  } catch (error) {
    throw new Error('默认变量不是合法 JSON 对象')
  }
}

async function submitCreate() {
  const name = String(form.name || '').trim()
  if (!name) {
    message.error('名称不能为空')
    return
  }

  let defaultExtraVars
  try {
    defaultExtraVars = parseDefaultExtraVars()
  } catch (error) {
    message.error(error.message || '默认变量不是合法 JSON 对象')
    return
  }

  submitting.value = true
  try {
    const res = await createWorkflow({
      name,
      description: String(form.description || '').trim(),
      enabled: Boolean(form.enabled),
      default_inventory: Number(form.default_inventory || 0) > 0 ? Number(form.default_inventory) : null,
      default_limit: String(form.default_limit || '').trim(),
      default_extra_vars: defaultExtraVars,
      remark: String(form.remark || '').trim(),
      nodes: [],
      edges: [],
      entry_node_key: '',
    })

    const data = res?.data?.data || {}
    const createdId = Number(data.id || 0)
    if (!Number.isInteger(createdId) || createdId <= 0) {
      message.error('创建成功但未返回有效ID')
      return
    }

    message.success('基础信息已保存，进入节点向导')
    await router.push({
      path: '/sys/automation/workflow/editor',
      query: { id: createdId, wizard: 'first-node' },
    })
  } finally {
    submitting.value = false
  }
}

function goBack() {
  router.push('/sys/automation/workflow')
}

function handleCreateLimitToggle(item) {
  const token = resolveMatchedHostLimitToken(item)
  form.default_limit = toggleLimitToken(form.default_limit, token)
}

function handleCreateRemoveLimitToken(token) {
  form.default_limit = removeLimitToken(form.default_limit, token)
}

watch(
  () => [form.default_inventory, form.default_limit],
  () => {
    limitPrecheckMessage.value = '正在预检...'
    scheduleLimitPrecheck(300)
  },
)

onMounted(async () => {
  await loadInventoryOptions()
})

onBeforeUnmount(() => {
  clearLimitPrecheckTimer()
})
</script>

<style scoped>
.workflow-create-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.block-card :deep(.ant-card-body) {
  padding-top: 12px;
}
</style>
