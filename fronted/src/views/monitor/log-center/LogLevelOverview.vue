<template>
  <div class="log-level-overview">
    <!-- 层级视图：非服务节点（全部/项目/业务系统/环境）下的内容。
         存在的理由：这两个 tab 原先只认"一个具体的逻辑服务"，选中上层节点时只剩一句
         "请在左侧选择逻辑服务或部署实例"——而用户在那个层级想问的是"这一片里哪些服务有问题"。
         所以这里给的是：指标条 + 下一层的清单，点行下钻一级，到最底层（环境）的行就是服务、
         点进去就是服务级界面。

         **两个 tab 的分工（variant）**：同一批服务行，两件事——
           - 日志查询：这一片**哪个能查、有没有数据**（能不能查＝采集开没开；有没有数据＝窗口内写入量）。
             与"查日志"无关的配置欠账（未认证/未挂规则/待下发）不在这里出现；
           - 日志配置：这一片**哪些服务有配置欠账**（没认证/没挂规则/被逐条关掉/配置待下发）。
             数据视角的"最近写入"不在这里出现。
         两边的指标条与列因此不重叠——否则两个 tab 会长得几乎一样（这正是上一版被指出的问题）。 -->
    <div v-if="variant === 'config' && (pendingError || !metrics.pendingKnown)" class="level-note">
      <a-alert
        type="warning"
        show-icon
        :message="pendingError ? `配置态没算出来：${pendingError}` : '配置态暂不可用（本进程没有接入下发评估）'"
        description="「配置态」这一列会显示“-”。它表示**没算出来**，不是“都已同步”——不要据此判断下发情况。"
      />
    </div>
    <!-- 指标条按 tab 分家：配置 tab 看"配置欠账"，查询 tab 看"能不能查、有没有数据"。
         共用的只有"下辖服务数"这一项（两边都需要知道这一层有多大）。
         用自适应网格而不是 a-col 的 span：格数是随 tab 与数据变化的（例如"无写入"只在算得出时出现），
         靠 span 凑 24 会越来越别扭。 -->
    <div v-if="rows.length" class="level-metrics">
      <div class="level-metric">
        <a-statistic :title="dimension.key === 'service' ? '逻辑服务' : '下辖逻辑服务'" :value="metrics.services" />
      </div>
      <div class="level-metric">
        <a-tooltip :title="variant === 'query'
          ? '服务级采集总开关为开（含未显式配置的服务）：这些服务的日志能查到。'
          : '服务级采集总开关为开（含未显式配置的服务；逐条日志的开关另算）。'" placement="top">
          <a-statistic :title="variant === 'query' ? '可查' : '已开采集'" :value="metrics.collectionOn" />
        </a-tooltip>
      </div>
      <div class="level-metric">
        <a-tooltip :title="variant === 'query'
          ? '采集总开关关闭：这些服务查不到日志（逐条开关不生效）。要看它们先去日志配置里打开。'
          : '服务级采集总开关为关：该服务下所有日志都不采集（逐条开关不生效）。'" placement="top">
          <a-statistic title="采集关闭" :value="metrics.collectionOff" :value-style="metrics.collectionOff ? { color: '#d4380d' } : undefined" />
        </a-tooltip>
      </div>
      <!-- 查询 tab 的"有没有数据"：**只有真的取到写入量才显示这一格**（环境层）。
           算不出来就不出现——不摆一个"-/本层不统计"的占位，那只是噪声（见底部说明）。 -->
      <div v-if="variant === 'query' && metrics.recentKnown" class="level-metric">
        <a-tooltip :title="'窗口内没有任何写入的服务数：采集开了却查不到日志，多半是配置没下发、进程没起来或文件路径不匹配——点进去看「采集链路」逐层定位。'" placement="top">
          <a-statistic
            title="无写入"
            :value="metrics.recentSilentServices"
            :value-style="metrics.recentSilentServices ? { color: '#d4380d' } : undefined"
          >
            <template #suffix>
              <span class="apply-muted" style="font-size: 12px">/ {{ metrics.recentServices }} 个有写入</span>
            </template>
          </a-statistic>
        </a-tooltip>
      </div>
      <template v-if="variant === 'config'">
        <div class="level-metric">
          <a-tooltip title="未认证 = 这条日志从没验证过格式能否被解析规则解析出必备字段（没验证就采集属静默坏数据）；需复验 = 配置指纹变了要重跑一次。数字是日志定义条数。" placement="top">
            <a-statistic title="未认证日志" :value="metrics.unverified" :value-style="metrics.unverified ? { color: '#d4380d' } : undefined">
              <template v-if="metrics.needsRecheck" #suffix>
                <span class="apply-muted" style="font-size: 12px">需复验 {{ metrics.needsRecheck }}</span>
              </template>
            </a-statistic>
          </a-tooltip>
        </div>
        <div class="level-metric">
          <a-tooltip :title="pendingTooltip" placement="top">
            <a-statistic
              title="待下发"
              :value="metrics.pendingKnown ? metrics.pendingPairs : '-'"
              :value-style="metrics.pendingKnown && metrics.pendingPairs ? { color: '#fa8c16' } : undefined"
            >
              <template #suffix>
                <span class="apply-muted" style="font-size: 12px">/ {{ metrics.managedPairs }} 台·服务</span>
              </template>
            </a-statistic>
          </a-tooltip>
        </div>
      </template>
    </div>

    <a-alert v-if="variant === 'query'" type="info" show-icon class="level-note" :message="queryNotice" />

    <a-table
      v-if="rows.length"
      :columns="columns"
      :data-source="rows"
      :pagination="false"
      :loading="loading"
      row-key="key"
      size="small"
      :locale="tableLocale"
      :scroll="{ x: variant === 'query' ? 900 : 1080 }"
      :custom-row="rowProps"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'title'">
          <span :class="{ 'level-service-link': record.isService }">{{ record.title }}</span>
        </template>
        <!-- 配置 tab 上层：把"欠账"直接从指标条落到行上（哪一片欠得多一眼可见）。 -->
        <template v-else-if="column.key === 'unverified'">
          <span v-if="record.unverified" class="level-warn">
            {{ record.unverified }}<template v-if="record.needsRecheck"> / 需复验 {{ record.needsRecheck }}</template>
          </span>
          <span v-else class="apply-muted">0</span>
        </template>
        <template v-else-if="column.key === 'noRule'">
          <span v-if="record.noRule" class="level-warn">{{ record.noRule }}</span>
          <span v-else class="apply-muted">0</span>
        </template>
        <!-- 查询 tab 上层：可查 / 采集关闭。 -->
        <template v-else-if="column.key === 'collectionOn'">
          <span :class="{ 'apply-muted': !record.collectionOn }">{{ record.collectionOn }}</span>
        </template>
        <template v-else-if="column.key === 'collectionOff'">
          <span v-if="record.collectionOff" class="level-warn">{{ record.collectionOff }}</span>
          <span v-else class="apply-muted">0</span>
        </template>
        <template v-else-if="column.key === 'collection'">
          <template v-if="record.isService">
            <a-tag v-if="record.collectionOff === 1" color="red">停采</a-tag>
            <a-tag v-else color="green">采集</a-tag>
          </template>
          <span v-else-if="record.collectionOff" class="level-warn">{{ record.collectionOff }} 个关闭</span>
          <span v-else class="apply-muted">全部开启</span>
        </template>
        <template v-else-if="column.key === 'logs'">
          <span v-if="record.logs">
            {{ record.logs }} 条
            <span v-if="record.unverified || record.needsRecheck" class="level-warn">
              （未认证 {{ record.unverified }}<template v-if="record.needsRecheck"> / 需复验 {{ record.needsRecheck }}</template>）
            </span>
            <a-tooltip v-if="record.disabledLogs" :title="`其中 ${record.disabledLogs} 条被这个服务逐条关掉了采集。`" placement="top">
              <a-tag color="orange">关闭 {{ record.disabledLogs }}</a-tag>
            </a-tooltip>
            <a-tooltip v-if="record.noRule" :title="`其中 ${record.noRule} 条没挂解析规则：它们不会被采集，要到部署模板里给它们挂规则。`" placement="top">
              <a-tag color="red">未挂规则 {{ record.noRule }}</a-tag>
            </a-tooltip>
          </span>
          <span v-else class="apply-muted">无日志定义</span>
        </template>
        <template v-else-if="column.key === 'tier'">
          <span v-if="!record.isService" class="apply-muted">-</span>
          <span v-else>{{ tierLabel(record.retentionTierId) }}</span>
        </template>
        <template v-else-if="column.key === 'pending'">
          <span v-if="!record.pendingKnown" class="apply-muted">-</span>
          <span v-else-if="record.pendingPairs" class="level-warn">
            待下发 {{ record.pendingPairs }}
            <span class="apply-muted">/ {{ record.managedPairs }} 台·服务</span>
          </span>
          <span v-else class="apply-muted">已同步 {{ record.managedPairs }} 台·服务</span>
        </template>
        <template v-else-if="column.key === 'recent'">
          <span v-if="!record.recentDocsKnown" class="apply-muted">-</span>
          <span v-else-if="record.recentDocs">{{ Number(record.recentDocs).toLocaleString() }} 条</span>
          <a-tooltip v-else title="窗口内没有写入：服务可能停用/停采、配置没下发、或最近确实没有日志。" placement="top">
            <a-tag color="orange">无写入</a-tag>
          </a-tooltip>
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-button type="link" size="small" @click.stop="emit('drilldown', record.scope)">
            {{ record.isService ? (variant === 'query' ? '查询日志' : '日志配置') : '下钻' }}
          </a-button>
        </template>
      </template>
    </a-table>

    <a-empty
      v-else
      :image="simpleImage"
      :description="`这个范围里还没有逻辑服务（${variant === 'query' ? '日志要按服务查' : '日志配置按服务生效'}，所以这里没有可列的内容）`"
    />
    <div v-if="rows.length" class="field-hint">
      <template v-if="variant === 'query'">
        这一层是**找服务**的下钻导航：点行进入下一层，到"环境"层的行就是逻辑服务、点进去查它的日志。
        日志检索按逻辑服务切分（索引里带服务段），所以这一层只帮你定位服务，不在这里直接搜。
        <template v-if="!metrics.recentKnown">要看写入量请下钻到环境层（写入量按 业务系统 × 环境 聚合，这一层不给这个数）。</template>
      </template>
      <template v-else>
        这一层是**配置欠账**的下钻导航：点行进入下一层，到"环境"层的行就是逻辑服务、点进去改它的日志配置。
        写操作仍然以**逻辑服务**为最小单位（层级上只读），当前范围共
        {{ metrics.services }} 个服务、{{ metrics.managedPairs }} 个 (服务 × 主机) 待下发。
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Empty } from 'ant-design-vue'
import { tableLocale } from '@/util/tableStyle'

