# Django Model 与领域数据分析

本文描述 Go/sqlc 必须承接的数据库事实与领域不变量。完整 DDL 仍应从应用全部 Django migrations 后的 MySQL 实例导出，不能根据本文手写猜测。

## 1. 全局模型约定

- `BaseModel` 提供 `id`、`create_time`、`update_time`、nullable `remark`。
- 普通 `models.Model` 也通过 Django 设置获得 `BigAutoField id`，少量旧模型显式使用 `AutoField`。
- 无显式 `db_table` 的模型使用 `<app>_<model>` 派生名。
- 当前业务模型没有重写 `save()` 或 `delete()`；复杂校验和删除保护主要在 serializer/view/service。
- 当前没有数据库 `CheckConstraint`；端口、并发、timeout 等边界大多只是应用校验。
- Django 开启时区，数据库值按 UTC；展示默认上下文为 Asia/Shanghai，用户还有独立 timezone。
- Go 类型必须从真实 information_schema/sqlc 生成，尤其核对 nullable、unsigned、decimal、JSON、text 和 datetime 精度。

## 2. 身份与 RBAC

### 2.1 用户

| Model / table | 关键字段与约束 | 关系/语义 |
|---|---|---|
| `SysUser` / `sys_user` | 显式 AutoField；username unique；password hash；status 1/0；timezone 默认 UTC | 识别 Django hash；登录时可把历史明文升级为 hash |
| `SysUserRole` / `sys_user_role` | unique(user,role) | user、role 均 CASCADE |
| `ApiToken` / `sys_agent_token` | agent_id、bind_mode agent/api、token hash、active、expires/last_used | creator SET_NULL；API 层检查 agent_id 唯一，但 DB 无对应唯一约束 |
| implicit M2M / `sys_user_alert_media` | user_id、alertmedia_id | 与带 recipients 的 UserAlertMediaBinding 并存，后者才是告警发送主关系 |

密码、API token 和 secret 型系统配置是**单向哈希**，不能设计解密接口。

### 2.2 角色与菜单

| Model / table | 关键字段与约束 | 关系/语义 |
|---|---|---|
| `SysRole` / `sys_role` | name、code 均 nullable 且 DB 不唯一 | 用户和菜单通过显式 join table |
| `SysMenu` / `sys_menu` | name unique；type M/C/F；perms；location；order/path/component | parent_id 是 nullable integer，不是 FK；DB 不保证树完整性 |
| `SysRoleMenu` / `sys_role_menu` | unique(menu,role) | 两端 CASCADE |

Go 写入必须把用户角色、角色菜单“全量替换”放在一个事务中。菜单树需要显式检测自环、祖先环和孤儿 parent_id。

## 3. Assets 与应用拓扑

### 3.1 基础应用字典

| Model / table | 关键约束 | 删除关系 |
|---|---|---|
| `Application` / `assets_application` | name/code 全局 unique；category/vendor/enabled | version/template CASCADE；service/profile/log rule PROTECT |
| `Project` / `assets_project` | name/code unique；owner/enabled | BusinessSystem.project PROTECT |
| `BusinessSystem` / `assets_business_system` | unique(project,name)、unique(project,code) | project PROTECT；模型允许 null，但 API 要求存在 |
| `BusinessEnvironment` / `assets_business_environment` | name/code 全局 unique；order/enabled | Host/Service PROTECT |
| `ApplicationVersion` / `assets_application_version` | unique(application,version) | application CASCADE；service PROTECT version |
| `ClusterProfile` / `assets_cluster_profile` | name/code unique；builtin/custom；多种 cluster type | application nullable PROTECT；service PROTECT；builtin 由 API 禁删改 |

### 3.2 模板与配置

