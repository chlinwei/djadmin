<template>
  <div class="monitor-page">
    <a-row :gutter="12" class="tools">
      <a-col :span="16">
        <a-space>
          <a-tag color="blue">Prometheus</a-tag>
          <span class="prom-url">{{ prometheusBaseUrl || '-' }}</span>
          <a-tag v-if="lastRefreshAtText" color="default">刷新于 {{ lastRefreshAtText }}</a-tag>
        </a-space>
      </a-col>
      <a-col :span="8" class="right-actions">
        <a-space>
          <a-switch v-model:checked="autoRefreshEnabled" checked-children="自动刷新" un-checked-children="手动" />
          <a-select v-model:value="refreshIntervalSeconds" style="width: 120px" :options="refreshIntervalOptions" :disabled="!autoRefreshEnabled" :getPopupContainer="getPopupContainer" />
          <a-tooltip title="刷新">
            <a-button type="primary" ghost :loading="loading" @click="loadAllData">
              刷新
            </a-button>
          </a-tooltip>
        </a-space>
      </a-col>
    </a-row>

    <div class="overview-grid">
      <a-card size="small" class="overview-card">
        <a-statistic title="监控目标总数" :value="overview.total" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="采集正常" :value="overview.up" :value-style="{ color: '#3f8600' }" />
      </a-card>
      <a-card size="small" class="overview-card">
        <a-statistic title="采集异常" :value="overview.down" :value-style="{ color: '#cf1322' }" />
      </a-card>
    </div>

    <a-card title="智能监控" size="small" class="monitor-card">
      <a-alert
        v-if="errorMessage"
        type="warning"
        show-icon
        :message="errorMessage"
        style="margin-bottom: 12px"
      />

      <a-tabs v-model:activeKey="activeTabKey">
        <a-tab-pane key="prom-targets" tab="Prometheus 采集目标">
          <a-table
            rowKey="instance"
            :columns="promTargetColumns"
            :data-source="promTargets"
            :loading="loading"
            size="small"
            :scroll="{ x: 1700 }"
            :pagination="{ pageSize: 10, showSizeChanger: true }"
          />
        </a-tab-pane>

        <a-tab-pane key="managed-targets" tab="纳管目标">
          <div class="managed-target-toolbar">
            <span class="managed-target-toolbar__title">主机纳管总览</span>
            <a-tooltip title="刷新">
              <a-button size="large" :loading="overviewLoading" @click="reloadOverviewHosts">
                <FontAwesomeIcon :icon="['fas', 'rotate']" />
                <span>&nbsp;刷新</span>
              </a-button>
            </a-tooltip>
          </div>

          <div class="fluent-bit-batch-bar">
            <span class="fluent-bit-batch-bar__count">已选 {{ overviewSelectedHostIds.length }} 台主机</span>
            <a-space :size="8" wrap>
              <a-tooltip title="为选中主机批量纳管并安装 Exporter" placement="top">
                <a-button
                  type="primary"
                  size="small"
                  :disabled="!overviewSelectedHostIds.length"
                  @click="openExporterCreateModal"
                >
                  <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
                  &nbsp;装 Exporter（{{ overviewSelectedHostIds.length }}）
                </a-button>
              </a-tooltip>
              <a-divider type="vertical" />
              <a-tooltip title="为选中的未纳管主机批量安装 Fluent Bit" placement="top">
                <a-button
                  type="primary"
                  size="small"
                  :disabled="!fluentBitSelectedUnmanaged.length"
                  :loading="fluentBitBatchLoading === 'create'"
                  @click="handleFluentBitBatchCreate"
                >
                  <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
                  &nbsp;装 Fluent Bit（{{ fluentBitSelectedUnmanaged.length }}）
                </a-button>
              </a-tooltip>
              <a-tooltip title="批量重新安装 Fluent Bit" placement="top">
                <a-button
                  type="primary"
                  ghost
                  size="small"
                  :disabled="!fluentBitSelectedManagedIds.length"
                  :loading="fluentBitBatchLoading === 'retry'"
                  @click="handleFluentBitBatch('retry')"
                >
                  <FontAwesomeIcon :icon="['fas', 'rotate']" />
                  &nbsp;重新安装（{{ fluentBitSelectedManagedIds.length }}）
                </a-button>
              </a-tooltip>
              <a-tooltip title="批量启动 Fluent Bit" placement="top">
                <a-button
                  type="primary"
                  ghost
                  size="small"
                  :disabled="!fluentBitSelectedManagedIds.length"
                  :loading="fluentBitBatchLoading === 'start'"
                  @click="handleFluentBitBatch('start')"
                >
                  <FontAwesomeIcon :icon="['fas', 'play']" />
                  &nbsp;启动
                </a-button>
              </a-tooltip>
              <a-tooltip title="批量停止 Fluent Bit" placement="top">
                <a-button
                  danger
                  ghost
                  size="small"
                  :disabled="!fluentBitSelectedManagedIds.length"
                  :loading="fluentBitBatchLoading === 'stop'"
                  @click="handleFluentBitBatch('stop')"
                >
                  <FontAwesomeIcon :icon="['fas', 'stop']" />
                  &nbsp;停止
                </a-button>
              </a-tooltip>
              <a-tooltip title="批量下发 Fluent Bit 配置" placement="top">
                <a-button
                  type="primary"
                  ghost
                  size="small"
                  :disabled="!fluentBitSelectedManagedIds.length"
                  :loading="fluentBitBatchLoading === 'apply'"
                  @click="handleFluentBitBatch('apply')"
                >
                  <FontAwesomeIcon :icon="['fas', 'paper-plane']" />
                  &nbsp;下发配置
                </a-button>
              </a-tooltip>
              <a-tooltip title="批量删除 Fluent Bit 目标" placement="top">
                <a-button
                  class="delBtn"
                  danger
                  type="primary"
                  size="small"
                  :disabled="!fluentBitSelectedManagedIds.length"
                  :loading="fluentBitBatchLoading === 'delete'"
                  @click="openFluentBitBatchDeleteConfirm"
                >
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  &nbsp;删除
                </a-button>
              </a-tooltip>
            </a-space>
          </div>

          <div class="fluent-bit-layout">
            <div class="fluent-bit-tree">
              <a-input
                v-model:value="overviewGroupKeyword"
                allow-clear
                size="small"
                placeholder="搜索分组"
                class="fluent-bit-tree__search"
              />
              <div class="fluent-bit-tree__body">
                <a-tree
                  block-node
                  :tree-data="overviewGroupTreeData"
                  :selected-keys="overviewSelectedGroupKeys"
                  :expanded-keys="overviewGroupExpandedKeys"
                  :auto-expand-parent="true"
                  @select="handleOverviewGroupSelect"
                  @expand="(keys) => (overviewGroupExpandedKeys = keys)"
                />
              </div>
            </div>
            <div class="fluent-bit-table">
              <div class="fluent-bit-table__filters">
                <a-input-search
                  v-model:value="overviewKeyword"
                  allow-clear
                  size="small"
                  placeholder="搜索主机名 / IP"
                  style="width: 200px"
                  @search="reloadOverviewHosts"
                />
                <a-select
                  v-model:value="exporterFilterType"
                  size="small"
                  style="width: 170px"
                  placeholder="全部 Exporter"
                  allow-clear
                  :options="exporterFilterOptions"
                  :getPopupContainer="getPopupContainer"
                  @change="reloadOverviewHosts"
                />
                <a-radio-group v-model:value="overviewManagedFilter" size="small" @change="reloadOverviewHosts">
                  <a-radio-button value="">全部 Exporter</a-radio-button>
                  <a-radio-button value="true">已纳管</a-radio-button>
                  <a-radio-button value="false">未纳管</a-radio-button>
                </a-radio-group>
                <a-radio-group v-model:value="fluentBitManagedFilter" size="small" @change="reloadOverviewHosts">
                  <a-radio-button value="">全部 Fluent Bit</a-radio-button>
                  <a-radio-button value="true">已安装</a-radio-button>
                  <a-radio-button value="false">未安装</a-radio-button>
                </a-radio-group>
              </div>
              <a-table
                rowKey="host_id"
                :columns="overviewColumns"
                :data-source="overviewHosts"
                :loading="overviewLoading"
                :row-selection="overviewRowSelection"
                size="small"
                :scroll="{ x: overviewScrollX }"
                :pagination="overviewPagination"
                @change="handleOverviewTableChange"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'exporters'">
                    <a-space v-if="record.exporters.length" :size="4" wrap>
                      <a-tag
                        v-for="item in record.exporters"
                        :key="item.id"
                        :color="exporterTagColor(item)"
                      >
                        {{ item.exporter_type }}:{{ item.scrape_port }}
                      </a-tag>
                    </a-space>
                    <a-tag v-else color="default">未纳管</a-tag>
                  </template>
                  <template v-else-if="column.key === 'managed_enabled'">
                    <a-tag v-if="record.managed" :color="record.managed_enabled ? 'green' : 'default'">
                      {{ record.managed_enabled ? '启用' : '禁用' }}
                    </a-tag>
                    <span v-else>-</span>
                  </template>
                  <template v-else-if="column.key === 'install_status'">
                    <a-tooltip v-if="record.managed && record.install_message" :title="record.install_message" placement="top">
                      <a-tag :color="statusColor(record.install_status)">{{ record.install_status || 'unknown' }}</a-tag>
                    </a-tooltip>
                    <a-tag v-else-if="record.managed" :color="statusColor(record.install_status)">
                      {{ record.install_status || 'unknown' }}
                    </a-tag>
                    <a-tag v-else color="default">未纳管</a-tag>
                  </template>
                  <template v-else-if="column.key === 'last_scrape_status'">
                    <a-tag v-if="record.managed" :color="scrapeColor(record.last_scrape_status)">
                      {{ record.last_scrape_status || 'unknown' }}
                    </a-tag>
                    <span v-else>-</span>
                  </template>
                  <template v-else-if="column.key === 'fluent_bit_status'">
                    <a-tooltip v-if="fluentBitStatusTooltip(record.fluent_bit)" :title="fluentBitStatusTooltip(record.fluent_bit)" placement="top">
                      <a-tag :color="fluentBitStatusColor(record.fluent_bit)">
                        {{ fluentBitStatusText(record.fluent_bit) }}
                      </a-tag>
                    </a-tooltip>
                    <a-tag v-else :color="fluentBitStatusColor(record.fluent_bit)">
                      {{ fluentBitStatusText(record.fluent_bit) }}
                    </a-tag>
                  </template>
                  <template v-else-if="column.key === 'last_applied_time'">
                    {{ record.fluent_bit.managed ? formatManagedTargetTime(record.fluent_bit.last_applied_time) : '-' }}
                  </template>
                  <template v-else-if="column.key === 'last_error'">
                    <a-tooltip v-if="record.fluent_bit.last_error" :title="record.fluent_bit.last_error" placement="top">
                      <a-typography-text type="danger" :content="record.fluent_bit.last_error" ellipsis />
                    </a-tooltip>
                    <span v-else>-</span>
                  </template>
                  <template v-else-if="column.key === 'action'">
                    <a-space :size="6">
                      <a-tooltip
                        v-if="!exporterFilterType || !record.managed"
                        :title="exporterActionTooltip(record)"
                        placement="top"
                      >
                        <a-button
                          type="primary"
                          ghost
                          size="small"
                          :disabled="!record.host_agent_online"
                          @click="openExporterCreateModal(record)"
                        >
                          <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
                          &nbsp;Exporter
                        </a-button>
                      </a-tooltip>
                      <a-dropdown v-else trigger="click" :getPopupContainer="getPopupContainer">
                        <a-tooltip :title="`${exporterFilterType} 操作`" placement="top">
                          <a-button type="primary" ghost size="small">
                            Exporter&nbsp;<FontAwesomeIcon :icon="['fas', 'angle-down']" />
                          </a-button>
                        </a-tooltip>
                        <template #overlay>
                          <div class="row-action-menu">
                            <a-tooltip :title="isManagedTargetActionDisabledByAgent(record)
                              ? 'dj-agent 离线，操作不可用'
                              : (record.managed_enabled ? '重新安装' : '重新卸载')" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="isManagedTargetActionDisabledByAgent(record)"
                                :loading="managedRetryLoading[record.id]"
                                @click="openManagedTargetRetryConfirm(record)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'rotate']" />
                                &nbsp;{{ record.managed_enabled ? '重新安装' : '重新卸载' }}
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="isManagedTargetActionDisabledByAgent(record)
                              ? 'dj-agent 离线，操作不可用'
                              : '查看监控安装历史日志'" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="isManagedTargetActionDisabledByAgent(record) || managedRetryLoading[record.id]"
                                @click="openManagedTargetJobLog(record)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'file-lines']" />
                                &nbsp;详细日志
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="isManagedTargetActionDisabledByAgent(record)
                              ? 'dj-agent 离线，操作不可用'
                              : (record.install_status !== 'success' ? '尚未安装成功，无法启动' : '启动服务')" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="isManagedTargetActionDisabledByAgent(record) || record.install_status !== 'success'"
                                :loading="managedStartLoading[record.id]"
                                @click="handleStartService(record)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'play']" />
                                &nbsp;运行
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="isManagedTargetActionDisabledByAgent(record)
                              ? 'dj-agent 离线，操作不可用'
                              : (record.install_status !== 'success' ? '尚未安装成功，无法停止' : '停止服务')" placement="left">
                              <a-button
                                block
                                danger
                                ghost
                                size="small"
                                :disabled="isManagedTargetActionDisabledByAgent(record) || record.install_status !== 'success'"
                                :loading="managedStopLoading[record.id]"
                                @click="handleStopService(record)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'stop']" />
                                &nbsp;停止
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="isManagedTargetActionDisabledByAgent(record)
                              ? 'dj-agent 离线，操作不可用'
                              : '查看状态图'" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="isManagedTargetActionDisabledByAgent(record)"
                                :loading="managedServiceStatusLoading[record.id]"
                                @click="openManagedServiceStatus(record)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'rotate']" />
                                &nbsp;查看状态图
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="!canCancelManagedTarget(record) ? '当前任务已结束，无需取消' : '取消'" placement="left">
                              <a-button
                                block
                                danger
                                ghost
                                size="small"
                                :disabled="!canCancelManagedTarget(record)"
                                :loading="managedCancelLoading[record.id]"
                                @click="onCancelManagedTarget(record)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'ban']" />
                                &nbsp;取消
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="isManagedTargetActionDisabledByAgent(record)
                              ? 'dj-agent 离线，操作不可用'
                              : (record.managed_enabled
                                ? '请先关闭纳管（会自动下发卸载）后再删除'
                                : (record.install_status === 'pending' ? '卸载任务尚未结束，暂不可删除' : '删除'))" placement="left">
                              <a-button
                                block
                                class="delBtn"
                                danger
                                type="primary"
                                size="small"
                                :disabled="isManagedTargetActionDisabledByAgent(record) || record.managed_enabled || record.install_status === 'pending'"
                                :loading="managedDeleteLoading[record.id]"
                                @click="openManagedTargetDeleteConfirm(record)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                                &nbsp;删除
                              </a-button>
                            </a-tooltip>
                          </div>
                        </template>
                      </a-dropdown>

                      <a-tooltip
                        v-if="!record.fluent_bit.managed"
                        :title="record.host_agent_online ? '纳管并安装 Fluent Bit' : 'dj-agent 离线，操作不可用'"
                        placement="top"
                      >
                        <a-button
                          type="primary"
                          ghost
                          size="small"
                          :disabled="!record.host_agent_online"
                          :loading="fluentBitCreateLoading[record.host_id]"
                          @click="handleFluentBitCreateOne(record.fluent_bit)"
                        >
                          <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
                          &nbsp;Fluent Bit
                        </a-button>
                      </a-tooltip>
                      <a-dropdown v-else trigger="click" :getPopupContainer="getPopupContainer">
                        <a-tooltip title="Fluent Bit 操作" placement="top">
                          <a-button type="primary" ghost size="small">
                            Fluent Bit&nbsp;<FontAwesomeIcon :icon="['fas', 'angle-down']" />
                          </a-button>
                        </a-tooltip>
                        <template #overlay>
                          <div class="row-action-menu">
                            <a-tooltip :title="record.host_agent_online ? '重新安装' : 'dj-agent 离线，操作不可用'" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="!record.host_agent_online"
                                :loading="fluentBitRetryLoading[record.fluent_bit.id]"
                                @click="openFluentBitRetryConfirm(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'rotate']" />
                                &nbsp;重新安装
                              </a-button>
                            </a-tooltip>
                            <a-tooltip title="查看日志" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="fluentBitRetryLoading[record.fluent_bit.id]"
                                @click="openFluentBitJobLog(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'file-lines']" />
                                &nbsp;查看日志
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="record.fluent_bit.agent_installed ? '运行' : 'Fluent Bit 尚未安装，无法启动'" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="!record.host_agent_online || !record.fluent_bit.agent_installed"
                                :loading="fluentBitStartLoading[record.fluent_bit.id]"
                                @click="handleStartFluentBitService(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'play']" />
                                &nbsp;运行
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="record.fluent_bit.agent_installed ? '停止服务' : 'Fluent Bit 尚未安装，无法停止'" placement="left">
                              <a-button
                                block
                                danger
                                ghost
                                size="small"
                                :disabled="!record.host_agent_online || !record.fluent_bit.agent_installed"
                                :loading="fluentBitStopLoading[record.fluent_bit.id]"
                                @click="handleStopFluentBitService(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'stop']" />
                                &nbsp;停止
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="fluentBitApplyTooltip(record.fluent_bit)" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="!canApplyFluentBitConfig(record.fluent_bit)"
                                :loading="fluentBitApplyLoading[record.fluent_bit.id]"
                                @click="handleApplyFluentBitConfig(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'paper-plane']" />
                                &nbsp;下发配置
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="record.host_agent_online ? '查看状态图' : 'dj-agent 离线，操作不可用'" placement="left">
                              <a-button
                                block
                                type="primary"
                                ghost
                                size="small"
                                :disabled="!record.host_agent_online"
                                :loading="fluentBitStatusLoading[record.fluent_bit.id]"
                                @click="handleCheckFluentBitStatus(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'rotate']" />
                                &nbsp;查看状态图
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="canCancelFluentBitTarget(record.fluent_bit) ? '取消' : '当前任务已结束，无需取消'" placement="left">
                              <a-button
                                block
                                danger
                                ghost
                                size="small"
                                :disabled="!canCancelFluentBitTarget(record.fluent_bit)"
                                :loading="fluentBitCancelLoading[record.fluent_bit.id]"
                                @click="handleCancelFluentBitTarget(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'ban']" />
                                &nbsp;取消
                              </a-button>
                            </a-tooltip>
                            <a-tooltip :title="record.fluent_bit.install_status === 'pending' ? '任务执行中，暂不可删除' : '删除'" placement="left">
                              <a-button
                                block
                                class="delBtn"
                                danger
                                type="primary"
                                size="small"
                                :disabled="record.fluent_bit.install_status === 'pending' || (!record.host_agent_online && record.fluent_bit.agent_installed)"
                                :loading="fluentBitDeleteLoading[record.fluent_bit.id]"
                                @click="openFluentBitDeleteConfirm(record.fluent_bit)"
                              >
                                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                                &nbsp;删除
                              </a-button>
                            </a-tooltip>
                          </div>
                        </template>
                      </a-dropdown>
                    </a-space>
                  </template>
                </template>
              </a-table>
            </div>
          </div>
        </a-tab-pane>

        <a-tab-pane key="packages" tab="软件仓库">
          <div class="package-toolbar">
            <a-segmented
              v-model:value="packageTypeFilter"
              :options="packageTypeOptions"
              @change="handlePackageTypeFilterChange"
            />
            <a-tooltip :title="packageCreateButtonText">
              <a-button size="large" @click="openPackageCreateModal">
                <FontAwesomeIcon :icon="['fas', 'plus-circle']" />
                <span>&nbsp;{{ packageCreateButtonText }}</span>
              </a-button>
            </a-tooltip>
          </div>
          <a-table
            rowKey="id"
            :columns="packageColumns"
            :data-source="packages"
            :loading="packagesLoading"
            size="small"
            :scroll="{ x: 1800 }"
            :pagination="packagePagination"
            @change="handlePackageTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'package_type'">
                <a-tag :color="record.package_type === 'fluent_bit' ? 'cyan' : 'blue'">
                  {{ record.package_type === 'fluent_bit' ? 'Fluent Bit' : 'Exporter' }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'enabled'">
                <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '禁用' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'size_bytes'">
                {{ formatSize(record.size_bytes) }}
              </template>
              <template v-else-if="column.key === 'synced'">
                <a-tag :color="record.synced ? 'green' : 'orange'">{{ record.synced ? '已同步' : '未同步' }}</a-tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <a-space>
                  <a-tooltip title="编辑">
                    <a-button type="primary" ghost @click="openPackageEditModal(record)">
                      <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="上传">
                    <a-upload
                      accept=".tar.gz,.rpm,.deb"
                      :show-upload-list="false"
                      :before-upload="(file) => beforePackageUpload(file, record)"
                      :custom-request="(options) => handlePackageUpload(options, record)"
                    >
                      <a-button type="primary" ghost :loading="packageUploadLoading[record.id]">
                        <FontAwesomeIcon :icon="['fas', 'upload']" />
                      </a-button>
                    </a-upload>
                  </a-tooltip>
                  <a-tooltip v-if="record.name === 'node_exporter'" title="自动更新">
                    <a-button type="primary" ghost :loading="packageSyncLoading[record.id]" @click="openSyncOfficialModal(record)">
                      <FontAwesomeIcon :icon="['fas', 'rotate']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="下载">
                    <a-button type="primary" ghost :disabled="!record.synced" :href="record.download_url" target="_blank">
                      <FontAwesomeIcon :icon="['fas', 'download']" />
                    </a-button>
                  </a-tooltip>
                  <a-tooltip title="删除">
                    <a-button class="delBtn" danger type="primary" :loading="packageRowLoading[record.id]" @click="openPackageDeleteConfirm(record)">
                      <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                    </a-button>
                  </a-tooltip>
                </a-space>
              </template>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="tsdb-status" tab="TSDB 状态">
          <a-alert
            v-if="tsdbLoadError"
            type="warning"
            show-icon
            :message="tsdbLoadError"
            style="margin-bottom: 12px"
          />

          <a-descriptions bordered size="small" :column="2">
            <a-descriptions-item label="Series 总数">{{ tsdbHeadStats.numSeries ?? '-' }}</a-descriptions-item>
            <a-descriptions-item label="Chunk 总数">{{ tsdbHeadStats.chunkCount ?? '-' }}</a-descriptions-item>
            <a-descriptions-item label="Label Pair 总数">{{ tsdbHeadStats.numLabelPairs ?? '-' }}</a-descriptions-item>
            <a-descriptions-item label="Chunk 最小时间">{{ formatTsdbTime(tsdbHeadStats.minTime) }}</a-descriptions-item>
            <a-descriptions-item label="Chunk 最大时间">{{ formatTsdbTime(tsdbHeadStats.maxTime) }}</a-descriptions-item>
            <a-descriptions-item label="指标分组数">{{ tsdbSeriesByMetric.length }}</a-descriptions-item>
          </a-descriptions>

          <a-row :gutter="12" style="margin-top: 12px">
            <a-col :xs="24" :lg="12" style="margin-bottom: 12px">
              <a-card size="small" title="Top 10 标签名（按取值数量）">
                <a-table
                  rowKey="_idx"
                  :columns="tsdbTopColumns"
                  :data-source="tsdbLabelValueCount.slice(0, 10)"
                  :pagination="false"
                  size="small"
                  :scroll="{ x: 420 }"
                />
              </a-card>
            </a-col>
            <a-col :xs="24" :lg="12" style="margin-bottom: 12px">
              <a-card size="small" title="Top 10 指标名（按时序数量）">
                <a-table
                  rowKey="_idx"
                  :columns="tsdbTopColumns"
                  :data-source="tsdbSeriesByMetric.slice(0, 10)"
                  :pagination="false"
                  size="small"
                  :scroll="{ x: 420 }"
                />
              </a-card>
            </a-col>
            <a-col :xs="24" :lg="12" style="margin-bottom: 12px">
              <a-card size="small" title="Top 10 标签名（按内存占用）">
                <a-table
                  rowKey="_idx"
                  :columns="tsdbTopColumns"
                  :data-source="tsdbMemoryByLabel.slice(0, 10)"
                  :pagination="false"
                  size="small"
                  :scroll="{ x: 420 }"
                />
              </a-card>
            </a-col>
            <a-col :xs="24" :lg="12" style="margin-bottom: 12px">
              <a-card size="small" title="Top 10 标签值对（按时序数量）">
                <a-table
                  rowKey="_idx"
                  :columns="tsdbTopColumns"
                  :data-source="tsdbSeriesByLabelValuePair.slice(0, 10)"
                  :pagination="false"
                  size="small"
                  :scroll="{ x: 420 }"
                />
              </a-card>
            </a-col>
          </a-row>
        </a-tab-pane>

        <a-tab-pane key="prom-config" tab="Config">
          <a-alert
            v-if="promConfigLoadError"
            type="warning"
            show-icon
            :message="promConfigLoadError"
            style="margin-bottom: 12px"
          />
          <a-space style="margin-bottom: 12px">
            <a-tooltip title="Copy">
              <a-button type="primary" ghost :disabled="!promConfigYaml" @click="copyPromConfig">
                Copy
              </a-button>
            </a-tooltip>
          </a-space>
          <a-empty v-if="!promConfigLoadError && !promConfigYaml" description="暂无配置数据" />
          <pre v-else-if="promConfigYaml" class="prom-config-text">{{ promConfigYaml }}</pre>
        </a-tab-pane>

        <a-tab-pane key="prom-flags" tab="启动参数">
          <a-alert
            v-if="promFlagsLoadError"
            type="warning"
            show-icon
            :message="promFlagsLoadError"
            style="margin-bottom: 12px"
          />
          <a-space style="margin-bottom: 12px">
            <a-input-search
              v-model:value="promFlagsKeyword"
              allow-clear
              placeholder="按参数名或值搜索"
              style="width: 320px"
            />
          </a-space>
          <a-empty v-if="!promFlagsLoadError && filteredPromFlagsRows.length === 0" description="暂无启动参数数据" />
          <a-table
            v-else
            rowKey="name"
            :columns="promFlagsColumns"
            :data-source="filteredPromFlagsRows"
            size="small"
            :pagination="{ pageSize: 20, showSizeChanger: true }"
            :scroll="{ x: 1300 }"
          />
        </a-tab-pane>

        <a-tab-pane key="alert-settings" tab="告警设置">
          <a-form layout="vertical" class="alert-settings-form">
            <a-form-item label="历史告警保留天数" required>
              <a-input-number
                v-model:value="alertHistoryRetentionDays"
                :min="1"
                :precision="0"
                :disabled="alertSettingsLoading"
                addon-after="天"
                style="width: 240px"
              />
            </a-form-item>
            <a-form-item>
              <a-button type="primary" :loading="alertSettingsSaving" @click="saveAlertSettings">
                <FontAwesomeIcon :icon="['fas', 'floppy-disk']" />
                <span>&nbsp;保存</span>
              </a-button>
            </a-form-item>
          </a-form>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <a-modal
      title="自动更新（从官方源下载）"
      :open="syncModalVisible"
      :confirm-loading="syncModalSubmitting"
      ok-text="确认更新"
      cancel-text="取消"
      @ok="submitSyncOfficial"
      @cancel="syncModalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="软件包">
          <span>{{ syncModalTarget ? `${syncModalTarget.name} (${syncModalTarget.os}-${syncModalTarget.arch})` : '-' }}</span>
        </a-form-item>
        <a-form-item label="目标版本" required>
          <a-input v-model:value="syncModalVersion" placeholder="例如 1.8.2" />
        </a-form-item>
        <a-alert type="info" show-icon message="将按官方 GitHub Release 命名规则拼接地址下载并覆盖当前文件" />
      </a-form>
    </a-modal>

    <a-modal
      :title="packageCreateButtonText"
      :open="packageCreateModalVisible"
      :confirm-loading="packageCreateModalSubmitting"
      ok-text="创建"
      cancel-text="取消"
      width="680px"
      @ok="submitPackageCreate"
      @cancel="packageCreateModalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="软件包类型" required>
          <a-segmented
            v-model:value="packageCreateForm.package_type"
            :options="packageTypeOptions"
            @change="handlePackageTypeChange"
          />
        </a-form-item>
        <a-form-item label="软件包名称" required>
          <a-input
            v-model:value="packageCreateForm.name"
            :disabled="packageCreateForm.package_type === 'fluent_bit'"
            placeholder="如 node_exporter（小写字母/数字/-/_）"
          />
          <div class="form-item-hint">{{ packageNameHint }}</div>
        </a-form-item>
        <a-form-item label="系统 / 架构" required>
          <a-space>
            <a-select v-model:value="packageCreateForm.os" :options="[{ value: 'linux', label: 'Linux' }]" style="width: 160px" :getPopupContainer="getPopupContainer" />
            <a-select
              v-model:value="packageCreateForm.arch"
              :options="[{ value: 'amd64', label: 'x86_64/amd64' }, { value: 'arm64', label: 'aarch64/arm64' }]"
              style="width: 200px"
              :getPopupContainer="getPopupContainer"
            />
          </a-space>
        </a-form-item>
        <a-row :gutter="12">
          <a-col :xs="24" :sm="7">
            <a-form-item label="包格式" required>
              <a-select
                v-model:value="packageCreateForm.package_format"
                :options="packageFormatOptions"
                style="width: 100%"
                :getPopupContainer="getPopupContainer"
                @change="handlePackageFormatChange"
              />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :sm="10">
            <a-form-item label="适用平台" required>
              <a-select
                v-model:value="packageCreateForm.platform_family"
                :options="platformFamilyOptions"
                style="width: 100%"
                :getPopupContainer="getPopupContainer"
              />
            </a-form-item>
          </a-col>
          <a-col v-if="packageCreateForm.package_format !== 'tar.gz'" :xs="24" :sm="7">
            <a-form-item label="主版本" required>
              <a-input
                v-model:value="packageCreateForm.platform_major"
                placeholder="如 7 / 8 / 9 / 22"
              />
            </a-form-item>
          </a-col>
        </a-row>
        <div class="form-item-hint package-platform-hint">RPM 按 RHEL/CentOS 主版本隔离；DEB 按 Ubuntu/Debian 主版本隔离；Host 无需联网</div>
        <a-form-item label="默认端口">
          <a-input-number v-model:value="packageCreateForm.default_port" :min="1" :max="65535" :precision="0" style="width: 100%" />
        </a-form-item>
        <a-alert type="info" show-icon message="创建后为“未同步”占位记录，请在列表行内点击“上传”补全软件包文件" />
      </a-form>
    </a-modal>

    <a-modal
      title="编辑软件包"
      :open="packageEditModalVisible"
      :confirm-loading="packageEditModalSubmitting"
      ok-text="保存"
      cancel-text="取消"
      width="640px"
      @ok="submitPackageEdit"
      @cancel="packageEditModalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="软件包">
          <span>{{ packageEditTarget ? `${packageEditTarget.name} (${packageEditTarget.os}-${packageEditTarget.arch})` : '-' }}</span>
        </a-form-item>
        <a-form-item label="默认监控端口" required>
          <a-input-number
            v-model:value="packageEditForm.default_port"
            :min="1"
            :max="65535"
            :precision="0"
            style="width: 100%"
            placeholder="例如 9100"
          />
          <div class="form-item-hint">主机编辑页新增该监控项时默认带入此端口，主机级可覆盖</div>
        </a-form-item>
        <a-form-item label="安装 Playbook 内容">
          <a-textarea
            v-model:value="packageEditForm.install_playbook_content"
            :rows="8"
            class="package-playbook-textarea"
            placeholder="安装该软件包使用的 Playbook YAML 内容，留空表示不配置安装（仅本软件包自身生效，与其他软件包相互独立）"
          />
          <div class="form-item-hint">直接在此编辑安装用 Playbook 内容，与下方“卸载 Playbook 内容”成对维护，无需再去“模板管理”页单独创建/挑选模板</div>
        </a-form-item>
        <a-form-item label="卸载 Playbook 内容">
          <a-textarea
            v-model:value="packageEditForm.uninstall_playbook_content"
            :rows="8"
            class="package-playbook-textarea"
            placeholder="卸载该软件包使用的 Playbook YAML 内容，留空表示不配置卸载"
          />
        </a-form-item>
        <a-form-item label="执行工作目录">
          <a-input
            v-model:value="packageEditForm.work_directory"
            placeholder="默认 /tmp"
          />
          <div class="form-item-hint">安装/卸载 Playbook 执行时的工作目录；安装/卸载过程本身固定以 root 身份执行（需要创建系统用户、写 systemd unit 等），不可配置</div>
        </a-form-item>
        <a-form-item label="systemd 服务名">
          <span>{{ packageEditTarget ? packageEditTarget.name + '.service' : '-' }}</span>
        </a-form-item>
        <a-form-item label="systemd unit 文件内容">
          <a-textarea
            v-model:value="packageEditForm.service_file_content"
            :rows="10"
            placeholder="安装 Playbook 中通过 extra_vars.service_file_content 拿到后写入 /usr/lib/systemd/system/<name>.service"
          />
        </a-form-item>
        <a-form-item label="服务运行用户" required>
          <a-input
            v-model:value="packageEditForm.service_run_as_user"
            placeholder="默认 dj-agent，必填"
          />
          <div class="form-item-hint">安装后 systemd 服务常驻运行使用的系统用户，安装 Playbook 会据此创建该系统用户；与"安装任务"本身的执行身份无关，默认与 dj-agent 自身运行账号保持一致</div>
        </a-form-item>
        <a-form-item label="服务运行用户组">
          <a-input
            v-model:value="packageEditForm.service_run_as_group"
            placeholder="默认 dj-agent，留空则使用服务运行用户的主组"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="纳管并安装 Exporter"
      :open="exporterCreateModalVisible"
      :confirm-loading="exporterCreateSubmitting"
      ok-text="纳管并安装"
      cancel-text="取消"
      width="520px"
      @ok="submitExporterCreate"
      @cancel="exporterCreateModalVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="目标主机">
          <a-input :value="`已选 ${exporterCreateHostIds.length} 台`" readonly />
        </a-form-item>
        <a-form-item label="Exporter" required>
          <a-select
            v-model:value="exporterCreateForm.exporter_type"
            :options="exporterOptionList"
            :field-names="{ label: 'name', value: 'name' }"
            placeholder="请选择 Exporter"
            style="width: 100%"
            :getPopupContainer="getPopupContainer"
            @change="handleExporterTypeChange"
          />
        </a-form-item>
        <a-form-item label="抓取端口" required>
          <a-input-number v-model:value="exporterCreateForm.scrape_port" :min="1" :max="65535" style="width: 100%" />
        </a-form-item>
        <a-alert
          type="info"
          show-icon
          message="同一主机的同一 Exporter 只能纳管一次，已存在的会自动跳过"
        />
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="serviceStatusModalVisible"
      title="服务运行状态"
      :footer="null"
      width="720px"
    >
      <template v-if="serviceStatusModalRecord && serviceStatusModalResult">
        <p>
          主机：{{ serviceStatusModalRecord.host_name || serviceStatusModalRecord.host_ip }}
          &nbsp;|&nbsp; Exporter：{{ serviceStatusModalRecord.exporter_type }}
          &nbsp;|&nbsp;
          <a-tag :color="serviceStatusModalResult.exit_code === 0 ? 'green' : 'red'">
            {{ serviceStatusModalResult.exit_code === 0 ? 'active' : (serviceStatusModalResult.status === 'success' ? 'inactive/failed' : serviceStatusModalResult.status) }}
          </a-tag>
        </p>
        <pre class="service-status-output">{{ serviceStatusModalResult.stdout || serviceStatusModalResult.stderr || '(无输出)' }}</pre>
      </template>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  applyLogCollectionConfig,
  batchApplyLogCollectionTargets,
  batchCreateLogCollectionTargets,
  batchCreateMonitorTargets,
  batchDeleteLogCollectionTargets,
  batchRetryLogCollectionTargets,
  batchStartLogCollectionTargets,
  batchStopLogCollectionTargets,
  cancelLogCollectionTarget,
  checkLogCollectionStatus,
  checkManagedTargetServiceStatus,
  createSoftwarePackage,
  deleteManagedTarget,
  deleteLogCollectionTarget,
  deleteSoftwarePackage,
  getPrometheusFlags,
  getMonitorInstallHistoryList,
  getMonitorSummary,
  getMonitorExporterOptions,
  getMonitorHostGroupTree,
  getMonitorHostOverview,
  getPrometheusConfig,
  getPrometheusOverview,
  getPrometheusTsdbStatus,
  getPrometheusTargets,
  getSoftwarePackages,
  retryManagedTarget,
  retryLogCollectionTarget,
  cancelManagedTarget,
  startManagedTargetService,
  startLogCollectionService,
  stopLogCollectionService,
  stopManagedTargetService,
  syncSoftwarePackageFromOfficial,
  updateSoftwarePackage,
  uploadSoftwarePackageFile,
} from '@/api/monitor'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import { formatTimeWithTimezone } from '@/util/timezone'
import store from '@/store'
import { CONFIG_KEYS, getConfigByKey, updateConfigByKey } from '@/api/sys/sysconfig'

