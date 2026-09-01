# Django Backend API 深度分析

本文基于当前 `backend/djadmin` 实现整理，是 Go 重写的接口事实基线。它记录现状，不代表所有现状都值得复制；需要修复的设计会在 `GO_REWRITE_GUIDE.md` 单独标记。

## 1. 全局协议

### 1.1 路由边界

| 领域 | 前缀 | 主要资源 |
|---|---|---|
| 身份与 RBAC | `/sys/` | login、usercenter、users、roles、menus、configs |
| 调度 | `/sys/scheduler/` | tasks、task-logs |
| 自动化 | `/sys/automation/` | playbooks、tasks、inventories、workflows、workflow-runs、jobs |
| 巡检 | `/sys/inspection/` | groups、tasks、executions |
| 审计 | `/sys/audit/` | WebSSH、登录、操作日志 |
| 资产 | `/assets/` | 凭证、业务拓扑、主机、部署、WebSSH 文件操作 |
| 监控 | `/monitor/` | Exporter、Prometheus、告警、OpenSearch、日志采集 |
| Agent 控制面 | `/api/agent/` | 安装、任务创建/查询/重试/取消 |

根路由见 [Django URL 配置](../../backend/djadmin/djadmin/urls.py)。

### 1.2 响应包络

普通业务成功和失败通常都返回 HTTP 200：

```json
{"code": 200, "msg": "success", "data": {}}
```

应用错误码主要为：`300` 登录失败、`301` JWT 无效、`400` 参数或业务冲突、`403` 无权限、`404` 资源不存在。DRF 未捕获异常可能保留原 HTTP 状态并被包装为 `code=600`。

以下响应故意不使用统一包络：文件上传下载流、Prometheus 原始代理、Prometheus HTTP-SD、Alertmanager webhook。

### 1.3 分页、搜索与排序

标准分页参数实际为 `page`、`page_size`，默认 10、最大 30。响应字段为：

```json
{
  "results": [], "count": 0,
  "pageNumber": 1, "pageSize": 10, "totalPages": 0,
  "next": null, "previous": null
}
```

当前 [分页实现](../../backend/djadmin/djadmin/utils.py) **没有实现** API_RULES 声称的 `pageNumber/pageSize` 请求别名。部分旧页面发送 `size`，标准分页会忽略并退回 10。Agent 查询是特例：使用 `page,size`，返回 `total`，且 `count` 只是当前页数量。

### 1.4 鉴权与权限

- `/sys/login` 公开；普通 API 从 `Authorization` 直接读取 JWT。
- JWT 无效返回 HTTP 200、`code=301`。
- `admin` 用户绕过菜单权限。
- 普通用户按 ViewSet action 映射到 `module:resource:action` 权限码。
- 当前 [菜单权限类](../../backend/djadmin/menu/permisssion.py) 对缺失或拼错的 action 映射采取 **放行**，属于 fail-open。
- Prometheus HTTP-SD 与 Alertmanager webhook 使用哈希后的 `ApiToken`：前者接受 query token，后者接受 Bearer token；失败是原生 HTTP 403 `{"error":"forbidden"}`。
- `/api/agent/*` 当前使用用户 JWT，不是 Agent 身份凭证。

Go 首次兼容上线需保持路由与响应形状，但权限必须建立完整矩阵并改为 fail-closed；这属于需要显式回归的安全行为变化。

## 2. 用户、角色、菜单与配置

### 2.1 用户和用户中心

