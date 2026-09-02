# autoadmin 缺失接口补齐计划

> 背景：前端（fronted）已在调用、但 Go 新后端（autoadmin）尚未实现的接口清单与移植方案。
> 参考实现：`backend/`（Django），**只读参考，不做修改**。
> 覆盖范围结论：经全量比对 fronted/src/api 与 autoadmin/internal/api/router/router.go，共 9 个前端在用的缺口（A1~A9）；另有 2 个前端已不调用的死代码接口（POST /api/agent/jobs/create、POST /sys/scheduler/tasks/），默认跳过。

## 进度

| 编号 | 接口 | 状态 |
|---|---|---|
| A1 | POST /monitor/targets/:id/retry/ | ✅ 已完成（target_install.go + target_install_test.go，路由 router.go targets 组） |
| A2 | POST /monitor/packages/:id/sync-official/ | ✅ 已完成（package_sync.go） |
| A3 | POST /monitor/media/:id/test/ | ✅ 已完成（media_send.go + media_send_test.go） |
| A4 | GET /monitor/opensearch-clusters/:id/log-health/ + OpenSearch 同步 | ✅ 已完成（log_health.go 六层对账 + log_management.go 期望态/同步；挂接档位 CRUD、规则 CRUD、集群保存） |
| A5 | POST /sys/automation/playbooks/:id/upload/ | ✅ 已完成（playbook_files.go + playbook_files_test.go） |
| A6 | GET /sys/automation/playbooks/:id/download/ | ✅ 已完成（playbook_files.go） |
| A7 | WS /ws/automation/jobs/:id/logs/（作业日志实时推送） | ✅ 已完成（job_log_ws.go） |
| A8 | POST /assets/credentials/batch-create/（CSV 批量导入） | ✅ 已完成（credential_batch.go） |
| A9 | POST /user/changeAvatar（头像上传） | ✅ 已完成（identity/avatar.go） |

**全部 9 项缺口已于 2026-09-02 补齐。**

### A2/A5/A6/A7/A8/A9 实施记录（2026-09-02）

- **A2**：`internal/monitor/package_sync.go`。仅 node_exporter；版本正则 `^\d+(\.\d+){1,3}$`（去 v 前缀）；同名同平台唯一约束预检；GitHub release 流式下载（独立 60s 客户端、SHA-256 边下边算、200MiB 上限、临时文件+原子 rename），复用 `softwarePackageRelativePath` 落盘并清理旧文件。
- **A5/A6**：`internal/automation/playbook_files.go`。上传：multipart `file`、仅 .yml/.yaml、10MB 上限、UTF-8 校验、复用现有 `validatePlaybook`（与 Go Create/Update 一致），UPDATE content 后返回模板对象（前端取 `data.content`）；下载：`text/yaml` + `filename*=UTF-8''`，文件名清洗规则与 Django 一致（`[^A-Za-z0-9._-]+`→`-`，纯中文名会清洗成 `-`，属 Django 原行为）。
- **A7**：`internal/automation/job_log_ws.go`。鉴权复用 `Authenticate`（原生支持 `?token=`）+ `automation:jobs:view`；消息协议与前端 `logs/center/controller.js` 逐字段对齐（snapshot/output/status/completed，payload 含 job_id/status/data）；终态推送 completed 后服务端关闭，前端不再重连。增量策略：全量重建 + 前缀比较，能续传发增量、历史行被原地改写时退回 snapshot 重同步（对前端语义等价于 Django 的行级 offset）；轮询间隔读 sys_config `sys.automation.websocket.job_log_poll_interval_seconds`（缺省 0.5s，范围 [0.2,10]，只读不建行）。
- **A8**：`internal/assets/credential_batch.go`。CSV 表头与凭证字段同名（name/username/password/private_key/port/auth_type/remark，必含 username 列）；逐行调用现有 `CreateCredential`（加密/校验复用服务层）；整批遇错中止并提示行号；单次上限 1000 行/5MB。
- **A9**：`internal/identity/avatar.go`。multipart `avatar`，存 `<MEDIA_ROOT>/userAvatar/<时间戳><后缀>`，返回 `{new_file_name}`，不更新用户记录（与 Django 一致）；新增了 Django 没有的后缀白名单（png/jpg/jpeg/gif/webp）与 5MB 上限。

