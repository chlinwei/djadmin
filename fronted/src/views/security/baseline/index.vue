<template>
  <div class="baseline-page">
    <a-tabs v-model:activeKey="activeTab" class="workspace-tabs" @change="handleTabChange">
      <a-tab-pane key="baselines" tab="基线标准">
        <div class="baseline-layout">
          <div class="baseline-pane">
            <div class="pane-title">基线标准</div>
            <a-menu
              class="baseline-menu"
              mode="inline"
              v-model:open-keys="openKeys"
              :selected-keys="[selectedCategory ? String(selectedCategory.id) : '']"
              @click="({ key }) => selectCategoryById(key)"
              @open-change="handleBaselineOpenChange"
            >
              <a-sub-menu v-for="item in baselines" :key="String(item.id)">
                <template #title>
                  <span class="baseline-item">
                    <span class="baseline-label" :title="item.description || item.name">{{ item.name }}</span>
                    <a-badge
                      :count="item.item_count"
                      :number-style="{ backgroundColor: item.item_count ? '#1677ff' : '#bfbfbf' }"
                      :show-zero="true"
                    />
                  </span>
                </template>
                <template v-if="item.id === selectedBaseline?.id">
                  <a-menu-item v-for="category in categories" :key="String(category.id)">
                    <span class="baseline-label" :title="category.name">{{ category.name }}</span>
                  </a-menu-item>
                  <a-menu-item v-if="!categories.length" key="__empty" disabled>暂无类目</a-menu-item>
                </template>
              </a-sub-menu>
            </a-menu>
            <div class="pane-actions">
              <a-space>
                <a-tooltip title="新增基线">
                  <a-button v-permission="'baseline:manage'" size="small" type="primary" @click="openBaselineModal()">
                    <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="编辑基线">
                  <a-button v-permission="'baseline:manage'" size="small" :disabled="!selectedBaseline" @click="openBaselineModal(selectedBaseline)">
                    <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除基线">
                  <a-button v-permission="'baseline:manage'" class="delBtn" size="small" type="primary" danger :disabled="!selectedBaseline" @click="confirmDeleteBaseline(selectedBaseline)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="发起扫描">
                  <a-button v-permission="'baseline:scan'" size="small" type="primary" ghost :disabled="!selectedBaseline" @click="openScanModal(selectedBaseline)">
                    <FontAwesomeIcon :icon="['fas', 'satellite-dish']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </div>
          </div>

          <div class="items-pane">
            <div class="toolbar">
              <span class="items-title">{{ selectedCategory ? `策略 - ${selectedCategory.name}（${categoryRows.length}）` : (selectedBaseline ? '策略（选择左侧类目）' : '策略') }}</span>
              <a-space>
                <a-button v-permission="'baseline:manage'" size="large" :disabled="!selectedBaseline" @click="openCategoryModal()">
                  <FontAwesomeIcon :icon="['fas', 'folder-plus']" />&nbsp;新增类目
                </a-button>
                <a-button v-permission="'baseline:manage'" size="large" :disabled="!selectedCategory" @click="openCategoryModal(selectedCategory)">
                  <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />&nbsp;重命名
                </a-button>
                <a-button v-permission="'baseline:manage'" size="large" :disabled="!selectedCategory" @click="moveCategory('up')">
                  <FontAwesomeIcon :icon="['fas', 'arrow-up']" />&nbsp;上移
                </a-button>
                <a-button v-permission="'baseline:manage'" size="large" :disabled="!selectedCategory" @click="moveCategory('down')">
                  <FontAwesomeIcon :icon="['fas', 'arrow-down']" />&nbsp;下移
                </a-button>
                <a-popconfirm title="删除该类目？（类目下有策略时会被拒绝）" @confirm="confirmDeleteCategory">
                  <a-button v-permission="'baseline:manage'" class="delBtn" size="large" type="primary" danger :disabled="!selectedCategory">
                    <FontAwesomeIcon :icon="['fas', 'folder-minus']" />&nbsp;删除类目
                  </a-button>
                </a-popconfirm>
                <a-button v-permission="'baseline:manage'" size="large" type="primary" :disabled="!selectedCategory" @click="openItemEditModal()">
                  <FontAwesomeIcon :icon="['fas', 'plus-circle']" />&nbsp;添加策略
                </a-button>
                <a-tooltip title="刷新">
                  <a-button size="large" :disabled="!selectedBaseline" @click="selectBaseline(selectedBaseline)">
                    <FontAwesomeIcon :icon="['fas', 'rotate']" />&nbsp;刷新
                  </a-button>
                </a-tooltip>
              </a-space>
            </div>
            <a-table
              row-key="sourceIndex"
              :columns="itemColumns"
              :data-source="categoryRows"
              :loading="itemsLoading"
              :pagination="false"
              size="small"
              :locale="{ emptyText: selectedCategory ? '该类目暂无策略，点击右上角「添加策略」' : (selectedBaseline ? '左侧选择一个类目查看策略' : '左侧选择一个基线标准') }"
            >
              <template #bodyCell="{ column, record, index }">
                <template v-if="column.key === 'severity'">
                  <a-tag :color="record.severity === 'high' ? 'red' : record.severity === 'medium' ? 'orange' : 'default'">{{ severityLabel(record.severity) }}</a-tag>
                </template>
                <template v-else-if="column.key === 'action'">
                  <a-space :size="6">
                    <a-tooltip title="编辑">
                      <a-button v-permission="'baseline:manage'" size="small" type="primary" ghost @click="openItemEditModal(record.sourceIndex)">编辑</a-button>
                    </a-tooltip>
                    <a-tooltip title="删除">
                      <a-popconfirm title="确认删除该策略？" @confirm="removeItemRow(record.sourceIndex)">
                        <a-button v-permission="'baseline:manage'" size="small" type="primary" danger>删除</a-button>
                      </a-popconfirm>
                    </a-tooltip>
                  </a-space>
                </template>
              </template>
            </a-table>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="scans" tab="扫描记录">
        <div class="toolbar">
          <a-button size="large" @click="loadScans">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="scanColumns"
          :data-source="scans"
          :loading="scanLoading"
          :pagination="false"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</a-tag>
            </template>
            <template v-else-if="column.key === 'summary'">
              <span v-if="record.summary?.total">{{ record.summary.success }} 成功 / {{ record.summary.failed }} 失败 / {{ record.summary.skipped ?? 0 }} 跳过</span>
              <span v-else>-</span>
            </template>
            <template v-else-if="column.key === 'action'">
              <a-button size="small" @click="openScanDetail(record)">详情</a-button>            </template>
          </template>
        </a-table>
      </a-tab-pane>
    </a-tabs>

    <a-modal v-model:open="baselineModalOpen" :title="baselineForm.id ? '编辑基线' : '新增基线'" centered @ok="submitBaseline">
      <a-form layout="vertical">
        <a-form-item label="基线名称" required><a-input v-model:value="baselineForm.name" placeholder="如 等保2.0 主机安全基线" /></a-form-item>
        <div class="form-grid">
          <a-form-item label="版本"><a-input v-model:value="baselineForm.version" placeholder="v1" /></a-form-item>
          <a-form-item label="状态">
            <a-switch v-model:checked="baselineForm.enabled" checked-children="启用" un-checked-children="停用" />
          </a-form-item>
        </div>
        <a-form-item label="描述"><a-textarea v-model:value="baselineForm.description" :rows="2" /></a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:open="itemEditModalOpen" :title="itemEditIndex === null ? '添加策略' : `编辑策略 - ${itemEditForm.name}`" width="80vw" style="max-width: 1200px" centered :mask-closable="false" :confirm-loading="itemsSaving" @ok="confirmItemEdit">
      <a-form layout="vertical">
        <div class="form-grid">
          <a-form-item label="条目名称" required class="mount-item"><a-input v-model:value="itemEditForm.name" placeholder="如 密码最小长度" /></a-form-item>
          <a-form-item label="所属类目" class="mount-item"><a-input :value="selectedCategory?.name" disabled /></a-form-item>
        </div>
        <div class="form-grid">
          <a-form-item label="严重级别" class="mount-item">
            <a-select v-model:value="itemEditForm.severity" :options="[{label:'高',value:'high'},{label:'中',value:'medium'},{label:'低',value:'low'}]" />
          </a-form-item>
          <a-form-item label="运行用户" class="mount-item">
            <a-input v-model:value="itemEditForm.run_user" placeholder="root" />
            <div class="field-hint">采集命令以此用户执行（su -l 登录环境，留空默认 root）；支持 <code>{{ '{' }}HOST_IP{{ '}' }}</code> / <code>{{ '{' }}HOST_NAME{{ '}' }}</code> 变量（在命令/路径中生效）。</div>
          </a-form-item>
        </div>
        <a-form-item label="说明" class="mount-item"><a-input v-model:value="itemEditForm.description" /></a-form-item>
        <a-form-item label="修复建议" class="mount-item">
          <a-textarea v-model:value="itemEditForm.remediation" :rows="2" placeholder="不符合时建议的处理方式，如：编辑 /etc/ssh/sshd_config 设置 PermitEmptyPasswords no 并重启 sshd" />
        </a-form-item>
        <a-alert type="info" show-icon class="variable-hint">
          <template #message>可用变量（主机级）</template>
          <template #description>
            <div class="variable-list">
              <div class="variable-item"><code>{{ '{' }}HOST_IP{{ '}' }}</code><span>目标主机 IP</span></div>
              <div class="variable-item"><code>{{ '{' }}HOST_NAME{{ '}' }}</code><span>目标主机名</span></div>
            </div>
            <div class="variable-note">变量仅对采集命令（exec）/文件路径（path）生效；Rego 策略是代码，不做变量展开。</div>
          </template>
        </a-alert>
        <div class="check-heading">
          <span>采集命令（{{ itemEditForm.input_commands.length }}）</span>
          <a-button size="small" @click="itemEditForm.input_commands.push({ key: '', exec: '', parse: 'raw' })">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;添加采集命令
          </a-button>
        </div>
        <div v-for="(input, inputIndex) in itemEditForm.input_commands" :key="inputIndex" class="input-grid">
          <a-input v-model:value="input.key" placeholder="key，如 es_limits" />
          <a-input v-model:value="input.exec" placeholder="命令，如 cat /proc/1/limits" />
          <a-select v-model:value="input.parse" :options="opaParseOptions" />
          <a-tooltip title="删除" placement="top">
            <a-button class="delBtn" size="small" type="primary" danger @click="itemEditForm.input_commands.splice(inputIndex, 1)">
              <FontAwesomeIcon :icon="['fas', 'trash-can']" />
            </a-button>
          </a-tooltip>
        </div>
        <div class="check-heading">
          <span>文件采集（{{ itemEditForm.input_files.length }}）</span>
          <a-button size="small" @click="itemEditForm.input_files.push({ key: '', path: '', parse: 'lines' })">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;添加文件采集
          </a-button>
        </div>
        <div v-for="(input, inputIndex) in itemEditForm.input_files" :key="'f' + inputIndex" class="input-grid">
          <a-input v-model:value="input.key" placeholder="key，如 sshd_config" />
          <a-input v-model:value="input.path" placeholder="路径，如 /etc/ssh/sshd_config" />
          <a-select v-model:value="input.parse" :options="opaParseOptions" />
          <a-tooltip title="删除" placement="top">
            <a-button class="delBtn" size="small" type="primary" danger @click="itemEditForm.input_files.splice(inputIndex, 1)">
              <FontAwesomeIcon :icon="['fas', 'trash-can']" />
            </a-button>
          </a-tooltip>
        </div>
        <div class="field-hint" style="margin-bottom:8px;">每条命令的输出成为 input 的一个顶层字段，策略里用 <code>input.{{ '{' }}key{{ '}' }}</code> 引用；命令失败时整个条目直接报错（附错误原因）。</div>
        <a-form-item label="Rego 策略" required>
          <a-textarea v-model:value="itemEditForm.policy" :rows="10" spellcheck="false" class="opa-policy-editor" />
          <div class="field-hint">
            必须以 <code>package baseline</code> 开头，产出 <code>assertions contains &lt;元素&gt; if {{ '{' }} ... {{ '}' }}</code> 全量断言清单（OPA v1 语法；pass/fail 都展示，空集即通过）。
            元素形如 {'{'}name, pass, expected, actual{'}'}，expected/actual 进报告对应列。
            策略里引用的 <code>input.key</code> 必须与上方采集 key 一致，引用了未采集的字段保存时会直接报错。
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:open="categoryModalOpen" :title="categoryForm.id ? '重命名类目' : '新增类目'" centered @ok="submitCategory">
      <a-form layout="vertical">
        <a-form-item label="类目名称" required>
          <a-input v-model:value="categoryForm.name" placeholder="如 密码策略 / 用户类目 / SSH 类目" @pressEnter="submitCategory" />
          <div class="field-hint">类目是策略的分组（二层菜单的第二层），同一基线内建议按检查对象命名。</div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:open="scanModalOpen" title="发起基线扫描" centered :confirm-loading="scanning" @ok="submitScan">
      <a-form layout="vertical">
        <a-form-item label="基线"><a-input :value="scanBaseline?.name" readonly /></a-form-item>
        <a-form-item label="项目" required>
          <a-select v-model:value="scanForm.project_id" :options="projectOptions" show-search option-filter-prop="label" placeholder="选择项目" />
        </a-form-item>
        <a-form-item label="环境（可选）">
          <a-select v-model:value="scanForm.environment_id" :options="environmentOptions" allow-clear placeholder="全部环境" />
        </a-form-item>
      </a-form>
    </a-modal>

  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import requestUtil from '@/util/request'