const props = defineProps({
  // config = 日志配置 tab（列里给档位/待下发），query = 日志查询 tab（列里给最近写入）。
  variant: { type: String, default: 'config' },
  // levelDimension() 的返回值：{ key, label }。
  dimension: { type: Object, required: true },
  rows: { type: Array, default: () => [] },
  metrics: { type: Object, default: () => ({}) },
  // 服务行的档位下拉候选（id → 名称），由调用方给（页面本来就有这份数据）。
  retentionTiers: { type: Array, default: () => [] },
  // 最近写入的窗口说明（服务行的"最近写入"列标题用它，如"最近 30 天写入"）。
  recentDocsTitle: { type: String, default: '最近写入' },
  // 配置态没算出来时的原因（后端 pending_error）；空串但也有未算出的行时用通用文案。
  pendingError: { type: String, default: '' },
  loading: { type: Boolean, default: false },
})
const emit = defineEmits(['drilldown'])

const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE

const pendingTooltip = computed(() => '待下发的单位是 (服务 × 主机)：同一台主机承载两个服务时各算一次——'
  + '这才是"下发这件事"的粒度。数字 = 配置变更过但没下发 + 从未下发；未纳管的主机单独算（它们下发不到）。'
  + `当前范围 ${props.metrics.unmanagedPairs || 0} 个 (服务 × 主机) 落在未纳管主机上。`)

