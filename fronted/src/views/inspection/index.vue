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
        <a-row :gutter="16">
        <a-col :span="5">
          <a-tree
            :tree-data="taskNavTreeData"
            v-model:selected-keys="taskNavSelected"
            :block-node="true"
            default-expand-all
            @select="handleTaskNodeSelect"
          >
            <template #title="{ key, title }">
              <div class="service-tree-node">
                <FontAwesomeIcon :icon="taskNavIcon(key)" :class="['service-tree-icon', `service-tree-icon--${taskNavIconType(key)}`]" />
                <span class="service-tree-node-label">{{ title }}</span>
              </div>
            </template>
          </a-tree>
        </a-col>
        <a-col :span="19">
        <div class="toolbar">
          <a-button v-permission="'inspection:tasks:create'" size="large" @click="openTaskModal()">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增任务</span>
          </a-button>
          <a-button size="large" @click="loadTasks">
            <FontAwesomeIcon :icon="['fas', 'rotate']" />
            <span>&nbsp;刷新</span>
          </a-button>
          <a-tooltip v-if="canRunTask" :title="taskSelectedRowKeys.length ? `运行选中的 ${taskSelectedRowKeys.length} 个任务` : '先勾选要运行的任务'">
            <a-button size="large" type="primary" ghost :disabled="!taskSelectedRowKeys.length" :loading="batchRunningTasks" @click="confirmRunSelectedTasks">
              <FontAwesomeIcon :icon="['fas', 'fa-forward']" />
              <span>&nbsp;批量运行</span>
            </a-button>
          </a-tooltip>
        </div>
        <a-table
          row-key="id"
          :row-selection="{ selectedRowKeys: taskSelectedRowKeys, onChange: (keys) => (taskSelectedRowKeys = keys) }"
          :columns="taskColumns"
          :data-source="visibleTasks"
          :loading="taskLoading"
          :pagination="taskPagination"
          :scroll="{ x: 1330 }"
          @change="handleTaskTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'group_name'">
              <a-space wrap>
                <a-tooltip v-for="group in (record.groups || [])" :key="group.id" :title="(mountText(group) || '静态范围（存量）') + (canUpdateGroup ? '，点击编辑巡检组' : '')">
                  <a-tag
                    :color="group.category === 'application' ? 'purple' : 'green'"
                    :class="{ 'task-group-tag--clickable': canUpdateGroup }"
                    @click="openGroupFromTask(group)"
                  >
                    <a-spin v-if="openingGroupFromTask === group.id" :spinning="true" size="small" />
                    {{ group.name }}<template v-if="mountText(group)"> · {{ mountText(group) }}</template>
                  </a-tag>
                </a-tooltip>
              </a-space>
            </template>
            <template v-else-if="column.key === 'target'">
              <a-space>
                <a-tag :color="record.target_type === 'host_group' ? 'gold' : 'geekblue'">
                  {{ record.target_type === 'host_group' ? '主机组' : '逻辑服务' }}
                </a-tag>
                <a-tooltip v-if="record.target_type === 'host_group'" title="查看巡检范围" placement="top">
                  <span>{{ record.target_name }}</span>
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
        </a-col>
        </a-row>
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
                  <span>{{ record.target_name }}</span>
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
        <a-row :gutter="16">
        <a-col :span="5">
          <a-tree
            :tree-data="groupNavTreeData"
            v-model:selected-keys="groupNavSelected"
            :block-node="true"
            default-expand-all
            @select="handleGroupNodeSelect"
          >
            <template #title="{ key, title }">
              <div class="service-tree-node">
                <FontAwesomeIcon :icon="groupNavIcon(key)" :class="['service-tree-icon', `service-tree-icon--${groupNavIconType(key)}`]" />
                <span class="service-tree-node-label">{{ title }}</span>
              </div>
            </template>
          </a-tree>
        </a-col>
        <a-col :span="19">
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
          :data-source="visibleGroups"
          :loading="groupLoading"
          :pagination="groupPagination"
          :scroll="{ x: 900 }"
          @change="handleGroupTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'category'">
              <a-tag :color="record.category === 'application' ? 'purple' : 'green'">{{ record.category === 'application' ? (record.application_name ? `应用 · ${record.application_name}` : '应用类型') : '通用' }}</a-tag>
            </template>
            <template v-else-if="column.key === 'checks'">
              <a-space wrap>
                <a-tag
                  v-for="check in record.checks"
                  :key="check.id || check.name"
                  :color="check.severity === 'warning' ? 'orange' : 'red'"
                >
                   {{ check.name }} · {{ severityLabel(check.severity) }}
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
        </a-col>
        </a-row>

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
      width="80vw"
      style="max-width: 1200px"
      centered
      :confirm-loading="savingGroup"
      @ok="submitGroup"
    >
      <a-form layout="vertical">
        <div class="form-grid">
          <a-form-item label="巡检组名称" required><a-input v-model:value="groupForm.name" /></a-form-item>
        </div>
        <div class="form-grid">
          <a-form-item label="分类">
            <a-select v-model:value="groupForm.category" :options="groupCategoryOptions" :getPopupContainer="getPopupContainer" />
            <div class="field-hint">仅用于组织和建议过滤，不做强制约束：通用 = 所有主机适用的基线；应用类型 = 某类应用专属。</div>
          </a-form-item>
          <a-form-item v-if="groupForm.category === 'application'" label="适用应用" required>
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
        <div class="check-heading">
          <span>检查参数</span>
          <a-button size="large" @click="addParam"><FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;添加参数</a-button>
        </div>
        <div class="field-hint" style="margin-bottom:8px;">参数供检查项以 <code>${'{'}参数名${'}'}</code> 引用；任务绑定时赋值（固定值 / 引用内置变量 / 引用服务宏）。</div>
        <a-form-item v-for="(param, index) in groupForm.params" :key="index">
          <div class="form-grid">
            <a-form-item label="参数名" required class="mount-item">
              <a-input v-model:value="param.name" placeholder="如 ORACLE_SID" />
            </a-form-item>
            <a-form-item label="说明" class="mount-item"><a-input v-model:value="param.description" /></a-form-item>
            <a-form-item label="必填" class="mount-item">
              <a-switch v-model:checked="param.required" checked-children="必填" un-checked-children="选填" />
            </a-form-item>
            <a-form-item class="mount-item">
              <a-tooltip title="删除" placement="top">
                <a-button class="delBtn" size="small" type="primary" danger @click="groupForm.params.splice(index, 1)">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-tooltip>
            </a-form-item>
          </div>
        </a-form-item>
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
            <a-form-item label="严重级别" required>
              <a-segmented v-model:value="check.severity" :options="severityOptions" block />
              <div class="field-hint">警告级失败只计入汇总，不会把巡检目标判为失败。</div>
            </a-form-item>
          </div>
          <div class="check-heading">
              <span>采集命令（{{ (check.config?.input_commands || []).length }}）</span>
              <a-button size="small" @click="(check.config.input_commands ??= []).push({ key: '', exec: '', parse: 'raw' })">
                <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;添加采集命令
              </a-button>
            </div>
            <div class="field-hint" style="margin-bottom:8px;">
              每条命令的输出成为 input 的一个顶层字段，策略里用 <code>input.{{ '{' }}key{{ '}' }}</code> 引用；命令失败时整个检查项直接报错（附错误原因）。
            </div>
            <div v-for="(input, inputIndex) in (check.config?.input_commands || [])" :key="inputIndex" class="input-grid">
              <a-input v-model:value="input.key" placeholder="key，如 es_limits" />
              <a-input v-model:value="input.exec" placeholder="命令，如 cat /proc/1/limits" />
              <a-select v-model:value="input.parse" :options="opaParseOptions" :getPopupContainer="getPopupContainer" />
              <a-tooltip title="删除" placement="top">
                <a-button class="delBtn" size="small" type="primary" danger @click="check.config.input_commands.splice(inputIndex, 1)">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-tooltip>
            </div>
            <div class="check-heading">
              <span>文件采集（{{ (check.config?.input_files || []).length }}）</span>
              <a-button size="small" @click="(check.config.input_files ??= []).push({ key: '', path: '', parse: 'lines' })">
                <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />&nbsp;添加文件采集
              </a-button>
            </div>
            <div v-for="(input, inputIndex) in (check.config?.input_files || [])" :key="'f' + inputIndex" class="input-grid">
              <a-input v-model:value="input.key" placeholder="key，如 sshd_config" />
              <a-input v-model:value="input.path" placeholder="路径，如 /etc/ssh/sshd_config" />
              <a-select v-model:value="input.parse" :options="opaParseOptions" :getPopupContainer="getPopupContainer" />
              <a-tooltip title="删除" placement="top">
                <a-button class="delBtn" size="small" type="primary" danger @click="check.config.input_files.splice(inputIndex, 1)">
                  <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                </a-button>
              </a-tooltip>
            </div>
            <a-form-item label="Rego 策略" required>
              <a-textarea v-model:value="check.config.policy" :rows="10" spellcheck="false" class="opa-policy-editor" />
              <div class="field-hint">
                必须以 <code>package baseline</code> 开头，产出 <code>assertions contains &lt;元素&gt; if {{ '{' }} ... {{ '}' }}</code> 全量断言清单（OPA v1 语法；pass/fail 都展示，空集即通过）。
                元素形如 {'{'}name, pass, expected, actual{'}'}，expected/actual 进报告对应列。
                策略里引用的 <code>input.key</code> 必须与上方采集 key 一致，引用了未采集的字段保存时会直接报错。
              </div>
            </a-form-item>
            <a-form-item label="运行用户">
              <a-input v-model:value="check.config.run_user" placeholder="root" />
              <div class="field-hint">采集命令以此用户执行（su -l 登录环境）；OPA 求值本身不降权。支持 ${'{'}HOST_IP{'}'} 等变量（在命令/路径中生效）。</div>
            </a-form-item>
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
            :options="taskGroupSelectOptions"
            :getPopupContainer="getPopupContainer"
            show-search
            option-filter-prop="label"
            placeholder="选择巡检组"
            @change="handleTaskGroupsChange"
          />
          <div class="field-hint">一个任务绑定一个巡检组；通用基线和应用巡检请分别创建任务，各自挂载、各自调度。</div>
        </a-form-item>
        <a-form-item label="巡检对象" required>
          <template v-if="taskTargetSummary">
            <a-input :value="taskTargetSummary" readonly>
              <template #addonAfter>
                <span class="field-hint">在左侧树点击节点可调整</span>
              </template>
            </a-input>
          </template>
          <template v-else>
            <a-input value="未选择 —— 请先在左侧树点击项目 / 业务 / 环境 / 服务节点" readonly class="target-missing" />
          </template>
        </a-form-item>
        <a-form-item v-if="Object.keys(taskParamAssignments).length" label="巡检参数" required>
          <div v-for="(meta, paramName) in taskParamAssignments" :key="paramName" class="param-assign-row">
            <span class="param-assign-name">{{ paramName }}</span>
            <a-select v-model:value="taskParamAssignments[paramName].mode" style="width: 140px" :options="paramAssignModes" :getPopupContainer="getPopupContainer" />
            <a-input-number v-if="taskParamAssignments[paramName].mode === 'value'" v-model:value="taskParamAssignments[paramName].literal" placeholder="固定值" style="flex:1" />
            <a-input v-else v-model:value="taskParamAssignments[paramName].refName" placeholder="变量名，如 ORACLE_SID / RUN_USER" style="flex:1" />
            <span class="field-hint">{{ meta.description }}</span>
          </div>
          <div class="field-hint">固定值 = 所有目标相同；引用 = 按每个部署实例的内置变量 / 服务宏展开。</div>
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

    <a-modal v-model:open="executionDrawerOpen" title="巡检执行详情" centered width="80vw" style="max-width: 1400px" :footer="null" class="execution-detail-modal">
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
            :expand-row-by-click="true"
            v-model:expandedRowKeys="expandedResultKeys"
            :row-expandable="(record) => resultRowExpandable(record)"
          >
            <template #expandedRowRender="{ record }">
              <a-row v-if="record.expected_value != null || record.actual_value != null" :gutter="12" class="result-value-row">
                <a-col v-if="record.expected_value != null" :span="12">
                  <div class="result-value-card">
                    <div class="result-value-title">期望值</div>
                    <pre>{{ formatValue(record.expected_value) }}</pre>
                  </div>
                </a-col>
                <a-col v-if="record.actual_value != null" :span="12">
                  <div class="result-value-card">
                    <div class="result-value-title">实际值</div>
                    <pre>{{ formatValue(record.actual_value) }}</pre>
                  </div>
                </a-col>
              </a-row>
              <a-table
                v-if="(record.goss_details || []).length"
                row-key="detail_key"
                size="small"
                :pagination="false"
                :data-source="record.goss_details"
                :columns="[
                  { title: '检查项', key: 'detail_title' },
                  { title: '类型', key: 'detail_type', width: 80 },
                  { title: '检查点', key: 'detail_property', width: 110 },
                  { title: '结果', key: 'detail_status', width: 80 },
                  { title: '期望', key: 'detail_expected', width: 220 },
                  { title: '实际', key: 'detail_actual', width: 220 },
                ]"
              >
                <template #bodyCell="{ column, record: detail }">
                  <template v-if="column.key === 'detail_title'">
                    <span>{{ detail.title || detail.resource }}</span>
                  </template>
                  <template v-else-if="column.key === 'detail_type'">
                    {{ gossResourceTypeLabel(detail.resource) }}
                  </template>
                  <template v-else-if="column.key === 'detail_property'">
                    {{ gossPropertyLabel(detail.property) }}
                  </template>
                  <template v-else-if="column.key === 'detail_status'">
                    <a-tooltip :title="detail.message">
                      <a-tag :color="detail.successful ? 'green' : 'red'">{{ detail.successful ? 'pass' : 'failed' }}</a-tag>
                    </a-tooltip>
                  </template>
                  <template v-else-if="column.key === 'detail_expected'">
                    {{ formatGossExpectation(detail) }}
                  </template>
                  <template v-else-if="column.key === 'detail_actual'">
                    {{ formatGossActual(detail.actual, detail.property) }}
                  </template>
                </template>
              </a-table>
            </template>
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'name'">
                <span>{{ record.name }}</span>
                <a-tag v-if="(record.goss_details || []).length" color="blue" class="goss-detail-badge" @click.stop="toggleExpand(record)">
                  明细 {{ record.goss_details.length }} 条
                </a-tag>
                <a-tag v-else-if="resultRowExpandable(record)" class="goss-detail-badge" @click.stop="toggleExpand(record)">详情</a-tag>
              </template>
              <template v-else-if="column.key === 'status'">
                <a-tag :color="record.status === 'pass' ? 'green' : record.status === 'skipped' ? 'default' : 'red'">{{ record.status }}</a-tag>
              </template>
              <template v-else-if="column.key === 'severity'">
                <a-tag :color="record.severity === 'warning' ? 'orange' : 'red'">{{ severityLabel(record.severity) }}</a-tag>
              </template>
            </template>
          </a-table>
        </a-collapse-panel>
      </a-collapse>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
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
  getInspectionGroup,
  getInspectionGroups,
  getInspectionTasks,
  runInspectionTask,
  cancelInspectionExecution,
  saveInspectionGroup,
  saveInspectionTask,
} from '@/api/inspection'
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
const services = ref([])
const executions = ref([])
const groupLoading = ref(false)
const taskLoading = ref(false)
const executionLoading = ref(false)
const taskOptionsLoading = ref(false)
const serviceTreeLoading = ref(false)
const groupModalOpen = ref(false)
const taskModalOpen = ref(false)
const scheduleModalOpen = ref(false)
const executionDrawerOpen = ref(false)
const savingGroup = ref(false)
const savingTask = ref(false)
const savingSchedule = ref(false)
const selectedExecution = ref(null)
const runningTaskIds = reactive(new Set())
const togglingTaskId = ref(null)
const canUpdateTask = checkPermission('inspection:tasks:update')
// 任务列表里的巡检组 tag 点击后直接打开该组的编辑弹窗（方案 A：免切 tab）
const canUpdateGroup = checkPermission('inspection:groups:update')
const openingGroupFromTask = ref(-1)
async function openGroupFromTask(group) {
  if (!canUpdateGroup || !group?.id || openingGroupFromTask.value === group.id) return
  openingGroupFromTask.value = group.id
  try {
    // 任务行内嵌的 group 只有 id/name/category，编辑前必须拉完整详情（checks/params）
    const data = responseData(await getInspectionGroup(group.id))
    await openGroupModal(data)
  } catch (error) {
    message.error(error?.message || '巡检组详情加载失败')
  } finally {
    openingGroupFromTask.value = -1
  }
}
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