**路由注册汇总**（均在 `internal/api/router/router.go`）：
- `/monitor/packages/:id/sync-official/`（POST，monitor 组）
- `/sys/automation/playbooks/:id/upload/`（POST，`automation:playbooks:update`）、`/:id/download/`（GET，`automation:playbooks:view`）
- `/ws/automation/jobs/:id/logs/`（GET，`automation:jobs:view`）
- `/assets/credentials/batch-create/`（POST，`assets:credentials:create`）
- `/user/changeAvatar`（POST，仅登录，Django 路径无尾斜杠）

### A1/A3/A4 实施记录（2026-09-02）

- **A1**：`internal/monitor/target_install.go`。选包逻辑与 Django `package_selector.py` 一致（rpm/deb 精确匹配平台族+主版本+架构，tar.gz(any) 兜底，checksums 清单）；业务失败（无 agent/离线/无包/无 playbook/pending 冲突）以 `guardFailure` 类型区分，原因写入 `target.install_status/install_message` 后仍返回目标对象（Django 行为），仅 DB 错误返回 5xx。stale pending（>31 分钟，对应派发 goroutine 30 分钟兜底超时）自动置 failed 放行。
- **A3**：`internal/monitor/media_send.go`。recipients 支持数组/逗号/分号分隔；useTLS→STARTTLS、useSSL→隐式 TLS；gmail 强制 TLS；密码经 SecretEncryptor 解密；html 格式发 multipart/alternative；中文主题 RFC 2047 编码。错误文案与 Django 逐字对齐（"测试邮件发送失败：…"等）。
- **A4**：`log_health.go` 六层对账（索引模板/ISM 策略/解析 pipeline/主机配置/采集进程/数据写入），复用 `openSearchRequest`，错误形如 "opensearch 404 …" 用于区分对象缺失与集群异常。`log_management.go` 移植期望态构建（`STANDARD_LOG_FIELDS`、ISM policy、pipeline 签名）与同步编排：
  - 档位增删改 → goroutine 对所有启用集群 bootstrap（Django celery `_apply_policies` 的等价实现）；
  - 解析规则增改 → 先发布 pipeline 再落库（saveResource 新增 beforeSave 钩子），删除 → 先校验引用（`assets_application_log_definition.processing_rule_id`）再删 pipeline 再删记录；
  - 集群保存 → 异步 bootstrap 并回写 `storage_sync_status/error/time`；
  - 档位删除增加引用守卫（service/log_setting 两表）。
- **与 Django 的已知差异**（有意为之）：
  - `host_configs` 层只判断"未安装/从未下发"，不做指纹内容比对——Go 侧 Fluent Bit 配置由 agent 渲染（`configure_fluent_bit_opensearch`），后端没有期望指纹生成器；内容漂移由数据流层兜底。
  - Go 侧不移植 Celery，全部异步点用 goroutine。
- **注意**：`loadOpenSearchCluster` 的 SELECT 增加了 `enabled` 列，相关 sqlmock（opensearch_pipeline_test.go）已同步更新。

## 公共约定

- 响应 envelope：`response.Success` / `response.BusinessError`（HTTP 恒 200，业务码区分成败），见 `internal/api/response/response.go`。
- monitor 模块无 service/repository 层，handler 直接持有 `*sql.DB`；时间统一 `time.Now().UTC()`。
- 密码/密钥加解密：`assets.SecretEncryptor`（handler.secrets），与 Django Fernet（`ASSETS_CREDENTIAL_ENCRYPTION_KEY`/SECRET_KEY 派生）互通。
- Django 异步任务（Celery）在 Go 中一律用 goroutine 等价替代。
- 新逻辑一律放新文件，最小侵入工作区已有未提交改动；路由集中在 `internal/api/router/router.go` 对应分组旁追加。