| Model / table | 关键约束 | 关系 |
|---|---|---|
| `ApplicationDeploymentTemplate` / `assets_application_deployment_template` | unique(application,name)；control type、run identity、路径、systemd/HA、macro JSON | application CASCADE；service PROTECT |
| `ApplicationPort` / `assets_application_port` | unique(template,protocol,port)；端口 1–65535 仅应用校验 | template CASCADE |
| `ApplicationPath` / `assets_application_path` | unique(template,name)；path type、owner/group/mode | template CASCADE |
| `ApplicationConfigFile` / `assets_application_config_file` | unique(template,path)；XML/YAML/JSON/INI/properties/text | template CASCADE |
| `ApplicationLogDefinition` / `assets_application_log_definition` | unique(template,name)；path pattern、collect、extra JSON | template CASCADE；processing rule PROTECT |
| `ApplicationControlAction` / `assets_application_control_action` | unique(template,action)；command、timeout、success codes JSON | template CASCADE |
| `DockerControlConfig` / `assets_docker_control_config` | 容器、daemon、image 预期 | template O2O CASCADE |
| `DockerComposeControlConfig` / `assets_docker_compose_control_config` | project/service/compose/work/env/image | template O2O CASCADE |

模板创建和更新会事务性全量替换嵌套配置。Go 中应保持 aggregate write，而不是逐子资源暴露半完成状态。

### 3.3 Service 与部署

| Model / table | 关键约束 | 关系/风险 |
|---|---|---|
| `ApplicationService` / `assets_application_service` | code unique；unique(business_system,environment,name)；topology、address、log config、macro JSON | system/environment/application/version/template/profile/tier 多为 PROTECT |
| `ApplicationDeployment` / `assets_application_deployment` | unique(host,instance_name)；enabled、runtime status、HA role | host CASCADE；通过 M2M 加入 service |
| `ApplicationServiceDeployment` / `assets_application_service_deployment` | unique(service,deployment) | 两端 CASCADE |
| `ApplicationServiceLogSetting` / `assets_application_service_log_setting` | unique(service,log_definition)；nullable override 表示继承 | service/definition CASCADE；tier/filter/process PROTECT |

当前 DB 允许一个 Deployment 属于多个 Service，但业务代码常用 `.first()` 推导唯一 Service。Go 重写前必须确认真实业务不变量；若应唯一，应新增数据库约束和数据清洗迁移。

Service serializer 还负责：version/template 必须属于 application、HA 至少两个成员且模板支持外部 HA、load-balancer 必须有 address、deployment 不能跨 application 复用。这些必须进入 Go domain service + transaction。

## 4. 主机、凭证、Agent 与 WebSSH

| Model / table | 关键字段与约束 | 关系/语义 |
|---|---|---|
| `Credential` / `assets_credential` | name、username=root、加密 password/private_key、port=22、auth type 1/2 | HostCredential 两端 CASCADE；secret 必须可逆加密 |
| `HostGroup` / `assets_hostgroup` | name unique | parent self FK SET_NULL；深度/环仅 serializer 校验 |
| `CloudAccount` / `assets_cloudaccount` | provider type、凭据 | Host SET_NULL；无唯一约束 |
| `Host` / `assets_host` | nullable agent_id unique；IP/cloud/status/deleted/preferences | environment PROTECT；cloud/group SET_NULL；删除会级联大量运行配置 |
| `HostCredential` / `assets_hostcredential` | unique(host,credential)；每 Host 条件唯一 default | 两端 CASCADE |
| `HostHardware` / `assets_hosthardware` | CPU/memory/disk/arch/collected | Host O2O CASCADE |
| `HostSystem` / `assets_hostsystem` | OS/kernel/hostname/agent version/timezone/source | Host O2O CASCADE |
| `HostRuntime` / `assets_hostruntime` | 最新 CPU/memory/IO JSON、uptime、fingerprint | Host O2O；不是历史时序表 |
| `HostDisk` / `assets_hostdisk` | device/mount/size/used/filesystem | Host CASCADE；DB 无 host+device+mount 唯一约束 |
| `AgentJob` / `assets_agent_job` | job_id unique；nullable client_request_id unique；action/type/params/status/output/result | Host SET_NULL；agent/status 与 status/time indexes |
| `AgentJobEvent` / `assets_agent_job_event` | tag、job_id、agent_id、event_type、payload JSON | 不用 FK 关联 AgentJob，保留独立事件快照 |
| `WebSSHSessionLog` / `assets_webssh_session_log` | requested/effective user、close metadata、input/output、计数/truncated | host CASCADE；user_id 是 integer snapshot，不是 FK |
| `WebSSHTempCredential` / `assets_webssh_temp_credential` | credential O2O；nullable indexed session_pk | credential CASCADE；session_pk 故意不是 FK |