defineOptions({
  name: 'MonitorMainPage',
})

const router = useRouter()
// a-select 弹层挂载容器统一复用公共工具，避免每个页面自行处理导致弹层时有时无法正常弹出。
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const loading = ref(false)
const errorMessage = ref('')
const activeTabKey = ref('prom-targets')
const alertHistoryRetentionDays = ref(90)
const alertSettingsLoading = ref(false)
const alertSettingsSaving = ref(false)

const lastRefreshAt = ref(null)
const lastRefreshAtText = computed(() => {
  if (!lastRefreshAt.value) return ''
  return lastRefreshAt.value.toLocaleTimeString('zh-CN', { hour12: false })
})

const prometheusBaseUrl = ref('')
const overview = reactive({ total: 0, up: 0, down: 0 })
const tsdbStatus = ref({})
const tsdbLoadError = ref('')
const promConfigYaml = ref('')
const promConfigLoadError = ref('')
const promFlagsRows = ref([])
const promFlagsLoadError = ref('')
const promFlagsKeyword = ref('')

const promTargets = ref([])
const fluentBitStatusLoading = reactive({})
const fluentBitApplyLoading = reactive({})
const fluentBitRetryLoading = reactive({})
const fluentBitStartLoading = reactive({})
const fluentBitStopLoading = reactive({})
const fluentBitCancelLoading = reactive({})
const fluentBitDeleteLoading = reactive({})
const fluentBitBatchLoading = ref('')
const fluentBitCreateLoading = reactive({})
const overviewHosts = ref([])
const overviewLoading = ref(false)
const overviewSelectedHostIds = ref([])
const overviewGroupTree = ref([])
const overviewGroupTotals = reactive({ total: 0, managed: 0 })
const overviewGroupKeyword = ref('')
const overviewGroupExpandedKeys = ref([])
const overviewSelectedGroupKeys = ref(['all'])
const overviewKeyword = ref('')
const overviewManagedFilter = ref('')
const fluentBitManagedFilter = ref('')
const exporterFilterType = ref(undefined)
const exporterOptionList = ref([])
const exporterCreateModalVisible = ref(false)
const exporterCreateSubmitting = ref(false)
const exporterCreateHostIds = ref([])
const exporterCreateForm = reactive({ exporter_type: undefined, scrape_port: 9100 })
const overviewPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showTotal: (total) => `共有 ${total} 台主机`,
})
const managedRetryLoading = reactive({})
const managedCancelLoading = reactive({})
const managedServiceStatusLoading = reactive({})
const managedStartLoading = reactive({})
const managedStopLoading = reactive({})
const managedDeleteLoading = reactive({})
// 按 record.id 缓存每行最近一次查询到的服务运行状态，让“服务状态”列常驻展示，
// 不需要每次都重新打开弹窗；弹窗仍用于查看完整 systemctl 输出。
const serviceStatusMap = reactive({})
const serviceStatusModalVisible = ref(false)
const serviceStatusModalRecord = ref(null)
const serviceStatusModalResult = ref(null)

