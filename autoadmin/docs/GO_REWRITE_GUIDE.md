# Go 重写实现规范

## 1. 重写原则

1. 以现有 Vue 请求和 Django 响应为首期 contract，不同时重写前后端协议。
2. 每次只迁移一个完整业务纵切面：route、permission、service、repository、audit、tests。
3. 不增加长期 legacy/fallback/shim；迁移期只在网关按路由切流。
4. 现有 MySQL schema 先原位复用，DDL 以真实 Django migration 后导出为准。
5. 配置记录与 execution snapshot 分离：配置可变，历史快照 append-only。

## 2. 建议领域包

```text
internal/
  identity/       login, usercenter, token
  rbac/           role, menu, permission
  sysconfig/      typed runtime configuration
  assets/         host, credential, topology, deployment
  agent/          authenticated gateway and command protocol
  scheduler/      definitions, leader lease, dispatch
  automation/     playbook, inventory, task, workflow, job
  inspection/     check plan, execution, alert projection
  monitor/        exporter, package, Prometheus, alert, logs
  audit/          login, operation, WebSSH retention
```

每个领域内部采用 `handler -> service -> repository`。Handler 不直接开启事务或发布 MQ；Service 决定事务边界；Repository 通过 sqlc generated interface 操作 DB。

## 3. API 兼容层

- 普通成功固定 HTTP 200、`{code:200,msg:"success",data}`。
- 业务错误首期按现有 code 映射；raw response 例外单独实现。
- 标准 pagination 保持字段大小写和 max 30。是否补充历史请求别名必须形成明确 API 决策，不能误称现状已有。
- `nil`、空数组、空对象在 Go JSON 中不同；list 字段初始化为空 slice。
- 时间输出 UTC ISO 8601，前端按用户 timezone 转换。
- 保留当前拼写/路径，如 `assginUserRoles`、`run_now`；未来更名应由协调版本完成。
- Precheck 失败仍可能顶层 code 200，真实结果在 `data.ok/status`。

## 4. 权限设计

当前 action mapping 缺失即放行必须改为 fail-closed：

1. 启动时注册 route + method + required permission。
2. CI 枚举 Gin routes，任何业务 route 无 auth policy 直接失败。
3. public、JWT、ApiToken、Agent-mTLS 四种 policy 显式标记。
4. admin bypass 单独记录审计，不散落在 handler。
5. 对现有“因拼写而无权限”的 endpoint 制作迁移矩阵，切流前给角色补正确 permission。

JWT 中的 perms 会陈旧。建议 token 增加 `permission_version`，角色变更时递增用户版本；middleware 检查版本或使用短期 access token。

## 5. 事务与并发

以下操作必须事务化：

- user-role、role-menu、user-alert-binding 全量替换。
- template/service 嵌套 aggregate replacement。
- HostGroup 递归删除及 JSON 引用保护。
- Job/Inspection/Workflow 创建 snapshot + targets + outbox。
- Agent install、monitor operation 的 active-operation claim。
- alert current-state upsert、notification event/delivery dedupe。

状态 claim 使用条件 SQL：

```sql
UPDATE execution
SET status = 'running', started_at = UTC_TIMESTAMP(6)
WHERE id = ? AND status = 'pending';
```

只有 `RowsAffected()==1` 的 Worker 获得执行权。需要读取并修改复合 aggregate 时使用短事务 `SELECT ... FOR UPDATE`，不要在事务内执行 Ansible、gRPC、SMTP、OpenSearch 或文件 IO。

## 6. RabbitMQ 与 Outbox

仅“数据库提交后直接 Publish”仍有崩溃丢消息窗口。推荐：

1. 同一事务写 business execution 和 outbox row。
2. 独立 publisher 读取未发送 outbox，Publish Confirm 成功后标记 sent。
3. RabbitMQ message 只携带 `schema_version,execution_id,kind,resource_id,triggered_at`，不携带 secret。
4. Worker 按 execution_id 幂等 claim。
5. transient error 进入有上限的延迟重试；invalid message/business terminal error 进入 DLQ 或终态。

Publisher 和 Consumer 使用独立 connection/channel，启用 durable exchange/queue、persistent message、manual ack、prefetch、publisher confirm 和 mandatory return 处理。

## 7. Scheduler

gocron v2 只管理本进程 timer。生产调度必须增加：

- MySQL leader lease：owner_id、lease_until、fencing_token。
- `(schedule_id,scheduled_for)` 唯一 execution。
- DB 配置变化的增量 reload 或版本轮询。
- misfire policy：skip、run_once、catch_up_bounded，逐任务显式配置。
- timezone：Cron definition 明确 location，scheduled_for 统一 UTC。
- fencing：旧 leader 即使暂停后恢复，也不能继续写有效 dispatch。

