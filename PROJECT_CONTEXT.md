# 项目上下文文档 - djadmin 运维管理平台

## 项目概述

面向 IT 运维团队的管理平台，提供主机资产管理、凭证管理、定时任务调度、用户权限管理等功能。

- **后端**: Django 5.1 + Django REST Framework + MySQL
- **前端**: Vue 3 + Vite + Ant Design Vue
- **调度**: Celery + RabbitMQ（Worker / Beat 独立进程）
- **认证**: JWT（rest_framework_jwt，1天有效期）
- **远程连接（已实现）**: Web SSH（基于 Channels + Paramiko + xterm，含在线会话与文件管理）

---

## 后端 APP 模块与 API 接口

### 主路由前缀

```
sys/        → user, role, menu, sys_config
sys/scheduler/ → scheduler
assets/     → assets (credentials, applications, hosts, host-groups)
media/      → 静态文件（头像等）
```

---

### 1. 用户模块（user）

**数据模型**

- `SysUser`：用户（id, username, password, avatar, email, phonenumber, status, timezone, create_time, update_time, remark）
- `SysUserRole`：用户-角色关联（user_id, role_id，唯一约束）

**API 接口**

| 方法   | 路径                                   | 功能                                     |
| ------ | -------------------------------------- | ---------------------------------------- |
| POST   | `/sys/login`                         | 登录（返回 token + 菜单树 + 角色权限码） |
| POST   | `/sys/usercenter/updateUserInfo`     | 更新个人信息（邮箱、手机）               |
| POST   | `/sys/usercenter/updateUserPassword` | 修改密码                                 |
| POST   | `/sys/changeAvatar`                  | 上传头像文件                             |
| GET    | `/sys/users/`                        | 用户列表（搜索/排序/分页，含角色信息）   |
| GET    | `/sys/users/{id}/`                   | 用户详情                                 |
| POST   | `/sys/users/`                        | 新增用户（默认密码123456）               |
| PATCH  | `/sys/users/{id}/`                   | 编辑用户                                 |
| GET    | `/sys/users/checkUserName`           | 检查用户名是否已存在                     |
| DELETE | `/sys/users/userBatchDelete`         | 批量删除用户（级联删除用户角色关联）     |
| POST   | `/sys/users/resetUserPwd`            | 重置密码为 123456                        |
| POST   | `/sys/users/assginUserRoles`         | 分配用户角色（全量替换）                 |
| POST   | `/sys/users/changeUserStatus`        | 启用/禁用用户                            |
| GET    | `/sys/users/getUserRolesById`        | 查询指定用户的角色列表                   |
| GET    | `/sys/users/current`                 | 获取当前登录用户信息（含时区）           |

---

### 2. 角色模块（role）

**数据模型**

- `SysRole`：角色（id, name, code, create_time, update_time, remark）

**API 接口**

| 方法   | 路径                                  | 功能                                         |
| ------ | ------------------------------------- | -------------------------------------------- |
| GET    | `/sys/roles/`                       | 角色列表（搜索/排序/分页）                   |
| GET    | `/sys/roles/{id}/`                  | 角色详情                                     |
| POST   | `/sys/roles/`                       | 新增角色                                     |
| PATCH  | `/sys/roles/{id}/`                  | 编辑角色                                     |
| DELETE | `/sys/roles/batch-delete`           | 批量删除角色（级联删除用户-角色、角色-菜单） |
| GET    | `/sys/roles/getCurrentUserRoleList` | 获取当前登录用户的角色列表                   |

---

### 3. 菜单模块（menu）

**数据模型**

- `SysMenu`：菜单（id, name, icon, parent_id, order_num, path, component, menu_type[M/C/F], perms, location, create_time, update_time, remark）
- `SysRoleMenu`：角色-菜单关联（role_id, menu_id，唯一约束）

**API 接口**