const emptyGroupForm = () => ({ id: null, name: '', description: '', enabled: true, category: 'general', application: undefined, params: [], checks: [] })
const emptyTaskForm = () => ({
  groups: [],
  bindings: [],
  param_values: {},
  id: null,
  name: '',
  group: undefined,
  logical_service: undefined,
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
// 组导航：通用 + 各应用（应用类型组按适用应用归组）。
const groupNavSelected = ref(['all-groups'])
const groupNavNode = computed(() => groupNavSelected.value?.[0] || 'all-groups')
const groupNavTreeData = computed(() => {
  const applicationNodes = applicationOptions.value
    .filter((application) => groupOptions.value.some((group) => String(group.application) === String(application.value)))
    .map((application) => {
      const count = groupOptions.value.filter((group) => String(group.application) === String(application.value)).length
      return { key: `app-${application.value}`, title: `${application.label}（${count}）` }
    })
  const unassignedCount = groupOptions.value.filter((group) => (group.category || 'general') === 'application' && !group.application).length
  if (unassignedCount > 0) {
    applicationNodes.push({ key: 'app-unassigned', title: `未指定应用（${unassignedCount}）` })
  }
  const generalCount = groupOptions.value.filter((group) => (group.category || 'general') === 'general').length
  return [
    { key: 'all-groups', title: '全部组' },
    { key: 'general', title: `通用（${generalCount}）` },
    ...applicationNodes,
  ]
})
function groupNavIconType(key) {
  if (key === 'all-groups') return 'all'
  if (key === 'general') return 'system'
  if (key === 'app-unassigned') return 'deployment'
  return 'service'
}
function groupNavIcon(key) {
  const type = groupNavIconType(key)
  if (type === 'all') return ['fas', 'layer-group']
  if (type === 'system') return ['fas', 'folder-tree']
  if (type === 'service') return ['fas', 'cubes']
  return ['fas', 'desktop']
}
function handleGroupNodeSelect(keys, info) {
  if (!keys.length) {
    groupNavSelected.value = [info?.node?.key ?? 'all-groups']
    return
  }
  groupNavSelected.value = keys
}
const visibleGroups = computed(() => {
  if (groupNavNode.value === 'all-groups') return groups.value
  if (groupNavNode.value === 'general') return groups.value.filter((group) => (group.category || 'general') === 'general')
  if (groupNavNode.value === 'app-unassigned') {
    return groups.value.filter((group) => (group.category || 'general') === 'application' && !group.application)
  }
  const applicationID = String(groupNavNode.value).slice(4)
  return groups.value.filter((group) => String(group.application) === applicationID)
})
const groupColumns = [
  { title: '巡检组', dataIndex: 'name', key: 'name', width: 180 },
  { title: '分类', key: 'category', width: 120 },
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
  { title: '检查项', dataIndex: 'name', key: 'name', width: 220 },
  { title: '巡检组', dataIndex: 'group_name', key: 'group_name', width: 130 },
  { title: '状态', key: 'status', width: 90 },
  { title: '级别', key: 'severity', width: 80 },
  { title: '消息', dataIndex: 'message', key: 'message' },
]

const runningCount = computed(() => executions.value.filter((item) => ['pending', 'running'].includes(item.status)).length)
const scheduledTasks = computed(() => taskOptions.value.filter((task) => task.cron_expression))
const unscheduledTaskOptions = computed(() => taskOptions.value.filter((task) => !task.cron_expression))
// 巡检唯一执行器：OPA（Rego 策略执行器，采集 + 策略求值），固定在 Agent 端执行。
const severityOptions = [
  { label: '严重', value: 'critical' },
  { label: '警告', value: 'warning' },
]
const opaParseOptions = [
  { label: 'raw 原文', value: 'raw' },
  { label: 'lines 按行', value: 'lines' },
  { label: 'json 对象', value: 'json' },
]
// 变量表与后端的变量解析一一对应，修改后端解析时需同步。
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
const scopeOptionsRemoved = [
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
const selectedTaskGroups = computed(() => (taskForm.groups || []).map((id) => groupOptions.value.find((group) => group.id === id)).filter(Boolean))
// undefined 表示尚未选巡检组，此时两类目标输入都不展示。
const groupTargetsHostGroup = computed(() => (groupForm.category || 'general') !== 'application')
const availableVariables = computed(() => (groupTargetsHostGroup.value ? HOST_VARIABLES : DEPLOYMENT_VARIABLES))
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


const responseData = (response) => response?.data?.data || {}
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const formatTime = (value) => value ? formatTimeWithTimezone(value, store.state.user?.timezone || 'Asia/Shanghai') : '-'
const groupCategoryOptions = [
  { label: '通用', value: 'general' },
  { label: '应用类型', value: 'application' },
]
const categoryLabel = (category) => (category === 'application' ? '应用类型' : '通用')
const groupById = (id) => groupOptions.value.find((group) => group.id === id)
// 巡检参数赋值：根据所选组的 params 声明生成编辑行（静态范围任务无参数）。
const taskParamAssignments = reactive({})
const paramAssignModes = [
  { label: '引用', value: 'ref' },
  { label: '固定值', value: 'value' },
]
function syncTaskParamAssignments(savedValues) {
  savedValues = savedValues || {}
  const binding = (taskForm.bindings || [])[0]
  const declarations = binding ? (groupById(binding.group_id)?.params || []) : []
  // 以组声明为准：重建全部编辑行（已保存赋值优先，缺的给默认）
  for (const key of Object.keys(taskParamAssignments)) {
    delete taskParamAssignments[key]
  }
  for (const param of declarations) {
    const saved = savedValues[param.name]
    taskParamAssignments[param.name] = {
      mode: saved?.ref ? 'ref' : 'value',
      refName: saved?.ref || param.name,
      literal: saved?.value ?? param.default ?? '',
      description: param.description || '',
    }
  }
}
// 把编辑行转成后端 param_values 形态；返回 null 表示校验失败。
function buildParamValuesPayload() {
  const result = {}
  for (const [name, row] of Object.entries(taskParamAssignments)) {
    if (row.mode === 'ref') {
      if (!row.refName?.trim()) return null
      result[name] = { ref: row.refName.trim() }
    } else {
      if (row.literal === '' || row.literal === null || row.literal === undefined) return null
      result[name] = { value: String(row.literal) }
    }
  }
  return result
}
function declaredParamsByKey(name) {
  const binding = (taskForm.bindings || [])[0]
  if (!binding) return false
  const declarations = groupById(binding.group_id)?.params || []
  return declarations.some((param) => param.name === name)
}
// 巡检对象摘要：完全由左侧树选中节点推导（新建），或由已有绑定还原（编辑）。
const taskTargetSummary = computed(() => {
  const nameOf = (list, id) => list.value.find((item) => String(item.id) === String(id))?.name || `#${id}`
  if (taskForm.bindings.length) {
    const binding = taskForm.bindings[0]
    const group = groupById(binding.group_id)
    if (!group) return ''
    const isApp = (group.category || 'general') === 'application'
    const envName = (id) => (id ? nameOf(businessEnvironments, id) : '全部环境')
    const modeText = binding.instance_mode === 'once' ? ' · 主实例（HA）' : ''
    if (binding.mount_type === 'service') {
      const service = services.value.find((item) => String(item.id) === String(binding.service_id))
      if (!service) return ''
      const system = businessSystems.value.find((item) => String(item.id) === String(service.business_system))
      const base = `逻辑服务 ${service.name}（${system?.name || '?'} / ${nameOf(businessEnvironments, service.environment)}）`
      return isApp ? `${base} 的部署实例${modeText}` : `${base} 包含的主机`
    }
    if (binding.mount_type === 'business' && binding.business_system_id) {
      return `业务 ${nameOf(businessSystems, binding.business_system_id)} @ ${envName(binding.environment_id)} 的${isApp ? `部署实例${modeText}` : '主机'}`
    }
    if (binding.project_id) {
      return `项目 ${nameOf(projects, binding.project_id)} 的全部实例主机`
    }
    return ''
  }
  return ''
})
// 任务导航树：项目 → 业务系统 → 环境 → 逻辑服务，结构与排序规则对齐服务树
// （serviceTreeData）：业务按名称排序，环境按 order 排序且只显示真实存在服务的环境，
// 逻辑服务为叶子节点。任务按挂载点归属到对应节点（可同时归属多个）。
const taskNavSelected = ref(['all'])
const taskNavNode = computed(() => taskNavSelected.value?.[0] || 'all')
// 新建任务弹窗打开期间回到树上换节点 → 巡检对象实时跟随（编辑态/静态范围不动）。
watch(taskNavNode, () => {
  if (!taskModalOpen.value || taskForm.id || !taskForm.groups?.length) return
  const group = groupById(taskForm.groups[0])
  taskForm.bindings = [defaultBinding(group)]
  syncTaskParamAssignments()
})
// 图标与配色对齐 ServiceTree.vue（项目/业务/环境/服务 同映射同色），保证两棵树观感一致。
// 树上选中的节点转成挂载上下文，新增任务时预填挂载点。
const taskNavContext = computed(() => {
  const key = String(taskNavNode.value)
  if (key.startsWith('svc-')) {
    const service = services.value.find((item) => `svc-${item.id}` === key)
    if (!service) return {}
    const system = businessSystems.value.find((item) => String(item.id) === String(service.business_system))
    return { project_id: system?.project, business_system_id: service.business_system ? Number(service.business_system) : undefined, environment_id: service.environment ? Number(service.environment) : undefined, service_id: service.id }
  }
  if (key.startsWith('biz-')) {
    const system = businessSystems.value.find((item) => `biz-${item.id}` === key)
    return system ? { project_id: system.project, business_system_id: system.id } : {}
  }
  if (key.startsWith('env-')) {
    const [, businessId, environmentId] = key.split('-')
    const system = businessSystems.value.find((item) => String(item.id) === businessId)
    return { project_id: system?.project, business_system_id: Number(businessId), environment_id: Number(environmentId) }
  }
  if (key.startsWith('proj-')) {
    return { project_id: Number(key.slice(5)) }
  }
  return {}
})
const taskNavIconType = (key) => {
  if (key === 'all') return 'all'
  if (String(key).startsWith('proj-')) return 'project'
  if (String(key).startsWith('biz-')) return 'system'
  if (String(key).startsWith('env-')) return 'environment'
  if (String(key).startsWith('svc-')) return 'service'
  return 'all'
}
const taskNavIcon = (key) => {
  const type = taskNavIconType(key)
  if (type === 'project') return ['fas', 'folder-tree']
  if (type === 'system') return ['fas', 'sitemap']
  if (type === 'environment') return ['fas', 'server']
  if (type === 'service') return ['fas', 'cubes']
  if (type === 'deployment') return ['fas', 'box-archive']
  return ['fas', 'layer-group']
}
const taskNavTreeData = computed(() => {
  const environmentsById = new Map(businessEnvironments.value.map((item) => [String(item.id), item]))
  const sortByName = (list) => [...list].sort((left, right) => String(left.name).localeCompare(String(right.name), 'zh-CN'))
  const environmentNodes = (businessId) => {
    const servicesByEnvironment = new Map()
    for (const service of services.value.filter((item) => String(item.business_system) === String(businessId))) {
      const key = String(service.environment ?? 'unassigned')
      if (!servicesByEnvironment.has(key)) servicesByEnvironment.set(key, [])
      servicesByEnvironment.get(key).push(service)
    }
    return [...servicesByEnvironment.entries()]
      .sort(([leftKey], [rightKey]) => (environmentsById.get(leftKey)?.order || 0) - (environmentsById.get(rightKey)?.order || 0))
      .map(([key, environmentServices]) => ({
        key: `env-${businessId}-${key}`,
        title: environmentsById.get(key)?.name || '未指定环境',
        children: sortByName(environmentServices).map((service) => ({
          key: `svc-${service.id}`,
          title: service.name,
          isLeaf: true,
        })),
      }))
  }
  const businessNode = (system) => ({
    key: `biz-${system.id}`,
    title: system.name,
    children: environmentNodes(system.id),
  })
  const children = sortByName(projects.value).map((project) => ({
    key: `proj-${project.id}`,
    title: project.name,
    children: sortByName(businessSystems.value.filter((system) => String(system.project) === String(project.id))).map(businessNode),
  }))
  return [
    { key: 'all', title: `全部任务`, children },
  ]
})
// 每个绑定挂载点对应的树节点路径（从深到浅），任务出现在其任一路径节点下。
function taskNavPaths(record) {
  const paths = []
  for (const group of record.groups || []) {
    const mountType = group.mount_type
    if (mountType === 'project') {
      paths.push([`proj-${group.project_id}`, 'all'])
    } else if (mountType === 'environment') {
      // 环境挂载没有业务维度，显示在项目节点下。
      paths.push([`proj-${group.project_id}`, 'all'])
    } else if (mountType === 'business') {
      const business = businessSystems.value.find((item) => String(item.id) === String(group.business_system_id))
      if (!business) { paths.push(['all']); continue }
      const projectKey = `proj-${business.project ?? business.project_id}`
      // 应用组覆盖该业务（×环境）下的具体逻辑服务：任务出现在每个被覆盖服务的节点上。
      const covered = services.value.filter((service) =>
        String(service.business_system) === String(group.business_system_id)
        && (!group.environment_id || String(service.environment) === String(group.environment_id)))
      for (const service of covered) {
        paths.push([projectKey, `biz-${business.id}`, `env-${business.id}-${String(service.environment ?? 'unassigned')}`, `svc-${service.id}`, 'all'])
      }
      if (!covered.length) {
        paths.push([projectKey, `biz-${business.id}`, 'all'])
      }
    } else if (mountType === 'service') {
      const service = services.value.find((item) => String(item.id) === String(group.service_id))
      const system = service && businessSystems.value.find((item) => String(item.id) === String(service.business_system))
      if (!service || !system) { paths.push(['all']); continue }
      paths.push([
        `proj-${system.project ?? system.project_id}`,
        `biz-${system.id}`,
        `env-${system.id}-${String(service.environment ?? 'unassigned')}`,
        `svc-${service.id}`,
        'all',
      ])
    } else {
      paths.push(['all'])
    }
  }
  if (!paths.length) paths.push(['all'])
  return paths
}
const visibleTasks = computed(() => {
  if (taskNavNode.value === 'all') return tasks.value
  return tasks.value.filter((record) => taskNavPaths(record).some((path) => path.includes(taskNavNode.value)))
})
function handleTaskNodeSelect(keys, info) {
  // 再点已选中节点时 antd 会取消选中（v-model 已清空），这里回填点击节点的 key 保持选中
  if (!keys.length) {
    taskNavSelected.value = [info?.node?.key ?? 'all']
    return
  }
  taskNavSelected.value = keys
}
const mountText = (group) => {
  if (!group) return ''
  const nameOf = (list, id) => list.value.find((item) => String(item.id) === String(id))?.name || `#${id}`
  if (group.mount_type === 'project') return `项目: ${nameOf(projects, group.project_id)}`
  if (group.mount_type === 'environment') return `环境: ${nameOf(projects, group.project_id)} / ${nameOf(businessEnvironments, group.environment_id)}`
  if (group.mount_type === 'business') {
    const env = group.environment_id ? `@${nameOf(businessEnvironments, group.environment_id)}` : ''
    return `业务: ${nameOf(businessSystems, group.business_system_id)}${env} · ${group.instance_mode === 'once' ? '主实例' : '全部实例'}`
  }
  if (group.mount_type === 'service') {
    const service = services.value.find((item) => String(item.id) === String(group.service_id))
    if (!service) return `逻辑服务（${group.service_id ?? '标识缺失，请重新保存任务'}）· ${group.instance_mode === 'once' ? '主实例' : '全部实例'}`
    return `逻辑服务: ${service.name} · ${group.instance_mode === 'once' ? '主实例' : '全部实例'}`
  }
  return ''
}
const projectOptions = computed(() => projects.value.map((item) => ({ label: item.name, value: item.id })))
const businessOptions = computed(() => businessSystems.value.map((item) => ({ label: item.name, value: item.id })))
const environmentOptions = computed(() => businessEnvironments.value.map((item) => ({ label: item.name, value: item.id })))
const appMountOptions = [
  { label: '业务系统（×环境，全部同类服务）', value: 'business' },
  { label: '逻辑服务（精确到单个服务）', value: 'service' },
]
function handleAppMountChange(binding) {
  binding.business_system_id = undefined
  binding.environment_id = undefined
  binding.service_id = undefined
}
const generalMountOptions = [
  { label: '项目（全部实例主机）', value: 'project' },
  { label: '环境（项目×环境实例主机）', value: 'environment' },
]
const instanceModeOptions = [
  { label: '全部实例', value: 'all' },
  { label: '主实例（HA 场景，选一台在线实例）', value: 'once' },
]
function defaultBinding(group) {
  // 挂载点由树节点决定，与组类型无关：
  //   服务节点 → 该服务；业务/环境节点 → 业务×环境；项目节点 → 项目。
  // 组类型只影响解析粒度（应用组按部署实例展开变量），instance_mode 仅应用组有。
  const context = taskNavContext.value
  const isApp = (group.category || 'general') === 'application'
  const mode = isApp ? 'all' : undefined
  if (context.service_id) {
    return { group_id: group.id, mount_type: 'service', service_id: context.service_id, instance_mode: mode }
  }
  if (context.business_system_id) {
    return { group_id: group.id, mount_type: 'business', business_system_id: context.business_system_id, environment_id: context.environment_id, instance_mode: mode }
  }
  return { group_id: group.id, mount_type: 'project', project_id: context.project_id ?? projects.value[0]?.id, environment_id: undefined }
}
const taskGroupSelectOptions = computed(() => {
  const context = taskNavContext.value
  // 目标为逻辑服务时，应用类型巡检组只保留与该服务所属应用一致的：
  // 组的"适用应用"(group.application) 必须等于服务的所属应用(service.application)。
  const targetService = context.service_id
    ? services.value.find((item) => String(item.id) === String(context.service_id))
    : null
  const pick = (category) => groupOptions.value
    .filter((group) => (group.category || 'general') === category)
    .filter((group) => (
      category !== 'application' || !targetService
      || String(group.application ?? '') === String(targetService.application ?? '')
    ))
    .map((group) => ({ label: group.name, value: group.id }))
  const options = []
  if (pick('general').length) options.push({ label: '通用巡检组', options: pick('general') })
  if (pick('application').length) options.push({ label: '应用类型巡检组', options: pick('application') })
  return options.length ? options : groupOptions.value.map((group) => ({ label: group.name, value: group.id }))
})
// 切换巡检对象后，已选巡检组可能不再出现在候选里（如应用组与应用不匹配），自动清掉防止提交脏数据
watch(taskGroupSelectOptions, (options) => {
  const current = taskForm.groups?.[0]
  if (current && !options.some((option) => option.value === current || option.options?.some((child) => child.value === current))) {
    taskForm.groups = []
    taskForm.bindings = []
  }
})
function handleTaskGroupsChange(value) {  // 单组模型：选择即替换，绑定数组至多一项。
  // a-select 单选 change 的参数是标量（多选才是数组），v-model 已把标量写入
  // taskForm.groups，这里统一规范回数组，后续 length/下标访问才成立。
  const id = Array.isArray(value) ? value[0] : value
  taskForm.groups = id ? [id] : []
  if (!id) {
    taskForm.bindings = []
    return
  }
  const existing = (taskForm.bindings || []).find((binding) => binding.group_id === id)
  taskForm.bindings = [existing || defaultBinding(groupById(id) || { id, category: 'general' })]
  syncTaskParamAssignments()
}
const scopeLabel = (scope) => ({
  per_deployment: '逻辑服务·每个部署实例',
  service_once: '逻辑服务·服务单次',
  per_host: '主机组·每台主机',
}[scope] || scope)
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
    // goss 检查项的子测试明细（含通过的），展开行逐条展示 pass/fail
    goss_details: filterGossDetails((Array.isArray(check.actual?.details) ? check.actual.details : [])).map((detail, detailIndex) => ({
      ...detail,
      detail_key: `${check.key || 'check'}-${index}-${detailIndex}`,
    })),
    message: check.message,
  }))
