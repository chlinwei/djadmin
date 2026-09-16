# djadmin

运维管理平台：主机资产、服务树与应用目录、自动化编排（Ansible）、巡检、监控告警、日志采集、基线扫描、定时任务与权限审计。

## 技术栈

- **后端**：Go（`autoadmin/`），Gin + MySQL（默认构建）；数据访问层同时支持 PostgreSQL（`-tags postgres`，见 [SQL 设计](docs/architecture/SQL_DESIGN.md)），进程内含 API 与定时调度
- **执行代理**：Go（`dj_agent/`），gRPC 双向流；必须通过 Makefile 构建（强制 `CGO_ENABLED=0`）
- **前端**：Vue 3 + Vite + Ant Design Vue（`fronted/`）
- `backend/`（Django）**已废弃**，仅存归档，勿作为实现或文档依据（见 [AGENTS.md](AGENTS.md)）

## 文档索引

**新增文档前必读：[docs/architecture/CONVENTIONS.md](docs/architecture/CONVENTIONS.md) 第三节"文档组织约定"——先查本索引，主题已存在则并入，不另起新文件。**

| 分类 | 位置 | 内容 |
|---|---|---|
| 概览 | [docs/overview](docs/overview) | [项目全局上下文](docs/overview/PROJECT_CONTEXT.md)（模块地图 + 各域文档入口） |
| 架构（最终逻辑） | [docs/architecture](docs/architecture) | 各功能域最终设计；[开发与设计约定](docs/architecture/CONVENTIONS.md)（删除 API 规范、前端表格规范、文档组织） |
| 计划/待办 | [docs/plans](docs/plans) | 未完成改造清单（缺失接口补齐、巡检规模化、**双数据库与 sqlc 迁移**） |
| 运维 | [docs/ops](docs/ops) | 告警媒介配置指南等操作手册 |
| 历史归档 | [docs/archive](docs/archive) | 旧方案与 Django 时代文档（文件头均标注勿作参考） |

### 常用入口

- 项目全局上下文：[docs/overview/PROJECT_CONTEXT.md](docs/overview/PROJECT_CONTEXT.md)
- 开发与设计约定（删除 API / 前端表格 / 文档组织）：[docs/architecture/CONVENTIONS.md](docs/architecture/CONVENTIONS.md)
- **SQL 设计与方言约定（选型原则 / 字段映射 / MySQL+PostgreSQL 可移植）：[docs/architecture/SQL_DESIGN.md](docs/architecture/SQL_DESIGN.md)**
- **进行中的改造计划（双数据库支持 + 内联 SQL 迁移到 sqlc，含待办与已知陷阱）：[docs/plans/SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md](docs/plans/SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md)**
- API 响应格式：[.github/API_RULES.md](.github/API_RULES.md)
- 巡检架构：[docs/architecture/INSPECTION_ARCHITECTURE.md](docs/architecture/INSPECTION_ARCHITECTURE.md)
- 日志采集架构：[docs/architecture/LOG_COLLECTION_ARCHITECTURE.md](docs/architecture/LOG_COLLECTION_ARCHITECTURE.md)
- Agent 架构：[docs/architecture/DJ_AGENT_ARCHITECTURE.md](docs/architecture/DJ_AGENT_ARCHITECTURE.md)
- 前端说明：[fronted/README.md](fronted/README.md)
- Agent 说明：[dj_agent/README.md](dj_agent/README.md)

## 快速启动

### 后端（autoadmin）

构建、测试与运行统一走 Makefile（它导出 `CGO_ENABLED=0`，命令包里还有一道故意在开启 CGO 时失败的守卫）：

```bash
cd autoadmin

# 默认构建：MySQL
make test && make vet && make build     # 产物 bin/autoadmin

# PostgreSQL 变体：查询走 PG 产物、连接走 pgx（需要 POSTGRES_DSN）
make build-postgres                     # 产物 bin/autoadmin-postgres
make vet-postgres && go test -tags postgres ./...

# 改 SQL 时的固定顺序（漏一步会被守卫测试挡住）
make derive      # db/queries/mysql → db/queries/postgres（PG 侧禁止手改）
make generate SQLC=~/go/bin/sqlc        # 两侧 sqlc 产物，须用 v1.30.0
make facade      # 重建方言门面

# 运行：一个二进制四个角色
./bin/autoadmin api        # HTTP :9000 + agent gRPC :9001
./bin/autoadmin scheduler  # 定时任务
./bin/autoadmin worker     # 消息消费
./bin/autoadmin migrate    # 执行 db/migrations/<dialect> 的 up 迁移
```

环境变量：默认构建读 `MYSQL_DSN`，`-tags postgres` 读 `POSTGRES_DSN`（须带 `TimeZone=UTC`）；`MIGRATION_DATABASE_URL` / `MIGRATION_SOURCE_URL` 供 `migrate` 用。配置不自动读 dotenv，需自行 `set -a; . ./config.env; set +a`。两个 tag 都要在 CI 里构建与测试——PG 侧的适配是手写的，只有编译 `postgres` tag 才会暴露两侧产物的新分歧。

更细的说明：[SQL 目录结构与改动顺序](docs/architecture/SQL_DESIGN.md) §4.6、[构建与验证命令](docs/architecture/SQL_DESIGN.md) §5.1、[autoadmin/README.md](autoadmin/README.md)。

### 前端

```bash
cd fronted
npm install
npm run dev      # 开发（Vite）
npm run build    # 构建
npm run test:run # 单测（vitest）
```

### Agent（dj_agent）

```bash
cd dj_agent
make build
```

## 维护原则

- 根目录只保留最重要的入口文件。
- 详细设计文档放入 [docs](docs) 目录下，按 [文档组织约定](docs/architecture/CONVENTIONS.md) 归类。
- 模块专属说明放在对应代码目录附近。
- 生成型报告和临时日志不要长期保留到仓库根目录。