| 方法   | 路径                               | 功能                                |
| ------ | ---------------------------------- | ----------------------------------- |
| GET    | `/sys/menus/`                    | 菜单列表                            |
| GET    | `/sys/menus/{id}/`               | 菜单详情                            |
| POST   | `/sys/menus/`                    | 新增菜单                            |
| PATCH  | `/sys/menus/{id}/`               | 编辑菜单                            |
| GET    | `/sys/menus/getMenuTree`         | 获取完整菜单树（按 order_num 排序） |
| GET    | `/sys/menus/getMenuListByRoleId` | 查询角色拥有的菜单 ID 列表          |
| POST   | `/sys/menus/grantMenu`           | 批量分配菜单给角色（全量替换）      |
| DELETE | `/sys/menus/deleteMenuById`      | 删除菜单（级联删除角色-菜单关联）   |

---

### 4. 资产模块（assets）

#### 4.1 凭证管理（Credential）

**数据模型**

- `Credential`：凭证（id, name, username, password, private_key, port[默认22], auth_type[1=密码/2=SSH Key], create_time, update_time, remark）

| 方法   | 路径                                 | 功能                       |
| ------ | ------------------------------------ | -------------------------- |
| GET    | `/assets/credentials/`             | 凭证列表（搜索/排序/分页） |
| GET    | `/assets/credentials/{id}/`        | 凭证详情                   |
| POST   | `/assets/credentials/`             | 新增凭证                   |
| PATCH  | `/assets/credentials/{id}/`        | 编辑凭证                   |
| DELETE | `/assets/credentials/batch-delete` | 批量删除凭证               |
| POST   | `/assets/credentials/batch-create` | 批量导入凭证（CSV 文件）   |

#### 4.2 应用管理（Application）

**数据模型**

- `Application`：应用（id, name[唯一], version, create_time, update_time, remark）

| 方法   | 路径                                  | 功能                       |
| ------ | ------------------------------------- | -------------------------- |
| GET    | `/assets/applications/`             | 应用列表（搜索/排序/分页） |
| GET    | `/assets/applications/{id}/`        | 应用详情                   |
| POST   | `/assets/applications/`             | 新增应用                   |
| PATCH  | `/assets/applications/{id}/`        | 编辑应用                   |
| DELETE | `/assets/applications/batch-delete` | 批量删除应用               |

#### 4.3 主机分组管理（HostGroup）

**数据模型**

- `HostGroup`：主机分组（id, name[唯一], parent[自关联，支持多级嵌套], create_time, update_time, remark）

| 方法   | 路径                                 | 功能                         |
| ------ | ------------------------------------ | ---------------------------- |
| GET    | `/assets/host-groups/`             | 分组列表（搜索/排序/分页）   |
| GET    | `/assets/host-groups/{id}/`        | 分组详情                     |
| POST   | `/assets/host-groups/`             | 新增分组                     |
| PATCH  | `/assets/host-groups/{id}/`        | 编辑分组                     |
| DELETE | `/assets/host-groups/{id}/`        | 删除分组                     |
| DELETE | `/assets/host-groups/batch-delete` | 批量删除分组                 |
| GET    | `/assets/host-groups/tree`         | 获取树形结构（含嵌套子分组） |

#### 4.4 主机管理（Host）

**数据模型**

- `Host`：主机（id, instance_name, name, ip, instance_id, cloud_account[FK], group[FK], status, is_deleted_in_cloud, port[默认22], collect_status[unknown/success/failed], collect_message, collect_time, create_time, update_time, remark）
- `HostCredential`：主机-凭证关联（host, credential, is_default）
- `HostHardware`：硬件信息（host[1:1], cpu_cores, cpu_model, memory_gb, disk_total_gb, architecture, collected_at）
- `HostSystem`：系统信息（host[1:1], os_type, os_version, kernel_version, hostname, agent_version, collected_at）
- `HostDisk`：磁盘分区（host[FK], device, mount_point, size_gb, used_gb, filesystem）

