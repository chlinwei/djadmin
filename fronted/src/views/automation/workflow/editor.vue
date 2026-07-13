<template>
  <div class="workflow-editor-page">
    <div class="editor-toolbar">
      <div class="editor-title">
        <span class="name">{{ form.name || 'Workflow 编排' }}</span>
        <a-tag :color="form.enabled ? 'green' : 'default'">{{ form.enabled ? '启用' : '禁用' }}</a-tag>
      </div>
      <a-space>
        <a-button @click="goBack">返回列表</a-button>
        <a-button @click="basicInfoVisible = true">基础信息</a-button>
        <a-button @click="openAddNodeWizard({ presetNodeType: 'task', forceRoot: true })">添加任务节点</a-button>
        <a-button type="primary" :loading="submitting" @click="saveWorkflow">保存</a-button>
      </a-space>
    </div>

    <a-skeleton active :loading="pageLoading">
      <div ref="canvasWrapRef" class="canvas-wrap">
        <VueFlow
          v-model:nodes="flowNodes"
          v-model:edges="flowEdges"
          :fit-view-on-init="true"
          :nodes-draggable="true"
          :nodes-connectable="true"
          :elements-selectable="true"
          :connection-mode="ConnectionMode.Strict"
          :class="['workflow-canvas', { 'is-connecting': isConnecting }]"
          @connect="handleConnect"
          @connect-start="handleConnectStart"
          @connect-end="handleConnectEnd"
          @node-click="handleNodeClick"
          @edge-click="handleEdgeClick"
          @pane-click="clearSelection"
        >
          <template #node-workflow-start-node>
            <div class="workflow-start-node-card">
              <Handle
                type="source"
                :position="Position.Right"
                :connectable="false"
                :connectable-start="false"
                :connectable-end="false"
              />
              START
            </div>
          </template>
          <template #node-workflow-node="nodeProps">
            <div class="workflow-node-wrap">
              <div class="workflow-node-card" @click.stop="handleNodeCardClick(nodeProps.id)">
                <Handle
                  type="target"
                  :position="Position.Left"
                  class="workflow-target-handle"
                  :connectable-start="false"
                  :connectable-end="true"
                />
                <Handle
                  type="source"
                  :position="Position.Right"
                  class="workflow-source-handle"
                  :connectable-start="true"
                  :connectable-end="false"
                />

                <div
                  class="workflow-node-type-icon"
                  :class="nodeProps.data?.node_type === 'workflow' ? 'is-workflow' : 'is-task'"
                  :title="nodeProps.data?.node_type === 'workflow' ? 'Workflow 节点' : '任务节点'"
                >
                  <ApartmentOutlined v-if="nodeProps.data?.node_type === 'workflow'" />
                  <ToolOutlined v-else />
                </div>
                <div
                  v-if="String(nodeProps.data?.convergence || 'any').toLowerCase() === 'all'"
                  class="workflow-node-convergence-tag"
                >
                  ALL
                </div>
                <div
                  v-if="resolveNodeEnableStatus(nodeProps.data).visible"
                  class="workflow-node-enable-tag"
                  :class="{
                    'is-enabled': resolveNodeEnableStatus(nodeProps.data).status === 'enabled',
                    'is-disabled': resolveNodeEnableStatus(nodeProps.data).status === 'disabled',
                    'is-missing': resolveNodeEnableStatus(nodeProps.data).status === 'missing',
                  }"
                >
                  {{ resolveNodeEnableStatus(nodeProps.data).label }}
                </div>

                <div v-if="nodeProps.data?.node_type === 'task'" class="workflow-node-quick-actions">
                  <span class="node-quick-ref-name" :title="resolveTaskNameByNodeData(nodeProps.data)">
                    {{ resolveTaskNameByNodeData(nodeProps.data) }}
                  </span>
                  <button
                    type="button"
                    class="node-quick-btn"
                    :title="resolveTaskNameByNodeData(nodeProps.data)"
                    @click.stop="openTaskDetailByNodeId(nodeProps.id)"
                  >
                    详细
                  </button>
                </div>
                <div v-else-if="nodeProps.data?.node_type === 'workflow'" class="workflow-node-quick-actions">
                  <span class="node-quick-ref-name" :title="resolveWorkflowNameByNodeData(nodeProps.data)">
                    {{ resolveWorkflowNameByNodeData(nodeProps.data) }}
                  </span>
                  <button
                    type="button"
                    class="node-quick-btn"
                    :title="resolveWorkflowNameByNodeData(nodeProps.data)"
                    @click.stop="openWorkflowDetailByNodeId(nodeProps.id)"
                  >
                    详细
                  </button>
                </div>

                <div class="workflow-node-tools" :class="{ visible: selectedNodeId === nodeProps.id }">
                  <button type="button" class="tool-btn" title="添加后续节点" @click.stop="openAddNodeWizard({ parentNodeId: nodeProps.id })">+</button>
                  <button type="button" class="tool-btn" title="编辑节点配置" @click.stop="openNodeEditDialog(nodeProps.id)">✎</button>
                  <button type="button" class="tool-btn danger" title="删除节点" @click.stop="removeNode(nodeProps.id)">🗑</button>
                </div>
              </div>
              <div class="workflow-node-name" @click.stop="handleNodeCardClick(nodeProps.id)">{{ nodeProps.data?.name || nodeProps.id }}</div>
            </div>
          </template>
          <Background pattern-color="#d9d9d9" :gap="18" />
        </VueFlow>

        <div
          v-if="selectedEditableEdge"
          class="edge-quick-editor"
          :style="edgeQuickEditorStyle"
        >
          <button type="button" class="edge-action-item" @click="setSelectedEdgeCondition('success')">
            <span class="edge-action-dot dot-success">✓</span>
            <span class="edge-action-label">Run on success</span>
            <span v-if="selectedEdgeCondition === 'success'" class="edge-action-check">✓</span>
          </button>
          <button type="button" class="edge-action-item" @click="setSelectedEdgeCondition('always')">
            <span class="edge-action-dot dot-always"></span>
            <span class="edge-action-label">Run always</span>
            <span v-if="selectedEdgeCondition === 'always'" class="edge-action-check">✓</span>
          </button>
          <button type="button" class="edge-action-item" @click="setSelectedEdgeCondition('failure')">
            <span class="edge-action-dot dot-failure">!</span>
            <span class="edge-action-label">Run on fail</span>
            <span v-if="selectedEdgeCondition === 'failure'" class="edge-action-check">✓</span>
          </button>
          <div class="edge-action-divider"></div>
          <button type="button" class="edge-action-item edge-action-delete" @click="removeSelectedEdge">
            <span class="edge-action-dot dot-delete">🗑</span>
            <span class="edge-action-label">Remove link</span>
          </button>
        </div>
      </div>
    </a-skeleton>

    <a-modal
      title="Add Node"
      :open="addNodeWizardVisible"
      :mask-closable="false"
      :closable="true"
      ok-text="创建节点"
      cancel-text="Cancel"
      @ok="createNodeFromWizard"
      @cancel="closeAddNodeWizard"
    >
      <div class="add-node-wizard-layout">
        <div class="add-node-wizard-content">
          <div v-if="addNodeWizardHasParentCondition" class="wizard-section">
            <div class="run-type-title">运行条件</div>
            <div class="run-type-desc">设置新节点在父节点什么状态下执行。</div>
            <a-alert
              v-if="!addNodeWizardForm.parent_node_key"
              type="info"
              show-icon
              class="run-type-alert"
              message="当前是首个节点，无父节点，执行条件固定为 success。"
            />
            <div class="run-type-grid">
              <div
                class="run-type-card"
                :class="{ active: addNodeWizardForm.run_type === 'success', disabled: !addNodeWizardForm.parent_node_key }"
                @click="selectRunType('success')"
              >
                <div class="card-title">On Success</div>
                <div class="card-desc">当父节点执行成功时，才执行当前节点。</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: addNodeWizardForm.run_type === 'failure', disabled: !addNodeWizardForm.parent_node_key }"
                @click="selectRunType('failure')"
              >
                <div class="card-title">On Failure</div>
                <div class="card-desc">当父节点执行失败时，才执行当前节点。</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: addNodeWizardForm.run_type === 'always', disabled: !addNodeWizardForm.parent_node_key }"
                @click="selectRunType('always')"
              >
                <div class="card-title">Always</div>
                <div class="card-desc">无论父节点最终状态如何，都执行当前节点。</div>
              </div>
            </div>
          </div>

          <div class="wizard-section">
            <a-form layout="vertical">
              <a-form-item label="节点类型" required>
                <a-radio-group v-model:value="addNodeWizardForm.node_type">
                  <a-radio-button value="task">任务节点</a-radio-button>
                  <a-radio-button value="workflow">Workflow节点</a-radio-button>
                </a-radio-group>
              </a-form-item>

              <a-form-item label="节点名称" required>
                <a-input v-model:value="addNodeWizardForm.name" placeholder="例如：任务节点" />
              </a-form-item>

              <a-form-item v-if="addNodeWizardForm.node_type === 'task'" label="执行任务" required>
                <a-select
                  v-model:value="addNodeWizardForm.task_id"
                  :getPopupContainer="getPopupContainer"
                  :options="taskOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择任务"
                />
                <div v-if="taskOptions.length === 0" class="wizard-help-link">
                  当前暂无可选任务，请先去
                  <a @click="goToTaskPage">任务管理</a>
                  创建任务。
                </div>
              </a-form-item>

              <a-form-item v-else label="选择Workflow" required>
                <a-select
                  v-model:value="addNodeWizardForm.workflow_id"
                  :getPopupContainer="getPopupContainer"
                  :options="workflowOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择Workflow"
                />
              </a-form-item>

              <a-form-item label="Convergence">
                <a-select v-model:value="addNodeWizardForm.convergence" :getPopupContainer="getPopupContainer" :options="convergenceOptions" />
              </a-form-item>

            </a-form>
          </div>
        </div>
      </div>
    </a-modal>

    <a-modal
      title="基础信息"
      :open="basicInfoVisible"
      :confirm-loading="submitting"
      ok-text="确认"
      cancel-text="取消"
      @ok="handleBasicInfoConfirm"
      @cancel="basicInfoVisible = false"
    >
      <a-form layout="vertical">
        <a-form-item label="名称" required>
          <a-input v-model:value="form.name" placeholder="例如：生产发布编排" />
        </a-form-item>
        <a-form-item label="启用状态">
          <a-switch v-model:checked="form.enabled" checked-children="启用" un-checked-children="禁用" />
        </a-form-item>
        <a-form-item label="描述">
          <a-input v-model:value="form.description" placeholder="可选" />
        </a-form-item>
        <a-form-item label="选择Inventory">
          <a-select
            v-model:value="form.default_inventory"
            :getPopupContainer="getPopupContainer"
            :options="inventoryOptions"
            show-search
            optionFilterProp="label"
            placeholder="可选"
            allow-clear
          />
        </a-form-item>
        <a-form-item label="默认 Limit">
          <a-input v-model:value="form.default_limit" :placeholder="LIMIT_INPUT_PLACEHOLDER" />
          <ScopePrecheckPanel
            :precheck-ok="basicLimitPrecheckOk"
            :prechecking="basicLimitPrechecking"
            :message="basicLimitPrecheckText"
            :hosts="basicLimitAllHosts"
            :matched-hosts="basicLimitMatchedHosts"
            :show-host-link="true"
            :show-limit-toggle="true"
            :show-target-filter="true"
            :limit-text="form.default_limit"
            @host-click="handleBasicLimitHostClick"
            @toggle-limit-host="handleBasicLimitLimitToggle"
            @remove-limit-token="handleBasicLimitRemoveToken"
          />
        </a-form-item>
        <a-form-item label="默认变量 JSON">
          <a-textarea v-model:value="form.default_extra_vars_text" :rows="3" placeholder='例如：{"env":"prod"}' />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      title="节点配置"
      :open="nodeConfigVisible"
      ok-text="保存"
      cancel-text="取消"
      @ok="saveNodeConfig"
      @cancel="nodeConfigVisible = false"
    >
      <div class="add-node-wizard-layout">
        <div class="add-node-wizard-content">
          <div v-if="nodeConfigHasParentCondition" class="wizard-section">
            <div class="run-type-title">运行条件</div>
            <div class="run-type-desc">设置当前节点在父节点什么状态下执行。</div>
            <a-alert
              v-if="!nodeConfigRunTypeEditable"
              type="info"
              show-icon
              class="run-type-alert"
              :message="nodeConfigRunTypeHint"
            />
            <div class="run-type-grid">
              <div
                class="run-type-card"
                :class="{ active: nodeConfigForm.run_type === 'success', disabled: !nodeConfigRunTypeEditable }"
                @click="selectNodeConfigRunType('success')"
              >
                <div class="card-title">On Success</div>
                <div class="card-desc">当父节点执行成功时，才执行当前节点。</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: nodeConfigForm.run_type === 'failure', disabled: !nodeConfigRunTypeEditable }"
                @click="selectNodeConfigRunType('failure')"
              >
                <div class="card-title">On Failure</div>
                <div class="card-desc">当父节点执行失败时，才执行当前节点。</div>
              </div>
              <div
                class="run-type-card"
                :class="{ active: nodeConfigForm.run_type === 'always', disabled: !nodeConfigRunTypeEditable }"
                @click="selectNodeConfigRunType('always')"
              >
                <div class="card-title">Always</div>
                <div class="card-desc">无论父节点最终状态如何，都执行当前节点。</div>
              </div>
            </div>
          </div>

          <div class="wizard-section">
            <a-form layout="vertical">
              <a-form-item label="节点类型" required>
                <a-radio-group v-model:value="nodeConfigForm.node_type">
                  <a-radio-button value="task">任务节点</a-radio-button>
                  <a-radio-button value="workflow">Workflow节点</a-radio-button>
                </a-radio-group>
              </a-form-item>

              <a-form-item label="节点名称" required>
                <a-input v-model:value="nodeConfigForm.name" />
              </a-form-item>

              <a-form-item v-if="nodeConfigForm.node_type === 'task'" label="执行任务" required>
                <a-select
                  v-model:value="nodeConfigForm.task_id"
                  :getPopupContainer="getPopupContainer"
                  :options="taskOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择任务"
                />
              </a-form-item>

              <a-form-item v-else label="选择Workflow" required>
                <a-select
                  v-model:value="nodeConfigForm.workflow_id"
                  :getPopupContainer="getPopupContainer"
                  :options="workflowOptions"
                  show-search
                  optionFilterProp="label"
                  placeholder="请选择Workflow"
                />
              </a-form-item>

              <a-form-item label="Convergence">
                <a-select v-model:value="nodeConfigForm.convergence" :getPopupContainer="getPopupContainer" :options="convergenceOptions" />
              </a-form-item>

            </a-form>
          </div>
        </div>
      </div>
    </a-modal>

  </div>