const queryNotice = '日志检索按逻辑服务切分（索引里带服务段），所以要先选到具体服务；'
  + '点下面某个服务的「查询日志」即进入它的检索面板。'

// 列按 tab 分家。两边只保留各自要回答的问题所需的那几列，**不共用什么"日志（条）"列**——
// 共用的列越多，两个 tab 就越像（上一版把"日志定义/未认证"这类配置欠账带进了查询 tab，
// 又把"最近写入"带进了配置 tab，结果两边看起来只是换了一列）。
const columns = computed(() => {
  const titleColumn = {
    title: props.dimension.key === 'service' ? '逻辑服务' : props.dimension.label,
    dataIndex: 'title', key: 'title', width: 200,
  }
  const actions = { title: '操作', key: 'actions', width: 110, fixed: 'right' }
  const leaf = props.dimension.key === 'service'

  // 查询 tab：能不能查 + 有没有数据。**没有**任何配置欠账列（未认证/未挂规则/待下发）。
  if (props.variant === 'query') {
    if (leaf) {
      return [
        titleColumn,
        { title: '采集', key: 'collection', width: 110 },
        { title: props.recentDocsTitle, key: 'recent', width: 220 },
        actions,
      ]
    }
    return [
      titleColumn,
      { title: '下辖服务', dataIndex: 'serviceCount', key: 'serviceCount', width: 110 },
      { title: '可查', dataIndex: 'collectionOn', key: 'collectionOn', width: 100 },
      { title: '采集关闭', key: 'collectionOff', width: 120 },
      actions,
    ]
  }

  // 配置 tab：配置欠账（未认证 / 未挂规则 / 逐条关闭 / 待下发 / 档位）。**没有**写入量列。
  if (leaf) {
    return [
      titleColumn,
      { title: '采集', key: 'collection', width: 100 },
      { title: '日志定义', key: 'logs', width: 360 },
      { title: '默认保留档位', key: 'tier', width: 150 },
      { title: '配置态', key: 'pending', width: 170 },
      actions,
    ]
  }
  return [
    titleColumn,
    { title: '下辖服务', dataIndex: 'serviceCount', key: 'serviceCount', width: 110 },
    { title: '采集关闭', key: 'collectionOff', width: 120 },
    { title: '未认证日志', dataIndex: 'unverified', key: 'unverified', width: 120 },
    { title: '未挂规则', dataIndex: 'noRule', key: 'noRule', width: 110 },
    { title: '配置态', key: 'pending', width: 170 },
    actions,
  ]
})

function tierLabel(tierId) {
  if (tierId === null || tierId === undefined) return '继承服务默认'
  const tier = props.retentionTiers.find((item) => item.id === tierId)
  return tier ? tier.name : `档位 #${tierId}`
}

// 整行可点：层级视图的主要动作就是"下钻"，只让操作列可点会让人在表格里找按钮。
function rowProps(record) {
  return {
    style: { cursor: 'pointer' },
    onClick: () => emit('drilldown', record.scope),
  }
}
</script>

<style scoped>
.level-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px 12px;
  margin-bottom: 12px;
}
.level-metric {
  padding: 6px 10px;
  background: #fafafa;
  border-radius: 6px;
}
.level-note {
  margin-bottom: 8px;
}
.level-warn {
  color: #d4380d;
}
.level-service-link {
  font-weight: 500;
  color: #1677ff;
}
.field-hint {
  margin-top: 8px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
