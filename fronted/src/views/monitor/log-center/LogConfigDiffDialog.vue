<template>
  <!-- 「查看配置差异」弹窗：回答"这次下发会把主机上的配置改成什么"。
       存在的理由：界面上只告诉用户"这台待下发"，但不给差异内容——用户没法判断该不该点下发。
       内容来自后端 /log-targets/:id/config-diff/（期望片段 = 与下发同一条渲染路径；已下发片段 =
       用 agent 读回主机上的 inputs.d），行级 diff 在前端算（见 util/configDiff）。 -->
  <a-modal
    :open="open"
    :title="`配置差异：${hostLabel}`"
    :width="1000"
    :footer="null"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-alert v-if="error" type="error" show-icon :message="error" />
      <template v-else-if="diff">
        <!-- 主机选择：差异是"某台主机的当前状态"与期望的对比，多台主机的已下发内容可能各不相同。 -->
        <div class="diff-toolbar">
          <a-select
            :value="targetId"
            :options="hostOptions"
            :get-popup-container="getPopupContainer"
            style="width: 320px"
            @update:value="(value) => emit('change-host', value)"
          />
          <a-tag :color="STATE_COLOR[diff.state] || 'default'">{{ STATE_LABEL[diff.state] || diff.state }}</a-tag>
          <a-space :size="8" wrap>
            <a-tag color="green">新增 {{ diff.summary.added }}</a-tag>
            <a-tag color="orange">修改 {{ diff.summary.changed }}</a-tag>
            <a-tag color="red">删除 {{ diff.summary.removed }}</a-tag>
            <a-tag>未变 {{ diff.summary.unchanged }}</a-tag>
            <a-tag v-if="diff.summary.unread" color="purple">读不全 {{ diff.summary.unread }}</a-tag>
          </a-space>
          <a-checkbox :checked="onlyThisService" @change="(event) => onlyThisService = event.target.checked">
            只看本服务的片段
          </a-checkbox>
          <a-button size="small" @click="reload()">重新比对</a-button>
        </div>

        <!-- 读不到主机上的配置：如实说明，并说明下面这份是"将要下发的内容"而不是差异。 -->
        <a-alert
          v-if="diff.read_error"
          type="warning"
          show-icon
          class="diff-note"
          :message="`读不到主机上已下发的配置：${diff.read_error}`"
          description="下面是**将要下发**的内容（期望侧）。读不到主机上的现状时不给差异结论——不猜。主机恢复在线后可点「重新比对」。"
        />
        <a-alert
          v-else-if="diff.state === 'synced' && !diff.summary.added && !diff.summary.removed && !diff.summary.changed"
          type="success"
          show-icon
          class="diff-note"
          message="主机上的配置与期望一致：这次下发不会改动任何片段（内容未变的主机会被自动跳过）。"
        />
        <a-alert
          v-else
          type="info"
          show-icon
          class="diff-note"
          message="下发是完全替换：这里「删除」的片段会在下发时从主机上删掉，「修改/新增」的会被写入。"
        />

        <div class="diff-meta">
          <div>主机：{{ diff.host_instance_name || '-' }}（{{ diff.host_ip || '-' }}）</div>
          <div>
            期望指纹：<code>{{ shortFingerprint(diff.expected_fingerprint) }}</code>
            ｜ 已下发指纹：<code>{{ shortFingerprint(diff.applied_fingerprint_from_db) }}</code>
            <span v-if="diff.service" class="apply-muted">
              ｜ 本服务子指纹：{{ shortFingerprint(diff.service.expected_fingerprint) }}
              vs {{ shortFingerprint(diff.service.applied_fingerprint) }}
              <a-tag v-if="diff.service.pending" color="orange">本服务待下发</a-tag>
            </span>
          </div>
        </div>

        <a-table
          :columns="columns"
          :data-source="visibleFiles"
          :pagination="false"
          row-key="path"
          size="small"
          :locale="tableLocale"
          :scroll="{ x: 900 }"
          :expanded-row-keys="expandedKeys"
          @expand="(expanded, record) => setExpanded(record, expanded)"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tooltip :title="CONFIG_DIFF_STATUS[record.status]?.hint || ''" placement="top">
                <a-tag :color="CONFIG_DIFF_STATUS[record.status]?.color || 'default'">
                  {{ CONFIG_DIFF_STATUS[record.status]?.label || record.status }}
                </a-tag>
              </a-tooltip>
              <a-tag v-if="record.read_error" color="purple">读不全</a-tag>
            </template>
            <template v-else-if="column.key === 'path'">
              <code>{{ record.base_name }}</code>
              <a-tooltip v-if="record.service_id !== serviceId" :title="`这个片段属于服务 #${record.service_id || '未知'}${record.service_code ? `（${record.service_code}）` : ''}：同主机上其他服务的配置。`" placement="top">
                <a-tag color="blue">他服务</a-tag>
              </a-tooltip>
            </template>
            <template v-else-if="column.key === 'lines'">
              <span v-if="record.status === 'unchanged'" class="apply-muted">-</span>
              <span v-else>{{ lineDelta(record) }}</span>
            </template>
            <template v-else-if="column.key === 'actions'">
              <a-button type="link" size="small" @click="toggleExpand(record)">
                {{ expandedKeys.includes(record.path) ? '收起' : '看差异' }}
              </a-button>
            </template>
          </template>
          <!-- 展开行 = 行级 diff：删除在前、新增在后（统一 diff 惯例），行号两侧各标各的。 -->
          <template #expandedRowRender="{ record }">
            <div v-if="record.read_error" class="apply-error">读取失败：{{ record.read_error }}</div>
            <template v-else>
              <a-alert v-if="diffOf(record).truncated" type="warning" show-icon class="diff-note" message="文件较大，差异未逐行对齐（整体替换）。" />
              <div class="diff-body">
                <div
                  v-for="(line, index) in diffOf(record).lines"
                  :key="index"
                  class="diff-line"
                  :class="`diff-line--${line.type}`"
                >
                  <span class="diff-gutter">{{ line.appliedLine ?? '' }}</span>
                  <span class="diff-gutter">{{ line.expectedLine ?? '' }}</span>
                  <span class="diff-sign">{{ line.type === 'add' ? '+' : (line.type === 'del' ? '-' : ' ') }}</span>
                  <span class="diff-text">{{ line.text || ' ' }}</span>
                </div>
              </div>
              <div class="field-hint">
                左列 = 主机上已下发的第 N 行，右列 = 将要下发的第 N 行；<code>-</code> 是会被删掉的（已下发侧），
                <code>+</code> 是会被写入的（期望侧）。
              </div>
            </template>
          </template>
        </a-table>

        <div class="field-hint">
          期望侧与下发用的是同一条渲染路径（同索引前缀、同宏展开、同排序），所以这里看到的就是
          agent 会写入的内容；已下发侧是**当前**从主机读回来的文件，随主机上的实际状态变化。
        </div>
      </template>
    </a-spin>
  </a-modal>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { tableLocale } from '@/util/tableStyle'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { CONFIG_DIFF_STATUS, lineDiff } from '@/util/configDiff'

