# Agent Job API（dj_agent 联调）

本文档描述后端 Agent 作业控制面的 HTTP API（创建、批量创建、查询、链路、取消、重试）以及基于 NATS 的实时消息交互方案。

## 统一约定

- 成功响应：HTTP 200 + `{ code: 200, msg: "success", data: ... }`
- 失败响应：仍为 HTTP 200，`code != 200`，`msg` 为错误原因
- 鉴权：沿用现有 agent token 中间件
- 幂等键：`client_request_id`（可选）

## NATS 交互方案（当前推荐）

目标：backend 与 dj_agent 的任务下发和结果回传统一走 NATS，不再依赖 `poll/report` 作为主链路。

### 1) 主题设计

下发主题（backend publish）：

- 单机：`cmd.agent.<agent_id>`
- 组播：`cmd.group.<group_name>`
- 全量：`cmd.all`

上报主题（agent publish）：

- 结果：`ret.job.<job_id>`
- 状态事件：`evt.job.<job_id>.<event_type>`
- 心跳：`hb.agent.<agent_id>`

### 2) 订阅规则

backend：

- 订阅：`ret.job.>`、`evt.job.>`、`hb.agent.>`、`rpt.host.>`

agent：

- 固定订阅：`cmd.agent.<self_agent_id>`
- 组订阅：`cmd.group.<joined_group>`（可多组）
- 全量订阅：`cmd.all`

### 3) 下发消息体（backend -> agent）

字段约定：

- `command_id`：消息唯一 ID
- `client_request_id`：控制面幂等键（建议必传）
- `job_id`：作业 ID（与 AgentJob.job_id 对齐）
- `target_type`：`single` / `group` / `all`
- `target_value`：`agent_id` 或 `group_name` 或 `all`
- `type`：任务类型（例如 `inventory`）
- `action`：任务方法（例如 `get_host_info`）
- `params`：执行参数
- `timeout_seconds`：超时秒数
- `created_at`：UTC 时间

示例（单机）：

```json
{
  "command_id": "cmd-3d7930e53f08412f",
  "client_request_id": "req-20260718-0001",
  "job_id": "get_host_info-a1b2c3d4e5f6a7b8",
  "target_type": "single",
  "target_value": "host-001",
  "type": "inventory",
  "action": "get_host_info",
  "params": {
    "host_id": 1
  },
  "timeout_seconds": 30,
  "created_at": "2026-07-18T09:30:00Z"
}
```

示例（组播）：

```json
{
  "command_id": "cmd-cc37bf7d98aa4a5f",
  "client_request_id": "batch-20260718-01",
  "job_id": "sync_profile-1111aaaa2222bbbb",
  "target_type": "group",
  "target_value": "prod-linux",
  "type": "ops",
  "action": "sync_profile",
  "params": {
    "profile": "baseline-v3"
  },
  "timeout_seconds": 120,
  "created_at": "2026-07-18T09:31:00Z"
}
```

示例（全量）：

```json
{
  "command_id": "cmd-62e9fd25af6f4c59",
  "client_request_id": "all-20260718-01",
  "job_id": "refresh_cache-abcd1234abcd1234",
  "target_type": "all",
  "target_value": "all",
  "type": "ops",
  "action": "refresh_cache",
  "params": {},
  "timeout_seconds": 60,
  "created_at": "2026-07-18T09:32:00Z"
}
```

### 4) 上报消息体（agent -> backend）

结果消息建议：

- `command_id`
- `job_id`
- `agent_id`
- `action`
- `status`：`running/success/failed/timeout/canceled`
- `retcode`
- `result_data`
- `error_message`
- `ts`

示例（执行成功）：

```json
{
  "command_id": "cmd-3d7930e53f08412f",
  "job_id": "get_host_info-a1b2c3d4e5f6a7b8",
  "agent_id": "host-001",
  "action": "get_host_info",
  "status": "success",
  "retcode": 0,
  "result_data": {
    "hostname": "node-a",
    "os": "linux",
    "arch": "amd64",
    "agent_version": "dj_agent:v1"
  },
  "error_message": "",
  "ts": "2026-07-18T09:30:10Z"
}
```

### 5) 可靠性建议

- 下发通道：Core NATS（低延迟）
- 上报通道：JetStream（可持久、可重放）
- backend 与 agent 都按 `command_id` 做幂等去重
- backend 状态落库使用“状态机跃迁校验”，重复消息不重复改状态

### 6) 迁移顺序

1. backend 增加 NATS 发布/订阅，先消费 `ret/evt`。
2. backend 创建任务后同时发布 `cmd.*`。
3. dj_agent 改为订阅 `cmd.agent.*` / `cmd.group.*` / `cmd.all`。
4. 验证稳定后，下线 `poll/report` 运行时主链路（已完成）。