## 8. Agent Gateway

不能复制 Django 的进程内隐式 registry。Gateway 需要：

- mTLS 或 per-agent challenge 验证 agent_id。
- connection owner instance、session epoch、last heartbeat。
- 同 agent 重连以 epoch fencing，旧连接结果不得覆盖新任务。
- request_id -> waiter/stream 映射，有界超时与 backpressure。
- API/Worker 到 connection owner 的显式路由。
- WebSSH、文件流和普通 command 分离并设置独立流量限制。

Agent protocol 应继续版本化。未知 capability 必须拒绝或降级为明确错误，不做静默 fallback。

## 9. Automation 与进程执行

go-ansible 实际调用外部 `ansible-playbook`，Worker 镜像仍需要 Python/ansible-core。每次运行要求：

- context deadline 对应 execution timeout。
- 独立 process group，取消先 TERM、超时后 KILL。
- 临时目录 0700，key/inventory 0600，退出必清理。
- stdout/stderr 分块持久化并限制总量，不能只在结束后一次写大字段。
- extra vars 使用结构化 JSON 文件或 stdin，禁止 shell 字符串拼接。
- Worker concurrency、每用户/每 Host 限制和全局资源配额。

## 10. Secret、审计与日志

| 数据 | 存储方式 |
|---|---|
| 用户密码、ApiToken、secret SysConfig | 单向 hash |
| SSH password/private key、controller private key、SMTP/OpenSearch password | 带 key version 的 AEAD 可逆加密 |
| JWT signing key、master encryption key | 外部 secret manager/environment file，不入 DB 明文 |

所有结构化日志和审计在序列化前递归脱敏：password、token、authorization、private_key、secret、smtp/openSearch credentials。日志记录 request_id、actor、permission、execution_id，但不记录原始 secret 或完整大输出。

## 11. JSON 数据策略

- Workflow nodes/edges、Inventory host membership 等配置引用需要反向查询和删除保护，逐步规范化到关系表。
- Execution inventory/template/graph/result 保持 JSON snapshot，并增加 `schema_version`。
- Go 使用 tagged union，而不是 `map[string]any`：节点类型、executor config、Agent payload 分别定义具名 struct。
- 读取旧 JSON 时做一次显式版本迁移；不在核心逻辑堆积永久 fallback 分支。

## 12. 外部系统一致性

OpenSearch pipeline/policy、Fluent Bit config、软件包文件、SMTP、Prometheus 都不能加入 MySQL 事务。统一采用 desired-state + reconciler：

1. API 事务写 desired state 和 outbox。
2. Worker 应用外部变化。
3. 写 observed state、fingerprint/version 和错误。
4. Reconciler 重试差异，用户可见 desired/observed/pending/failed。

文件类资源在多副本部署中使用对象存储或明确 worker affinity，不能依赖任意 Pod 本地路径。

## 13. 首期兼容与明确修复

### 必须兼容

- 路由、字段名、统一包络及 raw-response 例外。
- 分页返回字段、Agent 自定义分页。
- fixed host scope 和 execution snapshots。
- warning 不导致 Inspection target failed。
- pending uninstall、precheck ok/status、合成 status summary 等 UI 可见语义。

### 应明确修复并回归

- 权限 fail-open。
- reset password 回传固定明文。
- Avatar 无类型/大小限制。
- 多个 delete-and-recreate 无事务。
- Agent hello 未认证。
- Alert fingerprint 非唯一导致并发重复。
- 默认 OpenSearch/Retention 缺 DB 唯一约束。
- cancel 只改数据库、不确认远端停止。
- Web 进程 daemon thread、进程内 scheduler/registry。
- 外部 pipeline 先写、数据库后写的分叉风险。

这些修复不能伪装成无影响重构；每项都应有 migration note、前端/权限数据准备和回滚方案。

## 14. 每个领域的完成定义

1. sqlc schema/query 和 repository tests 完成。
2. API differential tests 覆盖成功、错误、分页、权限和字段 nullability。
3. 写路径具备事务、幂等 key 和并发测试。
4. Job 路径验证重复投递、Worker 崩溃、RabbitMQ 暂停和取消。
5. 所有时间使用 UTC，secret 不出现在日志/响应。
6. `make fmt && make vet && make test && make build` 通过，二进制静态链接。
7. 路由切流后观察 error rate、latency、DB 差异和 audit，再删除 Django 对应路径。