| 方法与路径 | 请求/查询 | 关键行为 |
|---|---|---|
| `POST /sys/login` | `username,password` | 校验状态和密码；当前库全部为 PBKDF2-SHA256；返回 `currentUser,token,menuList,role_codes`；错误凭证 code 300，禁用用户 code 1006；写登录审计 |
| `POST /sys/usercenter/updateUserInfo/` | `phonenumber` 等 | 直接更新当前用户，校验较弱 |
| `POST /sys/usercenter/updateUserPassword/` | `old_password,new_password` | 校验旧密码并重新哈希 |
| `GET /sys/usercenter/alertMediaBindings/` | 无 | 返回启用媒介 options 与当前绑定/收件人 |
| `POST /sys/usercenter/updateAlertMediaBindings/` | `bindings[{media_id,recipients,enabled}]` | 删除后重建；当前无事务，可能部分替换 |
| `GET /sys/usercenter/apiTokens/` | 无 | 返回所有 token 元数据，不仅创建者；不返回 hash/plaintext |
| `POST /sys/usercenter/createApiToken/` | `bind_mode,agent_id,name,is_active,expires_at,remark` | API token 要求未来过期时间和非 global agent_id；明文只返回一次 |
| `POST /sys/usercenter/rotateApiToken/` | `id` | 仅活动 token；替换 hash，返回一次新明文 |
| `POST /sys/usercenter/disableApiToken/` | `id` | 禁用活动 token |
| `POST /sys/usercenter/deleteApiToken/` | `id` | 硬删除 |
| `POST /sys/changeAvatar` | multipart `avatar` | 保存到 media；当前无类型/大小限制 |
| `GET/POST /sys/users/` | 标准分页、搜索、排序；创建用户字段 | 列表含角色；默认密码由 serializer 定义 |
| `GET/PUT/PATCH /sys/users/{id}/` | 用户字段 | retrieve 因 action 拼写问题仅 JWT；PATCH 有权限映射 |
| `GET /sys/users/checkUserName/` | `username` | 返回 `exists` |
| `DELETE /sys/users/userBatchDelete/` | `user_ids` | 先删角色关系再删用户；无事务 |
| `POST /sys/users/resetUserPwd/` | `id` | 重置并返回明文 `123456` |
| `POST /sys/users/assginUserRoles/` | `user_id,roleIds` | 全量替换；拼写错误是现有契约；当前无事务 |
| `POST /sys/users/changeUserStatus/` | `user_id,status` | 启停用户 |
| `GET /sys/users/getUserRolesById/` | `user_id` | 返回 `roleList` |
| `GET /sys/users/current/` | 无 | 返回当前用户及 timezone |

### 2.2 角色和菜单

| 方法与路径 | 关键行为 |
|---|---|
| `GET/POST /sys/roles/`、`GET/PUT/PATCH /sys/roles/{id}/` | 标准 CRUD；角色 name/code 在 DB 中并不唯一 |
| `GET /sys/roles/getCurrentUserRoleList/` | 当前用户角色列表 |
| `DELETE /sys/roles/batch-delete/` | 手工删除用户角色、角色菜单、角色；当前无事务 |
| `POST /sys/menus/`、`GET/PUT/PATCH /sys/menus/{id}/` | 菜单 CRUD；没有普通菜单 list 端点 |
| `GET /sys/menus/getMenuTree/` | 返回完整有序树，不分页 |
| `GET /sys/menus/getMenuListByRoleId/` | `role_id`；`data` 直接是菜单 ID 数组 |
| `POST /sys/menus/grantMenu/` | `role_id,menuIds`；全量替换，当前无事务 |
| `DELETE /sys/menus/deleteMenuById/` | `id`；删关联后删菜单，不检查子菜单 |

### 2.3 系统配置

资源实际路径是 `/sys/configs/`，不是部分旧文档中的 `/sys/config/`。

| 方法与路径 | 关键行为 |
|---|---|
| `GET/POST /sys/configs/` | 列表读取会惰性创建缺失内置项；POST 实际可用 |
| `GET/PATCH /sys/configs/{id}/` | readonly 禁改；按 string/int/bool/json/secret 规范化；secret 单向哈希 |
| `POST /sys/configs/{id}/reset-default/` | 恢复默认值；secret 默认值同样哈希 |
| `GET /sys/configs/by-key/{key}/` | 返回转换类型后的 `key,value,name` |
| `PATCH /sys/configs/update-by-key/{key}/` | 按 key 更新，不支持修改默认值 |

整个配置资源当前只有 JWT，没有菜单权限。

## 3. Scheduler API

| 方法与路径 | 关键行为 |
|---|---|
| `GET /sys/scheduler/tasks/` | 惰性创建默认任务；过滤 `enabled,is_running,search`；隐藏一个旧清理任务 |
| `GET/PATCH /sys/scheduler/tasks/{id}/` | 读取或修改可编辑字段；修改后计算 next run |
| `POST .../{id}/toggle_enabled/`、`enable/`、`disable/` | underscore 是真实路径；更新启用与下次时间 |
| `POST .../{id}/run_now/` | 要求任务启用、未运行且 Celery worker 在线；异步投递 |
| `GET .../{id}/status/` | 返回运行状态、全局调度开关、最后结果和时间 |
| `POST /tasks/start_scheduler/`、`stop_scheduler/` | 只切换配置，不启动或停止 Beat/Worker 进程 |
| `GET /sys/scheduler/task-logs/` | 分页；支持 task/status/duration/content 过滤 |
| `GET /sys/scheduler/task-logs/{id}/` | 日志详情和 output |