主机采集以 static fingerprint 避免无变化重写，事务 upsert hardware/system/runtime 并全量替换 disks。Go 需要保留“静态快照 + 最新运行快照”区分。

## 5. Automation

| Model / table | 关键字段 | 关系/历史语义 |
|---|---|---|
| `PlaybookTemplate` / `automation_playbook_template` | unique name、YAML content、category | 被 task PROTECT |
| `AutomationTask` / `automation_task` | unique name、env JSON、limit、enabled、timeout、run identity/workdir | template PROTECT；inventory SET_NULL |
| `AutomationControllerSSHKey` / `automation_controller_ssh_key` | public key unique、加密 private key | 全局控制机密钥 |
| `AutomationInventory` / `automation_inventory` | unique name、selected_host_ids JSON、enabled/sync state | 固定 host ID 成员，不是动态 host group |
| `AutomationExecutionJob` / `automation_execution_job` | UUID unique、status、完整 inventory/template/identity/requester snapshots、output/time | task SET_NULL；执行历史不可变 |
| `AutomationExecutionTargetLog` / `automation_execution_host_log` | host/agent snapshots、stdout/stderr/result | job CASCADE；host SET_NULL |
| `AutomationWorkflowTemplate` / `automation_workflow_template` | unique name、nodes/edges/default JSON | inventory SET_NULL；JSON 中引用 task/workflow |
| `AutomationWorkflowRun` / `automation_workflow_run` | graph/node/result/requester/time snapshots | workflow SET_NULL；历史不可变 |

执行记录的快照是核心审计数据。Go 不应只保存配置 FK；启动时必须复制 playbook、inventory、身份、变量、graph 和 node 元数据。配置 JSON 可逐步规范化，历史 snapshot 继续使用带 schema_version 的 JSON。

## 6. Inspection

| Model / table | 关键字段与约束 | 关系 |
|---|---|---|
| `InspectionGroup` / `inspection_group` | name unique；scope per_deployment/service_once/per_host | task PROTECT |
| `InspectionCheck` / `inspection_check` | unique(group,name)；executor、agent/controller location、config JSON、critical/warning、order | group CASCADE |
| `InspectionTask` / `inspection_task` | name unique；fixed host IDs；concurrency 1–100；timeout 5–3600；cron/next/last | group/service PROTECT |
| `InspectionExecution` / `inspection_execution` | task/group/service/target/check snapshots、summary、requester/time | task SET_NULL |
| `InspectionTargetExecution` / `inspection_target_execution` | deployment/host snapshots、status、raw result JSON | execution CASCADE；deployment/host SET_NULL |
| `InspectionResult` / `inspection_result` | check key/type/status/severity、expected/actual JSON | target CASCADE |

warning 检查失败只累计 warning，不把 target 判为失败；critical 失败才影响 target aggregate status。

## 7. Monitor