const targetDisplayResults = (target) => {
  const rows = target.results?.length ? target.results : rawTargetResults(target)
  return rows.map((row, index) => {
    // 后端 results[].actual_value 是 JSON 字符串（goss 的 details 在其中），统一解析后取明细
    let actual = row.actual_value
    if (typeof actual === 'string') {
      try { actual = JSON.parse(actual) } catch { /* 非法 JSON 保持原样展示 */ }
    }
    const details = Array.isArray(actual?.details) ? filterGossDetails(actual.details) : (row.goss_details || [])
    // 明细可能来自后端存档（actual.details）或本地原始回传（goss_details），两路都做 exit-status 折叠
    const normalizedDetails = details.map((detail, detailIndex) => ({ ...detail, detail_key: `${row.check_key || 'check'}-${index}-${detailIndex}` }))
    return {
      ...row,
      actual_value: actual ?? row.actual_value,
      goss_details: normalizedDetails,
    }
  })
}
const targetErrorMessage = (target) => target.error_message || rawTargetResults(target).find((check) => check.status === 'error')?.message || ''
// 检查项行展开状态（受控）：点击"明细 N 条"标签也能展开/收起
const expandedResultKeys = ref([])
function toggleExpand(record) {
  const key = record.check_key
  expandedResultKeys.value = expandedResultKeys.value.includes(key)
    ? expandedResultKeys.value.filter((item) => item !== key)
    : [...expandedResultKeys.value, key]
}
// 有 goss 明细或有期望/实际值的行才可展开
function resultRowExpandable(record) {
  return (record.goss_details || []).length > 0
    || (record.expected_value != null && record.expected_value !== 'null')
    || (record.actual_value != null && record.actual_value !== 'null')
}
// ---- goss 明细的语义化展示（词表未覆盖时回退原文，纯显示层转换） ----
const gossResourceTypeLabels = {
  Port: '端口', Command: '命令', Process: '进程', File: '文件',
  User: '用户', Group: '用户组', Package: '软件包', Addr: '地址',
  Service: '服务', DNS: 'DNS', HTTP: 'HTTP', Interface: '网卡', KernelParam: '内核参数',
}
const gossPropertyLabels = {
  assertion: '断言',
  listening: '端口监听', 'exit-status': '退出码', stdout: '标准输出', stderr: '标准错误',
  running: '运行状态', installed: '已安装', exists: '存在', enabled: '已启用',
  reachable: '可达', mode: '权限', owner: '属主', group: '属组', contents: '内容',
  size: '大小', type: '类型', uid: 'UID', gid: 'GID', home: '主目录', shell: 'Shell',
}
function gossResourceTypeLabel(resource) {
  const type = String(resource || '').split(':')[0]
  return gossResourceTypeLabels[type] || type
}
function gossPropertyLabel(property) {
  return gossPropertyLabels[property] || property
}