const router = useRouter()

const baselinePrefix = 'sys/security/baseline/'
const getBaselines = (params) => requestUtil.get(baselinePrefix, params)
const saveBaseline = (data) => requestUtil.post(baselinePrefix, data)
const updateBaseline = (id, data) => requestUtil.patch(`${baselinePrefix}${id}/`, data)
const deleteBaseline = (id) => requestUtil.del(`${baselinePrefix}${id}/`)
const getBaselineDetail = (id) => requestUtil.get(`${baselinePrefix}${id}/`)
const createBaselineCategory = (id, data) => requestUtil.post(`${baselinePrefix}${id}/categories/`, data)
const updateBaselineCategory = (baselineId, categoryId, data) => requestUtil.patch(`${baselinePrefix}${baselineId}/categories/${categoryId}/`, data)
const deleteBaselineCategory = (baselineId, categoryId) => requestUtil.del(`${baselinePrefix}${baselineId}/categories/${categoryId}/`)
const startBaselineScan = (id, data) => requestUtil.post(`${baselinePrefix}${id}/scan/`, data)
const getSecurityScans = (params) => requestUtil.get('sys/security/scans/', params)
const getProjectListSimple = (params) => requestUtil.get('assets/projects/', params)
const getEnvironmentListSimple = (params) => requestUtil.get('assets/business-environments/', params)