没有 task create/delete API，也没有菜单权限。Go 调度器应从同一任务表装载定义，但用 leader lease、执行表和 outbox 消除重复投递窗口。

## 4. Assets API

### 4.1 资产与应用拓扑 CRUD

| 资源 | API 与特殊规则 |
|---|---|
| credentials | `/assets/credentials/` 全 CRUD、`batch-delete`、CSV `batch-create`；secret 加密；连接字段变化会关闭关联在线 WebSSH |
| applications | CRUD，无 detail DELETE；通过 `batch-delete` 删除并处理 PROTECT |
| application-versions | 全 CRUD；删除受部署引用保护 |
| projects | 全 CRUD；BusinessSystem 以 PROTECT 引用 |
| business-systems | 全 CRUD；过滤 project/enabled，返回环境和部署计数 |
| business-environments | 全 CRUD；全局唯一字典，被 Host/Service 保护引用 |
| cluster-profiles | 全 CRUD；内置 profile 禁删改 |
| application-deployment-templates | 全 CRUD；嵌套替换 ports/paths/config/log/action/control 配置，serializer 内事务 |
| application-services | 全 CRUD；创建/更新事务替换成员和覆盖项，commit 后异步刷新 Fluent Bit |
| application-deployments | 全 CRUD；通过显式中间表关联 service |

Service 额外端点：

- `GET /application-services/{id}/log-config/`：返回模板、日志定义、解析/过滤规则、保留档位和解析后的路径/data stream。
- `POST /application-services/{id}/refresh-runtime-status/`：最多 8 线程同步检查所有启用部署。
- `POST /application-deployments/{id}/control/`：`action=start|stop|status`，创建 AgentJob、同步等待 gRPC、更新 runtime status。

### 4.2 主机和分组

| 方法与路径 | 关键行为 |
|---|---|
| `/assets/host-groups/` CRUD | 删除递归包含子组和主机；先扫描 Inventory/Inspection JSON 引用；当前多表删除非原子 |
| `GET /assets/host-groups/tree/` | 完整树，不分页 |
| `DELETE /assets/host-groups/batch-delete/` | `ids`；递归展开，空输入返回裸数组 |
| `/assets/hosts/` CRUD | 递归 group 过滤；按采集/Agent/环境过滤；list 与 detail serializer 不同 |
| `GET /assets/hosts/{id}/agent-runtime-status/` | 同步 gRPC；要求绑定且在线 Agent |
| `POST /assets/hosts/{id}/refresh-info/` | 同步采集并更新硬件/系统/磁盘 |
| `POST /assets/hosts/refresh-info/` | `ids`；最多 8 线程同步批量采集 |
| `DELETE /assets/hosts/batch-delete/` | `ids`；先关闭 WebSSH，再删主机 |
| `GET .../{id}/webssh-sessions/` | 当前主机会话审计列表 |
| `GET .../{id}/webssh-active-count/`、`webssh-active-sessions/` | 读取进程内会话注册表 |

Host create/update 的 `monitors[{name,enabled,port}]` 采用期望状态语义：首次启用触发安装，enabled→disabled 触发卸载，字段缺失则不改变现有监控项。

### 4.3 WebSSH 文件 API

所有文件操作除 JWT/菜单权限和 Agent 在线外，还要求“当前调用用户在该主机有活动 WebSSH 会话”。

| 方法与路径 | 协议 |
|---|---|
| `GET .../files/list/?path=` | 返回 current/parent path 与 entries |
| `GET .../files/download/?path=` | 原始二进制；支持单 Range、206/416；目录拒绝 |
| `POST .../files/upload/chunk/` | multipart `file,path,filename`；名字虽含 chunk，实际是单流上传 |
| `POST .../files/rename/` | `path,new_name`；名称禁止 slash |
| `DELETE .../files/delete/` | `path,recursive` |
| `POST .../files/create-dir/`、`create-file/` | `path,name` |