// command 资源带 exit-status + stdout 等多个断言时，exit-status 只表示"命令能执行"，
// 与 stdout 断言重复展示没有信息量；只有它失败（命令本身异常）或没有其他断言可看时才展示。
function filterGossDetails(details) {
  const byResource = new Map()
  for (const detail of details) {
    const key = String(detail.resource || '')
    if (!byResource.has(key)) byResource.set(key, [])
    byResource.get(key).push(detail)
  }
  return details.filter((detail) => {
    if (detail.property !== 'exit-status') return true
    const siblings = byResource.get(String(detail.resource || '')) || []
    const hasOtherAssertions = siblings.some((item) => item !== detail && item.property !== 'exit-status')
    return !hasOtherAssertions || !detail.successful
  })
}
function formatGossExpectation(detail) {
  const value = detail.expected
  switch (detail.property) {
    case 'assertion':
      return value != null && value !== '' ? value : '—' 
    case 'listening':
      return value === true ? '端口处于监听状态' : '端口未监听'
    case 'exit-status':
      return typeof value === 'number' ? `退出码等于 ${value}` : `退出码：${formatValue(value)}`
    case 'stdout':
    case 'stderr': {
      const patterns = Array.isArray(value) ? value : [value]
      const joined = patterns.map((item) => `"${item}"`).join('、')
      return patterns.length > 1 ? `输出同时包含 ${joined}` : `输出包含 ${joined}`
    }
    case 'running':
      return value === true ? '进程运行中' : '进程未运行'
    case 'installed':
      return value === true ? '已安装' : '未安装'
    case 'exists':
      return value === true ? '存在' : '不存在'
    default:
      return formatValue(value)
  }
}
function formatGossActual(value, property) {
  // OPA 违规行：actual 是 violation.item 对象，直接展示策略回传的真实实际值
  if (property === 'violation' && value && typeof value === 'object') {
    return value.actual != null ? formatValue(value.actual) : '-'
  }
  // goss 对命令输出类实际值序列化不出来（{}）或降级为 Go 类型名（bytes.Reader），
  // 这是 goss 的已知限制——stdout 的实际值不回传，只能从失败 message 看差异。
  if (value == null || (typeof value === 'object' && Object.keys(value).length === 0)) return 'goss 不回传输出实际值'
  if (typeof value === 'string') return value.includes('bytes.Reader') ? 'goss 不回传输出实际值' : value
  return formatValue(value)
}

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