const autoRefreshEnabled = ref(true)
const refreshIntervalSeconds = ref(15)
const refreshIntervalOptions = [
  { label: '5秒', value: 5 },
  { label: '10秒', value: 10 },
  { label: '15秒', value: 15 },
  { label: '30秒', value: 30 },
  { label: '60秒', value: 60 },
]
let refreshTimer = null

const promTargetColumns = [
  { title: 'Job', dataIndex: 'job', key: 'job', width: 140 },
  { title: 'Instance', dataIndex: 'instance', key: 'instance', width: 220 },
  { title: 'Health', dataIndex: 'health', key: 'health', width: 100 },
  { title: 'Scrape Pool', dataIndex: 'scrape_pool', key: 'scrape_pool', width: 180 },
  { title: 'Last Scrape', dataIndex: 'last_scrape', key: 'last_scrape', width: 220 },
  { title: 'Scrape URL', dataIndex: 'scrape_url', key: 'scrape_url', width: 260 },
  { title: 'Last Error', dataIndex: 'last_error', key: 'last_error', width: 260 },
]

const OVERVIEW_BASE_COLUMNS = [
  { title: '主机', dataIndex: 'host_name', key: 'host_name', width: 170, fixed: 'left' },
  { title: 'IP', dataIndex: 'host_ip', key: 'host_ip', width: 140 },
  { title: '分组', dataIndex: 'group_name', key: 'group_name', width: 120 },
]