</template>

<script setup>
import { ApartmentOutlined, ToolOutlined } from '@ant-design/icons-vue'
import { VueFlow, Handle, Position, ConnectionMode } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import ScopePrecheckPanel from '../components/ScopePrecheckPanel.vue'
import { LIMIT_INPUT_PLACEHOLDER } from '../utils/scopeHelpers'
import { useWorkflowEditorController } from './editorController'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import './editor.css'

const {
  getPopupContainer,
  pageLoading,
  submitting,
  form,
  flowNodes,
  flowEdges,
  canvasWrapRef,
  selectedNodeId,
  isConnecting,
  convergenceOptions,
  taskOptions,
  workflowOptions,
  inventoryOptions,
  selectedEditableEdge,
  selectedEdgeCondition,
  edgeQuickEditorStyle,
  addNodeWizardVisible,
  basicInfoVisible,
  nodeConfigVisible,
  basicLimitPrechecking,
  basicLimitPrecheckOk,
  basicLimitAllHosts,
  basicLimitMatchedHosts,
  nodeConfigForm,
  basicLimitPrecheckText,
  nodeConfigHasParentCondition,
  nodeConfigRunTypeEditable,
  nodeConfigRunTypeHint,
  addNodeWizardForm,
  addNodeWizardHasParentCondition,
  handleNodeCardClick,
  openTaskDetailByNodeId,
  openWorkflowDetailByNodeId,
  resolveTaskNameByNodeData,
  resolveWorkflowNameByNodeData,
  resolveNodeEnableStatus,
  openNodeEditDialog,
  removeNode,
  setSelectedEdgeCondition,
  removeSelectedEdge,
  closeAddNodeWizard,
  selectRunType,
  createNodeFromWizard,
  handleBasicInfoConfirm,
  handleBasicLimitHostClick,
  handleBasicLimitLimitToggle,
  handleBasicLimitRemoveToken,
  saveNodeConfig,
  selectNodeConfigRunType,
  openAddNodeWizard,
  goToTaskPage,
  goBack,
  saveWorkflow,
  handleConnect,
  handleConnectStart,
  handleConnectEnd,
  handleNodeClick,
  handleEdgeClick,
  clearSelection,
} = useWorkflowEditorController()
</script>
