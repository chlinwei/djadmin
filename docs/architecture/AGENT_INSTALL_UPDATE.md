# Agent 安装与更新（autoadmin）

本文档描述 Agent 安装（SSH 引导）与更新（gRPC 在线自更新）的**最终逻辑**。后端唯一实现为 Go 版 autoadmin；Django 后端已废弃、源码已移出版本库，不再维护对齐说明。

## 唯一配置源：Agent 安装专用模板

- 配置源是 **`automation_playbook_template` 表中 `category='agent'` 的模板**（name 固定 `agent_install`，内容即 agent_install playbook）。
- 种子数据：golang-migrate 迁移 `autoadmin/db/migrations/000001_agent_playbook_template.up.sql`（幂等，`category='agent'` 不存在时插入；`.down.sql` 支持回滚）。与 Django 一条命令等价：
  `autoadmin migrate`（读取 `MIGRATION_SOURCE_URL`/`MIGRATION_DATABASE_URL` 配置）
- 通过前端"自动化 → Playbook模板"页面筛选"Agent 安装专用"分类，即可**编辑 / 上传覆盖 / 下载**，与普通 playbook 模板一致；修改后下一次安装/更新请求立即生效，无需重启。
- 定位只按 `category='agent'`（不按 name），约定该分类仅一条模板；Go 侧取该分类最新一条（`ORDER BY id DESC`）。

### 保护规则（autoadmin internal/automation/handler.go + 前端）

- **禁止手动新建** agent 分类模板（Create 拦截，种子 SQL 是唯一落库通道）。
- **禁止删除**（Delete 拦截 + 前端隐藏删除按钮）。
- **禁止改回其他分类**（Update 拦截 + 前端分类下拉锁定）；名称/描述/内容/上传覆盖不受限。
- 内容可随意编辑，但改坏后安装/更新会报错——这是无兜底设计的刻意语义。

### 校验语义（入口一次性完成，任一步失败返回 400，不创建任务）

1. 读模板：查不到/内容为空 → `未找到 Agent 安装专用模板`。
2. YAML 解析失败 → 报错。
3. 提取 `dest: /etc/dj-agent/config.env` 与 `dest: /usr/lib/systemd/system/dj-agent.service` 两个 copy 任务的 content 模板，缺失任一 → 报错。

## 入口

`POST /api/agent/install`（`internal/assets/agent_update.go` 的 `AgentInstall`），权限 `assets:hosts:update`。

请求：`{host_ids, operation, credential_id}`；`operation` 缺省为 `install`。

- `operation=install`：必须带有效的 `credential_id`（`assets_credential` 中存在）；不要求主机已绑定/在线 Agent。
- `operation=update`：不要求 `credential_id`；要求每台主机已绑定 Agent 且当前在线。
- 两种操作共用同一拦截：目标主机存在活跃（queued/running 且 30 秒内 `update_time` 有更新）的 `install_agent` 任务时拒绝；超时 30 秒无更新的旧任务先标记失败。
  - 取消联动：在运行记录中心「取消」自动化作业（`CancelJob`）时，会一并把该作业下 `assets_agent_job`（action=install_agent）与 `automation_execution_host_log` 的 queued/running 置为 failed，否则 agent job 仍算活跃，下次安装/更新会被这条拦截误挡（agent job 与 automation job 是两套状态）。
  - 说明：项目没有应用层心跳，Agent 在线判定权威是活跃的 gRPC 会话（`Gateway.IsOnline`），保活由 gRPC 内建 keepalive（30s ping / 10s 超时）承担；任务失联判定依据是任务行 `update_time` 的更新间隔。
  - 死会话清理：`Gateway.Execute` 向会话发送帧失败（底层传输已断，典型报错 `transport is closing`）时，网关立即把该会话从 sessions 摘除（`dropSession`，同 ID 新连接顶替时不误删）并关闭其全部 pending 管道——等待方立刻得到 `agent offline` 语义，`IsOnline` 不再对死连接误报在线；agent 侧随后自动重连重建会话。

前置校验全部通过后：

