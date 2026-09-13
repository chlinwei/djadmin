# djadmin

运维管理平台：主机资产、服务树与应用目录、自动化编排（Ansible）、巡检、监控告警、日志采集、基线扫描、定时任务与权限审计。

## 技术栈

- **后端**：Go（`autoadmin/`），Gin + MySQL，进程内含 API 与定时调度
- **执行代理**：Go（`dj_agent/`），gRPC 双向流；必须通过 Makefile 构建（强制 `CGO_ENABLED=0`）
- **前端**：Vue 3 + Vite + Ant Design Vue（`fronted/`）
- `backend/`（Django）**已废弃**，仅存归档，勿作为实现或文档依据（见 [AGENTS.md](AGENTS.md)）

## 文档索引

**新增文档前必读：[docs/architecture/CONVENTIONS.md](docs/architecture/CONVENTIONS.md) 第三节"文档组织约定"——先查本索引，主题已存在则并入，不另起新文件。**

| 分类 | 位置 | 内容 |
|---|---|---|
| 概览 | [docs/overview](docs/overview) | [项目全局上下文](docs/overview/PROJECT_CONTEXT.md)（模块地图 + 各域文档入口） |
| 架构（最终逻辑） | [docs/architecture](docs/architecture) | 各功能域最终设计；[开发与设计约定](docs/architecture/CONVENTIONS.md)（删除 API 规范、前端表格规范、文档组织） |
| 计划/待办 | [docs/plans](docs/plans) | 未完成改造清单（缺失接口补齐、巡检规模化） |
| 运维 | [docs/ops](docs/ops) | 告警媒介配置指南等操作手册 |
| 历史归档 | [docs/archive](docs/archive) | 旧方案与 Django 时代文档（文件头均标注勿作参考） |

### 常用入口

- 项目全局上下文：[docs/overview/PROJECT_CONTEXT.md](docs/overview/PROJECT_CONTEXT.md)
- 开发与设计约定（删除 API / 前端表格 / 文档组织）：[docs/architecture/CONVENTIONS.md](docs/architecture/CONVENTIONS.md)
- API 响应格式：[.github/API_RULES.md](.github/API_RULES.md)
- 巡检架构：[docs/architecture/INSPECTION_ARCHITECTURE.md](docs/architecture/INSPECTION_ARCHITECTURE.md)
- 日志采集架构：[docs/architecture/LOG_COLLECTION_ARCHITECTURE.md](docs/architecture/LOG_COLLECTION_ARCHITECTURE.md)
- Agent 架构：[docs/architecture/DJ_AGENT_ARCHITECTURE.md](docs/architecture/DJ_AGENT_ARCHITECTURE.md)
- 前端说明：[fronted/README.md](fronted/README.md)
- Agent 说明：[dj_agent/README.md](dj_agent/README.md)

## 快速启动

### 后端（autoadmin）

```bash
cd autoadmin
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
go run ./cmd/autoadmin
```

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
