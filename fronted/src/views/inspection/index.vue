<template>
  <div class="inspection-page">
    <header class="page-header">
      <div>
        <h1>巡检中心</h1>
        <p>按逻辑服务组织检查，实例巡检由 Agent 并发执行。</p>
      </div>
      <div class="summary-strip">
        <div><strong>{{ groupPagination.total }}</strong><span>巡检组</span></div>
        <div><strong>{{ taskPagination.total }}</strong><span>巡检任务</span></div>
        <div><strong>{{ runningCount }}</strong><span>执行中</span></div>
      </div>
    </header>

    <a-tabs v-model:activeKey="activeTab" class="workspace-tabs" @change="handleTabChange">
      <a-tab-pane key="tasks" tab="巡检任务">
        <div class="toolbar">
          <a-button v-permission="'inspection:tasks:create'" size="large" @click="openTaskModal()">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增任务</span>
          </a-button>
          <a-button size="large" @click="loadTasks">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="taskColumns"
          :data-source="tasks"
          :loading="taskLoading"
          :pagination="taskPagination"
          :scroll="{ x: 1330 }"
          @change="handleTaskTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'group_name'">
              <a-space wrap>
                <a-tag v-for="group in (record.groups || [])" :key="group.id" :color="group.category === 'application' ? 'purple' : 'green'">{{ group.name }}</a-tag>
              </a-space>
            </template>
            <template v-else-if="column.key === 'scope'">
              <a-tag :color="record.scope === 'per_deployment' ? 'blue' : 'cyan'">
                {{ scopeLabel(record.scope) }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'target'">
              <a-space>
                <a-tag :color="record.target_type === 'host_group' ? 'gold' : 'geekblue'">
                  {{ record.target_type === 'host_group' ? '主机组' : '逻辑服务' }}
                </a-tag>
                <a-tooltip v-if="record.target_type === 'host_group'" title="查看巡检范围" placement="top">
                  <a-button type="link" size="small" @click="openScopeViewer(record)">{{ record.target_name }}</a-button>
                </a-tooltip>
                <span v-else>{{ record.target_name }}</span>
              </a-space>
            </template>
            <template v-else-if="column.key === 'enabled'">
              <a-switch
                :checked="record.enabled === true"
                :disabled="!canUpdateTask || togglingTaskId === record.id"
                :loading="togglingTaskId === record.id"
                checked-children="启用"
                un-checked-children="停用"
                @change="(checked) => toggleTaskEnabled(record, checked)"
              />
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-tooltip title="编辑" placement="top">
                  <a-button v-permission="'inspection:tasks:update'" size="small" type="primary" @click="openTaskModal(record)">
                    <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="运行" placement="top">
                  <a-button v-permission="'inspection:tasks:run'" size="small" type="primary" ghost :loading="runningTaskIds.has(record.id)" @click="runTask(record)">
                    <FontAwesomeIcon :icon="['fas', 'play']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除" placement="top">
                  <a-button v-permission="'inspection:tasks:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteTask(record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-tab-pane>

      <a-tab-pane key="schedules" tab="定时任务">
        <div class="toolbar">
          <a-button v-permission="'inspection:tasks:update'" size="large" @click="openCreateScheduleModal">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增定时任务</span>
          </a-button>
          <a-button size="large" @click="loadSelectOptions">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="scheduleColumns"
          :data-source="scheduledTasks"
          :loading="taskOptionsLoading"
          :pagination="false"
          :scroll="{ x: 1280 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'target'">
              <a-space>
                <a-tag :color="record.target_type === 'host_group' ? 'gold' : 'geekblue'">
                  {{ record.target_type === 'host_group' ? '主机组' : '逻辑服务' }}
                </a-tag>
                <a-tooltip v-if="record.target_type === 'host_group'" title="查看巡检范围" placement="top">
                  <a-button type="link" size="small" @click="openScopeViewer(record)">{{ record.target_name }}</a-button>
                </a-tooltip>
                <span v-else>{{ record.target_name }}</span>
              </a-space>
            </template>
            <template v-else-if="column.key === 'schedule'">
              <a-space v-if="record.cron_expression" direction="vertical" :size="0">
                <code>{{ record.cron_expression }}</code>
                <span class="schedule-next">下次 {{ formatTime(record.next_run_time) }}</span>
              </a-space>
              <span v-else class="schedule-next">未配置</span>
            </template>
            <template v-else-if="column.key === 'enabled'">
              <a-switch
                :checked="record.enabled === true"
                :disabled="!canUpdateTask || togglingTaskId === record.id"
                :loading="togglingTaskId === record.id"
                checked-children="启用"
                un-checked-children="停用"
                @change="(checked) => toggleTaskEnabled(record, checked)"
              />
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-tooltip title="编辑" placement="top">
                  <a-button v-permission="'inspection:tasks:update'" size="small" type="primary" @click="openScheduleModal(record)">
                    <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip v-if="record.cron_expression" title="删除" placement="top">
                  <a-button v-permission="'inspection:tasks:update'" class="delBtn" size="small" type="primary" danger @click="confirmClearSchedule(record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-tab-pane>

      <a-tab-pane key="groups" tab="巡检组">
        <div class="toolbar">
          <a-button v-permission="'inspection:groups:create'" size="large" @click="openGroupModal()">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增巡检组</span>
          </a-button>
          <a-button size="large" @click="loadGroups">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="groupColumns"
          :data-source="groups"
          :loading="groupLoading"
          :pagination="groupPagination"
          :scroll="{ x: 900 }"
          @change="handleGroupTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'category'">
              <a-tag :color="record.category === 'application' ? 'purple' : 'green'">{{ record.category === 'application' ? (record.application_name ? `应用 · ${record.application_name}` : '应用类型') : '通用' }}</a-tag>
            </template>
            <template v-else-if="column.key === 'scope'">
              <a-tag :color="record.scope === 'per_host' ? 'gold' : record.scope === 'service_once' ? 'cyan' : 'blue'">{{ scopeLabel(record.scope) }}</a-tag>
            </template>
            <template v-else-if="column.key === 'checks'">
              <a-space wrap>
                <a-tag
                  v-for="check in record.checks"
                  :key="check.id || check.name"
                  :color="check.severity === 'warning' ? 'orange' : 'red'"
                >
                  {{ check.name }} · {{ executorLabel(check.executor) }} · {{ severityLabel(check.severity) }}
                </a-tag>
              </a-space>
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-tooltip title="编辑" placement="top">
                  <a-button v-permission="'inspection:groups:update'" size="small" type="primary" @click="openGroupModal(record)">
                    <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                  </a-button>
                </a-tooltip>
                <a-tooltip title="删除" placement="top">
                  <a-button v-permission="'inspection:groups:delete'" class="delBtn" size="small" type="primary" danger @click="confirmDeleteGroup(record)">
                    <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-tab-pane>

      <a-tab-pane key="executions" tab="执行记录">
        <div class="toolbar">
          <a-select
            v-model:value="executionFilters.task"
            class="filter-select"
            allow-clear
            size="large"
            show-search
            option-filter-prop="label"
            placeholder="全部任务"
            :getPopupContainer="getPopupContainer"
            @change="handleExecutionFilterChange"
          >
            <a-select-option v-for="task in taskOptions" :key="task.id" :value="task.id" :label="task.name">{{ task.name }}</a-select-option>
          </a-select>
          <a-select
            v-model:value="executionFilters.status"
            class="filter-select"
            allow-clear
            size="large"
            placeholder="全部状态"
            :options="statusFilterOptions"
            :getPopupContainer="getPopupContainer"
            @change="handleExecutionFilterChange"
          />
          <a-select
            v-model:value="executionFilters.trigger_type"
            class="filter-select"
            allow-clear
            size="large"
            placeholder="全部触发方式"
            :options="triggerFilterOptions"
            :getPopupContainer="getPopupContainer"
            @change="handleExecutionFilterChange"
          />
          <a-range-picker
            v-model:value="executionFilters.range"
            :show-time="executionRangeShowTime"
            :presets="executionRangePresets"
            size="large"
            format="YYYY-MM-DD HH:mm:ss"
            :placeholder="['开始时间', '结束时间']"
            :getPopupContainer="getPopupContainer"
            @openChange="handleExecutionRangeOpenChange"
            @change="handleExecutionFilterChange"
          />
          <a-button size="large" @click="loadExecutions">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
        </div>
        <a-table
          row-key="id"
          :columns="executionColumns"
          :data-source="executions"
          :loading="executionLoading"
          :pagination="executionPagination"
          :scroll="{ x: 1200 }"
          @change="handleExecutionTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</a-tag>
            </template>
            <template v-else-if="column.key === 'trigger_type'">
              <a-tag :color="record.trigger_type === 'scheduled' ? 'purple' : 'default'">
                {{ record.trigger_type === 'scheduled' ? '定时' : '手动' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'summary'">
              {{ record.summary?.success || 0 }} 成功 / {{ record.summary?.failed || 0 }} 失败
              <a-tag v-if="record.summary?.warning" color="orange">{{ record.summary.warning }} 警告</a-tag>
            </template>
            <template v-else-if="column.key === 'create_time'">{{ formatTime(record.create_time) }}</template>
            <template v-else-if="column.key === 'action'">
              <a-space :size="6">
                <a-tooltip title="详细日志" placement="top">
                  <a-button size="small" @click="openExecution(record)"><FontAwesomeIcon :icon="['fas', 'list-check']" /></a-button>
                </a-tooltip>
                <a-tooltip v-if="['pending', 'running'].includes(record.status)" title="取消" placement="top">
                  <a-button
                    v-permission="'inspection:executions:cancel'"
                    danger
                    ghost
                    size="small"
                    :loading="cancelingExecutionId === record.id"
                    @click="cancelExecution(record)"
                  >取消</a-button>
                </a-tooltip>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-tab-pane>
    </a-tabs>

    <a-modal
      v-model:open="groupModalOpen"
      :title="groupForm.id ? '编辑巡检组' : '新增巡检组'"
      width="760px"
      centered
      :confirm-loading="savingGroup"
      @ok="submitGroup"
    >
      <a-form layout="vertical">
        <div class="form-grid">
          <a-form-item label="巡检组名称" required><a-input v-model:value="groupForm.name" /></a-form-item>
          <a-form-item label="执行范围" required>
            <a-select v-model:value="groupForm.scope" :options="scopeOptions" :getPopupContainer="getPopupContainer" />
            <div class="field-hint">范围决定任务选逻辑服务还是主机组，也决定可用变量。</div>
          </a-form-item>
        </div>
        <div class="form-grid">
          <a-form-item label="分类">
            <a-select v-model:value="groupForm.category" :options="groupCategoryOptions" :getPopupContainer="getPopupContainer" />
            <div class="field-hint">仅用于组织和建议过滤，不做强制约束：通用 = 所有主机适用的基线；应用类型 = 某类应用专属。</div>
          </a-form-item>
          <a-form-item v-if="groupForm.category === 'application'" label="适用应用">
            <a-select
              v-model:value="groupForm.application"
              :options="applicationOptions"
              :getPopupContainer="getPopupContainer"
              show-search
              option-filter-prop="label"
              allow-clear
              placeholder="建议选择（可选）"
            />
            <div class="field-hint">关联应用（跨环境），建任务时用于建议过滤。</div>
          </a-form-item>
        </div>
        <a-form-item label="描述"><a-textarea v-model:value="groupForm.description" :rows="2" /></a-form-item>
        <a-alert type="info" show-icon class="variable-hint">
          <template #message>
            可用变量（{{ groupTargetsHostGroup ? '主机组巡检' : '逻辑服务巡检' }}）
          </template>
          <template #description>
            <div class="variable-list">
              <div v-for="item in availableVariables" :key="item.name" class="variable-item">
                <code>{{ item.name }}</code><span>{{ item.desc }}</span>
              </div>
            </div>
            <div class="variable-note">
              {{ groupTargetsHostGroup
                ? '主机组巡检不解析应用上下文变量（如 ${APP_HOME}），填写后会保存失败。'
                : '仅逻辑服务范围可用应用上下文变量；主机组范围只能用 ${HOST_IP}、${HOST_NAME}。' }}
              变量对待校验文件路径生效；Schema 内容不做展开。
            </div>
          </template>
        </a-alert>
        <div class="check-heading">
          <span>检查项</span>
          <a-button size="large" @click="addCheck"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;添加检查项</a-button>
        </div>
        <div v-for="(check, index) in groupForm.checks" :key="check.localKey" class="check-editor">
          <div class="check-editor-head">
            <strong>检查项 {{ index + 1 }}</strong>
            <a-tooltip title="删除" placement="top">
              <a-button class="delBtn" size="small" type="primary" danger @click="confirmRemoveCheck(check, index)">
                <FontAwesomeIcon :icon="['fas', 'trash-can']" />
              </a-button>
            </a-tooltip>
          </div>
          <div class="form-grid">
            <a-form-item label="名称" required><a-input v-model:value="check.name" /></a-form-item>
            <a-form-item label="执行器" required>
              <a-select v-model:value="check.executor" :options="executorOptions" :getPopupContainer="getPopupContainer" @change="handleExecutorChange(check)" />
            </a-form-item>
          </div>
          <a-form-item label="严重级别" required>
            <a-segmented v-model:value="check.severity" :options="severityOptions" block />
            <div class="field-hint">警告级失败只计入汇总，不会把巡检目标判为失败。</div>
          </a-form-item>
          <template v-if="check.executor === 'goss'">
            <a-form-item label="Goss 套件 (YAML)" required>
              <div class="goss-spec-actions">
                <a-button size="small" :loading="gossValidatingKeys.has(check.localKey)" @click="handleGossValidate(check)">
                  <FontAwesomeIcon :icon="['fas', 'circle-check']" />&nbsp;校验 YAML
                </a-button>
              </div>
              <a-textarea v-model:value="check.config.spec" :rows="10" placeholder="file:&#10;  /etc/hosts:&#10;    exists: true&#10;    mode: '0644'" />
              <div class="field-hint">按 goss 官方 schema 校验后保存；支持 ${APP_HOME} 等变量（在待校验路径中生效）。</div>
            </a-form-item>
            <div class="form-grid">
              <a-form-item label="运行用户">
                <a-input v-model:value="check.config.run_user" placeholder="root" />
                <div class="field-hint">留空默认 root（主机组）；逻辑服务默认部署模板运行用户。goss 以该用户视角采集进程/文件/端口。</div>
              </a-form-item>
              <a-form-item label="变量 (JSON)">
                <a-textarea v-model:value="check.config.vars_json" :rows="3" placeholder='{"VAR": "value"}' />
                <div class="field-hint">透传给 goss --vars-inline，YAML 中通过 Go template 语法引用变量。</div>
              </a-form-item>
            </div>
          </template>
          <template v-else-if="check.executor === 'schema_validate'">
            <a-form-item label="待校验文件" required><a-input v-model:value="check.config.path" placeholder="${APP_HOME}/conf/server.xml" /></a-form-item>
            <div class="form-grid">
              <a-form-item label="Schema 类型" required>
                <a-select v-model:value="check.config.schema_type" :options="schemaTypeOptions" :getPopupContainer="getPopupContainer" @change="handleSchemaTypeChange(check)" />
              </a-form-item>
              <a-form-item label="文档类型" required>
                <a-select v-model:value="check.config.document_type" :options="schemaDocumentTypeOptions(check.config.schema_type)" :getPopupContainer="getPopupContainer" />
              </a-form-item>
            </div>
            <a-form-item label="Schema 内容" required>
              <a-textarea v-model:value="check.config.schema_content" :rows="8" />
            </a-form-item>
          </template>
        </div>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="taskModalOpen"
      :title="taskForm.id ? '编辑巡检任务' : '新增巡检任务'"
      centered
      :confirm-loading="savingTask"
      @ok="submitTask"
    >
      <a-form layout="vertical">
        <a-form-item label="任务名称" required><a-input v-model:value="taskForm.name" /></a-form-item>
        <a-form-item label="巡检组" required>
          <a-select
            v-model:value="taskForm.groups"
            mode="multiple"
            :options="taskGroupSelectOptions"
            :getPopupContainer="getPopupContainer"
            show-search
            option-filter-prop="label"
            placeholder="通用基线 + 应用专属检查，一次巡完"
            @change="handleTaskGroupsChange"
          />
          <div class="field-hint">可组合多个巡检组（典型：1 个通用 + 1 个应用类型），一次执行全部检查。要求所有组执行范围一致：{{ selectedTaskGroups.length ? `已选 ${scopeLabel(selectedTaskGroups[0].scope)}` : '请先选择巡检组' }}。</div>
        </a-form-item>
        <a-form-item v-if="taskTargetsHostGroup === false" label="项目">
          <a-select
            v-model:value="taskProjectFilter"
            allow-clear
            show-search
            option-filter-prop="label"
            placeholder="全部项目"
            :options="projectFilterOptions"
            :getPopupContainer="getPopupContainer"
            @change="handleTaskProjectChange"
          />
          <div class="field-hint">仅用于收窄下方候选，不会保存到任务。</div>
        </a-form-item>
        <a-form-item v-if="taskTargetsHostGroup === false" label="逻辑服务" required>
          <a-tree-select
            v-model:value="taskForm.logical_service"
            :tree-data="serviceTreeData"
            :getPopupContainer="getPopupContainer"
            :loading="serviceTreeLoading"
            tree-default-expand-all
            tree-line
            tree-node-filter-prop="title"
            show-search
            allow-clear
            placeholder="请选择业务系统 / 环境 / 逻辑服务"
            :dropdown-style="{ maxHeight: '360px', overflow: 'auto' }"
          />
        </a-form-item>
        <a-form-item v-else-if="taskTargetsHostGroup === true" label="主机范围" required>
          <a-input :value="taskScopePreviewText" readonly placeholder="尚未勾选主机组或主机" />
          <div class="scope-actions">
            <a-button size="small" type="primary" ghost :loading="hostScopeLoading" @click="openScopeEditor">
              编辑主机范围
            </a-button>
          </div>
          <div class="field-hint">任务绑定的是勾选时的主机列表；之后新加入分组的主机不会自动纳入，需重新勾选。</div>
        </a-form-item>
        <div class="form-grid">
          <a-form-item label="并发数"><a-input-number v-model:value="taskForm.concurrency" :min="1" :max="100" /></a-form-item>
          <a-form-item label="单目标超时（秒）"><a-input-number v-model:value="taskForm.timeout_seconds" :min="5" :max="3600" /></a-form-item>
        </div>
        <a-form-item label="状态">
          <a-switch v-model:checked="taskForm.enabled" checked-children="启用" un-checked-children="停用" />
          <div class="field-hint">停用后任务不会被定时调度，也不能手动运行。</div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:open="scopeEditorOpen"
      title="编辑巡检主机范围"
      centered
      :width="1080"
      @ok="scopeEditorOpen = false"
      @cancel="scopeEditorOpen = false"
    >
      <div class="scope-desc">勾选分组只是批量勾选入口，最终保存的是具体主机列表；分组后续新增的主机不会自动进入本任务。</div>
      <a-input v-model:value="scopeEditKeyword" allow-clear placeholder="搜索分组/主机/IP" class="scope-search" />
      <div class="scope-tree-wrap">
        <a-tree
          v-if="filteredScopeEditTree.length > 0"
          checkable
          block-node
          :checked-keys="scopeCheckedKeys"
          :expanded-keys="scopeEditExpandedKeys"
          :auto-expand-parent="true"
          :tree-data="filteredScopeEditTree"
          :selectable="false"
          :show-line="{ showLeafIcon: false }"
          @check="onScopeCheck"
        />
        <a-empty v-else description="未匹配到分组" />
      </div>
    </a-modal>

    <a-modal
      v-model:open="scopeViewerOpen"
      :title="scopeViewerTitle"
      centered
      :width="980"
      :footer="null"
    >
      <div class="scope-desc">仅展示该任务的巡检范围（主机组与主机），未命中节点已隐藏</div>
      <div class="scope-summary">当前范围：{{ countScopeHosts(scopeViewerTree) }}台主机</div>
      <a-input v-model:value="scopeViewKeyword" allow-clear placeholder="搜索分组/主机/IP" class="scope-search" />
      <div class="scope-tree-wrap">
        <a-tree
          v-if="filteredScopeViewTree.length > 0"
          block-node
          :expanded-keys="scopeViewExpandedKeys"
          :auto-expand-parent="true"
          :tree-data="filteredScopeViewTree"
          :selectable="false"
          :show-line="{ showLeafIcon: false }"
        />
        <a-empty v-else description="暂无已勾选范围" />
      </div>
    </a-modal>

    <a-modal
      v-model:open="scheduleModalOpen"
      :title="scheduleForm.id ? '编辑定时计划' : '新增定时任务'"
      centered
      :confirm-loading="savingSchedule"
      @ok="submitSchedule"
    >
      <a-form layout="vertical">
        <a-form-item label="巡检名称" required><a-input v-model:value="scheduleForm.inspection_name" /></a-form-item>
        <a-form-item v-if="scheduleForm.id" label="关联巡检任务"><a-input :value="scheduleForm.name" disabled /></a-form-item>
        <a-form-item v-else label="关联巡检任务" required>
          <a-select
            v-model:value="scheduleForm.task"
            show-search
            option-filter-prop="label"
            placeholder="请选择尚未配置计划的巡检任务"
            :getPopupContainer="getPopupContainer"
          >
            <a-select-option v-for="task in unscheduledTaskOptions" :key="task.id" :value="task.id" :label="task.name">
              {{ task.name }}
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="定时计划" required>
          <a-input v-model:value="scheduleForm.cron_expression" placeholder="例如 0 2 * * *" />
          <div class="field-hint">5 段 cron：分 时 日 月 周；保存后由调度器按分钟粒度扫描到期任务。</div>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-drawer v-model:open="executionDrawerOpen" title="巡检执行详情" width="760">
      <a-descriptions v-if="selectedExecution" bordered size="small" :column="2">
        <a-descriptions-item label="任务">{{ selectedExecution.task_name }}</a-descriptions-item>
        <a-descriptions-item label="状态">{{ statusLabel(selectedExecution.status) }}</a-descriptions-item>
        <a-descriptions-item :label="executionTargetLabel">{{ selectedExecution.service_snapshot?.name }}</a-descriptions-item>
        <a-descriptions-item label="触发方式">{{ selectedExecution.trigger_type === 'scheduled' ? '定时' : '手动' }}</a-descriptions-item>
        <a-descriptions-item label="开始时间">{{ formatTime(selectedExecution.start_time) }}</a-descriptions-item>
        <a-descriptions-item label="结束时间">{{ formatTime(selectedExecution.end_time) }}</a-descriptions-item>
        <a-descriptions-item v-if="selectedExecution.service_snapshot?.skipped_no_agent" label="已跳过" :span="2">
          {{ selectedExecution.service_snapshot.skipped_no_agent }} 台主机未安装 Agent，未纳入本次巡检
        </a-descriptions-item>
      </a-descriptions>
      <a-collapse v-if="selectedExecution" class="target-results">
        <a-collapse-panel v-for="target in selectedExecution.targets" :key="target.id">
          <template #header>
            <a-space><a-badge :status="target.status === 'skipped' ? 'default' : target.passed ? 'success' : 'error'" />{{ target.target_name }}</a-space>
          </template>
          <a-alert v-if="targetErrorMessage(target)" type="error" :message="targetErrorMessage(target)" show-icon />
          <a-table
            row-key="check_key"
            size="small"
            :pagination="false"
            :columns="resultColumns"
            :data-source="targetDisplayResults(target)"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'status'">
                <a-tag :color="record.status === 'pass' ? 'green' : record.status === 'skipped' ? 'default' : 'red'">{{ record.status }}</a-tag>
              </template>
              <template v-else-if="column.key === 'severity'">
                <a-tag :color="record.severity === 'warning' ? 'orange' : 'red'">{{ severityLabel(record.severity) }}</a-tag>
              </template>
              <template v-else-if="column.key === 'expected_value'"><pre>{{ formatValue(record.expected_value) }}</pre></template>
              <template v-else-if="column.key === 'actual_value'"><pre>{{ formatValue(record.actual_value) }}</pre></template>
            </template>
          </a-table>
        </a-collapse-panel>
      </a-collapse>
    </a-drawer>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'
import {
  getApplicationList,
  getApplicationServiceList,
  getBusinessEnvironmentList,
  getBusinessSystemList,
  getProjectList,
} from '@/api/assets/application'
import {
  deleteInspectionGroup,
  deleteInspectionTask,
  getInspectionExecution,
  getInspectionExecutions,
  getInspectionGroups,
  getInspectionHostScopeTree,
  getInspectionTasks,
  runInspectionTask,
  cancelInspectionExecution,
  saveInspectionGroup,
  saveInspectionTask,
  validateGossSpec,
} from '@/api/inspection'
import {
  appendHostCount,
  buildHostScopeTree,
  collectGroupKeys,
  collectHostIds,
  countScopeHosts,
  filterHostScopeTree,
  pickHostIds,
  pruneHostScopeTree,
  retainAvailableHostIds,
  toHostKeys,
} from '@/util/hostScopeTree'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { checkPermission } from '@/directives/permission/permission'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { formatTimeWithTimezone } from '@/util/timezone'
import { buildUserTimezoneRangePresets, buildUserTimezoneShowTime, toUtcQueryISOStringByUserTimezone } from '@/util/timezoneRange'
import store from '@/store'

const activeTab = ref('tasks')
const groups = ref([])
const tasks = ref([])
// 列表已分页，下拉候选必须另外取全量，否则只能选到第一页的巡检组/任务。
const groupOptions = ref([])
const applicationOptions = ref([])
async function ensureApplicationOptions() {
  if (applicationOptions.value.length) return
  try {
    const records = await fetchAll(getApplicationList)
    applicationOptions.value = records.map((item) => ({ label: item.name, value: item.id }))
  } catch { applicationOptions.value = [] }
}
const taskOptions = ref([])
const businessSystems = ref([])
const businessEnvironments = ref([])
const projects = ref([])
const taskProjectFilter = ref(undefined)
const services = ref([])
const hostScopeGroups = ref([])
const hostScopeHosts = ref([])
const executions = ref([])
const groupLoading = ref(false)
const taskLoading = ref(false)
const executionLoading = ref(false)
const taskOptionsLoading = ref(false)
const serviceTreeLoading = ref(false)
const hostScopeLoading = ref(false)
const hostScopeLoaded = ref(false)
const groupModalOpen = ref(false)
const taskModalOpen = ref(false)
const scheduleModalOpen = ref(false)
const scopeEditorOpen = ref(false)
const scopeViewerOpen = ref(false)
const scopeEditKeyword = ref('')
const scopeViewKeyword = ref('')
const scopeViewerTitle = ref('查看巡检范围')
const scopeViewerTree = ref([])
const scopeCheckedKeys = ref([])
const executionDrawerOpen = ref(false)
const savingGroup = ref(false)
const savingTask = ref(false)
const savingSchedule = ref(false)
const selectedExecution = ref(null)
const runningTaskIds = reactive(new Set())
const togglingTaskId = ref(null)
const canUpdateTask = checkPermission('inspection:tasks:update')
const cancelingExecutionId = ref(null)
let executionPollTimer = null
let localKey = 0

const createPagination = () => reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showTotal: (total) => `共 ${total} 条`,
})
const groupPagination = createPagination()
const taskPagination = createPagination()
const executionPagination = createPagination()
const executionFilters = reactive({ task: undefined, status: undefined, trigger_type: undefined, range: undefined })
const userTimezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const executionRangePresets = ref([])
const executionRangeShowTime = buildUserTimezoneShowTime(userTimezone.value)

const emptyGroupForm = () => ({ id: null, name: '', scope: 'per_deployment', description: '', enabled: true, category: 'general', application: undefined, checks: [] })
const emptyTaskForm = () => ({
  groups: [],
  id: null,
  name: '',
  group: undefined,
  logical_service: undefined,
  selected_host_ids: [],
  concurrency: 20,
  timeout_seconds: 60,
  enabled: true,
})
const emptyScheduleForm = () => ({ id: null, task: undefined, name: '', inspection_name: '', cron_expression: '' })
const groupForm = reactive(emptyGroupForm())
const taskForm = reactive(emptyTaskForm())
const scheduleForm = reactive(emptyScheduleForm())

const taskColumns = [
  { title: '任务名称', dataIndex: 'name', key: 'name', width: 180 },
  { title: '巡检组', dataIndex: 'group_name', key: 'group_name', width: 160 },
  { title: '目标', dataIndex: 'target_name', key: 'target', width: 200 },
  { title: '范围', key: 'scope', width: 140 },
  { title: '并发 / 超时', key: 'limits', customRender: ({ record }) => `${record.concurrency} / ${record.timeout_seconds}s`, width: 130 },
  { title: '状态', key: 'enabled', width: 90 },
  { title: '操作', key: 'action', fixed: 'right', width: 170 },
]
const scheduleColumns = [
  { title: '巡检名称', dataIndex: 'inspection_name', key: 'inspection_name', width: 180 },
  { title: '关联巡检任务', dataIndex: 'name', key: 'name', width: 200 },
  { title: '巡检组', dataIndex: 'group_name', key: 'group_name', width: 180 },
  { title: '目标', key: 'target', width: 260 },
  { title: '定时计划', key: 'schedule', width: 250 },
  { title: '任务状态', key: 'enabled', width: 110 },
  { title: '操作', key: 'action', fixed: 'right', width: 120 },
]
const groupColumns = [
  { title: '巡检组', dataIndex: 'name', key: 'name', width: 180 },
  { title: '分类', key: 'category', width: 120 },
  { title: '范围', key: 'scope', width: 140 },
  { title: '检查项', key: 'checks', width: 420 },
  { title: '操作', key: 'action', fixed: 'right', width: 120 },
]
const executionColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '任务', dataIndex: 'task_name', key: 'task_name', width: 180 },
  { title: '目标', dataIndex: 'target_name', key: 'target_name', width: 180 },
  { title: '状态', key: 'status', width: 100 },
  { title: '触发', key: 'trigger_type', width: 90 },
  { title: '结果', key: 'summary', width: 200 },
  { title: '发起人', dataIndex: 'requested_username', key: 'requested_username', width: 120 },
  { title: '创建时间', key: 'create_time', width: 180 },
  { title: '操作', key: 'action', fixed: 'right', width: 90 },
]
const resultColumns = [
  { title: '检查项', dataIndex: 'name', key: 'name', width: 170 },
  { title: '巡检组', dataIndex: 'group_name', key: 'group_name', width: 130 },
  { title: '状态', key: 'status', width: 90 },
  { title: '级别', key: 'severity', width: 80 },
  { title: '期望值', key: 'expected_value', width: 200 },
  { title: '实际值', key: 'actual_value', width: 220 },
  { title: '消息', dataIndex: 'message', key: 'message' },
]

