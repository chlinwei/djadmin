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
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { createWorkflow } from '@/api/sys/automation'

const router = useRouter()
const submitting = ref(false)

const form = reactive({
  name: '',
  description: '',
  enabled: true,
  default_extra_vars_text: '{}',
  remark: '',
})

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
