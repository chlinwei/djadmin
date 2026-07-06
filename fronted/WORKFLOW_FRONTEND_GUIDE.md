# Workflow 前端说明

适用范围：

- fronted/src/views/sys/automation/workflow.vue
- fronted/src/views/sys/automation/workflowEditor.vue
- fronted/src/views/sys/automation/workflowRun.vue
- fronted/src/api/sys/automation.js

本文描述当前前端实现与后端契约。

## 1. 页面职责

### 1.1 workflow.vue（列表页）

- 展示 workflow 模板列表。
- 支持新增、编辑、预演、启动、删除。
- 展示 workflow 运行记录并支持取消运行。
- 从列表中的运行记录跳转到状态图页面。

### 1.2 workflowEditor.vue（编排编辑器）

- 使用 Vue Flow 维护节点与连线。
- 支持节点类型：
  - task 节点（task_id）
  - workflow 节点（workflow_id）
- 支持边条件：success/failure/always。
- 支持 convergence：any/all。
- 保存时提交 nodes + edges + default_extra_vars。

### 1.3 workflowRun.vue（运行状态图）

- 只读渲染一次 run 的节点状态。
- 提供运行轮询刷新。
- 支持：
  - 取消整个 workflow run
  - 取消运行中的 task 节点作业
  - 查看 task 节点详情/日志（新窗口）

## 2. API 封装

位于 src/api/sys/automation.js：

- 模板：
  - getWorkflowList
  - getWorkflowDetail
  - createWorkflow
  - updateWorkflow
  - deleteWorkflow
  - previewWorkflow
  - launchWorkflow
- 运行：
  - getWorkflowRunList
  - getWorkflowRunDetail
  - cancelWorkflowRun
- 节点任务操作：
  - cancelJob
  - getJobLog

## 3. 编辑器数据结构

### 3.1 节点

提交 payload 结构：

- key
- name
- node_type
- convergence
- x, y
- task_id 或 workflow_id

规则：

- node_type=task 时必须有 task_id。
- node_type=workflow 时必须有 workflow_id。
- 节点名称不能为空。

### 3.2 连线

提交 payload 结构：

- source_key
- target_key
- condition（success/failure/always）

说明：

- 编辑器内部有 START 辅助边，仅用于画布体验。
- 保存时会过滤 START 系统边，不会提交到后端。

## 4. 关键交互约束

### 4.1 节点点击与跳转

- 运行页不允许点击节点卡片直接跳转。
- 仅按钮触发外部页面打开（任务、日志）。

### 4.2 新窗口行为

- task 详情/日志使用 window.open 新标签页。
- 不做当前页 router.push 回退，避免双页面跳转。

### 4.3 按钮显示条件

- 日志按钮仅 task 节点且存在 jobId 时显示。
- 节点取消按钮仅 task 节点且状态处于 pending/running/queued 时显示。

## 5. 运行态显示规则

节点状态标签映射：

- waiting: 等待前置节点
- pending: pending中
- queued: 排队中
- running: 运行中
- success: 成功
- failed: 失败
- cancelled: 已取消
- skipped: 已跳过（条件不满足）
- waiting_approval: 等待审批

顶部统计：

- 待执行
- 等待中
- 运行中
- 成功
- 已跳过
- 已取消
- 失败

## 6. 与后端约定

### 6.1 Snapshot 优先

运行页优先使用后端返回的：

- workflow_nodes
- workflow_edges
- node_results_runtime

这样可保证历史运行图不受模板后续修改影响。

### 6.2 递归检测

workflow 节点允许在编辑器中选择当前 workflow。

若形成递归调用链，后端会在运行时拦截并将节点标记 failed，前端按普通失败节点显示，并通过节点 message 展示原因。

## 7. 调试建议

### 7.1 常见问题

- 节点显示为失败但没有日志：
  - 可能是 workflow 节点递归拦截，不会产生 task job 日志。
- 已保存模板但运行图与模板不一致：
  - run 页面展示的是该次运行快照，不是当前模板实时内容。

### 7.2 本地联调步骤

1. 启动前端：npm run dev
2. 启动后端与调度：python manage.py runserver + python manage.py runscheduler
3. 在 workflow.vue 启动一个 workflow 并跳转到运行页观察状态推进