const runningCount = computed(() => executions.value.filter((item) => ['pending', 'running'].includes(item.status)).length)
const scheduledTasks = computed(() => taskOptions.value.filter((task) => task.cron_expression))
const unscheduledTaskOptions = computed(() => taskOptions.value.filter((task) => !task.cron_expression))
// Schema 校验读目标主机文件；Goss 是 YAML 声明式套件（端口/进程/服务/文件/命令等），两者都在 Agent 端执行。
const executorOptions = [
  { label: 'Schema', value: 'schema_validate' },
  { label: 'Goss', value: 'goss' },
]
const severityOptions = [
  { label: '严重', value: 'critical' },
  { label: '警告', value: 'warning' },
]
// 变量表与 executor.py 的 _resolve / _resolve_host 一一对应，修改后端解析时需同步。
const DEPLOYMENT_VARIABLES = [
  { name: '${APP_HOME}', desc: '部署模板的 App Home 目录' },
  { name: '${RUN_USER}', desc: '部署模板的运行用户' },
  { name: '${INSTANCE_NAME}', desc: '部署实例名称' },
  { name: '${APPLICATION_VERSION}', desc: '应用版本号' },
  { name: '${SERVICE_NAME}', desc: '模板服务名' },
  { name: '${HOST_IP}', desc: '实例所在主机 IP' },
]
const HOST_VARIABLES = [
  { name: '${HOST_IP}', desc: '主机 IP' },
  { name: '${HOST_NAME}', desc: '主机名称' },
]
const scopeOptions = [
  { label: '逻辑服务·每个部署实例', value: 'per_deployment' },
  { label: '逻辑服务·服务单次', value: 'service_once' },
  { label: '主机组·每台主机', value: 'per_host' },
]
const statusFilterOptions = [
  { label: '等待中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' },
]
const triggerFilterOptions = [
  { label: '手动', value: 'manual' },
  { label: '定时', value: 'scheduled' },
]
const executionTargetLabel = computed(
  () => selectedExecution.value?.service_snapshot?.target_type === 'host_group' ? '主机组' : '逻辑服务',
)
const schemaTypeOptions = [
  { label: 'JSON Schema', value: 'json_schema' },
  { label: 'Schematron', value: 'schematron' },
  { label: '正则表达式', value: 'regexp' },
]
const schemaDocumentTypes = {
  json_schema: [
    { label: 'JSON', value: 'json' },
    { label: 'YAML', value: 'yaml' },
    { label: 'TOML', value: 'toml' },
    { label: 'INI', value: 'ini' },
    { label: 'Properties', value: 'properties' },
  ],
  schematron: [{ label: 'XML', value: 'xml' }],
  regexp: [{ label: 'Text', value: 'text' }],
}
const selectedTaskGroups = computed(() => (taskForm.groups || []).map((id) => groupOptions.value.find((group) => group.id === id)).filter(Boolean))
// undefined 表示尚未选巡检组，此时两类目标输入都不展示。
const taskTargetsHostGroup = computed(() => (
  selectedTaskGroups.value.length ? selectedTaskGroups.value[0].scope === 'per_host' : undefined
))
const groupTargetsHostGroup = computed(() => groupForm.scope === 'per_host')
const availableVariables = computed(() => (groupTargetsHostGroup.value ? HOST_VARIABLES : DEPLOYMENT_VARIABLES))
const projectFilterOptions = computed(() => [...projects.value]
  .sort((left, right) => String(left.name).localeCompare(String(right.name), 'zh-CN'))
  .map((project) => ({ label: project.name, value: project.id })))