| Model / table | 关键约束与状态 | 关系/语义 |
|---|---|---|
| `MonitorTarget` / `monitor_target` | unique(host,exporter_type)；desired managed、port、install/retry/scrape、labels JSON | host CASCADE |
| `MonitorTargetInstallHistory` / `monitor_target_install_history` | action/trigger/status、host/exporter/user/output/result snapshots | target/log-target CASCADE；host SET_NULL |
| `AlertHistory` / `monitor_alert_history` | indexed fingerprint、rule/labels/annotations、firing/resolved、reconciled | fingerprint **不唯一**，并发 webhook 可重复 |
| `AlertMedia` / `monitor_alert_media` | name unique；email/webhook；加密 secret config JSON | route M2M，binding CASCADE |
| `AlertRoute` / `monitor_alert_route` | name unique；matcher JSON；firing/resolved switches | implicit `monitor_alert_route_media` |
| `AlertNotificationEvent` / `monitor_alert_notification_event` | dedupe key unique；event/attempt/status/error | alert CASCADE |
| `AlertNotificationDelivery` / `monitor_alert_notification_delivery` | unique(event,media,user,address) | event CASCADE；media/user SET_NULL |
| `UserAlertMediaBinding` / `monitor_user_alert_media_binding` | unique(user,media)；recipients JSON | user/media CASCADE |
| `SoftwarePackage` / `monitor_software_package` | package/platform tuple unique；file/hash/size、playbook refs、run identity | templates SET_NULL；删除还删物理文件 |
| `OpenSearchCluster` / `monitor_opensearch_cluster` | name unique；加密 password；default/check/sync | 业务单例/默认，但 DB 缺条件唯一 |
| `LogRetentionTier` / `monitor_log_retention_tier` | code unique；retention/rollover/default | service/log setting PROTECT |
| `LogProcessingRule` / `monitor_log_processing_rule` | cluster+application、name unique immutable、multiline/pipeline JSON | cluster CASCADE；application PROTECT |
| `LogCollectionFilterRule` / `monitor_log_collection_filter_rule` | name unique；regex/enabled | application nullable PROTECT |
| `LogCollectionTarget` / `monitor_log_collection_target` | install/runtime/config fingerprint state | host O2O CASCADE |

通知收件人属于 user × media binding，不属于 media 本身。每个地址的 delivery 独立去重和重试。

## 8. Scheduler、Audit 与 SysConfig

| Model / table | 语义 |
|---|---|
| `ScheduledTask` / `scheduler_scheduledtask` | name/code unique；menu SET_NULL；enabled/running；cron/interval；next/last/status/message |
| `ScheduledTaskLog` / `scheduler_scheduledtasklog` | task CASCADE；run/status/message/output/duration |
| `LoginAuditLog` / `audit_login_log` | username/user_id/request snapshot；user_id 非 FK |
| `OperationAuditLog` / `audit_operation_log` | actor/route/request/response/status/duration；user_id 非 FK |
| `SysConfig` / `sys_config` | key unique；value/default text；type string/int/bool/json/secret；readonly |

SysConfig 的 JSON 实际存成 text；secret 使用密码 hash，只能 verify。Go repository 不能把所有 value 都映射为 JSON。

## 9. 删除策略汇总

- `PROTECT` 是拓扑字典的主要保护：Project、Environment、Application、Version、Template、Profile、Service、Retention、Processing/Filter rule。
- HostGroup 删除会递归删除组和 Host，但先扫描 AutomationInventory 与 InspectionTask 的 JSON host IDs。
- 删除 Host 会关闭 WebSSH；配置类子记录多级 CASCADE；执行历史通过 SET_NULL + snapshots 保留。
- Workflow 对 task/workflow 的引用在 JSON 中，删除保护必须扫描所有 nodes。
- MonitorTarget 必须先 disabled 且完成 uninstall 才可删除。
- Log rule/tier 删除除 DB 引用外还涉及 OpenSearch 远端对象。
- SoftwarePackage 删除包含数据库记录和本地物理文件，当前不存在分布式事务。

## 10. Go/sqlc 建模规则

1. 以导出 DDL 为唯一 schema source，本文作为语义校验表。
2. Repository 输入输出与 generated sqlc models 分离，避免 DB nullable 类型泄露到 API。
3. 所有 aggregate replacement、删除链、状态 claim 使用显式 transaction。
4. 对 DB 未表达但业务必须的唯一性增加 migration，先检查并清洗历史重复数据。
5. 活动任务使用条件更新或 `SELECT ... FOR UPDATE`，不要“先查再写”。
6. 可逆密文与单向 hash 使用不同 Go 类型/API，禁止混用。
7. 配置引用逐步规范化；历史快照保持 append-only，并携带 schema version。
8. 所有 JSON ID 引用提供反向查询/删除保护；高频查询字段最终迁移到关系表。
9. 时间统一 `time.Time` UTC；duration 明确单位，避免裸 int。
10. 枚举定义为具名 string/int 类型，并在 service 与 DB migration 双层校验。