const OVERVIEW_EXPORTER_SUMMARY_COLUMN = { title: 'Exporter', key: 'exporters', width: 240 }

// 选定具体 exporter 后，后端会把该 exporter 的字段摊平到行上，行内直接可操作。
const OVERVIEW_EXPORTER_DETAIL_COLUMNS = [
  { title: '端口', dataIndex: 'scrape_port', key: 'scrape_port', width: 80 },
  { title: 'Exporter 纳管', key: 'managed_enabled', width: 120 },
  { title: 'Exporter 安装', key: 'install_status', width: 120 },
  { title: '采集状态', key: 'last_scrape_status', width: 100 },
]

const OVERVIEW_FLUENT_BIT_COLUMNS = [
  { title: 'Fluent Bit 状态', key: 'fluent_bit_status', width: 150 },
  { title: 'Fluent Bit 下发', key: 'last_applied_time', width: 170 },
  { title: 'Fluent Bit 错误', key: 'last_error', width: 200 },
]

const OVERVIEW_ACTION_COLUMN = { title: '操作', key: 'action', width: 250, fixed: 'right' }

const overviewColumns = computed(() => [
  ...OVERVIEW_BASE_COLUMNS,
  ...(exporterFilterType.value ? OVERVIEW_EXPORTER_DETAIL_COLUMNS : [OVERVIEW_EXPORTER_SUMMARY_COLUMN]),
  ...OVERVIEW_FLUENT_BIT_COLUMNS,
  OVERVIEW_ACTION_COLUMN,
])
// 列随筛选模式变化，横向滚动宽度跟着算，避免固定值与列对不上导致列挤压。
const overviewScrollX = computed(
  () => overviewColumns.value.reduce((sum, column) => sum + Number(column.width || 160), 0),
)
const exporterFilterOptions = computed(
  () => exporterOptionList.value.map((item) => ({ label: item.name, value: item.name })),
)


const packageColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '类型', dataIndex: 'package_type', key: 'package_type', width: 110 },
  { title: '名称', dataIndex: 'name', key: 'name', width: 150 },
  { title: '版本', dataIndex: 'version', key: 'version', width: 120 },
  { title: '默认端口', dataIndex: 'default_port', key: 'default_port', width: 110 },
  { title: '系统', dataIndex: 'os', key: 'os', width: 100 },
  { title: '架构', dataIndex: 'arch', key: 'arch', width: 100 },
  { title: '包格式', dataIndex: 'package_format', key: 'package_format', width: 100 },
  { title: '平台族', dataIndex: 'platform_family', key: 'platform_family', width: 150 },
  { title: '主版本', dataIndex: 'platform_major', key: 'platform_major', width: 90 },
  { title: '大小', dataIndex: 'size_bytes', key: 'size_bytes', width: 110 },
  { title: 'sha256', dataIndex: 'sha256', key: 'sha256', width: 260, ellipsis: true },
  { title: '同步状态', dataIndex: 'synced', key: 'synced', width: 100 },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: '更新时间', dataIndex: 'update_time', key: 'update_time', width: 180 },
  { title: '操作', key: 'action', width: 250, fixed: 'right' },
]

const packages = ref([])
const packagesLoading = ref(false)
const packageTypeFilter = ref('exporter')
const packageUploadLoading = reactive({})
const packageRowLoading = reactive({})
const packageSyncLoading = reactive({})
const syncModalVisible = ref(false)
const syncModalSubmitting = ref(false)
const syncModalTarget = ref(null)
const syncModalVersion = ref('')
const packageEditModalVisible = ref(false)
const packageEditModalSubmitting = ref(false)
const packageEditTarget = ref(null)
const packageEditForm = reactive({
  default_port: 9100,
  install_playbook_content: '',
  uninstall_playbook_content: '',
  service_file_content: '',
  service_run_as_user: '',
  service_run_as_group: '',
  work_directory: '',
})
const packagePagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showQuickJumper: true,
  showTotal: (total) => `共有 ${total} 条数据`,
})

// 新增软件包：先建“未同步”占位记录（与后端 ensure_defaults 预置行同一语义），再行内上传补全文件。
const packageCreateModalVisible = ref(false)
const packageCreateModalSubmitting = ref(false)
const packageCreateForm = reactive({
  package_type: 'exporter',
  name: '',
  version: '',
  os: 'linux',
  arch: 'amd64',
  package_format: 'tar.gz',
  platform_family: 'any',
  platform_major: '',
  default_port: 9100,
})

const packageTypeOptions = [
  { value: 'exporter', label: 'Exporter' },
  { value: 'fluent_bit', label: 'Fluent Bit' },
]
const packageCreateButtonText = computed(() => (
  packageTypeFilter.value === 'fluent_bit' ? '新增 Fluent Bit 包' : '新增 Exporter 包'
))
const packageNameHint = computed(() => (
  packageCreateForm.package_type === 'fluent_bit'
    ? 'Fluent Bit 使用固定包名；请根据目标系统上传对应的 RPM 或 DEB'
    : '上传文件名需以 Exporter 名称为前缀，如 node_exporter-1.8.2.linux-amd64.tar.gz'
))

const packageFormatOptions = [
  { value: 'tar.gz', label: '通用 tar.gz' },
  { value: 'rpm', label: 'RPM' },
  { value: 'deb', label: 'DEB' },
]
const platformFamilyOptions = computed(() => {
  if (packageCreateForm.package_format === 'rpm') {
    return [{ value: 'rhel', label: 'RHEL / CentOS / Rocky / AlmaLinux' }]
  }
  if (packageCreateForm.package_format === 'deb') {
    return [
      { value: 'ubuntu', label: 'Ubuntu' },
      { value: 'debian', label: 'Debian' },
    ]
  }
  return [{ value: 'any', label: '通用 Linux' }]
})