// 服务 -> 所属项目：编辑时靠它回填过滤器，避免选中服务落在过滤后的树外而显示为空。
function resolveServiceProjectId(serviceId) {
  const service = services.value.find((item) => String(item.id) === String(serviceId))
  if (!service) return undefined
  const system = businessSystems.value.find((item) => String(item.id) === String(service.business_system))
  return system?.project ?? undefined
}
const serviceTreeData = computed(() => {
  const serviceNodes = (records) => [...records]
    .sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
    .map((service) => ({
      title: service.name,
      value: service.id,
      // a-tree-select 要求 key 与 value 一致，否则会告警并影响选中态匹配
      key: service.id,
      isLeaf: true,
    }))
  const environmentsById = new Map(
    businessEnvironments.value.map((environment) => [String(environment.id), environment]),
  )
  const environmentNodes = (records, systemId) => {
    const servicesByEnvironment = new Map()
    for (const service of records) {
      const environmentKey = String(service.environment ?? 'unassigned')
      if (!servicesByEnvironment.has(environmentKey)) servicesByEnvironment.set(environmentKey, [])
      servicesByEnvironment.get(environmentKey).push(service)
    }
    return [...servicesByEnvironment.entries()]
      .sort(([leftKey], [rightKey]) => {
        const left = environmentsById.get(leftKey)
        const right = environmentsById.get(rightKey)
        return ((left?.order || 0) - (right?.order || 0))
          || String(left?.name || '未指定环境').localeCompare(String(right?.name || '未指定环境'), 'zh-CN')
      })
      .map(([environmentKey, environmentServices]) => ({
        title: environmentsById.get(environmentKey)?.name || '未指定环境',
        value: `system:${systemId}:environment:${environmentKey}`,
        key: `system:${systemId}:environment:${environmentKey}`,
        disabled: true,
        children: serviceNodes(environmentServices),
      }))
  }
  const systemNodes = [...businessSystems.value]
    .filter((system) => !taskProjectFilter.value || String(system.project) === String(taskProjectFilter.value))
    .sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
    .map((system) => ({
      title: system.name,
      value: `system:${system.id}`,
      key: `system:${system.id}`,
      disabled: true,
      children: environmentNodes(services.value.filter(
        (service) => String(service.business_system) === String(system.id),
      ), system.id),
    }))
  return systemNodes
})
const hostScopeTreeData = computed(() => buildHostScopeTree(hostScopeGroups.value, hostScopeHosts.value))
const filteredScopeEditTree = computed(() => filterHostScopeTree(hostScopeTreeData.value, scopeEditKeyword.value))
const scopeEditExpandedKeys = computed(() => collectGroupKeys(filteredScopeEditTree.value))
const filteredScopeViewTree = computed(() => filterHostScopeTree(scopeViewerTree.value, scopeViewKeyword.value))
const scopeViewExpandedKeys = computed(() => collectGroupKeys(filteredScopeViewTree.value))
const taskScopePreviewText = computed(() => {
  const hostCount = taskForm.selected_host_ids?.length || 0
  return hostCount ? `已选 ${hostCount} 台主机` : ''
})