### 7) 安全建议

- NATS 启用 TLS。
- backend 与 agent 使用独立账号。
- agent 账号仅允许订阅自身相关 `cmd.*`，仅允许发布 `ret/evt/hb`。
- 禁止 agent 直接发布控制面主题。

## 1) 创建任务（单机/组播/全量）

- `POST /api/agent/jobs/create`

请求体示例：

```json
{
  "target_type": "single",
  "target_value": "host-001",
  "type": "inventory",
  "action": "get_host_info",
  "params": {
    "host_id": 1
  },
  "timeout_seconds": 30,
  "client_request_id": "req-20260718-0001"
}
```

参数说明：

- `target_type`：`single` / `group` / `all`，默认 `single`
- `target_value`：
  - `single`：目标 `agent_id`
  - `group`：主机组名（`Host.group.name`）
  - `all`：可为空
- 兼容参数：`single` 模式仍支持只传 `agent_id`

返回（single）`data` 字段：

```json
{
  "job_id": "get_host_info-a1b2c3d4e5f6a7b8",
  "agent_id": "host-001",
  "type": "inventory",
  "action": "get_host_info",
  "status": "queued",
  "reused": false,
  "target_type": "single",
  "target_value": "host-001"
}
```

返回（group/all）`data` 字段：

```json
{
  "target_type": "group",
  "target_value": "prod-linux",
  "created_count": 2,
  "failed_count": 1,
  "results": [
    {
      "agent_id": "host-001",
      "ok": true,
      "job_id": "get_host_info-1111aaaa2222bbbb",
      "status": "queued",
      "reused": false
    },
    {
      "agent_id": "host-003",
      "ok": false,
      "error": "agent_id未绑定主机"
    }
  ]
}
```

说明：
- `single` 模式下，当 `client_request_id` 重复提交时，返回历史任务，`reused=true`。
- `group/all` 模式下，幂等键按 `client_request_id:agent_id` 派生。
- `group/all` 会展开为逐 agent 作业，便于状态追踪、重试链路与失败定位。

## 2) 批量创建任务

- `POST /api/agent/jobs/create-batch`

请求体示例：

```json
{
  "agent_ids": ["host-001", "host-002", "host-003"],
  "type": "inventory",
  "action": "get_host_info",
  "params": {},
  "timeout_seconds": 30,
  "client_request_id": "batch-20260718-01"
}
```

返回 `data` 字段：

```json
{
  "created_count": 2,
  "failed_count": 1,
  "results": [
    {
      "agent_id": "host-001",
      "ok": true,
      "job_id": "get_host_info-1111aaaa2222bbbb",
      "status": "queued",
      "reused": false
    },
    {
      "agent_id": "host-002",
      "ok": true,
      "job_id": "get_host_info-3333cccc4444dddd",
      "status": "queued",
      "reused": true
    },
    {
      "agent_id": "host-003",
      "ok": false,
      "error": "agent_id未绑定主机"
    }
  ]
}
```

说明：
- 批量幂等键按 `client_request_id:agent_id` 派生。

## 3) 查询任务列表

- `GET /api/agent/jobs/query?agent_id=host-001&status=running&limit=50`

支持参数：
- `job_id`
- `agent_id`
- `status`：`queued/running/success/failed/canceled/timeout`
- `limit`：默认 50，最大 200

返回包含：
- `results`：任务列表
- `summary`：状态计数
- `recent_failure_reasons`：最近失败/超时原因（最多 5 条）

## 4) 查询重试链路

- `GET /api/agent/jobs/query-chain?job_id=<job_id>`

返回包含：
- `root_job_id`
- `focus_job_id`
- `nodes`
- `edges`

链路来源：`remark=retry_from:<source_job_id>`。

## 5) 取消任务

- `POST /api/agent/jobs/cancel`

请求体示例：

```json
{
  "job_id": "get_host_info-a1b2c3d4e5f6a7b8",
  "reason": "operator canceled"
}
```

说明：
- 仅允许 `queued/running -> canceled`
- 已结束任务不可取消

## 6) 重试任务

- `POST /api/agent/jobs/retry`

请求体示例：

```json
{
  "job_id": "get_host_info-a1b2c3d4e5f6a7b8"
}
```

说明：
- 仅允许 `failed/timeout/canceled` 重试
- 重试会创建新的 `queued` 任务（新 `job_id`）
- 新任务 `remark` 记录来源：`retry_from:<source_job_id>`

## 前端调用建议

- 所有“手动触发”请求都带 `client_request_id`，避免重复点击造成重复下发。
- 查询面板优先使用 `summary` 渲染健康度卡片，再用 `results` 渲染列表。
- 失败排障优先展示 `recent_failure_reasons` 与 `query-chain`。
