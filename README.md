# djadmin

这是项目文档的统一入口，按职责分成不同目录，避免根目录堆积历史和临时资料。

## 文档分类

- 项目概览与架构：[docs/overview](docs/overview)
- 运行维护与运维：[docs/ops](docs/ops)
- 架构设计与方案：[docs/architecture](docs/architecture)
- 历史归档：[docs/archive](docs/archive)
- 功能模块文档：[backend/djadmin](backend/djadmin), [fronted](fronted), [dj_agent](dj_agent)
- 开发规范与 AI 协作：[.github](.github)

## 常用入口

- 项目全局上下文：[docs/overview/PROJECT_CONTEXT.md](docs/overview/PROJECT_CONTEXT.md)
- 调度说明：[docs/ops/SCHEDULER_README.md](docs/ops/SCHEDULER_README.md)
- Go Agent 架构：[docs/architecture/DJ_AGENT_ARCHITECTURE.md](docs/architecture/DJ_AGENT_ARCHITECTURE.md)
- 日志采集架构：[docs/architecture/LOG_COLLECTION_ARCHITECTURE.md](docs/architecture/LOG_COLLECTION_ARCHITECTURE.md)
- API 规范：[.github/API_RULES.md](.github/API_RULES.md)
- 前端说明：[fronted/README.md](fronted/README.md)
- Agent 说明：[dj_agent/README.md](dj_agent/README.md)
- 自动化模型关系：[backend/djadmin/automation/MODEL_RELATIONSHIP.md](backend/djadmin/automation/MODEL_RELATIONSHIP.md)

## 快速启动

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

### 调度器

```bash
cd backend/djadmin
python manage.py runscheduler
```

### dj-agent

必须通过 Makefile 构建（强制 `CGO_ENABLED=0`，禁止裸跑 `go build`）：

```bash
cd dj_agent
make build
```

## 维护原则

- 根目录只保留最重要的入口文件。
- 详细设计文档放入 [docs](docs) 目录下，按主题归类。
- 模块专属说明放在对应代码目录附近。
- 生成型报告和临时日志不要长期保留到仓库根目录。

## 注意事项

- 若需要查看项目架构与接口总览，优先看 [docs/overview/PROJECT_CONTEXT.md](docs/overview/PROJECT_CONTEXT.md)。
- 若需要排查任务调度、Beat/Worker 问题，优先看 [docs/ops/SCHEDULER_README.md](docs/ops/SCHEDULER_README.md)。
- 若需要查看 Agent/Golang 侧实现，优先看 [docs/architecture/DJ_AGENT_ARCHITECTURE.md](docs/architecture/DJ_AGENT_ARCHITECTURE.md)。