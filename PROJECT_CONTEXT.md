# 项目上下文文档 - djadmin 运维管理平台

## 项目概述

面向 IT 运维团队的管理平台，提供主机资产管理、凭证管理、定时任务调度、用户权限管理等功能。

- **后端**: Django 5.1 + Django REST Framework + MySQL
- **前端**: Vue 3 + Vite + Ant Design Vue
- **调度**: APScheduler（独立进程）
- **认证**: JWT（rest_framework_jwt，1天有效期）
- **远程连接（规划）**: Web SSH（基于 Channels + Paramiko + xterm）

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

| 方法 | 路径 | 功能 |
|------|------|------|
| POST | `/sys/login` | 登录（返回 token + 菜单树 + 角色权限码） |
| POST | `/sys/usercenter/updateUserInfo` | 更新个人信息（邮箱、手机） |
| POST | `/sys/usercenter/updateUserPassword` | 修改密码 |
| POST | `/sys/changeAvatar` | 上传头像文件 |
| GET | `/sys/users/` | 用户列表（搜索/排序/分页，含角色信息） |
| GET | `/sys/users/{id}/` | 用户详情 |
| POST | `/sys/users/` | 新增用户（默认密码123456） |
| PATCH | `/sys/users/{id}/` | 编辑用户 |
| GET | `/sys/users/checkUserName` | 检查用户名是否已存在 |
| DELETE | `/sys/users/userBatchDelete` | 批量删除用户（级联删除用户角色关联） |
| POST | `/sys/users/resetUserPwd` | 重置密码为 123456 |
| POST | `/sys/users/assginUserRoles` | 分配用户角色（全量替换） |
| POST | `/sys/users/changeUserStatus` | 启用/禁用用户 |
| GET | `/sys/users/getUserRolesById` | 查询指定用户的角色列表 |
| GET | `/sys/users/current` | 获取当前登录用户信息（含时区） |

---

### 2. 角色模块（role）

**数据模型**
- `SysRole`：角色（id, name, code, create_time, update_time, remark）

**API 接口**

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/sys/roles/` | 角色列表（搜索/排序/分页） |
| GET | `/sys/roles/{id}/` | 角色详情 |
| POST | `/sys/roles/` | 新增角色 |
| PATCH | `/sys/roles/{id}/` | 编辑角色 |
| DELETE | `/sys/roles/batch-delete` | 批量删除角色（级联删除用户-角色、角色-菜单） |
| GET | `/sys/roles/getCurrentUserRoleList` | 获取当前登录用户的角色列表 |

---

### 3. 菜单模块（menu）

**数据模型**
- `SysMenu`：菜单（id, name, icon, parent_id, order_num, path, component, menu_type[M/C/F], perms, location, create_time, update_time, remark）
- `SysRoleMenu`：角色-菜单关联（role_id, menu_id，唯一约束）

**API 接口**

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/sys/menus/` | 菜单列表 |
| GET | `/sys/menus/{id}/` | 菜单详情 |
| POST | `/sys/menus/` | 新增菜单 |
| PATCH | `/sys/menus/{id}/` | 编辑菜单 |
| GET | `/sys/menus/getMenuTree` | 获取完整菜单树（按 order_num 排序） |
| GET | `/sys/menus/getMenuListByRoleId` | 查询角色拥有的菜单 ID 列表 |
| POST | `/sys/menus/grantMenu` | 批量分配菜单给角色（全量替换） |
| DELETE | `/sys/menus/deleteMenuById` | 删除菜单（级联删除角色-菜单关联） |

---

### 4. 资产模块（assets）

#### 4.1 凭证管理（Credential）

**数据模型**
- `Credential`：凭证（id, name, username, password, private_key, port[默认22], auth_type[1=密码/2=SSH Key], create_time, update_time, remark）

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/assets/credentials/` | 凭证列表（搜索/排序/分页） |
| GET | `/assets/credentials/{id}/` | 凭证详情 |
| POST | `/assets/credentials/` | 新增凭证 |
| PATCH | `/assets/credentials/{id}/` | 编辑凭证 |
| DELETE | `/assets/credentials/batch-delete` | 批量删除凭证 |
| POST | `/assets/credentials/batch-create` | 批量导入凭证（CSV 文件） |

#### 4.2 应用管理（Application）

**数据模型**
- `Application`：应用（id, name[唯一], version, create_time, update_time, remark）

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/assets/applications/` | 应用列表（搜索/排序/分页） |
| GET | `/assets/applications/{id}/` | 应用详情 |
| POST | `/assets/applications/` | 新增应用 |
| PATCH | `/assets/applications/{id}/` | 编辑应用 |
| DELETE | `/assets/applications/batch-delete` | 批量删除应用 |