| 方法   | 路径                                            | 功能                                                                            |
| ------ | ----------------------------------------------- | ------------------------------------------------------------------------------- |
| GET    | `/assets/hosts/`                              | 主机列表（支持 group_id 过滤、递归子分组、collect_status 过滤、搜索/排序/分页） |
| GET    | `/assets/hosts/{id}/`                         | 主机详情（含硬件、系统、磁盘信息）                                              |
| POST   | `/assets/hosts/`                              | 新增主机                                                                        |
| PATCH  | `/assets/hosts/{id}/`                         | 编辑主机                                                                        |
| DELETE | `/assets/hosts/{id}/`                         | 删除主机                                                                        |
| DELETE | `/assets/hosts/batch-delete`                  | 批量删除主机                                                                    |
| POST   | `/assets/hosts/{id}/collect-info`             | 采集单台主机信息（SSH 连接，更新硬件/系统/磁盘）                                |
| POST   | `/assets/hosts/batch-collect-info`            | 批量采集指定主机信息                                                            |
| POST   | `/assets/hosts/collect-all`                   | 采集所有主机信息                                                                |
| GET    | `/assets/hosts/{id}/webssh-sessions/`         | 获取主机 WebSSH 会话日志列表                                                    |
| GET    | `/assets/hosts/{id}/webssh-active-count/`     | 获取当前在线会话人数                                                            |
| GET    | `/assets/hosts/{id}/webssh-active-sessions/`  | 获取当前在线会话明细（会话ID、用户名、开始时间）                                |
| GET    | `/assets/hosts/{id}/files/list/?path=...`     | 获取远端目录列表                                                                |
| GET    | `/assets/hosts/{id}/files/download/?path=...` | 下载远端文件（流式返回，支持 HTTP Range）                                       |
| POST   | `/assets/hosts/{id}/files/upload/chunk/`      | 上传文件到远端目录（当前上传接口名保留为 chunk，单请求上传）                    |
| POST   | `/assets/hosts/{id}/files/rename/`            | 重命名远端文件/目录                                                             |
| DELETE | `/assets/hosts/{id}/files/delete/`            | 删除远端文件/目录（目录支持递归）                                               |
| POST   | `/assets/hosts/{id}/files/create-dir/`        | 新建远端目录                                                                    |
| POST   | `/assets/hosts/{id}/files/create-file/`       | 新建远端空文件（后端能力保留）                                                  |

说明：历史上的 ticket/status/cancel 相关接口文档已下线，当前实现以 hosts 下的 WebSSH 文件接口为准。

**采集状态说明**

- `collect_status`：`unknown`（未采集）/ `success`（采集成功）/ `failed`（采集失败）
- `collect_message`：最近一次采集失败原因（成功时为空字符串）
- `collect_time`：最近一次采集时间（UTC）
- 前端主机列表支持按 `collect_status` 过滤，并对失败主机高亮显示

---

### 5. 定时任务模块（scheduler）

**数据模型**

- `ScheduledTask`：计划任务（id, name[唯一], code[唯一], description, menu[FK], enabled, is_running, interval_minutes, last_run_time, next_run_time, last_result, created_at, updated_at）
- `TaskExecutionLog`：执行日志（task[FK], started_at, ended_at, status, output, error_message）

| 方法   | 路径                                         | 功能                                                                      |
| ------ | -------------------------------------------- | ------------------------------------------------------------------------- |
| GET    | `/sys/scheduler/tasks/`                    | 任务列表（搜索/排序/分页）                                                |
| GET    | `/sys/scheduler/tasks/{id}/`               | 任务详情                                                                  |
| POST   | `/sys/scheduler/tasks/`                    | 新增任务                                                                  |
| PATCH  | `/sys/scheduler/tasks/{id}/`               | 编辑任务                                                                  |
| DELETE | `/sys/scheduler/tasks/{id}/`               | 删除任务                                                                  |
| POST   | `/sys/scheduler/tasks/{id}/toggle-enabled` | 切换启用/禁用状态                                                         |
| POST   | `/sys/scheduler/tasks/{id}/enable`         | 启用任务                                                                  |
| POST   | `/sys/scheduler/tasks/{id}/disable`        | 禁用任务                                                                  |
| POST   | `/sys/scheduler/tasks/{id}/run-now`        | 立即执行（有并发保护，is_running 防重入；若 Worker 不在线会直接返回错误） |
| POST   | `/sys/scheduler/tasks/start-scheduler`     | 启用调度分发开关（Celery Beat 仍需独立进程运行）                          |
| POST   | `/sys/scheduler/tasks/stop-scheduler`      | 关闭调度分发开关（Celery Beat 进程可继续运行）                            |

**主机批量采集任务状态语义**

