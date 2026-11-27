<template>
  <a-result v-if="!can('view')" status="403" title="无权查看维护窗口" />
  <a-card v-else title="维护窗口" size="small">
    <template #extra>
      <a-space>
        <a-button @click="load">刷新</a-button>
        <a-button v-if="can('create')" size="large" @click="openEditor()">
          <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;新增
        </a-button>
      </a-space>
    </template>
    <a-alert show-icon type="info" message="维护期间继续监测并记录告警，仅暂停命中范围的正式媒介通知。时间按您的时区显示。" />
    <a-alert v-if="listError" show-icon type="error" :message="listError" />
    <a-table row-key="id" size="small" :data-source="rows" :columns="columns" :loading="loading"
      :pagination="pagination" :locale="tableLocale" :scroll="{ x: 1410 }" @change="changePage">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'title'"><a @click="inspect(record.id)">{{ record.title }}</a></template>
        <template v-else-if="column.key === 'status'"><a-tag :color="colors[record.status]">{{ states[record.status] }}</a-tag></template>
        <template v-else-if="column.key === 'time'">{{ format(record.starts_at) }} → {{ format(record.ends_at) }}</template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-tooltip v-if="editable(record)" title="编辑" placement="top">
              <a-button size="small" type="primary" aria-label="编辑" @click="inspect(record.id)"><FontAwesomeIcon :icon="['fas','pen-to-square']" /></a-button>
            </a-tooltip>
            <ViewRecordButton v-else @click="inspect(record.id)" />
            <a-tooltip v-if="record.status === 'scheduled' && can('cancel')" title="取消" placement="top">
              <a-button size="small" @click="confirmClose(record)">取消</a-button>
            </a-tooltip>
            <a-tooltip v-if="record.status === 'active' && can('end')" title="提前结束" placement="top">
              <a-button size="small" @click="confirmClose(record)">提前结束</a-button>
            </a-tooltip>
            <a-tooltip v-if="can('delete')" title="删除" placement="top">
              <a-button size="small" danger class="delBtn" aria-label="删除" @click="confirmDelete(record)"><FontAwesomeIcon :icon="faTrash" /></a-button>
            </a-tooltip>
          </a-space>
        </template>
      </template>
    </a-table>
  </a-card>

  <a-modal v-model:open="editor" :title="readonly ? '查看维护窗口' : form.id ? '编辑维护窗口' : '新增维护窗口'" :footer="null" centered width="780px">
    <a-form layout="vertical">
      <a-descriptions v-if="detail" size="small">
        <a-descriptions-item label="状态">{{ states[detail.status] }}</a-descriptions-item>
        <a-descriptions-item label="创建人">{{ detail.created_by }}</a-descriptions-item>
        <a-descriptions-item label="抑制通知数">{{ detail.suppressed_count }}</a-descriptions-item>
      </a-descriptions>
      <a-form-item label="窗口名称" required><a-input v-model:value="form.title" :readonly="readonly" :maxlength="200" /></a-form-item>
      <a-form-item label="维护原因" required><a-textarea v-model:value="form.reason" :readonly="readonly" :rows="2" :maxlength="4000" /></a-form-item>
      <a-divider orientation="left">维护时段（{{ timezone }}）</a-divider>
      <a-row :gutter="16">
        <a-col :xs="24" :sm="12"><a-form-item label="开始时间" required>
          <a-date-picker v-model:value="form.start" :disabled="readonly || !!form.lockedStart" :show-time="{ defaultValue: dayjs().startOf('day') }" placeholder="开始时间" :getPopupContainer="getPopupContainer" />
        </a-form-item></a-col>
        <a-col :xs="24" :sm="12"><a-form-item label="结束时间（不含）" required>
          <a-date-picker v-model:value="form.end" :disabled="readonly" :show-time="{ defaultValue: dayjs().endOf('day') }" placeholder="结束时间" :getPopupContainer="getPopupContainer" />
        </a-form-item></a-col>
      </a-row>
      <a-divider orientation="left">影响范围</a-divider>
      <p>每行选择主机或服务树，命中任一行即可。选择项目包含整个项目；下方的 Label 条件对所有范围生效。</p>
      <a-alert v-if="unverified" show-icon type="warning" message="此告警没有已确认的资产主机。当前仅按告警名预填，请核对影响范围。" />
      <p v-if="!form.scopes.length">未限定主机或服务树</p>
      <div v-for="(scope, i) in form.scopes" :key="scope.key" class="scope-row">
        <a-select :value="scope.kind" :disabled="readonly" :aria-label="`第 ${i + 1} 行范围类型`" :getPopupContainer="getPopupContainer"
          :options="scopeTypes" @update:value="kind => { scope.kind = kind; scope.values = [] }" />
        <AssetSelector v-if="scope.kind === 'host'" v-model="scope.values" :disabled="readonly" :scope-names="detail?.scope_names" />
        <a-tree-select v-else v-model:value="scope.values" multiple allow-clear show-search :disabled="readonly"
          tree-node-filter-prop="pathTitle" tree-node-label-prop="pathTitle" :tree-data="readonly ? selectedTreeData(scope.values) : scopeTree" :loading="!readonly && treeLoading"
          :getPopupContainer="getPopupContainer" placeholder="选择项目、业务、环境或逻辑服务"
          @dropdownVisibleChange="open => open && loadScopeTree()" />
        <a-button v-if="!readonly" @click="form.scopes.splice(i, 1)">移除范围</a-button>
      </div>
      <small v-if="!readonly && treeError && form.scopes.some(s => s.kind === 'tree')" role="alert">{{ treeError }}</small>
      <a-form-item v-if="!readonly"><a-button @click="form.scopes.push(newScope())">添加范围</a-button></a-form-item>
      <a-form-item label="告警标签（告警名用 alertname，级别用 severity）">
        <p v-if="!form.labels.length">未限定告警标签</p>
        <a-space v-for="(m,i) in form.labels" :key="i" class="condition">
          <a-input v-model:value="m.label" :readonly="readonly" placeholder="Label 名称" />
          <a-select v-model:value="m.operator" :disabled="readonly" style="width: 8em" :options="operators" :getPopupContainer="getPopupContainer" />
          <a-input v-model:value="m.value" :readonly="readonly" placeholder="标签值" />
          <a-button v-if="!readonly" @click="form.labels.splice(i, 1)">移除</a-button>
        </a-space>
        <a-button v-if="!readonly" @click="form.labels.push({ type:'label', label:'', operator:'=', value:'' })">添加 Label 条件</a-button>
      </a-form-item>
      <a-alert v-if="preview" type="info" show-icon :message="`当前命中 ${preview.count} 条告警`">
        <template #description>
          <ul class="preview-list"><li v-for="(a, i) in preview.results" :key="i">{{ a.name }} · {{ a.host_name || a.instance || '无资产归属' }}
            · 项目：{{ a.project_ids?.map(id => `#${id}`).join('、') || '未关联' }} · 命中：{{ scopeSummary(a.matched_by) }}</li></ul>
          <span v-if="preview.count === 0">当前没有命中，不影响保存。</span>
        </template>
      </a-alert>
      <a-alert v-if="formError" type="error" show-icon :message="formError" />
      <a-space class="footer"><a-button @click="editor=false">{{ readonly ? '关闭' : '取消' }}</a-button>
        <a-button v-if="!readonly" :loading="previewing" @click="previewMatches">预览影响范围</a-button>
        <a-button type="primary" v-if="!readonly && can(form.id ? 'update' : 'create')" :loading="saving" @click="save">保存</a-button>
      </a-space>
    </a-form>
  </a-modal>
