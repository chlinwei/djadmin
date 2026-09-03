# 系统管理与操作审计（autoadmin）

描述系统参数（sysconfig）、RBAC 菜单/角色（rbac）与操作审计（audit）的关键逻辑与测试覆盖。后端唯一实现为 Go 版 autoadmin。

## RBAC 菜单树（internal/rbac/menu.go）

- 根菜单约定 `parent_id=0`（Django 遗留），但**历史数据存在 NULL**：`menuTree` 两者都按根处理（⚠️ 修复前 NULL 根菜单会被整条丢弃，勿回退）。
- 排序：同级按 `order_num` 升序，缺失/并列时按 `id`。
- 异常拓扑语义：孤儿节点（parent 指向不存在的 ID）、双节点环、自引用环都**不得输出到根**，直接丢弃。
- 叶子节点 `children` 输出空数组而非 null（前端依赖可迭代）。
- `validateParent`：parent 为 nil/0 合法；`parent=自身` 拒绝（ErrMenuSelfParent，纯内存判断不触库）。

## 系统参数（internal/sysconfig/service.go）

- `normalize` 按 `value_type` 严格校验：`int`（Atoi，不 trim）；`bool` 接受 true/1/yes、false/0/no（大小写不敏感）；`json` 接受对象/数组/合法 JSON 字符串；`secret` 哈希落库，**掩码 `******` 原样透传**（编辑页未改密时防重复哈希）；未知类型按字符串。
- `typed`：读取侧类型还原；`secret` 值对所有客户端输出一律掩码 `******`（含 default_value）。

## 操作审计（internal/audit）

- 审计中间件记录变更类请求并脱敏，豁免清单请求跳过（`middleware_test.go`）。
- 终端历史日志清洗 ANSI 转义、可读命令解析（`history_test.go`）。

## 测试覆盖

- `internal/rbac/menu_test.go`：层级/排序、孤儿丢弃、环保护（双向/自引用）、NULL 根、可空字段映射、validateParent。
- `internal/sysconfig/service_test.go`：normalize 全类型（含 secret 哈希+掩码透传）、typed 类型还原与掩码、mapConfig 输出契约（掩码+UTC 时间格式）。
- `internal/audit/history_test.go` + `middleware_test.go`：见上。

## 纳管目标的 agent 在线依赖（internal/monitor）

项目无应用层心跳，Agent 在线权威是活跃 gRPC 会话（`Gateway.IsOnline`）。纳管目标页所有"经 agent 下发"的操作必须在派发前校验在线，检查矩阵：

| 操作 | 派发函数 | 在线校验 |
|---|---|---|
| Exporter 安装/卸载（retry、批量纳管 install_now） | `dispatchExporterJob` | ✅ 离线→target 标记 failed + install_message |
| Exporter 启动/停止/查状态（单台/批量） | `dispatchTargetServiceControl` | ✅ 400 `host agent is offline` |
| Fluent Bit 安装/卸载（retry、批量纳管 install_now） | `dispatchLogTargetInstall` | ✅ 同上语义 |
| Fluent Bit 启停/查状态/下发配置 | `dispatchLogTargetServiceControl` / `ApplyLogTargetConfig` | ✅ |
| 取消任务 / 删除目标 / PATCH 目标字段 | 仅改库，不触 agent | 不需要在线 |

- ⚠️ `BatchCreateTargets`（Exporter 批量纳管）必须实现 `install_now=true` 时立即调用 `dispatchExporterJob`（前端固定传 install_now=true，Django 同语义）；此前曾漏实现导致纳管后目标停在未安装状态，勿回退。
- 列表展示的 `host_agent_online` 来自实时 gRPC 会话（`read_resources.go`），不是落库快照。
- 前端所有操作按钮以 `host_agent_online` 置灰（含 Fluent Bit 需 `agent_installed`），与后端校验双保险。
- TOCTOU 余留：在线校验与 agent 实际执行间极小窗口内掉线，`gateway.Execute` 会以错误返回并落库失败状态，属可接受语义。

## 纳管目标在线守卫测试（internal/monitor/target_online_guard_test.go）

集成回归用例，依赖 `MYSQL_DSN`（未配置自动跳过）；自建测试主机/目标并在结束时清理：

- `TestBatchCreateTargetsInstallNowOfflineGuard`：**核心回归**——`install_now=true` 必须真的触发派发（此前漏实现）；gateway 为 nil（全部离线）时派发被守卫拦截，target 落 `failed` + 可读 `install_message`，单条结果 ok=false 带原因。
- `TestTargetServiceControlRejectsMissingAgent`：主机无 agent 时服务控制拒绝（业务码 400 + 可读原因），不能当成功下发。