const activeTab = ref('baselines')
const baselines = ref([])
const baselineLoading = ref(false)
const scanColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 70 },
  { title: '基线', dataIndex: 'baseline', key: 'baseline', width: 220 },
  { title: '状态', key: 'status', width: 100 },
  { title: '结果', key: 'summary', width: 240 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 110 },
  { title: '开始时间', dataIndex: 'start_time', key: 'start_time', width: 180 },
  { title: '操作', key: 'action', width: 90 },
]
const itemColumns = [
  { title: '#', key: 'index', width: 50, customRender: ({ index }) => index + 1 },
  { title: '策略名称', dataIndex: 'name' },
  { title: '级别', key: 'severity', width: 70 },
  { title: '说明', dataIndex: 'description', key: 'description', ellipsis: true, width: 180 },
  { title: '操作', key: 'action', width: 130 },
]

const scans = ref([])
const scanLoading = ref(false)
const baselineModalOpen = ref(false)
const selectedBaseline = ref(null)
const selectedCategoryId = ref(null)
const categories = ref([])
const selectedCategory = computed(() => categories.value.find((category) => category.id === selectedCategoryId.value) || null)
const openKeys = ref([])
const itemsLoading = ref(false)
const itemForm = ref([])
const categoryModalOpen = ref(false)
const categoryForm = reactive({ id: null, name: '' })
const scanModalOpen = ref(false)
const scanning = ref(false)
const scanBaseline = ref(null)
const scanForm = reactive({ project_id: undefined, environment_id: undefined })
const projectOptions = ref([])
const environmentOptions = ref([])

