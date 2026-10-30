<template>
  <a-modal :open="open" title="交互类型管理" :width="900" centered :footer="null" @cancel="emit('update:open', false)">
    <a-alert type="info" show-icon message="类型编码创建后不可修改；停用后禁止新建引用，已有关系保留。被引用的类型不能删除。" style="margin-bottom: 16px" />
    <a-space style="margin-bottom: 16px">
      <a-button v-if="canCreate" size="large" :disabled="loading || !!deleting" @click="openEditor(null)"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" /><span>&nbsp;新增类型</span></a-button>
      <a-button :loading="loading" @click="load">刷新</a-button>
    </a-space>
    <a-alert v-if="error" type="error" show-icon :message="error" style="margin-bottom: 12px" />
    <a-table :columns="columns" :data-source="types" row-key="id" size="small" :locale="tableLocale" :loading="loading" :scroll="{ x: 800 }" :pagination="pagination" @change="handleTableChange">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'enabled'"><a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag></template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-tooltip v-if="canUpdate" title="编辑" placement="top"><a-button size="small" type="primary" :disabled="!!deleting" @click="openEditor(record)"><FontAwesomeIcon :icon="['fas', 'pen-to-square']" /></a-button></a-tooltip>
            <a-tooltip v-if="canDelete" title="删除" placement="top"><a-button class="delBtn" size="small" type="primary" danger :loading="deleting === record.id" :disabled="!!deleting" @click="remove(record)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip>
          </a-space>
        </template>
      </template>
    </a-table>
  </a-modal>
  <a-modal :open="editorOpen" :title="editingId ? '编辑交互类型' : '新增交互类型'" centered :mask-closable="false" :closable="!saving" @cancel="closeEditor">
    <a-form ref="formRef" :model="form" layout="vertical">
      <a-form-item label="稳定编码" name="code" :rules="[{ required: true, pattern: /^[a-z][a-z0-9_-]{0,63}$/, message: '小写字母开头，仅含小写字母、数字、下划线和短横线，最长 64 字符' }]">
        <a-input v-model:value="form.code" :disabled="!!editingId || saving" :maxlength="64" placeholder="例如 http、database、consume" />
      </a-form-item>
      <a-form-item label="显示名称" name="name" :rules="[{ required: true, whitespace: true, message: '请输入显示名称' }]"><a-input v-model:value="form.name" :maxlength="128" :disabled="saving" /></a-form-item>
      <a-form-item label="启用"><a-switch v-model:checked="form.enabled" :disabled="saving" /></a-form-item>
      <a-form-item label="说明"><a-textarea v-model:value="form.description" :maxlength="500" :rows="3" :disabled="saving" /></a-form-item>
    </a-form>
    <a-alert v-if="saveError" type="error" show-icon :message="saveError" />
    <template #footer><a-button :disabled="saving" @click="closeEditor">取消</a-button><a-button type="primary" :loading="saving" @click="save">保存</a-button></template>
  </a-modal>
</template>

<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { deleteServiceDependencyTypes, getServiceDependencyTypes, saveServiceDependencyType } from '@/api/assets/serviceDependencies'
import { checkPermission } from '@/directives/permission/permission'
import { assertBatchDeleteSucceeded, openDeleteConfirm } from '@/util/deleteConfirm'
import { fetchAllPages } from '@/util/fetchAllPages'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { dependencyData, dependencyPage } from './dependencyGraph'

const props = defineProps({ open: Boolean })
const emit = defineEmits(['update:open', 'changed'])
const canCreate = computed(() => checkPermission('assets:applications:create'))
const canUpdate = computed(() => checkPermission('assets:applications:update'))
const canDelete = computed(() => checkPermission('assets:applications:delete'))
const columns = [
  { title: '名称', dataIndex: 'name', width: 140 },
  { title: '编码', dataIndex: 'code', width: 130 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '说明', dataIndex: 'description', width: 260 },
  { title: '操作', key: 'action', fixed: 'right', width: 120 },
]
const types = ref([])
const pagination = reactive(createPagination())
const loading = ref(false)
const deleting = ref(null)
const error = ref('')
const editorOpen = ref(false)
const editingId = ref(null)
const formRef = ref()
const form = reactive({ code: '', name: '', enabled: true, description: '' })
const saving = ref(false)
const saveError = ref('')
let generation = 0
let alive = true

async function load(prefix = '') {
  const current = ++generation
  loading.value = true
  error.value = ''
  try {
    const records = await fetchAllPages(async (params) => dependencyPage(await getServiceDependencyTypes(params)))
    if (current === generation) {
      types.value = records
      Object.assign(pagination, { current: 1, total: records.length })
    }
  } catch (failure) {
    if (current !== generation) return
    types.value = []
    error.value = `${typeof prefix === 'string' ? prefix : ''}${failure.message || '加载交互类型失败'}`
  } finally { if (current === generation) loading.value = false }
}

function openEditor(record) {
  editingId.value = record?.id ?? null
  Object.assign(form, { code: record?.code ?? '', name: record?.name ?? '', enabled: record?.enabled ?? true, description: record?.description ?? '' })
  saveError.value = ''
  editorOpen.value = true
  formRef.value?.clearValidate()
}
function closeEditor() { if (!saving.value) editorOpen.value = false }
function handleTableChange(page) { Object.assign(pagination, { current: page.current, pageSize: page.pageSize }) }

async function save() {
  if (saving.value) return
  try { await formRef.value.validate() } catch (validation) {
    if (!validation?.errorFields) saveError.value = validation?.message || '表单校验失败'
    return
  }
  saving.value = true
  saveError.value = ''
  try {
    dependencyData(await saveServiceDependencyType(editingId.value, { ...form, name: form.name.trim(), description: form.description.trim() }))
    if (!alive) return
    editorOpen.value = false
    message.success('交互类型已保存')
    emit('changed')
    await load('类型已保存，但列表刷新失败：')
  } catch (failure) {
    if (alive) saveError.value = failure.message || '保存交互类型失败'
  } finally { saving.value = false }
}

function remove(record) {
  openDeleteConfirm({
    title: '删除交互类型',
    summary: '只有未被依赖关系引用的类型才能删除。',
    items: [`${record.name}（${record.code}，#${record.id}）`],
    onConfirm: async () => {
      deleting.value = record.id
      let failure
      try { assertBatchDeleteSucceeded(await deleteServiceDependencyTypes([record.id]), [record.id]) } catch (reason) { failure = reason }
      if (alive) {
        emit('changed')
        await load('删除请求已处理，但列表刷新失败：')
      }
      deleting.value = null
      if (failure) {
        error.value = [failure.message || '删除交互类型失败', error.value].filter(Boolean).join('；')
        throw failure
      }
    },
  })
}
watch(() => props.open, (open) => { if (open) load() }, { immediate: true })
onBeforeUnmount(() => { alive = false; generation++ })
</script>