## A1：目标重试 `POST /monitor/targets/:id/retry/`

- 前端：`fronted/src/api/monitor.js:117`（`retryManagedTarget`），调用方 `views/monitor/index.vue:2642`（managed 重新下发，覆盖重试失败与修复历史遗留，不校验 install_status；响应 data 直接 Object.assign 回行）。
- Django 参考：`monitor/views.py:470-492`（retry）、`assets/views.py:1168-1313/1374-1470`（exporter 安装/卸载派发）、`monitor/job_runner.py`、`monitor/playbook_runner.py`。
- Go 落点：新文件 `internal/monitor/target_install.go`，复刻 `log_target_actions.go:194` `dispatchLogTargetInstall` 模式：
  1. agent 在线检查（`handler.gateway.IsOnline`）。
  2. `monitor_target_install_history` pending 防重（有 pending 且未过期则报错）。
  3. 按 `managed_enabled` 定 action=install/uninstall；`retry_count=0`、`install_message='人工触发重试'`。
  4. 按 host 平台选 `monitor_software_package`（name = target.exporter_type，os/arch/platform 匹配，enabled，文件齐全）。
  5. 读 playbook 内容（install/uninstall_playbook_template_id）。
  6. 写 `automation_execution_job`（status=pending，inventory/extra_vars 快照）+ `monitor_target_install_history`（trigger_type=manual，exporter_type_snapshot）。
  7. 置 target `install_status='pending'`、`last_dispatch_manual=TRUE`。
  8. goroutine：`handler.jobs.RunJobByID(ctx, jobID)` → 回写 history/target 终态（同 fluent-bit 版）。
- extra_vars：对齐 Django L1258-1276（exporter_name/version/service_name/service_file_content/service_run_as_user/service_run_as_group/package_local_path/package_file_name/package_format/package_sha256 等；卸载仅 exporter_name/service_name）。
- 响应：更新后的 target 对象（复用 GetTarget 响应组装）。
- 验收：前端 managed 目标"重新下发"可用；单测参照 `log_target_actions_test.go`。

## A2：软件包官方同步 `POST /monitor/packages/:id/sync-official/`（待做）

- 前端：`monitor.js:25`（timeout 60s），调用方 `views/monitor/index.vue:1678`。
- Django 参考：`monitor/views.py:1145-1190`。
- 方案：新文件 `internal/monitor/package_sync.go`；仅 node_exporter；版本正则 `^\d+(\.\d+){1,3}$`；唯一约束预检；流式下载 GitHub tarball（60s 超时、SHA-256、200MB 上限），复用 `UploadSoftwarePackage`（package_files.go）的临时文件+rename 落盘模式；UPDATE version/file/sha256/size_bytes。

## A3：告警邮件测试 `POST /monitor/media/:id/test/`

- 前端：`monitor.js:97`，调用方 `views/monitor/media/index.vue:293`（body：recipients/subject/message）。
- Django 参考：`monitor/views.py:1451-1475`、`monitor/tasks.py:86-119`（send_smtp_email）。
- Go 落点：新文件 `internal/monitor/media_test.go`：
  - recipients 支持数组或逗号/分号分隔字符串；subject/message 必填；仅 media_type=email。
  - `net/smtp`：useTLS→STARTTLS（默认 587）、useSSL→隐式 TLS（465）；authType=password 时 SMTP AUTH；密码 `handler.secrets.Decrypt`；messageFormat=html 时 text/html + 纯文本 alternative；不写库。
  - 响应 `{"sent":true}`；失败 BusinessError 400 带原因。
- 验收：媒体页面"发送测试邮件"可用；参数解析部分补单测。