</template>

<script setup>
import ViewRecordButton from '@/components/ViewRecordButton.vue'
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Modal, message } from 'ant-design-vue'
import dayjs from 'dayjs'
import { faTrash } from '@fortawesome/free-solid-svg-icons'
import { openDeleteConfirm, assertBatchDeleteSucceeded } from '@/util/deleteConfirm'
import store from '@/store'
import { checkPermission } from '@/directives/permission/permission'
import { formatTimeWithTimezone } from '@/util/timezone'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { createPagination, tableLocale } from '@/util/tableStyle'
import { listWindows, windowDetail, previewWindow, saveWindow, cancelWindow, endWindow, deleteWindows, getSourceAlert } from '@/api/maintenance'
import AssetSelector from './AssetSelector.vue'
import { initForm, newScope, buildWindow, buildMatchers, operators, scopeSummary } from './form'
import { buildAlertScopeTreeData, loadAlertScopeAssets } from '@/util/alertScope'

const route = useRoute(), router = useRouter()
const can = action => checkPermission('monitor:view') && checkPermission('monitor:maintenance:view') && checkPermission(`monitor:maintenance:${action}`)
const timezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const format = value => formatTimeWithTimezone(value, timezone.value)
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const states = { scheduled:'待生效', active:'生效中', expired:'已结束', ended:'已提前结束', cancelled:'已取消' }
const colors = { scheduled:'blue', active:'orange' }
const scopeTypes = [{ value:'tree', label:'服务树' }, { value:'host', label:'主机' }]
const columns = [
  { title:'名称', key:'title', width:180 }, { title:'状态', key:'status', width:100 },
  { title:'时段', key:'time', width:320 },
  { title:'原因', dataIndex:'reason', width:160 }, { title:'创建人', dataIndex:'created_by', width:110 },
  { title:'抑制通知数', dataIndex:'suppressed_count', width:110 }, { title:'操作', key:'action', fixed:'right', width:245 },
]
const rows = ref([]), pagination = reactive(createPagination()), loading = ref(false), listError = ref('')
const editor = ref(false), form = ref(initForm({}, timezone.value)), formError = ref(''), saving = ref(false)
const scopeTree = ref([]), treeLoading = ref(false), treeError = ref('')
async function loadScopeTree() {
  if (readonly.value || treeLoading.value) return
  treeLoading.value = true; treeError.value = ''
  try {
    scopeTree.value = buildAlertScopeTreeData(await loadAlertScopeAssets(), { businessScopedEnvironments: true })
  } catch (e) { scopeTree.value = []; treeError.value = e.message || '服务树加载失败，请重新展开重试' }
  finally { treeLoading.value = false }
}
const unverified = ref(false)
const previewing = ref(false), preview = ref(null), detail = ref(null)
const editable = record => ['scheduled','active'].includes(record.status) && can('update')
const readonly = computed(() => !!detail.value && !editable(detail.value))
function selectedTreeData(values) {
  return values.map(value => ({ value, title: detail.value.scope_names[value], pathTitle: detail.value.scope_names[value] }))
}
let openVersion = 0
let previewVersion = 0, listVersion = 0
async function load() {
  const version = ++listVersion
  loading.value = true; listError.value = ''
  try {
    const { data: { data } } = await listWindows({ page:pagination.current, page_size:pagination.pageSize })
    if (version === listVersion) { rows.value = data.results; pagination.total = data.count }
  } catch (e) { if (version === listVersion) listError.value = e.message || '加载维护窗口失败' }
  finally { if (version === listVersion) loading.value = false }
}
function changePage(page) { Object.assign(pagination, { current:page.current, pageSize:page.pageSize }); load() }
function openEditor(row, prefill) {
  ++openVersion
  detail.value = row || null
  unverified.value = Boolean(prefill?.alertname && !prefill.host_id)
  form.value = initForm(row, timezone.value, prefill); formError.value = ''; preview.value = null; editor.value = true
  if (!readonly.value && form.value.scopes.some(s => s.kind === 'tree')) loadScopeTree()
}
watch(() => [form.value.scopes,form.value.labels], () => { ++previewVersion; previewing.value = false; preview.value = null }, { deep:true, flush:'sync' })
async function previewMatches() {
  const version = ++previewVersion
  previewing.value = true; formError.value = ''
  try {
    const { data: { data } } = await previewWindow(buildMatchers(form.value))
    if (version === previewVersion) preview.value = data
  } catch (e) { if (version === previewVersion) formError.value = e.message || '预览失败' }
  finally { if (version === previewVersion) previewing.value = false }
}
async function save() {
  if (saving.value || readonly.value || !can(form.value.id ? 'update' : 'create')) return
  saving.value = true; formError.value = ''
  try {
    await saveWindow(buildWindow(form.value, timezone.value)); message.success('维护窗口已保存'); editor.value = false; await load()
  } catch (e) { formError.value = e.message || '保存失败' }
  finally { saving.value = false }
}
async function inspect(id) {
  const version = ++openVersion
  try {
    const row = (await windowDetail(id)).data.data
    if (version === openVersion && can('view')) openEditor(row)
  } catch (e) { if (version === openVersion) listError.value = e.message || '详情加载失败' }
}
function confirmClose(record) {
  const active = record.status === 'active'
  Modal.confirm({ centered:true, title:active ? '提前结束维护窗口' : '取消维护窗口',
    content:`${record.title}：${active ? '结束后将重新评估被抑制的通知。' : '此窗口将不再生效。'}`,
    onOk:async () => { await (active ? endWindow : cancelWindow)(record.id); message.success('窗口状态已更新'); await load() } })
}
function confirmDelete(record) {
  openDeleteConfirm({ title:'删除维护窗口', items:[record.title],
    summary:'删除后，该窗口不再暂停通知，历史抑制记录和审计保留。',
    onConfirm:async () => {
      const ids = [record.id]
      assertBatchDeleteSucceeded(await deleteWindows(ids), ids)
      message.success('维护窗口已删除')
      if (rows.value.length === 1 && pagination.current > 1) pagination.current--
      await load()
    } })
}
watch([() => route.fullPath, () => can('view')], async ([path, allowed]) => {
  ++openVersion
  if (route.path !== '/monitor/notification/maintenance') return
  if (!allowed) { editor.value = false; return }
  load()
  if (can('create') && typeof route.query.alertname === 'string' && route.query.alertname) {
    const draft = { alertname: route.query.alertname }
    try {
      if (route.query.history_id) {
        const alert = (await getSourceAlert(route.query.history_id)).data.data
        draft.alertname = alert.alertname; draft.host_id = alert.verified_host_id
      }
      if (route.fullPath !== path) return
      openEditor(undefined, draft)
      router.replace({ path:route.path })
    } catch (e) { listError.value = e.message || '无法确认来源告警，未创建窗口' }
  }
}, { immediate:true })
</script>

<style scoped>
.scope-row { display:grid; grid-template-columns: 100px minmax(0, 1fr) auto; gap:8px; margin-bottom:8px }
.condition { display:flex; margin-bottom:8px }
.footer { margin-top:16px }
.preview-list { max-height:220px; overflow:auto }
</style>