- `collect_all_hosts_info` 对单台主机失败采取“记录并继续”策略，不会因为单台失败中断整个批次。
- 因此定时任务总体状态通常按“任务函数是否抛异常”判定：单台失败不一定导致任务总体失败。
- `dispatch_due_tasks` 在 Worker 不可用时不会继续投递执行任务，并会在任务记录中写入“Worker 不可用，任务未投递”的失败提示，避免静默 pending。

**Celery 任务清单（当前全量）**

| Celery 任务名                        | 定义位置                                | 触发方式                                                                              | 说明                                                         |
| ------------------------------------ | --------------------------------------- | ------------------------------------------------------------------------------------- | ------------------------------------------------------------ |
| `scheduler.dispatch_due_tasks`     | `backend/djadmin/scheduler/tasks.py`  | Celery Beat 周期触发（`CELERY_BEAT_SCHEDULE`，每 60 秒）                            | 调度扫描器：检查`ScheduledTask` 是否到期，并投递执行任务   |
| `scheduler.execute_scheduled_task` | `backend/djadmin/scheduler/tasks.py`  | 由`dispatch_due_tasks` 投递；或 `/sys/scheduler/tasks/{id}/run-now` 手动投递      | 执行具体定时任务（例如`collect_all_hosts_info`）           |
| `automation.execute_ansible_job`   | `backend/djadmin/automation/tasks.py` | 兼容保留（当前默认执行路径不依赖该任务） | 自动化中心 Ansible 作业执行任务（保留 Celery 入口，默认改为本地后台线程执行） |

补充说明：

- `scheduler.dispatch_due_tasks` 是唯一依赖 **Beat** 的任务。
- `scheduler.execute_scheduled_task` 与 `automation.execute_ansible_job` 只依赖 **Worker**，不依赖 Beat。
- `python manage.py runscheduler` 会同时启动 Worker + Beat，覆盖上述全部任务场景。

---

### 6. 系统参数模块（sys_config）

**数据模型**

- `SysConfig`：系统参数（id, name, key[唯一], value, default_value, value_type[string/int/bool/json], is_readonly, description, create_time, update_time, remark）

| 方法  | 路径                               | 功能                         |
| ----- | ---------------------------------- | ---------------------------- |
| GET   | `/sys/config/`                   | 参数列表（搜索/分页）        |
| GET   | `/sys/config/{id}/`              | 参数详情                     |
| POST  | `/sys/config/`                   | 新增参数                     |
| PATCH | `/sys/config/{id}/`              | 编辑参数（只读参数不可修改） |
| POST  | `/sys/config/{id}/reset-default` | 重置为默认值                 |
| GET   | `/sys/config/by-key`             | 按 key 查询参数值            |
| POST  | `/sys/config/update-by-key`      | 按 key 更新参数值            |

---

## 通用响应格式

```json
{
  "code": 200,
  "msg": "success",
  "data": { ... }
}
```