#### 4.3 主机分组管理（HostGroup）

**数据模型**
- `HostGroup`：主机分组（id, name[唯一], parent[自关联，支持多级嵌套], create_time, update_time, remark）

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/assets/host-groups/` | 分组列表（搜索/排序/分页） |
| GET | `/assets/host-groups/{id}/` | 分组详情 |
| POST | `/assets/host-groups/` | 新增分组 |
| PATCH | `/assets/host-groups/{id}/` | 编辑分组 |
| DELETE | `/assets/host-groups/{id}/` | 删除分组 |
| DELETE | `/assets/host-groups/batch-delete` | 批量删除分组 |
| GET | `/assets/host-groups/tree` | 获取树形结构（含嵌套子分组） |

#### 4.4 主机管理（Host）

**数据模型**
- `Host`：主机（id, instance_name, name, ip, instance_id, cloud_account[FK], group[FK], status, is_deleted_in_cloud, port[默认22], collect_status[unknown/success/failed], collect_message, collect_time, create_time, update_time, remark）
- `HostCredential`：主机-凭证关联（host, credential, is_default）
- `HostHardware`：硬件信息（host[1:1], cpu_cores, cpu_model, memory_gb, disk_total_gb, architecture, collected_at）
- `HostSystem`：系统信息（host[1:1], os_type, os_version, kernel_version, hostname, agent_version, collected_at）
- `HostDisk`：磁盘分区（host[FK], device, mount_point, size_gb, used_gb, filesystem）

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/assets/hosts/` | 主机列表（支持 group_id 过滤、递归子分组、collect_status 过滤、搜索/排序/分页） |
| GET | `/assets/hosts/{id}/` | 主机详情（含硬件、系统、磁盘信息） |
| POST | `/assets/hosts/` | 新增主机 |
| PATCH | `/assets/hosts/{id}/` | 编辑主机 |
| DELETE | `/assets/hosts/{id}/` | 删除主机 |
| DELETE | `/assets/hosts/batch-delete` | 批量删除主机 |
| POST | `/assets/hosts/{id}/collect-info` | 采集单台主机信息（SSH 连接，更新硬件/系统/磁盘） |
| POST | `/assets/hosts/batch-collect-info` | 批量采集指定主机信息 |
| POST | `/assets/hosts/collect-all` | 采集所有主机信息 |

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

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/sys/scheduler/tasks/` | 任务列表（搜索/排序/分页） |
| GET | `/sys/scheduler/tasks/{id}/` | 任务详情 |
| POST | `/sys/scheduler/tasks/` | 新增任务 |
| PATCH | `/sys/scheduler/tasks/{id}/` | 编辑任务 |
| DELETE | `/sys/scheduler/tasks/{id}/` | 删除任务 |
| POST | `/sys/scheduler/tasks/{id}/toggle-enabled` | 切换启用/禁用状态 |
| POST | `/sys/scheduler/tasks/{id}/enable` | 启用任务 |
| POST | `/sys/scheduler/tasks/{id}/disable` | 禁用任务 |
| POST | `/sys/scheduler/tasks/{id}/run-now` | 立即执行（有并发保护，is_running 防重入） |
| POST | `/sys/scheduler/tasks/start-scheduler` | 启动 APScheduler 进程 |
| POST | `/sys/scheduler/tasks/stop-scheduler` | 停止 APScheduler 进程 |

**主机批量采集任务状态语义**
- `collect_all_hosts_info` 对单台主机失败采取“记录并继续”策略，不会因为单台失败中断整个批次。
- 因此定时任务总体状态通常按“任务函数是否抛异常”判定：单台失败不一定导致任务总体失败。

---

### 6. 系统参数模块（sys_config）

**数据模型**
- `SysConfig`：系统参数（id, name, key[唯一], value, default_value, value_type[string/int/bool/json], is_readonly, description, create_time, update_time, remark）

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/sys/config/` | 参数列表（搜索/分页） |
| GET | `/sys/config/{id}/` | 参数详情 |
| POST | `/sys/config/` | 新增参数 |
| PATCH | `/sys/config/{id}/` | 编辑参数（只读参数不可修改） |
| POST | `/sys/config/{id}/reset-default` | 重置为默认值 |
| GET | `/sys/config/by-key` | 按 key 查询参数值 |
| POST | `/sys/config/update-by-key` | 按 key 更新参数值 |

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

| 函数/类 | 说明 |
|---------|------|
| `Response_200(data, msg)` | 成功响应 |
| `Response_error(error)` | 预定义错误响应 |
| `Response_error_str(msg, code, data)` | 自定义错误响应 |
| `Response_djerror(djerror, data)` | Django 异常响应 |
| `CustomPagination` | 分页器（默认10条/页，最大30，参数名 `page_size`） |