## 5. Automation API

所有自动化资源使用菜单权限和标准分页。

| 资源/动作 | 关键行为 |
|---|---|
| `/playbooks/` CRUD | YAML/Ansible 校验；category general/software_package |
| `POST /playbooks/validate/` | `content`，返回 valid 或业务错误 |
| `GET /playbooks/host-options/`、`group-tree/` | Job 创建范围选择数据 |
| `POST /playbooks/{id}/upload/` | 仅 UTF-8 `.yml/.yaml`；校验后替换 |
| `GET /playbooks/{id}/download/` | 原始 YAML attachment |
| `POST /playbooks/{id}/run/` | `host_ids,extra_vars,run_as_user,run_as_group,work_directory`；创建快照并在 HTTP 进程同步执行到结束 |
| `/tasks/` CRUD | 模板、Inventory、env、limit、身份、目录、timeout；被 Workflow 引用时禁删 |
| `POST /tasks/{id}/precheck/` | 可传 limit/host_ids；无论预检成功失败顶层 code 都是 200，业务看 `data.ok/status` |
| `POST /tasks/{id}/run_now/` | 创建快照并同步执行 |
| `/inventories/` CRUD | `selected_host_ids` 是唯一范围来源，固定快照而非动态组 |
| `POST /inventories/{id}/precheck-limit/` | 返回解析数量、离线主机、preview |
| `/workflows/` CRUD | nodes/edges/defaults；校验线性图、引用和环 |
| `POST /workflows/{id}/precheck-launch/` | 同样使用 `data.ok/status` |
| `POST /workflows/{id}/launch/` | 快照图并在请求内同步推进到终态，支持 node outcomes |
| `/workflow-runs/` list/detail | 过滤 workflow/status/requester/time；详情扩展节点、子流程、Job |
| `POST /workflow-runs/{id}/cancel/` | DB 标记活动 Job canceled、等待节点 skipped；不保证远端进程终止 |
| `/jobs/` list/detail | status/task/job/keyword/output/time 过滤；可附合成 summary |
| `GET /jobs/{id}/log/` | 汇总 target logs |
| `GET /jobs/{id}/events/` | 当前固定返回空数组 |
| `GET /jobs/{id}/status_summary/` | 根据父 Job 状态合成，不是目标真实聚合 |
| `POST /jobs/{id}/cancel/` | 只改 DB 状态，无远端 gRPC cancel |

## 6. Inspection API

| 方法与路径 | 关键行为 |
|---|---|
| `/sys/inspection/groups/` CRUD | 嵌套 checks；被 task 引用禁删 |
| `/sys/inspection/tasks/` CRUD | group、目标类型/服务/固定 host IDs、并发、timeout、cron、enabled |
| `GET /tasks/host-scope-tree/` | 返回非云删除主机的完整 groups/hosts |
| `POST /tasks/{id}/run/` | 事务创建 execution/targets，commit 后启动进程内后台线程，立即返回 execution_id |
| `/executions/` list/detail | list 紧凑、detail 展开 target/results；按 task/status/trigger/time 过滤 |
| `POST /executions/{id}/cancel/` | pending/running 才允许；标记 execution/targets 并 best-effort Agent cancel |

## 7. Monitor API

### 7.1 Exporter 与 Prometheus

- `/monitor/targets/` 全 CRUD；删除要求 `managed_enabled=false` 且无 pending uninstall。
- `host-group-tree`、`exporter-options`、分页 `host-overview` 为前端聚合视图。
- `batch-create` 返回逐项成功/失败；安装放进 Web 进程内线程池。
- target 的 `retry/cancel/check-service-status/start-service/stop-service` 分别操作安装状态或同步 Agent systemd。
- `/targets/summary/` 与 `/monitor/summary/` 是同一 summary 的两个入口。
- Prometheus targets/alerts/rules/overview/tsdb/config/flags/query/query-range 经过后端转换；上游失败可能仍是顶层 code 200、内部 `status=error`。
- `/targets/prometheus/proxy/{api_path}` 返回原生 Prometheus JSON，仅允许 `/api/v1/*`。
- `/monitor/prometheus/http-sd/` 及 alias 返回原始 HTTP-SD 数组。
- `/monitor/alert-webhook/api/v2/alerts` 接收 Alertmanager v2 数组，写历史后 commit 投递通知。

