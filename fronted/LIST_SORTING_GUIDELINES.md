# 列表排序功能规范

本规范用于统一前端列表页是否开启列排序，避免“每列都可排序”导致的体验噪音与后端成本膨胀。

## 目标

- 只给高价值字段开启排序。
- 排序行为与后端 `ordering` 字段一致，避免前后端语义不一致。
- 默认采用“最小可用排序集”，按需渐进扩展。

## 字段类型与排序建议

### 必加排序（默认开启）

- 时间类字段：`create_time`、`update_time`、`start_time`、`end_time`、`run_time`
- 状态类字段：`status`、`enabled`
- 标识类字段：`id`、`job_id`、`run_id`
- 数值类核心指标：`duration_seconds`、`count`、`total`

### 选加排序（业务确认后开启）

- 名称类字段：`name`、`task_name`、`workflow_name`
- 优先级/权重类字段：`priority`、`weight`
- 版本号/序号类字段：`version`、`order_num`

说明：名称类字段仅在“列表规模较大且用户确有按字母定位需求”时开启。

### 不建议排序（默认关闭）

- 操作列：`action`
- 复杂拼接展示列（如多标签聚合文本）
- JSON 摘要列（如环境变量片段）
- 跳转链接列（仅用于导航，不用于比较）

## 前端实现要求

- 仅在列定义中显式声明可排序列：`sorter: true`
- 表格 `@change` 事件中统一处理排序参数并转换为后端 `ordering`
- 未声明排序的列，禁止展示排序 UI（避免误导）

## 后端契约要求

- API 必须声明可排序字段白名单（`ordering_fields`）
- 前端只能传白名单内字段，防止无效排序请求
- 推荐默认排序：`-id` 或 `-create_time`

## 页面最小可用排序集（当前项目建议）

- 自动化任务列表：`name`、`enabled`、`update_time`
- Workflow 列表：`name`、`enabled`、`update_time`
- 运行记录中心（任务）：`id`、`status`、`start_time`、`duration_seconds`
- 运行记录中心（Workflow）：`id`、`status`、`start_time`、`duration_seconds`

## 验收清单

- 排序字段是否都在后端白名单中
- 升序/降序是否都正确
- 翻页后排序状态是否保持
- 与筛选联用时结果是否正确