## A4：日志链路健康 `GET /monitor/opensearch-clusters/:id/log-health/` + OpenSearch 同步（完整对齐）

- 前端：`monitor.js:173`，调用方 `views/monitor/log-storage/index.vue:287`（期望 `{status, layers:[{key,status,...}]}`，自动展开非 ok 层）。
- Django 参考：`monitor/log_health.py`（六层对账）、`monitor/log_management.py`（期望态构建）、`monitor/tasks.py`（log_storage_sync 异步任务）、`views.py:1915-1985`（档位 CRUD `_apply_policies`）。
- Go 落点：两个新文件：
  - `internal/monitor/log_health.go`：六层只读对账（index template / ISM 策略 / ingest pipeline / 主机配置指纹 / 运行时状态 / 数据流聚合），复用 `opensearch.go` 的 `openSearchRequest`+`loadOpenSearchCluster`；响应 `{status(ok|warn|drift|error), checked_at, layers:[{key,name,status,summary,items:[{name,status,detail}]}]}`；层内异常吞掉转为该层 error，不返回 4xx。
  - `internal/monitor/log_management.go`：移植期望态构建（index template name/body、ISM policy name/body、pipeline body、fluent-bit 主机片段指纹）+ `log_storage_sync`（按启用集群 upsert index template + ISM policies + pipelines）；挂接到 `config_resources.go` 的保留档位/处理规则增删改（goroutine 异步）；处理规则删除同步删 pipeline。
- 说明：不移植的话 Go 界面新建的策略会一直显示"漂移"，故本轮完整对齐。

## A5/A6：Playbook 上传/下载（待做）

- 前端：`sys/automation.js:21-27`，调用方 `views/automation/templates/index.vue:371/388`。
- 方案：上传——multipart `file`、仅 .yml/.yaml、UTF-8 校验、复用 `validatePlaybook`（与 Go 现有 Create/Update 一致的 YAML 校验），UPDATE content；下载——`text/yaml` + `Content-Disposition: attachment; filename*=UTF-8''`（参考 audit `writeTextDownload`）。
- Django 参考：`automation/views_playbook.py:208-244`。

## A7：作业日志 WebSocket `GET /ws/automation/jobs/:id/logs/`（待做）

- 前端：`views/automation/logs/center/controller.js:817-866`（`?token=` 鉴权；消息协议 `{type:'snapshot',data:{data}}` / `{type:'output',data:{data}}` / `{type:'status'}` / `{type:'completed'}`；未完成断线自动重连）。
- 方案：新文件 `internal/automation/job_log_ws.go`；gorilla/websocket（与 webssh 同栈）；~1s 轮询 `automation_execution_host_log` + `automation_execution_job.status`；终态推 completed 后关闭。实现前需精读 controller.js 确认字段形态。

## A8：凭证 CSV 批量导入 `POST /assets/credentials/batch-create/`（待做）

- 前端：`assets/credential/index.js:28`，调用方 `views/assets/credential/index.vue:355`（仅 .csv）。
- Django 参考：`assets/views.py:310-322`（csv.DictReader → serializer many）。
- 方案：`encoding/csv` 解析，逐行走现有凭证创建逻辑（SecretEncryptor 加密）；前端只看成功态。

## A9：头像上传 `POST /user/changeAvatar`（待做）

- 前端：`views/userCenter/components/Avatar.vue:4`（a-upload action，字段名 avatar，Authorization 头）。
- Django 参考：`user/views.py:514-531`（存 `<MEDIA_ROOT>/userAvatar/<时间戳><后缀>`，返回 `{new_file_name}`，不更新 user 记录）。
- 方案：新文件 `internal/identity/avatar.go`，行为与 Django 完全一致。

## 验证约定

- 每项完成后：`go build ./...` + `go vet` + 相关单测；全部完成后 `go test ./...` 回归。
- 动作类 handler 参照 `log_target_actions_test.go` 风格补单测。