const statusLabel = (status) => ({ pending: '等待中', running: '扫描中', success: '完成', failed: '存在不符合', skipped: '已跳过' }[status] || status)
const statusColor = (status) => ({ pending: 'default', running: 'processing', success: 'green', failed: 'red', skipped: 'default' }[status] || 'default')
const severityLabel = (severity) => ({ high: '高', medium: '中', low: '低' }[severity] || severity)
const opaParseOptions = [
  { label: 'raw 原文', value: 'raw' },
  { label: 'lines 按行', value: 'lines' },
  { label: 'json 对象', value: 'json' },
]
const emptyOpaConfig = () => ({ input_commands: [], input_files: [], policy: '', run_user: '', remediation: '' })
const responseData = (response) => response?.data?.data || {}

async function loadBaselines() {
  baselineLoading.value = true
  try {
    const data = responseData(await getBaselines({ search: '' }))
    const previousSelectedId = selectedBaseline.value?.id
    baselines.value = data.results || []
    if (previousSelectedId && !baselines.value.some((item) => item.id === previousSelectedId)) {
      selectedBaseline.value = null
      categories.value = []
      selectedCategoryId.value = null
      itemForm.value = []
    }
  } finally { baselineLoading.value = false }
}
async function loadScans() {
  scanLoading.value = true
  try {
    const data = responseData(await getSecurityScans({ type: 'baseline' }))
    scans.value = data.results || []
  } finally { scanLoading.value = false }
}
function handleTabChange(key) {
  if (key === 'scans') loadScans()
}
function openBaselineModal(record) {
  baselineForm.id = record?.id || null
  baselineForm.name = record?.name || ''
  baselineForm.version = record?.version || 'v1'
  baselineForm.description = record?.description || ''
  baselineForm.enabled = record?.enabled ?? true
  baselineModalOpen.value = true
}
const baselineForm = reactive({ id: null, name: '', version: 'v1', description: '', enabled: true })