function handlePackageTypeChange(packageType) {
  if (packageType === 'fluent_bit') {
    packageCreateForm.name = 'fluent-bit'
    packageCreateForm.default_port = 2020
    packageCreateForm.package_format = 'rpm'
    handlePackageFormatChange('rpm')
    return
  }
  packageCreateForm.name = ''
  packageCreateForm.default_port = 9100
  packageCreateForm.package_format = 'tar.gz'
  handlePackageFormatChange('tar.gz')
}

function handlePackageTypeFilterChange() {
  packagePagination.current = 1
  loadPackages()
}

function handlePackageFormatChange(packageFormat) {
  if (packageFormat === 'tar.gz') {
    packageCreateForm.platform_family = 'any'
    packageCreateForm.platform_major = ''
  } else if (packageFormat === 'rpm') {
    packageCreateForm.platform_family = 'rhel'
    packageCreateForm.platform_major = '9'
  } else {
    packageCreateForm.platform_family = 'ubuntu'
    packageCreateForm.platform_major = '22'
  }
}

function openPackageCreateModal() {
  packageCreateForm.package_type = packageTypeFilter.value
  packageCreateForm.version = ''
  packageCreateForm.os = 'linux'
  packageCreateForm.arch = 'amd64'
  handlePackageTypeChange(packageTypeFilter.value)
  packageCreateModalVisible.value = true
}

async function submitPackageCreate() {
  const name = String(packageCreateForm.name || '').trim()
  if (!name) {
    message.error('请填写软件包名称（如 fluent-bit）')
    return
  }
  if (packageCreateForm.package_format !== 'tar.gz' && !String(packageCreateForm.platform_major || '').trim()) {
    message.error('RPM/DEB 必须填写适用平台主版本')
    return
  }
  packageCreateModalSubmitting.value = true
  try {
    await createSoftwarePackage({
      package_type: packageCreateForm.package_type,
      name,
      // 占位记录的 version 仅用于满足唯一约束，行内上传时会按文件名自动识别并覆盖
      version: String(packageCreateForm.version || '').trim() || '0.0.0',
      os: packageCreateForm.os,
      arch: packageCreateForm.arch,
      package_format: packageCreateForm.package_format,
      platform_family: packageCreateForm.platform_family,
      platform_major: packageCreateForm.package_format === 'tar.gz'
        ? ''
        : String(packageCreateForm.platform_major || '').trim(),
      default_port: Number(packageCreateForm.default_port || 9100),
      service_run_as_user: 'dj-agent',
    })
    message.success('已创建占位记录，请在行内上传软件包文件')
    packageCreateModalVisible.value = false
    await loadPackages()
  } catch (error) {
    message.error(resolvePackageErrorMessage(error, '创建失败'))
  } finally {
    packageCreateModalSubmitting.value = false
  }
}

const tsdbTopColumns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 280 },
  { title: '数值', dataIndex: 'value', key: 'value', width: 120 },
]

const promFlagsColumns = [
  { title: '参数', dataIndex: 'name', key: 'name', width: 360 },
  { title: '值', dataIndex: 'value', key: 'value', width: 920 },
]

const tsdbHeadStats = computed(() => {
  const headStats = tsdbStatus.value?.headStats
  return headStats && typeof headStats === 'object' ? headStats : {}
})

const tsdbSeriesByMetric = computed(() => {
  const rows = tsdbStatus.value?.seriesCountByMetricName
  return normalizeTsdbTopRows(rows)
})

const tsdbLabelValueCount = computed(() => {
  const rows = tsdbStatus.value?.labelValueCountByLabelName
  return normalizeTsdbTopRows(rows)
})

const tsdbMemoryByLabel = computed(() => {
  const rows = tsdbStatus.value?.memoryInBytesByLabelName
  return normalizeTsdbTopRows(rows)
})

const tsdbSeriesByLabelValuePair = computed(() => {
  const rows = tsdbStatus.value?.seriesCountByLabelValuePair
  return normalizeTsdbTopRows(rows)
})

const filteredPromFlagsRows = computed(() => {
  const keyword = String(promFlagsKeyword.value || '').trim().toLowerCase()
  if (!keyword) return promFlagsRows.value
  return promFlagsRows.value.filter((row) => {
    const name = String(row?.name || '').toLowerCase()
    const value = String(row?.value || '').toLowerCase()
    return name.includes(keyword) || value.includes(keyword)
  })
})

function formatSize(bytes) {
  const value = Number(bytes || 0)
  if (value <= 0) return '-'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

// 统一提取接口错误提示：优先取后端 {code,msg,data} 中的 msg，404 单独提示为“记录不存在”
function resolvePackageErrorMessage(error, fallback) {
  if (Number(error?.response?.status) === 404) {
    return '记录不存在，可能已被删除，列表已刷新'
  }
  return error?.response?.data?.msg || error?.message || fallback
}

function escapeRegex(value) {
  return String(value || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// 按当前仓库记录解析官方包名，避免 RPM 的 name/version/release 连字符产生歧义。
function parsePackageFilename(filename, record) {
  const value = String(filename || '')
  const expectedName = String(record?.name || '').toLowerCase()
  if (record?.package_format === 'deb') {
    const match = new RegExp(`^${escapeRegex(expectedName)}_([^_]+)_(amd64|arm64)\\.deb$`, 'i').exec(value)
    return match ? { name: expectedName, version: match[1], os: 'linux', arch: match[2].toLowerCase() } : null
  }
  if (record?.package_format === 'rpm') {
    const match = new RegExp(`^${escapeRegex(expectedName)}-([0-9][A-Za-z0-9.+~_-]*?)-[A-Za-z0-9.+~_-]+\\.(x86_64|aarch64)\\.rpm$`, 'i').exec(value)
    if (!match) return null
    return {
      name: expectedName,
      version: match[1],
      os: 'linux',
      arch: match[2].toLowerCase() === 'x86_64' ? 'amd64' : 'arm64',
    }
  }
  const match = /^([a-z0-9][a-z0-9_-]*)-([A-Za-z0-9][A-Za-z0-9.+-]*)\.([a-z0-9]+)-([a-z0-9]+)\.tar\.gz$/i.exec(value)
  return match ? {
    name: match[1].toLowerCase(),
    version: match[2],
    os: match[3].toLowerCase(),
    arch: match[4].toLowerCase(),
  } : null
}

async function loadPackages() {
  packagesLoading.value = true
  try {
    const res = await getSoftwarePackages({
      package_type: packageTypeFilter.value,
      page: packagePagination.current,
      page_size: packagePagination.pageSize,
      ordering: '-id',
    })
    const data = parseApiData(res)
    packages.value = Array.isArray(data.results) ? data.results : []
    packagePagination.total = Number(data.count || 0)
  } catch (error) {
    message.warning(error?.message || '加载软件仓库失败')
  } finally {
    packagesLoading.value = false
  }
}

// 首次打开软件仓库页面时，默认软件包由后端一次性数据迁移预置，前端不再自动调用预置接口，
// 避免用户删除后因每次进页自动重建而“删不掉”。

function openSyncOfficialModal(record) {
  syncModalTarget.value = record
  syncModalVersion.value = String(record.version || '')
  syncModalVisible.value = true
}

function openPackageEditModal(record) {
  packageEditTarget.value = record
  packageEditForm.default_port = Number(record.default_port || 9100)
  // 安装/卸载 Playbook 内容直接来自后端 to_representation 补充的 install/uninstall_playbook_content
  // （实际存放在关联 PlaybookTemplate.content 上，这里只是展示成对内联编辑，不再要求先去模板页选择）
  packageEditForm.install_playbook_content = record.install_playbook_content || ''
  packageEditForm.uninstall_playbook_content = record.uninstall_playbook_content || ''
  packageEditForm.service_file_content = record.service_file_content || ''
  // 历史记录可能仍是迁移前的空值，这里兜底回填默认账号 dj-agent，与后端模型默认值保持一致
  packageEditForm.service_run_as_user = record.service_run_as_user || 'dj-agent'
  packageEditForm.service_run_as_group = record.service_run_as_group || 'dj-agent'
  packageEditForm.work_directory = record.work_directory || '/tmp'
  packageEditModalVisible.value = true
}

async function submitPackageEdit() {
  const record = packageEditTarget.value
  if (!record) return
  // 服务运行用户为必填项，前端提前拦截空值，避免打到后端才因 allow_blank=False 报错
  if (!packageEditForm.service_run_as_user.trim()) {
    message.error('服务运行用户为必填项')
    return
  }
  const defaultPort = Number(packageEditForm.default_port || 0)
  if (!Number.isInteger(defaultPort) || defaultPort < 1 || defaultPort > 65535) {
    message.error('默认监控端口必须是 1-65535 的整数')
    return
  }
  packageEditModalSubmitting.value = true
  try {
    await updateSoftwarePackage(record.id, {
      default_port: defaultPort,
      install_playbook_content: packageEditForm.install_playbook_content,
      uninstall_playbook_content: packageEditForm.uninstall_playbook_content,
      service_file_content: packageEditForm.service_file_content,
      service_run_as_user: packageEditForm.service_run_as_user,
      service_run_as_group: packageEditForm.service_run_as_group,
      work_directory: packageEditForm.work_directory,
    })
    message.success('保存成功')
    packageEditModalVisible.value = false
    await loadPackages()
  } catch (error) {
    message.error(resolvePackageErrorMessage(error, '保存失败'))
  } finally {
    packageEditModalSubmitting.value = false
  }
}

async function submitSyncOfficial() {
  const record = syncModalTarget.value
  const version = syncModalVersion.value.trim()
  if (!record || !version) {
    message.error('请输入目标版本')
    return
  }
  syncModalSubmitting.value = true
  packageSyncLoading[record.id] = true
  try {
    await syncSoftwarePackageFromOfficial(record.id, version)
    message.success('自动更新成功')
    syncModalVisible.value = false
  } catch (error) {
    message.error(resolvePackageErrorMessage(error, '自动更新失败'))
  } finally {
    syncModalSubmitting.value = false
    packageSyncLoading[record.id] = false
    // 无论成功失败都刷新列表，清除本地过期数据（如记录已被删除导致 404）
    await loadPackages()
  }
}

function handlePackageTableChange(pagination) {
  packagePagination.current = Number(pagination?.current || 1)
  packagePagination.pageSize = Number(pagination?.pageSize || 10)
  loadPackages()
}

function beforePackageUpload(file, record) {
  const filename = String(file?.name || '')
  if (!['.tar.gz', '.rpm', '.deb'].some((suffix) => filename.toLowerCase().endsWith(suffix))) {
    message.error('仅支持上传 .tar.gz / .rpm / .deb 软件包')
    return false
  }
  const parsed = parsePackageFilename(filename, record)
  if (!parsed) {
    message.error(`文件名与 ${record.package_format} 官方命名或当前记录名称不匹配`)
    return false
  }
  // 前端提前校验名称与架构匹配，避免无谓上传后被后端拒绝
  if (parsed.name !== String(record.name || '').toLowerCase()) {
    message.error(`文件名称前缀（${parsed.name}）与当前记录（${record.name}）不一致`)
    return false
  }
  if (parsed.os !== record.os || parsed.arch !== record.arch) {
    message.error(`文件架构（${parsed.os}-${parsed.arch}）与当前记录（${record.os}-${record.arch}）不一致`)
    return false
  }
  return true
}

async function handlePackageUpload(options, record) {
  const file = options?.file
  const parsed = parsePackageFilename(file?.name, record)
  if (!parsed) {
    options?.onError?.(new Error('invalid filename'))
    return
  }
  const formData = new FormData()
  formData.append('file', file)
  packageUploadLoading[record.id] = true
  try {
    await uploadSoftwarePackageFile(record.id, formData)
    message.success('软件包上传成功')
    await loadPackages()
    options?.onSuccess?.(null, file)
  } catch (error) {
    message.error(resolvePackageErrorMessage(error, '软件包上传失败'))
    options?.onError?.(error)
  } finally {
    packageUploadLoading[record.id] = false
  }
}

function openPackageDeleteConfirm(record) {
  openDeleteConfirm({
    title: '确认删除软件包',
    summary: '删除后将无法从本地仓库下发该软件包，agent 端安装会回退到官方下载源。',
    items: [`${record.name} ${record.version} (${record.os}-${record.arch})`],
    onConfirm: async () => {
      packageRowLoading[record.id] = true
      try {
        await deleteSoftwarePackage(record.id)
        message.success('删除成功')
      } catch (error) {
        // 删除失败（如记录已被其他会话删除导致 404）时只提示，不向上抛出，避免弹窗因 Promise 拒绝卡住/控制台报 Uncaught rejection
        message.error(resolvePackageErrorMessage(error, '删除失败'))
      } finally {
        packageRowLoading[record.id] = false
        // 无论成功失败都刷新列表，清除本地过期数据
        await loadPackages()
      }
    },
  })
}

function statusColor(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'success') return 'green'
  if (value === 'failed') return 'red'
  if (value === 'pending') return 'orange'
  return 'default'
}

function scrapeColor(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'up') return 'green'
  if (value === 'down') return 'red'
  return 'default'
}

function parseApiData(resp) {
  return resp?.data?.data || {}
}

async function loadAlertSettings() {
  alertSettingsLoading.value = true
  try {
    const data = parseApiData(await getConfigByKey(CONFIG_KEYS.ALERT_HISTORY_RETENTION_DAYS))
    alertHistoryRetentionDays.value = Number(data.value) || 90
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '加载告警设置失败')
  } finally {
    alertSettingsLoading.value = false
  }
}

