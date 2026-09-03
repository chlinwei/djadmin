# Agent 安装与更新（autoadmin）

本文档描述 Agent 安装（SSH 引导）与更新（gRPC 在线自更新）的**最终逻辑**。后端唯一实现为 Go 版 autoadmin；Django 后端（backend/）已废弃，不再维护对齐说明。

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
  - 说明：项目没有应用层心跳，Agent 在线判定权威是活跃的 gRPC 会话（`Gateway.IsOnline`），保活由 gRPC 内建 keepalive（30s ping / 10s 超时）承担；任务失联判定依据是任务行 `update_time` 的更新间隔。

前置校验全部通过后：

1. 校验 Agent 二进制（`../dj_agent/bin/dj-agent`）：拒绝旧 RabbitMQ 构建产物、要求含 `DJ_AGENT_GRPC_FILE_ADDR` 标记。
2. 加载 Agent 安装专用模板并完成上述校验。
3. 读取系统参数 `sys.assets.agent.grpc_advertise_addr`（缺失报错）。
4. 创建 `automation_execution_job`（running）+ 每主机 `assets_agent_job`（queued，action=install_agent）+ `automation_execution_host_log`；前端跳转 `/sys/automation/logs?job_id=<automation_job_id>` 看进度。
   - install：`job_type=ansible`，params 含 credential_id；无 agent_id 的主机作业行回填 `host-<id>` 占位。
   - update：`job_type=grpc`。

## install 链路（SSH + Ansible 引导，`agent_install.go`）

多主机并发执行（每主机一个 goroutine），单台流程：

1. agent_id 取主机绑定值，空则 `host-<id>`；按主机 IP 与对外地址计算 gRPC 地址（同机走 `127.0.0.1`）。
2. 建临时目录，写 inventory（JSON 格式）：
   - 密码凭证：解密后写入 `ansible_password`；SSH Key 凭证：解密后写私钥文件（0600）+ `ansible_ssh_private_key_file`。
   - 非 root 用户自动加 sudo become（含 become_password）。凭证解密失败/为空 → 该主机失败。
3. 写入二进制副本（0755）与模板内容 playbook，执行 `ansible-playbook -i inventory --timeout 10 -e dj_agent_binary_source/...`，超时 300 秒（进程组 SIGKILL）。
4. stdout 每秒流式回写 `assets_agent_job.stdout` 与 host log。
5. 结束判定：exit code ≠ 0 或 recap 的 failed/unreachable > 0 → 失败；成功后轮询 `gateway.IsOnline` 最多 10 秒确认 agent 回连 gRPC，未回连仍判失败。
6. 成功且主机原先无 agent_id 时，回填 `assets_host.agent_id`。
7. 超时：状态 `timeout`、exit 124。全部主机结束后汇总更新 automation job 的 status/result_summary。

## update 链路（gRPC 在线自更新，`agent_update.go`）

多主机顺序执行，单台流程：

1. 经 gRPC 文件通道把新二进制推到主机 `/var/lib/dj-agent/update/dj-agent.new`。
2. 下发 `apply_agent_update` 动作，参数为两个**文件内容**（非 playbook 本身）：
   - `env_content`：模板中 `config.env` 的 copy content 渲染 `{{ dj_agent_id }}` / `{{ dj_agent_grpc_addr }}` 后的结果。
   - `unit_content`：模板中 `dj-agent.service` 的 copy content。
3. agent 侧自替换二进制、重写配置与 unit 并重启；服务端轮询重连最多 15 秒作为最终结论。
4. 全部主机结束后汇总更新 automation job。

## 失败语义（两条链路一致）

- 任何步骤失败都落库（job/host log 状态 failed + error_message），不静默假成功。
- 模板缺失、解析失败或缺少 config.env / dj-agent.service 的 copy 任务：入口直接 400，任务不会创建。
- update 不自动回退到 install（避免两条链路交叉导致状态难排查）。