### 7.2 包、安装历史与告警

- `/monitor/packages/` 全 CRUD；生命周期 Playbook 会创建隐藏 automation template。
- `upload` 校验格式、计算 SHA-256/size；`sync-official` 当前只支持 node_exporter，网络调用同步且限时/限大小。
- `/monitor/install-histories/` 只读；cancel 使用 `select_for_update`，同时更新关联 target。
- `/monitor/alert-histories/` 只读；`notification-status` 展开 event 和每个用户/媒介/地址的 delivery。
- `/monitor/media/` 全 CRUD，目前主要是 email；密码加密并输出 `********`；`test` 同步发 SMTP。
- `/monitor/alert-routes/` 全 CRUD，matchers 是 exact-match JSON，关联多个 media。

### 7.3 OpenSearch 与日志采集

- OpenSearch cluster CRUD 当前业务要求单例；save 后 commit 投递 storage sync。
- `test-connection/bootstrap/log-health/pipeline-simulate` 是同步 OpenSearch 操作。
- `log-search` 必须给 service ID，窗口最大 30 天，size≤200，offset+size≤2000。
- `log-facet-stats` 只允许白名单 field，并限制 size/interval。
- retention tier、processing rule、filter rule 均 CRUD；默认项和远端 policy/pipeline 同步存在跨系统一致性问题。
- `/monitor/log-targets/` 只有 GET/POST/detail GET/DELETE；已安装删除转 pending uninstall，未安装直接删。
- retry/start/stop/cancel/render-config/apply/check-status/log-tail 与批量版本均返回逐项结果；批量操作不是全有或全无。
- Fluent Bit apply 通过配置 fingerprint 幂等；相同 fingerprint 返回 `skipped=true`。

## 8. Audit API

| 资源 | 行为 |
|---|---|
| `/sys/audit/webssh-sessions/` | 分页和状态/用户/关键字/时间过滤；会把运行时已不存在的 connected 记录修正为 closed |
| `/{id}/content/` | 先刷新内存缓冲再返回完整输出 |
| `/{id}/download/`、`download-all` | 原始文本或 ZIP；批量下载继承当前筛选条件 |
| `/sys/audit/login-logs/` | 状态、用户/IP、时间过滤 |
| `/sys/audit/operation-logs/` | 排除 GET；按关键字/method/status/time 过滤 |

操作审计由 middleware 同步写入，跳过登录、审计自身、静态/media 和 404。请求正文截断到 4KB，但成功 JSON 响应当前没有同等明确上限。

## 9. Agent HTTP API

| 方法与路径 | 关键行为 |
|---|---|
| `POST /api/agent/jobs/create` | type/action/target/params；`client_request_id` 幂等；建 event 后同步等待 gRPC dispatch |
| `POST /api/agent/install` | host_ids、install/update；事务锁主机并拒绝活动任务；commit 后逐主机线程，立即返回 automation_job_id/jobs |
| `POST /api/agent/jobs/create-batch` | agent_ids；逐 Agent 幂等与投递，允许部分成功 |
| `POST /api/agent/jobs/retry` | job_id；行锁；仅 failed/timeout/canceled；创建 retry child |
| `POST /api/agent/jobs/cancel` | job_id/reason；行锁并改本地状态，不发送远端取消 |
| `GET /api/agent/jobs/query` | 自定义 page/size≤200；支持 group_by=action；count 是当前页数量 |
| `GET /api/agent/jobs/query-chain` | 按 retry_from 形成有界 nodes/edges |
| `GET /api/agent/jobs/events` | job/agent/tag/type 过滤，limit≤500，不分页 |

## 10. Go 契约测试清单

每迁移一个 endpoint，至少比较：

1. HTTP status、顶层 `code/msg/data` 与 raw-response 例外。
2. JSON 字段名、null/空数组/空对象差异和时间格式。
3. 标准分页、Agent 自定义分页与所有筛选/排序参数。
4. admin 与普通用户权限、缺权限、过期 JWT、ApiToken。
5. 正常写入、重复提交、引用保护、部分成功和事务回滚。
6. 同步响应、异步受理、取消和最终状态之间的语义。
7. 文件 Range、attachment header、Prometheus 原生响应等非 JSON 包络。
8. Vue 依赖的拼写和路径：`assginUserRoles`、`run_now`、underscore action。