async function submitBaseline() {
  if (!baselineForm.name.trim()) {
    message.warning('请填写基线名称')
    return
  }
  try {
    const payload = { ...baselineForm }
    if (baselineForm.id) await updateBaseline(baselineForm.id, payload)
    else await saveBaseline(payload)
    baselineModalOpen.value = false
    message.success('基线已保存')
    await loadBaselines()
  } catch (error) {
    message.error(error?.message || '基线保存失败')
  }
}
async function confirmDeleteBaseline(record) {
  if (!record) return
  try {
    await deleteBaseline(record.id)
    if (selectedBaseline.value?.id === record.id) {
      selectedBaseline.value = null
      categories.value = []
      selectedCategoryId.value = null
      itemForm.value = []
    }
    message.success('基线已删除')
    await loadBaselines()
  } catch (error) {
    message.error(error?.message || '删除失败')
  }
}
// ---- 左侧二层菜单（基线 → 类目）→ 右侧策略表格；增删改先改本地 itemForm，「保存策略」一次性提交。

// 展开基线节点即加载它的类目（基线选择挂在展开事件上，而不是点击类目）。
const lastOpenedKeys = ref([])
function handleBaselineOpenChange(keys) {
  const opened = keys.find((key) => !lastOpenedKeys.value.includes(key))
  lastOpenedKeys.value = [...keys]
  const record = baselines.value.find((item) => String(item.id) === String(opened))
  if (record) selectBaseline(record)
}

function selectCategoryById(key) {
  const record = categories.value.find((category) => String(category.id) === String(key))
  if (record) selectedCategoryId.value = record.id
}

function selectBaseline(record) {
  if (!record) return
  selectedBaseline.value = record
  selectedCategoryId.value = null
  itemsLoading.value = true
  itemForm.value = []
  categories.value = []
  const baselineKey = String(record.id)
  if (!openKeys.value.includes(baselineKey)) openKeys.value = [...openKeys.value, baselineKey]
  getBaselineDetail(record.id)
    .then((response) => {
      const detail = responseData(response)
      categories.value = detail.categories || []
      selectedCategoryId.value = categories.value[0]?.id ?? null
      itemForm.value = (detail.items || []).map((item) => ({
        id: item.id, name: item.name, category_id: item.category_id, description: item.description,
        severity: item.severity,
        input_commands: item.config?.input_commands || [],
        input_files: item.config?.input_files || [],
        policy: item.config?.policy || '',
        run_user: item.config?.run_user || '',
        remediation: item.config?.remediation || '',
      }))
    })
    .finally(() => { itemsLoading.value = false })
}

// 右侧表格只展示当前选中类目的策略；保留 itemForm 原始下标供编辑/删除定位。
const categoryRows = computed(() => itemForm.value
  .map((item, index) => ({ ...item, sourceIndex: index }))
  .filter((item) => item.category_id === selectedCategoryId.value))
