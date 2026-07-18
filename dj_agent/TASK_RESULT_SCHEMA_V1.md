# dj_agent Task Result Schema v1

更新时间：2026-07-18

## 1. 设计目标

- 统一外层字段，便于检索、统计、审计。
- 不同任务的差异放在 `data` 字段，便于扩展新任务。
- 保持严格协议：任务必须显式声明 `type` 和 `action`。

## 2. 统一结果外层（所有任务一致）

所有任务执行完成后，返回统一结构（字段含义固定）：

```json
{
  "job_id": "job-001",
  "type": "custom",
  "action": "get_agent_version",
  "status": "success",
  "exit_code": 0,
  "stdout": "",
  "stderr": "",
  "data": {},
  "started_at": "2026-07-18T10:00:00Z",
  "finished_at": "2026-07-18T10:00:00Z",
  "cost_ms": 12,
  "error": ""
}
```

### 2.1 外层字段说明

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| job_id | string | 是 | 任务唯一 ID |
| type | string | 是 | 任务类型：`command` / `automation` / `inventory` / `custom` |
| action | string | 是 | 任务动作名，例如 `get_agent_version` |
| status | string | 是 | 任务状态：`queued` / `running` / `success` / `failed` / `canceled` / `timeout` |
| exit_code | number | 是 | 进程退出码；内置任务一般为 0 |
| stdout | string | 是 | 标准输出 |
| stderr | string | 是 | 标准错误 |
| data | object | 是 | 结构化结果，按 action 定义 |
| started_at | string | 是 | 开始时间（RFC3339） |
| finished_at | string | 是 | 结束时间（RFC3339） |
| cost_ms | number | 是 | 执行耗时，毫秒 |
| error | string | 是 | 错误信息（无错误为空字符串） |

## 3. 各 action 的 data 规范（v1）

## 3.1 action = get_agent_version

适用类型：`custom`（推荐）

```json
{
  "agent_version": "dj_agent:v1",
  "agent_version_raw": "v1",
  "go_version": "go1.26.5",
  "os": "linux",
  "arch": "amd64"
}
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| agent_version | string | 是 | 带前缀版本号，格式 `dj_agent:<version>` |
| agent_version_raw | string | 是 | 纯版本号，如 `v1` |
| go_version | string | 是 | Go 运行时版本 |
| os | string | 是 | 运行系统 |
| arch | string | 是 | CPU 架构 |

## 3.2 action = get_host_info

适用类型：`inventory`（推荐）或 `custom`

```json
{
  "agent_version": "dj_agent:v1",
  "hostname": "node-01",
  "os": "linux",
  "arch": "amd64",
  "go_version": "go1.26.5",
  "cpu_count": 8,
  "local_ipv4": ["10.0.0.12"],
  "pid": 12345,
  "network_error": ""
}
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| agent_version | string | 是 | 当前 agent 版本标签 |
| hostname | string | 否 | 主机名，读取失败可为空 |
| os | string | 是 | 运行系统 |
| arch | string | 是 | CPU 架构 |
| go_version | string | 是 | Go 运行时版本 |
| cpu_count | number | 是 | CPU 核心数 |
| local_ipv4 | array[string] | 是 | 本机 IPv4 列表（不含回环） |
| pid | number | 是 | agent 当前进程 ID |
| network_error | string | 否 | 网络接口读取失败时附带错误信息 |

## 3.3 命令执行类任务（type = command / automation）

这类任务核心结果在外层字段，`data` 暂按业务需要扩展：

- 重点依赖：`status`、`exit_code`、`stdout`、`stderr`、`cost_ms`。
- 若需要结构化摘要（例如 playbook 统计），建议放到 `data.summary`。

示例：

```json
{
  "job_id": "job-auto-001",
  "type": "automation",
  "action": "run_playbook",
  "status": "failed",
  "exit_code": 2,
  "stdout": "...",
  "stderr": "...",
  "data": {
    "summary": {
      "ok": 5,
      "changed": 1,
      "failed": 1
    }
  },
  "started_at": "2026-07-18T10:00:00Z",
  "finished_at": "2026-07-18T10:00:10Z",
  "cost_ms": 10000,
  "error": "command exited with code 2"
}
```

## 4. 后端存储建议

- 固定列：存统一外层字段（便于筛选、统计、排序）。
- JSON 列：存 `data`（便于按 action 扩展而不改表结构）。
- 索引建议：`job_id` 唯一索引，`status` 普通索引，`type+action` 组合索引。

## 5. 前端渲染建议

- 列表页：统一展示外层字段（状态、耗时、开始/结束时间）。
- 详情页：根据 `action` 切换渲染 `data` 面板。
- 原始日志：统一展示 `stdout/stderr`，不要混入 `data`。

## 6. 协议演进规则

- 新增 action 时：只新增该 action 的 `data` 规范，不破坏统一外层。
- 旧 action 的字段变更：只允许新增字段，不允许删除或改含义。
- 必填约束变更：需升级版本（例如 v2）并保留迁移说明。
