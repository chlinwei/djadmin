<template>
  <div class="playbook-template-page">
    <a-row :gutter="16" class="top-tools">
      <a-col :span="14">
        <a-input-search
          v-model:value="keyword"
          placeholder="搜索模板名称或编码"
          allow-clear
          enter-button
          size="large"
          @search="loadPlaybooks"
        />
      </a-col>
      <a-col :span="10" class="right-tools">
        <a-space>
          <a-button size="large" @click="openPlaybookModal()" v-permission="'automation:playbooks:create'">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增模板</span>
          </a-button>
          <a-button size="large" type="primary" ghost :loading="playbookLoading" @click="loadPlaybooks">
            <FontAwesomeIcon :icon="['fas', 'arrows-rotate']" :spin="playbookLoading" />
            <span>&nbsp;刷新</span>
          </a-button>
        </a-space>
      </a-col>
    </a-row>

    <a-card title="Playbook 模板" size="small" class="block-card">
      <a-table
        :columns="playbookColumns"
        :data-source="playbooks"
        :loading="playbookLoading"
        :pagination="playbookPagination"
        rowKey="id"
        size="small"
        @change="handlePlaybookTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-tag :color="record.enabled ? 'green' : 'default'">
              {{ record.enabled ? '启用' : '禁用' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button size="small" type="primary" @click="openPlaybookModal(record)" v-permission="'automation:playbooks:update'">
                <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
              </a-button>
              <a-popconfirm
                title="确认删除该模板吗？"
                ok-text="确认"
                cancel-text="取消"
                @confirm="onDeletePlaybook(record)"
              >
                <a-button size="small" danger v-permission="'automation:playbooks:delete'">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      :title="playbookEdit.id ? '编辑模板' : '新增模板'"
      :open="playbookModalVisible"
      :confirmLoading="playbookSubmitting"
      :width="840"
      @ok="submitPlaybook"
      @cancel="playbookModalVisible = false"
    >
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="模板名称" required>
              <a-input v-model:value="playbookEdit.name" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="模板编码" required>
              <a-input v-model:value="playbookEdit.code" :disabled="!!playbookEdit.id" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="描述">
          <a-input v-model:value="playbookEdit.description" />
        </a-form-item>
        <a-form-item label="默认变量 JSON">
          <a-input v-model:value="playbookEdit.defaultExtraVarsText" placeholder='例如: {"batch":5}' />
        </a-form-item>
        <a-form-item label="模板内容（YAML）" required>
          <a-textarea v-model:value="playbookEdit.content" :rows="14" placeholder="---&#10;- hosts: all&#10;  gather_facts: false&#10;  tasks:&#10;    - name: ping&#10;      ping:" />
        </a-form-item>
        <a-form-item>
          <a-switch v-model:checked="playbookEdit.enabled" checked-children="启用" un-checked-children="禁用" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  getPlaybookList,
  createPlaybook,
  updatePlaybook,
  deletePlaybook,
} from '@/api/sys/automation'

const keyword = ref('')
const playbooks = ref([])
const playbookLoading = ref(false)
const playbookModalVisible = ref(false)
const playbookSubmitting = ref(false)
const playbookPagination = reactive({ current: 1, pageSize: 10, total: 0, showSizeChanger: true, showQuickJumper: true })

const playbookEdit = reactive({
  id: null,
  name: '',
  code: '',
  description: '',
  defaultExtraVarsText: '',
  content: '',
  enabled: true,
})

const playbookColumns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '编码', dataIndex: 'code', key: 'code' },
  { title: '描述', dataIndex: 'description', key: 'description' },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: '操作', key: 'action', width: 120 },
]

function parseJsonText(text, fieldLabel) {
  if (!text || !String(text).trim()) {
    return {}
  }
  try {
    const parsed = JSON.parse(text)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed
    }
    throw new Error('必须是 JSON 对象')
  } catch (error) {
    throw new Error(`${fieldLabel} 格式错误: ${error.message}`)
  }
}

async function loadPlaybooks() {
  playbookLoading.value = true
  try {
    const res = await getPlaybookList({
      page: playbookPagination.current,
      page_size: playbookPagination.pageSize,
      search: keyword.value,
      ordering: '-id',
    })
    const data = res?.data?.data || {}
    playbooks.value = data.results || []
    playbookPagination.total = data.count || 0
  } finally {
    playbookLoading.value = false
  }
}

function resetPlaybookEdit() {
  playbookEdit.id = null
  playbookEdit.name = ''
  playbookEdit.code = ''
  playbookEdit.description = ''
  playbookEdit.defaultExtraVarsText = ''
  playbookEdit.content = ''
  playbookEdit.enabled = true
}

function openPlaybookModal(record = null) {
  resetPlaybookEdit()
  if (record) {
    playbookEdit.id = record.id
    playbookEdit.name = record.name || ''
    playbookEdit.code = record.code || ''
    playbookEdit.description = record.description || ''
    playbookEdit.defaultExtraVarsText = JSON.stringify(record.default_extra_vars || {})
    playbookEdit.content = record.content || ''
    playbookEdit.enabled = !!record.enabled
  }
  playbookModalVisible.value = true
}

async function submitPlaybook() {
  if (!playbookEdit.name || !playbookEdit.code || !playbookEdit.content) {
    message.error('名称、编码、模板内容为必填项')
    return
  }

  let defaultExtraVars = {}
  try {
    defaultExtraVars = parseJsonText(playbookEdit.defaultExtraVarsText, '默认变量 JSON')
  } catch (error) {
    message.error(error.message)
    return
  }

  playbookSubmitting.value = true
  const payload = {
    name: playbookEdit.name,
    code: playbookEdit.code,
    description: playbookEdit.description,
    content: playbookEdit.content,
    enabled: playbookEdit.enabled,
    default_extra_vars: defaultExtraVars,
  }

  try {
    if (playbookEdit.id) {
      await updatePlaybook(playbookEdit.id, payload)
      message.success('模板更新成功')
    } else {
      await createPlaybook(payload)
      message.success('模板创建成功')
    }
    playbookModalVisible.value = false
    await loadPlaybooks()
  } finally {
    playbookSubmitting.value = false
  }
}

async function onDeletePlaybook(record) {
  await deletePlaybook(record.id)
  message.success('模板删除成功')
  await loadPlaybooks()
}

function handlePlaybookTableChange(page) {
  playbookPagination.current = page.current
  playbookPagination.pageSize = page.pageSize
  loadPlaybooks()
}

onMounted(async () => {
  await loadPlaybooks()
})
</script>

<style scoped>
.playbook-template-page {
  padding: 2px;
}

.top-tools {
  margin-bottom: 12px;
}

.right-tools {
  display: flex;
  justify-content: flex-end;
}

.block-card {
  margin-bottom: 12px;
}
</style>
