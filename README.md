# djadmin

统一文档入口（建议从这里开始）：

## 核心文档

- 项目全局上下文：[`PROJECT_CONTEXT.md`](./PROJECT_CONTEXT.md)
- 调度与 Celery 说明：[`SCHEDULER_README.md`](./SCHEDULER_README.md)
- 前端开发说明：[`fronted/README.md`](./fronted/README.md)
- API 规范：[`.github/API_RULES.md`](./.github/API_RULES.md)
- API 合规报告：[`.github/API_COMPLIANCE_REPORT.md`](./.github/API_COMPLIANCE_REPORT.md)
- API 合规摘要：[`.github/API_COMPLIANCE_SUMMARY.md`](./.github/API_COMPLIANCE_SUMMARY.md)
- Agent 作业控制面 API：[`backend/djadmin/assets/AGENT_JOB_API.md`](./backend/djadmin/assets/AGENT_JOB_API.md)

## 快速启动（开发环境）

### 后端

```bash
cd backend/djadmin
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python manage.py runserver 0.0.0.0:8000
```

### 前端

```bash
cd fronted
npm install
npm run dev
```

### 调度（Celery Worker + Beat）

```bash
cd backend/djadmin
python manage.py runscheduler
```


### 传输服务（transfer）

```bash
cd backend/djadmin
python manage.py runtransfer --host 0.0.0.0 --port 9101
```

说明：
- transfer 是 WebSSH 文件上传/下载的数据面服务，建议独立进程运行。
- 默认端口 `9101`，可按需调整。
- 后端签发票据后，前端会调用 transfer 服务的下载/上传接口执行实际传输。
- `runtransfer` 已改为内部直接使用 Daphne 启动，并自动设置 `DJANGO_SETTINGS_MODULE=djadmin.transfer_settings`。

#### 手动使用 Daphne 启动（与 runtransfer 等价）

`runtransfer` 内部本质就是这条命令；手动启动时要确保使用 `transfer_settings`：

```bash
cd backend/djadmin
export DJANGO_SETTINGS_MODULE=djadmin.transfer_settings
daphne -b 0.0.0.0 -p 9101 djadmin.asgi:application
```

如果不设置 `DJANGO_SETTINGS_MODULE=djadmin.transfer_settings`，会走默认 `djadmin.settings`，不是纯 transfer 路由。

#### 相关环境变量

必需（仅手动 Daphne 方式）：

- `DJANGO_SETTINGS_MODULE=djadmin.transfer_settings`

常用（后端控制面签发 ticket 时使用）：

- `TRANSFER_SERVICE_BASE_URL`（默认 `http://127.0.0.1:9101`）
- `TRANSFER_TICKET_SECRET`（默认使用 Django `SECRET_KEY`）
- `TRANSFER_TICKET_EXPIRE_SECONDS`（默认 `7200`）

性能相关（transfer 下载链路）：

- `TRANSFER_SSH_POOL_MAX_PER_KEY`（默认 `4`）
- `TRANSFER_SSH_POOL_IDLE_SECONDS`（默认 `120`）
- `TRANSFER_STREAM_FIRST_CHUNK_BYTES`（默认 `262144`）
- `TRANSFER_STREAM_CHUNK_BYTES`（默认 `8388608`）
- `TRANSFER_STREAM_PROGRESS_LOG_SECONDS`（默认 `5`）
- `TRANSFER_SFTP_WINDOW_SIZE`（默认 `8388608`）
- `TRANSFER_SFTP_MAX_PACKET_SIZE`（默认 `262144`）
- `TRANSFER_SFTP_PREFETCH_REQUESTS`（默认 `32`）

说明：
- `TRANSFER_SFTP_PREFETCH_REQUESTS_RANGE` 当前代码未读取，配置该变量不会生效。

## 常见问题入口

- 定时任务 pending / 不执行：先看 [`SCHEDULER_README.md`](./SCHEDULER_README.md) 的“排查命令”章节。
- WebSSH 与文件传输行为说明：看 [`PROJECT_CONTEXT.md`](./PROJECT_CONTEXT.md) 的 WebSSH 章节。
- transfer 无法上传/下载：先确认 `runtransfer` 进程是否启动、端口是否可达（默认 9101）。