const itemEditModalOpen = ref(false)
const itemEditIndex = ref(null)
const itemsSaving = ref(false)
const itemEditForm = reactive({ name: '', description: '', severity: 'high', input_commands: [], input_files: [], policy: '', run_user: '', remediation: '' })

// ---- 策略单条操作：弹窗确认 / 删除确认即调后端，立即生效（无批量保存）。 ----
const addBaselineItem = (baselineId, data) => requestUtil.post(`${baselinePrefix}${baselineId}/items/`, data)
const updateBaselineItem = (baselineId, itemId, data) => requestUtil.patch(`${baselinePrefix}${baselineId}/items/${itemId}/`, data)
const deleteBaselineItem = (baselineId, itemId) => requestUtil.del(`${baselinePrefix}${baselineId}/items/${itemId}/`)

function openItemEditModal(index = null) {
  itemEditIndex.value = index
  Object.assign(itemEditForm, {
    name: '', description: '', severity: 'high', run_user: '', remediation: '',
    ...emptyOpaConfig(),
  })
  if (index !== null) Object.assign(itemEditForm, itemForm.value[index])
  itemEditModalOpen.value = true
}

async function confirmItemEdit() {
  if (!itemEditForm.name.trim()) {
    message.warning('策略名称不能为空')
    return
  }
  if (!itemEditForm.input_commands.length && !itemEditForm.input_files.length) {
    message.warning('OPA 策略至少要有一条采集')
    return
  }
  if (!(itemEditForm.policy || '').trim()) {
    message.warning('OPA 策略必须填写 Rego 策略')
    return
  }
  if (!(itemEditForm.policy || '').includes('package baseline') || !(itemEditForm.policy || '').includes('assertions contains')) {
    message.warning('策略必须以 package baseline 开头且包含 assertions contains 规则')
    return
  }
  const editing = itemEditIndex.value !== null
  const categoryId = editing ? itemForm.value[itemEditIndex.value].category_id : selectedCategoryId.value
  const payload = {
    category_id: categoryId,
    name: itemEditForm.name.trim(),
    description: itemEditForm.description || '',
    severity: itemEditForm.severity,
    config: {
      input_commands: itemEditForm.input_commands,
      input_files: itemEditForm.input_files,
      policy: itemEditForm.policy,
      run_user: (itemEditForm.run_user || '').trim(),
      remediation: itemEditForm.remediation || '',
    },
  }
  itemsSaving.value = true
  try {
    if (editing) await updateBaselineItem(selectedBaseline.value.id, itemForm.value[itemEditIndex.value].id, payload)
    else await addBaselineItem(selectedBaseline.value.id, payload)
    itemEditModalOpen.value = false
    message.success(editing ? '策略已保存' : '策略已添加')
    await selectBaseline(selectedBaseline.value)
  } catch (error) {
    message.error(error?.message || '策略保存失败')
  } finally { itemsSaving.value = false }
}

async function removeItemRow(index) {
  const item = itemForm.value[index]
  if (!item?.id) return
  try {
    await deleteBaselineItem(selectedBaseline.value.id, item.id)
    message.success('策略已删除')
    await selectBaseline(selectedBaseline.value)
  } catch (error) {
    message.error(error?.message || '策略删除失败')
  }
}

// ---- 类目管理：新增 / 重命名 / 上下移 / 删除（非空由后端拒绝）。

function openCategoryModal(record) {
  categoryForm.id = record?.id || null
  categoryForm.name = record?.name || ''
  categoryModalOpen.value = true
}

async function submitCategory() {
  const name = categoryForm.name.trim()
  if (!name) {
    message.warning('请填写类目名称')
    return
  }
  try {
    if (categoryForm.id) {
      await updateBaselineCategory(selectedBaseline.value.id, categoryForm.id, { name })
      const target = categories.value.find((category) => category.id === categoryForm.id)
      if (target) target.name = name
      message.success('类目已重命名')
    } else {
      const data = responseData(await createBaselineCategory(selectedBaseline.value.id, { name }))
      categories.value = [...categories.value, { id: data.id, name: data.name, sort: data.sort }]
      message.success('类目已创建')
    }
    categoryModalOpen.value = false
  } catch (error) {
    message.error(error?.message || '类目保存失败')
  }
}