async function saveAlertSettings() {
  const retentionDays = Number(alertHistoryRetentionDays.value)
  if (!Number.isInteger(retentionDays) || retentionDays < 1) {
    message.warning('历史告警保留天数必须是大于等于 1 的整数')
    return
  }
  alertSettingsSaving.value = true
  try {
    await updateConfigByKey(CONFIG_KEYS.ALERT_HISTORY_RETENTION_DAYS, { value: retentionDays })
    message.success('告警设置已保存')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '保存告警设置失败')
  } finally {
    alertSettingsSaving.value = false
  }
}

function formatTsdbTime(rawValue) {
  const value = Number(rawValue)
  if (!Number.isFinite(value) || value <= 0) return '-'
  const ts = value > 100000000000 ? value : value * 1000
  return formatTimeWithTimezone(ts, store.state.user?.timezone || 'Asia/Shanghai')
}

function pickTsdbName(item) {
  if (item === null || item === undefined) return '-'
  if (Array.isArray(item)) return String(item[0] ?? '-')
  if (typeof item !== 'object') return String(item)
  const candidate = item.name ?? item.labelName ?? item.label ?? item.metric ?? item.pair ?? item.key
  if (candidate !== undefined && candidate !== null && String(candidate) !== '') {
    return String(candidate)
  }
  const firstStringField = Object.values(item).find((v) => typeof v === 'string' && v)
  return firstStringField ? String(firstStringField) : '-'
}

function pickTsdbValue(item) {
  if (item === null || item === undefined) return '-'
  if (Array.isArray(item)) return item[1] ?? '-'
  if (typeof item !== 'object') return '-'
  const candidate = item.value ?? item.count ?? item.memoryInBytes ?? item.series ?? item.numSeries ?? item.bytes
  if (candidate !== undefined && candidate !== null && candidate !== '') {
    return candidate
  }
  const firstNumberField = Object.values(item).find((v) => typeof v === 'number')
  return firstNumberField ?? '-'
}

function normalizeTsdbTopRows(rows) {
  if (!Array.isArray(rows)) return []
  return rows.map((item, index) => ({
    name: pickTsdbName(item),
    value: pickTsdbValue(item),
    _idx: index,
  }))
}

async function loadMonitorSummary() {
  const res = await getMonitorSummary()
  const data = parseApiData(res)
  const targets = data.targets || {}
  return {
    total: Number(targets.total || 0),
    managedEnabled: Number(targets.managed_enabled || 0),
    scrapeUp: Number(targets.scrape_up || 0),
  }
}

async function loadPromOverview() {
  const res = await getPrometheusOverview()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus overview 查询失败')
  }
  const targets = data.targets || {}
  return {
    baseUrl: String(data.prometheus_base_url || ''),
    total: Number(targets.total || 0),
    up: Number(targets.up || 0),
    down: Number(targets.down || 0),
  }
}

async function loadPromTargets() {
  const res = await getPrometheusTargets()
  const data = parseApiData(res)
  if (String(data.status || '').toLowerCase() === 'error') {
    throw new Error(data.error || 'Prometheus targets 查询失败')
  }
  return Array.isArray(data.results) ? data.results : []
}

async function loadPromTsdbStatus() {
  try {
    const res = await getPrometheusTsdbStatus()
    const data = parseApiData(res)
    if (String(data.status || '').toLowerCase() === 'error') {
      return {
        result: {},
        error: data.error || 'Prometheus TSDB 状态查询失败',
      }
    }
    return {
      result: data.result && typeof data.result === 'object' ? data.result : {},
      error: '',
    }
  } catch (error) {
    return {
      result: {},
      error: error?.response?.data?.msg || error?.message || 'Prometheus TSDB 状态查询失败',
    }
  }
}

async function loadPromConfig() {
  try {
    const res = await getPrometheusConfig()
    const data = parseApiData(res)
    if (String(data.status || '').toLowerCase() === 'error') {
      return {
        yaml: '',
        error: data.error || 'Prometheus Config 查询失败',
      }
    }
    const yaml = String(data.config_yaml || data.result?.yaml || '')
    return {
      yaml,
      error: '',
    }
  } catch (error) {
    return {
      yaml: '',
      error: error?.response?.data?.msg || error?.message || 'Prometheus Config 查询失败',
    }
  }
}

async function loadPromFlags() {
  try {
    const res = await getPrometheusFlags()
    const data = parseApiData(res)
    if (String(data.status || '').toLowerCase() === 'error') {
      return {
        rows: [],
        error: data.error || 'Prometheus 启动参数查询失败',
      }
    }

    const result = data.result && typeof data.result === 'object' ? data.result : {}
    const rows = Object.entries(result).map(([key, value]) => ({
      name: String(key || ''),
      value: value === null || value === undefined ? '' : String(value),
    }))
    rows.sort((a, b) => a.name.localeCompare(b.name))
    return {
      rows,
      error: '',
    }
  } catch (error) {
    return {
      rows: [],
      error: error?.response?.data?.msg || error?.message || 'Prometheus 启动参数查询失败',
    }
  }
}

async function copyPromConfig() {
  const text = String(promConfigYaml.value || '')
  if (!text) {
    message.warning('暂无可复制的配置内容')
    return
  }

  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      message.success('配置已复制到剪贴板')
      return
    }

    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.left = '-9999px'
    textarea.style.top = '0'
    document.body.appendChild(textarea)
    textarea.focus()
    textarea.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(textarea)
    if (ok) {
      message.success('配置已复制到剪贴板')
      return
    }
    message.error('复制失败，请手动复制')
  } catch (_error) {
    message.error('复制失败，请手动复制')
  }
}


function collectGroupKeys(nodes) {
  const keys = []
  const walk = (items) => {
    ;(Array.isArray(items) ? items : []).forEach((node) => {
      keys.push(node.key)
      walk(node.children)
    })
  }
  walk(nodes)
  return keys
}

function buildMonitorGroupTreeData(groups, keyword, totals) {
  const kw = String(keyword || '').trim().toLowerCase()
  const build = (nodes) => (Array.isArray(nodes) ? nodes : []).reduce((rows, node) => {
    const children = build(node.children)
    const matched = !kw || String(node.name || '').toLowerCase().includes(kw)
    if (matched || children.length) {
      rows.push({
        key: `group-${node.id}`,
        title: `${node.name}（${node.managed_count}/${node.host_count}）`,
        children,
      })
    }
    return rows
  }, [])
  return [{
    key: 'all',
    title: `全部主机（${totals.managed}/${totals.total}）`,
    children: build(groups),
  }]
}

const overviewGroupTreeData = computed(
  () => buildMonitorGroupTreeData(overviewGroupTree.value, overviewGroupKeyword.value, overviewGroupTotals),
)

function handleOverviewGroupSelect(keys) {
  // 点已选中的节点时 antd 会回传空数组，这里保持原选中，避免过滤条件被意外清空。
  overviewSelectedGroupKeys.value = keys.length ? keys : overviewSelectedGroupKeys.value
  reloadOverviewHosts()
}

function fluentBitStatusColor(record) {
  if (!record || !record.managed) return 'default'
  if (record.install_status === 'pending') return 'processing'
  if (record.install_status === 'failed') return 'error'
  if (record.agent_installed) {
    if (record.runtime_status === 'running') return 'success'
    if (record.runtime_status === 'stopped') return 'warning'
    if (record.runtime_status === 'error') return 'error'
    return 'success'
  }
  return 'default'
}

function fluentBitStatusText(record) {
  if (!record || !record.managed) return '未安装'
  if (record.install_status === 'pending') return '安装中'
  if (record.install_status === 'failed') return '安装失败'
  if (record.agent_installed) {
    if (record.runtime_status === 'running') return '运行中 (2020)'
    if (record.runtime_status === 'stopped') return '已停止'
    if (record.runtime_status === 'error') return '异常'
    return '已安装'
  }
  return '未安装'
}

function fluentBitStatusTooltip(record) {
  if (!record || !record.managed) return '未纳管 Fluent Bit 日志采集'
  if (record.install_status === 'pending') return '任务执行中，请稍候'
  if (record.install_status === 'failed') return record.last_error || '安装失败，请点击重试'
  if (record.agent_installed) {
    if (record.runtime_status === 'running') return 'Fluent Bit 正常运行 (HTTP API: 2020)'
    if (record.runtime_status === 'stopped') return 'Fluent Bit 服务已停止'
    if (record.runtime_status === 'error') return record.last_error || 'Fluent Bit 运行异常'
  }
  return ''
}

function exporterActionTooltip(record) {
  if (!record.host_agent_online) return 'dj-agent 离线，操作不可用'
  if (!exporterFilterType.value) return '纳管并安装 Exporter（先在上方选定具体 Exporter 才能重装/启停）'
  return `纳管并安装 ${exporterFilterType.value}`
}

async function loadOverviewHosts() {
  overviewLoading.value = true
  try {
    const groupKey = overviewSelectedGroupKeys.value[0]
    const data = parseApiData(await getMonitorHostOverview({
      page: overviewPagination.current,
      page_size: overviewPagination.pageSize,
      group_id: String(groupKey || '').startsWith('group-') ? String(groupKey).slice('group-'.length) : undefined,
      search: overviewKeyword.value.trim() || undefined,
      exporter_type: exporterFilterType.value || undefined,
      exporter_managed: overviewManagedFilter.value || undefined,
      fluent_bit_managed: fluentBitManagedFilter.value || undefined,
    }))
    overviewHosts.value = Array.isArray(data.results) ? data.results : []
    overviewPagination.total = Number(data.count || 0)
  } finally {
    overviewLoading.value = false
  }
}

async function loadOverviewGroupTree() {
  try {
    const data = parseApiData(await getMonitorHostGroupTree())
    overviewGroupTree.value = Array.isArray(data.groups) ? data.groups : []
    overviewGroupTotals.total = Number(data.total_host_count || 0)
    overviewGroupTotals.managed = Number(data.total_managed_count || 0)
    overviewGroupExpandedKeys.value = collectGroupKeys(overviewGroupTreeData.value)
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '主机分组树加载失败')
  }
}

async function loadExporterOptions() {
  try {
    const data = parseApiData(await getMonitorExporterOptions())
    exporterOptionList.value = Array.isArray(data) ? data : []
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Exporter 列表加载失败')
  }
}

function reloadOverviewHosts() {
  overviewPagination.current = 1
  overviewSelectedHostIds.value = []
  loadOverviewHosts()
}

