<template>
  <div class="log-center">
    <!-- 日志中心：把「日志查询」「日志存储水位」「逻辑服务的日志配置」三块合成一个入口——
         左侧是所有资产页共用的服务树（groupByProject 让它显示项目层级，这是组件里为该类页面预留的选项），
         右侧按 tab 分开展示。选中的节点决定右上的一切上下文，所以三块不再需要各自找服务。
         tab 顺序按使用频率排：日志查询最常用，放第一个并作为默认 tab。 -->
    <ServiceTree
      :selected-scope="scope"
      :show-stats="false"
      group-by-project
      @select="scope = $event"
    />
    <main class="log-center-content">
      <header class="log-center-header">
        <div class="log-center-title">
          {{ scope.nodeTitle || '全部业务' }}
          <a-tag v-if="scope.businessSystemName" color="blue">{{ scope.businessSystemName }}</a-tag>
          <a-tag v-if="scope.environmentName">{{ scope.environmentName }}</a-tag>
          <a-tag v-if="scope.nodeType === 'deployment'" color="green">实例：{{ scope.nodeTitle }}</a-tag>
        </div>
      </header>

      <a-tabs v-model:active-key="activeTab">
        <!-- 日志查询最常用，放第一个 tab，也是默认打开的那个。 -->
        <a-tab-pane key="query" tab="日志查询">
          <LogQueryPanel v-if="serviceId" :scope="scope" />
          <a-empty v-else :image="simpleImage" description="请在左侧选择逻辑服务或部署实例" />
        </a-tab-pane>

        <a-tab-pane key="config" tab="日志配置" force-render>
          <a-empty v-if="!serviceId" :image="simpleImage" description="请在左侧选择逻辑服务或部署实例" />
          <template v-else>
            <a-alert v-if="configError" type="error" show-icon :message="configError" />
            <!-- 采集链路：查不到日志时按层回答"断在哪"。判定全部来自后端（与链路体检同源），
                 前端只呈现，不自己算阈值——两处各算一套必然出现"体检说没事、这里说有事"。 -->
            <div class="chain-bar">
              <div class="chain-head">
                <span class="chain-title">采集链路</span>
                <a-tooltip title="向承载主机各查一次 Filebeat 状态（systemctl status）后重新拉取链路。状态未知（新纳管、刚下发重启过）时会自动查，不必手动点；这个按钮用来强制再查一次。" placement="top">
                  <a-button size="small" :loading="chainRefreshing" @click="refreshChainRuntime()">
                    <ReloadOutlined />
                    <span>&nbsp;刷新运行态</span>
                  </a-button>
                </a-tooltip>
                <a-button size="small" type="text" :loading="chainLoading" @click="loadChain()">重新检查</a-button>
                <!-- 自动刷新要看得见，否则按钮自己转圈会让人以为"谁在动我的机器"。 -->
                <span v-if="chainAutoRefreshing" class="apply-muted">
                  <a-spin size="small" />
                  &nbsp;状态未知，自动查一次运行态…
                </span>
                <span v-if="chainError" class="apply-error">{{ chainError }}</span>
                <span v-else-if="chainCheckedAt" class="apply-muted">本次检查：{{ chainCheckedAt }}</span>
              </div>
              <div v-if="chainLayers.length" class="chain-layers">
                <a-tooltip v-for="layer in chainLayers" :key="layer.key" placement="top">
                  <template #title>
                    <!-- 一行一台主机：断点在哪台机器上一眼可见（tooltip 用多行渲染，不靠 \n 换行）。 -->
                    <div v-for="(line, index) in chainLayerTooltipLines(layer)" :key="index">{{ line }}</div>
                  </template>
                  <span class="chain-layer" :class="`chain-${layer.status}`">
                    <a-badge :status="CHAIN_BADGE[layer.status] || 'default'" />
                    {{ layer.name }}：{{ layer.summary }}
                  </span>
                </a-tooltip>
              </div>
              <!-- 服务停用 / 服务级采集总开关关掉是"查不到日志"最常见的原因，而且不在上面五层里。 -->
              <a-alert
                v-if="chainServiceStopReason"
                type="warning"
                show-icon
                class="apply-unmanaged"
                :message="chainServiceStopReason"
              />
            </div>
            <!-- 本服务下发区：聚合展示承载主机的配置态，一键对它们全量重下发。
                 语义是"对承载本服务的主机重新下发"而不是"下发给服务"——agent 侧 apply 会
                 以本次交付的文件集为全量、删掉未交付的 .yml，所以最小下发单位只能是主机。 -->
            <div class="apply-bar">
              <div class="apply-left">
                <!-- 服务级采集总开关：与逻辑服务编辑弹窗同一个字段、同一句提示。
                     它与下面每一行的采集开关是两层：总开关关闭时任何日志都不采集，
                     逐条开关只是配置意图（保留下次打开总开关时的选择）。 -->
                <a-tooltip :title="`关闭时该服务下所有实例均不采集；逐条日志的开关随之不生效（配置仍保留，重新打开即恢复）。${pendingHint}`" placement="top">
                  <span class="apply-switch">
                    <a-switch
                      :checked="serviceCollectEnabled === true"
                      :loading="configLoading || savingServiceCollect"
                      :disabled="serviceCollectEnabled === null"
                      checked-children="采集"
                      un-checked-children="停采"
                      @change="toggleServiceCollect"
                    />
                    <span class="apply-switch-label">服务采集总开关</span>
                  </span>
                </a-tooltip>
                <span class="apply-sep">|</span>
                <div class="apply-summary">
                  <a-spin v-if="applyStateLoading" size="small" />
                  <template v-else>
                    <span v-if="applySummaryText">{{ applySummaryText }}</span>
                    <span v-else-if="applyStateError" class="apply-error">{{ applyStateError }}</span>
                    <span v-else class="apply-muted">该服务还没有绑定任何部署实例，没有可下发的主机</span>
                  </template>
                </div>
              </div>
              <a-tooltip :title="applyButtonTooltip" placement="top">
                <a-button
                  type="primary"
                  size="small"
                  :loading="applying"
                  :disabled="!applyManagedCount"
                  @click="applyService"
                >
                  下发本服务的采集配置
                </a-button>
              </a-tooltip>
            </div>
            <a-alert
              v-if="serviceCollectEnabled === false"
              type="warning"
              show-icon
              class="apply-unmanaged"
              message="服务采集总开关已关闭：该服务下所有日志都不会被采集，下面逐条的采集开关不生效。"
            />
            <a-alert v-if="applyUnmanagedText" type="warning" show-icon class="apply-unmanaged" :message="applyUnmanagedText" />
            <div v-if="applyJob" class="apply-progress">
              <a-progress
                :percent="applyJobPercent"
                size="small"
                :status="applyJob.is_running ? 'active' : (applyJob.failed_count ? 'exception' : 'success')"
              />
              <div class="field-hint">
                下发作业 #{{ applyJob.id }}：共 {{ applyJob.total_count }} 台，成功 {{ applyJob.success_count }}，
                失败 {{ applyJob.failed_count }}，待处理 {{ applyJob.pending_count }}。
                <span v-if="!applyJob.is_running">{{ applyJob.message || '作业已结束' }}（逐台明细见「日志采集」页）</span>
              </div>
            </div>
            <a-table
              :columns="configColumns"
              :data-source="logRows"
              :loading="configLoading"
              :pagination="false"
              row-key="log_definition"
              size="small"
              :locale="tableLocale"
              :scroll="{ x: 2000 }"
            >
              <template #headerCell="{ column }">
                <template v-if="column.key === 'resolved_path'">
                  路径
                  <a-tooltip :title="showResolvedPath ? '当前显示：解析后的路径（模板默认值 + 服务覆盖）。切回原始可看模板里的路径模式' : '当前显示：模板里的路径模式（原始 ${宏}）。切到解析后可看能展开的部分'" placement="top">
                    <a-switch
                      v-model:checked="showResolvedPath"
                      size="small"
                      checked-children="解析后"
                      un-checked-children="原始"
                      class="path-toggle"
                    />
                  </a-tooltip>
                </template>
                <template v-else>{{ column.title }}</template>
              </template>
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'resolved_path'">
                  <a-tooltip :title="PATH_MACRO_HINT" placement="top">
                    <code>{{ pathCellValue(record, showResolvedPath) }}</code>
                    <!-- 解析后仍留着的宏是**实例级**的（服务这一层拿不到），显式标出来、不猜值。 -->
                    <a-tag
                      v-if="showResolvedPath && unexpandedMacros(record.resolved_path).length"
                      color="orange"
                      class="path-macro-tag"
                    >
                      {{ unexpandedMacros(record.resolved_path).join(' ') }} 实例上展开
                    </a-tag>
                  </a-tooltip>
                  <!-- 含通配的路径按需展开成主机上的真实文件清单（逐台调 agent，不进首屏）。 -->
                  <LogGlobPreview
                    v-if="showResolvedPath && !unexpandedMacros(record.resolved_path).length && hasGlobMeta(record.resolved_path)"
                    :service-id="serviceId"
                    :log-definition-id="record.log_definition"
                  />
                </template>
                <template v-else-if="column.key === 'carrying_hosts'">
                  <a-tooltip v-if="carryingHostsSummary" :title="carryingHostsTooltip" placement="top">
                    <span>{{ carryingHostsSummary }}</span>
                  </a-tooltip>
                  <span v-else class="apply-muted">未绑定部署实例</span>
                </template>
                <template v-else-if="column.key === 'processing_rule'">
                  <span :class="{ 'log-rule-missing': !record.template_processing_rule_id }">
                    {{ record.template_processing_rule_name || '未配置（不会采集）' }}
                  </span>
                </template>
                <template v-else-if="column.key === 'collection_enabled'">
                  <!-- 默认采：只有"关"才落库（覆盖值 false），显式 true 与无覆盖等价——
                       与逻辑服务编辑弹窗同一口径。改动只影响这一条日志，不影响同服务其他日志。 -->
                  <a-tooltip :title="`关闭表示本服务不再采集这条日志（同一模板下其他服务不受影响）。${pendingHint}`" placement="top">
                    <a-switch
                      :checked="isLogCollected(record)"
                      :loading="Boolean(savingRows[record.log_definition])"
                      checked-children="采"
                      un-checked-children="不采"
                      @change="(checked) => confirmLogCollectChange(record, checked)"
                    />
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'format_state'">
                  <a-tooltip :title="formatStateTooltip(record)" placement="top">
                    <a-tag :color="FORMAT_STATE_COLOR[record.format_state] || 'default'">
                      {{ FORMAT_STATE_LABEL[record.format_state] || '未验证' }}
                    </a-tag>
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'format_action'">
                  <a-tooltip :title="formatActionTooltip(record, serviceId)" placement="top">
                    <a-button
                      type="link"
                      size="small"
                      :disabled="!canVerifyLogFormat(record, serviceId)"
                      @click="openVerifyDialog(record)"
                    >
                      {{ record.format_state === 'verified' ? '重新认证' : '发起认证' }}
                    </a-button>
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'retention_tier'">
                  <!-- 档位决定写入哪个 data stream：改档位会写新流，旧流按原档位保留到期、不迁移数据。 -->
                  <a-tooltip :title="`改档位会写入新流（旧流停止写入、按原档位保留到期，不迁移数据）。${pendingHint}`" placement="top">
                    <a-select
                      :value="record.retention_tier ?? null"
                      :options="[{ label: '继承服务默认', value: null }, ...retentionTierOptions]"
                      :loading="Boolean(savingRows[record.log_definition])"
                      :get-popup-container="getPopupContainer"
                      size="small"
                      style="min-width: 140px"
                      @update:value="(value) => confirmTierChange(record, value)"
                    />
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'filter_include'">
                  <!-- 三态：null 继承模板 / 0 不过滤 / >0 指定规则。选项按方向过滤，
                       白名单不会出现在排除槽里（反着用会只采到噪声）。 -->
                  <a-tooltip
                    :title="`只保留匹配的记录（白名单）。${pendingHint} 被滤掉的日志不会进 ES、也补不回来。`"
                    placement="top"
                  >
                    <a-select
                      :value="record.collection_filter_rule_id ?? null"
                      :options="[{ label: inheritedFilterLabel(record.template_filter_include_rule_id), value: null }, ...filterRuleOptions('include')]"
                      :loading="Boolean(savingRows[record.log_definition])"
                      :get-popup-container="getPopupContainer"
                      size="small"
                      style="min-width: 160px"
                      @update:value="(value) => confirmFilterChange(record, 'include', value)"
                    />
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'filter_exclude'">
                  <a-tooltip
                    :title="`丢掉匹配的记录（黑名单，先保留后排除）。${pendingHint} 被滤掉的日志不会进 ES、也补不回来。`"
                    placement="top"
                  >
                    <a-select
                      :value="record.collection_exclude_filter_rule_id ?? null"
                      :options="[{ label: inheritedFilterLabel(record.template_filter_exclude_rule_id), value: null }, ...filterRuleOptions('exclude')]"
                      :loading="Boolean(savingRows[record.log_definition])"
                      :get-popup-container="getPopupContainer"
                      size="small"
                      style="min-width: 160px"
                      @update:value="(value) => confirmFilterChange(record, 'exclude', value)"
                    />
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'data_stream'"><code>{{ record.data_stream }}</code></template>
              </template>
            </a-table>
            <a-alert
              type="info"
              show-icon
              class="apply-unmanaged"
              message="这一页怎么保存、怎么生效"
            >
              <template #description>
                <div>① <b>每处改动都先确认一次，确认后即时入库</b>：采集总开关、逐条采集开关、保留档位、采集过滤都走同一套
                  （确认框里写清这一处的后果）；这一页没有"保存"按钮，改完页头的"待下发"计数会立刻变。</div>
                <div>② <b>入库 ≠ 生效</b>：还要点页头「<b>下发本服务的采集配置</b>」才写到主机上（采集侧的事，绕不过）。
                  隔壁逻辑服务编辑弹窗是另一套节奏——那边是整表单一起提交（改完点"保存"）。</div>
                <div>③ <b>只读的两列来自模板</b>：日志名称、路径、处理规则都跟着部署模板走，要改去「部署模板 → 路径与文件 → 日志」；
                  格式认证是**动作**不是配置，走每行的「发起认证」弹窗。</div>
              </template>
            </a-alert>
            <div class="field-hint">
              这里的路径与解析规则来自部署模板的日志定义（同一模板下所有服务共用同一份规则）；
              采集开关、保留档位、格式认证按 (服务 × 日志定义) 单独配置，改完立即生效于配置（**需要
              重新下发采集配置才在主机上生效**，下发入口在「日志采集」页）。每次改动只作用于这一条日志，
              不会影响同服务其他日志的覆盖值。
              <br />
              采集过滤（保留/排除）随时可改，但**改完必须重新下发采集配置才在主机上生效**；
              被过滤掉的记录在采集侧就丢掉了，ES 里查不到、也补不回来——白名单（保留）写窄之前先用
              「日志处理规则 → 采集过滤规则」里的试算确认一遍。
              <br />
              「所在主机」是该服务的承载主机（主机实例名 + IP）：路径里的宏在每台主机上各自展开，
              所以同一份日志定义在这些机器上落到各自的绝对路径；标注「未纳管」的主机还没纳管日志采集，
              配置下发不到它们（详见「日志采集」页）。
            </div>
          </template>
        </a-tab-pane>

        <a-tab-pane key="storage" :tab="serviceId ? '本服务水位' : '存储水位'">
          <template v-if="applyStateError && !storageRows.length && !storageLoading"></template>
          <!-- 顶部：集群 + 数据时间。水位是"某个集群在某个时刻的快照"，不给这两项就没法解读。 -->
          <div class="apply-bar">
            <div class="apply-summary">
              <span v-if="storageCluster">集群：<code>{{ storageCluster.index_prefix }}</code></span>
              <span v-if="storageGeneratedAt" class="apply-muted">数据时间：{{ storageGeneratedAt }}</span>
            </div>
            <a-button size="small" :loading="storageLoading" @click="loadStorage()">
              <ReloadOutlined />
              <span>&nbsp;刷新水位</span>
            </a-button>
          </div>
          <a-alert v-if="storageError" type="error" show-icon :message="storageError" />

          <!-- 统计：全局口径（不随树选中而变），所以未选服务时也有内容可看。 -->
          <a-row v-if="storageSummary" :gutter="8" style="margin-bottom: 12px">
            <a-col :span="6"><a-statistic title="data stream 数" :value="storageSummary.streams" /></a-col>
            <a-col :span="6"><a-statistic title="总文档数" :value="storageSummary.docs" /></a-col>
            <a-col :span="6">
              <a-statistic title="总占用" :value="formatBytes(storageSummary.bytes)">
                <template #suffix>
                  <span class="apply-muted" style="font-size: 12px">
                    (活跃 {{ formatBytes(storageSummary.activeBytes) }}
                    <template v-if="storageSummary.historicalStreams">
                      / 历史 {{ formatBytes(storageSummary.historicalBytes) }}
                    </template>)
                  </span>
                </template>
              </a-statistic>
            </a-col>
            <a-col :span="6">
              <a-statistic title="健康异常流" :value="storageSummary.unhealthy">
                <template #suffix>
                  <a-tooltip title="health 不是 green 的 data stream（副本未分配/节点离线等），与容量无关但会影响可用性。">
                    <span class="apply-muted" style="font-size: 12px">非 green</span>
                  </a-tooltip>
                </template>
              </a-statistic>
            </a-col>
          </a-row>

          <!-- 节点磁盘水位：集群级、与选没选服务无关。盘满了是"还能不能再写"的问题，
               比任何单条流的信息都紧急，所以放在流明细之前。 -->
          <template v-if="storageAllocation.length || storageAllocError">
            <div class="storage-section-title">Elasticsearch 节点磁盘</div>
            <a-table
              size="small"
              row-key="node"
              :columns="allocationColumns"
              :data-source="storageAllocation"
              :pagination="false"
              :locale="tableLocale"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'disk.used'">{{ formatBytes(record['disk.used']) }}</template>
                <template v-else-if="column.key === 'disk.total'">{{ formatBytes(record['disk.total']) }}</template>
                <template v-else-if="column.key === 'disk.percent'">
                  <a-tag :color="Number(record['disk.percent']) >= 85 ? 'red' : (Number(record['disk.percent']) >= 70 ? 'orange' : 'green')">
                    {{ record['disk.percent'] }}%
                  </a-tag>
                </template>
              </template>
            </a-table>
            <div v-if="storageAllocError" class="apply-error field-hint">磁盘水位获取失败：{{ storageAllocError }}</div>
            <div class="field-hint" style="margin-bottom: 12px">分片数只统计本平台前缀（<code>{{ storageCluster?.index_prefix }}</code>）下的索引。</div>
          </template>

          <!-- 服务级：状态里的"历史档位"判定依赖"本服务当前生效档位"，所以只有选中服务时才有。 -->
          <!-- 容量视角：按当前层级的下一层聚合占用（全部→项目、项目→业务系统…），
               降序 + 占比，便于回答"哪个项目/业务系统最占盘"。 -->
          <template v-if="storageGroups.length">
            <div class="storage-section-title">按{{ groupDimension.label }}统计（占用降序）</div>
            <a-row :gutter="12">
              <a-col :span="storagePieOption ? 14 : 24">
                <a-table
              size="small"
              row-key="name"
              :columns="groupColumns"
              :data-source="storageGroups"
              :pagination="false"
              :locale="tableLocale"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'name'">
                  {{ record.name }}
                  <a-tooltip v-if="!record.streams" title="这一层已配置，但没有任何 data stream（可能是还没建服务、日志定义被删，或维度已停用）。">
                    <a-tag>无日志</a-tag>
                  </a-tooltip>
                  <a-tooltip v-else-if="record.extra" title="有 data stream 用到了这个编码，但它不在启用的维度表里（维度被停用或删除后的遗留流）。">
                    <a-tag color="orange">维度已停用</a-tag>
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'docs'">{{ Number(record.docs || 0).toLocaleString() }}</template>
                <template v-else-if="column.key === 'bytes'">{{ formatBytes(record.bytes) }}</template>
                <template v-else-if="column.key === 'historicalBytes'">
                  <span v-if="record.historicalBytes">
                    {{ formatBytes(record.historicalBytes) }}（{{ record.historicalStreams }} 条）
                  </span>
                  <span v-else class="apply-muted">-</span>
                </template>
                <template v-else-if="column.key === 'share'">
                  <a-progress :percent="record.share" size="small" style="max-width: 150px" />
                </template>
                </template>
              </a-table>
              </a-col>
              <a-col v-if="storagePieOption" :span="10">
                <StorageUsagePie :option="storagePieOption" />
              </a-col>
            </a-row>
          </template>

          <div v-if="serviceId" class="storage-section-title">本服务的 data stream</div>
          <div v-else class="storage-section-title">
            全部 data stream（左侧选择逻辑服务可只看该服务；选项目/业务系统/环境可只看该范围）
          </div>
          <a-alert
            v-if="!serviceId && unrecognizedStreams.length"
            type="warning"
            show-icon
            class="apply-unmanaged"
            :message="`有 ${unrecognizedStreams.length} 条 data stream 无法按 <前缀>-<项目>-<业务系统>-<环境>-<逻辑服务>-<档位> 解析（可能为手工创建或维度已删除），它们不归属任何服务。`"
          />
          <a-table
            :columns="storageColumns"
            :data-source="visibleStorageRows"
            :loading="storageLoading"
            :pagination="false"
            row-key="name"
            size="small"
            :locale="tableLocale"
            :scroll="{ x: 1200 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'owner'">
                <a-tag v-if="record.recognized" color="blue">{{ record.service }}</a-tag>
                <a-tag v-else color="orange">未识别</a-tag>
              </template>
              <template v-else-if="column.key === 'bytes'">{{ formatBytes(record.bytes) }}</template>
              <template v-else-if="column.key === 'docs'">{{ Number(record.docs || 0).toLocaleString() }}</template>
              <template v-else-if="column.key === 'ilm_state'">
                <a-tag v-if="record.ilm_state" color="blue">{{ record.ilm_state }}</a-tag>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.key === 'collect_state'">
                <template v-if="isHistoricalTier(record)">
                  <a-tooltip title="改档位留下的历史流：已停止写入，存量数据按原档位保留到期后由 ILM 删除，不迁移、也不会自动清理。" placement="top">
                    <a-tag color="default">历史档位（已停写）</a-tag>
                  </a-tooltip>
                </template>
                <a-tooltip v-else-if="serviceCollectState(record)" :title="collectStateTooltip()" placement="top">
                  <a-tag color="orange">{{ serviceCollectState(record) }}</a-tag>
                </a-tooltip>
                <a-tag v-else-if="record.recognized" color="green">采集中</a-tag>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.key === 'backing'">
                <a-button type="link" size="small" @click="toggleBackingIndices(record)">
                  {{ (record.backing_indices || []).length }} 个{{ expandedStream?.name === record.name ? '（收起）' : '' }}
                </a-button>
              </template>
              <!-- 操作列（2026-09-19）：**每条流都能清**，不再只给历史档位流。
                   历史档位流多一个「切回该档位」（回收前还能救回来）。 -->
              <template v-else-if="column.key === 'actions'">
                <!-- 「切回该档位」要改"这个服务的哪条日志定义"，只有选中服务时才有上下文；
                     全量视图下只给清理（清理按服务维度执行，服务 id 由 dims 映射得到，不需要选中）。 -->
                <template v-if="isHistoricalTier(record) && serviceId">
                  <a-tooltip title="把这条日志的档位改回该档位，写入会继续进入这条流（不会新建）；改完需要重新下发才在主机上生效。" placement="top">
                    <a-button type="link" size="small" @click="openSwitchTier(record)">切回该档位</a-button>
                  </a-tooltip>
                </template>
                <a-tooltip :title="streamCleanupTooltip(record)" placement="top">
                  <a-button
                    v-if="streamCleanupTarget(record)"
                    type="link"
                    size="small"
                    danger
                    @click="cleanupStream(record)"
                  >
                    {{ isHistoricalTier(record) ? '立即清理' : '清理数据' }}
                  </a-button>
                </a-tooltip>
                <span v-if="!streamCleanupTarget(record) && !(isHistoricalTier(record) && serviceId)" class="apply-muted">-</span>
              </template>
            </template>
          </a-table>

          <!-- 清理弹窗：与「日志查询」原来的清理入口合并成一个（见 LogCleanupDialog）。 -->
      <LogCleanupDialog v-model:open="cleanupOpen" :scope="cleanupScope" @cleaned="loadStorage" />

      <!-- 后备索引：真实磁盘占用的最细粒度（一个流由多个 backing index 组成）。 -->
          <template v-if="expandedStream">
            <a-divider style="margin: 12px 0" />
            <a-alert
              type="info"
              show-icon
              :message="`data stream ${expandedStream.name} 的后备索引（真实磁盘占用的最细粒度）`"
              style="margin-bottom: 8px"
            />
            <a-table
              size="small"
              row-key="index"
              :columns="backingColumns"
              :data-source="expandedStream.backing_indices || []"
              :pagination="false"
              :locale="tableLocale"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'bytes'">{{ formatBytes(record.bytes) }}</template>
                <template v-else-if="column.key === 'docs'">{{ Number(record.docs || 0).toLocaleString() }}</template>
                <template v-else-if="column.key === 'ilm_state'">
                  <a-tag v-if="record.ilm_state" color="blue">{{ record.ilm_state }}</a-tag>
                  <span v-else>-</span>
                </template>
              </template>
            </a-table>
          </template>

          <div class="field-hint">
            只含按新命名（<code>前缀-项目-业务系统-环境-服务-档位</code>）能识别到服务归属的流；
            未识别流标在上方提示里（它们不归属任何服务，也不受本页的按服务收窄影响）。
            <b>改档位会留下历史流</b>：旧流停止写入、按原档位保留到期（不迁移数据），所以这里可能同时
            出现当前档位与历史档位的流——历史流标为「历史档位（已停写）」，它占的磁盘仍算在本服务头上。
            水位是流级真实磁盘占用，查询对集群有成本，因此只在切到本 tab 或手动刷新时取一次。
          </div>
        </a-tab-pane>
      </a-tabs>
      <!-- 切回历史档位：要改的是"哪条日志的档位"，而一条流可能对应多条日志（同档位共享一条流），
           所以让用户勾选，逐条按行提交（每条一次请求，各自独立）。 -->
      <a-modal
        :open="switchTierOpen"
        :title="`切回档位：${switchTierTarget?.tier || ''}`"
        :width="520"
        :confirm-loading="switchTierSaving"
        ok-text="切换"
        cancel-text="取消"
        @ok="submitSwitchTier"
        @cancel="switchTierOpen = false"
      >
        <div class="field-hint">
          这条流按 <b>{{ switchTierTarget?.tier }}</b> 档位保留数据。切回该档位后，写入会继续进入这条流，
          不会新建一条空流；改完需要重新下发采集配置才在主机上生效。
        </div>
        <a-checkbox-group v-model:value="switchTierSelected" class="switch-tier-list">
          <a-checkbox v-for="row in switchTierCandidates" :key="row.log_definition" :value="row.log_definition">
            {{ row.name }}（当前档位：{{ tierNameByCode(row.tier_code) || '未知' }}）
          </a-checkbox>
        </a-checkbox-group>
        <div v-if="!switchTierCandidates.length" class="field-hint">本服务的日志已经都在这个档位上了。</div>
      </a-modal>
      <LogFormatVerifyDialog
        :open="verifyDialogVisible"
        :service-id="serviceId"
        :target="verifyTarget"
        :deployment-options="verifyDeploymentOptions"
        @update:open="verifyDialogVisible = $event"
        @verified="loadLogConfig()"
      />
    </main>
  </div>