async function moveCategory(direction) {
  if (!selectedCategory.value) return
  try {
    await updateBaselineCategory(selectedBaseline.value.id, selectedCategory.value.id, { direction })
    const index = categories.value.findIndex((category) => category.id === selectedCategory.value.id)
    const neighbor = direction === 'up' ? index - 1 : index + 1
    if (neighbor < 0 || neighbor >= categories.value.length) {
      message.warning('类目已在边界，无法移动')
      return
    }
    const next = [...categories.value]
    ;[next[index], next[neighbor]] = [next[neighbor], next[index]]
    categories.value = next
  } catch (error) {
    message.error(error?.message || '类目移动失败')
  }
}

async function confirmDeleteCategory() {
  if (!selectedCategory.value) return
  try {
    await deleteBaselineCategory(selectedBaseline.value.id, selectedCategory.value.id)
    message.success('类目已删除')
    await selectBaseline(selectedBaseline.value)
  } catch (error) {
    message.error(error?.message || '类目删除失败')
  }
}

// 发起扫描：按项目（×环境）解析主机集合，逐主机执行基线套件。
async function submitScan() {
  if (!scanForm.project_id) {
    message.warning('请选择项目')
    return
  }
  scanning.value = true
  try {
    const data = responseData(await startBaselineScan(scanBaseline.value.id, {
      mount_type: scanForm.environment_id ? 'environment' : 'project',
      project_id: scanForm.project_id,
      environment_id: scanForm.environment_id ?? null,
    }))
    scanModalOpen.value = false
    message.success(`扫描已发起（${data.targets} 台主机）`)
    activeTab.value = 'scans'
    await loadScans()
  } catch (error) {
    message.error(error?.message || '扫描发起失败')
   } finally { scanning.value = false }
}
function openScanModal(record) {
  scanBaseline.value = record
  scanForm.project_id = undefined
  scanForm.environment_id = undefined
  scanModalOpen.value = true
}
function openScanDetail(record) {
  router.push(`/sys/security/baseline/scans/${record.id}`)
}
onMounted(() => {
  loadBaselines()
  getProjectListSimple({ page: 1, page_size: 200 }).then((response) => {
    const data = responseData(response)
    projectOptions.value = (data.results || []).map((item) => ({ label: item.name, value: item.id }))
  })
  getEnvironmentListSimple({ page: 1, page_size: 100 }).then((response) => {
    const data = responseData(response)
    environmentOptions.value = (data.results || []).map((item) => ({ label: item.name, value: item.id }))
  })
})

defineOptions({ name: 'ViewSecurityBaseline' })
</script>

<style scoped>
/* 条目编辑器：与巡检组检查项编辑器保持一致的布局语义（fronted/src/views/inspection/index.vue）。 */
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.input-grid { display: grid; grid-template-columns: minmax(150px, 200px) 1fr minmax(110px, 150px) auto; gap: 8px; align-items: center; margin-bottom: 8px; }
.check-heading { display: flex; align-items: center; justify-content: space-between; margin: 4px 0 12px; font-weight: 600; }
.field-hint { color: #66727d; font-size: 12px; }
.variable-hint { margin-bottom: 12px; }
.variable-item { display: flex; gap: 8px; align-items: baseline; }
.variable-item code { color: #126e82; }
.variable-note { margin-top: 8px; color: #66727d; font-size: 12px; }

.opa-policy-editor {
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 13px;
  line-height: 1.55;
  tab-size: 2;
}

/* 基线标准 tab：左基线菜单 / 右条目表格的主从布局（对齐「日志处理规则」页排版）。 */
.baseline-layout {
  display: flex;
  align-items: stretch;
  gap: 16px;
}

.baseline-pane {
  flex: 0 0 260px;
  width: 260px;
  display: flex;
  flex-direction: column;
  background: #fff;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  overflow: hidden;
}

.pane-title {
  padding: 10px 16px;
  font-weight: 600;
  border-bottom: 1px solid #f0f0f0;
}

.baseline-menu {
  border-inline-end: none;
  flex: 1;
  max-height: calc(100vh - 300px);
  overflow: auto;
}

.baseline-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.baseline-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pane-actions {
  padding: 10px 16px;
  border-top: 1px solid #f0f0f0;
  background: #fafafa;
}

.items-pane {
  flex: 1;
  min-width: 0;
}

.items-title {
  flex: 1;
  font-weight: 600;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}

.toolbar .ant-btn {
  display: inline-flex;
  align-items: center;
}
</style>