const props = defineProps({
  open: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' },
  // 后端 /log-targets/:id/config-diff/ 的响应体（未加载时为 null）。
  diff: { type: Object, default: null },
  // 当前选中的采集目标 id + 可选主机列表（{label, value}），由页面给（它已经有一份承载主机清单）。
  targetId: { type: [Number, String], default: null },
  hostOptions: { type: Array, default: () => [] },
  // 本服务 id：用来标注"他服务"的片段、以及"只看本服务"的默认过滤。
  serviceId: { type: [Number, String], default: null },
  hostLabel: { type: String, default: '' },
})
const emit = defineEmits(['update:open', 'change-host', 'reload'])

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const STATE_LABEL = { synced: '已同步', drift: '待下发', never: '从未下发', unknown: '状态未知' }
const STATE_COLOR = { synced: 'green', drift: 'orange', never: 'red', unknown: 'default' }

const onlyThisService = ref(true)
const expandedKeys = ref([])

watch(() => props.open, (open) => {
  if (!open) return
  // 每次打开都重置：上一次的主机/展开态与这一个主机无关。
  onlyThisService.value = true
  expandedKeys.value = []
})

const visibleFiles = computed(() => {
  const files = props.diff?.files || []
  if (!onlyThisService.value || !props.serviceId) return files
  return files.filter((file) => String(file.service_id) === String(props.serviceId))
})

const columns = computed(() => [
  { title: '状态', key: 'status', width: 130 },
  { title: '片段（inputs.d）', key: 'path', width: 380 },
  { title: '差异行数', key: 'lines', width: 120 },
  { title: '操作', key: 'actions', width: 100, fixed: 'right' },
])

// 行级 diff 按需算并缓存：一个主机的片段可能有几十个，但用户通常只会展开一两个。
const diffCache = ref({})
function diffOf(record) {
  const cached = diffCache.value[record.path]
  if (cached && cached.forExpected === record.expected && cached.forApplied === record.applied) return cached.result
  const result = lineDiff(record.expected, record.applied)
  diffCache.value = { ...diffCache.value, [record.path]: { forExpected: record.expected, forApplied: record.applied, result } }
  return result
}

function lineDelta(record) {
  const result = diffOf(record)
  const added = result.lines.filter((line) => line.type === 'add').length
  const removed = result.lines.filter((line) => line.type === 'del').length
  if (!added && !removed) return '无内容差异'
  return `+${added} / -${removed}`
}

// 展开态由 expandedKeys 单一驱动：表格自带的展开箭头与「看差异」按钮都走这里，
// 各 toggle 一次会互相抵消（点了箭头又点按钮 = 没反应）。
function setExpanded(record, expanded) {
  if (expanded) {
    if (!expandedKeys.value.includes(record.path)) expandedKeys.value = [...expandedKeys.value, record.path]
    return
  }
  expandedKeys.value = expandedKeys.value.filter((key) => key !== record.path)
}

function toggleExpand(record) {
  setExpanded(record, !expandedKeys.value.includes(record.path))
}

function reload() {
  emit('reload')
}

function shortFingerprint(value) {
  const text = String(value || '')
  if (!text) return '（空）'
  return text.length > 12 ? `${text.slice(0, 12)}…` : text
}
</script>

<style scoped>
.diff-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.diff-note {
  margin-bottom: 8px;
}
.diff-meta {
  margin-bottom: 8px;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.65);
  word-break: break-all;
}
.diff-body {
  max-height: 360px;
  overflow: auto;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 4px;
  font-family: Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.7;
}
.diff-line {
  display: flex;
  white-space: pre;
}
.diff-gutter {
  width: 44px;
  flex: none;
  text-align: right;
  padding-right: 8px;
  color: rgba(0, 0, 0, 0.35);
  user-select: none;
}
.diff-sign {
  width: 16px;
  flex: none;
  text-align: center;
}
.diff-text {
  flex: 1;
}
.diff-line--add {
  background: #f6ffed;
  color: #237804;
}
.diff-line--del {
  background: #fff1f0;
  color: #a8071a;
}
.field-hint {
  margin-top: 8px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