function handleOverviewTableChange(pagination) {
  overviewPagination.current = Number(pagination?.current || 1)
  overviewPagination.pageSize = Number(pagination?.pageSize || 10)
  // 勾选态只对当前页有效，翻页后不清会把上一页的 id 带进批量请求。
  overviewSelectedHostIds.value = []
  loadOverviewHosts()
}

const overviewRowSelection = computed(() => ({
  selectedRowKeys: overviewSelectedHostIds.value,
  onChange: (keys) => {
    overviewSelectedHostIds.value = keys
  },
}))

function exporterTagColor(item) {
  if (!item.managed_enabled) return 'default'
  return ({ success: 'green', failed: 'red', pending: 'blue' })[item.install_status] || 'orange'
}

function openExporterCreateModal(record) {
  // 行内按钮传 record，工具栏按钮不传参（事件对象不是行数据，用 host_id 判断）。
  const hostIds = record?.host_id ? [record.host_id] : [...overviewSelectedHostIds.value]
  if (!hostIds.length) {
    return
  }
  exporterCreateHostIds.value = hostIds
  // 已在上方筛选具体 exporter 时直接预选它，避免用户再选一次。
  const preferred = exporterOptionList.value.find((item) => item.name === exporterFilterType.value)
  const target = preferred || exporterOptionList.value[0]
  exporterCreateForm.exporter_type = target?.name
  exporterCreateForm.scrape_port = Number(target?.default_port || 9100)
  exporterCreateModalVisible.value = true
}

function handleExporterTypeChange(value) {
  const matched = exporterOptionList.value.find((item) => item.name === value)
  if (matched) {
    exporterCreateForm.scrape_port = Number(matched.default_port || 9100)
  }
}

async function submitExporterCreate() {
  if (!exporterCreateForm.exporter_type) {
    message.error('请选择 Exporter')
    return
  }
  const port = Number(exporterCreateForm.scrape_port)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    message.error('抓取端口必须是 1-65535 之间的整数')
    return
  }
  exporterCreateSubmitting.value = true
  try {
    const data = parseApiData(await batchCreateMonitorTargets({
      host_ids: exporterCreateHostIds.value,
      exporter_type: exporterCreateForm.exporter_type,
      scrape_port: port,
      install_now: true,
    }))
    reportFluentBitBatchResult('纳管并下发安装', data)
    exporterCreateModalVisible.value = false
    overviewSelectedHostIds.value = []
    await Promise.all([loadOverviewHosts(), loadOverviewGroupTree()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '纳管失败')
  } finally {
    exporterCreateSubmitting.value = false
  }
}

// 选中项按 host_id 存；Fluent Bit 批量动作要按「已纳管/未纳管」拆成两条链路，前者用 target id。
const overviewSelectedRows = computed(
  () => overviewHosts.value.filter((item) => overviewSelectedHostIds.value.includes(item.host_id)),
)
const fluentBitSelectedManagedIds = computed(
  () => overviewSelectedRows.value.filter((item) => item.fluent_bit.managed).map((item) => item.fluent_bit.id),
)
const fluentBitSelectedUnmanaged = computed(
  () => overviewSelectedRows.value.filter((item) => !item.fluent_bit.managed),
)

const FLUENT_BIT_BATCH_ACTIONS = {
  retry: { label: '批量下发安装', request: batchRetryLogCollectionTargets },
  start: { label: '批量启动', request: batchStartLogCollectionTargets },
  stop: { label: '批量停止', request: batchStopLogCollectionTargets },
  apply: { label: '批量下发配置', request: batchApplyLogCollectionTargets },
  delete: { label: '批量删除', request: batchDeleteLogCollectionTargets },
}

async function handleFluentBitBatch(action) {
  const config = FLUENT_BIT_BATCH_ACTIONS[action]
  const ids = [...fluentBitSelectedManagedIds.value]
  if (!config || !ids.length) {
    return
  }
  fluentBitBatchLoading.value = action
  try {
    const data = parseApiData(await config.request(ids))
    reportFluentBitBatchResult(config.label, data)
    overviewSelectedHostIds.value = []
    await Promise.all([loadOverviewHosts(), loadOverviewGroupTree()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || `${config.label}失败`)
  } finally {
    fluentBitBatchLoading.value = ''
  }
}

function reportFluentBitBatchResult(label, data) {
  const failed = Array.isArray(data.results) ? data.results.filter((item) => !item.ok) : []
  if (failed.length === 0) {
    message.success(`${label}成功：${data.success} 台`)
    return
  }
  // 逐台执行，部分失败是常态；把失败主机和原因摊开，避免只报一个笼统错误。
  const detail = failed.slice(0, 3).map((item) => `${item.host}：${item.message}`).join('；')
  const suffix = failed.length > 3 ? ` 等 ${failed.length} 台` : ''
  message.warning(`${label}完成：成功 ${data.success} 台，失败 ${data.failed} 台。${detail}${suffix}`)
}

async function handleFluentBitBatchCreate() {
  const hostIds = fluentBitSelectedUnmanaged.value.map((item) => item.host_id)
  if (!hostIds.length) {
    return
  }
  fluentBitBatchLoading.value = 'create'
  try {
    const data = parseApiData(await batchCreateLogCollectionTargets(hostIds, true))
    reportFluentBitBatchResult('纳管并下发安装', data)
    overviewSelectedHostIds.value = []
    await Promise.all([loadOverviewHosts(), loadOverviewGroupTree()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '纳管失败')
  } finally {
    fluentBitBatchLoading.value = ''
  }
}

async function handleFluentBitCreateOne(record) {
  fluentBitCreateLoading[record.host_id] = true
  try {
    const data = parseApiData(await batchCreateLogCollectionTargets([record.host_id], true))
    reportFluentBitBatchResult('纳管并下发安装', data)
    await Promise.all([loadOverviewHosts(), loadOverviewGroupTree()])
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '纳管失败')
  } finally {
    fluentBitCreateLoading[record.host_id] = false
  }
}

function openFluentBitBatchDeleteConfirm() {
  const selected = overviewSelectedRows.value.filter((item) => item.fluent_bit.managed)
  if (!selected.length) {
    return
  }
  openDeleteConfirm({
    title: '确认批量删除 Fluent Bit 目标',
    summary: '已安装的主机会先下发卸载任务，卸载成功后自动删除纳管记录；卸载失败则保留记录和安装日志。',
    items: selected.map((item) => `${item.host_name || item.host_ip || `Host-${item.host_id}`} - Fluent Bit`),
    onConfirm: () => handleFluentBitBatch('delete'),
  })
}

function canApplyFluentBitConfig(record) {
  return Boolean(record?.host_agent_online)
    && Boolean(record?.agent_installed)
    && record?.runtime_status === 'running'
}

function fluentBitApplyTooltip(record) {
  if (!record?.host_agent_online) return 'dj-agent 离线，操作不可用'
  if (!record?.agent_installed) return 'Fluent Bit 未安装，请先完成离线安装'
  if (record?.runtime_status !== 'running') return 'Fluent Bit 未运行，请先启动服务'
  return '运行'
}

function formatManagedTargetTime(value) {
  if (!value) return '-'
  return formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai')
}

async function handleCheckFluentBitStatus(record) {
  fluentBitStatusLoading[record.id] = true
  try {
    await checkLogCollectionStatus(record.id)
    message.success('Fluent Bit 状态已更新')
    await loadOverviewHosts()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Fluent Bit 状态检查失败')
  } finally {
    fluentBitStatusLoading[record.id] = false
  }
}

async function handleApplyFluentBitConfig(record) {
  fluentBitApplyLoading[record.id] = true
  try {
    const result = parseApiData(await applyLogCollectionConfig(record.id))
    message.success(result?.skipped ? '配置未变化，无需重复下发' : 'Fluent Bit 配置已下发')
    await loadOverviewHosts()
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Fluent Bit 配置下发失败')
  } finally {
    fluentBitApplyLoading[record.id] = false
  }
}

async function handleFluentBitRedispatch(record) {
  fluentBitRetryLoading[record.id] = true
  try {
    await retryLogCollectionTarget(record.id)
    message.success('已下发 Fluent Bit 重新安装任务，请稍后刷新查看结果')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Fluent Bit 重新安装失败')
  } finally {
    fluentBitRetryLoading[record.id] = false
    await loadOverviewHosts()
  }
}

async function openFluentBitRetryConfirm(record) {
  if (!record?.host_agent_online) return
  const hostLabel = record.host_name || record.host_ip || String(record.host || '-')
  await openDeleteConfirm({
    title: '确认重新安装',
    okText: '确认',
    summary: '将从 djadmin 本地仓库选择与目标系统精确匹配的 RPM/DEB 包并重新安装。',
    items: [`${hostLabel} - Fluent Bit`],
    onConfirm: () => {
      void handleFluentBitRedispatch(record)
    },
  })
}

async function openFluentBitJobLog(record) {
  let latestHistoryId = null
  try {
    const historyRes = await getMonitorInstallHistoryList({
      page: 1,
      page_size: 1,
      ordering: '-id',
      log_collection_target_id: String(record.id),
    })
    const historyData = parseApiData(historyRes)
    const rows = Array.isArray(historyData?.results) ? historyData.results : []
    const latestId = Number(rows[0]?.id)
    if (Number.isInteger(latestId) && latestId > 0) latestHistoryId = latestId
  } catch (_error) {
    // 历史页仍可按 Fluent Bit 目标 ID 打开，详情查询失败不阻断入口。
  }
  const query = {
    tab: 'monitor_history',
    log_collection_target_id: String(record.id),
    keyword: String(record.host_ip || record.host_name || ''),
  }
  if (latestHistoryId) query.history_id = String(latestHistoryId)
  router.push({ path: '/sys/automation/logs', query })
}

async function handleStartFluentBitService(record) {
  fluentBitStartLoading[record.id] = true
  try {
    const result = parseApiData(await startLogCollectionService(record.id))
    if (result?.status === 'success' && result?.exit_code === 0) {
      message.success('Fluent Bit 启动成功')
    } else {
      message.error(`Fluent Bit 启动失败：${result?.stderr || result?.error_message || result?.status}`)
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Fluent Bit 启动失败')
  } finally {
    fluentBitStartLoading[record.id] = false
    await loadOverviewHosts()
  }
}

async function handleStopFluentBitService(record) {
  fluentBitStopLoading[record.id] = true
  try {
    const result = parseApiData(await stopLogCollectionService(record.id))
    if (result?.status === 'success' && result?.exit_code === 0) {
      message.success('Fluent Bit 停止成功')
    } else {
      message.error(`Fluent Bit 停止失败：${result?.stderr || result?.error_message || result?.status}`)
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || 'Fluent Bit 停止失败')
  } finally {
    fluentBitStopLoading[record.id] = false
    await loadOverviewHosts()
  }
}

function canCancelFluentBitTarget(record) {
  return ['pending', 'running'].includes(String(record?.install_status || '').toLowerCase())
}

async function handleCancelFluentBitTarget(record) {
  if (!canCancelFluentBitTarget(record)) return
  fluentBitCancelLoading[record.id] = true
  try {
    await cancelLogCollectionTarget(record.id)
    message.success('任务已取消')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '取消任务失败')
  } finally {
    fluentBitCancelLoading[record.id] = false
    await loadOverviewHosts()
  }
}

function openFluentBitDeleteConfirm(record) {
  const hostLabel = record.host_name || record.host_ip || String(record.host || '-')
  openDeleteConfirm({
    title: '确认删除 Fluent Bit 目标',
    summary: record.agent_installed
      ? '会先下发卸载任务，卸载成功后自动删除纳管记录；卸载失败则保留记录和安装日志。'
      : '目标删除后，如需继续采集日志必须重新创建并安装。',
    items: [`${hostLabel} - Fluent Bit`],
    onConfirm: async () => {
      fluentBitDeleteLoading[record.id] = true
      try {
        const data = parseApiData(await deleteLogCollectionTarget(record.id))
        message.success(data?.pending_uninstall
          ? '已下发卸载任务，卸载成功后自动删除'
          : 'Fluent Bit 目标已删除')
      } catch (error) {
        message.error(error?.response?.data?.msg || error?.message || 'Fluent Bit 目标删除失败')
      } finally {
        fluentBitDeleteLoading[record.id] = false
        await loadOverviewHosts()
      }
    },
  })
}



function isManagedTargetActionDisabledByAgent(record) {
  return !Boolean(record?.host_agent_online)
}

async function loadAllData() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [summaryPayload, overviewPayload, targetsPayload, tsdbPayload, configPayload, flagsPayload] = await Promise.all([
      loadMonitorSummary(),
      loadPromOverview(),
      loadPromTargets(),
      loadPromTsdbStatus(),
      loadPromConfig(),
      loadPromFlags(),
    ])

    overview.total = overviewPayload.total
    overview.up = overviewPayload.up
    overview.down = overviewPayload.down
    prometheusBaseUrl.value = overviewPayload.baseUrl

    promTargets.value = targetsPayload
    tsdbStatus.value = tsdbPayload.result
    tsdbLoadError.value = tsdbPayload.error
    promConfigYaml.value = configPayload.yaml
    promConfigLoadError.value = configPayload.error
    promFlagsRows.value = flagsPayload.rows
    promFlagsLoadError.value = flagsPayload.error

    await Promise.all([
      loadOverviewHosts(),
      loadOverviewGroupTree(),
      loadExporterOptions(),
    ])
    if (overview.total <= 0 && summaryPayload.total > 0) {
      overview.total = summaryPayload.total
      overview.up = summaryPayload.scrapeUp
      overview.down = Math.max(0, summaryPayload.total - summaryPayload.scrapeUp)
    }
    lastRefreshAt.value = new Date()
  } catch (error) {
    const msg = error?.message || '加载监控数据失败'
    errorMessage.value = msg
    message.warning(msg)
  } finally {
    loading.value = false
  }
}