const responseData = (response) => response?.data?.data || {}
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formatTime = (value) => value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
const groupCategoryOptions = [
  { label: '通用', value: 'general' },
  { label: '应用类型', value: 'application' },
]
const categoryLabel = (category) => (category === 'application' ? '应用类型' : '通用')
const taskGroupSelectOptions = computed(() => {
  const pick = (category) => groupOptions.value
    .filter((group) => (group.category || 'general') === category)
    .map((group) => ({ label: group.name, value: group.id }))
  const options = []
  if (pick('general').length) options.push({ label: '通用巡检组', options: pick('general') })
  if (pick('application').length) options.push({ label: '应用类型巡检组', options: pick('application') })
  return options.length ? options : groupOptions.value.map((group) => ({ label: group.name, value: group.id }))
})
function handleTaskGroupsChange(nextIDs) {
  // 所有组必须同 scope 才能组合（后端同样校验）；不一致时提示并回退本次选择。
  const selected = (nextIDs || []).map((id) => groupOptions.value.find((group) => group.id === id)).filter(Boolean)
  if (selected.length > 1 && new Set(selected.map((group) => group.scope)).size > 1) {
    message.warning('组合的巡检组必须具有相同的执行范围，已撤销本次选择')
    taskForm.groups = selected.slice(0, -1).map((group) => group.id)
  }
  if (taskTargetsHostGroup.value === true) {
    taskForm.logical_service = undefined
    return
  }
  taskForm.selected_host_ids = []
  scopeCheckedKeys.value = []
}
const scopeLabel = (scope) => ({
  per_deployment: '逻辑服务·每个部署实例',
  service_once: '逻辑服务·服务单次',
  per_host: '主机组·每台主机',
}[scope] || scope)
const executorLabel = (executor) => ({ shell: 'Shell', schema_validate: 'Schema', goss: 'Goss', http: 'HTTP', tcp: 'TCP' }[executor] || executor)
const severityLabel = (severity) => severity === 'warning' ? '警告' : '严重'
const statusLabel = (status) => ({ pending: '等待中', running: '执行中', success: '成功', failed: '失败', canceled: '已取消', skipped: '已跳过' }[status] || status)
const statusColor = (status) => ({ pending: 'default', running: 'processing', success: 'green', failed: 'red', canceled: 'default', skipped: 'default' }[status] || 'default')
const formatValue = (value) => typeof value === 'object' && value !== null ? JSON.stringify(value, null, 2) : String(value ?? '-')
const rawTargetResults = (target) => (Array.isArray(target.raw_result?.checks) ? target.raw_result.checks : [])
  .filter((check) => check?.key !== 'control')
  .map((check, index) => ({
    // Agent 回传的 key 在计划级错误下可能重复，补上下标保证表格 row-key 唯一。
    check_key: `${check.key || 'check'}#${index}`,
    check_type: check.type,
    name: check.name,
    status: check.status,
    severity: check.severity || 'critical',
    expected_value: check.expected,
    actual_value: check.actual,
    message: check.message,
  }))