1. 加载 Agent 二进制（见下节"二进制来源选择"）：激活包优先，回退构建产物；无论哪种来源都做双标记校验（拒绝旧 RabbitMQ 构建产物、要求含 `DJ_AGENT_GRPC_FILE_ADDR`）。
2. 加载 Agent 安装专用模板并完成上述校验。
3. 读取系统参数 `sys.assets.agent.grpc_advertise_addr`（缺失报错）。
4. 创建 `automation_execution_job`（running）+ 每主机 `assets_agent_job`（queued，action=install_agent）+ `automation_execution_host_log`；前端跳转 `/sys/automation/logs?job_id=<automation_job_id>` 看进度。
   - install：`job_type=ansible`，params 含 credential_id；用于部署后回连的 `dj_agent_instance_name` 渲染变量取主机的 `assets_host.instance_name`（前端创建主机时必填，见 [主机身份](DJ_AGENT_ARCHITECTURE.md#2-主机身份实例名instance_name)），不再用 `host-<id>` 之类的占位标识。
   - update：`job_type=grpc`。

响应 `data` 携带 `agent_package` 字段，供前端展示本次使用的包来源：

```json
{
  "automation_job_id": 123,
  "jobs": [{"job_id": "...", "host_id": 1}],
  "agent_package": {"source": "uploaded", "sha256": "<64位hex>"}
}
```

`source` 取值 `uploaded`（已上传激活包）或 `build`（本机构建产物）。

agent 二进制自身内嵌版本元数据（`dj_agent/internal/buildinfo`，源码默认 `dev`/`none`，Makefile 构建时经 `-ldflags -X` 注入 `git describe` 结果），出现在 agent 启动日志、运行时状态接口及 `dj-agent --version`（`-v`）输出中。gRPC `Hello` 握手帧携带 `version`，autoadmin 网关校验通过后写入 `assets_hostsystem.agent_version`，因此安装/更新重启后主机列表版本即自动刷新；上传包管理已删除版本号概念，该内嵌版本不参与包管理与服务端来源判定。

## 二进制来源选择（`loadAgentBinary`，agent_update.go）

1. 优先查 `agent_package` 表中 `is_active=1` 的记录，从 mediaRoot + `file` 读取二进制并校验 sha256 与记录一致；激活包存在但文件缺失 / sha256 不匹配 / 标记校验失败时**直接报错，不静默回退**——操作者显式激活的包损坏应暴露问题。
2. 无激活包记录时回退历史行为：读取 `../dj_agent/bin/dj-agent` 并做双标记校验。

## dj-agent 安装包管理（`agent_package` 表 + agent_package.go）

- 存储：文件落盘 `<mediaRoot>/agent_packages/default/dj-agent`（mediaRoot 解析与 monitor 软件包相同，默认 autoadmin 工作目录下的 `media/` 取绝对路径；Django 后端废弃后媒体根已从 Django MEDIA_ROOT 迁出）；单槽位"当前包"语义，存储目录与 DB `version` 列固定 `default`（历史遗留列，不对外暴露）；记录含 `file`（相对 mediaRoot 路径）/`sha256`/`size_bytes`/`is_active`/`create_time`（迁移 `000013_agent_package`）。
- API（均挂 `Authenticate + RequirePermission("assets:hosts:update")`，与 `/api/agent/install` 相同中间件链）：
  - `GET /api/agent/packages/`：查询当前包，响应直接是包对象 `{id,file,sha256,size_bytes,is_active,create_time}`，无包时为空对象（非列表）。
  - `GET /api/agent/packages/download/`：下载当前包二进制（`Content-Disposition: attachment; filename="dj-agent"`）；未上传或文件缺失返回 404 业务错误；路径限制在 mediaRoot 内防目录穿越。
  - `POST /api/agent/packages/upload/`：multipart 上传，字段仅 `file`（二进制）。校验 ≤200MiB 与双字节标记；`sha256` 服务端计算；重复上传覆盖文件并更新记录；上传成功即在事务内独占激活。
  - `POST /api/agent/packages/batch-delete/`：body `{"ids":[...]}`，响应 `{"count":n,"results":[{"id","ok","message"}]}`（项目批删约定，删除当前包传 `ids:[id]`）；删除记录时同步删除磁盘文件（路径限制在 mediaRoot 内，防目录穿越）。
  - `POST /api/agent/packages/:id/activate/`：历史保留接口；单包语义下上传即激活，前端不再使用。
- 上传/激活包的校验标记与构建产物完全一致（`validateAgentBinary` 共用）。

## 前端交互（fronted）

- **API 层**：`fronted/src/api/assets/agentPackage.js` 封装 `listAgentPackages()`、`uploadAgentPackage(file)`（FormData，走 `requestUtil.fileUpload`）、`downloadAgentPackage()`（blob，走 `requestUtil.download`）、`batchDeleteAgentPackages(ids)`（唯一批删接口，单删传 `ids:[id]`，遵循项目删除约定）。
- **主机列表"批量管理 Agent"弹窗**（`fronted/src/views/assets/host/index.vue`）：顶部展示当前包状态（sha256 前 12 位 / 大小 / 上传时间）；列表接口拉取失败不阻塞安装/更新流程。无包时显示警示：将回退使用服务端构建产物 `dj_agent/bin/dj-agent`（路径依赖部署目录，不可靠）。
- **"Agent 包管理"弹窗**：两个入口——主机列表工具栏"Agent 包"按钮（**无需选择主机**，纯包管理场景）与"批量管理 Agent"弹窗内的"Agent 包管理"链接，打开同一弹窗。单包语义：无表格列表，直接用 descriptions 展示当前包（状态/sha256/大小/上传时间），三个操作——上传（有包时文案"重新上传（覆盖）"，点击后由隐藏的 `<input type="file">` 直接唤起系统文件选择，选中即上传覆盖，不再弹出二次上传弹窗）、下载（blob 按统一鉴权拉取，落盘文件名 `dj-agent`）、删除（`openDeleteConfirm` 二次确认，按 sha256 标识，走批删接口）。
- **提交反馈**：`POST /api/agent/install` 响应携带 `agent_package` 时，成功提示追加来源——`uploaded` 显示"（包：uploaded，sha256 xxx）"，`build` 显示"（包：构建产物）"；响应无该字段时（旧后端）保持原提示不变。

## install 链路（SSH + Ansible 引导，`agent_install.go`）

多主机并发执行（每主机一个 goroutine），单台流程：

1. 取主机 `instance_name` 作为 agent 身份；实例名为空直接判该主机失败（提示在主机列表补填实例名），不再有占位或 IP 派生兜底。按主机 IP 与对外地址计算 gRPC 地址（同机走 `127.0.0.1`）。
2. 建临时目录，写 inventory（JSON 格式）：
   - 密码凭证：解密后写入 `ansible_password`；SSH Key 凭证：解密后写私钥文件（0600）+ `ansible_ssh_private_key_file`。
   - 非 root 用户自动加 sudo become（含 become_password）。凭证解密失败/为空 → 该主机失败。
3. 写入二进制副本（0755）与模板内容 playbook，执行 `ansible-playbook -i inventory --timeout 10 -e dj_agent_binary_source/... -e dj_agent_instance_name=<instance_name>`，超时 300 秒（进程组 SIGKILL）。命令统一由 `internal/automation/ansiblecmd.CommandContext` 构造（见下）。
4. stdout 每秒流式回写 `assets_agent_job.stdout` 与 host log。
5. 结束判定：exit code ≠ 0 或 recap 的 failed/unreachable > 0 → 失败；成功后按实例名轮询 `gateway.IsOnline(instance_name)` 最多 10 秒确认 agent 回连 gRPC，未回连仍判失败。
6. 不回填任何主机标识：主机身份只有实例名，安装流程不改写 `assets_host`（实例名由创建主机时保证）。
7. 超时：状态 `timeout`、exit 124。全部主机结束后汇总更新 automation job 的 status/result_summary。

### Ansible 运行方式（构建开关，2026-09-17）

`ansible-playbook` 的执行统一走 `internal/automation/ansiblecmd`，按构建标签分两个变体：

| 变体 | 构建 | 运行时依赖 | 产物 |
|---|---|---|---|
| 默认 | `make build` | 部署机自带 Python + `ansible-playbook`（PATH 查找） | 30MB |
| 内嵌 | `make ansible-embed && make build`（默认即内嵌；`ANSIBLE_EMBED=` 可关闭） | 无（自带 CPython + ansible-core） | ~74MB |

内嵌变体（`-tags embedansible`）：把 CPython 3.11 与 `requirements.txt` 钉的 `ansible-core==2.16.14`（及依赖）经 `go:embed` 打进二进制，首次运行解压到临时目录（约 1s，之后走内容哈希缓存）。ansible 路径通过嵌入 Python 的 `.pth` 注入，**不设 `PYTHONHOME/PYTHONPATH`**，避免被 Ansible 的 local 连接继承而污染目标端 Python；同时用自带的空 `ansible.cfg` 屏蔽部署机的 `/etc/ansible/ansible.cfg`。

为什么钉 2.16：ansible-core **2.17 起要求目标机 Python ≥3.7**，老目标机（如 CentOS 7 的 Python 3.6）会因 `module_utils/basic.py` 的 `from __future__ import annotations` 报 SyntaxError；2.16 是最后支持目标机 3.6 的版本。嵌入数据由 `make ansible-embed` 生成（不入库，见 `.gitignore`），改用 `requirements.txt` 里的版本后需重跑。

> ⚠️ `ansible-core` 是 **GPL-3.0-or-later**，随二进制分发（尤其内嵌变体）需评估许可证义务。

## update 链路（gRPC 在线自更新，`agent_update.go`）

多主机顺序执行，单台流程：

1. 经 gRPC 文件通道把新二进制推到主机 `/var/lib/dj-agent/update/dj-agent.new`。
2. 下发 `apply_agent_update` 动作，参数为两个**文件内容**（非 playbook 本身）：
   - `env_content`：模板中 `config.env` 的 copy content 渲染 `{{ dj_agent_instance_name }}` / `{{ dj_agent_grpc_addr }}` 后的结果（实例名取主机 `instance_name`）。
   - `unit_content`：模板中 `dj-agent.service` 的 copy content。
3. agent 侧自替换二进制、重写配置与 unit 并重启；服务端轮询重连最多 15 秒作为最终结论。
4. 全部主机结束后汇总更新 automation job。

## 失败语义（两条链路一致）

- 任何步骤失败都落库（job/host log 状态 failed + error_message），不静默假成功。
- 模板缺失、解析失败或缺少 config.env / dj-agent.service 的 copy 任务：入口直接 400，任务不会创建。
- update 不自动回退到 install（避免两条链路交叉导致状态难排查）。