function clearRefreshTimer() {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
}

function restartRefreshTimer() {
  clearRefreshTimer()
  if (!autoRefreshEnabled.value) {
    return
  }
  const intervalMs = Number(refreshIntervalSeconds.value || 15) * 1000
  refreshTimer = window.setInterval(() => {
    if (loading.value) return
    loadAllData()
  }, intervalMs)
}



// 重新下发：统一入口，无论当前 install_status 是什么（success/failed/pending）都可以点。
// 后端 retry 接口本身只看 managed_enabled 决定装还是卸（不校验 install_status），
// 这里去掉前端原来“必须 install_status===failed 才能点”的限制，用一个按钮同时覆盖
// “重试失败的任务”和“修复历史遗留问题需要重新执行一次已成功的任务”（如 unit 文件里
// User/Group 是旧版本 Playbook 生成的、需要用最新 Playbook 重新生成）两种场景。
async function handleManagedRedispatch(record) {
  managedRetryLoading[record.id] = true
  try {
    const res = await retryManagedTarget(record.id)
    const latest = parseApiData(res)
    // 立即同步后端返回的新 job_id，避免列表刷新前仍指向旧任务。
    if (latest && typeof latest === 'object') {
      Object.assign(record, latest)
    }
    message.success('已重新下发任务')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '重新下发失败')
  } finally {
    managedRetryLoading[record.id] = false
    await loadOverviewHosts()
  }
}

async function openManagedTargetRetryConfirm(record) {
  if (isManagedTargetActionDisabledByAgent(record)) {
    return
  }
  const isInstall = Boolean(record?.managed_enabled)
  const actionText = isInstall ? '重新安装' : '重新卸载'
  const summary = isInstall
    ? '将重新下发安装任务（无论当前安装状态是否已成功），请确认影响范围。'
    : '将重新下发卸载任务（无论当前安装状态是否已成功），请确认影响范围。'
  const hostLabel = record?.host_name || record?.host_ip || String(record?.host_id || '-')
  await openDeleteConfirm({
    title: `确认${actionText}`,
    okText: '确认',
    summary,
    items: [`${hostLabel} - ${record.exporter_type}`],
    onConfirm: () => {
      void handleManagedRedispatch(record)
    },
  })
}

async function onCancelManagedTarget(record) {
  if (!canCancelManagedTarget(record)) {
    return
  }
  managedCancelLoading[record.id] = true
  try {
    await cancelManagedTarget(record.id)
    message.success('任务已取消')
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '取消任务失败')
  } finally {
    managedCancelLoading[record.id] = false
    await loadOverviewHosts()
  }
}

function canCancelManagedTarget(record) {
  const status = String(record?.install_status || '').toLowerCase()
  return status === 'pending' || status === 'running'
}

async function openManagedTargetJobLog(record) {
  if (managedRetryLoading[record?.id]) {
    message.info('任务重新下发中，请稍候再查看日志')
    return
  }

  // 仅以“监控安装历史”作为日志来源：优先打开该目标最新一条历史记录详情。
  let latestHistoryId = null
  try {
    const historyRes = await getMonitorInstallHistoryList({
      page: 1,
      page_size: 1,
      ordering: '-id',
      target_id: String(record.id),
    })
    const historyData = parseApiData(historyRes)
    const rows = Array.isArray(historyData?.results) ? historyData.results : []
    const latestId = Number(rows[0]?.id)
    if (Number.isInteger(latestId) && latestId > 0) {
      latestHistoryId = latestId
    }
  } catch (_error) {
    // 查询最新历史失败时，回退为仅按 target_id 打开历史页签。
  }

  const query = {
    tab: 'monitor_history',
    target_id: String(record.id),
    keyword: String(record.host_ip || record.host_name || ''),
  }
  if (latestHistoryId) {
    query.history_id = String(latestHistoryId)
  }

  router.push({
    path: '/sys/automation/logs',
    query,
  })
}

// 删除操作统一走公共确认弹窗（openDeleteConfirm），禁止自行用 a-popconfirm 拼一套删除确认。
// 前置校验（managed_enabled===false 且 install_status!=='pending'）已由按钮 disabled 状态和后端 destroy() 双重把关，
// 这里的确认弹窗只负责二次确认 + 展示待删除对象，不重复做业务校验。
function openManagedTargetDeleteConfirm(record) {
  openDeleteConfirm({
    title: '确认删除纳管目标',
    summary: '删除后将无法再追踪该主机这项监控的安装/卸载历史记录，如需重新纳管请到主机编辑页重新开启。',
    items: [`${record.host_name || record.host_ip || record.host_id} - ${record.exporter_type}`],
    onConfirm: async () => {
      managedDeleteLoading[record.id] = true
      try {
        await deleteManagedTarget(record.id)
        message.success('删除成功')
      } catch (error) {
        message.error(error?.response?.data?.msg || error?.message || '删除失败')
      } finally {
        managedDeleteLoading[record.id] = false
        await loadOverviewHosts()
      }
    },
  })
}

async function handleCheckServiceStatus(record) {
  managedServiceStatusLoading[record.id] = true
  try {
    const createRes = await checkManagedTargetServiceStatus(record.id)
    const job = parseApiData(createRes)
    if (!job || !job.status) return

    // 写入“服务状态”列缓存（常驻展示），同时保留完整 stdout/stderr 供点击标签时查看详情。
    serviceStatusMap[record.id] = {
      status: job.status,
      exitCode: job.exit_code,
      stdout: job.stdout,
      stderr: job.stderr,
      checkedAt: new Date().toISOString(),
    }
  } catch (error) {
    console.warn('[service_status] 查询运行状态失败', record.id, error?.response?.data?.msg || error?.message)
  } finally {
    managedServiceStatusLoading[record.id] = false
  }
}

async function handleStartService(record) {
  managedStartLoading[record.id] = true
  try {
    const job = parseApiData(await startManagedTargetService(record.id))
    if (job.status === 'success' && job.exit_code === 0) {
      message.success('启动命令已执行成功')
    } else {
      message.error(`启动失败：${job.stderr || job.stdout || job.status}`)
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '启动服务失败')
  } finally {
    managedStartLoading[record.id] = false
    // 启动/停止命令本身的 exit_code 不直接等价于最终服务状态，命令执行完成后主动刷新一次
    // “服务状态”列（真正调用 systemctl status），确保展示的是权威结果而不是猜测。
    handleCheckServiceStatus(record)
  }
}

async function handleStopService(record) {
  managedStopLoading[record.id] = true
  try {
    const job = parseApiData(await stopManagedTargetService(record.id))
    if (job.status === 'success' && job.exit_code === 0) {
      message.success('停止命令已执行成功')
    } else {
      message.error(`停止失败：${job.stderr || job.stdout || job.status}`)
    }
  } catch (error) {
    message.error(error?.response?.data?.msg || error?.message || '停止服务失败')
  } finally {
    managedStopLoading[record.id] = false
    handleCheckServiceStatus(record)
  }
}

function openServiceStatusModal(record) {
  const cached = serviceStatusMap[record.id]
  if (!cached) return
  serviceStatusModalRecord.value = record
  serviceStatusModalResult.value = {
    status: cached.status,
    exit_code: cached.exitCode,
    stdout: cached.stdout,
    stderr: cached.stderr,
  }
  serviceStatusModalVisible.value = true
}

// 点「查看状态图」时先实时拉一次 systemctl status，避免弹窗展示陈旧缓存。
async function openManagedServiceStatus(record) {
  await handleCheckServiceStatus(record)
  openServiceStatusModal(record)
}

watch(() => autoRefreshEnabled.value, restartRefreshTimer)
watch(() => refreshIntervalSeconds.value, restartRefreshTimer)

useKeepAliveRefreshLifecycle(restartRefreshTimer, clearRefreshTimer)

onMounted(async () => {
  await Promise.all([loadAllData(), loadPackages(), loadAlertSettings()])
  restartRefreshTimer()
})

onBeforeUnmount(() => {
  clearRefreshTimer()
})
</script>

<style scoped>
.monitor-page {
  padding: 0;
}

.tools {
  margin-bottom: 12px;
}

.managed-target-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.managed-target-toolbar__title {
  font-size: 15px;
  font-weight: 600;
}

.fluent-bit-batch-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  margin-bottom: 12px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}

.fluent-bit-batch-bar__count {
  color: #666;
  font-size: 13px;
}

.fluent-bit-layout {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.fluent-bit-tree {
  flex: 0 0 200px;
  width: 200px;
  padding: 8px;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}

.fluent-bit-tree__search {
  margin-bottom: 8px;
}

.fluent-bit-tree__body {
  max-height: 520px;
  overflow: auto;
}

/* 表格区必须能收缩，否则 flex 子项默认 min-width:auto 会被 1900px 的表格撑破布局。 */
.fluent-bit-table {
  flex: 1;
  min-width: 0;
}

.fluent-bit-table__filters {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

/* 行内动作收进下拉面板，避免合并后操作列被 14 个按钮撑到不可用。 */
.row-action-menu {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 150px;
  padding: 8px;
  background: #fff;
  border-radius: 6px;
  box-shadow: 0 3px 6px -4px rgb(0 0 0 / 12%), 0 6px 16px 0 rgb(0 0 0 / 8%);
}

.row-action-menu :deep(.ant-btn) {
  text-align: left;
}

.package-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

@media (max-width: 576px) {
  .package-toolbar {
    align-items: stretch;
    flex-direction: column;
  }
}

.right-actions {
  text-align: right;
}

.prom-url {
  color: #1f1f1f;
}

.service-status-output {
  max-height: 400px;
  overflow: auto;
  background: #1f1f1f;
  color: #d9d9d9;
  padding: 12px;
  border-radius: 6px;
  font-family: 'Courier New', monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}

.monitor-card {
  border-radius: 12px;
}

.alert-settings-form {
  max-width: 480px;
  padding: 12px 0;
}

.overview-grid {
  margin-bottom: 12px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
}

.overview-card {
  border-radius: 12px;
}

.form-item-hint {
  margin-top: 4px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}

.package-playbook-textarea {
  font-family: 'Courier New', Consolas, Monaco, monospace;
  font-size: 12px;
}

.prom-config-text {
  max-height: 620px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  border-radius: 8px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  font-family: 'Courier New', Consolas, Monaco, monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre;
}
</style>