const targetDisplayResults = (target) => target.results?.length ? target.results : rawTargetResults(target)
const targetErrorMessage = (target) => target.error_message || rawTargetResults(target).find((check) => check.status === 'error')?.message || ''

async function fetchAll(loader, params = {}) {
  const firstData = responseData(await loader({ ...params, page: 1, page_size: 30 }))
  const records = [...(firstData.results || [])]
  const totalPages = Number(firstData.totalPages || 1)
  if (totalPages > 1) {
    const responses = await Promise.all(
      Array.from({ length: totalPages - 1 }, (_, index) => loader({ ...params, page: index + 2, page_size: 30 })),
    )
    for (const response of responses) records.push(...(responseData(response).results || []))
  }
  return records
}

function applyPagination(pagination, data) {
  pagination.total = Number(data.count || 0)
  pagination.current = Number(data.pageNumber || pagination.current)
  pagination.pageSize = Number(data.pageSize || pagination.pageSize)
}

async function loadGroups() {
  groupLoading.value = true
  try {
    const data = responseData(await getInspectionGroups({ page: groupPagination.current, page_size: groupPagination.pageSize }))
    groups.value = data.results || []
    applyPagination(groupPagination, data)
  } finally { groupLoading.value = false }
}
async function loadTasks() {
  taskLoading.value = true
  try {
    const data = responseData(await getInspectionTasks({ page: taskPagination.current, page_size: taskPagination.pageSize }))
    tasks.value = data.results || []
    applyPagination(taskPagination, data)
  } finally { taskLoading.value = false }
}
async function loadServiceTree() {
  serviceTreeLoading.value = true
  try {
    const [systems, environments, serviceRecords, projectRecords] = await Promise.all([
      fetchAll(getBusinessSystemList),
      fetchAll(getBusinessEnvironmentList),
      fetchAll(getApplicationServiceList),
      fetchAll(getProjectList),
    ])
    businessSystems.value = systems
    businessEnvironments.value = environments
    services.value = serviceRecords
    projects.value = projectRecords
  } finally {
    serviceTreeLoading.value = false
  }
}
async function loadHostScopeTree() {
  hostScopeLoading.value = true
  try {
    const data = responseData(await getInspectionHostScopeTree())
    hostScopeGroups.value = data.groups || []
    hostScopeHosts.value = data.hosts || []
    hostScopeLoaded.value = true
  } finally {
    hostScopeLoading.value = false
  }
}
async function loadSelectOptions() {
  taskOptionsLoading.value = true
  try {
    const [groupRecords, taskRecords] = await Promise.all([
      fetchAll(getInspectionGroups),
      fetchAll(getInspectionTasks),
    ])
    groupOptions.value = groupRecords
    taskOptions.value = taskRecords
  } finally {
    taskOptionsLoading.value = false
  }
}
function buildExecutionParams() {
  const params = {
    page: executionPagination.current,
    page_size: executionPagination.pageSize,
    task: executionFilters.task,
    status: executionFilters.status,
    trigger_type: executionFilters.trigger_type,
  }
  const [start, end] = executionFilters.range || []
  if (start && end) {
    params.start_time = toUtcQueryISOStringByUserTimezone(start, userTimezone.value)
    params.end_time = toUtcQueryISOStringByUserTimezone(end, userTimezone.value)
  }
  return params
}
async function loadExecutions() {
  executionLoading.value = true
  try {
    const data = responseData(await getInspectionExecutions(buildExecutionParams()))
    executions.value = data.results || []
    applyPagination(executionPagination, data)
  } finally { executionLoading.value = false }
}
function handleGroupTableChange(pagination) {
  groupPagination.current = pagination.current
  groupPagination.pageSize = pagination.pageSize
  loadGroups()
}
function handleTaskTableChange(pagination) {
  taskPagination.current = pagination.current
  taskPagination.pageSize = pagination.pageSize
  loadTasks()
}
function handleExecutionTableChange(pagination) {
  executionPagination.current = pagination.current
  executionPagination.pageSize = pagination.pageSize
  loadExecutions()
}
function handleExecutionFilterChange() {
  executionPagination.current = 1
  loadExecutions()
}
function handleExecutionRangeOpenChange(open) {
  if (open) executionRangePresets.value = buildUserTimezoneRangePresets(userTimezone.value)
}

