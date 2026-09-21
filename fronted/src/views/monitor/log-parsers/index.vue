<template>
  <div class="log-parser-page">
    <div class="page-title-row">
      <h2>日志处理规则</h2>
    </div>

    <div class="parser-layout">
      <div class="application-pane">
        <div class="pane-title">应用</div>
        <!-- 应用多了以后在左侧逐个找太慢：按名称或编码把不相关的条目筛掉。
             「全部规则」始终保留——它是回到全量的入口，也是筛空时的落点。 -->
        <a-input-search
          v-model:value="applicationKeyword"
          allow-clear
          size="small"
          class="application-search"
          placeholder="筛选应用 / 编码"
        />
        <a-menu
          class="application-menu"
          mode="inline"
          :selected-keys="[selectedApplicationKey]"
          @select="({ key }) => (selectedApplicationKey = key)"
        >
          <a-menu-item v-for="item in visibleApplicationGroups" :key="item.key">
            <span class="application-item">
              <span class="application-label" :title="item.label">{{ item.label }}</span>
              <a-badge
                :count="item.count"
                :number-style="{ backgroundColor: item.count ? '#1677ff' : '#bfbfbf' }"
                :show-zero="true"
              />
            </span>
          </a-menu-item>
        </a-menu>
        <div v-if="applicationFilterMissed" class="application-empty">
          没有匹配的应用，清空筛选回到全部
        </div>
      </div>

      <div class="rule-pane">
        <a-tabs v-model:active-key="catalogTab">
          <a-tab-pane key="processing" tab="解析规则">
            <div class="toolbar">
          <a-button size="large" :disabled="!selectedClusterId" @click="openCreate">
            <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
            <span>&nbsp;新增规则</span>
          </a-button>
          <a-tooltip title="刷新">
            <a-button size="large" :disabled="!selectedClusterId" @click="loadRules">
              <FontAwesomeIcon :icon="['fas', 'rotate']" />
              <span>&nbsp;刷新</span>
            </a-button>
          </a-tooltip>
        </div>

        <a-table
          size="small"
          row-key="name"
          :columns="columns"
          :data-source="visibleRules"
          :loading="loading"
          :pagination="false"
          :scroll="{ x: 1150 }"
          :locale="{ emptyText: selectedClusterId ? '该应用暂无解析规则' : '尚未配置日志存储集群，请先在「日志存储集群」页面添加' }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'name'">
              <span class="pipeline-name">{{ record.name }}</span>
            </template>
            <template v-else-if="column.key === 'description'">
              <span>{{ record.description || '-' }}</span>
            </template>
            <template v-else-if="column.key === 'input_format'">
              <a-tag :color="record.input_format === 'json' ? 'cyan' : 'default'">
                {{ record.input_format === 'json' ? 'JSON' : '文本' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'multiline'">
              <a-tag :color="record.multiline_enabled ? 'blue' : 'default'">
                {{ record.multiline_enabled ? '多行' : '单行' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'processors'">
              <a-tag color="green">{{ record.pipeline_body?.processors?.length || 0 }}</a-tag>
            </template>
            <template v-else-if="column.key === 'required_fields'">
              <!-- 必备字段缺失是静默失败：日志能查到但级别/消息列为空、关键词搜不到、错误清单失效。
                   这里直接暴露，也是切换 mapping-guard 到 drop 模式前的巡检清单。 -->
              <a-tooltip :title="requiredFieldsTooltip(record)">
                <a-tag v-if="(record.missing_required_fields || []).length" color="red">
                  缺 {{ (record.missing_required_fields || []).join('、') }}
                </a-tag>
                <a-tag v-else color="green">齐</a-tag>
              </a-tooltip>
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space :size="6">
                <a-tooltip title="编辑">
                  <a-button type="primary" @click="openEdit(record)">
                    <FontAwesomeIcon :icon="['fa', 'edit']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="运行">
                  <a-button @click="openTest(record)">
                    <FontAwesomeIcon :icon="['fas', 'play']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除">
                  <a-button class="delBtn" danger type="primary" @click="confirmDelete(record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
          </a-tab-pane>
          <a-tab-pane key="filter" tab="采集过滤规则">
            <div class="toolbar">
              <a-button size="large" @click="openFilterCreate"><FontAwesomeIcon :icon="['fas', 'plus-circle']" /><span>&nbsp;新增过滤规则</span></a-button>
              <a-tooltip title="刷新"><a-button size="large" @click="loadFilterRules"><FontAwesomeIcon :icon="['fas', 'rotate']" /><span>&nbsp;刷新</span></a-button></a-tooltip>
            </div>
            <a-table size="small" row-key="id" :columns="filterColumns" :data-source="visibleFilterRules" :loading="filterLoading" :pagination="false" :scroll="{ x: 1000 }" :locale="{ emptyText: '当前应用暂无采集过滤规则' }">
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'name'"><span class="pipeline-name">{{ record.name }}</span></template>
                <template v-else-if="column.key === 'rule_type'"><a-tag :color="record.rule_type === 'exclude' ? 'orange' : 'green'">{{ filterRuleTypeLabel(record.rule_type) }}</a-tag></template>
                <template v-else-if="column.key === 'description'"><span>{{ record.description || '-' }}</span></template>
                <template v-else-if="column.key === 'pattern'"><code>{{ record.pattern }}</code></template>
                <template v-else-if="column.key === 'enabled'"><a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag></template>
                <template v-else-if="column.key === 'action'"><a-space :size="6"><a-tooltip title="编辑"><a-button type="primary" @click="openFilterEdit(record)"><FontAwesomeIcon :icon="['fa', 'edit']" /></a-button></a-tooltip><a-tooltip title="删除"><a-button class="delBtn" danger type="primary" @click="confirmDeleteFilter(record)"><FontAwesomeIcon :icon="['fas', 'trash-can']" /></a-button></a-tooltip></a-space></template>
              </template>
            </a-table>
          </a-tab-pane>
          <a-tab-pane key="usage">
            <template #tab>
              <a-badge :count="usageCountByRule.size" :number-style="{ backgroundColor: usageCountByRule.size ? '#1677ff' : '#bfbfbf' }">关联模板</a-badge>
            </template>
            <div class="toolbar">
              <a-tooltip title="刷新"><a-button size="large" @click="loadRuleUsages"><FontAwesomeIcon :icon="['fas', 'rotate']" /><span>&nbsp;刷新</span></a-button></a-tooltip>
              <a-input-search v-model:value="usageKeyword" allow-clear style="width: 260px" placeholder="筛选模板 / 日志 / 路径" />
              <span class="usage-hint">展示每条解析规则被哪些部署模板的日志定义引用；未被引用的规则不出现在这里，可在「解析规则」表里清理。</span>
            </div>
            <a-table size="small" row-key="key" :columns="usageColumns" :data-source="visibleRuleUsages" :loading="usageLoading" :pagination="false" :scroll="{ x: 1000 }" :locale="{ emptyText: '当前应用没有规则被部署模板引用' }">
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'rule_name'"><span class="pipeline-name">{{ record.rule_name }}</span></template>
                <template v-else-if="column.key === 'application_name'">{{ applicationNameById.get(String(record.application)) || '-' }}</template>
                <template v-else-if="column.key === 'template_name'"><a-tag color="blue">{{ record.template_name }}</a-tag></template>
                <template v-else-if="column.key === 'log_definition'">
                  <span>{{ record.log_name }}</span>
                  <code class="usage-path">{{ record.path_pattern }}</code>
                </template>
                <template v-else-if="column.key === 'service_count'">
                  <a-tooltip v-if="record.service_count > 0" title="点击查看引用该模板的逻辑服务">
                    <a class="service-count-link" @click="openTemplateServices(record)">{{ record.service_count }} 个服务</a>
                  </a-tooltip>
                  <a-tooltip v-else title="引用该模板的逻辑服务数">
                    <a-badge :count="0" :number-style="{ backgroundColor: '#bfbfbf' }" />
                  </a-tooltip>
                </template>
              </template>
            </a-table>
          </a-tab-pane>
        </a-tabs>
      </div>
    </div>

    <a-modal v-model:open="templateServices.open" :title="`引用模板「${templateServices.templateName}」的服务`" :footer="null" :width="760" centered>
      <a-table size="small" row-key="id" :columns="templateServiceColumns" :data-source="templateServices.items" :loading="templateServices.loading" :pagination="false" :locale="{ emptyText: '没有逻辑服务引用该模板' }">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'service_name'">
            <a class="service-count-link" @click="gotoService(record)">{{ record.name }} <RightOutlined /></a>
          </template>
          <template v-else-if="column.key === 'enabled'"><a-badge :status="record.enabled ? 'success' : 'default'" :text="record.enabled ? '启用' : '停用'" /></template>
        </template>
      </a-table>
      <div class="usage-hint" style="margin-top: 8px;">点服务名可跳转到服务树查看详情。</div>
    </a-modal>

    <a-modal
      v-model:open="editorOpen"
      :title="editingOriginalName ? `编辑处理规则：${editingOriginalName}` : '新增日志处理规则'"
      :width="960"
      centered
      :confirm-loading="saving"
      ok-text="发布"
      cancel-text="取消"
      @ok="publishPipeline"
    >
      <a-tabs v-model:activeKey="activeTab">
        <a-tab-pane key="preprocess" tab="发送前处理（Filebeat）">
          <a-form ref="formRef" :model="form" :rules="formRules" layout="vertical">
            <a-form-item name="name" label="规则名称">
              <a-input
                v-model:value="form.name"
                :disabled="Boolean(editingOriginalName)"
                placeholder="例如 autoadmin-tomcat-access"
              />
            </a-form-item>
            <a-form-item label="说明">
              <a-input v-model:value="form.description" placeholder="描述该规则适用的日志格式" />
            </a-form-item>
            <a-form-item name="application" label="所属应用">
              <a-select
                v-model:value="form.application"
                allow-clear
                placeholder="留空表示不限应用的通用规则"
                :options="applicationOptions"
                :getPopupContainer="getPopupContainer"
              />
            </a-form-item>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item name="input_format" label="日志格式">
                  <a-segmented v-model:value="form.input_format" :options="inputFormatOptions" block />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="多行合并">
                  <a-switch v-model:checked="form.multiline_enabled" />
                </a-form-item>
              </a-col>
            </a-row>
            <template v-if="form.multiline_enabled">
              <a-row :gutter="16">
                <a-col :span="12">
                  <a-form-item name="start_pattern" label="首行正则">
                    <a-input v-model:value="form.start_pattern" placeholder="例如 ^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}" />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item name="flush_timeout" label="合并超时（毫秒）">
                    <a-input-number v-model:value="form.flush_timeout" :min="100" :max="60000" style="width: 100%" />
                  </a-form-item>
                </a-col>
              </a-row>
            </template>
          </a-form>
        </a-tab-pane>
        <a-tab-pane key="ingest" tab="字段解析（Elasticsearch Ingest）">
          <a-form layout="vertical">
            <a-form-item label="Pipeline JSON">
              <a-textarea
                v-model:value="form.bodyText"
                class="json-editor"
                :rows="18"
                spellcheck="false"
              />
            </a-form-item>
          </a-form>
        </a-tab-pane>
        <a-tab-pane key="test" tab="在线调试">
          <a-form layout="vertical">
            <a-form-item label="样例格式">
              <a-segmented v-model:value="sampleMode" :options="sampleModeOptions" />
            </a-form-item>
            <a-form-item :label="sampleMode === 'raw' ? '原始日志' : '样例文档 JSON'">
              <a-textarea
                v-model:value="sampleText"
                class="json-editor"
                :rows="12"
                :placeholder="sampleMode === 'raw' ? '直接粘贴包含换行的完整日志' : '输入包含 message 字段的 JSON 对象（Filebeat filestream）'"
                spellcheck="false"
              />
            </a-form-item>
            <a-button type="primary" :loading="simulating" @click="simulate">
              <FontAwesomeIcon :icon="['fas', 'play']" />
              <span>&nbsp;运行</span>
            </a-button>
            <a-alert
              v-if="missingFields.length"
              class="schema-violation-alert"
              type="error"
              show-icon
              :message="`缺少必备字段：${missingFields.join('、')}`"
            >
              <template #description>
                <div>处理规则产物必须包含这些字段，否则错误清单/聚类会失效。请在 Pipeline 里补对应处理器（如 <code>fingerprint.target_field=error_fingerprint</code>）后再发布。</div>
                <div>判定口径与「格式认证」完全一致：认证是拿**主机上该日志文件的尾部若干行**跑同一个判定，
                  所以这里的结论就是认证的结论。两边唯一的差别是输入——这里是你粘的样例，
                  认证是文件尾部（窗口切出来的残尾行会被丢掉，与 Filebeat 合并语义一致）。</div>
              </template>
            </a-alert>
            <a-alert
              v-if="schemaViolations.length"
              class="schema-violation-alert"
              type="error"
              show-icon
              message="存在不符合标准字段的输出"
            >
              <template #description>
                <div>以下字段不在标准字段列表内，写入 Elasticsearch 时会被丢弃（不报错但无法检索和聚合），请改写到 <code>app_fields.&lt;字段名&gt;</code> 下：</div>
                <a-space wrap class="schema-violation-tags">
                  <a-tag v-for="field in schemaViolations" :key="field" color="red">{{ field }}</a-tag>
                </a-space>
              </template>
            </a-alert>
            <div v-if="simulationText" class="result-field">
              <div class="result-header">
                <span class="result-title">运行结果</span>
                <a-space>
                  <a-tooltip title="展开日志换行">
                    <a-switch
                      v-model:checked="expandNewline"
                      checked-children="换行"
                      un-checked-children="原始"
                    />
                  </a-tooltip>
                  <a-tooltip title="复制">
                    <a-button size="small" @click="copyResult">
                      <FontAwesomeIcon :icon="['fas', 'copy']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="全屏查看">
                    <a-button size="small" @click="resultFullscreen = true">
                      <FontAwesomeIcon :icon="['fas', 'expand']" />
                    </a-button>
                  </a-tooltip>
                </a-space>
              </div>
              <pre class="json-editor result-viewer">{{ resultDisplayText }}</pre>
            </div>
          </a-form>
        </a-tab-pane>
      </a-tabs>
    </a-modal>

    <a-modal v-model:open="filterEditorOpen" :title="filterForm.id ? `编辑过滤规则：${filterForm.name}` : '新增采集过滤规则'" :confirm-loading="filterSaving" ok-text="保存" cancel-text="取消" @ok="saveFilterRule">
      <a-form ref="filterFormRef" :model="filterForm" :rules="filterFormRules" layout="vertical">
        <a-form-item name="name" label="规则名称"><a-input v-model:value="filterForm.name" placeholder="例如 error-critical-only" /></a-form-item>
        <a-form-item name="application" label="所属应用"><a-select v-model:value="filterForm.application" allow-clear placeholder="留空表示通用规则" :options="applicationOptions" :getPopupContainer="getPopupContainer" /></a-form-item>
        <a-form-item label="说明"><a-input v-model:value="filterForm.description" placeholder="例如 仅采集错误、失败和严重级别日志" /></a-form-item>
        <a-form-item name="rule_type" label="规则类型">
          <a-segmented v-model:value="filterForm.rule_type" :options="filterRuleTypeOptions" block />
          <div class="field-hint">
            保留（include）= 只采匹配的记录；排除（exclude）= 丢掉匹配的记录。两者同时配时先保留、后排除。
            方向由规则自己声明，落槽时校验一致——反着用会只采到噪声。
          </div>
        </a-form-item>
        <a-form-item name="pattern" label="匹配正则">
          <a-textarea v-model:value="filterForm.pattern" :rows="3" :placeholder="filterForm.rule_type === 'exclude' ? '例如 (?i)healthcheck|/ping|DEBUG' : '例如 (?i)(error|failed|critical|fatal)'" spellcheck="false" />
          <div class="field-hint">Go RE2 方言（与 Filebeat 同一个引擎）：不支持 \uXXXX（要写 \x{XXXX}）与断言；过滤发生在多行合并之后，<code>^</code> 只匹配整条记录的开头。</div>
        </a-form-item>
        <a-form-item label="启用"><a-switch v-model:checked="filterForm.enabled" /></a-form-item>
        <a-form-item label="试算（可选）">
          <a-textarea v-model:value="filterSample" :rows="5" placeholder="粘几行日志，下面显示这条规则会保留/丢掉哪些行——白名单写错的代价是数据永久丢失，先算一次。" spellcheck="false" />
          <div v-if="filterSample.trim()" class="filter-sample-result">
            <div class="field-hint">保留 {{ filterSampleKept.length }} 行 / 丢弃 {{ filterSampleDropped.length }} 行（共 {{ filterSampleLines.length }} 行）</div>
            <div v-if="filterSampleError" class="apply-error">{{ filterSampleError }}</div>
            <template v-else>
              <div v-if="filterSampleDropped.length" class="filter-sample-dropped">
                <div class="field-hint">会被丢弃的示例行：</div>
                <code v-for="(line, index) in filterSampleDropped.slice(0, 5)" :key="index" class="filter-sample-line">{{ line }}</code>
              </div>
              <div v-else class="field-hint">没有任何行被丢弃。</div>
            </template>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="resultFullscreen"
      title="运行结果"
      class="result-fullscreen-modal"
      :width="'96vw'"
      :footer="null"
      centered
      destroy-on-close
    >
      <div class="result-header">
        <a-space>
          <a-tooltip title="展开日志换行">
            <a-switch
              v-model:checked="expandNewline"
              checked-children="换行"
              un-checked-children="原始"
            />
          </a-tooltip>
          <a-tooltip title="复制">
            <a-button size="small" @click="copyResult">
              <FontAwesomeIcon :icon="['fas', 'copy']" />
            </a-button>
          </a-tooltip>
        </a-space>
      </div>
      <pre class="json-editor result-viewer result-viewer-full">{{ resultDisplayText }}</pre>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { RightOutlined } from '@ant-design/icons-vue'
import { useRouter } from 'vue-router'
import {
  batchDeleteLogCollectionFilterRules,
  batchDeleteLogProcessingRules,
  getLogCollectionFilterRules,
  getElasticsearchClusterList,
  getLogProcessingRuleUsages,
  getLogProcessingRules,
  saveLogCollectionFilterRule,
  saveLogProcessingRule,
  simulateElasticsearchPipeline,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { getApplicationList, getApplicationDeploymentTemplateServices } from '@/api/assets/application'
import { filterApplicationGroups, isApplicationFilterMissed } from '@/util/applicationFilter'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { assertRe2Compatible, compilePreviewRegExp } from '@/util/re2Pattern'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const selectedClusterId = ref(null)
const applications = ref([])
const selectedApplicationKey = ref('all')
const processingRules = ref([])
const ruleUsages = ref([])
const usageLoading = ref(false)
const collectionFilterRules = ref([])
const loading = ref(false)
const filterLoading = ref(false)
const saving = ref(false)
const filterSaving = ref(false)
const simulating = ref(false)
const editorOpen = ref(false)
const editingOriginalName = ref('')
const activeTab = ref('preprocess')
const catalogTab = ref('processing')
const formRef = ref(null)
const filterFormRef = ref(null)
const filterEditorOpen = ref(false)
const sampleMode = ref('raw')
const sampleText = ref('2026-08-27 13:00:00 INFO service started')
const simulationText = ref('')
const schemaViolations = ref([])
const missingFields = ref([])
const resultFullscreen = ref(false)
const expandNewline = ref(true)

// 模拟结果里的多行日志会被 JSON 序列化成 \n 转义串，展开成真实换行才能直接阅读堆栈
const resultDisplayText = computed(() => {
  if (!expandNewline.value) return simulationText.value
  return simulationText.value.replace(/\\r\\n|\\n/g, '\n').replace(/\\t/g, '  ')
})

async function copyResult() {
  try {
    await navigator.clipboard.writeText(resultDisplayText.value)
    message.success('已复制运行结果')
  } catch {
    message.error('复制失败，请手动选中复制')
  }
}
const form = reactive({
  id: null,
  name: '',
  description: '',
  application: null,
  input_format: 'text',
  multiline_enabled: false,
  start_pattern: '',
  continuation_pattern: '',
  flush_timeout: 1000,
  bodyText: '',
})
const filterForm = reactive({ id: null, name: '', application: null, description: '', pattern: '', rule_type: 'include', enabled: true })
const filterRuleTypeOptions = [
  { label: '保留（include）', value: 'include' },
  { label: '排除（exclude）', value: 'exclude' },
]
const filterRuleTypeLabel = (value) => (value === 'exclude' ? '排除' : '保留')
// 试算：把这条规则套在用户粘的样例上，直接告诉他"会丢掉哪些行"。
// 用 re2Pattern 的预览编译（RE2 → 浏览器可编译），所以方言与主机上跑的 Filebeat 一致。
const filterSample = ref('')
const filterSampleLines = computed(() => filterSample.value.split(/\r?\n/).filter((line) => line.trim() !== ''))
const filterSampleError = ref('')
const filterSampleMatched = computed(() => {
  filterSampleError.value = ''
  const pattern = String(filterForm.pattern || '').trim()
  if (!pattern) return []
  try {
    const expression = compilePreviewRegExp(pattern, '过滤正则')
    return filterSampleLines.value.filter((line) => expression.test(line))
  } catch (error) {
    filterSampleError.value = error.message
    return []
  }
})
// 保留 = 命中的留下；排除 = 命中的丢掉。两个方向对样例的作用正好相反。
const filterSampleKept = computed(() => (filterForm.rule_type === 'exclude'
  ? filterSampleLines.value.filter((line) => !filterSampleMatched.value.includes(line))
  : filterSampleMatched.value))
const filterSampleDropped = computed(() => (filterForm.rule_type === 'exclude'
  ? filterSampleMatched.value
  : filterSampleLines.value.filter((line) => !filterSampleMatched.value.includes(line))))
const applicationOptions = computed(() =>
  applications.value.map((item) => ({ label: item.name, value: item.id })),
)

// 左侧分组：数量统一由当前集群的规则列表当场统计，避免每个应用再发一次请求。
// 除已建应用外额外给出“全部”与“通用”（application 为空，不限应用的规则）两个入口。
const applicationGroups = computed(() => {
  const countByApplication = new Map()
  let genericCount = 0
  for (const rule of processingRules.value) {
    if (rule.application) {
      countByApplication.set(rule.application, (countByApplication.get(rule.application) || 0) + 1)
    } else {
      genericCount += 1
    }
  }
  return [
    { key: 'all', label: '全部规则', count: processingRules.value.length },
    ...applications.value.map((item) => ({
      key: String(item.id),
      label: item.name,
      // code 只用于筛选匹配（列表里显示 name，与应用下拉的口径一致）。
      code: item.code || '',
      count: countByApplication.get(item.id) || 0,
    })),
    { key: 'generic', label: '通用（不限应用）', count: genericCount },
  ]
})

// 左侧筛选框的输入（2026-09-19 加）：匹配逻辑抽到 util/applicationFilter.js（有单测），
// 这里只负责把"输入 + 分组"喂进去。
const applicationKeyword = ref('')
const visibleApplicationGroups = computed(() => (
  filterApplicationGroups(applicationGroups.value, applicationKeyword.value)
))
// 筛空了要说一声，否则左侧只剩一行「全部规则」，看起来像应用丢了。
const applicationFilterMissed = computed(() => (
  isApplicationFilterMissed(visibleApplicationGroups.value, applicationKeyword.value)
))

const visibleRules = computed(() => {
  if (selectedApplicationKey.value === 'all') return processingRules.value
  if (selectedApplicationKey.value === 'generic') {
    return processingRules.value.filter((rule) => !rule.application)
  }
  return processingRules.value.filter((rule) => String(rule.application) === selectedApplicationKey.value)
})
const visibleFilterRules = computed(() => {
  if (selectedApplicationKey.value === 'all') return collectionFilterRules.value
  if (selectedApplicationKey.value === 'generic') return collectionFilterRules.value.filter((rule) => !rule.application)
  return collectionFilterRules.value.filter((rule) => String(rule.application) === selectedApplicationKey.value)
})

// 关联模板 tab：一行 = (规则 × 模板日志定义)。与另两个 tab 同模式——接口一次全量，这里按左侧
// 选中的应用客户端过滤（"全部"全量、"通用"只留 application 为空的规则）。
const visibleRuleUsages = computed(() => {
  let items = ruleUsages.value
  if (selectedApplicationKey.value === 'generic') {
    items = items.filter((item) => !item.application)
  } else if (selectedApplicationKey.value !== 'all') {
    items = items.filter((item) => String(item.application) === selectedApplicationKey.value)
  }
  // 关键字筛选：匹配模板名 / 日志定义名 / 路径（大小写不敏感）。
  const keyword = usageKeyword.value.trim().toLowerCase()
  if (keyword) {
    items = items.filter((item) => (
      `${item.template_name}\n${item.log_name}\n${item.path_pattern}`.toLowerCase().includes(keyword)
    ))
  }
  return items
})
// tab 标题上的 badge：有引用关系的规则去重计数（一条规则可被多个模板引用，只算一次）。
const usageCountByRule = computed(() => new Set(ruleUsages.value.map((item) => item.rule_id)))

// 「所属应用」列：usage 行只带 application id，应用名用左侧菜单已加载的应用列表就地反查。
const applicationNameById = computed(() => {
  const byId = new Map()
  for (const item of applications.value) byId.set(String(item.id), item.name)
  return byId
})

// 「影响服务数」弹窗：点开才拉该模板的承载服务清单（懒加载，失败不阻塞表格）。
const router = useRouter()
const templateServices = reactive({ open: false, loading: false, templateName: '', items: [] })
const templateServiceColumns = [
  { title: '项目', dataIndex: 'project_name', key: 'project_name', width: 160 },
  { title: '业务系统', dataIndex: 'business_system_name', key: 'business_system_name', width: 160 },
  { title: '环境', dataIndex: 'environment_name', key: 'environment_name', width: 140 },
  { title: '服务名', key: 'service_name', width: 200 },
  { title: '状态', key: 'enabled', width: 90, align: 'center' },
]

async function openTemplateServices(record) {
  templateServices.open = true
  templateServices.loading = true
  templateServices.templateName = record.template_name
  templateServices.items = []
  try {
    const response = await getApplicationDeploymentTemplateServices(record.template_id)
    templateServices.items = response?.data?.data?.results || []
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '获取服务列表失败')
  } finally {
    templateServices.loading = false
  }
}

// 跳转到服务树并定位到该服务：服务树页读这些 query 初始化 scope（见 index.vue 的 onMounted）。
function gotoService(item) {
  templateServices.open = false
  router.push({
    path: '/assets/service-tree',
    query: {
      application_service_id: String(item.id),
      service_name: item.name,
      business_system_id: String(item.business_system_id),
      environment_id: item.environment_id == null ? '' : String(item.environment_id),
      environment_name: item.environment_name || '',
    },
  })
}
const inputFormatOptions = [
  { label: '文本', value: 'text' },
  { label: 'JSON', value: 'json' },
]
const sampleModeOptions = [
  { label: '原始日志', value: 'raw' },
  { label: '文档 JSON', value: 'json' },
]

const columns = [
  { title: '规则名称', key: 'name', width: 260, fixed: 'left' },
  { title: '说明', key: 'description', width: 430 },
  { title: '发送前格式', key: 'input_format', width: 120, align: 'center' },
  { title: '发送前行处理', key: 'multiline', width: 130, align: 'center' },
  { title: 'Ingest 处理器', key: 'processors', width: 130, align: 'center' },
  { title: '必备字段', key: 'required_fields', width: 200, align: 'center' },
  { title: '操作', key: 'action', width: 160, fixed: 'right' },
]

// 必备字段巡检提示：说清"缺了会怎样"，以及怎么补（字段必须由 pipeline 产出）。
function requiredFieldsTooltip(record) {
  const missing = record.missing_required_fields || []
  if (!missing.length) return '产出了全部必备字段（log_level / log_message / error_fingerprint）'
  return `缺少 ${missing.join('、')}：索引模板里其余字段由平台保证，这三个必须由 pipeline 产出。`
    + '缺 log_level 则级别筛选/错误清单失效；缺 log_message 则查询页消息列为空、直接搜关键词搜不到；'
    + '缺 error_fingerprint 则错误聚类失效。这类文档会被 mapping-guard 判为违规。'
}
const filterColumns = [
  { title: '规则名称', key: 'name', width: 240, fixed: 'left' },
  { title: '类型', key: 'rule_type', width: 110 },
  { title: '说明', key: 'description', width: 300 },
  { title: '匹配正则', key: 'pattern', width: 300 },
  { title: '状态', key: 'enabled', width: 100, align: 'center' },
  { title: '操作', key: 'action', width: 140, fixed: 'right' },
]

const usageColumns = [
  { title: '规则名称', key: 'rule_name', width: 220, fixed: 'left' },
  { title: '所属应用', key: 'application_name', width: 140 },
  { title: '所属模板', key: 'template_name', width: 180 },
  { title: '日志定义', key: 'log_definition', width: 340 },
  { title: '影响服务数', key: 'service_count', width: 110, align: 'center' },
]
const usageKeyword = ref('')

async function loadRuleUsages() {
  usageLoading.value = true
  try {
    const response = await getLogProcessingRuleUsages()
    const results = response?.data?.data?.results || []
    ruleUsages.value = results.map((item) => ({ ...item, key: `${item.rule_id}:${item.log_definition_id}` }))
  } catch (error) {
    ruleUsages.value = []
    message.error(error?.response?.data?.msg || error?.message || '获取关联模板失败')
  } finally {
    usageLoading.value = false
  }
}

const formRules = {
  name: [
    { required: true, message: '请输入规则名称' },
    { pattern: /^[a-z0-9][a-z0-9._-]*$/, message: '仅支持小写字母、数字、点、下划线和连字符' },
  ],
  start_pattern: [
    {
      validator: () => {
        if (!form.multiline_enabled) return Promise.resolve()
        const pattern = String(form.start_pattern || '').trim()
        if (!pattern) return Promise.reject(new Error('请输入首行正则'))
        // 首行正则由主机上的 Filebeat 执行，Filebeat 与服务端都是 Go RE2。
        // 这里先按 RE2 预检一遍：不然 Java/JS 写法（现场是 `[A-Za-z\u4e00-\u9fa5]`）能存能调试，
        // 却在主机上编译不过、采集静默失效。权威判定仍在服务端保存/认证时那次编译。
        try {
          assertRe2Compatible(pattern)
        } catch (error) {
          return Promise.reject(error)
        }
        return Promise.resolve()
      },
    },
  ],
}

function parseJson(text, label) {
  try {
    const value = JSON.parse(text)
    if (!value || Array.isArray(value) || typeof value !== 'object') {
      throw new Error(`${label}必须是 JSON 对象`)
    }
    return value
  } catch (error) {
    if (error instanceof SyntaxError) {
      throw new Error(`${label}格式错误：${error.message}`)
    }
    throw error
  }
}

// 平台只对接一套 Elasticsearch，集群不给用户选，直接取默认（或唯一启用）集群。
async function loadClusters() {
  const response = await getElasticsearchClusterList({ page: 1, page_size: 100 })
  const enabledClusters = (response?.data?.data?.results || []).filter((item) => item.enabled)
  const preferred = enabledClusters.find((item) => item.is_default) || enabledClusters[0]
  selectedClusterId.value = preferred?.id || null
  await loadRules()
}

async function loadApplications() {
  try {
    const response = await getApplicationList({ page_size: 100 })
    applications.value = response?.data?.data?.results || []
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '获取应用列表失败')
  }
}

async function loadRules() {
  if (!selectedClusterId.value) {
    processingRules.value = []
    return
  }
  loading.value = true
  try {
    const response = await getLogProcessingRules({ cluster: selectedClusterId.value, page_size: 100 })
    processingRules.value = response?.data?.data?.results || []
  } catch (error) {
    processingRules.value = []
    message.error(error?.response?.data?.msg || error?.message || '获取解析规则失败')
  } finally {
    loading.value = false
  }
}

async function loadFilterRules() {
  filterLoading.value = true
  try {
    const response = await getLogCollectionFilterRules({ page_size: 100 })
    collectionFilterRules.value = response?.data?.data?.results || []
  } catch (error) {
    collectionFilterRules.value = []
    message.error(error?.response?.data?.msg || error?.message || '获取采集过滤规则失败')
  } finally {
    filterLoading.value = false
  }
}

function resetEditor() {
  Object.assign(form, {
    id: null,
    name: '',
    description: '',
    application: null,
    input_format: 'text',
    multiline_enabled: false,
    start_pattern: '',
    continuation_pattern: '',
    flush_timeout: 1000,
    bodyText: '',
  })
  editingOriginalName.value = ''
  activeTab.value = 'preprocess'
  simulationText.value = ''
  schemaViolations.value = []
  missingFields.value = []
  sampleText.value = ''
}

function openCreate() {
  resetEditor()
  // 从左侧已选应用入口新建时直接带入归属，避免再手选一次
  const selectedApplicationId = Number(selectedApplicationKey.value)
  form.application = Number.isNaN(selectedApplicationId) ? null : selectedApplicationId
  form.bodyText = JSON.stringify({ processors: [] }, null, 2)
  editorOpen.value = true
}

function openEdit(record) {
  resetEditor()
  editingOriginalName.value = record.name
  Object.assign(form, {
    ...record,
    bodyText: JSON.stringify(record.pipeline_body || { processors: [] }, null, 2),
  })
  // 恢复该规则上次保存的调试样例，省得每次重贴
  sampleText.value = record.sample_log || ''
  editorOpen.value = true
}

function openTest(record) {
  openEdit(record)
  activeTab.value = 'test'
}

function openFilterCreate() {
  filterSample.value = ''
  Object.assign(filterForm, { id: null, name: '', application: null, description: '', pattern: '', rule_type: 'include', enabled: true })
  const selectedApplicationId = Number(selectedApplicationKey.value)
  filterForm.application = Number.isNaN(selectedApplicationId) ? null : selectedApplicationId
  filterEditorOpen.value = true
}

function openFilterEdit(record) {
  Object.assign(filterForm, record)
  // 迁移前建的规则没有类型字段，一律按 include（白名单）处理。
  if (!filterForm.rule_type) filterForm.rule_type = 'include'
  filterEditorOpen.value = true
}

const filterFormRules = {
  name: [{ required: true, message: '请输入规则名称' }],
  pattern: [
    {
      validator: () => {
        const pattern = String(filterForm.pattern || '').trim()
        if (!pattern) return Promise.reject(new Error('请输入匹配正则'))
        // 空模式在 Filebeat 里匹配全部：include 会放行一切、exclude 会丢弃一切，
        // 两个方向都是灾难，所以只填空格也要拦住（后端同样校验）。
        try {
          assertRe2Compatible(pattern, filterForm.rule_type === 'exclude' ? '排除正则' : '保留正则')
        } catch (error) {
          return Promise.reject(error)
        }
        return Promise.resolve()
      },
    },
  ],
}

async function saveFilterRule() {
  await filterFormRef.value?.validate()
  filterSaving.value = true
  try {
    await saveLogCollectionFilterRule({ ...filterForm })
    message.success('过滤规则保存成功')
    filterEditorOpen.value = false
    await loadFilterRules()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '过滤规则保存失败')
  } finally {
    filterSaving.value = false
  }
}

async function publishPipeline() {
  await formRef.value?.validate()
  let body
  try {
    body = parseJson(form.bodyText, 'Pipeline JSON')
  } catch (error) {
    message.error(error.message)
    activeTab.value = 'ingest'
    return
  }
  saving.value = true
  try {
    await saveLogProcessingRule({
      id: form.id,
      cluster: selectedClusterId.value,
      application: form.application || null,
      name: form.name,
      description: form.description,
      input_format: form.input_format,
      multiline_enabled: form.multiline_enabled,
      start_pattern: form.multiline_enabled ? form.start_pattern : '',
      continuation_pattern: form.multiline_enabled ? form.continuation_pattern : '',
      sample_log: sampleText.value || '',
      flush_timeout: form.flush_timeout,
      pipeline_body: body,
    })
    message.success('发布成功')
    editorOpen.value = false
    await loadRules()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '发布失败')
  } finally {
    saving.value = false
  }
}

// 还原 Filebeat filestream 行为：默认逐行成记录；开启多行后 `negate: true, match: after`，
// 即"不以首行正则开头的行并入上一行"，不需要单独的续行正则。
// 调试时若不做这一步，粘贴多条日志会被当成单条文档送进 pipeline，结果与真实采集不一致。
function buildRawDocs(text) {
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  if (!form.multiline_enabled) {
    return lines.filter((line) => line.trim()).map((line) => ({ message: line }))
  }
  if (!form.start_pattern.trim()) {
    throw new Error('已启用多行合并，请先在“发送前处理”填写首行正则')
  }
  let startRe
  try {
    // 走 RE2 → 浏览器可编译形式的翻译（`\x{4e00}` → `\u4e00`），否则用户按提示改成 RE2 写法后，
    // 浏览器反而抛 "Invalid hexadecimal escape sequence"，等于把人推回错误写法。
    startRe = compilePreviewRegExp(form.start_pattern)
  } catch (error) {
    throw new Error(`首行正则不合法：${error.message}`)
  }
  const docs = []
  let buffer = []
  const flush = () => {
    if (buffer.length) {
      docs.push({ message: buffer.join('\n') })
      buffer = []
    }
  }
  for (const line of lines) {
    if (startRe.test(line)) {
      flush()
      buffer = [line]
    } else if (buffer.length) {
      // 非首行一律并到上一行（Filebeat negate+after 语义）
      buffer.push(line)
    }
    // else：第一条记录之前、又不匹配首行正则的行 —— **丢掉**，与服务端认证抽样同一口径。
    // 这些行是反向读取窗口切出来的上一条记录的尾巴（堆栈续行），真实采集时 Filebeat 会把它们
    // 并进上一条记录。两边口径不一，同一段文本就会出现"调试页判定不通过、认证却通过"（或反过来），
    // 正是平台文档里点名要避免的情形。
  }
  flush()
  if (!docs.length) throw new Error('样例日志为空或未命中首行正则')
  return docs
}

async function simulate() {
  let body
  let docs
  try {
    body = parseJson(form.bodyText, 'Pipeline JSON')
    if (!sampleText.value.trim()) {
      throw new Error(sampleMode.value === 'raw' ? '请输入原始日志' : '请输入样例文档 JSON')
    }
    docs = sampleMode.value === 'raw'
      ? buildRawDocs(sampleText.value)
      : [parseJson(sampleText.value, '样例文档')]
  } catch (error) {
    message.error(error.message)
    return
  }
  simulating.value = true
  try {
    const response = await simulateElasticsearchPipeline(selectedClusterId.value, {
      pipeline: body,
      docs,
    })
    const result = response?.data?.data || {}
    schemaViolations.value = result.schema_violations || []
    missingFields.value = result.missing_fields || []
    simulationText.value = JSON.stringify(result, null, 2)
    if (missingFields.value.length) {
      // 不说"运行成功"：管道跑起来了，但**判定没通过**，而认证用的是同一个判定函数与同一套
      // 必备字段口径——把它呈成成功，就会让人以为"规则没问题、认证在挑刺"。
      message.error(`判定不通过：缺少必备字段 ${missingFields.value.join('、')}（认证会用同一口径判不通过）`)
    } else if (schemaViolations.value.length) {
      message.warning(`运行成功，但有 ${schemaViolations.value.length} 个字段不符合标准字段规范`)
    } else {
      message.success(`运行成功，共 ${docs.length} 条事件`)
    }
  } catch (error) {
    simulationText.value = ''
    schemaViolations.value = []
    missingFields.value = []
    message.error(error?.response?.data?.msg || error?.message || '运行失败')
  } finally {
    simulating.value = false
  }
}

function confirmDelete(record) {
  openDeleteConfirm({
    title: '删除解析规则',
    summary: '删除后，引用该 Pipeline 的日志将无法完成解析。',
    items: [`解析规则: ${record.name}`],
    onConfirm: async () => {
      await batchDeleteLogProcessingRules([record.id])
      message.success('删除成功')
      await loadRules()
    },
  })
}

function confirmDeleteFilter(record) {
  openDeleteConfirm({
    title: '删除过滤规则',
    summary: '删除后，逻辑服务将不能再选择该规则。',
    items: [`过滤规则: ${record.name}`],
    onConfirm: async () => {
      await batchDeleteLogCollectionFilterRules([record.id])
      message.success('删除成功')
      await loadFilterRules()
    },
  })
}

onMounted(() => {
  loadApplications()
  loadClusters()
  loadFilterRules()
})

// 关联模板数据懒加载：第一次切到该 tab 才拉取（引用关系变化低频，不必随页面加载）；
// 之后走手动刷新。
watch(catalogTab, (tab) => {
  if (tab === 'usage' && !ruleUsages.value.length && !usageLoading.value) loadRuleUsages()
})
</script>

<style scoped>
.log-parser-page {
  padding: 16px;
}
.page-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.page-title-row h2 {
  margin: 0;
  font-size: 20px;
}
.parser-layout {
  display: flex;
  align-items: flex-start;
  gap: 16px;
}
.application-pane {
  flex: 0 0 240px;
  width: 240px;
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
.application-search {
  padding: 8px 8px 0;
}
.application-menu {
  border-inline-end: none;
  /* 减去页面标题、面板标题与筛选框的高度，让列表自己滚、筛选框留在原位。 */
  max-height: calc(100vh - 288px);
  overflow: auto;
}
.application-empty {
  padding: 12px 16px;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 18px;
}
.application-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.application-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rule-pane {
  flex: 1;
  min-width: 0;
}
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.toolbar .ant-btn {
  display: inline-flex;
  align-items: center;
}
.pipeline-name {
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 13px;
}
.usage-hint {
  color: #8c95a5;
  font-size: 12px;
  line-height: 32px;
}
.usage-path {
  margin-left: 8px;
  padding: 1px 6px;
  background: #f7f8fa;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 12px;
}
.service-count-link { color: #1677ff; }
.json-editor {
  font-family: "JetBrains Mono", "Cascadia Code", monospace;
  font-size: 13px;
  line-height: 1.55;
  tab-size: 2;
}
.result-field {
  margin-top: 16px;
}
.schema-violation-alert {
  margin-top: 16px;
}
.schema-violation-tags {
  margin-top: 8px;
}
.result-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.result-title {
  color: rgba(0, 0, 0, 0.88);
}
.result-viewer {
  margin: 0;
  padding: 12px;
  background: #f7f8fa;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  max-height: 46vh;
  overflow: auto;
  white-space: pre;
  resize: vertical;
}
.result-viewer-full {
  max-height: none;
  height: calc(100vh - 220px);
}
@media (max-width: 720px) {
  .page-title-row {
    align-items: stretch;
    flex-direction: column;
  }
  .cluster-select {
    width: 100%;
  }
}
</style>