</template>

<script setup>
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { Empty, message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { tableLocale } from '@/util/tableStyle'
import { getApplicationServiceLogConfig, getApplicationDeploymentList, saveApplicationServiceLogSetting, setApplicationServiceLogCollection } from '@/api/assets/application'
import { applyLogTargetsForService, checkLogCollectionStatus, getElasticsearchClusterList, getLogBatchJob, getLogCollectionFilterRules, getLogRetentionTiers, getLogStorageOverview, getServiceCollectionChain, getServiceLogConfigState } from '@/api/monitor'
import ServiceTree from '@/views/assets/application/components/ServiceTree.vue'
import LogQueryPanel from './LogQueryPanel.vue'
import LogCleanupDialog from '@/components/LogCleanupDialog.vue'
import LogFormatVerifyDialog from '@/components/LogFormatVerifyDialog.vue'
import { FORMAT_STATE_COLOR, FORMAT_STATE_LABEL, canVerifyLogFormat, formatActionTooltip, formatStateTooltip } from '@/util/logFormatState'
import { fetchAllPages } from '@/util/fetchAllPages'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { buildStorageUsagePieOption } from '@/util/storageUsagePie'
import { PATH_MACRO_HINT, pathCellValue, unexpandedMacros, hasGlobMeta } from '@/util/logPathMacro'
import LogGlobPreview from '@/components/LogGlobPreview.vue'
import StorageUsagePie from './StorageUsagePie.vue'

const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE

// 「路径」列显示原始模式还是解析后的路径（解析后 = 模板默认值 + 服务覆盖；实例级宏仍留占位符）。
const showResolvedPath = ref(true)

// 树的选中 scope：服务节点与部署实例节点都带 applicationServiceId，所以两者都能用同一个服务上下文。
const scope = ref({ nodeType: 'all', nodeTitle: '全部业务' })
const serviceId = computed(() => scope.value?.applicationServiceId || null)

const activeTab = ref('query')
const configLoading = ref(false)
const configError = ref('')
const logRows = ref([])
const deploymentRecords = ref([])
const retentionTierRecords = ref([])

const storageLoading = ref(false)
const storageError = ref('')
const storageRows = ref([])
// 水位是"集群 + 时刻"的快照：把集群前缀与数据时间一起存下来，不展示就没法解读这些数字。
const storageCluster = ref(null)
const storageGeneratedAt = ref('')
const storageAllocation = ref([])
const storageAllocError = ref('')
const storageDims = ref({ projects: [], business_systems: [], environments: [] })
// 展开后备索引的那条流（旧页的「后备索引」按钮，真实磁盘占用的最细粒度）。
const expandedStream = ref(null)

const verifyDialogVisible = ref(false)
const verifyTarget = ref(null)
// 本服务下发：聚合状态 + 批量作业进度（作业在服务端，页面只是它的视图，刷新后按 id 继续看）。
// 服务级采集总开关（来自 log-config 响应，与逐条开关是两层）。
// **三态**：null = 还没读到。初值取 false 会让"点开服务、配置还没回来"的那一瞬间按
// "总开关已关闭"渲染，闪出一条黄色警告再消失（用户看到的就是这个）。
const serviceCollectEnabled = ref(null)
// 本服务编码（响应顶层）：水位查询用它，不依赖 logs 是否为空。
const serviceCodeRef = ref('')
const savingServiceCollect = ref(false)
const applyState = ref(null)
const applyStateLoading = ref(false)
const applyStateError = ref('')
const applying = ref(false)
const applyJob = ref(null)
// 历史流的两个动作：切回该档位（逐条改档位）/ 立即清理（按流删文档）。
const switchTierOpen = ref(false)
const switchTierTarget = ref(null)
const switchTierSelected = ref([])
const switchTierSaving = ref(false)
// 清理弹窗（共享组件）的状态：scope 决定"清哪个服务的哪条流"还是"清哪条未识别流"。
const cleanupOpen = ref(false)
const cleanupScope = ref(null)
let applyJobTimer = null
const APPLY_POLL_INTERVAL = 3000
// 按行保存中的标记：log_definition -> true（控件显示 loading 并防重复提交）。
const savingRows = reactive({})
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

// 这一页所有可改列的**共同语义**（改动即时入库、点下发才在主机上生效），
// 各列 tooltip 统一引用它，避免"有的说保存后、有的说改动后"这种措辞漂移。
const pendingHint = '改动即时入库（这一页没有"保存"按钮）；点页头「下发本服务的采集配置」后才在主机上生效。'

const configColumns = [
  { title: '日志名称', dataIndex: 'name', key: 'name', width: 150 },
  { title: '路径', key: 'resolved_path', width: 260 },
  // 主机是服务级的（同一服务的全部日志都由这些主机采集），所以每行内容相同——
  // 但用户是"看着某一行问这条日志在哪台机器上"，按行给比只写在页头好找。
  { title: '所在主机', key: 'carrying_hosts', width: 220 },
  { title: '处理规则（模板）', key: 'processing_rule', width: 200 },
  { title: '采集', key: 'collection_enabled', width: 100 },
  { title: '格式校验', key: 'format_state', width: 110 },
  { title: '格式认证', key: 'format_action', width: 110 },
  { title: '保留档位', key: 'retention_tier', width: 140 },
  // 采集过滤的两个方向：随时可开可关。改完必须重新下发才在主机上生效（与采集开关同一条动线）。
  { title: '采集过滤（保留）', key: 'filter_include', width: 190 },
  { title: '采集过滤（排除）', key: 'filter_exclude', width: 190 },
  { title: 'Data Stream', key: 'data_stream', width: 260 },
]

const storageColumns = computed(() => [
  { title: 'Data Stream', dataIndex: 'name', key: 'name', width: 300 },
  // 全量视图下必须能看到归属，否则一堆流分不清是谁的；选中服务时服务是已知的，不重复占位置。
  ...(serviceId.value ? [] : [{ title: '归属服务', key: 'owner', width: 110 }]),
  { title: '档位', dataIndex: 'tier', key: 'tier', width: 120 },
  { title: '文档数', key: 'docs', width: 110 },
  { title: '磁盘占用', key: 'bytes', width: 110 },
  { title: 'ILM', key: 'ilm_state', width: 100 },
  { title: '状态', key: 'collect_state', width: 150 },
  { title: '后备索引', key: 'backing', width: 100 },
  // 操作列**恒在**（2026-09-19 修）：清理对每条流都成立（未识别流也能清），
  // 之前整列按"是否选中服务"开关，于是全量视图/项目/业务系统/环境节点下根本没有这一列。
  // 只有「切回该档位」需要服务上下文（要改该服务的日志定义），它在单元格里单独把门。
  { title: '操作', key: 'actions', width: 160, fixed: 'right' },
])

const allocationColumns = [
  { title: '节点', dataIndex: 'name', key: 'name' },
  { title: '地址', dataIndex: 'node', key: 'node' },
  { title: '分片数', dataIndex: 'shards', key: 'shards', width: 90 },
  { title: '磁盘已用', key: 'disk.used', width: 110 },
  { title: '磁盘总量', key: 'disk.total', width: 110 },
  { title: '使用率', key: 'disk.percent', width: 90 },
]

const backingColumns = [
  { title: '索引', dataIndex: 'index', key: 'index' },
  { title: '健康', dataIndex: 'health', key: 'health', width: 80 },
  { title: '占用', key: 'bytes', width: 110 },
  { title: '文档数', key: 'docs', width: 110 },
  { title: '创建时间', dataIndex: 'create_at', key: 'create_at', width: 180 },
  { title: 'ILM 状态', dataIndex: 'ilm_state', key: 'ilm_state', width: 120 },
]

// 采集开关按 (服务 × 日志定义) 生效：无覆盖行表示"采"，显式 false 才是"关"（与后端同口径）。
function isLogCollected(record) {
  return record.collection_enabled !== false
}

const retentionTierOptions = computed(() => retentionTierRecords.value.map((item) => ({ label: item.name, value: item.id })))

const applyManagedCount = computed(() => applyState.value?.summary?.managed || 0)
const applyUnmanagedText = computed(() => {
  const unmanaged = applyState.value?.unmanaged_hosts || []
  if (!unmanaged.length) return ''
  const names = unmanaged.map((item) => item.host_instance_name || item.host_ip || `host-${item.host_id}`)
  return `${unmanaged.length} 台承载主机还没有纳管日志采集，本服务下发不了它们：${names.join('、')}`
})
// 聚合文案：这里的计数是**本服务**在各承载主机上的配置态（后端按 (主机 × 服务) 子指纹判定，
// 共享主机上其他服务的改动不会把本服务带成待下发，见 docs §8.3）。
const applySummaryText = computed(() => {
  const summary = applyState.value?.summary
  if (!summary || !summary.hosts) return ''
  const parts = [`承载 ${summary.hosts} 台主机`]
  if (summary.synced) parts.push(`${summary.synced} 台已同步`)
  if (summary.drift) parts.push(`${summary.drift} 台待下发`)
  if (summary.never) parts.push(`${summary.never} 台从未下发`)
  if (summary.unknown) parts.push(`${summary.unknown} 台状态未知`)
  if (summary.unmanaged) parts.push(`${summary.unmanaged} 台未纳管`)
  return parts.join('，')
})
const applyButtonTooltip = computed(() => {
  if (!applyManagedCount.value) return '没有已纳管的承载主机可下发；先到「日志采集」页把主机纳管。'
  return '对承载本服务的全部已纳管主机重下发采集配置。会连带重算这些主机上其他服务的配置（内容等价，'
    + '除非它们本来就有未下发的改动）；配置未变化的主机会被自动跳过。'
})
const applyJobPercent = computed(() => {
  const job = applyJob.value
  if (!job || !job.total_count) return 0
  return Math.round(((job.success_count + job.failed_count) / job.total_count) * 100)
})

// 日志在哪台机器上：日志定义的路径是模板级、按实例宏展开，所以"这条日志所在的主机"
// 就是"承载该服务的主机"。数据来自 service-config-state：hosts 已纳管（可下发）、
// unmanaged_hosts 只在资产里绑了实例还没纳管日志采集（下发不到，要标出来）。
// 两个列表一起显示：只说已纳管的话，用户会以为这个服务就这几台机器。
const carryingHosts = computed(() => [
  ...(applyState.value?.hosts || []),
  ...(applyState.value?.unmanaged_hosts || []),
])
function hostLabel(host) {
  const name = host?.host_instance_name || host?.host_ip || `host-${host?.host_id ?? '?'}`
  return host?.host_ip ? `${name}（${host.host_ip}）` : name
}
const carryingHostsSummary = computed(() => {
  const hosts = carryingHosts.value
  if (!hosts.length) return ''
  // 表格里每行都一样（主机是服务级的），多了撑宽列：首台完整显示，其余进 tooltip。
  return hosts.length > 1 ? `${hostLabel(hosts[0])} 等 ${hosts.length} 台` : hostLabel(hosts[0])
})
const carryingHostsTooltip = computed(() => carryingHosts.value
  .map((host) => `${hostLabel(host)}${host.managed === false ? '（未纳管，配置下发不到）' : ''}`)
  .join('、'))

// ---- 采集链路（查不到日志时按层回答"断在哪"）----
const chain = ref(null)
const chainLoading = ref(false)
const chainRefreshing = ref(false)
const chainError = ref('')
const CHAIN_BADGE = { ok: 'success', warn: 'warning', drift: 'error', error: 'error' }
const chainLayers = computed(() => chain.value?.layers || [])
const chainCheckedAt = computed(() => (chain.value?.checked_at || '').replace('T', ' ').slice(0, 19))
// 服务停用 / 服务级采集开关关闭：日志本来就不会再采，且不属于上面五层里的任何一层。
const chainServiceStopReason = computed(() => {
  const service = chain.value?.service
  if (!service) return ''
  if (service.enabled === false) return '该逻辑服务已停用：不会再采集日志（存量数据按保留档位到期）。'
  if (service.log_collection_enabled === false) return '服务采集总开关已关闭：该服务下所有日志都不采集。'
  return ''
})

// 层明细进 tooltip：主机多的时候一行放不下，而"哪台机器有问题"正是要看的。
function chainLayerTooltipLines(layer) {
  const items = layer?.items || []
  const lines = [`${layer?.name || ''}：${layer?.summary || ''}`]
  for (const item of items) {
    lines.push(`${item.name} — ${item.detail}`)
  }
  return lines
}

async function loadChain() {
  if (!serviceId.value) {
    chain.value = null
    return
  }
  chainLoading.value = true
  chainError.value = ''
  try {
    const response = await getServiceCollectionChain(serviceId.value)
    chain.value = response?.data?.data || null
    // 读完就走一遍"要不要自己查运行态"：状态未知时不该让人再点一次按钮。
    autoRefreshChainRuntime()
  } catch (error) {
    chain.value = null
    chainError.value = error?.response?.data?.msg || error?.message || '读取采集链路失败'
  } finally {
    chainLoading.value = false
  }
}

// 刷新运行态：落库的 Filebeat 状态是快照，只有查过才新鲜。逐台查（与「日志采集」页同一接口、
// 同一动作），单台失败不影响其他台，查完重拉链路——不在前端拼状态文案，判定只留后端一处。
async function refreshChainRuntime() {
  const hosts = (chain.value?.hosts?.items || [])
    .filter((host) => host.managed && host.agent_online && host.target_id)
  if (!hosts.length) {
    await loadChain()
    return
  }
  chainRefreshing.value = true
  try {
    await Promise.all(hosts.map(async (host) => {
      try {
        await checkLogCollectionStatus(host.target_id)
      } catch (error) {
        console.warn('[log_center] 查询 Filebeat 运行状态失败', host.target_id, error?.response?.data?.msg || error?.message)
      }
    }))
    await loadChain()
  } finally {
    chainRefreshing.value = false
  }
}

// 「状态未知」的主机：已纳管 + agent 在线 + 没查过状态。
//
// 未知 ≠ 已停止：前者是"不知道"（新纳管、或刚下发重启过），处置就是查一次。以前这一步要人点
// 「刷新运行态」（2026-09-19 现场："每次修改了东西都要手动刷新，麻烦"），现在自己查。
function hostsWithUnknownRuntime(items) {
  return (items || []).filter((host) => (
    host.managed && host.agent_online && host.target_id && !host.runtime_status
  ))
}

// 自动查一次的节流：同一个页面 60 秒最多自动查一次。
// 主机侧命令失败（agent 掉线等）时快照会一直是空的，没有节流就会在每次链路重拉时反复打主机。
const AUTO_RUNTIME_REFRESH_INTERVAL_MS = 60 * 1000
let lastAutoRuntimeRefreshAt = 0
const chainAutoRefreshing = ref(false)

async function autoRefreshChainRuntime() {
  if (chainRefreshing.value || !hostsWithUnknownRuntime(chain.value?.hosts?.items).length) return
  if (Date.now() - lastAutoRuntimeRefreshAt < AUTO_RUNTIME_REFRESH_INTERVAL_MS) return
  lastAutoRuntimeRefreshAt = Date.now()
  chainAutoRefreshing.value = true
  try {
    // refreshChainRuntime 结束时会把链路重拉一遍：那时状态已经有值，不会再触发自动刷新。
    await refreshChainRuntime()
  } finally {
    chainAutoRefreshing.value = false
  }
}

// 服务级采集状态的判据对全量视图与按服务视图一致：服务停用 / 未开采集时该流不会再写入新配置，
// 存量数据按档位保留到期（后端只回事实字段，结论在前端算）。
function serviceCollectState(stream) {
  if (!stream?.recognized) return null
  if (stream.service_enabled === false) return '已停用'
  if (stream.service_collection_enabled === false) return '未开启采集'
  return null
}

function collectStateTooltip() {
  return '该逻辑服务已停用或未开启采集：不会再下发新的采集配置，已写入的数据按保留档位到期，不会自动清理。'
}

function formatBytes(value) {
  const bytes = Number(value || 0)
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let index = 0
  let size = bytes
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

// 采集过滤规则（下拉用）：一次拉全量。规则不多，且通用规则（无应用）对所有服务都适用，
// 所以这里不按应用过滤——方向校验才是关键（白名单不能出现在排除槽里，见 filterRuleOptions）。
const filterRuleRecords = ref([])
function filterRuleOptions(direction) {
  return [
    { label: '不过滤', value: 0 },
    ...filterRuleRecords.value
      .filter((rule) => rule.enabled && (rule.rule_type || 'include') === direction)
      .map((rule) => ({ label: rule.name, value: rule.id })),
  ]
}
// 继承模板时的说明文字：模板没配就写"无"，配了就带上规则名（用户要知道自己继承了什么）。
function inheritedFilterLabel(ruleId) {
  if (!ruleId) return '继承模板（无）'
  const rule = filterRuleRecords.value.find((item) => item.id === ruleId)
  return `继承模板（${rule ? rule.name : `规则 #${ruleId}（已删除）`}）`
}

const verifyDeploymentOptions = computed(() => deploymentRecords.value
  .filter((item) => (scope.value.deploymentId ? item.id === scope.value.deploymentId : true))
  .map((item) => ({ label: `${item.instance_name}（${item.host_name || item.host_ip || '未知主机'}）`, value: item.id })))

// 按行保存覆盖值（采集开关 / 保留档位）。
//
// 不做乐观更新：控件值由 record 派生（:checked / :value），所以请求失败时控件会自动回到原值，
// 不需要手工回滚——手工回滚最容易被忘掉，然后界面显示的和库里的不一致。
//
// **patch 里缺的那一列要带上当前值**：接口把提交体当作该行覆盖值的全集（缺 = 清成"不覆盖"），
// 只传一个字段会把另一列静默清掉。
// 按行提交的覆盖值**全集**。接口把提交体当作该行的全集（缺列 = 清成"不覆盖"），
// 所以即使只改一列也要把其他列带原值。抽成一处是因为调用点有两个（内联改值、切回档位），
// 手拼两次必然出现"新增列只改了一处 → 另一处静默清空"。
function overridePayload(record, patch = {}) {
  const pick = (key, current) => (key in patch ? patch[key] : current)
  return {
    log_definition_id: record.log_definition,
    collection_enabled: pick('collection_enabled', record.collection_enabled ?? null),
    retention_tier: pick('retention_tier', record.retention_tier ?? null),
    // 采集过滤三态原样提交：null 继承模板 / 0 不过滤 / >0 指定规则。
    collection_filter_rule: pick('collection_filter_rule', record.collection_filter_rule_id ?? null),
    collection_exclude_filter_rule: pick('collection_exclude_filter_rule', record.collection_exclude_filter_rule_id ?? null),
  }
}

// 这一页**所有**可写改动都先确认一次，再即时入库——统一节奏，不搞"有的弹有的不弹"。
//
// 为什么统一成"都弹"：这一页的每次改动都是"入了库但还没生效"（要再点下发），且后果各不相同
// （关采集、换档位=写新流、过滤=可能永久丢数据）。给每处一句"会怎样 + 能不能回头 + 什么时候生效"，
// 比让人靠记忆判断哪些是要紧的改动可靠。
// 逻辑服务编辑弹窗不弹：那边改完要点"保存"，那次保存本身就是确认。
async function confirmConfigChange({ title, summary, consequences, apply }) {
  const confirmed = await openDeleteConfirm({
    title,
    okText: '确认修改',
    summary,
    items: [...consequences, '改动即时入库（这一页没有"保存"按钮）；点页头「下发本服务的采集配置」后才在主机上生效。'],
  })
  if (!confirmed) return false
  await apply()
  return true
}

const FILTER_DIRECTIONS = {
  include: { label: '保留（白名单）', effect: '改完之后**只有**匹配该规则正则的记录会被采集' },
  exclude: { label: '排除（黑名单）', effect: '改完之后匹配该规则正则的记录会被**丢掉**' },
}

// 逐条采集开关：默认采，只有"关"才落库；关闭=这条日志不再采集（同模板其他服务不受影响）。
async function confirmLogCollectChange(record, checked) {
  if (checked) {
    // 打开就是回到默认（不落覆盖行），无需确认——它不会让任何东西停止采集。
    await saveOverride(record, { collection_enabled: null })
    return
  }
  await confirmConfigChange({
    title: '确认停止采集这条日志？',
    summary: `日志「${record.name}」将不再被采集。`,
    consequences: [
      '只影响本服务：同一模板下的其他服务不受影响。',
      '已写入的数据按保留档位到期，**不会被删除**；重新打开这个开关即恢复采集。',
    ],
    apply: () => saveOverride(record, { collection_enabled: false }),
  })
}

// 保留档位：改档位会写入**新**流，旧流停止写入并按原档位保留到期（不迁移数据）。
async function confirmTierChange(record, value) {
  const name = value === null
    ? '继承服务默认档位'
    : (retentionTierOptions.value.find((item) => item.value === value)?.label || `档位 #${value}`)
  await confirmConfigChange({
    title: '确认修改保留档位？',
    summary: `日志「${record.name}」的保留档位将改为：${name}。`,
    consequences: [
      '改档位会写入**新的 data stream**；旧流停止写入，按原档位保留到期后由 ILM 删除，**不迁移数据**。',
      '数据不会立刻消失，但同一份日志会短暂出现两条流（这正是水位页里"历史档位"的来源）。',
    ],
    apply: () => saveOverride(record, { retention_tier: value ?? null }),
  })
}

async function confirmFilterChange(record, direction, value) {
  const meta = FILTER_DIRECTIONS[direction] || FILTER_DIRECTIONS.include
  const ruleName = value > 0 ? (filterRuleRecords.value.find((item) => item.id === value)?.name || `规则 #${value}`) : ''
  const target = value === null
    ? inheritedFilterLabel(record[direction === 'include' ? 'template_filter_include_rule_id' : 'template_filter_exclude_rule_id'])
    : value === 0
      ? '不过滤（显式关闭这个方向）'
      : `规则「${ruleName}」`
  await confirmConfigChange({
    title: `确认修改采集过滤：${meta.label}`,
    summary: `日志「${record.name}」的采集过滤（${meta.label}）将改为：${target}。`,
    consequences: [
      meta.effect + '（在**采集侧**过滤，被滤掉的日志不会进 ES，事后无法补回）。',
    ],
    apply: () => saveOverride(record, direction === 'include'
      ? { collection_filter_rule: value ?? null }
      : { collection_exclude_filter_rule: value ?? null }),
  })
}

async function saveOverride(record, patch) {
  const key = record.log_definition
  savingRows[key] = true
  try {
    const response = await saveApplicationServiceLogSetting(serviceId.value, overridePayload(record, patch))
    const updated = response?.data?.data
    if (updated) Object.assign(record, updated)
    // 任何一处覆盖值改完，期望配置都变了（指纹变）→ 承载主机变"待下发"。
    // **必须回读**：否则用户改完开关/档位看不到任何反馈（页头待下发数不变），会以为"没保存成功"。
    // 这也正是这一页的语义：**改动即时入库，点「下发本服务的采集配置」才到主机上**。
    //
    // 链路也一起重拉：**期望配置变了，链路的「主机配置」层结论就变了**（一致 → 待下发）。
    // 少这一步的话，改完过滤/档位后链路还停在旧结论上，用户只能去点「刷新运行态」——
    // 而那个按钮只是顺带重拉了链路，看起来像"运行态需要刷新"（2026-09-19 现场）。
    await Promise.all([loadApplyState(), loadChain()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存失败')
  } finally {
    delete savingRows[key]
  }
}

// 认证弹窗的实例候选：认证按库里的绑定取实例，与表单编辑态无关，所以选中服务时拉一次。
async function loadVerifyDeployments() {
  if (!serviceId.value) {
    deploymentRecords.value = []
    return
  }
  try {
    deploymentRecords.value = await fetchAllPages(getApplicationDeploymentList, { application_service: serviceId.value })
  } catch {
    deploymentRecords.value = []
  }
}

// 档位编码 → 档位名/ID：流的档位是编码，配置里用的是档位 id。
function tierNameByCode(code) {
  const found = retentionTierRecords.value.find((item) => item.code === code)
  return found ? found.name : ''
}

function tierIdByCode(code) {
  const found = retentionTierRecords.value.find((item) => item.code === code)
  return found ? found.id : null
}

// 候选 = 当前生效档位不等于目标档位的日志（只有这些改了才有意义）。
const switchTierCandidates = computed(() => {
  const target = switchTierTarget.value?.tier
  if (!target) return []
  return logRows.value.filter((row) => row.tier_code !== target)
})

function openSwitchTier(stream) {
  switchTierTarget.value = stream
  switchTierSelected.value = switchTierCandidates.value.map((row) => row.log_definition)
  switchTierOpen.value = true
}

// 逐条按行提交：每条是独立的覆盖值改动（不同日志的档位本就互不相干），失败逐条累计汇报，
// 不做"全成功/全失败"的假原子性。
async function submitSwitchTier() {
  const target = switchTierTarget.value?.tier
  const tierId = tierIdByCode(target)
  if (!target || !tierId) {
    message.error('找不到该档位，请刷新后重试')
    return
  }
  const selected = switchTierCandidates.value.filter((row) => switchTierSelected.value.includes(row.log_definition))
  if (!selected.length) {
    message.warning('请至少选择一条日志')
    return
  }
  switchTierSaving.value = true
  let ok = 0
  const failures = []
  for (const row of selected) {
    try {
      await saveApplicationServiceLogSetting(serviceId.value, overridePayload(row, { retention_tier: tierId }))
      ok += 1
    } catch (error) {
      failures.push(`${row.name}：${error?.response?.data?.msg || error?.message || '保存失败'}`)
    }
  }
  switchTierSaving.value = false
  switchTierOpen.value = false
  if (ok) message.success(`已切换 ${ok} 条日志的档位；重新下发采集配置后主机才会走这条流`)
  if (failures.length) message.error(`有 ${failures.length} 条未切换：${failures.join('；')}`)
  await loadLogConfig()
  await loadStorage()
  // 与按行保存同一条口径：改了覆盖值就要重拉链路（「主机配置」层结论变了），
  // 不能让用户靠"点一下刷新运行态"来顺带刷新它。
  await Promise.all([loadApplyState(), loadChain()])
}

// 一条流该怎么清：能归属到逻辑服务的走**服务维度**（服务端自己拼流名、可选按档位收窄），
// 未识别流没有服务可归属 → 走"按流名清理"（服务端校验名字能在集群上找到、且格式合法）。
//
// 服务维度优先不是洁癖：它能按档位收窄（只清这一条流），而按流名那条是"我就要清这个名字"的兜底。
// 找不到服务 id（识别得到但 dims 里没这条服务，例如服务已删）时才退回按流名。
function streamCleanupTarget(stream) {
  if (!stream?.name) return null
  if (stream.recognized) {
    const service = (storageDims.value.services || []).find((item) => item.code === stream.service)
    if (service) return { kind: 'service', serviceId: service.id, tier: stream.tier }
  }
  return { kind: 'stream', stream: stream.name }
}

// 清理按钮的悬停说明：历史档位流、活跃流、未识别流的后果各不相同，先说清再点。
function streamCleanupTooltip(stream) {
  if (isHistoricalTier(stream)) {
    return '删除这条历史档位流里的全部文档、立刻释放磁盘。不可恢复；流对象保留（切回该档位会继续写入它）。'
  }
  if (!stream.recognized) {
    return '这是一条未识别流（不归属任何逻辑服务）：删除它里面的全部文档、立刻释放磁盘。不可恢复；不会停止任何采集。'
  }
  return '删除这条流里的全部文档、立刻释放磁盘。不可恢复；**不会停止采集**——之后新写入的日志仍会进入这条流。'
}

// 清理：打开共享清理弹窗（与「日志查询」原来那个是**同一个**：范围可选保留 N 小时/N 天/全部清空）。
// 作用域按上面的分流规则给：能归属到服务 → 服务维度（带 tier，只清这一条流）；否则按流名。
function cleanupStream(stream) {
  const target = streamCleanupTarget(stream)
  if (!target) return
  cleanupScope.value = target.kind === 'service'
    ? {
      kind: 'service', serviceId: target.serviceId, tier: target.tier,
      label: `${stream.name}（${formatBytes(stream.bytes)}${isHistoricalTier(stream) && stream.tier ? `，档位 ${stream.tier}` : ''}）`,
    }
    : { kind: 'stream', stream: target.stream, label: `${stream.name}（${formatBytes(stream.bytes)}，未识别流）` }
  cleanupOpen.value = true
}

async function loadApplyState() {
  if (!serviceId.value) {
    applyState.value = null
    return
  }
  applyStateLoading.value = true
  applyStateError.value = ''
  try {
    applyState.value = (await getServiceLogConfigState(serviceId.value))?.data?.data || null
  } catch (error) {
    applyState.value = null
    applyStateError.value = error?.response?.data?.msg || error?.message || '读取下发状态失败'
  } finally {
    applyStateLoading.value = false
  }
}

// 切换服务级总开关。不做乐观更新：值来自后端（serviceCollectEnabled），失败就保持原样。
async function toggleServiceCollect(checked) {
  // 先把开关弹回原位（控件值由 serviceCollectEnabled 派生），确认后才真的写——
  // 否则用户取消时开关会停在"已改"的状态上，界面与库不一致。
  const confirmed = await confirmConfigChange({
    title: checked ? '确认开启服务采集？' : '确认关闭服务采集？',
    summary: `服务「${scope.value.nodeTitle || serviceId.value}」的采集总开关将${checked ? '打开' : '关闭'}。`,
    consequences: checked
      ? ['打开后该服务下的日志按逐条开关采集（逐条开关为"不采"的仍不采）。']
      : ['关闭后该服务下**所有**日志都不再采集（逐条开关随之不生效，配置保留，重新打开即恢复）。',
        '已写入的数据按保留档位到期，**不会被删除**。'],
    apply: async () => {
      savingServiceCollect.value = true
      try {
        const response = await setApplicationServiceLogCollection(serviceId.value, checked)
        serviceCollectEnabled.value = response?.data?.data?.log_collection_enabled === true
        // 总开关会改变渲染内容（该服务的片段整体出现/消失）→ 主机配置态与链路随之变化。
        await Promise.all([loadApplyState(), loadChain()])
      } catch (error) {
        message.error(error?.response?.data?.msg || error?.message || '保存采集开关失败')
      } finally {
        savingServiceCollect.value = false
      }
    },
  })
  if (!confirmed) {
    // 取消时把开关显示回真实状态（它本来就是派生值，重新读一次最稳）。
    await loadLogConfig()
  }
}

async function applyService() {
  applying.value = true
  try {
    const response = await applyLogTargetsForService(serviceId.value)
    const data = response?.data?.data || {}
    const jobId = data.job?.id || data.job?.job_id
    message.success(`已创建下发作业，共 ${data.target_total ?? 0} 台主机`)
    await loadApplyState()
    if (jobId) startApplyPolling(jobId)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '下发失败')
  } finally {
    applying.value = false
  }
}

// 轮询作业进度：作业在服务端跑，页面只是它的视图；结束后停表。
function startApplyPolling(jobId) {
  stopApplyPolling()
  const poll = async () => {
    try {
      const job = (await getLogBatchJob(jobId))?.data?.data
      applyJob.value = job
      if (!job?.is_running) {
        // 作业结束后配置态会变（原来待下发的变成已同步），刷一次聚合；链路里的"主机配置"
        // 与"数据写入"两层也都跟着变，一起刷掉，免得留在旧结论上。
        await Promise.all([loadApplyState(), loadChain()])
        // 下发会重启 Filebeat：**重启后的进程态谁都不知道**，这里立刻查一次。
        // 以前要人再点一次「刷新运行态」（2026-09-19 现场："每次修改了东西都要手动刷新"）。
        await refreshChainRuntime()
        return
      }
    } catch {
      return
    }
    applyJobTimer = setTimeout(poll, APPLY_POLL_INTERVAL)
  }
  poll()
}

function stopApplyPolling() {
  if (applyJobTimer) {
    clearTimeout(applyJobTimer)
    applyJobTimer = null
  }
}

onUnmounted(stopApplyPolling)

function openVerifyDialog(record) {
  verifyTarget.value = record
  verifyDialogVisible.value = true
}

async function loadLogConfig() {
  if (!serviceId.value) {
    logRows.value = []
    return
  }
  configLoading.value = true
  configError.value = ''
  // 先清掉上一个服务的数据：留着会让表格在新响应到达前显示**别的服务的日志**（串台），
  // 总开关也会短暂沿用上一个服务的值。
  logRows.value = []
  serviceCollectEnabled.value = null
  serviceCodeRef.value = ''
  try {
    const response = await getApplicationServiceLogConfig(serviceId.value)
    const data = response?.data?.data || {}
    logRows.value = data.logs || []
    serviceCollectEnabled.value = data.log_collection_enabled === true
    serviceCodeRef.value = data.service_code || ''
  } catch (error) {
    // 不静默：加载失败与"确实没有日志定义"在界面上都是空表格（2026-09-19 现场教训）。
    logRows.value = []
    configError.value = error?.response?.data?.msg || error?.message || '加载日志配置失败'
  } finally {
    configLoading.value = false
  }
}

async function loadStorage() {
  // 选中服务：按服务收窄（后端把 ES 查询缩到该服务的索引）。
  // 未选中（全部/项目/业务系统/环境）：取全量，再按树的 scope 在本地过滤——水位是集群级快照，
  // 与"看哪一层"无关，所以顶层数据一次取回，切层级不再打 ES。
  const serviceCode = serviceId.value ? serviceCodeRef.value : ''
  if (serviceId.value && !serviceCode) {
    storageRows.value = []
    return
  }
  storageLoading.value = true
  storageError.value = ''
  expandedStream.value = null
  try {
    const clusters = await getElasticsearchClusterList({ page_size: 1 })
    const clusterId = clusters?.data?.data?.results?.[0]?.id
    if (!clusterId) {
      storageError.value = '还没有可用的 Elasticsearch 集群，请先到「日志存储」页添加'
      storageRows.value = []
      return
    }
    const response = await getLogStorageOverview(clusterId, serviceId.value ? { application_service_id: serviceId.value } : undefined)
    const data = response?.data?.data || {}
    storageCluster.value = data.cluster || null
    storageGeneratedAt.value = data.generated_at || ''
    storageAllocation.value = data.allocation || []
    storageAllocError.value = data.alloc_error || ''
    storageDims.value = data.dims || { projects: [], business_systems: [], environments: [] }
    storageRows.value = (data.data_streams || [])
      // 同一服务可能有多个档位/多条日志，按流名排序让表格稳定。
      .slice()
      .sort((left, right) => String(left.name).localeCompare(String(right.name), 'zh-CN'))
  } catch (error) {
    storageRows.value = []
    storageError.value = error?.response?.data?.msg || error?.message || '读取水位失败'
  } finally {
    storageLoading.value = false
  }
}

// 树 scope → 维度编码过滤。scope 里给的是 id，水位数据的流名里是 code，
// 所以用响应里的 dims 做映射；映射不到就不过滤（宁可多显示，也不要显示成"空"让人以为没数据）。
function scopeCodes(scope) {
  const dims = storageDims.value || {}
  const codes = { project: [], environment: [], businessSystem: [] }
  const projectIds = scope?.nodeType === 'project' ? [scope.projectId] : (scope?.projectIds || [])
  const environmentIds = scope?.nodeType === 'environment' ? [scope.environment] : (scope?.environmentIds || [])
  const businessSystemId = scope?.businessSystemId
  codes.project = (dims.projects || []).filter((item) => projectIds.includes(item.id)).map((item) => item.code)
  codes.environment = (dims.environments || []).filter((item) => environmentIds.includes(item.id)).map((item) => item.code)
  codes.businessSystem = businessSystemId
    ? (dims.business_systems || []).filter((item) => item.id === businessSystemId).map((item) => item.code)
    : []
  return codes
}

// 全量视图下按树的选中层级过滤；未识别的流没有维度信息，任何层级都保留（它们不归属任何服务，
// 藏起来就没人看得见了——这正是旧页「未识别」分组的用意）。
const visibleStorageRows = computed(() => {
  if (serviceId.value) return storageRows.value
  const codes = scopeCodes(scope.value)
  const filtered = storageRows.value.filter((row) => {
    if (!row.recognized) return true
    if (codes.project.length && !codes.project.includes(row.project)) return false
    if (codes.environment.length && !codes.environment.includes(row.environment)) return false
    if (codes.businessSystem.length && !codes.businessSystem.includes(row.business_system)) return false
    return true
  })
  // 未识别流排最后：它们需要人看一眼，但不是这个页面的主角。
  // （recognized=true 是 1，降序才是"已识别的在前"；写成升序会把未识别顶到最前面。）
  return filtered.slice().sort((left, right) => Number(right.recognized) - Number(left.recognized)
    || String(left.name).localeCompare(String(right.name), 'zh-CN'))
})

const unrecognizedStreams = computed(() => storageRows.value.filter((row) => !row.recognized))

function toggleBackingIndices(record) {
  expandedStream.value = expandedStream.value?.name === record.name ? null : record
}

// 历史流（改档位后留下的旧流，已停止写入）由**后端**判定并随每条流返回 `historical`：
// 只有后端知道"该服务当前生效的档位集合"（覆盖档位 → 服务默认 → is_default → 'std' 的
// COALESCE 链），而且这个判定在全局视图里同样成立（前端推不出全局视图的生效档位）。
// 字段缺失（旧构建）时一律按"不是历史流"处理——宁可少标，也不要把正在写的流错标成停写。
function isHistoricalTier(stream) {
  return Boolean(stream?.historical)
}

// 按树选中的层级决定聚合维度：全部 → 按项目；项目 → 按业务系统；业务系统 → 按环境；
// 环境 → 按逻辑服务。选中具体服务时不聚合（直接看它的流）。
const groupDimension = computed(() => {
  switch (scope.value?.nodeType) {
    case 'all': return { key: 'project', label: '项目' }
    case 'project': return { key: 'business_system', label: '业务系统' }
    case 'businessSystem': return { key: 'environment', label: '环境' }
    case 'environment': return { key: 'service', label: '逻辑服务' }
    default: return null
  }
})

function environmentCodeById(environmentId) {
  const found = (storageDims.value.environments || []).find((item) => item.id === environmentId)
  return found ? found.code : ''
}

function environmentNameByCode(code) {
  const found = (storageDims.value.environments || []).find((item) => item.code === code)
  return found ? found.name : code
}

// 该层的**全部**成员（含一条日志都没有的），数据源是 dims 而不是流——否则"某个项目一条日志
// 都没有"这件事就看不见了，而那正是要看的（漏配/维度已删/服务没建）。
const groupEntries = computed(() => {
  const dimension = groupDimension.value
  if (!dimension || serviceId.value) return []
  const dims = storageDims.value || {}
  const current = scope.value || {}
  if (dimension.key === 'project') {
    return (dims.projects || []).map((item) => ({ code: item.code, name: item.name }))
  }
  if (dimension.key === 'business_system') {
    return (dims.business_systems || [])
      .filter((item) => item.project_id === current.projectId)
      .map((item) => ({ code: item.code, name: item.name }))
  }
  if (dimension.key === 'environment') {
    // 环境是**服务上的属性**（assets_business_environment 不挂在业务系统下），所以"该业务系统下的
    // 环境"只能由它名下服务反推：这样即使一条流都没有，配置过的环境也能列出来。
    const businessSystemCode = (dims.business_systems || [])
      .find((item) => item.id === current.businessSystemId)?.code
    const codes = [...new Set((dims.services || [])
      .filter((item) => !businessSystemCode || item.business_system === businessSystemCode)
      .map((item) => item.environment)
      .filter(Boolean))]
    return codes.map((code) => ({ code, name: environmentNameByCode(code) }))
  }
  if (dimension.key === 'service') {
    const environmentCode = environmentCodeById(current.environment)
    return (dims.services || [])
      .filter((item) => !environmentCode || item.environment === environmentCode)
      .map((item) => ({ code: item.code, name: item.name }))
  }
  return []
})

// 容量视角：按维度聚合占用，降序 + 占比。未识别流没有维度信息，不进聚合（另有提示）。
//
// 分组以 groupEntries（维度表的全部成员）为准，再补上"流里有、维度表里没有"的编码
// （维度被停用/删除后的遗留流）——两个方向都不能漏：漏了前者看不到空分组，
// 漏了后者会让这些流从统计里消失（它们在明细表里还在，数字却对不上）。
const storageGroups = computed(() => {
  const dimension = groupDimension.value
  if (!dimension || serviceId.value) return []
  const buckets = new Map()
  const ensure = (code, name, extra = false) => {
    const key = code || '(未设置)'
    if (!buckets.has(key)) {
      buckets.set(key, {
        name: name || key, extra,
        streams: 0, docs: 0, bytes: 0, historicalBytes: 0, historicalStreams: 0, unhealthy: 0,
      })
    }
    return buckets.get(key)
  }
  for (const entry of groupEntries.value) ensure(entry.code, entry.name)

  for (const row of visibleStorageRows.value) {
    if (!row.recognized) continue
    const code = row[dimension.key] || ''
    const bucket = ensure(code, code, !groupEntries.value.some((entry) => entry.code === code))
    bucket.streams += 1
    bucket.docs += Number(row.docs || 0)
    bucket.bytes += Number(row.bytes || 0)
    if (row.historical) {
      bucket.historicalBytes += Number(row.bytes || 0)
      bucket.historicalStreams += 1
    }
    if (row.health && row.health !== 'green') bucket.unhealthy += 1
  }
  // 零占用的分组按名字排序放在后面（有占用的按占用降序）。
  const list = [...buckets.values()].sort((left, right) => right.bytes - left.bytes
    || String(left.name).localeCompare(String(right.name), 'zh-CN'))
  const total = list.reduce((sum, item) => sum + item.bytes, 0)
  return list.map((item) => ({ ...item, share: total ? Math.round((item.bytes / total) * 1000) / 10 : 0 }))
})

// 饼图只画有占用的分组（占比对 0 没有意义），前 8 项 + 其他。
const storagePieOption = computed(() => buildStorageUsagePieOption(storageGroups.value, {
  title: `${groupDimension.value?.label || ''}占用占比`,
  formatBytes,
}))

const groupColumns = computed(() => [
  { title: groupDimension.value?.label || '分组', dataIndex: 'name', key: 'name' },
  { title: 'data stream', dataIndex: 'streams', key: 'streams', width: 110 },
  { title: '文档数', key: 'docs', width: 130 },
  { title: '磁盘占用', key: 'bytes', width: 120 },
  { title: '其中历史', key: 'historicalBytes', width: 120 },
  { title: '占比', key: 'share', width: 180 },
  { title: '健康异常', dataIndex: 'unhealthy', key: 'unhealthy', width: 100 },
])

// 统计口径 = 当前**显示中**的流（全量视图会按树的层级过滤），否则数字和表格对不上。
const storageSummary = computed(() => {
  if (!visibleStorageRows.value.length) return null
  // 活跃/历史分开算：两者语义不同——前者是当前成本，后者是"还占着盘、但已不再写入"的沉淀。
  // 合成一个数看不出问题，也没法判断"该不该清理"。
  return visibleStorageRows.value.reduce((acc, row) => {
    const historical = isHistoricalTier(row)
    return {
      streams: acc.streams + 1,
      docs: acc.docs + Number(row.docs || 0),
      bytes: acc.bytes + Number(row.bytes || 0),
      historicalStreams: acc.historicalStreams + (historical ? 1 : 0),
      activeBytes: acc.activeBytes + (historical ? 0 : Number(row.bytes || 0)),
      historicalBytes: acc.historicalBytes + (historical ? Number(row.bytes || 0) : 0),
      unhealthy: acc.unhealthy + (row.health && row.health !== 'green' ? 1 : 0),
    }
  }, { streams: 0, docs: 0, bytes: 0, historicalStreams: 0, activeBytes: 0, historicalBytes: 0, unhealthy: 0 })
})

// 切服务：日志配置立刻读（页面主体），水位留到切到那个 tab 再读（一次 ES 调用）。
watch(serviceId, async () => {
  verifyDialogVisible.value = false
  storageRows.value = []
  storageError.value = ''
  expandedStream.value = null
  stopApplyPolling()
  applyJob.value = null
  applyState.value = null
  applyStateError.value = ''
  deploymentRecords.value = []
  // 一开始就置 loading：否则从"清空"到 loadApplyState 真正发出请求之间，汇总区会显示
  // "该服务还没有绑定任何部署实例"——又是一句一闪而过的错话。
  applyStateLoading.value = Boolean(serviceId.value)
  // 三个读操作互不依赖，并发发出去（原先串行，把闪烁窗口拉长到三次往返）。
  await Promise.all([loadLogConfig(), loadApplyState(), loadVerifyDeployments()])
  // 采集链路只读库 + 两次 ES 查询（pipeline 存在性、窗口内写入量），跟着服务一起刷。
  // 有意不在这里自动查 Filebeat 运行态：那是逐台给主机发命令，服务下主机多时会让选服务变慢；
  // 状态是快照，链路里会明确写"状态未知，需刷新"，用户点一下按钮即可（见 refreshChainRuntime）。
  loadChain()
  // 水位依赖 log-config 里的服务编码，必须等它回来再读；未选中服务时也要读（全量视图）。
  if (activeTab.value === 'storage') await loadStorage()
}, { immediate: true })

// 树选到项目/业务系统/环境时，数据不用重取（全量已在本页），只按维度过滤即可；
// 但若还没取过全量（例如一直停在配置 tab），切到水位 tab 时会补一次。
watch(() => [scope.value?.nodeType, scope.value?.projectId, scope.value?.businessSystemId, scope.value?.environment],
  () => {
    if (expandedStream.value) expandedStream.value = null
    if (activeTab.value === 'storage' && !serviceId.value && !storageRows.value.length && !storageLoading.value) {
      loadStorage()
    }
  })

watch(activeTab, async (tab) => {
  // 选中服务时受"必须知道服务编码"约束；未选中时任何层级都可以读全量。
  const canLoad = serviceId.value ? Boolean(serviceCodeRef.value) : true
  if (tab === 'storage' && canLoad && !storageRows.value.length && !storageError.value) {
    await loadStorage()
  }
})

// 采集过滤规则同样只在首屏拉一次（两列下拉用）。
getLogCollectionFilterRules({ page_size: 200 })
  .then((response) => { filterRuleRecords.value = response?.data?.data?.results || [] })
  .catch(() => { filterRuleRecords.value = [] })

// 档位名要显示成人话（"继承服务默认"与具体档位名），只在首屏拉一次。
getLogRetentionTiers({ page_size: 100 })
  .then((response) => { retentionTierRecords.value = response?.data?.data?.results || [] })
  .catch(() => { retentionTierRecords.value = [] })
</script>

<style scoped>
.log-center {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  gap: 12px;
  height: 100%;
}
.log-center-content {
  min-width: 0;
  padding: 12px;
  background: #fff;
  border-radius: 6px;
}
.log-center-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.log-center-title {
  font-size: 14px;
  font-weight: 500;
}
.apply-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.apply-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.apply-switch {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.apply-switch-label {
  font-size: 13px;
}
.apply-sep {
  color: rgba(0, 0, 0, 0.15);
}
.apply-summary {
  font-size: 13px;
}
.apply-muted {
  color: rgba(0, 0, 0, 0.45);
}
.apply-error {
  color: #d4380d;
}
.apply-unmanaged {
  margin-bottom: 8px;
}
.apply-progress {
  margin-bottom: 8px;
}
.storage-section-title {
  margin: 4px 0 8px;
  font-size: 13px;
  font-weight: 500;
}
/* 采集链路：一行放不下就换行；异常层用颜色标出来（层名本身也是结论，不只是装饰）。 */
.chain-bar {
  padding: 8px 12px;
  margin-bottom: 8px;
  background: #fafafa;
  border-radius: 6px;
}
.chain-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.chain-title {
  font-size: 13px;
  font-weight: 500;
}
.chain-layers {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 16px;
}
.chain-layer {
  font-size: 12px;
  white-space: nowrap;
  cursor: default;
}
.path-toggle {
  margin-left: 6px;
}
.path-macro-tag {
  margin-left: 6px;
  font-size: 11px;
}
.chain-layer.chain-warn {
  color: #d48806;
}
.chain-layer.chain-drift,
.chain-layer.chain-error {
  color: #cf1322;
}
.switch-tier-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 8px;
}
.field-hint {
  margin-top: 4px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
.log-rule-missing {
  color: #d4380d;
}
</style>