---

## Web SSH（单机最小可用技术方案）

### 目标
- 在主机列表页面提供浏览器内 SSH 终端能力，减少本地 SSH 工具切换。

### 最小技术栈（去除可选项）
- 后端：`paramiko` + `channels` + `daphne`
- 前端：`@xterm/xterm` + `@xterm/addon-fit`
- 通道层：`InMemoryChannelLayer`（单机开发）

### 各组件作用
- `paramiko`：建立 SSH 连接并读写远程 shell。
- `channels`：为 Django 提供 WebSocket 双向通信能力。
- `daphne`：ASGI 服务承载 WebSocket 连接。
- `@xterm/xterm`：浏览器终端渲染与键盘输入处理。
- `@xterm/addon-fit`：终端尺寸自适应弹窗/容器。
- `InMemoryChannelLayer`：单机内消息分发（不依赖 Redis）。

### 单机开发边界
- 单机开发阶段可不引入 Redis。
- 多实例生产部署时再升级到 `channels-redis`。

### 依赖安装命令（当前推荐）
- 后端：`pip install paramiko channels daphne`
- 前端：`npm install @xterm/xterm @xterm/addon-fit`

> 说明：旧包名 `xterm`、`xterm-addon-fit` 已弃用，建议统一使用 `@xterm/*` 命名空间。

### 功能点（规划）

#### 前端功能点（主机列表）
- 在主机列表操作列新增「Web SSH」按钮（建议图标：`terminal`）。
- 点击按钮弹出终端弹窗：显示主机名、IP、连接状态。
- 弹窗内嵌 `xterm`，支持键盘输入、回车执行、终端输出滚动。
- 终端窗口支持自适应（`fit`），弹窗大小变化时自动重排。
- 连接失败时给出明确错误提示（认证失败/连接超时/网络不可达）。
- 关闭弹窗时主动断开会话，避免僵尸连接。

#### 后端功能点
- 基于主机默认凭证建立 SSH 会话，不允许前端直接传入密码/私钥。
- WebSocket 握手校验当前用户是否有 Web SSH 权限。
- 会话生命周期管理：连接创建、心跳存活、断开清理。
- 双向数据转发：浏览器输入 -> SSH stdin；SSH stdout/stderr -> 浏览器。
- 异常兜底：任一异常都关闭 SSH 连接并回传错误事件。

#### 权限与安全
- 新增权限码：`assets:hosts:webssh`。
- 仅允许连接资产库中已登记的主机（按 `host_id` 建连）。
- 连接使用超时与空闲超时（建议：连接超时 10-20s，空闲 15-30min）。
- 日志中禁止打印明文密码、私钥等敏感信息。

#### 接口与事件建议（最小版）
- HTTP（可选）：`POST /assets/hosts/{id}/webssh-token`（签发短时连接令牌）。
- WS：`/ws/assets/hosts/{id}/webssh/?token=...`
- 事件类型建议：
  - `connected`：连接成功
  - `output`：终端输出
  - `error`：错误信息
  - `closed`：会话关闭

#### 验收标准（MVP）
- 用户在主机列表可打开终端并执行基础命令（如 `hostname`、`uname -a`）。
- 无权限用户点击时返回统一格式无权限提示（403）。
- 主机不可达/认证失败时，前端能看到明确失败原因。
- 关闭终端弹窗后，后端 SSH 会话被释放（无残留连接）。

#### 暂不包含（后续增强）
- 多实例水平扩展（Redis Channel Layer）。
- 会话录屏/命令审计回放。
- 文件上传下载（SFTP）。
- 终端多标签与会话共享。

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

### 5. 启动调度器（独立进程）

必须使用独立终端启动，不与 Django Web 进程复用。

在 `backend/djadmin` 目录执行：

```bash
python manage.py runapscheduler
```

或使用仓库脚本：
- Windows：`backend/start_scheduler.ps1`
- Linux/Mac：`backend/start_scheduler.sh`

### 6. 验收清单

1. 后端接口可访问（登录接口返回正常）。
2. 前端页面可打开并正常登录。
3. 菜单加载正常，角色权限可生效。
4. 自动化执行中心可进入并提交任务。
5. 调度器运行后，任务最近执行时间会更新。

### 7. 端口联动注意事项

如果修改后端端口，需同步确认以下配置：

1. 后端监听端口：`backend/djadmin/djadmin/settings.py`。
2. 前端 HTTP 基础地址：`fronted/src/util/request.js`。
3. WebSSH 地址已改为复用前端统一基础地址配置，无需单独硬编码端口。

### 8. 后端端口修改方法

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