function defaultOpaConfig() {
  return {
    input_commands: [],
    input_files: [],
    run_user: '',
    policy: 'package baseline\n\n# 全量断言清单：每条断言 pass/fail 都进报告，空集即通过。\n# OPA v1 语法：assertions contains <元素> if { ... }。\nassertions contains assertion if {\n\tassertion := {"name": "示例断言", "pass": input.<采集key>.raw == "期望值",\n\t\t"expected": "期望值", "actual": input.<采集key>.raw}\n}',
  }
}
function addCheck() {
  groupForm.checks.push({
    localKey: ++localKey,
    name: '',
    config: defaultOpaConfig(),
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
function addParam() {
  groupForm.params.push({ name: '', description: '', required: false, default: '' })
}
function openGroupModal(record) {
  ensureApplicationOptions()
  Object.assign(groupForm, emptyGroupForm(), record ? JSON.parse(JSON.stringify(record)) : {})
  // 存量数据 params/checks 可能为 null（建组时未填），归一化成数组避免后续遍历崩掉。
  groupForm.params = Array.isArray(groupForm.params) ? groupForm.params : []
  groupForm.checks = Array.isArray(groupForm.checks) ? groupForm.checks : []
  groupForm.checks = (groupForm.checks || []).map((check) => ({
    ...check,
    localKey: ++localKey,
    severity: check.severity || 'critical',
    config: {
      ...defaultOpaConfig(),
      ...(check.config || {}),
      input_commands: (check.config && Array.isArray(check.config.input_commands)) ? check.config.input_commands : [],
      input_files: (check.config && Array.isArray(check.config.input_files)) ? check.config.input_files : [],
      policy: check.config?.policy || '',
      run_user: check.config?.run_user || '',
    },
  }))
  if (!groupForm.checks.length) addCheck()
  groupModalOpen.value = true
}
function openTaskModal(record) {
  Object.assign(taskForm, emptyTaskForm(), record ? JSON.parse(JSON.stringify(record)) : {})
  // 编辑时把组对象（含挂载点）转成本地绑定配置；兼容仅返回单组 group 的旧数据。
  taskForm.groups = (record?.groups?.length ? record.groups.map((group) => group.id) : (record?.group ? [record.group] : []))
  taskForm.bindings = (record?.groups?.length
    ? record.groups.map((group) => ({
        group_id: group.id,
        mount_type: group.mount_type,
        project_id: group.project_id ?? undefined,
        environment_id: group.environment_id ?? undefined,
        business_system_id: group.business_system_id ?? undefined,
        service_id: group.service_id ?? undefined,
        instance_mode: group.instance_mode || ((group.mount_type === 'business' || group.mount_type === 'service') ? 'all' : undefined),
        param_values: group.param_values ?? undefined,
      }))
    : [])
  // 编辑：还原已保存的参数赋值（组声明为骨架，已保存值优先）
  const savedGroup = (record?.groups || [])[0]
  let savedValues = {}
  if (savedGroup?.param_values) {
    try {
      savedValues = typeof savedGroup.param_values === 'string' ? JSON.parse(savedGroup.param_values) : savedGroup.param_values
    } catch { savedValues = {} }
  }
  syncTaskParamAssignments(savedValues)
  // 新建时若树上有选中节点，选组后即按树上下文生成挂载（handleTaskGroupsChange 处理）。
  taskModalOpen.value = true
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
  if (groupForm.category === 'application' && !groupForm.application) {
    message.warning('应用类型巡检组必须选择适用应用')
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
  const builtins = ['APP_HOME', 'RUN_USER', 'INSTANCE_NAME', 'APPLICATION_VERSION', 'SERVICE_NAME']
  const conflict = groupForm.params.find((param) => builtins.includes((param.name || '').trim()))
  if (conflict) {
    message.warning(`检查参数 ${conflict.name} 与内置变量重名，请换个名字`)
    return
  }
  savingGroup.value = true
  try {
    const payload = JSON.parse(JSON.stringify(groupForm))
    payload.checks.forEach((check, index) => {
      delete check.localKey; delete check.id; check.order = index
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
  const hasTarget = !!taskTargetSummary.value
  if (!taskForm.name.trim() || !taskForm.groups?.length || !hasTarget) {
    message.warning('请完整填写任务信息，并在左侧树选择巡检对象')
    return
  }
  const paramValues = buildParamValuesPayload()
  if (paramValues === null) {
    message.warning('巡检参数赋值不完整：引用需填变量名，固定值不能为空')
    return
  }
  savingTask.value = true
  try {
    const payload = { ...taskForm, group: taskForm.groups[0], bindings: (taskForm.bindings || []).map((binding) => ({ ...binding, param_values: paramValues || {} })) }
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
// 批量运行：逐个提交选中任务，部分失败不影响其余任务
const canRunTask = checkPermission('inspection:tasks:run')
const taskSelectedRowKeys = ref([])
const batchRunningTasks = ref(false)
function confirmRunSelectedTasks() {
  const ids = [...taskSelectedRowKeys.value]
  if (!ids.length) return
  Modal.confirm({
    title: '批量运行巡检任务',
    content: `确定运行选中的 ${ids.length} 个巡检任务吗？`,
    onOk: async () => {
      batchRunningTasks.value = true
      const failed = []
      try {
        for (const id of ids) {
          try {
            await runInspectionTask(id)
          } catch (error) {
            failed.push(`#${id}: ${error?.message || '提交失败'}`)
          }
        }
        if (failed.length) {
          message.warning(`已提交 ${ids.length - failed.length}/${ids.length} 个任务，失败：${failed.join('；')}`)
        } else {
          message.success(`已提交 ${ids.length} 个巡检任务`)
        }
        taskSelectedRowKeys.value = []
        activeTab.value = 'executions'
        executionPagination.current = 1
        await loadExecutions()
        startExecutionPolling()
      } finally {
        batchRunningTasks.value = false
      }
    },
  })
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
  if (key === 'tasks' || key === 'schedules') loadSelectOptions()
  // 组导航树按适用应用分组，需要应用列表
  if (key === 'groups') ensureApplicationOptions()
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
    loadExecutions(),
    ensureApplicationOptions(),
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
.target-missing :deep(input) { color: #b0b7c0; }
.param-assign-row { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.param-assign-name { min-width: 120px; font-weight: 600; }
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
.input-grid { display: grid; grid-template-columns: minmax(150px, 200px) 1fr minmax(110px, 150px) auto; gap: 8px; align-items: center; margin-bottom: 8px; }
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
.service-tree-node {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
}
.service-tree-icon {
  flex: none;
}
.service-tree-icon--all { color: #5b6472; }
.service-tree-icon--project { color: #36709b; }
.service-tree-icon--system { color: #25856d; }
.service-tree-icon--environment { color: #b56d2d; }
.service-tree-icon--service { color: #ad6800; }
.service-tree-icon--deployment { color: #8c8c8c; }
.service-tree-node-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.task-group-tag--clickable {
  cursor: pointer;
}
.task-group-tag--clickable:hover {
  opacity: 0.8;
  text-decoration: underline;
}
.goss-detail-badge {
  margin-left: 6px;
  cursor: pointer;
}
.result-value-row {
  margin-bottom: 8px;
}
.result-value-card {
  height: 100%;
  padding: 8px 10px;
  border: 1px solid #eef1f5;
  border-radius: 6px;
  background: #fafbfc;
}
.result-value-title {
  margin-bottom: 4px;
  color: #687386;
  font-size: 12px;
  font-weight: 600;
}
.result-value-card pre {
  max-height: 280px;
  margin: 0;
  overflow: auto;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