**分页格式**：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [...],
    "count": 100,
    "pageNumber": 1,
    "pageSize": 10,
    "totalPages": 10,
    "next": null,
    "previous": null
  }
}
```

**错误码**：

- `200` 成功
- `300` 账号/密码错误
- `301` Token 失效（前端跳转登录页）
- `400` 请求参数错误
- `403` 无权限
- `404` 资源不存在

---

## 权限系统

权限标识格式：`模块:资源:操作`，例如：

- `assets:credentials:view` - 查看凭证
- `system:users:delete` - 删除用户
- `system:menus:create` - 创建菜单

权限校验通过 `CustomMenuPermission` 中间件，权限码嵌入 JWT token 的 `perms` 字段。

---

## 关键公共工具（djadmin/utils.py）

| 函数/类                                 | 说明                                               |
| --------------------------------------- | -------------------------------------------------- |
| `Response_200(data, msg)`             | 成功响应                                           |
| `Response_error(error)`               | 预定义错误响应                                     |
| `Response_error_str(msg, code, data)` | 自定义错误响应                                     |
| `Response_djerror(djerror, data)`     | Django 异常响应                                    |
| `CustomPagination`                    | 分页器（默认10条/页，最大30，参数名`page_size`） |

---

## Web SSH（当前功能盘点）

### 技术栈

- 后端：`channels` + `daphne` + `paramiko`（终端链路）+ `asyncssh`（下载/上传主链路）
- 前端：`@xterm/xterm` + `@xterm/addon-fit` + Ant Design Vue
- 通道层：`InMemoryChannelLayer`（单机开发）

### 当前已实现功能

#### 1) 终端与会话能力

- 主机页面支持建立 WebSSH 终端连接（JWT 校验 + 权限校验）。
- 终端支持输入、输出、窗口 resize、重连、关闭、全屏切换。
- 连接成功后返回会话 ID、主机信息、推导的 home_dir。
- 支持下载当前会话日志。

#### 2) 在线会话监控

- 页面展示当前主机在线会话人数。
- 支持查看在线会话列表（会话 ID、用户名、开始时间）。
- 凭证/主机连接关键字段变化时，会强制关闭相关在线会话。

#### 3) 左侧文件管理（SFTP）

- 支持目录浏览、进入目录、回到上级、路径输入跳转、关键字过滤。
- 支持文件上传、文件下载、重命名、删除、新建目录、新建空文件（后端能力）。
- 文件操作要求当前主机存在在线 WebSSH 会话，否则拒绝并提示离线。
- 文件管理类接口（list/rename/delete/create-dir/create-file）当前仍基于 Paramiko SFTP。

#### 4) 下载能力增强

- 后端下载链路：AsyncSSH `get` 到本地临时文件，再以 StreamingHttpResponse 返回。
- 支持 HTTP Range（206）与 Accept-Ranges。
- 前端显示下载进度、已用时/总耗时，支持取消。
- 同时仅允许一个下载任务，下载中再次触发会提示稍后重试。
- 目录下载已关闭（前后端均拦截并提示）。

#### 5) 上传能力增强

- 后端上传链路：先接收请求文件到本地临时文件，再 AsyncSSH `put` 到远端临时文件并 rename。
- 前端显示上传进度、已用时/总耗时，支持取消。
- 同时仅允许一个上传任务，上传中再次触发会提示稍后重试。
- 已移除上传传输列表与排队机制，不再提供队列可视化。

### WebSocket 路径与事件

- WS：`/ws/assets/hosts/{id}/webssh/?token=...`
- 事件类型：
  - `connected`：连接成功
  - `output`：终端输出
  - `error`：错误信息
  - `closed`：会话关闭

### 生产部署建议（当前项目约束）

- 建议将 WebSSH/文件传输流量与普通业务 API 进程隔离部署。
- 多实例生产场景建议升级 `channels-redis`，并独立扩展 WebSSH 相关 worker。
- 当前文件传输接口以 `/assets/hosts/{id}/files/*` 为主，文档与实现需保持同步。

---

## 项目启动步骤（开发环境）

以下步骤基于当前仓库代码，可用于本地或 Linux 远程开发环境。

### 1. 后端环境准备

在 `backend/djadmin` 目录执行：

```bash
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

Windows PowerShell 激活方式：

```powershell
(Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned) ; (& .\.venv\Scripts\Activate.ps1)
```

### 2. 数据库迁移与健康检查

在 `backend/djadmin` 目录执行：

```bash
python manage.py migrate
python manage.py check
```

### 3. 启动 Django 服务

保持命令不变，直接执行：

```bash
python manage.py runserver
```

说明：

- 默认监听地址和端口由 `settings.py` 中的 `SERVER_HOST`、`SERVER_PORT` 控制。
- 也可通过环境变量覆盖：`DJADMIN_SERVER_HOST`、`DJADMIN_SERVER_PORT`。

### 4. 启动前端服务

在 `fronted` 目录执行：

```bash
npm install
npm run dev
```

### 5. 启动调度器（Celery 独立进程）

必须使用独立终端启动，不与 Django Web 进程复用。

在 `backend/djadmin` 目录执行：

```bash
python manage.py runscheduler
```

该命令会同时拉起 Celery Worker 与 Beat，适合日常使用。

如需分开调试，可分别执行：

```bash
python manage.py runceleryworker --loglevel=info --concurrency=2
python manage.py runcelerybeat --loglevel=info
```

兼容脚本入口：

- Windows：`backend/start_scheduler.ps1`
- Linux/Mac：`backend/start_scheduler.sh`

### 6. 启动传输服务（独立传输数据面）

用于 WebSSH 上传/下载加速，建议与 Django 主服务分开进程运行。

在 `backend/djadmin` 目录执行：

```bash
python manage.py runtransfer --host 0.0.0.0 --port 9101
```

说明：`runtransfer` 内部使用 Daphne 启动，并自动设置 `DJANGO_SETTINGS_MODULE=djadmin.transfer_settings`。

前端默认读取：

- `VITE_TRANSFER_BASE_URL`（默认 `http://{当前主机}:9101`）

后端/transfer 服务相关环境变量：

- `TRANSFER_SERVICE_BASE_URL`（默认 `http://127.0.0.1:9101`）
- `TRANSFER_TICKET_EXPIRE_SECONDS`（默认 `7200` 秒）
- `TRANSFER_SSH_POOL_MAX_PER_KEY`（默认 `4`，单主机连接池上限）
- `TRANSFER_SSH_POOL_IDLE_SECONDS`（默认 `120`，连接池空闲回收秒数）
- `TRANSFER_STREAM_FIRST_CHUNK_BYTES`（默认 `262144`，首包块大小）
- `TRANSFER_STREAM_CHUNK_BYTES`（默认 `8388608`，流式下载块大小）
- `TRANSFER_STREAM_PROGRESS_LOG_SECONDS`（默认 `5`，下载进度日志间隔秒）
- `TRANSFER_SFTP_WINDOW_SIZE`（默认 `8388608`）
- `TRANSFER_SFTP_MAX_PACKET_SIZE`（默认 `262144`）
- `TRANSFER_SFTP_PREFETCH_REQUESTS`（默认 `32`）
- `DJANGO_SETTINGS_MODULE=djadmin.transfer_settings`（仅 Daphne 启动 transfer 时必需）

无效配置说明（当前代码未读取）：

- `TRANSFER_SFTP_PREFETCH_REQUESTS_RANGE`（不生效，可移除）

### 7. 验收清单

1. 后端接口可访问（登录接口返回正常）。
2. 前端页面可打开并正常登录。
3. 菜单加载正常，角色权限可生效。
4. 自动化任务可进入并提交任务。
5. 调度器运行后，任务最近执行时间会更新。
6. 自动化任务的 Playbook / Task / Workflow 节点执行默认走本地后台线程（非 Celery），请求快速返回，不阻塞 Web 线程。
7. WebSSH 下载使用 `/assets/hosts/{id}/files/download/?path=...`，支持 Range 与流式返回。
8. WebSSH 上传使用 `/assets/hosts/{id}/files/upload/chunk/`，上传中再次上传会提示稍后重试（无传输列表/无排队）。
9. 自动化“任务运行记录”中，`pending` 状态任务不展示“下载日志”按钮。

### 8. 端口联动注意事项

如果修改后端端口，需同步确认以下配置：

1. 后端监听端口：`backend/djadmin/djadmin/settings.py`。
2. 前端 HTTP 基础地址：`fronted/src/util/request.js`。
3. WebSSH 地址已改为复用前端统一基础地址配置，无需单独硬编码端口。

### 9. 后端端口修改方法

后端端口支持两种修改方式。

方式 A：修改配置文件（长期固定）

在 `backend/djadmin/djadmin/settings.py` 修改：

```python
SERVER_HOST = os.getenv('DJADMIN_SERVER_HOST', '0.0.0.0')
SERVER_PORT = int(os.getenv('DJADMIN_SERVER_PORT', '8000'))
```

例如改为 9000：

```python
SERVER_PORT = int(os.getenv('DJADMIN_SERVER_PORT', '9000'))
```

然后使用原命令启动：

```bash
python manage.py runserver
```

方式 B：环境变量覆盖（不改代码）

Linux/Mac：

```bash
export DJADMIN_SERVER_HOST=0.0.0.0
export DJADMIN_SERVER_PORT=9000
python manage.py runserver
```

Windows PowerShell：

```powershell
$env:DJADMIN_SERVER_HOST = "0.0.0.0"
$env:DJADMIN_SERVER_PORT = "9000"
python manage.py runserver
```

修改后端端口后，前端联调请同时确认：

1. `fronted/src/util/request.js` 中的后端基础地址端口与后端一致。
2. WebSSH 连接地址会复用基础地址配置，无需额外修改端口常量。