function defaultExecutorConfig(executor, targetsHostGroup = groupTargetsHostGroup.value) {
  if (executor === 'schema_validate') {
    return {
      path: targetsHostGroup ? '' : '${APP_HOME}/conf/server.xml',
      schema_type: 'schematron',
      document_type: 'xml',
      schema_content: '',
    }
  }
  if (executor === 'goss') {
    return {
      spec: '',
      run_user: '',
      vars: {},
      environment: {},
    }
  }
  return {}
}
function schemaDocumentTypeOptions(schemaType) { return schemaDocumentTypes[schemaType] || [] }
function handleExecutorChange(check) {
  check.config = defaultExecutorConfig(check.executor, groupTargetsHostGroup.value)
}
function handleSchemaTypeChange(check) {
  check.config.document_type = schemaDocumentTypeOptions(check.config.schema_type)[0]?.value
}
// goss YAML 在线校验：与保存时的官方 schema 校验同源，按钮即时反馈。
const gossValidatingKeys = reactive(new Set())
async function handleGossValidate(check) {
  const spec = (check.config.spec || '').trim()
  if (!spec) {
    message.warning('请先填写 Goss YAML')
    return
  }
  gossValidatingKeys.add(check.localKey)
  try {
    await validateGossSpec(spec)
    message.success('YAML 符合 goss 官方 schema')
  } catch (error) {
    message.error(error?.message || 'YAML 校验失败')
  } finally {
    gossValidatingKeys.delete(check.localKey)
  }
}
function addCheck() {
  const config = defaultExecutorConfig('schema_validate')
  groupForm.checks.push({
    localKey: ++localKey,
    name: '',
    executor: 'schema_validate',
    config,
    severity: 'critical',
    enabled: true,
    order: groupForm.checks.length,
  })
}
function confirmRemoveCheck(check, index) {
  openDeleteConfirm({
    title: '删除检查项',
    summary: '该检查项将从当前巡检组中移除。',
    items: [check.name?.trim() || `检查项 ${index + 1}`],
    onConfirm: async () => { groupForm.checks.splice(index, 1) },
  })
}
function openGroupModal(record) {
  ensureApplicationOptions()
  Object.assign(groupForm, emptyGroupForm(), record ? JSON.parse(JSON.stringify(record)) : {})
  groupForm.checks = (groupForm.checks || []).map((check) => ({
    ...check,
    localKey: ++localKey,
    severity: check.severity || 'critical',
    config: {
      ...(check.config || {}),
      // goss 变量对象转回 JSON 文本供编辑框使用。
      vars_json: check.config?.vars && typeof check.config.vars === 'object' ? JSON.stringify(check.config.vars, null, 2) : '',
    },
  }))
  if (!groupForm.checks.length) addCheck()
  groupModalOpen.value = true
}
function openTaskModal(record) {
  Object.assign(taskForm, emptyTaskForm(), record ? JSON.parse(JSON.stringify(record)) : {})
  // 编辑时把组对象列表转成 ID 列表；兼容仅返回单组 group 的旧数据。
  taskForm.groups = (record?.groups?.length ? record.groups.map((group) => group.id) : (record?.group ? [record.group] : []))
  // 旧任务可能仍保存已删除主机，编辑时按当前完整主机树清理，避免隐藏 ID 阻断保存。
  if (hostScopeLoaded.value) {
    taskForm.selected_host_ids = retainAvailableHostIds(taskForm.selected_host_ids, hostScopeHosts.value)
  }
  taskProjectFilter.value = taskForm.logical_service ? resolveServiceProjectId(taskForm.logical_service) : undefined
  scopeCheckedKeys.value = toHostKeys(taskForm.selected_host_ids)
  taskModalOpen.value = true
}
function openScopeEditor() {
  scopeEditKeyword.value = ''
  scopeCheckedKeys.value = toHostKeys(taskForm.selected_host_ids)
  scopeEditorOpen.value = true
}
function onScopeCheck(nextChecked) {
  // 只保存主机 ID；分组勾选只是批量入口，不能让之后新入组的主机自动进入任务。
  const checkedInView = pickHostIds(nextChecked)
  // 搜索后树被裁剪，只能改当前可见节点的勾选态，否则会误删被隐藏的已选主机。
  const visibleInView = new Set(collectHostIds(filteredScopeEditTree.value))
  const kept = (taskForm.selected_host_ids || []).filter((id) => !visibleInView.has(Number(id)))
  taskForm.selected_host_ids = [...new Set([...kept, ...checkedInView])]
  scopeCheckedKeys.value = toHostKeys(taskForm.selected_host_ids)
}
function openScopeViewer(record) {
  scopeViewKeyword.value = ''
  scopeViewerTitle.value = `查看巡检范围 - ${record?.name || ''}`
  scopeViewerTree.value = appendHostCount(pruneHostScopeTree(hostScopeTreeData.value, record?.selected_host_ids))
  scopeViewerOpen.value = true
}
function handleTaskProjectChange() {
  // 清空过滤器时树恢复全量，已选服务仍可见，不能连带清掉。
  if (!taskProjectFilter.value || !taskForm.logical_service) return
  if (String(resolveServiceProjectId(taskForm.logical_service)) !== String(taskProjectFilter.value)) {
    taskForm.logical_service = undefined
  }
}
function openScheduleModal(record) {
  Object.assign(scheduleForm, emptyScheduleForm(), {
    id: record.id,
    name: record.name,
    inspection_name: record.inspection_name,
    cron_expression: record.cron_expression || '',
  })
  scheduleModalOpen.value = true
}
function openCreateScheduleModal() {
  Object.assign(scheduleForm, emptyScheduleForm())
  scheduleModalOpen.value = true
}
async function submitGroup() {
  if (!groupForm.name.trim() || !groupForm.checks.length || groupForm.checks.some((check) => !check.name.trim())) {
    message.warning('请完整填写巡检组和检查项')
    return
  }
  // 名称重复在本地先拦截，直接指出重复项，避免后端 400 再改一轮。
  const nameCounts = {}
  groupForm.checks.forEach((check) => {
    const name = check.name.trim()
    nameCounts[name] = (nameCounts[name] || 0) + 1
  })
  const duplicated = Object.keys(nameCounts).filter((name) => nameCounts[name] > 1)
  if (duplicated.length) {
    message.warning(`同一巡检组内检查项名称不能重复: ${duplicated.join('、')}`)
    return
  }
  savingGroup.value = true
  try {
    const payload = JSON.parse(JSON.stringify(groupForm))
    payload.checks.forEach((check, index) => {
      delete check.localKey; delete check.id; check.order = index
      // goss 变量在编辑框里是 JSON 文本（vars_json），提交时转成对象。
      if (check.executor === 'goss') {
        const rawVars = (check.config.vars_json || '').trim()
        if (rawVars) {
          try { check.config.vars = JSON.parse(rawVars) } catch { check.config.vars = {} }
        } else check.config.vars = {}
      }
    })
    await saveInspectionGroup(payload)
    groupModalOpen.value = false
    message.success('巡检组已保存')
    await Promise.all([loadGroups(), loadSelectOptions()])
  } catch (error) {
    // 保留弹窗，让用户能直接改掉被后端拒绝的内容。
    message.error(error?.message || '巡检组保存失败')
  } finally { savingGroup.value = false }
}
async function submitTask() {
  if (!hostScopeLoaded.value) await loadHostScopeTree()
  taskForm.selected_host_ids = retainAvailableHostIds(taskForm.selected_host_ids, hostScopeHosts.value)
  const hasScope = (taskForm.selected_host_ids?.length || 0) > 0
  const hasTarget = taskTargetsHostGroup.value ? hasScope : taskForm.logical_service
  if (!taskForm.name.trim() || !taskForm.groups?.length || !hasTarget) {
    message.warning('请完整填写任务信息')
    return
  }
  savingTask.value = true
  try {
    const payload = { ...taskForm, group: taskForm.groups[0] }
    if (taskTargetsHostGroup.value) payload.logical_service = null
    else payload.selected_host_ids = []
    await saveInspectionTask(payload)
    taskModalOpen.value = false
    message.success('巡检任务已保存')
    await Promise.all([loadTasks(), loadSelectOptions()])
  } catch (error) {
    message.error(error?.message || '巡检任务保存失败')
  } finally { savingTask.value = false }
}
async function submitSchedule() {
  if (!scheduleForm.id && !scheduleForm.task) {
    message.warning('请选择巡检任务')
    return
  }
  if (!scheduleForm.inspection_name.trim()) {
    message.warning('请输入巡检名称')
    return
  }
  if (!scheduleForm.cron_expression.trim()) {
    message.warning('请输入定时计划')
    return
  }
  savingSchedule.value = true
  try {
    await saveInspectionTask({
      id: scheduleForm.id || scheduleForm.task,
      inspection_name: scheduleForm.inspection_name.trim(),
      cron_expression: scheduleForm.cron_expression.trim(),
    })
    scheduleModalOpen.value = false
    message.success('定时计划已保存')
    await Promise.all([loadTasks(), loadSelectOptions()])
  } catch (error) {
    message.error(error?.message || '定时计划保存失败')
  } finally { savingSchedule.value = false }
}
async function toggleTaskEnabled(record, checked) {
  togglingTaskId.value = record.id
  try {
    await saveInspectionTask({ id: record.id, enabled: checked })
    message.success(checked ? '巡检任务已启用' : '巡检任务已停用')
    await Promise.all([loadTasks(), loadSelectOptions()])
  } catch (error) {
    message.error(error?.message || '状态切换失败')
    // 失败后重拉，避免开关停在与后端不一致的位置。
    await loadSelectOptions()
  } finally { togglingTaskId.value = null }
}
async function runTask(record) {
  runningTaskIds.add(record.id)
  try {
    await runInspectionTask(record.id)
    message.success('巡检任务已提交')
    activeTab.value = 'executions'
    executionPagination.current = 1
    await loadExecutions()
    startExecutionPolling()
  } catch (error) {
    message.error(error?.message || '巡检任务提交失败')
  } finally { runningTaskIds.delete(record.id) }
}
async function openExecution(record) {
  try {
    selectedExecution.value = responseData(await getInspectionExecution(record.id))
    executionDrawerOpen.value = true
  } catch (error) {
    message.error(error?.message || '获取执行详情失败')
  }
}
function cancelExecution(record) {
  Modal.confirm({
    title: '取消巡检',
    content: `确定取消执行记录 ${record.id} 吗？`,
    okText: '取消执行',
    cancelText: '返回',
    onOk: async () => {
      cancelingExecutionId.value = record.id
      try {
        await cancelInspectionExecution(record.id)
        message.success('巡检执行已取消')
        await loadExecutions()
      } catch (error) {
        message.error(error?.message || '取消执行失败')
      } finally {
        cancelingExecutionId.value = null
      }
    },
  })
}
function confirmDeleteGroup(record) {
  openDeleteConfirm({
    title: '删除巡检组',
    summary: '删除后无法恢复。',
    items: [record.name],
    onConfirm: async () => {
      try {
        await deleteInspectionGroup(record.id)
        await Promise.all([loadGroups(), loadSelectOptions()])
      } catch (error) {
        message.error(error?.message || '巡检组删除失败')
      }
    },
  })
}
function confirmDeleteTask(record) {
  openDeleteConfirm({
    title: '删除巡检任务',
    summary: '历史执行记录会保留。',
    items: [record.name],
    onConfirm: async () => {
      try {
        await deleteInspectionTask(record.id)
        await Promise.all([loadTasks(), loadSelectOptions()])
      } catch (error) {
        message.error(error?.message || '巡检任务删除失败')
      }
    },
  })
}
function confirmClearSchedule(record) {
  openDeleteConfirm({
    title: '取消定时计划',
    summary: '任务将保留，之后仅可手动触发。',
    items: [record.name],
    onConfirm: async () => {
      try {
        await saveInspectionTask({ id: record.id, inspection_name: '', cron_expression: '' })
        message.success('定时计划已取消')
        await Promise.all([loadTasks(), loadSelectOptions()])
      } catch (error) {
        message.error(error?.message || '定时计划取消失败')
      }
    },
  })
}
function handleTabChange(key) {
  if (key === 'executions') loadExecutions()
  if (key === 'schedules') loadSelectOptions()
}
function startExecutionPolling() {
  if (executionPollTimer) return
  executionPollTimer = window.setInterval(async () => {
    if (activeTab.value !== 'executions') return
    await loadExecutions()
    if (!executions.value.some((item) => ['pending', 'running'].includes(item.status))) stopExecutionPolling()
  }, 3000)
}
function stopExecutionPolling() {
  if (executionPollTimer) window.clearInterval(executionPollTimer)
  executionPollTimer = null
}

onMounted(async () => {
  executionRangePresets.value = buildUserTimezoneRangePresets(userTimezone.value)
  await Promise.all([
    loadGroups(),
    loadTasks(),
    loadSelectOptions(),
    loadServiceTree(),
    loadHostScopeTree(),
    loadExecutions(),
  ])
})
onBeforeUnmount(stopExecutionPolling)
// 本页被 keep-alive 缓存，切 tab 只会触发 onDeactivated；必须主动停轮询，否则会在后台一直请求。
// 切回时若还有未完成的执行记录，重新恢复轮询。
useKeepAliveRefreshLifecycle(() => {
  if (executions.value.some((item) => ['pending', 'running'].includes(item.status))) {
    startExecutionPolling()
  }
}, stopExecutionPolling)
</script>

<style scoped>
.inspection-page { min-height: 100%; padding: 24px; background: #f4f6f8; color: #17212b; }
.page-header { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; padding: 8px 4px 22px; border-bottom: 1px solid #d9e0e6; }
.page-header h1 { margin: 0 0 6px; font-family: "Noto Sans SC", "Microsoft YaHei", sans-serif; font-size: 28px; font-weight: 700; letter-spacing: 0; }
.page-header p { margin: 0; color: #66727d; }
.summary-strip { display: flex; gap: 1px; border: 1px solid #d9e0e6; background: #d9e0e6; }
.summary-strip div { min-width: 92px; padding: 10px 16px; background: #fff; text-align: center; }
.summary-strip strong, .summary-strip span { display: block; }
.summary-strip strong { color: #126e82; font-size: 20px; }
.summary-strip span { color: #66727d; font-size: 12px; }
.workspace-tabs { margin-top: 18px; padding: 0 20px 20px; background: #fff; border: 1px solid #e1e6ea; }
.toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 16px; }
.filter-select { width: 170px; }
.field-hint { margin-top: 4px; color: #66727d; font-size: 12px; }
.goss-spec-actions { margin-bottom: 6px; }
.scope-actions { margin-top: 8px; }
.scope-desc { margin-bottom: 12px; color: #66727d; font-size: 12px; }
.scope-summary { margin-bottom: 8px; font-weight: 600; }
.scope-search { margin-bottom: 12px; }
.scope-tree-wrap { max-height: 460px; padding: 8px; overflow: auto; border: 1px solid #e6ebf1; border-radius: 6px; }
.variable-hint { margin-bottom: 12px; }
.variable-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 2px 16px; }
.variable-item { display: flex; gap: 8px; font-size: 12px; }
.variable-item code { color: #126e82; }
.variable-item span { color: #66727d; }
.variable-note { margin-top: 8px; color: #66727d; font-size: 12px; }
.schedule-next { color: #66727d; font-size: 12px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.check-heading, .check-editor-head { display: flex; align-items: center; justify-content: space-between; }
.check-heading { margin: 4px 0 12px; font-weight: 600; }
.check-editor { margin-bottom: 12px; padding: 14px 16px 2px; border-left: 3px solid #126e82; background: #f6f8fa; }
.target-results { margin-top: 18px; }
pre { max-width: 240px; margin: 0; white-space: pre-wrap; word-break: break-word; font-size: 12px; }
@media (max-width: 760px) {
  .inspection-page { padding: 12px; }
  .page-header { align-items: flex-start; flex-direction: column; }
  .summary-strip { width: 100%; }
  .summary-strip div { flex: 1; min-width: 0; padding: 8px; }
  .form-grid { grid-template-columns: 1fr; gap: 0; }
}
</style>